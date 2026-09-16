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

// Package tests contains end to end tests meant to verify the Toolbox Server
// works as expected when executed as a binary.

package tests

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigtable"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources/cloudsqlmysql"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"google.golang.org/api/iterator"
)

// GetToolsConfig returns a mock tools config file
func GetToolsConfig(sourceConfig map[string]any, toolType, paramToolStatement, idParamToolStmt, nameParamToolStmt, arrayToolStatement, authToolStatement string) map[string]any {
	// Write config into a file and pass it to command
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"type":     "google",
				"clientId": ClientId,
			},
		},
		"tools": map[string]any{
			"my-simple-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
				"statement":   "SELECT 1",
			},
			"my-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"statement":   paramToolStatement,
				"parameters": []any{
					map[string]any{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
					map[string]any{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
			},
			"my-tool-by-id": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"statement":   idParamToolStmt,
				"parameters": []any{
					map[string]any{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
				},
			},
			"my-tool-by-name": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"statement":   nameParamToolStmt,
				"parameters": []any{
					map[string]any{
						"name":        "name",
						"type":        "string",
						"description": "user name",
						"required":    false,
					},
				},
			},
			"my-array-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with array params.",
				"statement":   arrayToolStatement,
				"parameters": []any{
					map[string]any{
						"name":        "idArray",
						"type":        "array",
						"description": "ID array",
						"items": map[string]any{
							"name":        "id",
							"type":        "integer",
							"description": "ID",
						},
					},
					map[string]any{
						"name":        "nameArray",
						"type":        "array",
						"description": "user name array",
						"items": map[string]any{
							"name":        "name",
							"type":        "string",
							"description": "user name",
						},
					},
				},
			},
			"my-auth-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test authenticated parameters.",
				// statement to auto-fill authenticated parameter
				"statement": authToolStatement,
				"parameters": []map[string]any{
					{
						"name":        "email",
						"type":        "string",
						"description": "user email",
						"authServices": []map[string]string{
							{
								"name":  "my-google-auth",
								"field": "email",
							},
						},
					},
				},
			},
			"my-auth-required-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test auth required invocation.",
				"statement":   "SELECT 1",
				"authRequired": []string{
					"my-google-auth",
				},
			},
			"my-secure-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test secure parameters.",
				"statement":   paramToolStatement,
				"parameters": []any{
					map[string]any{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
					map[string]any{
						"name":        "name",
						"type":        "string",
						"description": "user name",
						"secure":      true,
					},
				},
			},
			"my-fail-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test statement with incorrect syntax.",
				"statement":   "SELEC 1;",
			},
		},
	}

	return toolsFile
}

// AddExecuteSqlConfig gets the tools config for `execute-sql` tools
func AddExecuteSqlConfig(t *testing.T, config map[string]any, toolType string) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	config["tools"] = tools
	return config
}

