// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mariadb

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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	MariaDBSourceType = "mysql"
	MariaDBToolType   = "mysql-sql"
	MariaDBDatabase   = os.Getenv("MARIADB_DATABASE")
	MariaDBHost       = os.Getenv("MARIADB_HOST")
	MariaDBPort       = os.Getenv("MARIADB_PORT")
	MariaDBUser       = os.Getenv("MARIADB_USER")
	MariaDBPass       = os.Getenv("MARIADB_PASS")
)

func getMariaDBVars(t *testing.T) map[string]any {
	switch "" {
	case MariaDBDatabase:
		t.Fatal("'MARIADB_DATABASE' not set")
	case MariaDBHost:
		t.Fatal("'MARIADB_HOST' not set")
	case MariaDBPort:
		t.Fatal("'MARIADB_PORT' not set")
	case MariaDBUser:
		t.Fatal("'MARIADB_USER' not set")
	case MariaDBPass:
		t.Fatal("'MARIADB_PASS' not set")
	}

	return map[string]any{
		"type":     MariaDBSourceType,
		"host":     MariaDBHost,
		"port":     MariaDBPort,
		"database": MariaDBDatabase,
		"user":     MariaDBUser,
		"password": MariaDBPass,
	}
}

// Copied over from mysql.go
func initMariaDB(host, port, user, pass, dbname string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", user, pass, host, port, dbname)

	// Interact with the driver directly as you normally would
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return pool, nil
}

type mariaDBTestFixture struct {
	ctx                                  context.Context
	pool                                 *sql.DB
	paramTable, authTable, templateTable string
}

func TestMySQLToolEndpoints(t *testing.T) {
	fixture := setupMariaDBTest(t, "--enable-api")
	tests.RunToolGetTest(t)

	// Get configs for tests
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := GetMariaDBWants()

	t.Run("invoke", func(t *testing.T) {
		tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest())
	})
	t.Run("mcp_call", func(t *testing.T) {
		tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	})
	t.Run("execute_sql", func(t *testing.T) {
		tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	})
	t.Run("template_parameters", func(t *testing.T) {
		tests.RunToolInvokeWithTemplateParameters(t, fixture.templateTable)
	})
	t.Run("list_tables", func(t *testing.T) {
		RunMariDBListTablesTest(t, MariaDBDatabase, fixture.paramTable, fixture.authTable)
	})
	t.Run("list_active_queries", func(t *testing.T) {
		tests.RunMySQLListActiveQueriesTest(t, fixture.ctx, fixture.pool)
	})
	t.Run("list_tables_missing_unique_indexes", func(t *testing.T) {
		tests.RunMySQLListTablesMissingUniqueIndexes(t, fixture.ctx, fixture.pool, MariaDBDatabase)
	})
	t.Run("list_table_fragmentation", func(t *testing.T) {
		tests.RunMySQLListTableFragmentationTest(t, MariaDBDatabase, fixture.paramTable, fixture.authTable)
	})
}

func setupMariaDBTest(t *testing.T, args ...string) mariaDBTestFixture {
	t.Helper()

	sourceConfig := getMariaDBVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)

	pool, err := initMariaDB(MariaDBHost, MariaDBPort, MariaDBUser, MariaDBPass, MariaDBDatabase)
	if err != nil {
		t.Fatalf("unable to create MySQL connection pool: %s", err)
	}

	t.Cleanup(func() { pool.Close() })

	// cleanup test environment
	tests.CleanupMySQLTables(t, ctx, pool)

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := tests.GetMySQLParamToolInfo(tableNameParam)
	teardownTable1 := tests.SetupMySQLTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	t.Cleanup(func() { teardownTable1(t) })

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := tests.GetMySQLAuthToolInfo(tableNameAuth)
	teardownTable2 := tests.SetupMySQLTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	t.Cleanup(func() { teardownTable2(t) })

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, MariaDBToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddMySqlExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMySQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, MariaDBToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

	toolsFile = tests.AddMySQLPrebuiltToolConfig(t, toolsFile)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(func() {
		cmd.Stop()
		waitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := cmd.Wait(waitCtx); err != nil {
			t.Errorf("toolbox shutdown: %v", err)
		}
		cmd.Close()
		cleanup()
	})

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	return mariaDBTestFixture{ctx, pool, tableNameParam, tableNameAuth, tableNameTemplateParam}
}

