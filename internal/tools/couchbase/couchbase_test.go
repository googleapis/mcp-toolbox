// Copyright 2024 Google LLC
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

package couchbase_test

import (
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/couchbase"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYaml(t *testing.T) {
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
			type: couchbase-sql
			source: my-couchbase-instance
			description: some tool description
			statement: |
				select * from hotel WHERE name = $hotel;
			parameters:
				- name: hotel
				  type: string
				  description: hotel parameter description
			`,
			want: server.ToolConfigs{
				"example_tool": couchbase.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						AuthRequired: []string{},
						Description:  "some tool description",
					},
					Type:      "couchbase-sql",
					Source:    "my-couchbase-instance",
					Statement: "select * from hotel WHERE name = $hotel;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("hotel", "hotel parameter description"),
					},
				},
			},
		},
		{
			desc: "with template",
			in: `
			kind: tool
			name: example_tool
			type: couchbase-sql
			source: my-couchbase-instance
			description: some tool description
			statement: |
				select * from {{.tableName}} WHERE name = $hotel;
			parameters:
				- name: hotel
				  type: string
				  description: hotel parameter description
			templateParameters:
				- name: tableName
				  type: string
				  description: The table to select hotels from.
			`,
			want: server.ToolConfigs{
				"example_tool": couchbase.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						AuthRequired: []string{},
						Description:  "some tool description",
					},
					Type:      "couchbase-sql",
					Source:    "my-couchbase-instance",
					Statement: "select * from {{.tableName}} WHERE name = $hotel;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("hotel", "hotel parameter description"),
					},
					TemplateParameters: []parameters.Parameter{
						parameters.NewStringParameter("tableName", "The table to select hotels from."),
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
