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

package cockroachdbsql_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cockroachdb/cockroachdbsql"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlCockroachDB(t *testing.T) {
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
            type: cockroachdb-sql
            source: my-crdb-instance
            description: some description
            statement: |
                SELECT * FROM SQL_STATEMENT;
            parameters:
                - name: user_id
                  type: string
                  description: user id parameter
			`,
			want: server.ToolConfigs{
				"example_tool": cockroachdbsql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:      "cockroachdb-sql",
					Source:    "my-crdb-instance",
					Statement: "SELECT * FROM SQL_STATEMENT;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("user_id", "user id parameter"),
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

func TestCockroachDBSQLToolConfigType(t *testing.T) {
	cfg := cockroachdbsql.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test-tool",
			Description: "test description",
		},
		Type:      "cockroachdb-sql",
		Source:    "test-source",
		Statement: "SELECT 1",
	}

	if cfg.ToolConfigType() != "cockroachdb-sql" {
		t.Errorf("expected ToolConfigType 'cockroachdb-sql', got %q", cfg.ToolConfigType())
	}
}