// RunMariDBListTablesTest run tests against the mysql-list-tables tool
func RunMariDBListTablesTest(t *testing.T, databaseName, tableNameParam, tableNameAuth string, opts ...tests.ToolExecOption) {
	config := &tests.ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}
	type tableInfo struct {
		ObjectName    string `json:"object_name"`
		SchemaName    string `json:"schema_name"`
		ObjectDetails string `json:"object_details"`
	}

	type column struct {
		DataType        string `json:"data_type"`
		ColumnName      string `json:"column_name"`
		ColumnComment   string `json:"column_comment"`
		ColumnDefault   any    `json:"column_default"`
		IsNotNullable   bool   `json:"is_not_nullable"`
		OrdinalPosition int    `json:"ordinal_position"`
	}

	type objectDetails struct {
		Owner       any      `json:"owner"`
		Columns     []column `json:"columns"`
		Comment     string   `json:"comment"`
		Indexes     []any    `json:"indexes"`
		Triggers    []any    `json:"triggers"`
		Constraints []any    `json:"constraints"`
		ObjectName  string   `json:"object_name"`
		ObjectType  string   `json:"object_type"`
		SchemaName  string   `json:"schema_name"`
	}

	paramTableWant := objectDetails{
		ObjectName: tableNameParam,
		SchemaName: databaseName,
		ObjectType: "TABLE",
		Columns: []column{
			{DataType: "int(11)", ColumnName: "id", IsNotNullable: true, OrdinalPosition: 1, ColumnDefault: nil},
			{DataType: "varchar(255)", ColumnName: "name", OrdinalPosition: 2, ColumnDefault: "NULL"},
		},
		Indexes:     []any{map[string]any{"index_columns": []any{"id"}, "index_name": "PRIMARY", "is_primary": true, "is_unique": true}},
		Triggers:    []any{},
		Constraints: []any{map[string]any{"constraint_columns": []any{"id"}, "constraint_name": "PRIMARY", "constraint_type": "PRIMARY KEY", "foreign_key_referenced_columns": any(nil), "foreign_key_referenced_table": any(nil), "constraint_definition": ""}},
	}

	authTableWant := objectDetails{
		ObjectName: tableNameAuth,
		SchemaName: databaseName,
		ObjectType: "TABLE",
		Columns: []column{
			{DataType: "int(11)", ColumnName: "id", IsNotNullable: true, OrdinalPosition: 1, ColumnDefault: nil},
			{DataType: "varchar(255)", ColumnName: "name", OrdinalPosition: 2, ColumnDefault: "NULL"},
			{DataType: "varchar(255)", ColumnName: "email", OrdinalPosition: 3, ColumnDefault: "NULL"},
		},
		Indexes:     []any{map[string]any{"index_columns": []any{"id"}, "index_name": "PRIMARY", "is_primary": true, "is_unique": true}},
		Triggers:    []any{},
		Constraints: []any{map[string]any{"constraint_columns": []any{"id"}, "constraint_name": "PRIMARY", "constraint_type": "PRIMARY KEY", "foreign_key_referenced_columns": any(nil), "foreign_key_referenced_table": any(nil), "constraint_definition": ""}},
	}

	invokeTcs := []struct {
		name           string
		arguments      map[string]any
		wantStatusCode int
		want           any
		isSimple       bool
		isAllTables    bool
	}{
		{
			name:           "invoke list_tables for all tables detailed output",
			arguments:      map[string]any{"table_names": ""},
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{authTableWant, paramTableWant},
			isAllTables:    true,
		},
		{
			name:           "invoke list_tables detailed output",
			arguments:      map[string]any{"table_names": tableNameAuth},
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{authTableWant},
		},
		{
			name:           "invoke list_tables simple output",
			arguments:      map[string]any{"table_names": tableNameAuth, "output_format": "simple"},
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{{"name": tableNameAuth}},
			isSimple:       true,
		},
		{
			name:           "invoke list_tables with multiple table names",
			arguments:      map[string]any{"table_names": tableNameParam + "," + tableNameAuth},
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{authTableWant, paramTableWant},
		},
		{
			name:           "invoke list_tables with one existing and one non-existent table",
			arguments:      map[string]any{"table_names": tableNameAuth + ",non_existent_table"},
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{authTableWant},
		},
		{
			name:           "invoke list_tables with non-existent table",
			arguments:      map[string]any{"table_names": "non_existent_table"},
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{},
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var tables []tableInfo
			if config.IsMCP() {
				status, response, err := tests.InvokeMCPTool(t, "list_tables", tc.arguments, nil)
				if err != nil {
					t.Fatalf("list_tables request failed: %v", err)
				}
				if status != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d", status, tc.wantStatusCode)
				}
				if response.Error != nil || response.Result.IsError {
					t.Fatalf("list_tables returned an error: %+v", response)
				}
				if response.Result.Content == nil {
					t.Fatal("list_tables response is missing the content array")
				}
				tables = make([]tableInfo, 0, len(response.Result.Content))
				for _, content := range response.Result.Content {
					if content.Type != "text" {
						t.Fatalf("unexpected content type: %q", content.Type)
					}
					var table tableInfo
					if err := json.Unmarshal([]byte(content.Text), &table); err != nil {
						t.Fatalf("failed to decode table content: %v", err)
					}
					tables = append(tables, table)
				}
			} else {
				args, err := json.Marshal(tc.arguments)
				if err != nil {
					t.Fatal(err)
				}
				response, body := tests.RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/api/tool/list_tables/invoke", bytes.NewReader(args), nil)
				if response.StatusCode != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d; body: %s", response.StatusCode, tc.wantStatusCode, body)
				}
				var wrapper struct {
					Result json.RawMessage `json:"result"`
				}
				if err := json.Unmarshal(body, &wrapper); err != nil {
					t.Fatal(err)
				}
				var result string
				if err := json.Unmarshal(wrapper.Result, &result); err != nil {
					result = string(wrapper.Result)
				}
				if err := json.Unmarshal([]byte(result), &tables); err != nil {
					t.Fatalf("failed to decode tables: %v", err)
				}
			}

			var got any
			if tc.isSimple {
				details := []map[string]any{}
				for _, table := range tables {
					var d map[string]any
					if err := json.Unmarshal([]byte(table.ObjectDetails), &d); err != nil {
						t.Fatalf("failed to unmarshal nested ObjectDetails string: %v", err)
					}
					details = append(details, d)
				}
				got = details
			} else {
				details := []objectDetails{}
				for _, table := range tables {
					var d objectDetails
					if err := json.Unmarshal([]byte(table.ObjectDetails), &d); err != nil {
						t.Fatalf("failed to unmarshal nested ObjectDetails string: %v", err)
					}
					details = append(details, d)
				}
				got = details
			}

			opts := []cmp.Option{
				cmpopts.SortSlices(func(a, b objectDetails) bool { return a.ObjectName < b.ObjectName }),
				cmpopts.SortSlices(func(a, b column) bool { return a.ColumnName < b.ColumnName }),
				cmpopts.SortSlices(func(a, b map[string]any) bool { return a["name"].(string) < b["name"].(string) }),
			}

			// Checking only the current database where the test tables are created to avoid brittle tests.
			if tc.isAllTables {
				filteredGot := []objectDetails{}
				if got != nil {
					for _, item := range got.([]objectDetails) {
						if item.SchemaName == databaseName {
							filteredGot = append(filteredGot, item)
						}
					}
				}
				got = filteredGot
			}

			if diff := cmp.Diff(tc.want, got, opts...); diff != "" {
				t.Errorf("Unexpected result: got %#v, want: %#v", got, tc.want)
			}
		})
	}
}

// GetMariaDBWants return the expected wants for mariaDB
func GetMariaDBWants() (string, string, string, string) {
	select1Want := `[{"1":1}]`
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: Error 1064 (42000): You have an error in your SQL syntax; check the manual that corresponds to your MariaDB server version for the right syntax to use near 'SELEC 1' at line 1"}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id INT AUTO_INCREMENT PRIMARY KEY, name TEXT)"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"1\":1}"}]}}`
	return select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want
}
