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

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/telemetry"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
)

// recordingSourceConfig records whether Initialize was called. Tests use it to
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

func typeSet(entries ...string) map[string]struct{} {
	s := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		s[e] = struct{}{}
	}
	return s
}

func TestNewTypeSet(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
		want    map[string]struct{}
	}{
		{name: "nil", entries: nil, want: nil},
		{name: "empty", entries: []string{}, want: nil},
		{name: "single", entries: []string{"spanner"}, want: typeSet("spanner")},
		{name: "multiple", entries: []string{"cloud-sql-postgres", "cloud-sql-mysql", "spanner"}, want: typeSet("cloud-sql-postgres", "cloud-sql-mysql", "spanner")},
		{name: "trims whitespace", entries: []string{" cloud-sql-postgres ", " spanner "}, want: typeSet("cloud-sql-postgres", "spanner")},
		{name: "drops empty entries", entries: []string{"", "spanner", "", "alloydb-postgres", ""}, want: typeSet("spanner", "alloydb-postgres")},
		{name: "duplicates collapse", entries: []string{"spanner", "spanner"}, want: typeSet("spanner")},
		{name: "separators only gives empty non-nil set", entries: []string{" ", " ", ""}, want: typeSet()},
		{name: "case is preserved", entries: []string{"Spanner"}, want: typeSet("Spanner")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := newTypeSet(tc.entries)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("newTypeSet(%q) nil = %v, want nil = %v", tc.entries, got == nil, tc.want == nil)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("newTypeSet(%q) mismatch (-want +got):\n%s", tc.entries, diff)
			}
		})
	}
}

