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

package clickhouse

import (
	"context"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/tests"
)

// clickHouseMCP is the transport used by the MCP tests.
var clickHouseMCP = clickHouseTransport{isMCP: true}

func TestClickHouseMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	_, toolsFile := setupClickHouseToolsTest(t, ctx)
	clickHouseMCP.startServer(t, ctx, toolsFile)

	expectedTools := tests.GetBaseMCPExpectedTools()
	expectedTools = append(expectedTools, getClickHouseExecuteSQLMCPExpectedTools()...)
	expectedTools = append(expectedTools, getClickHouseTemplateParamMCPExpectedTools()...)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, expectedTools)
	})
}

func TestClickHouseMCPTemplateParameters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tableNameTemplateParam, toolsFile := setupClickHouseToolsTest(t, ctx)
	clickHouseMCP.startServer(t, ctx, toolsFile)

	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithMCPTemplate())
}

// TestClickHouseMCPCallTool runs the shared tool invocation fixtures over MCP.
// These fixtures require a Google ID token.
func TestClickHouseMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	_, toolsFile := setupClickHouseToolsTest(t, ctx)
	clickHouseMCP.startServer(t, ctx, toolsFile)

	select1Want, mcpSelect1Want, mcpMyFailToolWant, createTableStatement, nilIdWant := getClickHouseWants()

	tests.RunToolInvokeTest(t, select1Want, tests.WithMCP(), tests.WithMyToolById4Want(nilIdWant))
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, tests.WithMCPSql())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
}

func TestClickHouseMCPSQLTool(t *testing.T) {
	runClickHouseSQLToolTest(t, clickHouseMCP)
}

func TestClickHouseMCPExecuteSQLTool(t *testing.T) {
	runClickHouseExecuteSQLToolTest(t, clickHouseMCP)
}

func TestClickHouseMCPEdgeCases(t *testing.T) {
	runClickHouseEdgeCasesTest(t, clickHouseMCP)
}

func TestClickHouseMCPListDatabasesTool(t *testing.T) {
	runClickHouseListDatabasesToolTest(t, clickHouseMCP)
}

func TestClickHouseMCPListTablesTool(t *testing.T) {
	runClickHouseListTablesToolTest(t, clickHouseMCP)
}

// getClickHouseExecuteSQLMCPExpectedTools returns the MCP manifests for the
// tools loaded by addClickHouseExecuteSqlConfig. The clickhouse-execute-sql
// parameter description differs from the shared execute-sql tools, so
// tests.GetExecuteSQLMCPExpectedTools can't be reused.
func getClickHouseExecuteSQLMCPExpectedTools() []tests.MCPToolManifest {
	inputSchema := map[string]any{
		"type":       "object",
		"properties": map[string]any{"sql": map[string]any{"type": "string", "description": "The SQL statement to execute."}},
		"required":   []any{"sql"},
	}
	return []tests.MCPToolManifest{
		{Name: "my-exec-sql-tool", Description: "Tool to execute sql", InputSchema: inputSchema},
		{Name: "my-auth-exec-sql-tool", Description: "Tool to execute sql", InputSchema: inputSchema},
	}
}

// getClickHouseTemplateParamMCPExpectedTools returns the MCP manifests for the
// ClickHouse-specific template parameter tools loaded by
// addClickHouseTemplateParamConfig.
func getClickHouseTemplateParamMCPExpectedTools() []tests.MCPToolManifest {
	return []tests.MCPToolManifest{
		{
			Name:        "create-table-templateParams-tool",
			Description: "Create table tool with template parameters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"columns":   map[string]any{"description": "The columns to create", "items": map[string]any{"description": "A column name that will be created", "type": "string"}, "type": "array"},
					"tableName": map[string]any{"description": "some description", "type": "string"},
				},
				"required": []any{"tableName", "columns"},
			},
		},
		{
			Name:        "drop-table-templateParams-tool",
			Description: "Drop table tool with template parameters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tableName": map[string]any{"description": "some description", "type": "string"},
				},
				"required": []any{"tableName"},
			},
		},
		{
			Name:        "insert-table-templateParams-tool",
			Description: "Insert table tool with template parameters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"columns":   map[string]any{"description": "The columns to insert into", "items": map[string]any{"description": "A column name that will be returned from the query.", "type": "string"}, "type": "array"},
					"tableName": map[string]any{"description": "some description", "type": "string"},
					"values":    map[string]any{"description": "The values to insert as a comma separated string", "type": "string"},
				},
				"required": []any{"tableName", "columns", "values"},
			},
		},
		{
			Name:        "select-fields-templateParams-tool",
			Description: "Select specific fields tool with template parameters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tableName": map[string]any{"description": "some description", "type": "string"},
				},
				"required": []any{"tableName"},
			},
		},
		{
			Name:        "select-filter-templateParams-combined-tool",
			Description: "Select table tool with filter template parameters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"columnFilter": map[string]any{"description": "some description", "type": "string"},
					"name":         map[string]any{"description": "the name to filter by", "type": "string"},
					"tableName":    map[string]any{"description": "some description", "type": "string"},
				},
				"required": []any{"name", "tableName", "columnFilter"},
			},
		},
		{
			Name:        "select-templateParams-combined-tool",
			Description: "Select table tool with combined template parameters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":        map[string]any{"description": "the id of the user", "type": "integer"},
					"tableName": map[string]any{"description": "some description", "type": "string"},
				},
				"required": []any{"id", "tableName"},
			},
		},
		{
			Name:        "select-templateParams-tool",
			Description: "Select table tool with template parameters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tableName": map[string]any{"description": "some description", "type": "string"},
				},
				"required": []any{"tableName"},
			},
		},
	}
}
