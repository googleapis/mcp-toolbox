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

package trino

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	_ "github.com/trinodb/trino-go-client/trino" // Import Trino SQL driver
)

// setupTrinoMCPServer sets up the test tables and starts a Toolbox server
// serving the Trino tools over the MCP endpoint. Every teardown step is
// registered with t.Cleanup, so nothing leaks if setup fails partway through.
// Cleanups run LIFO, so the server stops before the tables it queries are
// dropped, and the pool is closed last.
func setupTrinoMCPServer(t *testing.T, ctx context.Context) string {
	sourceConfig := getTrinoVars(t)

	pool, err := initTrinoConnectionPool(TrinoHost, TrinoPort, TrinoUser, TrinoPass, TrinoCatalog, TrinoSchema)
	if err != nil {
		t.Fatalf("unable to create Trino connection pool: %s", err)
	}
	t.Cleanup(func() { pool.Close() })

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getTrinoParamToolInfo(tableNameParam)
	teardownTable1 := setupTrinoTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	t.Cleanup(func() { teardownTable1(t) })

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getTrinoAuthToolInfo(tableNameAuth)
	teardownTable2 := setupTrinoTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	t.Cleanup(func() { teardownTable2(t) })

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, TrinoToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addTrinoExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := getTrinoTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, TrinoToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	// Registered last so it runs first, stopping the server before the tables
	// it queries are dropped.
	t.Cleanup(cleanup)

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	return tableNameTemplateParam
}

func TestTrinoMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	setupTrinoMCPServer(t, ctx)

	expectedTools := tests.GetBaseMCPExpectedTools()
	expectedTools = append(expectedTools, tests.GetExecuteSQLMCPExpectedTools()...)
	expectedTools = append(expectedTools, tests.GetTemplateParamMCPExpectedTools()...)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, expectedTools)
	})
}

func TestTrinoMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tableNameTemplateParam := setupTrinoMCPServer(t, ctx)

	// Get configs for tests
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := getTrinoWants()

	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest(), tests.WithMCP())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, tests.WithMCPSql())
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithInsert1Want(`[{"rows":1}]`), tests.WithMCPTemplate())
}
