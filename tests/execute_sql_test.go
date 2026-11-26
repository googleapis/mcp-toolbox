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
)

type executeSQLRoundTripper func(*http.Request) (*http.Response, error)

func (f executeSQLRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Exercise the real harness in a child process so expected t.Fatal failures do
// not fail the parent. No database or Google credentials are used.
func TestExecuteSQLMCPErrorResponses(t *testing.T) {
	const scenarioEnv = "TOOLBOX_TEST_SQL_ERROR_RESPONSE"
	if scenario := os.Getenv(scenarioEnv); scenario != "" {
		original := http.DefaultClient
		t.Cleanup(func() { http.DefaultClient = original })
		http.DefaultClient = &http.Client{Transport: executeSQLRoundTripper(func(r *http.Request) (*http.Response, error) {
			defer r.Body.Close()
			var request MCPCallToolRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				return nil, err
			}
			t.Logf("SQL request: %s", request.Params.Name)
			if r.URL.Path != "/mcp" || request.Method != "tools/call" {
				return nil, fmt.Errorf("unexpected request: %s %+v", r.URL.Path, request)
			}
			if scenario == "transport_error" {
				return nil, fmt.Errorf("synthetic network failure")
			}
			status := http.StatusOK
			message := "syntax error"
			if len(request.Params.Arguments) == 0 {
				message = `parameter "sql" is required`
			}
			if strings.Contains(fmt.Sprint(request.Params.Arguments["sql"]), "non_existent_table") {
				message = "table non_existent_table does not exist"
			}
			result := map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": message}}}
			response := map[string]any{"jsonrpc": "2.0", "id": request.Id, "result": result}
			if request.Params.Name == "my-auth-exec-sql-tool" {
				status = http.StatusUnauthorized
				delete(response, "result")
				response["error"] = map[string]any{"code": -32600, "message": "unauthorized Tool call"}
			}
			switch scenario {
			case "success":
				status = http.StatusOK
				delete(response, "error")
				response["result"] = map[string]any{"isError": false, "content": []any{}}
			case "http_500":
				status = http.StatusInternalServerError
			case "protocol_error":
				delete(response, "result")
				response["error"] = map[string]any{"code": -32603, "message": "unrelated server error"}
			case "empty_tool_error":
				result["content"] = []any{}
			case "wrong_tool_error":
				result["content"] = []any{map[string]any{"type": "text", "text": "unrelated failure"}}
			case "valid":
			default:
				return nil, fmt.Errorf("unknown scenario: %s", scenario)
			}
			body, err := json.Marshal(response)
			if err != nil {
				return nil, err
			}
			return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		})}
		RunExecuteSqlToolInvokeTest(t, `"CREATE TABLE t (id INT)"`, `[{"1":1}]`, WithMCPSql())
		return
	}
	for _, scenario := range []string{"valid", "success", "http_500", "protocol_error", "empty_tool_error", "wrong_tool_error", "transport_error"} {
		for _, testCase := range []string{
			"invoke_my-exec-sql-tool_without_body",
			"invoke_my-exec-sql-tool_with_invalid_SELECT_SQL",
			"invoke_my-exec-sql-tool_with_invalid_ALTER_SQL",
			"Invoke_my-auth-exec-sql-tool_with_invalid_auth_token",
			"Invoke_my-auth-exec-sql-tool_without_auth_token",
		} {
			if (scenario == "empty_tool_error" || scenario == "wrong_tool_error") && strings.HasPrefix(testCase, "Invoke_my-auth") {
				continue
			}
			// SQL syntax errors vary by backend; that case requires nonempty
			// tool-error content, while missing arguments/table names are stable.
			if scenario == "wrong_tool_error" && strings.HasSuffix(testCase, "ALTER_SQL") {
				continue
			}
			t.Run(scenario+"/"+testCase, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
				defer cancel()
				// Run each negative case separately: one failing assertion must
				// not hide a different case that incorrectly accepts success.
				pattern := "^TestExecuteSQLMCPErrorResponses$/^" + testCase + "$"
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.v", "-test.run="+pattern)
				cmd.Env = append(os.Environ(), scenarioEnv+"="+scenario)
				output, err := cmd.CombinedOutput()
				if ctx.Err() != nil {
					t.Fatalf("child timed out: %v\n%s", ctx.Err(), output)
				}
				if count := strings.Count(string(output), "SQL request:"); count != 1 {
					t.Fatalf("executed %d requests, want 1:\n%s", count, output)
				}
				if (err == nil) != (scenario == "valid") {
					t.Fatalf("scenario %s: exit error=%v\n%s", scenario, err, output)
				}
			})
		}
	}
}
