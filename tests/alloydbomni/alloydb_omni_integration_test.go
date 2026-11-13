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
	"os"
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

// columnarEnvVar gates the tests that need the AlloyDB columnar engine. Only CI
// sets it: a columnar-enabled container needs more memory than a typical
// development machine has to spare.
const columnarEnvVar = "ALLOYDB_OMNI_COLUMNAR"

// skipUnlessColumnar skips the test unless columnarEnvVar is set to "true".
func skipUnlessColumnar(t *testing.T) {
	t.Helper()
	if os.Getenv(columnarEnvVar) != "true" {
		t.Skipf("%s not set; columnar engine tests run only in CI", columnarEnvVar)
	}
}

// setupAlloyDBContainer starts an AlloyDB Omni container, with the columnar
// engine enabled when columnar is set.
func setupAlloyDBContainer(ctx context.Context, t *testing.T, columnar bool) (string, string, func()) {
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
	if columnar {
		req.Cmd = []string{"-c", "google_columnar_engine.enabled=on"}
	}

	return tests.SetupGenericPostgresTestContainer(ctx, t, req)
}

// setupAlloyDBOmniTest starts an AlloyDB Omni container and a toolbox server
// with the alloydb-omni prebuilt tools, and returns a connection pool to the
// database and a unique ID for test resources. The REST API is only enabled
// when isMCP is false, and the columnar engine only when columnar is set.
// Everything is torn down on cleanup.
func setupAlloyDBOmniTest(t *testing.T, ctx context.Context, isMCP, columnar bool) (*pgxpool.Pool, string) {
	t.Helper()

	AlloyDBHost, AlloyDBPort, containerCleanup := setupAlloyDBContainer(ctx, t, columnar)
	t.Cleanup(containerCleanup)

	t.Setenv("ALLOYDB_OMNI_HOST", AlloyDBHost)
	t.Setenv("ALLOYDB_OMNI_PORT", AlloyDBPort)
	t.Setenv("ALLOYDB_OMNI_USER", AlloyDBUser)
	t.Setenv("ALLOYDB_OMNI_PASSWORD", AlloyDBPass)
	t.Setenv("ALLOYDB_OMNI_DATABASE", AlloyDBDatabase)

	// Generate a unique ID
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")

	pool, err := initPostgresConnectionPool(AlloyDBHost, AlloyDBPort, AlloyDBUser, AlloyDBPass, AlloyDBDatabase)
	if err != nil {
		t.Fatalf("unable to create alloydb connection pool: %s", err)
	}
	t.Cleanup(pool.Close)

	args := []string{"--prebuilt", "alloydb-omni"}
	if !isMCP {
		args = append(args, "--enable-api")
	}
	cmd, cleanup, err := tests.StartCmd(ctx, map[string]any{}, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	// Wait for server to be ready
	waitCtx, waitCancel := context.WithTimeout(ctx, 30*time.Second)
	defer waitCancel()

	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	return pool, uniqueID
}

// runAlloyDBOmniTests runs the Postgres prebuilt tool tests and the AlloyDB
// Omni statement tools that don't need the columnar engine, passing opts to
// every helper.
func runAlloyDBOmniTests(t *testing.T, ctx context.Context, pool *pgxpool.Pool, uniqueID string, opts ...tests.ToolExecOption) {
	tests.RunPostgresListViewsTest(t, ctx, pool, opts...)
	tests.RunPostgresListSchemasTest(t, ctx, pool, AlloyDBUser, uniqueID, opts...)
	tests.RunPostgresListActiveQueriesTest(t, ctx, pool, opts...)
	tests.RunPostgresListAvailableExtensionsTest(t, opts...)
	tests.RunPostgresListInstalledExtensionsTest(t, opts...)
	tests.RunPostgresDatabaseOverviewTest(t, ctx, pool, opts...)
	tests.RunPostgresListTriggersTest(t, ctx, pool, opts...)
	tests.RunPostgresListIndexesTest(t, ctx, pool, opts...)
	tests.RunPostgresListSequencesTest(t, ctx, pool, opts...)
	tests.RunPostgresLongRunningTransactionsTest(t, ctx, pool, opts...)
	tests.RunPostgresListLocksTest(t, ctx, pool, opts...)
	tests.RunPostgresReplicationStatsTest(t, ctx, pool, opts...)
	tests.RunPostgresGetColumnCardinalityTest(t, ctx, pool, opts...)
	tests.RunPostgresListTableStatsTest(t, ctx, pool, opts...)
	tests.RunPostgresListPublicationTablesTest(t, ctx, pool, opts...)
	tests.RunPostgresListTableSpacesTest(t, opts...)
	tests.RunPostgresListPgSettingsTest(t, ctx, pool, opts...)
	tests.RunPostgresListDatabaseStatsTest(t, ctx, pool, opts...)
	tests.RunPostgresListRolesTest(t, ctx, pool, opts...)
	tests.RunPostgresListStoredProcedureTest(t, ctx, pool, opts...)

	toolsToTest := map[string]string{
		"list_autovacuum_configurations": `{}`,
		"list_memory_configurations":     `{}`,
		"list_top_bloated_tables":        `{"limit": 10}`,
		"list_replication_slots":         `{}`,
		"list_invalid_indexes":           `{}`,
		"get_query_plan":                 `{"query": "SELECT 1"}`,
	}
	tests.RunStatementToolsTest(t, toolsToTest, opts...)
}

// runAlloyDBOmniColumnarTests runs the statement tools that need the columnar
// engine enabled.
func runAlloyDBOmniColumnarTests(t *testing.T, opts ...tests.ToolExecOption) {
	toolsToTest := map[string]string{
		"list_columnar_configurations":      `{}`,
		"list_columnar_recommended_columns": `{}`,
	}
	tests.RunStatementToolsTest(t, toolsToTest, opts...)
}

func TestAlloyDBOmni(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, uniqueID := setupAlloyDBOmniTest(t, ctx, false, false)
	runAlloyDBOmniTests(t, ctx, pool, uniqueID)
}

func TestAlloyDBOmniColumnar(t *testing.T) {
	skipUnlessColumnar(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	setupAlloyDBOmniTest(t, ctx, false, true)
	runAlloyDBOmniColumnarTests(t)
}
