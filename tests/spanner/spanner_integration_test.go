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

package spanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	SpannerSourceType = "spanner"
	SpannerToolType   = "spanner-sql"
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
	dataClient, err := spanner.NewClient(context.Background(), db)
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

func TestSpannerToolEndpoints(t *testing.T) {
	sourceConfig := getSpannerVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	// Create Spanner client
	dataClient, adminClient, err := initSpannerClients(ctx, SpannerProject, SpannerInstance, SpannerDatabase)
	if err != nil {
		t.Fatalf("unable to create Spanner client: %s", err)
	}

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getSpannerParamToolInfo(tableNameParam)
	dbString := fmt.Sprintf(
		"projects/%s/instances/%s/databases/%s",
		SpannerProject,
		SpannerInstance,
		SpannerDatabase,
	)
	teardownTable1 := setupSpannerTable(t, ctx, adminClient, dataClient, createParamTableStmt, insertParamTableStmt, tableNameParam, dbString, paramTestParams)
	defer teardownTable1(t)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getSpannerAuthToolInfo(tableNameAuth)
	teardownTable2 := setupSpannerTable(t, ctx, adminClient, dataClient, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, dbString, authTestParams)
	defer teardownTable2(t)

	// set up data for template param tool
	createStatementTmpl := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX), age INT64) PRIMARY KEY (id)", tableNameTemplateParam)
	teardownTableTmpl := setupSpannerTable(t, ctx, adminClient, dataClient, createStatementTmpl, "", tableNameTemplateParam, dbString, nil)
	defer teardownTableTmpl(t)

	// set up for graph tool
	nodeTableName := "node_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createNodeStatementTmpl := fmt.Sprintf("CREATE TABLE %s (id INT64 NOT NULL) PRIMARY KEY (id)", nodeTableName)
	teardownNodeTableTmpl := setupSpannerTable(t, ctx, adminClient, dataClient, createNodeStatementTmpl, "", nodeTableName, dbString, nil)
	defer teardownNodeTableTmpl(t)

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
	defer teardownEdgeTableTmpl(t)

	graphName := "graph_" + strings.ReplaceAll(uuid.New().String(), "-", "")
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
	defer teardownGraph(t)

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
	defer tearDownVectorTable(t)

	// Add semantic search tool config
	insertStmt, searchStmt := getSpannerVectorSearchStmts(vectorTableName)
	toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, SpannerToolType, insertStmt, searchStmt)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

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
	mcpMyFailToolWant := `"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"unable to execute client: unable to parse row: spanner: code = \"InvalidArgument\", desc = \"Syntax error: Unexpected identifier \\\\\\\"SELEC\\\\\\\" [at 1:1]\\\\nSELEC 1;\\\\n^\"`
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
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want, tests.WithMcpMyToolId3NameAliceWant(mcpMyToolId3NameAliceWant))
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

// getSpannerToolInfo returns statements and param for my-tool for spanner-sql type
func getSpannerParamToolInfo(tableName string) (string, string, string, string, string, string, map[string]any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX)) PRIMARY KEY (id)", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (id, name) VALUES (1, @name1), (2, @name2), (3, @name3), (4, @name4)", tableName)
	toolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = @id OR name = @name", tableName)
	idToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = @id", tableName)
	nameToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = @name", tableName)
	arrayToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id IN UNNEST(@idArray) AND name IN UNNEST(@nameArray)", tableName)
	params := map[string]any{"name1": "Alice", "name2": "Jane", "name3": "Sid", "name4": nil}
	return createStatement, insertStatement, toolStatement, idToolStatement, nameToolStatement, arrayToolStatement, params
}

// getSpannerAuthToolInfo returns statements and param of my-auth-tool for spanner-sql type
func getSpannerAuthToolInfo(tableName string) (string, string, string, map[string]any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX), email STRING(MAX)) PRIMARY KEY (id)", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (id, name, email) VALUES (1, @name1, @email1), (2, @name2, @email2)", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = @email", tableName)
	params := map[string]any{
		"name1":  "Alice",
		"email1": tests.ServiceAccountEmail,
		"name2":  "Jane",
		"email2": "janedoe@gmail.com",
	}
	return createStatement, insertStatement, toolStatement, params
}

