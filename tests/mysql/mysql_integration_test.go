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

package mysql

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
)

var (
	MySQLSourceType = "mysql"
	MySQLToolType   = "mysql-sql"
	MySQLDatabase   = os.Getenv("MYSQL_DATABASE")
	MySQLHost       = os.Getenv("MYSQL_HOST")
	MySQLPort       = os.Getenv("MYSQL_PORT")
	MySQLUser       = os.Getenv("MYSQL_USER")
	MySQLPass       = os.Getenv("MYSQL_PASS")
)

func getMySQLVars(t *testing.T) map[string]any {
	switch "" {
	case MySQLDatabase:
		t.Fatal("'MYSQL_DATABASE' not set")
	case MySQLHost:
		t.Fatal("'MYSQL_HOST' not set")
	case MySQLPort:
		t.Fatal("'MYSQL_PORT' not set")
	case MySQLUser:
		t.Fatal("'MYSQL_USER' not set")
	case MySQLPass:
		t.Fatal("'MYSQL_PASS' not set")
	}

	return map[string]any{
		"type":     MySQLSourceType,
		"host":     MySQLHost,
		"port":     MySQLPort,
		"database": MySQLDatabase,
		"user":     MySQLUser,
		"password": MySQLPass,
	}
}

// Copied over from mysql.go
func initMySQLConnectionPool(host, port, user, pass, dbname string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", user, pass, host, port, dbname)

	// Interact with the driver directly as you normally would
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return pool, nil
}

type mysqlTestFixture struct {
	ctx           context.Context
	pool          *sql.DB
	paramTable    string
	authTable     string
	templateTable string
}

type mysqlTestOptions struct {
	invoke     []tests.InvokeTestOption
	executeSQL []tests.ExecuteSqlOption
	template   []tests.TemplateParamOption
	prebuilt   []tests.ToolExecOption
}

func TestMySQLToolEndpoints(t *testing.T) {
	fixture := setupMySQLTest(t, "--enable-api")
	t.Run("discovery", tests.RunToolGetTest)
	runMySQLCallTests(t, fixture, mysqlTestOptions{})
}

func setupMySQLTest(t *testing.T, args ...string) mysqlTestFixture {
	t.Helper()
	sourceConfig := getMySQLVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)

	pool, err := initMySQLConnectionPool(MySQLHost, MySQLPort, MySQLUser, MySQLPass, MySQLDatabase)
	if err != nil {
		t.Fatalf("unable to create MySQL connection pool: %s", err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Errorf("failed to close MySQL connection pool: %v", err)
		}
	})

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
	toolsFile := tests.GetToolsConfig(sourceConfig, MySQLToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddMySqlExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMySQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, MySQLToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

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

	return mysqlTestFixture{ctx: ctx, pool: pool, paramTable: tableNameParam, authTable: tableNameAuth, templateTable: tableNameTemplateParam}
}

func runMySQLCallTests(t *testing.T, fixture mysqlTestFixture, options mysqlTestOptions) {
	t.Helper()
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := tests.GetMySQLWants()
	t.Run("invoke", func(t *testing.T) {
		opts := append([]tests.InvokeTestOption{tests.DisableArrayTest()}, options.invoke...)
		tests.RunToolInvokeTest(t, select1Want, opts...)
	})
	t.Run("mcp_call", func(t *testing.T) {
		tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	})
	t.Run("execute_sql", func(t *testing.T) {
		tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, options.executeSQL...)
	})
	t.Run("template_parameters", func(t *testing.T) {
		tests.RunToolInvokeWithTemplateParameters(t, fixture.templateTable, options.template...)
	})
	t.Run("list_tables", func(t *testing.T) {
		tests.RunMySQLListTablesTest(t, MySQLDatabase, fixture.paramTable, fixture.authTable, "", options.prebuilt...)
	})
	t.Run("list_active_queries", func(t *testing.T) {
		tests.RunMySQLListActiveQueriesTest(t, fixture.ctx, fixture.pool, options.prebuilt...)
	})
	t.Run("list_tables_missing_unique_indexes", func(t *testing.T) {
		tests.RunMySQLListTablesMissingUniqueIndexes(t, fixture.ctx, fixture.pool, MySQLDatabase, options.prebuilt...)
	})
	t.Run("list_table_fragmentation", func(t *testing.T) {
		tests.RunMySQLListTableFragmentationTest(t, MySQLDatabase, fixture.paramTable, fixture.authTable, options.prebuilt...)
	})
	t.Run("get_query_plan", func(t *testing.T) {
		tests.RunMySQLGetQueryPlanTest(t, fixture.ctx, fixture.pool, MySQLDatabase, fixture.paramTable, options.prebuilt...)
	})
	t.Run("list_all_locks", func(t *testing.T) {
		tests.RunMySQLListAllLocks(t, fixture.ctx, fixture.pool, MySQLDatabase, options.prebuilt...)
	})
	t.Run("show_query_stats", func(t *testing.T) {
		tests.RunMySQLShowQueryStats(t, fixture.ctx, fixture.pool, MySQLDatabase, options.prebuilt...)
	})
	t.Run("list_table_stats", func(t *testing.T) {
		tests.RunMySQLListTableStatsTest(t, fixture.ctx, fixture.pool, MySQLDatabase, fixture.paramTable, fixture.authTable, options.prebuilt...)
	})
}
