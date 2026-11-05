// Copyright 2024 Google LLC
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

//go:build !omni_only

package spanner

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"google.golang.org/api/iterator"
)

var (
	SpannerProject    = os.Getenv("SPANNER_PROJECT")
	SpannerDatabase   = os.Getenv("SPANNER_DATABASE")
	SpannerInstance   = os.Getenv("SPANNER_INSTANCE")
	SpannerPgDatabase = os.Getenv("SPANNER_PG_DATABASE")
)

func getSpannerVars(t *testing.T) map[string]any {
	switch "" {
	case SpannerProject:
		t.Fatal("'SPANNER_PROJECT' not set")
	case SpannerDatabase:
		t.Fatal("'SPANNER_DATABASE' not set")
	case SpannerInstance:
		t.Fatal("'SPANNER_INSTANCE' not set")
	}

	return map[string]any{
		"type":     SpannerSourceType,
		"project":  SpannerProject,
		"instance": SpannerInstance,
		"database": SpannerDatabase,
	}
}

func initSpannerClients(ctx context.Context, project, instance, dbname string) (*spanner.Client, *database.DatabaseAdminClient, error) {
	// Configure the connection to the database
	db := fmt.Sprintf("projects/%s/instances/%s/databases/%s", project, instance, dbname)

	// Create Spanner client (for queries)
	dataClient, err := spanner.NewClient(ctx, db)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create new Spanner client: %w", err)
	}

	// Create Spanner admin client (for creating databases)
	adminClient, err := database.NewDatabaseAdminClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create new Spanner admin client: %w", err)
	}

	return dataClient, adminClient, nil
}

func stopSpannerServer(ctx context.Context, cmd *tests.CmdExec) {
	cmd.Close()
	exitCtx, exitCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer exitCancel()
	_ = cmd.Wait(exitCtx)
	http.DefaultClient.CloseIdleConnections()
}

// isOrphanedSpannerResource reports whether a test table or property graph with
// the given prefix is either a legacy resource without a timestamp or older
// than maxAge.
func isOrphanedSpannerResource(name, prefix string, now int64, maxAge time.Duration) bool {
	if !strings.HasPrefix(name, prefix+"_") {
		return false
	}
	rest := strings.TrimPrefix(name, prefix+"_")
	parts := strings.SplitN(rest, "_", 2)
	if len(parts) < 2 {
		// Legacy resource name (<prefix>_<uuid>) without a creation timestamp.
		return true
	}
	createdAt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return true
	}
	return now-createdAt > int64(maxAge.Seconds())
}

func runBatchedSpannerDdl(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, dbString string, statements []string) {
	t.Helper()
	if len(statements) == 0 {
		return
	}
	const batchSize = 1000
	for i := 0; i < len(statements); i += batchSize {
		end := i + batchSize
		if end > len(statements) {
			end = len(statements)
		}
		batch := statements[i:end]
		t.Logf("Spanner cleanup: executing DDL batch (%d statements)...", len(batch))
		op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
			Database:   dbString,
			Statements: batch,
		})
		if err != nil {
			t.Logf("warning: unable to start Spanner cleanup DDL batch: %v", err)
			continue
		}
		if err := op.Wait(ctx); err != nil {
			t.Logf("warning: Spanner cleanup DDL batch failed: %v", err)
			continue
		}
	}
}

