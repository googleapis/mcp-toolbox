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
	"io"
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

	args := []string{"--enable-api"}

	pool, err := initMariaDB(MariaDBHost, MariaDBPort, MariaDBUser, MariaDBPass, MariaDBDatabase)
	if err != nil {
		t.Fatalf("unable to create MySQL connection pool: %s", err)
	}

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
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := GetMariaDBWants()

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)

	// Run specific MySQL tool tests
	RunMariDBListTablesTest(t, MariaDBDatabase, tableNameParam, tableNameAuth)
	tests.RunMySQLListActiveQueriesTest(t, ctx, pool)
	tests.RunMySQLListTablesMissingUniqueIndexes(t, ctx, pool, MariaDBDatabase)
	tests.RunMySQLListTableFragmentationTest(t, MariaDBDatabase, tableNameParam, tableNameAuth)
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
		requestBody    io.Reader
		wantStatusCode int
		want           any
		isSimple       bool
		isAllTables    bool
	}{
		{
			name:           "invoke list_tables for all tables detailed output",
			requestBody:    bytes.NewBufferString(`{"table_names":""}`),
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{authTableWant, paramTableWant},
			isAllTables:    true,
		},
		{
			name:           "invoke list_tables detailed output",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_names": "%s"}`, tableNameAuth)),
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{authTableWant},
		},
		{
			name:           "invoke list_tables simple output",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_names": "%s", "output_format": "simple"}`, tableNameAuth)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{{"name": tableNameAuth}},
			isSimple:       true,
		},
		{
			name:           "invoke list_tables with multiple table names",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_names": "%s,%s"}`, tableNameParam, tableNameAuth)),
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{authTableWant, paramTableWant},
		},
		{
			name:           "invoke list_tables with one existing and one non-existent table",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_names": "%s,non_existent_table"}`, tableNameAuth)),
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{authTableWant},
		},
		{
			name:           "invoke list_tables with non-existent table",
			requestBody:    bytes.NewBufferString(`{"table_names": "non_existent_table"}`),
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{},
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_tables/invoke"
			resp, body := tests.RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(body))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(body, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got any
			if tc.isSimple {
				var tables []tableInfo
				if err := json.Unmarshal([]byte(resultString), &tables); err != nil {
					t.Fatalf("failed to unmarshal outer JSON array into []tableInfo: %v", err)
				}
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
				var tables []tableInfo
				if err := json.Unmarshal([]byte(resultString), &tables); err != nil {
					t.Fatalf("failed to unmarshal outer JSON array into []tableInfo: %v", err)
				}
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
