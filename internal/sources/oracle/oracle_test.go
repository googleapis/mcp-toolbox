// Copyright © 2025, Oracle and/or its affiliates.

package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	_ "modernc.org/sqlite"
)

func TestParseFromYamlOracle(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "connection string and useOCI=true",
			in: `
			kind: source
			name: my-oracle-cs
			type: oracle
			connectionString: "my-host:1521/XEPDB1"
			user: my_user
			password: my_pass
			useOCI: true
			`,
			want: map[string]sources.SourceConfig{
				"my-oracle-cs": Config{
					Name:             "my-oracle-cs",
					Type:             SourceType,
					ConnectionString: "my-host:1521/XEPDB1",
					User:             "my_user",
					Password:         "my_pass",
					UseOCI:           true,
				},
			},
		},
		{
			desc: "host/port/serviceName and default useOCI=false",
			in: `
			kind: source
			name: my-oracle-host
			type: oracle
			host: my-host
			port: 1521
			serviceName: ORCLPDB
			user: my_user
			password: my_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-oracle-host": Config{
					Name:        "my-oracle-host",
					Type:        SourceType,
					Host:        "my-host",
					Port:        1521,
					ServiceName: "ORCLPDB",
					User:        "my_user",
					Password:    "my_pass",
					UseOCI:      false,
				},
			},
		},
		{
			desc: "tnsAlias and TnsAdmin specified with explicit useOCI=true",
			in: `
			kind: source
			name: my-oracle-tns-oci
			type: oracle
			tnsAlias: FINANCE_DB
			tnsAdmin: /opt/oracle/network/admin
			user: my_user
			password: my_pass
			useOCI: true 
			`,
			want: map[string]sources.SourceConfig{
				"my-oracle-tns-oci": Config{
					Name:     "my-oracle-tns-oci",
					Type:     SourceType,
					TnsAlias: "FINANCE_DB",
					TnsAdmin: "/opt/oracle/network/admin",
					User:     "my_user",
					Password: "my_pass",
					UseOCI:   true,
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
				t.Fatalf("incorrect parse:\nwant: %v\ngot:  %v\ndiff: %s", tc.want, got, cmp.Diff(tc.want, got))
			}
		})
	}
}

func TestBuildGoOraConnString(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name           string
		user           string
		password       string
		connectBase    string
		walletLocation string
		want           string
	}{
		{
			name:           "encodes_credentials_and_wallet",
			user:           "user[client]",
			password:       "pa:ss@word",
			connectBase:    "dbhost:1521/XEPDB1",
			walletLocation: "/tmp/my wallet",
			want:           "oracle://user%5Bclient%5D:pa%3Ass%40word@dbhost:1521/XEPDB1?ssl=true&wallet=%2Ftmp%2Fmy+wallet",
		},
		{
			name:        "no_wallet",
			user:        "scott",
			password:    "tiger",
			connectBase: "dbhost:1521/ORCL",
			want:        "oracle://scott:tiger@dbhost:1521/ORCL",
		},
		{
			name:        "does_not_double_encode_percent_encoded_user",
			user:        "app_user%5BCLIENT_A%5D",
			password:    "secret",
			connectBase: "dbhost:1521/ORCL",
			want:        "oracle://app_user%5BCLIENT_A%5D:secret@dbhost:1521/ORCL",
		},
		{
			name:           "uses_trimmed_wallet_location",
			user:           "scott",
			password:       "tiger",
			connectBase:    "dbhost:1521/ORCL",
			walletLocation: "  /tmp/wallet  ",
			want:           "oracle://scott:tiger@dbhost:1521/ORCL?ssl=true&wallet=%2Ftmp%2Fwallet",
		},
		{
			name:           "appends_wallet_query_to_existing_query",
			user:           "scott",
			password:       "tiger",
			connectBase:    "dbhost:1521/ORCL?custom_opt=true",
			walletLocation: " /tmp/wallet ",
			want:           "oracle://scott:tiger@dbhost:1521/ORCL?custom_opt=true&ssl=true&wallet=%2Ftmp%2Fwallet",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildGoOraConnString(tc.user, tc.password, tc.connectBase, tc.walletLocation)
			if got != tc.want {
				t.Fatalf("buildGoOraConnString() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFailParseFromYaml(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-oracle-instance
			type: oracle
			host: my-host
			serviceName: ORCL
			user: my_user
			password: my_pass
			extraField: value
			`,
			err: "error unmarshaling source: unable to parse source \"my-oracle-instance\" as \"oracle\": [1:1] unknown field \"extraField\"\n>  1 | extraField: value\n       ^\n   2 | host: my-host\n   3 | name: my-oracle-instance\n   4 | password: my_pass\n   5 | ",
		},
		{
			desc: "missing required password field",
			in: `
			kind: source
			name: my-oracle-instance
			type: oracle
			host: my-host
			serviceName: ORCL
			user: my_user
			`,
			err: "error unmarshaling source: unable to parse source \"my-oracle-instance\" as \"oracle\": Key: 'Config.Password' Error:Field validation for 'Password' failed on the 'required' tag",
		},
		{
			desc: "missing connection method fields (validate fails)",
			in: `
			kind: source
			name: my-oracle-instance
			type: oracle
			user: my_user
			password: my_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-oracle-instance\" as \"oracle\": invalid Oracle configuration: must provide one of: 'tns_alias', 'connection_string', or both 'host' and 'service_name'",
		},
		{
			desc: "multiple connection methods provided (validate fails)",
			in: `
			kind: source
			name: my-oracle-instance
			type: oracle
			host: my-host
			serviceName: ORCL
			connectionString: "my-host:1521/XEPDB1"
			user: my_user
			password: my_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-oracle-instance\" as \"oracle\": invalid Oracle configuration: provide only one connection method: 'tns_alias', 'connection_string', or 'host'+'service_name'",
		},
		{
			desc: "fail on tnsAdmin with useOCI=false",
			in: `
			kind: source
			name: my-oracle-fail
			type: oracle
			tnsAlias: FINANCE_DB
			tnsAdmin: /opt/oracle/network/admin
			user: my_user
			password: my_pass
			useOCI: false
			`,
			err: "error unmarshaling source: unable to parse source \"my-oracle-fail\" as \"oracle\": invalid Oracle configuration: `tnsAdmin` can only be used when `UseOCI` is true, or use `walletLocation` instead",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := strings.ReplaceAll(err.Error(), "\r", "")

			if errStr != tc.err {
				t.Fatalf("unexpected error:\ngot:\n%q\nwant:\n%q\n", errStr, tc.err)
			}
		})
	}
}

