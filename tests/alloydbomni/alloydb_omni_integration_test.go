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

package alloydbomni

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	AlloyDBUser     = "postgres"
	AlloyDBPass     = "mysecretpassword"
	AlloyDBDatabase = "postgres"
)

func buildPostgresURL(host, port, user, pass, dbname string) *url.URL {
	return &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   dbname,
	}
}

func initPostgresConnectionPool(host, port, user, pass, dbname string) (*pgxpool.Pool, error) {
	url := buildPostgresURL(host, port, user, pass, dbname)
	pool, err := pgxpool.New(context.Background(), url.String())
	if err != nil {
		return nil, fmt.Errorf("Unable to create connection pool: %w", err)
	}

	return pool, nil
}

func setupAlloyDBContainer(ctx context.Context, t *testing.T) (string, string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "google/alloydbomni:16.9.0-ubi9", // Pinning version for stability
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_PASSWORD": AlloyDBPass,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system was shut down at"),
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForExposedPort(),
		),
	}

	return tests.SetupGenericPostgresTestContainer(ctx, t, req)
}

func TestAlloyDBOmni(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	AlloyDBHost, AlloyDBPort, containerCleanup := setupAlloyDBContainer(ctx, t)
	t.Cleanup(containerCleanup)

	t.Setenv("ALLOYDB_OMNI_HOST", AlloyDBHost)
	t.Setenv("ALLOYDB_OMNI_PORT", AlloyDBPort)
	t.Setenv("ALLOYDB_OMNI_USER", AlloyDBUser)
	t.Setenv("ALLOYDB_OMNI_PASSWORD", AlloyDBPass)
	t.Setenv("ALLOYDB_OMNI_DATABASE", AlloyDBDatabase)

	// Generate a unique ID
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")

	args := []string{"--prebuilt", "alloydb-omni", "--enable-api"}

	pool, err := initPostgresConnectionPool(AlloyDBHost, AlloyDBPort, AlloyDBUser, AlloyDBPass, AlloyDBDatabase)
	if err != nil {
		t.Fatalf("unable to create alloydb connection pool: %s", err)
	}
	t.Cleanup(pool.Close)

	cmd, cleanup, err := tests.StartCmd(ctx, map[string]any{}, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)

	// Wait for server to be ready
	waitCtx, waitCancel := context.WithTimeout(ctx, 30*time.Second)
	defer waitCancel()

	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Run Postgres prebuilt tool tests
	tests.RunPostgresListViewsTest(t, ctx, pool)
	tests.RunPostgresListSchemasTest(t, ctx, pool, AlloyDBUser, uniqueID)
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
	tests.RunPostgresGetColumnCardinalityTest(t, ctx, pool)
	tests.RunPostgresListTableStatsTest(t, ctx, pool)
	tests.RunPostgresListPublicationTablesTest(t, ctx, pool)
	tests.RunPostgresListTableSpacesTest(t)
	tests.RunPostgresListPgSettingsTest(t, ctx, pool)
	tests.RunPostgresListDatabaseStatsTest(t, ctx, pool)
	tests.RunPostgresListRolesTest(t, ctx, pool)
	tests.RunPostgresListStoredProcedureTest(t, ctx, pool)

	toolsToTest := map[string]string{
		"list_autovacuum_configurations":    `{}`,
		"list_memory_configurations":        `{}`,
		"list_top_bloated_tables":           `{"limit": 10}`,
		"list_replication_slots":            `{}`,
		"list_invalid_indexes":              `{}`,
		"get_query_plan":                    `{"query": "SELECT 1"}`,
		"list_columnar_configurations":      `{}`,
		"list_columnar_recommended_columns": `{}`,
	}
	tests.RunStatementToolsTest(t, toolsToTest)
}

// TestAlloyDBOmniToolEndpoints runs the engine-level postgres-sql,
// postgres-execute-sql, template-parameter, semantic-search, and list_tables
// subtests against an isolated AlloyDB Omni container. AlloyDB Go Connector,
// IAM authentication, IP-type selection, and alloydb_session_read_only kernel
// enforcement require managed AlloyDB and remain in tests/alloydbpg.
func TestAlloyDBOmniToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	host, port, containerCleanup := setupAlloyDBContainer(ctx, t)
	t.Cleanup(containerCleanup)

	pool, err := initPostgresConnectionPool(host, port, AlloyDBUser, AlloyDBPass, AlloyDBDatabase)
	if err != nil {
		t.Fatalf("unable to create alloydb connection pool: %s", err)
	}
	t.Cleanup(pool.Close)

	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Cleanup(func() {
		tests.CleanupPostgresTables(t, context.Background(), pool, uniqueID)
	})

	tableNameParam := "param_table_" + uniqueID
	tableNameAuth := "auth_table_" + uniqueID
	tableNameTemplateParam := "template_param_table_" + uniqueID
	tableNameExecSql := "exec_sql_table_" + uniqueID

	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := tests.GetPostgresSQLParamToolInfo(tableNameParam)
	teardownTable1 := tests.SetupPostgresSQLTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := tests.GetPostgresSQLAuthToolInfo(tableNameAuth)
	teardownTable2 := tests.SetupPostgresSQLTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)

	vectorTableName, tearDownVectorTable := tests.SetupPostgresVectorTable(t, ctx, pool)
	defer tearDownVectorTable(t)

	sourceConfig := map[string]any{
		"type":     "postgres",
		"host":     host,
		"port":     port,
		"database": AlloyDBDatabase,
		"user":     AlloyDBUser,
		"password": AlloyDBPass,
	}
	toolsFile := tests.GetToolsConfig(sourceConfig, "postgres-sql", paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddExecuteSqlConfig(t, toolsFile, "postgres-execute-sql")
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetPostgresSQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, "postgres-sql", tmplSelectCombined, tmplSelectFilterCombined, "")
	toolsFile = tests.AddPostgresPrebuiltConfig(t, toolsFile)
	insertStmt, searchStmt := tests.GetPostgresVectorSearchStmts(vectorTableName)
	toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, "postgres-sql", insertStmt, searchStmt)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, "--enable-api")
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)

	waitCtx, waitCancel := context.WithTimeout(ctx, 30*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	select1Want, mcpMyFailToolWant, _, mcpSelect1Want := tests.GetPostgresWants()
	createTableStatement := tests.GetPostgresExecuteSqlCreateTableStatement(tableNameExecSql)

	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want)
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, tests.WithExecuteSqlTableName(tableNameExecSql))
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)
	tests.RunPostgresListTablesTest(t, tableNameParam, tableNameAuth, AlloyDBUser)
	tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox")
}
