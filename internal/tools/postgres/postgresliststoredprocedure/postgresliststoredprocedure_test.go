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
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/postgres/postgresliststoredprocedure"
	"github.com/googleapis/mcp-toolbox/internal/util"
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

type mockPostgresSource struct {
	sources.Source
	runSQLFunc func(ctx context.Context, statement string, params []any) (any, error)
}

func (m *mockPostgresSource) PostgresPool() *pgxpool.Pool {
	return nil
}

func (m *mockPostgresSource) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	if m.runSQLFunc != nil {
		return m.runSQLFunc(ctx, statement, params)
	}
	return nil, nil
}

type mockIncompatibleSource struct {
	sources.Source
}

func TestInvokePostgresListStoredProcedure(t *testing.T) {
	ctx := context.Background()
	cfg := postgresliststoredprocedure.Config{
		ConfigBase: tools.ConfigBase{
			Name: "test_tool",
		},
		Type:   "postgres-list-stored-procedure",
		Source: "my-pg",
	}

	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unexpected Initialize error: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		wantRows := []any{
			map[string]any{
				"schema_name": "public",
				"name":        "my_proc",
				"owner":       "postgres",
				"language":    "plpgsql",
				"definition":  "CREATE PROCEDURE my_proc() ...",
				"description": "sample procedure",
			},
		}

		mockSrc := &mockPostgresSource{
			runSQLFunc: func(ctx context.Context, stmt string, params []any) (any, error) {
				return wantRows, nil
			},
		}

		params := parameters.ParamValues{
			parameters.ParamValue{Name: "role_name", Value: "postgres"},
			parameters.ParamValue{Name: "schema_name", Value: "public"},
			parameters.ParamValue{Name: "limit", Value: 10},
		}

		got, toolErr := tool.Invoke(ctx, mockSrc, params, tools.AccessToken(""))
		if toolErr != nil {
			t.Fatalf("unexpected Invoke error: %v", toolErr)
		}

		if diff := cmp.Diff(wantRows, got); diff != "" {
			t.Fatalf("unexpected result: diff %v", diff)
		}
	})

	t.Run("runSQL error (mid-stream iteration error)", func(t *testing.T) {
		mockSrc := &mockPostgresSource{
			runSQLFunc: func(ctx context.Context, stmt string, params []any) (any, error) {
				return nil, errors.New("unable to execute query: connection terminated")
			},
		}

		params := parameters.ParamValues{}
		got, toolErr := tool.Invoke(ctx, mockSrc, params, tools.AccessToken(""))
		if toolErr == nil {
			t.Fatalf("expected error from Invoke, got result: %v", got)
		}
	})

	t.Run("incompatible source", func(t *testing.T) {
		incompatibleSrc := &mockIncompatibleSource{}
		params := parameters.ParamValues{}

		got, toolErr := tool.Invoke(ctx, incompatibleSrc, params, tools.AccessToken(""))
		if toolErr == nil {
			t.Fatalf("expected error for incompatible source, got result: %v", got)
		}
		var csErr *util.ClientServerError
		if !errors.As(toolErr, &csErr) {
			t.Fatalf("expected ClientServerError, got %T: %v", toolErr, toolErr)
		}
		if csErr.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", csErr.Code)
		}
	})
}

func TestValidateSourcePostgresListStoredProcedure(t *testing.T) {
	ctx := context.Background()
	cfg := postgresliststoredprocedure.Config{
		ConfigBase: tools.ConfigBase{
			Name: "test_tool",
		},
		Type:   "postgres-list-stored-procedure",
		Source: "my-pg",
	}

	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unexpected Initialize error: %v", err)
	}

	t.Run("compatible source", func(t *testing.T) {
		mockSrc := &mockPostgresSource{}
		if err := tool.ValidateSource(mockSrc); err != nil {
			t.Errorf("unexpected error validating compatible source: %v", err)
		}
	})

	t.Run("incompatible source", func(t *testing.T) {
		incompatibleSrc := &mockIncompatibleSource{}
		if err := tool.ValidateSource(incompatibleSrc); err == nil {
			t.Error("expected error validating incompatible source, got nil")
		}
	})
}
