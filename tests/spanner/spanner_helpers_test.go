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
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/googleapis/mcp-toolbox/tests"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	spannerTeardownTimeout   = 5 * time.Minute
	orphanedSpannerObjectTTL = 3 * time.Hour
)

var (
	SpannerSourceType = "spanner"
	SpannerToolType   = "spanner-sql"

	serverReadyRegex = regexp.MustCompile(`Server ready to serve`)

	spannerTestTablePrefixes = []string{
		"param_table_",
		"auth_table_",
		"template_param_table_",
		"node_table_",
		"edge_table_",
		"vector_table_",
		"neg_ddl_table_",
	}
	spannerTestGraphPrefixes = []string{
		"graph_",
	}
)

// newSpannerTestObjectName builds a lowercase, timestamped, UUID-suffixed
// identifier (<prefix>_<unix>_<12hex>) safe for concurrent shared-DB runs and
// TTL-based orphan sweeping across both GoogleSQL and PostgreSQL dialects.
func newSpannerTestObjectName(prefix string, now time.Time) string {
	base := strings.TrimSuffix(prefix, "_")
	u := uuid.New()
	return fmt.Sprintf("%s_%d_%x", base, now.Unix(), u[:6])
}

func startSpannerToolbox(t *testing.T, ctx context.Context, toolsConfig map[string]any, args ...string) {
	t.Helper()
	cmd, cleanup, err := tests.StartCmd(ctx, toolsConfig, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, serverReadyRegex, cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

// shouldSweepSpannerObject reports whether name matches one of prefixes and
// carries a Unix timestamp older than orphanedSpannerObjectTTL relative to now.
// Un-timestamped names return false so concurrent runs from branches without
// embedded timestamps are never disrupted on shared databases.
func shouldSweepSpannerObject(name string, now time.Time, prefixes ...string) bool {
	for _, rawPrefix := range prefixes {
		prefix := strings.TrimSuffix(rawPrefix, "_") + "_"
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rest := strings.TrimPrefix(name, prefix)
		tsStr, _, ok := strings.Cut(rest, "_")
		if !ok || tsStr == "" {
			return false
		}
		createdUnix, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil || createdUnix <= 0 {
			return false
		}
		return now.Sub(time.Unix(createdUnix, 0)) > orphanedSpannerObjectTTL
	}
	return false
}

func quoteSpannerGoogleSQLIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func quoteSpannerPostgresIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// orderSpannerDropStatements builds DROP ... IF EXISTS statements in strict
// reverse dependency order: property graphs first, interleaved/child edge
// tables second, and parent/standalone tables last.
func orderSpannerDropStatements(graphs, tables []string, quoteIdent func(string) string) []string {
	sortedGraphs := slices.Clone(graphs)
	slices.Sort(sortedGraphs)

	var edgeTables, baseTables []string
	for _, tbl := range tables {
		if strings.HasPrefix(tbl, "edge_table_") {
			edgeTables = append(edgeTables, tbl)
		} else {
			baseTables = append(baseTables, tbl)
		}
	}
	slices.Sort(edgeTables)
	slices.Sort(baseTables)

	stmts := make([]string, 0, len(sortedGraphs)+len(edgeTables)+len(baseTables))
	for _, g := range sortedGraphs {
		stmts = append(stmts, fmt.Sprintf("DROP PROPERTY GRAPH IF EXISTS %s", quoteIdent(g)))
	}
	for _, tbl := range edgeTables {
		stmts = append(stmts, fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(tbl)))
	}
	for _, tbl := range baseTables {
		stmts = append(stmts, fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteIdent(tbl)))
	}
	return stmts
}

func isSpannerNotFound(err error) bool {
	return status.Code(err) == codes.NotFound
}

func runSpannerTeardownDdl(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, dbString string, dropStatements []string) {
	t.Helper()
	if len(dropStatements) == 0 {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), spannerTeardownTimeout)
	defer cancel()

	op, err := adminClient.UpdateDatabaseDdl(cleanupCtx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbString,
		Statements: dropStatements,
	})
	if err != nil {
		if !isSpannerNotFound(err) {
			t.Errorf("unable to start Spanner DDL teardown on %s: %s", dbString, err)
		}
		return
	}
	if opErr := op.Wait(cleanupCtx); opErr != nil && !isSpannerNotFound(opErr) {
		t.Errorf("Spanner DDL teardown failed on %s: %s", dbString, opErr)
	}
}

