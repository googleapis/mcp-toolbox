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

package orcarouter_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels"
	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels/orcarouter"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlOrcaRouter(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.EmbeddingModelConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: embeddingModel
			name: my-orcarouter-model
			type: orcarouter
			model: openai/text-embedding-3-small
            `,
			want: map[string]embeddingmodels.EmbeddingModelConfig{
				"my-orcarouter-model": orcarouter.Config{
					Name:  "my-orcarouter-model",
					Type:  orcarouter.EmbeddingModelType,
					Model: "openai/text-embedding-3-small",
				},
			},
		},
		{
			desc: "full example with api key, base url and dimension",
			in: `
            kind: embeddingModel
            name: complex-orcarouter
            type: orcarouter
            model: openai/text-embedding-3-large
            apiKey: "test-api-key"
            baseUrl: "https://api.orcarouter.ai/v1"
            dimension: 3072
            `,
			want: map[string]embeddingmodels.EmbeddingModelConfig{
				"complex-orcarouter": orcarouter.Config{
					Name:      "complex-orcarouter",
					Type:      orcarouter.EmbeddingModelType,
					Model:     "openai/text-embedding-3-large",
					ApiKey:    "test-api-key",
					BaseURL:   "https://api.orcarouter.ai/v1",
					Dimension: 3072,
				},
			},
		},
		{
			desc: "config with env var api key",
			in: `
            kind: embeddingModel
            name: env-key-orcarouter
            type: orcarouter
            model: google/gemini-embedding-001
            apiKey: ${ORCAROUTER_API_KEY}
            `,
			want: map[string]embeddingmodels.EmbeddingModelConfig{
				"env-key-orcarouter": orcarouter.Config{
					Name:   "env-key-orcarouter",
					Type:   orcarouter.EmbeddingModelType,
					Model:  "google/gemini-embedding-001",
					ApiKey: "${ORCAROUTER_API_KEY}",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			_, _, got, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse: %v", cmp.Diff(tc.want, got))
			}
		})
	}
}

func TestFailParseFromYamlOrcaRouter(t *testing.T) {
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
            type: orcarouter
            `,
			err: "error unmarshaling embeddingModel: unable to parse as \"bad-model\": Key: 'Config.Model' Error:Field validation for 'Model' failed on the 'required' tag",
		},
		{
			desc: "unknown field",
			in: `
            kind: embeddingModel
            name: bad-field
            type: orcarouter
            model: openai/text-embedding-3-small
            invalid_param: true
            `,
			err: "error unmarshaling embeddingModel: unable to parse as \"bad-field\": [1:1] unknown field \"invalid_param\"\n>  1 | invalid_param: true\n       ^\n   2 | model: openai/text-embedding-3-small\n   3 | name: bad-field\n   4 | type: orcarouter",
		},
		{
			desc: "missing credentials",
			in: `
        kind: embeddingModel
        name: missing-creds
        type: orcarouter
        model: openai/text-embedding-3-small
        `,
			err: "missing credentials for OrcaRouter embedding: Provide 'apiKey' in YAML or set ORCAROUTER_API_KEY env var. See documentation for details: https://mcp-toolbox.dev/documentation/configuration/embedding-models/orcarouter/",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			t.Setenv("ORCAROUTER_API_KEY", "")

			_, _, embeddingConfigs, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				if err.Error() != tc.err {
					t.Fatalf("unexpected unmarshal error:\ngot:  %q\nwant: %q", err.Error(), tc.err)
				}
				return
			}

			for _, cfg := range embeddingConfigs {
				_, err = cfg.Initialize(context.Background())
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
