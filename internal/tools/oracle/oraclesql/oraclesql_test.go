// Copyright © 2025, Oracle and/or its affiliates.
package oraclesql_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/oracle/oraclesql"
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