// setupSpannerTable creates and inserts data into a table of tool
// compatible with spanner-sql tool
func setupSpannerTable(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, dataClient *spanner.Client, createStatement, insertStatement, tableName, dbString string, params map[string]any) func(*testing.T) {

	// Create table
	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbString,
		Statements: []string{createStatement},
	})
	if err != nil {
		t.Fatalf("unable to start create table operation %s: %s", tableName, err)
	}
	err = op.Wait(ctx)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	if insertStatement != "" {
		_, err = dataClient.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			stmt := spanner.Statement{
				SQL:    insertStatement,
				Params: params,
			}
			_, err := txn.Update(ctx, stmt)
			return err
		})
		if err != nil {
			t.Fatalf("unable to insert test data: %s", err)
		}
	}

	return func(t *testing.T) {
		// tear down test
		op, err = adminClient.UpdateDatabaseDdl(context.WithoutCancel(ctx), &databasepb.UpdateDatabaseDdlRequest{
			Database:   dbString,
			Statements: []string{fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)},
		})
		if err != nil {
			t.Errorf("unable to start drop %s operation: %s", tableName, err)
			return
		}

		opErr := op.Wait(context.WithoutCancel(ctx))
		if opErr != nil {
			t.Errorf("Teardown failed: %s", opErr)
		}
	}
}

// setupSpannerGraph creates a graph and inserts data into it.
func setupSpannerGraph(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, createStatement, graphName, dbString string) func(*testing.T) {
	// Create graph
	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbString,
		Statements: []string{createStatement},
	})
	if err != nil {
		t.Fatalf("unable to start create graph operation %s: %s", graphName, err)
	}
	err = op.Wait(ctx)
	if err != nil {
		t.Fatalf("unable to create test graph %s: %s", graphName, err)
	}

	return func(t *testing.T) {
		// tear down test
		op, err = adminClient.UpdateDatabaseDdl(context.WithoutCancel(ctx), &databasepb.UpdateDatabaseDdlRequest{
			Database:   dbString,
			Statements: []string{fmt.Sprintf("DROP PROPERTY GRAPH IF EXISTS %s", graphName)},
		})
		if err != nil {
			t.Errorf("unable to start drop %s operation: %s", graphName, err)
			return
		}

		opErr := op.Wait(context.WithoutCancel(ctx))
		if opErr != nil {
			t.Errorf("Teardown failed: %s", opErr)
		}
	}
}

// addSpannerExecuteSqlConfig gets the tools config for `spanner-execute-sql`
func addSpannerExecuteSqlConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool-read-only"] = map[string]any{
		"type":        "spanner-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"readOnly":    true,
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "spanner-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "spanner-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	config["tools"] = tools
	return config
}

func addSpannerReadOnlyConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["access-schema-read-only"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Tool to access information schema in read-only mode.",
		"statement":   "SELECT schema_name FROM `INFORMATION_SCHEMA`.SCHEMATA WHERE schema_name='INFORMATION_SCHEMA';",
		"readOnly":    true,
	}
	tools["access-schema"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Tool to access information schema.",
		"statement":   "SELECT schema_name FROM `INFORMATION_SCHEMA`.SCHEMATA WHERE schema_name='INFORMATION_SCHEMA';",
	}
	config["tools"] = tools
	return config
}

// addSpannerListTablesConfig adds the spanner-list-tables tool configuration
func addSpannerListTablesConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}

	// Add spanner-list-tables tool
	tools["list-tables-tool"] = map[string]any{
		"type":        "spanner-list-tables",
		"source":      "my-instance",
		"description": "Lists tables with their schema information",
	}

	config["tools"] = tools
	return config
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

// addSpannerListGraphsConfig adds the spanner-list-graphs tool configuration
func addSpannerListGraphsConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}

	// Add spanner-list-graphs tool
	tools["list-graphs-tool"] = map[string]any{
		"type":        "spanner-list-graphs",
		"source":      "my-instance",
		"description": "Lists graphs with their schema information",
	}

	config["tools"] = tools
	return config
}

