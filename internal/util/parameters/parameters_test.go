// Copyright 2024 Google LLC
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

package parameters_test

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParametersMarshal(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		name string
		in   []map[string]any
		want parameters.Parameters
	}{
		{
			name: "string",
			in: []map[string]any{
				{
					"name":        "my_string",
					"type":        "string",
					"description": "this param is a string",
				},
			},
			want: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string"),
			},
		},
		{
			name: "string not required",
			in: []map[string]any{
				{
					"name":        "my_string",
					"type":        "string",
					"description": "this param is a string",
					"required":    false,
				},
			},
			want: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringRequired(false)),
			},
		},
		{
			name: "int",
			in: []map[string]any{
				{
					"name":        "my_integer",
					"type":        "integer",
					"description": "this param is an int",
				},
			},
			want: parameters.Parameters{
				parameters.NewIntParameter("my_integer", "this param is an int"),
			},
		},
		{
			name: "int not required",
			in: []map[string]any{
				{
					"name":        "my_integer",
					"type":        "integer",
					"description": "this param is an int",
					"required":    false,
				},
			},
			want: parameters.Parameters{
				parameters.NewIntParameter("my_integer", "this param is an int", parameters.WithIntRequired(false)),
			},
		},
		{
			name: "float",
			in: []map[string]any{
				{
					"name":        "my_float",
					"type":        "float",
					"description": "my param is a float",
				},
			},
			want: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "my param is a float"),
			},
		},
		{
			name: "float not required",
			in: []map[string]any{
				{
					"name":        "my_float",
					"type":        "float",
					"description": "my param is a float",
					"required":    false,
				},
			},
			want: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "my param is a float", parameters.WithFloatRequired(false)),
			},
		},
		{
			name: "bool",
			in: []map[string]any{
				{
					"name":        "my_bool",
					"type":        "boolean",
					"description": "this param is a boolean",
				},
			},
			want: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a boolean"),
			},
		},
		{
			name: "bool not required",
			in: []map[string]any{
				{
					"name":        "my_bool",
					"type":        "boolean",
					"description": "this param is a boolean",
					"required":    false,
				},
			},
			want: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a boolean", parameters.WithBooleanRequired(false)),
			},
		},
		{
			name: "string array",
			in: []map[string]any{
				{
					"name":        "my_array",
					"type":        "array",
					"description": "this param is an array of strings",
					"items": map[string]string{
						"name":        "my_string",
						"type":        "string",
						"description": "string item",
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewArrayParameter("my_array", "this param is an array of strings", parameters.NewStringParameter("my_string", "string item")),
			},
		},
		{
			name: "string array not required",
			in: []map[string]any{
				{
					"name":        "my_array",
					"type":        "array",
					"description": "this param is an array of strings",
					"required":    false,
					"items": map[string]string{
						"name":        "my_string",
						"type":        "string",
						"description": "string item",
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewArrayParameter("my_array", "this param is an array of strings", parameters.NewStringParameter("my_string", "string item"), parameters.WithArrayRequired(false)),
			},
		},
		{
			name: "float array",
			in: []map[string]any{
				{
					"name":        "my_array",
					"type":        "array",
					"description": "this param is an array of floats",
					"items": map[string]string{
						"name":        "my_float",
						"type":        "float",
						"description": "float item",
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewArrayParameter("my_array", "this param is an array of floats", parameters.NewFloatParameter("my_float", "float item")),
			},
		},
		{
			name: "string default",
			in: []map[string]any{
				{
					"name":        "my_string",
					"type":        "string",
					"default":     "foo",
					"description": "this param is a string",
				},
			},
			want: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringDefault("foo")),
			},
		},
		{
			name: "int default",
			in: []map[string]any{
				{
					"name":        "my_integer",
					"type":        "integer",
					"default":     5,
					"description": "this param is an int",
				},
			},
			want: parameters.Parameters{
				parameters.NewIntParameter("my_integer", "this param is an int", parameters.WithIntDefault(5)),
			},
		},
		{
			name: "float default",
			in: []map[string]any{
				{
					"name":        "my_float",
					"type":        "float",
					"default":     1.1,
					"description": "my param is a float",
				},
			},
			want: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "my param is a float", parameters.WithFloatDefault(1.1)),
			},
		},
		{
			name: "bool default",
			in: []map[string]any{
				{
					"name":        "my_bool",
					"type":        "boolean",
					"default":     true,
					"description": "this param is a boolean",
				},
			},
			want: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a boolean", parameters.WithBooleanDefault(true)),
			},
		},
		{
			name: "string array default",
			in: []map[string]any{
				{
					"name":        "my_array",
					"type":        "array",
					"default":     []any{"foo", "bar"},
					"description": "this param is an array of strings",
					"items": map[string]string{
						"name":        "my_string",
						"type":        "string",
						"description": "string item",
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewArrayParameter("my_array", "this param is an array of strings", parameters.NewStringParameter("my_string", "string item"), parameters.WithArrayDefault([]any{"foo", "bar"})),
			},
		},
		{
			name: "float array default",
			in: []map[string]any{
				{
					"name":        "my_array",
					"type":        "array",
					"default":     []any{1.0, 1.1},
					"description": "this param is an array of floats",
					"items": map[string]string{
						"name":        "my_float",
						"type":        "float",
						"description": "float item",
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewArrayParameter("my_array", "this param is an array of floats", parameters.NewFloatParameter("my_float", "float item"), parameters.WithArrayDefault([]any{1.0, 1.1})),
			},
		},
		{
			name: "map with string values",
			in: []map[string]any{
				{
					"name":        "my_map",
					"type":        "map",
					"description": "this param is a map of strings",
					"valueType":   "string",
				},
			},
			want: parameters.Parameters{
				parameters.NewMapParameter("my_map", "this param is a map of strings", "string"),
			},
		},
		{
			name: "map not required",
			in: []map[string]any{
				{
					"name":        "my_map",
					"type":        "map",
					"description": "this param is a map of strings",
					"required":    false,
					"valueType":   "string",
				},
			},
			want: parameters.Parameters{
				parameters.NewMapParameter("my_map", "this param is a map of strings", "string", parameters.WithMapRequired(false)),
			},
		},
		{
			name: "map with default",
			in: []map[string]any{
				{
					"name":        "my_map",
					"type":        "map",
					"description": "this param is a map of strings",
					"default":     map[string]any{"key1": "val1"},
					"valueType":   "string",
				},
			},
			want: parameters.Parameters{
				parameters.NewMapParameter("my_map", "this param is a map of strings", "string", parameters.WithMapDefault(map[string]any{"key1": "val1"})),
			},
		},
		{
			name: "generic map (no valueType)",
			in: []map[string]any{
				{
					"name":        "my_generic_map",
					"type":        "map",
					"description": "this param is a generic map",
				},
			},
			want: parameters.Parameters{
				parameters.NewMapParameter("my_generic_map", "this param is a generic map", ""),
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			var got parameters.Parameters
			// parse map to bytes
			data, err := yaml.Marshal(tc.in)
			if err != nil {
				t.Fatalf("unable to marshal input to yaml: %s", err)
			}
			// parse bytes to object
			err = yaml.UnmarshalContext(ctx, data, &got)
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}

func TestAuthParametersMarshal(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	authServices := []parameters.ParamAuthService{{Name: "my-google-auth-service", Field: "user_id"}, {Name: "other-auth-service", Field: "user_id"}}
	tcs := []struct {
		name string
		in   []map[string]any
		want parameters.Parameters
	}{
		{
			name: "string",
			in: []map[string]any{
				{
					"name":        "my_string",
					"type":        "string",
					"description": "this param is a string",
					"authServices": []map[string]string{
						{
							"name":  "my-google-auth-service",
							"field": "user_id",
						},
						{
							"name":  "other-auth-service",
							"field": "user_id",
						},
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringAuth(authServices)),
			},
		},
		{
			name: "string with authServices",
			in: []map[string]any{
				{
					"name":        "my_string",
					"type":        "string",
					"description": "this param is a string",
					"authServices": []map[string]string{
						{
							"name":  "my-google-auth-service",
							"field": "user_id",
						},
						{
							"name":  "other-auth-service",
							"field": "user_id",
						},
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringAuth(authServices)),
			},
		},
		{
			name: "int",
			in: []map[string]any{
				{
					"name":        "my_integer",
					"type":        "integer",
					"description": "this param is an int",
					"authServices": []map[string]string{
						{
							"name":  "my-google-auth-service",
							"field": "user_id",
						},
						{
							"name":  "other-auth-service",
							"field": "user_id",
						},
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewIntParameter("my_integer", "this param is an int", parameters.WithIntAuth(authServices)),
			},
		},
		{
			name: "int with authServices",
			in: []map[string]any{
				{
					"name":        "my_integer",
					"type":        "integer",
					"description": "this param is an int",
					"authServices": []map[string]string{
						{
							"name":  "my-google-auth-service",
							"field": "user_id",
						},
						{
							"name":  "other-auth-service",
							"field": "user_id",
						},
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewIntParameter("my_integer", "this param is an int", parameters.WithIntAuth(authServices)),
			},
		},
		{
			name: "float",
			in: []map[string]any{
				{
					"name":        "my_float",
					"type":        "float",
					"description": "my param is a float",
					"authServices": []map[string]string{
						{
							"name":  "my-google-auth-service",
							"field": "user_id",
						},
						{
							"name":  "other-auth-service",
							"field": "user_id",
						},
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "my param is a float", parameters.WithFloatAuth(authServices)),
			},
		},
		{
			name: "float with authServices",
			in: []map[string]any{
				{
					"name":        "my_float",
					"type":        "float",
					"description": "my param is a float",
					"authServices": []map[string]string{
						{
							"name":  "my-google-auth-service",
							"field": "user_id",
						},
						{
							"name":  "other-auth-service",
							"field": "user_id",
						},
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "my param is a float", parameters.WithFloatAuth(authServices)),
			},
		},
		{
			name: "bool",
			in: []map[string]any{
				{
					"name":        "my_bool",
					"type":        "boolean",
					"description": "this param is a boolean",
					"authServices": []map[string]string{
						{
							"name":  "my-google-auth-service",
							"field": "user_id",
						},
						{
							"name":  "other-auth-service",
							"field": "user_id",
						},
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a boolean", parameters.WithBooleanAuth(authServices)),
			},
		},
		{
			name: "bool with authServices",
			in: []map[string]any{
				{
					"name":        "my_bool",
					"type":        "boolean",
					"description": "this param is a boolean",
					"authServices": []map[string]string{
						{
							"name":  "my-google-auth-service",
							"field": "user_id",
						},
						{
							"name":  "other-auth-service",
							"field": "user_id",
						},
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a boolean", parameters.WithBooleanAuth(authServices)),
			},
		},
		{
			name: "string array",
			in: []map[string]any{
				{
					"name":        "my_array",
					"type":        "array",
					"description": "this param is an array of strings",
					"items": map[string]string{
						"name":        "my_string",
						"type":        "string",
						"description": "string item",
					},
					"authServices": []map[string]string{
						{
							"name":  "my-google-auth-service",
							"field": "user_id",
						},
						{
							"name":  "other-auth-service",
							"field": "user_id",
						},
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewArrayParameter("my_array", "this param is an array of strings", parameters.NewStringParameter("my_string", "string item"), parameters.WithArrayAuth(authServices)),
			},
		},
		{
			name: "string array with authServices",
			in: []map[string]any{
				{
					"name":        "my_array",
					"type":        "array",
					"description": "this param is an array of strings",
					"items": map[string]string{
						"name":        "my_string",
						"type":        "string",
						"description": "string item",
					},
					"authServices": []map[string]string{
						{
							"name":  "my-google-auth-service",
							"field": "user_id",
						},
						{
							"name":  "other-auth-service",
							"field": "user_id",
						},
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewArrayParameter("my_array", "this param is an array of strings", parameters.NewStringParameter("my_string", "string item"), parameters.WithArrayAuth(authServices)),
			},
		},
		{
			name: "float array",
			in: []map[string]any{
				{
					"name":        "my_array",
					"type":        "array",
					"description": "this param is an array of floats",
					"items": map[string]string{
						"name":        "my_float",
						"type":        "float",
						"description": "float item",
					},
					"authServices": []map[string]string{
						{
							"name":  "my-google-auth-service",
							"field": "user_id",
						},
						{
							"name":  "other-auth-service",
							"field": "user_id",
						},
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewArrayParameter("my_array", "this param is an array of floats", parameters.NewFloatParameter("my_float", "float item"), parameters.WithArrayAuth(authServices)),
			},
		},
		{
			name: "map",
			in: []map[string]any{
				{
					"name":        "my_map",
					"type":        "map",
					"description": "this param is a map of strings",
					"valueType":   "string",
					"authServices": []map[string]string{
						{"name": "my-google-auth-service", "field": "user_id"},
						{"name": "other-auth-service", "field": "user_id"},
					},
				},
			},
			want: parameters.Parameters{
				parameters.NewMapParameter("my_map", "this param is a map of strings", "string", parameters.WithMapAuth(authServices)),
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			var got parameters.Parameters
			// parse map to bytes
			data, err := yaml.Marshal(tc.in)
			if err != nil {
				t.Fatalf("unable to marshal input to yaml: %s", err)
			}
			// parse bytes to object
			err = yaml.UnmarshalContext(ctx, data, &got)
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}

func TestParametersParse(t *testing.T) {
	intValue := 2
	floatValue := 1.5
	tcs := []struct {
		name   string
		params parameters.Parameters
		in     map[string]any
		want   parameters.ParamValues
	}{
		// ... (primitive type tests are unchanged)
		{
			name: "string",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string"),
			},
			in: map[string]any{
				"my_string": "hello world",
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: "hello world"}},
		},
		{
			name: "not string",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string"),
			},
			in: map[string]any{
				"my_string": 4,
			},
		},
		{
			name: "string allowed",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringAllowedValues([]any{"foo"})),
			},
			in: map[string]any{
				"my_string": "foo",
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: "foo"}},
		},
		{
			name: "string allowed regex",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringAllowedValues([]any{"^f.*"})),
			},
			in: map[string]any{
				"my_string": "foo",
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: "foo"}},
		},
		{
			name: "string not allowed",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringAllowedValues([]any{"foo"})),
			},
			in: map[string]any{
				"my_string": "bar",
			},
		},
		{
			name: "string not allowed regex",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringAllowedValues([]any{"^f.*"})),
			},
			in: map[string]any{
				"my_string": "bar",
			},
		},
		{
			name: "string excluded",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringExcludedValues([]any{"foo"})),
			},
			in: map[string]any{
				"my_string": "foo",
			},
		},
		{
			name: "string excluded regex",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringExcludedValues([]any{"^f.*"})),
			},
			in: map[string]any{
				"my_string": "foo",
			},
		},
		{
			name: "string not excluded",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringExcludedValues([]any{"foo"})),
			},
			in: map[string]any{
				"my_string": "bar",
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: "bar"}},
		},
		{
			name: "string with escape backticks",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringEscape("backticks")),
			},
			in: map[string]any{
				"my_string": "foo",
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: "`foo`"}},
		},
		{
			name: "string with escape double quotes",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringEscape("double-quotes")),
			},
			in: map[string]any{
				"my_string": "foo",
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: `"foo"`}},
		},
		{
			name: "string with escape single quotes",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringEscape("single-quotes")),
			},
			in: map[string]any{
				"my_string": "foo",
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: `'foo'`}},
		},
		{
			name: "string with escape square brackets",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringEscape("square-brackets")),
			},
			in: map[string]any{
				"my_string": "foo",
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: "[foo]"}},
		},
		{
			name: "string with escape backticks containing backtick (injection prevention)",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringEscape("backticks")),
			},
			in: map[string]any{
				"my_string": "users` OR 1=1--",
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: "`users`` OR 1=1--`"}},
		},
		{
			name: "string with escape double quotes containing double quote (injection prevention)",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringEscape("double-quotes")),
			},
			in: map[string]any{
				"my_string": `col" OR 1=1--`,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: `"col"" OR 1=1--"`}},
		},
		{
			name: "string with escape single quotes containing single quote (injection prevention)",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringEscape("single-quotes")),
			},
			in: map[string]any{
				"my_string": "val' OR 1=1--",
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: "'val'' OR 1=1--'"}},
		},
		{
			name: "string with escape square brackets containing closing bracket (injection prevention)",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringEscape("square-brackets")),
			},
			in: map[string]any{
				"my_string": "col] OR 1=1--",
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: "[col]] OR 1=1--]"}},
		},
		{
			name: "int",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int"),
			},
			in: map[string]any{
				"my_int": 100,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_int", Value: 100}},
		},
		{
			name: "not int",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int"),
			},
			in: map[string]any{
				"my_int": 14.5,
			},
		},
		{
			name: "not int (big)",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int"),
			},
			in: map[string]any{
				"my_int": math.MaxInt64,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_int", Value: math.MaxInt64}},
		},
		{
			name: "int allowed",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntAllowedValues([]any{1})),
			},
			in: map[string]any{
				"my_int": 1,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_int", Value: 1}},
		},
		{
			name: "int allowed regex",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntAllowedValues([]any{"^\\d{2}$"})),
			},
			in: map[string]any{
				"my_int": 10,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_int", Value: 10}},
		},
		{
			name: "int not allowed",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntAllowedValues([]any{1})),
			},
			in: map[string]any{
				"my_int": 2,
			},
		},
		{
			name: "int not allowed regex",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntAllowedValues([]any{"^\\d{2}$"})),
			},
			in: map[string]any{
				"my_int": 100,
			},
		},
		{
			name: "int excluded",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntExcludedValues([]any{1})),
			},
			in: map[string]any{
				"my_int": 1,
			},
		},
		{
			name: "int excluded regex",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntExcludedValues([]any{"^\\d{2}$"})),
			},
			in: map[string]any{
				"my_int": 10,
			},
		},
		{
			name: "int not excluded",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntExcludedValues([]any{1})),
			},
			in: map[string]any{
				"my_int": 2,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_int", Value: 2}},
		},
		{
			name: "int not excluded regex",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntExcludedValues([]any{"^\\d{2}$"})),
			},
			in: map[string]any{
				"my_int": 2,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_int", Value: 2}},
		},
		{
			name: "int minValue",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntMinValue(&intValue), parameters.WithIntMaxValue(nil)),
			},
			in: map[string]any{
				"my_int": 3,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_int", Value: 3}},
		},
		{
			name: "int minValue disallow",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntMinValue(&intValue), parameters.WithIntMaxValue(nil)),
			},
			in: map[string]any{
				"my_int": 1,
			},
		},
		{
			name: "int maxValue",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntMinValue(nil), parameters.WithIntMaxValue(&intValue)),
			},
			in: map[string]any{
				"my_int": 1,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_int", Value: 1}},
		},
		{
			name: "int maxValue disallow",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntMinValue(nil), parameters.WithIntMaxValue(&intValue)),
			},
			in: map[string]any{
				"my_int": 3,
			},
		},
		{
			name: "float",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float"),
			},
			in: map[string]any{
				"my_float": 1.5,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_float", Value: 1.5}},
		},
		{
			name: "not float",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float"),
			},
			in: map[string]any{
				"my_float": true,
			},
		},
		{
			name: "float allowed",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatAllowedValues([]any{1.1})),
			},
			in: map[string]any{
				"my_float": 1.1,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_float", Value: 1.1}},
		},
		{
			name: "float allowed regex",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatAllowedValues([]any{"^0\\.\\d+$"})),
			},
			in: map[string]any{
				"my_float": 0.99,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_float", Value: 0.99}},
		},
		{
			name: "float not allowed",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatAllowedValues([]any{1.1})),
			},
			in: map[string]any{
				"my_float": 1.2,
			},
		},
		{
			name: "float not allowed regex",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatAllowedValues([]any{"^0\\.\\d+$"})),
			},
			in: map[string]any{
				"my_float": 1.99,
			},
		},
		{
			name: "float excluded",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatExcludedValues([]any{1.1})),
			},
			in: map[string]any{
				"my_float": 1.1,
			},
		},
		{
			name: "float excluded regex",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatExcludedValues([]any{"^0\\.\\d+$"})),
			},
			in: map[string]any{
				"my_float": 0.99,
			},
		},
		{
			name: "float not excluded",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatExcludedValues([]any{1.1})),
			},
			in: map[string]any{
				"my_float": 1.2,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_float", Value: 1.2}},
		},
		{
			name: "float not excluded regex",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatExcludedValues([]any{"^0\\.\\d+$"})),
			},
			in: map[string]any{
				"my_float": 1.99,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_float", Value: 1.99}},
		},

		{
			name: "float minValue",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatMinValue(&floatValue), parameters.WithFloatMaxValue(nil)),
			},
			in: map[string]any{
				"my_float": 1.8,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_float", Value: 1.8}},
		},
		{
			name: "float minValue disallow",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatMinValue(&floatValue), parameters.WithFloatMaxValue(nil)),
			},
			in: map[string]any{
				"my_float": 1.2,
			},
		},
		{
			name: "float maxValue",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatMinValue(nil), parameters.WithFloatMaxValue(&floatValue)),
			},
			in: map[string]any{
				"my_float": 1.2,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_float", Value: 1.2}},
		},
		{
			name: "float maxValue disallow",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatMinValue(nil), parameters.WithFloatMaxValue(&floatValue)),
			},
			in: map[string]any{
				"my_float": 1.8,
			},
		},
		{
			name: "bool",
			params: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a bool"),
			},
			in: map[string]any{
				"my_bool": true,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_bool", Value: true}},
		},
		{
			name: "not bool",
			params: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a bool"),
			},
			in: map[string]any{
				"my_bool": 1.5,
			},
		},
		{
			name: "bool allowed",
			params: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a bool", parameters.WithBooleanAllowedValues([]any{false})),
			},
			in: map[string]any{
				"my_bool": false,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_bool", Value: false}},
		},
		{
			name: "bool not allowed",
			params: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a bool", parameters.WithBooleanAllowedValues([]any{false})),
			},
			in: map[string]any{
				"my_bool": true,
			},
		},
		{
			name: "bool excluded",
			params: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a bool", parameters.WithBooleanExcludedValues([]any{true})),
			},
			in: map[string]any{
				"my_bool": true,
			},
		},
		{
			name: "bool not excluded",
			params: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a bool", parameters.WithBooleanExcludedValues([]any{false})),
			},
			in: map[string]any{
				"my_bool": true,
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_bool", Value: true}},
		},
		{
			name: "string default",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringDefault("foo")),
			},
			in:   map[string]any{},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: "foo"}},
		},
		{
			name: "int default",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntDefault(100)),
			},
			in:   map[string]any{},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_int", Value: 100}},
		},
		{
			name: "int (big)",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_big_int", "this param is an int", parameters.WithIntDefault(math.MaxInt64)),
			},
			in:   map[string]any{},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_big_int", Value: math.MaxInt64}},
		},
		{
			name: "float default",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatDefault(1.1)),
			},
			in:   map[string]any{},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_float", Value: 1.1}},
		},
		{
			name: "bool default",
			params: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a bool", parameters.WithBooleanDefault(true)),
			},
			in:   map[string]any{},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_bool", Value: true}},
		},
		{
			name: "string not required",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringRequired(false)),
			},
			in:   map[string]any{},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: nil}},
		},
		{
			name: "int not required",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntRequired(false)),
			},
			in:   map[string]any{},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_int", Value: nil}},
		},
		{
			name: "float not required",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatRequired(false)),
			},
			in:   map[string]any{},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_float", Value: nil}},
		},
		{
			name: "bool not required",
			params: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a bool", parameters.WithBooleanRequired(false)),
			},
			in:   map[string]any{},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_bool", Value: nil}},
		},
		{
			name: "array with string escape",
			params: parameters.Parameters{
				parameters.NewArrayParameter("my_array", "an array", parameters.NewStringParameter("my_string", "string item", parameters.WithStringEscape("backticks"))),
			},
			in: map[string]any{
				"my_array": []string{"val1", "val2"},
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_array", Value: []any{string("`val1`"), string("`val2`")}}},
		},
		{
			name: "map",
			params: parameters.Parameters{
				parameters.NewMapParameter("my_map", "a map", "string"),
			},
			in: map[string]any{
				"my_map": map[string]any{"key1": "val1", "key2": "val2"},
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_map", Value: map[string]any{"key1": "val1", "key2": "val2"}}},
		},
		{
			name: "generic map",
			params: parameters.Parameters{
				parameters.NewMapParameter("my_map_generic_type", "a generic map", ""),
			},
			in: map[string]any{
				"my_map_generic_type": map[string]any{"key1": "val1", "key2": 123, "key3": true},
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_map_generic_type", Value: map[string]any{"key1": "val1", "key2": int64(123), "key3": true}}},
		},
		{
			name: "not map (value type mismatch)",
			params: parameters.Parameters{
				parameters.NewMapParameter("my_map", "a map", "string"),
			},
			in: map[string]any{
				"my_map": map[string]any{"key1": 123},
			},
		},
		{
			name: "map default",
			params: parameters.Parameters{
				parameters.NewMapParameter("my_map_default", "a map", "string", parameters.WithMapDefault(map[string]any{"default_key": "default_val"})),
			},
			in:   map[string]any{},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_map_default", Value: map[string]any{"default_key": "default_val"}}},
		},
		{
			name: "map not required",
			params: parameters.Parameters{
				parameters.NewMapParameter("my_map_not_required", "a map", "string", parameters.WithMapRequired(false)),
			},
			in:   map[string]any{},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_map_not_required", Value: nil}},
		},
		{
			name: "map allowed",
			params: parameters.Parameters{
				parameters.NewMapParameter("my_map", "a map", "string", parameters.WithMapAllowedValues([]any{map[string]any{"key1": "val1"}})),
			},
			in: map[string]any{
				"my_map": map[string]any{"key1": "val1"},
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_map", Value: map[string]any{"key1": "val1"}}},
		},
		{
			name: "map not allowed",
			params: parameters.Parameters{
				parameters.NewMapParameter("my_map", "a map", "string", parameters.WithMapAllowedValues([]any{map[string]any{"key1": "val1"}})),
			},
			in: map[string]any{
				"my_map": map[string]any{"key1": "val2"},
			},
		},
		{
			name: "map excluded",
			params: parameters.Parameters{
				parameters.NewMapParameter("my_map", "a map", "string", parameters.WithMapExcludedValues([]any{map[string]any{"key1": "val1"}})),
			},
			in: map[string]any{
				"my_map": map[string]any{"key1": "val1"},
			},
		},
		{
			name: "map not excluded",
			params: parameters.Parameters{
				parameters.NewMapParameter("my_map", "a map", "string", parameters.WithMapExcludedValues([]any{map[string]any{"key1": "val1"}})),
			},
			in: map[string]any{
				"my_map": map[string]any{"key1": "val2"},
			},
			want: parameters.ParamValues{parameters.ParamValue{Name: "my_map", Value: map[string]any{"key1": "val2"}}},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			// parse map to bytes
			data, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatalf("unable to marshal input to yaml: %s", err)
			}
			// parse bytes to object
			var m map[string]any

			d := json.NewDecoder(bytes.NewReader(data))
			d.UseNumber()
			err = d.Decode(&m)
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}

			wantErr := len(tc.want) == 0 // error is expected if no items in want
			gotAll, err := parameters.ParseParams(tc.params, m, make(map[string]map[string]any))
			if err != nil {
				if wantErr {
					return
				}
				t.Fatalf("unexpected error from ParseParams: %s", err)
			}
			if wantErr {
				t.Fatalf("expected error but Param parsed successfully: %s", gotAll)
			}

			// Use cmp.Diff for robust comparison
			if diff := cmp.Diff(tc.want, gotAll); diff != "" {
				t.Fatalf("ParseParams() mismatch (-want +got):\n%s", diff)
			}
		})
	}
	t.Run("CheckNullForRequiredParam", func(t *testing.T) {
		// Define a required string parameter
		params := parameters.Parameters{
			parameters.NewStringParameter("required_param", "this is required"),
		}

		// Input map with explicit nil
		input := map[string]any{
			"required_param": nil,
		}

		// Call ParseParams
		_, err := parameters.ParseParams(params, input, nil)

		// Expect an error because the parameter is required
		if err == nil {
			t.Errorf("ParseParams allowed explicit nil for required parameter, expected error")
		}
	})
}

