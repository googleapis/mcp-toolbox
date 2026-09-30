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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"google.golang.org/api/option"
)

// Spanner Omni tests run only when SPANNER_OMNI_ENDPOINT is set. Set
// SPANNER_OMNI_USE_PLAINTEXT=true for a local plaintext deployment, or
// SPANNER_OMNI_CA_CERT (and optionally SPANNER_OMNI_CLIENT_CERT and
// SPANNER_OMNI_CLIENT_KEY) for TLS/mTLS.
var (
	SpannerOmniEndpoint     = os.Getenv("SPANNER_OMNI_ENDPOINT")
	SpannerOmniDatabase     = os.Getenv("SPANNER_OMNI_DATABASE")
	SpannerOmniUsePlainText = os.Getenv("SPANNER_OMNI_USE_PLAINTEXT") == "true"
	SpannerOmniCaCert       = os.Getenv("SPANNER_OMNI_CA_CERT")
	SpannerOmniClientCert   = os.Getenv("SPANNER_OMNI_CLIENT_CERT")
	SpannerOmniClientKey    = os.Getenv("SPANNER_OMNI_CLIENT_KEY")
)

func getSpannerOmniVars(t *testing.T) map[string]any {
	if SpannerOmniEndpoint == "" {
		t.Skip("'SPANNER_OMNI_ENDPOINT' not set")
	}
	if SpannerOmniDatabase == "" {
		t.Fatal("'SPANNER_OMNI_DATABASE' not set")
	}

	// project and instance are omitted on purpose: they default for Omni.
	config := map[string]any{
		"type":         SpannerSourceType,
		"database":     SpannerOmniDatabase,
		"instanceType": "omni",
		"omniEndpoint": SpannerOmniEndpoint,
	}
	if SpannerOmniUsePlainText {
		config["omniUsePlainText"] = true
	}
	if SpannerOmniCaCert != "" {
		config["omniCaCertificateFile"] = SpannerOmniCaCert
	}
	if SpannerOmniClientCert != "" {
		config["omniClientCertificateFile"] = SpannerOmniClientCert
		config["omniClientKeyFile"] = SpannerOmniClientKey
	}
	return config
}

func initSpannerOmniClients(ctx context.Context, dbString string) (*spanner.Client, *database.DatabaseAdminClient, error) {
	config := spanner.ClientConfig{
		Type:                  spanner.OMNI,
		UsePlainText:          SpannerOmniUsePlainText,
		CaCertificateFile:     SpannerOmniCaCert,
		ClientCertificateFile: SpannerOmniClientCert,
		ClientKeyFile:         SpannerOmniClientKey,
	}
	endpoint := option.WithEndpoint(SpannerOmniEndpoint)

	dataClient, err := spanner.NewClientWithConfig(ctx, dbString, config, endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create new Spanner Omni client: %w", err)
	}
	adminClient, err := database.NewDatabaseAdminClientWithConfig(ctx, config, endpoint)
	if err != nil {
		dataClient.Close()
		return nil, nil, fmt.Errorf("unable to create new Spanner Omni admin client: %w", err)
	}
	return dataClient, adminClient, nil
}

