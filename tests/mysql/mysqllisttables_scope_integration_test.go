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

package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	mysqlsource "github.com/googleapis/mcp-toolbox/internal/sources/mysql"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/mysql/mysqllisttables"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestMySQLListTablesDatabaseScope(t *testing.T) {
	for _, name := range []string{"MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASS"} {
		if os.Getenv(name) == "" {
			t.Fatalf("%q not set", name)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	connectionConfig := driver.NewConfig()
	connectionConfig.Net = "tcp"
	connectionConfig.Addr = net.JoinHostPort(os.Getenv("MYSQL_HOST"), os.Getenv("MYSQL_PORT"))
	connectionConfig.User = os.Getenv("MYSQL_USER")
	connectionConfig.Passwd = os.Getenv("MYSQL_PASS")
	openPool := func(t *testing.T, database string) *sql.DB {
		t.Helper()
		config := *connectionConfig
		config.DBName = database
		pool, err := sql.Open("mysql", config.FormatDSN())
		if err != nil {
			t.Fatalf("unable to create MySQL connection pool: %v", err)
		}
		t.Cleanup(func() {
			if err := pool.Close(); err != nil {
				t.Errorf("unable to close MySQL connection pool: %v", err)
			}
		})
		return pool
	}

	pool := openPool(t, "")
	var lowerCaseTableNames int
	if err := pool.QueryRowContext(ctx, "SELECT @@lower_case_table_names").Scan(&lowerCaseTableNames); err != nil {
		t.Fatalf("unable to read database name case behavior: %v", err)
	}
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	database := "list_tables_" + suffix
	leadingSpaceDatabase := " " + database
	unaccentedDatabase := database + "_cafe"
	accentedDatabase := database + "_café"
	upperCaseDatabase := strings.ToUpper(database)
	upperCaseSchema := database
	schemas := []string{database, leadingSpaceDatabase, unaccentedDatabase, accentedDatabase}
	if lowerCaseTableNames == 0 {
		upperCaseSchema = upperCaseDatabase
		schemas = append(schemas, upperCaseSchema)
	}
	tableName := "scope_probe_" + suffix
	// All schemas contain the same table name so selecting the wrong schema
	// cannot pass. This fixture requires CREATE and DROP DATABASE privileges.
	for _, schema := range schemas {
		if _, err := pool.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s`", schema)); err != nil {
			t.Fatalf("unable to create test database %q: %v", schema, err)
		}
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			if _, err := pool.ExecContext(cleanupCtx, fmt.Sprintf("DROP DATABASE `%s`", schema)); err != nil {
				t.Errorf("unable to drop test database %q: %v", schema, err)
			}
		})
		if _, err := pool.ExecContext(ctx, fmt.Sprintf("CREATE TABLE `%s`.`%s` (id INT PRIMARY KEY)", schema, tableName)); err != nil {
			t.Fatalf("unable to create test table in %q: %v", schema, err)
		}
	}

	cfg := mysqllisttables.Config{
		ConfigBase: tools.ConfigBase{Name: "list_tables", Description: "List tables in the configured database."},
		Type:       "mysql-list-tables",
		Source:     "mysql-source",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unable to initialize list-tables tool: %v", err)
	}
	type tableInfo struct {
		SchemaName string `json:"schema_name"`
		ObjectName string `json:"object_name"`
	}
	allTables := make([]tableInfo, 0, len(schemas))
	for _, schema := range schemas {
		allTables = append(allTables, tableInfo{schema, tableName})
	}
	for _, tc := range []struct {
		name       string
		database   string
		tableNames string
		want       []tableInfo
	}{
		{"configured named table", database, tableName, []tableInfo{{database, tableName}}},
		{"configured all tables", database, "", []tableInfo{{database, tableName}}},
		{"leading space named table", leadingSpaceDatabase, tableName, []tableInfo{{leadingSpaceDatabase, tableName}}},
		{"leading space all tables", leadingSpaceDatabase, "", []tableInfo{{leadingSpaceDatabase, tableName}}},
		{"alternate case named table", upperCaseDatabase, tableName, []tableInfo{{upperCaseSchema, tableName}}},
		{"alternate case all tables", upperCaseDatabase, "", []tableInfo{{upperCaseSchema, tableName}}},
		{"unaccented named table", unaccentedDatabase, tableName, []tableInfo{{unaccentedDatabase, tableName}}},
		{"unaccented all tables", unaccentedDatabase, "", []tableInfo{{unaccentedDatabase, tableName}}},
		{"accented named table", accentedDatabase, tableName, []tableInfo{{accentedDatabase, tableName}}},
		{"accented all tables", accentedDatabase, "", []tableInfo{{accentedDatabase, tableName}}},
		{"unconfigured named table", "", tableName, allTables},
	} {
		for _, outputFormat := range []string{"simple", "detailed"} {
			t.Run(tc.name+"/"+outputFormat, func(t *testing.T) {
				pool := openPool(t, tc.database)
				var selectedDatabase sql.NullString
				if err := pool.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&selectedDatabase); err != nil {
					t.Fatalf("unable to read selected database: %v", err)
				}
				wantSelectedDatabase := ""
				if tc.database != "" {
					wantSelectedDatabase = tc.want[0].SchemaName
				}
				selectedMatches := selectedDatabase.String == wantSelectedDatabase
				if lowerCaseTableNames != 0 {
					selectedMatches = strings.EqualFold(selectedDatabase.String, wantSelectedDatabase)
				}
				if !selectedMatches || selectedDatabase.Valid != (tc.database != "") {
					t.Fatalf("selected database = %#v, want %q", selectedDatabase, wantSelectedDatabase)
				}
				source := &mysqlsource.Source{
					Config: mysqlsource.Config{Database: tc.database},
					Pool:   pool,
				}
				response, invokeErr := tool.Invoke(ctx, source, parameters.ParamValues{
					{Name: "table_names", Value: tc.tableNames},
					{Name: "output_format", Value: outputFormat},
				}, "")
				if invokeErr != nil {
					t.Fatalf("unable to invoke list-tables tool: %v", invokeErr)
				}
				encoded, err := json.Marshal(response)
				if err != nil {
					t.Fatalf("unable to encode list-tables response: %v", err)
				}
				var got []tableInfo
				if err := json.Unmarshal(encoded, &got); err != nil {
					t.Fatalf("unable to decode list-tables response: %v", err)
				}
				if diff := cmp.Diff(tc.want, got, cmpopts.SortSlices(func(a, b tableInfo) bool {
					return a.SchemaName < b.SchemaName
				})); diff != "" {
					t.Errorf("incorrect table scope (-want +got):\n%s", diff)
				}
			})
		}
	}
}
