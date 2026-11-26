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
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/api/option"
)

const (
	SpannerOmniImage = "us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r4-lts"
	SpannerOmniPort  = "15000"
)

var (
	omniInitOnce    sync.Once
	omniContainer   testcontainers.Container
	omniEndpoint    string
	omniAdminClient *database.DatabaseAdminClient
	omniInitErr     error
)

func TestMain(m *testing.M) {
	code := m.Run()
	teardownSharedSpannerOmni()
	os.Exit(code)
}

func terminateOmniContainer(c testcontainers.Container) {
	if c == nil {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := c.Terminate(cleanupCtx); err != nil {
		log.Printf("failed to terminate Spanner Omni container: %v", err)
	}
}

func teardownSharedSpannerOmni() {
	if omniAdminClient != nil {
		_ = omniAdminClient.Close()
		omniAdminClient = nil
	}
	if omniContainer != nil {
		terminateOmniContainer(omniContainer)
		omniContainer = nil
	}
}

// sharedSpannerOmni lazily starts a single Spanner Omni container and admin
// client shared across all Omni integration tests in this binary, avoiding
// duplicate container boots while keeping unit tests container-free.
func sharedSpannerOmni(t *testing.T) (string, *database.DatabaseAdminClient) {
	t.Helper()
	omniInitOnce.Do(func() {
		const startupTimeout = 5 * time.Minute
		startupCtx, cancel := context.WithTimeout(context.Background(), startupTimeout)
		defer cancel()

		container, err := testcontainers.GenericContainer(startupCtx, testcontainers.GenericContainerRequest{
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
		if err != nil {
			terminateOmniContainer(container)
			omniInitErr = fmt.Errorf("failed to start Spanner Omni container: %w", err)
			return
		}

		endpoint, err := container.PortEndpoint(startupCtx, SpannerOmniPort+"/tcp", "")
		if err != nil {
			terminateOmniContainer(container)
			omniInitErr = fmt.Errorf("failed to get Spanner Omni endpoint: %w", err)
			return
		}

		cfg := spanner.ClientConfig{Type: spanner.OMNI, UsePlainText: true, DisableNativeMetrics: true}
		adminClient, err := database.NewDatabaseAdminClientWithConfig(
			context.Background(),
			cfg,
			option.WithEndpoint(endpoint),
			option.WithGRPCConnectionPool(1),
		)
		if err != nil {
			terminateOmniContainer(container)
			omniInitErr = fmt.Errorf("unable to create Spanner Omni admin client: %w", err)
			return
		}

		omniContainer = container
		omniEndpoint = endpoint
		omniAdminClient = adminClient
	})
	if omniInitErr != nil {
		t.Fatal(omniInitErr)
	}
	return omniEndpoint, omniAdminClient
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

// initSpannerOmniDatabase creates an isolated database on the shared Omni
// server. For GoogleSQL, extraDDL is executed atomically inside CreateDatabase
// in a single LRO; for PostgreSQL (where ExtraStatements is unsupported in
// CreateDatabase), extraDDL runs in one batched UpdateDatabaseDdl LRO.
func initSpannerOmniDatabase(ctx context.Context, t *testing.T, adminClient *database.DatabaseAdminClient, endpoint, dbName string, dialect databasepb.DatabaseDialect, extraDDL []string) *spanner.Client {
	t.Helper()

	req := &databasepb.CreateDatabaseRequest{
		Parent:          "projects/default/instances/default",
		CreateStatement: fmt.Sprintf("CREATE DATABASE `%s`", dbName),
		DatabaseDialect: dialect,
	}
	if dialect == databasepb.DatabaseDialect_POSTGRESQL {
		req.CreateStatement = fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)
	} else {
		req.ExtraStatements = extraDDL
	}

	op, err := adminClient.CreateDatabase(ctx, req)
	if err != nil {
		t.Fatalf("unable to start create database operation: %s", err)
	}
	if _, err := op.Wait(ctx); err != nil {
		t.Fatalf("unable to create database %s: %s", dbName, err)
	}
	dbString := fmt.Sprintf("projects/default/instances/default/databases/%s", dbName)

	if dialect == databasepb.DatabaseDialect_POSTGRESQL && len(extraDDL) > 0 {
		applySpannerDdl(t, ctx, adminClient, dbString, extraDDL, nil)
	}

	dataClient, err := spanner.NewClientWithConfig(
		context.Background(),
		dbString,
		spanner.ClientConfig{Type: spanner.OMNI, UsePlainText: true, DisableNativeMetrics: true},
		option.WithEndpoint(endpoint),
		option.WithGRPCConnectionPool(1),
	)
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

	endpoint, adminClient := sharedSpannerOmni(t)

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

	createParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramMutations := getSpannerParamToolInfo(tableNameParam)
	createAuthTableStmt, authToolStmt, authMutations := getSpannerAuthToolInfo(tableNameAuth)
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

	dataClient := initSpannerOmniDatabase(ctx, t, adminClient, endpoint, dbName, databasepb.DatabaseDialect_GOOGLE_STANDARD_SQL, createStmts)
	seedSpannerMutations(t, ctx, dataClient, slices.Concat(paramMutations, authMutations)...)

	startSpannerToolbox(t, ctx, toolsFile, "--enable-api")

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

	endpoint, adminClient := sharedSpannerOmni(t)

	dbName := "omni_pg_test_db"
	sourceConfig := getSpannerOmniVars(endpoint, dbName, "postgresql")

	runNow := time.Now()
	pgParamTable := newSpannerTestObjectName("param_table", runNow)
	pgNegDdlTable := newSpannerTestObjectName("neg_ddl_table", runNow)

	createPgParamStmt, pgParamToolStmt, pgParamMutations := getSpannerPgParamToolInfo(pgParamTable)
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

	dataClient := initSpannerOmniDatabase(ctx, t, adminClient, endpoint, dbName, databasepb.DatabaseDialect_POSTGRESQL, createStmts)
	seedSpannerMutations(t, ctx, dataClient, pgParamMutations...)

	startSpannerToolbox(t, ctx, toolsConfig, "--enable-api")

	runSpannerPgToolsTest(t, pgParamTable, pgNegDdlTable, expectedTables...)
	if runSemanticSearch {
		tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox")
	}
}

func runSpannerOmniSearchCatalogTest(t *testing.T) {
	t.Run("search catalog is not supported", func(t *testing.T) {
		api := "http://127.0.0.1:5000/api/tool/my-search-catalog-tool/invoke"
		resp, respBody := tests.RunRequest(t, http.MethodPost, api, strings.NewReader(`{"prompt":"table"}`), nil)
		want := "search catalog is not supported for Spanner Omni sources"
		if !strings.Contains(string(respBody), want) {
			t.Fatalf("expected %q, got %d: %s", want, resp.StatusCode, respBody)
		}
	})
}
