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

//go:build omni_only

package spanner

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// dropSpannerTools filters the GoogleSQL manifest, removing the named tools.
func dropSpannerTools(names ...string) []tests.MCPToolManifest {
	drop := make(map[string]bool, len(names))
	for _, n := range names {
		drop[n] = true
	}
	var out []tests.MCPToolManifest
	for _, tool := range getSpannerMCPExpectedTools() {
		if !drop[tool.Name] {
			out = append(out, tool)
		}
	}
	return out
}

// setupSpannerOmniMCPServer brings up the Spanner Omni container, creates the
// fixtures and starts a Toolbox server serving them over the MCP endpoint. It
// reports whether the semantic search tools were configured, which depends on
// an embedding API key being available.
func setupSpannerOmniMCPServer(t *testing.T, ctx context.Context) (tableNameParam, tableNameAuth, tableNameTemplateParam, graphName string, semanticSearch bool) {
	endpoint, cleanupContainer := setupSpannerOmniContainer(ctx, t)
	t.Cleanup(cleanupContainer)

	// The container is discarded at the end, so the database, tables, and
	// graph below aren't torn down individually.
	dbName := "omni_test_db"
	dataClient, adminClient, dbString, closeClients := initSpannerOmniClients(ctx, t, endpoint, dbName)
	t.Cleanup(closeClients)
	sourceConfig := getSpannerOmniVars(endpoint, dbName)

	// create table names with UUID
	tableNameParam = "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth = "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam = "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getSpannerParamToolInfo(tableNameParam)
	setupSpannerTable(t, ctx, adminClient, dataClient, createParamTableStmt, insertParamTableStmt, tableNameParam, dbString, paramTestParams)

	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getSpannerAuthToolInfo(tableNameAuth)
	setupSpannerTable(t, ctx, adminClient, dataClient, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, dbString, authTestParams)

	createStatementTmpl := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX), age INT64) PRIMARY KEY (id)", tableNameTemplateParam)
	setupSpannerTable(t, ctx, adminClient, dataClient, createStatementTmpl, "", tableNameTemplateParam, dbString, nil)

	nodeTableName := "node_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createNodeStatementTmpl := fmt.Sprintf("CREATE TABLE %s (id INT64 NOT NULL) PRIMARY KEY (id)", nodeTableName)
	setupSpannerTable(t, ctx, adminClient, dataClient, createNodeStatementTmpl, "", nodeTableName, dbString, nil)

	edgeTableName := "edge_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createEdgeStatementTmpl := fmt.Sprintf(`
	CREATE TABLE %[1]s (
		id INT64 NOT NULL,
		target_id INT64 NOT NULL,
		FOREIGN KEY (target_id) REFERENCES %[2]s (id)
	) PRIMARY KEY (id, target_id),
	 INTERLEAVE IN PARENT %[2]s ON DELETE CASCADE
	`, edgeTableName, nodeTableName)
	setupSpannerTable(t, ctx, adminClient, dataClient, createEdgeStatementTmpl, "", edgeTableName, dbString, nil)

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
	setupSpannerGraph(t, ctx, adminClient, createGraphStmt, graphName, dbString)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, SpannerToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addSpannerExecuteSqlConfig(t, toolsFile)
	toolsFile = addSpannerReadOnlyConfig(t, toolsFile)
	toolsFile = addTemplateParamConfig(t, toolsFile)
	toolsFile = addSpannerListTablesConfig(t, toolsFile)
	toolsFile = addSpannerListGraphsConfig(t, toolsFile)
	// addSpannerSearchCatalogConfig adds a useClientOAuth source, which Omni
	// rejects, so only the plain search catalog tool is added here.
	toolsFile["tools"].(map[string]any)["my-search-catalog-tool"] = map[string]any{
		"type":        "spanner-search-catalog",
		"source":      "my-instance",
		"description": "Searches for data assets in catalog",
	}

	// Semantic search needs a Gemini API key for the embedding model.
	semanticSearch = os.Getenv("API_KEY") != ""
	if semanticSearch {
		vectorTableName, _ := setupSpannerVectorTable(t, ctx, adminClient, dbString)
		insertStmt, searchStmt := getSpannerVectorSearchStmts(vectorTableName)
		toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, SpannerToolType, insertStmt, searchStmt)
	}

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
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

	return tableNameParam, tableNameAuth, tableNameTemplateParam, graphName, semanticSearch
}

func TestSpannerOmniMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)

	_, _, _, _, semanticSearch := setupSpannerOmniMCPServer(t, ctx)

	// Omni rejects the useClientOAuth source, so it registers only the plain
	// search catalog tool, and the semantic search tools are configured only
	// when an embedding API key is available.
	drop := []string{"my-auth-search-catalog-tool", "my-client-auth-search-catalog-tool"}
	if !semanticSearch {
		drop = append(drop, "insert_docs", "search_docs")
	}

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, dropSpannerTools(drop...))
	})
}

func TestSpannerOmniMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)

	tableNameParam, tableNameAuth, tableNameTemplateParam, graphName, semanticSearch := setupSpannerOmniMCPServer(t, ctx)

	select1Want := "[{\"\":\"1\"}]"
	invokeParamWant := "[{\"id\":\"1\",\"name\":\"Alice\"},{\"id\":\"3\",\"name\":\"Sid\"}]"
	accessSchemaWant := "[{\"schema_name\":\"INFORMATION_SCHEMA\"}]"
	toolInvokeMyToolById4Want := `[{"id":"4","name":null}]`
	mcpMyFailToolWant := `"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing GCP request: unable to execute client: unable to parse row: spanner: code = \"InvalidArgument\", desc = \"Syntax error: Unexpected identifier \\\\\\\"SELEC\\\\\\\" [at 1:1]\\\\nSELEC 1;\\\\n^\"`
	mcpMyToolId3NameAliceWant := `{"jsonrpc":"2.0","id":"my-tool","result":{"content":[{"type":"text","text":"{\"id\":\"1\",\"name\":\"Alice\"}"},{"type":"text","text":"{\"id\":\"3\",\"name\":\"Sid\"}"}]}}`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"\":\"1\"}"}]}}`
	tmplSelectAllWwant := "[{\"age\":\"21\",\"id\":\"1\",\"name\":\"Alex\"},{\"age\":\"100\",\"id\":\"2\",\"name\":\"Alice\"}]"
	tmplSelectId1Want := "[{\"age\":\"21\",\"id\":\"1\",\"name\":\"Alex\"}]"

	// These helpers also cover the Toolbox auth services, which need Google
	// credentials (available in Cloud Build) regardless of the database.
	if hasGoogleCredentials(t) {
		tests.RunToolInvokeTest(t, select1Want,
			tests.WithMyToolId3NameAliceWant(invokeParamWant),
			tests.WithMyArrayToolWant(invokeParamWant),
			tests.WithMyToolById4Want(toolInvokeMyToolById4Want),
			tests.WithMCP(),
		)
		tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want,
			tests.WithMcpMyToolId3NameAliceWant(mcpMyToolId3NameAliceWant),
			// Spanner returns INT64 values as JSON strings.
			tests.WithMcpMySecureToolWant(invokeParamWant),
		)
		runSpannerExecuteSqlToolInvokeTest(t, select1Want, invokeParamWant, tableNameParam, tests.WithMCPExec())
	} else {
		t.Log("Google credentials not found; skipping tool invoke, MCP, and execute SQL tests that include auth")
	}
	tests.RunToolInvokeWithTemplateParameters(
		t, tableNameTemplateParam,
		tests.WithSelectAllWant(tmplSelectAllWwant),
		tests.WithTmplSelectId1Want(tmplSelectId1Want),
		tests.DisableDdlTest(),
		tests.WithMCPTemplate(),
	)
	runSpannerSchemaToolInvokeTest(t, accessSchemaWant, tests.WithMCPExec())
	runSpannerListTablesTest(t, tableNameParam, tableNameAuth, tableNameTemplateParam, tests.WithMCPExec())
	runSpannerListGraphsTest(t, graphName, tests.WithMCPExec())
	runSpannerOmniSearchCatalogTest(t, tests.WithMCPExec())
	if semanticSearch {
		tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox", tests.WithMCPExec())
	}
}
