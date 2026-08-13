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

package spannersql_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/spanner/spannersql"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlSpanner(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	boolPtr := func(b bool) *bool { return &b }
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
            type: spanner-sql
            source: my-pg-instance
            description: some description
            statement: |
                SELECT * FROM SQL_STATEMENT;
            parameters:
                - name: country
                  type: string
                  description: some description
			`,
			want: server.ToolConfigs{
				"example_tool": spannersql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:      "spanner-sql",
					Source:    "my-pg-instance",
					Statement: "SELECT * FROM SQL_STATEMENT;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("country", "some description"),
					},
				},
			},
		},
		{
			desc: "read only set to true",
			in: `
            kind: tool
            name: example_tool
            type: spanner-sql
            source: my-pg-instance
            description: some description
            readOnly: true
            statement: |
                SELECT * FROM SQL_STATEMENT;
            parameters:
                - name: country
                  type: string
                  description: some description
			`,
			want: server.ToolConfigs{
				"example_tool": spannersql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:      "spanner-sql",
					Source:    "my-pg-instance",
					Statement: "SELECT * FROM SQL_STATEMENT;\n",
					ReadOnly:  boolPtr(true),
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("country", "some description"),
					},
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			_, _, _, got, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}

}

func TestParseFromYamlWithTemplateParamsSpanner(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	boolPtr := func(b bool) *bool { return &b }
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
            type: spanner-sql
            source: my-pg-instance
            description: some description
            statement: |
                SELECT * FROM SQL_STATEMENT;
            parameters:
                - name: country
                  type: string
                  description: some description
            templateParameters:
                - name: tableName
                  type: string
                  description: The table to select hotels from.
                - name: fieldArray
                  type: array
                  description: The columns to return for the query.
                  items: 
                    name: column
                    type: string
                    description: A column name that will be returned from the query.
			`,
			want: server.ToolConfigs{
				"example_tool": spannersql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:      "spanner-sql",
					Source:    "my-pg-instance",
					Statement: "SELECT * FROM SQL_STATEMENT;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("country", "some description"),
					},
					TemplateParameters: []parameters.Parameter{
						parameters.NewStringParameter("tableName", "The table to select hotels from."),
						parameters.NewArrayParameter("fieldArray", "The columns to return for the query.", parameters.NewStringParameter("column", "A column name that will be returned from the query.")),
					},
				},
			},
		},
		{
			desc: "read only set to true",
			in: `
            kind: tool
            name: example_tool
            type: spanner-sql
            source: my-pg-instance
            description: some description
            readOnly: true
            statement: |
                SELECT * FROM SQL_STATEMENT;
            parameters:
                - name: country
                  type: string
                  description: some description
			`,
			want: server.ToolConfigs{
				"example_tool": spannersql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:      "spanner-sql",
					Source:    "my-pg-instance",
					Statement: "SELECT * FROM SQL_STATEMENT;\n",
					ReadOnly:  boolPtr(true),
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("country", "some description"),
					},
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			_, _, _, got, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}

type fakeSource struct {
	readOnly           bool
	lastReadOnlyPassed bool
}

func (f *fakeSource) SpannerClient() *spanner.Client { return nil }
func (f *fakeSource) DatabaseDialect() string        { return "googlesql" }
func (f *fakeSource) RunSQL(ctx context.Context, readOnly bool, sql string, params map[string]any) (any, error) {
	f.lastReadOnlyPassed = readOnly
	return map[string]any{"status": "ok"}, nil
}
func (f *fakeSource) IsReadOnly() bool               { return f.readOnly }
func (f *fakeSource) SourceType() string             { return "spanner" }
func (f *fakeSource) ToConfig() sources.SourceConfig { return nil }

func TestSpannerSQL_ValidateSource(t *testing.T) {
	ctx := context.Background()
	toolCfg := spannersql.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test_spanner_tool",
			Description: "test tool",
		},
		Type:      "spanner-sql",
		Source:    "test-source",
		Statement: "SELECT 1;",
	}
	tool, err := toolCfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unexpected error initializing tool: %v", err)
	}
	if err := tool.ValidateSource(&fakeSource{readOnly: false}); err != nil {
		t.Errorf("ValidateSource(readWrite) unexpected error: %v", err)
	}
	if err := tool.ValidateSource(&fakeSource{readOnly: true}); err != nil {
		t.Errorf("ValidateSource(readOnly) unexpected error: %v", err)
	}
}

