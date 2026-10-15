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

package utility

import (
	"net/http"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/tests"
)

func TestWaitMCPListTools(t *testing.T) {
	setupWaitTest(t)
	tests.RunMCPToolsListMethod(t, []tests.MCPToolManifest{{
		Name: "my-wait-for-tool", Description: "Wait for a specified duration.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{"duration": map[string]any{
				"type": "string", "description": "The duration to wait for, specified as a string (e.g., '10s', '2m', '1h').",
			}},
			"required": []any{"duration"},
		},
	}})
}

func TestWaitMCPCallTools(t *testing.T) {
	setupWaitTest(t)
	for _, tc := range []struct {
		name            string
		args            map[string]any
		want, wantError string
	}{
		{name: "wait completes", args: map[string]any{"duration": "1ms"}, want: `"Wait for 1ms completed successfully."`},
		{name: "zero duration", args: map[string]any{"duration": "0s"}, want: `"Wait for 0s completed successfully."`},
		{name: "invalid duration", args: map[string]any{"duration": "invalid"}, wantError: "invalid duration format"},
		{name: "missing duration", args: map[string]any{}, wantError: `parameter "duration" is required`},
		{name: "wrong duration type", args: map[string]any{"duration": 1}, wantError: `unable to parse value for "duration"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, response, err := tests.InvokeMCPTool(t, "my-wait-for-tool", tc.args, nil)
			if err != nil || status != http.StatusOK {
				t.Fatalf("tools/call status=%d: %v", status, err)
			}
			if response.Error != nil {
				t.Fatalf("unexpected JSON-RPC error: %+v", response.Error)
			}
			if response.Result.IsError != (tc.wantError != "") {
				t.Fatalf("unexpected result: %+v", response.Result)
			}
			content := response.Result.Content
			if len(content) != 1 || content[0].Type != "text" {
				t.Fatalf("unexpected content: %+v", content)
			}
			if tc.wantError != "" {
				if !strings.Contains(content[0].Text, tc.wantError) {
					t.Fatalf("error = %q, want %q", content[0].Text, tc.wantError)
				}
			} else if content[0].Text != tc.want {
				t.Fatalf("result = %q, want %q", content[0].Text, tc.want)
			}
		})
	}
}
