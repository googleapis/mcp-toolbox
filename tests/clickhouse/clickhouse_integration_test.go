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
	"io"
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
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// clickHouseImage is the image used for the ephemeral test container.
	clickHouseImage = "clickhouse/clickhouse-server:25.7"
	// clickHouseContainerPassword is the password set on the ephemeral test
	// container's default user.
	clickHouseContainerPassword = "toolbox-test-password"
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

// setupClickHouseContainer starts an ephemeral ClickHouse container and
// returns its host and mapped HTTP port, along with a cleanup function that
// terminates it.
func setupClickHouseContainer(ctx context.Context, t *testing.T) (string, string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        clickHouseImage,
		ExposedPorts: []string{"8123/tcp"},
		Env: map[string]string{
			"CLICKHOUSE_PASSWORD":                  clickHouseContainerPassword,
			"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT": "1",
		},
		WaitingFor: wait.ForHTTP("/ping").WithPort("8123/tcp").WithStartupTimeout(120 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start ClickHouse container: %s", err)
	}

	cleanup := func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	}

	host, err := container.Host(ctx)
	if err != nil {
		cleanup()
		t.Fatalf("failed to get container host: %s", err)
	}

	port, err := container.MappedPort(ctx, "8123")
	if err != nil {
		cleanup()
		t.Fatalf("failed to get container mapped port 8123: %s", err)
	}

	return host, port.Port(), cleanup
}

// setupClickHouseInstance points the suite at a ClickHouse instance. It
// defaults to an ephemeral container; the CLICKHOUSE_* environment variables
// take precedence when pointing the suite at an external instance. The
// package-level connection variables are restored on cleanup so each test gets
// its own container.
func setupClickHouseInstance(ctx context.Context, t *testing.T) {
	t.Helper()
	origHost, origPort, origUser, origPass := ClickHouseHost, ClickHousePort, ClickHouseUser, ClickHousePass
	origDatabase, origProtocol := ClickHouseDatabase, ClickHouseProtocol
	t.Cleanup(func() {
		ClickHouseHost, ClickHousePort, ClickHouseUser, ClickHousePass = origHost, origPort, origUser, origPass
		ClickHouseDatabase, ClickHouseProtocol = origDatabase, origProtocol
	})

	if ClickHouseHost != "" {
		return
	}
	host, port, cleanup := setupClickHouseContainer(ctx, t)
	t.Cleanup(cleanup)
	ClickHouseHost, ClickHousePort = host, port
	ClickHouseUser = "default"
	ClickHousePass = clickHouseContainerPassword
	ClickHouseDatabase = "default"
	ClickHouseProtocol = "http"
}

// openClickHousePool opens a connection pool to the configured ClickHouse
// instance and closes it on cleanup.
func openClickHousePool(t *testing.T) *sql.DB {
	t.Helper()
	pool, err := initClickHouseConnectionPool(ClickHouseHost, ClickHousePort, ClickHouseUser, ClickHousePass, ClickHouseDatabase, ClickHouseProtocol)
	if err != nil {
		t.Fatalf("unable to create ClickHouse connection pool: %s", err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Errorf("unable to close ClickHouse connection pool: %s", err)
		}
	})
	return pool
}

// clickHouseTransport selects how the tests talk to the toolbox server: the
// legacy REST API, or the MCP endpoint when isMCP is set.
type clickHouseTransport struct {
	isMCP bool
}

// clickHouseResult is the outcome of a tool invocation. result holds the tool
// result as the REST API returns it: for MCP, the content blocks are parsed and
// flattened into a single JSON array. toolErr is set when the tool reported an
// error: an MCP error result, or an {"error": ...} result over REST.
type clickHouseResult struct {
	status  int
	result  string
	toolErr bool
}

