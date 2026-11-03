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
	"context"
	"database/sql"
	"fmt"
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
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := tests.GetMSSQLWants()

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)

	// Run specific MSSQL tool tests
	tests.RunMSSQLListTablesTest(t, tableNameParam, tableNameAuth)
	tests.RunMSSQLListIndexesTest(t, tableNameParam)
}