func AddPostgresPrebuiltConfig(t *testing.T, config map[string]any) map[string]any {
	var (
		PostgresListSchemasToolType             = "postgres-list-schemas"
		PostgresListTablesToolType              = "postgres-list-tables"
		PostgresListActiveQueriesToolType       = "postgres-list-active-queries"
		PostgresListInstalledExtensionsToolType = "postgres-list-installed-extensions"
		PostgresListAvailableExtensionsToolType = "postgres-list-available-extensions"
		PostgresListViewsToolType               = "postgres-list-views"
		PostgresDatabaseOverviewToolType        = "postgres-database-overview"
		PostgresListTriggersToolType            = "postgres-list-triggers"
		PostgresListIndexesToolType             = "postgres-list-indexes"
		PostgresListSequencesToolType           = "postgres-list-sequences"
		PostgresLongRunningTransactionsToolType = "postgres-long-running-transactions"
		PostgresListLocksToolType               = "postgres-list-locks"
		PostgresReplicationStatsToolType        = "postgres-replication-stats"
		PostgresListQueryStatsToolType          = "postgres-list-query-stats"
		PostgresGetColumnCardinalityToolType    = "postgres-get-column-cardinality"
		PostgresListTableStats                  = "postgres-list-table-stats"
		PostgresListPublicationTablesToolType   = "postgres-list-publication-tables"
		PostgresListTablespacesToolType         = "postgres-list-tablespaces"
		PostgresListPGSettingsToolType          = "postgres-list-pg-settings"
		PostgresListDatabaseStatsToolType       = "postgres-list-database-stats"
		PostgresListRolesToolType               = "postgres-list-roles"
		PostgresListStoredProcedureToolType     = "postgres-list-stored-procedure"
	)

	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["list_tables"] = map[string]any{
		"type":        PostgresListTablesToolType,
		"source":      "my-instance",
		"description": "Lists tables in the database.",
	}
	tools["list_active_queries"] = map[string]any{
		"type":        PostgresListActiveQueriesToolType,
		"source":      "my-instance",
		"description": "Lists active queries in the database.",
	}
	tools["list_installed_extensions"] = map[string]any{
		"type":        PostgresListInstalledExtensionsToolType,
		"source":      "my-instance",
		"description": "Lists installed extensions in the database.",
	}
	tools["list_available_extensions"] = map[string]any{
		"type":        PostgresListAvailableExtensionsToolType,
		"source":      "my-instance",
		"description": "Lists available extensions in the database.",
	}
	tools["list_views"] = map[string]any{
		"type":   PostgresListViewsToolType,
		"source": "my-instance",
	}
	tools["list_schemas"] = map[string]any{
		"type":   PostgresListSchemasToolType,
		"source": "my-instance",
	}
	tools["database_overview"] = map[string]any{
		"type":   PostgresDatabaseOverviewToolType,
		"source": "my-instance",
	}
	tools["list_triggers"] = map[string]any{
		"type":   PostgresListTriggersToolType,
		"source": "my-instance",
	}
	tools["list_indexes"] = map[string]any{
		"type":   PostgresListIndexesToolType,
		"source": "my-instance",
	}
	tools["list_sequences"] = map[string]any{
		"type":   PostgresListSequencesToolType,
		"source": "my-instance",
	}
	tools["list_publication_tables"] = map[string]any{
		"type":   PostgresListPublicationTablesToolType,
		"source": "my-instance",
	}
	tools["long_running_transactions"] = map[string]any{
		"type":   PostgresLongRunningTransactionsToolType,
		"source": "my-instance",
	}
	tools["list_locks"] = map[string]any{
		"type":   PostgresListLocksToolType,
		"source": "my-instance",
	}
	tools["replication_stats"] = map[string]any{
		"type":   PostgresReplicationStatsToolType,
		"source": "my-instance",
	}
	tools["list_query_stats"] = map[string]any{
		"type":   PostgresListQueryStatsToolType,
		"source": "my-instance",
	}
	tools["get_column_cardinality"] = map[string]any{
		"type":   PostgresGetColumnCardinalityToolType,
		"source": "my-instance",
	}

	tools["list_table_stats"] = map[string]any{
		"type":   PostgresListTableStats,
		"source": "my-instance",
	}

	tools["list_tablespaces"] = map[string]any{
		"type":   PostgresListTablespacesToolType,
		"source": "my-instance",
	}
	tools["list_pg_settings"] = map[string]any{
		"type":   PostgresListPGSettingsToolType,
		"source": "my-instance",
	}
	tools["list_database_stats"] = map[string]any{
		"type":   PostgresListDatabaseStatsToolType,
		"source": "my-instance",
	}

	tools["list_roles"] = map[string]any{
		"type":   PostgresListRolesToolType,
		"source": "my-instance",
	}

	tools["list_stored_procedure"] = map[string]any{
		"type":   PostgresListStoredProcedureToolType,
		"source": "my-instance",
	}
	config["tools"] = tools
	return config
}

