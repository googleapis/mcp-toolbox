// Copyright 2024 Google LLC
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
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	pgsource "github.com/googleapis/mcp-toolbox/internal/sources/postgres"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel/trace/noop"
)

var (
	PostgresSourceType = "postgres"
	PostgresToolType   = "postgres-sql"
	PostgresDatabase   = os.Getenv("POSTGRES_DATABASE")
	PostgresUser       = os.Getenv("POSTGRES_USER")
	PostgresPass       = os.Getenv("POSTGRES_PASS")
)

func getPostgresVars(t *testing.T, host string, port string) map[string]any {
	switch "" {
	case host:
		t.Fatal("'PostgresHost' was empty")
	case port:
		t.Fatal("'PostgressPort' was empty")
	case PostgresDatabase:
		t.Fatal("'POSTGRES_DATABASE' not set")
	case PostgresUser:
		t.Fatal("'POSTGRES_USER' not set")
	case PostgresPass:
		t.Fatal("'POSTGRES_PASS' not set")
	}

	return map[string]any{
		"type":     PostgresSourceType,
		"host":     host,
		"port":     port,
		"database": PostgresDatabase,
		"user":     PostgresUser,
		"password": PostgresPass,
	}
}

// Copied over from postgres.go
func initPostgresConnectionPool(host, port, user, pass, dbname string) (*pgxpool.Pool, error) {
	// urlExample := "postgres:dd//username:password@localhost:5432/database_name"
	url := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   dbname,
	}
	pool, err := pgxpool.New(context.Background(), url.String())
	if err != nil {
		return nil, fmt.Errorf("Unable to create connection pool: %w", err)
	}

	return pool, nil
}

func setupPostgresTestContainer(ctx context.Context, t *testing.T) (string, string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "pgvector/pgvector:pg16",
		Cmd:          []string{"postgres", "-c", "shared_preload_libraries=pg_stat_statements"},
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     PostgresUser,
			"POSTGRES_PASSWORD": PostgresPass,
			"POSTGRES_DB":       PostgresDatabase,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForExposedPort(),
		),
	}

	return tests.SetupGenericPostgresTestContainer(ctx, t, req)
}

func TestPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	PostgresHost, PostgresPort, containerCleanup := setupPostgresTestContainer(ctx, t)
	t.Cleanup(containerCleanup)

	sourceConfig := getPostgresVars(t, PostgresHost, PostgresPort)

	args := []string{"--enable-api"}

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

	//Create table names using the UUID
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

	// Set up table for semantic search
	vectorTableName, tearDownVectorTable := tests.SetupPostgresVectorTable(t, ctx, pool)
	defer tearDownVectorTable(t)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, PostgresToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddExecuteSqlConfig(t, toolsFile, "postgres-execute-sql")
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetPostgresSQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, PostgresToolType, tmplSelectCombined, tmplSelectFilterCombined, "")
	toolsFile = tests.AddPostgresPrebuiltConfig(t, toolsFile)

	// Add semantic search tool config
	insertStmt, searchStmt := tests.GetPostgresVectorSearchStmts(vectorTableName)
	toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, PostgresToolType, insertStmt, searchStmt)

	// Add read-only test sources and tools to the shared Toolbox config
	toolsFile = addPostgresReadOnlyConfig(t, ctx, pool, toolsFile, PostgresHost, PostgresPort, uniqueID)

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
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := tests.GetPostgresWants()

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want)
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)

	// Run Postgres prebuilt tool tests
	tests.RunPostgresListTablesTest(t, tableNameParam, tableNameAuth, PostgresUser)
	tests.RunPostgresListViewsTest(t, ctx, pool)
	tests.RunPostgresListSchemasTest(t, ctx, pool, PostgresUser, uniqueID)
	tests.RunPostgresListActiveQueriesTest(t, ctx, pool)
	tests.RunPostgresListAvailableExtensionsTest(t)
	tests.RunPostgresListInstalledExtensionsTest(t)
	tests.RunPostgresDatabaseOverviewTest(t, ctx, pool)
	tests.RunPostgresListTriggersTest(t, ctx, pool)
	tests.RunPostgresListIndexesTest(t, ctx, pool)
	tests.RunPostgresListSequencesTest(t, ctx, pool)
	tests.RunPostgresLongRunningTransactionsTest(t, ctx, pool)
	tests.RunPostgresListLocksTest(t, ctx, pool)
	tests.RunPostgresReplicationStatsTest(t, ctx, pool)
	tests.RunPostgresListQueryStatsTest(t, ctx, pool)
	tests.RunPostgresGetColumnCardinalityTest(t, ctx, pool)
	tests.RunPostgresListTableStatsTest(t, ctx, pool)
	tests.RunPostgresListPublicationTablesTest(t, ctx, pool)
	tests.RunPostgresListTableSpacesTest(t)
	tests.RunPostgresListPgSettingsTest(t, ctx, pool)
	tests.RunPostgresListDatabaseStatsTest(t, ctx, pool)
	tests.RunPostgresListRolesTest(t, ctx, pool)
	tests.RunPostgresListStoredProcedureTest(t, ctx, pool)
	tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox")
	runPostgresReadOnlyTest(t, ctx, PostgresHost, PostgresPort, uniqueID)
}

