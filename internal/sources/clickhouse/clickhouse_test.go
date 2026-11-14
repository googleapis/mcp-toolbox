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
	"time"

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
		{
			desc: "connection pool settings",
			in: `
			kind: source
			name: pool-clickhouse
			type: clickhouse
			host: localhost
			port: "8443"
			user: default
			database: mydb
			maxOpenConns: 50
			maxIdleConns: 0
			connMaxLifetime: 10m
			`,
			want: map[string]sources.SourceConfig{
				"pool-clickhouse": Config{
					Name:            "pool-clickhouse",
					Type:            "clickhouse",
					Host:            "localhost",
					Port:            "8443",
					User:            "default",
					Database:        "mydb",
					MaxOpenConns:    intPtr(50),
					MaxIdleConns:    intPtr(0),
					ConnMaxLifetime: "10m",
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
		{
			desc: "maxOpenConns below minimum",
			in: `
			kind: source
			name: test-clickhouse
			type: clickhouse
			host: localhost
			port: "8443"
			user: default
			database: mydb
			maxOpenConns: 0
			`,
			err: "error unmarshaling source: unable to parse source \"test-clickhouse\" as \"clickhouse\": [3:15] Key: 'Config.MaxOpenConns' Error:Field validation for 'MaxOpenConns' failed on the 'gte' tag\n   1 | database: mydb\n   2 | host: localhost\n>  3 | maxOpenConns: 0\n                     ^\n   4 | name: test-clickhouse\n   5 | port: \"8443\"\n   6 | type: clickhouse\n   7 | ",
		},
		{
			desc: "negative maxIdleConns",
			in: `
			kind: source
			name: test-clickhouse
			type: clickhouse
			host: localhost
			port: "8443"
			user: default
			database: mydb
			maxIdleConns: -1
			`,
			err: "error unmarshaling source: unable to parse source \"test-clickhouse\" as \"clickhouse\": [3:15] Key: 'Config.MaxIdleConns' Error:Field validation for 'MaxIdleConns' failed on the 'gte' tag\n   1 | database: mydb\n   2 | host: localhost\n>  3 | maxIdleConns: -1\n                     ^\n   4 | name: test-clickhouse\n   5 | port: \"8443\"\n   6 | type: clickhouse\n   7 | ",
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
	defaultPool, err := Config{}.poolSettings()
	if err != nil {
		t.Fatalf("unexpected error resolving default pool settings: %v", err)
	}

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
			pool, err := initClickHouseConnectionPool(ctx, tracer, "test", tt.host, tt.port, tt.user, tt.pass, tt.dbname, tt.protocol, tt.secure, defaultPool)

			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if pool != nil {
				pool.Close()
			}
		})
	}
}

func intPtr(i int) *int {
	return &i
}

func TestPoolSettings(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		want    poolSettings
		wantErr string
	}{
		{
			name: "defaults when unset",
			cfg:  Config{},
			want: poolSettings{maxOpenConns: 25, maxIdleConns: 5, connMaxLifetime: 5 * time.Minute},
		},
		{
			name: "all fields overridden",
			cfg:  Config{MaxOpenConns: intPtr(100), MaxIdleConns: intPtr(20), ConnMaxLifetime: "1h"},
			want: poolSettings{maxOpenConns: 100, maxIdleConns: 20, connMaxLifetime: time.Hour},
		},
		{
			name: "zero idle connections allowed",
			cfg:  Config{MaxIdleConns: intPtr(0)},
			want: poolSettings{maxOpenConns: 25, maxIdleConns: 0, connMaxLifetime: 5 * time.Minute},
		},
		{
			name:    "idle greater than open",
			cfg:     Config{MaxOpenConns: intPtr(5), MaxIdleConns: intPtr(10)},
			wantErr: "maxIdleConns (10) must not be greater than maxOpenConns (5)",
		},
		{
			name:    "idle greater than default open",
			cfg:     Config{MaxIdleConns: intPtr(30)},
			wantErr: "maxIdleConns (30) must not be greater than maxOpenConns (25)",
		},
		{
			name: "default idle capped at lower open",
			cfg:  Config{MaxOpenConns: intPtr(2)},
			want: poolSettings{maxOpenConns: 2, maxIdleConns: 2, connMaxLifetime: 5 * time.Minute},
		},
		{
			name:    "invalid lifetime",
			cfg:     Config{ConnMaxLifetime: "ten minutes"},
			wantErr: `invalid connMaxLifetime "ten minutes"`,
		},
		{
			name:    "non-positive lifetime",
			cfg:     Config{ConnMaxLifetime: "0s"},
			wantErr: `invalid connMaxLifetime "0s": must be positive`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cfg.poolSettings()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("incorrect pool settings: want %+v, got %+v", tt.want, got)
			}
		})
	}
}

func TestInitClickHouseConnectionPoolAppliesSettings(t *testing.T) {
	poolCfg := poolSettings{maxOpenConns: 7, maxIdleConns: 3, connMaxLifetime: time.Minute}
	pool, err := initClickHouseConnectionPool(context.Background(), otel.Tracer("test"), "test", "localhost", "8443", "default", "", "default", "https", true, poolCfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pool.Close()

	if got := pool.Stats().MaxOpenConnections; got != poolCfg.maxOpenConns {
		t.Errorf("MaxOpenConnections: want %d, got %d", poolCfg.maxOpenConns, got)
	}
}