func AddTemplateParamConfig(t *testing.T, config map[string]any, toolType, tmplSelectCombined, tmplSelectFilterCombined string, tmplSelectAll string) map[string]any {
	toolsMap, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}

	selectAll := "SELECT * FROM {{.tableName}} ORDER BY id"
	if tmplSelectAll != "" {
		selectAll = tmplSelectAll
	}

	toolsMap["create-table-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "CREATE TABLE {{.tableName}} ({{array .columns}})",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewArrayParameter("columns", "The columns to create", parameters.NewStringParameter("column", "A column name that will be created")),
		},
	}
	toolsMap["insert-table-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Insert tool with template parameters",
		"statement":   "INSERT INTO {{.tableName}} ({{array .columns}}) VALUES ({{.values}})",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewArrayParameter("columns", "The columns to insert into", parameters.NewStringParameter("column", "A column name that will be returned from the query.")),
			parameters.NewStringParameter("values", "The values to insert as a comma separated string"),
		},
	}
	toolsMap["select-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   selectAll,
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-templateParams-combined-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   tmplSelectCombined,
		"parameters":  []parameters.Parameter{parameters.NewIntParameter("id", "the id of the user")},
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-fields-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT {{array .fields}} FROM {{.tableName}} ORDER BY id",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewArrayParameter("fields", "The fields to select from", parameters.NewStringParameter("field", "A field that will be returned from the query.")),
		},
	}
	toolsMap["select-filter-templateParams-combined-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   tmplSelectFilterCombined,
		"parameters":  []parameters.Parameter{parameters.NewStringParameter("name", "the name of the user")},
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewStringParameter("columnFilter", "some description"),
		},
	}
	toolsMap["drop-table-templateParams-tool"] = map[string]any{
		"type":        toolType,
		"source":      "my-instance",
		"description": "Drop table tool with template parameters",
		"statement":   "DROP TABLE IF EXISTS {{.tableName}}",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	config["tools"] = toolsMap
	return config
}

// AddMySqlExecuteSqlConfig gets the tools config for `mysql-execute-sql`
func AddMySqlExecuteSqlConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "mysql-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "mysql-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	config["tools"] = tools
	return config
}

// AddMySQLPrebuiltToolConfig gets the tools config for mysql prebuilt tools
func AddMySQLPrebuiltToolConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["list_tables"] = map[string]any{
		"type":        "mysql-list-tables",
		"source":      "my-instance",
		"description": "Lists tables in the database.",
	}
	tools["list_active_queries"] = map[string]any{
		"type":        "mysql-list-active-queries",
		"source":      "my-instance",
		"description": "Lists active queries in the database.",
	}
	tools["list_tables_missing_unique_indexes"] = map[string]any{
		"type":        "mysql-list-tables-missing-unique-indexes",
		"source":      "my-instance",
		"description": "Lists tables that do not have primary or unique indexes in the database.",
	}
	tools["list_table_fragmentation"] = map[string]any{
		"type":        "mysql-list-table-fragmentation",
		"source":      "my-instance",
		"description": "Lists table fragmentation in the database.",
	}
	tools["list_table_stats"] = map[string]any{
		"type":        "mysql-list-table-stats",
		"source":      "my-instance",
		"description": "Lists table stats in the database.",
	}
	tools["get_query_plan"] = map[string]any{
		"type":        "mysql-get-query-plan",
		"source":      "my-instance",
		"description": "Gets the query plan for a SQL statement.",
	}
	tools["show_query_stats"] = map[string]any{
		"type":        "mysql-show-query-stats",
		"source":      "my-instance",
		"description": "Lists query statistics in the database.",
	}
	tools["list_all_locks"] = map[string]any{
		"type":        "mysql-list-all-locks",
		"source":      "my-instance",
		"description": "Lists all table, row locks in the database.",
	}
	config["tools"] = tools
	return config
}

// AddMSSQLExecuteSqlConfig gets the tools config for `mssql-execute-sql`
func AddMSSQLExecuteSqlConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "mssql-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "mssql-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	config["tools"] = tools
	return config
}

// AddMSSQLPrebuiltToolConfig gets the tools config for mssql prebuilt tools
func AddMSSQLPrebuiltToolConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["list_tables"] = map[string]any{
		"type":        "mssql-list-tables",
		"source":      "my-instance",
		"description": "Lists tables in the database.",
	}
	config["tools"] = tools
	return config
}

// GetPostgresSQLParamToolInfo returns statements and param for my-tool postgres-sql type
func GetPostgresSQLParamToolInfo(tableName string) (string, string, string, string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, name TEXT);", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name) VALUES ($1), ($2), ($3), ($4);", tableName)
	toolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = $1 OR name = $2;", tableName)
	idParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = $1;", tableName)
	nameParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = $1;", tableName)
	arrayToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ANY($1) AND name = ANY($2);", tableName)
	params := []any{"Alice", "Jane", "Sid", nil}
	return createStatement, insertStatement, toolStatement, idParamStatement, nameParamStatement, arrayToolStatement, params
}

