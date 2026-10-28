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
	cockroachdbsource "github.com/googleapis/mcp-toolbox/internal/sources/cockroachdb"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tccockroachdb "github.com/testcontainers/testcontainers-go/modules/cockroachdb"
)

func TestCockroachDBQueryLimits(t *testing.T) {
	source := &cockroachdbsource.Source{Config: cockroachdbsource.Config{MaxRowLimit: 100}}
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{"existing limit", "SELECT *\nFROM users\nLIMIT 50", "SELECT *\nFROM users\nLIMIT 50"},
		{"limit in line comment", "SELECT * FROM users -- LIMIT is not here", "SELECT * FROM users -- LIMIT is not here\nLIMIT 100"},
		{"limit in nested block comment", "SELECT * FROM users /* outer /* LIMIT */ comment */", "SELECT * FROM users /* outer /* LIMIT */ comment */ LIMIT 100"},
		{"limit in quoted text", `SELECT 'LIMIT', "LIMIT", $$LIMIT$$ FROM users`, `SELECT 'LIMIT', "LIMIT", $$LIMIT$$ FROM users LIMIT 100`},
		{"multiline semicolon", "\n\tSELECT *\n\tFROM users\n\tORDER BY id;\n", "SELECT *\n\tFROM users\n\tORDER BY id LIMIT 100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := source.ApplyQueryLimits(tt.sql)
			if err != nil {
				t.Fatalf("ApplyQueryLimits returned an error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ApplyQueryLimits() = %q, want %q", got, tt.want)
			}
		})
	}
}

var (
	CockroachDBSourceType = "cockroachdb"
	CockroachDBToolType   = "cockroachdb-sql"
	CockroachDBDatabase   = "defaultdb"
)

func getCockroachDBVars(host, port string) map[string]any {
	return map[string]any{
		"type":           CockroachDBSourceType,
		"host":           host,
		"port":           port,
		"database":       CockroachDBDatabase,
		"user":           "root",
		"password":       "",
		"maxRetries":     5,
		"retryBaseDelay": "500ms",
		"queryParams": map[string]string{
			"sslmode": "disable",
		},
	}
}

func initCockroachDBConnectionPool(connString string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	return pool, nil
}

func TestCockroachDB(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

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
	args := []string{"--enable-api"}

	pool, err := initCockroachDBConnectionPool(connString)
	if err != nil {
		t.Fatalf("unable to create cockroachdb connection pool: %s", err)
	}
	// Note: Don't defer pool.Close() here - the pool is only used for test setup/teardown.
	// Closing it explicitly can cause hangs if the server's pool is still active.
	// The pool will be cleaned up when the test exits.

	// Verify CockroachDB version
	var version string
	err = pool.QueryRow(ctx, "SELECT version()").Scan(&version)
	if err != nil {
		t.Fatalf("failed to query version: %s", err)
	}
	if !strings.Contains(version, "CockroachDB") {
		t.Fatalf("not connected to CockroachDB, got: %s", version)
	}
	t.Logf("✅ Connected to: %s", version)

	// Generate a unique ID
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")

	//Create table names using the UUID
	tableNameParam := "param_table_" + uniqueID
	tableNameAuth := "auth_table_" + uniqueID
	tableNameTemplateParam := "template_param_table_" + uniqueID

	// set up data for param tool (using CockroachDB explicit INT primary keys)
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := tests.GetCockroachDBParamToolInfo(tableNameParam)
	teardownTable1 := tests.SetupPostgresSQLTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := tests.GetCockroachDBAuthToolInfo(tableNameAuth)
	teardownTable2 := tests.SetupPostgresSQLTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, CockroachDBToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)

	// Add execute-sql tool with write-enabled source (CockroachDB MCP security requires explicit opt-in)
	toolsFile = addCockroachDBExecuteSqlConfig(t, toolsFile, sourceConfig)

	tmplSelectCombined, tmplSelectFilterCombined := tests.GetPostgresSQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, CockroachDBToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

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

	// Get configs for tests (use CockroachDB-specific expectations)
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := tests.GetCockroachDBWants()

	// Run required integration test suites (per CONTRIBUTING.md)
	t.Run("ToolGetTest", func(t *testing.T) {
		tests.RunToolGetTest(t)
	})

	t.Run("ToolInvokeTest", func(t *testing.T) {
		tests.RunToolInvokeTest(t, select1Want)
	})

	t.Run("MCPToolCallMethod", func(t *testing.T) {
		tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	})

	t.Run("ExecuteSqlToolInvokeTest", func(t *testing.T) {
		tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	})

	t.Run("ToolInvokeWithTemplateParameters", func(t *testing.T) {
		tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)
	})

	t.Logf("✅✅✅ All CockroachDB integration tests passed!")
}

// addCockroachDBExecuteSqlConfig adds execute-sql tool with write-enabled source
// CockroachDB has MCP security enabled by default, so execute-sql needs a separate source with enableWriteMode
func addCockroachDBExecuteSqlConfig(t *testing.T, config map[string]any, baseSourceConfig map[string]any) map[string]any {
	// Add write-enabled source for execute-sql tool
	sources, ok := config["sources"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get sources from config")
	}

	// Create a copy of the base source config with write mode enabled
	writeEnabledSource := make(map[string]any)
	for k, v := range baseSourceConfig {
		writeEnabledSource[k] = v
	}
	writeEnabledSource["enableWriteMode"] = true
	writeEnabledSource["readOnlyMode"] = false

	sources["my-write-instance"] = writeEnabledSource

	// Add tools using the write-enabled source
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}

	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "cockroachdb-execute-sql",
		"source":      "my-write-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "cockroachdb-execute-sql",
		"source":      "my-write-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}

	return config
}
