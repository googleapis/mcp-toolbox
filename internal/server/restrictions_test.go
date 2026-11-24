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
	"errors"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
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

// failingToolConfig is a tool config whose Initialize always fails. If
// initialized is set, Initialize records that it was called.
type failingToolConfig struct {
	initialized *bool
}

func (failingToolConfig) ToolConfigType() string { return "failing-tool" }

func (c failingToolConfig) Initialize(context.Context) (tools.Tool, error) {
	if c.initialized != nil {
		*c.initialized = true
	}
	return nil, errors.New("initialize failed")
}

// errLines joins lines with newlines to build a multi-line error message.
func errLines(lines ...string) string {
	return strings.Join(lines, "\n")
}

// errString returns err.Error(), or "" if err is nil.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestCheckRestrictions(t *testing.T) {
	ctx := testutils.ContextWithLoggerAndInstrumentation(t)
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
	gcpSources := make(SourceConfigs, len(sourcePresets["gcp-sources"]))
	for _, st := range sourcePresets["gcp-sources"] {
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
			name:            "empty and whitespace-only entries mean no restriction",
			allowedSources:  []string{"", "   "},
			disallowedTools: []string{"", "   "},
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
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  sources not allowed by TOOLBOX_ALLOWED_SOURCES:",
				"    - bq (type: bigquery)",
				"    - local (type: sqlite)",
			),
		},
		{
			name:           "gcp-sources alone rejects non-gcp sources",
			allowedSources: []string{"gcp-sources"},
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  sources not allowed by TOOLBOX_ALLOWED_SOURCES:",
				"    - local (type: sqlite)",
			),
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
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  tools disallowed by TOOLBOX_DISALLOWED_TOOLS:",
				"    - exec (type: postgres-execute-sql)",
				"    - sleeper (type: wait)",
			),
		},
		{
			name:            "denylist matching is case-insensitive and trimmed",
			disallowedTools: []string{" Postgres-Execute-SQL "},
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  tools disallowed by TOOLBOX_DISALLOWED_TOOLS:",
				"    - exec (type: postgres-execute-sql)",
			),
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
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  sources not allowed by TOOLBOX_ALLOWED_SOURCES:",
				"    - bq (type: bigquery)",
				"  tools disallowed by TOOLBOX_DISALLOWED_TOOLS:",
				"    - sleeper (type: wait)",
			),
		},
		{
			name:           "tools using rejected sources are listed",
			allowedSources: []string{"cloud-sql-postgres"},
			tools: ToolConfigs{
				"bq-query":  testutils.MockToolConfig{Type: "bigquery-sql", Source: "bq"},
				"bq-schema": testutils.MockToolConfig{Type: "bigquery-get-table-info", Source: "bq"},
				"pg-query":  testutils.MockToolConfig{Type: "postgres-sql", Source: "pg"},
				"sleeper":   testutils.MockToolConfig{Type: "wait"},
			},
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  sources not allowed by TOOLBOX_ALLOWED_SOURCES:",
				"    - bq (type: bigquery), used by tools: bq-query, bq-schema",
				"    - local (type: sqlite)",
			),
		},
		{
			name:           "every tool using a rejected source is listed",
			allowedSources: []string{"cloud-sql-postgres", "sqlite"},
			tools: ToolConfigs{
				"t1": testutils.MockToolConfig{Type: "bigquery-sql", Source: "bq"},
				"t2": testutils.MockToolConfig{Type: "bigquery-sql", Source: "bq"},
				"t3": testutils.MockToolConfig{Type: "bigquery-sql", Source: "bq"},
				"t4": testutils.MockToolConfig{Type: "bigquery-sql", Source: "bq"},
				"t5": testutils.MockToolConfig{Type: "bigquery-sql", Source: "bq"},
			},
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  sources not allowed by TOOLBOX_ALLOWED_SOURCES:",
				"    - bq (type: bigquery), used by tools: t1, t2, t3, t4, t5",
			),
		},
		{
			name:            "denylisted tool is not listed as used by a rejected source",
			allowedSources:  []string{"cloud-sql-postgres", "sqlite"},
			disallowedTools: []string{"bigquery-execute-sql"},
			tools: ToolConfigs{
				"bq-exec":  testutils.MockToolConfig{Type: "bigquery-execute-sql", Source: "bq"},
				"bq-query": testutils.MockToolConfig{Type: "bigquery-sql", Source: "bq"},
			},
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  sources not allowed by TOOLBOX_ALLOWED_SOURCES:",
				"    - bq (type: bigquery), used by tools: bq-query",
				"  tools disallowed by TOOLBOX_DISALLOWED_TOOLS:",
				"    - bq-exec (type: bigquery-execute-sql)",
			),
		},
		{
			name:           "tools that fail to initialize are skipped",
			allowedSources: []string{"cloud-sql-postgres", "sqlite"},
			tools: ToolConfigs{
				"broken":   failingToolConfig{},
				"bq-query": testutils.MockToolConfig{Type: "bigquery-sql", Source: "bq"},
			},
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  sources not allowed by TOOLBOX_ALLOWED_SOURCES:",
				"    - bq (type: bigquery), used by tools: bq-query",
			),
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

