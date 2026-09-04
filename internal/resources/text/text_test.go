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

package text_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/resources/text"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func floatPtr(f float64) *float64 { return &f }

func TestTextResourceInitialization(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		config      text.Config
		wantError   bool
		errContains string
		wantMime    string
		wantPrior   *float64
	}{
		{
			name: "success with defaults",
			config: text.Config{
				ResourceConfigBase: resources.ResourceConfigBase{ConfigBase: resources.ConfigBase{Name: "test1"}, URI: "text://test1"},
				Text:               "Hello, world!",
			},
			wantError: false,
			wantMime:  "text/plain",
			wantPrior: nil,
		},
		{
			name: "success with overrides",
			config: text.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name:        "test2",
						MimeType:    "application/json",
						Annotations: &resources.ResourceAnnotations{Priority: floatPtr(0.5)},
					},
					URI: "text://test2",
				},
				Text: `{"hello":"world"}`,
			},
			wantError: false,
			wantMime:  "application/json",
			wantPrior: floatPtr(0.5),
		},

		{
			name: "explicit 0.0 priority",
			config: text.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name:        "test-priority",
						Annotations: &resources.ResourceAnnotations{Priority: floatPtr(0.0)},
					},
					URI: "text://test-priority",
				},
				Text: "priority test",
			},
			wantError: false,
			wantMime:  "text/plain",
			wantPrior: floatPtr(0.0),
		},
		{
			name: "multi-byte unicode size calculation",
			config: text.Config{
				ResourceConfigBase: resources.ResourceConfigBase{ConfigBase: resources.ConfigBase{Name: "test-unicode"}, URI: "text://test-unicode"},
				Text:               "Hello 🌍",
			},
			wantError: false,
			wantMime:  "text/plain",
			wantPrior: nil,
		},
		{
			name: "pure whitespace payload",
			config: text.Config{
				ResourceConfigBase: resources.ResourceConfigBase{ConfigBase: resources.ConfigBase{Name: "test-whitespace"}, URI: "text://test-whitespace"},
				Text:               "   \n  ",
			},
			wantError: false,
			wantMime:  "text/plain",
			wantPrior: nil,
		},
		{
			name: "explicit empty mimetype defaults to text/plain",
			config: text.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name:     "test-empty-mime",
						MimeType: "",
					},
					URI: "text://test-empty-mime",
				},
				Text: "hello",
			},
			wantError: false,
			wantMime:  "text/plain",
			wantPrior: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.config.Initialize(ctx)
			if tc.wantError {
				if err == nil {
					t.Fatalf("Initialize() expected error, got nil")
				}
				if tc.errContains != "" {
					if !strings.Contains(err.Error(), tc.errContains) {
						t.Errorf("Initialize() err = %v, want to contain %q", err, tc.errContains)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Initialize() unexpected error: %v", err)
			}

			// Verify execution (Read)
			data, err := res.Read(ctx, nil)
			if err != nil {
				t.Fatalf("Read() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tc.config.Text, data); diff != "" {
				t.Errorf("Read() mismatch (-want +got):\n%s", diff)
			}

			textRes := res.(*text.Resource)
			expectedSize := int64(len(tc.config.Text))
			if textRes.Size != expectedSize {
				t.Errorf("Size = %d, want %d", textRes.Size, expectedSize)
			}
		})
	}
}