// GetPostgresSQLAuthToolInfo returns statements and param of my-auth-tool for postgres-sql type
func GetPostgresSQLAuthToolInfo(tableName string) (string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, name TEXT, email TEXT);", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name, email) VALUES ($1, $2), ($3, $4)", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = $1;", tableName)
	params := []any{"Alice", ServiceAccountEmail, "Jane", "janedoe@gmail.com"}
	return createStatement, insertStatement, toolStatement, params
}

// GetPostgresSQLTmplToolStatement returns statements and param for template parameter test cases for postgres-sql type
func GetPostgresSQLTmplToolStatement() (string, string) {
	tmplSelectCombined := "SELECT * FROM {{.tableName}} WHERE id = $1"
	tmplSelectFilterCombined := "SELECT * FROM {{.tableName}} WHERE {{.columnFilter}} = $1"
	return tmplSelectCombined, tmplSelectFilterCombined
}

// GetCockroachDBParamToolInfo returns statements and param for my-tool cockroachdb-sql type
// Uses explicit INT PRIMARY KEY instead of SERIAL to ensure deterministic IDs
func GetCockroachDBParamToolInfo(tableName string) (string, string, string, string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT PRIMARY KEY, name TEXT);", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (id, name) VALUES (1, $1), (2, $2), (3, $3), (4, $4);", tableName)
	toolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = $1 OR name = $2 ORDER BY id;", tableName)
	idParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = $1;", tableName)
	nameParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = $1;", tableName)
	arrayToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ANY($1) AND name = ANY($2) ORDER BY id;", tableName)
	params := []any{"Alice", "Jane", "Sid", nil}
	return createStatement, insertStatement, toolStatement, idParamStatement, nameParamStatement, arrayToolStatement, params
}

// GetCockroachDBAuthToolInfo returns statements and param of my-auth-tool for cockroachdb-sql type
// Uses explicit INT PRIMARY KEY instead of SERIAL to ensure deterministic IDs
func GetCockroachDBAuthToolInfo(tableName string) (string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT PRIMARY KEY, name TEXT, email TEXT);", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (id, name, email) VALUES (1, $1, $2), (2, $3, $4)", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = $1;", tableName)
	params := []any{"Alice", ServiceAccountEmail, "Jane", "janedoe@gmail.com"}
	return createStatement, insertStatement, toolStatement, params
}

