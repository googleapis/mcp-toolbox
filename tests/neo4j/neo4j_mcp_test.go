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

package neo4j

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// startNeo4jMCPServer sets up a Neo4j instance, starts the toolbox server with
// the Neo4j tools config and waits until it is ready to serve.
func startNeo4jMCPServer(t *testing.T, ctx context.Context) {
	t.Helper()
	setupNeo4jInstance(ctx, t)

	cmd, cleanup, err := tests.StartCmd(ctx, getNeo4jToolsConfig(t))
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

func TestNeo4jMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	startNeo4jMCPServer(t, ctx)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getNeo4jMCPExpectedTools())
	})
}

func TestNeo4jMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	startNeo4jMCPServer(t, ctx)

	for _, tc := range getNeo4jInvokeTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			if tc.prepareData != nil {
				tc.prepareData(t)
			}

			statusCode, mcpResp, err := tests.InvokeMCPTool(t, tc.toolName, tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing %s: %s", tc.toolName, err)
			}
			if statusCode != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, statusCode)
			}
			if mcpResp.Error != nil {
				t.Fatalf("%s returned JSON-RPC error: %v", tc.toolName, mcpResp.Error)
			}

			var got string
			if tc.wantErr {
				if !mcpResp.Result.IsError {
					t.Fatalf("%s: expected an error result, got: %v", tc.toolName, mcpResp.Result)
				}
				var errText strings.Builder
				for _, content := range mcpResp.Result.Content {
					errText.WriteString(content.Text)
				}
				got = errText.String()
			} else {
				if mcpResp.Result.IsError {
					t.Fatalf("%s returned error result: %v", tc.toolName, mcpResp.Result)
				}
				// Neo4j results are returned as a single JSON document, which the
				// MCP server sends as one text content block.
				if len(mcpResp.Result.Content) != 1 {
					t.Fatalf("%s returned %d content blocks, want 1: %v", tc.toolName, len(mcpResp.Result.Content), mcpResp.Result.Content)
				}
				got = mcpResp.Result.Content[0].Text
			}

			if tc.validateFunc != nil {
				tc.validateFunc(t, got)
			} else if got != tc.want {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}

// getNeo4jMCPExpectedTools returns the MCP manifests for the tools loaded by getNeo4jToolsConfig.
func getNeo4jMCPExpectedTools() []tests.MCPToolManifest {
	return []tests.MCPToolManifest{
		{
			Name:        "my-populated-schema-tool",
			Description: "A tool to get the Neo4j schema from a populated DB.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-readonly-execute-cypher-tool",
			Description: "A readonly cypher execution tool.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cypher":  map[string]any{"description": "The cypher to execute.", "type": "string"},
					"dry_run": map[string]any{"default": false, "description": "If set to true, the query will be validated and information about the execution will be returned without running the query. Defaults to false.", "type": "boolean"},
				},
				"required": []any{"cypher"},
			},
		},
		{
			Name:        "my-schema-tool",
			Description: "A tool to get the Neo4j schema.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-schema-tool-with-cache",
			Description: "A schema tool with a custom cache expiration.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-simple-cypher-tool",
			Description: "Simple tool to test end to end functionality.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-simple-execute-cypher-tool",
			Description: "Simple tool to test end to end functionality.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cypher":  map[string]any{"description": "The cypher to execute.", "type": "string"},
					"dry_run": map[string]any{"default": false, "description": "If set to true, the query will be validated and information about the execution will be returned without running the query. Defaults to false.", "type": "boolean"},
				},
				"required": []any{"cypher"},
			},
		},
	}
}
