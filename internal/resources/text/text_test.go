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
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/resources/text"
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

func TestTextResourceYAMLUnmarshaling(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		yamlData     string
		wantText     string
		wantPriority *float64
		wantSize     *int64
	}{
		{
			name: "Valid YAML",
			yamlData: `
name: test-yaml
type: text
uri: info://test
annotations:
  priority: 0.9
  audience:
    - user
text: |
  Line 1
  Line 2
`,
			wantText:     "Line 1\nLine 2\n",
			wantPriority: floatPtr(0.9),
			wantSize:     func(i int64) *int64 { return &i }(14),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dec := yaml.NewDecoder(bytes.NewReader([]byte(tc.yamlData)), yaml.Strict(), yaml.Validator(validator.New()))
			resCfg, err := resources.DecodeConfig(ctx, "text", "test-yaml", dec)
			if err != nil {
				t.Fatalf("unexpected error decoding text resource: %v", err)
			}

			cfg := resCfg.(*text.Config)
			if cfg.Text != tc.wantText {
				t.Errorf("unexpected text payload: %q", cfg.Text)
			}

			if tc.wantPriority != nil {
				if cfg.Annotations == nil || cfg.Annotations.Priority == nil || *cfg.Annotations.Priority != *tc.wantPriority {
					t.Errorf("unexpected priority: %v", cfg.Annotations)
				}
			}

			// We need to initialize it to get the size calculated
			res, err := cfg.Initialize(ctx)
			if err != nil {
				t.Fatalf("unexpected error initializing text resource: %v", err)
			}

			if tc.wantSize != nil {
				if res.(*text.Resource).Size != *tc.wantSize {
					t.Errorf("unexpected size: got %v, want %v", res.(*text.Resource).Size, tc.wantSize)
				}
			}
		})
	}
}

func TestTextResourceYAMLUnmarshaling_Fail(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		yamlData    string
		errContains string
	}{
		{
			name: "Strict Decoder Validation",
			yamlData: `
name: test-invalid
type: text
textContent: "hello" # invalid field
`,
			errContains: "unknown field",
		},
		{
			name: "Missing required text field",
			yamlData: `
name: test-missing-text
type: text
`,
			errContains: "Field validation for 'Text' failed",
		},
		{
			name: "Empty text field",
			yamlData: `
name: test-empty-text
type: text
text: ""
`,
			errContains: "Field validation for 'Text' failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dec := yaml.NewDecoder(bytes.NewReader([]byte(tc.yamlData)), yaml.Strict(), yaml.Validator(validator.New()))
			resCfg, err := resources.DecodeConfig(ctx, "text", "test-invalid", dec)
			if err == nil {
				err = resCfg.Validate()
			}

			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("expected error to contain %q, got: %v", tc.errContains, err)
			}
		})
	}
}
