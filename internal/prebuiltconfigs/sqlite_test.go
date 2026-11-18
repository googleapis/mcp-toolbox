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

package prebuiltconfigs_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/prebuiltconfigs"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources/sqlite"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/sqlite/sqliteexecutesql"
	"github.com/googleapis/mcp-toolbox/internal/tools/sqlite/sqlitesql"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.opentelemetry.io/otel/trace/noop"
)

type listTablesRow struct {
	ObjectDetails map[string]any `json:"object_details"`
}

func invokePrebuiltListTables(t *testing.T, ctx context.Context, tool tools.Tool, src *sqlite.Source, args map[string]any) []listTablesRow {
	t.Helper()
	toolParams, err := tool.GetParameters(src)
	if err != nil {
		t.Fatalf("GetParameters failed: %v", err)
	}
	parsedParams, err := parameters.ParseParams(toolParams, args, nil)
	if err != nil {
		t.Fatalf("ParseParams(%v) failed: %v", args, err)
	}
	res, tbErr := tool.Invoke(ctx, src, parsedParams, "")
	if tbErr != nil {
		t.Fatalf("Invoke(%v) failed: %v", args, tbErr)
	}
	encoded, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var rows []listTablesRow
	if err := json.Unmarshal(encoded, &rows); err != nil {
		t.Fatalf("json.Unmarshal(%s) failed: %v", encoded, err)
	}
	return rows
}

func assertDatabaseStateIntact(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()

	for _, table := range []string{"users", "orders"} {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			t.Fatalf("expected table %q to remain intact, query failed: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected 1 row in %q, got %d", table, count)
		}
	}

	rows, err := db.QueryContext(ctx, "PRAGMA database_list")
	if err != nil {
		t.Fatalf("PRAGMA database_list failed: %v", err)
	}
	defer rows.Close()

	var dbNames []string
	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			t.Fatalf("scanning database_list failed: %v", err)
		}
		dbNames = append(dbNames, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating database_list failed: %v", err)
	}
	if diff := cmp.Diff([]string{"main"}, dbNames); diff != "" {
		t.Fatalf("unexpected attached databases (-want +got):\n%s", diff)
	}
}

