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

package clickhouse

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestConfigToolConfigType(t *testing.T) {
	config := Config{}
	if config.ToolConfigType() != sqlType {
		t.Errorf("Expected %s, got %s", sqlType, config.ToolConfigType())
	}
}

func TestParseFromYamlClickHouseSQL(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		want server.ToolConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: tool
			name: example_tool
			type: clickhouse-sql
			source: my-instance
			description: some description
			statement: SELECT 1
			`,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:      "clickhouse-sql",
					Source:    "my-instance",
					Statement: "SELECT 1",
				},
			},
		},
		{
			desc: "with parameters",
			in: `
			kind: tool
			name: param_tool
			type: clickhouse-sql
			source: test-source
			description: Test ClickHouse tool
			statement: SELECT * FROM test_table WHERE id = $1
			parameters:
			  - name: id
			    type: string
			    description: Test ID
			`,
			want: server.ToolConfigs{
				"param_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "param_tool",
						Description:  "Test ClickHouse tool",
						AuthRequired: []string{},
					},
					Type:      "clickhouse-sql",
					Source:    "test-source",
					Statement: "SELECT * FROM test_table WHERE id = $1",
					Parameters: parameters.Parameters{
						parameters.NewStringParameter("id", "Test ID"),
					},
				},
			},
		},
		{
			desc: "with embeddedBy parameters",
			in: `
			kind: tool
			name: vector_insert
			type: clickhouse-sql
			source: my-instance
			description: Stores content and its vector embedding.
			statement: INSERT INTO docs (content, embedding) VALUES (?, ?)
			parameters:
			  - name: content
			    type: string
			    description: The text content to store.
			  - name: text_to_embed
			    type: string
			    description: The text content used to generate the vector.
			    embeddedBy: gemini-model
			    valueFromParam: content
			`,
			want: server.ToolConfigs{
				"vector_insert": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "vector_insert",
						Description:  "Stores content and its vector embedding.",
						AuthRequired: []string{},
					},
					Type:      "clickhouse-sql",
					Source:    "my-instance",
					Statement: "INSERT INTO docs (content, embedding) VALUES (?, ?)",
					Parameters: parameters.Parameters{
						parameters.NewStringParameter("content", "The text content to store."),
						&parameters.StringParameter{
							CommonParameter: parameters.CommonParameter{
								Name:           "text_to_embed",
								Type:           "string",
								Desc:           "The text content used to generate the vector.",
								EmbeddedBy:     "gemini-model",
								ValueFromParam: "content",
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}

func TestSQLConfigInitializeValidSource(t *testing.T) {
	config := Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test-tool",
			Description: "Test tool",
		},
		Type:       sqlType,
		Source:     "test-clickhouse",
		Statement:  "SELECT 1",
		Parameters: parameters.Parameters{},
	}

	tool, err := config.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	clickhouseTool, ok := tool.(Tool)
	if !ok {
		t.Fatalf("Expected Tool type, got %T", tool)
	}

	if clickhouseTool.GetName() != "test-tool" {
		t.Errorf("Expected name 'test-tool', got %s", clickhouseTool.GetName())
	}
}

func TestToolManifest(t *testing.T) {
	tool := Tool{
		BaseTool: tools.NewBaseTool(
			Config{},
			nil,
			tools.Manifest{
				Description: "Test description",
				Parameters:  []parameters.ParameterManifest{},
			},
			nil,
		),
	}

	manifest, err := tool.Manifest(nil)
	if err != nil {
		t.Fatalf("Manifest() returned unexpected error: %v", err)
	}
	if manifest.Description != "Test description" {
		t.Errorf("Expected description 'Test description', got %s", manifest.Description)
	}
}

func TestToolAuthorized(t *testing.T) {
	tests := []struct {
		name                 string
		authRequired         []string
		verifiedAuthServices []string
		expectedAuthorized   bool
	}{
		{
			name:                 "no auth required",
			authRequired:         []string{},
			verifiedAuthServices: []string{},
			expectedAuthorized:   true,
		},
		{
			name:                 "auth required and verified",
			authRequired:         []string{"google"},
			verifiedAuthServices: []string{"google"},
			expectedAuthorized:   true,
		},
		{
			name:                 "auth required but not verified",
			authRequired:         []string{"google"},
			verifiedAuthServices: []string{},
			expectedAuthorized:   false,
		},
		{
			name:                 "auth required but different service verified",
			authRequired:         []string{"google"},
			verifiedAuthServices: []string{"aws"},
			expectedAuthorized:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := Tool{
				BaseTool: tools.NewBaseTool(
					Config{
						ConfigBase: tools.ConfigBase{
							AuthRequired: tt.authRequired,
						},
					},
					nil,
					tools.Manifest{},
					nil,
				),
			}

			authorized := tool.Authorized(tt.verifiedAuthServices)
			if authorized != tt.expectedAuthorized {
				t.Errorf("Expected authorized %t, got %t", tt.expectedAuthorized, authorized)
			}
		})
	}
}
