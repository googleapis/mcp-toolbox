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

package clickhouse

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	ClickHouseSourceType = "clickhouse"
	ClickHouseToolType   = "clickhouse-sql"
	ClickHouseDatabase   = os.Getenv("CLICKHOUSE_DATABASE")
	ClickHouseHost       = os.Getenv("CLICKHOUSE_HOST")
	ClickHousePort       = os.Getenv("CLICKHOUSE_PORT")
	ClickHouseUser       = os.Getenv("CLICKHOUSE_USER")
	ClickHousePass       = os.Getenv("CLICKHOUSE_PASS")
	ClickHouseProtocol   = os.Getenv("CLICKHOUSE_PROTOCOL")
)

func getClickHouseVars(t *testing.T) map[string]any {
	switch "" {
	case ClickHouseHost:
		t.Skip("'CLICKHOUSE_HOST' not set")
	case ClickHousePort:
		t.Skip("'CLICKHOUSE_PORT' not set")
	case ClickHouseUser:
		t.Skip("'CLICKHOUSE_USER' not set")
	}

	// Set defaults for optional parameters
	if ClickHouseDatabase == "" {
		ClickHouseDatabase = "default"
	}
	if ClickHouseProtocol == "" {
		ClickHouseProtocol = "http"
	}

	return map[string]any{
		"type":     ClickHouseSourceType,
		"host":     ClickHouseHost,
		"port":     ClickHousePort,
		"database": ClickHouseDatabase,
		"user":     ClickHouseUser,
		"password": ClickHousePass,
		"protocol": ClickHouseProtocol,
		"secure":   false,
	}
}

// initClickHouseConnectionPool creates a ClickHouse connection using HTTP protocol only.
// Note: ClickHouse tools in this codebase only support HTTP/HTTPS protocols, not the native protocol.
// Typical setup: localhost:8123 (HTTP) or localhost:8443 (HTTPS)
func initClickHouseConnectionPool(host, port, user, pass, dbname, protocol string) (*sql.DB, error) {
	if protocol == "" {
		protocol = "https"
	}

	var dsn string
	switch protocol {
	case "http":
		dsn = fmt.Sprintf("http://%s:%s@%s:%s/%s", user, pass, host, port, dbname)
	case "https":
		dsn = fmt.Sprintf("https://%s:%s@%s:%s/%s?secure=true&skip_verify=false", user, pass, host, port, dbname)
	default:
		dsn = fmt.Sprintf("https://%s:%s@%s:%s/%s?secure=true&skip_verify=false", user, pass, host, port, dbname)
	}

	pool, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	return pool, nil
}

func TestClickHouse(t *testing.T) {
	sourceConfig := getClickHouseVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	pool, err := initClickHouseConnectionPool(ClickHouseHost, ClickHousePort, ClickHouseUser, ClickHousePass, ClickHouseDatabase, ClickHouseProtocol)
	if err != nil {
		t.Fatalf("unable to create ClickHouse connection pool: %s", err)
	}
	defer pool.Close()

	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getClickHouseSQLParamToolInfo(tableNameParam)
	teardownTable1 := setupClickHouseSQLTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getClickHouseSQLAuthToolInfo(tableNameAuth)
	teardownTable2 := setupClickHouseSQLTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)

	toolsFile := tests.GetToolsConfig(sourceConfig, ClickHouseToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addClickHouseExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := getClickHouseSQLTmplToolStatement()
	toolsFile = addClickHouseTemplateParamConfig(t, toolsFile, ClickHouseToolType, tmplSelectCombined, tmplSelectFilterCombined)

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
	select1Want, mcpSelect1Want, mcpMyFailToolWant, createTableStatement, nilIdWant := getClickHouseWants()

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want, tests.WithMyToolById4Want(nilIdWant))
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)
}

func addClickHouseExecuteSqlConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "clickhouse-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "clickhouse-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	config["tools"] = tools
	return config
}

