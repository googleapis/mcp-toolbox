// Copyright 2025 Google LLC
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

package bigqueryexecutesql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	bigqueryds "github.com/googleapis/mcp-toolbox/internal/sources/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	bqutil "github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerycommon"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigqueryexecutesql"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	bigqueryrestapi "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

func TestParseFromYamlBigQueryExecuteSql(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		want server.ToolConfigs
	}{
		{
			desc: "basic example",
			in: `
            kind: tool
            name: example_tool
            type: bigquery-execute-sql
            source: my-instance
            description: some description
            `,
			want: server.ToolConfigs{
				"example_tool": bigqueryexecutesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "bigquery-execute-sql",
					Source: "my-instance",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}

func TestInvokeDatasetRestrictions(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/jobs") {
			var body struct {
				Configuration struct {
					DryRun bool `json:"dryRun"`
					Query  struct {
						Query string `json:"query"`
					} `json:"query"`
				} `json:"configuration"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if body.Configuration.DryRun {
				var referencedTables []map[string]any
				query := body.Configuration.Query.Query
				if strings.Contains(query, "allowed_dataset.my_table") {
					referencedTables = append(referencedTables, map[string]any{
						"projectId": "test-project",
						"datasetId": "allowed_dataset",
						"tableId":   "my_table",
					})
				}
				if strings.Contains(query, "forbidden_dataset.my_table") {
					referencedTables = append(referencedTables, map[string]any{
						"projectId": "test-project",
						"datasetId": "forbidden_dataset",
						"tableId":   "my_table",
					})
				}

				resp := map[string]any{
					"kind": "bigquery#job",
					"jobReference": map[string]string{
						"projectId": "test-project",
						"jobId":     "mock-job-id",
					},
					"status": map[string]any{
						"state": "DONE",
					},
					"configuration": map[string]any{
						"query": map[string]any{
							"query": query,
						},
					},
					"statistics": map[string]any{
						"creationTime": "123456789",
						"startTime":    "123456789",
						"endTime":      "123456789",
						"query": map[string]any{
							"statementType":    "SELECT",
							"referencedTables": referencedTables,
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
		}

		http.Error(w, "not implemented", http.StatusNotFound)
	}))
	defer mockServer.Close()

	var outBuf, errBuf bytes.Buffer
	logger, err := log.NewStdLogger(&outBuf, &errBuf, "debug")
	if err != nil {
		t.Fatalf("failed to create debug logger: %v", err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	bqClient, err := bigqueryapi.NewClient(ctx, "test-project", option.WithEndpoint(mockServer.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("failed to create mocked BigQuery client: %v", err)
	}

	restService, err := bigqueryrestapi.NewService(ctx, option.WithEndpoint(mockServer.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("failed to create mocked BigQuery REST service: %v", err)
	}

	testSrc := &bqutil.MockSource{
		Client:          bqClient,
		Service:         restService,
		AllowedDatasets: []string{"test-project.allowed_dataset"},
	}

	cfg := bigqueryexecutesql.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "execute_sql_tool",
			Description: "Execute SQL",
		},
		Type:   "bigquery-execute-sql",
		Source: "my-bq-source",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	executeSqlTool, ok := tool.(bigqueryexecutesql.Tool)
	if !ok {
		t.Fatalf("expected bigqueryexecutesql.Tool, got %T", tool)
	}

	tcs := []struct {
		desc    string
		sql     string
		wantErr bool
		wantSub string
	}{
		{
			desc:    "simple query without tables",
			sql:     "SELECT 1 + 1",
			wantErr: false,
		},
		{
			desc:    "querying allowed dataset table",
			sql:     "SELECT * FROM allowed_dataset.my_table",
			wantErr: false,
		},
		{
			desc:    "querying forbidden dataset table",
			sql:     "SELECT * FROM forbidden_dataset.my_table",
			wantErr: true,
			wantSub: "access to dataset 'test-project.forbidden_dataset' is not allowed",
		},
		{
			desc:    "querying allowed dataset INFORMATION_SCHEMA tables",
			sql:     "SELECT * FROM allowed_dataset.INFORMATION_SCHEMA.TABLES",
			wantErr: false,
		},
		{
			desc:    "querying forbidden dataset INFORMATION_SCHEMA tables",
			sql:     "SELECT * FROM forbidden_dataset.INFORMATION_SCHEMA.TABLES",
			wantErr: true,
			wantSub: "access to dataset 'test-project.forbidden_dataset' is not allowed",
		},
		{
			desc:    "querying regional INFORMATION_SCHEMA schemata",
			sql:     "SELECT * FROM region-us.INFORMATION_SCHEMA.SCHEMATA",
			wantErr: true,
			wantSub: "querying non-dataset-level INFORMATION_SCHEMA view \"SCHEMATA\" is not allowed when dataset restrictions are in place",
		},
		{
			desc:    "querying project-level INFORMATION_SCHEMA schemata",
			sql:     "SELECT * FROM INFORMATION_SCHEMA.SCHEMATA",
			wantErr: true,
			wantSub: "querying non-dataset-level INFORMATION_SCHEMA view \"SCHEMATA\" is not allowed when dataset restrictions are in place",
		},
		{
			desc:    "querying mixed allowed table and forbidden INFORMATION_SCHEMA view",
			sql:     "SELECT * FROM allowed_dataset.my_table JOIN forbidden_dataset.INFORMATION_SCHEMA.TABLES ON true",
			wantErr: true,
			wantSub: "access to dataset 'test-project.forbidden_dataset' is not allowed",
		},
		{
			desc:    "querying EXTERNAL_QUERY",
			sql:     "SELECT * FROM EXTERNAL_QUERY('my-connection', 'SELECT 1')",
			wantErr: true,
			wantSub: "EXTERNAL_QUERY is not allowed when dataset restrictions are in place",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			outBuf.Reset()
			data := map[string]any{
				"sql":     tc.sql,
				"dry_run": true,
			}

			params, err := executeSqlTool.GetParameters(testSrc)
			if err != nil {
				t.Fatalf("failed to get parameters: %v", err)
			}
			paramVals, err := parameters.ParseParams(params, data, nil)
			if err != nil {
				t.Fatalf("unexpected error parsing parameters: %v", err)
			}

			_, err = tool.Invoke(ctx, testSrc, paramVals, "")
			wantLog := "executing `bigquery-execute-sql` tool query: " + tc.sql
			if !strings.Contains(outBuf.String(), wantLog) {
				t.Errorf("expected debug log to contain %q, got %q", wantLog, outBuf.String())
			}
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantSub) {
					t.Errorf("expected error to contain %q, got %v", tc.wantSub, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetAnnotations(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	boolPtr := func(b bool) *bool { return &b }
	readOnlySrc := &bigqueryds.Source{Config: bigqueryds.Config{WriteMode: bigqueryds.WriteModeBlocked}}
	readWriteAllowedSrc := &bigqueryds.Source{Config: bigqueryds.Config{WriteMode: bigqueryds.WriteModeAllowed}}
	readOnlyProtectedSrc := &bigqueryds.Source{Config: bigqueryds.Config{WriteMode: bigqueryds.WriteModeProtected}}
	mockReadOnlySrc := &testutils.MockSource{MockSourceConfig: testutils.MockSourceConfig{ReadOnly: true}}
	mockReadWriteSrc := &testutils.MockSource{MockSourceConfig: testutils.MockSourceConfig{ReadOnly: false}}

	tests := []struct {
		desc        string
		src         sources.Source
		annotations *tools.ToolAnnotations
		want        *tools.ToolAnnotations
	}{
		{
			desc: "nil source returns default destructive annotations unmodified",
			src:  nil,
			want: tools.NewDestructiveAnnotations(),
		},
		{
			desc: "read-write source (WriteModeAllowed) returns default destructive annotations unmodified",
			src:  readWriteAllowedSrc,
			want: tools.NewDestructiveAnnotations(),
		},
		{
			desc: "read-only source (WriteModeProtected) dynamically flips default destructive annotations to read-only",
			src:  readOnlyProtectedSrc,
			want: tools.NewReadOnlyAnnotations(),
		},
		{
			desc: "read-write mock source returns default destructive annotations unmodified",
			src:  mockReadWriteSrc,
			want: tools.NewDestructiveAnnotations(),
		},
		{
			desc: "read-only source (WriteModeBlocked) dynamically flips default destructive annotations to read-only",
			src:  readOnlySrc,
			want: tools.NewReadOnlyAnnotations(),
		},
		{
			desc: "read-only mock source dynamically flips default destructive annotations to read-only",
			src:  mockReadOnlySrc,
			want: tools.NewReadOnlyAnnotations(),
		},
		{
			desc:        "read-only source with explicit read-only base remains read-only",
			src:         readOnlySrc,
			annotations: tools.NewReadOnlyAnnotations(),
			want:        tools.NewReadOnlyAnnotations(),
		},
		{
			desc: "read-only source preserves custom hints (idempotent, openWorld) when flipped",
			src:  readOnlySrc,
			annotations: &tools.ToolAnnotations{
				DestructiveHint: boolPtr(true),
				ReadOnlyHint:    boolPtr(false),
				IdempotentHint:  boolPtr(true),
				OpenWorldHint:   boolPtr(true),
			},
			want: &tools.ToolAnnotations{
				ReadOnlyHint:   boolPtr(true),
				IdempotentHint: boolPtr(true),
				OpenWorldHint:  boolPtr(true),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := bigqueryexecutesql.Config{
				ConfigBase: tools.ConfigBase{Name: "bigquery-execute-sql", Description: "execute sql query", Annotations: tc.annotations},
				Type:       "bigquery-execute-sql",
				Source:     "my-instance",
			}
			tool, err := cfg.Initialize(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			got := tool.GetAnnotations(tc.src)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("GetAnnotations() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetParameters_AllowedDatasetsDeterministicOrder(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	cfg := bigqueryexecutesql.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "execute_sql_tool",
			Description: "Execute SQL",
		},
		Type:   "bigquery-execute-sql",
		Source: "my-bq-source",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	src := &bigqueryds.Source{
		Config: bigqueryds.Config{
			WriteMode: bigqueryds.WriteModeBlocked,
		},
		AllowedDatasets: map[string]struct{}{
			"my-project.gamma": {},
			"my-project.alpha": {},
			"my-project.beta":  {},
		},
	}

	wantSuffix := "The query must only access datasets from the following list: `my-project.alpha`, `my-project.beta`, `my-project.gamma`."

	for i := 0; i < 20; i++ {
		params, err := tool.GetParameters(src)
		if err != nil {
			t.Fatalf("iteration %d: GetParameters() unexpected error: %v", i, err)
		}
		var sqlParamDesc string
		for _, p := range params {
			if p.GetName() == "sql" {
				sqlParamDesc = p.GetDesc()
				break
			}
		}
		if !strings.HasSuffix(sqlParamDesc, wantSuffix) {
			t.Fatalf("iteration %d: GetParameters() sql description = %q, want suffix %q", i, sqlParamDesc, wantSuffix)
		}

		manifest, err := tool.Manifest(src)
		if err != nil {
			t.Fatalf("iteration %d: Manifest() unexpected error: %v", i, err)
		}
		var sqlManifestDesc string
		for _, p := range manifest.Parameters {
			if p.Name == "sql" {
				sqlManifestDesc = p.Description
				break
			}
		}
		if !strings.HasSuffix(sqlManifestDesc, wantSuffix) {
			t.Fatalf("iteration %d: Manifest() sql description = %q, want suffix %q", i, sqlManifestDesc, wantSuffix)
		}
	}
}

type customSessionMockSource struct {
	*bqutil.MockSource
	sessionProvider bigqueryds.BigQuerySessionProvider
}

func (c *customSessionMockSource) BigQuerySession() bigqueryds.BigQuerySessionProvider {
	return c.sessionProvider
}

func TestInvokeWriteModeProtected(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("failed to create context with logger: %v", err)
	}

	cfg := bigqueryexecutesql.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "execute_sql_protected",
			Description: "Execute SQL in protected mode",
		},
		Type:   "bigquery-execute-sql",
		Source: "my-bq-source",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	executeSqlTool := tool.(bigqueryexecutesql.Tool)

	t.Run("broken or missing session metadata returns 500 without dry run or execution", func(t *testing.T) {
		dryRunHits := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dryRunHits++
			http.Error(w, "unexpected call", http.StatusInternalServerError)
		}))
		defer srv.Close()

		bqClient, err := bigqueryapi.NewClient(ctx, "test-project", option.WithEndpoint(srv.URL), option.WithoutAuthentication())
		if err != nil {
			t.Fatalf("failed to create BigQuery client: %v", err)
		}
		restService, err := bigqueryrestapi.NewService(ctx, option.WithEndpoint(srv.URL), option.WithoutAuthentication())
		if err != nil {
			t.Fatalf("failed to create BigQuery REST service: %v", err)
		}

		sessionCases := []struct {
			name     string
			provider bigqueryds.BigQuerySessionProvider
		}{
			{
				name:     "nil session provider",
				provider: nil,
			},
			{
				name: "nil session returned",
				provider: func(context.Context) (*bigqueryds.Session, error) {
					return nil, nil
				},
			},
			{
				name: "empty session DatasetID",
				provider: func(context.Context) (*bigqueryds.Session, error) {
					return &bigqueryds.Session{ID: "sess-1", ProjectID: "test-project", DatasetID: ""}, nil
				},
			},
			{
				name: "empty session ID",
				provider: func(context.Context) (*bigqueryds.Session, error) {
					return &bigqueryds.Session{ID: "", ProjectID: "test-project", DatasetID: "_anon_session_ds"}, nil
				},
			},
		}

		for _, sc := range sessionCases {
			t.Run(sc.name, func(t *testing.T) {
				baseMock := &bqutil.MockSource{
					Client:    bqClient,
					Service:   restService,
					WriteMode: bigqueryds.WriteModeProtected,
				}
				src := &customSessionMockSource{
					MockSource:      baseMock,
					sessionProvider: sc.provider,
				}
				params, err := executeSqlTool.GetParameters(src)
				if err != nil {
					t.Fatalf("GetParameters failed: %v", err)
				}
				paramVals, err := parameters.ParseParams(params, map[string]any{"sql": "SELECT 1"}, nil)
				if err != nil {
					t.Fatalf("ParseParams failed: %v", err)
				}

				_, invokeErr := tool.Invoke(ctx, src, paramVals, "")
				if invokeErr == nil {
					t.Fatalf("expected 500 error, got nil")
				}
				var srvErr *util.ClientServerError
				if !errors.As(invokeErr, &srvErr) || srvErr.Code != http.StatusInternalServerError {
					t.Fatalf("expected ClientServerError 500, got %T (%v)", invokeErr, invokeErr)
				}
				if baseMock.CalledSQL != "" {
					t.Fatalf("expected RunSQL not to be called, got %q", baseMock.CalledSQL)
				}
				if dryRunHits != 0 {
					t.Fatalf("expected 0 dry-run HTTP calls, got %d", dryRunHits)
				}
			})
		}
	})

	sessionRef := &bigqueryds.Session{
		ID:        "sess-123",
		ProjectID: "test-project",
		DatasetID: "_anon_session_ds",
	}

	type testCase struct {
		name                string
		sql                 string
		dryRun              bool
		allowedDatasets     []string
		statementType       string
		configDest          *bigqueryrestapi.TableReference
		ddlTargetTable      *bigqueryrestapi.TableReference
		ddlDestinationTable *bigqueryrestapi.TableReference
		ddlTargetDataset    *bigqueryrestapi.DatasetReference
		ddlTargetRoutine    *bigqueryrestapi.RoutineReference
		ddlTargetRAP        *bigqueryrestapi.RowAccessPolicyReference
		referencedTables    []*bigqueryrestapi.TableReference
		wantErr             bool
		wantErrSub          string
	}

	sessionDest := &bigqueryrestapi.TableReference{
		ProjectId: "test-project",
		DatasetId: "_anon_session_ds",
		TableId:   "my_shared_temp_table",
	}
	permDest := &bigqueryrestapi.TableReference{
		ProjectId: "test-project",
		DatasetId: "perm_ds",
		TableId:   "my_table",
	}

	testCases := []testCase{
		// Allowed SELECT cases
		{name: "SELECT with nil DestinationTable allowed", sql: "SELECT * FROM perm_ds.my_table", statementType: "SELECT", referencedTables: []*bigqueryrestapi.TableReference{permDest}},
		{name: "SELECT with session DestinationTable allowed", sql: "SELECT * FROM perm_ds.my_table", statementType: "SELECT", configDest: sessionDest},

		// Allowed DDL and DML targeting session dataset
		{name: "CREATE TEMP TABLE allowed", sql: "CREATE TEMP TABLE my_shared_temp_table (x INT64)", statementType: "CREATE_TABLE", configDest: sessionDest, ddlTargetTable: sessionDest},
		{name: "CREATE TEMP TABLE AS SELECT allowed", sql: "CREATE TEMP TABLE my_shared_temp_table AS SELECT 1 AS x", statementType: "CREATE_TABLE_AS_SELECT", ddlTargetTable: sessionDest},
		{name: "ALTER TABLE in session dataset allowed", sql: "ALTER TABLE _SESSION.my_shared_temp_table ADD COLUMN y STRING", statementType: "ALTER_TABLE", configDest: sessionDest, ddlTargetTable: sessionDest},
		{name: "DROP TABLE in session dataset allowed", sql: "DROP TABLE _SESSION.my_shared_temp_table", statementType: "DROP_TABLE", configDest: sessionDest, ddlTargetTable: sessionDest},
		{name: "CREATE TEMP MODEL allowed", sql: "CREATE TEMP MODEL m OPTIONS(model_type='linear_reg') AS SELECT 1 AS label, 2 AS f", statementType: "CREATE_MODEL", ddlTargetTable: sessionDest},
		{name: "INSERT into session temp table reading permanent table allowed", sql: "INSERT INTO my_shared_temp_table SELECT x FROM perm_ds.my_table", statementType: "INSERT", configDest: sessionDest, referencedTables: []*bigqueryrestapi.TableReference{sessionDest, permDest}},
		{name: "UPDATE session temp table allowed", sql: "UPDATE _SESSION.my_shared_temp_table SET x = 2 WHERE true", statementType: "UPDATE", configDest: sessionDest},
		{name: "DELETE from session temp table allowed", sql: "DELETE FROM my_shared_temp_table WHERE true", statementType: "DELETE", configDest: sessionDest},
		{name: "MERGE into session temp table allowed", sql: "MERGE INTO my_shared_temp_table T USING perm_ds.my_table S ON T.x = S.x WHEN MATCHED THEN DELETE", statementType: "MERGE", configDest: sessionDest},
		{name: "TRUNCATE TABLE session temp table with case-insensitive project allowed", sql: "TRUNCATE TABLE my_shared_temp_table", statementType: "TRUNCATE_TABLE", configDest: &bigqueryrestapi.TableReference{ProjectId: "TEST-PROJECT", DatasetId: "_anon_session_ds", TableId: "my_shared_temp_table"}},

		// Blocked DDL & DML with explicit non-session destination
		{name: "CREATE TABLE in permanent dataset blocked", sql: "CREATE TABLE perm_ds.my_table (x INT64)", statementType: "CREATE_TABLE", configDest: permDest, ddlTargetTable: permDest, wantErr: true, wantErrSub: `destination was "perm_ds"`},
		{name: "ALTER TABLE RENAME to permanent dataset blocked", sql: "ALTER TABLE _SESSION.my_shared_temp_table RENAME TO perm_t", statementType: "ALTER_TABLE", ddlTargetTable: sessionDest, ddlDestinationTable: permDest, wantErr: true, wantErrSub: `destination was "perm_ds"`},
		{name: "DROP TABLE in permanent dataset blocked", sql: "DROP TABLE perm_ds.my_table", statementType: "DROP_TABLE", ddlTargetTable: permDest, wantErr: true, wantErrSub: `destination was "perm_ds"`},
		{name: "CREATE SCHEMA in project blocked by DdlTargetDataset", sql: "CREATE SCHEMA perm_ds", statementType: "CREATE_SCHEMA", ddlTargetDataset: &bigqueryrestapi.DatasetReference{ProjectId: "test-project", DatasetId: "perm_ds"}, wantErr: true, wantErrSub: `destination was "perm_ds"`},
		{name: "CREATE FUNCTION in permanent dataset blocked by DdlTargetRoutine", sql: "CREATE FUNCTION perm_ds.f() RETURNS INT64 AS (1)", statementType: "CREATE_FUNCTION", ddlTargetRoutine: &bigqueryrestapi.RoutineReference{ProjectId: "test-project", DatasetId: "perm_ds", RoutineId: "f"}, wantErr: true, wantErrSub: `destination was "perm_ds"`},
		{name: "CREATE ROW ACCESS POLICY in permanent dataset blocked by DdlTargetRowAccessPolicy", sql: "CREATE ROW ACCESS POLICY p ON perm_ds.my_table GRANT TO ('user:a@b.com') FILTER USING (true)", statementType: "CREATE_ROW_ACCESS_POLICY", ddlTargetRAP: &bigqueryrestapi.RowAccessPolicyReference{ProjectId: "test-project", DatasetId: "perm_ds", TableId: "my_table", PolicyId: "p"}, wantErr: true, wantErrSub: `destination was "perm_ds"`},
		{name: "INSERT into permanent table blocked by DestinationTable", sql: "INSERT INTO perm_ds.my_table (x) VALUES (1)", statementType: "INSERT", configDest: permDest, wantErr: true, wantErrSub: `destination was "perm_ds"`},
		{name: "UPDATE permanent table blocked by DestinationTable", sql: "UPDATE perm_ds.my_table SET x = 2 WHERE true", statementType: "UPDATE", configDest: permDest, wantErr: true, wantErrSub: `destination was "perm_ds"`},
		{name: "DELETE from permanent table blocked by DestinationTable", sql: "DELETE FROM perm_ds.my_table WHERE true", statementType: "DELETE", configDest: permDest, wantErr: true, wantErrSub: `destination was "perm_ds"`},
		{name: "MERGE into permanent table blocked by DestinationTable", sql: "MERGE INTO perm_ds.my_table T USING _SESSION.temp_t S ON T.x = S.x WHEN NOT MATCHED THEN INSERT ROW", statementType: "MERGE", configDest: permDest, wantErr: true, wantErrSub: `destination was "perm_ds"`},
		{name: "TRUNCATE TABLE permanent table blocked by DestinationTable", sql: "TRUNCATE TABLE perm_ds.my_table", statementType: "TRUNCATE_TABLE", configDest: permDest, wantErr: true, wantErrSub: `destination was "perm_ds"`},
		{name: "dry_run true also rejects permanent DML", sql: "DELETE FROM perm_ds.my_table WHERE true", dryRun: true, statementType: "DELETE", configDest: permDest, wantErr: true, wantErrSub: `destination was "perm_ds"`},
		{name: "cross-project target with identical session DatasetID blocked", sql: "DELETE FROM `evil-project._anon_session_ds.my_table` WHERE true", statementType: "DELETE", configDest: &bigqueryrestapi.TableReference{ProjectId: "evil-project", DatasetId: "_anon_session_ds", TableId: "my_table"}, wantErr: true, wantErrSub: `destination was "evil-project._anon_session_ds"`},
		{name: "combined protected writeMode and allowedDatasets still blocks permanent DML", sql: "DELETE FROM perm_ds.my_table WHERE true", allowedDatasets: []string{"test-project.perm_ds"}, statementType: "DELETE", configDest: permDest, referencedTables: []*bigqueryrestapi.TableReference{permDest}, wantErr: true, wantErrSub: `protected write mode only supports SELECT statements, or write operations in the anonymous dataset of a BigQuery session, but destination was "perm_ds"`},

		// Fail-closed cases when DestinationTable and DdlTargetTable are nil on non-SELECT statements
		{name: "DELETE with nil DestinationTable fails closed", sql: "DELETE FROM perm_ds.my_table WHERE true", statementType: "DELETE", wantErr: true, wantErrSub: `statement type was "DELETE"`},
		{name: "UPDATE with nil DestinationTable fails closed", sql: "UPDATE perm_ds.my_table SET x = 1 WHERE true", statementType: "UPDATE", wantErr: true, wantErrSub: `statement type was "UPDATE"`},
		{name: "INSERT with nil DestinationTable fails closed", sql: "INSERT INTO perm_ds.my_table VALUES (1)", statementType: "INSERT", wantErr: true, wantErrSub: `statement type was "INSERT"`},
		{name: "MERGE with nil DestinationTable fails closed", sql: "MERGE INTO perm_ds.my_table T USING perm_ds.s S ON true WHEN MATCHED THEN DELETE", statementType: "MERGE", wantErr: true, wantErrSub: `statement type was "MERGE"`},
		{name: "TRUNCATE_TABLE with nil DestinationTable fails closed", sql: "TRUNCATE TABLE perm_ds.my_table", statementType: "TRUNCATE_TABLE", wantErr: true, wantErrSub: `statement type was "TRUNCATE_TABLE"`},
		{name: "CREATE_TABLE with nil targets fails closed", sql: "CREATE TABLE perm_ds.t (x INT64)", statementType: "CREATE_TABLE", wantErr: true, wantErrSub: `statement type was "CREATE_TABLE"`},
		{name: "SCRIPT blocked", sql: "BEGIN DELETE FROM perm_ds.my_table WHERE true; END", statementType: "SCRIPT", wantErr: true, wantErrSub: `statement type was "SCRIPT"`},
		{name: "CALL blocked", sql: "CALL perm_ds.p()", statementType: "CALL", wantErr: true, wantErrSub: `statement type was "CALL"`},
		{name: "SET blocked", sql: "SET @@dataset_id = 'perm_ds'", statementType: "SET", wantErr: true, wantErrSub: `statement type was "SET"`},
		{name: "EXPORT_DATA blocked", sql: "EXPORT DATA OPTIONS(uri='gs://b/*') AS SELECT 1", statementType: "EXPORT_DATA", wantErr: true, wantErrSub: `statement type was "EXPORT_DATA"`},
		{name: "LOAD_DATA blocked", sql: "LOAD DATA INTO my_temp FROM FILES(uris=['gs://b/*'])", statementType: "LOAD_DATA", wantErr: true, wantErrSub: `statement type was "LOAD_DATA"`},
		{name: "CREATE_SCHEMA with nil target fails closed", sql: "CREATE SCHEMA new_ds", statementType: "CREATE_SCHEMA", wantErr: true, wantErrSub: `statement type was "CREATE_SCHEMA"`},
		{name: "DROP_SCHEMA with nil target fails closed", sql: "DROP SCHEMA perm_ds", statementType: "DROP_SCHEMA", wantErr: true, wantErrSub: `statement type was "DROP_SCHEMA"`},
		{name: "CREATE_PROCEDURE blocked", sql: "CREATE PROCEDURE perm_ds.p() BEGIN END", statementType: "CREATE_PROCEDURE", wantErr: true, wantErrSub: `statement type was "CREATE_PROCEDURE"`},
		{name: "GRANT blocked", sql: "GRANT `roles/bigquery.dataViewer` ON SCHEMA perm_ds TO 'user:a@b.com'", statementType: "GRANT", wantErr: true, wantErrSub: `statement type was "GRANT"`},
		{name: "empty statementType blocked", sql: "SELECT 1", statementType: "", wantErr: true, wantErrSub: `statement type was ""`},
		{name: "UNKNOWN_FUTURE_TYPE blocked", sql: "SELECT 1", statementType: "UNKNOWN_FUTURE_TYPE", wantErr: true, wantErrSub: `statement type was "UNKNOWN_FUTURE_TYPE"`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				job := &bigqueryrestapi.Job{
					Configuration: &bigqueryrestapi.JobConfiguration{
						Query: &bigqueryrestapi.JobConfigurationQuery{
							Query:            tc.sql,
							DestinationTable: tc.configDest,
						},
					},
					Statistics: &bigqueryrestapi.JobStatistics{
						Query: &bigqueryrestapi.JobStatistics2{
							StatementType:            tc.statementType,
							ReferencedTables:         tc.referencedTables,
							DdlTargetTable:           tc.ddlTargetTable,
							DdlDestinationTable:      tc.ddlDestinationTable,
							DdlTargetDataset:         tc.ddlTargetDataset,
							DdlTargetRoutine:         tc.ddlTargetRoutine,
							DdlTargetRowAccessPolicy: tc.ddlTargetRAP,
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(job)
			}))
			defer srv.Close()

			bqClient, err := bigqueryapi.NewClient(ctx, "test-project", option.WithEndpoint(srv.URL), option.WithoutAuthentication())
			if err != nil {
				t.Fatalf("failed to create BigQuery client: %v", err)
			}
			restService, err := bigqueryrestapi.NewService(ctx, option.WithEndpoint(srv.URL), option.WithoutAuthentication())
			if err != nil {
				t.Fatalf("failed to create BigQuery REST service: %v", err)
			}

			src := &bqutil.MockSource{
				Client:          bqClient,
				Service:         restService,
				WriteMode:       bigqueryds.WriteModeProtected,
				Session:         sessionRef,
				AllowedDatasets: tc.allowedDatasets,
				RunSQLResult:    "ok",
			}

			params, err := executeSqlTool.GetParameters(src)
			if err != nil {
				t.Fatalf("GetParameters failed: %v", err)
			}
			paramVals, err := parameters.ParseParams(params, map[string]any{
				"sql":     tc.sql,
				"dry_run": tc.dryRun,
			}, nil)
			if err != nil {
				t.Fatalf("ParseParams failed: %v", err)
			}

			_, invokeErr := tool.Invoke(ctx, src, paramVals, "")
			if tc.wantErr {
				if invokeErr == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrSub)
				}
				if !strings.Contains(invokeErr.Error(), tc.wantErrSub) {
					t.Fatalf("error = %q, want substring %q", invokeErr.Error(), tc.wantErrSub)
				}
				if src.CalledSQL != "" {
					t.Fatalf("expected RunSQL not to be called on rejected query, got %q", src.CalledSQL)
				}
				return
			}

			if invokeErr != nil {
				t.Fatalf("unexpected error: %v", invokeErr)
			}
			if !tc.dryRun && src.CalledSQL != tc.sql {
				t.Fatalf("CalledSQL = %q, want %q", src.CalledSQL, tc.sql)
			}
		})
	}
}

func TestInvokeWriteModeBlocked(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("failed to create context with logger: %v", err)
	}

	cfg := bigqueryexecutesql.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "execute_sql_blocked",
			Description: "Execute SQL in blocked mode",
		},
		Type:   "bigquery-execute-sql",
		Source: "my-bq-source",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	executeSqlTool := tool.(bigqueryexecutesql.Tool)

	for _, tc := range []struct {
		name          string
		sql           string
		statementType string
		wantErr       bool
	}{
		{name: "SELECT allowed", sql: "SELECT 1", statementType: "SELECT", wantErr: false},
		{name: "DELETE blocked", sql: "DELETE FROM perm_ds.t WHERE true", statementType: "DELETE", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				job := &bigqueryrestapi.Job{
					Statistics: &bigqueryrestapi.JobStatistics{
						Query: &bigqueryrestapi.JobStatistics2{StatementType: tc.statementType},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(job)
			}))
			defer srv.Close()

			bqClient, _ := bigqueryapi.NewClient(ctx, "test-project", option.WithEndpoint(srv.URL), option.WithoutAuthentication())
			restService, _ := bigqueryrestapi.NewService(ctx, option.WithEndpoint(srv.URL), option.WithoutAuthentication())
			src := &bqutil.MockSource{
				Client:       bqClient,
				Service:      restService,
				WriteMode:    bigqueryds.WriteModeBlocked,
				RunSQLResult: "ok",
			}
			params, _ := executeSqlTool.GetParameters(src)
			paramVals, _ := parameters.ParseParams(params, map[string]any{"sql": tc.sql}, nil)
			_, invokeErr := tool.Invoke(ctx, src, paramVals, "")
			if (invokeErr != nil) != tc.wantErr {
				t.Fatalf("Invoke() error = %v, wantErr %v", invokeErr, tc.wantErr)
			}
		})
	}
}