func TestParseTypeErrorReportsValue(t *testing.T) {
	// When a value fails the type assertion, the error must report the actual
	// offending value, not the zero value of the target type. The primitive
	// parameters already do this; array and map parameters used to pass their
	// (nil) asserted variable instead of the input, producing useless errors
	// like `%!q(<nil>) not type "array"` that hid the bad value from the user.
	tcs := []struct {
		name  string
		param parameters.Parameter
		in    string
	}{
		{
			name:  "array given a non-array",
			param: parameters.NewArrayParameter("my_array", "an array", parameters.NewStringParameter("my_string", "string item")),
			in:    "not-an-array",
		},
		{
			name:  "map given a non-map",
			param: parameters.NewMapParameter("my_map", "a map", "string"),
			in:    "not-a-map",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.param.Parse(tc.in)
			if err == nil {
				t.Fatalf("Parse(%q) expected a type error, got nil", tc.in)
			}
			if got := err.Error(); !strings.Contains(got, tc.in) {
				t.Errorf("error %q does not mention the offending value %q", got, tc.in)
			}
		})
	}
}

func TestAuthParametersParse(t *testing.T) {
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
		name      string
		params    parameters.Parameters
		in        map[string]any
		claimsMap map[string]map[string]any
		want      parameters.ParamValues
	}{
		{
			name: "string",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringAuth(authServices)),
			},
			in: map[string]any{
				"my_string": "hello world",
			},
			claimsMap: map[string]map[string]any{"my-google-auth-service": {"auth_field": "hello"}},
			want:      parameters.ParamValues{parameters.ParamValue{Name: "my_string", Value: "hello"}},
		},
		{
			name: "not string",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string", "this param is a string", parameters.WithStringAuth(authServices)),
			},
			in: map[string]any{
				"my_string": 4,
			},
			claimsMap: map[string]map[string]any{},
		},
		{
			name: "int",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntAuth(authServices)),
			},
			in: map[string]any{
				"my_int": 100,
			},
			claimsMap: map[string]map[string]any{"other-auth-service": {"other_auth_field": 120}},
			want:      parameters.ParamValues{parameters.ParamValue{Name: "my_int", Value: 120}},
		},
		{
			name: "not int",
			params: parameters.Parameters{
				parameters.NewIntParameter("my_int", "this param is an int", parameters.WithIntAuth(authServices)),
			},
			in: map[string]any{
				"my_int": 14.5,
			},
			claimsMap: map[string]map[string]any{},
		},
		{
			name: "float",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatAuth(authServices)),
			},
			in: map[string]any{
				"my_float": 1.5,
			},
			claimsMap: map[string]map[string]any{"my-google-auth-service": {"auth_field": 2.1}},
			want:      parameters.ParamValues{parameters.ParamValue{Name: "my_float", Value: 2.1}},
		},
		{
			name: "not float",
			params: parameters.Parameters{
				parameters.NewFloatParameter("my_float", "this param is a float", parameters.WithFloatAuth(authServices)),
			},
			in: map[string]any{
				"my_float": true,
			},
			claimsMap: map[string]map[string]any{},
		},
		{
			name: "bool",
			params: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a bool", parameters.WithBooleanAuth(authServices)),
			},
			in: map[string]any{
				"my_bool": true,
			},
			claimsMap: map[string]map[string]any{"my-google-auth-service": {"auth_field": false}},
			want:      parameters.ParamValues{parameters.ParamValue{Name: "my_bool", Value: false}},
		},
		{
			name: "not bool",
			params: parameters.Parameters{
				parameters.NewBooleanParameter("my_bool", "this param is a bool", parameters.WithBooleanAuth(authServices)),
			},
			in: map[string]any{
				"my_bool": 1.5,
			},
			claimsMap: map[string]map[string]any{},
		},
		{
			name: "username",
			params: parameters.Parameters{
				parameters.NewStringParameter("username", "username string", parameters.WithStringAuth(authServices)),
			},
			in: map[string]any{
				"username": "Violet",
			},
			claimsMap: map[string]map[string]any{"my-google-auth-service": {"auth_field": "Alice"}},
			want:      parameters.ParamValues{parameters.ParamValue{Name: "username", Value: "Alice"}},
		},
		{
			name: "expect claim error",
			params: parameters.Parameters{
				parameters.NewStringParameter("username", "username string", parameters.WithStringAuth(authServices)),
			},
			in: map[string]any{
				"username": "Violet",
			},
			claimsMap: map[string]map[string]any{"my-google-auth-service": {"not_an_auth_field": "Alice"}},
		},
		{
			name: "map",
			params: parameters.Parameters{
				parameters.NewMapParameter("my_map", "a map", "string", parameters.WithMapAuth(authServices)),
			},
			in:        map[string]any{"my_map": map[string]any{"key1": "val1"}},
			claimsMap: map[string]map[string]any{"my-google-auth-service": {"auth_field": map[string]any{"authed_key": "authed_val"}}},
			want:      parameters.ParamValues{parameters.ParamValue{Name: "my_map", Value: map[string]any{"authed_key": "authed_val"}}},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			// parse map to bytes
			data, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatalf("unable to marshal input to yaml: %s", err)
			}
			// parse bytes to object
			var m map[string]any
			d := json.NewDecoder(bytes.NewReader(data))
			d.UseNumber()
			err = d.Decode(&m)
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}

			gotAll, err := parameters.ParseParams(tc.params, m, tc.claimsMap)
			if err != nil {
				if len(tc.want) == 0 {
					// error is expected if no items in want
					return
				}
				t.Fatalf("unexpected error from ParseParams: %s", err)
			}

			if diff := cmp.Diff(tc.want, gotAll); diff != "" {
				t.Fatalf("ParseParams() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParamValues(t *testing.T) {
	tcs := []struct {
		name              string
		in                parameters.ParamValues
		wantSlice         []any
		wantMap           map[string]interface{}
		wantMapOrdered    map[string]interface{}
		wantMapWithDollar map[string]interface{}
	}{
		{
			name:           "string",
			in:             parameters.ParamValues{parameters.ParamValue{Name: "my_bool", Value: true}, parameters.ParamValue{Name: "my_string", Value: "hello world"}},
			wantSlice:      []any{true, "hello world"},
			wantMap:        map[string]interface{}{"my_bool": true, "my_string": "hello world"},
			wantMapOrdered: map[string]interface{}{"p1": true, "p2": "hello world"},
			wantMapWithDollar: map[string]interface{}{
				"$my_bool":   true,
				"$my_string": "hello world",
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			gotSlice := tc.in.AsSlice()
			gotMap := tc.in.AsMap()
			gotMapOrdered := tc.in.AsMapByOrderedKeys()
			gotMapWithDollar := tc.in.AsMapWithDollarPrefix()

			for i, got := range gotSlice {
				want := tc.wantSlice[i]
				if got != want {
					t.Fatalf("unexpected value: got %q, want %q", got, want)
				}
			}
			for i, got := range gotMap {
				want := tc.wantMap[i]
				if got != want {
					t.Fatalf("unexpected value: got %q, want %q", got, want)
				}
			}
			for i, got := range gotMapOrdered {
				want := tc.wantMapOrdered[i]
				if got != want {
					t.Fatalf("unexpected value: got %q, want %q", got, want)
				}
			}
			for key, got := range gotMapWithDollar {
				want := tc.wantMapWithDollar[key]
				if got != want {
					t.Fatalf("unexpected value in AsMapWithDollarPrefix: got %q, want %q", got, want)
				}
			}
		})
	}
}

func TestParamManifest(t *testing.T) {
	tcs := []struct {
		name string
		in   parameters.Parameter
		want parameters.ParameterManifest
	}{
		{
			name: "string",
			in:   parameters.NewStringParameter("foo-string", "bar"),
			want: parameters.ParameterManifest{Name: "foo-string", Type: "string", Required: true, Description: "bar", AuthServices: []string{}},
		},
		{
			name: "int",
			in:   parameters.NewIntParameter("foo-int", "bar"),
			want: parameters.ParameterManifest{Name: "foo-int", Type: "integer", Required: true, Description: "bar", AuthServices: []string{}},
		},
		{
			name: "float",
			in:   parameters.NewFloatParameter("foo-float", "bar"),
			want: parameters.ParameterManifest{Name: "foo-float", Type: "float", Required: true, Description: "bar", AuthServices: []string{}},
		},
		{
			name: "boolean",
			in:   parameters.NewBooleanParameter("foo-bool", "bar"),
			want: parameters.ParameterManifest{Name: "foo-bool", Type: "boolean", Required: true, Description: "bar", AuthServices: []string{}},
		},
		{
			name: "array",
			in:   parameters.NewArrayParameter("foo-array", "bar", parameters.NewStringParameter("foo-string", "bar")),
			want: parameters.ParameterManifest{
				Name:         "foo-array",
				Type:         "array",
				Required:     true,
				Description:  "bar",
				AuthServices: []string{},
				Items:        &parameters.ParameterManifest{Name: "foo-string", Type: "string", Required: true, Description: "bar", AuthServices: []string{}},
			},
		},
		{
			name: "string default",
			in:   parameters.NewStringParameter("foo-string", "bar", parameters.WithStringDefault("foo")),
			want: parameters.ParameterManifest{Name: "foo-string", Type: "string", Required: false, Description: "bar", Default: "foo", AuthServices: []string{}},
		},
		{
			name: "int default",
			in:   parameters.NewIntParameter("foo-int", "bar", parameters.WithIntDefault(1)),
			want: parameters.ParameterManifest{Name: "foo-int", Type: "integer", Required: false, Description: "bar", Default: 1, AuthServices: []string{}},
		},
		{
			name: "float default",
			in:   parameters.NewFloatParameter("foo-float", "bar", parameters.WithFloatDefault(1.1)),
			want: parameters.ParameterManifest{Name: "foo-float", Type: "float", Required: false, Description: "bar", Default: 1.1, AuthServices: []string{}},
		},
		{
			name: "boolean default",
			in:   parameters.NewBooleanParameter("foo-bool", "bar", parameters.WithBooleanDefault(true)),
			want: parameters.ParameterManifest{Name: "foo-bool", Type: "boolean", Required: false, Description: "bar", Default: true, AuthServices: []string{}},
		},
		{
			name: "array default",
			in:   parameters.NewArrayParameter("foo-array", "bar", parameters.NewStringParameter("foo-string", "bar"), parameters.WithArrayDefault([]any{"foo", "bar"})),
			want: parameters.ParameterManifest{
				Name:         "foo-array",
				Type:         "array",
				Required:     false,
				Description:  "bar",
				Default:      []any{"foo", "bar"},
				AuthServices: []string{},
				Items:        &parameters.ParameterManifest{Name: "foo-string", Type: "string", Required: false, Description: "bar", AuthServices: []string{}},
			},
		},
		{
			name: "string not required",
			in:   parameters.NewStringParameter("foo-string", "bar", parameters.WithStringRequired(false)),
			want: parameters.ParameterManifest{Name: "foo-string", Type: "string", Required: false, Description: "bar", AuthServices: []string{}},
		},
		{
			name: "int not required",
			in:   parameters.NewIntParameter("foo-int", "bar", parameters.WithIntRequired(false)),
			want: parameters.ParameterManifest{Name: "foo-int", Type: "integer", Required: false, Description: "bar", AuthServices: []string{}},
		},
		{
			name: "float not required",
			in:   parameters.NewFloatParameter("foo-float", "bar", parameters.WithFloatRequired(false)),
			want: parameters.ParameterManifest{Name: "foo-float", Type: "float", Required: false, Description: "bar", AuthServices: []string{}},
		},
		{
			name: "boolean not required",
			in:   parameters.NewBooleanParameter("foo-bool", "bar", parameters.WithBooleanRequired(false)),
			want: parameters.ParameterManifest{Name: "foo-bool", Type: "boolean", Required: false, Description: "bar", AuthServices: []string{}},
		},
		{
			name: "array not required",
			in:   parameters.NewArrayParameter("foo-array", "bar", parameters.NewStringParameter("foo-string", "bar"), parameters.WithArrayRequired(false)),
			want: parameters.ParameterManifest{
				Name:         "foo-array",
				Type:         "array",
				Required:     false,
				Description:  "bar",
				AuthServices: []string{},
				Items:        &parameters.ParameterManifest{Name: "foo-string", Type: "string", Required: false, Description: "bar", AuthServices: []string{}},
			},
		},
		{
			name: "map with string values",
			in:   parameters.NewMapParameter("foo-map", "bar", "string"),
			want: parameters.ParameterManifest{
				Name:                 "foo-map",
				Type:                 "object",
				Required:             true,
				Description:          "bar",
				AuthServices:         []string{},
				AdditionalProperties: map[string]any{"type": "string"},
			},
		},
		{
			name: "map not required",
			in:   parameters.NewMapParameter("foo-map", "bar", "string", parameters.WithMapRequired(false)),
			want: parameters.ParameterManifest{
				Name:                 "foo-map",
				Type:                 "object",
				Required:             false,
				Description:          "bar",
				AuthServices:         []string{},
				AdditionalProperties: map[string]any{"type": "string"},
			},
		},
		{
			name: "generic map (additionalProperties true)",
			in:   parameters.NewMapParameter("foo-map", "bar", ""),
			want: parameters.ParameterManifest{
				Name:                 "foo-map",
				Type:                 "object",
				Required:             true,
				Description:          "bar",
				AuthServices:         []string{},
				AdditionalProperties: true,
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Manifest()
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("unexpected manifest (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParamMcpManifest(t *testing.T) {
	tcs := []struct {
		name          string
		in            parameters.Parameter
		want          parameters.ParameterMcpManifest
		wantAuthParam []string
	}{
		{
			name:          "string",
			in:            parameters.NewStringParameter("foo-string", "bar"),
			want:          parameters.ParameterMcpManifest{Type: "string", Description: "bar"},
			wantAuthParam: []string{},
		},
		{
			name:          "int",
			in:            parameters.NewIntParameter("foo-int", "bar"),
			want:          parameters.ParameterMcpManifest{Type: "integer", Description: "bar"},
			wantAuthParam: []string{},
		},
		{
			name:          "float",
			in:            parameters.NewFloatParameter("foo-float", "bar"),
			want:          parameters.ParameterMcpManifest{Type: "number", Description: "bar"},
			wantAuthParam: []string{},
		},
		{
			name:          "boolean",
			in:            parameters.NewBooleanParameter("foo-bool", "bar"),
			want:          parameters.ParameterMcpManifest{Type: "boolean", Description: "bar"},
			wantAuthParam: []string{},
		},
		{
			name: "array",
			in:   parameters.NewArrayParameter("foo-array", "bar", parameters.NewStringParameter("foo-string", "bar")),
			want: parameters.ParameterMcpManifest{
				Type:        "array",
				Description: "bar",
				Items:       &parameters.ParameterMcpManifest{Type: "string", Description: "bar"},
			},
			wantAuthParam: []string{},
		},

		{
			name: "map with string values",
			in:   parameters.NewMapParameter("foo-map", "bar", "string"),
			want: parameters.ParameterMcpManifest{
				Type:                 "object",
				Description:          "bar",
				AdditionalProperties: map[string]any{"type": "string"},
			},
			wantAuthParam: []string{},
		},
		{
			name: "generic map (additionalProperties true)",
			in:   parameters.NewMapParameter("foo-map", "bar", ""),
			want: parameters.ParameterMcpManifest{
				Type:                 "object",
				Description:          "bar",
				AdditionalProperties: true,
			},
			wantAuthParam: []string{},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got, gotAuthParam := tc.in.McpManifest()
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("unexpected manifest (-want +got):\n%s", diff)
			}
			slices.Sort(gotAuthParam)
			if !reflect.DeepEqual(gotAuthParam, tc.wantAuthParam) {
				t.Fatalf("unexpected auth param list: got %s, want %s", gotAuthParam, tc.wantAuthParam)
			}
		})
	}
}

func TestFailParametersUnmarshal(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		name string
		in   []map[string]any
		err  string
	}{
		{
			name: "common parameter missing name",
			in: []map[string]any{
				{
					"type":        "string",
					"description": "this is a param for string",
				},
			},
			err: "unable to parse as \"string\": Key: 'CommonParameter.Name' Error:Field validation for 'Name' failed on the 'required' tag",
		},
		{
			name: "common parameter missing type",
			in: []map[string]any{
				{
					"name":        "string",
					"description": "this is a param for string",
				},
			},
			err: "parameter is missing 'type' field",
		},
		{
			name: "common parameter with non-string type",
			in: []map[string]any{
				{
					"name":        "my_string",
					"type":        123,
					"description": "this is a param with a numeric type",
				},
			},
			err: "parameter 'type' field must be a string",
		},
		{
			name: "common parameter missing description",
			in: []map[string]any{
				{
					"name": "my_string",
					"type": "string",
				},
			},
			err: "unable to parse as \"string\": Key: 'CommonParameter.Desc' Error:Field validation for 'Desc' failed on the 'required' tag",
		},
		{
			name: "array parameter missing items",
			in: []map[string]any{
				{
					"name":        "my_array",
					"type":        "array",
					"description": "this param is an array of strings",
				},
			},
			err: "unable to parse as \"array\": unable to parse 'items' field: error parsing parameters: nothing to unmarshal",
		},
		{
			name: "array parameter missing items' name",
			in: []map[string]any{
				{
					"name":        "my_array",
					"type":        "array",
					"description": "this param is an array of strings",
					"items": map[string]string{
						"type":        "string",
						"description": "string item",
					},
				},
			},
			err: "unable to parse as \"array\": unable to parse 'items' field: unable to parse as \"string\": Key: 'CommonParameter.Name' Error:Field validation for 'Name' failed on the 'required' tag",
		},
		// --- MODIFIED MAP PARAMETER TEST ---
		{
			name: "map with invalid valueType",
			in: []map[string]any{
				{
					"name":        "my_map",
					"type":        "map",
					"description": "this param is a map",
					"valueType":   "not-a-real-type",
				},
			},
			err: "unsupported valueType \"not-a-real-type\" for map parameter",
		},
		{
			name: "both secure and authServices",
			in: []map[string]any{
				{
					"name":        "my_string",
					"type":        "string",
					"description": "this param",
					"secure":      true,
					"authServices": []map[string]any{
						{"name": "service_one"},
					},
				},
			},
			err: "parameter \"my_string\" cannot have both 'secure' set to true and 'authServices' specified",
		},
		{
			name: "both secure and default",
			in: []map[string]any{
				{
					"name":        "my_string",
					"type":        "string",
					"description": "this param",
					"secure":      true,
					"default":     "def_val",
				},
			},
			err: "parameter \"my_string\" cannot have both 'secure' set to true and 'default' specified",
		},
		{
			name: "both secure and required false",
			in: []map[string]any{
				{
					"name":        "my_string",
					"type":        "string",
					"description": "this param",
					"secure":      true,
					"required":    false,
				},
			},
			err: "parameter \"my_string\" cannot have both 'secure' set to true and 'required' set to false",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			var got parameters.Parameters
			// parse map to bytes
			data, err := yaml.Marshal(tc.in)
			if err != nil {
				t.Fatalf("unable to marshal input to yaml: %s", err)
			}
			// parse bytes to object
			err = yaml.UnmarshalContext(ctx, data, &got)
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()

			if !strings.Contains(errStr, tc.err) {
				t.Fatalf("unexpected error: got %q, want to contain %q", errStr, tc.err)
			}
		})
	}
}

// ... (Remaining test functions do not involve parameter definitions and need no changes)

func TestConvertArrayParamToString(t *testing.T) {

	tcs := []struct {
		name string
		in   []any
		want string
	}{
		{
			in: []any{
				"id",
				"name",
				"location",
			},
			want: "id, name, location",
		},
		{
			in: []any{
				"id",
			},
			want: "id",
		},
		{
			in: []any{
				"id",
				"5",
				"false",
			},
			want: "id, 5, false",
		},
		{
			in:   []any{},
			want: "",
		},
		{
			in:   []any{},
			want: "",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := parameters.ConvertArrayParamToString(tc.in)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect array param conversion: diff %v", diff)
			}
		})
	}
}