func addTemplateParamConfig(t *testing.T, config map[string]any) map[string]any {
	toolsMap, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	toolsMap["insert-table-templateParams-tool"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Insert tool with template parameters",
		"statement":   "INSERT INTO {{.tableName}} ({{array .columns}}) VALUES ({{.values}})",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewArrayParameter("columns", "The columns to insert into", parameters.NewStringParameter("column", "A column name that will be returned from the query.")),
			parameters.NewStringParameter("values", "The values to insert as a comma separated string"),
		},
	}
	toolsMap["select-templateParams-tool"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT * FROM {{.tableName}}",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-templateParams-combined-tool"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT * FROM {{.tableName}} WHERE id = @id",
		"parameters":  []parameters.Parameter{parameters.NewIntParameter("id", "the id of the user")},
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-fields-templateParams-tool"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT {{array .fields}} FROM {{.tableName}}",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewArrayParameter("fields", "The fields to select from", parameters.NewStringParameter("field", "A field that will be returned from the query.")),
		},
	}
	toolsMap["select-filter-templateParams-combined-tool"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT * FROM {{.tableName}} WHERE {{.columnFilter}} = @name",
		"parameters":  []parameters.Parameter{parameters.NewStringParameter("name", "the name of the user")},
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewStringParameter("columnFilter", "some description"),
		},
	}
	config["tools"] = toolsMap
	return config
}

