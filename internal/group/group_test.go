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

package group_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func testFixtures() (map[string]tools.Tool, map[string]prompts.Prompt, map[string]resources.Resource, map[string]resources.ResourceTemplate) {
	toolsMap := map[string]tools.Tool{
		"tool1": testutils.NewMockTool("tool1", "first tool", "", []parameters.Parameter{}, false, false),
		"tool2": testutils.NewMockTool("tool2", "second tool", "", []parameters.Parameter{}, false, false),
	}
	promptsMap := map[string]prompts.Prompt{
		"prompt1": testutils.NewMockPrompt("prompt1", "first prompt", prompts.Arguments{}),
		"prompt2": testutils.NewMockPrompt("prompt2", "second prompt", prompts.Arguments{}),
	}
	resourcesMap := map[string]resources.Resource{
		"res1": testutils.NewMockResource("res1", "file://res1", "Title 1", "Desc 1", "", nil, nil),
		"res2": testutils.NewMockResource("res2", "file://res2", "Title 2", "Desc 2", "", nil, nil),
	}
	resourceTemplatesMap := map[string]resources.ResourceTemplate{
		"tmpl1": testutils.NewMockResourceTemplate("tmpl1", "file://tmpl1", "Title 1", "Desc 1", "", nil),
		"tmpl2": testutils.NewMockResourceTemplate("tmpl2", "file://tmpl2", "Title 2", "Desc 2", "", nil),
	}
	return toolsMap, promptsMap, resourcesMap, resourceTemplatesMap
}

func intPtr(v int) *int {
	return &v
}