// GetCockroachDBWants return the expected wants for cockroachdb
func GetCockroachDBWants() (string, string, string, string) {
	select1Want := "[{\"?column?\":1}]"
	// CockroachDB formats syntax errors differently than PostgreSQL:
	// - Uses lowercase for SQL keywords in error messages
	// - Uses format: 'at or near "token": syntax error' instead of 'syntax error at or near "TOKEN"'
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: ERROR: at or near \"selec\": syntax error (SQLSTATE 42601)"}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id INT PRIMARY KEY, name TEXT)"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"?column?\":1}"}]}}`
	return select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want
}

// GetMSSQLParamToolInfo returns statements and param for my-tool mssql-sql type
func GetMSSQLParamToolInfo(tableName string) (string, string, string, string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT IDENTITY(1,1) PRIMARY KEY, name VARCHAR(255));", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name) VALUES (@alice), (@jane), (@sid), (@nil);", tableName)
	toolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = @id OR name = @p2;", tableName)
	idParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = @id;", tableName)
	nameParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = @name;", tableName)
	arrayToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ANY(@idArray) OR name = ANY(@p2);", tableName)
	params := []any{sql.Named("alice", "Alice"), sql.Named("jane", "Jane"), sql.Named("sid", "Sid"), sql.Named("nil", nil)}
	return createStatement, insertStatement, toolStatement, idParamStatement, nameParamStatement, arrayToolStatement, params
}

// GetMSSQLAuthToolInfo returns statements and param of my-auth-tool for mssql-sql type
func GetMSSQLAuthToolInfo(tableName string) (string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT IDENTITY(1,1) PRIMARY KEY, name VARCHAR(255), email VARCHAR(255));", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name, email) VALUES (@alice, @aliceemail), (@jane, @janeemail);", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = @email;", tableName)
	params := []any{sql.Named("alice", "Alice"), sql.Named("aliceemail", ServiceAccountEmail), sql.Named("jane", "Jane"), sql.Named("janeemail", "janedoe@gmail.com")}
	return createStatement, insertStatement, toolStatement, params
}

// GetMSSQLTmplToolStatement returns statements and param for template parameter test cases for mysql-sql type
func GetMSSQLTmplToolStatement() (string, string) {
	tmplSelectCombined := "SELECT * FROM {{.tableName}} WHERE id = @id"
	tmplSelectFilterCombined := "SELECT * FROM {{.tableName}} WHERE {{.columnFilter}} = @name"
	return tmplSelectCombined, tmplSelectFilterCombined
}

// GetMySQLParamToolInfo returns statements and param for my-tool mysql-sql type
func GetMySQLParamToolInfo(tableName string) (string, string, string, string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT NOT NULL AUTO_INCREMENT PRIMARY KEY, name VARCHAR(255));", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name) VALUES (?), (?), (?), (?);", tableName)
	toolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ? OR name = ?;", tableName)
	idParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ?;", tableName)
	nameParamStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = ?;", tableName)
	arrayToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = ANY(?) AND name = ANY(?);", tableName)
	params := []any{"Alice", "Jane", "Sid", nil}
	return createStatement, insertStatement, toolStatement, idParamStatement, nameParamStatement, arrayToolStatement, params
}

// GetMySQLAuthToolInfo returns statements and param of my-auth-tool for mysql-sql type
func GetMySQLAuthToolInfo(tableName string) (string, string, string, []any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT NOT NULL AUTO_INCREMENT PRIMARY KEY, name VARCHAR(255), email VARCHAR(255));", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (name, email) VALUES (?, ?), (?, ?)", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = ?;", tableName)
	params := []any{"Alice", ServiceAccountEmail, "Jane", "janedoe@gmail.com"}
	return createStatement, insertStatement, toolStatement, params
}

// GetMySQLTmplToolStatement returns statements and param for template parameter test cases for mysql-sql type
func GetMySQLTmplToolStatement() (string, string) {
	tmplSelectCombined := "SELECT * FROM {{.tableName}} WHERE id = ?"
	tmplSelectFilterCombined := "SELECT * FROM {{.tableName}} WHERE {{.columnFilter}} = ?"
	return tmplSelectCombined, tmplSelectFilterCombined
}

// GetPostgresWants return the expected wants for postgres
func GetPostgresWants() (string, string, string, string) {
	select1Want := "[{\"?column?\":1}]"
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: ERROR: syntax error at or near \"SELEC\" (SQLSTATE 42601)"}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id SERIAL PRIMARY KEY, name TEXT)"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"?column?\":1}"}]}}`
	return select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want
}

// GetMSSQLWants return the expected wants for mssql
func GetMSSQLWants() (string, string, string, string) {
	select1Want := "[{\"\":1}]"
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: mssql: Could not find stored procedure 'SELEC'."}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id INT IDENTITY(1,1) PRIMARY KEY, name NVARCHAR(MAX))"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"\":1}"}]}}`
	return select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want
}

// GetMySQLWants return the expected wants for mysql
func GetMySQLWants() (string, string, string, string) {
	select1Want := "[{\"1\":1}]"
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: Error 1064 (42000): You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near 'SELEC 1' at line 1"}],"isError":true}}`
	createTableStatement := `"CREATE TABLE t (id SERIAL PRIMARY KEY, name TEXT)"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"1\":1}"}]}}`
	return select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want
}

// SetupPostgresSQLTable creates and inserts data into a table of tool
// compatible with postgres-sql tool
func SetupPostgresSQLTable(t *testing.T, ctx context.Context, pool *pgxpool.Pool, createStatement, insertStatement, tableName string, params []any) func(*testing.T) {
	err := pool.Ping(ctx)
	if err != nil {
		t.Fatalf("unable to connect to test database: %s", err)
	}

	// Create table
	_, err = pool.Query(ctx, createStatement)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	_, err = pool.Query(ctx, insertStatement, params...)
	if err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}

	return func(t *testing.T) {
		// tear down test
		_, err = pool.Exec(context.WithoutCancel(ctx), fmt.Sprintf("DROP TABLE %s;", tableName))
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}
}

