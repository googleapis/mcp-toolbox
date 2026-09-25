// Copyright © 2025, Oracle and/or its affiliates.
package oraclesql_test

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
	"github.com/googleapis/mcp-toolbox/internal/tools/oracle/oraclesql"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlOracleSql(t *testing.T) {
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
			desc: "basic example with statement and auth",
			in: `
            kind: tool
            name: get_user_by_id
            type: oracle-sql
            source: my-oracle-instance
            description: Retrieves user details by ID.
            statement: "SELECT id, name, email FROM users WHERE id = :1"
            authRequired:
                - my-google-auth-service
            `,
			want: server.ToolConfigs{
				"get_user_by_id": oraclesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "get_user_by_id",
						Description:  "Retrieves user details by ID.",
						AuthRequired: []string{"my-google-auth-service"},
					},
					Type:      "oracle-sql",
					Source:    "my-oracle-instance",
					Statement: "SELECT id, name, email FROM users WHERE id = :1",
					ReadOnly:  nil,
				},
			},
		},
		{
			desc: "example with parameters and template parameters",
			in: `
            kind: tool
            name: get_orders
            type: oracle-sql
            source: db-prod
            description: Gets orders for a customer with optional filtering.
            statement: "SELECT * FROM ${SCHEMA}.ORDERS WHERE customer_id = :customer_id AND status = :status"
            `,
			want: server.ToolConfigs{
				"get_orders": oraclesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "get_orders",
						Description:  "Gets orders for a customer with optional filtering.",
						AuthRequired: []string{},
					},
					Type:      "oracle-sql",
					Source:    "db-prod",
					Statement: "SELECT * FROM ${SCHEMA}.ORDERS WHERE customer_id = :customer_id AND status = :status",
					ReadOnly:  nil,
				},
			},
		},
		{
			desc: "explicit: readOnly set to true",
			in: `
			kind: tool
			name: safe_query
			type: oracle-sql
			source: db-prod
			description: Safe read operation.
			readOnly: true
			statement: "SELECT * FROM orders"
			`,
			want: server.ToolConfigs{
				"safe_query": oraclesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "safe_query",
						Description:  "Safe read operation.",
						AuthRequired: []string{},
					},
					Type:      "oracle-sql",
					Source:    "db-prod",
					Statement: "SELECT * FROM orders",
					ReadOnly:  &valTrue,
				},
			},
		},
		{
			desc: "example with readonly flag set to false (DML)",
			in: `
			kind: tool
			name: update_user
			type: oracle-sql
			source: db-prod
			description: Updates user email.
			readOnly: false
			statement: "UPDATE users SET email = :1 WHERE id = :2"
			`,
			want: server.ToolConfigs{
				"update_user": oraclesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "update_user",
						Description:  "Updates user email.",
						AuthRequired: []string{},
					},
					Type:      "oracle-sql",
					Source:    "db-prod",
					Statement: "UPDATE users SET email = :1 WHERE id = :2",
					ReadOnly:  &valFalse,
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
	lastCtx  context.Context
	lastSQL  string
	lastArgs []any
}

func (m *mockSource) OracleDB() *sql.DB {
	return nil
}

func (m *mockSource) RunSQL(ctx context.Context, s string, args []any, _ bool) (any, error) {
	m.lastCtx = ctx
	m.lastSQL = s
	m.lastArgs = args
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

func TestOracleSqlAuthAndHeaderName(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// 1. Test GetAuthTokenHeaderName with authRequired
	cfgWithAuth := oraclesql.Config{
		ConfigBase: tools.ConfigBase{
			Name:         "get_user",
			Description:  "Gets user",
			AuthRequired: []string{"google-auth"},
		},
		Type:      "oracle-sql",
		Source:    "my-oracle",
		Statement: "SELECT 1 FROM DUAL WHERE id = :1",
		Parameters: parameters.Parameters{
			parameters.NewStringParameter("id", "User ID"),
		},
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
	cfgWithoutAuth := oraclesql.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "get_user_no_auth",
			Description: "Gets user no auth",
		},
		Type:      "oracle-sql",
		Source:    "my-oracle",
		Statement: "SELECT 1 FROM DUAL",
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

	// 3. Test Invoke passes parsed JWT claims and mergedParams to Source via context
	mockSrc := &mockSource{}
	fakeJWT := "header.eyJlbWFpbCI6ImVicy1zZXJ2aWNlLWFjY291bnRAYWktbnktZGVtby5pYW0uZ3NlcnZpY2VhY2NvdW50LmNvbSIsInN1YiI6IjExNjMyNDY4NzI3NDczODI1NjIxOSJ9.signature"
	params := parameters.ParamValues{
		{Name: "id", Value: "123"},
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
	if toolParams == nil || toolParams["id"] != "123" {
		t.Errorf("expected toolParams with id=123, got: %v", toolParams)
	}
}
