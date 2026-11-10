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

package spannerlisttables_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/spanner/spannerlisttables"
)

func TestParseFromYamlListTables(t *testing.T) {
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
            type: spanner-list-tables
            source: my-spanner-instance
            description: Lists tables in the database
			`,
			want: server.ToolConfigs{
				"example_tool": spannerlisttables.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "Lists tables in the database",
						AuthRequired: []string{},
					},
					Type:   "spanner-list-tables",
					Source: "my-spanner-instance",
				},
			},
		},
		{
			desc: "with auth required",
			in: `
            kind: tool
            name: example_tool
            type: spanner-list-tables
            source: my-spanner-instance
            description: Lists tables in the database
            authRequired:
                - auth1
                - auth2
			`,
			want: server.ToolConfigs{
				"example_tool": spannerlisttables.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "Lists tables in the database",
						AuthRequired: []string{"auth1", "auth2"},
					},
					Type:   "spanner-list-tables",
					Source: "my-spanner-instance",
				},
			},
		},
		{
			desc: "minimal config",
			in: `
            kind: tool
            name: example_tool
            type: spanner-list-tables
            source: my-spanner-instance
			`,
			want: server.ToolConfigs{
				"example_tool": spannerlisttables.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "",
						AuthRequired: []string{},
					},
					Type:   "spanner-list-tables",
					Source: "my-spanner-instance",
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