// SetupMsSQLTable creates and inserts data into a table of tool
// compatible with mssql-sql tool
func SetupMsSQLTable(t *testing.T, ctx context.Context, pool *sql.DB, createStatement, insertStatement, tableName string, params []any) func(*testing.T) {
	err := pool.PingContext(ctx)
	if err != nil {
		t.Fatalf("unable to connect to test database: %s", err)
	}

	// Create table
	_, err = pool.QueryContext(ctx, createStatement)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	_, err = pool.QueryContext(ctx, insertStatement, params...)
	if err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}

	return func(t *testing.T) {
		// tear down test
		_, err = pool.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("DROP TABLE %s;", tableName))
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}
}

// SetupMySQLTable creates and inserts data into a table of tool
// compatible with mysql-sql tool
func SetupMySQLTable(t *testing.T, ctx context.Context, pool *sql.DB, createStatement, insertStatement, tableName string, params []any) func(*testing.T) {
	err := pool.PingContext(ctx)
	if err != nil {
		t.Fatalf("unable to connect to test database: %s", err)
	}

	// Create table
	_, err = pool.QueryContext(ctx, createStatement)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	_, err = pool.QueryContext(ctx, insertStatement, params...)
	if err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}

	return func(t *testing.T) {
		// tear down test
		_, err = pool.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("DROP TABLE %s;", tableName))
		if err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	}
}

