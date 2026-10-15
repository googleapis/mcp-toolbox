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

package spanner

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// sqlParamSchema is the input schema shared by the spanner-execute-sql tools.
func sqlParamSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"sql": map[string]any{"type": "string", "description": "The sql to execute."}},
		"required":   []any{"sql"},
	}
}

// noParamSchema is the input schema of a spanner-sql tool that takes no parameters.
func noParamSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []any{},
	}
}

// searchCatalogSchema is the input schema shared by the spanner-search-catalog tools.
func searchCatalogSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{"type": "string", "description": "Prompt representing search intention. Do not rewrite the prompt."},
			"databaseIds": map[string]any{
				"type": "array", "description": "Array of database IDs.", "default": []any{},
				"items": map[string]any{"type": "string", "description": "The IDs of the spanner database."},
			},
			"projectIds": map[string]any{
				"type": "array", "description": "Array of project IDs.", "default": []any{},
				"items": map[string]any{"type": "string", "description": "The IDs of the GCP project."},
			},
			"types": map[string]any{
				"type": "array", "description": "Array of data types to filter by.", "default": []any{},
				"items": map[string]any{"type": "string", "description": "The type of the data. Accepted values are: SERVICE, DATABASE, TABLE, VIEW."},
			},
			// Numbers in the served schema arrive as float64 once decoded.
			"pageSize": map[string]any{"type": "integer", "description": "Number of results in the search page.", "default": float64(5)},
		},
		"required": []any{"prompt"},
	}
}

// getSpannerMCPExpectedTools returns the manifest tools/list serves for the
// GoogleSQL test config: the shared base tools, the subset of the shared
// template parameter tools this source registers, and the Spanner-specific
// tools.
func getSpannerMCPExpectedTools() []tests.MCPToolManifest {
	expected := tests.GetBaseMCPExpectedTools()

	// Spanner registers a third, read-only execute-sql tool alongside the two
	// the shared helper describes.
	expected = append(expected,
		tests.MCPToolManifest{Name: "my-exec-sql-tool", Description: "Tool to execute sql", InputSchema: sqlParamSchema()},
		tests.MCPToolManifest{Name: "my-exec-sql-tool-read-only", Description: "Tool to execute sql", InputSchema: sqlParamSchema()},
		tests.MCPToolManifest{Name: "my-auth-exec-sql-tool", Description: "Tool to execute sql", InputSchema: sqlParamSchema()},
	)

	// addTemplateParamConfig in this package registers five of the seven shared
	// template parameter tools; it has no DDL tools, which is also why the
	// legacy test passes DisableDdlTest.
	for _, tool := range tests.GetTemplateParamMCPExpectedTools() {
		switch tool.Name {
		case "create-table-templateParams-tool", "drop-table-templateParams-tool":
			continue
		}
		expected = append(expected, tool)
	}

	expected = append(expected,
		tests.MCPToolManifest{Name: "access-schema", Description: "Tool to access information schema.", InputSchema: noParamSchema()},
		tests.MCPToolManifest{Name: "access-schema-read-only", Description: "Tool to access information schema in read-only mode.", InputSchema: noParamSchema()},
		tests.MCPToolManifest{
			Name:        "list-tables-tool",
			Description: "Lists tables with their schema information",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"table_names":   map[string]any{"type": "string", "default": "", "description": "Optional: A comma-separated list of table names. If empty, details for all tables in user-accessible schemas will be listed."},
					"output_format": map[string]any{"type": "string", "default": "detailed", "description": "Optional: Use 'simple' to return table names only or use 'detailed' to return the full information schema."},
				},
				"required": []any{},
			},
		},
		tests.MCPToolManifest{
			Name:        "list-graphs-tool",
			Description: "Lists graphs with their schema information",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"graph_names":   map[string]any{"type": "string", "default": "", "description": "Optional: A comma-separated list of graph names. If empty, details for all graphs in user-accessible schemas will be listed."},
					"output_format": map[string]any{"type": "string", "default": "detailed", "description": "Optional: Use 'simple' to return graph names only or use 'detailed' to return the full information schema."},
				},
				"required": []any{},
			},
		},
		tests.MCPToolManifest{Name: "my-search-catalog-tool", Description: "Searches for data assets in catalog", InputSchema: searchCatalogSchema()},
		tests.MCPToolManifest{Name: "my-auth-search-catalog-tool", Description: "Searches for data assets in catalog", InputSchema: searchCatalogSchema()},
		tests.MCPToolManifest{Name: "my-client-auth-search-catalog-tool", Description: "Searches for data assets in catalog", InputSchema: searchCatalogSchema()},
		tests.MCPToolManifest{
			Name:        "insert_docs",
			Description: "Stores content and its vector embedding into the documents table.",
			InputSchema: map[string]any{
				"type": "object",
				// text_to_embed is generated by the embedding model, so it is
				// not part of the served schema.
				"properties": map[string]any{"content": map[string]any{"type": "string", "description": "The text content associated with the vector."}},
				"required":   []any{"content"},
			},
		},
		tests.MCPToolManifest{
			Name:        "search_docs",
			Description: "Finds the most semantically similar document to the query vector.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string", "description": "The text content to search for."}},
				"required":   []any{"query"},
			},
		},
	)

	return expected
}