// cleanupOldSpannerResources drops leaked test property graphs and tables older
// than maxAge in dependency order: property graphs first, interleaved/FK edge
// tables second, and remaining test tables third.
func cleanupOldSpannerResources(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, dataClient *spanner.Client, dbString string, maxAge time.Duration) {
	t.Helper()
	now := time.Now().Unix()

	var graphDrops []string
	graphIter := dataClient.Single().Query(ctx, spanner.Statement{
		SQL: "SELECT PROPERTY_GRAPH_NAME FROM INFORMATION_SCHEMA.PROPERTY_GRAPHS",
	})
	defer graphIter.Stop()
	for {
		row, err := graphIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Logf("warning: failed to query Spanner property graphs for cleanup: %v", err)
			break
		}
		var graphName string
		if err := row.Columns(&graphName); err != nil {
			continue
		}
		if isOrphanedSpannerResource(graphName, "graph", now, maxAge) {
			graphDrops = append(graphDrops, fmt.Sprintf("DROP PROPERTY GRAPH IF EXISTS %s", graphName))
		}
	}

	var edgeDrops []string
	var otherDrops []string
	otherPrefixes := []string{"node_table", "param_table", "auth_table", "template_param_table", "vector_table"}

	tableIter := dataClient.Single().Query(ctx, spanner.Statement{
		SQL: "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ''",
	})
	defer tableIter.Stop()
	for {
		row, err := tableIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Logf("warning: failed to query Spanner tables for cleanup: %v", err)
			break
		}
		var tableName string
		if err := row.Columns(&tableName); err != nil {
			continue
		}
		if isOrphanedSpannerResource(tableName, "edge_table", now, maxAge) {
			edgeDrops = append(edgeDrops, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
			continue
		}
		for _, prefix := range otherPrefixes {
			if isOrphanedSpannerResource(tableName, prefix, now, maxAge) {
				otherDrops = append(otherDrops, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
				break
			}
		}
	}

	runBatchedSpannerDdl(t, ctx, adminClient, dbString, graphDrops)
	runBatchedSpannerDdl(t, ctx, adminClient, dbString, edgeDrops)
	runBatchedSpannerDdl(t, ctx, adminClient, dbString, otherDrops)
}

func cleanupOldSpannerPgTables(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, dataClient *spanner.Client, dbString string, maxAge time.Duration) {
	t.Helper()
	now := time.Now().Unix()

	var drops []string
	tableIter := dataClient.Single().Query(ctx, spanner.Statement{
		SQL: "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public'",
	})
	defer tableIter.Stop()
	for {
		row, err := tableIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Logf("warning: failed to query Spanner PG tables for cleanup: %v", err)
			break
		}
		var tableName string
		if err := row.Columns(&tableName); err != nil {
			continue
		}
		if isOrphanedSpannerResource(tableName, "vector_table", now, maxAge) {
			drops = append(drops, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
		}
	}

	runBatchedSpannerDdl(t, ctx, adminClient, dbString, drops)
}

