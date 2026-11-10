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

package cockroachdblisttables_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cockroachdb/cockroachdblisttables"
)

func TestParseFromYamlCockroachDBListTables(t *testing.T) {
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
            name: list_tables_tool
            type: cockroachdb-list-tables
            source: my-crdb-instance
            description: List tables in CockroachDB
			`,
			want: server.ToolConfigs{
				"list_tables_tool": cockroachdblisttables.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "list_tables_tool",
						Description:  "List tables in CockroachDB",
						AuthRequired: []string{},
					},
					Type:   "cockroachdb-list-tables",
					Source: "my-crdb-instance",
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

func TestCockroachDBListTablesToolConfigType(t *testing.T) {
	cfg := cockroachdblisttables.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test-tool",
			Description: "test description",
		},
		Type:   "cockroachdb-list-tables",
		Source: "test-source",
	}

	if cfg.ToolConfigType() != "cockroachdb-list-tables" {
		t.Errorf("expected ToolConfigType 'cockroachdb-list-tables', got %q", cfg.ToolConfigType())
	}
}
