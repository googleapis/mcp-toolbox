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

package bigqueryforecast_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerycommon"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigqueryforecast"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/api/option"
)

func TestParseFromYamlBigQueryForecast(t *testing.T) {
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
            type: bigquery-forecast
            source: my-instance
            description: some description
            `,
			want: server.ToolConfigs{
				"example_tool": bigqueryforecast.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "bigquery-forecast",
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

func TestInvoke(t *testing.T) {
	cfg := bigqueryforecast.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "forecast_tool",
			Description: "Forecast",
		},
		Type:   "bigquery-forecast",
		Source: "my-bq-source",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	forecastTool, ok := tool.(bigqueryforecast.Tool)
	if !ok {
		t.Fatalf("expected bigqueryforecast.Tool, got %T", tool)
	}

	tcs := []struct {
		desc         string
		historyData  any
		timestampCol any
		dataCol      any
		horizon      any
		idCols       any
		wantErr      bool
		wantSubstr   string
		wantSQLSub   string
	}{
		{
			desc:         "happy path",
			historyData:  "my_dataset.my_table",
			timestampCol: "ts",
			dataCol:      "val",
			horizon:      5,
			wantSQLSub:   "timestamp_col => 'ts',",
		},
		{
			desc:         "happy path with id_cols",
			historyData:  "my_dataset.my_table",
			timestampCol: "ts",
			dataCol:      "val",
			idCols:       []any{"id1", "id2"},
			wantSQLSub:   "id_cols => ['id1', 'id2']",
		},
		{
			desc:         "invalid timestamp_col with spaces",
			historyData:  "my_dataset.my_table",
			timestampCol: "ts col",
			dataCol:      "val",
			wantErr:      true,
			wantSubstr:   "invalid column name for 'timestamp_col'",
		},
		{
			desc:         "SQL injection attempt in data_col",
			historyData:  "my_dataset.my_table",
			timestampCol: "ts",
			dataCol:      "val; drop table x",
			wantErr:      true,
			wantSubstr:   "invalid column name for 'data_col'",
		},
		{
			desc:         "SQL injection attempt in id_cols",
			historyData:  "my_dataset.my_table",
			timestampCol: "ts",
			dataCol:      "val",
			idCols:       []any{"id1; drop table x"},
			wantErr:      true,
			wantSubstr:   "invalid column name in 'id_cols'",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			src := &bigquerycommon.MockSource{RunSQLResult: "mocked_forecast_result"}
			data := map[string]any{}
			if tc.historyData != nil {
				data["history_data"] = tc.historyData
			}
			if tc.timestampCol != nil {
				data["timestamp_col"] = tc.timestampCol
			}
			if tc.dataCol != nil {
				data["data_col"] = tc.dataCol
			}
			if tc.horizon != nil {
				data["horizon"] = tc.horizon
			}
			if tc.idCols != nil {
				data["id_cols"] = tc.idCols
			}

			params, err := forecastTool.GetParameters(src)
			if err != nil {
				t.Fatalf("failed to get parameters: %v", err)
			}
			paramVals, err := parameters.ParseParams(params, data, nil)
			if err != nil {
				if tc.wantErr {
					if !strings.Contains(err.Error(), tc.wantSubstr) {
						t.Errorf("expected parse error to contain %q, got %v", tc.wantSubstr, err)
					}
					return
				}
				t.Fatalf("unexpected error parsing parameters: %v", err)
			}

			ctx, err := testutils.ContextWithNewLogger()
			if err != nil {
				t.Fatalf("failed to create context with logger: %v", err)
			}

			resp, err := tool.Invoke(ctx, src, paramVals, "")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Errorf("expected error to contain %q, got %v", tc.wantSubstr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp != "mocked_forecast_result" {
				t.Errorf("unexpected response: got %v", resp)
			}

			if tc.wantSQLSub != "" && !strings.Contains(src.CalledSQL, tc.wantSQLSub) {
				t.Errorf("expected SQL to contain %q, but got:\n%s", tc.wantSQLSub, src.CalledSQL)
			}
		})
	}
}

func TestInvokeAllowedDatasetsValidation(t *testing.T) {
	// 1. Start httptest Server to mock BigQuery jobs.insert API
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Mock server received: %s %s", r.Method, r.URL.String())
		// Verify this is a jobs insert request
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/jobs") {
			// Read request body to verify it's a dry run query
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
			t.Logf("Mock server decoded configuration: %+v", body.Configuration)

			// We only mock the dry run query validation
			if body.Configuration.DryRun {
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
							"query": body.Configuration.Query.Query,
						},
					},
					"statistics": map[string]any{
						"creationTime": "123456789",
						"startTime":    "123456789",
						"endTime":      "123456789",
						"query": map[string]any{
							"referencedTables": []map[string]any{
								{
									"projectId": "test-project",
									"datasetId": "unauthorized_dataset", // This dataset is NOT in the allowed list!
									"tableId":   "some_table",
								},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
		}

		// Fallback error
		http.Error(w, "not implemented", http.StatusNotFound)
	}))
	defer mockServer.Close()

	// 2. Initialize BigQuery client pointing to the mock server
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("failed to create context with logger: %v", err)
	}

	bqClient, err := bigqueryapi.NewClient(ctx, "test-project", option.WithEndpoint(mockServer.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("failed to create mocked BigQuery client: %v", err)
	}

	// 3. Define mock source that returns this client and allowed datasets configuration
	testSrc := &bigquerycommon.MockSource{
		Client:          bqClient,
		AllowedDatasets: []string{"allowed_dataset"}, // only "allowed_dataset" is allowed!
	}

	cfg := bigqueryforecast.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "forecast_tool",
			Description: "Forecast",
		},
		Type:   "bigquery-forecast",
		Source: "my-bq-source",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	forecastTool, ok := tool.(bigqueryforecast.Tool)
	if !ok {
		t.Fatalf("expected bigqueryforecast.Tool, got %T", tool)
	}

	// 4. Set up parameters mimicking the bypass/injection attempt
	// We try to run the tool, but the dry-run of the final query will detect the reference to "unauthorized_dataset"
	data := map[string]any{
		"history_data":  "allowed_dataset.my_table",
		"timestamp_col": "ts",
		"data_col":      "val",
		"horizon":       5,
	}

	params, err := forecastTool.GetParameters(testSrc)
	if err != nil {
		t.Fatalf("failed to get parameters: %v", err)
	}
	paramVals, err := parameters.ParseParams(params, data, nil)
	if err != nil {
		t.Fatalf("unexpected error parsing parameters: %v", err)
	}

	// 5. Invoke the tool and assert it fails with the dataset permission check error
	_, err = tool.Invoke(ctx, testSrc, paramVals, "")
	if err == nil {
		t.Fatal("expected Invoke to return an error due to out-of-allowlist dataset reference, but got nil")
	}

	expectedErr := "query accesses dataset 'test-project.unauthorized_dataset', which is not in the allowed list"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("expected error to contain %q, got: %v", expectedErr, err)
	}
}
