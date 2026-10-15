// Copyright 2025 Google LLC
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

package trino

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	_ "github.com/trinodb/trino-go-client/trino" // Import Trino SQL driver
)

var (
	TrinoSourceType = "trino"
	TrinoToolType   = "trino-sql"
	TrinoHost       = os.Getenv("TRINO_HOST")
	TrinoPort       = os.Getenv("TRINO_PORT")
	TrinoUser       = os.Getenv("TRINO_USER")
	TrinoPass       = os.Getenv("TRINO_PASS")
	TrinoCatalog    = os.Getenv("TRINO_CATALOG")
	TrinoSchema     = os.Getenv("TRINO_SCHEMA")
)

func getTrinoVars(t *testing.T) map[string]any {
	switch "" {
	case TrinoHost:
		t.Fatal("'TRINO_HOST' not set")
	case TrinoPort:
		t.Fatal("'TRINO_PORT' not set")
	// TrinoUser is optional for anonymous access
	case TrinoCatalog:
		t.Fatal("'TRINO_CATALOG' not set")
	case TrinoSchema:
		t.Fatal("'TRINO_SCHEMA' not set")
	}

	return map[string]any{
		"type":     TrinoSourceType,
		"host":     TrinoHost,
		"port":     TrinoPort,
		"user":     TrinoUser,
		"password": TrinoPass,
		"catalog":  TrinoCatalog,
		"schema":   TrinoSchema,
	}
}

// initTrinoConnectionPool creates a Trino connection pool (copied from trino.go)
func initTrinoConnectionPool(host, port, user, pass, catalog, schema string) (*sql.DB, error) {
	dsn, err := buildTrinoDSN(host, port, user, pass, catalog, schema, "", "", false, false)
	if err != nil {
		return nil, fmt.Errorf("failed to build DSN: %w", err)
	}

	db, err := sql.Open("trino", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// buildTrinoDSN builds a Trino DSN string (simplified version from trino.go)
func buildTrinoDSN(host, port, user, password, catalog, schema, queryTimeout, accessToken string, kerberosEnabled, sslEnabled bool) (string, error) {
	scheme := "http"
	if sslEnabled {
		scheme = "https"
	}

	// Build base DSN without user info
	dsn := fmt.Sprintf("%s://%s:%s?catalog=%s&schema=%s", scheme, host, port, catalog, schema)

	// Add user authentication if provided
	if user != "" {
		if password != "" {
			dsn = fmt.Sprintf("%s://%s:%s@%s:%s?catalog=%s&schema=%s", scheme, user, password, host, port, catalog, schema)
		} else {
			dsn = fmt.Sprintf("%s://%s@%s:%s?catalog=%s&schema=%s", scheme, user, host, port, catalog, schema)
		}
	}

	if queryTimeout != "" {
		dsn += "&queryTimeout=" + queryTimeout
	}

	if accessToken != "" {
		dsn += "&accessToken=" + accessToken
	}

	if kerberosEnabled {
		dsn += "&KerberosEnabled=true"
	}

	return dsn, nil
}

// getTrinoParamToolInfo returns statements and param for my-tool trino-sql type
func getTrinoParamToolInfo(tableName string) (string, string, string, string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id BIGINT NOT NULL, name VARCHAR(255))", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (id, name) VALUES (1, ?), (2, ?), (3, ?), (4, ?)", tableName)
	toolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ? OR name = ?", tableName)
	idParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ?", tableName)
	nameParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = ?", tableName)
	arrayToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id IN (?, ?) AND name IN (?, ?)", tableName) // Trino doesn't use ANY() like MySQL/PostgreSQL
	params := []any{"Alice", "Jane", "Sid", nil}
	return createStatement, insertStatement, toolStatement, idParamStatement, nameParamStatement, arrayToolStatement, params
}

// getTrinoAuthToolInfo returns statements and param of my-auth-tool for trino-sql type
func getTrinoAuthToolInfo(tableName string) (string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id BIGINT NOT NULL, name VARCHAR(255), email VARCHAR(255))", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (id, name, email) VALUES (1, ?, ?), (2, ?, ?)", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = ?", tableName)
	params := []any{"Alice", tests.ServiceAccountEmail, "Jane", "janedoe@gmail.com"}
	return createStatement, insertStatement, toolStatement, params
}

