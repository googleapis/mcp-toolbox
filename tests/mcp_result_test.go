// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestMCPResultText(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want []any
	}{
		{"empty content", `{"result":{"content":[]}}`, []any{}},
		{"JSON array", `{"result":{"content":[{"type":"text","text":"[{\"name\":\"first\"}]"}]}}`, []any{map[string]any{"name": "first"}}},
		{"JSON string", `{"result":{"content":[{"type":"text","text":"\"created\""}]}}`, []any{"created"}},
		{"plain text", `{"result":{"content":[{"type":"text","text":"created"}]}}`, []any{"created"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response MCPCallToolResponse
			if err := json.Unmarshal([]byte(tc.body), &response); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, getMCPResultText(t, &response)); diff != "" {
				t.Fatalf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// The child runs the real prebuilt-tool harness, whose no-row case previously
// accepted HTTP 200 error responses as an empty successful result.
func TestMCPPrebuiltErrorResponses(t *testing.T) {
	const scenarioEnv = "TOOLBOX_TEST_PREBUILT_RESPONSE"
	if scenario := os.Getenv(scenarioEnv); scenario != "" {
		if scenario == "nil response" {
			getMCPResultText(t, nil)
			return
		}
		original := http.DefaultClient
		t.Cleanup(func() { http.DefaultClient = original })
		http.DefaultClient = &http.Client{Transport: executeSQLRoundTripper(func(r *http.Request) (*http.Response, error) {
			defer r.Body.Close()
			var request MCPCallToolRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				return nil, err
			}
			if r.URL.Path != "/mcp" || request.Method != "tools/call" ||
				request.Params.Name != "list_tables" || request.Params.Arguments["table_names"] != "non_existent_table" {
				return nil, fmt.Errorf("unexpected prebuilt request: %s %+v", r.URL.Path, request)
			}
			t.Log("prebuilt request received")
			result := map[string]any{"content": []any{}}
			response := map[string]any{"jsonrpc": "2.0", "id": request.Id, "result": result}
			switch scenario {
			case "success":
			case "tool error":
				result["isError"] = true
			case "protocol error":
				delete(response, "result")
				response["error"] = map[string]any{"code": -32603, "message": "backend failed"}
			default:
				return nil, fmt.Errorf("unknown scenario: %s", scenario)
			}
			body, err := json.Marshal(response)
			if err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		})}
		RunMySQLListTablesTest(t, "toolbox", "params", "auth", "", WithMCPExec())
		return
	}
	for _, tc := range []struct {
		scenario  string
		wantError string
	}{
		{"success", ""},
		{"tool error", "unexpected MCP tool error"},
		{"protocol error", "unexpected JSON-RPC error"},
		{"nil response", "missing MCP response"},
	} {
		t.Run(tc.scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			pattern := "^TestMCPPrebuiltErrorResponses$/^invoke_list_tables_with_non-existent_table$"
			child := exec.CommandContext(ctx, os.Args[0], "-test.v", "-test.run="+pattern)
			child.Env = append(os.Environ(), scenarioEnv+"="+tc.scenario)
			output, err := child.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("child timed out: %v\n%s", ctx.Err(), output)
			}
			wantRequests := 1
			if tc.scenario == "nil response" {
				wantRequests = 0
			}
			if got := strings.Count(string(output), "prebuilt request received"); got != wantRequests {
				t.Fatalf("executed %d requests, want %d:\n%s", got, wantRequests, output)
			}
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("successful empty result was rejected: %v\n%s", err, output)
				}
			} else if err == nil || !strings.Contains(string(output), tc.wantError) {
				t.Fatalf("expected %q, got exit %v:\n%s", tc.wantError, err, output)
			}
		})
	}
}

// An invalid query must fail at the tool layer, not just return HTTP 200.
func TestMySQLQueryPlanMCPErrorResponses(t *testing.T) {
	const scenarioEnv = "TOOLBOX_TEST_QUERY_PLAN_RESPONSE"
	if scenario := os.Getenv(scenarioEnv); scenario != "" {
		original := http.DefaultClient
		t.Cleanup(func() { http.DefaultClient = original })
		http.DefaultClient = &http.Client{Transport: executeSQLRoundTripper(func(r *http.Request) (*http.Response, error) {
			defer r.Body.Close()
			var request MCPCallToolRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				return nil, err
			}
			if r.URL.Path != "/mcp" || request.Method != "tools/call" ||
				request.Params.Name != "get_query_plan" || request.Params.Arguments["sql_statement"] != "SELECT * FROM non_existent_table" {
				return nil, fmt.Errorf("unexpected query-plan request: %s %+v", r.URL.Path, request)
			}
			t.Log("query-plan request received")
			result := map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": "table non_existent_table does not exist"}}}
			response := map[string]any{"jsonrpc": "2.0", "id": request.Id, "result": result}
			switch scenario {
			case "valid tool error":
			case "success":
				result["isError"] = false
				result["content"] = []any{}
			case "wrong tool error":
				result["content"] = []any{map[string]any{"type": "text", "text": "unrelated failure"}}
			case "protocol error":
				delete(response, "result")
				response["error"] = map[string]any{"code": -32603, "message": "table non_existent_table does not exist"}
			default:
				return nil, fmt.Errorf("unknown scenario: %s", scenario)
			}
			body, err := json.Marshal(response)
			if err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		})}
		RunMySQLGetQueryPlanTest(t, t.Context(), nil, "toolbox", "params", WithMCPExec())
		return
	}
	for _, tc := range []struct {
		scenario  string
		wantError string
	}{
		{"valid tool error", ""},
		{"success", "expected a tool error"},
		{"wrong tool error", "expected error text containing"},
		{"protocol error", "unexpected JSON-RPC error"},
	} {
		t.Run(tc.scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			pattern := "^TestMySQLQueryPlanMCPErrorResponses$/^invoke_get_query_plan_with_invalid_query$"
			child := exec.CommandContext(ctx, os.Args[0], "-test.v", "-test.run="+pattern)
			child.Env = append(os.Environ(), scenarioEnv+"="+tc.scenario)
			output, err := child.CombinedOutput()
			if ctx.Err() != nil {
				t.Fatalf("child timed out: %v\n%s", ctx.Err(), output)
			}
			if got := strings.Count(string(output), "query-plan request received"); got != 1 {
				t.Fatalf("executed %d requests, want 1:\n%s", got, output)
			}
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("expected tool error was rejected: %v\n%s", err, output)
				}
			} else if err == nil || !strings.Contains(string(output), tc.wantError) {
				t.Fatalf("expected %q, got exit %v:\n%s", tc.wantError, err, output)
			}
		})
	}
}
