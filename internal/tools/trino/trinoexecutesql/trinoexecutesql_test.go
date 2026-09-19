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

package trinoexecutesql_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/trino/trinoexecutesql"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlTrinoExecuteSQL(t *testing.T) {
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
			type: trino-execute-sql
			source: my-trino-instance
			description: some description
			authRequired:
				- my-google-auth-service
				- other-auth-service
			`,
			want: server.ToolConfigs{
				"example_tool": trinoexecutesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:   "trino-execute-sql",
					Source: "my-trino-instance",
				},
			},
		},
		{
			desc: "with user impersonation",
			in: `
			kind: tool
			name: example_tool
			type: trino-execute-sql
			source: my-trino-instance
			description: some description
			impersonateUser: true
			`,
			want: server.ToolConfigs{
				"example_tool": trinoexecutesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:            "trino-execute-sql",
					Source:          "my-trino-instance",
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
			params:          parameters.ParamValues{{Name: "sql", Value: "SELECT 1"}},
			wantAsUser:      false,
		},
		{
			desc:            "impersonation forwards trino_user",
			impersonateUser: true,
			params:          parameters.ParamValues{{Name: "sql", Value: "SELECT 1"}, {Name: "trino_user", Value: "alice@seedtag.com"}},
			wantAsUser:      true,
			wantUser:        "alice@seedtag.com",
		},
		{
			desc:            "empty trino_user falls back to source user",
			impersonateUser: true,
			params:          parameters.ParamValues{{Name: "sql", Value: "SELECT 1"}, {Name: "trino_user", Value: ""}},
			wantAsUser:      true,
			wantUser:        "",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := trinoexecutesql.Config{
				ConfigBase:      tools.ConfigBase{Name: "tool", Description: "d"},
				Type:            "trino-execute-sql",
				Source:          "s",
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
