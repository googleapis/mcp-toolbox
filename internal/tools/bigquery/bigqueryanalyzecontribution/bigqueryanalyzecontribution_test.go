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

package bigqueryanalyzecontribution_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigqueryanalyzecontribution"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerycommon"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/api/option"
)

func TestParseFromYamlBigQueryAnalyzeContribution(t *testing.T) {
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
            type: bigquery-analyze-contribution
            source: my-instance
            description: some description
            `,
			want: server.ToolConfigs{
				"example_tool": bigqueryanalyzecontribution.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "bigquery-analyze-contribution",
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

type mockTransport struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

func TestInvoke(t *testing.T) {
	mockClient := &http.Client{
		Transport: &mockTransport{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				// We expect requests to the jobs API (either POST to create, or GET to check status)
				respBody := `{
					"kind": "bigquery#job",
					"jobReference": {
						"projectId": "my-project",
						"jobId": "mock-job-id",
						"location": "US"
					},
					"status": {
						"state": "DONE"
					}
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(respBody)),
				}, nil
			},
		},
	}
	bqClient, err := bigqueryapi.NewClient(context.Background(), "my-project", option.WithHTTPClient(mockClient))
	if err != nil {
		t.Fatalf("failed to create bigquery client: %v", err)
	}

	cfg := bigqueryanalyzecontribution.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "analyze_contribution_tool",
			Description: "Analyze Contribution",
		},
		Type:   "bigquery-analyze-contribution",
		Source: "my-bq-source",
	}
	src := &bigquerycommon.MockSource{Client: bqClient, RunSQLResult: "mocked_analyze_contribution_result"}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	analyzeContributionTool, ok := tool.(bigqueryanalyzecontribution.Tool)
	if !ok {
		t.Fatalf("expected bigqueryanalyzecontribution.Tool, got %T", tool)
	}

	tcs := []struct {
		desc               string
		inputData          any
		contributionMetric any
		isTestCol          any
		dimensionIdCols    any
		wantErr            bool
		wantSubstr         string
		wantSQLSub         string
	}{
		{
			desc:               "happy path",
			inputData:          "my_dataset.my_table",
			contributionMetric: "SUM(metric)",
			isTestCol:          "is_test",
			dimensionIdCols:    []any{"dim1", "dim2"},
			wantSQLSub:         "SELECT * FROM ML.GET_INSIGHTS(MODEL contribution_analysis_model_",
		},
		{
			desc:               "SQL injection attempt in dimension_id_cols",
			inputData:          "my_dataset.my_table",
			contributionMetric: "SUM(metric)",
			isTestCol:          "is_test",
			dimensionIdCols:    []any{"dim1", "dim2; drop table x"},
			wantErr:            true,
			wantSubstr:         "invalid column name in 'dimension_id_cols'",
		},
		{
			desc:               "SQL injection attempt in is_test_col",
			inputData:          "my_dataset.my_table",
			contributionMetric: "SUM(metric)",
			isTestCol:          "is_test; drop table x",
			dimensionIdCols:    []any{"dim1"},
			wantErr:            true,
			wantSubstr:         "invalid column name for 'is_test_col'",
		},
		{
			desc:               "single quote in contribution_metric",
			inputData:          "my_dataset.my_table",
			contributionMetric: "SUM('metric')",
			isTestCol:          "is_test",
			dimensionIdCols:    []any{"dim1"},
			wantErr:            true,
			wantSubstr:         "invalid 'contribution_metric': must not contain single quotes",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			data := map[string]any{}
			if tc.inputData != nil {
				data["input_data"] = tc.inputData
			}
			if tc.contributionMetric != nil {
				data["contribution_metric"] = tc.contributionMetric
			}
			if tc.isTestCol != nil {
				data["is_test_col"] = tc.isTestCol
			}
			if tc.dimensionIdCols != nil {
				data["dimension_id_cols"] = tc.dimensionIdCols
			}

			params, err := analyzeContributionTool.GetParameters(src)
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

			if resp != "mocked_analyze_contribution_result" {
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
		AllowedDatasets: []string{"allowed_dataset"},
	}

	cfg := bigqueryanalyzecontribution.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "analyze_contribution_tool",
			Description: "Analyze Contribution",
		},
		Type:   "bigquery-analyze-contribution",
		Source: "my-bq-source",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	analyzeContributionTool, ok := tool.(bigqueryanalyzecontribution.Tool)
	if !ok {
		t.Fatalf("expected bigqueryanalyzecontribution.Tool, got %T", tool)
	}

	// 4. Set up parameters
	data := map[string]any{
		"input_data":          "allowed_dataset.my_table",
		"contribution_metric": "SUM(metric)",
		"is_test_col":         "is_test",
		"dimension_id_cols":   []any{"dim1"},
	}

	params, err := analyzeContributionTool.GetParameters(testSrc)
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
