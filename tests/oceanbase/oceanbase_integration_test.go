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

package oceanbase

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"

	_ "github.com/go-sql-driver/mysql"
)

var (
	OceanBaseSourceType = "oceanbase"
	OceanBaseToolType   = "oceanbase-sql"
	OceanBaseDatabase   = os.Getenv("OCEANBASE_DATABASE")
	OceanBaseHost       = os.Getenv("OCEANBASE_HOST")
	OceanBasePort       = os.Getenv("OCEANBASE_PORT")
	OceanBaseUser       = os.Getenv("OCEANBASE_USER")
	OceanBasePass       = os.Getenv("OCEANBASE_PASSWORD")
)

func getOceanBaseVars(t *testing.T) map[string]any {
	switch "" {
	case OceanBaseDatabase:
		t.Fatal("'OCEANBASE_DATABASE' not set")
	case OceanBaseHost:
		t.Fatal("'OCEANBASE_HOST' not set")
	case OceanBasePort:
		t.Fatal("'OCEANBASE_PORT' not set")
	case OceanBaseUser:
		t.Fatal("'OCEANBASE_USER' not set")
	case OceanBasePass:
		t.Fatal("'OCEANBASE_PASSWORD' not set")
	}

	return map[string]any{
		"type":     OceanBaseSourceType,
		"host":     OceanBaseHost,
		"port":     OceanBasePort,
		"database": OceanBaseDatabase,
		"user":     OceanBaseUser,
		"password": OceanBasePass,
	}
}

// Copied over from oceanbase.go
func initOceanBaseConnectionPool(host, port, user, pass, dbname string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", user, pass, host, port, dbname)

	// Interact with the driver directly as you normally would
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return pool, nil
}

func TestOceanBaseToolEndpoints(t *testing.T) {
	tableName := setupOceanBaseTest(t, "--enable-api")
	t.Run("discovery", tests.RunToolGetTest)
	runOceanBaseCallTests(t, tableName, nil, nil, nil)
}

func setupOceanBaseTest(t *testing.T, args ...string) string {
	t.Helper()
	sourceConfig := getOceanBaseVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)

	pool, err := initOceanBaseConnectionPool(OceanBaseHost, OceanBasePort, OceanBaseUser, OceanBasePass, OceanBaseDatabase)
	if err != nil {
		t.Fatalf("unable to create OceanBase connection pool: %s", err)
	}
	t.Cleanup(func() { pool.Close() })

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getOceanBaseParamToolInfo(tableNameParam)
	teardownTable1 := setupOceanBaseTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	t.Cleanup(func() { teardownTable1(t) })

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getOceanBaseAuthToolInfo(tableNameAuth)
	teardownTable2 := setupOceanBaseTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	t.Cleanup(func() { teardownTable2(t) })

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, OceanBaseToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addOceanBaseExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := getOceanBaseTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, OceanBaseToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

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

	return tableNameTemplateParam
}

func runOceanBaseCallTests(t *testing.T, tableName string, invokeOptions []tests.InvokeTestOption, sqlOptions []tests.ExecuteSqlOption, templateOptions []tests.TemplateParamOption) {
	t.Helper()
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := getOceanBaseWants()
	t.Run("invoke", func(t *testing.T) {
		opts := []tests.InvokeTestOption{tests.DisableArrayTest()}
		tests.RunToolInvokeTest(t, select1Want, append(opts, invokeOptions...)...)
	})
	t.Run("mcp_call", func(t *testing.T) {
		tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	})
	t.Run("execute_sql", func(t *testing.T) {
		tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, sqlOptions...)
	})
	t.Run("template_parameters", func(t *testing.T) {
		tests.RunToolInvokeWithTemplateParameters(t, tableName, templateOptions...)
	})
}

// OceanBase specific parameter tool info
func getOceanBaseParamToolInfo(tableName string) (string, string, string, string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT NOT NULL AUTO_INCREMENT PRIMARY KEY, name VARCHAR(255));", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name) VALUES (?), (?), (?), (?);", tableName)
	toolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ? OR name = ?;", tableName)
	idParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ?;", tableName)
	nameParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = ?;", tableName)
	arrayToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ANY(?) AND name = ANY(?);", tableName)
	params := []any{"Alice", "Jane", "Sid", nil}
	return createStatement, insertStatement, toolStatement, idParamStatement, nameParamStatement, arrayToolStatement, params
}

// OceanBase specific auth tool info
func getOceanBaseAuthToolInfo(tableName string) (string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT NOT NULL AUTO_INCREMENT PRIMARY KEY, name VARCHAR(255), email VARCHAR(255));", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name, email) VALUES (?, ?), (?, ?)", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = ?;", tableName)
	params := []any{"Alice", tests.ServiceAccountEmail, "Jane", "janedoe@gmail.com"}
	return createStatement, insertStatement, toolStatement, params
}

// OceanBase specific template tool statements
func getOceanBaseTmplToolStatement() (string, string) {
	tmplSelectCombined := "SELECT * FROM {{.tableName}} WHERE id = ?"
	tmplSelectFilterCombined := "SELECT * FROM {{.tableName}} WHERE {{.columnFilter}} = ?"
	return tmplSelectCombined, tmplSelectFilterCombined
}

// OceanBase specific expected results
func getOceanBaseWants() (string, string, string, string) {
	select1Want := "[{\"1\":1}]"
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: Error 1064 (42000): You have an error in your SQL syntax; check the manual that corresponds to your OceanBase version for the right syntax to use near 'SELEC 1;' at line 1"}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id INT NOT NULL AUTO_INCREMENT PRIMARY KEY, name VARCHAR(255))"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"1\":1}"}]}}`
	return select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want
}

// Add OceanBase Execute SQL configuration
func addOceanBaseExecuteSqlConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "oceanbase-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "oceanbase-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	config["tools"] = tools
	return config
}

// Setup OceanBase table
func setupOceanBaseTable(t *testing.T, ctx context.Context, pool *sql.DB, createStatement, insertStatement, tableName string, params []any) func(*testing.T) {
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

	return func(t *testing.T) {
		// tear down test
		_, err = pool.ExecContext(ctx, fmt.Sprintf("DROP TABLE %s;", tableName))
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}
}
