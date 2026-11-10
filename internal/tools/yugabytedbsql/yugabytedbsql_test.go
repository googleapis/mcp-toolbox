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

package yugabytedbsql_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/yugabytedbsql"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlYugabyteDBSQL(t *testing.T) {
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
			desc: "basic valid config",
			in: `
			kind: tool
			name: hotel_search
			type: yugabytedb-sql
			source: yb-source
			description: search hotels by city
			statement: |
			  SELECT * FROM hotels WHERE city = $1;
			authRequired:
			  - auth-service-a
			  - auth-service-b
			parameters:
			  - name: city
			    type: string
			    description: city name
			    authServices:
			      - name: auth-service-a
			        field: user_id
			      - name: auth-service-b
			        field: user_id
			`,
			want: server.ToolConfigs{
				"hotel_search": yugabytedbsql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "hotel_search",
						Description:  "search hotels by city",
						AuthRequired: []string{"auth-service-a", "auth-service-b"},
					},
					Type:      "yugabytedb-sql",
					Source:    "yb-source",
					Statement: "SELECT * FROM hotels WHERE city = $1;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("city", "city name", parameters.WithStringAuth(
							[]parameters.ParamAuthService{
								{Name: "auth-service-a", Field: "user_id"},
								{Name: "auth-service-b", Field: "user_id"},
							})),
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

func TestFailParseFromYamlYugabyteDBSQL(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	cases := []struct {
		desc string
		in   string
	}{
		{
			desc: "missing required field (statement)",
			in: `
			kind: tool
			name: tool1
			type: yugabytedb-sql
			source: yb-source
			description: incomplete config
			`,
		},
		{
			desc: "unknown field (foo)",
			in: `
			kind: tool
			name: tool2
			type: yugabytedb-sql
			source: yb-source
			description: test
			statement: SELECT 1;
			foo: bar
			`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expected error but got none")
			}
		})
	}
}

func TestParseFromYamlWithTemplateParamsYugabyteDB(t *testing.T) {
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
			type: yugabytedb-sql
			source: my-yb-instance
			description: some description
			statement: |
				SELECT * FROM SQL_STATEMENT;
			parameters:
				- name: name
				  type: string
				  description: some description
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
				"example_tool": yugabytedbsql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:      "yugabytedb-sql",
					Source:    "my-yb-instance",
					Statement: "SELECT * FROM SQL_STATEMENT;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("name", "some description"),
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
