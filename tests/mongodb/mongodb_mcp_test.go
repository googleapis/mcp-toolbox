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

package mongodb

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// startMongoDBMCPServer starts a MongoDB container, seeds the test collection,
// starts the toolbox server with the MongoDB tools config and waits until it is
// ready to serve.
func startMongoDBMCPServer(t *testing.T, ctx context.Context) {
	t.Helper()
	uri := setupMongoDBInstance(ctx, t)
	seedMongoDB(ctx, t, uri)

	cmd, cleanup, err := tests.StartCmd(ctx, getMongoDBToolsConfig(getMongoDBVars(uri), MongoDbToolType))
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
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
}

// mcpResultValue decodes the MCP content blocks into the value the REST
// endpoint returns. The MCP server emits one block per element when a tool
// returns a []any (e.g. query rows), so the blocks are parsed and flattened
// into a single array.
func mcpResultValue(t *testing.T, resp *tests.MCPCallToolResponse) []any {
	t.Helper()
	values := []any{}
	for _, content := range resp.Result.Content {
		var item any
		if err := json.Unmarshal([]byte(content.Text), &item); err != nil {
			values = append(values, content.Text)
			continue
		}
		if slice, ok := item.([]any); ok {
			values = append(values, slice...)
		} else {
			values = append(values, item)
		}
	}
	return values
}

// wantResultValue decodes a REST result string into the flattened form
// produced by mcpResultValue. A scalar result (e.g. a count or an inserted id)
// is a single content block, so it is compared as a one-element array.
func wantResultValue(t *testing.T, want string) []any {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(want), &value); err != nil {
		t.Fatalf("unable to parse want %q: %s", want, err)
	}
	if slice, ok := value.([]any); ok {
		return slice
	}
	return []any{value}
}

func TestMongoDBMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	startMongoDBMCPServer(t, ctx)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getMongoDBMCPExpectedTools())
	})
}

// TestMongoDBMCPInvokeTool runs the delete, insert, update, aggregate and
// runtime collection invocations over MCP. These cases need no auth token.
func TestMongoDBMCPInvokeTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	startMongoDBMCPServer(t, ctx)

	for _, tc := range getMongoDBInvokeTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, tc.toolName, tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing %s: %s", tc.toolName, err)
			}
			if statusCode != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, statusCode)
			}

			if tc.wantMCPErr != "" {
				tests.AssertMCPError(t, mcpResp, tc.wantMCPErr)
				return
			}

			if mcpResp.Error != nil {
				t.Fatalf("%s returned JSON-RPC error: %v", tc.toolName, mcpResp.Error)
			}
			if mcpResp.Result.IsError {
				t.Fatalf("%s returned error result: %v", tc.toolName, mcpResp.Result)
			}
			got := mcpResultValue(t, mcpResp)
			want := wantResultValue(t, tc.want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("unexpected value: got %v, want %v", got, want)
			}
		})
	}
}

// TestMongoDBMCPCallTool runs the shared tool invocation fixtures over MCP.
// These fixtures require a Google ID token.
func TestMongoDBMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	startMongoDBMCPServer(t, ctx)

	tests.RunToolInvokeTest(t, mongoDBSelect1Want,
		tests.WithMCP(),
		tests.WithMyToolId3NameAliceWant(mongoDBMyToolId3NameAliceWant),
		tests.WithMyArrayToolWant(mongoDBMyToolId3NameAliceWant),
		tests.WithMyToolById4Want(mongoDBMyToolById4Want),
	)
	tests.RunMCPToolCallMethod(t, mongoDBMcpMyFailToolWant, mongoDBSelect1Want,
		tests.WithMcpMyToolId3NameAliceWant(mongoDBMcpMyToolId3NameAliceWant),
		tests.WithMcpSelect1Want(mongoDBMcpAuthRequiredWant),
	)
}

// getMongoDBMCPExpectedTools returns the MCP manifests for the tools loaded by
// getMongoDBToolsConfig. my-secure-tool is not listed because tools with secure
// parameters are excluded from tools/list in this protocol version.
func getMongoDBMCPExpectedTools() []tests.MCPToolManifest {
	return []tests.MCPToolManifest{
		{
			Name:        "my-aggregate-tool",
			Description: "Tool to test an aggregation.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"description": "user name", "type": "string"},
				},
				"required": []any{"name"},
			},
		},
		{
			Name:        "my-array-tool",
			Description: "Tool to test invocation with array.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"nameArray": map[string]any{"description": "user names", "items": map[string]any{"description": "string item", "type": "string"}, "type": "array"},
				},
				"required": []any{"nameArray"},
			},
		},
		{
			Name:        "my-auth-required-tool",
			Description: "Tool to test auth required invocation.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-auth-tool",
			Description: "Tool to test authenticated parameters.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"email": map[string]any{"description": "user email", "type": "string"},
				},
				"required": []any{"email"},
			},
		},
		{
			Name:        "my-delete-many-tool",
			Description: "Tool to test deleting multiple entries.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-delete-one-tool",
			Description: "Tool to test deleting an entry.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-fail-tool",
			Description: "Tool to test statement with incorrect syntax.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-insert-many-tool",
			Description: "Tool to test inserting multiple entries.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"data": map[string]any{"description": "the JSON payload to insert, should be a JSON array of documents", "type": "string"},
				},
				"required": []any{"data"},
			},
		},
		{
			Name:        "my-insert-one-tool",
			Description: "Tool to test inserting an entry.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"data": map[string]any{"description": "the JSON payload to insert, should be a JSON object", "type": "string"},
				},
				"required": []any{"data"},
			},
		},
		{
			Name:        "my-read-only-aggregate-tool",
			Description: "Tool to test an aggregation.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"description": "user name", "type": "string"},
				},
				"required": []any{"name"},
			},
		},
		{
			Name:        "my-read-write-aggregate-tool",
			Description: "Tool to test an aggregation.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"description": "user name", "type": "string"},
				},
				"required": []any{"name"},
			},
		},
		{
			Name:        "my-runtime-collection-tool",
			Description: "Tool to test runtime collection selection.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"collection": map[string]any{"description": "The name of the collection to operate on.", "type": "string"},
					"id":         map[string]any{"description": "user id", "type": "integer"},
				},
				"required": []any{"id", "collection"},
			},
		},
		{
			Name:        "my-simple-tool",
			Description: "Simple tool to test end to end functionality.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-tool",
			Description: "Tool to test invocation with params.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":   map[string]any{"description": "user id", "type": "integer"},
					"name": map[string]any{"description": "user name", "type": "string"},
				},
				"required": []any{"id", "name"},
			},
		},
		{
			Name:        "my-tool-by-id",
			Description: "Tool to test invocation with params.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"description": "user id", "type": "integer"},
				},
				"required": []any{"id"},
			},
		},
		{
			Name:        "my-tool-by-name",
			Description: "Tool to test invocation with params.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"description": "user name", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "my-update-many-tool",
			Description: "Tool to test updating multiple entries.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":   map[string]any{"description": "id", "type": "integer"},
					"name": map[string]any{"description": "user name", "type": "string"},
				},
				"required": []any{"id", "name"},
			},
		},
		{
			Name:        "my-update-one-tool",
			Description: "Tool to test updating an entry.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":   map[string]any{"description": "id", "type": "integer"},
					"name": map[string]any{"description": "user name", "type": "string"},
				},
				"required": []any{"id", "name"},
			},
		},
	}
}
