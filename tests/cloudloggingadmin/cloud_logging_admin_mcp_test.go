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

package cloudloggingadmin

import (
	"context"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/tests"
)

// logAdminMCP is the transport used by the MCP tests.
var logAdminMCP = logAdminTransport{isMCP: true}

func TestLogAdminMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	logAdminMCP.startServer(t, ctx, getCloudLoggingAdminToolsConfig(getLogAdminVars(t)))

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getLogAdminMCPExpectedTools())
	})
}

func TestLogAdminMCPToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	toolsFile, logName := setupLogAdminTest(t, ctx)
	logAdminMCP.startServer(t, ctx, toolsFile)

	runLogAdminTests(t, ctx, logAdminMCP, logName)
}

// getLogAdminMCPExpectedTools returns the MCP manifests for the tools loaded by
// getCloudLoggingAdminToolsConfig.
func getLogAdminMCPExpectedTools() []tests.MCPToolManifest {
	return []tests.MCPToolManifest{
		{
			Name:        "auth-list-log-names",
			Description: "Lists log names with authentication",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{"description": "Maximum number of log entries to return. Default: 200.", "type": "integer"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list-log-names",
			Description: "Lists log names in the project",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{"description": "Maximum number of log entries to return. Default: 200.", "type": "integer"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list-resource-types",
			Description: "Lists monitored resource types",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "query-logs",
			Description: "Queries log entries",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"endTime":     map[string]any{"description": "End time in RFC3339 format (e.g., 2025-12-09T23:59:59Z). Defaults to now.", "type": "string"},
					"filter":      map[string]any{"description": "Cloud Logging filter query. Common fields: resource.type, resource.labels.*, logName, severity, textPayload, jsonPayload.*, protoPayload.*, labels.*, httpRequest.*. Operators: =, !=, <, <=, >, >=, :, =~, AND, OR, NOT.", "type": "string"},
					"limit":       map[string]any{"description": "Maximum number of log entries to return. Default: 200.", "type": "integer"},
					"newestFirst": map[string]any{"description": "Set to true for newest logs first. Defaults to oldest first.", "type": "boolean"},
					"startTime":   map[string]any{"description": "Start time in RFC3339 format (e.g., 2025-12-09T00:00:00Z). Defaults to 30 days ago.", "type": "string"},
					"verbose":     map[string]any{"description": "Include additional fields (insertId, trace, spanId, httpRequest, labels, operation, sourceLocation). Defaults to false.", "type": "boolean"},
				},
				"required": []any{},
			},
		},
	}
}
