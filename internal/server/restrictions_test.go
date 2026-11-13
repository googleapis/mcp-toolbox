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

package server

import (
	"context"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/telemetry"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
)

// recordingSourceConfig records whether Initialize was called, so tests can
// check that a rejected config never opens a source connection.
type recordingSourceConfig struct {
	sourceType  string
	initialized *bool
}

func (c recordingSourceConfig) SourceConfigType() string { return c.sourceType }

func (c recordingSourceConfig) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	*c.initialized = true
	return testutils.MockSource{MockSourceConfig: testutils.MockSourceConfig{Type: c.sourceType}}, nil
}

// newTestContext returns a context with a logger and telemetry
// instrumentation, as required by InitializeConfigs.
func newTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}
	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("error setting up instrumentation: %s", err)
	}
	return util.WithInstrumentation(ctx, instrumentation)
}

// errString returns err.Error(), or "" if err is nil.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestCheckRestrictions(t *testing.T) {
	ctx := newTestContext(t)
	l, err := util.LoggerFromContext(ctx)
	if err != nil {
		t.Fatalf("error getting logger: %s", err)
	}

	defaultSources := SourceConfigs{
		"pg":    testutils.MockSourceConfig{Name: "pg", Type: "cloud-sql-postgres"},
		"bq":    testutils.MockSourceConfig{Name: "bq", Type: "bigquery"},
		"local": testutils.MockSourceConfig{Name: "local", Type: "sqlite"},
	}
	defaultTools := ToolConfigs{
		"list":    testutils.MockToolConfig{Type: "postgres-list-tables"},
		"exec":    testutils.MockToolConfig{Type: "postgres-execute-sql"},
		"sleeper": testutils.MockToolConfig{Type: "wait"},
	}
	gcpSources := make(SourceConfigs, len(gcpSourceTypes))
	for _, st := range gcpSourceTypes {
		gcpSources[st] = testutils.MockSourceConfig{Name: st, Type: st}
	}

	tests := []struct {
		name            string
		allowedSources  []string
		disallowedTools []string
		sources         SourceConfigs // defaultSources if nil
		tools           ToolConfigs   // defaultTools if nil
		wantErr         string
	}{
		{
			name: "no restrictions",
		},
		{
			name:           "allowlisted source types pass",
			allowedSources: []string{"cloud-sql-postgres", "bigquery", "sqlite"},
		},
		{
			name:           "allowlist matching is case-insensitive and trimmed",
			allowedSources: []string{" Cloud-SQL-Postgres ", "BigQuery", "SQLite"},
		},
		{
			name:           "sources not in allowlist are rejected in sorted order",
			allowedSources: []string{"cloud-sql-postgres"},
			wantErr:        `config contains disallowed sources or tools: source "bq" has type "bigquery", which is not allowed by TOOLBOX_ALLOWED_SOURCES; source "local" has type "sqlite", which is not allowed by TOOLBOX_ALLOWED_SOURCES`,
		},
		{
			name:           "gcp-sources alone rejects non-gcp sources",
			allowedSources: []string{"gcp-sources"},
			wantErr:        `config contains disallowed sources or tools: source "local" has type "sqlite", which is not allowed by TOOLBOX_ALLOWED_SOURCES`,
		},
		{
			name:           "gcp-sources allows every gcp source type",
			allowedSources: []string{"gcp-sources"},
			sources:        gcpSources,
		},
		{
			name:           "gcp-sources combined with non-overlapping source",
			allowedSources: []string{"gcp-sources", "sqlite"},
		},
		{
			name:           "gcp-sources combined with overlapping source",
			allowedSources: []string{"gcp-sources", "bigquery", "sqlite"},
		},
		{
			name:           "gcp-sources is case-insensitive and trimmed",
			allowedSources: []string{" GCP-Sources ", "sqlite"},
		},
		{
			name:            "tools not in denylist pass",
			disallowedTools: []string{"spanner-execute-sql"},
		},
		{
			name:            "denylisted tools are rejected in sorted order",
			disallowedTools: []string{"wait", "postgres-execute-sql"},
			wantErr:         `config contains disallowed sources or tools: tool "exec" has type "postgres-execute-sql", which is disallowed by TOOLBOX_DISALLOWED_TOOLS; tool "sleeper" has type "wait", which is disallowed by TOOLBOX_DISALLOWED_TOOLS`,
		},
		{
			name:            "denylist matching is case-insensitive and trimmed",
			disallowedTools: []string{" Postgres-Execute-SQL "},
			wantErr:         `config contains disallowed sources or tools: tool "exec" has type "postgres-execute-sql", which is disallowed by TOOLBOX_DISALLOWED_TOOLS`,
		},
		{
			name:            "gcp-sources is not expanded in denylist",
			disallowedTools: []string{"gcp-sources"},
			tools:           ToolConfigs{"bq-tool": testutils.MockToolConfig{Type: "bigquery"}},
		},
		{
			name:            "source and tool violations are both reported",
			allowedSources:  []string{"cloud-sql-postgres", "sqlite"},
			disallowedTools: []string{"wait"},
			wantErr:         `config contains disallowed sources or tools: source "bq" has type "bigquery", which is not allowed by TOOLBOX_ALLOWED_SOURCES; tool "sleeper" has type "wait", which is disallowed by TOOLBOX_DISALLOWED_TOOLS`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ServerConfig{
				AllowedSources:  tc.allowedSources,
				DisallowedTools: tc.disallowedTools,
				SourceConfigs:   tc.sources,
				ToolConfigs:     tc.tools,
			}
			if cfg.SourceConfigs == nil {
				cfg.SourceConfigs = defaultSources
			}
			if cfg.ToolConfigs == nil {
				cfg.ToolConfigs = defaultTools
			}

			err := checkRestrictions(ctx, cfg, l)
			if got := errString(err); got != tc.wantErr {
				t.Errorf("checkRestrictions() error = %q, want %q", got, tc.wantErr)
			}
		})
	}
}