// applySpannerDdl executes createStatements in a single batched UpdateDatabaseDdl
// LRO. When dropStatements is non-empty, an idempotent t.Cleanup hook is
// registered before issuing the LRO so partial or timed-out DDL operations are
// still cleaned up. Ephemeral container tests pass nil dropStatements.
func applySpannerDdl(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, dbString string, createStatements, dropStatements []string) func() {
	t.Helper()
	var once sync.Once
	teardown := func() {
		once.Do(func() {
			runSpannerTeardownDdl(t, ctx, adminClient, dbString, dropStatements)
		})
	}
	if len(dropStatements) > 0 {
		t.Cleanup(teardown)
	}

	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbString,
		Statements: createStatements,
	})
	if err != nil {
		t.Fatalf("unable to start Spanner DDL setup on %s: %s", dbString, err)
	}
	if err := op.Wait(ctx); err != nil {
		t.Fatalf("unable to complete Spanner DDL setup on %s: %s", dbString, err)
	}
	return teardown
}

// seedSpannerMutations writes all seed rows in a single storage-layer Commit
// RPC via Client.Apply, bypassing SQL parsing and read-write lock round-trips.
func seedSpannerMutations(t *testing.T, ctx context.Context, dataClient *spanner.Client, mutations ...*spanner.Mutation) {
	t.Helper()
	if len(mutations) == 0 {
		return
	}
	if _, err := dataClient.Apply(ctx, mutations); err != nil {
		t.Fatalf("unable to seed Spanner test data: %s", err)
	}
}

func getSpannerParamSeedMutations(tableName string) []*spanner.Mutation {
	cols := []string{"id", "name"}
	return []*spanner.Mutation{
		spanner.Insert(tableName, cols, []any{int64(1), "Alice"}),
		spanner.Insert(tableName, cols, []any{int64(2), "Jane"}),
		spanner.Insert(tableName, cols, []any{int64(3), "Sid"}),
		spanner.Insert(tableName, cols, []any{int64(4), spanner.NullString{}}),
	}
}

func getSpannerAuthSeedMutations(tableName string) []*spanner.Mutation {
	cols := []string{"id", "name", "email"}
	return []*spanner.Mutation{
		spanner.Insert(tableName, cols, []any{int64(1), "Alice", tests.ServiceAccountEmail}),
		spanner.Insert(tableName, cols, []any{int64(2), "Jane", "janedoe@gmail.com"}),
	}
}

