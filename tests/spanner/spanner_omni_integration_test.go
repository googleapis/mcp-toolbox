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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/api/option"
)

const (
	SpannerOmniImage = "us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r4-lts"
	SpannerOmniPort  = "15000"
)

// setupSpannerOmniContainer starts Spanner Omni, registers t.Cleanup to
// terminate the container last in LIFO order, and returns its host:port.
func setupSpannerOmniContainer(ctx context.Context, t *testing.T) string {
	t.Helper()

	const startupTimeout = 5 * time.Minute
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: SpannerOmniImage,
			// Omni listens only on localhost by default, which the mapped port can't reach.
			Cmd:          []string{"start-single-server", "--listen-addresses=0.0.0.0"},
			ExposedPorts: []string{SpannerOmniPort + "/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForLog("Spanner is ready").WithStartupTimeout(startupTimeout),
				// The Omni image has no shell tools for the in-container port check.
				wait.ForListeningPort(SpannerOmniPort+"/tcp").SkipInternalCheck().WithStartupTimeout(startupTimeout),
			).WithDeadline(startupTimeout),
		},
		Started: true,
	})
	t.Cleanup(func() {
		if container == nil {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanupCancel()
		if termErr := container.Terminate(cleanupCtx); termErr != nil {
			t.Errorf("failed to terminate Spanner Omni container: %s", termErr)
		}
	})
	if err != nil {
		t.Fatalf("failed to start Spanner Omni container: %s", err)
	}

	endpoint, err := container.PortEndpoint(ctx, SpannerOmniPort+"/tcp", "")
	if err != nil {
		t.Fatalf("failed to get Spanner Omni endpoint: %s", err)
	}
	return endpoint
}

func getSpannerOmniVars(endpoint, dbName, dialect string) map[string]any {
	// project and instance are omitted on purpose: they default for Omni.
	cfg := map[string]any{
		"type":             SpannerSourceType,
		"database":         dbName,
		"instanceType":     "omni",
		"omniEndpoint":     endpoint,
		"omniUsePlainText": true,
	}
	if dialect != "" {
		cfg["dialect"] = dialect
	}
	return cfg
}

// initSpannerOmniClients creates the database, applies extraDDL in a single
// batched UpdateDatabaseDdl LRO, and registers client cleanup via t.Cleanup.
// Schema objects are not dropped individually because the ephemeral container
// is terminated at the end of the test.
func initSpannerOmniClients(ctx context.Context, t *testing.T, endpoint, dbName string, dialect databasepb.DatabaseDialect, extraDDL []string) *spanner.Client {
	t.Helper()
	config := spanner.ClientConfig{Type: spanner.OMNI, UsePlainText: true}
	opt := option.WithEndpoint(endpoint)

	adminClient, err := database.NewDatabaseAdminClientWithConfig(context.Background(), config, opt)
	if err != nil {
		t.Fatalf("unable to create Spanner Omni admin client: %s", err)
	}
	t.Cleanup(func() {
		if closeErr := adminClient.Close(); closeErr != nil {
			t.Errorf("failed to close Spanner Omni admin client: %s", closeErr)
		}
	})

	createStmt := fmt.Sprintf("CREATE DATABASE `%s`", dbName)
	if dialect == databasepb.DatabaseDialect_POSTGRESQL {
		createStmt = fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)
	}

	op, err := adminClient.CreateDatabase(ctx, &databasepb.CreateDatabaseRequest{
		Parent:          "projects/default/instances/default",
		CreateStatement: createStmt,
		DatabaseDialect: dialect,
	})
	if err != nil {
		t.Fatalf("unable to start create database operation: %s", err)
	}
	if _, err := op.Wait(ctx); err != nil {
		t.Fatalf("unable to create database %s: %s", dbName, err)
	}
	dbString := fmt.Sprintf("projects/default/instances/default/databases/%s", dbName)

	if len(extraDDL) > 0 {
		applySpannerDdl(t, ctx, adminClient, dbString, extraDDL, nil)
	}

	dataClient, err := spanner.NewClientWithConfig(context.Background(), dbString, config, opt)
	if err != nil {
		t.Fatalf("unable to create Spanner Omni client: %s", err)
	}
	t.Cleanup(func() {
		dataClient.Close()
	})

	return dataClient
}

// hasGoogleCredentials reports whether the Google ID and access tokens used by
// the Toolbox auth tests are available. Spanner Omni itself doesn't need them.
func hasGoogleCredentials(t *testing.T) bool {
	if _, err := tests.GetGoogleIdToken(t); err != nil {
		return false
	}
	if _, err := sources.GetIAMAccessToken(t.Context()); err != nil {
		return false
	}
	return true
}

func TestSpannerOmniToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	endpoint := setupSpannerOmniContainer(ctx, t)

	dbName := "omni_test_db"
	sourceConfig := getSpannerOmniVars(endpoint, dbName, "")

	runNow := time.Now()
	tableNameParam := newSpannerTestObjectName("param_table", runNow)
	tableNameAuth := newSpannerTestObjectName("auth_table", runNow)
	tableNameTemplateParam := newSpannerTestObjectName("template_param_table", runNow)
	nodeTableName := newSpannerTestObjectName("node_table", runNow)
	edgeTableName := newSpannerTestObjectName("edge_table", runNow)
	graphName := newSpannerTestObjectName("graph", runNow)
	negDdlTableName := newSpannerTestObjectName("neg_ddl_table", runNow)

	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getSpannerParamToolInfo(tableNameParam)
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getSpannerAuthToolInfo(tableNameAuth)
	createStatementTmpl := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX), age INT64) PRIMARY KEY (id)", tableNameTemplateParam)
	createNodeStmt, createEdgeStmt, createGraphStmt := getSpannerGraphSchemaDDL(nodeTableName, edgeTableName, graphName)

	createStmts := []string{
		createParamTableStmt,
		createAuthTableStmt,
		createStatementTmpl,
		createNodeStmt,
		createEdgeStmt,
		createGraphStmt,
	}

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
	runSemanticSearch := os.Getenv("API_KEY") != ""
	if runSemanticSearch {
		vectorTableName := newSpannerTestObjectName("vector_table", runNow)
		createStmts = append(createStmts, getSpannerVectorTableDDL(vectorTableName))
		insertStmt, searchStmt := getSpannerVectorSearchStmts(vectorTableName)
		toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, SpannerToolType, insertStmt, searchStmt)
	}

	dataClient := initSpannerOmniClients(ctx, t, endpoint, dbName, databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, createStmts)
	seedSpannerStatements(t, ctx, dataClient,
		spanner.Statement{SQL: insertParamTableStmt, Params: paramTestParams},
		spanner.Statement{SQL: insertAuthTableStmt, Params: authTestParams},
	)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, "--enable-api")
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
	// These helpers also cover the Toolbox auth services, which need Google
	// credentials (available in Cloud Build) regardless of the database.
	if hasGoogleCredentials(t) {
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
		runSpannerExecuteSqlToolInvokeTest(t, select1Want, invokeParamWant, tableNameParam, negDdlTableName)
	} else {
		t.Log("Google credentials not found; skipping tool invoke, MCP, and execute SQL tests that include auth")
	}
	tests.RunToolInvokeWithTemplateParameters(
		t, tableNameTemplateParam,
		tests.WithSelectAllWant(tmplSelectAllWwant),
		tests.WithTmplSelectId1Want(tmplSelectId1Want),
		tests.DisableDdlTest(),
	)
	runSpannerSchemaToolInvokeTest(t, accessSchemaWant)
	runSpannerListTablesTest(t, tableNameParam, tableNameAuth, tableNameTemplateParam)
	runSpannerListGraphsTest(t, graphName)
	runSpannerOmniSearchCatalogTest(t)
	if runSemanticSearch {
		tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox")
	}
}

func TestSpannerOmniPostgresqlToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	endpoint := setupSpannerOmniContainer(ctx, t)

	dbName := "omni_pg_test_db"
	sourceConfig := getSpannerOmniVars(endpoint, dbName, "postgresql")

	runNow := time.Now()
	pgParamTable := newSpannerTestObjectName("param_table", runNow)
	pgNegDdlTable := newSpannerTestObjectName("neg_ddl_table", runNow)

	createPgParamStmt, insertPgParamStmt, pgParamToolStmt := getSpannerPgParamToolInfo(pgParamTable)
	createStmts := []string{createPgParamStmt}
	expectedTables := []string{pgParamTable}

	toolsConfig := buildSpannerPgToolsConfig(t, sourceConfig, pgParamToolStmt)

	runSemanticSearch := os.Getenv("API_KEY") != ""
	if runSemanticSearch {
		pgVectorTable := newSpannerTestObjectName("vector_table", runNow)
		createStmts = append(createStmts, getSpannerPgVectorTableDDL(pgVectorTable))
		expectedTables = append(expectedTables, pgVectorTable)
		insertStmt, searchStmt := getSpannerPgVectorSearchStmts(pgVectorTable)
		toolsConfig = tests.AddSemanticSearchConfig(t, toolsConfig, SpannerToolType, insertStmt, searchStmt)
	}

	dataClient := initSpannerOmniClients(ctx, t, endpoint, dbName, databasepb.DatabaseDialect_POSTGRESQL, createStmts)
	seedSpannerStatements(t, ctx, dataClient, insertPgParamStmt)

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

	runSpannerPgToolsTest(t, pgParamTable, pgNegDdlTable, expectedTables...)
	if runSemanticSearch {
		tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox")
	}
}

func runSpannerOmniSearchCatalogTest(t *testing.T) {
	t.Run("search catalog is not supported", func(t *testing.T) {
		api := "http://127.0.0.1:5000/api/tool/my-search-catalog-tool/invoke"
		resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer([]byte(`{"prompt":"table"}`)), nil)
		want := "search catalog is not supported for Spanner Omni sources"
		if !strings.Contains(string(respBody), want) {
			t.Fatalf("expected %q, got %d: %s", want, resp.StatusCode, respBody)
		}
	})
}
