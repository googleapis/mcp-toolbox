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

package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// TestSQLitePrebuiltListTablesNamesStartingWithSQLite checks that the
// prebuilt list_tables tool hides only SQLite's internal tables (names that
// start with "sqlite_"), and still lists user tables whose names merely start
// with "sqlite", such as "sqlite3_cache" or "SQLiteSettings".
func TestSQLitePrebuiltListTablesNamesStartingWithSQLite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	dbPath := filepath.Join(t.TempDir(), "names.db")
	setupDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening sqlite db failed: %v", err)
	}
	setupStatements := []string{
		// AUTOINCREMENT creates the internal sqlite_sequence table.
		`CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL);`,
		`CREATE TABLE sqlite3_cache (k TEXT PRIMARY KEY, v BLOB);`,
		`CREATE INDEX sqlite3_cache_v ON sqlite3_cache(v);`,
		`CREATE TABLE SQLiteSettings (id INTEGER PRIMARY KEY, value TEXT);`,
		`INSERT INTO users (name) VALUES ('Alice');`,
		// ANALYZE creates the internal sqlite_stat1 table.
		`ANALYZE;`,
	}
	for _, stmt := range setupStatements {
		if _, err := setupDB.ExecContext(ctx, stmt); err != nil {
			_ = setupDB.Close()
			t.Fatalf("executing setup SQL %q failed: %v", stmt, err)
		}
	}
	if err := setupDB.Close(); err != nil {
		t.Fatalf("closing setup sqlite db failed: %v", err)
	}

	t.Setenv("SQLITE_DATABASE", filepath.ToSlash(dbPath))

	args := []string{"--prebuilt", "sqlite", "--enable-api"}
	cmd, cleanup, err := tests.StartCmd(ctx, map[string]any{}, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %v", err)
	}
	defer cleanup()

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs:\n%s", out)
		t.Fatalf("toolbox didn't start successfully: %v", err)
	}

	t.Run("all tables are listed and internal tables are hidden", func(t *testing.T) {
		rows := invokePrebuiltSQLiteListTables(t, map[string]any{
			"output_format": "simple",
		})
		var got []string
		for _, r := range rows {
			name, _ := r.ObjectDetails["name"].(string)
			got = append(got, name)
		}
		slices.Sort(got)
		want := []string{"SQLiteSettings", "sqlite3_cache", "users"}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("table names mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("table_names finds a table whose name starts with sqlite", func(t *testing.T) {
		rows := invokePrebuiltSQLiteListTables(t, map[string]any{
			"table_names": "sqlite3_cache",
		})
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d: %+v", len(rows), rows)
		}
		details := rows[0].ObjectDetails
		if details["object_name"] != "sqlite3_cache" {
			t.Fatalf("expected object_name 'sqlite3_cache', got %v", details["object_name"])
		}
		cols, ok := details["columns"].([]any)
		if !ok || len(cols) != 2 {
			t.Errorf("expected 2 columns for sqlite3_cache, got %v", details["columns"])
		}
		indexes, ok := details["indexes"].([]any)
		if !ok || len(indexes) != 2 {
			t.Errorf("expected 2 indexes for sqlite3_cache, got %v", details["indexes"])
		}
	})
}