// TestRunSQLExecutesDML verifies that RunSQL correctly routes operations to
// ExecContext instead of QueryContext when the readOnly flag is set to false.
func TestRunSQLExecutesDML(t *testing.T) {
	// Initialize a mock database connection.
	// This connection is not established with a real backend but
	// satisfies the interface requirements for the test.
	db, err := sql.Open("oracle", "oracle://user:pass@localhost:1521/service")
	if err != nil {
		t.Fatalf("failed to open mock db: %v", err)
	}
	defer db.Close()

	src := &Source{
		Config: Config{
			Name: "test-dml-source",
			Type: SourceType,
			User: "test-user",
		},
		DB: db,
	}

	// Invoke RunSQL with readOnly=false to force the DML execution path.
	_, err = src.RunSQL(context.Background(),
		"UPDATE users SET email='x' WHERE id=1", nil, false)

	// We expect an error because the mock database cannot execute the query.
	// If err is nil, it implies the logic skipped the execution block.
	if err == nil {
		t.Fatal("expected error from fake DB execution, but got nil; " +
			"DML path may not have been executed")
	}
}

func TestValidateReadOnlySQL(t *testing.T) {
	t.Parallel()

	allowed := []string{
		"SELECT 1 FROM DUAL",
		"  ( SELECT * FROM users ) ; ",
		"WITH cte AS (SELECT 1 AS n FROM DUAL) SELECT n FROM cte",
		"-- comment\n/* block */ SELECT 'a;b', n'c;d', q'[e;f]', nq'{g;h}', \"i;j\", 'FOR UPDATE', u.update FROM users u WHERE name = :q",
	}
	for _, q := range allowed {
		if err := validateReadOnlySQL(q); err != nil {
			t.Errorf("validateReadOnlySQL(%q) unexpected error: %v", q, err)
		}
	}

	blocked := []string{
		"   ;  ",
		"SELEC 1;",
		"INSERT INTO users (id) VALUES (1)",
		"UPDATE users SET name = 'x' WHERE id = 1",
		"DELETE FROM users WHERE id = 1",
		"MERGE INTO users u USING src s ON (u.id = s.id) WHEN MATCHED THEN UPDATE SET u.name = s.name",
		"DROP TABLE users",
		"CREATE TABLE t (id NUMBER)",
		"ALTER TABLE users ADD col NUMBER",
		"TRUNCATE TABLE users",
		"BEGIN NULL; END;",
		"DECLARE x NUMBER; BEGIN NULL; END;",
		"CALL my_proc()",
		"/* comment */ -- line\nDROP TABLE users",
		"WITH FUNCTION f RETURN NUMBER IS BEGIN RETURN 1; END; SELECT f() FROM DUAL",
		"SELECT /*+ WITH_PLSQL */ * FROM (WITH PROCEDURE p IS BEGIN NULL; END; SELECT 1 FROM DUAL)",
		"SELECT * FROM users FOR UPDATE",
		"SELECT * FROM users FOR /* lock */ UPDATE NOWAIT",
		"SELECT 1 FROM DUAL; DROP TABLE users",
		"SELECT aq'[x]'; DROP TABLE users -- ]'",
		"SELECT 'unterminated FROM DUAL",
		"SELECT /* unterminated FROM DUAL",
	}
	for _, q := range blocked {
		if err := validateReadOnlySQL(q); err == nil {
			t.Errorf("validateReadOnlySQL(%q) = nil, expected error", q)
		}
	}
}

