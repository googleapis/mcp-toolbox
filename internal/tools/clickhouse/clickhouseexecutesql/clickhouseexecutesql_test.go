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
	"fmt"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

func TestParseFromYamlClickHouseExecuteSQL(t *testing.T) {
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
            type: clickhouse-execute-sql
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
					Type:   "clickhouse-execute-sql",
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

func TestToQueryParameters(t *testing.T) {
	tcs := []struct {
		desc    string
		in      any
		want    clickhouse.Parameters
		wantErr bool
	}{
		{
			desc: "absent",
			in:   nil,
			want: nil,
		},
		{
			desc: "empty",
			in:   map[string]any{},
			want: clickhouse.Parameters{},
		},
		{
			desc: "values in ClickHouse's text format",
			in:   map[string]any{"n": "42", "names": "['a','b']", "s": "x'; DROP TABLE t"},
			want: clickhouse.Parameters{"n": "42", "names": "['a','b']", "s": "x'; DROP TABLE t"},
		},
		{
			desc:    "value that is not a string",
			in:      map[string]any{"n": 42},
			wantErr: true,
		},
		{
			desc:    "not an object",
			in:      "n=42",
			wantErr: true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := toQueryParameters(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error: got %v, wantErr %v", err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("incorrect parameters: diff %v", diff)
			}
		})
	}
}

func TestExecuteSQLManifestParams(t *testing.T) {
	tool, err := Config{
		ConfigBase: tools.ConfigBase{Name: "my-tool", Description: "some description"},
		Type:       executeSQLType,
		Source:     "my-instance",
	}.Initialize(t.Context())
	if err != nil {
		t.Fatalf("unable to initialize tool: %s", err)
	}
	var got []string
	for _, p := range tool.StaticManifest().Parameters {
		got = append(got, fmt.Sprintf("%s required=%t", p.Name, p.Required))
	}
	want := []string{"sql required=true", "params required=false"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("incorrect manifest parameters: diff %v", diff)
	}
}
