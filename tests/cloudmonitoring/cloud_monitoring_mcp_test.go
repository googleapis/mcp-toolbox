// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloudmonitoring

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/tests"
)

func TestCloudMonitoringMCPListTools(t *testing.T) {
	calls := setupCloudMonitoringTest(t)
	tests.RunMCPToolsListMethod(t, []tests.MCPToolManifest{{
		Name: "query-prometheus", Description: "Query Prometheus metrics.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectId": map[string]any{"type": "string", "description": "The Id of the Google Cloud project."},
				"query":     map[string]any{"type": "string", "description": "The promql query to execute."},
			},
			"required": []any{"projectId", "query"},
		},
	}})
	if got := calls.Load(); got != 0 {
		t.Fatalf("discovery made %d backend requests, want 0", got)
	}
}

func TestCloudMonitoringMCPCallTools(t *testing.T) {
	calls := setupCloudMonitoringTest(t)
	for _, tc := range []struct {
		name, query string
		wantError   bool
	}{
		{name: "empty query result", query: "up"},
		{name: "backend failure", query: "backend-error", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls.Load()
			status, response, err := tests.InvokeMCPTool(t, "query-prometheus", map[string]any{"projectId": "test-project", "query": tc.query}, nil)
			if err != nil || status != http.StatusOK {
				t.Fatalf("tools/call status=%d: %v", status, err)
			}
			if response.Error != nil {
				t.Fatalf("unexpected JSON-RPC error: %+v", response.Error)
			}
			if response.Result.IsError != tc.wantError {
				t.Fatalf("isError=%v, want %v: %+v", response.Result.IsError, tc.wantError, response.Result)
			}
			if got := calls.Load() - before; got != 1 {
				t.Fatalf("backend requests=%d, want 1", got)
			}
			if tc.wantError {
				tests.AssertMCPError(t, response, "monitoring backend unavailable")
				return
			}
			content := response.Result.Content
			if len(content) != 1 || content[0].Type != "text" {
				t.Fatalf("unexpected content: %+v", content)
			}
			var got any
			if err := json.Unmarshal([]byte(content[0].Text), &got); err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": []any{}}}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("query result mismatch (-want +got):\n%s", diff)
			}
		})
	}
	t.Run("missing query", func(t *testing.T) {
		before := calls.Load()
		status, response, err := tests.InvokeMCPTool(t, "query-prometheus", map[string]any{"projectId": "test-project"}, nil)
		if err != nil || status != http.StatusOK {
			t.Fatalf("tools/call status=%d: %v", status, err)
		}
		tests.AssertMCPError(t, response, `parameter "query" is required`)
		if got := calls.Load() - before; got != 0 {
			t.Fatalf("invalid call made %d backend requests", got)
		}
	})
}
