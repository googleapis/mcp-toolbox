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

package singlestore

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

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	singlestoresrc "github.com/googleapis/mcp-toolbox/internal/sources/singlestore"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	SingleStoreSourceType = "singlestore"
	SingleStoreToolType   = "singlestore-sql"
	SingleStoreDatabase   = os.Getenv("SINGLESTORE_DATABASE")
	SingleStoreHost       = os.Getenv("SINGLESTORE_HOST")
	SingleStorePort       = os.Getenv("SINGLESTORE_PORT")
	SingleStoreUser       = os.Getenv("SINGLESTORE_USER")
	SingleStorePass       = os.Getenv("SINGLESTORE_PASSWORD")
)

func getSingleStoreVars(t *testing.T) map[string]any {
	switch "" {
	case SingleStoreDatabase:
		t.Fatal("'SINGLESTORE_DATABASE' not set")
	case SingleStoreHost:
		t.Fatal("'SINGLESTORE_HOST' not set")
	case SingleStorePort:
		t.Fatal("'SINGLESTORE_PORT' not set")
	case SingleStoreUser:
		t.Fatal("'SINGLESTORE_USER' not set")
	case SingleStorePass:
		t.Fatal("'SINGLESTORE_PASSWORD' not set")
	}

	return map[string]any{
		"type":     SingleStoreSourceType,
		"host":     SingleStoreHost,
		"port":     SingleStorePort,
		"database": SingleStoreDatabase,
		"user":     SingleStoreUser,
		"password": SingleStorePass,
	}
}

// getSingleStoreParamToolInfo returns statements and params for my-tool
func getSingleStoreParamToolInfo(tableName string) (string, string, string, string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id BIGINT NOT NULL PRIMARY KEY, name VARCHAR(255));", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (id, name) VALUES (?, ?), (?, ?), (?, ?), (?, ?);", tableName)
	toolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ? OR name = ? ORDER BY id;", tableName)
	idParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ? ORDER BY id;", tableName)
	nameParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = ? ORDER BY id;", tableName)
	// SingleStore doesn't support array parameters in IN clause unlike some other databases
	arrayToolStmt := ""
	insertParams := []any{1, "Alice", 2, "Jane", 3, "Sid", 4, nil}
	return createStatement, insertStatement, toolStatement, idParamStatement, nameParamStatement, arrayToolStmt, insertParams
}

// getSingleStoreAuthToolInfo returns statements and param of my-auth-tool
func getSingleStoreAuthToolInfo(tableName string) (string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, name VARCHAR(255), email VARCHAR(255));", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name, email) VALUES (?, ?), (?, ?)", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = ?;", tableName)
	params := []any{"Alice", tests.ServiceAccountEmail, "Jane", "janedoe@gmail.com"}
	return createStatement, insertStatement, toolStatement, params
}

// getSingleStoreTmplToolStatement returns statements and param for template parameter test cases for singlestore-sql type
func getSingleStoreTmplToolStatement() (string, string) {
	tmplSelectCombined := "SELECT * FROM {{.tableName}} WHERE id = ?"
	tmplSelectFilterCombined := "SELECT * FROM {{.tableName}} WHERE {{.columnFilter}} = ?"
	return tmplSelectCombined, tmplSelectFilterCombined
}