func TestFailConvertArrayParamToString(t *testing.T) {
	tcs := []struct {
		name string
		in   []any
		err  string
	}{
		{
			in:  []any{5, 10, 15},
			err: "templateParameter only supports string arrays",
		},
		{
			in:  []any{"id", "name", 15},
			err: "templateParameter only supports string arrays",
		},
		{
			in:  []any{false},
			err: "templateParameter only supports string arrays",
		},
		{
			in:  []any{10, true},
			err: "templateParameter only supports string arrays",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parameters.ConvertArrayParamToString(tc.in)
			errStr := err.Error()
			if errStr != tc.err {
				t.Fatalf("unexpected error: got %q, want %q", errStr, tc.err)
			}
		})
	}
}

func TestGetParams(t *testing.T) {
	tcs := []struct {
		name   string
		in     map[string]any
		params parameters.Parameters
		want   parameters.ParamValues
	}{
		{
			name: "parameters to include and exclude",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string_inc", "this should be included"),
				parameters.NewStringParameter("my_string_inc2", "this should be included"),
			},
			in: map[string]any{
				"my_string_inc":  "hello world A",
				"my_string_inc2": "hello world B",
				"my_string_exc":  "hello world C",
			},
			want: parameters.ParamValues{
				parameters.ParamValue{Name: "my_string_inc", Value: "hello world A"},
				parameters.ParamValue{Name: "my_string_inc2", Value: "hello world B"},
			},
		},
		{
			name: "include all",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string_inc", "this should be included"),
			},
			in: map[string]any{
				"my_string_inc": "hello world A",
			},
			want: parameters.ParamValues{
				parameters.ParamValue{Name: "my_string_inc", Value: "hello world A"},
			},
		},
		{
			name:   "exclude all",
			params: parameters.Parameters{},
			in: map[string]any{
				"my_string_exc":  "hello world A",
				"my_string_exc2": "hello world B",
			},
			want: parameters.ParamValues{},
		},
		{
			name:   "empty",
			params: parameters.Parameters{},
			in:     map[string]any{},
			want:   parameters.ParamValues{},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := parameters.GetParams(tc.params, tc.in)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect get params: diff %v", diff)
			}
		})
	}
}

