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

package primitives_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/auth"
	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/resources/text"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

func TestUpdateServer(t *testing.T) {
	newSources := map[string]sources.Source{
		"example-source": testutils.MockSource{
			MockSourceConfig: testutils.MockSourceConfig{
				Name: "example-source",
				Type: "mock-source",
			},
		},
	}
	newAuth := map[string]auth.AuthService{"example-auth": nil}
	newEmbeddingModels := map[string]embeddingmodels.EmbeddingModel{"example-model": nil}
	newTools := map[string]tools.Tool{"example-tool": nil}
	newPrompts := map[string]prompts.Prompt{"example-prompt": testutils.NewMockPrompt("example-prompt", "", prompts.Arguments{})}
	newGroups := map[string]group.Group{
		"example-toolset": group.NewGroup(group.GroupConfig{Name: "example-toolset", ToolNames: []string{"example-tool"}}),
	}
	// A real resource, not nil: NewPrimitiveManager derives the skill registry
	// from this map, so it reads every value's URI.
	newResources := map[string]resources.Resource{"example-resource": testutils.MockResource1}
	newResourceTemplates := map[string]resources.ResourceTemplate{"example-template": nil}
	primMgr := primitives.NewPrimitiveManager(newSources, newAuth, newEmbeddingModels, newTools, newPrompts, newResources, newResourceTemplates, newGroups)

	gotSource, _ := primMgr.GetSource("example-source")
	if diff := cmp.Diff(gotSource, newSources["example-source"]); diff != "" {
		t.Errorf("error updating server, sources (-want +got):\n%s", diff)
	}

	gotAuthService, _ := primMgr.GetAuthService("example-auth")
	if diff := cmp.Diff(gotAuthService, newAuth["example-auth"]); diff != "" {
		t.Errorf("error updating server, authServices (-want +got):\n%s", diff)
	}

	// Compared by identity, not by cmp.Diff: a real resource carries unexported
	// fields that cmp refuses to walk.
	gotResource, _ := primMgr.GetResource("example-resource")
	if gotResource != newResources["example-resource"] {
		t.Errorf("error updating server, resources: got %v, want %v", gotResource, newResources["example-resource"])
	}

	gotTool, _ := primMgr.GetTool("example-tool")
	if diff := cmp.Diff(gotTool, newTools["example-tool"]); diff != "" {
		t.Errorf("error updating server, tools (-want +got):\n%s", diff)
	}

	wantGroup := newGroups["example-toolset"]
	gotGroup, ok := primMgr.GetGroup("example-toolset")
	if !ok {
		t.Fatal("expected group \"example-toolset\" to exist")
	}
	if diff := cmp.Diff(wantGroup, gotGroup, cmp.AllowUnexported(group.Group{})); diff != "" {
		t.Errorf("error updating server, group (-want +got):\n%s", diff)
	}

	gotPrompt, _ := primMgr.GetPrompt("example-prompt")
	if diff := cmp.Diff(gotPrompt, newPrompts["example-prompt"], cmp.AllowUnexported(testutils.MockPrompt{})); diff != "" {
		t.Errorf("error updating server, prompts (-want +got):\n%s", diff)
	}

	gotTemplate, _ := primMgr.GetResourceTemplate("example-template")
	if diff := cmp.Diff(gotTemplate, newResourceTemplates["example-template"]); diff != "" {
		t.Errorf("error updating server, resource templates (-want +got):\n%s", diff)
	}
	updateSource := map[string]sources.Source{
		"example-source2": testutils.MockSource{
			MockSourceConfig: testutils.MockSourceConfig{
				Name: "example-source2",
				Type: "mock-source",
			},
		},
	}

	primMgr.SetPrimitives(updateSource, newAuth, newEmbeddingModels, newTools, newPrompts, newResources, newResourceTemplates, newGroups)
	gotSource, _ = primMgr.GetSource("example-source2")
	if diff := cmp.Diff(gotSource, updateSource["example-source2"]); diff != "" {
		t.Errorf("error updating server, sources (-want +got):\n%s", diff)
	}
}

func TestGetUIResourcesAndTemplates(t *testing.T) {
	regularRes := testutils.NewMockResource("regular-res", "file:///reg", "", "", "", nil, nil)
	uiRes := testutils.NewMockUIResource("ui-res", "ui://test", "", "", "", nil, nil, nil, nil, "", nil)
	resourcesMap := map[string]resources.Resource{
		"regular-res": regularRes,
		"ui-res":      uiRes,
	}

	regularTmpl := testutils.NewMockResourceTemplate("regular-tmpl", "file:///tmpl/{path}", "", "", "", nil)
	uiTmpl := testutils.NewMockUIResourceTemplate("ui-tmpl", "ui://tmpl/{path}", "", "", "", nil, nil, nil, "", nil)
	templatesMap := map[string]resources.ResourceTemplate{
		"regular-tmpl": regularTmpl,
		"ui-tmpl":      uiTmpl,
	}

	primMgr := primitives.NewPrimitiveManager(nil, nil, nil, nil, nil, resourcesMap, templatesMap, nil)

	// Test GetUIResourceFromURI
	gotUIResource, ok := primMgr.GetUIResourceFromURI("ui://test")
	if !ok || gotUIResource.GetName() != "ui-res" {
		t.Errorf("expected UI resource 'ui-res' for URI ui://test, got %v (ok=%v)", gotUIResource, ok)
	}
	if _, ok := primMgr.GetUIResourceFromURI("file:///reg"); ok {
		t.Errorf("expected regular resource to not be returned by GetUIResourceFromURI")
	}
	if _, ok := primMgr.GetUIResourceFromURI("ui://nonexistent"); ok {
		t.Errorf("expected nonexistent URI to not be returned by GetUIResourceFromURI")
	}

	// Test GetUIResourceTemplateByURI
	gotUITemplate, params, ok := primMgr.GetUIResourceTemplateByURI("ui://tmpl/sub/file.html")
	if !ok || gotUITemplate.GetName() != "ui-tmpl" || params["path"] != "sub/file.html" {
		t.Errorf("expected UI template 'ui-tmpl' with path 'sub/file.html', got %v (params=%v, ok=%v)", gotUITemplate, params, ok)
	}
	if _, _, ok := primMgr.GetUIResourceTemplateByURI("file:///tmpl/sub/file.html"); ok {
		t.Errorf("expected regular template to not be matched by GetUIResourceTemplateByURI")
	}
	if _, _, ok := primMgr.GetUIResourceTemplateByURI("ui://nonexistent/path"); ok {
		t.Errorf("expected nonexistent URI to not be matched by GetUIResourceTemplateByURI")
	}
}

