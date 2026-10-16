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

package mindsdb

import (
	"context"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/tests"
)

// mindsDBMCP is the transport used by the MCP tests.
var mindsDBMCP = mindsDBTransport{isMCP: true}

func TestMindsDBMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	toolsFile := setupMindsDBTest(t, ctx)
	mindsDBMCP.startServer(t, ctx, toolsFile)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getMindsDBMCPExpectedTools())
	})
}

func TestMindsDBMCPToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	toolsFile := setupMindsDBTest(t, ctx)
	mindsDBMCP.startServer(t, ctx, toolsFile)

	tests.RunToolInvokeTest(t, mindsDBSelect1Want,
		tests.DisableArrayTest(), // MindsDB doesn't support array parameters
		tests.WithMCP(),
	)

	runMindsDBTests(t, ctx, mindsDBMCP)
}

// getMindsDBMCPExpectedTools returns the MCP manifests for the tools loaded by
// getMindsDBToolsConfig. my-secure-tool is not listed because tools with secure
// parameters are excluded from tools/list in this protocol version.
func getMindsDBMCPExpectedTools() []tests.MCPToolManifest {
	return []tests.MCPToolManifest{
		{
			Name:        "my-array-tool",
			Description: "Tool to test invocation with array params.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-auth-exec-sql-tool",
			Description: "Tool to execute sql with auth",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sql": map[string]any{"description": "The sql to execute.", "type": "string"},
				},
				"required": []any{"sql"},
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
			Name:        "my-exec-sql-tool",
			Description: "Tool to execute sql",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sql": map[string]any{"description": "The sql to execute.", "type": "string"},
				},
				"required": []any{"sql"},
			},
		},
		{
			Name:        "my-fail-tool",
			Description: "Tool to test statement with incorrect syntax.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
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
					"id":   map[string]any{"description": "user ID", "type": "integer"},
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
					"id": map[string]any{"description": "user ID", "type": "integer"},
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
	}
}