func TestFailGetParams(t *testing.T) {

	tcs := []struct {
		name   string
		params parameters.Parameters
		in     map[string]any
		err    string
	}{
		{
			name:   "missing the only parameter",
			params: parameters.Parameters{parameters.NewStringParameter("my_string", "this was missing")},
			in:     map[string]any{},
			err:    "missing parameter my_string",
		},
		{
			name: "missing one parameter of multiple",
			params: parameters.Parameters{
				parameters.NewStringParameter("my_string_inc", "this should be included"),
				parameters.NewStringParameter("my_string_exc", "this was missing"),
			},
			in: map[string]any{
				"my_string_inc": "hello world A",
			},
			err: "missing parameter my_string_exc",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parameters.GetParams(tc.params, tc.in)
			errStr := err.Error()
			if errStr != tc.err {
				t.Fatalf("unexpected error: got %q, want %q", errStr, tc.err)
			}
		})
	}
}

func TestResolveTemplateParameters(t *testing.T) {
	tcs := []struct {
		name           string
		templateParams parameters.Parameters
		statement      string
		in             map[string]any
		want           string
	}{
		{
			name: "single template parameter",
			templateParams: parameters.Parameters{
				parameters.NewStringParameter("tableName", "this is a string template parameter"),
			},
			statement: "SELECT * FROM {{.tableName}}",
			in: map[string]any{
				"tableName": "hotels",
			},
			want: "SELECT * FROM hotels",
		},
		{
			name: "multiple template parameters",
			templateParams: parameters.Parameters{
				parameters.NewStringParameter("tableName", "this is a string template parameter"),
				parameters.NewStringParameter("columnName", "this is a string template parameter"),
			},
			statement: "SELECT * FROM {{.tableName}} WHERE {{.columnName}} = 'Hilton'",
			in: map[string]any{
				"tableName":  "hotels",
				"columnName": "name",
			},
			want: "SELECT * FROM hotels WHERE name = 'Hilton'",
		},
		{
			name: "standard and template parameter",
			templateParams: parameters.Parameters{
				parameters.NewStringParameter("tableName", "this is a string template parameter"),
				parameters.NewStringParameter("hotelName", "this is a string parameter"),
			},
			statement: "SELECT * FROM {{.tableName}} WHERE name = $1",
			in: map[string]any{
				"tableName": "hotels",
				"hotelName": "name",
			},
			want: "SELECT * FROM hotels WHERE name = $1",
		},
		{
			name: "standard parameter",
			templateParams: parameters.Parameters{
				parameters.NewStringParameter("hotelName", "this is a string parameter"),
			},
			statement: "SELECT * FROM hotels WHERE name = $1",
			in: map[string]any{
				"hotelName": "hotels",
			},
			want: "SELECT * FROM hotels WHERE name = $1",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := parameters.ResolveTemplateParams(tc.templateParams, tc.statement, tc.in)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect resolved template params: diff %v", diff)
			}
		})
	}
}

