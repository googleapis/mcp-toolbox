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

package mysql

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
	"github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	MySQLSourceType = "mysql"
	MySQLToolType   = "mysql-sql"
	MySQLDatabase   = os.Getenv("MYSQL_DATABASE")
	MySQLHost       = os.Getenv("MYSQL_HOST")
	MySQLPort       = os.Getenv("MYSQL_PORT")
	MySQLUser       = os.Getenv("MYSQL_USER")
	MySQLPass       = os.Getenv("MYSQL_PASS")
)

const (
	mysqlImage             = "mysql:8.4.9"
	mysqlContainerDatabase = "test_database"
)

func setupMySQLContainer(ctx context.Context, t *testing.T) (host, port, user, pass, database string) {
	t.Helper()

	user = "root"
	pass = strings.ReplaceAll(uuid.New().String(), "-", "")
	database = mysqlContainerDatabase
	req := testcontainers.ContainerRequest{
		Image:        mysqlImage,
		ExposedPorts: []string{"3306/tcp"},
		Env: map[string]string{
			"MYSQL_ROOT_PASSWORD": pass,
			"MYSQL_DATABASE":      database,
		},
		Cmd: []string{"--innodb-use-native-aio=0", "--innodb-read-io-threads=4", "--innodb-write-io-threads=4"},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("3306/tcp"),
			wait.ForLog("port: 3306  MySQL Community Server"),
		).WithStartupTimeoutDefault(3 * time.Minute).WithDeadline(3 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Logger:           log.TestLogger(t),
	})
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("failed to start MySQL container: %s", err)
	}

	host, err = container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get MySQL container host: %s", err)
	}
	mappedPort, err := container.MappedPort(ctx, "3306/tcp")
	if err != nil {
		t.Fatalf("failed to get MySQL container mapped port: %s", err)
	}
	return host, mappedPort.Port(), user, pass, database
}

func useMySQL(ctx context.Context, t *testing.T) {
	t.Helper()
	if MySQLHost != "" {
		return
	}
	t.Logf("MYSQL_HOST not set; starting ephemeral %s container", mysqlImage)
	host, port, user, pass, database := setupMySQLContainer(ctx, t)
	origHost, origPort, origUser, origPass, origDatabase := MySQLHost, MySQLPort, MySQLUser, MySQLPass, MySQLDatabase
	t.Cleanup(func() {
		MySQLHost, MySQLPort, MySQLUser, MySQLPass, MySQLDatabase = origHost, origPort, origUser, origPass, origDatabase
	})
	MySQLHost, MySQLPort, MySQLUser, MySQLPass, MySQLDatabase = host, port, user, pass, database
}

func getMySQLVars(t *testing.T) map[string]any {
	switch "" {
	case MySQLDatabase:
		t.Fatal("'MYSQL_DATABASE' not set")
	case MySQLHost:
		t.Fatal("'MYSQL_HOST' not set")
	case MySQLPort:
		t.Fatal("'MYSQL_PORT' not set")
	case MySQLUser:
		t.Fatal("'MYSQL_USER' not set")
	case MySQLPass:
		t.Fatal("'MYSQL_PASS' not set")
	}

	return map[string]any{
		"type":     MySQLSourceType,
		"host":     MySQLHost,
		"port":     MySQLPort,
		"database": MySQLDatabase,
		"user":     MySQLUser,
		"password": MySQLPass,
	}
}

// Copied over from mysql.go
func initMySQLConnectionPool(host, port, user, pass, dbname string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", user, pass, host, port, dbname)

	// Interact with the driver directly as you normally would
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return pool, nil
}

func TestMySQLToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	useMySQL(ctx, t)
	sourceConfig := getMySQLVars(t)

	args := []string{"--enable-api"}

	pool, err := initMySQLConnectionPool(MySQLHost, MySQLPort, MySQLUser, MySQLPass, MySQLDatabase)
	if err != nil {
		t.Fatalf("unable to create MySQL connection pool: %s", err)
	}
	defer pool.Close()

	// cleanup test environment
	tests.CleanupMySQLTables(t, ctx, pool)

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
	toolsFile := tests.GetToolsConfig(sourceConfig, MySQLToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddMySqlExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMySQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, MySQLToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

	toolsFile = tests.AddMySQLPrebuiltToolConfig(t, toolsFile)

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

	// Get configs for tests
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := tests.GetMySQLWants()

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)

	// Run specific MySQL tool tests
	const expectedOwner = ""
	tests.RunMySQLListTablesTest(t, MySQLDatabase, tableNameParam, tableNameAuth, expectedOwner)
	tests.RunMySQLListActiveQueriesTest(t, ctx, pool)
	tests.RunMySQLListTablesMissingUniqueIndexes(t, ctx, pool, MySQLDatabase)
	tests.RunMySQLListTableFragmentationTest(t, MySQLDatabase, tableNameParam, tableNameAuth)
	tests.RunMySQLGetQueryPlanTest(t, ctx, pool, MySQLDatabase, tableNameParam)
	tests.RunMySQLListAllLocks(t, ctx, pool, MySQLDatabase)
	tests.RunMySQLShowQueryStats(t, ctx, pool, MySQLDatabase)
	tests.RunMySQLListTableStatsTest(t, ctx, pool, MySQLDatabase, tableNameParam, tableNameAuth)
}
