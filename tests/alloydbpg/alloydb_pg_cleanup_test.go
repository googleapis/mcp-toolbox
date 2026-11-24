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

package alloydbpg

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/googleapis/mcp-toolbox/tests"
)

func TestRegisterTeardownIdempotencyAndLIFO(t *testing.T) {
	var (
		calls atomic.Int32
		order []string
	)

	t.Run("explicit teardown and t.Cleanup execute once in LIFO order", func(t *testing.T) {
		t.Cleanup(func() {
			order = append(order, "close-pool-and-dialer")
		})
		dropTable := tests.RegisterTeardown(t, func() {
			calls.Add(1)
			order = append(order, "drop-table")
		})
		rollbackTx := tests.RegisterTeardown(t, func() {
			order = append(order, "rollback-tx")
		})

		// Calling dropTable explicitly once still leaves t.Cleanup as a no-op
		// second invocation while rollbackTx runs during t.Cleanup.
		rollbackTx()
		dropTable()
		dropTable()
	})

	if got := calls.Load(); got != 1 {
		t.Fatalf("RegisterTeardown callback invoked %d times, want 1", got)
	}
	wantOrder := []string{"rollback-tx", "drop-table", "close-pool-and-dialer"}
	if len(order) != len(wantOrder) {
		t.Fatalf("order = %v, want %v", order, wantOrder)
	}
	for i, want := range wantOrder {
		if order[i] != want {
			t.Errorf("order[%d] = %q, want %q (all: %v)", i, order[i], want, order)
		}
	}
}

func TestGetPostgresExecuteSqlCreateTableStatement(t *testing.T) {
	const tableName = "exec_sql_table_abc123"
	got := tests.GetPostgresExecuteSqlCreateTableStatement(tableName)
	if !strings.Contains(got, "CREATE TABLE "+tableName+" ") {
		t.Fatalf("GetPostgresExecuteSqlCreateTableStatement(%q) = %s, want table name embedded", tableName, got)
	}
}

func TestGetAlloyDBDialOpts(t *testing.T) {
	for _, valid := range []string{"public", "PUBLIC", "private", "Private"} {
		opts, err := getAlloyDBDialOpts(valid)
		if err != nil {
			t.Errorf("getAlloyDBDialOpts(%q) returned unexpected error: %v", valid, err)
		}
		if len(opts) != 1 {
			t.Errorf("getAlloyDBDialOpts(%q) returned %d options, want 1", valid, len(opts))
		}
	}
	if _, err := getAlloyDBDialOpts("invalid"); err == nil {
		t.Errorf("expected error for invalid ipType, got nil")
	}
}
