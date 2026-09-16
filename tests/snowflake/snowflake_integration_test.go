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

package snowflake

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/jmoiron/sqlx"
	_ "github.com/snowflakedb/gosnowflake/v2"
)

var (
	SnowflakeSourceType = "snowflake"
	SnowflakeToolType   = "snowflake-sql"
	SnowflakeAccount    = os.Getenv("SNOWFLAKE_ACCOUNT")
	SnowflakeUser       = os.Getenv("SNOWFLAKE_USER")
	SnowflakePassword   = os.Getenv("SNOWFLAKE_PASS")
	SnowflakeDatabase   = os.Getenv("SNOWFLAKE_DATABASE")
	SnowflakeSchema     = os.Getenv("SNOWFLAKE_SCHEMA")
	SnowflakeWarehouse  = os.Getenv("SNOWFLAKE_WAREHOUSE")
	SnowflakeRole       = os.Getenv("SNOWFLAKE_ROLE")
)

func getSnowflakeVars(t *testing.T) map[string]any {
	switch "" {
	case SnowflakeAccount:
		t.Fatal("'SNOWFLAKE_ACCOUNT' not set")
	case SnowflakeUser:
		t.Fatal("'SNOWFLAKE_USER' not set")
	case SnowflakePassword:
		t.Fatal("'SNOWFLAKE_PASSWORD' not set")
	case SnowflakeDatabase:
		t.Fatal("'SNOWFLAKE_DATABASE' not set")
	case SnowflakeSchema:
		t.Fatal("'SNOWFLAKE_SCHEMA' not set")
	}

	// Set defaults for optional parameters
	if SnowflakeWarehouse == "" {
		SnowflakeWarehouse = "COMPUTE_WH"
	}
	if SnowflakeRole == "" {
		SnowflakeRole = "ACCOUNTADMIN"
	}

	return map[string]any{
		"type":      SnowflakeSourceType,
		"account":   SnowflakeAccount,
		"user":      SnowflakeUser,
		"password":  SnowflakePassword,
		"database":  SnowflakeDatabase,
		"schema":    SnowflakeSchema,
		"warehouse": SnowflakeWarehouse,
		"role":      SnowflakeRole,
	}
}

// Copied over from snowflake.go
func initSnowflakeConnectionPool(ctx context.Context, account, user, password, database, schema, warehouse, role string) (*sqlx.DB, error) {
	// Set defaults for optional parameters
	if warehouse == "" {
		warehouse = "COMPUTE_WH"
	}
	if role == "" {
		role = "ACCOUNTADMIN"
	}

	// Snowflake DSN format: user:password@account/database/schema?warehouse=warehouse&role=role
	dsn := fmt.Sprintf("%s:%s@%s/%s/%s?warehouse=%s&role=%s", user, password, account, database, schema, warehouse, role)
	db, err := sqlx.ConnectContext(ctx, "snowflake", dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection: %w", err)
	}

	return db, nil
}

func TestSnowflake(t *testing.T) {
	sourceConfig := getSnowflakeVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	db, err := initSnowflakeConnectionPool(ctx, SnowflakeAccount, SnowflakeUser, SnowflakePassword, SnowflakeDatabase, SnowflakeSchema, SnowflakeWarehouse, SnowflakeRole)
	if err != nil {
		t.Fatalf("unable to create snowflake connection pool: %s", err)
	}

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getSnowflakeParamToolInfo(tableNameParam)
	teardownTable1 := setupSnowflakeTable(t, ctx, db, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getSnowflakeAuthToolInfo(tableNameAuth)
	teardownTable2 := setupSnowflakeTable(t, ctx, db, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)
	t.Logf("Test table setup complete.")

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, SnowflakeToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addSnowflakeExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := getSnowflakeTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, SnowflakeToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

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

	select1Want, failInvocationWant, createTableStatement, mcpSelect1Want := getSnowflakeWants()

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want,
		tests.DisableArrayTest(),
		tests.WithMyToolId3NameAliceWant(`[{"ID":"1","NAME":"Alice"},{"ID":"3","NAME":"Sid"}]`),
		tests.WithMyToolById4Want(`[{"ID":"4","NAME":null}]`),
	)
	tests.RunMCPToolCallMethod(t, failInvocationWant, mcpSelect1Want,
		tests.WithMcpMyToolId3NameAliceWant(`{"jsonrpc":"2.0","id":"my-tool","result":{"content":[{"type":"text","text":"{\"ID\":\"1\",\"NAME\":\"Alice\"}"},{"type":"text","text":"{\"ID\":\"3\",\"NAME\":\"Sid\"}"}]}}`),
		tests.WithMcpMySecureToolWant(`[{"ID":"1","NAME":"Alice"},{"ID":"3","NAME":"Sid"}]`),
	)

	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want,
		tests.WithExecuteCreateWant(`[{"status":"Table T successfully created."}]`),
		tests.WithExecuteDropWant(`[{"status":"T successfully dropped."}]`))
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)
}

