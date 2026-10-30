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

package dgraph

import (
	"net/http"
	"testing"

	"github.com/googleapis/mcp-toolbox/tests"
)

func TestDgraphMCPListTools(t *testing.T) {
	setupDgraphTest(t)
	tests.RunMCPToolsListMethod(t, []tests.MCPToolManifest{{
		Name:        "my-simple-dql-tool",
		Description: "Simple tool to test end to end functionality.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
	}})
}

func TestDgraphMCPCallTools(t *testing.T) {
	setupDgraphTest(t)
	status, response, err := tests.InvokeMCPTool(t, "my-simple-dql-tool", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("tools/call failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("tools/call status = %d, want %d", status, http.StatusOK)
	}
	if response.Error != nil || response.Result.IsError {
		t.Fatalf("tools/call returned an error: %+v", response)
	}
	content := response.Result.Content
	if len(content) != 1 || content[0].Type != "text" {
		t.Fatalf("unexpected content: %+v", content)
	}
	const want = `{"result":[{"constant":1}]}`
	if content[0].Text != want {
		t.Fatalf("result = %q, want %q", content[0].Text, want)
	}
}
