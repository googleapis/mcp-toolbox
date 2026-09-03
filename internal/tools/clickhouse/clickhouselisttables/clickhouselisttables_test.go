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

package clickhouse

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestListTablesConfigToolConfigType(t *testing.T) {
	cfg := Config{}
	if cfg.ToolConfigType() != listTablesType {
		t.Errorf("expected %q, got %q", listTablesType, cfg.ToolConfigType())
	}
}

func TestParseFromYamlClickHouseListTables(t *testing.T) {
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
            type: clickhouse-list-tables
            source: my-instance
            description: some description
            `,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "clickhouse-list-tables",
					Source: "my-instance",
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

func TestListTablesToolParseParams(t *testing.T) {
	databaseParam := parameters.NewStringParameter("database", "The database to list tables from.")
	tool := Tool{
		BaseTool: tools.BaseTool[Config]{
			StaticParameters: parameters.Parameters{databaseParam},
		},
	}

	toolParams, _ := tool.GetParameters(nil)
	params, err := parameters.ParseParams(toolParams, map[string]any{"database": "test_db"}, map[string]map[string]any{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(params) != 1 {
		t.Errorf("expected 1 parameter, got %d", len(params))
	}

	mapParams := params.AsMap()
	if mapParams["database"] != "test_db" {
		t.Errorf("expected database parameter to be 'test_db', got %v", mapParams["database"])
	}
}

func TestValidIdentifierRegexp(t *testing.T) {
	tcs := []struct {
		in    string
		valid bool
	}{
		// Allowed: plain identifiers.
		{"default", true},
		{"my_db", true},
		{"DB1", true},
		{"_internal", true},
		{"a", true},

		// Rejected: empty, whitespace, control characters.
		{"", false},
		{" ", false},
		{"\t", false},
		{"\n", false},

		// Rejected: leading digit (ClickHouse identifier rule).
		{"1db", false},

		// Rejected: identifier-quoting characters that would let the value
		// re-open the identifier and append clauses after it.
		{"`default`", false},
		{`"default"`, false},

		// Rejected: separators / statement terminators / metacharacters.
		{"default;DROP TABLE x", false},
		{"default LIKE '%'", false},
		{"default LIMIT 0 FORMAT JSON", false},
		{"default INTO OUTFILE '/tmp/x'", false},
		{"system.tables", false},
		{"default--", false},
		{"default/*", false},

		// Rejected: unicode and surrounding spaces.
		{"𝐝𝐞𝐟𝐚𝐮𝐥𝐭", false},
		{"  default  ", false},
		{"default ", false},
		{" default", false},
	}
	for _, tc := range tcs {
		if got := validIdentifier.MatchString(tc.in); got != tc.valid {
			t.Errorf("validIdentifier.MatchString(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}
