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

package cloudsqlmysql

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

func TestCloudSQLMySQLMCPListTools(t *testing.T) {
	sourceConfig := getCloudSQLMySQLVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := initCloudSQLMySQLConnectionPool(CloudSQLMySQLProject, CloudSQLMySQLRegion, CloudSQLMySQLInstance, "public", CloudSQLMySQLUser, CloudSQLMySQLPass, CloudSQLMySQLDatabase)
	if err != nil {
		t.Fatalf("unable to create Cloud SQL connection pool: %s", err)
	}

	// cleanup test environment
	tests.CleanupMySQLTables(t, ctx, pool)

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := tests.GetMySQLParamToolInfo(tableNameParam)
	teardownTable1 := tests.SetupMySQLTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := tests.GetMySQLAuthToolInfo(tableNameAuth)
	teardownTable2 := tests.SetupMySQLTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, CloudSQLMySQLToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddMySqlExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMySQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, CloudSQLMySQLToolType, tmplSelectCombined, tmplSelectFilterCombined, "")
	toolsFile = tests.AddMySQLPrebuiltToolConfig(t, toolsFile)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Expected Manifest
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
					"table_names":   map[string]any{"default": "", "description": "Optional: A comma-separated list of table names. If empty, details for all tables in the configured database will be listed. If the source has no configured database, all user schemas are listed.", "type": "string"},
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

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, expectedTools)
	})
}

func TestCloudSQLMySQLMCPCallTool(t *testing.T) {
	sourceConfig := getCloudSQLMySQLVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := initCloudSQLMySQLConnectionPool(CloudSQLMySQLProject, CloudSQLMySQLRegion, CloudSQLMySQLInstance, "public", CloudSQLMySQLUser, CloudSQLMySQLPass, CloudSQLMySQLDatabase)
	if err != nil {
		t.Fatalf("unable to create Cloud SQL connection pool: %s", err)
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
	toolsFile := tests.GetToolsConfig(sourceConfig, CloudSQLMySQLToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddMySqlExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMySQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, CloudSQLMySQLToolType, tmplSelectCombined, tmplSelectFilterCombined, "")
	toolsFile = tests.AddMySQLPrebuiltToolConfig(t, toolsFile)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := tests.GetMySQLWants()

	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest(), tests.WithMCP(), tests.WithNullWant("[]"))
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, tests.WithMCPSql(), tests.WithExecuteCreateWant("[]"), tests.WithExecuteDropWant("[]"), tests.WithExecuteSelectEmptyWant("[]"))
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithMCPTemplate())

	// Run specific MySQL tool tests over MCP
	const expectedOwner = "'toolbox-identity'@'%'"
	tests.RunMySQLListTablesTest(t, CloudSQLMySQLDatabase, tableNameParam, tableNameAuth, expectedOwner, tests.WithMCPExec())
	tests.RunMySQLListActiveQueriesTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunMySQLGetQueryPlanTest(t, ctx, pool, CloudSQLMySQLDatabase, tableNameParam, tests.WithMCPExec())
}