func addClickHouseTemplateParamConfig(t *testing.T, config map[string]any, toolType, tmplSelectCombined, tmplSelectFilterCombined string) map[string]any {
	toolsMap, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}

	// ClickHouse-specific template parameter tools with compatible syntax
	toolsMap["create-table-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "CREATE TABLE {{.tableName}} ({{array .columns}}) ORDER BY id",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewArrayParameter("columns", "The columns to create", parameters.NewStringParameter("column", "A column name that will be created")),
		},
	}
	toolsMap["insert-table-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Insert table tool with template parameters",
		"statement":   "INSERT INTO {{.tableName}} ({{array .columns}}) VALUES ({{.values}})",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewArrayParameter("columns", "The columns to insert into", parameters.NewStringParameter("column", "A column name that will be returned from the query.")),
			parameters.NewStringParameter("values", "The values to insert as a comma separated string"),
		},
	}
	toolsMap["select-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Select table tool with template parameters",
		"statement":   "SELECT id AS \"id\", name AS \"name\", age AS \"age\" FROM {{.tableName}} ORDER BY id",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-templateParams-combined-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Select table tool with combined template parameters",
		"statement":   tmplSelectCombined,
		"parameters": []parameters.Parameter{
			parameters.NewIntParameter("id", "the id of the user"),
		},
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-fields-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Select specific fields tool with template parameters",
		"statement":   "SELECT name AS \"name\" FROM {{.tableName}} ORDER BY id",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-filter-templateParams-combined-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Select table tool with filter template parameters",
		"statement":   tmplSelectFilterCombined,
		"parameters": []parameters.Parameter{
			parameters.NewStringParameter("name", "the name to filter by"),
		},
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewStringParameter("columnFilter", "some description"),
		},
	}
	// Firebird uses simple DROP TABLE syntax without IF EXISTS
	toolsMap["drop-table-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Drop table tool with template parameters",
		"statement":   "DROP TABLE {{.tableName}}",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	config["tools"] = toolsMap
	return config
}

func TestClickHouseBasicConnection(t *testing.T) {
	sourceConfig := getClickHouseVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	pool, err := initClickHouseConnectionPool(ClickHouseHost, ClickHousePort, ClickHouseUser, ClickHousePass, ClickHouseDatabase, ClickHouseProtocol)
	if err != nil {
		t.Fatalf("unable to create ClickHouse connection pool: %s", err)
	}
	defer pool.Close()

	// Test basic connection
	err = pool.PingContext(ctx)
	if err != nil {
		t.Fatalf("unable to ping ClickHouse: %s", err)
	}

	// Test basic query
	rows, err := pool.QueryContext(ctx, "SELECT 1 as test_value")
	if err != nil {
		t.Fatalf("unable to execute basic query: %s", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatalf("expected at least one row from basic query")
	}

	var testValue int
	err = rows.Scan(&testValue)
	if err != nil {
		t.Fatalf("unable to scan result: %s", err)
	}

	if testValue != 1 {
		t.Fatalf("expected test_value to be 1, got %d", testValue)
	}

	// Write a basic tools config and test the server endpoint (without auth services)
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{
			"my-simple-tool": map[string]any{
				"type":        ClickHouseToolType,
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
				"statement":   "SELECT 1;",
			},
		},
	}

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

	tests.RunToolGetTest(t)
	t.Logf("✅ ClickHouse basic connection test completed successfully")
}