func addPostgresReadOnlyConfig(t *testing.T, ctx context.Context, pool *pgxpool.Pool, toolsFile map[string]any, host, port, uniqueID string) map[string]any {
	readerUser := "reader_" + uniqueID
	writerUser := "writer_" + uniqueID
	writerGroup := "writer_grp_" + uniqueID
	inheritedWriterUser := "inh_writer_" + uniqueID
	tableOwnerUser := "owner_" + uniqueID
	userPass := "test_pass"
	tableName := "ro_test_" + uniqueID
	ownedTableName := "ro_owned_" + uniqueID

	setupSQL := fmt.Sprintf(`
		CREATE TABLE %s (id INT);
		REVOKE CREATE ON SCHEMA public FROM PUBLIC;

		-- Direct table writer
		CREATE USER %s WITH PASSWORD '%s';
		GRANT CONNECT ON DATABASE %s TO %s;
		GRANT USAGE ON SCHEMA public TO %s;
		GRANT SELECT, INSERT ON TABLE %s TO %s;

		-- Inherited role writer (privileges granted to group, inherited by user)
		CREATE ROLE %s;
		GRANT SELECT, UPDATE ON TABLE %s TO %s;
		CREATE USER %s WITH PASSWORD '%s';
		GRANT CONNECT ON DATABASE %s TO %s;
		GRANT USAGE ON SCHEMA public TO %s;
		GRANT %s TO %s;

		-- Table owner (owns a table, giving implicit write/DDL rights even without explicit table_privileges row)
		CREATE USER %s WITH PASSWORD '%s';
		GRANT CONNECT ON DATABASE %s TO %s;
		GRANT USAGE ON SCHEMA public TO %s;
		CREATE TABLE %s (id INT);
		ALTER TABLE %s OWNER TO %s;

		-- Strictly read-only user
		CREATE USER %s WITH PASSWORD '%s';
		GRANT CONNECT ON DATABASE %s TO %s;
		GRANT USAGE ON SCHEMA public TO %s;
		GRANT SELECT ON TABLE %s TO %s;
	`, tableName,
		writerUser, userPass, PostgresDatabase, writerUser, writerUser, tableName, writerUser,
		writerGroup, tableName, writerGroup, inheritedWriterUser, userPass, PostgresDatabase, inheritedWriterUser, inheritedWriterUser, writerGroup, inheritedWriterUser,
		tableOwnerUser, userPass, PostgresDatabase, tableOwnerUser, tableOwnerUser, ownedTableName, ownedTableName, tableOwnerUser,
		readerUser, userPass, PostgresDatabase, readerUser, readerUser, tableName, readerUser)
	if _, err := pool.Exec(ctx, setupSQL); err != nil {
		t.Fatalf("failed to setup read-only test roles: %v", err)
	}

	t.Cleanup(func() {
		cleanupSQL := fmt.Sprintf(`
			DROP TABLE IF EXISTS %s;
			DROP TABLE IF EXISTS %s;
			DROP ROLE IF EXISTS %s;
			DROP ROLE IF EXISTS %s;
			DROP ROLE IF EXISTS %s;
			DROP ROLE IF EXISTS %s;
			DROP ROLE IF EXISTS %s;
			GRANT CREATE ON SCHEMA public TO PUBLIC;
		`, tableName, ownedTableName, readerUser, writerUser, inheritedWriterUser, writerGroup, tableOwnerUser)
		_, _ = pool.Exec(context.Background(), cleanupSQL)
	})

	readerSourceConfig := getPostgresVars(t, host, port)
	readerSourceConfig["user"] = readerUser
	readerSourceConfig["password"] = userPass
	readerSourceConfig["readOnly"] = true

	readerSimpleSourceConfig := getPostgresVars(t, host, port)
	readerSimpleSourceConfig["user"] = readerUser
	readerSimpleSourceConfig["password"] = userPass
	readerSimpleSourceConfig["readOnly"] = true
	readerSimpleSourceConfig["queryExecMode"] = "simple_protocol"

	sourcesMap := toolsFile["sources"].(map[string]any)
	sourcesMap["pg-reader-ro"] = readerSourceConfig
	sourcesMap["pg-reader-ro-simple"] = readerSimpleSourceConfig

	toolsMap := toolsFile["tools"].(map[string]any)
	toolsMap["valid_select_tool"] = map[string]any{
		"type":        PostgresToolType,
		"source":      "pg-reader-ro",
		"description": "Valid read query tool",
		"annotations": map[string]any{"readOnlyHint": true},
		"statement":   "SELECT 1 AS val;",
	}
	toolsMap["suppressed_unannotated_tool"] = map[string]any{
		"type":        PostgresToolType,
		"source":      "pg-reader-ro",
		"description": "Unannotated tool defaults to destructive and should be suppressed at startup",
		"statement":   fmt.Sprintf("INSERT INTO %s VALUES (99);", tableName),
	}
	toolsMap["vulnerable_write_tool"] = map[string]any{
		"type":        PostgresToolType,
		"source":      "pg-reader-ro",
		"description": "Write tool falsely claiming readOnlyHint: true",
		"annotations": map[string]any{"readOnlyHint": true},
		"statement":   fmt.Sprintf("INSERT INTO %s VALUES (1);", tableName),
	}
	toolsMap["vulnerable_ddl_tool"] = map[string]any{
		"type":        PostgresToolType,
		"source":      "pg-reader-ro",
		"description": "DDL tool falsely claiming readOnlyHint: true",
		"annotations": map[string]any{"readOnlyHint": true},
		"statement":   fmt.Sprintf("CREATE TABLE %s_hacker (id INT);", tableName),
	}
	toolsMap["simple_protocol_chained_insert_tool"] = map[string]any{
		"type":        PostgresToolType,
		"source":      "pg-reader-ro-simple",
		"description": "Multi-statement semicolon & commit-chaining injection over simple_protocol attempting transaction escape and INSERT",
		"annotations": map[string]any{"readOnlyHint": true},
		"statement":   fmt.Sprintf("SELECT 1; COMMIT; BEGIN READ WRITE; SET SESSION CHARACTERISTICS AS TRANSACTION READ WRITE; SET default_transaction_read_only = off; INSERT INTO %s VALUES (2); COMMIT;", tableName),
	}

	return toolsFile
}

