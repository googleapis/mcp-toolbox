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

package tidbsql_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/tidb/tidbsql"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlTiDB(t *testing.T) {
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
            type: tidb-sql
            source: my-tidb-instance
            description: some description
            statement: |
                SELECT * FROM SQL_STATEMENT;
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
			`,
			want: server.ToolConfigs{
				"example_tool": tidbsql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:      "tidb-sql",
					Source:    "my-tidb-instance",
					Statement: "SELECT * FROM SQL_STATEMENT;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("country", "some description", parameters.WithStringAuth(
							[]parameters.ParamAuthService{{Name: "my-google-auth-service", Field: "user_id"},
								{Name: "other-auth-service", Field: "user_id"}})),
					},
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

func TestParseFromYamlWithTemplateParamsTiDB(t *testing.T) {
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
            type: tidb-sql
            source: my-tidb-instance
            description: some description
            statement: |
                SELECT * FROM SQL_STATEMENT;
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
				"example_tool": tidbsql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:      "tidb-sql",
					Source:    "my-tidb-instance",
					Statement: "SELECT * FROM SQL_STATEMENT;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("country", "some description", parameters.WithStringAuth(
							[]parameters.ParamAuthService{{Name: "my-google-auth-service", Field: "user_id"},
								{Name: "other-auth-service", Field: "user_id"}})),
					},
					TemplateParameters: []parameters.Parameter{
						parameters.NewStringParameter("tableName", "The table to select hotels from."),
						parameters.NewArrayParameter("fieldArray", "The columns to return for the query.", parameters.NewStringParameter("column", "A column name that will be returned from the query.")),
					},
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