func TestCheckRestrictionsInitializesToolsOnlyOnSourceRejection(t *testing.T) {
	ctx := testutils.ContextWithLoggerAndInstrumentation(t)
	l, err := util.LoggerFromContext(ctx)
	if err != nil {
		t.Fatalf("error getting logger: %s", err)
	}

	tests := []struct {
		name            string
		allowedSources  []string
		disallowedTools []string
		wantInitialized bool
	}{
		{
			name:            "no source rejected",
			allowedSources:  []string{"bigquery"},
			disallowedTools: []string{"wait"},
		},
		{
			name:            "source rejected",
			allowedSources:  []string{"spanner"},
			wantInitialized: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var initialized bool
			cfg := ServerConfig{
				AllowedSources:  tc.allowedSources,
				DisallowedTools: tc.disallowedTools,
				SourceConfigs:   SourceConfigs{"bq": testutils.MockSourceConfig{Name: "bq", Type: "bigquery"}},
				ToolConfigs:     ToolConfigs{"tool": failingToolConfig{initialized: &initialized}},
			}
			_ = checkRestrictions(ctx, cfg, l)
			if initialized != tc.wantInitialized {
				t.Errorf("tool initialized = %t, want %t", initialized, tc.wantInitialized)
			}
		})
	}
}

func TestInitializeConfigsRestrictions(t *testing.T) {
	ctx := testutils.ContextWithLoggerAndInstrumentation(t)

	tests := []struct {
		name            string
		allowedSources  []string
		disallowedTools []string
		sourceTypes     map[string]string // defaults to {"my-source": "mock-source"}
		tools           ToolConfigs       // defaults to {"my-tool": ...}
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
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  sources not allowed by TOOLBOX_ALLOWED_SOURCES:",
				"    - my-source (type: mock-source), used by tools: my-tool",
			),
		},
		{
			name:           "tool using disallowed source fails before any source initializes",
			allowedSources: []string{"mock-source"},
			sourceTypes: map[string]string{
				"allowed-source":    "mock-source",
				"disallowed-source": "other-source",
			},
			tools: ToolConfigs{
				"allowed-tool":    testutils.NewMockTool("allowed-tool", "mock tool", "allowed-source", nil, false, false).ToConfig(),
				"disallowed-tool": testutils.NewMockTool("disallowed-tool", "mock tool", "disallowed-source", nil, false, false).ToConfig(),
			},
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  sources not allowed by TOOLBOX_ALLOWED_SOURCES:",
				"    - disallowed-source (type: other-source), used by tools: disallowed-tool",
			),
		},
		{
			name:            "disallowed tool fails before source initializes",
			disallowedTools: []string{"mock-tool"},
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  tools disallowed by TOOLBOX_DISALLOWED_TOOLS:",
				"    - my-tool (type: mock-tool)",
			),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var initialized bool
			sourceTypes := tc.sourceTypes
			if sourceTypes == nil {
				sourceTypes = map[string]string{"my-source": "mock-source"}
			}
			sourceConfigs := make(SourceConfigs, len(sourceTypes))
			for name, typ := range sourceTypes {
				sourceConfigs[name] = recordingSourceConfig{sourceType: typ, initialized: &initialized}
			}
			toolConfigs := tc.tools
			if toolConfigs == nil {
				toolConfigs = ToolConfigs{
					"my-tool": testutils.NewMockTool("my-tool", "mock tool", "my-source", nil, false, false).ToConfig(),
				}
			}
			cfg := ServerConfig{
				Version:         "0.0.0",
				AllowedSources:  tc.allowedSources,
				DisallowedTools: tc.disallowedTools,
				SourceConfigs:   sourceConfigs,
				ToolConfigs:     toolConfigs,
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
	ctx := testutils.ContextWithLoggerAndInstrumentation(t)

	tests := []struct {
		name            string
		allowedSources  []string
		disallowedTools []string
		sources         SourceConfigs // defaults to {"my-source": mock-source}
		tools           ToolConfigs   // defaults to {"my-tool": ...}
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
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  sources not allowed by TOOLBOX_ALLOWED_SOURCES:",
				"    - my-source (type: mock-source), used by tools: my-tool",
			),
		},
		{
			name:           "tool using disallowed source fails",
			allowedSources: []string{"mock-source"},
			sources: SourceConfigs{
				"allowed-source":    testutils.MockSourceConfig{Name: "allowed-source", Type: "mock-source"},
				"disallowed-source": testutils.MockSourceConfig{Name: "disallowed-source", Type: "other-source"},
			},
			tools: ToolConfigs{
				"allowed-tool":    testutils.NewMockTool("allowed-tool", "mock tool", "allowed-source", nil, false, false).ToConfig(),
				"disallowed-tool": testutils.NewMockTool("disallowed-tool", "mock tool", "disallowed-source", nil, false, false).ToConfig(),
			},
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  sources not allowed by TOOLBOX_ALLOWED_SOURCES:",
				"    - disallowed-source (type: other-source), used by tools: disallowed-tool",
			),
		},
		{
			name:            "disallowed tool fails",
			disallowedTools: []string{"mock-tool"},
			wantErr: errLines(
				"config contains disallowed sources or tools:",
				"  tools disallowed by TOOLBOX_DISALLOWED_TOOLS:",
				"    - my-tool (type: mock-tool)",
			),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sourceConfigs := tc.sources
			if sourceConfigs == nil {
				sourceConfigs = SourceConfigs{
					"my-source": testutils.MockSourceConfig{Name: "my-source", Type: "mock-source"},
				}
			}
			toolConfigs := tc.tools
			if toolConfigs == nil {
				toolConfigs = ToolConfigs{
					"my-tool": testutils.NewMockTool("my-tool", "mock tool", "my-source", nil, false, false).ToConfig(),
				}
			}
			cfg := ServerConfig{
				Version:         "0.0.0",
				AllowedSources:  tc.allowedSources,
				DisallowedTools: tc.disallowedTools,
				SourceConfigs:   sourceConfigs,
				ToolConfigs:     toolConfigs,
			}

			_, _, err := InitializeOfflineConfigs(ctx, cfg)
			if got := errString(err); got != tc.wantErr {
				t.Errorf("InitializeOfflineConfigs() error = %q, want %q", got, tc.wantErr)
			}
		})
	}
}