// getSingleStoreWants return the expected wants for singlestore
func getSingleStoreWants() (string, string, string, string) {
	select1Want := "[{\"1\":1}]"
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: Error 1064 (42000): You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near 'SELEC 1' at line 1"}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id BIGINT PRIMARY KEY, name TEXT)"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"1\":1}"}]}}`
	return select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want
}

// setupSingleStoreTable creates and inserts data into a table of tool
// compatible with singlestore-sql tool. The table is dropped on cleanup; the
// cleanup is registered first so a partially failed setup is still cleaned up.
func setupSingleStoreTable(t *testing.T, ctx context.Context, pool *sql.DB, createStatement, insertStatement, tableName string, params []any) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := pool.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("DROP TABLE IF EXISTS %s;", tableName)); err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	})

	err := pool.PingContext(ctx)
	if err != nil {
		t.Fatalf("unable to connect to test database: %s", err)
	}

	// Create table
	_, err = pool.ExecContext(ctx, createStatement)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	_, err = pool.ExecContext(ctx, insertStatement, params...)
	if err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}
}

func getSingleStoreToolsConfig(sourceConfig map[string]any, toolType, paramToolStatement, idParamToolStmt, nameParamToolStmt, arrayToolStatement, authToolStatement string) map[string]any {
	toolsFile := tests.GetToolsConfig(sourceConfig, toolType, paramToolStatement, idParamToolStmt, nameParamToolStmt, arrayToolStatement, authToolStatement)

	toolsMap, ok := toolsFile["tools"].(map[string]any)
	if !ok {
		return toolsFile
	}
	// Remove tools that are not supported
	delete(toolsMap, "my-array-tool")

	toolsFile["tools"] = toolsMap
	return toolsFile
}

// addSingleStoreExecuteSQLConfig gets the tools config for `singlestore-execute-sql`
func addSingleStoreExecuteSQLConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "singlestore-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "singlestore-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	config["tools"] = tools
	return config
}

// Copied over from singlestore.go, with context and tracer removed
func initSingleStoreConnectionPool(cfg singlestoresrc.Config) (*sql.DB, error) {
	// Build query parameters via url.Values for deterministic order and proper escaping.
	connectionParams := url.Values{}

	mysqlCfg := mysql.Config{
		User:                 cfg.User,
		Passwd:               cfg.Password,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		DBName:               cfg.Database,
		ParseTime:            true,
		AllowNativePasswords: true,
		CheckConnLiveness:    true,
		MaxAllowedPacket:     64 << 20,
		ConnectionAttributes: "_connector_name:MCP toolbox for Databases",
		Params: map[string]string{
			"vector_type_project_format": "JSON",
		},
	}

	// Default to TLS preferred; can be overridden via connectionParams.
	connectionParams.Set("tls", "preferred")

	// Derive readTimeout from queryTimeout when provided.
	if cfg.QueryTimeout != "" {
		timeout, err := time.ParseDuration(cfg.QueryTimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid queryTimeout %q: %w", cfg.QueryTimeout, err)
		}
		connectionParams.Set("readTimeout", timeout.String())
	}

	// Custom user parameters (e.g. tls, compress) — may override defaults above.
	for k, v := range cfg.ConnectionParams {
		if v == "" {
			continue // skip empty values
		}
		connectionParams.Set(k, v)
	}
	dsn := mysqlCfg.FormatDSN()
	if enc := connectionParams.Encode(); enc != "" {
		dsn += "&" + enc
	}

	// Interact with the driver directly as you normally would
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return pool, nil
}

// setupSingleStoreTest opens a connection pool, seeds the tables used by the
// shared tool fixtures and returns the template parameter table name, the pool
// and the tools file. The pool is closed and the tables dropped on cleanup.
func setupSingleStoreTest(t *testing.T, ctx context.Context) (string, *sql.DB, map[string]any) {
	t.Helper()
	sourceConfig := getSingleStoreVars(t)

	cfg := singlestoresrc.Config{
		Host:     SingleStoreHost,
		Port:     SingleStorePort,
		User:     SingleStoreUser,
		Password: SingleStorePass,
		Database: SingleStoreDatabase,
	}
	pool, err := initSingleStoreConnectionPool(cfg)
	if err != nil {
		t.Fatalf("unable to create SingleStore connection pool: %s", err)
	}
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Errorf("unable to close SingleStore connection pool: %s", err)
		}
	})

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getSingleStoreParamToolInfo(tableNameParam)
	setupSingleStoreTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getSingleStoreAuthToolInfo(tableNameAuth)
	setupSingleStoreTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)

	// Write config into a file and pass it to command
	toolsFile := getSingleStoreToolsConfig(sourceConfig, SingleStoreToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addSingleStoreExecuteSQLConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := getSingleStoreTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, SingleStoreToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

	return tableNameTemplateParam, pool, toolsFile
}

func TestSingleStoreToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	tableNameTemplateParam, pool, toolsFile := setupSingleStoreTest(t, ctx)

	insertStmt := `INSERT INTO senseai_docs (content, embedding) VALUES (?, JSON_ARRAY_PACK(?))`
	searchStmt := `SELECT content FROM senseai_docs ORDER BY DOT_PRODUCT(embedding, JSON_ARRAY_PACK(?)) DESC LIMIT 1`
	toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, SingleStoreToolType, insertStmt, searchStmt)
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Get configs for tests
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := getSingleStoreWants()

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)

	// Create table for semantic search. The drop is registered first so a
	// partially failed setup is still cleaned up.
	t.Cleanup(func() {
		if _, err := pool.ExecContext(context.WithoutCancel(ctx), "DROP TABLE IF EXISTS senseai_docs;"); err != nil {
			t.Logf("Teardown failed: %s", err)
		}
	})
	_, err = pool.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS senseai_docs (id INT AUTO_INCREMENT PRIMARY KEY, content TEXT, embedding BLOB);")
	if err != nil {
		t.Fatalf("unable to create semantic search table: %s", err)
	}

	// Semantic search tests
	httpSemanticInsertWant := `[]`
	mcpSemanticInsertWant := ``
	semanticSearchWant := `The quick brown fox jumps over the lazy dog`
	tests.RunSemanticSearchToolInvokeTest(t, httpSemanticInsertWant, mcpSemanticInsertWant, semanticSearchWant)
}
