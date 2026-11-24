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
	"strings"
	"testing"

	yaml "github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestMessageUnmarshalYAML(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		yamlInput map[string]any
		want      prompts.Message
		wantErr   string
	}{
		{
			name:      "Valid role: user",
			yamlInput: map[string]any{"role": "user", "content": "Hello"},
			want:      prompts.Message{Role: "user", Content: "Hello"},
		},
		{
			name:      "Valid role: assistant",
			yamlInput: map[string]any{"role": "assistant", "content": "Hi there"},
			want:      prompts.Message{Role: "assistant", Content: "Hi there"},
		},
		{
			name:      "Role is omitted, defaults to user",
			yamlInput: map[string]any{"content": "A message with no role"},
			want:      prompts.Message{Role: "user", Content: "A message with no role"},
		},
		{
			name:      "Invalid role: other",
			yamlInput: map[string]any{"role": "other", "content": "Some other role"},
			wantErr:   `invalid role "other": must be 'user' or 'assistant'`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			yamlBytes, err := yaml.Marshal(tc.yamlInput)
			if err != nil {
				t.Fatalf("Test setup failure: could not marshal test input: %v", err)
			}

			var got prompts.Message
			err = yaml.Unmarshal(yamlBytes, &got)

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
					t.Errorf("unmarshal mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestSubstituteMessages(t *testing.T) {
	t.Parallel()
	t.Run("Success", func(t *testing.T) {
		arguments := prompts.Arguments{
			{Parameter: parameters.NewStringParameter("name", "The name to use.")},
			{Parameter: parameters.NewStringParameter("location", "The location.", parameters.WithStringRequired(false))},
		}
		messages := []prompts.Message{
			{Role: "user", Content: "Hello, my name is {{.name}} and I am in {{.location}}."},
			{Role: "assistant", Content: "Nice to meet you, {{.name}}!"},
		}
		argValues := parameters.ParamValues{
			{Name: "name", Value: "Alice"},
			{Name: "location", Value: "Wonderland"},
		}

		want := []prompts.Message{
			{Role: "user", Content: "Hello, my name is Alice and I am in Wonderland."},
			{Role: "assistant", Content: "Nice to meet you, Alice!"},
		}

		got, err := prompts.SubstituteMessages(messages, arguments, argValues)
		if err != nil {
			t.Fatalf("SubstituteMessages() failed: %v", err)
		}

		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("SubstituteMessages() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("OmittedOptionalArgument", func(t *testing.T) {
		arguments := prompts.Arguments{
			{Parameter: parameters.NewStringParameter("name", "The name to use.")},
			{Parameter: parameters.NewStringParameter("location", "The location.", parameters.WithStringRequired(false))},
		}
		messages := []prompts.Message{
			{Role: "user", Content: "Hello, my name is {{.name}}.{{with .location}} I am in {{.}}.{{end}} Location: {{.location}}"},
		}
		argValues, err := prompts.ParseArguments(arguments, map[string]any{"name": "Alice"}, nil)
		if err != nil {
			t.Fatalf("ParseArguments() failed: %v", err)
		}

		want := []prompts.Message{
			{Role: "user", Content: "Hello, my name is Alice. Location: "},
		}

		got, err := prompts.SubstituteMessages(messages, arguments, argValues)
		if err != nil {
			t.Fatalf("SubstituteMessages() failed: %v", err)
		}

		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("SubstituteMessages() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("FailureInvalidTemplate", func(t *testing.T) {
		arguments := prompts.Arguments{}
		messages := []prompts.Message{
			{Content: "This has an {{.unclosed template"},
		}
		argValues := parameters.ParamValues{}

		_, err := prompts.SubstituteMessages(messages, arguments, argValues)
		if err == nil {
			t.Fatal("expected an error for invalid template, but got nil")
		}
		wantErr := "unexpected <template> in operand"
		if !strings.Contains(err.Error(), wantErr) {
			t.Errorf("error mismatch:\n  want to contain: %q\n  got: %q", wantErr, err.Error())
		}
	})
}
