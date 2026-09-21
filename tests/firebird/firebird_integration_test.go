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

package firebird

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
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/googleapis/mcp-toolbox/tests"
	_ "github.com/nakagami/firebirdsql"
)

var (
	FirebirdSourceType = "firebird"
	FirebirdToolType   = "firebird-sql"
	FirebirdDatabase   = os.Getenv("FIREBIRD_DATABASE")
	FirebirdHost       = os.Getenv("FIREBIRD_HOST")
	FirebirdPort       = os.Getenv("FIREBIRD_PORT")
	FirebirdUser       = os.Getenv("FIREBIRD_USER")
	FirebirdPass       = os.Getenv("FIREBIRD_PASS")
)

func getFirebirdVars(t *testing.T) map[string]any {
	switch "" {
	case FirebirdDatabase:
		t.Fatal("'FIREBIRD_DATABASE' not set")
	case FirebirdHost:
		t.Fatal("'FIREBIRD_HOST' not set")
	case FirebirdPort:
		t.Fatal("'FIREBIRD_PORT' not set")
	case FirebirdUser:
		t.Fatal("'FIREBIRD_USER' not set")
	case FirebirdPass:
		t.Fatal("'FIREBIRD_PASS' not set")
	}

	return map[string]any{
		"type":     FirebirdSourceType,
		"host":     FirebirdHost,
		"port":     FirebirdPort,
		"database": FirebirdDatabase,
		"user":     FirebirdUser,
		"password": FirebirdPass,
	}
}

func initFirebirdConnection(host, port, user, pass, dbname string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@%s:%s/%s", user, pass, host, port, dbname)
	db, err := sql.Open("firebirdsql", dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Configure connection pool to prevent deadlocks
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	return db, nil
}

func TestFirebirdToolEndpoints(t *testing.T) {
	tableName := setupFirebirdTest(t, "--enable-api")
	tests.RunToolGetTest(t)
	runFirebirdCallTests(t, tableName, nil, nil, nil)
}

func setupFirebirdTest(t *testing.T, args ...string) string {
	t.Helper()
	sourceConfig := getFirebirdVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)

	db, err := initFirebirdConnection(FirebirdHost, FirebirdPort, FirebirdUser, FirebirdPass, FirebirdDatabase)
	if err != nil {
		t.Fatalf("unable to create firebird connection pool: %s", err)
	}
	t.Cleanup(func() { db.Close() })

	shortUUID := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	tableNameParam := fmt.Sprintf("param_table_%s", shortUUID)
	tableNameAuth := fmt.Sprintf("auth_table_%s", shortUUID)
	tableNameTemplateParam := fmt.Sprintf("template_param_table_%s", shortUUID)

	createParamTableStmts, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getFirebirdParamToolInfo(tableNameParam)
	teardownTable1 := setupFirebirdTable(t, ctx, db, createParamTableStmts, insertParamTableStmt, tableNameParam, paramTestParams)
	t.Cleanup(func() { teardownTable1(t) })

	createAuthTableStmts, insertAuthTableStmt, authToolStmt, authTestParams := getFirebirdAuthToolInfo(tableNameAuth)
	teardownTable2 := setupFirebirdTable(t, ctx, db, createAuthTableStmts, insertAuthTableStmt, tableNameAuth, authTestParams)
	t.Cleanup(func() { teardownTable2(t) })

	toolsFile := getFirebirdToolsConfig(sourceConfig, FirebirdToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addFirebirdExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := getFirebirdTmplToolStatement()
	toolsFile = addFirebirdTemplateParamConfig(t, toolsFile, FirebirdToolType, tmplSelectCombined, tmplSelectFilterCombined)

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

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	return tableNameTemplateParam
}

func runFirebirdCallTests(t *testing.T, tableName string, invokeOptions []tests.InvokeTestOption, sqlOptions []tests.ExecuteSqlOption, templateOptions []tests.TemplateParamOption) {
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := getFirebirdWants()
	t.Run("invoke", func(t *testing.T) {
		opts := []tests.InvokeTestOption{tests.WithNullWant(`[{"id":4,"name":null}]`), tests.DisableArrayTest()}
		tests.RunToolInvokeTest(t, select1Want, append(opts, invokeOptions...)...)
	})
	t.Run("mcp_call", func(t *testing.T) {
		tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	})
	t.Run("execute_sql", func(t *testing.T) {
		opts := []tests.ExecuteSqlOption{tests.WithSelect1Statement(`SELECT 1 AS "constant" FROM RDB$DATABASE;`)}
		tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, append(opts, sqlOptions...)...)
	})
	t.Run("template_parameters", func(t *testing.T) {
		opts := []tests.TemplateParamOption{tests.WithCreateColArray(`["id INTEGER","name VARCHAR(255)","age INTEGER"]`)}
		tests.RunToolInvokeWithTemplateParameters(t, tableName, append(opts, templateOptions...)...)
	})
}