func runPostgresReadOnlyTest(t *testing.T, ctx context.Context, host, port, uniqueID string) {
	t.Run("ReadOnly", func(t *testing.T) {
		readerUser := "reader_" + uniqueID
		writerUser := "writer_" + uniqueID
		inheritedWriterUser := "inh_writer_" + uniqueID
		tableOwnerUser := "owner_" + uniqueID
		userPass := "test_pass"

		baseCfg := func(user, pass, execMode string) pgsource.Config {
			return pgsource.Config{
				Name:          "test-ro-source",
				Type:          PostgresSourceType,
				Host:          host,
				Port:          port,
				User:          user,
				Password:      pass,
				Database:      PostgresDatabase,
				QueryExecMode: execMode,
				ReadOnly:      true,
			}
		}

		// 1. Table-driven startup verification testing Config.Initialize directly against the shared Postgres container
		startupTCs := []struct {
			name    string
			cfg     pgsource.Config
			wantErr string
		}{
			{
				name:    "superuser fails closed",
				cfg:     baseCfg(PostgresUser, PostgresPass, ""),
				wantErr: "is a superuser",
			},
			{
				name:    "direct table writer fails closed",
				cfg:     baseCfg(writerUser, userPass, ""),
				wantErr: "has table write privileges",
			},
			{
				name:    "inherited role writer fails closed",
				cfg:     baseCfg(inheritedWriterUser, userPass, ""),
				wantErr: "has table write privileges",
			},
			{
				name:    "table owner fails closed",
				cfg:     baseCfg(tableOwnerUser, userPass, ""),
				wantErr: "has table write privileges",
			},
			{
				name:    "strictly read-only user succeeds (extended protocol)",
				cfg:     baseCfg(readerUser, userPass, ""),
				wantErr: "",
			},
			{
				name:    "strictly read-only user succeeds (simple protocol)",
				cfg:     baseCfg(readerUser, userPass, "simple_protocol"),
				wantErr: "",
			},
		}
		tracer := noop.NewTracerProvider().Tracer("test")
		for _, tc := range startupTCs {
			t.Run(tc.name, func(t *testing.T) {
				src, err := tc.cfg.Initialize(ctx, tracer)
				if src != nil {
					defer src.(*pgsource.Source).Pool.Close()
				}
				if tc.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
						t.Fatalf("expected Initialize error containing %q for %q, got: %v", tc.wantErr, tc.name, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("expected Initialize to succeed for %q, got error: %v", tc.name, err)
				}
				if !src.IsReadOnly() {
					t.Fatalf("expected source.IsReadOnly() == true for %q", tc.name)
				}
			})
		}

		// 2. Table-driven runtime enforcement test reusing the single shared Toolbox server on port 5000
		runtimeTCs := []struct {
			name        string
			toolName    string
			wantContain string
		}{
			{
				name:        "valid SELECT succeeds on read-only source",
				toolName:    "valid_select_tool",
				wantContain: "1",
			},
			{
				name:        "unannotated write tool is suppressed at startup (404 Not Found)",
				toolName:    "suppressed_unannotated_tool",
				wantContain: "does not exist",
			},
			{
				name:        "direct INSERT over extended protocol blocked by RBAC",
				toolName:    "vulnerable_write_tool",
				wantContain: "permission denied",
			},
			{
				name:        "direct CREATE TABLE over extended protocol blocked by RBAC",
				toolName:    "vulnerable_ddl_tool",
				wantContain: "permission denied",
			},
			{
				name:        "commit-chaining & transaction escape INSERT over simple_protocol blocked by RBAC",
				toolName:    "simple_protocol_chained_insert_tool",
				wantContain: "permission denied",
			},
		}
		for _, tc := range runtimeTCs {
			t.Run(tc.name, func(t *testing.T) {
				api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
				resp, respBody := tests.RunRequest(t, "POST", api, strings.NewReader(`{}`), map[string]string{})
				respBodyLower := strings.ToLower(string(respBody))
				if !strings.Contains(respBodyLower, tc.wantContain) {
					t.Fatalf("expected response for tool %q to contain %q, got status %d and body:\n%s", tc.toolName, tc.wantContain, resp.StatusCode, string(respBody))
				}
			})
		}
	})
}
