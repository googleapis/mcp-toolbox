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
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
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
			name:       "preserve large integer in JSON",
			sql:        "SELECT json_object('id', 9007199254740993) AS payload",
			wantStatus: 200,
			wantBody:   `\"payload\":{\"id\":9007199254740993}`,
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

type sqliteListTablesRow struct {
	ObjectDetails map[string]any `json:"object_details"`
}

func invokeSQLiteToolJSON(t *testing.T, toolName string, payload map[string]any) string {
	t.Helper()
	reqBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("unable to marshal request payload: %v", err)
	}
	api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", toolName)
	req, err := http.NewRequest(http.MethodPost, api, bytes.NewReader(reqBytes))
	if err != nil {
		t.Fatalf("unable to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unable to send request: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("unable to read response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var wrapper struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(bodyBytes, &wrapper); err != nil {
		t.Fatalf("unable to decode response wrapper (%s): %v", string(bodyBytes), err)
	}
	var resultStr string
	if err := json.Unmarshal(wrapper.Result, &resultStr); err != nil {
		resultStr = string(wrapper.Result)
	}
	return resultStr
}

func invokePrebuiltSQLiteListTables(t *testing.T, payload map[string]any) []sqliteListTablesRow {
	t.Helper()
	resultStr := invokeSQLiteToolJSON(t, "list_tables", payload)
	var rows []sqliteListTablesRow
	if err := json.Unmarshal([]byte(resultStr), &rows); err != nil {
		t.Fatalf("unable to unmarshal list_tables rows (%s): %v", resultStr, err)
	}
	return rows
}

func assertPrebuiltSQLiteDatabaseIntact(t *testing.T, ctx context.Context, dbPath string) {
	t.Helper()

	dbListRaw := invokeSQLiteToolJSON(t, "execute_sql", map[string]any{
		"sql": "PRAGMA database_list",
	})
	var dbList []map[string]any
	if err := json.Unmarshal([]byte(dbListRaw), &dbList); err != nil {
		t.Fatalf("unable to unmarshal PRAGMA database_list (%s): %v", dbListRaw, err)
	}
	var dbNames []string
	for _, entry := range dbList {
		name, _ := entry["name"].(string)
		dbNames = append(dbNames, name)
	}
	if diff := cmp.Diff([]string{"main"}, dbNames); diff != "" {
		t.Fatalf("unexpected attached databases (-want +got):\n%s", diff)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening sqlite db for verification failed: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"users", "orders"} {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			t.Fatalf("expected table %q to remain intact, query failed: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected 1 row in %q, got %d", table, count)
		}
	}
}

func TestSQLitePrebuiltListTablesTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tempDir := t.TempDir()
	primaryPath := filepath.Join(tempDir, "primary.db")
	secondaryPath := filepath.Join(tempDir, "secondary.db")

	secDB, err := sql.Open("sqlite", secondaryPath)
	if err != nil {
		t.Fatalf("opening secondary sqlite db failed: %v", err)
	}
	if _, err := secDB.ExecContext(ctx, "CREATE TABLE archives (id INTEGER PRIMARY KEY, label TEXT);"); err != nil {
		_ = secDB.Close()
		t.Fatalf("creating secondary table failed: %v", err)
	}
	if err := secDB.Close(); err != nil {
		t.Fatalf("closing secondary sqlite db failed: %v", err)
	}

	setupDB, err := sql.Open("sqlite", primaryPath)
	if err != nil {
		t.Fatalf("opening primary sqlite db failed: %v", err)
	}
	setupStatements := []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, total REAL, FOREIGN KEY(user_id) REFERENCES users(id));`,
		`INSERT INTO users (id, name) VALUES (1, 'Alice');`,
		`INSERT INTO orders (id, user_id, total) VALUES (1, 1, 19.95);`,
	}
	for _, stmt := range setupStatements {
		if _, err := setupDB.ExecContext(ctx, stmt); err != nil {
			_ = setupDB.Close()
			t.Fatalf("executing setup SQL %q failed: %v", stmt, err)
		}
	}
	if err := setupDB.Close(); err != nil {
		t.Fatalf("closing setup sqlite db failed: %v", err)
	}

	t.Setenv("SQLITE_DATABASE", filepath.ToSlash(primaryPath))

	args := []string{"--prebuilt", "sqlite", "--enable-api"}
	cmd, cleanup, err := tests.StartCmd(ctx, map[string]any{}, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %v", err)
	}
	defer cleanup()

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs:\n%s", out)
		t.Fatalf("toolbox didn't start successfully: %v", err)
	}

	t.Run("default arguments return all tables in detailed format", func(t *testing.T) {
		rows := invokePrebuiltSQLiteListTables(t, map[string]any{})
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d: %+v", len(rows), rows)
		}
		var gotNames []string
		for _, r := range rows {
			name, _ := r.ObjectDetails["object_name"].(string)
			gotNames = append(gotNames, name)
			if r.ObjectDetails["schema_name"] != "main" {
				t.Errorf("expected schema_name 'main', got %v", r.ObjectDetails["schema_name"])
			}
			if r.ObjectDetails["object_type"] != "table" {
				t.Errorf("expected object_type 'table', got %v", r.ObjectDetails["object_type"])
			}
			cols, ok := r.ObjectDetails["columns"].([]any)
			if !ok || len(cols) == 0 {
				t.Errorf("expected non-empty columns slice for %q, got %T (%v)", name, r.ObjectDetails["columns"], r.ObjectDetails["columns"])
			}
		}
		slices.Sort(gotNames)
		if diff := cmp.Diff([]string{"orders", "users"}, gotNames); diff != "" {
			t.Fatalf("table names mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("simple output_format returns table names only", func(t *testing.T) {
		rows := invokePrebuiltSQLiteListTables(t, map[string]any{
			"output_format": "simple",
		})
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d: %+v", len(rows), rows)
		}
		var gotNames []string
		for _, r := range rows {
			if len(r.ObjectDetails) != 1 {
				t.Errorf("expected single 'name' key in simple output, got %v", r.ObjectDetails)
			}
			name, _ := r.ObjectDetails["name"].(string)
			gotNames = append(gotNames, name)
		}
		slices.Sort(gotNames)
		if diff := cmp.Diff([]string{"orders", "users"}, gotNames); diff != "" {
			t.Fatalf("table names mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("table_names filters matching tables", func(t *testing.T) {
		rows := invokePrebuiltSQLiteListTables(t, map[string]any{
			"table_names": "users",
		})
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d: %+v", len(rows), rows)
		}
		if rows[0].ObjectDetails["object_name"] != "users" {
			t.Fatalf("expected object_name 'users', got %v", rows[0].ObjectDetails["object_name"])
		}

		bothRows := invokePrebuiltSQLiteListTables(t, map[string]any{
			"output_format": "simple",
			"table_names":   "orders,users",
		})
		if len(bothRows) != 2 {
			t.Fatalf("expected 2 rows for comma-separated table_names, got %d", len(bothRows))
		}
	})

	t.Run("special characters and SQL statements in parameters are bound as literals", func(t *testing.T) {
		secondarySlashPath := filepath.ToSlash(secondaryPath)

		tableFilterInputs := []string{
			"', 'orders') > 0; DROP TABLE users;",
			"nonexistent', ',nonexistent,') > 0; DROP TABLE users; SELECT instr('a",
			fmt.Sprintf("', 'orders') > 0; ATTACH DATABASE '%s' AS extra_db;", secondarySlashPath),
			fmt.Sprintf("nonexistent', ',nonexistent,') > 0; ATTACH DATABASE '%s' AS extra_db; SELECT instr('a", secondarySlashPath),
		}
		for _, filterVal := range tableFilterInputs {
			rows := invokePrebuiltSQLiteListTables(t, map[string]any{
				"table_names": filterVal,
			})
			if len(rows) != 0 {
				t.Fatalf("expected 0 rows for literal table_names %q, got %d: %+v", filterVal, len(rows), rows)
			}
			assertPrebuiltSQLiteDatabaseIntact(t, ctx, primaryPath)
		}

		outputFormatInputs := []string{
			"simple' = 'simple' THEN json_object('name', 'other') WHEN 'a",
			"detailed'; DROP TABLE users;",
			fmt.Sprintf("simple'; ATTACH DATABASE '%s' AS extra_db;", secondarySlashPath),
		}
		for _, formatVal := range outputFormatInputs {
			rows := invokePrebuiltSQLiteListTables(t, map[string]any{
				"output_format": formatVal,
				"table_names":   "users",
			})
			if len(rows) != 1 {
				t.Fatalf("expected 1 detailed row for literal output_format %q, got %d", formatVal, len(rows))
			}
			if rows[0].ObjectDetails["object_name"] != "users" {
				t.Fatalf("expected object_name 'users' for literal output_format %q, got %v", formatVal, rows[0].ObjectDetails)
			}
			assertPrebuiltSQLiteDatabaseIntact(t, ctx, primaryPath)
		}
	})
}