// addSnowflakeExecuteSqlConfig gets the tools config for `snowflake-execute-sql`
func addSnowflakeExecuteSqlConfig(t *testing.T, config map[string]any) map[string]any {

	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}

	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "snowflake-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}

	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "snowflake-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	config["tools"] = tools
	return config
}

// getSnowflakeParamToolInfo returns statements and param for my-param-tool snowflake-sql type
func getSnowflakeParamToolInfo(tableName string) (string, string, string, string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INTEGER AUTOINCREMENT PRIMARY KEY, name STRING);", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name) VALUES (?), (?), (?), (?);", tableName)
	toolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ? OR name = ?;", tableName)
	idParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ?;", tableName)
	nameParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = ?;", tableName)
	arrayToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ANY(?) AND name = ANY(?);", tableName)
	params := []any{"Alice", "Jane", "Sid", nil}
	return createStatement, insertStatement, toolStatement, idParamStatement, nameParamStatement, arrayToolStatement, params
}

// getSnowflakeAuthToolInfo returns statements and param of my-auth-tool for snowflake-sql type
func getSnowflakeAuthToolInfo(tableName string) (string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INTEGER AUTOINCREMENT PRIMARY KEY, name STRING, email STRING);", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name, email) VALUES (?, ?), (?, ?)", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = ?;", tableName)
	params := []any{"Alice", tests.ServiceAccountEmail, "Jane", "janedoe@gmail.com"}
	return createStatement, insertStatement, toolStatement, params
}

// getSnowflakeTmplToolStatement returns statements and param for template parameter test cases for snowflake-sql type
func getSnowflakeTmplToolStatement() (string, string) {
	tmplSelectCombined := "SELECT * FROM {{.tableName}} WHERE id = ?"
	tmplSelectFilterCombined := "SELECT * FROM {{.tableName}} WHERE {{.columnFilter}} = ?"
	return tmplSelectCombined, tmplSelectFilterCombined
}

// getSnowflakeWants return the expected wants for snowflake
func getSnowflakeWants() (string, string, string, string) {
	select1Want := `[{"1":"1"}]`
	failInvocationWant := `unexpected 'SELEC'`
	createTableStatement := `"CREATE TABLE IF NOT EXISTS t (id INTEGER AUTOINCREMENT PRIMARY KEY, name STRING)"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"1\":\"1\"}"}]}}`
	return select1Want, failInvocationWant, createTableStatement, mcpSelect1Want
}

// setupSnowflakeTable creates and inserts data into a table of tool
// compatible with snowflake-sql tool
func setupSnowflakeTable(t *testing.T, ctx context.Context, db *sqlx.DB, createStatement, insertStatement, tableName string, params []any) func(*testing.T) {
	err := db.PingContext(ctx)
	if err != nil {
		t.Fatalf("unable to connect to test database: %s", err)
	}

	// Create table
	_, err = db.QueryxContext(ctx, createStatement)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	_, err = db.QueryxContext(ctx, insertStatement, params...)
	if err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}

	return func(t *testing.T) {
		// tear down test
		_, err = db.ExecContext(ctx, fmt.Sprintf("DROP TABLE %s;", tableName))
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}
}