func TestNewConfigRestrictions(t *testing.T) {
	t.Run("unset means no restriction", func(t *testing.T) {
		if newConfigRestrictions(ServerConfig{}).active() {
			t.Fatalf("newConfigRestrictions().active() = true, want false")
		}
	})
	t.Run("empty means no restriction", func(t *testing.T) {
		r := newConfigRestrictions(ServerConfig{AllowedSources: []string{}, DisallowedTools: []string{}})
		if r.active() {
			t.Fatalf("newConfigRestrictions().active() = true, want false")
		}
	})
	t.Run("reads both fields", func(t *testing.T) {
		r := newConfigRestrictions(ServerConfig{
			AllowedSources:  []string{"cloud-sql-postgres", " spanner"},
			DisallowedTools: []string{"wait", "postgres-execute-sql"},
		})
		if !r.active() {
			t.Fatalf("newConfigRestrictions().active() = false, want true")
		}
		if diff := cmp.Diff(typeSet("cloud-sql-postgres", "spanner"), r.allowedSources); diff != "" {
			t.Errorf("allowedSources mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(typeSet("wait", "postgres-execute-sql"), r.disallowedTools); diff != "" {
			t.Errorf("disallowedTools mismatch (-want +got):\n%s", diff)
		}
	})
	t.Run("allowlist of separators only is active", func(t *testing.T) {
		if !newConfigRestrictions(ServerConfig{AllowedSources: []string{"", ""}}).active() {
			t.Fatalf("newConfigRestrictions().active() = false, want true")
		}
	})
	t.Run("denylist of separators only is inactive", func(t *testing.T) {
		if newConfigRestrictions(ServerConfig{DisallowedTools: []string{" ", " "}}).active() {
			t.Fatalf("newConfigRestrictions().active() = true, want false")
		}
	})
}

func TestConfigRestrictionsValidate(t *testing.T) {
	cfg := ServerConfig{
		SourceConfigs: SourceConfigs{
			"pg":    testutils.MockSourceConfig{Name: "pg", Type: "cloud-sql-postgres"},
			"bq":    testutils.MockSourceConfig{Name: "bq", Type: "bigquery"},
			"local": testutils.MockSourceConfig{Name: "local", Type: "sqlite"},
		},
		ToolConfigs: ToolConfigs{
			"list":    testutils.MockToolConfig{Type: "postgres-list-tables"},
			"exec":    testutils.MockToolConfig{Type: "postgres-execute-sql"},
			"sleeper": testutils.MockToolConfig{Type: "wait"},
		},
	}

	tests := []struct {
		name         string
		restrictions configRestrictions
		wantErr      bool
		wantContains []string
		wantMissing  []string
	}{
		{
			name:         "no restrictions",
			restrictions: configRestrictions{},
		},
		{
			name: "all sources allowed and no tools denied",
			restrictions: configRestrictions{
				allowedSources:  typeSet("cloud-sql-postgres", "bigquery", "sqlite"),
				disallowedTools: typeSet("spanner-execute-sql"),
			},
		},
		{
			name:         "disallowed sources are reported in sorted order",
			restrictions: configRestrictions{allowedSources: typeSet("cloud-sql-postgres")},
			wantErr:      true,
			wantContains: []string{
				`source "bq" has type "bigquery", which is not allowed by TOOLBOX_ALLOWED_SOURCES; source "local" has type "sqlite"`,
			},
			wantMissing: []string{`source "pg"`, "tool "},
		},
		{
			name:         "empty non-nil allowlist rejects every source",
			restrictions: configRestrictions{allowedSources: typeSet()},
			wantErr:      true,
			wantContains: []string{`source "bq"`, `source "local"`, `source "pg"`},
		},
		{
			name:         "disallowed tools are reported",
			restrictions: configRestrictions{disallowedTools: typeSet("postgres-execute-sql", "wait")},
			wantErr:      true,
			wantContains: []string{
				`tool "exec" has type "postgres-execute-sql", which is disallowed by TOOLBOX_DISALLOWED_TOOLS; tool "sleeper" has type "wait"`,
			},
			wantMissing: []string{`tool "list"`, "source "},
		},
		{
			name: "source and tool violations are both reported",
			restrictions: configRestrictions{
				allowedSources:  typeSet("cloud-sql-postgres", "sqlite"),
				disallowedTools: typeSet("wait"),
			},
			wantErr:      true,
			wantContains: []string{`source "bq"`, `tool "sleeper"`},
		},
		{
			name:         "type matching is case sensitive",
			restrictions: configRestrictions{allowedSources: typeSet("Cloud-SQL-Postgres", "bigquery", "sqlite")},
			wantErr:      true,
			wantContains: []string{`source "pg"`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.restrictions.validate(cfg)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validate() = nil, want error")
			}
			for _, s := range tc.wantContains {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("validate() error = %q, want it to contain %q", err, s)
				}
			}
			for _, s := range tc.wantMissing {
				if strings.Contains(err.Error(), s) {
					t.Errorf("validate() error = %q, want it not to contain %q", err, s)
				}
			}
		})
	}

	t.Run("empty config passes any restriction", func(t *testing.T) {
		r := configRestrictions{allowedSources: typeSet(), disallowedTools: typeSet("wait")}
		if err := r.validate(ServerConfig{}); err != nil {
			t.Fatalf("validate() = %v, want nil", err)
		}
	})
	t.Run("nil configs are skipped", func(t *testing.T) {
		r := configRestrictions{allowedSources: typeSet(), disallowedTools: typeSet("wait")}
		nilCfg := ServerConfig{
			SourceConfigs: SourceConfigs{"nil-source": nil},
			ToolConfigs:   ToolConfigs{"nil-tool": nil},
		}
		if err := r.validate(nilCfg); err != nil {
			t.Fatalf("validate() = %v, want nil", err)
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

	t.Run("unset restrictions do not restrict", func(t *testing.T) {
		var initialized bool
		sourcesMap, _, _, toolsMap, _, _, _, _, err := InitializeConfigs(ctx, newCfg(&initialized, nil, nil))
		if err != nil {
			t.Fatalf("InitializeConfigs() = %v, want nil", err)
		}
		if _, ok := sourcesMap["my-source"]; !ok {
			t.Errorf("sourcesMap missing my-source")
		}
		if _, ok := toolsMap["my-tool"]; !ok {
			t.Errorf("toolsMap missing my-tool")
		}
	})

	t.Run("allowed source and non-denied tool start", func(t *testing.T) {
		var initialized bool
		cfg := newCfg(&initialized, []string{"cloud-sql-postgres", "mock-source"}, []string{"wait", "postgres-execute-sql"})
		_, _, _, toolsMap, _, _, _, _, err := InitializeConfigs(ctx, cfg)
		if err != nil {
			t.Fatalf("InitializeConfigs() = %v, want nil", err)
		}
		if !initialized {
			t.Errorf("source was not initialized")
		}
		if _, ok := toolsMap["my-tool"]; !ok {
			t.Errorf("toolsMap missing my-tool")
		}
	})

	t.Run("disallowed source fails before the source initializes", func(t *testing.T) {
		var initialized bool
		cfg := newCfg(&initialized, []string{"cloud-sql-postgres", "spanner"}, nil)
		_, _, _, _, _, _, _, _, err := InitializeConfigs(ctx, cfg)
		if err == nil {
			t.Fatalf("InitializeConfigs() = nil, want error")
		}
		want := `source "my-source" has type "mock-source", which is not allowed by TOOLBOX_ALLOWED_SOURCES`
		if !strings.Contains(err.Error(), want) {
			t.Errorf("InitializeConfigs() error = %q, want it to contain %q", err, want)
		}
		if initialized {
			t.Errorf("source was initialized, want the check to run before any source connects")
		}
	})

	t.Run("disallowed tool fails before the source initializes", func(t *testing.T) {
		var initialized bool
		cfg := newCfg(&initialized, nil, []string{"wait", "mock-tool"})
		_, _, _, _, _, _, _, _, err := InitializeConfigs(ctx, cfg)
		if err == nil {
			t.Fatalf("InitializeConfigs() = nil, want error")
		}
		want := `tool "my-tool" has type "mock-tool", which is disallowed by TOOLBOX_DISALLOWED_TOOLS`
		if !strings.Contains(err.Error(), want) {
			t.Errorf("InitializeConfigs() error = %q, want it to contain %q", err, want)
		}
		if initialized {
			t.Errorf("source was initialized, want the check to run before any source connects")
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

	newCfg := func(allowedSources, disallowedTools []string) ServerConfig {
		return ServerConfig{
			Version:         "0.0.0",
			AllowedSources:  allowedSources,
			DisallowedTools: disallowedTools,
			SourceConfigs: SourceConfigs{
				"my-source": testutils.MockSourceConfig{Name: "my-source", Type: "mock-source"},
			},
			ToolConfigs: ToolConfigs{
				"my-tool": testutils.NewMockTool("my-tool", "mock tool", "my-source", nil, false, false).ToConfig(),
			},
		}
	}

	t.Run("unset restrictions do not restrict", func(t *testing.T) {
		toolsMap, _, err := InitializeOfflineConfigs(ctx, newCfg(nil, nil))
		if err != nil {
			t.Fatalf("InitializeOfflineConfigs() = %v, want nil", err)
		}
		if _, ok := toolsMap["my-tool"]; !ok {
			t.Errorf("toolsMap missing my-tool")
		}
	})

	t.Run("disallowed source fails", func(t *testing.T) {
		_, _, err := InitializeOfflineConfigs(ctx, newCfg([]string{"spanner"}, nil))
		if err == nil {
			t.Fatalf("InitializeOfflineConfigs() = nil, want error")
		}
		want := `source "my-source" has type "mock-source", which is not allowed by TOOLBOX_ALLOWED_SOURCES`
		if !strings.Contains(err.Error(), want) {
			t.Errorf("InitializeOfflineConfigs() error = %q, want it to contain %q", err, want)
		}
	})

	t.Run("disallowed tool fails", func(t *testing.T) {
		_, _, err := InitializeOfflineConfigs(ctx, newCfg(nil, []string{"mock-tool"}))
		if err == nil {
			t.Fatalf("InitializeOfflineConfigs() = nil, want error")
		}
		want := `tool "my-tool" has type "mock-tool", which is disallowed by TOOLBOX_DISALLOWED_TOOLS`
		if !strings.Contains(err.Error(), want) {
			t.Errorf("InitializeOfflineConfigs() error = %q, want it to contain %q", err, want)
		}
	})
}
