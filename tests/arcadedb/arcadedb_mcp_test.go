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

package arcadedb

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// startArcadeDBMCPServer sets up an ArcadeDB instance, starts the toolbox
// server with the ArcadeDB tools config and waits until it is ready to serve.
func startArcadeDBMCPServer(t *testing.T, ctx context.Context) {
	t.Helper()
	setupArcadeDBInstance(ctx, t)

	cmd, cleanup, err := tests.StartCmd(ctx, getArcadeDBToolsConfig(t))
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	waitCtx, cancelWait := context.WithTimeout(ctx, 30*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

// mcpResultBody rebuilds the JSON array the REST endpoint returns from the MCP
// content blocks. The MCP server emits one block per result row when a tool
// returns a []any (ArcadeDB SQL), and a single block for other results
// (ArcadeDB Cypher), so blocks are parsed and flattened into one array.
func mcpResultBody(t *testing.T, resp *tests.MCPCallToolResponse) string {
	t.Helper()
	rows := []any{}
	for _, content := range resp.Result.Content {
		var item any
		if err := json.Unmarshal([]byte(content.Text), &item); err != nil {
			rows = append(rows, content.Text)
			continue
		}
		if slice, ok := item.([]any); ok {
			rows = append(rows, slice...)
		} else {
			rows = append(rows, item)
		}
	}
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("error marshaling MCP result: %v", err)
	}
	return string(b)
}

func TestArcadeDBMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	startArcadeDBMCPServer(t, ctx)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getArcadeDBMCPExpectedTools())
	})
}

func TestArcadeDBMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	startArcadeDBMCPServer(t, ctx)

	t.Cleanup(func() { teardownFixtures(t) })
	seedFixtures(t)

	for _, tc := range getArcadeDBInvokeTestCases() {
		t.Run(tc.name, func(t *testing.T) {
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
				got = mcpResultBody(t, mcpResp)
			}

			if tc.validateFunc != nil {
				tc.validateFunc(t, got)
			}
		})
	}
}

// getArcadeDBMCPExpectedTools returns the MCP manifests for the tools loaded by getArcadeDBToolsConfig.
func getArcadeDBMCPExpectedTools() []tests.MCPToolManifest {
	return []tests.MCPToolManifest{
		{
			Name:        "my-execute-cypher",
			Description: "Execute Cypher against ArcadeDB.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cypher":  map[string]any{"description": "The cypher to execute.", "type": "string"},
					"dry_run": map[string]any{"default": false, "description": "If set to true, the query will be validated and information about the execution will be returned without running the query. Defaults to false.", "type": "boolean"},
					"params":  map[string]any{"additionalProperties": true, "default": map[string]any{}, "description": "Optional query parameters to use with the cypher statement.", "type": "object"},
				},
				"required": []any{"cypher"},
			},
		},
		{
			Name:        "my-execute-sql",
			Description: "Execute SQL against ArcadeDB.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"dry_run": map[string]any{"default": false, "description": "If set to true, the SQL will be validated and execution plan metadata will be returned without running it. Defaults to false.", "type": "boolean"},
					"params":  map[string]any{"additionalProperties": true, "default": map[string]any{}, "description": "Optional query parameters to use with the SQL statement.", "type": "object"},
					"sql":     map[string]any{"description": "The SQL statement to execute.", "type": "string"},
				},
				"required": []any{"sql"},
			},
		},
		{
			Name:        "my-readonly-cypher",
			Description: "Read-only Cypher against ArcadeDB.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cypher":  map[string]any{"description": "The cypher to execute.", "type": "string"},
					"dry_run": map[string]any{"default": false, "description": "If set to true, the query will be validated and information about the execution will be returned without running the query. Defaults to false.", "type": "boolean"},
					"params":  map[string]any{"additionalProperties": true, "default": map[string]any{}, "description": "Optional query parameters to use with the cypher statement.", "type": "object"},
				},
				"required": []any{"cypher"},
			},
		},
		{
			Name:        "my-readonly-sql",
			Description: "Read-only SQL against ArcadeDB.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"dry_run": map[string]any{"default": false, "description": "If set to true, the SQL will be validated and execution plan metadata will be returned without running it. Defaults to false.", "type": "boolean"},
					"params":  map[string]any{"additionalProperties": true, "default": map[string]any{}, "description": "Optional query parameters to use with the SQL statement.", "type": "object"},
					"sql":     map[string]any{"description": "The SQL statement to execute.", "type": "string"},
				},
				"required": []any{"sql"},
			},
		},
	}
}
