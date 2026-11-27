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

package postgres_test

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/postgres"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgproto3"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestParseFromYamlPostgres(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-pg-instance
			type: postgres
			host: my-host
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-pg-instance": postgres.Config{
					Name:     "my-pg-instance",
					Type:     postgres.SourceType,
					Host:     "my-host",
					Port:     "my-port",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
				},
			},
		},
		{
			desc: "example with query params",
			in: `
			kind: source
			name: my-pg-instance
			type: postgres
			host: my-host
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			queryParams:
				sslmode: verify-full
				sslrootcert: /tmp/ca.crt
			`,
			want: map[string]sources.SourceConfig{
				"my-pg-instance": postgres.Config{
					Name:     "my-pg-instance",
					Type:     postgres.SourceType,
					Host:     "my-host",
					Port:     "my-port",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
					QueryParams: map[string]string{
						"sslmode":     "verify-full",
						"sslrootcert": "/tmp/ca.crt",
					},
				},
			},
		},
		{
			desc: "example with query exec mode",
			in: `
			kind: source
			name: my-pg-instance
			type: postgres
			host: my-host
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			queryExecMode: simple_protocol
			`,
			want: map[string]sources.SourceConfig{
				"my-pg-instance": postgres.Config{
					Name:          "my-pg-instance",
					Type:          postgres.SourceType,
					Host:          "my-host",
					Port:          "my-port",
					Database:      "my_db",
					User:          "my_user",
					Password:      "my_pass",
					QueryExecMode: "simple_protocol",
				},
			},
		},
		{
			desc: "example with connect timeout",
			in: `
			kind: source
			name: my-pg-instance
			type: postgres
			host: my-host
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			connectTimeout: 5
			`,
			want: map[string]sources.SourceConfig{
				"my-pg-instance": postgres.Config{
					Name:           "my-pg-instance",
					Type:           postgres.SourceType,
					Host:           "my-host",
					Port:           "my-port",
					Database:       "my_db",
					User:           "my_user",
					Password:       "my_pass",
					ConnectTimeout: intPtr(5),
				},
			},
		},
		{
			desc: "example with readOnly",
			in: `
			kind: source
			name: my-pg-instance
			type: postgres
			host: my-host
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			readOnly: true
			`,
			want: map[string]sources.SourceConfig{
				"my-pg-instance": postgres.Config{
					Name:     "my-pg-instance",
					Type:     postgres.SourceType,
					Host:     "my-host",
					Port:     "my-port",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
					ReadOnly: true,
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
				t.Fatalf("incorrect parse: want %v, got %v", tc.want, got)
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
			name: my-pg-instance
			type: postgres
			host: my-host
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-pg-instance\" as \"postgres\": [2:1] unknown field \"foo\"\n   1 | database: my_db\n>  2 | foo: bar\n       ^\n   3 | host: my-host\n   4 | name: my-pg-instance\n   5 | password: my_pass\n   6 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-pg-instance
			type: postgres
			host: my-host
			port: my-port
			database: my_db
			user: my_user
			`,
			err: "error unmarshaling source: unable to parse source \"my-pg-instance\" as \"postgres\": Key: 'Config.Password' Error:Field validation for 'Password' failed on the 'required' tag",
		},
		{
			desc: "invalid query exec mode",
			in: `
			kind: source
			name: my-pg-instance
			type: postgres
			host: my-host
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			queryExecMode: invalid_mode
			`,
			err: "error unmarshaling source: unable to parse source \"my-pg-instance\" as \"postgres\": [6:16] Key: 'Config.QueryExecMode' Error:Field validation for 'QueryExecMode' failed on the 'oneof' tag\n   3 | name: my-pg-instance\n   4 | password: my_pass\n   5 | port: my-port\n>  6 | queryExecMode: invalid_mode\n                      ^\n   7 | type: postgres\n   8 | user: my_user",
		},
		{
			desc: "connect timeout below minimum",
			in: `
			kind: source
			name: my-pg-instance
			type: postgres
			host: my-host
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			connectTimeout: 0
			`,
			err: "error unmarshaling source: unable to parse source \"my-pg-instance\" as \"postgres\": [1:17] Key: 'Config.ConnectTimeout' Error:Field validation for 'ConnectTimeout' failed on the 'gte' tag\n>  1 | connectTimeout: 0\n                       ^\n   2 | database: my_db\n   3 | host: my-host\n   4 | name: my-pg-instance\n   5 | ",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if errStr != tc.err {
				t.Fatalf("unexpected error: got %q, want %q", errStr, tc.err)
			}
		})
	}
}

func intPtr(v int) *int {
	return &v
}

func TestBuildPostgresURL(t *testing.T) {
	tcs := []struct {
		desc        string
		host        string
		port        string
		queryParams map[string]string
		want        string
	}{
		{
			desc: "hostname",
			host: "db.example.com",
			port: "5432",
			want: "postgres://u:p@db.example.com:5432/mydb",
		},
		{
			desc: "ipv4",
			host: "127.0.0.1",
			port: "5432",
			want: "postgres://u:p@127.0.0.1:5432/mydb",
		},
		{
			desc: "ipv6 loopback",
			host: "::1",
			port: "5432",
			want: "postgres://u:p@[::1]:5432/mydb",
		},
		{
			desc: "ipv6 documentation",
			host: "2001:db8::1",
			port: "5432",
			want: "postgres://u:p@[2001:db8::1]:5432/mydb",
		},
		{
			desc: "ipv6 link-local with zone id",
			host: "fe80::1%eth0",
			port: "5432",
			want: "postgres://u:p@[fe80::1%25eth0]:5432/mydb",
		},
		{
			desc:        "query params sorted and encoded",
			host:        "db.example.com",
			port:        "5432",
			queryParams: map[string]string{"sslmode": "verify-full", "application_name": "my app"},
			want:        "postgres://u:p@db.example.com:5432/mydb?application_name=my+app&sslmode=verify-full",
		},
		{
			desc:        "query param value with special characters",
			host:        "db.example.com",
			port:        "5432",
			queryParams: map[string]string{"options": "-c statement_timeout=5s&key=val"},
			want:        "postgres://u:p@db.example.com:5432/mydb?options=-c+statement_timeout%3D5s%26key%3Dval",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got := postgres.BuildPostgresURL(tc.host, tc.port, "u", "p", "mydb", tc.queryParams)
			if got != tc.want {
				t.Fatalf("BuildPostgresURL(%q, %q, ...) = %q, want %q", tc.host, tc.port, got, tc.want)
			}
			if _, err := pgx.ParseConfig(got); err != nil {
				t.Fatalf("pgx.ParseConfig(%q) returned error: %v", got, err)
			}
		})
	}
}

func TestParseQueryExecMode(t *testing.T) {
	tcs := []struct {
		desc    string
		in      string
		want    pgx.QueryExecMode
		wantErr bool
	}{
		{desc: "empty (default)", in: "", want: pgx.QueryExecModeCacheStatement},
		{desc: "cache_statement", in: "cache_statement", want: pgx.QueryExecModeCacheStatement},
		{desc: "cache_describe", in: "cache_describe", want: pgx.QueryExecModeCacheDescribe},
		{desc: "describe_exec", in: "describe_exec", want: pgx.QueryExecModeDescribeExec},
		{desc: "exec", in: "exec", want: pgx.QueryExecModeExec},
		{desc: "simple_protocol", in: "simple_protocol", want: pgx.QueryExecModeSimpleProtocol},
		{desc: "invalid mode", in: "invalid_mode", wantErr: true},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := postgres.ParseQueryExecMode(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseQueryExecMode() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("parseQueryExecMode() = %v, want %v", got, tc.want)
			}
		})
	}
}

type mockRow struct {
	isSuper         bool
	hasTableWrite   bool
	hasSchemaCreate bool
	err             error
}

func (m mockRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	*(dest[0].(*bool)) = m.isSuper
	*(dest[1].(*bool)) = m.hasTableWrite
	*(dest[2].(*bool)) = m.hasSchemaCreate
	return nil
}

type mockQuerier struct {
	row mockRow
}

func (m mockQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return m.row
}

func TestVerifyReadOnlyPermissions(t *testing.T) {
	tcs := []struct {
		desc        string
		row         mockRow
		wantErr     bool
		errContains string
	}{
		{
			desc:    "valid strictly read-only user",
			row:     mockRow{isSuper: false, hasTableWrite: false, hasSchemaCreate: false},
			wantErr: false,
		},
		{
			desc:        "superuser rejected",
			row:         mockRow{isSuper: true, hasTableWrite: false, hasSchemaCreate: false},
			wantErr:     true,
			errContains: "is a superuser",
		},
		{
			desc:        "user with table write grants rejected",
			row:         mockRow{isSuper: false, hasTableWrite: true, hasSchemaCreate: false},
			wantErr:     true,
			errContains: "has table write privileges",
		},
		{
			desc:        "user with schema CREATE privilege rejected",
			row:         mockRow{isSuper: false, hasTableWrite: false, hasSchemaCreate: true},
			wantErr:     true,
			errContains: "has CREATE privilege",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			err := postgres.VerifyReadOnlyPermissions(context.Background(), mockQuerier{row: tc.row}, "test-source", "test-user")
			if (err != nil) != tc.wantErr {
				t.Fatalf("VerifyReadOnlyPermissions() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("VerifyReadOnlyPermissions() error = %q, want it to contain %q", err.Error(), tc.errContains)
			}
		})
	}
}

func TestSource_IsReadOnly(t *testing.T) {
	rwSource := &postgres.Source{Config: postgres.Config{ReadOnly: false}}
	if rwSource.IsReadOnly() {
		t.Errorf("expected IsReadOnly() == false, got true")
	}

	roSource := &postgres.Source{Config: postgres.Config{ReadOnly: true}}
	if !roSource.IsReadOnly() {
		t.Errorf("expected IsReadOnly() == true, got false")
	}
}

func TestInitialize_ReadOnly_AllQueryExecModes(t *testing.T) {
	modes := []string{
		"cache_statement",
		"cache_describe",
		"describe_exec",
		"exec",
		"simple_protocol",
	}

	for _, mode := range modes {
		t.Run("valid_reader_"+mode, func(t *testing.T) {
			host, port, cleanup := startMockPostgresWireServer(t, false, false, false)
			defer cleanup()

			cfg := postgres.Config{
				Name:          "ro-pg-" + mode,
				Type:          postgres.SourceType,
				Host:          host,
				Port:          port,
				User:          "reader",
				Password:      "secret",
				Database:      "mydb",
				QueryExecMode: mode,
				ReadOnly:      true,
			}

			src, err := cfg.Initialize(context.Background(), noop.NewTracerProvider().Tracer("test"))
			if err != nil {
				t.Fatalf("expected Initialize with queryExecMode %q to succeed for read-only user, got error: %v", mode, err)
			}
			if !src.IsReadOnly() {
				t.Errorf("expected IsReadOnly() == true for mode %q", mode)
			}
		})

		t.Run("superuser_blocked_"+mode, func(t *testing.T) {
			host, port, cleanup := startMockPostgresWireServer(t, true, false, false)
			defer cleanup()

			cfg := postgres.Config{
				Name:          "ro-pg-super-" + mode,
				Type:          postgres.SourceType,
				Host:          host,
				Port:          port,
				User:          "admin_user",
				Password:      "secret",
				Database:      "mydb",
				QueryExecMode: mode,
				ReadOnly:      true,
			}

			_, err := cfg.Initialize(context.Background(), noop.NewTracerProvider().Tracer("test"))
			if err == nil {
				t.Fatalf("expected Initialize with queryExecMode %q to fail closed for superuser", mode)
			}
			if !strings.Contains(err.Error(), "is a superuser") {
				t.Errorf("expected error for mode %q to contain 'is a superuser', got: %v", mode, err)
			}
		})
	}
}

func startMockPostgresWireServer(t *testing.T, isSuper, hasTableWrite, hasSchemaCreate bool) (string, string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host port: %v", err)
	}

	boolToByte := func(b bool) []byte {
		if b {
			return []byte{1}
		}
		return []byte{0}
	}

	boolToText := func(b bool) []byte {
		if b {
			return []byte("t")
		}
		return []byte("f")
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				backend := pgproto3.NewBackend(c, c)
				if _, err := backend.ReceiveStartupMessage(); err != nil {
					return
				}
				backend.Send(&pgproto3.AuthenticationOk{})
				backend.Send(&pgproto3.ParameterStatus{Name: "standard_conforming_strings", Value: "on"})
				backend.Send(&pgproto3.ParameterStatus{Name: "client_encoding", Value: "UTF8"})
				backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
				if err := backend.Flush(); err != nil {
					return
				}

				var isVerifyQuery bool
				for {
					msg, err := backend.Receive()
					if err != nil {
						return
					}
					switch m := msg.(type) {
					case *pgproto3.Parse:
						isVerifyQuery = strings.Contains(m.Query, "is_superuser")
						backend.Send(&pgproto3.ParseComplete{})
					case *pgproto3.Describe:
						if m.ObjectType == 'S' {
							if isVerifyQuery {
								backend.Send(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{25}})
							} else {
								backend.Send(&pgproto3.ParameterDescription{})
							}
						}
						if isVerifyQuery {
							backend.Send(&pgproto3.RowDescription{
								Fields: []pgproto3.FieldDescription{
									{Name: []byte("is_superuser"), DataTypeOID: 16, DataTypeSize: 1, Format: 1},
									{Name: []byte("has_table_write"), DataTypeOID: 16, DataTypeSize: 1, Format: 1},
									{Name: []byte("has_schema_create"), DataTypeOID: 16, DataTypeSize: 1, Format: 1},
								},
							})
						} else {
							backend.Send(&pgproto3.NoData{})
						}
					case *pgproto3.Bind:
						backend.Send(&pgproto3.BindComplete{})
					case *pgproto3.Execute:
						if isVerifyQuery {
							backend.Send(&pgproto3.DataRow{
								Values: [][]byte{
									boolToByte(isSuper),
									boolToByte(hasTableWrite),
									boolToByte(hasSchemaCreate),
								},
							})
							backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})
						} else {
							backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("PING")})
						}
					case *pgproto3.Sync:
						backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
						if err := backend.Flush(); err != nil {
							return
						}
					case *pgproto3.Query:
						if strings.Contains(m.String, "is_superuser") {
							backend.Send(&pgproto3.RowDescription{
								Fields: []pgproto3.FieldDescription{
									{Name: []byte("is_superuser"), DataTypeOID: 16, DataTypeSize: 1, Format: 0},
									{Name: []byte("has_table_write"), DataTypeOID: 16, DataTypeSize: 1, Format: 0},
									{Name: []byte("has_schema_create"), DataTypeOID: 16, DataTypeSize: 1, Format: 0},
								},
							})
							backend.Send(&pgproto3.DataRow{
								Values: [][]byte{
									boolToText(isSuper),
									boolToText(hasTableWrite),
									boolToText(hasSchemaCreate),
								},
							})
						}
						backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})
						backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
						if err := backend.Flush(); err != nil {
							return
						}
					case *pgproto3.Terminate:
						return
					}
				}
			}(conn)
		}
	}()

	return host, port, func() { _ = listener.Close() }
}

func TestInitialize_ReadOnly_WireProtocol(t *testing.T) {
	tcs := []struct {
		desc            string
		isSuper         bool
		hasTableWrite   bool
		hasSchemaCreate bool
		wantErr         bool
		errContains     string
	}{
		{
			desc:            "strictly read-only user connects and initializes successfully",
			isSuper:         false,
			hasTableWrite:   false,
			hasSchemaCreate: false,
			wantErr:         false,
		},
		{
			desc:            "admin superuser fails closed at initialization",
			isSuper:         true,
			hasTableWrite:   false,
			hasSchemaCreate: false,
			wantErr:         true,
			errContains:     "is a superuser",
		},
		{
			desc:            "table writer fails closed at initialization",
			isSuper:         false,
			hasTableWrite:   true,
			hasSchemaCreate: false,
			wantErr:         true,
			errContains:     "has table write privileges",
		},
		{
			desc:            "schema creator fails closed at initialization",
			isSuper:         false,
			hasTableWrite:   false,
			hasSchemaCreate: true,
			wantErr:         true,
			errContains:     "has CREATE privilege",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			host, port, cleanup := startMockPostgresWireServer(t, tc.isSuper, tc.hasTableWrite, tc.hasSchemaCreate)
			defer cleanup()

			cfg := postgres.Config{
				Name:     "wire-test-pg",
				Type:     postgres.SourceType,
				Host:     host,
				Port:     port,
				User:     "test_user",
				Password: "test_password",
				Database: "test_db",
				ReadOnly: true,
			}

			src, err := cfg.Initialize(context.Background(), noop.NewTracerProvider().Tracer("test"))
			if (err != nil) != tc.wantErr {
				t.Fatalf("Initialize() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("Initialize() error = %q, want it to contain %q", err.Error(), tc.errContains)
				}
				return
			}
			if !src.IsReadOnly() {
				t.Errorf("expected initialized source IsReadOnly() == true, got false")
			}
		})
	}
}
