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

package tools_test

import (
	"context"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/go-playground/validator/v10"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// Compile-time check: ConfigBase satisfies ToolMeta on its own.
var _ tools.ToolMeta = tools.ConfigBase{}

func newBaseTool() (tools.BaseTool[tools.ConfigBase], tools.Manifest) {
	cfg := tools.ConfigBase{
		Name:           "my-tool",
		Description:    "my tool description",
		AuthRequired:   []string{"google"},
		ScopesRequired: []string{"scope-a", "scope-b"},
	}
	manifest := tools.Manifest{
		Description:  "manifest description",
		AuthRequired: []string{"google"},
	}
	b := tools.NewBaseTool(
		cfg,
		tools.NewReadOnlyAnnotations(),
		manifest,
		parameters.Parameters{parameters.NewStringParameter("p1", "first param")},
	)
	return b, manifest
}

func TestBaseToolGetters(t *testing.T) {
	b, wantManifest := newBaseTool()

	if got, want := b.GetName(), "my-tool"; got != want {
		t.Errorf("GetName() = %q, want %q", got, want)
	}
	if got, want := b.GetDescription(), "my tool description"; got != want {
		t.Errorf("GetDescription() = %q, want %q", got, want)
	}
	if diff := cmp.Diff([]string{"google"}, b.GetAuthRequired()); diff != "" {
		t.Errorf("GetAuthRequired() mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(tools.NewReadOnlyAnnotations(), b.GetAnnotations(nil)); diff != "" {
		t.Errorf("GetAnnotations() mismatch (-want +got):\n%s", diff)
	}
	gotManifest, err := b.Manifest(nil)
	if err != nil {
		t.Fatalf("Manifest() error = %v", err)
	}
	if diff := cmp.Diff(wantManifest, gotManifest); diff != "" {
		t.Errorf("Manifest() mismatch (-want +got):\n%s", diff)
	}
	p, err := b.GetParameters(nil)
	if err != nil {
		t.Fatalf("GetParameters() error = %v", err)
	}
	if len(p) != 1 || p[0].GetName() != "p1" {
		t.Errorf("GetParameters() = %+v, want one param named p1", p)
	}
	if diff := cmp.Diff([]string{"scope-a", "scope-b"}, b.GetScopesRequired()); diff != "" {
		t.Errorf("GetScopesRequired() mismatch (-want +got):\n%s", diff)
	}
}

func TestBaseToolAuthorized(t *testing.T) {
	tcs := []struct {
		desc         string
		authRequired []string
		verified     []string
		want         bool
	}{
		{"empty required is always authorized", nil, nil, true},
		{"empty required ignores verified", nil, []string{"foo"}, true},
		{"verified includes required", []string{"google"}, []string{"google", "github"}, true},
		{"verified missing required", []string{"google"}, []string{"github"}, false},
		{"verified empty when required non-empty", []string{"google"}, nil, false},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			b := tools.NewBaseTool(tools.ConfigBase{AuthRequired: tc.authRequired}, nil, tools.Manifest{}, nil)
			if got := b.Authorized(tc.verified); got != tc.want {
				t.Errorf("Authorized(%v) = %v, want %v", tc.verified, got, tc.want)
			}
		})
	}
}

func TestBaseToolRequiresClientAuthorization(t *testing.T) {
	b := tools.NewBaseTool(tools.ConfigBase{}, nil, tools.Manifest{}, nil)
	got, err := b.RequiresClientAuthorization(nil)
	if err != nil {
		t.Fatalf("RequiresClientAuthorization() error = %v", err)
	}
	if got {
		t.Errorf("RequiresClientAuthorization() = true, want false")
	}
}

func TestBaseToolGetAuthTokenHeaderName(t *testing.T) {
	b := tools.NewBaseTool(tools.ConfigBase{}, nil, tools.Manifest{}, nil)
	got, err := b.GetAuthTokenHeaderName(nil)
	if err != nil {
		t.Fatalf("GetAuthTokenHeaderName() error = %v", err)
	}
	if got != "Authorization" {
		t.Errorf("GetAuthTokenHeaderName() = %q, want %q", got, "Authorization")
	}
}

func TestBaseToolEmbedParamsPassthrough(t *testing.T) {
	b := tools.NewBaseTool(
		tools.ConfigBase{},
		nil,
		tools.Manifest{},
		parameters.Parameters{parameters.NewStringParameter("p1", "first")},
	)
	values := parameters.ParamValues{{Name: "p1", Value: "hello"}}
	got, err := b.EmbedParams(context.Background(), values, nil)
	if err != nil {
		t.Fatalf("EmbedParams() error = %v", err)
	}
	if diff := cmp.Diff(values, got); diff != "" {
		t.Errorf("EmbedParams() mismatch (-want +got):\n%s", diff)
	}
}

