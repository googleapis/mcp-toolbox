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
	"log"
	"os"
	"regexp"
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

func TestMain(m *testing.M) {
	if SpannerProject != "" && SpannerInstance != "" && os.Getenv("SPANNER_EMULATOR_HOST") == "" {
		sweepOrphanedSpannerObjects()
	}
	os.Exit(m.Run())
}

func sweepOrphanedSpannerObjects() {
	sweepCtx, cancel := context.WithTimeout(context.Background(), spannerTeardownTimeout)
	defer cancel()

	adminClient, err := database.NewDatabaseAdminClient(sweepCtx)
	if err != nil {
		log.Printf("spanner orphan sweep: unable to create admin client: %v", err)
		return
	}
	defer func() { _ = adminClient.Close() }()

	now := time.Now()
	if SpannerDatabase != "" {
		dbString := fmt.Sprintf("projects/%s/instances/%s/databases/%s", SpannerProject, SpannerInstance, SpannerDatabase)
		sweepSpannerGoogleSQLDatabase(sweepCtx, adminClient, dbString, now)
	}
	if SpannerPgDatabase != "" && SpannerPgDatabase != SpannerDatabase {
		pgDbString := fmt.Sprintf("projects/%s/instances/%s/databases/%s", SpannerProject, SpannerInstance, SpannerPgDatabase)
		sweepSpannerPostgresDatabase(sweepCtx, adminClient, pgDbString, now)
	}
}

func querySpannerNames(ctx context.Context, client *spanner.Client, sql string) ([]string, error) {
	iter := client.Single().Query(ctx, spanner.NewStatement(sql))
	defer iter.Stop()

	var names []string
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			return names, nil
		}
		if err != nil {
			return nil, err
		}
		var name string
		if err := row.Columns(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
}

func sweepSpannerGoogleSQLDatabase(ctx context.Context, adminClient *database.DatabaseAdminClient, dbString string, now time.Time) {
	dataClient, err := spanner.NewClient(ctx, dbString)
	if err != nil {
		log.Printf("spanner orphan sweep: unable to create client for %s: %v", dbString, err)
		return
	}
	defer dataClient.Close()

	allGraphs, err := querySpannerNames(ctx, dataClient, "SELECT PROPERTY_GRAPH_NAME FROM INFORMATION_SCHEMA.PROPERTY_GRAPHS WHERE PROPERTY_GRAPH_SCHEMA = ''")
	if err != nil {
		log.Printf("spanner orphan sweep: failed listing graphs in %s: %v", dbString, err)
		return
	}
	var staleGraphs []string
	for _, g := range allGraphs {
		if shouldSweepSpannerObject(g, now, spannerTestGraphPrefixes...) {
			staleGraphs = append(staleGraphs, g)
		}
	}

	allTables, err := querySpannerNames(ctx, dataClient, "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = '' AND TABLE_TYPE = 'BASE TABLE'")
	if err != nil {
		log.Printf("spanner orphan sweep: failed listing tables in %s: %v", dbString, err)
		return
	}
	var staleTables []string
	for _, tbl := range allTables {
		if shouldSweepSpannerObject(tbl, now, spannerTestTablePrefixes...) {
			staleTables = append(staleTables, tbl)
		}
	}

	drops := orderSpannerDropStatements(staleGraphs, staleTables, quoteSpannerGoogleSQLIdent)
	if len(drops) == 0 {
		return
	}
	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbString,
		Statements: drops,
	})
	if err != nil {
		log.Printf("spanner orphan sweep: failed starting DDL drop on %s: %v", dbString, err)
		return
	}
	if err := op.Wait(ctx); err != nil {
		log.Printf("spanner orphan sweep: failed completing DDL drop on %s: %v", dbString, err)
	}
}

func sweepSpannerPostgresDatabase(ctx context.Context, adminClient *database.DatabaseAdminClient, dbString string, now time.Time) {
	dataClient, err := spanner.NewClient(ctx, dbString)
	if err != nil {
		log.Printf("spanner orphan sweep: unable to create PG client for %s: %v", dbString, err)
		return
	}
	defer dataClient.Close()

	allTables, err := querySpannerNames(ctx, dataClient, "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'")
	if err != nil {
		log.Printf("spanner orphan sweep: failed listing PG tables in %s: %v", dbString, err)
		return
	}
	var staleTables []string
	for _, tbl := range allTables {
		if shouldSweepSpannerObject(tbl, now, spannerTestTablePrefixes...) {
			staleTables = append(staleTables, tbl)
		}
	}

	drops := orderSpannerDropStatements(nil, staleTables, quoteSpannerPostgresIdent)
	if len(drops) == 0 {
		return
	}
	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbString,
		Statements: drops,
	})
	if err != nil {
		log.Printf("spanner orphan sweep: failed starting PG DDL drop on %s: %v", dbString, err)
		return
	}
	if err := op.Wait(ctx); err != nil {
		log.Printf("spanner orphan sweep: failed completing PG DDL drop on %s: %v", dbString, err)
	}
}

