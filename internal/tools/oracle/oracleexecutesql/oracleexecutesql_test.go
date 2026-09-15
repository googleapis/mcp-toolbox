// Copyright © 2025, Oracle and/or its affiliates.

package oracleexecutesql_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/oracle/oracleexecutesql"
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
