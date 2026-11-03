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

package cloudsqlmssql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/cloudsqlconn"
	"cloud.google.com/go/cloudsqlconn/sqlserver/mssql"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	CloudSQLMSSQLSourceType = "cloud-sql-mssql"
	CloudSQLMSSQLToolType   = "mssql-sql"
	CloudSQLMSSQLProject    = os.Getenv("CLOUD_SQL_MSSQL_PROJECT")
	CloudSQLMSSQLRegion     = os.Getenv("CLOUD_SQL_MSSQL_REGION")
	CloudSQLMSSQLInstance   = os.Getenv("CLOUD_SQL_MSSQL_INSTANCE")
	CloudSQLMSSQLDatabase   = os.Getenv("CLOUD_SQL_MSSQL_DATABASE")
	CloudSQLMSSQLUser       = os.Getenv("CLOUD_SQL_MSSQL_USER")
	CloudSQLMSSQLPass       = os.Getenv("CLOUD_SQL_MSSQL_PASS")
)

func getCloudSQLMSSQLVars(t *testing.T) map[string]any {
	switch "" {
	case CloudSQLMSSQLProject:
		t.Fatal("'CLOUD_SQL_MSSQL_PROJECT' not set")
	case CloudSQLMSSQLRegion:
		t.Fatal("'CLOUD_SQL_MSSQL_REGION' not set")
	case CloudSQLMSSQLInstance:
		t.Fatal("'CLOUD_SQL_MSSQL_INSTANCE' not set")
	case CloudSQLMSSQLDatabase:
		t.Fatal("'CLOUD_SQL_MSSQL_DATABASE' not set")
	case CloudSQLMSSQLUser:
		t.Fatal("'CLOUD_SQL_MSSQL_USER' not set")
	case CloudSQLMSSQLPass:
		t.Fatal("'CLOUD_SQL_MSSQL_PASS' not set")
	}

	return map[string]any{
		"type":     CloudSQLMSSQLSourceType,
		"project":  CloudSQLMSSQLProject,
		"instance": CloudSQLMSSQLInstance,
		"region":   CloudSQLMSSQLRegion,
		"database": CloudSQLMSSQLDatabase,
		"user":     CloudSQLMSSQLUser,
		"password": CloudSQLMSSQLPass,
	}
}

// Copied over from cloud_sql_mssql.go
func initCloudSQLMSSQLConnection(project, region, instance, ipType, user, pass, dbname string) (*sql.DB, error) {
	// Create dsn
	query := fmt.Sprintf("database=%s&cloudsql=%s:%s:%s", dbname, project, region, instance)
	url := &url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(user, pass),
		RawQuery: query,
	}

	// Get dial options
	dialOpts, err := tests.GetCloudSQLDialOpts(ipType)
	if err != nil {
		return nil, err
	}

	// Register sql server driver
	if !slices.Contains(sql.Drivers(), "cloudsql-sqlserver-driver") {
		_, err := mssql.RegisterDriver("cloudsql-sqlserver-driver", cloudsqlconn.WithDefaultDialOptions(dialOpts...))
		if err != nil {
			return nil, err
		}
	}

	// Open database connection
	db, err := sql.Open(
		"cloudsql-sqlserver-driver",
		url.String(),
	)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func TestCloudSQLMSSQLToolEndpoints(t *testing.T) {
	sourceConfig := getCloudSQLMSSQLVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	db, err := initCloudSQLMSSQLConnection(CloudSQLMSSQLProject, CloudSQLMSSQLRegion, CloudSQLMSSQLInstance, "public", CloudSQLMSSQLUser, CloudSQLMSSQLPass, CloudSQLMSSQLDatabase)
	if err != nil {
		t.Fatalf("unable to create Cloud SQL connection pool: %s", err)
	}

	// cleanup test environment
	tests.CleanupMSSQLTables(t, ctx, db)

	// create table name with UUID
	tableNameParam := "param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameAuth := "auth_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableNameTemplateParam := "template_param_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := tests.GetMSSQLParamToolInfo(tableNameParam)
	teardownTable1 := tests.SetupMsSQLTable(t, ctx, db, createParamTableStmt, insertParamTableStmt, tableNameParam, paramTestParams)
	defer teardownTable1(t)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := tests.GetMSSQLAuthToolInfo(tableNameAuth)
	teardownTable2 := tests.SetupMsSQLTable(t, ctx, db, createAuthTableStmt, insertAuthTableStmt, tableNameAuth, authTestParams)
	defer teardownTable2(t)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, CloudSQLMSSQLToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddMSSQLExecuteSqlConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := tests.GetMSSQLTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, CloudSQLMSSQLToolType, tmplSelectCombined, tmplSelectFilterCombined, "")
	toolsFile = tests.AddMSSQLPrebuiltToolConfig(t, toolsFile)

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
	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := tests.GetMSSQLWants()

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam)

	// Run specific MSSQL tool tests
	tests.RunMSSQLListTablesTest(t, tableNameParam, tableNameAuth)
	tests.RunMSSQLValueTypesTest(t, "my-exec-sql-tool")
}

// Test connection with different IP type
func TestCloudSQLMSSQLIpConnection(t *testing.T) {
	sourceConfig := getCloudSQLMSSQLVars(t)

	tcs := []struct {
		name   string
		ipType string
	}{
		{
			name:   "public ip",
			ipType: "public",
		},
		{
			name:   "private ip",
			ipType: "private",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			sourceConfig["ipType"] = tc.ipType
			err := tests.RunSourceConnectionTest(t, sourceConfig, CloudSQLMSSQLToolType)
			if err != nil {
				t.Fatalf("Connection test failure: %s", err)
			}
		})
	}
}