// setupSpannerMCPServer sets up the test fixtures and starts a Toolbox server
// serving the Spanner GoogleSQL tools over the MCP endpoint. Every teardown
// step is registered with t.Cleanup, so nothing leaks if setup fails partway
// through. Cleanups run LIFO, so the server stops before the objects it
// queries are dropped.
func setupSpannerMCPServer(t *testing.T, ctx context.Context) (tableNameParam, tableNameAuth, tableNameTemplateParam, graphName string) {
	sourceConfig := getSpannerVars(t)

	dataClient, adminClient, err := initSpannerClients(ctx, SpannerProject, SpannerInstance, SpannerDatabase)
	if err != nil {
		t.Fatalf("unable to create Spanner client: %s", err)
	}

	dbString := fmt.Sprintf("projects/%s/instances/%s/databases/%s", SpannerProject, SpannerInstance, SpannerDatabase)

	// create table names with UUID
	tableNameParam = "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth = "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam = "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getSpannerParamToolInfo(tableNameParam)
	teardownTable1 := setupSpannerTable(t, ctx, adminClient, dataClient, createParamTableStmt, insertParamTableStmt, tableNameParam, dbString, paramTestParams)
	t.Cleanup(func() { teardownTable1(t) })

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getSpannerAuthToolInfo(tableNameAuth)
	teardownTable2 := setupSpannerTable(t, ctx, adminClient, dataClient, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, dbString, authTestParams)
	t.Cleanup(func() { teardownTable2(t) })

	// set up data for template param tool
	createStatementTmpl := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX), age INT64) PRIMARY KEY (id)", tableNameTemplateParam)
	teardownTableTmpl := setupSpannerTable(t, ctx, adminClient, dataClient, createStatementTmpl, "", tableNameTemplateParam, dbString, nil)
	t.Cleanup(func() { teardownTableTmpl(t) })

	// set up for graph tool
	nodeTableName := "node_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createNodeStatementTmpl := fmt.Sprintf("CREATE TABLE %s (id INT64 NOT NULL) PRIMARY KEY (id)", nodeTableName)
	teardownNodeTableTmpl := setupSpannerTable(t, ctx, adminClient, dataClient, createNodeStatementTmpl, "", nodeTableName, dbString, nil)
	t.Cleanup(func() { teardownNodeTableTmpl(t) })

	edgeTableName := "edge_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createEdgeStatementTmpl := fmt.Sprintf(`
	CREATE TABLE %[1]s (
		id INT64 NOT NULL,
		target_id INT64 NOT NULL,
		FOREIGN KEY (target_id) REFERENCES %[2]s (id)
	) PRIMARY KEY (id, target_id),
	 INTERLEAVE IN PARENT %[2]s ON DELETE CASCADE
	`, edgeTableName, nodeTableName)
	teardownEdgeTableTmpl := setupSpannerTable(t, ctx, adminClient, dataClient, createEdgeStatementTmpl, "", edgeTableName, dbString, nil)
	t.Cleanup(func() { teardownEdgeTableTmpl(t) })

	graphName = "graph_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createGraphStmt := fmt.Sprintf(`
	CREATE PROPERTY GRAPH %[3]s
		NODE TABLES (
			%[1]s
		)
		EDGE TABLES (
			%[2]s
				SOURCE KEY (id) REFERENCES %[1]s
				DESTINATION KEY (target_id) REFERENCES %[1]s
				LABEL EDGE
		)
	`, nodeTableName, edgeTableName, graphName)
	teardownGraph := setupSpannerGraph(t, ctx, adminClient, createGraphStmt, graphName, dbString)
	t.Cleanup(func() { teardownGraph(t) })

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, SpannerToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addSpannerExecuteSqlConfig(t, toolsFile)
	toolsFile = addSpannerReadOnlyConfig(t, toolsFile)
	toolsFile = addTemplateParamConfig(t, toolsFile)
	toolsFile = addSpannerListTablesConfig(t, toolsFile)
	toolsFile = addSpannerListGraphsConfig(t, toolsFile)
	toolsFile = addSpannerSearchCatalogConfig(t, toolsFile)

	// Set up table for semantic search
	vectorTableName, tearDownVectorTable := setupSpannerVectorTable(t, ctx, adminClient, dbString)
	t.Cleanup(func() { tearDownVectorTable(t) })

	// Add semantic search tool config
	insertStmt, searchStmt := getSpannerVectorSearchStmts(vectorTableName)
	toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, SpannerToolType, insertStmt, searchStmt)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	// Registered last so it runs first. StartCmd's cleanup only removes the
	// temporary config file, so Close is what stops the server and closes its
	// pipes, before the objects it queries are dropped.
	t.Cleanup(func() {
		cmd.Close()
		cleanup()
	})

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	return tableNameParam, tableNameAuth, tableNameTemplateParam, graphName
}

func TestSpannerMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	setupSpannerMCPServer(t, ctx)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getSpannerMCPExpectedTools())
	})
}

func TestSpannerMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	tableNameParam, tableNameAuth, tableNameTemplateParam, graphName := setupSpannerMCPServer(t, ctx)

	// Spanner returns INT64 values as JSON strings, so the row expectations
	// from the legacy test are carried over unchanged.
	select1Want := "[{\"\":\"1\"}]"
	invokeParamWant := "[{\"id\":\"1\",\"name\":\"Alice\"},{\"id\":\"3\",\"name\":\"Sid\"}]"
	accessSchemaWant := "[{\"schema_name\":\"INFORMATION_SCHEMA\"}]"
	toolInvokeMyToolById4Want := `[{"id":"4","name":null}]`
	mcpMyFailToolWant := `"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing GCP request: unable to execute client: unable to parse row: spanner: code = \"InvalidArgument\", desc = \"Syntax error: Unexpected identifier \\\\\\\"SELEC\\\\\\\" [at 1:1]\\\\nSELEC 1;\\\\n^\"`
	mcpMyToolId3NameAliceWant := `{"jsonrpc":"2.0","id":"my-tool","result":{"content":[{"type":"text","text":"{\"id\":\"1\",\"name\":\"Alice\"}"},{"type":"text","text":"{\"id\":\"3\",\"name\":\"Sid\"}"}]}}`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"\":\"1\"}"}]}}`
	tmplSelectAllWwant := "[{\"age\":\"21\",\"id\":\"1\",\"name\":\"Alex\"},{\"age\":\"100\",\"id\":\"2\",\"name\":\"Alice\"}]"
	tmplSelectId1Want := "[{\"age\":\"21\",\"id\":\"1\",\"name\":\"Alex\"}]"

	tests.RunToolInvokeTest(t, select1Want,
		tests.WithMyToolId3NameAliceWant(invokeParamWant),
		tests.WithMyArrayToolWant(invokeParamWant),
		tests.WithMyToolById4Want(toolInvokeMyToolById4Want),
		tests.WithMCP(),
	)
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want,
		tests.WithMcpMyToolId3NameAliceWant(mcpMyToolId3NameAliceWant),
		tests.WithMcpMySecureToolWant(invokeParamWant),
	)
	tests.RunToolInvokeWithTemplateParameters(
		t, tableNameTemplateParam,
		tests.WithSelectAllWant(tmplSelectAllWwant),
		tests.WithTmplSelectId1Want(tmplSelectId1Want),
		tests.DisableDdlTest(),
		tests.WithMCPTemplate(),
	)
	runSpannerSchemaToolInvokeTest(t, accessSchemaWant, tests.WithMCPExec())
	runSpannerExecuteSqlToolInvokeTest(t, select1Want, invokeParamWant, tableNameParam, tests.WithMCPExec())
	runSpannerListTablesTest(t, tableNameParam, tableNameAuth, tableNameTemplateParam, tests.WithMCPExec())
	runSpannerListGraphsTest(t, graphName, tests.WithMCPExec())
	tests.RunSearchCatalogToolTest(t, tests.SearchCatalogTestParams{
		ContainerParamName: "databaseIds",
		ContainerName:      SpannerDatabase,
		ProjectID:          SpannerProject,
		TargetName:         tableNameParam,
		WantKey:            "DisplayName",
		AllowEmpty:         true,
		CheckValue:         false,
	}, tests.WithMCPExec())
	tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox", tests.WithMCPExec())
}
