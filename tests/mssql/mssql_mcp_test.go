// Copyright 2026 Google LLC
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
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	_ "github.com/microsoft/go-mssqldb"
)

// setupMSSQLMCPServer sets up the test tables and starts a Toolbox server
// serving the SQL Server tools over the MCP endpoint. Every teardown step is
// registered with t.Cleanup, so nothing leaks if setup fails partway through.
// Cleanups run LIFO, so the server stops before the tables it queries are
// dropped, and the pool is closed last.
func setupMSSQLMCPServer(t *testing.T, ctx context.Context) (string, string, string) {
	sourceConfig := getMsSQLVars(t)

	pool, err := initMSSQLConnection(MSSQLHost, MSSQLPort, MSSQLUser, MSSQLPass, MSSQLDatabase)
	if err != nil {
		t.Fatalf("unable to create SQL Server connection pool: %s", err)
	}
	t.Cleanup(func() { pool.Close() })

	// cleanup test environment
	tests.CleanupMSSQLTables(t, ctx, pool)

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := tests.GetMSSQLParamToolInfo(tableNameParam)
	teardownTable1 := tests.SetupMsSQLTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	t.Cleanup(func() { teardownTable1(t) })

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := tests.GetMSSQLAuthToolInfo(tableNameAuth)
	teardownTable2 := tests.SetupMsSQLTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	t.Cleanup(func() { teardownTable2(t) })

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, MSSQLToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddMSSQLExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMSSQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, MSSQLToolType, tmplSelectCombined, tmplSelectFilterCombined, "")
	toolsFile = tests.AddMSSQLPrebuiltToolConfig(t, toolsFile)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	// Registered last so it runs first. StartCmd's cleanup only removes the
	// temporary config file, so Close is what stops the server and closes its
	// pipes, before the tables it queries are dropped.
	t.Cleanup(func() {
		cmd.Close()
		cleanup()
	})

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	return tableNameParam, tableNameAuth, tableNameTemplateParam
}

func TestMSSQLMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	setupMSSQLMCPServer(t, ctx)

	expectedTools := tests.GetBaseMCPExpectedTools()
	expectedTools = append(expectedTools, tests.GetExecuteSQLMCPExpectedTools()...)
	expectedTools = append(expectedTools, tests.GetTemplateParamMCPExpectedTools()...)
	expectedTools = append(expectedTools, tests.MCPToolManifest{
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
	})

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, expectedTools)
	})
}

func TestMSSQLMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tableNameParam, tableNameAuth, tableNameTemplateParam := setupMSSQLMCPServer(t, ctx)

	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := tests.GetMSSQLWants()

	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest(), tests.WithMCP())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, tests.WithMCPSql())
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithMCPTemplate())

	// Run the SQL Server specific tool test over MCP
	tests.RunMSSQLListTablesTest(t, tableNameParam, tableNameAuth, tests.WithMCPExec())
}
