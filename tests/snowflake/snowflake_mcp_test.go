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

package snowflake

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	_ "github.com/snowflakedb/gosnowflake/v2"
)

// setupSnowflakeMCPServer sets up the test tables and starts a Toolbox server
// serving the Snowflake tools over the MCP endpoint. Every teardown step is
// registered with t.Cleanup, so nothing leaks if setup fails partway through.
// Cleanups run LIFO, so the server stops before the tables it queries are
// dropped, and the connection is closed last.
func setupSnowflakeMCPServer(t *testing.T, ctx context.Context) string {
	sourceConfig := getSnowflakeVars(t)

	db, err := initSnowflakeConnectionPool(ctx, SnowflakeAccount, SnowflakeUser, SnowflakePassword, SnowflakeDatabase, SnowflakeSchema, SnowflakeWarehouse, SnowflakeRole)
	if err != nil {
		t.Fatalf("unable to create snowflake connection pool: %s", err)
	}
	t.Cleanup(func() { db.Close() })

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getSnowflakeParamToolInfo(tableNameParam)
	teardownTable1 := setupSnowflakeTable(t, ctx, db, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	t.Cleanup(func() { teardownTable1(t) })

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getSnowflakeAuthToolInfo(tableNameAuth)
	teardownTable2 := setupSnowflakeTable(t, ctx, db, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	t.Cleanup(func() { teardownTable2(t) })

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, SnowflakeToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addSnowflakeExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := getSnowflakeTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, SnowflakeToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

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

	return tableNameTemplateParam
}

func TestSnowflakeMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	setupSnowflakeMCPServer(t, ctx)

	expectedTools := tests.GetBaseMCPExpectedTools()
	expectedTools = append(expectedTools, tests.GetExecuteSQLMCPExpectedTools()...)
	expectedTools = append(expectedTools, tests.GetTemplateParamMCPExpectedTools()...)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, expectedTools)
	})
}

func TestSnowflakeMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tableNameTemplateParam := setupSnowflakeMCPServer(t, ctx)

	select1Want, failInvocationWant, createTableStatement, mcpSelect1Want := getSnowflakeWants()

	// Snowflake returns upper-case column names and numeric columns as strings,
	// so the row expectations from the legacy test are carried over unchanged.
	tests.RunToolInvokeTest(t, select1Want,
		tests.DisableArrayTest(),
		tests.WithMyToolId3NameAliceWant(`[{"ID":"1","NAME":"Alice"},{"ID":"3","NAME":"Sid"}]`),
		tests.WithMyToolById4Want(`[{"ID":"4","NAME":null}]`),
		tests.WithMCP(),
	)
	tests.RunMCPToolCallMethod(t, failInvocationWant, mcpSelect1Want,
		tests.WithMcpMyToolId3NameAliceWant(`{"jsonrpc":"2.0","id":"my-tool","result":{"content":[{"type":"text","text":"{\"ID\":\"1\",\"NAME\":\"Alice\"}"},{"type":"text","text":"{\"ID\":\"3\",\"NAME\":\"Sid\"}"}]}}`),
		tests.WithMcpMySecureToolWant(`[{"ID":"1","NAME":"Alice"},{"ID":"3","NAME":"Sid"}]`),
	)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want,
		tests.WithExecuteCreateWant(`[{"status":"Table T successfully created."}]`),
		tests.WithExecuteDropWant(`[{"status":"T successfully dropped."}]`),
		tests.WithMCPSql(),
	)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithMCPTemplate())
}
