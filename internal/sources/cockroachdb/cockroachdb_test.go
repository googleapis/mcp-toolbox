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

package cockroachdb

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestCockroachDBSourceConfig(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "valid config",
			yaml: `
name: test-cockroachdb
type: cockroachdb
host: localhost
port: "26257"
user: root
password: ""
database: defaultdb
maxRetries: 5
retryBaseDelay: 500ms
queryParams:
  sslmode: disable
`,
		},
		{
			name: "with optional queryParams",
			yaml: `
name: test-cockroachdb
type: cockroachdb
host: localhost
port: "26257"
user: root
password: testpass
database: testdb
queryParams:
  sslmode: require
  sslcert: /path/to/cert
`,
		},
		{
			name: "with custom retry settings",
			yaml: `
name: test-cockroachdb
type: cockroachdb
host: localhost
port: "26257"
user: root
password: ""
database: defaultdb
maxRetries: 10
retryBaseDelay: 1s
`,
		},
		{
			name: "without password (insecure mode)",
			yaml: `
name: test-cockroachdb
type: cockroachdb
host: localhost
port: "26257"
user: root
database: defaultdb
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := yaml.NewDecoder(strings.NewReader(tt.yaml))
			cfg, err := newConfig(context.Background(), "test", decoder)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cfg == nil {
				t.Fatal("expected config but got nil")
			}

			// Verify it's the right type
			cockroachCfg, ok := cfg.(Config)
			if !ok {
				t.Fatalf("expected Config type, got %T", cfg)
			}

			// Verify SourceConfigType
			if cockroachCfg.SourceConfigType() != SourceType {
				t.Errorf("expected SourceConfigType %q, got %q", SourceType, cockroachCfg.SourceConfigType())
			}

			t.Logf("✅ Config parsed successfully: %+v", cockroachCfg)
		})
	}
}

func TestCockroachDBSourceType(t *testing.T) {
	yamlContent := `
name: test-cockroachdb
type: cockroachdb
host: localhost
port: "26257"
user: root
password: ""
database: defaultdb
`
	decoder := yaml.NewDecoder(strings.NewReader(yamlContent))
	cfg, err := newConfig(context.Background(), "test", decoder)
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	if cfg.SourceConfigType() != "cockroachdb" {
		t.Errorf("expected SourceConfigType 'cockroachdb', got %q", cfg.SourceConfigType())
	}
}

func TestCockroachDBDefaultValues(t *testing.T) {
	yamlContent := `
name: test-cockroachdb
type: cockroachdb
host: localhost
port: "26257"
user: root
password: ""
database: defaultdb
`
	decoder := yaml.NewDecoder(strings.NewReader(yamlContent))
	cfg, err := newConfig(context.Background(), "test", decoder)
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	cockroachCfg, ok := cfg.(Config)
	if !ok {
		t.Fatalf("expected Config type")
	}

	// Check default values
	if cockroachCfg.MaxRetries != 5 {
		t.Errorf("expected default MaxRetries 5, got %d", cockroachCfg.MaxRetries)
	}

	if cockroachCfg.RetryBaseDelay != "500ms" {
		t.Errorf("expected default RetryBaseDelay '500ms', got %q", cockroachCfg.RetryBaseDelay)
	}

	t.Logf("✅ Default values set correctly")
}

func TestConvertParamMapToRawQuery(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]string
		want   []string // Expected substrings in any order
	}{
		{
			name:   "empty params",
			params: map[string]string{},
			want:   []string{},
		},
		{
			name: "single param",
			params: map[string]string{
				"sslmode": "disable",
			},
			want: []string{"sslmode=disable"},
		},
		{
			name: "multiple params",
			params: map[string]string{
				"sslmode":          "require",
				"application_name": "test-app",
			},
			want: []string{"sslmode=require", "application_name=test-app"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertParamMapToRawQuery(tt.params)

			if len(tt.want) == 0 {
				if result != "" {
					t.Errorf("expected empty string, got %q", result)
				}
				return
			}

			// Check that all expected substrings are in the result
			for _, want := range tt.want {
				if !contains(result, want) {
					t.Errorf("expected result to contain %q, got %q", want, result)
				}
			}

			t.Logf("✅ Query string: %s", result)
		})
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// pgxPoolGoroutines counts live goroutines whose stacks still sit inside
// pgx (e.g. a pool's background checks for a client nobody closed).
func pgxPoolGoroutines() int {
	buf := make([]byte, 2<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "github.com/jackc/pgx/v5")
}

func TestInitCockroachDBConnectionPoolWithRetryReleasesFailedPools(t *testing.T) {
	_, err := initCockroachDBConnectionPoolWithRetry(
		context.Background(),
		noop.NewTracerProvider().Tracer("cockroachdb-test"),
		"test-source", "127.0.0.1", "1", "u", "p", "db", nil, 2, time.Millisecond,
	)
	if err == nil {
		t.Fatal("expected the connection to fail against a dead endpoint")
	}
	// Give any surviving background goroutines a moment to surface.
	time.Sleep(500 * time.Millisecond)
	if n := pgxPoolGoroutines(); n > 0 {
		b := make([]byte, 2<<20)
		runtime.Stack(b, true)
		t.Fatalf("failed Initialize left %d pgx goroutine(s) running in the background:\n%s", n, b[:2<<20])
	}
}