func TestFailResolveTemplateParameters(t *testing.T) {
	tcs := []struct {
		name           string
		templateParams parameters.Parameters
		statement      string
		in             map[string]any
		err            string
	}{
		{
			name: "wrong param name",
			templateParams: parameters.Parameters{
				parameters.NewStringParameter("tableName", "this is a string template parameter"),
			},
			statement: "SELECT * FROM {{.missingParam}}",
			in:        map[string]any{},
			err:       "error getting template params missing parameter tableName",
		},
		{
			name: "incomplete param template",
			templateParams: parameters.Parameters{
				parameters.NewStringParameter("tableName", "this is a string template parameter"),
			},
			statement: "SELECT * FROM {{.tableName",
			in: map[string]any{
				"tableName": "hotels",
			},
			err: "error creating go template template: statement:1: unclosed action",
		},
		{
			name: "undefined function",
			templateParams: parameters.Parameters{
				parameters.NewStringParameter("tableName", "this is a string template parameter"),
			},
			statement: "SELECT * FROM {{json .tableName}}",
			in: map[string]any{
				"tableName": "hotels",
			},
			err: "error creating go template template: statement:1: function \"json\" not defined",
		},
		{
			name: "undefined method",
			templateParams: parameters.Parameters{
				parameters.NewStringParameter("tableName", "this is a string template parameter"),
			},
			statement: "SELECT * FROM {{.tableName .wrong}}",
			in: map[string]any{
				"tableName": "hotels",
			},
			err: "error executing go template template: statement:1:16: executing \"statement\" at <.tableName>: tableName is not a method but has arguments",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parameters.ResolveTemplateParams(tc.templateParams, tc.statement, tc.in)
			errStr := err.Error()
			if errStr != tc.err {
				t.Fatalf("unexpected error: got %q, want %q", errStr, tc.err)
			}
		})
	}
}

