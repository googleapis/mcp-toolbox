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

package gemini_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels"
	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels/gemini"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlGemini(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.EmbeddingModelConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: embeddingModel
			name: my-gemini-model
			type: gemini
			model: gemini-embedding-001
            `,
			want: map[string]embeddingmodels.EmbeddingModelConfig{
				"my-gemini-model": gemini.Config{
					Name:  "my-gemini-model",
					Type:  gemini.EmbeddingModelType,
					Model: "gemini-embedding-001",
				},
			},
		},
		{
			desc: "full example with Google AI fields",
			in: `
            kind: embeddingModel
            name: complex-gemini
            type: gemini
            model: gemini-embedding-001
            apiKey: "test-api-key"
            dimension: 768
            `,
			want: map[string]embeddingmodels.EmbeddingModelConfig{
				"complex-gemini": gemini.Config{
					Name:      "complex-gemini",
					Type:      gemini.EmbeddingModelType,
					Model:     "gemini-embedding-001",
					ApiKey:    "test-api-key",
					Dimension: 768,
				},
			},
		},
		{
			desc: "Vertex AI configuration",
			in: `
            kind: embeddingModel
            name: vertex-gemini
            type: gemini
            model: gemini-embedding-001
            project: "my-project"
            location: "us-central1"
            dimension: 512
            `,
			want: map[string]embeddingmodels.EmbeddingModelConfig{
				"vertex-gemini": gemini.Config{
					Name:      "vertex-gemini",
					Type:      gemini.EmbeddingModelType,
					Model:     "gemini-embedding-001",
					Project:   "my-project",
					Location:  "us-central1",
					Dimension: 512,
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			_, _, got, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse: %v", cmp.Diff(tc.want, got))
			}
		})
	}
}

func TestFailParseFromYamlGemini(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "missing required model field",
			in: `
            kind: embeddingModel
            name: bad-model
            type: gemini
            `,
			err: "error unmarshaling embeddingModel: unable to parse as \"bad-model\": Key: 'Config.Model' Error:Field validation for 'Model' failed on the 'required' tag",
		},
		{
			desc: "unknown field",
			in: `
            kind: embeddingModel
            name: bad-field
            type: gemini
            model: gemini-embedding-001
            invalid_param: true
            `,
			err: "error unmarshaling embeddingModel: unable to parse as \"bad-field\": [1:1] unknown field \"invalid_param\"\n>  1 | invalid_param: true\n       ^\n   2 | model: gemini-embedding-001\n   3 | name: bad-field\n   4 | type: gemini",
		},
		{
			desc: "missing both Vertex and Google AI credentials",
			in: `
        kind: embeddingModel
        name: missing-creds
        type: gemini
        model: text-embedding-004
        `,
			err: "unable to initialize embedding model \"missing-creds\": missing credentials for Gemini embedding: For Google AI: Provide 'apiKey' in YAML or set GOOGLE_API_KEY/GEMINI_API_KEY env vars. For Vertex AI: Provide 'project'/'location' in YAML or via GOOGLE_CLOUD_PROJECT/GOOGLE_CLOUD_LOCATION env vars. See documentation for details: https://mcp-toolbox.dev/documentation/configuration/embedding-models/gemini/",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			t.Setenv("GOOGLE_API_KEY", "")
			t.Setenv("GEMINI_API_KEY", "")
			t.Setenv("GOOGLE_CLOUD_PROJECT", "")
			t.Setenv("GOOGLE_CLOUD_LOCATION", "")

			_, embeddingConfigs, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				if err.Error() != tc.err {
					t.Fatalf("unexpected unmarshal error:\ngot:  %q\nwant: %q", err.Error(), tc.err)
				}
				return
			}

			for _, cfg := range embeddingConfigs {
				_, err = cfg.Initialize()
				if err == nil {
					t.Fatalf("expect initialization to fail for case: %s", tc.desc)
				}
				if !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("unexpected init error:\ngot:  %q\nwant: %q", err.Error(), tc.err)
				}
			}
		})
	}
}