func getSpannerGraphSchemaDDL(nodeTableName, edgeTableName, graphName string) (string, string, string) {
	createNodeStmt := fmt.Sprintf("CREATE TABLE %s (id INT64 NOT NULL) PRIMARY KEY (id)", nodeTableName)
	createEdgeStmt := fmt.Sprintf(`
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
	return createNodeStmt, createEdgeStmt, createGraphStmt
}

// getSpannerParamToolInfo returns DDL, tool statements, and seed mutations for my-tool.
func getSpannerParamToolInfo(tableName string) (string, string, string, string, string, []*spanner.Mutation) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX)) PRIMARY KEY (id)", tableName)
	toolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = @id OR name = @name", tableName)
	idToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = @id", tableName)
	nameToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = @name", tableName)
	arrayToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id IN UNNEST(@idArray) AND name IN UNNEST(@nameArray)", tableName)
	return createStatement, toolStatement, idToolStatement, nameToolStatement, arrayToolStatement, getSpannerParamSeedMutations(tableName)
}

// getSpannerAuthToolInfo returns DDL, tool statement, and seed mutations for my-auth-tool.
func getSpannerAuthToolInfo(tableName string) (string, string, []*spanner.Mutation) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX), email STRING(MAX)) PRIMARY KEY (id)", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = @email", tableName)
	return createStatement, toolStatement, getSpannerAuthSeedMutations(tableName)
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

type spannerInvokeCase struct {
	name          string
	api           string
	requestHeader map[string]string
	requestBody   string
	want          string
	isErr         bool
}

func runSpannerInvokeCases(t *testing.T, tcs []spannerInvokeCase) {
	t.Helper()
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			resp, respBody := tests.RunRequest(t, http.MethodPost, tc.api, strings.NewReader(tc.requestBody), tc.requestHeader)
			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
			}

			var body map[string]any
			if err := json.Unmarshal(respBody, &body); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body: %s", string(respBody))
			}

			// Database errors are returned with a 200 status and an error in the result.
			if tc.isErr {
				if !strings.Contains(got, `"error"`) {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}

func runSpannerExecuteSqlToolInvokeTest(t *testing.T, select1Want, invokeParamWant, tableNameParam, negDdlTableName string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	createNegDdlBody := fmt.Sprintf("{\"sql\":\"CREATE TABLE %s (id INT64) PRIMARY KEY (id)\"}", negDdlTableName)
	dropNegDdlBody := fmt.Sprintf("{\"sql\":\"DROP TABLE %s\"}", negDdlTableName)

	runSpannerInvokeCases(t, []spannerInvokeCase{
		{
			name:        "invoke my-exec-sql-tool-read-only",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestBody: `{"sql":"SELECT 1"}`,
			want:        select1Want,
		},
		{
			name:        "invoke my-exec-sql-tool-read-only with data present in table",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestBody: fmt.Sprintf("{\"sql\":\"SELECT * FROM %s WHERE id = 3 OR name = 'Alice'\"}", tableNameParam),
			want:        invokeParamWant,
		},
		{
			name:        "invoke my-exec-sql-tool-read-only create table",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestBody: createNegDdlBody,
			isErr:       true,
		},
		{
			name:        "invoke my-exec-sql-tool-read-only drop table",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestBody: dropNegDdlBody,
			isErr:       true,
		},
		{
			name:        "invoke my-exec-sql-tool-read-only insert entry",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestBody: fmt.Sprintf("{\"sql\":\"INSERT INTO %s (id, name) VALUES (4, 'test_name')\"}", tableNameParam),
			isErr:       true,
		},
		{
			name:        "invoke my-exec-sql-tool without body",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestBody: `{}`,
			isErr:       true,
		},
		{
			name:        "invoke my-exec-sql-tool",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestBody: `{"sql":"SELECT 1"}`,
			want:        select1Want,
		},
		{
			name:        "invoke my-exec-sql-tool create table",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestBody: createNegDdlBody,
			isErr:       true,
		},
		{
			name:        "invoke my-exec-sql-tool drop table",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestBody: dropNegDdlBody,
			isErr:       true,
		},
		{
			name:        "invoke my-exec-sql-tool insert entry",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestBody: fmt.Sprintf("{\"sql\":\"INSERT INTO %s (id, name) VALUES (5, 'test_name')\"}", tableNameParam),
			want:        "[]",
		},
		{
			name:          "Invoke my-auth-exec-sql-tool with auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-exec-sql-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody:   `{"sql":"SELECT 1"}`,
			want:          select1Want,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool with invalid auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-exec-sql-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			requestBody:   `{"sql":"SELECT 1"}`,
			isErr:         true,
		},
		{
			name:        "Invoke my-auth-exec-sql-tool without auth token",
			api:         "http://127.0.0.1:5000/api/tool/my-auth-exec-sql-tool/invoke",
			requestBody: `{"sql":"SELECT 1"}`,
			isErr:       true,
		},
	})
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

	if len(expectedTables) == 0 {
		if len(tables) != 0 {
			t.Errorf("expected empty table list, got %d tables", len(tables))
		}
		return
	}

	// Verify expected tables exist
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

// runSpannerListTablesTest tests the spanner-list-tables tool
func runSpannerListTablesTest(t *testing.T, tableNames ...string) {
	specificTables := tableNames
	if len(specificTables) > 2 {
		specificTables = specificTables[:2]
	}

	invokeTcs := []struct {
		name            string
		requestBody     string
		expectedTables  []string // empty means expect 0 tables
		useSimpleFormat bool
	}{
		{
			name:           "list all tables with detailed format",
			requestBody:    `{}`,
			expectedTables: tableNames,
		},
		{
			name:            "list tables with simple format",
			requestBody:     `{"output_format": "simple"}`,
			expectedTables:  tableNames,
			useSimpleFormat: true,
		},
		{
			name:           "list specific tables",
			requestBody:    fmt.Sprintf(`{"table_names": "%s"}`, strings.Join(specificTables, ",")),
			expectedTables: specificTables,
		},
		{
			name:           "list non-existent table",
			requestBody:    `{"table_names": "non_existent_table_xyz"}`,
			expectedTables: []string{},
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			url := "http://127.0.0.1:5000/api/tool/list-tables-tool/invoke"
			resp, respBody := tests.RunRequest(t, http.MethodPost, url, strings.NewReader(tc.requestBody), nil)

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

	if len(expectedGraphs) == 0 {
		if len(graphs) != 0 {
			t.Errorf("expected empty graph list, got %d graphs", len(graphs))
		}
		return
	}

	// Verify expected graphs exist
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

// runSpannerListGraphsTest tests the spanner-list-graphs tool
func runSpannerListGraphsTest(t *testing.T, graphName string) {
	invokeTcs := []struct {
		name            string
		requestBody     string
		expectedGraphs  []string // empty means expect 0 graphs
		useSimpleFormat bool
	}{
		{
			name:           "list all graphs with detailed format",
			requestBody:    `{}`,
			expectedGraphs: []string{graphName},
		},
		{
			name:            "list graphs with simple format",
			requestBody:     `{"output_format": "simple"}`,
			expectedGraphs:  []string{graphName},
			useSimpleFormat: true,
		},
		{
			name:           "list specific graphs",
			requestBody:    fmt.Sprintf(`{"graph_names": "%s"}`, graphName),
			expectedGraphs: []string{graphName},
		},
		{
			name:           "list non-existent graph",
			requestBody:    `{"graph_names": "non_existent_graph_xyz"}`,
			expectedGraphs: []string{},
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			url := "http://127.0.0.1:5000/api/tool/list-graphs-tool/invoke"
			resp, respBody := tests.RunRequest(t, http.MethodPost, url, strings.NewReader(tc.requestBody), nil)

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

func getSpannerVectorTableDDL(tableName string) string {
	return fmt.Sprintf(`CREATE TABLE %s (
		id INT64,
		content STRING(MAX),
		embedding ARRAY<FLOAT32>
	) PRIMARY KEY (id)`, tableName)
}

// getSpannerVectorSearchStmts returns statements for spanner semantic search
func getSpannerVectorSearchStmts(vectorTableName string) (string, string) {
	insertStmt := fmt.Sprintf("INSERT INTO %s (id, content, embedding) VALUES (1, @content, @text_to_embed)", vectorTableName)
	searchStmt := fmt.Sprintf("SELECT id, content, COSINE_DISTANCE(embedding, @query) AS distance FROM %s ORDER BY distance LIMIT 1", vectorTableName)
	return insertStmt, searchStmt
}

func getSpannerPgVectorTableDDL(tableName string) string {
	return fmt.Sprintf(`CREATE TABLE %s (
		id bigint PRIMARY KEY,
		content text,
		embedding float4[]
	)`, tableName)
}

// getSpannerPgVectorSearchStmts returns statements for spanner semantic search (PostgreSQL dialect)
func getSpannerPgVectorSearchStmts(vectorTableName string) (string, string) {
	insertStmt := fmt.Sprintf("INSERT INTO %s (id, content, embedding) VALUES (1, $1, $2)", vectorTableName)
	searchStmt := fmt.Sprintf("SELECT id, content, spanner.cosine_distance(embedding, $1::float4[]) AS distance FROM %s ORDER BY distance LIMIT 1", vectorTableName)
	return insertStmt, searchStmt
}

func getSpannerPgParamToolInfo(tableName string) (string, string, []*spanner.Mutation) {
	createStmt := fmt.Sprintf("CREATE TABLE %s (id bigint PRIMARY KEY, name varchar(256))", tableName)
	paramToolStmt := fmt.Sprintf("SELECT id, name FROM %s WHERE id = $1 OR name = $2 ORDER BY id", tableName)
	return createStmt, paramToolStmt, getSpannerParamSeedMutations(tableName)
}

func buildSpannerPgToolsConfig(t *testing.T, sourceConfig map[string]any, paramToolStmt string) map[string]any {
	t.Helper()
	config := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{
			"my-pg-param-tool": map[string]any{
				"type":        SpannerToolType,
				"source":      "my-instance",
				"description": "Parameterized query in PostgreSQL dialect",
				"statement":   paramToolStmt,
				"readOnly":    true,
				"parameters": []any{
					map[string]any{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
					map[string]any{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
			},
			"my-exec-sql-tool-read-only": map[string]any{
				"type":        "spanner-execute-sql",
				"source":      "my-instance",
				"description": "Tool to execute SQL in read-only mode",
				"readOnly":    true,
			},
			"my-exec-sql-tool": map[string]any{
				"type":        "spanner-execute-sql",
				"source":      "my-instance",
				"description": "Tool to execute SQL",
			},
		},
	}
	return addSpannerListTablesConfig(t, config)
}

func runSpannerPgToolsTest(t *testing.T, tableNameParam, negDdlTableName string, expectedTables ...string) {
	t.Helper()
	wantRows := `[{"id":"1","name":"Alice"},{"id":"3","name":"Sid"}]`
	createNegDdl := fmt.Sprintf(`{"sql":"CREATE TABLE %s (id bigint PRIMARY KEY)"}`, negDdlTableName)

	runSpannerInvokeCases(t, []spannerInvokeCase{
		{
			name:        "invoke pg param tool",
			api:         "http://127.0.0.1:5000/api/tool/my-pg-param-tool/invoke",
			requestBody: `{"id": 3, "name": "Alice"}`,
			want:        wantRows,
		},
		{
			name:        "invoke pg read-only execute sql select",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestBody: `{"sql":"SELECT 1 AS col1"}`,
			want:        `[{"col1":"1"}]`,
		},
		{
			name:        "invoke pg read-only execute sql table query",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestBody: fmt.Sprintf(`{"sql":"SELECT id, name FROM %s WHERE id = 3 OR name = 'Alice' ORDER BY id"}`, tableNameParam),
			want:        wantRows,
		},
		{
			name:        "invoke pg read-only execute sql blocks create table",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestBody: createNegDdl,
			isErr:       true,
		},
		{
			name:        "invoke pg read-only execute sql blocks insert",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool-read-only/invoke",
			requestBody: fmt.Sprintf(`{"sql":"INSERT INTO %s (id, name) VALUES (5, 'blocked')"}`, tableNameParam),
			isErr:       true,
		},
		{
			name:        "invoke pg execute sql blocks create table",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestBody: createNegDdl,
			isErr:       true,
		},
		{
			name:        "invoke pg execute sql insert",
			api:         "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke",
			requestBody: fmt.Sprintf(`{"sql":"INSERT INTO %s (id, name) VALUES (5, 'test_name')"}`, tableNameParam),
			want:        "[]",
		},
	})

	if len(expectedTables) == 0 {
		expectedTables = []string{tableNameParam}
	}
	runSpannerListTablesTest(t, expectedTables...)
}

func runSpannerSchemaToolInvokeTest(t *testing.T, accessSchemaWant string) {
	runSpannerInvokeCases(t, []spannerInvokeCase{
		{
			name:        "invoke list-tables-read-only",
			api:         "http://127.0.0.1:5000/api/tool/access-schema-read-only/invoke",
			requestBody: `{}`,
			want:        accessSchemaWant,
		},
		{
			name:        "invoke list-tables",
			api:         "http://127.0.0.1:5000/api/tool/access-schema/invoke",
			requestBody: `{}`,
			isErr:       true,
		},
	})
}
