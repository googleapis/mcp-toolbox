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

package elasticsearchexecuteesql

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

func TestParseFromYamlElasticsearchExecuteEsql(t *testing.T) {
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
			desc: "basic execute tool example",
			in: `
            kind: tool
            name: example_tool
            type: elasticsearch-execute-esql
            source: my-elasticsearch-instance
            description: Elasticsearch execute ES|QL tool
		`,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "Elasticsearch execute ES|QL tool",
						AuthRequired: []string{},
					},
					Type:   "elasticsearch-execute-esql",
					Source: "my-elasticsearch-instance",
				},
			},
		},
		{
			desc: "execute tool with format",
			in: `
            kind: tool
            name: example_tool_csv
            type: elasticsearch-execute-esql
            source: my-elasticsearch-instance
            description: Elasticsearch execute ES|QL tool in CSV
            format: csv
		`,
			want: server.ToolConfigs{
				"example_tool_csv": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool_csv",
						Description:  "Elasticsearch execute ES|QL tool in CSV",
						AuthRequired: []string{},
					},
					Type:   "elasticsearch-execute-esql",
					Source: "my-elasticsearch-instance",
					Format: "csv",
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
