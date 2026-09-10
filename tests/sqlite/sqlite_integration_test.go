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

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	v20251125 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20251125"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	SQLiteSourceType = "sqlite"
	SQLiteToolType   = "sqlite-sql"
	SQLiteDatabase   = os.Getenv("SQLITE_DATABASE")
)

func getSQLiteVars(t *testing.T) map[string]any {
	return map[string]any{
		"type":     SQLiteSourceType,
		"database": SQLiteDatabase,
	}
}

func initSQLiteDb(t *testing.T, sqliteDb string) (*sql.DB, func(t *testing.T), string, error) {
	if sqliteDb == "" {
		// Create a temporary database file
		tmpFile, err := os.CreateTemp("", "test-*.db")
		if err != nil {
			return nil, nil, "", fmt.Errorf("failed to create temp file: %v", err)
		}
		sqliteDb = tmpFile.Name()
	}

	// Open database connection
	db, err := sql.Open("sqlite", sqliteDb)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to open database: %v", err)
	}

	cleanup := func(t *testing.T) {
		if err := os.Remove(sqliteDb); err != nil {
			t.Errorf("Failed to remove test database: %s", err)
		}
	}

	return db, cleanup, sqliteDb, nil
}

// setupSQLiteTestDB creates a temporary SQLite database for testing
func setupSQLiteTestDB(t *testing.T, ctx context.Context, db *sql.DB, createStatement string, insertStatement string, tableName string, params []any) {
	// Create test table
	_, err := db.ExecContext(ctx, createStatement)
	if err != nil {
		t.Fatalf("unable to connect to create test table %s: %s", tableName, err)
	}

	_, err = db.ExecContext(ctx, insertStatement, params...)
	if err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}
}

func getSQLiteParamToolInfo(tableName string) (string, string, string, string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id INTEGER PRIMARY KEY, name TEXT);", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name) VALUES (?), (?), (?), (?);", tableName)
	toolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ? OR name = ?;", tableName)
	idToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ?;", tableName)
	nameToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = ?;", tableName)
	arrayToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ANY({{.idArray}}) AND name = ANY({{.nameArray}});", tableName)
	params := []any{"Alice", "Jane", "Sid", nil}
	return createStatement, insertStatement, toolStatement, idToolStatement, nameToolStatement, arrayToolStatement, params
}

func getSQLiteAuthToolInfo(tableName string) (string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name, email) VALUES (?, ?), (?,?) RETURNING id, name, email;", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = ?;", tableName)
	params := []any{"Alice", tests.ServiceAccountEmail, "Jane", "janedoe@gmail.com"}
	return createStatement, insertStatement, toolStatement, params
}

func getSQLiteTmplToolStatement() (string, string) {
	tmplSelectCombined := "SELECT * FROM {{.tableName}} WHERE id = ?"
	tmplSelectFilterCombined := "SELECT * FROM {{.tableName}} WHERE {{.columnFilter}} = ?"
	return tmplSelectCombined, tmplSelectFilterCombined
}

func TestSQLiteToolEndpoint(t *testing.T) {
	db, teardownDb, sqliteDb, err := initSQLiteDb(t, SQLiteDatabase)
	if err != nil {
		t.Fatal(err)
	}
	defer teardownDb(t)
	defer db.Close()

	sourceConfig := getSQLiteVars(t)
	sourceConfig["database"] = sqliteDb
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getSQLiteParamToolInfo(tableNameParam)
	setupSQLiteTestDB(t, ctx, db, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getSQLiteAuthToolInfo(tableNameAuth)
	setupSQLiteTestDB(t, ctx, db, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, SQLiteToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	tmplSelectCombined, tmplSelectFilterCombined := getSQLiteTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, SQLiteToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
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
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: SQL logic error: near \"SELEC\": syntax error (1)"}],"isError":true}}`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"1\":1}"}]}}`

	// Keep credential-dependent helpers in separate subtests so that discovery
	// and template parameters can also be exercised without Google credentials.
	t.Run("list_tools", func(t *testing.T) {
		expectedTools := tests.GetBaseMCPExpectedTools()
		expectedTools = append(expectedTools, tests.GetTemplateParamMCPExpectedTools()...)
		tests.RunMCPToolsListMethod(t, expectedTools)
	})
	t.Run("invoke", func(t *testing.T) {
		tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest(), tests.WithMCP())
	})
	t.Run("mcp_call", func(t *testing.T) {
		tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	})
	t.Run("template_parameters", func(t *testing.T) {
		tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithMCPTemplate())
	})
}

func TestSQLiteExecuteSqlTool(t *testing.T) {
	db, teardownDb, sqliteDb, err := initSQLiteDb(t, SQLiteDatabase)
	if err != nil {
		t.Fatal(err)
	}
	defer teardownDb(t)
	defer db.Close()

	sourceConfig := getSQLiteVars(t)
	sourceConfig["database"] = sqliteDb
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create a table and insert data
	tableName := "exec_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createStmt := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id INTEGER PRIMARY KEY, name TEXT);", tableName)
	insertStmt := fmt.Sprintf("INSERT INTO %s (name) VALUES (?);", tableName)
	params := []any{"Bob"}
	setupSQLiteTestDB(t, ctx, db, createStmt, insertStmt, tableName, params)

	// Add sqlite-execute-sql tool config
	toolConfig := map[string]any{
		"tools": map[string]any{
			"my-exec-sql-tool": map[string]any{
				"type":        "sqlite-execute-sql",
				"source":      "my-instance",
				"description": "Tool to execute SQL statements",
			},
		},
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
	}

	cmd, cleanup, err := tests.StartCmd(ctx, toolConfig)
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

	// Table-driven test cases
	testCases := []struct {
		name        string
		arguments   map[string]any
		wantContent []v20251125.TextContent
		wantError   string
	}{
		{
			name:        "select existing row",
			arguments:   map[string]any{"sql": fmt.Sprintf("SELECT name FROM %s WHERE id = 1", tableName)},
			wantContent: []v20251125.TextContent{{Type: "text", Text: `{"name":"Bob"}`}},
		},
		{
			name:        "select no rows",
			arguments:   map[string]any{"sql": fmt.Sprintf("SELECT name FROM %s WHERE id = 999", tableName)},
			wantContent: []v20251125.TextContent{},
		},
		{
			name:      "invalid SQL",
			arguments: map[string]any{"sql": "SELEC name FROM not_a_table"},
			wantError: "error processing request: unable to execute query: SQL logic error",
		},
		{
			name:      "missing SQL",
			arguments: map[string]any{},
			wantError: `parameter "sql" is required`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			status, resp, err := tests.InvokeMCPTool(t, "my-exec-sql-tool", tc.arguments, nil)
			if err != nil {
				t.Fatalf("unable to invoke tool: %s", err)
			}
			if status != http.StatusOK {
				t.Fatalf("unexpected HTTP status: got %d, want %d", status, http.StatusOK)
			}
			if resp.Error != nil {
				t.Fatalf("unexpected JSON-RPC error: %+v", resp.Error)
			}
			if got, want := resp.Result.IsError, tc.wantError != ""; got != want {
				t.Fatalf("unexpected isError: got %t, want %t; content: %+v", got, want, resp.Result.Content)
			}
			if tc.wantError != "" {
				tests.AssertMCPError(t, resp, tc.wantError)
				return
			}
			if diff := cmp.Diff(tc.wantContent, resp.Result.Content); diff != "" {
				t.Fatalf("unexpected MCP content (-want +got):\n%s", diff)
			}
		})
	}
}
