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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type templateRoundTripper func(*http.Request) (*http.Response, error)

func (f templateRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// Check requests, not just subtest results: disabled cases used to return PASS
// without exercising either endpoint.
func TestTemplateParametersExecuteRequests(t *testing.T) {
	for _, mcp := range []bool{false, true} {
		for _, tc := range []struct {
			name    string
			options []TemplateParamOption
			omit    map[string]bool
			fields  bool
		}{
			{name: "default"},
			{name: "no_ddl", options: []TemplateParamOption{DisableDdlTest()}, omit: map[string]bool{"create-table-templateParams-tool": true, "drop-table-templateParams-tool": true}},
			{name: "no_insert", options: []TemplateParamOption{DisableInsertTest()}, omit: map[string]bool{"insert-table-templateParams-tool": true}},
			{name: "select_fields", options: []TemplateParamOption{func(c *TemplateParameterTestConfig) { c.supportSelectFields = true }}, fields: true},
		} {
			t.Run(fmt.Sprintf("mcp=%t/%s", mcp, tc.name), func(t *testing.T) {
				var got []string
				// The harness uses http.DefaultClient. These subtests must remain
				// sequential, and restore it before another test can use it.
				original := http.DefaultClient
				t.Cleanup(func() { http.DefaultClient = original })
				http.DefaultClient = &http.Client{Transport: templateRoundTripper(func(r *http.Request) (*http.Response, error) {
					defer r.Body.Close()
					if r.Method != http.MethodPost {
						return nil, fmt.Errorf("unexpected method %s", r.Method)
					}
					var request map[string]any
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						return nil, err
					}
					var tool string
					args := request
					response := map[string]any{"result": "[]"}
					if mcp {
						if r.URL.Path != "/mcp" || request["method"] != "tools/call" {
							return nil, fmt.Errorf("unexpected MCP request: %s %v", r.URL.Path, request)
						}
						params, ok := request["params"].(map[string]any)
						if !ok {
							return nil, fmt.Errorf("missing MCP params")
						}
						tool, _ = params["name"].(string)
						args, _ = params["arguments"].(map[string]any)
						response = map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": map[string]any{"content": []any{}}}
					} else {
						if !strings.HasPrefix(r.URL.Path, "/api/tool/") || !strings.HasSuffix(r.URL.Path, "/invoke") {
							return nil, fmt.Errorf("unexpected REST path: %s", r.URL.Path)
						}
						tool = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/tool/"), "/invoke")
					}
					if args["tableName"] != "request_test" {
						return nil, fmt.Errorf("missing template tableName: %v", args)
					}
					got = append(got, tool)
					// Select-fields has a fixed expected result; other expectations
					// below use empty results to isolate request execution from SQL.
					if tool == "select-fields-templateParams-tool" {
						response["result"] = `[{"name":"Alex"},{"name":"Alice"}]`
						if mcp {
							response["result"] = map[string]any{"content": []any{map[string]any{"type": "text", "text": `[{"name":"Alex"},{"name":"Alice"}]`}}}
						}
					}
					body, err := json.Marshal(response)
					if err != nil {
						return nil, err
					}
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
				})}
				options := []TemplateParamOption{WithSelectAllWant("[]"), WithTmplSelectId1Want("[]"), WithTmplSelectNameWant("[]")}
				options = append(options, tc.options...)
				if mcp {
					options = append(options, WithMCPTemplate())
				}
				RunToolInvokeWithTemplateParameters(t, "request_test", options...)
				var want []string
				for _, tool := range []string{"create-table-templateParams-tool", "insert-table-templateParams-tool", "insert-table-templateParams-tool", "select-templateParams-tool", "select-templateParams-combined-tool", "select-templateParams-combined-tool", "select-fields-templateParams-tool", "select-filter-templateParams-combined-tool", "drop-table-templateParams-tool"} {
					if !tc.omit[tool] && (tool != "select-fields-templateParams-tool" || tc.fields) {
						want = append(want, tool)
					}
				}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("template requests (-want +got):\n%s", diff)
				}
			})
		}
	}
}
