// Copyright © 2026, Oracle and/or its affiliates.

package oracle

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
)

// getOracleWants returns the expected wants for oracle. The legacy test
// declares these inline; they live here so both files can share one copy.
func getOracleWants() (string, string, string, string) {
	select1Want := "[{\"1\":1}]"
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: dpiStmt_execute: ORA-00900: invalid SQL statement\nHelp: https://docs.oracle.com/error-help/db/ora-00900/"}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id NUMBER GENERATED AS IDENTITY PRIMARY KEY, name VARCHAR2(255))"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"1\":1}"}]}}`
	return select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want
}

// setupOracleMCPServer sets up the test tables and starts a Toolbox server
// serving the Oracle tools over the MCP endpoint. Every teardown step is
// registered with t.Cleanup, so nothing leaks if setup fails partway through.
// Cleanups run LIFO, so the server stops before the tables it queries are
// dropped, and the connection is closed last.
func setupOracleMCPServer(t *testing.T, ctx context.Context) string {
	sourceConfig := getOracleVars(t)

	db, err := initOracleConnection(ctx, OracleUser, OraclePass, OracleConnStr)
	if err != nil {
		t.Fatalf("unable to create Oracle connection pool: %s", err)
	}
	t.Cleanup(func() { db.Close() })

	dropAllUserTables(t, ctx, db)

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getOracleParamToolInfo(tableNameParam)
	teardownTable1 := setupOracleTable(t, ctx, db, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	t.Cleanup(func() { teardownTable1(t) })

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getOracleAuthToolInfo(tableNameAuth)
	teardownTable2 := setupOracleTable(t, ctx, db, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	t.Cleanup(func() { teardownTable2(t) })

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, OracleToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddExecuteSqlConfig(t, toolsFile, "oracle-execute-sql")
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMySQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, OracleToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

	// Configure a DML tool to verify the 'readonly: false' logic.
	updateStmt := fmt.Sprintf(`UPDATE %s SET "name" = :1 WHERE "id" = :2`, tableNameParam)

	toolsMap, ok := toolsFile["tools"].(map[string]any)
	if !ok {
		t.Fatal("Configuration error: 'tools' key is missing or is not a map")
	}

	toolsMap["my-update-tool"] = map[string]any{
		"type":        "oracle-sql",
		"source":      "my-instance",
		"statement":   updateStmt,
		"readOnly":    false,
		"description": "Update user name by ID.",
		"parameters": []map[string]any{
			{
				"name":        "name",
				"type":        "string",
				"description": "The new name for the user.",
			},
			{
				"name":        "id",
				"type":        "integer",
				"description": "The user ID to update.",
			},
		},
	}

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

func TestOracleMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	setupOracleMCPServer(t, ctx)

	expectedTools := tests.GetBaseMCPExpectedTools()
	expectedTools = append(expectedTools, tests.GetExecuteSQLMCPExpectedTools()...)
	expectedTools = append(expectedTools, tests.GetTemplateParamMCPExpectedTools()...)
	expectedTools = append(expectedTools, tests.MCPToolManifest{
		Name:        "my-update-tool",
		Description: "Update user name by ID.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "The new name for the user."},
				"id":   map[string]any{"type": "integer", "description": "The user ID to update."},
			},
			"required": []any{"name", "id"},
		},
	})

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, expectedTools)
	})
}

func TestOracleMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tableNameTemplateParam := setupOracleMCPServer(t, ctx)

	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := getOracleWants()

	tests.RunToolInvokeTest(t, select1Want,
		tests.DisableOptionalNullParamTest(),
		tests.WithMyToolById4Want("[{\"id\":4,\"name\":\"\"}]"),
		tests.DisableArrayTest(),
		tests.WithMCP(),
	)
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, tests.WithMCPSql())
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithMCPTemplate())

	// Invoke the 'my-update-tool' and verify the result. This already ran over
	// /mcp in the legacy test.
	testDmlQueries(t, "my-update-tool",
		`{"name": "UpdatedAlice", "id": 1}`,
		`\"rows_affected\":1`)
}