func runSpannerExecuteSqlToolInvokeTest(t *testing.T, select1Want, invokeParamWant, tableNameParam string) {
	// Get ID token
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Test tool invoke endpoint
	invokeTcs := []struct {
		name          string
		api           string
		requestHeader map[string]string
		requestBody   io.Reader
		want          string
		isErr         bool
	}{
		{
			name:          "invoke my-exec-sql-tool-read-only",
			api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1"}`)),
			want:          select1Want,
			isErr:         false,
		},
		{
			name:          "invoke my-exec-sql-tool-read-only with data present in table",
			api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"sql\":\"SELECT * FROM %s WHERE id = 3 OR name = 'Alice'\"}", tableNameParam))),
			want:          invokeParamWant,
			isErr:         false,
		},
		{
			name:          "invoke my-exec-sql-tool-read-only create table",
			api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{"sql":"CREATE TABLE t (id SERIAL PRIMARY KEY, name TEXT)"}`)),
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool-read-only drop table",
			api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{"sql":"DROP TABLE t"}`)),
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool-read-only insert entry",
			api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"sql\":\"INSERT INTO %s (id, name) VALUES (4, 'test_name')\"}", tableNameParam))),
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool without body",
			api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{}`)),
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool",
			api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1"}`)),
			want:          select1Want,
			isErr:         false,
		},
		{
			name:          "invoke my-exec-sql-tool create table",
			api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{"sql":"CREATE TABLE t (id SERIAL PRIMARY KEY, name TEXT)"}`)),
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool drop table",
			api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{"sql":"DROP TABLE t"}`)),
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool insert entry",
			api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"sql\":\"INSERT INTO %s (id, name) VALUES (5, 'test_name')\"}", tableNameParam))),
			want:          "[]",
			isErr:         false,
		},
		{
			name:          "invoke my-exec-sql-tool without body",
			api:           "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{}`)),
			isErr:         true,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool with auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-exec-sql-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1"}`)),
			isErr:         false,
			want:          select1Want,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool with invalid auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-exec-sql-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			requestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1"}`)),
			isErr:         true,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool without auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-exec-sql-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{"sql":"SELECT 1"}`)),
			isErr:         true,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			// Send Tool invocation request
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")
			for k, v := range tc.requestHeader {
				req.Header.Add(k, v)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			// Check response body
			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if got != tc.want {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}

// Helper function to verify table list results
func verifyTableListResult(t *testing.T, body map[string]interface{}, expectedTables []string, expectedSimpleFormat bool) {
	// Parse the result
	result, ok := body["result"].(string)
	if !ok {
		t.Fatalf("unable to find result in response body")
	}

	var tables []interface{}
	err := json.Unmarshal([]byte(result), &tables)
	if err != nil {
		t.Fatalf("unable to parse result as JSON array: %s", err)
	}

	// If we expect specific tables, verify they exist
	if len(expectedTables) > 0 {
		tableNames := make(map[string]bool)
		requiredKeys := []string{"schema_name", "object_name", "object_type", "columns", "constraints", "indexes"}
		if expectedSimpleFormat {
			requiredKeys = []string{"name"}
		}

		for _, table := range tables {
			tableMap, ok := table.(map[string]interface{})
			if !ok {
				continue
			}

			objectDetails, ok := tableMap["object_details"].(map[string]interface{})
			if !ok {
				t.Fatalf("object_details is not of type map[string]interface{}, got: %T", tableMap["object_details"])
			}

			for _, reqKey := range requiredKeys {
				if _, hasKey := objectDetails[reqKey]; !hasKey {
					t.Errorf("missing required key '%s', for object_details: %v", reqKey, objectDetails)
				}
			}

			if name, ok := tableMap["object_name"].(string); ok {
				tableNames[name] = true
			}
		}

		for _, expected := range expectedTables {
			if !tableNames[expected] {
				t.Errorf("expected table %s not found in results", expected)
			}
		}
	}
}

// runSpannerListTablesTest tests the spanner-list-tables tool
func runSpannerListTablesTest(t *testing.T, tableNameParam, tableNameAuth, tableNameTemplateParam string) {
	invokeTcs := []struct {
		name            string
		requestBody     io.Reader
		expectedTables  []string // empty means don't check specific tables
		useSimpleFormat bool
	}{
		{
			name:           "list all tables with detailed format",
			requestBody:    bytes.NewBuffer([]byte(`{}`)),
			expectedTables: []string{tableNameParam, tableNameAuth, tableNameTemplateParam},
		},
		{
			name:            "list tables with simple format",
			requestBody:     bytes.NewBuffer([]byte(`{"output_format": "simple"}`)),
			expectedTables:  []string{tableNameParam, tableNameAuth, tableNameTemplateParam},
			useSimpleFormat: true,
		},
		{
			name:           "list specific tables",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"table_names": "%s,%s"}`, tableNameParam, tableNameAuth))),
			expectedTables: []string{tableNameParam, tableNameAuth},
		},
		{
			name:           "list non-existent table",
			requestBody:    bytes.NewBuffer([]byte(`{"table_names": "non_existent_table_xyz"}`)),
			expectedTables: []string{},
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			// Use RunRequest helper function from tests package
			url := "http://127.0.0.1:5000/api/tool/list-tables-tool/invoke"
			headers := map[string]string{}

			resp, respBody := tests.RunRequest(t, http.MethodPost, url, tc.requestBody, headers)

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
			}

			// Check response body
			var body map[string]interface{}
			err := json.Unmarshal(respBody, &body)
			if err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}

			verifyTableListResult(t, body, tc.expectedTables, tc.useSimpleFormat)
		})
	}
}

// Helper function to verify graph list results
func verifyGraphListResult(t *testing.T, body map[string]interface{}, expectedGraphs []string, expectedSimpleFormat bool) {
	// Parse the result
	result, ok := body["result"].(string)
	if !ok {
		t.Fatalf("unable to find result in response body")
	}

	var graphs []interface{}
	err := json.Unmarshal([]byte(result), &graphs)
	if err != nil {
		t.Fatalf("unable to parse result as JSON array: %s", err)
	}

	// If we expect specific graphs, verify they exist
	if len(expectedGraphs) > 0 {
		graphNames := make(map[string]bool)
		requiredKeys := []string{"schema_name", "object_name", "catalog", "node_tables", "edge_tables", "labels", "property_declarations"}
		if expectedSimpleFormat {
			requiredKeys = []string{"name"}
		}

		for _, graph := range graphs {
			graphMap, ok := graph.(map[string]interface{})
			if !ok {
				continue
			}

			objectDetails, ok := graphMap["object_details"].(map[string]interface{})
			if !ok {
				t.Fatalf("object_details is not of type map[string]interface{}, got: %T", graphMap["object_details"])
			}
			for _, reqKey := range requiredKeys {
				if _, hasKey := objectDetails[reqKey]; !hasKey {
					t.Errorf("missing required key '%s', for object_details: %v", reqKey, objectDetails)
				}
			}

			if name, ok := graphMap["object_name"].(string); ok {
				graphNames[name] = true
			}
		}

		for _, expected := range expectedGraphs {
			if !graphNames[expected] {
				t.Errorf("expected graph %s not found in results", expected)
			}
		}
	}
}

