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

package prompts_test

import (
	"fmt"
	"strings"
	"testing"

	yaml "github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// Test type aliases for convenience.
type (
	Argument  = prompts.Argument
	Arguments = prompts.Arguments
)

// Ptr is a helper function to create a pointer to a value.
func Ptr[T any](v T) *T {
	return &v
}

func makeArrayArg(name, desc string, items parameters.Parameter) Argument {
	return Argument{Parameter: parameters.NewArrayParameter(name, desc, items)}
}

// TestArguments_UnmarshalYAML tests all unmarshaling logic for the Arguments type.
func TestArgumentsUnmarshalYAML(t *testing.T) {
	t.Parallel()
	// paramComparer allows cmp.Diff to intelligently compare the parsed results.
	var transformFunc func(parameters.Parameter) any
	transformFunc = func(p parameters.Parameter) any {
		s := struct{ Name, Type, Desc string }{
			Name: p.GetName(),
			Type: p.GetType(),
			Desc: p.Manifest().Description,
		}
		if arr, ok := p.(*parameters.ArrayParameter); ok {
			s.Desc = fmt.Sprintf("%s items:%v", s.Desc, transformFunc(arr.GetItems()))
		}
		return s
	}
	paramComparer := cmp.Transformer("Parameter", transformFunc)

	testCases := []struct {
		name         string
		yamlInput    []map[string]any
		expectedArgs Arguments
		wantErr      string
	}{
		{
			name: "Defaults type to string when omitted",
			yamlInput: []map[string]any{
				{"name": "p1", "description": "d1"},
			},
			expectedArgs: Arguments{
				{Parameter: parameters.NewStringParameter("p1", "d1")},
			},
		},
		{
			name: "Respects type when present",
			yamlInput: []map[string]any{
				{"name": "p1", "description": "d1", "type": "integer"},
			},
			expectedArgs: Arguments{
				{Parameter: parameters.NewIntParameter("p1", "d1")},
			},
		},
		{
			name: "Parses complex types like arrays correctly",
			yamlInput: []map[string]any{
				{
					"name":        "param_array",
					"description": "an array",
					"type":        "array",
					"items": map[string]any{
						"name":        "item_name",
						"type":        "string",
						"description": "an item",
					},
				},
			},
			expectedArgs: Arguments{
				makeArrayArg("param_array", "an array", parameters.NewStringParameter("item_name", "an item")),
			},
		},
		{
			name: "Propagates parsing error for unsupported type",
			yamlInput: []map[string]any{
				{"name": "p1", "description": "d1", "type": "unsupported"},
			},
			wantErr: `"unsupported" is not valid type for a parameter`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			yamlBytes, err := yaml.Marshal(tc.yamlInput)
			if err != nil {
				t.Fatalf("Test setup failure: could not marshal test input to YAML: %v", err)
			}
			var got Arguments
			ctx, err := testutils.ContextWithNewLogger()
			if err != nil {
				t.Fatalf("Failed to create logger using testutils: %v", err)
			}
			err = yaml.UnmarshalContext(ctx, yamlBytes, &got)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("UnmarshalContext() expected error but got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("UnmarshalContext() error mismatch:\nwant to contain: %q\ngot: %q", tc.wantErr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("UnmarshalContext() returned unexpected error: %v", err)
				}
				if diff := cmp.Diff(tc.expectedArgs, got, paramComparer); diff != "" {
					t.Errorf("UnmarshalContext() result mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestParseArguments(t *testing.T) {
	t.Parallel()
	testArguments := prompts.Arguments{
		{Parameter: parameters.NewStringParameter("name", "A required name.")},
		{Parameter: parameters.NewIntParameter("count", "An optional count.", parameters.WithIntRequired(false))},
	}

	testCases := []struct {
		name    string
		argsIn  map[string]any
		want    parameters.ParamValues
		wantErr string
	}{
		{
			name: "Success with all parameters provided",
			argsIn: map[string]any{
				"name":  "test-name",
				"count": 42,
			},
			want: parameters.ParamValues{
				{Name: "name", Value: "test-name"},
				{Name: "count", Value: 42},
			},
		},
		{
			name: "Success with only required parameters",
			argsIn: map[string]any{
				"name": "another-name",
			},
			want: parameters.ParamValues{
				{Name: "name", Value: "another-name"},
				{Name: "count", Value: nil},
			},
		},
		{
			name: "Failure with missing required parameter",
			argsIn: map[string]any{
				"count": 123,
			},
			wantErr: `parameter "name" is required`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := prompts.ParseArguments(testArguments, tc.argsIn, nil)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected an error but got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error mismatch:\n  want to contain: %q\n  got: %q", tc.wantErr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if diff := cmp.Diff(tc.want, got); diff != "" {
					t.Errorf("ParseArguments() result mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestParseArgumentsStringValues(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		parameter parameters.Parameter
		value     any
		want      any
	}{
		{"integer", parameters.NewIntParameter("value", ""), "3", 3},
		{"integer zero overrides default", parameters.NewIntParameter("value", "", parameters.WithIntDefault(5)), "0", 0},
		{"float", parameters.NewFloatParameter("value", ""), "1.25", 1.25},
		{"float zero overrides default", parameters.NewFloatParameter("value", "", parameters.WithFloatDefault(5)), "0", float64(0)},
		{"boolean true", parameters.NewBooleanParameter("value", ""), "true", true},
		{"boolean false overrides default", parameters.NewBooleanParameter("value", "", parameters.WithBooleanDefault(true)), "false", false},
		{"integer array", parameters.NewArrayParameter("value", "", parameters.NewIntParameter("item", "")), "[0, 3]", []any{0, 3}},
		{"integer map", parameters.NewMapParameter("value", "", "integer"), `{"count":0}`, map[string]any{"count": 0}},
		{"generic map", parameters.NewMapParameter("value", "", ""), `{"count":3,"ratio":1.25}`, map[string]any{"count": int64(3), "ratio": 1.25}},
		{"numeric string remains text", parameters.NewStringParameter("value", ""), "3", "3"},
		{"JSON string remains text", parameters.NewStringParameter("value", ""), `{"enabled":false}`, `{"enabled":false}`},
		{"typed integer", parameters.NewIntParameter("value", ""), 3, 3},
		{"typed float", parameters.NewFloatParameter("value", ""), 1.25, 1.25},
		{"typed boolean", parameters.NewBooleanParameter("value", ""), false, false},
		{"typed array", parameters.NewArrayParameter("value", "", parameters.NewIntParameter("item", "")), []any{0, 3}, []any{0, 3}},
		{"typed map", parameters.NewMapParameter("value", "", "integer"), map[string]any{"count": 3}, map[string]any{"count": 3}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			argsIn := map[string]any{"value": tc.value}
			got, err := prompts.ParseArguments(prompts.Arguments{{Parameter: tc.parameter}}, argsIn, nil)
			if err != nil {
				t.Fatalf("ParseArguments() returned unexpected error: %v", err)
			}
			want := parameters.ParamValues{{Name: "value", Value: tc.want}}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("ParseArguments() mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(map[string]any{"value": tc.value}, argsIn); diff != "" {
				t.Errorf("ParseArguments() changed its input (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseArgumentsDefaultsAndOptionalValues(t *testing.T) {
	t.Parallel()
	arguments := prompts.Arguments{
		{Parameter: parameters.NewIntParameter("count", "", parameters.WithIntDefault(4))},
		{Parameter: parameters.NewBooleanParameter("enabled", "", parameters.WithBooleanDefault(false))},
		{Parameter: parameters.NewStringParameter("optional", "", parameters.WithStringRequired(false))},
	}
	got, err := prompts.ParseArguments(arguments, nil, nil)
	if err != nil {
		t.Fatalf("ParseArguments() returned unexpected error: %v", err)
	}
	want := parameters.ParamValues{
		{Name: "count", Value: 4},
		{Name: "enabled", Value: false},
		{Name: "optional", Value: nil},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ParseArguments() mismatch (-want +got):\n%s", diff)
	}
}

func TestParseArgumentsValueFromParam(t *testing.T) {
	t.Parallel()
	count := parameters.NewIntParameter("count", "")
	count.ValueFromParam = "source"
	arguments := prompts.Arguments{
		{Parameter: parameters.NewStringParameter("source", "")},
		{Parameter: count},
	}
	argsIn := map[string]any{"source": "3"}
	got, err := prompts.ParseArguments(arguments, argsIn, nil)
	if err != nil {
		t.Fatalf("ParseArguments() returned unexpected error: %v", err)
	}
	want := parameters.ParamValues{{Name: "source", Value: "3"}, {Name: "count", Value: 3}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ParseArguments() mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(map[string]any{"source": "3"}, argsIn); diff != "" {
		t.Errorf("ParseArguments() changed its input (-want +got):\n%s", diff)
	}
}

func TestParseArgumentsInvalidStringValues(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"unknown", "[3", "3 true", "true"} {
		t.Run(value, func(t *testing.T) {
			arguments := prompts.Arguments{{Parameter: parameters.NewIntParameter("value", "")}}
			if _, err := prompts.ParseArguments(arguments, map[string]any{"value": value}, nil); err == nil {
				t.Errorf("ParseArguments() accepted invalid integer argument %q", value)
			}
		})
	}
}