// startServer starts the toolbox server with toolsFile and waits until it is
// ready to serve. The REST API is only enabled for the non-MCP transport.
func (tr clickHouseTransport) startServer(t *testing.T, ctx context.Context, toolsFile map[string]any) {
	t.Helper()
	var args []string
	if !tr.isMCP {
		args = append(args, "--enable-api")
	}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

// invoke calls toolName with args through the selected transport.
func (tr clickHouseTransport) invoke(t *testing.T, ctx context.Context, toolName string, args map[string]any) clickHouseResult {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}

	if tr.isMCP {
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, toolName, args, nil)
		if err != nil {
			return clickHouseResult{status: statusCode, result: err.Error(), toolErr: true}
		}
		if mcpResp.Error != nil {
			return clickHouseResult{status: statusCode, result: mcpResp.Error.Message, toolErr: true}
		}
		if mcpResp.Result.IsError {
			var errText strings.Builder
			for _, content := range mcpResp.Result.Content {
				errText.WriteString(content.Text)
			}
			return clickHouseResult{status: statusCode, result: errText.String(), toolErr: true}
		}
		rows := []any{}
		for _, content := range mcpResp.Result.Content {
			var item any
			if err := json.Unmarshal([]byte(content.Text), &item); err != nil {
				rows = append(rows, content.Text)
				continue
			}
			if slice, ok := item.([]any); ok {
				rows = append(rows, slice...)
			} else {
				rows = append(rows, item)
			}
		}
		b, err := json.Marshal(rows)
		if err != nil {
			t.Fatalf("error marshaling MCP result: %s", err)
		}
		return clickHouseResult{status: statusCode, result: string(b)}
	}

	api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", toolName)
	reqBytes, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("error marshaling request body: %s", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api, bytes.NewBuffer(reqBytes))
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("error when sending a request: %s", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading response body: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return clickHouseResult{status: resp.StatusCode, result: string(respBody)}
	}

	var body map[string]any
	if err := json.Unmarshal(respBody, &body); err != nil {
		t.Fatalf("error parsing response body %q: %s", string(respBody), err)
	}
	got, ok := body["result"].(string)
	if !ok {
		got = string(respBody)
	}
	// Tool errors are returned over REST as a successful response whose result
	// is an {"error": ...} object.
	var errResult map[string]any
	toolErr := json.Unmarshal([]byte(got), &errResult) == nil && errResult["error"] != nil
	return clickHouseResult{status: resp.StatusCode, result: got, toolErr: toolErr}
}

// setupClickHouseToolsTest seeds the tables used by the shared tool fixtures
// and returns the template parameter table name and the tools file.
func setupClickHouseToolsTest(t *testing.T, ctx context.Context) (string, map[string]any) {
	t.Helper()
	setupClickHouseInstance(ctx, t)
	sourceConfig := getClickHouseVars(t)
	pool := openClickHousePool(t)

	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getClickHouseSQLParamToolInfo(tableNameParam)
	setupClickHouseSQLTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)

	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getClickHouseSQLAuthToolInfo(tableNameAuth)
	setupClickHouseSQLTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)

	toolsFile := tests.GetToolsConfig(sourceConfig, ClickHouseToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addClickHouseExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := getClickHouseSQLTmplToolStatement()
	toolsFile = addClickHouseTemplateParamConfig(t, toolsFile, ClickHouseToolType, tmplSelectCombined, tmplSelectFilterCombined)
	return tableNameTemplateParam, toolsFile
}

func TestClickHouse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tableNameTemplateParam, toolsFile := setupClickHouseToolsTest(t, ctx)
	clickHouseTransport{}.startServer(t, ctx, toolsFile)

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	setupClickHouseInstance(ctx, t)
	sourceConfig := getClickHouseVars(t)
	pool := openClickHousePool(t)

	// Test basic connection
	err := pool.PingContext(ctx)
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

	clickHouseTransport{}.startServer(t, ctx, toolsFile)

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
	runClickHouseSQLToolTest(t, clickHouseTransport{})
}