// runSpannerListGraphsTest tests the spanner-list-graphs tool
func runSpannerListGraphsTest(t *testing.T, graphName string) {
	invokeTcs := []struct {
		name            string
		requestBody     io.Reader
		expectedGraphs  []string // empty means don't check specific graphs
		useSimpleFormat bool
	}{
		{
			name:           "list all graphs with detailed format",
			requestBody:    bytes.NewBuffer([]byte(`{}`)),
			expectedGraphs: []string{graphName},
		},
		{
			name:            "list graphs with simple format",
			requestBody:     bytes.NewBuffer([]byte(`{"output_format": "simple"}`)),
			expectedGraphs:  []string{graphName},
			useSimpleFormat: true,
		},
		{
			name:           "list specific graphs",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"graph_names": "%s"}`, graphName))),
			expectedGraphs: []string{graphName},
		},
		{
			name:           "list non-existent graph",
			requestBody:    bytes.NewBuffer([]byte(`{"graph_names": "non_existent_graph_xyz"}`)),
			expectedGraphs: []string{},
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			// Use RunRequest helper function from tests package
			url := "http://127.0.0.1:5000/api/tool/list-graphs-tool/invoke"
			headers := map[string]string{}

			resp, respBody := tests.RunRequest(t, http.MethodPost, url, tc.requestBody, headers)

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
			}

			// Check response body
			var body map[string]interface{}
			err := json.Unmarshal(respBody, &body)
			if err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}

			verifyGraphListResult(t, body, tc.expectedGraphs, tc.useSimpleFormat)
		})
	}
}

// setupSpannerVectorTable creates a vector table in Spanner for semantic search testing
func setupSpannerVectorTable(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, dbString string) (string, func(*testing.T)) {
	tableName := "vector_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createStatement := fmt.Sprintf(`CREATE TABLE %s (
		id INT64,
		content STRING(MAX),
		embedding ARRAY<FLOAT32>
	) PRIMARY KEY (id)`, tableName)

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
		op, err = adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
			Database:   dbString,
			Statements: []string{fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)},
		})
		if err != nil {
			t.Errorf("unable to start drop %s operation: %s", tableName, err)
			return
		}
		opErr := op.Wait(ctx)
		if opErr != nil {
			t.Errorf("Teardown failed: %s", opErr)
		}
	}
}

// getSpannerVectorSearchStmts returns statements for spanner semantic search
func getSpannerVectorSearchStmts(vectorTableName string) (string, string) {
	insertStmt := fmt.Sprintf("INSERT INTO %s (id, content, embedding) VALUES (1, @content, @text_to_embed)", vectorTableName)
	searchStmt := fmt.Sprintf("SELECT id, content, COSINE_DISTANCE(embedding, @query) AS distance FROM %s ORDER BY distance LIMIT 1", vectorTableName)
	return insertStmt, searchStmt
}

func runSpannerSchemaToolInvokeTest(t *testing.T, accessSchemaWant string) {
	invokeTcs := []struct {
		name          string
		api           string
		requestHeader map[string]string
		requestBody   io.Reader
		want          string
		isErr         bool
	}{
		{
			name:          "invoke list-tables-read-only",
			api:           "http://127.0.0.1:5000/api/tool/access-schema-read-only/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{}`)),
			want:          accessSchemaWant,
			isErr:         false,
		},
		{
			name:          "invoke list-tables",
			api:           "http://127.0.0.1:5000/api/tool/access-schema/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{}`)),
			isErr:         true,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			// Send Tool invocation request
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")
			for k, v := range tc.requestHeader {
				req.Header.Add(k, v)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			// Check response body
			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if got != tc.want {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}

// setupSpannerPgVectorTable creates a vector table in Spanner (PostgreSQL dialect) for semantic search testing
func setupSpannerPgVectorTable(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, dbString string) (string, func(*testing.T)) {
	tableName := "vector_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
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
		op, err = adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
			Database:   dbString,
			Statements: []string{fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)},
		})
		if err != nil {
			t.Errorf("unable to start drop %s operation: %s", tableName, err)
			return
		}
		opErr := op.Wait(ctx)
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

	cmd, cleanup, err := tests.StartCmd(ctx, toolsConfig)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

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

