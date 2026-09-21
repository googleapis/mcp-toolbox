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

package singlestore_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/singlestore"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestParseFromYaml(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-s2-instance
			type: singlestore
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-s2-instance": singlestore.Config{
					Name:     "my-s2-instance",
					Type:     singlestore.SourceType,
					Host:     "0.0.0.0",
					Port:     "my-port",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
				},
			},
		},
		{
			desc: "with query timeout",
			in: `
			kind: source
			name: my-s2-instance
			type: singlestore
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			queryTimeout: 45s
			`,
			want: map[string]sources.SourceConfig{
				"my-s2-instance": singlestore.Config{
					Name:         "my-s2-instance",
					Type:         singlestore.SourceType,
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
			desc: "with connection params",
			in: `
			kind: source
			name: my-s2-instance
			type: singlestore
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			connectionParams:
				tls: preferred
				compress: true
			`,
			want: map[string]sources.SourceConfig{
				"my-s2-instance": singlestore.Config{
					Name:     "my-s2-instance",
					Type:     singlestore.SourceType,
					Host:     "0.0.0.0",
					Port:     "my-port",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
					ConnectionParams: map[string]string{
						"tls":      "preferred",
						"compress": "true",
					},
				},
			},
		},
		{
			desc: "with tls via connection params",
			in: `
			kind: source
			name: my-s2-instance
			type: singlestore
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			connectionParams:
				tls: skip-verify
			`,
			want: map[string]sources.SourceConfig{
				"my-s2-instance": singlestore.Config{
					Name:     "my-s2-instance",
					Type:     singlestore.SourceType,
					Host:     "0.0.0.0",
					Port:     "my-port",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
					ConnectionParams: map[string]string{
						"tls": "skip-verify",
					},
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
			name: my-s2-instance
			type: singlestore
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-s2-instance\" as \"singlestore\": [2:1] unknown field \"foo\"\n   1 | database: my_db\n>  2 | foo: bar\n       ^\n   3 | host: 0.0.0.0\n   4 | name: my-s2-instance\n   5 | password: my_pass\n   6 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-s2-instance
			type: singlestore
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-s2-instance\" as \"singlestore\": Key: 'Config.Host' Error:Field validation for 'Host' failed on the 'required' tag",
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

func TestFailInitialization(t *testing.T) {
	t.Parallel()

	cfg := singlestore.Config{
		Name:         "instance",
		Type:         "singlestore",
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