func TestCheckParamRequired(t *testing.T) {
	tcs := []struct {
		name     string
		required bool
		defaultV any
		want     bool
	}{
		{
			name:     "required and no default",
			required: true,
			defaultV: nil,
			want:     true,
		},
		{
			name:     "required and default",
			required: true,
			defaultV: "foo",
			want:     false,
		},
		{
			name:     "not required and no default",
			required: false,
			defaultV: nil,
			want:     false,
		},
		{
			name:     "not required and default",
			required: false,
			defaultV: "foo",
			want:     false,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got := parameters.CheckParamRequired(tc.required, tc.defaultV)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProcessParametersValidation(t *testing.T) {
	tcs := []struct {
		name           string
		templateParams parameters.Parameters
		params         parameters.Parameters
		wantErr        bool
		errStr         string
	}{
		{
			name:           "valid - only secure",
			templateParams: nil,
			params: parameters.Parameters{
				&parameters.StringParameter{
					CommonParameter: parameters.CommonParameter{
						Name:   "p1",
						Type:   parameters.TypeString,
						Desc:   "desc",
						Secure: true,
					},
				},
			},
			wantErr: false,
		},
		{
			name:           "valid - only authServices",
			templateParams: nil,
			params: parameters.Parameters{
				&parameters.StringParameter{
					CommonParameter: parameters.CommonParameter{
						Name: "p1",
						Type: parameters.TypeString,
						Desc: "desc",
						AuthServices: []parameters.ParamAuthService{
							{Name: "auth1"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:           "invalid - both secure and authServices",
			templateParams: nil,
			params: parameters.Parameters{
				&parameters.StringParameter{
					CommonParameter: parameters.CommonParameter{
						Name:   "p1",
						Type:   parameters.TypeString,
						Desc:   "desc",
						Secure: true,
						AuthServices: []parameters.ParamAuthService{
							{Name: "auth1"},
						},
					},
				},
			},
			wantErr: true,
			errStr:  `parameter "p1" cannot have both 'secure' set to true and 'authServices' specified`,
		},
		{
			name: "invalid - both secure and authServices in templateParams",
			templateParams: parameters.Parameters{
				&parameters.StringParameter{
					CommonParameter: parameters.CommonParameter{
						Name:   "p1",
						Type:   parameters.TypeString,
						Desc:   "desc",
						Secure: true,
						AuthServices: []parameters.ParamAuthService{
							{Name: "auth1"},
						},
					},
				},
			},
			params:  nil,
			wantErr: true,
			errStr:  `parameter "p1" cannot have both 'secure' set to true and 'authServices' specified`,
		},
		{
			name:           "valid - secure with required true",
			templateParams: nil,
			params: parameters.Parameters{
				&parameters.StringParameter{
					CommonParameter: parameters.CommonParameter{
						Name:     "p1",
						Type:     parameters.TypeString,
						Desc:     "desc",
						Secure:   true,
						Required: ptr(true),
					},
				},
			},
			wantErr: false,
		},
		{
			name:           "invalid - both secure and default",
			templateParams: nil,
			params: parameters.Parameters{
				&parameters.StringParameter{
					CommonParameter: parameters.CommonParameter{
						Name:   "p1",
						Type:   parameters.TypeString,
						Desc:   "desc",
						Secure: true,
					},
					Default: ptr("def"),
				},
			},
			wantErr: true,
			errStr:  `parameter "p1" cannot have both 'secure' set to true and 'default' specified`,
		},
		{
			name: "invalid - both secure and default in templateParams",
			templateParams: parameters.Parameters{
				&parameters.StringParameter{
					CommonParameter: parameters.CommonParameter{
						Name:   "p1",
						Type:   parameters.TypeString,
						Desc:   "desc",
						Secure: true,
					},
					Default: ptr("def"),
				},
			},
			params:  nil,
			wantErr: true,
			errStr:  `parameter "p1" cannot have both 'secure' set to true and 'default' specified`,
		},
		{
			name:           "invalid - both secure and required false",
			templateParams: nil,
			params: parameters.Parameters{
				&parameters.StringParameter{
					CommonParameter: parameters.CommonParameter{
						Name:     "p1",
						Type:     parameters.TypeString,
						Desc:     "desc",
						Secure:   true,
						Required: ptr(false),
					},
				},
			},
			wantErr: true,
			errStr:  `parameter "p1" cannot have both 'secure' set to true and 'required' set to false`,
		},
		{
			name: "invalid - both secure and required false in templateParams",
			templateParams: parameters.Parameters{
				&parameters.StringParameter{
					CommonParameter: parameters.CommonParameter{
						Name:     "p1",
						Type:     parameters.TypeString,
						Desc:     "desc",
						Secure:   true,
						Required: ptr(false),
					},
				},
			},
			params:  nil,
			wantErr: true,
			errStr:  `parameter "p1" cannot have both 'secure' set to true and 'required' set to false`,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parameters.ProcessParameters(tc.templateParams, tc.params)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ProcessParameters() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && err != nil && !strings.Contains(err.Error(), tc.errStr) {
				t.Fatalf("ProcessParameters() error = %v, want error containing %q", err, tc.errStr)
			}
		})
	}
}

func ptr[T any](v T) *T {
	return &v
}
