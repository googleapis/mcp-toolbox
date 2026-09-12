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

package trinosql_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/trino/trinosql"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlTrino(t *testing.T) {
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
			type: trino-sql
			source: my-trino-instance
			description: some description
			statement: |
				SELECT * FROM catalog.schema.table WHERE id = ?;
			authRequired:
				- my-google-auth-service
				- other-auth-service
			parameters:
				- name: id
				  type: string
				  description: ID to filter by
				  authServices:
					- name: my-google-auth-service
					  field: user_id
					- name: other-auth-service
					  field: user_id
			`,
			want: server.ToolConfigs{
				"example_tool": trinosql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:      "trino-sql",
					Source:    "my-trino-instance",
					Statement: "SELECT * FROM catalog.schema.table WHERE id = ?;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("id", "ID to filter by", parameters.WithStringAuth(
							[]parameters.ParamAuthService{{Name: "my-google-auth-service", Field: "user_id"},
								{Name: "other-auth-service", Field: "user_id"}})),
					},
				},
			},
		},
		{
			desc: "with user impersonation",
			in: `
			kind: tool
			name: example_tool
			type: trino-sql
			source: my-trino-instance
			description: some description
			statement: |
				SELECT 1;
			impersonateUser: true
			`,
			want: server.ToolConfigs{
				"example_tool": trinosql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:            "trino-sql",
					Source:          "my-trino-instance",
					Statement:       "SELECT 1;\n",
					ImpersonateUser: true,
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

func TestParseFromYamlWithTemplateParamsTrino(t *testing.T) {
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
			type: trino-sql
			source: my-trino-instance
			description: some description
			statement: |
				SELECT * FROM {{ .catalog }}.{{ .schema }}.{{ .tableName }} WHERE country = ?;
			authRequired:
				- my-google-auth-service
				- other-auth-service
			parameters:
				- name: country
				  type: string
				  description: some description
				  authServices:
					- name: my-google-auth-service
					  field: user_id
					- name: other-auth-service
					  field: user_id
			templateParameters:
				- name: catalog
				  type: string
				  description: The catalog to query from.
				- name: schema
				  type: string
				  description: The schema to query from.
				- name: tableName
				  type: string
				  description: The table to select data from.
				- name: fieldArray
				  type: array
				  description: The columns to return for the query.
				  items: 
						name: column
						type: string
						description: A column name that will be returned from the query.
			`,
			want: server.ToolConfigs{
				"example_tool": trinosql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:      "trino-sql",
					Source:    "my-trino-instance",
					Statement: "SELECT * FROM {{ .catalog }}.{{ .schema }}.{{ .tableName }} WHERE country = ?;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("country", "some description", parameters.WithStringAuth(
							[]parameters.ParamAuthService{{Name: "my-google-auth-service", Field: "user_id"},
								{Name: "other-auth-service", Field: "user_id"}})),
					},
					TemplateParameters: []parameters.Parameter{
						parameters.NewStringParameter("catalog", "The catalog to query from."),
						parameters.NewStringParameter("schema", "The schema to query from."),
						parameters.NewStringParameter("tableName", "The table to select data from."),
						parameters.NewArrayParameter("fieldArray", "The columns to return for the query.", parameters.NewStringParameter("column", "A column name that will be returned from the query.")),
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

// mockSource implements the tool's compatibleSource interface and records how
// the tool routed the call (plain RunSQL vs. impersonating RunSQLAsUser).
type mockSource struct {
	ranPlain  bool
	ranAsUser bool
	gotUser   string
	gotStmt   string
	gotParams []any
}

func (m *mockSource) SourceType() string             { return "trino" }
func (m *mockSource) ToConfig() sources.SourceConfig { return nil }
func (m *mockSource) TrinoDB() *sql.DB               { return nil }

func (m *mockSource) RunSQL(_ context.Context, stmt string, params []any) (any, error) {
	m.ranPlain = true
	m.gotStmt = stmt
	m.gotParams = params
	return []any{}, nil
}

func (m *mockSource) RunSQLAsUser(_ context.Context, user, stmt string, params []any) (any, error) {
	m.ranAsUser = true
	m.gotUser = user
	m.gotStmt = stmt
	m.gotParams = params
	return []any{}, nil
}

type mockSourceProvider struct {
	tools.SourceProvider
	source *mockSource
}

func (m *mockSourceProvider) GetSource(string) (sources.Source, bool) { return m.source, true }

func TestInvokeImpersonation(t *testing.T) {
	tcs := []struct {
		desc            string
		impersonateUser bool
		params          parameters.ParamValues
		wantAsUser      bool
		wantUser        string
	}{
		{
			desc:            "impersonation disabled uses plain RunSQL",
			impersonateUser: false,
			params:          parameters.ParamValues{},
			wantAsUser:      false,
		},
		{
			desc:            "impersonation forwards trino_user",
			impersonateUser: true,
			params:          parameters.ParamValues{{Name: "trino_user", Value: "alice@seedtag.com"}},
			wantAsUser:      true,
			wantUser:        "alice@seedtag.com",
		},
		{
			desc:            "empty trino_user falls back to source user",
			impersonateUser: true,
			params:          parameters.ParamValues{{Name: "trino_user", Value: ""}},
			wantAsUser:      true,
			wantUser:        "",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := trinosql.Config{
				ConfigBase:      tools.ConfigBase{Name: "tool", Description: "d"},
				Type:            "trino-sql",
				Source:          "s",
				Statement:       "SELECT 1",
				ImpersonateUser: tc.impersonateUser,
			}
			tool, err := cfg.Initialize(context.Background())
			if err != nil {
				t.Fatalf("initialize: %v", err)
			}
			src := &mockSource{}
			if _, toolErr := tool.Invoke(context.Background(), &mockSourceProvider{source: src}, tc.params, ""); toolErr != nil {
				t.Fatalf("invoke: %v", toolErr)
			}
			if src.ranAsUser != tc.wantAsUser {
				t.Errorf("ranAsUser = %v, want %v", src.ranAsUser, tc.wantAsUser)
			}
			if src.ranPlain == tc.wantAsUser {
				t.Errorf("ranPlain = %v, want %v", src.ranPlain, !tc.wantAsUser)
			}
			if tc.wantAsUser && src.gotUser != tc.wantUser {
				t.Errorf("forwarded user = %q, want %q", src.gotUser, tc.wantUser)
			}
			if src.gotStmt != "SELECT 1" {
				t.Errorf("forwarded statement = %q, want %q", src.gotStmt, "SELECT 1")
			}
			// trino_user must never be bound as a SQL parameter.
			if len(src.gotParams) != 0 {
				t.Errorf("expected no bind params, got %v", src.gotParams)
			}
		})
	}
}