func TestSpannerOmniToolEndpoints(t *testing.T) {
	sourceConfig := getSpannerOmniVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	dbString := fmt.Sprintf("projects/default/instances/default/databases/%s", SpannerOmniDatabase)
	dataClient, adminClient, err := initSpannerOmniClients(ctx, dbString)
	if err != nil {
		t.Fatalf("unable to create Spanner Omni clients: %s", err)
	}
	defer dataClient.Close()
	defer adminClient.Close()

	suffix := strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameParam := "omni_param_table_" + suffix
	tableNameAuth := "omni_auth_table_" + suffix
	tableNameTemplateParam := "omni_template_param_table_" + suffix

	createParamTableStmt, insertParamTableStmt, _, _, _, _, paramTestParams := getSpannerParamToolInfo(tableNameParam)
	teardownTable1 := setupSpannerTable(t, ctx, adminClient, dataClient, createParamTableStmt, insertParamTableStmt, tableNameParam, dbString, paramTestParams)
	defer teardownTable1(t)

	createAuthTableStmt, insertAuthTableStmt, _, authTestParams := getSpannerAuthToolInfo(tableNameAuth)
	teardownTable2 := setupSpannerTable(t, ctx, adminClient, dataClient, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, dbString, authTestParams)
	defer teardownTable2(t)

	createStatementTmpl := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX), age INT64) PRIMARY KEY (id)", tableNameTemplateParam)
	teardownTableTmpl := setupSpannerTable(t, ctx, adminClient, dataClient, createStatementTmpl, "", tableNameTemplateParam, dbString, nil)
	defer teardownTableTmpl(t)

	nodeTableName := "omni_node_table_" + suffix
	createNodeStmt := fmt.Sprintf("CREATE TABLE %s (id INT64 NOT NULL) PRIMARY KEY (id)", nodeTableName)
	teardownNodeTable := setupSpannerTable(t, ctx, adminClient, dataClient, createNodeStmt, "", nodeTableName, dbString, nil)
	defer teardownNodeTable(t)

	edgeTableName := "omni_edge_table_" + suffix
	createEdgeStmt := fmt.Sprintf(`
	CREATE TABLE %[1]s (
		id INT64 NOT NULL,
		target_id INT64 NOT NULL,
		FOREIGN KEY (target_id) REFERENCES %[2]s (id)
	) PRIMARY KEY (id, target_id),
	 INTERLEAVE IN PARENT %[2]s ON DELETE CASCADE
	`, edgeTableName, nodeTableName)
	teardownEdgeTable := setupSpannerTable(t, ctx, adminClient, dataClient, createEdgeStmt, "", edgeTableName, dbString, nil)
	defer teardownEdgeTable(t)

	graphName := "omni_graph_" + suffix
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

	toolsFile := map[string]any{
		"sources": map[string]any{"my-instance": sourceConfig},
		"tools":   map[string]any{},
	}
	toolsFile = addSpannerExecuteSqlConfig(t, toolsFile)
	toolsFile = addSpannerListTablesConfig(t, toolsFile)
	toolsFile = addSpannerListGraphsConfig(t, toolsFile)
	tools := toolsFile["tools"].(map[string]any)
	// Omni has no Google auth; drop the auth-required tool added above.
	delete(tools, "my-auth-exec-sql-tool")
	tools["my-search-catalog-tool"] = map[string]any{
		"type":        "spanner-search-catalog",
		"source":      "my-instance",
		"description": "Search catalog tool",
	}

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, "--enable-api")
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

	runSpannerOmniExecuteSqlTest(t, tableNameParam)
	runSpannerListTablesTest(t, tableNameParam, tableNameAuth, tableNameTemplateParam)
	runSpannerListGraphsTest(t, graphName)
	runSpannerOmniSearchCatalogTest(t)
}

func runSpannerOmniExecuteSqlTest(t *testing.T, tableNameParam string) {
	tcs := []struct {
		name  string
		tool  string
		sql   string
		want  string
		isErr bool
	}{
		{
			name: "read-only select 1",
			tool: "my-exec-sql-tool-read-only",
			sql:  "SELECT 1",
			want: `[{"":"1"}]`,
		},
		{
			name: "read-only select from table",
			tool: "my-exec-sql-tool-read-only",
			sql:  fmt.Sprintf("SELECT * FROM %s WHERE id = 3 OR name = 'Alice'", tableNameParam),
			want: `[{"id":"1","name":"Alice"},{"id":"3","name":"Sid"}]`,
		},
		{
			name:  "read-only rejects DML",
			tool:  "my-exec-sql-tool-read-only",
			sql:   fmt.Sprintf("INSERT INTO %s (id, name) VALUES (5, 'omni')", tableNameParam),
			isErr: true,
		},
		{
			name: "read-write insert",
			tool: "my-exec-sql-tool",
			sql:  fmt.Sprintf("INSERT INTO %s (id, name) VALUES (5, 'omni')", tableNameParam),
			want: "[]",
		},
		{
			name: "read-only sees insert",
			tool: "my-exec-sql-tool-read-only",
			sql:  fmt.Sprintf("SELECT name FROM %s WHERE id = 5", tableNameParam),
			want: `[{"name":"omni"}]`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.tool)
			reqBody, err := json.Marshal(map[string]string{"sql": tc.sql})
			if err != nil {
				t.Fatalf("unable to marshal request: %s", err)
			}
			resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer(reqBody), nil)
			if tc.isErr {
				if resp.StatusCode == http.StatusOK && !strings.Contains(string(respBody), "error") {
					t.Fatalf("expected an error, got %d: %s", resp.StatusCode, respBody)
				}
				return
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, respBody)
			}
			var body map[string]any
			if err := json.Unmarshal(respBody, &body); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body: %s", respBody)
			}
			if got != tc.want {
				t.Fatalf("unexpected result: got %q, want %q", got, tc.want)
			}
		})
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
