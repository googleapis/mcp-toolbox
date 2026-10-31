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

package tidb

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
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	TiDBSourceType = "tidb"
	TiDBToolType   = "tidb-sql"
	TiDBDatabase   = os.Getenv("TIDB_DATABASE")
	TiDBHost       = os.Getenv("TIDB_HOST")
	TiDBPort       = os.Getenv("TIDB_PORT")
	TiDBUser       = os.Getenv("TIDB_USER")
	TiDBPass       = os.Getenv("TIDB_PASS")
)

const (
	// tidbImage is a standalone tidb-server backed by embedded unistore
	// storage. It doesn't need PD/TiKV and doesn't ship Grafana or other
	// monitoring components.
	tidbImage = "pingcap/tidb:v8.5.8"
	// tidbContainerDatabase and tidbContainerUser are created inside the
	// ephemeral container when no external TiDB instance is configured.
	tidbContainerDatabase = "test_database"
	tidbContainerUser     = "toolbox"
)

// setupTiDBContainer starts an ephemeral TiDB container and provisions a
// database plus a non-root user with a random per-run password (the tidb
// source requires a non-empty password). It returns the connection details
// and a cleanup function that terminates the container.
func setupTiDBContainer(ctx context.Context, t *testing.T) (host, port, user, pass, database string, cleanup func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        tidbImage,
		ExposedPorts: []string{"4000/tcp", "10080/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("4000/tcp"),
			wait.ForHTTP("/status").WithPort("10080/tcp"),
		).WithDeadline(3 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start TiDB container: %s", err)
	}

	cleanup = func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanupCancel()
		if err := container.Terminate(cleanupCtx); err != nil {
			t.Logf("failed to terminate TiDB container: %s", err)
		}
	}

	host, err = container.Host(ctx)
	if err != nil {
		cleanup()
		t.Fatalf("failed to get TiDB container host: %s", err)
	}

	mappedPort, err := container.MappedPort(ctx, "4000/tcp")
	if err != nil {
		cleanup()
		t.Fatalf("failed to get TiDB container mapped port: %s", err)
	}
	port = mappedPort.Port()

	// The image's root user has an empty password; use it only to bootstrap
	// a dedicated user with a random password.
	rootPool, err := initTiDBConnectionPool(host, port, "root", "", "", false)
	if err != nil {
		cleanup()
		t.Fatalf("unable to create TiDB root connection pool: %s", err)
	}
	defer rootPool.Close()

	user = tidbContainerUser
	pass = strings.ReplaceAll(uuid.New().String(), "-", "")
	database = tidbContainerDatabase
	bootstrapStmts := []string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", database),
		fmt.Sprintf("CREATE USER '%s'@'%%' IDENTIFIED BY '%s'", user, pass),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'", database, user),
	}
	for _, stmt := range bootstrapStmts {
		if _, err := rootPool.ExecContext(ctx, stmt); err != nil {
			cleanup()
			t.Fatalf("failed to bootstrap TiDB container: %s", err)
		}
	}

	return host, port, user, pass, database, cleanup
}

func getTiDBVars(t *testing.T) map[string]any {
	switch "" {
	case TiDBDatabase:
		t.Fatal("'TIDB_DATABASE' not set")
	case TiDBHost:
		t.Fatal("'TIDB_HOST' not set")
	case TiDBPort:
		t.Fatal("'TIDB_PORT' not set")
	case TiDBUser:
		t.Fatal("'TIDB_USER' not set")
	case TiDBPass:
		t.Fatal("'TIDB_PASS' not set")
	}

	return map[string]any{
		"type":     TiDBSourceType,
		"host":     TiDBHost,
		"port":     TiDBPort,
		"database": TiDBDatabase,
		"user":     TiDBUser,
		"password": TiDBPass,
	}
}

// Copied over from tidb.go
func initTiDBConnectionPool(host, port, user, pass, dbname string, useSSL bool) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&tls=%t", user, pass, host, port, dbname, useSSL)

	// Interact with the driver directly as you normally would
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return pool, nil
}

// getTiDBWants return the expected wants for tidb
func getTiDBWants() (string, string, string, string) {
	select1Want := "[{\"1\":1}]"
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: Error 1064 (42000): You have an error in your SQL syntax; check the manual that corresponds to your TiDB version for the right syntax to use line 1 column 5 near \"SELEC 1;\" "}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id SERIAL PRIMARY KEY, name TEXT)"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"1\":1}"}]}}`
	return select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want
}

// addTiDBExecuteSqlConfig gets the tools config for `tidb-execute-sql`
func addTiDBExecuteSqlConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "tidb-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "tidb-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	config["tools"] = tools
	return config
}

func TestTiDBToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	// Use an external TiDB instance when TIDB_HOST is set; otherwise start an
	// ephemeral container.
	if TiDBHost == "" {
		host, port, user, pass, database, containerCleanup := setupTiDBContainer(ctx, t)
		t.Cleanup(containerCleanup)
		TiDBHost, TiDBPort, TiDBUser, TiDBPass, TiDBDatabase = host, port, user, pass, database
	}

	sourceConfig := getTiDBVars(t)

	args := []string{"--enable-api"}

	pool, err := initTiDBConnectionPool(TiDBHost, TiDBPort, TiDBUser, TiDBPass, TiDBDatabase, false)
	if err != nil {
		t.Fatalf("unable to create TiDB connection pool: %s", err)
	}

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
	toolsFile := tests.GetToolsConfig(sourceConfig, TiDBToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addTiDBExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMySQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, TiDBToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

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
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := getTiDBWants()

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)
}
