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
	"github.com/google/uuid"
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

// setupSpannerOmniContainer starts Spanner Omni and returns its host:port.
func setupSpannerOmniContainer(ctx context.Context, t *testing.T) (string, func()) {
	t.Helper()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: SpannerOmniImage,
			// Omni listens only on localhost by default, which the mapped port can't reach.
			Cmd:          []string{"start-single-server", "--listen-addresses=0.0.0.0"},
			ExposedPorts: []string{SpannerOmniPort + "/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForLog("Spanner is ready"),
				// The Omni image has no shell tools for the in-container port check.
				wait.ForListeningPort(SpannerOmniPort+"/tcp").SkipInternalCheck(),
			).WithDeadline(5 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		if container != nil {
			_ = container.Terminate(context.Background())
		}
		t.Fatalf("failed to start Spanner Omni container: %s", err)
	}

	cleanup := func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanupCancel()
		if err := container.Terminate(cleanupCtx); err != nil {
			t.Errorf("failed to terminate container: %s", err)
		}
	}

	endpoint, err := container.PortEndpoint(ctx, SpannerOmniPort+"/tcp", "")
	if err != nil {
		cleanup()
		t.Fatalf("failed to get Spanner Omni endpoint: %s", err)
	}
	return endpoint, cleanup
}

func getSpannerOmniVars(endpoint, dbName string) map[string]any {
	// project and instance are omitted on purpose: they default for Omni.
	return map[string]any{
		"type":             SpannerSourceType,
		"database":         dbName,
		"instanceType":     "omni",
		"omniEndpoint":     endpoint,
		"omniUsePlainText": true,
	}
}

// initSpannerOmniClients creates the database and returns clients for it.
// Nothing is dropped afterwards because the container is discarded.
func initSpannerOmniClients(ctx context.Context, t *testing.T, endpoint, dbName string) (*spanner.Client, *database.DatabaseAdminClient, string, func()) {
	t.Helper()
	config := spanner.ClientConfig{Type: spanner.OMNI, UsePlainText: true}
	opt := option.WithEndpoint(endpoint)

	adminClient, err := database.NewDatabaseAdminClientWithConfig(ctx, config, opt)
	if err != nil {
		t.Fatalf("unable to create Spanner Omni admin client: %s", err)
	}
	op, err := adminClient.CreateDatabase(ctx, &databasepb.CreateDatabaseRequest{
		Parent:          "projects/default/instances/default",
		CreateStatement: fmt.Sprintf("CREATE DATABASE `%s`", dbName),
	})
	if err != nil {
		adminClient.Close()
		t.Fatalf("unable to start create database operation: %s", err)
	}
	if _, err := op.Wait(ctx); err != nil {
		adminClient.Close()
		t.Fatalf("unable to create database %s: %s", dbName, err)
	}
	dbString := fmt.Sprintf("projects/default/instances/default/databases/%s", dbName)

	dataClient, err := spanner.NewClientWithConfig(ctx, dbString, config, opt)
	if err != nil {
		adminClient.Close()
		t.Fatalf("unable to create Spanner Omni client: %s", err)
	}

	return dataClient, adminClient, dbString, func() {
		dataClient.Close()
		adminClient.Close()
	}
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

	endpoint, cleanupContainer := setupSpannerOmniContainer(ctx, t)
	defer cleanupContainer()

	// The container is discarded at the end, so the database, tables, and
	// graph below aren't torn down individually.
	dbName := "omni_test_db"
	dataClient, adminClient, dbString, closeClients := initSpannerOmniClients(ctx, t, endpoint, dbName)
	defer closeClients()
	sourceConfig := getSpannerOmniVars(endpoint, dbName)

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getSpannerParamToolInfo(tableNameParam)
	setupSpannerTable(t, ctx, adminClient, dataClient, createParamTableStmt, insertParamTableStmt, tableNameParam, dbString, paramTestParams)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getSpannerAuthToolInfo(tableNameAuth)
	setupSpannerTable(t, ctx, adminClient, dataClient, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, dbString, authTestParams)

	// set up data for template param tool
	createStatementTmpl := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX), age INT64) PRIMARY KEY (id)", tableNameTemplateParam)
	setupSpannerTable(t, ctx, adminClient, dataClient, createStatementTmpl, "", tableNameTemplateParam, dbString, nil)

	// set up for graph tool
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
	runSemanticSearch := os.Getenv("API_KEY") != ""
	if runSemanticSearch {
		vectorTableName, _ := setupSpannerVectorTable(t, ctx, adminClient, dbString)
		insertStmt, searchStmt := getSpannerVectorSearchStmts(vectorTableName)
		toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, SpannerToolType, insertStmt, searchStmt)
	}

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, "--enable-api")
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

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
		runSpannerExecuteSqlToolInvokeTest(t, select1Want, invokeParamWant, tableNameParam)
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