// TestSpanner_ReadOnlyVulnerabilityBlock verifies that if a tool executes against a read-only Spanner source,
// or via execute_sql_readonly / snapshot reads, write operations (DML) are blocked by the database kernel.
func TestSpanner_ReadOnlyVulnerabilityBlock(t *testing.T) {
	sourceConfig := getSpannerVars(t)
	sourceConfig["readOnly"] = true

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	args := []string{"--enable-api", "--port", "5002"}

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-readonly-spanner": sourceConfig,
		},
		"tools": map[string]any{
			"vulnerable_write_tool": map[string]any{
				"type":        "spanner-sql",
				"source":      "my-readonly-spanner",
				"description": "I try to mutate data on a read-only Spanner source!",
				"annotations": map[string]any{
					"readOnlyHint": true,
				},
				"statement": "INSERT INTO users (id, name) VALUES (9999, 'Hacker');",
			},
		},
	}

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancelWait := context.WithTimeout(ctx, 30*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Make the API request
	api := "http://127.0.0.1:5002/api/tool/vulnerable_write_tool/invoke"
	requestBody := strings.NewReader(`{}`)
	resp, respBody := tests.RunRequest(t, "POST", api, requestBody, map[string]string{})

	bodyLower := strings.ToLower(string(respBody))
	if !strings.Contains(bodyLower, "invalidargument") && !strings.Contains(bodyLower, "read-only") && !strings.Contains(bodyLower, "read only") && !strings.Contains(bodyLower, "cannot execute") && resp.StatusCode == http.StatusOK {
		t.Fatalf("Vulnerability check failed! Expected Spanner snapshot read to reject DML write, but got response code %d and body:\n%s", resp.StatusCode, string(respBody))
	}
}

// TestSpanner_DDLRejection verifies that DDL statements like CREATE TABLE / ALTER TABLE
// are rejected when invoked through execute_sql_readonly / snapshot read paths.
func TestSpanner_DDLRejection(t *testing.T) {
	sourceConfig := getSpannerVars(t)
	sourceConfig["readOnly"] = true

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	args := []string{"--enable-api", "--port", "5002"}

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-readonly-spanner": sourceConfig,
		},
		"tools": map[string]any{
			"execute_sql_readonly": map[string]any{
				"type":        "spanner-execute-sql",
				"source":      "my-readonly-spanner",
				"description": "Execute readonly SQL against Spanner",
				"annotations": map[string]any{
					"readOnlyHint": true,
				},
			},
		},
	}

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancelWait := context.WithTimeout(ctx, 30*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Make the API request attempting DDL
	api := "http://127.0.0.1:5002/api/tool/execute_sql_readonly/invoke"
	requestBody := strings.NewReader(`{"sql": "CREATE TABLE fake_ddl_test (id INT64) PRIMARY KEY (id)"}`)
	resp, respBody := tests.RunRequest(t, "POST", api, requestBody, map[string]string{})

	if resp.StatusCode == http.StatusOK {
		t.Fatalf("DDL rejection check failed! Expected Spanner snapshot read to reject DDL query, but got 200 OK:\n%s", string(respBody))
	}
}
