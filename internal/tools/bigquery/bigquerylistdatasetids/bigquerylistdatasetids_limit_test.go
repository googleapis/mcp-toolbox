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

package bigquerylistdatasetids_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	bqutil "github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerycommon"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerylistdatasetids"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/api/option"
)

// Covers https://github.com/googleapis/mcp-toolbox/issues/3975: list_dataset_ids
// iterated (and returned) every dataset in the project with no way to bound
// the response, which produced multi-megabyte results on large projects.
func TestInvokeBoundsResultWithLimitAndPrefix(t *testing.T) {
	allDatasetIDs := []string{"analytics_a", "analytics_b", "reporting_a", "reporting_b", "staging"}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/datasets") {
			http.Error(w, "not implemented", http.StatusNotFound)
			return
		}
		datasets := make([]map[string]any, 0, len(allDatasetIDs))
		for _, id := range allDatasetIDs {
			datasets = append(datasets, map[string]any{
				"datasetReference": map[string]any{
					"projectId": "test-project",
					"datasetId": id,
				},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"kind": "bigquery#datasetList", "datasets": datasets})
	}))
	defer mockServer.Close()

	ctx := t.Context()
	bqClient, err := bigqueryapi.NewClient(ctx, "test-project", option.WithEndpoint(mockServer.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("failed to create mocked BigQuery client: %v", err)
	}

	testSrc := &bqutil.MockSource{Client: bqClient, Project: "test-project"}

	cfg := bigquerylistdatasetids.Config{
		ConfigBase: tools.ConfigBase{Name: "list_dataset_ids_tool", Description: "List dataset ids"},
		Type:       "bigquery-list-dataset-ids",
		Source:     "my-bq-source",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	tcs := []struct {
		desc      string
		extraArgs map[string]any
		want      []string
	}{
		{
			desc: "no limit or prefix returns every dataset, unchanged from today's behavior",
			want: allDatasetIDs,
		},
		{
			desc:      "limit truncates to the first N without fetching the rest",
			extraArgs: map[string]any{"limit": 2},
			want:      []string{"analytics_a", "analytics_b"},
		},
		{
			desc:      "prefix filters regardless of limit",
			extraArgs: map[string]any{"prefix": "reporting_"},
			want:      []string{"reporting_a", "reporting_b"},
		},
		{
			desc:      "limit and prefix combine: limit counts only prefix matches",
			extraArgs: map[string]any{"prefix": "reporting_", "limit": 1},
			want:      []string{"reporting_a"},
		},
		{
			desc:      "prefix matching nothing returns an empty, not nil, result",
			extraArgs: map[string]any{"prefix": "does-not-exist"},
			want:      []string{},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			data := map[string]any{"project": "test-project"}
			for k, v := range tc.extraArgs {
				data[k] = v
			}

			params, err := tool.GetParameters(testSrc)
			if err != nil {
				t.Fatalf("failed to get parameters: %v", err)
			}
			paramVals, err := parameters.ParseParams(params, data, nil)
			if err != nil {
				t.Fatalf("unexpected error parsing parameters: %v", err)
			}

			got, toolErr := tool.Invoke(ctx, testSrc, paramVals, "")
			if toolErr != nil {
				t.Fatalf("unexpected error: %v", toolErr)
			}

			gotIDs := make([]string, 0)
			for _, v := range got.([]any) {
				gotIDs = append(gotIDs, v.(string))
			}
			if len(gotIDs) != len(tc.want) {
				t.Fatalf("got %v, want %v", gotIDs, tc.want)
			}
			for i, id := range tc.want {
				if gotIDs[i] != id {
					t.Fatalf("got %v, want %v", gotIDs, tc.want)
				}
			}
		})
	}
}

func TestInvokeRejectsNegativeLimit(t *testing.T) {
	ctx := t.Context()
	testSrc := &bqutil.MockSource{Project: "test-project"}

	cfg := bigquerylistdatasetids.Config{
		ConfigBase: tools.ConfigBase{Name: "list_dataset_ids_tool", Description: "List dataset ids"},
		Type:       "bigquery-list-dataset-ids",
		Source:     "my-bq-source",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	params, err := tool.GetParameters(testSrc)
	if err != nil {
		t.Fatalf("failed to get parameters: %v", err)
	}
	paramVals, err := parameters.ParseParams(params, map[string]any{"project": "test-project", "limit": -1}, nil)
	if err != nil {
		t.Fatalf("unexpected error parsing parameters: %v", err)
	}

	_, toolErr := tool.Invoke(ctx, testSrc, paramVals, "")
	if toolErr == nil {
		t.Fatalf("expected an error for a negative limit, got nil")
	}
	if !strings.Contains(toolErr.Error(), "must be >= 0") {
		t.Fatalf("expected error to mention 'must be >= 0', got %v", toolErr)
	}
}
