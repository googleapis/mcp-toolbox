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

func TestMySQLToolEndpoints(t *testing.T) {
	sourceConfig := getMariaDBVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := initMariaDB(MariaDBHost, MariaDBPort, MariaDBUser, MariaDBPass, MariaDBDatabase)
	if err != nil {
		t.Fatalf("unable to create MySQL connection pool: %s", err)
	}

	defer pool.Close()

	// cleanup test environment
	tests.CleanupMySQLTables(t, ctx, pool)

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := tests.GetMySQLParamToolInfo(tableNameParam)
	teardownTable1 := tests.SetupMySQLTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := tests.GetMySQLAuthToolInfo(tableNameAuth)
	teardownTable2 := tests.SetupMySQLTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, MariaDBToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddMySqlExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMySQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, MariaDBToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

	toolsFile = tests.AddMySQLPrebuiltToolConfig(t, toolsFile)

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
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := GetMariaDBWants()

	// Keep credential-dependent helpers separate so local runs can select the
	// discovery, template and prebuilt cases without weakening auth coverage.
	t.Run("list_tools", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getMariaDBMCPExpectedTools())
	})
	t.Run("invoke", func(t *testing.T) {
		tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest(), tests.WithMCP())
	})
	t.Run("mcp_call", func(t *testing.T) {
		tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	})
	t.Run("execute_sql", func(t *testing.T) {
		tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, tests.WithMCPSql())
	})
	t.Run("template_parameters", func(t *testing.T) {
		tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithMCPTemplate())
	})
	t.Run("list_tables", func(t *testing.T) {
		RunMariDBListTablesTest(t, MariaDBDatabase, tableNameParam, tableNameAuth)
	})
	t.Run("list_active_queries", func(t *testing.T) {
		tests.RunMySQLListActiveQueriesTest(t, ctx, pool, tests.WithMCPExec())
	})
	t.Run("list_tables_missing_unique_indexes", func(t *testing.T) {
		tests.RunMySQLListTablesMissingUniqueIndexes(t, ctx, pool, MariaDBDatabase, tests.WithMCPExec())
	})
	t.Run("list_table_fragmentation", func(t *testing.T) {
		tests.RunMySQLListTableFragmentationTest(t, MariaDBDatabase, tableNameParam, tableNameAuth, tests.WithMCPExec())
	})
}

// RunMariDBListTablesTest run tests against the mysql-list-tables tool
func RunMariDBListTablesTest(t *testing.T, databaseName, tableNameParam, tableNameAuth string) {
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
			tables := make([]tableInfo, 0, len(response.Result.Content))
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

// MariaDB exposes the same tool schemas as the shared MySQL fixtures.
func getMariaDBMCPExpectedTools() []tests.MCPToolManifest {
	expectedTools := tests.GetBaseMCPExpectedTools()
	expectedTools = append(expectedTools, tests.GetExecuteSQLMCPExpectedTools()...)
	expectedTools = append(expectedTools, tests.GetTemplateParamMCPExpectedTools()...)
	expectedTools = append(expectedTools, []tests.MCPToolManifest{
		{
			Name:        "list_tables",
			Description: "Lists tables in the database.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"output_format": map[string]any{"default": "detailed", "description": "Optional: Use 'simple' for names only or 'detailed' for full info.", "type": "string"},
					"table_names":   map[string]any{"default": "", "description": "Optional: A comma-separated list of table names. If empty, details for all tables will be listed.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_active_queries",
			Description: "Lists active queries in the database.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":             map[string]any{"default": float64(100), "description": "Optional: The maximum number of rows to return.", "type": "integer"},
					"min_duration_secs": map[string]any{"default": float64(0), "description": "Optional: Only show queries running for at least this long in seconds", "type": "integer"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_tables_missing_unique_indexes",
			Description: "Lists tables that do not have primary or unique indexes in the database.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":        map[string]any{"default": float64(50), "description": "(Optional) Max rows to return, default is 50", "type": "integer"},
					"table_schema": map[string]any{"default": "", "description": "(Optional) The database where the check is to be performed. Check all tables visible to the current user if not specified", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_table_fragmentation",
			Description: "Lists table fragmentation in the database.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"data_free_threshold_bytes": map[string]any{"default": float64(1), "description": "(Optional) Only show tables with at least this much free space in bytes. Default is 1", "type": "integer"},
					"limit":                     map[string]any{"default": float64(10), "description": "(Optional) Max rows to return, default is 10", "type": "integer"},
					"table_name":                map[string]any{"default": "", "description": "(Optional) Name of the table to be checked. Check all tables visible to the current user if not specified.", "type": "string"},
					"table_schema":              map[string]any{"default": "", "description": "(Optional) The database where fragmentation check is to be executed. Check all tables visible to the current user if not specified", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_table_stats",
			Description: "Lists table stats in the database.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"connected_schema": map[string]any{"description": "(Optional) The connected db", "type": "string"},
					"limit":            map[string]any{"default": float64(10), "description": "(Optional) Max rows to return, default is 10", "type": "integer"},
					"sort_by":          map[string]any{"default": "", "description": "(Optional) The column to sort by", "type": "string"},
					"table_name":       map[string]any{"default": "", "description": "(Optional) Name of the table to be checked. Check all tables visible to the current user if not specified.", "type": "string"},
					"table_schema":     map[string]any{"default": "", "description": "(Optional) The database where statistics  is to be executed. Check all tables visible to the current user if not specified", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "get_query_plan",
			Description: "Gets the query plan for a SQL statement.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sql_statement": map[string]any{"type": "string", "description": "The sql statement to explain."},
				},
				"required": []any{"sql_statement"},
			},
		},
		{
			Name:        "show_query_stats",
			Description: "Lists query statistics in the database.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"connected_schema": map[string]any{"description": "(Optional) The database user is connected to, the value is set from env variable CLOUD_SQL_MYSQL_DATABASE or MYSQL_DATABASE", "type": "string"},
					"limit":            map[string]any{"default": float64(10), "description": "(Optional) Max rows to return, default is 10", "type": "integer"},
					"table_schema":     map[string]any{"default": "", "description": "(Optional) The database where query statistics is to be executed. Check all queries visible to the current user if not specified", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_all_locks",
			Description: "Lists all table, row locks in the database.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"connected_schema": map[string]any{"description": "(Optional) The database user is connected to, the value is set from env variable CLOUD_SQL_MYSQL_DATABASE or MYSQL_DATABASE", "type": "string"},
					"limit":            map[string]any{"default": float64(10), "description": "(Optional) Max rows to return, default is 10", "type": "integer"},
					"table_name":       map[string]any{"default": "", "description": "(Optional) Name of the table to be checked. Check all tables visible to the current user if not specified.", "type": "string"},
					"table_schema":     map[string]any{"default": "", "description": "(Optional) The database where locked object is detected. Check all databases if not specified.", "type": "string"},
				},
				"required": []any{},
			},
		},
	}...)

	return expectedTools
}
