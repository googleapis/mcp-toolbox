// Copyright © 2025, Oracle and/or its affiliates.

package oracle

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	OracleSourceType = "oracle"
	OracleToolType   = "oracle-sql"
	OracleHost       = os.Getenv("ORACLE_HOST")
	OracleUser       = os.Getenv("ORACLE_USERNAME")
	OraclePass       = os.Getenv("ORACLE_PASSWORD")
	OracleServerName = os.Getenv("ORACLE_SERVER_NAME")
	OracleConnStr    = fmt.Sprintf(
		"%s:%s/%s", OracleHost, "1521", OracleServerName)
)

func getOracleVars(t *testing.T) map[string]any {
	switch "" {
	case OracleHost:
		t.Fatal("'ORACLE_HOST not set")
	case OracleUser:
		t.Fatal("'ORACLE_USERNAME' not set")
	case OraclePass:
		t.Fatal("'ORACLE_PASSWORD' not set")
	case OracleServerName:
		t.Fatal("'ORACLE_SERVER_NAME' not set")
	}

	return map[string]any{
		"type":             OracleSourceType,
		"connectionString": OracleConnStr,
		"useOCI":           true,
		"user":             OracleUser,
		"password":         OraclePass,
	}
}

// Copied over from oracle.go
func initOracleConnection(ctx context.Context, user, pass, connStr string) (*sql.DB, error) {
	// Build the full Oracle connection string for godror driver
	fullConnStr := fmt.Sprintf(`user="%s" password="%s" connectString="%s"`,
		user, pass, connStr)

	db, err := sql.Open("godror", fullConnStr)
	if err != nil {
		return nil, fmt.Errorf("unable to open Oracle connection: %w", err)
	}

	err = db.PingContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to ping Oracle connection: %w", err)
	}

	return db, nil
}

