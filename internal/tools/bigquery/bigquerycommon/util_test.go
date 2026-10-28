// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigquerycommon_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerycommon"
	"google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

type mockDatasetValidator struct{}

func (m mockDatasetValidator) IsDatasetAllowed(projectID, datasetID string) bool {
	if projectID == "google.com:corp-proj" && datasetID == "allowed_ds" {
		return true
	}
	return projectID == "proj" && datasetID == "allowed_ds"
}

func TestValidateQueryAgainstAllowedDatasets(t *testing.T) {
	testCases := []struct {
		name             string
		sql              string
		referencedTables []*bigquery.TableReference
		ddlTargetTable   *bigquery.TableReference
		statementType    string
		opts             []bigquerycommon.ValidateOption
		wantErr          bool
		wantErrSubs      []string
	}{
		{
			name: "authorized view allowed",
			sql:  "SELECT * FROM proj.allowed_ds.v",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "forbidden_ds", TableId: "base"},
			},
			statementType: "SELECT",
			wantErr:       false,
		},
		{
			name: "explicit reference forbidden",
			sql:  "SELECT * FROM proj.forbidden_ds.base",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "forbidden_ds", TableId: "base"},
			},
			statementType: "SELECT",
			wantErr:       true,
			wantErrSubs:   []string{"access to dataset 'proj.forbidden_ds' is not allowed"},
		},
		{
			name: "domain-scoped project explicit reference forbidden",
			sql:  "SELECT * FROM `google.com:corp-proj.forbidden_ds.base`",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "google.com:corp-proj", DatasetId: "forbidden_ds", TableId: "base"},
			},
			statementType: "SELECT",
			wantErr:       true,
			wantErrSubs:   []string{"access to dataset 'google.com:corp-proj.forbidden_ds' is not allowed"},
		},
		{
			name: "domain-scoped project authorized view allowed",
			sql:  "SELECT * FROM `google.com:corp-proj.allowed_ds.v`",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "google.com:corp-proj", DatasetId: "forbidden_ds", TableId: "base"},
			},
			statementType: "SELECT",
			wantErr:       false,
		},
		{
			// Note: SQL with wildcards like `events_*` does not match literal table references like
			// `events_20240101` in IsAnyTableExplicitlyReferenced, but is caught by TableParser
			// checking `proj.forbidden_ds.events_*` against the allowed dataset list.
			name: "wildcard reference forbidden",
			sql:  "SELECT * FROM `proj.forbidden_ds.events_*`",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "forbidden_ds", TableId: "events_20240101"},
			},
			statementType: "SELECT",
			wantErr:       true,
			wantErrSubs:   []string{"access to dataset 'proj.forbidden_ds' is not allowed"},
		},
		{
			name: "CTE prefix attempt does not bypass forbidden dataset",
			sql:  "WITH forbidden_ds.dummy AS (SELECT 1) SELECT * FROM `proj.forbidden_ds.events_*`",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "forbidden_ds", TableId: "events_20240101"},
			},
			statementType: "SELECT",
			wantErr:       true,
			wantErrSubs:   []string{"access to dataset 'proj.forbidden_ds' is not allowed"},
		},
		{
			name: "unqualified table name forbidden when dry run references disallowed dataset",
			sql:  "SELECT * FROM secret_table",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "forbidden_ds", TableId: "secret_table"},
			},
			statementType: "SELECT",
			wantErr:       true,
			wantErrSubs:   []string{"query references table \"secret_table\" without a dataset qualifier"},
		},
		{
			name: "unqualified table name allowed when dry run references allowed dataset",
			sql:  "SELECT * FROM t",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "allowed_ds", TableId: "t"},
			},
			statementType: "SELECT",
			wantErr:       false,
		},
		{
			name: "mixed authorized view and unqualified table forbidden",
			sql:  "SELECT * FROM proj.allowed_ds.v JOIN t ON true",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "forbidden_ds", TableId: "base"},
				{ProjectId: "proj", DatasetId: "forbidden_ds", TableId: "t"},
			},
			statementType: "SELECT",
			wantErr:       true,
			wantErrSubs:   []string{"without a dataset qualifier"},
		},
		{
			name: "all allowed",
			sql:  "SELECT * FROM proj.allowed_ds.t",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "allowed_ds", TableId: "t"},
			},
			statementType: "SELECT",
			wantErr:       false,
		},
		{
			name: "multiple forbidden datasets",
			sql:  "SELECT * FROM proj.forbidden_a.t1 JOIN proj.forbidden_b.t2 ON true",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "forbidden_a", TableId: "t1"},
				{ProjectId: "proj", DatasetId: "forbidden_b", TableId: "t2"},
			},
			statementType: "SELECT",
			wantErr:       true,
			wantErrSubs:   []string{"is not allowed", "proj.forbidden_a", "proj.forbidden_b"},
		},
		{
			name:             "statement type gate",
			sql:              "CREATE TABLE FUNCTION ds.f() AS (SELECT 1)",
			referencedTables: nil,
			statementType:    "CREATE_TABLE_FUNCTION",
			wantErr:          true,
			wantErrSubs:      []string{"creating stored routines ('CREATE_TABLE_FUNCTION') is not allowed"},
		},
		{
			name:             "statement type gate CREATE_SCHEMA",
			sql:              "CREATE SCHEMA ds",
			referencedTables: nil,
			statementType:    "CREATE_SCHEMA",
			wantErr:          true,
			wantErrSubs:      []string{"dataset-level operations like 'CREATE_SCHEMA' are not allowed"},
		},
		{
			name:             "statement type gate CALL",
			sql:              "CALL ds.p()",
			referencedTables: nil,
			statementType:    "CALL",
			wantErr:          true,
			wantErrSubs:      []string{"calling stored procedures ('CALL') is not allowed"},
		},
		{
			name:             "statement type gate SET",
			sql:              "SET @@dataset_id = 'disallowed_ds'",
			referencedTables: nil,
			statementType:    "SET",
			wantErr:          true,
			wantErrSubs:      []string{"session variable assignment ('SET') is not allowed"},
		},
		{
			// The dry run accepts dynamic SQL as a SCRIPT, so only the deferred
			// lexical analysis error rejects it.
			name:          "EXECUTE IMMEDIATE rejected even when dry run succeeds",
			sql:           "EXECUTE IMMEDIATE 'SELECT * FROM proj.forbidden_ds.secret'",
			statementType: "SCRIPT",
			wantErr:       true,
			wantErrSubs:   []string{"EXECUTE IMMEDIATE is not allowed"},
		},
		{
			// A table named only inside a string literal is not an explicit reference, so
			// without the deferred lexical analysis error this would be exempted like an
			// authorized view.
			name: "EXECUTE IMMEDIATE rejected even when dry run reports the forbidden table",
			sql:  "EXECUTE IMMEDIATE 'SELECT * FROM proj.forbidden_ds.secret'",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "forbidden_ds", TableId: "secret"},
			},
			statementType: "SCRIPT",
			wantErr:       true,
			wantErrSubs:   []string{"EXECUTE IMMEDIATE is not allowed"},
		},
		{
			name:          "EXTERNAL_QUERY rejected even when dry run succeeds",
			sql:           "SELECT * FROM EXTERNAL_QUERY('proj.us.conn', 'SELECT * FROM secret')",
			statementType: "SELECT",
			wantErr:       true,
			wantErrSubs:   []string{"EXTERNAL_QUERY is not allowed"},
		},
		{
			name:          "region-level INFORMATION_SCHEMA rejected even when dry run succeeds",
			sql:           "SELECT * FROM `region-us`.INFORMATION_SCHEMA.SCHEMATA",
			statementType: "SELECT",
			wantErr:       true,
			wantErrSubs:   []string{"non-dataset-level INFORMATION_SCHEMA view \"SCHEMATA\" is not allowed"},
		},
		{
			name: "CREATE TEMP MODEL over allowed table with session temp object option",
			sql:  "CREATE TEMP MODEL contribution_analysis_model_123 OPTIONS(model_type = 'contribution_analysis') AS SELECT * FROM `proj.allowed_ds.t`",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "allowed_ds", TableId: "t"},
			},
			ddlTargetTable: &bigquery.TableReference{
				ProjectId: "proj",
				DatasetId: "_anon_session_ds",
				TableId:   "contribution_analysis_model_123",
			},
			statementType: "CREATE_MODEL",
			opts: []bigquerycommon.ValidateOption{
				bigquerycommon.WithSessionTempObject("contribution_analysis_model_123"),
			},
			wantErr: false,
		},
		{
			name: "CREATE TEMP MODEL over authorized view with session temp object option",
			sql:  "CREATE TEMP MODEL contribution_analysis_model_123 OPTIONS(model_type = 'contribution_analysis') AS (SELECT * FROM `proj.allowed_ds.v`)",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "forbidden_ds", TableId: "base"},
			},
			ddlTargetTable: &bigquery.TableReference{
				ProjectId: "proj",
				DatasetId: "_anon_session_ds",
				TableId:   "contribution_analysis_model_123",
			},
			statementType: "CREATE_MODEL",
			opts: []bigquerycommon.ValidateOption{
				bigquerycommon.WithSessionTempObject("contribution_analysis_model_123"),
			},
			wantErr: false,
		},
		{
			name: "CREATE TEMP MODEL without session temp object option fails on unqualified model name",
			sql:  "CREATE TEMP MODEL contribution_analysis_model_123 OPTIONS(model_type = 'contribution_analysis') AS SELECT * FROM `proj.allowed_ds.t`",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "allowed_ds", TableId: "t"},
			},
			ddlTargetTable: &bigquery.TableReference{
				ProjectId: "proj",
				DatasetId: "_anon_session_ds",
				TableId:   "contribution_analysis_model_123",
			},
			statementType: "CREATE_MODEL",
			wantErr:       true,
			wantErrSubs:   []string{"query references table \"contribution_analysis_model_123\" without a dataset qualifier"},
		},
		{
			name: "CREATE TEMP MODEL with session temp object option still rejects unqualified table in body",
			sql:  "CREATE TEMP MODEL contribution_analysis_model_123 OPTIONS(model_type = 'contribution_analysis') AS (SELECT * FROM secret_table)",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "forbidden_ds", TableId: "secret_table"},
			},
			ddlTargetTable: &bigquery.TableReference{
				ProjectId: "proj",
				DatasetId: "_anon_session_ds",
				TableId:   "contribution_analysis_model_123",
			},
			statementType: "CREATE_MODEL",
			opts: []bigquerycommon.ValidateOption{
				bigquerycommon.WithSessionTempObject("contribution_analysis_model_123"),
			},
			wantErr:     true,
			wantErrSubs: []string{"query references table \"secret_table\" without a dataset qualifier"},
		},
		{
			name: "CREATE TEMP MODEL with session temp object option still rejects forbidden dataset in body",
			sql:  "CREATE TEMP MODEL contribution_analysis_model_123 OPTIONS(model_type = 'contribution_analysis') AS SELECT * FROM `proj.forbidden_ds.t`",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "forbidden_ds", TableId: "t"},
			},
			ddlTargetTable: &bigquery.TableReference{
				ProjectId: "proj",
				DatasetId: "_anon_session_ds",
				TableId:   "contribution_analysis_model_123",
			},
			statementType: "CREATE_MODEL",
			opts: []bigquerycommon.ValidateOption{
				bigquerycommon.WithSessionTempObject("contribution_analysis_model_123"),
			},
			wantErr:     true,
			wantErrSubs: []string{"access to dataset 'proj.forbidden_ds' is not allowed"},
		},
		{
			name: "CREATE TEMP MODEL with mismatched session temp object option fails",
			sql:  "CREATE TEMP MODEL contribution_analysis_model_123 OPTIONS(model_type = 'contribution_analysis') AS SELECT * FROM `proj.allowed_ds.t`",
			referencedTables: []*bigquery.TableReference{
				{ProjectId: "proj", DatasetId: "allowed_ds", TableId: "t"},
			},
			ddlTargetTable: &bigquery.TableReference{
				ProjectId: "proj",
				DatasetId: "_anon_session_ds",
				TableId:   "contribution_analysis_model_123",
			},
			statementType: "CREATE_MODEL",
			opts: []bigquerycommon.ValidateOption{
				bigquerycommon.WithSessionTempObject("contribution_analysis_model_other"),
			},
			wantErr:     true,
			wantErrSubs: []string{"query references table \"contribution_analysis_model_123\" without a dataset qualifier"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				job := &bigquery.Job{
					Statistics: &bigquery.JobStatistics{
						Query: &bigquery.JobStatistics2{
							ReferencedTables: tc.referencedTables,
							DdlTargetTable:   tc.ddlTargetTable,
							StatementType:    tc.statementType,
						},
					},
				}
				if err := json.NewEncoder(w).Encode(job); err != nil {
					t.Errorf("failed to encode mock response: %v", err)
				}
			}))
			defer server.Close()

			restService, err := bigquery.NewService(context.Background(), option.WithoutAuthentication(), option.WithEndpoint(server.URL))
			if err != nil {
				t.Fatalf("failed to create bigquery service: %v", err)
			}

			_, errToolbox := bigquerycommon.ValidateQueryAgainstAllowedDatasets(
				context.Background(),
				restService,
				"proj",
				"US",
				tc.sql,
				nil,
				nil,
				mockDatasetValidator{},
				0,
				false,
				tc.opts...,
			)

			if tc.wantErr {
				if errToolbox == nil {
					t.Fatalf("expected error containing %v, got nil", tc.wantErrSubs)
				}
				for _, sub := range tc.wantErrSubs {
					if !strings.Contains(errToolbox.Error(), sub) {
						t.Errorf("error = %q, want it to contain %q", errToolbox.Error(), sub)
					}
				}
			} else if errToolbox != nil {
				t.Fatalf("expected no error, got %v", errToolbox)
			}
		})
	}
}