func TestShouldSuppress(t *testing.T) {
	roSource := testutils.MockSource{MockSourceConfig: testutils.MockSourceConfig{ReadOnly: true}}
	rwSource := testutils.MockSource{MockSourceConfig: testutils.MockSourceConfig{ReadOnly: false}}

	tests := []struct {
		desc        string
		src         sources.Source
		annotations *tools.ToolAnnotations
		want        bool
	}{
		{
			desc:        "nil source -> not suppressed",
			src:         nil,
			annotations: tools.NewWriteAnnotations(),
			want:        false,
		},
		{
			desc:        "read-write source with write tool -> not suppressed",
			src:         rwSource,
			annotations: tools.NewWriteAnnotations(),
			want:        false,
		},
		{
			desc:        "read-only source with write tool (readOnlyHint: false) -> suppressed",
			src:         roSource,
			annotations: tools.NewWriteAnnotations(),
			want:        true,
		},
		{
			desc:        "read-only source with destructive write tool (readOnlyHint: false, destructiveHint: true) -> suppressed",
			src:         roSource,
			annotations: tools.NewDestructiveAnnotations(),
			want:        true,
		},
		{
			desc:        "read-only source with read tool (readOnlyHint: true) -> not suppressed",
			src:         roSource,
			annotations: tools.NewReadOnlyAnnotations(),
			want:        false,
		},
		{
			desc:        "read-only source with nil annotations -> not suppressed",
			src:         roSource,
			annotations: nil,
			want:        false,
		},
		{
			desc: "read-only source with custom tool explicitly setting readOnlyHint: false -> suppressed",
			src:  roSource,
			annotations: &tools.ToolAnnotations{
				ReadOnlyHint: func(b bool) *bool { return &b }(false),
			},
			want: true,
		},
		{
			desc: "read-only source with custom tool explicitly setting readOnlyHint: true -> not suppressed",
			src:  roSource,
			annotations: &tools.ToolAnnotations{
				ReadOnlyHint: func(b bool) *bool { return &b }(true),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			cfg := testutils.MockToolConfig{
				ConfigBase:  tools.ConfigBase{Name: "my-tool"},
				Annotations: tt.annotations,
			}
			tool, err := cfg.Initialize(context.Background())
			if err != nil {
				t.Fatalf("unexpected error initializing mock tool: %v", err)
			}
			if got := tools.ShouldSuppress(context.Background(), tool, tt.src); got != tt.want {
				t.Errorf("ShouldSuppress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigBaseYamlParsing_AsyncTask(t *testing.T) {
	yamlData := `
name: async-tool
enableAsync:
  pollIntervalMs: 5000
  ttlMs: 3600000
`
	var cfg tools.ConfigBase
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal error: %v", err)
	}

	if cfg.Name != "async-tool" {
		t.Errorf("cfg.Name = %v, want %v", cfg.Name, "async-tool")
	}

	asyncSettings, ok := cfg.GetAsyncSetting()
	if !ok {
		t.Fatalf("GetAsyncSetting() = false, want true")
	}
	if asyncSettings.PollIntervalMs != 5000 {
		t.Errorf("PollIntervalMs = %v, want 5000", asyncSettings.PollIntervalMs)
	}
	if asyncSettings.TTLMs != 3600000 {
		t.Errorf("TTLMs = %v, want 3600000", asyncSettings.TTLMs)
	}
}

func TestConfigBaseYamlParsing_NoAsync(t *testing.T) {
	yamlData := `
name: sync-tool
`
	var cfg tools.ConfigBase
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal error: %v", err)
	}

	_, ok := cfg.GetAsyncSetting()
	if ok {
		t.Fatalf("GetAsyncSetting() = true, want false")
	}
}

func TestConfigBaseYamlParsing_Async_Invalid(t *testing.T) {
	testCases := []struct {
		name     string
		yamlData string
	}{
		{
			name: "missing pollIntervalMs",
			yamlData: `
name: async-tool
enableAsync:
  ttlMs: 3600000
`,
		},
		{
			name: "negative pollIntervalMs",
			yamlData: `
name: async-tool
enableAsync:
  pollIntervalMs: -1
  ttlMs: 3600000
`,
		},
		{
			name: "zero ttlMs",
			yamlData: `
name: async-tool
enableAsync:
  pollIntervalMs: 5000
  ttlMs: 0
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg tools.ConfigBase
			// Note: we don't just call yaml.Unmarshal. The actual codebase uses util.UnmarshalYAML which uses the validator.
			// However, in tools_test.go, we should test what util.UnmarshalYAML does or test the validator directly.
			err := yaml.UnmarshalWithOptions([]byte(tc.yamlData), &cfg, yaml.Validator(validator.New()))
			if err == nil {
				t.Fatalf("expected validation error, got none")
			}
		})
	}
}
