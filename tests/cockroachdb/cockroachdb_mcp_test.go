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

package cockroachdb

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/testcontainers/testcontainers-go"
	tccockroachdb "github.com/testcontainers/testcontainers-go/modules/cockroachdb"
)

// setupCockroachDBMCPServer starts a CockroachDB container, sets up the test
// tables and starts a Toolbox server serving the CockroachDB tools over the MCP
// endpoint. Every teardown step is registered with t.Cleanup, so nothing leaks
// if setup fails partway through.
func setupCockroachDBMCPServer(t *testing.T, ctx context.Context) string {
	tccockroachdbContainer, err := tccockroachdb.Run(ctx, "cockroachdb/cockroach:latest-v23.1",
		testcontainers.WithCmd("start-single-node", "--insecure"),
	)
	if err != nil {
		t.Fatalf("failed to start container: %s", err)
	}
	t.Cleanup(func() {
		if err := tccockroachdbContainer.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate testcontainer: %s", err)
		}
	})

	host, err := tccockroachdbContainer.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get host: %s", err)
	}

	port, err := tccockroachdbContainer.MappedPort(ctx, "26257/tcp")
	if err != nil {
		t.Fatalf("failed to get port: %s", err)
	}

	connString := fmt.Sprintf("postgres://root@%s:%s/defaultdb?sslmode=disable", host, port.Port())
	sourceConfig := getCockroachDBVars(host, port.Port())

	pool, err := initCockroachDBConnectionPool(connString)
	if err != nil {
		t.Fatalf("unable to create cockroachdb connection pool: %s", err)
	}
	// Note: Don't close the pool in the teardown - it is only used for test
	// setup/teardown. Closing it explicitly can cause hangs if the server's pool
	// is still active. The pool will be cleaned up when the test exits.

	// Generate a unique ID and create table names using it
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameParam := "param_table_" + uniqueID
	tableNameAuth := "auth_table_" + uniqueID
	tableNameTemplateParam := "template_param_table_" + uniqueID

	// set up data for param tool (using CockroachDB explicit INT primary keys)
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := tests.GetCockroachDBParamToolInfo(tableNameParam)
	teardownTable1 := tests.SetupPostgresSQLTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	t.Cleanup(func() { teardownTable1(t) })

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := tests.GetCockroachDBAuthToolInfo(tableNameAuth)
	teardownTable2 := tests.SetupPostgresSQLTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	t.Cleanup(func() { teardownTable2(t) })

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, CockroachDBToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)

	// Add execute-sql tool with write-enabled source (CockroachDB MCP security requires explicit opt-in)
	toolsFile = addCockroachDBExecuteSqlConfig(t, toolsFile, sourceConfig)

	tmplSelectCombined, tmplSelectFilterCombined := tests.GetPostgresSQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, CockroachDBToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

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

func TestCockroachDBMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	setupCockroachDBMCPServer(t, ctx)

	expectedTools := tests.GetBaseMCPExpectedTools()
	expectedTools = append(expectedTools, tests.GetExecuteSQLMCPExpectedTools()...)
	expectedTools = append(expectedTools, tests.GetTemplateParamMCPExpectedTools()...)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, expectedTools)
	})
}

func TestCockroachDBMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	tableNameTemplateParam := setupCockroachDBMCPServer(t, ctx)

	// Get configs for tests (use CockroachDB-specific expectations)
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := tests.GetCockroachDBWants()

	tests.RunToolInvokeTest(t, select1Want, tests.WithMCP())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, tests.WithMCPSql())
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithMCPTemplate())
}