// GetRedisWants return the expected wants for redis
func GetRedisValkeyWants() (string, string, string, string, string, string, string) {
	select1Want := "[\"PONG\"]"
	mcpMyFailToolWant := `unknown command 'SELEC 1;', with args beginning with: \""}]}}`
	invokeParamWant := "[{\"id\":\"1\",\"name\":\"Alice\"},{\"id\":\"3\",\"name\":\"Sid\"}]"
	invokeIdNullWant := `[{"id":"4","name":""}]`
	nullWant := `["null"]`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"\"PONG\""}]}}`
	mcpInvokeParamWant := `{"jsonrpc":"2.0","id":"my-tool","result":{"content":[{"type":"text","text":"{\"id\":\"1\",\"name\":\"Alice\"}"},{"type":"text","text":"{\"id\":\"3\",\"name\":\"Sid\"}"}]}}`
	return select1Want, mcpMyFailToolWant, invokeParamWant, invokeIdNullWant, nullWant, mcpSelect1Want, mcpInvokeParamWant
}

func GetRedisValkeyToolsConfig(sourceConfig map[string]any, toolType string) map[string]any {
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"type":     "google",
				"clientId": ClientId,
			},
		},
		"tools": map[string]any{
			"my-simple-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
				"commands":    [][]string{{"PING"}},
			},
			"my-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"commands":    [][]string{{"HGETALL", "row1"}, {"HGETALL", "row3"}},
				"parameters": []any{
					map[string]any{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
					map[string]any{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
			},
			"my-tool-by-id": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"commands":    [][]string{{"HGETALL", "row4"}},
				"parameters": []any{
					map[string]any{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
				},
			},
			"my-tool-by-name": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"commands":    [][]string{{"GET", "null"}},
				"parameters": []any{
					map[string]any{
						"name":        "name",
						"type":        "string",
						"description": "user name",
						"required":    false,
					},
				},
			},
			"my-array-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with array params.",
				"commands":    [][]string{{"HGETALL", "row1"}, {"$cmdArray"}},
				"parameters": []any{
					map[string]any{
						"name":        "cmdArray",
						"type":        "array",
						"description": "cmd array",
						"items": map[string]any{
							"name":        "cmd",
							"type":        "string",
							"description": "field",
						},
					},
				},
			},
			"my-auth-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test authenticated parameters.",
				// statement to auto-fill authenticated parameter
				"commands": [][]string{{"HGETALL", "$email"}},
				"parameters": []map[string]any{
					{
						"name":        "email",
						"type":        "string",
						"description": "user email",
						"authServices": []map[string]string{
							{
								"name":  "my-google-auth",
								"field": "email",
							},
						},
					},
				},
			},
			"my-secure-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test secure parameters.",
				"commands":    [][]string{{"HGETALL", "row1"}, {"HGETALL", "row3"}},
				"parameters": []any{
					map[string]any{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
					map[string]any{
						"name":        "name",
						"type":        "string",
						"description": "user name",
						"secure":      true,
					},
				},
			},
			"my-auth-required-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test auth required invocation.",
				"commands":    [][]string{{"PING"}},
				"authRequired": []string{
					"my-google-auth",
				},
			},
			"my-fail-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test statement with incorrect syntax.",
				"commands":    [][]string{{"SELEC 1;"}},
			},
		},
	}
	return toolsFile
}

// TestCloudSQLMySQL_IPTypeParsingFromYAML verifies the IPType field parsing from YAML
// for the cloud-sql-mysql source, mimicking the structure of tests in cloudsql_mysql_test.go.
func TestCloudSQLMySQL_IPTypeParsingFromYAML(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "IPType Defaulting to Public",
			in: `
			kind: source
			name: my-mysql-instance
			type: cloud-sql-mysql
			project: my-project
			region: my-region
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
			`,
			want: server.SourceConfigs{
				"my-mysql-instance": cloudsqlmysql.Config{
					Name:     "my-mysql-instance",
					Type:     cloudsqlmysql.SourceType,
					Project:  "my-project",
					Region:   "my-region",
					Instance: "my-instance",
					IPType:   "public", // Default value
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
				},
			},
		},
		{
			desc: "IPType Explicit Public",
			in: `
			kind: source
			name: my-mysql-instance
			type: cloud-sql-mysql
			project: my-project
			region: my-region
			instance: my-instance
			ipType: Public
			database: my_db
			user: my_user
			password: my_pass
			`,
			want: server.SourceConfigs{
				"my-mysql-instance": cloudsqlmysql.Config{
					Name:     "my-mysql-instance",
					Type:     cloudsqlmysql.SourceType,
					Project:  "my-project",
					Region:   "my-region",
					Instance: "my-instance",
					IPType:   "public",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
				},
			},
		},
		{
			desc: "IPType Explicit Private",
			in: `
			kind: source
			name: my-mysql-instance
			type: cloud-sql-mysql
			project: my-project
			region: my-region
			instance: my-instance
			ipType: private
			database: my_db
			user: my_user
			password: my_pass
			`,
			want: server.SourceConfigs{
				"my-mysql-instance": cloudsqlmysql.Config{
					Name:     "my-mysql-instance",
					Type:     cloudsqlmysql.SourceType,
					Project:  "my-project",
					Region:   "my-region",
					Instance: "my-instance",
					IPType:   "private",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse: diff (-want +got):\n%s", cmp.Diff(tc.want, got))
			}
		})
	}
}

// Finds and drops all tables in a postgres database.
func CleanupPostgresTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool, uniqueID string) {

	query := `
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' 
		AND table_name LIKE $1;`
	rows, err := pool.Query(ctx, query, "%\\_"+uniqueID)
	if err != nil {
		t.Fatalf("Failed to query for tables for uniqueID %s in 'public' schema: %v", uniqueID, err)
	}
	defer rows.Close()

	var tablesToDrop []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Errorf("Failed to scan table name: %v", err)
			continue
		}
		tablesToDrop = append(tablesToDrop, fmt.Sprintf("public.%q", tableName))
	}

	if len(tablesToDrop) == 0 {
		t.Logf("INTEGRATION CLEANUP: No tables found for uniqueID: %s", uniqueID)
		return
	}

	t.Logf("INTEGRATION CLEANUP: Dropping %d isolated tables: %v", len(tablesToDrop), tablesToDrop)

	dropQuery := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;", strings.Join(tablesToDrop, ", "))

	if _, err := pool.Exec(ctx, dropQuery); err != nil {
		t.Fatalf("Failed to drop tables for uniqueID %s in 'public' schema: %v", uniqueID, err)
	} else {
		t.Logf("INTEGRATION CLEANUP SUCCESS: Wiped all tables for uniqueID: %s", uniqueID)
	}
}

// Finds and drops all tables in a mysql database.
func CleanupMySQLTables(t *testing.T, ctx context.Context, pool *sql.DB) {
	query := `
	SELECT table_name FROM information_schema.tables
	WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE';`

	rows, err := pool.QueryContext(ctx, query)
	if err != nil {
		t.Fatalf("Failed to query for all MySQL tables: %v", err)
	}
	defer rows.Close()

	var tablesToDrop []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Errorf("Failed to scan MySQL table name: %v", err)
			continue
		}
		tablesToDrop = append(tablesToDrop, fmt.Sprintf("`%s`", tableName))
	}

	if len(tablesToDrop) == 0 {
		return
	}

	// Disable foreign key checks, drop all tables and re-enable
	if _, err := pool.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0;"); err != nil {
		t.Fatalf("Failed to disable MySQL foreign key checks: %v", err)
	}

	dropQuery := fmt.Sprintf("DROP TABLE IF EXISTS %s;", strings.Join(tablesToDrop, ", "))

	if _, err := pool.ExecContext(ctx, dropQuery); err != nil {
		// Try to re-enable checks even if drop fails
		if _, err := pool.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1;"); err != nil {
			t.Logf("Also failed to re-enable foreign key checks: %v", err)
		}
		t.Fatalf("Failed to drop all MySQL tables: %v", err)
	}

	// Re-enable foreign key checks
	if _, err := pool.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1;"); err != nil {
		t.Fatalf("Failed to re-enable MySQL foreign key checks: %v", err)
	}
}

