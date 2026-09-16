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

package falkordbschema

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

func TestParseFromYamlFalkorDB(t *testing.T) {
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
            type: falkordb-schema
            source: my-falkordb-instance
            description: some tool description
			`,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some tool description",
						AuthRequired: []string{},
					},
					Type:   "falkordb-schema",
					Source: "my-falkordb-instance",
				},
			},
		},
		{
			desc: "with sample size",
			in: `
            kind: tool
            name: example_tool
            type: falkordb-schema
            source: my-falkordb-instance
            description: some tool description
            sampleSize: 25
			`,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some tool description",
						AuthRequired: []string{},
					},
					Type:       "falkordb-schema",
					Source:     "my-falkordb-instance",
					SampleSize: 25,
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

func TestEscapeIdentifier(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want string
	}{
		{desc: "plain identifier is unchanged", in: "Person", want: "Person"},
		{desc: "spaces are preserved", in: "Order Item", want: "Order Item"},
		{desc: "backtick is doubled, not dropped", in: "we`ird", want: "we``ird"},
		{desc: "every backtick is doubled", in: "a`b`c", want: "a``b``c"},
		{desc: "leading and trailing backticks", in: "`x`", want: "``x``"},
		{desc: "empty identifier", in: "", want: ""},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if got := escapeIdentifier(tc.in); got != tc.want {
				t.Fatalf("escapeIdentifier(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFirstString(t *testing.T) {
	tcs := []struct {
		desc string
		in   any
		want string
	}{
		{desc: "first label of a list", in: []any{"Person", "User"}, want: "Person"},
		{desc: "empty list", in: []any{}, want: ""},
		{desc: "nil value", in: nil, want: ""},
		{desc: "non-list value", in: "Person", want: ""},
		{desc: "non-string first element", in: []any{int64(1)}, want: ""},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if got := firstString(tc.in); got != tc.want {
				t.Fatalf("firstString(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
