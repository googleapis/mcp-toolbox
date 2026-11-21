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
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// TestSQLitePrebuiltListTablesGeneratedColumns checks that the prebuilt
// list_tables tool reports generated columns (STORED and VIRTUAL) together
// with the regular columns, at their declared positions.
func TestSQLitePrebuiltListTablesGeneratedColumns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	dbPath := filepath.Join(t.TempDir(), "generated.db")
	setupDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening sqlite db failed: %v", err)
	}
	createStmt := `CREATE TABLE line_items (
		id INTEGER PRIMARY KEY,
		qty INTEGER NOT NULL,
		price REAL NOT NULL,
		total REAL GENERATED ALWAYS AS (qty * price) STORED,
		note TEXT,
		total_virtual REAL GENERATED ALWAYS AS (qty * price) VIRTUAL
	);`
	if _, err := setupDB.ExecContext(ctx, createStmt); err != nil {
		_ = setupDB.Close()
		t.Fatalf("creating test table failed: %v", err)
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

	reqBody, err := json.Marshal(map[string]any{"table_names": "line_items"})
	if err != nil {
		t.Fatalf("unable to marshal request payload: %v", err)
	}
	api := "http://127.0.0.1:5000/api/tool/list_tables/invoke"
	resp, err := http.Post(api, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("unable to send request: %v", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("unable to read response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var wrapper struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		t.Fatalf("unable to decode response (%s): %v", string(respBody), err)
	}
	var resultStr string
	if err := json.Unmarshal(wrapper.Result, &resultStr); err != nil {
		resultStr = string(wrapper.Result)
	}
	var rows []struct {
		ObjectDetails struct {
			Columns []struct {
				ColumnName      string `json:"column_name"`
				OrdinalPosition int    `json:"ordinal_position"`
			} `json:"columns"`
		} `json:"object_details"`
	}
	if err := json.Unmarshal([]byte(resultStr), &rows); err != nil {
		t.Fatalf("unable to unmarshal list_tables rows (%s): %v", resultStr, err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d: %s", len(rows), resultStr)
	}

	var got []string
	for i, c := range rows[0].ObjectDetails.Columns {
		if c.OrdinalPosition != i {
			t.Errorf("column %q: got ordinal_position %d, want %d", c.ColumnName, c.OrdinalPosition, i)
		}
		got = append(got, c.ColumnName)
	}
	want := []string{"id", "qty", "price", "total", "note", "total_virtual"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected columns: got %v, want %v (result: %s)", got, want, resultStr)
	}
}