func TestSpannerToolEndpoints(t *testing.T) {
	sourceConfig := getSpannerVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	// Create Spanner client
	dataClient, adminClient, err := initSpannerClients(ctx, SpannerProject, SpannerInstance, SpannerDatabase)
	if err != nil {
		t.Fatalf("unable to create Spanner client: %s", err)
	}
	defer dataClient.Close()
	defer adminClient.Close()

	dbString := fmt.Sprintf(
		"projects/%s/instances/%s/databases/%s",
		SpannerProject,
		SpannerInstance,
		SpannerDatabase,
	)

	// Clean up orphaned test tables and graphs older than 2 hours
	cleanupOldSpannerResources(t, ctx, adminClient, dataClient, dbString, 2*time.Hour)

	// Create table and graph names with timestamp and UUID
	tableNameParam := newSpannerTestResourceName("param_table")
	tableNameAuth := newSpannerTestResourceName("auth_table")
	tableNameTemplateParam := newSpannerTestResourceName("template_param_table")
	nodeTableName := newSpannerTestResourceName("node_table")
	edgeTableName := newSpannerTestResourceName("edge_table")
	graphName := newSpannerTestResourceName("graph")
	vectorTableName := newSpannerTestResourceName("vector_table")

	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getSpannerParamToolInfo(tableNameParam)
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getSpannerAuthToolInfo(tableNameAuth)
	createStatementTmpl := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX), age INT64) PRIMARY KEY (id)", tableNameTemplateParam)
	createNodeStatementTmpl := fmt.Sprintf("CREATE TABLE %s (id INT64 NOT NULL) PRIMARY KEY (id)", nodeTableName)
	createEdgeStatementTmpl := fmt.Sprintf(`
	CREATE TABLE %[1]s (
		id INT64 NOT NULL,
		target_id INT64 NOT NULL,
		FOREIGN KEY (target_id) REFERENCES %[2]s (id)
	) PRIMARY KEY (id, target_id),
	 INTERLEAVE IN PARENT %[2]s ON DELETE CASCADE
	`, edgeTableName, nodeTableName)
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
	createVectorTableStmt := fmt.Sprintf(`CREATE TABLE %s (
		id INT64,
		content STRING(MAX),
		embedding ARRAY<FLOAT32>
	) PRIMARY KEY (id)`, vectorTableName)

	// Register atomic batched teardown before executing DDL
	defer func() {
		teardownCtx := context.WithoutCancel(ctx)
		dropStatements := []string{
			fmt.Sprintf("DROP PROPERTY GRAPH IF EXISTS %s", graphName),
			fmt.Sprintf("DROP TABLE IF EXISTS %s", edgeTableName),
			fmt.Sprintf("DROP TABLE IF EXISTS %s", nodeTableName),
			fmt.Sprintf("DROP TABLE IF EXISTS %s", vectorTableName),
			fmt.Sprintf("DROP TABLE IF EXISTS %s", tableNameTemplateParam),
			fmt.Sprintf("DROP TABLE IF EXISTS %s", tableNameAuth),
			fmt.Sprintf("DROP TABLE IF EXISTS %s", tableNameParam),
		}
		op, err := adminClient.UpdateDatabaseDdl(teardownCtx, &databasepb.UpdateDatabaseDdlRequest{
			Database:   dbString,
			Statements: dropStatements,
		})
		if err != nil {
			t.Errorf("unable to start batched teardown DDL operation: %s", err)
			return
		}
		if opErr := op.Wait(teardownCtx); opErr != nil {
			t.Errorf("Teardown failed: %s", opErr)
		}
	}()

	// Create all test tables and property graph in a single batched DDL operation
	createOp, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database: dbString,
		Statements: []string{
			createParamTableStmt,
			createAuthTableStmt,
			createStatementTmpl,
			createNodeStatementTmpl,
			createEdgeStatementTmpl,
			createGraphStmt,
			createVectorTableStmt,
		},
	})
	if err != nil {
		t.Fatalf("unable to start batched create DDL operation: %s", err)
	}
	if err := createOp.Wait(ctx); err != nil {
		t.Fatalf("unable to create test tables and graph: %s", err)
	}

	// Insert initial test data in a single transaction
	_, err = dataClient.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		if _, err := txn.Update(ctx, spanner.Statement{SQL: insertParamTableStmt, Params: paramTestParams}); err != nil {
			return err
		}
		_, err := txn.Update(ctx, spanner.Statement{SQL: insertAuthTableStmt, Params: authTestParams})
		return err
	})
	if err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, SpannerToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addSpannerExecuteSqlConfig(t, toolsFile)
	toolsFile = addSpannerReadOnlyConfig(t, toolsFile)
	toolsFile = addTemplateParamConfig(t, toolsFile)
	toolsFile = addSpannerListTablesConfig(t, toolsFile)
	toolsFile = addSpannerListGraphsConfig(t, toolsFile)
	toolsFile = addSpannerSearchCatalogConfig(t, toolsFile)

	// Add semantic search tool config
	insertStmt, searchStmt := getSpannerVectorSearchStmts(vectorTableName)
	toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, SpannerToolType, insertStmt, searchStmt)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()
	defer stopSpannerServer(ctx, cmd)

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Get configs for tests
	select1Want := "[{\"\":\"1\"}]"
	invokeParamWant := "[{\"id\":\"1\",\"name\":\"Alice\"},{\"id\":\"3\",\"name\":\"Sid\"}]"
	accessSchemaWant := "[{\"schema_name\":\"INFORMATION_SCHEMA\"}]"
	toolInvokeMyToolById4Want := `[{"id":"4","name":null}]`
	mcpMyFailToolWant := `"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing GCP request: unable to execute client: unable to parse row: spanner: code = \"InvalidArgument\", desc = \"Syntax error: Unexpected identifier \\\\\\\"SELEC\\\\\\\" [at 1:1]\\\\nSELEC 1;\\\\n^\"`
	mcpMyToolId3NameAliceWant := `{"jsonrpc":"2.0","id":"my-tool","result":{"content":[{"type":"text","text":"{\"id\":\"1\",\"name\":\"Alice\"}"},{"type":"text","text":"{\"id\":\"3\",\"name\":\"Sid\"}"}]}}`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"\":\"1\"}"}]}}`
	tmplSelectAllWwant := "[{\"age\":\"21\",\"id\":\"1\",\"name\":\"Alex\"},{\"age\":\"100\",\"id\":\"2\",\"name\":\"Alice\"}]"
	tmplSelectId1Want := "[{\"age\":\"21\",\"id\":\"1\",\"name\":\"Alex\"}]"

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want,
		tests.WithMyToolId3NameAliceWant(invokeParamWant),
		tests.WithMyArrayToolWant(invokeParamWant),
		tests.WithMyToolById4Want(toolInvokeMyToolById4Want),
	)
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want,
		tests.WithMcpMyToolId3NameAliceWant(mcpMyToolId3NameAliceWant),
		// Spanner returns INT64 values as JSON strings.
		tests.WithMcpMySecureToolWant(invokeParamWant),
	)
	tests.RunToolInvokeWithTemplateParameters(
		t, tableNameTemplateParam,
		tests.WithSelectAllWant(tmplSelectAllWwant),
		tests.WithTmplSelectId1Want(tmplSelectId1Want),
		tests.DisableDdlTest(),
	)
	runSpannerSchemaToolInvokeTest(t, accessSchemaWant)
	runSpannerExecuteSqlToolInvokeTest(t, select1Want, invokeParamWant, tableNameParam)
	runSpannerListTablesTest(t, tableNameParam, tableNameAuth, tableNameTemplateParam)
	runSpannerListGraphsTest(t, graphName)
	tests.RunSearchCatalogToolTest(t, tests.SearchCatalogTestParams{
		ContainerParamName: "databaseIds",
		ContainerName:      SpannerDatabase,
		ProjectID:          SpannerProject,
		TargetName:         tableNameParam,
		WantKey:            "DisplayName",
		AllowEmpty:         true,
		CheckValue:         false,
	})
	tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox")
}

// addSpannerSearchCatalogConfig adds the spanner-search-catalog tool configuration
func addSpannerSearchCatalogConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	sources, ok := config["sources"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get sources from config")
	}
	myInstanceConfig, ok := sources["my-instance"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get my-instance from sources")
	}

	// Create a copy of the source config with client OAuth enabled
	oauthSourceConfig := make(map[string]any)
	for k, v := range myInstanceConfig {
		oauthSourceConfig[k] = v
	}
	oauthSourceConfig["useClientOAuth"] = true
	sources["my-oauth-instance"] = oauthSourceConfig

	// Add tools
	tools["my-search-catalog-tool"] = map[string]any{
		"type":        "spanner-search-catalog",
		"source":      "my-instance",
		"description": "Searches for data assets in catalog",
	}
	tools["my-auth-search-catalog-tool"] = map[string]any{
		"type":        "spanner-search-catalog",
		"source":      "my-instance",
		"description": "Searches for data assets in catalog",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	tools["my-client-auth-search-catalog-tool"] = map[string]any{
		"type":        "spanner-search-catalog",
		"source":      "my-oauth-instance",
		"description": "Searches for data assets in catalog",
	}

	config["tools"] = tools
	config["sources"] = sources
	return config
}

// setupSpannerPgVectorTable creates a vector table in Spanner (PostgreSQL dialect) for semantic search testing
func setupSpannerPgVectorTable(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, dbString string) (string, func(*testing.T)) {
	tableName := newSpannerTestResourceName("vector_table")
	createStatement := fmt.Sprintf(`CREATE TABLE %s (
		id bigint PRIMARY KEY,
		content text,
		embedding float4[]
	)`, tableName)

	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbString,
		Statements: []string{createStatement},
	})
	if err != nil {
		t.Fatalf("unable to start create vector table operation %s: %s", tableName, err)
	}
	err = op.Wait(ctx)
	if err != nil {
		t.Fatalf("unable to create test vector table %s: %s", tableName, err)
	}

	return tableName, func(t *testing.T) {
		teardownCtx := context.WithoutCancel(ctx)
		op, err = adminClient.UpdateDatabaseDdl(teardownCtx, &databasepb.UpdateDatabaseDdlRequest{
			Database:   dbString,
			Statements: []string{fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)},
		})
		if err != nil {
			t.Errorf("unable to start drop %s operation: %s", tableName, err)
			return
		}
		opErr := op.Wait(teardownCtx)
		if opErr != nil {
			t.Errorf("Teardown failed: %s", opErr)
		}
	}
}

// getSpannerPgVectorSearchStmts returns statements for spanner semantic search (PostgreSQL dialect)
func getSpannerPgVectorSearchStmts(vectorTableName string) (string, string) {
	insertStmt := fmt.Sprintf("INSERT INTO %s (id, content, embedding) VALUES (1, $1, $2)", vectorTableName)
	searchStmt := fmt.Sprintf("SELECT id, content, spanner.cosine_distance(embedding, $1::float4[]) AS distance FROM %s ORDER BY distance LIMIT 1", vectorTableName)
	return insertStmt, searchStmt
}

func TestSpannerPostgresqlToolEndpoints(t *testing.T) {
	// Skip if environment variables are not set
	if SpannerProject == "" || SpannerInstance == "" {
		t.Skip("SPANNER_PROJECT or SPANNER_INSTANCE not set, skipping PostgreSQL tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Create admin client
	adminClient, err := database.NewDatabaseAdminClient(ctx)
	if err != nil {
		t.Fatalf("unable to create admin client: %s", err)
	}
	defer adminClient.Close()

	dbName := SpannerPgDatabase
	if dbName == "" {
		dbName = "pg_test_database"
	}
	t.Logf("Using PostgreSQL database: %s", dbName)

	dbString := fmt.Sprintf("projects/%s/instances/%s/databases/%s", SpannerProject, SpannerInstance, dbName)
	dataClient, err := spanner.NewClient(ctx, dbString)
	if err != nil {
		t.Fatalf("unable to create data client: %s", err)
	}
	defer dataClient.Close()

	// Clean up orphaned PostgreSQL vector tables older than 2 hours before creating new ones.
	cleanupOldSpannerPgTables(t, ctx, adminClient, dataClient, dbString, 2*time.Hour)

	// Set up table for semantic search
	vectorTableName, tearDownVectorTable := setupSpannerPgVectorTable(t, ctx, adminClient, dbString)
	defer tearDownVectorTable(t)

	// Add semantic search tool config
	insertStmt, searchStmt := getSpannerPgVectorSearchStmts(vectorTableName)

	config := map[string]any{
		"sources": map[string]any{
			"my-instance": map[string]any{
				"type":     "spanner",
				"project":  SpannerProject,
				"instance": SpannerInstance,
				"database": dbName,
				"dialect":  "postgresql",
			},
		},
		"tools": map[string]any{},
	}

	toolsConfig := tests.AddSemanticSearchConfig(t, config, SpannerToolType, insertStmt, searchStmt)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsConfig, "--enable-api")
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()
	defer stopSpannerServer(ctx, cmd)

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Run semantic search test
	tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox")
}
