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

package v20250326

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestGenerateToolManifest(t *testing.T) {
	trueVal := true
	falseVal := false

	authServices := []parameters.ParamAuthService{
		{
			Name:  "my-google-auth-service",
			Field: "auth_field",
		},
		{
			Name:  "other-auth-service",
			Field: "other_auth_field",
		}}
	tcs := []struct {
		desc            string
		name            string
		description     string
		authInvoke      []string
		params          parameters.Parameters
		annotations     *tools.ToolAnnotations
		wantMetadata    map[string]any
		wantAnnotations []byte
	}{
		{
			desc:         "basic manifest without metadata",
			name:         "basic",
			description:  "foo bar",
			authInvoke:   []string{},
			params:       parameters.Parameters{parameters.NewStringParameter("string-param", "string parameter")},
			annotations:  nil,
			wantMetadata: nil,
		},
		{
			desc:            "basic manifest without metadata with annotations",
			name:            "basic",
			description:     "foo bar",
			authInvoke:      []string{},
			params:          parameters.Parameters{parameters.NewStringParameter("string-param", "string parameter")},
			annotations:     &tools.ToolAnnotations{ReadOnlyHint: &trueVal, DestructiveHint: &falseVal},
			wantMetadata:    nil,
			wantAnnotations: []byte(`{"readOnlyHint":true,"destructiveHint":false}`),
		},
		{
			desc:         "with auth invoke metadata",
			name:         "basic",
			description:  "foo bar",
			authInvoke:   []string{"auth1", "auth2"},
			params:       parameters.Parameters{parameters.NewStringParameter("string-param", "string parameter")},
			annotations:  nil,
			wantMetadata: map[string]any{"toolbox/authInvoke": []string{"auth1", "auth2"}},
		},
		{
			desc:        "with auth param metadata",
			name:        "basic",
			description: "foo bar",
			authInvoke:  []string{},
			params:      parameters.Parameters{parameters.NewStringParameter("string-param", "string parameter", parameters.WithStringAuth(authServices))},
			annotations: nil,
			wantMetadata: map[string]any{
				"toolbox/authParam": map[string][]string{
					"string-param": {"my-google-auth-service", "other-auth-service"},
				},
			},
		},
		{
			desc:        "with auth invoke and auth param metadata",
			name:        "basic",
			description: "foo bar",
			authInvoke:  []string{"auth1", "auth2"},
			params:      parameters.Parameters{parameters.NewStringParameter("string-param", "string parameter", parameters.WithStringAuth(authServices))},
			annotations: nil,
			wantMetadata: map[string]any{
				"toolbox/authInvoke": []string{"auth1", "auth2"},
				"toolbox/authParam": map[string][]string{
					"string-param": {"my-google-auth-service", "other-auth-service"},
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got := generateToolManifest(tc.name, tc.description, tc.authInvoke, tc.params, tc.annotations, nil)
			gotM := got.Metadata
			if diff := cmp.Diff(tc.wantMetadata, gotM); diff != "" {
				t.Fatalf("unexpected metadata (-want +got):\n%s", diff)
			}

			if tc.wantAnnotations != nil {
				annotations, err := json.Marshal(got.Annotations)
				if err != nil {
					t.Fatalf("error marshaling annotations")
				}
				if diff := cmp.Diff(tc.wantAnnotations, annotations); diff != "" {
					t.Fatalf("unexpected annotations (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestParamManifest(t *testing.T) {
	authServices := []parameters.ParamAuthService{
		{
			Name:  "my-google-auth-service",
			Field: "auth_field",
		},
		{
			Name:  "other-auth-service",
			Field: "other_auth_field",
		}}
	tcs := []struct {
		name          string
		in            parameters.Parameters
		urlParams     map[string]string
		wantSchema    InputSchema
		wantAuthParam map[string][]string
	}{
		{
			name: "all types",
			in: parameters.Parameters{
				parameters.NewStringParameter("foo-string", "bar", parameters.WithStringDefault("foo")),
				parameters.NewStringParameter("foo-string2", "bar"),
				parameters.NewStringParameter("foo-string3-auth", "bar", parameters.WithStringAuth(authServices)),
				parameters.NewIntParameter("foo-int2", "bar"),
				parameters.NewFloatParameter("foo-float", "bar"),
				parameters.NewArrayParameter("foo-array2", "bar", parameters.NewStringParameter("foo-string", "bar")),
				parameters.NewMapParameter("foo-map-int", "a map of ints", "integer"),
				parameters.NewMapParameter("foo-map-any", "a map of any", ""),
			},
			wantSchema: InputSchema{
				Type: "object",
				Properties: map[string]parameters.ParameterMcpManifest{
					"foo-string":       {Type: "string", Description: "bar", Default: "foo"},
					"foo-string2":      {Type: "string", Description: "bar"},
					"foo-string3-auth": {Type: "string", Description: "bar"},
					"foo-int2":         {Type: "integer", Description: "bar"},
					"foo-float":        {Type: "number", Description: "bar"},
					"foo-array2": {
						Type:        "array",
						Description: "bar",
						Items:       &parameters.ParameterMcpManifest{Type: "string", Description: "bar"},
					},
					"foo-map-int": {
						Type:                 "object",
						Description:          "a map of ints",
						AdditionalProperties: map[string]any{"type": "integer"},
					},
					"foo-map-any": {
						Type:                 "object",
						Description:          "a map of any",
						AdditionalProperties: true,
					},
				},
				Required: []string{"foo-string2", "foo-string3-auth", "foo-int2", "foo-float", "foo-array2", "foo-map-int", "foo-map-any"},
			},
			wantAuthParam: map[string][]string{
				"foo-string3-auth": []string{"my-google-auth-service", "other-auth-service"},
			},
		},
		{
			name: "urlParams is not nil, skips matched params",
			in: parameters.Parameters{
				parameters.NewStringParameter("foo-string", "bar"),
				parameters.NewIntParameter("foo-int", "bar"),
			},
			urlParams: map[string]string{
				"foo-string": "url-val",
			},
			wantSchema: InputSchema{
				Type: "object",
				Properties: map[string]parameters.ParameterMcpManifest{
					"foo-int": {Type: "integer", Description: "bar"},
				},
				Required: []string{"foo-int"},
			},
			wantAuthParam: map[string][]string{},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			gotSchema, gotAuthParam := generateParamManifest(tc.in, tc.urlParams)
			if diff := cmp.Diff(tc.wantSchema, gotSchema); diff != "" {
				t.Fatalf("unexpected manifest (-want +got):\n%s", diff)
			}
			if len(gotAuthParam) != len(tc.wantAuthParam) {
				t.Fatalf("got %d items in auth param map, want %d", len(gotAuthParam), len(tc.wantAuthParam))
			}
			for k, want := range tc.wantAuthParam {
				got, ok := gotAuthParam[k]
				if !ok {
					t.Fatalf("missing auth param: %s", k)
				}
				slices.Sort(got)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("unexpected auth param, got %s, want %s", got, want)
				}
			}
		})
	}
}

func TestGenerateListToolsResult(t *testing.T) {
	tool1 := testutils.NewMockTool("no_params", "", "", []parameters.Parameter{}, false, false)
	tool2 := testutils.NewMockTool(
		"some_params",
		"", "",
		parameters.Parameters{
			parameters.NewIntParameter("param1", "This is the first parameter."),
			parameters.NewIntParameter("param2", "This is the second parameter."),
		}, false, false)
	toolsMap := make(map[string]tools.Tool)
	toolsMap[tool1.GetName()] = tool1
	toolsMap[tool2.GetName()] = tool2
	g := group.NewGroup(group.GroupConfig{
		Name:      "test-toolset",
		ToolNames: []string{"no_params", "some_params"},
	})

	pMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, nil, nil, nil, nil)
	got, err := GenerateListToolsResult(pMgr, g, nil)
	if err != nil {
		t.Fatalf("unable to generate list tools result: %s", err)
	}
	want := ListToolsResult{
		Tools: []Tool{
			Tool{
				Name:        "no_params",
				Description: "",
				ToolInputSchema: InputSchema{
					Type:       "object",
					Properties: map[string]parameters.ParameterMcpManifest{},
					Required:   []string{},
				},
			},
			Tool{
				Name:        "some_params",
				Description: "",
				ToolInputSchema: InputSchema{
					Type: "object",
					Properties: map[string]parameters.ParameterMcpManifest{
						"param1": parameters.ParameterMcpManifest{
							Type:                 "integer",
							Description:          "This is the first parameter.",
							Items:                nil,
							Default:              nil,
							AdditionalProperties: nil,
						},
						"param2": parameters.ParameterMcpManifest{
							Type:                 "integer",
							Description:          "This is the second parameter.",
							Items:                nil,
							Default:              nil,
							AdditionalProperties: nil,
						},
					},
					Required: []string{"param1", "param2"},
				},
			},
		},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatalf("unexpected list tools result (-want +got):\n%s", diff)
	}
}

func TestGeneratePromptManifest(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		promptName  string
		description string
		args        prompts.Arguments
		want        Prompt
	}{
		{
			name:        "No arguments",
			promptName:  "test-prompt",
			description: "A test prompt.",
			args:        prompts.Arguments{},
			want: Prompt{
				Name:        "test-prompt",
				Description: "A test prompt.",
				Arguments:   []PromptArgument{},
			},
		},
		{
			name:        "With arguments",
			promptName:  "arg-prompt",
			description: "Prompt with args.",
			args: prompts.Arguments{
				{Parameter: parameters.NewStringParameter("param1", "First param")},
				{Parameter: parameters.NewIntParameter("param2", "Second param", parameters.WithIntRequired(false))},
			},
			want: Prompt{
				Name:        "arg-prompt",
				Description: "Prompt with args.",
				Arguments: []PromptArgument{
					{Name: "param1", Description: "First param", Required: true},
					{Name: "param2", Description: "Second param", Required: false},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := generatePromptManifest(tc.promptName, tc.description, tc.args)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("generatePromptManifest() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGenerateListPromptsResult(t *testing.T) {
	args := prompts.Arguments{
		{Parameter: parameters.NewStringParameter("arg1", "Test argument")},
	}
	prompt1 := testutils.NewMockPrompt("prompt1", "First test prompt", prompts.Arguments{})
	prompt2 := testutils.NewMockPrompt("prompt2", "Second test prompt", args)

	promptsMap := make(map[string]prompts.Prompt)
	promptsMap[prompt1.Name] = prompt1
	promptsMap[prompt2.Name] = prompt2
	g := group.NewGroup(group.GroupConfig{
		Name:        "test-promptset",
		PromptNames: []string{"prompt1", "prompt2"},
	})
	gMap := map[string]group.Group{
		g.Name: g,
	}
	pMgr := primitives.NewPrimitiveManager(nil, nil, nil, nil, promptsMap, nil, nil, gMap)
	got, err := GenerateListPromptsResult(pMgr, g)
	if err != nil {
		t.Fatalf("unable to generate list prompt result: %s", err)
	}
	want := ListPromptsResult{
		Prompts: []Prompt{
			Prompt{
				Name:        "prompt1",
				Description: "First test prompt",
				Arguments:   []PromptArgument{},
			},
			Prompt{
				Name:        "prompt2",
				Description: "Second test prompt",
				Arguments: []PromptArgument{
					PromptArgument{
						Name:        "arg1",
						Description: "Test argument",
						Required:    true,
					},
				},
			},
		},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatalf("unexpected list tools result (-want +got):\n%s", diff)
	}
}

func TestGenerateListToolsResultWithSecureParams(t *testing.T) {
	paramsStandard := parameters.Parameters{
		parameters.NewStringParameter("param1", "desc"),
	}
	paramsSecure := parameters.Parameters{
		&parameters.StringParameter{
			CommonParameter: parameters.CommonParameter{
				Name:   "param2",
				Type:   parameters.TypeString,
				Desc:   "desc",
				Secure: true,
			},
		},
	}
	toolStandard := testutils.NewMockTool("standard_tool", "", "", paramsStandard, false, false)
	toolSecure := testutils.NewMockTool("secure_tool", "", "", paramsSecure, false, false)

	toolsMap := map[string]tools.Tool{
		"standard_tool": toolStandard,
		"secure_tool":   toolSecure,
	}

	g := group.NewGroup(group.GroupConfig{
		Name:      "test-toolset",
		ToolNames: []string{"standard_tool", "secure_tool"},
	})
	pMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, nil, nil, nil, nil)

	got, err := GenerateListToolsResult(pMgr, g, nil)
	if err != nil {
		t.Fatalf("failed GenerateListToolsResult: %s", err)
	}

	if len(got.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d: %+v", len(got.Tools), got.Tools)
	}
	if got.Tools[0].Name != "standard_tool" {
		t.Errorf("expected standard_tool, got: %s", got.Tools[0].Name)
	}
}
