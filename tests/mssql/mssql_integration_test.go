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

package mssql

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	_ "github.com/microsoft/go-mssqldb"
)

var (
	MSSQLSourceType = "mssql"
	MSSQLToolType   = "mssql-sql"
	MSSQLDatabase   = os.Getenv("MSSQL_DATABASE")
	MSSQLHost       = os.Getenv("MSSQL_HOST")
	MSSQLPort       = os.Getenv("MSSQL_PORT")
	MSSQLUser       = os.Getenv("MSSQL_USER")
	MSSQLPass       = os.Getenv("MSSQL_PASS")
)

func getMsSQLVars(t *testing.T) map[string]any {
	switch "" {
	case MSSQLDatabase:
		t.Fatal("'MSSQL_DATABASE' not set")
	case MSSQLHost:
		t.Fatal("'MSSQL_HOST' not set")
	case MSSQLPort:
		t.Fatal("'MSSQL_PORT' not set")
	case MSSQLUser:
		t.Fatal("'MSSQL_USER' not set")
	case MSSQLPass:
		t.Fatal("'MSSQL_PASS' not set")
	}

	return map[string]any{
		"type":     MSSQLSourceType,
		"host":     MSSQLHost,
		"port":     MSSQLPort,
		"database": MSSQLDatabase,
		"user":     MSSQLUser,
		"password": MSSQLPass,
	}
}

// Copied over from mssql.go
func initMSSQLConnection(host, port, user, pass, dbname string) (*sql.DB, error) {
	// Create dsn
	query := url.Values{}
	query.Add("database", dbname)
	url := &url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(user, pass),
		Host:     fmt.Sprintf("%s:%s", host, port),
		RawQuery: query.Encode(),
	}

	// Open database connection
	db, err := sql.Open("sqlserver", url.String())
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return db, nil
}

func TestMSSQLToolEndpoints(t *testing.T) {
	sourceConfig := getMsSQLVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	pool, err := initMSSQLConnection(MSSQLHost, MSSQLPort, MSSQLUser, MSSQLPass, MSSQLDatabase)
	if err != nil {
		t.Fatalf("unable to create SQL Server connection pool: %s", err)
	}
	defer pool.Close()

	// cleanup test environment
	tests.CleanupMSSQLTables(t, ctx, pool)

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := tests.GetMSSQLParamToolInfo(tableNameParam)
	teardownTable1 := tests.SetupMsSQLTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := tests.GetMSSQLAuthToolInfo(tableNameAuth)
	teardownTable2 := tests.SetupMsSQLTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, MSSQLToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddMSSQLExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMSSQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, MSSQLToolType, tmplSelectCombined, tmplSelectFilterCombined, "")
	toolsFile = tests.AddMSSQLPrebuiltToolConfig(t, toolsFile)
	toolsFile = addNonCloudMSSQLPrebuiltToolConfig(t, toolsFile)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()
	defer cmd.Close()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Get configs for tests
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := tests.GetMSSQLWants()

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)

	// Run specific MSSQL tool tests
	tests.RunMSSQLListTablesTest(t, tableNameParam, tableNameAuth)
	tests.RunMSSQLGetDatabaseObjectSourceCodeTest(t, tableNameParam, tableNameAuth)
	runMSSQLListTablesForeignKeyTest(t, ctx, pool)
}

// runMSSQLListTablesForeignKeyTest checks that list_tables reports a definition
// for foreign key constraints. It creates its own tables, so it runs after the
// list_tables tests that expect only the param and auth tables in dbo.
func runMSSQLListTablesForeignKeyTest(t *testing.T, ctx context.Context, pool *sql.DB) {
	suffix := strings.ReplaceAll(uuid.New().String(), "-", "")
	parentTable := "fk_parent_" + suffix
	childTable := "fk_child_" + suffix
	fkName := "fk_child_parent_" + suffix

	stmts := []string{
		fmt.Sprintf("CREATE TABLE %s (id INT PRIMARY KEY, code INT NOT NULL, CONSTRAINT uq_%s UNIQUE (id, code))", parentTable, suffix),
		fmt.Sprintf("CREATE TABLE %s (id INT PRIMARY KEY, parent_id INT NOT NULL, parent_code INT NOT NULL, CONSTRAINT %s FOREIGN KEY (parent_id, parent_code) REFERENCES dbo.%s (id, code) ON DELETE CASCADE)", childTable, fkName, parentTable),
	}
	for _, stmt := range stmts {
		if _, err := pool.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("unable to set up foreign key tables: %s", err)
		}
	}
	defer func() {
		for _, table := range []string{childTable, parentTable} {
			if _, err := pool.ExecContext(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
				t.Errorf("unable to drop table %s: %s", table, err)
			}
		}
	}()

	const api = "http://127.0.0.1:5000/api/tool/list_tables/invoke"
	reqBytes, _ := json.Marshal(map[string]any{"table_names": childTable})
	resp, respBytes := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer(reqBytes), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBytes))
	}

	var body struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(respBytes, &body); err != nil {
		t.Fatalf("error parsing response body: %s, body: %s", err, string(respBytes))
	}
	var rows []struct {
		ObjectDetails json.RawMessage `json:"object_details"`
	}
	if err := json.Unmarshal([]byte(body.Result), &rows); err != nil {
		t.Fatalf("error parsing result: %s, result: %s", err, body.Result)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 table, got %d: %s", len(rows), body.Result)
	}
	// object_details is the JSON document built by the statement, returned as a string.
	detailsJSON := []byte(rows[0].ObjectDetails)
	var detailsStr string
	if err := json.Unmarshal(detailsJSON, &detailsStr); err == nil {
		detailsJSON = []byte(detailsStr)
	}
	var details struct {
		Constraints []map[string]any `json:"constraints"`
	}
	if err := json.Unmarshal(detailsJSON, &details); err != nil {
		t.Fatalf("error parsing object_details: %s, object_details: %s", err, string(detailsJSON))
	}

	want := fmt.Sprintf("FOREIGN KEY (parent_id, parent_code) REFERENCES dbo.%s (id, code) ON DELETE CASCADE", parentTable)
	for _, c := range details.Constraints {
		if c["constraint_name"] != fkName {
			continue
		}
		if c["constraint_type"] != "FOREIGN KEY" {
			t.Errorf("constraint_type: got %v, want FOREIGN KEY", c["constraint_type"])
		}
		if got := c["constraint_definition"]; got != want {
			t.Errorf("constraint_definition: got %v, want %q", got, want)
		}
		return
	}
	t.Fatalf("foreign key %s not found in constraints: %s", fkName, string(detailsJSON))
}

func addNonCloudMSSQLPrebuiltToolConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["get_database_object_source_code"] = map[string]any{
		"type":        "mssql-get-database-object-source-code",
		"source":      "my-instance",
		"description": "Executes a query to retrieve source code for all coding objects.",
	}
	config["tools"] = tools
	return config
}