func TestGroupConfig_Initialize(t *testing.T) {
	t.Parallel()
	toolsMap, promptsMap, resourcesMap, resourceTemplatesMap := testFixtures()

	testCases := []struct {
		name           string
		config         group.GroupConfig
		wantTools      []string
		wantPrompts    []string
		wantRes        []string
		wantResTmpl    []string
		wantErr        string
		wantTTLMs      *int
		wantCacheScope string
	}{
		{
			name: "all primitives",
			config: group.GroupConfig{
				Name:                  "mygroup",
				Description:           "a group",
				ToolNames:             []string{"tool1", "tool2"},
				PromptNames:           []string{"prompt1", "prompt2"},
				ResourceNames:         []string{"res1", "res2"},
				ResourceTemplateNames: []string{"tmpl1", "tmpl2"},
			},
			wantTools:   []string{"tool1", "tool2"},
			wantPrompts: []string{"prompt1", "prompt2"},
			wantRes:     []string{"res1", "res2"},
			wantResTmpl: []string{"tmpl1", "tmpl2"},
		},
		{
			name: "resources only",
			config: group.GroupConfig{
				Name:          "resonly",
				ResourceNames: []string{"res1"},
			},
			wantRes: []string{"res1"},
		},
		{
			name: "templates only",
			config: group.GroupConfig{
				Name:                  "tmplonly",
				ResourceTemplateNames: []string{"tmpl1"},
			},
			wantResTmpl: []string{"tmpl1"},
		},
		{
			name: "missing resource",
			config: group.GroupConfig{
				Name:          "g",
				ResourceNames: []string{"nope"},
			},
			wantErr: "resource does not exist: \"nope\"",
		},
		{
			name: "missing resource template",
			config: group.GroupConfig{
				Name:                  "g",
				ResourceTemplateNames: []string{"nope"},
			},
			wantErr: "resource template does not exist: \"nope\"",
		},

		{
			name: "tools only",
			config: group.GroupConfig{
				Name:      "toolsonly",
				ToolNames: []string{"tool1"},
			},
			wantTools:   []string{"tool1"},
			wantPrompts: nil,
		},
		{
			name: "prompts only",
			config: group.GroupConfig{
				Name:        "promptsonly",
				PromptNames: []string{"prompt1"},
			},
			wantTools:   nil,
			wantPrompts: []string{"prompt1"},
		},
		{
			name: "default nameless group",
			config: group.GroupConfig{
				Name:        "",
				ToolNames:   []string{"tool1"},
				PromptNames: []string{"prompt1"},
			},
			wantTools:   []string{"tool1"},
			wantPrompts: []string{"prompt1"},
		},
		{
			name: "invalid group name",
			config: group.GroupConfig{
				Name:      "bad name!",
				ToolNames: []string{"tool1"},
			},
			wantErr: "invalid group name",
		},
		{
			name: "missing tool",
			config: group.GroupConfig{
				Name:      "g",
				ToolNames: []string{"nope"},
			},
			wantErr: "tool does not exist: \"nope\"",
		},
		{
			name: "missing prompt",
			config: group.GroupConfig{
				Name:        "g",
				PromptNames: []string{"nope"},
			},
			wantErr: "prompt does not exist: \"nope\"",
		},
		{
			name: "valid ttlMs",
			config: group.GroupConfig{
				Name:  "g",
				TTLMs: intPtr(10000),
			},
			wantTTLMs: intPtr(10000),
		},
		{
			name: "empty ttlMs",
			config: group.GroupConfig{
				Name: "g",
			},
			wantTTLMs: intPtr(300000),
		},
		{
			name: "public cacheScope",
			config: group.GroupConfig{
				Name:       "g",
				CacheScope: "public",
			},
			wantCacheScope: "public",
		},
		{
			name: "private cacheScope",
			config: group.GroupConfig{
				Name:       "g",
				CacheScope: "private",
			},
			wantCacheScope: "private",
		},
		{
			name: "empty cacheScope",
			config: group.GroupConfig{
				Name: "g",
			},
			wantCacheScope: "public",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, err := tc.config.Initialize(toolsMap, promptsMap, resourcesMap, resourceTemplatesMap)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !slices.Equal(g.ToolNames, tc.wantTools) {
				t.Errorf("tools = %v, want %v", g.ToolNames, tc.wantTools)
			}
			if !slices.Equal(g.PromptNames, tc.wantPrompts) {
				t.Errorf("prompts = %v, want %v", g.PromptNames, tc.wantPrompts)
			}
			if !slices.Equal(g.ResourceNames, tc.wantRes) {
				t.Errorf("resources = %v, want %v", g.ResourceNames, tc.wantRes)
			}
			if !slices.Equal(g.ResourceTemplateNames, tc.wantResTmpl) {
				t.Errorf("templates = %v, want %v", g.ResourceTemplateNames, tc.wantResTmpl)
			}
			for _, name := range tc.wantTools {
				if !g.ContainsTool(name) {
					t.Errorf("group missing tool %q", name)
				}
			}
			for _, name := range tc.wantPrompts {
				if !g.ContainsPrompt(name) {
					t.Errorf("group missing prompt %q", name)
				}
			}
			for _, name := range tc.wantRes {
				if !g.ContainsResource(name) {
					t.Errorf("group missing resource %q", name)
				}
			}
			for _, name := range tc.wantResTmpl {
				if !g.ContainsResourceTemplate(name) {
					t.Errorf("group missing resource template %q", name)
				}
			}

			expectedScope := tc.wantCacheScope
			if expectedScope == "" {
				expectedScope = group.DefaultCacheScope
			}
			if g.GetCacheScope() != expectedScope {
				t.Errorf("CacheScope = %q, want %q", g.GetCacheScope(), expectedScope)
			}

			expectedTTL := group.DefaultTTLMs
			if tc.wantTTLMs != nil {
				expectedTTL = *tc.wantTTLMs
			}
			if g.GetTTLMs() != expectedTTL {
				t.Errorf("TTLMs = %d, want %d", g.GetTTLMs(), expectedTTL)
			}
		})
	}
}