func TestSQLitePrebuiltListTables(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("ContextWithNewLogger failed: %v", err)
	}

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "primary.db")
	secondaryPath := filepath.Join(tempDir, "secondary.db")

	secDB, err := sql.Open("sqlite", secondaryPath)
	if err != nil {
		t.Fatalf("opening secondary db failed: %v", err)
	}
	if _, err := secDB.ExecContext(ctx, "CREATE TABLE archives (id INTEGER PRIMARY KEY, label TEXT);"); err != nil {
		_ = secDB.Close()
		t.Fatalf("creating secondary table failed: %v", err)
	}
	if err := secDB.Close(); err != nil {
		t.Fatalf("closing secondary db failed: %v", err)
	}

	raw, err := prebuiltconfigs.Get("sqlite")
	if err != nil {
		t.Fatalf("prebuiltconfigs.Get(\"sqlite\") failed: %v", err)
	}
	configuredYAML := bytes.ReplaceAll(raw, []byte("${SQLITE_DATABASE}"), []byte(filepath.ToSlash(dbPath)))

	sourceCfgs, _, _, toolCfgs, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, configuredYAML)
	if err != nil {
		t.Fatalf("UnmarshalPrimitiveConfig failed: %v", err)
	}

	srcCfg, ok := sourceCfgs["sqlite-source"]
	if !ok {
		t.Fatalf("sqlite-source not found in prebuilt config")
	}
	srcIface, err := srcCfg.Initialize(ctx, noop.NewTracerProvider().Tracer("test"))
	if err != nil {
		t.Fatalf("initializing sqlite-source failed: %v", err)
	}
	src, ok := srcIface.(*sqlite.Source)
	if !ok {
		t.Fatalf("expected *sqlite.Source, got %T", srcIface)
	}
	t.Cleanup(func() {
		if closeErr := src.SQLiteDB().Close(); closeErr != nil {
			t.Errorf("closing sqlite db: %v", closeErr)
		}
	})

	setupStatements := []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL);`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, total REAL, FOREIGN KEY(user_id) REFERENCES users(id));`,
		`INSERT INTO users (id, name) VALUES (1, 'Alice');`,
		`INSERT INTO orders (id, user_id, total) VALUES (1, 1, 19.95);`,
	}
	for _, stmt := range setupStatements {
		if _, err := src.SQLiteDB().ExecContext(ctx, stmt); err != nil {
			t.Fatalf("executing setup SQL %q failed: %v", stmt, err)
		}
	}

	listTablesCfgIface, ok := toolCfgs["list_tables"]
	if !ok {
		t.Fatalf("list_tables tool not found in sqlite prebuilt config")
	}
	listTablesCfg, ok := listTablesCfgIface.(sqlitesql.Config)
	if !ok {
		t.Fatalf("expected sqlitesql.Config, got %T", listTablesCfgIface)
	}
	if len(listTablesCfg.TemplateParameters) != 0 {
		t.Fatalf("expected 0 templateParameters on list_tables, got %d", len(listTablesCfg.TemplateParameters))
	}
	if len(listTablesCfg.Parameters) != 2 {
		t.Fatalf("expected 2 bound parameters on list_tables, got %d", len(listTablesCfg.Parameters))
	}

	listTablesTool, err := listTablesCfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("initializing list_tables failed: %v", err)
	}
	if err := listTablesTool.ValidateSource(src); err != nil {
		t.Fatalf("ValidateSource failed: %v", err)
	}

	t.Run("default arguments return all tables in detailed format", func(t *testing.T) {
		rows := invokePrebuiltListTables(t, ctx, listTablesTool, src, map[string]any{})
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d: %+v", len(rows), rows)
		}
		var gotNames []string
		for _, r := range rows {
			name, _ := r.ObjectDetails["object_name"].(string)
			gotNames = append(gotNames, name)
			if r.ObjectDetails["schema_name"] != "main" {
				t.Errorf("expected schema_name 'main', got %v", r.ObjectDetails["schema_name"])
			}
			if r.ObjectDetails["object_type"] != "table" {
				t.Errorf("expected object_type 'table', got %v", r.ObjectDetails["object_type"])
			}
			cols, ok := r.ObjectDetails["columns"].([]any)
			if !ok || len(cols) == 0 {
				t.Errorf("expected non-empty columns slice for %q, got %T (%v)", name, r.ObjectDetails["columns"], r.ObjectDetails["columns"])
			}
		}
		slices.Sort(gotNames)
		if diff := cmp.Diff([]string{"orders", "users"}, gotNames); diff != "" {
			t.Fatalf("table names mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("simple output_format returns table names only", func(t *testing.T) {
		rows := invokePrebuiltListTables(t, ctx, listTablesTool, src, map[string]any{
			"output_format": "simple",
		})
		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d: %+v", len(rows), rows)
		}
		var gotNames []string
		for _, r := range rows {
			if len(r.ObjectDetails) != 1 {
				t.Errorf("expected single 'name' key in simple output, got %v", r.ObjectDetails)
			}
			name, _ := r.ObjectDetails["name"].(string)
			gotNames = append(gotNames, name)
		}
		slices.Sort(gotNames)
		if diff := cmp.Diff([]string{"orders", "users"}, gotNames); diff != "" {
			t.Fatalf("table names mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("table_names filters matching tables", func(t *testing.T) {
		rows := invokePrebuiltListTables(t, ctx, listTablesTool, src, map[string]any{
			"table_names": "users",
		})
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d: %+v", len(rows), rows)
		}
		if rows[0].ObjectDetails["object_name"] != "users" {
			t.Fatalf("expected object_name 'users', got %v", rows[0].ObjectDetails["object_name"])
		}

		bothRows := invokePrebuiltListTables(t, ctx, listTablesTool, src, map[string]any{
			"output_format": "simple",
			"table_names":   "orders,users",
		})
		if len(bothRows) != 2 {
			t.Fatalf("expected 2 rows for comma-separated table_names, got %d", len(bothRows))
		}
	})

	t.Run("special characters and SQL statements in parameters are bound as literals", func(t *testing.T) {
		secondarySlashPath := filepath.ToSlash(secondaryPath)

		tableFilterInputs := []string{
			"', 'orders') > 0; DROP TABLE users;",
			"nonexistent', ',nonexistent,') > 0; DROP TABLE users; SELECT instr('a",
			fmt.Sprintf("', 'orders') > 0; ATTACH DATABASE '%s' AS extra_db;", secondarySlashPath),
			fmt.Sprintf("nonexistent', ',nonexistent,') > 0; ATTACH DATABASE '%s' AS extra_db; SELECT instr('a", secondarySlashPath),
		}
		for _, filterVal := range tableFilterInputs {
			rows := invokePrebuiltListTables(t, ctx, listTablesTool, src, map[string]any{
				"table_names": filterVal,
			})
			if len(rows) != 0 {
				t.Fatalf("expected 0 rows for literal table_names %q, got %d: %+v", filterVal, len(rows), rows)
			}
			assertDatabaseStateIntact(t, ctx, src.SQLiteDB())
		}

		outputFormatInputs := []string{
			"simple' = 'simple' THEN json_object('name', 'other') WHEN 'a",
			"detailed'; DROP TABLE users;",
			fmt.Sprintf("simple'; ATTACH DATABASE '%s' AS extra_db;", secondarySlashPath),
		}
		for _, formatVal := range outputFormatInputs {
			rows := invokePrebuiltListTables(t, ctx, listTablesTool, src, map[string]any{
				"output_format": formatVal,
				"table_names":   "users",
			})
			if len(rows) != 1 {
				t.Fatalf("expected 1 detailed row for literal output_format %q, got %d", formatVal, len(rows))
			}
			if rows[0].ObjectDetails["object_name"] != "users" {
				t.Fatalf("expected object_name 'users' for literal output_format %q, got %v", formatVal, rows[0].ObjectDetails)
			}
			assertDatabaseStateIntact(t, ctx, src.SQLiteDB())
		}
	})
}
