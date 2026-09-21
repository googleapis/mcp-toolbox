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

package mysql_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/mysql"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlCloudSQLMySQL(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-mysql-instance
			type: mysql
			host: 0.0.0.0
			port: my-port
			`,
			want: map[string]sources.SourceConfig{
				"my-mysql-instance": mysql.Config{
					Name: "my-mysql-instance",
					Type: mysql.SourceType,
					Host: "0.0.0.0",
					Port: "my-port",
				},
			},
		},
		{
			desc: "with non-required fields",
			in: `
			kind: source
			name: my-mysql-instance
			type: mysql
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			queryTimeout: 45s
			`,
			want: map[string]sources.SourceConfig{
				"my-mysql-instance": mysql.Config{
					Name:         "my-mysql-instance",
					Type:         mysql.SourceType,
					Host:         "0.0.0.0",
					Port:         "my-port",
					Database:     "my_db",
					User:         "my_user",
					Password:     "my_pass",
					QueryTimeout: "45s",
				},
			},
		},
		{
			desc: "with query params",
			in: `
			kind: source
			name: my-mysql-instance
			type: mysql
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			queryParams:
				tls: preferred
				charset: utf8mb4
			`,
			want: map[string]sources.SourceConfig{
				"my-mysql-instance": mysql.Config{
					Name:     "my-mysql-instance",
					Type:     mysql.SourceType,
					Host:     "0.0.0.0",
					Port:     "my-port",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
					QueryParams: map[string]string{
						"tls":     "preferred",
						"charset": "utf8mb4",
					},
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			got, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Fatalf("mismatch (-want +got):\n%s", diff)
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
			name: my-mysql-instance
			type: mysql
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-mysql-instance\" as \"mysql\": [2:1] unknown field \"foo\"\n   1 | database: my_db\n>  2 | foo: bar\n       ^\n   3 | host: 0.0.0.0\n   4 | name: my-mysql-instance\n   5 | password: my_pass\n   6 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-mysql-instance
			type: mysql
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-mysql-instance\" as \"mysql\": Key: 'Config.Host' Error:Field validation for 'Host' failed on the 'required' tag",
		},
		{
			desc: "invalid query params type",
			in: `
			kind: source
			name: my-mysql-instance
			type: mysql
			host: 0.0.0.0
			port: 3306
			database: my_db
			user: my_user
			password: my_pass
			queryParams: not-a-map
			`,
			err: "error unmarshaling source: unable to parse source \"my-mysql-instance\" as \"mysql\": [6:14] string was used where mapping is expected\n   3 | name: my-mysql-instance\n   4 | password: my_pass\n   5 | port: 3306\n>  6 | queryParams: not-a-map\n                    ^\n   7 | type: mysql\n   8 | user: my_user",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.err) {
				t.Fatalf("unexpected error: got %q, want substring %q", errStr, tc.err)
			}
		})
	}
}

// TestFailInitialization test error during initialization without attempting a DB connection.
func TestFailInitialization(t *testing.T) {
	t.Parallel()

	cfg := mysql.Config{
		Name:         "instance",
		Type:         "mysql",
		Host:         "localhost",
		Port:         "3306",
		Database:     "db",
		User:         "user",
		Password:     "pass",
		QueryTimeout: "abc", // invalid duration
	}
	_, err := cfg.Initialize(context.Background(), noop.NewTracerProvider().Tracer("test"))
	if err == nil {
		t.Fatalf("expected error for invalid queryTimeout, got nil")
	}
	if !strings.Contains(err.Error(), "invalid queryTimeout") {
		t.Fatalf("unexpected error: %v", err)
	}
}
