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
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/telemetry"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
)

// recordingSourceConfig records whether Initialize was called.
type recordingSourceConfig struct {
	sourceType  string
	initialized *bool
}

func (c recordingSourceConfig) SourceConfigType() string { return c.sourceType }

func (c recordingSourceConfig) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	*c.initialized = true
	return testutils.MockSource{MockSourceConfig: testutils.MockSourceConfig{Type: c.sourceType}}, nil
}

func TestCheckRestrictions(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}
	l, err := util.LoggerFromContext(ctx)
	if err != nil {
		t.Fatalf("error getting logger: %s", err)
	}

	baseSources := SourceConfigs{
		"pg":    testutils.MockSourceConfig{Name: "pg", Type: "cloud-sql-postgres"},
		"bq":    testutils.MockSourceConfig{Name: "bq", Type: "bigquery"},
		"local": testutils.MockSourceConfig{Name: "local", Type: "sqlite"},
	}
	baseTools := ToolConfigs{
		"list":    testutils.MockToolConfig{Type: "postgres-list-tables"},
		"exec":    testutils.MockToolConfig{Type: "postgres-execute-sql"},
		"sleeper": testutils.MockToolConfig{Type: "wait"},
	}

	tests := []struct {
		name            string
		allowedSources  []string
		disallowedTools []string
		wantErr         bool
		wantContains    []string
		wantMissing     []string
	}{
		{
			name: "no restrictions",
		},
		{
			name:            "allowed sources and non-disallowed tools pass",
			allowedSources:  []string{"cloud-sql-postgres", "bigquery", "sqlite"},
			disallowedTools: []string{"spanner-execute-sql"},
		},
		{
			name:           "case-insensitive and whitespace-trimmed matching",
			allowedSources: []string{" Cloud-SQL-Postgres ", "BigQuery", "SQLite"},
		},
		{
			name:           "gcp-sources preset alone",
			allowedSources: []string{"gcp-sources"},
			wantErr:        true,
			wantContains:   []string{`source "local" has type "sqlite"`},
			wantMissing:    []string{`source "pg"`, `source "bq"`},
		},
		{
			name:           "gcp-sources preset combined with other sources",
			allowedSources: []string{" GCP-Sources ", "sqlite"},
		},
		{
			name:            "gcp-sources is treated literally in disallowedTools",
			disallowedTools: []string{"gcp-sources"},
		},
		{
			name:           "disallowed sources reported in sorted order",
			allowedSources: []string{"cloud-sql-postgres"},
			wantErr:        true,
			wantContains: []string{
				`source "bq" has type "bigquery", which is not allowed by TOOLBOX_ALLOWED_SOURCES; source "local" has type "sqlite"`,
			},
			wantMissing: []string{`source "pg"`, "tool "},
		},
		{
			name:            "disallowed tools reported in sorted order",
			disallowedTools: []string{" Postgres-Execute-SQL ", "wait"},
			wantErr:         true,
			wantContains: []string{
				`tool "exec" has type "postgres-execute-sql", which is disallowed by TOOLBOX_DISALLOWED_TOOLS; tool "sleeper" has type "wait"`,
			},
			wantMissing: []string{`tool "list"`, "source "},
		},
		{
			name:            "source and tool violations both reported",
			allowedSources:  []string{"cloud-sql-postgres", "sqlite"},
			disallowedTools: []string{"wait"},
			wantErr:         true,
			wantContains:    []string{`source "bq"`, `tool "sleeper"`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ServerConfig{
				AllowedSources:  tc.allowedSources,
				DisallowedTools: tc.disallowedTools,
				SourceConfigs:   baseSources,
				ToolConfigs:     baseTools,
			}
			err := checkRestrictions(ctx, cfg, l)
			if (err != nil) != tc.wantErr {
				t.Fatalf("checkRestrictions() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil {
				return
			}
			for _, s := range tc.wantContains {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("checkRestrictions() error = %q, want it to contain %q", err, s)
				}
			}
			for _, s := range tc.wantMissing {
				if strings.Contains(err.Error(), s) {
					t.Errorf("checkRestrictions() error = %q, want it not to contain %q", err, s)
				}
			}
		})
	}

	t.Run("all gcpSourceTypes pass under gcp-sources preset", func(t *testing.T) {
		gcpConfigs := make(SourceConfigs, len(gcpSourceTypes))
		for _, st := range gcpSourceTypes {
			gcpConfigs[st] = testutils.MockSourceConfig{Name: st, Type: st}
		}
		cfg := ServerConfig{
			AllowedSources: []string{"gcp-sources"},
			SourceConfigs:  gcpConfigs,
		}
		if err := checkRestrictions(ctx, cfg, l); err != nil {
			t.Fatalf("checkRestrictions() = %v, want nil", err)
		}
	})
}

func TestInitializeConfigsRestrictions(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}
	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	newCfg := func(initialized *bool, allowedSources, disallowedTools []string) ServerConfig {
		return ServerConfig{
			Version:         "0.0.0",
			AllowedSources:  allowedSources,
			DisallowedTools: disallowedTools,
			SourceConfigs: SourceConfigs{
				"my-source": recordingSourceConfig{sourceType: "mock-source", initialized: initialized},
			},
			ToolConfigs: ToolConfigs{
				"my-tool": testutils.NewMockTool("my-tool", "mock tool", "my-source", nil, false, false).ToConfig(),
			},
		}
	}

	t.Run("disallowed source fails before source initializes", func(t *testing.T) {
		var initialized bool
		_, _, _, _, _, _, _, _, err := InitializeConfigs(ctx, newCfg(&initialized, []string{"spanner"}, nil))
		if err == nil {
			t.Fatalf("InitializeConfigs() = nil, want error")
		}
		if initialized {
			t.Errorf("source was initialized, want check to run before any source connects")
		}
	})

	t.Run("disallowed tool fails before source initializes", func(t *testing.T) {
		var initialized bool
		_, _, _, _, _, _, _, _, err := InitializeConfigs(ctx, newCfg(&initialized, nil, []string{"mock-tool"}))
		if err == nil {
			t.Fatalf("InitializeConfigs() = nil, want error")
		}
		if initialized {
			t.Errorf("source was initialized, want check to run before any source connects")
		}
	})
}

func TestInitializeOfflineConfigsRestrictions(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}
	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	cfg := ServerConfig{
		Version:         "0.0.0",
		AllowedSources:  []string{"spanner"},
		DisallowedTools: []string{"mock-tool"},
		SourceConfigs: SourceConfigs{
			"my-source": testutils.MockSourceConfig{Name: "my-source", Type: "mock-source"},
		},
		ToolConfigs: ToolConfigs{
			"my-tool": testutils.NewMockTool("my-tool", "mock tool", "my-source", nil, false, false).ToConfig(),
		},
	}
	if _, _, err := InitializeOfflineConfigs(ctx, cfg); err == nil {
		t.Fatalf("InitializeOfflineConfigs() = nil, want error")
	}
}
