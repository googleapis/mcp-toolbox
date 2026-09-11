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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	bigqueryds "github.com/googleapis/mcp-toolbox/internal/sources/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	bqutil "github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerycommon"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigqueryexecutesql"
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

	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("failed to create context with logger: %v", err)
	}

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
			wantSub: "query accesses dataset 'test-project.forbidden_dataset', which is not in the allowed list",
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
			wantSub: "query accesses dataset 'test-project.forbidden_dataset', which is not in the allowed list",
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
			wantSub: "query accesses dataset 'test-project.forbidden_dataset', which is not in the allowed list",
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
				ConfigBase:  tools.ConfigBase{Name: "bigquery-execute-sql", Description: "execute sql query"},
				Type:        "bigquery-execute-sql",
				Source:      "my-instance",
				Annotations: tc.annotations,
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