func TestSpannerSQL_Invoke_ReadOnlyRouting(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	tcs := []struct {
		desc               string
		readOnly           *bool
		annotations        *tools.ToolAnnotations
		wantReadOnlyPassed bool
	}{
		{
			desc:               "legacy readOnly: true routes to read-only snapshot",
			readOnly:           boolPtr(true),
			annotations:        nil,
			wantReadOnlyPassed: true,
		},
		{
			desc:               "legacy readOnly: false routes to read-write transaction",
			readOnly:           boolPtr(false),
			annotations:        nil,
			wantReadOnlyPassed: false,
		},
		{
			desc:               "modern annotations readOnlyHint: true routes to read-only snapshot",
			readOnly:           nil,
			annotations:        &tools.ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			wantReadOnlyPassed: true,
		},
		{
			desc:               "modern annotations readOnlyHint: false routes to read-write transaction",
			readOnly:           nil,
			annotations:        &tools.ToolAnnotations{ReadOnlyHint: boolPtr(false)},
			wantReadOnlyPassed: false,
		},
		{
			desc:               "no annotations and no legacy readOnly routes to read-write transaction",
			readOnly:           nil,
			annotations:        nil,
			wantReadOnlyPassed: false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			toolCfg := spannersql.Config{
				ConfigBase: tools.ConfigBase{
					Name:        "test_sql_tool",
					Description: "test tool",
				},
				Type:        "spanner-sql",
				Source:      "my-source",
				Statement:   "SELECT 1;",
				ReadOnly:    tc.readOnly,
				Annotations: tc.annotations,
			}

			tool, err := toolCfg.Initialize(context.Background())
			if err != nil {
				t.Fatalf("unexpected error initializing tool: %v", err)
			}

			src := &fakeSource{readOnly: false}
			params := parameters.ParamValues{}
			_, toolErr := tool.Invoke(context.Background(), src, params, tools.AccessToken(""))
			if toolErr != nil {
				t.Fatalf("unexpected tool invocation error: %v", toolErr)
			}

			if src.lastReadOnlyPassed != tc.wantReadOnlyPassed {
				t.Errorf("lastReadOnlyPassed = %v, want %v", src.lastReadOnlyPassed, tc.wantReadOnlyPassed)
			}
		})
	}
}

func TestSpannerSQL_ConflictValidation(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	boolPtr := func(b bool) *bool { return &b }

	tcs := []struct {
		desc        string
		readOnly    *bool
		annotations *tools.ToolAnnotations
		wantError   bool
	}{
		{
			desc:        "readOnly: true with readOnlyHint: true -> valid",
			readOnly:    boolPtr(true),
			annotations: &tools.ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			wantError:   false,
		},
		{
			desc:        "readOnly: false with readOnlyHint: false -> valid",
			readOnly:    boolPtr(false),
			annotations: &tools.ToolAnnotations{ReadOnlyHint: boolPtr(false)},
			wantError:   false,
		},
		{
			desc:        "readOnly: true with readOnlyHint: false -> conflict error",
			readOnly:    boolPtr(true),
			annotations: &tools.ToolAnnotations{ReadOnlyHint: boolPtr(false)},
			wantError:   true,
		},
		{
			desc:        "readOnly: false with readOnlyHint: true -> conflict error",
			readOnly:    boolPtr(false),
			annotations: &tools.ToolAnnotations{ReadOnlyHint: boolPtr(true)},
			wantError:   true,
		},
		{
			desc:        "readOnly: true with nil annotations -> valid",
			readOnly:    boolPtr(true),
			annotations: nil,
			wantError:   false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			toolCfg := spannersql.Config{
				ConfigBase: tools.ConfigBase{
					Name:        "test_spanner_tool",
					Description: "test tool",
				},
				Type:        "spanner-sql",
				Source:      "test-source",
				Statement:   "SELECT 1;",
				ReadOnly:    tc.readOnly,
				Annotations: tc.annotations,
			}

			_, err := toolCfg.Initialize(ctx)
			if (err != nil) != tc.wantError {
				t.Errorf("Initialize() error = %v, wantError = %v", err, tc.wantError)
			}
		})
	}
}

func TestSpannerSQL_DeprecationWarning(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	tcs := []struct {
		desc        string
		readOnly    *bool
		wantWarning bool
	}{
		{
			desc:        "readOnly: true emits deprecation warning",
			readOnly:    boolPtr(true),
			wantWarning: true,
		},
		{
			desc:        "readOnly: false emits deprecation warning",
			readOnly:    boolPtr(false),
			wantWarning: true,
		},
		{
			desc:        "readOnly omitted (nil) does not emit deprecation warning",
			readOnly:    nil,
			wantWarning: false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			var buf bytes.Buffer
			logger, err := log.NewStdLogger(&buf, &buf, "warn")
			if err != nil {
				t.Fatalf("unexpected error creating logger: %v", err)
			}
			ctx := util.WithLogger(context.Background(), logger)

			toolCfg := spannersql.Config{
				ConfigBase: tools.ConfigBase{
					Name:        "test_spanner_tool",
					Description: "test tool",
				},
				Type:      "spanner-sql",
				Source:    "my-source",
				Statement: "SELECT 1;",
				ReadOnly:  tc.readOnly,
			}

			_, err = toolCfg.Initialize(ctx)
			if err != nil {
				t.Fatalf("unexpected error initializing tool: %v", err)
			}

			logs := buf.String()
			hasWarning := strings.Contains(logs, "[DEPRECATED] The 'readOnly' field on tool") && strings.Contains(logs, "test_spanner_tool")
			if hasWarning != tc.wantWarning {
				t.Errorf("hasWarning = %v, wantWarning = %v; logs:\n%s", hasWarning, tc.wantWarning, logs)
			}
		})
	}
}