func getSpannerVars(t *testing.T) map[string]any {
	t.Helper()
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

func getSpannerPgVars(t *testing.T) map[string]any {
	t.Helper()
	switch "" {
	case SpannerProject:
		t.Fatal("'SPANNER_PROJECT' not set")
	case SpannerPgDatabase:
		t.Fatal("'SPANNER_PG_DATABASE' not set")
	case SpannerInstance:
		t.Fatal("'SPANNER_INSTANCE' not set")
	}

	return map[string]any{
		"type":     SpannerSourceType,
		"project":  SpannerProject,
		"instance": SpannerInstance,
		"database": SpannerPgDatabase,
		"dialect":  "postgresql",
	}
}

// newSpannerTestClients creates data and admin clients from uncanceled contexts
// and registers t.Cleanup hooks so both clients close after all schema teardown.
func newSpannerTestClients(t *testing.T, project, instance, dbname string) (*spanner.Client, *database.DatabaseAdminClient, string) {
	t.Helper()
	dbString := fmt.Sprintf("projects/%s/instances/%s/databases/%s", project, instance, dbname)

	adminClient, err := database.NewDatabaseAdminClient(context.Background())
	if err != nil {
		t.Fatalf("unable to create new Spanner admin client: %s", err)
	}
	t.Cleanup(func() {
		if closeErr := adminClient.Close(); closeErr != nil {
			t.Errorf("failed to close Spanner admin client: %s", closeErr)
		}
	})

	dataClient, err := spanner.NewClient(context.Background(), dbString)
	if err != nil {
		t.Fatalf("unable to create new Spanner client: %s", err)
	}
	t.Cleanup(func() {
		dataClient.Close()
	})

	return dataClient, adminClient, dbString
}

func TestSpannerToolEndpoints(t *testing.T) {
	sourceConfig := getSpannerVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	dataClient, adminClient, dbString := newSpannerTestClients(t, SpannerProject, SpannerInstance, SpannerDatabase)

	runNow := time.Now()
	tableNameParam := newSpannerTestObjectName("param_table", runNow)
	tableNameAuth := newSpannerTestObjectName("auth_table", runNow)
	tableNameTemplateParam := newSpannerTestObjectName("template_param_table", runNow)
	nodeTableName := newSpannerTestObjectName("node_table", runNow)
	edgeTableName := newSpannerTestObjectName("edge_table", runNow)
	graphName := newSpannerTestObjectName("graph", runNow)
	vectorTableName := newSpannerTestObjectName("vector_table", runNow)
	negDdlTableName := newSpannerTestObjectName("neg_ddl_table", runNow)

	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getSpannerParamToolInfo(tableNameParam)
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getSpannerAuthToolInfo(tableNameAuth)
	createStatementTmpl := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX), age INT64) PRIMARY KEY (id)", tableNameTemplateParam)
	createNodeStmt, createEdgeStmt, createGraphStmt := getSpannerGraphSchemaDDL(nodeTableName, edgeTableName, graphName)
	createVectorStmt := getSpannerVectorTableDDL(vectorTableName)

	createStmts := []string{
		createParamTableStmt,
		createAuthTableStmt,
		createStatementTmpl,
		createNodeStmt,
		createEdgeStmt,
		createGraphStmt,
		createVectorStmt,
	}
	dropStmts := orderSpannerDropStatements(
		[]string{graphName},
		[]string{
			tableNameParam,
			tableNameAuth,
			tableNameTemplateParam,
			nodeTableName,
			edgeTableName,
			vectorTableName,
			negDdlTableName,
		},
		quoteSpannerGoogleSQLIdent,
	)
	applySpannerDdl(t, ctx, adminClient, dbString, createStmts, dropStmts)
	seedSpannerStatements(t, ctx, dataClient,
		spanner.Statement{SQL: insertParamTableStmt, Params: paramTestParams},
		spanner.Statement{SQL: insertAuthTableStmt, Params: authTestParams},
	)

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
	t.Cleanup(cleanup)

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
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
	runSpannerExecuteSqlToolInvokeTest(t, select1Want, invokeParamWant, tableNameParam, negDdlTableName)
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

func TestSpannerPostgresqlToolEndpoints(t *testing.T) {
	sourceConfig := getSpannerPgVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	t.Logf("Using PostgreSQL database: %s", SpannerPgDatabase)
	dataClient, adminClient, dbString := newSpannerTestClients(t, SpannerProject, SpannerInstance, SpannerPgDatabase)

	runNow := time.Now()
	pgParamTable := newSpannerTestObjectName("param_table", runNow)
	pgVectorTable := newSpannerTestObjectName("vector_table", runNow)
	pgNegDdlTable := newSpannerTestObjectName("neg_ddl_table", runNow)

	createPgParamStmt, insertPgParamStmt, pgParamToolStmt := getSpannerPgParamToolInfo(pgParamTable)
	createPgVectorStmt := getSpannerPgVectorTableDDL(pgVectorTable)

	createStmts := []string{createPgParamStmt, createPgVectorStmt}
	dropStmts := orderSpannerDropStatements(
		nil,
		[]string{pgParamTable, pgVectorTable, pgNegDdlTable},
		quoteSpannerPostgresIdent,
	)
	applySpannerDdl(t, ctx, adminClient, dbString, createStmts, dropStmts)
	seedSpannerStatements(t, ctx, dataClient, insertPgParamStmt)

	insertStmt, searchStmt := getSpannerPgVectorSearchStmts(pgVectorTable)
	toolsConfig := buildSpannerPgToolsConfig(t, sourceConfig, pgParamToolStmt)
	toolsConfig = tests.AddSemanticSearchConfig(t, toolsConfig, SpannerToolType, insertStmt, searchStmt)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsConfig, "--enable-api")
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	runSpannerPgToolsTest(t, pgParamTable, pgNegDdlTable, pgParamTable, pgVectorTable)
	tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox")
}