func setupFirebirdTable(t *testing.T, ctx context.Context, db *sql.DB, createStatements []string, insertStatement, tableName string, params []any) func(*testing.T) {
	err := db.PingContext(ctx)
	if err != nil {
		t.Fatalf("unable to connect to test database: %s", err)
	}

	for _, stmt := range createStatements {
		_, err = db.ExecContext(ctx, stmt)
		if err != nil {
			t.Fatalf("unable to execute create statement for table %s: %s\nStatement: %s", tableName, err, stmt)
		}
	}

	if insertStatement != "" && len(params) > 0 {
		stmt, err := db.PrepareContext(ctx, insertStatement)
		if err != nil {
			t.Fatalf("unable to prepare insert statement: %v", err)
		}
		defer stmt.Close()

		numPlaceholders := strings.Count(insertStatement, "?")
		if numPlaceholders == 0 {
			t.Fatalf("insert statement has no placeholders '?' but params were provided")
		}
		for i := 0; i < len(params); i += numPlaceholders {
			end := i + numPlaceholders
			if end > len(params) {
				end = len(params)
			}
			batchParams := params[i:end]

			_, err = stmt.ExecContext(ctx, batchParams...)
			if err != nil {
				t.Fatalf("unable to insert test data row with params %v: %v", batchParams, err)
			}
		}
	}

	return func(t *testing.T) {
		// Close the main connection to free up resources
		db.Close()

		// Helper function to check if error indicates object doesn't exist
		isNotFoundError := func(err error) bool {
			if err == nil {
				return false
			}
			errMsg := strings.ToLower(err.Error())
			return strings.Contains(errMsg, "does not exist") ||
				strings.Contains(errMsg, "not found") ||
				strings.Contains(errMsg, "is not defined") ||
				strings.Contains(errMsg, "unknown") ||
				strings.Contains(errMsg, "invalid")
		}

		// Create dedicated cleanup connection with minimal configuration
		createCleanupConnection := func() (*sql.DB, error) {
			dsn := fmt.Sprintf("%s:%s@%s:%s/%s", FirebirdUser, FirebirdPass, FirebirdHost, FirebirdPort, FirebirdDatabase)

			cleanupDb, err := sql.Open("firebirdsql", dsn)
			if err != nil {
				return nil, err
			}

			// Ultra minimal connection pool for cleanup only
			cleanupDb.SetMaxOpenConns(1)
			cleanupDb.SetMaxIdleConns(0)
			cleanupDb.SetConnMaxLifetime(5 * time.Second)
			cleanupDb.SetConnMaxIdleTime(1 * time.Second)

			return cleanupDb, nil
		}

		// Drop each object with its own dedicated connection and aggressive timeout
		dropObjects := []struct {
			objType string
			query   string
		}{
			{"trigger", fmt.Sprintf("DROP TRIGGER BI_%s_ID", tableName)},
			{"table", fmt.Sprintf("DROP TABLE %s", tableName)},
			{"generator", fmt.Sprintf("DROP GENERATOR GEN_%s_ID", tableName)},
		}

		for _, obj := range dropObjects {
			cleanupDb, err := createCleanupConnection()
			if err != nil {
				t.Logf("Failed to create cleanup connection for %s: %s", obj.objType, err)
				continue
			}

			// Use aggressive short timeout for each operation
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, dropErr := cleanupDb.ExecContext(ctx, obj.query)
			cancel()
			cleanupDb.Close()

			if dropErr == nil {
				t.Logf("Successfully dropped %s", obj.objType)
			} else if isNotFoundError(dropErr) {
				t.Logf("%s does not exist, skipping", obj.objType)
			} else if ctx.Err() == context.DeadlineExceeded {
				t.Logf("Timeout dropping %s (3s limit exceeded) - continuing anyway", obj.objType)
			} else {
				t.Logf("Failed to drop %s: %s - continuing anyway", obj.objType, dropErr)
			}

			// Small delay between operations to reduce contention
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func getFirebirdParamToolInfo(tableName string) ([]string, string, string, string, string, string, []any) {
	createStatements := []string{
		fmt.Sprintf("CREATE TABLE %s (id INTEGER NOT NULL PRIMARY KEY, name VARCHAR(255));", tableName),
		fmt.Sprintf("CREATE GENERATOR GEN_%s_ID;", tableName),
		fmt.Sprintf(`
			CREATE TRIGGER BI_%s_ID FOR %s
			ACTIVE BEFORE INSERT POSITION 0
			AS
			BEGIN
				IF (NEW.id IS NULL) THEN
					NEW.id = GEN_ID(GEN_%s_ID, 1);
			END;
		`, tableName, tableName, tableName),
	}

	insertStatement := fmt.Sprintf("INSERT INTO %s (name) VALUES (?);", tableName)
	toolStatement := fmt.Sprintf("SELECT id AS \"id\", name AS \"name\" FROM %s WHERE id = ? OR name = ?;", tableName)
	idParamStatement := fmt.Sprintf("SELECT id AS \"id\", name AS \"name\" FROM %s WHERE id = ?;", tableName)
	nameParamStatement := fmt.Sprintf("SELECT id AS \"id\", name AS \"name\" FROM %s WHERE name IS NOT DISTINCT FROM ?;", tableName)
	// Firebird doesn't support array parameters in IN clause the same way as other databases
	// We'll use a simpler approach for testing
	arrayToolStatement := fmt.Sprintf("SELECT id AS \"id\", name AS \"name\" FROM %s WHERE id = ? ORDER BY id;", tableName)

	params := []any{"Alice", "Jane", "Sid", nil}
	return createStatements, insertStatement, toolStatement, idParamStatement, nameParamStatement, arrayToolStatement, params
}

func getFirebirdAuthToolInfo(tableName string) ([]string, string, string, []any) {
	createStatements := []string{
		fmt.Sprintf("CREATE TABLE %s (id INTEGER NOT NULL PRIMARY KEY, name VARCHAR(255), email VARCHAR(255));", tableName),
		fmt.Sprintf("CREATE GENERATOR GEN_%s_ID;", tableName),
		fmt.Sprintf(`
			CREATE TRIGGER BI_%s_ID FOR %s
			ACTIVE BEFORE INSERT POSITION 0
			AS
			BEGIN
				IF (NEW.id IS NULL) THEN
					NEW.id = GEN_ID(GEN_%s_ID, 1);
			END;
		`, tableName, tableName, tableName),
	}

	insertStatement := fmt.Sprintf("INSERT INTO %s (name, email) VALUES (?, ?)", tableName)
	toolStatement := fmt.Sprintf("SELECT name AS \"name\" FROM %s WHERE email = ?;", tableName)
	params := []any{"Alice", tests.ServiceAccountEmail, "Jane", "janedoe@gmail.com"}
	return createStatements, insertStatement, toolStatement, params
}

func getFirebirdWants() (string, string, string, string) {
	select1Want := `[{"constant":1}]`
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: Dynamic SQL Error\nSQL error code = -104\nToken unknown - line 1, column 1\nSELEC\n"}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id INTEGER PRIMARY KEY, name VARCHAR(50))"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"constant\":1}"}]}}`
	return select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want
}

func getFirebirdToolsConfig(sourceConfig map[string]any, toolType, paramToolStatement, idParamToolStmt, nameParamToolStmt, arrayToolStatement, authToolStatement string) map[string]any {
	toolsFile := tests.GetToolsConfig(sourceConfig, toolType, paramToolStatement, idParamToolStmt, nameParamToolStmt, arrayToolStatement, authToolStatement)

	toolsMap, ok := toolsFile["tools"].(map[string]any)
	if !ok {
		return toolsFile
	}

	if simpleTool, ok := toolsMap["my-simple-tool"].(map[string]any); ok {
		simpleTool["statement"] = "SELECT 1 AS \"constant\" FROM RDB$DATABASE;"
		toolsMap["my-simple-tool"] = simpleTool
	}
	if authRequiredTool, ok := toolsMap["my-auth-required-tool"].(map[string]any); ok {
		authRequiredTool["statement"] = "SELECT 1 AS \"constant\" FROM RDB$DATABASE;"
		toolsMap["my-auth-required-tool"] = authRequiredTool
	}

	if arrayTool, ok := toolsMap["my-array-tool"].(map[string]any); ok {
		// Firebird array tool - accept array but use only first element for compatibility
		arrayTool["parameters"] = []any{
			map[string]any{
				"name":        "idArray",
				"type":        "array",
				"description": "ID array (Firebird will use first element only)",
				"items": map[string]any{
					"name":        "id",
					"type":        "integer",
					"description": "ID",
				},
			},
		}
		// Statement is already defined in arrayToolStatement parameter
		toolsMap["my-array-tool"] = arrayTool
	}

	toolsFile["tools"] = toolsMap
	return toolsFile
}

func addFirebirdTemplateParamConfig(t *testing.T, config map[string]any, toolType, tmplSelectCombined, tmplSelectFilterCombined string) map[string]any {
	toolsMap, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}

	// Firebird-specific template parameter tools with compatible syntax
	toolsMap["create-table-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "CREATE TABLE {{.tableName}} ({{array .columns}})",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewArrayParameter("columns", "The columns to create", parameters.NewStringParameter("column", "A column name that will be created")),
		},
	}
	toolsMap["insert-table-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Insert table tool with template parameters",
		"statement":   "INSERT INTO {{.tableName}} ({{array .columns}}) VALUES ({{.values}})",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewArrayParameter("columns", "The columns to insert into", parameters.NewStringParameter("column", "A column name that will be returned from the query.")),
			parameters.NewStringParameter("values", "The values to insert as a comma separated string"),
		},
	}
	toolsMap["select-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Select table tool with template parameters",
		"statement":   "SELECT id AS \"id\", name AS \"name\", age AS \"age\" FROM {{.tableName}}",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-templateParams-combined-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Select table tool with combined template parameters",
		"statement":   tmplSelectCombined,
		"parameters": []parameters.Parameter{
			parameters.NewIntParameter("id", "the id of the user"),
		},
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-fields-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Select specific fields tool with template parameters",
		"statement":   "SELECT name AS \"name\" FROM {{.tableName}}",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-filter-templateParams-combined-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Select table tool with filter template parameters",
		"statement":   tmplSelectFilterCombined,
		"parameters": []parameters.Parameter{
			parameters.NewStringParameter("name", "the name to filter by"),
		},
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewStringParameter("columnFilter", "some description"),
		},
	}
	// Firebird uses simple DROP TABLE syntax without IF EXISTS
	toolsMap["drop-table-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Drop table tool with template parameters",
		"statement":   "DROP TABLE {{.tableName}}",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	config["tools"] = toolsMap
	return config
}

func addFirebirdExecuteSqlConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "firebird-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "firebird-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	config["tools"] = tools
	return config
}

func getFirebirdTmplToolStatement() (string, string) {
	tmplSelectCombined := "SELECT id AS \"id\", name AS \"name\", age AS \"age\" FROM {{.tableName}} WHERE id = ?"
	tmplSelectFilterCombined := "SELECT id AS \"id\", name AS \"name\", age AS \"age\" FROM {{.tableName}} WHERE {{.columnFilter}} = ?"
	return tmplSelectCombined, tmplSelectFilterCombined
}