// runClickHouseSQLToolTest exercises the clickhouse-sql tool: plain and
// parameterized selects, empty results and invalid SQL.
func runClickHouseSQLToolTest(t *testing.T, tr clickHouseTransport) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	setupClickHouseInstance(ctx, t)
	sourceConfig := getClickHouseVars(t)
	pool := openClickHousePool(t)

	tableName := "test_sql_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Cleanup(func() {
		_, _ = pool.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
	})
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE %s (
			id UInt32,
			name String,
			age UInt8,
			created_at DateTime DEFAULT now()
		) ENGINE = Memory
	`, tableName)

	_, err := pool.ExecContext(ctx, createTableSQL)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	insertSQL := fmt.Sprintf("INSERT INTO %s (id, name, age) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)", tableName)
	_, err = pool.ExecContext(ctx, insertSQL, 1, "Alice", 25, 2, "Bob", 30, 3, "Charlie", 35)
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
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

	tr.startServer(t, ctx, toolsFile)

	tcs := []struct {
		name           string
		toolName       string
		args           map[string]any
		resultSliceLen int
		isErr          bool
	}{
		{
			name:           "SimpleSelect",
			toolName:       "test-select",
			args:           map[string]any{},
			resultSliceLen: 3,
		},
		{
			name:           "ParameterizedQuery",
			toolName:       "test-param-query",
			args:           map[string]any{"min_age": 28},
			resultSliceLen: 2,
		},
		{
			name:           "EmptyResult",
			toolName:       "test-empty-result",
			args:           map[string]any{"id": 999}, // non-existent id
			resultSliceLen: 0,
		},
		{
			name:     "InvalidSQL",
			toolName: "test-invalid-sql",
			args:     map[string]any{},
			isErr:    true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			res := tr.invoke(t, ctx, tc.toolName, tc.args)
			if tc.isErr {
				if res.status != http.StatusOK || res.toolErr {
					return
				}
				t.Fatalf("expected an error, got: %s", res.result)
			}
			if res.status != http.StatusOK {
				t.Fatalf("response status code is not 200, got %d: %s", res.status, res.result)
			}
			if res.toolErr {
				t.Fatalf("unexpected error result: %s", res.result)
			}
			t.Logf("result is %s", res.result)

			var rows []any
			err := json.Unmarshal([]byte(res.result), &rows)
			if err != nil {
				t.Fatalf("error parsing result %q: %s", res.result, err)
			}

			if len(rows) != tc.resultSliceLen {
				t.Errorf("Expected %d results, got %d", tc.resultSliceLen, len(rows))
			}
		})
	}

	t.Logf("✅ clickhouse-sql tool tests completed successfully")
}

func TestClickHouseExecuteSQLTool(t *testing.T) {
	runClickHouseExecuteSQLToolTest(t, clickHouseTransport{})
}

// runClickHouseExecuteSQLToolTest exercises the clickhouse-execute-sql tool:
// DDL, DML and queries, plus rejected invocations.
func runClickHouseExecuteSQLToolTest(t *testing.T, tr clickHouseTransport) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	setupClickHouseInstance(ctx, t)
	sourceConfig := getClickHouseVars(t)
	pool := openClickHousePool(t)

	tableName := "test_exec_sql_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	// The table is created through the tool; drop it if a case fails midway.
	t.Cleanup(func() {
		_, _ = pool.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
	})

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{
			"execute-sql-tool": map[string]any{
				"type":        "clickhouse-execute-sql",
				"source":      "my-instance",
				"description": "Test create table",
			},
		},
	}

	tr.startServer(t, ctx, toolsFile)

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
			res := tr.invoke(t, ctx, "execute-sql-tool", map[string]any{"sql": tc.sql})
			if res.status != http.StatusOK {
				if tc.isErr {
					return
				}
				t.Fatalf("response status code is not 200, got %d: %s", res.status, res.result)
			}
			if tc.isErr {
				t.Fatalf("expecting an error from server")
			}
			if tc.isAgentErr {
				// Over MCP a rejected invocation must be reported as an error result.
				if tr.isMCP && !res.toolErr {
					t.Fatalf("expected an error result, got: %s", res.result)
				}
				return
			}
			if res.toolErr {
				t.Fatalf("unexpected error result: %s", res.result)
			}

			var rows []any
			err := json.Unmarshal([]byte(res.result), &rows)
			if err != nil {
				t.Fatalf("error parsing result %q: %s", res.result, err)
			}

			if len(rows) != tc.resultSliceLen {
				t.Errorf("Expected %d results, got %d", tc.resultSliceLen, len(rows))
			}
		})
	}

	t.Logf("✅ clickhouse-execute-sql tool tests completed successfully")
}

func TestClickHouseEdgeCases(t *testing.T) {
	runClickHouseEdgeCasesTest(t, clickHouseTransport{})
}

// runClickHouseEdgeCasesTest exercises long queries, null values and
// concurrent invocations.
func runClickHouseEdgeCasesTest(t *testing.T, tr clickHouseTransport) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	setupClickHouseInstance(ctx, t)
	sourceConfig := getClickHouseVars(t)
	pool := openClickHousePool(t)

	tableName := "test_nulls_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
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

	tr.startServer(t, ctx, toolsFile)

	t.Run("VeryLongQuery", func(t *testing.T) {
		// Create a very long but valid query
		var conditions []string
		for i := 1; i <= 100; i++ {
			conditions = append(conditions, fmt.Sprintf("(%d = %d)", i, i))
		}
		longQuery := "SELECT 1 WHERE " + strings.Join(conditions, " AND ")

		res := tr.invoke(t, ctx, "execute-sql-tool", map[string]any{"sql": longQuery})
		if res.status != http.StatusOK || res.toolErr {
			t.Fatalf("unexpected response, status %d: %s", res.status, res.result)
		}

		var rows []any
		err := json.Unmarshal([]byte(res.result), &rows)
		if err != nil {
			t.Fatalf("error parsing result %q: %s", res.result, err)
		}

		// Should return [{1:1}]
		if len(rows) != 1 {
			t.Errorf("Expected 1 result from long query, got %d", len(rows))
		}
	})

	t.Run("NullValues", func(t *testing.T) {
		t.Cleanup(func() {
			_, _ = pool.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
		})
		createSQL := fmt.Sprintf(`
			CREATE TABLE %s (
				id UInt32,
				nullable_field Nullable(String)
			) ENGINE = Memory
		`, tableName)

		_, err := pool.ExecContext(ctx, createSQL)
		if err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}

		// Insert null value
		insertSQL := fmt.Sprintf("INSERT INTO %s (id, nullable_field) VALUES (1, NULL), (2, 'not null')", tableName)
		_, err = pool.ExecContext(ctx, insertSQL)
		if err != nil {
			t.Fatalf("Failed to insert null value: %v", err)
		}

		res := tr.invoke(t, ctx, "test-null-values", map[string]any{})
		if res.status != http.StatusOK || res.toolErr {
			t.Fatalf("unexpected response, status %d: %s", res.status, res.result)
		}

		var rows []any
		err = json.Unmarshal([]byte(res.result), &rows)
		if err != nil {
			t.Fatalf("error parsing result %q: %s", res.result, err)
		}

		if len(rows) != 2 {
			t.Errorf("Expected 2 result from long query, got %d", len(rows))
		}

		// Check that null is properly handled
		if firstRow, ok := rows[0].(map[string]any); ok {
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

				res := tr.invoke(t, ctx, "test-concurrent", map[string]any{"limit": n + 1})
				if res.status != http.StatusOK || res.toolErr {
					t.Errorf("unexpected response, status %d: %s", res.status, res.result)
					return
				}

				var rows []any
				err := json.Unmarshal([]byte(res.result), &rows)
				if err != nil {
					t.Errorf("error parsing result %q: %s", res.result, err)
				}

				if len(rows) != n+1 {
					t.Errorf("Query %d: expected %d results, got %d", n, n+1, len(rows))
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

// setupClickHouseSQLTable creates and inserts data into a table of tool
// compatible with clickhouse-sql tool. The table is dropped on cleanup; the
// cleanup is registered first so a partially failed setup is still cleaned up.
func setupClickHouseSQLTable(t *testing.T, ctx context.Context, pool *sql.DB, createStatement, insertStatement, tableName string, params []any) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := pool.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)); err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	})

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
}

func TestClickHouseListDatabasesTool(t *testing.T) {
	runClickHouseListDatabasesToolTest(t, clickHouseTransport{})
}

// runClickHouseListDatabasesToolTest exercises the clickhouse-list-databases
// tool.
func runClickHouseListDatabasesToolTest(t *testing.T, tr clickHouseTransport) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	setupClickHouseInstance(ctx, t)
	sourceConfig := getClickHouseVars(t)
	pool := openClickHousePool(t)

	// Create a test database
	testDBName := "test_list_db_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	t.Cleanup(func() {
		_, _ = pool.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))
	})
	_, err := pool.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", testDBName))
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{
			"test-list-databases": map[string]any{
				"type":        "clickhouse-list-databases",
				"source":      "my-instance",
				"description": "Test listing databases",
			},
		},
	}

	tr.startServer(t, ctx, toolsFile)

	t.Run("ListDatabases", func(t *testing.T) {
		res := tr.invoke(t, ctx, "test-list-databases", map[string]any{})
		if res.status != http.StatusOK || res.toolErr {
			t.Fatalf("unexpected response, status %d: %s", res.status, res.result)
		}

		var databases []map[string]any
		err := json.Unmarshal([]byte(res.result), &databases)
		if err != nil {
			t.Errorf("error parsing result %q: %s", res.result, err)
		}

		// Should contain at least the default database and our test database - system and default
		if len(databases) < 2 {
			t.Errorf("Expected at least 2 databases, got %d", len(databases))
		}

		found := false
		foundDefault := false
		for _, db := range databases {
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
	runClickHouseListTablesToolTest(t, clickHouseTransport{})
}

// runClickHouseListTablesToolTest exercises the clickhouse-list-tables tool.
func runClickHouseListTablesToolTest(t *testing.T, tr clickHouseTransport) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	setupClickHouseInstance(ctx, t)
	sourceConfig := getClickHouseVars(t)
	pool := openClickHousePool(t)

	// Create a test database with tables
	testDBName := "test_list_tables_db_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	t.Cleanup(func() {
		_, _ = pool.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))
	})
	_, err := pool.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", testDBName))
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

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
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{
			"test-list-tables": map[string]any{
				"type":        "clickhouse-list-tables",
				"source":      "my-instance",
				"description": "Test listing tables",
			},
		},
	}

	tr.startServer(t, ctx, toolsFile)

	t.Run("ListTables", func(t *testing.T) {
		res := tr.invoke(t, ctx, "test-list-tables", map[string]any{"database": testDBName})
		if res.status != http.StatusOK || res.toolErr {
			t.Fatalf("unexpected response, status %d: %s", res.status, res.result)
		}

		var tables []map[string]any
		err := json.Unmarshal([]byte(res.result), &tables)
		if err != nil {
			t.Errorf("error parsing result %q: %s", res.result, err)
		}

		// Should contain exactly 2 tables that we created
		if len(tables) != 2 {
			t.Errorf("Expected 2 tables, got %d", len(tables))
		}

		foundTable1 := false
		foundTable2 := false
		for _, table := range tables {
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
		res := tr.invoke(t, ctx, "test-list-tables", map[string]any{})
		if res.status != http.StatusOK {
			t.Errorf("Expected 200 OK for missing database parameter, but got %d", res.status)
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
	// CI secret was dropped) — fail loudly rather than skip silently. This test
	// intentionally has no container fallback.
	sourceConfig := getClickHouseVars(t)
	if os.Getenv("API_KEY") == "" {
		t.Fatal("'API_KEY' not set; required for the embedding integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool := openClickHousePool(t)

	vectorTableName := setupClickHouseVectorTable(t, ctx, pool)

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{},
	}

	insertStmt, searchStmt := getClickHouseVectorSearchStmts(vectorTableName)
	toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, ClickHouseToolType, insertStmt, searchStmt)

	clickHouseTransport{}.startServer(t, ctx, toolsFile)

	tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox")
}

// setupClickHouseVectorTable creates a ClickHouse table with an Array(Float32)
// embedding column for the semantic search test and returns its name. The
// table is dropped on cleanup.
func setupClickHouseVectorTable(t *testing.T, ctx context.Context, pool *sql.DB) string {
	t.Helper()

	tableName := "vector_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Cleanup(func() {
		if _, err := pool.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)); err != nil {
			t.Errorf("failed to drop table %s: %v", tableName, err)
		}
	})

	createTableStmt := fmt.Sprintf(`CREATE TABLE %s (
		content String,
		embedding Array(Float32)
	) ENGINE = MergeTree ORDER BY tuple()`, tableName)

	if _, err := pool.ExecContext(ctx, createTableStmt); err != nil {
		t.Fatalf("failed to create table %s: %v", tableName, err)
	}

	return tableName
}

// getClickHouseVectorSearchStmts returns the insert and cosine-distance search
// statements used by the ClickHouse semantic-search integration test.
func getClickHouseVectorSearchStmts(vectorTableName string) (string, string) {
	insertStmt := fmt.Sprintf("INSERT INTO %s (content, embedding) VALUES (?, ?)", vectorTableName)
	searchStmt := fmt.Sprintf("SELECT content, cosineDistance(embedding, ?) AS distance FROM %s ORDER BY distance ASC LIMIT 1", vectorTableName)
	return insertStmt, searchStmt
}
