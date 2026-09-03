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

package valkey_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/valkey"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlvalkey(t *testing.T) {
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
			name: valkey_tool
			type: valkey
			source: my-valkey-instance
			description: some description
			commands:
				- [SET, greeting, "hello, {{.name}}"]
				- [GET, id]
			parameters:
				- name: name
				  type: string
				  description: user name
			`,
			want: server.ToolConfigs{
				"valkey_tool": valkey.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "valkey_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:     "valkey",
					Source:   "my-valkey-instance",
					Commands: [][]string{{"SET", "greeting", "hello, {{.name}}"}, {"GET", "id"}},
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("name", "user name"),
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