func TestGroup_ToolsetManifest(t *testing.T) {
	t.Parallel()
	toolsMap, _, _, _ := testFixtures()

	g := group.NewGroup(group.GroupConfig{
		Name:      "mygroup",
		ToolNames: []string{"tool1", "tool2"},
	})
	mgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, nil, nil, nil, nil)

	manifest, err := g.ToolsetManifest("v1.2.3", mgr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manifest.ServerVersion != "v1.2.3" {
		t.Errorf("ServerVersion = %q, want %q", manifest.ServerVersion, "v1.2.3")
	}
	wantTools := []string{"tool1", "tool2"}
	gotTools := make([]string, 0, len(manifest.ToolsManifest))
	for name := range manifest.ToolsManifest {
		gotTools = append(gotTools, name)
	}
	slices.Sort(gotTools)
	if !slices.Equal(gotTools, wantTools) {
		t.Errorf("tools manifest keys = %v, want %v", gotTools, wantTools)
	}
	if manifest.ToolsManifest["tool1"].Description != "first tool" {
		t.Errorf("tool1 description = %q, want %q", manifest.ToolsManifest["tool1"].Description, "first tool")
	}

	// Missing tool error path.
	missing := group.NewGroup(group.GroupConfig{Name: "g", ToolNames: []string{"nope"}})
	if _, err := missing.ToolsetManifest("v1", mgr); err == nil {
		t.Fatal("expected error for missing tool, got nil")
	} else if !strings.Contains(err.Error(), "tool does not exist: nope") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "tool does not exist: nope")
	}
}

func TestGroup_Contains(t *testing.T) {
	t.Parallel()
	toolsMap, promptsMap, resourcesMap, resourceTemplatesMap := testFixtures()

	g, err := group.GroupConfig{
		Name:                  "mygroup",
		Description:           "a group",
		ToolNames:             []string{"tool1", "tool2"},
		PromptNames:           []string{"prompt1", "prompt2"},
		ResourceNames:         []string{"res1", "res2"},
		ResourceTemplateNames: []string{"tmpl1", "tmpl2"},
	}.Initialize(toolsMap, promptsMap, resourcesMap, resourceTemplatesMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !g.ContainsTool("tool1") || !g.ContainsTool("tool2") {
		t.Errorf("group missing expected tools")
	}
	if g.ContainsTool("tool3") {
		t.Errorf("group reports an absent tool")
	}
	if !g.ContainsPrompt("prompt1") || !g.ContainsPrompt("prompt2") {
		t.Errorf("group missing expected prompts")
	}
	if g.ContainsPrompt("prompt3") {
		t.Errorf("group reports an absent prompt")
	}
	if !g.ContainsResource("res1") || !g.ContainsResource("res2") {
		t.Errorf("group missing expected resources")
	}
	if g.ContainsResource("res3") {
		t.Errorf("group reports an absent resource")
	}
	if !g.ContainsResourceTemplate("tmpl1") || !g.ContainsResourceTemplate("tmpl2") {
		t.Errorf("group missing expected resource templates")
	}
	if g.ContainsResourceTemplate("tmpl3") {
		t.Errorf("group reports an absent resource template")
	}
}

func TestParseFromYamlGroup(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.GroupConfigs
	}{
		{
			desc: "basic group",
			in: `
			kind: group
			name: my-group
			ttlMs: 60000
			cacheScope: private
			`,
			want: map[string]group.GroupConfig{
				"my-group": {
					Name:       "my-group",
					TTLMs:      intPtr(60000),
					CacheScope: "private",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			_, _, _, _, _, _, _, got, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
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
			desc: "invalid cacheScope",
			in: `
			kind: group
			name: my-group
			cacheScope: secret
			`,
			err: "Field validation for 'CacheScope' failed on the 'oneof' tag",
		},
		{
			desc: "invalid ttlMs",
			in: `
			kind: group
			name: my-group
			ttlMs: -100
			`,
			err: "Field validation for 'TTLMs' failed on the 'gte' tag",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.err) {
				t.Fatalf("unexpected error: got %q, want it to contain %q", errStr, tc.err)
			}
		})
	}
}
