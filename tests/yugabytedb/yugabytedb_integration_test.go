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

package yugabytedb

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/yugabyte/pgx/v5/pgxpool"
)

var (
	YBDB_SOURCE_KIND = "yugabytedb"
	YBDB_TOOL_KIND   = "yugabytedb-sql"
	YBDB_DATABASE    = os.Getenv("YUGABYTEDB_DATABASE")
	YBDB_HOST        = os.Getenv("YUGABYTEDB_HOST")
	YBDB_PORT        = os.Getenv("YUGABYTEDB_PORT")
	YBDB_USER        = os.Getenv("YUGABYTEDB_USER")
	YBDB_PASS        = os.Getenv("YUGABYTEDB_PASS")
	YBDB_LB          = os.Getenv("YUGABYTEDB_LOADBALANCE")
)

func getYBVars(t *testing.T) map[string]any {
	switch "" {
	case YBDB_DATABASE:
		t.Fatal("'YUGABYTEDB_DATABASE' not set")
	case YBDB_HOST:
		t.Fatal("'YUGABYTEDB_HOST' not set")
	case YBDB_PORT:
		t.Fatal("'YUGABYTEDB_PORT' not set")
	case YBDB_USER:
		t.Fatal("'YUGABYTEDB_USER' not set")
	case YBDB_PASS:
		t.Fatal("'YUGABYTEDB_PASS' not set")
	case YBDB_LB:
		fmt.Printf("YUGABYTEDB_LOADBALANCE value not set. Setting default value: false")
		YBDB_LB = "false"
	}

	return map[string]any{
		"type":        YBDB_SOURCE_KIND,
		"host":        YBDB_HOST,
		"port":        YBDB_PORT,
		"database":    YBDB_DATABASE,
		"user":        YBDB_USER,
		"password":    YBDB_PASS,
		"loadBalance": YBDB_LB,
	}
}

func initYBConnectionPool(host, port, user, pass, dbname, loadBalance string) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?load_balance=%s", user, pass, host, port, dbname, loadBalance)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to create YugabyteDB connection pool: %w", err)
	}
	return pool, nil
}

// SetupYugabyteDBSQLTable creates and inserts data into a table of tool
// compatible with yugabytedb-sql tool
func SetupYugabyteDBSQLTable(t *testing.T, ctx context.Context, pool *pgxpool.Pool, create_statement, insert_statement, tableName string, params []any) func(*testing.T) {
	err := pool.Ping(ctx)
	if err != nil {
		t.Fatalf("unable to connect to test database: %s", err)
	}

	// Create table
	_, err = pool.Exec(ctx, create_statement)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	_, err = pool.Exec(ctx, insert_statement, params...)
	if err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}

	return func(t *testing.T) {
		// tear down test
		_, err = pool.Exec(ctx, fmt.Sprintf("DROP TABLE %s;", tableName))
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}
}

func TestYugabyteDB(t *testing.T) {
	sourceConfig := getYBVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	pool, err := initYBConnectionPool(YBDB_HOST, YBDB_PORT, YBDB_USER, YBDB_PASS, YBDB_DATABASE, YBDB_LB)
	if err != nil {
		t.Fatalf("unable to create YugabyteDB connection pool: %s", err)
	}
	defer pool.Close()

	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := tests.GetPostgresSQLParamToolInfo(tableNameParam)
	teardownTable1 := SetupYugabyteDBSQLTable(t, ctx, pool, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := tests.GetPostgresSQLAuthToolInfo(tableNameAuth)
	teardownTable2 := SetupYugabyteDBSQLTable(t, ctx, pool, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)

	toolsFile := tests.GetToolsConfig(sourceConfig, YBDB_TOOL_KIND, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetPostgresSQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, YBDB_TOOL_KIND, tmplSelectCombined, tmplSelectFilterCombined, "")

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer func() {
		cmd.Close()
		cleanup()
	}()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	select1Want, mcpMyFailToolWant, _, mcpSelect1Want := tests.GetPostgresWants()

	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want)
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)
}