// Finds and drops all tables in an mssql database.
func CleanupMSSQLTables(t *testing.T, ctx context.Context, pool *sql.DB) {
	disableConstraintsCmd := "EXEC sp_MSforeachtable 'ALTER TABLE ? NOCHECK CONSTRAINT ALL'"
	if _, err := pool.ExecContext(ctx, disableConstraintsCmd); err != nil {
		t.Fatalf("Failed to disable MSSQL constraints: %v", err)
	}

	// drop 'U' (User Tables)
	dropTablesCmd := "EXEC sp_MSforeachtable 'DROP TABLE ?', @whereand = 'AND O.Type = ''U'''"
	if _, err := pool.ExecContext(ctx, dropTablesCmd); err != nil {
		t.Fatalf("Failed to drop all MSSQL tables: %v", err)
	}

}

func CleanupBigQueryDatasets(t *testing.T, ctx context.Context, client *bigquery.Client, datasetIDs []string) {
	for _, id := range datasetIDs {
		t.Logf("INTEGRATION CLEANUP: Purging dataset %s", id)
		ds := client.Dataset(id)

		//Delete tables first since Dataset.Delete fails if not empty
		tableIt := ds.Tables(ctx)
		for {
			table, err := tableIt.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Errorf("INTEGRATION CLEANUP: Failed to iterate tables in %s: %v", id, err)
				break
			}
			if err := table.Delete(ctx); err != nil {
				t.Errorf("INTEGRATION CLEANUP: Failed to delete table %s: %v", table.TableID, err)
			}
		}
		//delete empty dataset
		if err := ds.Delete(ctx); err != nil {
			t.Errorf("INTEGRATION CLEANUP: Failed to delete dataset %s: %v", id, err)
		} else {
			t.Logf("INTEGRATION CLEANUP SUCCESS: Wiped dataset %s", id)
		}
	}
}

// finds and deletes all tables in a Bigtable instance that match the uniqueID.
func CleanupBigtableTables(t *testing.T, ctx context.Context, adminClient *bigtable.AdminClient, uniqueID string) {
	tables, err := adminClient.Tables(ctx)
	if err != nil {
		t.Errorf("INTEGRATION CLEANUP: Failed to list tables: %v", err)
		return
	}

	for _, table := range tables {
		// Delete tables containing our uniqueID
		if strings.Contains(table, uniqueID) {
			t.Logf("INTEGRATION CLEANUP: Deleting table %s", table)
			if err := adminClient.DeleteTable(ctx, table); err != nil {
				t.Errorf("INTEGRATION CLEANUP: Failed to delete table %s: %v", table, err)
			} else {
				t.Logf("INTEGRATION CLEANUP SUCCESS: Wiped table %s", table)
			}
		}
	}
}

// SetupPostgresTestContainer spins up a generic PostgreSQL-compatible test container.
func SetupGenericPostgresTestContainer(ctx context.Context, t *testing.T, req testcontainers.ContainerRequest) (string, string, func()) {
	t.Helper()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start postgres container: %s", err)
	}

	cleanup := func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanupCancel()
		if err := container.Terminate(cleanupCtx); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	}

	host, err := container.Host(ctx)
	if err != nil {
		cleanup()
		t.Fatalf("failed to get container host: %s", err)
	}

	mappedPort, err := container.MappedPort(ctx, "5432")
	if err != nil {
		cleanup()
		t.Fatalf("failed to get container mapped port: %s", err)
	}

	return host, mappedPort.Port(), cleanup
}
