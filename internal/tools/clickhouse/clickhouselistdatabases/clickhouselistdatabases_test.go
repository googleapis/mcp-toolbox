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

func TestListDatabasesConfigToolConfigType(t *testing.T) {
	cfg := Config{}
	if cfg.ToolConfigType() != listDatabasesType {
		t.Errorf("expected %q, got %q", listDatabasesType, cfg.ToolConfigType())
	}
}

func TestParseFromYamlClickHouseListDatabases(t *testing.T) {
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
            type: clickhouse-list-databases
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
					Type:   "clickhouse-list-databases",
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

func TestListDatabasesToolParseParams(t *testing.T) {
	tool := Tool{
		BaseTool: tools.BaseTool[Config]{
			StaticParameters: parameters.Parameters{},
		},
	}

	toolParams, _ := tool.GetParameters(nil)
	params, err := parameters.ParseParams(toolParams, map[string]any{}, map[string]map[string]any{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(params) != 0 {
		t.Errorf("expected 0 parameters, got %d", len(params))
	}
}
