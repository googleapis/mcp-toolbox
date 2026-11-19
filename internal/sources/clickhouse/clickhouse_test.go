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

package clickhouse

import (
	"context"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"go.opentelemetry.io/otel"
)

func TestParseFromYamlClickhouse(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "all fields specified",
			in: `
			kind: source
			name: test-clickhouse
			type: clickhouse
			host: localhost
			port: "8443"
			user: default
			password: "mypass"
			database: mydb
			protocol: https
			secure: true
			`,
			want: map[string]sources.SourceConfig{
				"test-clickhouse": Config{
					Name:     "test-clickhouse",
					Type:     "clickhouse",
					Host:     "localhost",
					Port:     "8443",
					User:     "default",
					Password: "mypass",
					Database: "mydb",
					Protocol: "https",
					Secure:   true,
				},
			},
		},
		{
			desc: "minimal configuration with defaults",
			in: `
			kind: source
			name: minimal-clickhouse
			type: clickhouse
			host: 127.0.0.1
			port: "8123"
			user: testuser
			database: testdb
			`,
			want: map[string]sources.SourceConfig{
				"minimal-clickhouse": Config{
					Name:     "minimal-clickhouse",
					Type:     "clickhouse",
					Host:     "127.0.0.1",
					Port:     "8123",
					User:     "testuser",
					Password: "",
					Database: "testdb",
					Protocol: "",
					Secure:   false,
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
			name: test-clickhouse
			type: clickhouse
			host: localhost
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"test-clickhouse\" as \"clickhouse\": [1:1] unknown field \"foo\"\n>  1 | foo: bar\n       ^\n   2 | host: localhost\n   3 | name: test-clickhouse\n   4 | type: clickhouse",
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

func TestNewConfigInvalidYAML(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		expectError bool
	}{
		{
			name: "invalid yaml syntax",
			yaml: `
				name: test-clickhouse
				type: clickhouse
				host: [invalid
			`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := yaml.NewDecoder(strings.NewReader(string(testutils.FormatYaml(tt.yaml))))
			_, err := newConfig(context.Background(), "test-clickhouse", decoder)
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestSource_SourceType(t *testing.T) {
	source := &Source{}
	if source.SourceType() != SourceType {
		t.Errorf("Expected %s, got %s", SourceType, source.SourceType())
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name        string
		protocol    string
		expectError bool
	}{
		{
			name:        "valid https protocol",
			protocol:    "https",
			expectError: false,
		},
		{
			name:        "valid http protocol",
			protocol:    "http",
			expectError: false,
		},
		{
			name:        "invalid protocol",
			protocol:    "invalid",
			expectError: true,
		},
		{
			name:        "invalid protocol - native not supported",
			protocol:    "native",
			expectError: true,
		},
		{
			name:        "empty values use defaults",
			protocol:    "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.protocol)
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestInitClickHouseConnectionPoolDSNGeneration(t *testing.T) {
	tracer := otel.Tracer("test")
	ctx := context.Background()

	tests := []struct {
		name      string
		host      string
		port      string
		user      string
		pass      string
		dbname    string
		protocol  string
		secure    bool
		shouldErr bool
	}{
		{
			name:      "http protocol with defaults",
			host:      "localhost",
			port:      "8123",
			user:      "default",
			pass:      "",
			dbname:    "default",
			protocol:  "http",
			secure:    false,
			shouldErr: true,
		},
		{
			name:      "https protocol with secure",
			host:      "localhost",
			port:      "8443",
			user:      "default",
			pass:      "",
			dbname:    "default",
			protocol:  "https",
			secure:    true,
			shouldErr: true,
		},
		{
			name:      "special characters in password",
			host:      "localhost",
			port:      "8443",
			user:      "test@user",
			pass:      "pass@word:with/special&chars",
			dbname:    "default",
			protocol:  "https",
			secure:    true,
			shouldErr: true,
		},
		{
			name:      "invalid protocol should fail",
			host:      "localhost",
			port:      "9000",
			user:      "default",
			pass:      "",
			dbname:    "default",
			protocol:  "invalid",
			secure:    false,
			shouldErr: true,
		},
		{
			name:      "empty protocol defaults to https",
			host:      "localhost",
			port:      "8443",
			user:      "user",
			pass:      "pass",
			dbname:    "testdb",
			protocol:  "",
			secure:    true,
			shouldErr: true,
		},
		{
			name:      "http with secure flag should upgrade to https",
			host:      "example.com",
			port:      "8443",
			user:      "user",
			pass:      "pass",
			dbname:    "db",
			protocol:  "http",
			secure:    true,
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := initClickHouseConnectionPool(ctx, tracer, "test", tt.host, tt.port, tt.user, tt.pass, tt.dbname, tt.protocol, tt.secure)

			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if pool != nil {
				pool.Close()
			}
		})
	}
}
