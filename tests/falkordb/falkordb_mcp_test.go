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

package falkordb

import (
	"context"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// startFalkorDBMCPServer sets up a FalkorDB instance, starts the toolbox server
// with the FalkorDB tools config and waits until it is ready to serve.
func startFalkorDBMCPServer(t *testing.T, ctx context.Context) {
	t.Helper()
	setupFalkorDBInstance(ctx, t)

	cmd, cleanup, err := tests.StartCmd(ctx, getFalkorDBToolsConfig(t))
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

func TestFalkorDBMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	startFalkorDBMCPServer(t, ctx)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getFalkorDBMCPExpectedTools())
	})
}

func TestFalkorDBMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	startFalkorDBMCPServer(t, ctx)

	for _, tc := range getFalkorDBInvokeTestCases() {
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

			if tc.wantContentErr != "" {
				tests.AssertMCPError(t, mcpResp, tc.wantContentErr)
				return
			}

			if mcpResp.Error != nil {
				t.Fatalf("%s returned JSON-RPC error: %v", tc.toolName, mcpResp.Error)
			}
			if mcpResp.Result.IsError {
				t.Fatalf("%s returned error result: %v", tc.toolName, mcpResp.Result)
			}
			// FalkorDB results are returned as a single JSON document, which the
			// MCP server sends as one text content block.
			if len(mcpResp.Result.Content) != 1 {
				t.Fatalf("%s returned %d content blocks, want 1: %v", tc.toolName, len(mcpResp.Result.Content), mcpResp.Result.Content)
			}
			got := mcpResp.Result.Content[0].Text

			if tc.validateFunc != nil {
				tc.validateFunc(t, got)
			} else if got != tc.want {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}

// getFalkorDBMCPExpectedTools returns the MCP manifests for the tools loaded by getFalkorDBToolsConfig.
func getFalkorDBMCPExpectedTools() []tests.MCPToolManifest {
	return []tests.MCPToolManifest{
		{
			Name:        "my-empty-graph-schema-tool",
			Description: "A schema tool pointed at a graph that does not exist yet.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-graph-override-execute-cypher-tool",
			Description: "A cypher execution tool allowing graph override.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cypher":  map[string]any{"description": "The cypher to execute.", "type": "string"},
					"dry_run": map[string]any{"default": false, "description": "If set to true, the query will be validated and its execution plan returned without running the query. Defaults to false.", "type": "boolean"},
					"graph":   map[string]any{"default": "", "description": "The name of the graph to query. Defaults to the source's configured graph.", "type": "string"},
				},
				"required": []any{"cypher"},
			},
		},
		{
			Name:        "my-list-graphs-tool",
			Description: "A tool to list the graphs on the instance.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-param-cypher-tool",
			Description: "A tool with a parameterized statement.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"value": map[string]any{"description": "The value to echo back.", "type": "string"},
				},
				"required": []any{"value"},
			},
		},
		{
			Name:        "my-readonly-execute-cypher-tool",
			Description: "A readonly cypher execution tool.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cypher":  map[string]any{"description": "The cypher to execute.", "type": "string"},
					"dry_run": map[string]any{"default": false, "description": "If set to true, the query will be validated and its execution plan returned without running the query. Defaults to false.", "type": "boolean"},
				},
				"required": []any{"cypher"},
			},
		},
		{
			Name:        "my-schema-tool",
			Description: "A tool to get the FalkorDB graph schema.",
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
					"dry_run": map[string]any{"default": false, "description": "If set to true, the query will be validated and its execution plan returned without running the query. Defaults to false.", "type": "boolean"},
				},
				"required": []any{"cypher"},
			},
		},
	}
}
