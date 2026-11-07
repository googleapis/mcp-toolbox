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

package serverlessspark

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/googleapis/mcp-toolbox/tests"
)

//go:embed testdata/mcp-tools.json
var serverlessSparkManifests []byte

func TestServerlessSparkMCPCallTools(t *testing.T) {
	ctx := setupServerlessSparkTest(t)
	runServerlessSparkCallTests(t, ctx)
}

func TestServerlessSparkMCPListTools(t *testing.T) {
	setupServerlessSparkTest(t)
	var expected []tests.MCPToolManifest
	if err := json.Unmarshal(serverlessSparkManifests, &expected); err != nil {
		t.Fatal(err)
	}
	tests.RunMCPToolsListMethod(t, expected)
}

// Successful HTTP status alone must not turn an MCP error into a passing call.
func TestServerlessSparkMCPResult(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		wantError        bool
	}{
		{name: "text result", body: `{"result":{"content":[{"type":"text","text":"{}"}]}}`, want: "{}"},
		{name: "tool error", body: `{"result":{"isError":true,"content":[{"type":"text","text":"failed"}]}}`, wantError: true},
		{name: "JSON-RPC error", body: `{"error":{"code":-32602,"message":"invalid arguments"}}`, wantError: true},
		{name: "missing content", body: `{"result":{}}`, wantError: true},
		{name: "empty content", body: `{"result":{"content":[]}}`, wantError: true},
		{name: "non-text content", body: `{"result":{"content":[{"type":"image"}]}}`, wantError: true},
		{name: "multiple results", body: `{"result":{"content":[{"type":"text","text":"{}"},{"type":"text","text":"{}"}]}}`, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response tests.MCPCallToolResponse
			if err := json.Unmarshal([]byte(tc.body), &response); err != nil {
				t.Fatal(err)
			}
			got, err := mcpResultText(&response)
			if (err != nil) != tc.wantError {
				t.Fatalf("result error=%v, want error=%v", err, tc.wantError)
			}
			if err == nil && got != tc.want {
				t.Fatalf("result=%q, want %q", got, tc.want)
			}
		})
	}
}