func TestOracleSimpleToolEndpoints(t *testing.T) {
	sourceConfig := getOracleVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	db, err := initOracleConnection(ctx, OracleUser, OraclePass, OracleConnStr)
	if err != nil {
		t.Fatalf("unable to create Oracle connection pool: %s", err)
	}

	dropAllUserTables(t, ctx, db)

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getOracleParamToolInfo(tableNameParam)
	teardownTable1 := setupOracleTable(t, ctx, db, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getOracleAuthToolInfo(tableNameAuth)
	teardownTable2 := setupOracleTable(t, ctx, db, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, OracleToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddExecuteSqlConfig(t, toolsFile, "oracle-execute-sql")
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMySQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, OracleToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

	// Configure a DML tool to verify the 'readonly: false' logic.
	updateStmt := fmt.Sprintf(`UPDATE %s SET "name" = :1 WHERE "id" = :2`, tableNameParam)

	toolsMap, ok := toolsFile["tools"].(map[string]any)
	if !ok {
		t.Fatal("Configuration error: 'tools' key is missing or is not a map")
	}

	toolsMap["my-update-tool"] = map[string]any{
		"type":        "oracle-sql",
		"source":      "my-instance",
		"statement":   updateStmt,
		"readOnly":    false,
		"description": "Update user name by ID.",
		"parameters": []map[string]any{
			{
				"name":        "name",
				"type":        "string",
				"description": "The new name for the user.",
			},
			{
				"name":        "id",
				"type":        "integer",
				"description": "The user ID to update.",
			},
		},
	}

	// Configure read-only tools to verify that Oracle, not the tool, rejects
	// statements with side effects.
	toolsMap["my-readonly-locking-tool"] = map[string]any{
		"type":        "oracle-sql",
		"source":      "my-instance",
		"statement":   fmt.Sprintf(`SELECT * FROM %s FOR UPDATE`, tableNameParam),
		"readOnly":    true,
		"description": "Verify that read-only Oracle tools reject locking queries.",
	}

	toolsMap["my-readonly-update-tool"] = map[string]any{
		"type":        "oracle-sql",
		"source":      "my-instance",
		"statement":   fmt.Sprintf(`UPDATE %s SET "name" = 'Mallory' WHERE "id" = 1`, tableNameParam),
		"readOnly":    true,
		"description": "Verify that read-only Oracle tools reject writes.",
	}

	// `oracle-execute-sql` takes its statement at call time, so `readOnly: true`
	// has to be enforced on whatever the caller supplies rather than on a
	// statement fixed in the config.
	toolsMap["my-readonly-exec-sql-tool"] = map[string]any{
		"type":        "oracle-execute-sql",
		"source":      "my-instance",
		"readOnly":    true,
		"description": "Verify that read-only oracle-execute-sql tools reject writes.",
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

	// Get configs for tests
	select1Want := "[{\"1\":1}]"
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: dpiStmt_execute: ORA-00900: invalid SQL statement\nHelp: https://docs.oracle.com/error-help/db/ora-00900/"}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id NUMBER GENERATED AS IDENTITY PRIMARY KEY, name VARCHAR2(255))"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"1\":1}"}]}}`

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want,
		tests.DisableOptionalNullParamTest(),
		tests.WithMyToolById4Want("[{\"id\":4,\"name\":\"\"}]"),
		tests.DisableArrayTest(),
	)
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)

	// Invoke the 'my-update-tool' and verify the result.
	testToolResponseContains(t, "my-update-tool",
		`{"name": "UpdatedAlice", "id": 1}`,
		`\"rows_affected\":1`)

	// Oracle itself must reject locking reads and writes inside the read-only
	// transaction (ORA-01456: may not perform insert/delete/update operation
	// inside a READ ONLY transaction).
	testToolResponseContains(t, "my-readonly-locking-tool", `{}`, "ORA-01456")
	testToolResponseContains(t, "my-readonly-update-tool", `{}`, "ORA-01456")

	// The same contract applies to the statement `oracle-execute-sql` is handed.
	testToolResponseContains(t, "my-readonly-exec-sql-tool",
		fmt.Sprintf(`{"sql": "UPDATE %s SET \"name\" = 'Mallory' WHERE \"id\" = 1"}`, tableNameParam),
		"ORA-01456")
	testToolResponseContains(t, "my-readonly-exec-sql-tool",
		fmt.Sprintf(`{"sql": "SELECT \"name\" FROM %s WHERE \"id\" = 1 FOR UPDATE"}`, tableNameParam),
		"ORA-01456")

	// The rows must be untouched and unlocked afterwards.
	testToolResponseContains(t, "my-tool", `{"id": 1, "name": "Alice"}`, `\"name\":\"UpdatedAlice\"`)
	assertRowsNotLocked(t, ctx, db, tableNameParam)

	// `oracle-execute-sql` has no readOnly setting here, so it may still take
	// locks - but it must not hand the connection back to the pool while it
	// holds them.
	testToolResponseContains(t, "my-exec-sql-tool",
		fmt.Sprintf(`{"sql": "SELECT \"name\" FROM %s WHERE \"id\" = 1 FOR UPDATE"}`, tableNameParam),
		"UpdatedAlice")
	assertRowsNotLocked(t, ctx, db, tableNameParam)
}

// assertRowsNotLocked fails if another session still holds row locks on
// tableName, which is what happens when a tool leaves an open transaction on
// the connection it returns to the pool.
func assertRowsNotLocked(t *testing.T, ctx context.Context, db *sql.DB, tableName string) {
	t.Helper()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("unable to begin verification transaction: %s", err)
	}
	defer func() { _ = tx.Rollback() }()

	// NOWAIT fails immediately with ORA-00054 instead of blocking if the rows
	// are locked by someone else.
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT "id" FROM %s FOR UPDATE NOWAIT`, tableName))
	if err != nil {
		t.Fatalf("rows of %s are still locked after the tool call: %s", tableName, err)
	}
	defer rows.Close()
}

func setupOracleTable(t *testing.T, ctx context.Context, pool *sql.DB, createStatement, insertStatement, tableName string, params []any) func(*testing.T) {
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

func getOracleParamToolInfo(tableName string) (string, string, string, string, string, string, []any) {
	// Use GENERATED AS IDENTITY for auto-incrementing primary keys.
	// VARCHAR2 is the standard string type in Oracle.
	createStatement := fmt.Sprintf(`CREATE TABLE %s ("id" NUMBER GENERATED AS IDENTITY PRIMARY KEY, "name" VARCHAR2(255))`, tableName)

	// MODIFIED: Use a PL/SQL block for multiple inserts
	insertStatement := fmt.Sprintf(`
		BEGIN
			INSERT INTO %s ("name") VALUES (:1);
			INSERT INTO %s ("name") VALUES (:2);
			INSERT INTO %s ("name") VALUES (:3);
			INSERT INTO %s ("name") VALUES (:4);
		END;`, tableName, tableName, tableName, tableName)

	toolStatement := fmt.Sprintf(`SELECT * FROM %s WHERE "id" = :1 OR "name" = :2`, tableName)
	idParamStatement := fmt.Sprintf(`SELECT * FROM %s WHERE "id" = :1`, tableName)
	nameParamStatement := fmt.Sprintf(`SELECT * FROM %s WHERE "name" = :1`, tableName)

	// Oracle's equivalent for array parameters is using the 'MEMBER OF' operator
	// with a collection type defined in the database schema.
	arrayToolStatement := fmt.Sprintf(`SELECT * FROM %s WHERE "id" MEMBER OF :1 AND "name" MEMBER OF :2`, tableName)

	params := []any{"Alice", "Jane", "Sid", nil}

	return createStatement, insertStatement, toolStatement, idParamStatement, nameParamStatement, arrayToolStatement, params
}

// getOracleAuthToolInfo returns statements and params for my-auth-tool for Oracle SQL
func getOracleAuthToolInfo(tableName string) (string, string, string, []any) {
	createStatement := fmt.Sprintf(`CREATE TABLE %s ("id" NUMBER GENERATED AS IDENTITY PRIMARY KEY, "name" VARCHAR2(255), "email" VARCHAR2(255))`, tableName)

	// MODIFIED: Use a PL/SQL block for multiple inserts
	insertStatement := fmt.Sprintf(`
		BEGIN
			INSERT INTO %s ("name", "email") VALUES (:1, :2);
			INSERT INTO %s ("name", "email") VALUES (:3, :4);
		END;`, tableName, tableName)

	toolStatement := fmt.Sprintf(`SELECT "name" FROM %s WHERE "email" = :1`, tableName)

	params := []any{"Alice", tests.ServiceAccountEmail, "Jane", "janedoe@gmail.com"}

	return createStatement, insertStatement, toolStatement, params
}

// dropAllUserTables finds and drops all tables owned by the current user.
func dropAllUserTables(t *testing.T, ctx context.Context, db *sql.DB) {
	// Query for only the tables we know are created by this test suite.
	const query = `
		SELECT table_name FROM user_tables
		WHERE table_name LIKE 'param_table_%'
		   OR table_name LIKE 'auth_table_%'
		   OR table_name LIKE 'template_param_table_%'`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		t.Fatalf("failed to query for user tables: %v", err)
	}
	defer rows.Close()

	var tablesToDrop []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Fatalf("failed to scan table name: %v", err)
		}
		tablesToDrop = append(tablesToDrop, tableName)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("error iterating over tables: %v", err)
	}

	for _, tableName := range tablesToDrop {
		_, err := db.ExecContext(ctx, fmt.Sprintf("DROP TABLE %s CASCADE CONSTRAINTS", tableName))
		if err != nil {
			t.Logf("failed to drop table %s: %v", tableName, err)
		}
	}
}

// testToolResponseContains sends a JSON-RPC request to the running test server
// and asserts that the response body contains the expected substring.
func testToolResponseContains(t *testing.T, toolName, paramsJSON, wantResponseSubStr string) {
	t.Helper()

	// The test server runs on port 5000.
	url := "http://localhost:5000/mcp"

	// Construct the JSON-RPC request body
	reqBody := fmt.Sprintf(`{
		"jsonrpc": "2.0", 
		"method": "tools/call", 
		"params": {
			"name": "%s", 
			"arguments": %s
		}, 
		"id": 1
	}`, toolName, paramsJSON)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(reqBody)))
	if err != nil {
		t.Fatalf("Failed to create HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request to %s: %v", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Verify that the response contains the expected success message (e.g., "rows_affected")
	if !strings.Contains(string(body), wantResponseSubStr) {
		t.Errorf("Tool invocation '%s' failed.\nGot response: %s\nExpected substring: %s",
			toolName, string(body), wantResponseSubStr)
	}
}