func TestRunSQLReadOnlyEnforcementAndReadWrite(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "CREATE TABLE users (id TEXT PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO users (id, name) VALUES ('1', 'Alice'), ('2', 'Bob')"); err != nil {
		t.Fatalf("failed to seed rows: %v", err)
	}

	src := &Source{
		Config: Config{Name: "test-oracle", Type: SourceType, User: "u"},
		DB:     db,
	}

	// Read-only SELECT succeeds.
	gotRows, err := src.RunSQL(ctx, "SELECT id, name FROM users ORDER BY id", nil, true)
	if err != nil {
		t.Fatalf("RunSQL(readOnly=true, SELECT) failed: %v", err)
	}
	wantRows := []any{
		map[string]any{"id": "1", "name": "Alice"},
		map[string]any{"id": "2", "name": "Bob"},
	}
	if diff := cmp.Diff(wantRows, gotRows); diff != "" {
		t.Fatalf("RunSQL(readOnly=true, SELECT) mismatch (-want +got):\n%s", diff)
	}

	// Read-only DML, DDL, and stacked statements are blocked without mutating data.
	blockedStmts := []string{
		"DELETE FROM users WHERE id = '1'",
		"INSERT INTO users (id, name) VALUES ('3', 'Eve')",
		"UPDATE users SET name = 'Tampered' WHERE id = '1'",
		"DROP TABLE users",
		"SELECT id FROM users; DROP TABLE users",
	}
	for _, stmt := range blockedStmts {
		if _, err := src.RunSQL(ctx, stmt, nil, true); err == nil {
			t.Fatalf("RunSQL(readOnly=true, %q) succeeded, expected error", stmt)
		} else if !strings.Contains(err.Error(), "read-only mode violation") {
			t.Fatalf("RunSQL(readOnly=true, %q) error = %q, want 'read-only mode violation'", stmt, err.Error())
		}
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("failed to count rows after blocked writes: %v", err)
	}
	if count != 2 {
		t.Fatalf("users row count = %d, want 2", count)
	}

	// Database-level query errors inside a read-only transaction must return cleanly without nil dereference.
	if _, err := src.RunSQL(ctx, "SELECT * FROM nonexistent_table", nil, true); err == nil {
		t.Fatal("RunSQL(readOnly=true, nonexistent_table) succeeded, expected error")
	} else if !strings.Contains(err.Error(), "unable to execute query") {
		t.Fatalf("RunSQL(readOnly=true, nonexistent_table) error = %q, want 'unable to execute query'", err.Error())
	}

	// Read-write DML and SELECT both work when readOnly=false.
	gotDML, err := src.RunSQL(ctx, "UPDATE users SET name = ? WHERE id = ?", []any{"UpdatedAlice", "1"}, false)
	if err != nil {
		t.Fatalf("RunSQL(readOnly=false, UPDATE) failed: %v", err)
	}
	wantDML := map[string]any{"status": "success", "rows_affected": int64(1)}
	if diff := cmp.Diff(wantDML, gotDML); diff != "" {
		t.Fatalf("RunSQL(readOnly=false, UPDATE) mismatch (-want +got):\n%s", diff)
	}

	gotRWSelect, err := src.RunSQL(ctx, "SELECT name FROM users WHERE id = ?", []any{"1"}, false)
	if err != nil {
		t.Fatalf("RunSQL(readOnly=false, SELECT) failed: %v", err)
	}
	wantRWSelect := []any{map[string]any{"name": "UpdatedAlice"}}
	if diff := cmp.Diff(wantRWSelect, gotRWSelect); diff != "" {
		t.Fatalf("RunSQL(readOnly=false, SELECT) mismatch (-want +got):\n%s", diff)
	}

	gotDDL, err := src.RunSQL(ctx, "CREATE TABLE temp_items (id TEXT)", nil, false)
	if err != nil {
		t.Fatalf("RunSQL(readOnly=false, CREATE TABLE) failed: %v", err)
	}
	gotDDLMap, ok := gotDDL.(map[string]any)
	if !ok || gotDDLMap["status"] != "success" {
		t.Fatalf("RunSQL(readOnly=false, CREATE TABLE) = %#v, want status=success", gotDDL)
	}
}

