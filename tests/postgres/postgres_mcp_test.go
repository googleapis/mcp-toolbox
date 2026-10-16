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

package postgres

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

func TestPostgresMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	PostgresHost, PostgresPort, containerCleanup := setupPostgresTestContainer(ctx, t)
	t.Cleanup(containerCleanup)

	sourceConfig := getPostgresVars(t, PostgresHost, PostgresPort)

	// Generate a unique ID
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")

	// create table names with UUID
	tableNameParam := "param_table_" + uniqueID
	tableNameAuth := "auth_table_" + uniqueID

	// get tool statements; tools/list does not execute them, so no tables are created
	_, _, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, _ := tests.GetPostgresSQLParamToolInfo(tableNameParam)
	_, _, authToolStmt, _ := tests.GetPostgresSQLAuthToolInfo(tableNameAuth)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, PostgresToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddExecuteSqlConfig(t, toolsFile, "postgres-execute-sql")
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetPostgresSQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, PostgresToolType, tmplSelectCombined, tmplSelectFilterCombined, "")
	toolsFile = tests.AddPostgresPrebuiltConfig(t, toolsFile)

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
	expectedTools = append(expectedTools, tests.GetPostgresPrebuiltMCPExpectedTools()...)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, expectedTools)
	})
}

func TestPostgresMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	PostgresHost, PostgresPort, containerCleanup := setupPostgresTestContainer(ctx, t)
	t.Cleanup(containerCleanup)

	sourceConfig := getPostgresVars(t, PostgresHost, PostgresPort)

	pool, err := initPostgresConnectionPool(PostgresHost, PostgresPort, PostgresUser, PostgresPass, PostgresDatabase)
	if err != nil {
		t.Fatalf("unable to create postgres connection pool: %s", err)
	}

	// Generate a unique ID
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")

	// This will execute after all tool tests complete (success, fail, or t.Fatal)
	t.Cleanup(func() {
		tests.CleanupPostgresTables(t, context.Background(), pool, uniqueID)
	})

	// Create table names using the UUID
	tableNameParam := "param_table_" + uniqueID
	tableNameAuth := "auth_table_" + uniqueID
	tableNameTemplateParam := "template_param_table_" + uniqueID

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := tests.GetPostgresSQLParamToolInfo(tableNameParam)
	teardownTable1 := tests.SetupPostgresSQLTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := tests.GetPostgresSQLAuthToolInfo(tableNameAuth)
	teardownTable2 := tests.SetupPostgresSQLTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, PostgresToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddExecuteSqlConfig(t, toolsFile, "postgres-execute-sql")
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetPostgresSQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, PostgresToolType, tmplSelectCombined, tmplSelectFilterCombined, "")
	toolsFile = tests.AddPostgresPrebuiltConfig(t, toolsFile)

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

	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := tests.GetPostgresWants()

	tests.RunToolInvokeTest(t, select1Want, tests.WithMCP())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, tests.WithMCPSql())
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithMCPTemplate())

	// Run Postgres prebuilt tool tests over MCP
	tests.RunPostgresListTablesTest(t, tableNameParam, tableNameAuth, PostgresUser, tests.WithMCPExec())
	tests.RunPostgresListViewsTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListSchemasTest(t, ctx, pool, PostgresUser, uniqueID, tests.WithMCPExec())
	tests.RunPostgresListActiveQueriesTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListAvailableExtensionsTest(t, tests.WithMCPExec())
	tests.RunPostgresListInstalledExtensionsTest(t, tests.WithMCPExec())
	tests.RunPostgresDatabaseOverviewTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListTriggersTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListIndexesTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListSequencesTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresLongRunningTransactionsTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListLocksTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresReplicationStatsTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListQueryStatsTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresGetColumnCardinalityTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListTableStatsTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListPublicationTablesTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListTableSpacesTest(t, tests.WithMCPExec())
	tests.RunPostgresListPgSettingsTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListDatabaseStatsTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListRolesTest(t, ctx, pool, tests.WithMCPExec())
	tests.RunPostgresListStoredProcedureTest(t, ctx, pool, tests.WithMCPExec())
}
