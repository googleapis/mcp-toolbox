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

package elasticsearch

import (
	"context"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// elasticsearchMCPIndex is the index seeded for the MCP tests.
const elasticsearchMCPIndex = "test-index"

// startElasticsearchMCPServer starts an Elasticsearch container, starts the
// toolbox server with the Elasticsearch tools config, waits until it is ready
// to serve and seeds the test index.
func startElasticsearchMCPServer(t *testing.T, ctx context.Context) {
	t.Helper()
	setupElasticsearchInstance(ctx, t)

	paramToolStatement, idParamToolStatement, nameParamToolStatement, arrayParamToolStatement, authToolStatement := getElasticsearchQueries(elasticsearchMCPIndex)
	toolsConfig := getElasticsearchToolsConfig(getElasticsearchVars(t), ElasticsearchToolType, paramToolStatement, idParamToolStatement, nameParamToolStatement, arrayParamToolStatement, authToolStatement)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsConfig)
	if err != nil {
		t.Fatalf("failed to start cmd: %v", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	seedElasticsearchIndex(ctx, t, elasticsearchMCPIndex)
}

// getElasticsearchMCPExpectedTools returns the MCP manifests for the tools
// loaded by getElasticsearchToolsConfig. The config has no array tool, and
// my-secure-tool is not listed because tools with secure parameters are
// excluded from tools/list in this protocol version.
func getElasticsearchMCPExpectedTools() []tests.MCPToolManifest {
	expected := []tests.MCPToolManifest{}
	for _, manifest := range tests.GetBaseMCPExpectedTools() {
		if manifest.Name == "my-array-tool" {
			continue
		}
		expected = append(expected, manifest)
	}
	return append(expected, tests.MCPToolManifest{
		Name:        "my-execute-tool",
		Description: "Tool to test arbitrary ES|QL execution.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"description": "The ES|QL statement to execute.", "type": "string"},
			},
			"required": []any{"query"},
		},
	})
}

func TestElasticsearchMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	startElasticsearchMCPServer(t, ctx)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getElasticsearchMCPExpectedTools())
	})
}

func TestElasticsearchMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	startElasticsearchMCPServer(t, ctx)

	wants := getElasticsearchWants()

	// Elasticsearch returns query errors as a successful result holding the
	// error document, which the MCP harness wraps in a JSON array.
	tests.RunToolInvokeTest(t, wants.Select1,
		tests.DisableArrayTest(),
		tests.WithMCP(),
		tests.WithMyToolId3NameAliceWant(wants.MyToolId3NameAlice),
		tests.WithMyToolById4Want(wants.MyToolById4),
		tests.WithNullWant("["+wants.Null+"]"),
	)
	tests.RunMCPToolCallMethod(t, wants.McpMyFailTool, wants.McpSelect1, tests.WithMcpMyToolId3NameAliceWant(wants.McpMyToolId3NameAlice))
	runExecuteEsqlMCPTest(t, elasticsearchMCPIndex)
}

// runExecuteEsqlMCPTest invokes my-execute-tool through the MCP endpoint.
func runExecuteEsqlMCPTest(t *testing.T, index string) {
	t.Run("invoke my-execute-tool", func(t *testing.T) {
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, "my-execute-tool", map[string]any{"query": executeEsqlQuery(index)}, nil)
		if err != nil {
			t.Fatalf("native error executing my-execute-tool: %s", err)
		}
		if statusCode != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, statusCode)
		}
		if mcpResp.Error != nil {
			t.Fatalf("my-execute-tool returned JSON-RPC error: %v", mcpResp.Error)
		}
		if mcpResp.Result.IsError {
			t.Fatalf("my-execute-tool returned error result: %v", mcpResp.Result)
		}
		// ES|QL results are returned as a single JSON document, which the MCP
		// server sends as one text content block.
		if len(mcpResp.Result.Content) != 1 {
			t.Fatalf("my-execute-tool returned %d content blocks, want 1: %v", len(mcpResp.Result.Content), mcpResp.Result.Content)
		}
		if got := mcpResp.Result.Content[0].Text; got != executeEsqlWant {
			t.Fatalf("unexpected value: got %q, want %q", got, executeEsqlWant)
		}
	})
}