type fakeGoOraState struct {
	mu              sync.Mutex
	readOnlyTxTried int
	fallbackTxTried int
	executedStmts   []string
	queriedStmts    []string
	rolledBackTxCnt int
}

type fakeGoOraConnector struct {
	state *fakeGoOraState
}

func (c *fakeGoOraConnector) Connect(context.Context) (driver.Conn, error) {
	return &fakeGoOraConn{state: c.state}, nil
}

func (c *fakeGoOraConnector) Driver() driver.Driver {
	return &fakeGoOraDriver{state: c.state}
}

type fakeGoOraDriver struct {
	state *fakeGoOraState
}

func (d *fakeGoOraDriver) Open(string) (driver.Conn, error) {
	return &fakeGoOraConn{state: d.state}, nil
}

type fakeGoOraConn struct {
	state *fakeGoOraState
}

func (c *fakeGoOraConn) Prepare(query string) (driver.Stmt, error) {
	return nil, errors.New("unexpected Prepare")
}

func (c *fakeGoOraConn) Close() error { return nil }

func (c *fakeGoOraConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *fakeGoOraConn) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if opts.ReadOnly {
		c.state.readOnlyTxTried++
		return nil, errors.New("readonly transaction is not supported")
	}
	c.state.fallbackTxTried++
	return &fakeGoOraTx{state: c.state}, nil
}

func (c *fakeGoOraConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	c.state.executedStmts = append(c.state.executedStmts, query)
	c.state.mu.Unlock()
	return driver.RowsAffected(0), nil
}

func (c *fakeGoOraConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	c.state.queriedStmts = append(c.state.queriedStmts, query)
	c.state.mu.Unlock()
	return &fakeGoOraRows{}, nil
}

type fakeGoOraTx struct {
	state *fakeGoOraState
}

func (tx *fakeGoOraTx) Commit() error { return nil }

func (tx *fakeGoOraTx) Rollback() error {
	tx.state.mu.Lock()
	tx.state.rolledBackTxCnt++
	tx.state.mu.Unlock()
	return nil
}

type fakeGoOraRows struct {
	done bool
}

func (r *fakeGoOraRows) Columns() []string { return []string{"VAL"} }
func (r *fakeGoOraRows) Close() error      { return nil }
func (r *fakeGoOraRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = "ok"
	return nil
}

func TestRunSQLReadOnlyFallbackForGoOraDriver(t *testing.T) {
	t.Parallel()

	state := &fakeGoOraState{}
	db := sql.OpenDB(&fakeGoOraConnector{state: state})
	defer db.Close()

	src := &Source{
		Config: Config{Name: "test-go-ora", Type: SourceType, User: "u"},
		DB:     db,
	}

	got, err := src.RunSQL(context.Background(), "SELECT 'ok' AS VAL FROM DUAL", nil, true)
	if err != nil {
		t.Fatalf("RunSQL(readOnly=true) fallback failed: %v", err)
	}
	want := []any{map[string]any{"VAL": "ok"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("RunSQL(readOnly=true) fallback result mismatch (-want +got):\n%s", diff)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.readOnlyTxTried != 1 {
		t.Fatalf("readOnlyTxTried = %d, want 1", state.readOnlyTxTried)
	}
	if state.fallbackTxTried != 1 {
		t.Fatalf("fallbackTxTried = %d, want 1", state.fallbackTxTried)
	}
	if diff := cmp.Diff([]string{"SET TRANSACTION READ ONLY"}, state.executedStmts); diff != "" {
		t.Fatalf("executedStmts mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"SELECT 'ok' AS VAL FROM DUAL"}, state.queriedStmts); diff != "" {
		t.Fatalf("queriedStmts mismatch (-want +got):\n%s", diff)
	}
	if state.rolledBackTxCnt != 1 {
		t.Fatalf("rolledBackTxCnt = %d, want 1", state.rolledBackTxCnt)
	}
}