func getClickHouseWants() (string, string, string, string, string) {
	select1Want := "[{\"1\":1}]"
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"1\":1}"}]}}`
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: sendQuery: [HTTP 400] response body: \"Code: 62. DB::Exception: Syntax error: failed at position 1 (SELEC): SELEC 1;. Expected one of: Query, Query with output, EXPLAIN, EXPLAIN, SELECT query, possibly with UNION, list of union elements, SELECT query, subquery, possibly with UNION, SELECT subquery, SELECT query, WITH, FROM, SELECT, SHOW CREATE QUOTA query, SHOW CREATE, SHOW [FULL] [TEMPORARY] TABLES|DATABASES|CLUSTERS|CLUSTER|MERGES 'name' [[NOT] [I]LIKE 'str'] [LIMIT expr], SHOW, SHOW COLUMNS query, SHOW ENGINES query, SHOW ENGINES, SHOW FUNCTIONS query, SHOW FUNCTIONS, SHOW INDEXES query, SHOW SETTING query, SHOW SETTING, EXISTS or SHOW CREATE query, EXISTS, DESCRIBE FILESYSTEM CACHE query, DESCRIBE, DESC, DESCRIBE query, SHOW PROCESSLIST query, SHOW PROCESSLIST, CREATE TABLE or ATTACH TABLE query, CREATE, ATTACH, REPLACE, CREATE DATABASE query, CREATE VIEW query, CREATE DICTIONARY, CREATE LIVE VIEW query, CREATE WINDOW VIEW query, ALTER query, ALTER TABLE, ALTER TEMPORARY TABLE, ALTER DATABASE, RENAME query, RENAME DATABASE, RENAME TABLE, EXCHANGE TABLES, RENAME DICTIONARY, EXCHANGE DICTIONARIES, RENAME, DROP query, DROP, DETACH, TRUNCATE, UNDROP query, UNDROP, CHECK ALL TABLES, CHECK TABLE, KILL QUERY query, KILL, OPTIMIZE query, OPTIMIZE TABLE, WATCH query, WATCH, SHOW ACCESS query, SHOW ACCESS, ShowAccessEntitiesQuery, SHOW GRANTS query, SHOW GRANTS, SHOW PRIVILEGES query, SHOW PRIVILEGES, BACKUP or RESTORE query, BACKUP, RESTORE, INSERT query, INSERT INTO, USE query, USE, SET ROLE or SET DEFAULT ROLE query, SET ROLE DEFAULT, SET ROLE, SET DEFAULT ROLE, SET query, SET, SYSTEM query, SYSTEM, CREATE USER or ALTER USER query, ALTER USER, CREATE USER, CREATE ROLE or ALTER ROLE query, ALTER ROLE, CREATE ROLE, CREATE QUOTA or ALTER QUOTA query, ALTER QUOTA, CREATE QUOTA, CREATE ROW POLICY or ALTER ROW POLICY query, ALTER POLICY, ALTER ROW POLICY, CREATE POLICY, CREATE ROW POLICY, CREATE SETTINGS PROFILE or ALTER SETTINGS PROFILE query, ALTER SETTINGS PROFILE, ALTER PROFILE, CREATE SETTINGS PROFILE, CREATE PROFILE, CREATE FUNCTION query, DROP FUNCTION query, CREATE WORKLOAD query, DROP WORKLOAD query, CREATE RESOURCE query, DROP RESOURCE query, CREATE NAMED COLLECTION, DROP NAMED COLLECTION query, Alter NAMED COLLECTION query, ALTER, CREATE INDEX query, DROP INDEX query, DROP access entity query, MOVE access entity query, MOVE, GRANT or REVOKE query, REVOKE, GRANT, CHECK GRANT, CHECK GRANT, EXTERNAL DDL query, EXTERNAL DDL FROM, TCL query, BEGIN TRANSACTION, START TRANSACTION, COMMIT, ROLLBACK, SET TRANSACTION SNAPSHOT, Delete query, DELETE, Update query, UPDATE. (SYNTAX_ERROR) (version 25.7.5.34 (official build))\n\""}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id UInt32, name String) ENGINE = Memory"`
	nullWant := `[{"id":4,"name":""}]`
	return select1Want, mcpSelect1Want, mcpMyFailToolWant, createTableStatement, nullWant
}