func TestMatchResourceTemplateURI(t *testing.T) {
	tests := []struct {
		name       string
		tmpl       string
		uri        string
		wantParams map[string]any
		wantOk     bool
	}{
		{
			name:       "valid match simple path",
			tmpl:       "ui://tmpl/{path}",
			uri:        "ui://tmpl/dashboard.html",
			wantParams: map[string]any{"path": "dashboard.html"},
			wantOk:     true,
		},
		{
			name:       "valid match nested path",
			tmpl:       "ui://tmpl/{path}",
			uri:        "ui://tmpl/sub/nested/app.js",
			wantParams: map[string]any{"path": "sub/nested/app.js"},
			wantOk:     true,
		},
		{
			name:       "valid match with suffix",
			tmpl:       "file:///static/{path}.html",
			uri:        "file:///static/index.html",
			wantParams: map[string]any{"path": "index"},
			wantOk:     true,
		},
		{
			name:       "valid match empty path",
			tmpl:       "ui://tmpl/{path}",
			uri:        "ui://tmpl/",
			wantParams: map[string]any{"path": ""},
			wantOk:     true,
		},
		{
			name:       "valid match with regex special characters in template",
			tmpl:       "ui://app-v1.0[test]/{path}",
			uri:        "ui://app-v1.0[test]/main.js",
			wantParams: map[string]any{"path": "main.js"},
			wantOk:     true,
		},
		{
			name:       "mismatched prefix",
			tmpl:       "ui://tmpl/{path}",
			uri:        "ui://other/dashboard.html",
			wantParams: nil,
			wantOk:     false,
		},
		{
			name:       "mismatched suffix",
			tmpl:       "ui://tmpl/{path}.html",
			uri:        "ui://tmpl/dashboard.css",
			wantParams: nil,
			wantOk:     false,
		},
		{
			name:       "template without path variable",
			tmpl:       "ui://static/exact",
			uri:        "ui://static/exact",
			wantParams: nil,
			wantOk:     false,
		},
		{
			name:       "uri does not match anchored template",
			tmpl:       "ui://tmpl/{path}",
			uri:        "prefix-ui://tmpl/dashboard.html",
			wantParams: nil,
			wantOk:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotParams, gotOk := primitives.MatchResourceTemplateURI(tc.tmpl, tc.uri)
			if gotOk != tc.wantOk {
				t.Fatalf("MatchResourceTemplateURI(%q, %q) ok = %v, want %v", tc.tmpl, tc.uri, gotOk, tc.wantOk)
			}
			if diff := cmp.Diff(tc.wantParams, gotParams); diff != "" {
				t.Errorf("MatchResourceTemplateURI(%q, %q) params mismatch (-want +got):\n%s", tc.tmpl, tc.uri, diff)
			}
		})
	}
}

// TestSkillRegistry pins the registry as derived from resources: built by the
// constructor and rebuilt whenever SetPrimitives replaces the resources map.
func TestSkillRegistry(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatal(err)
	}
	skillDoc := textResource(t, ctx, "guide", "skill://analytics-guide/SKILL.md",
		"---\nname: analytics-guide\ndescription: Query the warehouse\n---\n\n# analytics-guide\n")

	primMgr := primitives.NewPrimitiveManager(nil, nil, nil, nil, nil, nil, nil, nil)
	if got := primMgr.SkillRegistry().Len(); got != 0 {
		t.Errorf("Len() = %d with no resources, want 0", got)
	}

	primMgr.SetPrimitives(nil, nil, nil, nil, nil,
		map[string]resources.Resource{"guide": skillDoc}, nil, nil)
	if got := primMgr.SkillRegistry().Len(); got != 1 {
		t.Fatalf("Len() = %d after SetPrimitives, want 1", got)
	}
	if got := primMgr.SkillRegistry().URIs(); !slices.Equal(got, []string{"skill://analytics-guide/SKILL.md"}) {
		t.Errorf("URIs() = %v, want the one skill", got)
	}

	// A reload that drops the skill must drop it from the registry too.
	primMgr.SetPrimitives(nil, nil, nil, nil, nil, nil, nil, nil)
	if got := primMgr.SkillRegistry().Len(); got != 0 {
		t.Errorf("Len() = %d after the skill was removed, want 0", got)
	}
}

func textResource(t *testing.T, ctx context.Context, name, uri, content string) resources.Resource {
	t.Helper()
	cfg := &text.Config{
		ResourceConfigBase: resources.ResourceConfigBase{
			ConfigBase: resources.ConfigBase{Name: name, Type: "text", MimeType: "text/markdown"},
			URI:        uri,
		},
		Text: content,
	}
	res, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unable to initialize %q: %s", uri, err)
	}
	return res
}
