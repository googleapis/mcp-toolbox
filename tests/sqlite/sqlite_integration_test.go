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

	args := []string{"--enable-api"}

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
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: SQL logic error: near \"SELEC\": syntax error (1)"}],"isError":true}}`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"1\":1}"}]}}`

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)
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

	args := []string{"--enable-api"}
	cmd, cleanup, err := tests.StartCmd(ctx, toolConfig, args...)
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
		name       string
		sql        string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "select existing row",
			sql:        fmt.Sprintf("SELECT name FROM %s WHERE id = 1", tableName),
			wantStatus: 200,
			wantBody:   "Bob",
		},
		{
			name:       "select no rows",
			sql:        fmt.Sprintf("SELECT name FROM %s WHERE id = 999", tableName),
			wantStatus: 200,
			wantBody:   "[]",
		},
		{
			name:       "invalid SQL",
			sql:        "SELEC name FROM not_a_table",
			wantStatus: 200,
			wantBody:   "error processing request: unable to execute query: SQL logic error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			api := "http://127.0.0.1:5000/api/tool/my-exec-sql-tool/invoke"
			reqBody := strings.NewReader(fmt.Sprintf(`{"sql":"%s"}`, tc.sql))
			req, err := http.NewRequest("POST", api, reqBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("unable to read response: %s", err)
			}
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("unexpected status: %d, body: %s", resp.StatusCode, string(bodyBytes))
			}
			if tc.wantBody != "" && !strings.Contains(string(bodyBytes), tc.wantBody) {
				t.Fatalf("expected body to contain %q, got: %s", tc.wantBody, string(bodyBytes))
			}
		})
	}
}

// TestSQLiteResponseEncodingGCF is an end-to-end test of the --response-encoding=gcf
// flag: it boots the toolbox server against a real SQLite source, invokes a query
// tool over the MCP JSON-RPC transport, and asserts the result set comes back as a
// single Graph Compact Format block instead of per-row JSON.
func TestSQLiteResponseEncodingGCF(t *testing.T) {
	db, teardownDb, sqliteDb, err := initSQLiteDb(t, SQLiteDatabase)
	if err != nil {
		t.Fatal(err)
	}
	defer teardownDb(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Enough uniform rows/columns that GCF's header-factoring beats the per-row JSON
	// (clearing the never-grow guard).
	tableName := "gcf_e2e_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createStmt := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id INTEGER PRIMARY KEY, name TEXT, region TEXT, status TEXT);", tableName)
	if _, err := db.ExecContext(ctx, createStmt); err != nil {
		t.Fatalf("create table: %s", err)
	}
	insertStmt := fmt.Sprintf("INSERT INTO %s (name, region, status) VALUES (?, ?, ?);", tableName)
	for i := 0; i < 15; i++ {
		if _, err := db.ExecContext(ctx, insertStmt, fmt.Sprintf("instance-%02d", i), "us-central1", "RUNNABLE"); err != nil {
			t.Fatalf("insert: %s", err)
		}
	}

	sourceConfig := getSQLiteVars(t)
	sourceConfig["database"] = sqliteDb
	toolsFile := map[string]any{
		"sources": map[string]any{"my-instance": sourceConfig},
		"tools": map[string]any{
			"list-rows": map[string]any{
				"type":        SQLiteToolType,
				"source":      "my-instance",
				"description": "List all rows.",
				"statement":   fmt.Sprintf("SELECT id, name, region, status FROM %s;", tableName),
			},
		},
	}

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, "--response-encoding=gcf")
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	if out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out); err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	_, resp, err := tests.InvokeMCPTool(t, "list-rows", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("InvokeMCPTool: %s", err)
	}
	if len(resp.Result.Content) != 1 {
		t.Fatalf("expected 1 content block with --response-encoding=gcf, got %d", len(resp.Result.Content))
	}
	text := resp.Result.Content[0].Text
	if !strings.HasPrefix(text, "GCF profile=generic") {
		t.Fatalf("expected a GCF block, got: %q", text)
	}
	if !strings.Contains(text, "{id,name,region,status}") {
		t.Errorf("expected the field names factored into one header, got: %q", text)
	}
	if !strings.Contains(text, "instance-00") {
		t.Errorf("expected row data in the GCF block, got: %q", text)
	}
}