func TestClickHouseSQLTool(t *testing.T) {
	_ = getClickHouseVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := initClickHouseConnectionPool(ClickHouseHost, ClickHousePort, ClickHouseUser, ClickHousePass, ClickHouseDatabase, ClickHouseProtocol)
	if err != nil {
		t.Fatalf("unable to create ClickHouse connection pool: %s", err)
	}
	defer pool.Close()

	tableName := "test_sql_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE %s (
			id UInt32,
			name String,
			age UInt8,
			created_at DateTime DEFAULT now()
		) ENGINE = Memory
	`, tableName)

	_, err = pool.ExecContext(ctx, createTableSQL)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}
	defer func() {
		_, _ = pool.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
	}()

	insertSQL := fmt.Sprintf("INSERT INTO %s (id, name, age) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)", tableName)
	_, err = pool.ExecContext(ctx, insertSQL, 1, "Alice", 25, 2, "Bob", 30, 3, "Charlie", 35)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": getClickHouseVars(t),
		},
		"tools": map[string]any{
			"test-select": map[string]any{
				"type":        ClickHouseToolType,
				"source":      "my-instance",
				"description": "Test select query",
				"statement":   fmt.Sprintf("SELECT * FROM %s ORDER BY id", tableName),
			},
			"test-param-query": map[string]any{
				"type":        ClickHouseToolType,
				"source":      "my-instance",
				"description": "Test parameterized query",
				"statement":   fmt.Sprintf("SELECT * FROM %s WHERE age > ? ORDER BY id", tableName),
				"parameters": []parameters.Parameter{
					parameters.NewIntParameter("min_age", "Minimum age"),
				},
			},
			"test-empty-result": map[string]any{
				"type":        ClickHouseToolType,
				"source":      "my-instance",
				"description": "Test query with no results",
				"statement":   fmt.Sprintf("SELECT * FROM %s WHERE id = ?", tableName),
				"parameters": []parameters.Parameter{
					parameters.NewIntParameter("id", "Record ID"),
				},
			},
			"test-invalid-sql": map[string]any{
				"type":        ClickHouseToolType,
				"source":      "my-instance",
				"description": "Test invalid SQL",
				"statement":   "SELEC * FROM nonexistent_table", // Typo in SELECT
			},
		},
	}

	args := []string{"--enable-api"}
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

	tcs := []struct {
		name           string
		toolName       string
		requestBody    []byte
		resultSliceLen int
		isErr          bool
	}{
		{
			name:           "SimpleSelect",
			toolName:       "test-select",
			requestBody:    []byte(`{}`),
			resultSliceLen: 3,
		},
		{
			name:           "ParameterizedQuery",
			toolName:       "test-param-query",
			requestBody:    []byte(`{"min_age": 28}`),
			resultSliceLen: 2,
		},
		{
			name:           "EmptyResult",
			toolName:       "test-empty-result",
			requestBody:    []byte(`{"id": 999}`), // non-existent id
			resultSliceLen: 0,
		},
		{
			name:        "InvalidSQL",
			toolName:    "test-invalid-sql",
			requestBody: []byte(``),
			isErr:       true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
			resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer(tc.requestBody), nil)
			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
			}

			var body map[string]interface{}
			err := json.Unmarshal(respBody, &body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}
			t.Logf("result is %s", got)

			var res []any
			err = json.Unmarshal([]byte(got), &res)
			if err != nil {
				t.Fatalf("error parsing result")
			}

			if len(res) != tc.resultSliceLen {
				t.Errorf("Expected %d results, got %d", tc.resultSliceLen, len(res))
			}
		})
	}

	t.Logf("✅ clickhouse-sql tool tests completed successfully")
}

func TestClickHouseExecuteSQLTool(t *testing.T) {
	_ = getClickHouseVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := initClickHouseConnectionPool(ClickHouseHost, ClickHousePort, ClickHouseUser, ClickHousePass, ClickHouseDatabase, ClickHouseProtocol)
	if err != nil {
		t.Fatalf("unable to create ClickHouse connection pool: %s", err)
	}
	defer pool.Close()

	tableName := "test_exec_sql_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": getClickHouseVars(t),
		},
		"tools": map[string]any{
			"execute-sql-tool": map[string]any{
				"type":        "clickhouse-execute-sql",
				"source":      "my-instance",
				"description": "Test create table",
			},
		},
	}

	args := []string{"--enable-api"}
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
	tcs := []struct {
		name           string
		sql            string
		resultSliceLen int
		isErr          bool
		isAgentErr     bool
	}{
		{
			name:           "CreateTable",
			sql:            fmt.Sprintf(`CREATE TABLE %s (id UInt32, data String) ENGINE = Memory`, tableName),
			resultSliceLen: 0,
		},
		{
			name:           "InsertData",
			sql:            fmt.Sprintf("INSERT INTO %s (id, data) VALUES (1, 'test1'), (2, 'test2')", tableName),
			resultSliceLen: 0,
		},
		{
			name:           "SelectData",
			sql:            fmt.Sprintf("SELECT * FROM %s ORDER BY id", tableName),
			resultSliceLen: 2,
		},
		{
			name:           "DropTable",
			sql:            fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName),
			resultSliceLen: 0,
		},
		{
			name:       "MissingSQL",
			sql:        "",
			isAgentErr: true,
		},

		{
			name:       "SQLInjectionAttempt",
			sql:        "SELECT 1; DROP TABLE system.users; SELECT 2",
			isAgentErr: true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			param := fmt.Sprintf(`{"sql": "%s"}`, tc.sql)
			api := "http://127.0.0.1:5000/api/tool/execute-sql-tool/invoke"
			resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer([]byte(param)), nil)
			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
			}
			if tc.isErr {
				t.Fatalf("expecting an error from server")
			}
			if tc.isAgentErr {
				return
			}

			var body map[string]interface{}
			err := json.Unmarshal(respBody, &body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			var res []any
			err = json.Unmarshal([]byte(got), &res)
			if err != nil {
				t.Fatalf("error parsing result")
			}

			if len(res) != tc.resultSliceLen {
				t.Errorf("Expected %d results, got %d", tc.resultSliceLen, len(res))
			}
		})
	}

	t.Logf("✅ clickhouse-execute-sql tool tests completed successfully")
}

func TestClickHouseEdgeCases(t *testing.T) {
	_ = getClickHouseVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := initClickHouseConnectionPool(ClickHouseHost, ClickHousePort, ClickHouseUser, ClickHousePass, ClickHouseDatabase, ClickHouseProtocol)
	if err != nil {
		t.Fatalf("unable to create ClickHouse connection pool: %s", err)
	}
	defer pool.Close()

	tableName := "test_nulls_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": getClickHouseVars(t),
		},
		"tools": map[string]any{
			"execute-sql-tool": map[string]any{
				"type":        "clickhouse-execute-sql",
				"source":      "my-instance",
				"description": "Test create table",
			},
			"test-null-values": map[string]any{
				"type":        "clickhouse-sql",
				"source":      "my-instance",
				"description": "Test null values",
				"statement":   fmt.Sprintf("SELECT * FROM %s ORDER BY id", tableName),
			},
			"test-concurrent": map[string]any{
				"type":        "clickhouse-sql",
				"source":      "my-instance",
				"description": "Test concurrent queries",
				"statement":   "SELECT number FROM system.numbers LIMIT ?",
				"parameters": []parameters.Parameter{
					parameters.NewIntParameter("limit", "Limit"),
				},
			},
		},
	}

	args := []string{"--enable-api"}
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
	t.Run("VeryLongQuery", func(t *testing.T) {
		// Create a very long but valid query
		var conditions []string
		for i := 1; i <= 100; i++ {
			conditions = append(conditions, fmt.Sprintf("(%d = %d)", i, i))
		}
		longQuery := "SELECT 1 WHERE " + strings.Join(conditions, " AND ")

		api := "http://127.0.0.1:5000/api/tool/execute-sql-tool/invoke"
		param := fmt.Sprintf(`{"sql": "%s"}`, longQuery)
		resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer([]byte(param)), nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var body map[string]interface{}
		err := json.Unmarshal(respBody, &body)
		if err != nil {
			t.Fatalf("error parsing response body")
		}

		got, ok := body["result"].(string)
		if !ok {
			t.Fatalf("unable to find result in response body")
		}

		var res []any
		err = json.Unmarshal([]byte(got), &res)
		if err != nil {
			t.Fatalf("error parsing result")
		}

		// Should return [{1:1}]
		if len(res) != 1 {
			t.Errorf("Expected 1 result from long query, got %d", len(res))
		}
	})

	t.Run("NullValues", func(t *testing.T) {
		createSQL := fmt.Sprintf(`
			CREATE TABLE %s (
				id UInt32,
				nullable_field Nullable(String)
			) ENGINE = Memory
		`, tableName)

		_, err = pool.ExecContext(ctx, createSQL)
		if err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}
		defer func() {
			_, _ = pool.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
		}()

		// Insert null value
		insertSQL := fmt.Sprintf("INSERT INTO %s (id, nullable_field) VALUES (1, NULL), (2, 'not null')", tableName)
		_, err = pool.ExecContext(ctx, insertSQL)
		if err != nil {
			t.Fatalf("Failed to insert null value: %v", err)
		}

		api := "http://127.0.0.1:5000/api/tool/test-null-values/invoke"
		resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer([]byte(`{}`)), nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var body map[string]interface{}
		err := json.Unmarshal(respBody, &body)
		if err != nil {
			t.Fatalf("error parsing response body")
		}

		got, ok := body["result"].(string)
		if !ok {
			t.Fatalf("unable to find result in response body")
		}

		var res []any
		err = json.Unmarshal([]byte(got), &res)
		if err != nil {
			t.Fatalf("error parsing result")
		}

		if len(res) != 2 {
			t.Errorf("Expected 2 result from long query, got %d", len(res))
		}

		// Check that null is properly handled
		if firstRow, ok := res[0].(map[string]any); ok {
			if _, hasNullableField := firstRow["nullable_field"]; !hasNullableField {
				t.Error("Expected nullable_field in result")
			}
		}
	})

	t.Run("ConcurrentQueries", func(t *testing.T) {
		// Run multiple queries concurrently
		done := make(chan bool, 5)
		for i := 0; i < 5; i++ {
			go func(n int) {
				defer func() { done <- true }()

				params := fmt.Sprintf(`{"limit": %d}`, n+1)
				api := "http://127.0.0.1:5000/api/tool/test-concurrent/invoke"
				resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer([]byte(params)), nil)
				if resp.StatusCode != http.StatusOK {
					t.Errorf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
				}

				var body map[string]interface{}
				err := json.Unmarshal(respBody, &body)
				if err != nil {
					t.Errorf("error parsing response body")
				}

				got, ok := body["result"].(string)
				if !ok {
					t.Errorf("unable to find result in response body")
				}

				var res []any
				err = json.Unmarshal([]byte(got), &res)
				if err != nil {
					t.Errorf("error parsing result")
				}

				if len(res) != n+1 {
					t.Errorf("Query %d: expected %d results, got %d", n, n+1, len(res))
				}
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 5; i++ {
			<-done
		}
	})

	t.Logf("✅ Edge case tests completed successfully")
}

// getClickHouseSQLParamToolInfo returns statements and param for my-tool clickhouse-sql type
func getClickHouseSQLParamToolInfo(tableName string) (string, string, string, string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id UInt32, name String) ENGINE = Memory", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (id, name) VALUES (?, ?), (?, ?), (?, ?), (?, ?)", tableName)
	paramStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ? OR name = ?", tableName)
	idParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ?", tableName)
	nameParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = ?", tableName)
	arrayStatement := fmt.Sprintf("SELECT * FROM %s WHERE id IN (?) AND name IN (?)", tableName)
	params := []any{1, "Alice", 2, "Bob", 3, "Sid", 4, nil}
	return createStatement, insertStatement, paramStatement, idParamStatement, nameParamStatement, arrayStatement, params
}

// getClickHouseSQLAuthToolInfo returns statements and param of my-auth-tool for clickhouse-sql type
func getClickHouseSQLAuthToolInfo(tableName string) (string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id UInt32, name String, email String) ENGINE = Memory", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (id, name, email) VALUES (?, ?, ?), (?, ?, ?)", tableName)
	authStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = ?", tableName)
	params := []any{1, "Alice", tests.ServiceAccountEmail, 2, "jane", "janedoe@gmail.com"}
	return createStatement, insertStatement, authStatement, params
}

// getClickHouseSQLTmplToolStatement returns statements and param for template parameter test cases for clickhouse-sql type
func getClickHouseSQLTmplToolStatement() (string, string) {
	tmplSelectCombined := "SELECT * FROM {{.tableName}} WHERE id = ?"
	tmplSelectFilterCombined := "SELECT * FROM {{.tableName}} WHERE {{.columnFilter}} = ?"
	return tmplSelectCombined, tmplSelectFilterCombined
}

// SetupClickHouseSQLTable creates and inserts data into a table of tool
// compatible with clickhouse-sql tool
func setupClickHouseSQLTable(t *testing.T, ctx context.Context, pool *sql.DB, createStatement, insertStatement, tableName string, params []any) func(*testing.T) {
	err := pool.PingContext(ctx)
	if err != nil {
		t.Fatalf("unable to connect to test database: %s", err)
	}

	// Create table
	_, err = pool.ExecContext(ctx, createStatement)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	_, err = pool.ExecContext(ctx, insertStatement, params...)
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

func TestClickHouseListDatabasesTool(t *testing.T) {
	_ = getClickHouseVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := initClickHouseConnectionPool(ClickHouseHost, ClickHousePort, ClickHouseUser, ClickHousePass, ClickHouseDatabase, ClickHouseProtocol)
	if err != nil {
		t.Fatalf("unable to create ClickHouse connection pool: %s", err)
	}
	defer pool.Close()

	// Create a test database
	testDBName := "test_list_db_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	_, err = pool.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", testDBName))
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() {
		_, _ = pool.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))
	}()

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": getClickHouseVars(t),
		},
		"tools": map[string]any{
			"test-list-databases": map[string]any{
				"type":        "clickhouse-list-databases",
				"source":      "my-instance",
				"description": "Test listing databases",
			},
		},
	}

	args := []string{"--enable-api"}
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

	t.Run("ListDatabases", func(t *testing.T) {
		api := "http://127.0.0.1:5000/api/tool/test-list-databases/invoke"
		resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer([]byte(`{}`)), nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var body map[string]interface{}
		err := json.Unmarshal(respBody, &body)
		if err != nil {
			t.Fatalf("error parsing response body")
		}

		databases, ok := body["result"].(string)
		if !ok {
			t.Fatalf("unable to find result in response body")
		}
		var res []map[string]any
		err = json.Unmarshal([]byte(databases), &res)
		if err != nil {
			t.Errorf("error parsing result")
		}

		// Should contain at least the default database and our test database - system and default
		if len(res) < 2 {
			t.Errorf("Expected at least 2 databases, got %d", len(res))
		}

		found := false
		foundDefault := false
		for _, db := range res {
			if name, ok := db["name"].(string); ok {
				if name == testDBName {
					found = true
				}
				if name == "default" || name == "system" {
					foundDefault = true
				}
			}
		}

		if !found {
			t.Errorf("Test database %s not found in list", testDBName)
		}
		if !foundDefault {
			t.Errorf("Default/system database not found in list")
		}

		t.Logf("Successfully listed %d databases", len(databases))
	})

	t.Logf("✅ clickhouse-list-databases tool tests completed successfully")
}

func TestClickHouseListTablesTool(t *testing.T) {
	_ = getClickHouseVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := initClickHouseConnectionPool(ClickHouseHost, ClickHousePort, ClickHouseUser, ClickHousePass, ClickHouseDatabase, ClickHouseProtocol)
	if err != nil {
		t.Fatalf("unable to create ClickHouse connection pool: %s", err)
	}
	defer pool.Close()

	// Create a test database with tables
	testDBName := "test_list_tables_db_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	_, err = pool.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", testDBName))
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() {
		_, _ = pool.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))
	}()

	// Create test tables in the test database
	testTable1 := "test_table_1"
	testTable2 := "test_table_2"
	_, err = pool.ExecContext(ctx, fmt.Sprintf("CREATE TABLE %s.%s (id UInt32, name String) ENGINE = Memory", testDBName, testTable1))
	if err != nil {
		t.Fatalf("Failed to create test table 1: %v", err)
	}
	_, err = pool.ExecContext(ctx, fmt.Sprintf("CREATE TABLE %s.%s (id UInt32, value Float64) ENGINE = Memory", testDBName, testTable2))
	if err != nil {
		t.Fatalf("Failed to create test table 2: %v", err)
	}

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": getClickHouseVars(t),
		},
		"tools": map[string]any{
			"test-list-tables": map[string]any{
				"type":        "clickhouse-list-tables",
				"source":      "my-instance",
				"description": "Test listing tables",
			},
		},
	}

	args := []string{"--enable-api"}
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

	t.Run("ListTables", func(t *testing.T) {
		api := "http://127.0.0.1:5000/api/tool/test-list-tables/invoke"
		params := fmt.Sprintf(`{"database": "%s"}`, testDBName)
		resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer([]byte(params)), nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var body map[string]interface{}
		err := json.Unmarshal(respBody, &body)
		if err != nil {
			t.Fatalf("error parsing response body")
		}

		tables, ok := body["result"].(string)
		if !ok {
			t.Fatalf("Expected result to be []map[string]any, got %T", tables)
		}
		var res []map[string]any
		err = json.Unmarshal([]byte(tables), &res)
		if err != nil {
			t.Errorf("error parsing result")
		}

		// Should contain exactly 2 tables that we created
		if len(res) != 2 {
			t.Errorf("Expected 2 tables, got %d", len(res))
		}

		foundTable1 := false
		foundTable2 := false
		for _, table := range res {
			if name, ok := table["name"].(string); ok {
				if name == testTable1 {
					foundTable1 = true
				}
				if name == testTable2 {
					foundTable2 = true
				}
				// Verify database field is set correctly
				if db, ok := table["database"].(string); ok {
					if db != testDBName {
						t.Errorf("Expected database to be %s, got %s", testDBName, db)
					}
				}
			}
		}

		if !foundTable1 {
			t.Errorf("Test table %s not found in list", testTable1)
		}
		if !foundTable2 {
			t.Errorf("Test table %s not found in list", testTable2)
		}

		t.Logf("Successfully listed %d tables from database %s", len(tables), testDBName)
	})

	t.Run("ListTablesWithMissingDatabase", func(t *testing.T) {
		api := "http://127.0.0.1:5000/api/tool/test-list-tables/invoke"
		resp, _ := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer([]byte(`{}`)), nil)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 OK for missing database parameter, but got %d", resp.StatusCode)
		}
	})

	t.Logf("✅ clickhouse-list-tables tool tests completed successfully")
}

// TestClickHouseSQLToolWithEmbedding verifies that the clickhouse-sql tool can
// embed a string parameter via an embedding model and store/query an
// Array(Float32) column. Skips when ClickHouse infra is not configured;
// requires API_KEY (gemini) when it is.
func TestClickHouseSQLToolWithEmbedding(t *testing.T) {
	// Skip if ClickHouse infra isn't configured (matches the rest of this
	// suite). If it is, a missing API_KEY is a real misconfiguration (e.g. the
	// CI secret was dropped) — fail loudly rather than skip silently.
	sourceConfig := getClickHouseVars(t)
	if os.Getenv("API_KEY") == "" {
		t.Fatal("'API_KEY' not set; required for the embedding integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	pool, err := initClickHouseConnectionPool(ClickHouseHost, ClickHousePort, ClickHouseUser, ClickHousePass, ClickHouseDatabase, ClickHouseProtocol)
	if err != nil {
		t.Fatalf("unable to create ClickHouse connection pool: %s", err)
	}
	defer pool.Close()

	vectorTableName, tearDownVectorTable := setupClickHouseVectorTable(t, ctx, pool)
	defer tearDownVectorTable(t)

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{},
	}

	insertStmt, searchStmt := getClickHouseVectorSearchStmts(vectorTableName)
	toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, ClickHouseToolType, insertStmt, searchStmt)

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

	tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox")
}

// setupClickHouseVectorTable creates a ClickHouse table with an Array(Float32)
// embedding column for the semantic search test, and returns its name plus a
// teardown helper.
func setupClickHouseVectorTable(t *testing.T, ctx context.Context, pool *sql.DB) (string, func(*testing.T)) {
	t.Helper()

	tableName := "vector_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	createTableStmt := fmt.Sprintf(`CREATE TABLE %s (
		content String,
		embedding Array(Float32)
	) ENGINE = MergeTree ORDER BY tuple()`, tableName)

	if _, err := pool.ExecContext(ctx, createTableStmt); err != nil {
		t.Fatalf("failed to create table %s: %v", tableName, err)
	}

	return tableName, func(t *testing.T) {
		if _, err := pool.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)); err != nil {
			t.Errorf("failed to drop table %s: %v", tableName, err)
		}
	}
}

// getClickHouseVectorSearchStmts returns the insert and cosine-distance search
// statements used by the ClickHouse semantic-search integration test.
func getClickHouseVectorSearchStmts(vectorTableName string) (string, string) {
	insertStmt := fmt.Sprintf("INSERT INTO %s (content, embedding) VALUES (?, ?)", vectorTableName)
	searchStmt := fmt.Sprintf("SELECT content, cosineDistance(embedding, ?) AS distance FROM %s ORDER BY distance ASC LIMIT 1", vectorTableName)
	return insertStmt, searchStmt
}