func TestValidateQueryAgainstAllowedDatasetsBeforeDryRun(t *testing.T) {
	testCases := []struct {
		name          string
		projectID     string
		sql           string
		wantDryRun    bool
		wantErrSubs   []string
		unwantedInErr string
	}{
		{
			name:          "forbidden dataset surfaces allowlist error before dry run 404",
			projectID:     "proj",
			sql:           "SELECT * FROM forbidden_ds.no_such_table",
			wantDryRun:    false,
			wantErrSubs:   []string{"access to dataset 'proj.forbidden_ds' is not allowed"},
			unwantedInErr: "no_such_table",
		},
		{
			name:          "domain-scoped default project forbidden dataset surfaces allowlist error before dry run 404",
			projectID:     "google.com:corp-proj",
			sql:           "SELECT * FROM forbidden_ds.no_such_table",
			wantDryRun:    false,
			wantErrSubs:   []string{"access to dataset 'google.com:corp-proj.forbidden_ds' is not allowed"},
			unwantedInErr: "no_such_table",
		},
		{
			name:          "allowed dataset still surfaces dry run 404",
			projectID:     "proj",
			sql:           "SELECT * FROM allowed_ds.no_such_table",
			wantDryRun:    true,
			wantErrSubs:   []string{"no_such_table"},
			unwantedInErr: "is not allowed",
		},
		{
			name:          "restricted statement on non-existent object surfaces restriction error instead of 404",
			projectID:     "proj",
			sql:           "CALL forbidden_ds.no_such_proc()",
			wantDryRun:    true,
			wantErrSubs:   []string{"CALL is not allowed when dataset restrictions are in place"},
			unwantedInErr: "no_such_table",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dryRunCalled := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dryRunCalled = true
				http.Error(w, "Not found: Table no_such_table", http.StatusNotFound)
			}))
			defer server.Close()

			restService, err := bigquery.NewService(context.Background(), option.WithoutAuthentication(), option.WithEndpoint(server.URL))
			if err != nil {
				t.Fatalf("failed to create bigquery service: %v", err)
			}

			_, errToolbox := bigquerycommon.ValidateQueryAgainstAllowedDatasets(
				context.Background(),
				restService,
				tc.projectID,
				"US",
				tc.sql,
				nil,
				nil,
				mockDatasetValidator{},
				0,
				false,
			)
			if errToolbox == nil {
				t.Fatalf("expected error containing %v, got nil", tc.wantErrSubs)
			}
			for _, sub := range tc.wantErrSubs {
				if !strings.Contains(errToolbox.Error(), sub) {
					t.Errorf("error = %q, want it to contain %q", errToolbox.Error(), sub)
				}
			}
			if tc.unwantedInErr != "" && strings.Contains(errToolbox.Error(), tc.unwantedInErr) {
				t.Errorf("error = %q, want it NOT to contain %q", errToolbox.Error(), tc.unwantedInErr)
			}
			if dryRunCalled != tc.wantDryRun {
				t.Errorf("dryRunCalled = %v, want %v", dryRunCalled, tc.wantDryRun)
			}
		})
	}
}
