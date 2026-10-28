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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/postgres"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/jackc/pgx/v5"
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
