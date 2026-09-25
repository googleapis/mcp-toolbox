// Copyright © 2025, Oracle and/or its affiliates.

package oracleexecutesql_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/oracle"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/oracle/oracleexecutesql"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlOracleExecuteSql(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	valTrue := true
	valFalse := false
	tcs := []struct {
		desc string
		in   string
		want server.ToolConfigs
	}{
		{
			desc: "basic example with auth",
			in: `
            kind: tool
            name: run_adhoc_query
            type: oracle-execute-sql
            source: my-oracle-instance
            description: Executes arbitrary SQL statements like INSERT or UPDATE.
            authRequired:
                - my-google-auth-service
            `,
			want: server.ToolConfigs{
				"run_adhoc_query": oracleexecutesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "run_adhoc_query",
						Description:  "Executes arbitrary SQL statements like INSERT or UPDATE.",
						AuthRequired: []string{"my-google-auth-service"},
					},
					Type:   "oracle-execute-sql",
					Source: "my-oracle-instance",
				},
			},
		},
		{
			desc: "example without authRequired",
			in: `
            kind: tool
            name: run_simple_update
            type: oracle-execute-sql
            source: db-dev
            description: Runs a simple update operation.
            `,
			want: server.ToolConfigs{
				"run_simple_update": oracleexecutesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "run_simple_update",
						Description:  "Runs a simple update operation.",
						AuthRequired: []string{},
					},
					Type:   "oracle-execute-sql",
					Source: "db-dev",
				},
			},
		},
		{
			desc: "example with explicit readOnly true",
			in: `
            kind: tool
            name: safe_query
            type: oracle-execute-sql
            source: db-prod
            description: Safe read operation.
            readOnly: true
            `,
			want: server.ToolConfigs{
				"safe_query": oracleexecutesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "safe_query",
						Description:  "Safe read operation.",
						AuthRequired: []string{},
					},
					Type:     "oracle-execute-sql",
					Source:   "db-prod",
					ReadOnly: &valTrue,
				},
			},
		},
		{
			desc: "example with explicit readOnly false (DML)",
			in: `
            kind: tool
            name: update_user
            type: oracle-execute-sql
            source: db-prod
            description: Updates user table.
            readOnly: false
            `,
			want: server.ToolConfigs{
				"update_user": oracleexecutesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "update_user",
						Description:  "Updates user table.",
						AuthRequired: []string{},
					},
					Type:     "oracle-execute-sql",
					Source:   "db-prod",
					ReadOnly: &valFalse,
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
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

type mockSource struct {
	lastCtx context.Context
	lastSQL string
}

func (m *mockSource) OracleDB() *sql.DB {
	return nil
}

func (m *mockSource) RunSQL(ctx context.Context, s string, _ []any, _ bool) (any, error) {
	m.lastCtx = ctx
	m.lastSQL = s
	return "ok", nil
}

func (m *mockSource) SourceType() string {
	return "oracle"
}

func (m *mockSource) IsReadOnly() bool {
	return false
}

func (m *mockSource) ToConfig() sources.SourceConfig {
	return nil
}

func TestOracleExecuteSqlAuthAndHeaderName(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// 1. Test GetAuthTokenHeaderName with authRequired
	cfgWithAuth := oracleexecutesql.Config{
		ConfigBase: tools.ConfigBase{
			Name:         "execute_sql",
			Description:  "Executes SQL",
			AuthRequired: []string{"google-auth"},
		},
		Type:   "oracle-execute-sql",
		Source: "my-oracle",
	}
	toolWithAuth, err := cfgWithAuth.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	headerName, err := toolWithAuth.GetAuthTokenHeaderName(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if headerName != "google-auth_token" {
		t.Errorf("expected header name 'google-auth_token', got %q", headerName)
	}

	// 2. Test GetAuthTokenHeaderName without authRequired
	cfgWithoutAuth := oracleexecutesql.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "execute_sql_no_auth",
			Description: "Executes SQL no auth",
		},
		Type:   "oracle-execute-sql",
		Source: "my-oracle",
	}
	toolNoAuth, err := cfgWithoutAuth.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	headerNameNoAuth, err := toolNoAuth.GetAuthTokenHeaderName(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if headerNameNoAuth != "Authorization" {
		t.Errorf("expected header name 'Authorization', got %q", headerNameNoAuth)
	}

	// 3. Test Invoke passes parsed JWT claims to Source via context
	mockSrc := &mockSource{}
	fakeJWT := "header.eyJlbWFpbCI6ImVicy1zZXJ2aWNlLWFjY291bnRAYWktbnktZGVtby5pYW0uZ3NlcnZpY2VhY2NvdW50LmNvbSIsInN1YiI6IjExNjMyNDY4NzI3NDczODI1NjIxOSJ9.signature"
	params := parameters.ParamValues{
		{Name: "sql", Value: "SELECT 1 FROM DUAL"},
	}

	_, invokeErr := toolWithAuth.Invoke(ctx, mockSrc, params, tools.AccessToken(fakeJWT))
	if invokeErr != nil {
		t.Fatalf("Invoke failed: %v", invokeErr)
	}

	if mockSrc.lastCtx == nil {
		t.Fatal("expected mockSrc.lastCtx to be non-nil")
	}

	claims := oracle.AuthClaimsFromContext(mockSrc.lastCtx)
	if claims == nil {
		t.Fatal("expected auth claims on context, got nil")
	}
	if claims["email"] != "ebs-service-account@ai-ny-demo.iam.gserviceaccount.com" {
		t.Errorf("expected email claim in context, got: %v", claims["email"])
	}

	toolParams := oracle.ToolParamsFromContext(mockSrc.lastCtx)
	if toolParams == nil || toolParams["sql"] != "SELECT 1 FROM DUAL" {
		t.Errorf("expected toolParams with sql, got: %v", toolParams)
	}
}