func TestParseFromYamlText(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.ResourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: resource
			name: my-text-resource
			type: text
			text: "Hello, world!"
			`,
			want: server.ResourceConfigs{
				"my-text-resource": &text.Config{
					ResourceConfigBase: resources.ResourceConfigBase{
						ConfigBase: resources.ConfigBase{
							Name:        "my-text-resource",
							Type:        "text",
							MimeType:    "text/plain",
							Annotations: &resources.ResourceAnnotations{Priority: floatPtr(1.0)},
						},
						URI: "text://my-text-resource",
					},
					Text: "Hello, world!",
				},
			},
		},
		{
			desc: "with annotations and custom mimeType",
			in: `
			kind: resource
			name: my-json-resource
			type: text
			mimeType: application/json
			uri: custom://my-json-resource
			annotations:
				priority: 0.8
				audience:
					- user
					- assistant
				lastModified: 2024-01-01T00:00:00Z
			text: '{"key": "value"}'
			`,
			want: server.ResourceConfigs{
				"my-json-resource": &text.Config{
					ResourceConfigBase: resources.ResourceConfigBase{
						ConfigBase: resources.ConfigBase{
							Name:     "my-json-resource",
							Type:     "text",
							MimeType: "application/json",
							Annotations: &resources.ResourceAnnotations{
								Priority:     floatPtr(0.8),
								Audience:     []resources.AudienceRole{resources.RoleUser, resources.RoleAssistant},
								LastModified: "2024-01-01T00:00:00Z",
							},
						},
						URI: "custom://my-json-resource",
					},
					Text: `{"key": "value"}`,
				},
			},
		},
		{
			desc: "multiline text",
			in: `
			kind: resource
			name: my-multiline-resource
			type: text
			text: |
				Line 1
				Line 2
			`,
			want: server.ResourceConfigs{
				"my-multiline-resource": &text.Config{
					ResourceConfigBase: resources.ResourceConfigBase{
						ConfigBase: resources.ConfigBase{
							Name:        "my-multiline-resource",
							Type:        "text",
							MimeType:    "text/plain",
							Annotations: &resources.ResourceAnnotations{Priority: floatPtr(1.0)},
						},
						URI: "text://my-multiline-resource",
					},
					Text: "Line 1\nLine 2\n",
				},
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, got, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse (-want +got):\n%s", diff)
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
			desc: "unknown field",
			in: `
			kind: resource
			name: test-invalid
			type: text
			text: "hello"
			textContent: "hello"
			`,
			err: "unknown field \"textContent\"",
		},
		{
			desc: "missing required text field",
			in: `
			kind: resource
			name: test-missing-text
			type: text
			`,
			err: "Field validation for 'Text' failed on the 'required' tag",
		},
		{
			desc: "empty text field",
			in: `
			kind: resource
			name: test-empty-text
			type: text
			text: ""
			`,
			err: "Field validation for 'Text' failed on the 'required' tag",
		},
		{
			desc: "unknown annotation field",
			in: `
			kind: resource
			name: test-invalid
			type: text
			text: "hello"
			annotations:
				unknownField: "should error"
			`,
			err: "unknown field \"unknownField\"",
		},
		{
			desc: "invalid priority type",
			in: `
			kind: resource
			name: test-invalid
			type: text
			text: "hello"
			annotations:
				priority: "high"
			`,
			err: "cannot unmarshal",
		},
		{
			desc: "invalid audience scalar",
			in: `
			kind: resource
			name: test-invalid
			type: text
			text: "hello"
			annotations:
				audience: user
			`,
			err: "string was used where sequence is expected",
		},
		{
			desc: "invalid audience value",
			in: `
			kind: resource
			name: test-invalid
			type: text
			text: "hello"
			annotations:
				audience:
					- admin
			`,
			err: "invalid audience \"admin\"",
		},
		{
			desc: "duplicate audience value",
			in: `
			kind: resource
			name: test-duplicate
			type: text
			text: "hello"
			annotations:
				audience:
					- user
					- user
			`,
			err: "duplicate audience \"user\"",
		},
		{
			desc: "invalid mimeType",
			in: `
			kind: resource
			name: test-mime
			type: text
			text: "hello"
			mimeType: invalid_mime
			`,
			err: "invalid mimeType \"invalid_mime\"",
		},
		{
			desc: "invalid lastModified",
			in: `
			kind: resource
			name: test-lastmod
			type: text
			text: "hello"
			annotations:
				lastModified: "2025-01-12"
			`,
			err: "not a valid ISO 8601 string",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			if !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("unexpected error: got %q, want %q", err.Error(), tc.err)
			}
		})
	}
}
