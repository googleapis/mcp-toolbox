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

package spannerupdatedatabaseschema_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/spanner/spannerupdatedatabaseschema"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlUpdateDatabaseSchema(t *testing.T) {
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
            name: update_database_schema
            type: spanner-update-database-schema
            source: my-instance
            description: some description
            `,
			want: server.ToolConfigs{
				"update_database_schema": spannerupdatedatabaseschema.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "update_database_schema",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "spanner-update-database-schema",
					Source: "my-instance",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
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

type mockSpannerSource struct {
	executedStatements []string
	err                error
	readOnly           bool
	useAuth            bool
}

func (m *mockSpannerSource) UpdateDatabaseDdl(ctx context.Context, statements []string, tokenString string) error {
	m.executedStatements = statements
	return m.err
}
func (m *mockSpannerSource) UseClientAuthorization() bool   { return m.useAuth }
func (m *mockSpannerSource) IsReadOnly() bool               { return m.readOnly }
func (m *mockSpannerSource) SourceType() string             { return "spanner" }
func (m *mockSpannerSource) ToConfig() sources.SourceConfig { return nil }

func TestConfig_Initialize(t *testing.T) {
	cfg := spannerupdatedatabaseschema.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "update_database_schema",
			Description: "Test DDL update",
		},
		Type:   "spanner-update-database-schema",
		Source: "test-source",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if tool.GetName() != "update_database_schema" {
		t.Errorf("GetName() = %v, want %v", tool.GetName(), "update_database_schema")
	}

	// Destructive annotations default (ReadOnlyHint: false, DestructiveHint: true)
	ann := tool.GetAnnotations(nil)
	if ann == nil || ann.ReadOnlyHint == nil || *ann.ReadOnlyHint {
		t.Errorf("expected ReadOnlyHint=false, got %+v", ann)
	}
	if ann.DestructiveHint == nil || !*ann.DestructiveHint {
		t.Errorf("expected DestructiveHint=true, got %+v", ann)
	}
}

func TestTool_ShouldSuppress(t *testing.T) {
	cfg := spannerupdatedatabaseschema.Config{
		ConfigBase: tools.ConfigBase{Name: "update_database_schema"},
		Type:       "spanner-update-database-schema",
		Source:     "test-source",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	roSource := &mockSpannerSource{readOnly: true}
	rwSource := &mockSpannerSource{readOnly: false}

	if !tools.ShouldSuppress(context.Background(), tool, roSource) {
		t.Errorf("expected update_database_schema to be suppressed on read-only source")
	}
	if tools.ShouldSuppress(context.Background(), tool, rwSource) {
		t.Errorf("expected update_database_schema to NOT be suppressed on read-write source")
	}
}

func TestTool_ValidateSource(t *testing.T) {
	cfg := spannerupdatedatabaseschema.Config{
		ConfigBase: tools.ConfigBase{Name: "update_database_schema"},
		Type:       "spanner-update-database-schema",
		Source:     "test-source",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	roSource := &mockSpannerSource{readOnly: true}
	rwSource := &mockSpannerSource{readOnly: false}

	if err := tool.ValidateSource(roSource); err != nil {
		t.Errorf("expected ValidateSource to succeed on read-only source (suppression is handled by ShouldSuppress), got %v", err)
	}
	if err := tool.ValidateSource(rwSource); err != nil {
		t.Errorf("expected ValidateSource to succeed on read-write source, got %v", err)
	}
}

func TestTool_Invoke(t *testing.T) {
	ctx := context.Background()
	cfg := spannerupdatedatabaseschema.Config{
		ConfigBase: tools.ConfigBase{Name: "update_database_schema"},
		Type:       "spanner-update-database-schema",
		Source:     "test-source",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	tcs := []struct {
		desc           string
		source         *mockSpannerSource
		params         parameters.ParamValues
		token          tools.AccessToken
		wantExecuted   int
		wantErr        bool
		wantStatusCode int
	}{
		{
			desc:   "successful invocation with statement array",
			source: &mockSpannerSource{readOnly: false},
			params: parameters.ParamValues{
				{
					Name:  "statements",
					Value: []any{"CREATE TABLE test_table (id INT64) PRIMARY KEY (id)"},
				},
			},
			wantExecuted: 1,
			wantErr:      false,
		},
		{
			desc:    "error on missing statements",
			source:  &mockSpannerSource{readOnly: false},
			params:  parameters.ParamValues{},
			wantErr: true,
		},
		{
			desc:   "error on non-string element in statements array",
			source: &mockSpannerSource{readOnly: false},
			params: parameters.ParamValues{
				{
					Name:  "statements",
					Value: []any{"CREATE TABLE valid (id INT64) PRIMARY KEY (id)", 12345},
				},
			},
			wantErr: true,
		},
		{
			desc:   "error on empty string in statements array",
			source: &mockSpannerSource{readOnly: false},
			params: parameters.ParamValues{
				{
					Name:  "statements",
					Value: []any{"CREATE TABLE valid (id INT64) PRIMARY KEY (id)", ""},
				},
			},
			wantErr: true,
		},
		{
			desc: "error on invalid client authorization token",
			source: &mockSpannerSource{
				readOnly: false,
				useAuth:  true,
			},
			params: parameters.ParamValues{
				{
					Name:  "statements",
					Value: []any{"CREATE TABLE test_table (id INT64) PRIMARY KEY (id)"},
				},
			},
			token:          tools.AccessToken("invalid-token"),
			wantErr:        true,
			wantStatusCode: http.StatusUnauthorized,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			resp, toolErr := tool.Invoke(ctx, tc.source, tc.params, tc.token)
			if tc.wantErr {
				if toolErr == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.wantStatusCode != 0 {
					var clientServerErr *util.ClientServerError
					if !errors.As(toolErr, &clientServerErr) {
						t.Fatalf("expected *util.ClientServerError, got %T", toolErr)
					}
					if clientServerErr.Code != tc.wantStatusCode {
						t.Errorf("status code = %d, want %d", clientServerErr.Code, tc.wantStatusCode)
					}
				}
				return
			}

			if toolErr != nil {
				t.Fatalf("unexpected error: %v", toolErr)
			}

			respMap, ok := resp.(map[string]any)
			if !ok {
				t.Fatalf("expected map[string]any, got %T", resp)
			}
			if respMap["statements_executed"] != tc.wantExecuted {
				t.Errorf("statements_executed = %v, want %v", respMap["statements_executed"], tc.wantExecuted)
			}
		})
	}
}
