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
	defer containerCleanup()

	os.Setenv("ALLOYDB_OMNI_HOST", AlloyDBHost)
	os.Setenv("ALLOYDB_OMNI_PORT", AlloyDBPort)
	os.Setenv("ALLOYDB_OMNI_USER", AlloyDBUser)
	os.Setenv("ALLOYDB_OMNI_PASSWORD", AlloyDBPass)
	os.Setenv("ALLOYDB_OMNI_DATABASE", AlloyDBDatabase)

	// Generate a unique ID
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")

	args := []string{"--prebuilt", "alloydb-omni", "--enable-api"}

	pool, err := initPostgresConnectionPool(AlloyDBHost, AlloyDBPort, AlloyDBUser, AlloyDBPass, AlloyDBDatabase)
	if err != nil {
		t.Fatalf("unable to create alloydb connection pool: %s", err)
	}

	cmd, cleanup, err := tests.StartCmd(ctx, map[string]any{}, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

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
