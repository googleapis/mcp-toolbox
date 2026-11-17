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

package postgresliststoredprocedure_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/postgres/postgresliststoredprocedure"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestParseFromYamlPostgresListStoredProcedure(t *testing.T) {
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
			type: postgres-list-stored-procedure
			source: my-postgres-instance
			description: some description
			authRequired:
				- my-google-auth-service
				- other-auth-service
			`,
			want: server.ToolConfigs{
				"example_tool": postgresliststoredprocedure.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:   "postgres-list-stored-procedure",
					Source: "my-postgres-instance",
				},
			},
		},
		{
			desc: "basic example",
			in: `
			kind: tool
			name: example_tool
			type: postgres-list-stored-procedure
			source: my-postgres-instance
			description: some description
			`,
			want: server.ToolConfigs{
				"example_tool": postgresliststoredprocedure.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "postgres-list-stored-procedure",
					Source: "my-postgres-instance",
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

type mockStoredProcedureSource struct {
	runSQLFunc func(ctx context.Context, statement string, params []any) (any, error)
}

func (m *mockStoredProcedureSource) IsReadOnly() bool               { return false }
func (m *mockStoredProcedureSource) SourceType() string             { return "postgres" }
func (m *mockStoredProcedureSource) ToConfig() sources.SourceConfig { return nil }
func (m *mockStoredProcedureSource) PostgresPool() *pgxpool.Pool    { return nil }
func (m *mockStoredProcedureSource) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	return m.runSQLFunc(ctx, statement, params)
}

func TestInvokePostgresListStoredProcedure(t *testing.T) {
	ctx := context.Background()
	cfg := postgresliststoredprocedure.Config{
		ConfigBase: tools.ConfigBase{
			Name: "list_stored_procedure",
		},
		Type:   "postgres-list-stored-procedure",
		Source: "my-pg",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize() unexpected error: %v", err)
	}

	t.Run("delegates to source.RunSQL and returns rows", func(t *testing.T) {
		wantRows := []any{map[string]any{"schema_name": "public", "name": "test_proc"}}
		var gotParams []any
		src := &mockStoredProcedureSource{
			runSQLFunc: func(_ context.Context, _ string, params []any) (any, error) {
				gotParams = params
				return wantRows, nil
			},
		}
		params := parameters.ParamValues{
			{Name: "role_name", Value: "postgres"},
			{Name: "schema_name", Value: "public"},
			{Name: "limit", Value: 10},
		}
		got, tbErr := tool.Invoke(ctx, src, params, "")
		if tbErr != nil {
			t.Fatalf("Invoke() unexpected error: %v", tbErr)
		}
		if diff := cmp.Diff(wantRows, got); diff != "" {
			t.Errorf("Invoke() rows diff (-want +got):\n%s", diff)
		}
		wantParams := []any{"postgres", "public", 10}
		if diff := cmp.Diff(wantParams, gotParams); diff != "" {
			t.Errorf("Invoke() params diff (-want +got):\n%s", diff)
		}
	})

	t.Run("propagates query/iteration error from RunSQL (including results.Err())", func(t *testing.T) {
		src := &mockStoredProcedureSource{
			runSQLFunc: func(_ context.Context, _ string, _ []any) (any, error) {
				return nil, errors.New("unable to execute query: mid-stream row error")
			},
		}
		params := parameters.ParamValues{
			{Name: "role_name", Value: nil},
			{Name: "schema_name", Value: nil},
			{Name: "limit", Value: 20},
		}
		_, tbErr := tool.Invoke(ctx, src, params, "")
		if tbErr == nil {
			t.Fatalf("Invoke() expected error, got nil")
		}
	})
}