func TestInitializeConfigsRestrictions(t *testing.T) {
	ctx := newTestContext(t)

	tests := []struct {
		name            string
		allowedSources  []string
		disallowedTools []string
		wantErr         string
		wantInitialized bool
	}{
		{
			name:            "allowed config initializes",
			allowedSources:  []string{"mock-source"},
			disallowedTools: []string{"wait"},
			wantInitialized: true,
		},
		{
			name:           "disallowed source fails before source initializes",
			allowedSources: []string{"spanner"},
			wantErr:        `config contains disallowed sources or tools: source "my-source" has type "mock-source", which is not allowed by TOOLBOX_ALLOWED_SOURCES`,
		},
		{
			name:            "disallowed tool fails before source initializes",
			disallowedTools: []string{"mock-tool"},
			wantErr:         `config contains disallowed sources or tools: tool "my-tool" has type "mock-tool", which is disallowed by TOOLBOX_DISALLOWED_TOOLS`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var initialized bool
			cfg := ServerConfig{
				Version:         "0.0.0",
				AllowedSources:  tc.allowedSources,
				DisallowedTools: tc.disallowedTools,
				SourceConfigs: SourceConfigs{
					"my-source": recordingSourceConfig{sourceType: "mock-source", initialized: &initialized},
				},
				ToolConfigs: ToolConfigs{
					"my-tool": testutils.NewMockTool("my-tool", "mock tool", "my-source", nil, false, false).ToConfig(),
				},
			}

			_, _, _, _, _, _, _, _, err := InitializeConfigs(ctx, cfg)
			if got := errString(err); got != tc.wantErr {
				t.Errorf("InitializeConfigs() error = %q, want %q", got, tc.wantErr)
			}
			if initialized != tc.wantInitialized {
				t.Errorf("source initialized = %t, want %t", initialized, tc.wantInitialized)
			}
		})
	}
}

func TestInitializeOfflineConfigsRestrictions(t *testing.T) {
	ctx := newTestContext(t)

	tests := []struct {
		name            string
		allowedSources  []string
		disallowedTools []string
		wantErr         string
	}{
		{
			name:            "allowed config initializes",
			allowedSources:  []string{"mock-source"},
			disallowedTools: []string{"wait"},
		},
		{
			name:           "disallowed source fails",
			allowedSources: []string{"spanner"},
			wantErr:        `config contains disallowed sources or tools: source "my-source" has type "mock-source", which is not allowed by TOOLBOX_ALLOWED_SOURCES`,
		},
		{
			name:            "disallowed tool fails",
			disallowedTools: []string{"mock-tool"},
			wantErr:         `config contains disallowed sources or tools: tool "my-tool" has type "mock-tool", which is disallowed by TOOLBOX_DISALLOWED_TOOLS`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ServerConfig{
				Version:         "0.0.0",
				AllowedSources:  tc.allowedSources,
				DisallowedTools: tc.disallowedTools,
				SourceConfigs: SourceConfigs{
					"my-source": testutils.MockSourceConfig{Name: "my-source", Type: "mock-source"},
				},
				ToolConfigs: ToolConfigs{
					"my-tool": testutils.NewMockTool("my-tool", "mock tool", "my-source", nil, false, false).ToConfig(),
				},
			}

			_, _, err := InitializeOfflineConfigs(ctx, cfg)
			if got := errString(err); got != tc.wantErr {
				t.Errorf("InitializeOfflineConfigs() error = %q, want %q", got, tc.wantErr)
			}
		})
	}
}