// getTrinoTmplToolStatement returns statements and param for template parameter test cases for trino-sql type
func getTrinoTmplToolStatement() (string, string) {
	tmplSelectCombined := "SELECT * FROM {{.tableName}} WHERE id = ?"
	tmplSelectFilterCombined := "SELECT * FROM {{.tableName}} WHERE {{.columnFilter}} = ?"
	return tmplSelectCombined, tmplSelectFilterCombined
}

// getTrinoWants return the expected wants for trino
func getTrinoWants() (string, string, string, string) {
	select1Want := `[{"_col0":1}]`
	failInvocationWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: trino: query failed (200 OK): \"USER_ERROR: line 1:1: mismatched input 'SELEC'. Expecting: 'ALTER', 'ANALYZE', 'CALL', 'COMMENT', 'COMMIT', 'CREATE', 'DEALLOCATE', 'DELETE', 'DENY', 'DESC', 'DESCRIBE', 'DROP', 'EXECUTE', 'EXPLAIN', 'GRANT', 'INSERT', 'MERGE', 'PREPARE', 'REFRESH', 'RESET', 'REVOKE', 'ROLLBACK', 'SET', 'SHOW', 'START', 'TRUNCATE', 'UPDATE', 'USE', 'WITH', \u003cquery\u003e\""}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id BIGINT NOT NULL, name VARCHAR(255))"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"_col0\":1}"}]}}`
	return select1Want, failInvocationWant, createTableStatement, mcpSelect1Want
}

// setupTrinoTable creates and inserts data into a table of tool
// compatible with trino-sql tool
func setupTrinoTable(t *testing.T, ctx context.Context, pool *sql.DB, createStatement, insertStatement, tableName string, params []any) func(*testing.T) {
	err := pool.PingContext(ctx)
	if err != nil {
		t.Fatalf("unable to connect to test database: %s", err)
	}

	// Create table
	_, err = pool.QueryContext(ctx, createStatement)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	_, err = pool.QueryContext(ctx, insertStatement, params...)
	if err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}

	return func(t *testing.T) {
		// tear down test
		_, err = pool.ExecContext(ctx, fmt.Sprintf("DROP TABLE %s", tableName))
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}
}

// addTrinoExecuteSqlConfig gets the tools config for `trino-execute-sql`
func addTrinoExecuteSqlConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "trino-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "trino-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	tools["my-impersonate-exec-sql-tool"] = map[string]any{
		"type":            "trino-execute-sql",
		"source":          "my-instance",
		"description":     "Tool to execute sql impersonating a Trino user",
		"impersonateUser": true,
	}
	config["tools"] = tools
	return config
}

func TestTrinoToolEndpoints(t *testing.T) {
	sourceConfig := getTrinoVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	pool, err := initTrinoConnectionPool(TrinoHost, TrinoPort, TrinoUser, TrinoPass, TrinoCatalog, TrinoSchema)
	if err != nil {
		t.Fatalf("unable to create Trino connection pool: %s", err)
	}

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getTrinoParamToolInfo(tableNameParam)
	teardownTable1 := setupTrinoTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getTrinoAuthToolInfo(tableNameAuth)
	teardownTable2 := setupTrinoTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, TrinoToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addTrinoExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := getTrinoTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, TrinoToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

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
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := getTrinoWants()

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithInsert1Want(`[{"rows":1}]`))

	// Verify user impersonation: the `trino_user` parameter is forwarded as the
	// X-Trino-User header, so the query runs as that user (current_user reflects
	// it rather than the source's configured user).
	impersonateParams := []byte(`{"sql": "SELECT current_user", "trino_user": "impersonated_user"}`)
	tests.RunToolInvokeParametersTest(t, "my-impersonate-exec-sql-tool", impersonateParams, "impersonated_user")
}
