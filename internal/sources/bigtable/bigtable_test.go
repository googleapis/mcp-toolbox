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

package bigtable_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/bigtable"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlBigtableDb(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "can configure with a bigtable table",
			in: `
			kind: source
			name: my-bigtable-instance
			type: bigtable
			project: my-project
			instance: my-instance
			`,
			want: map[string]sources.SourceConfig{
				"my-bigtable-instance": bigtable.Config{
					Name:     "my-bigtable-instance",
					Type:     bigtable.SourceType,
					Project:  "my-project",
					Instance: "my-instance",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse: want %v, got %v", tc.want, got)
			}
		})
	}

}

func TestFailParseFromYaml(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-bigtable-instance
			type: bigtable
			project: my-project
			instance: my-instance
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-bigtable-instance\" as \"bigtable\": [1:1] unknown field \"foo\"\n>  1 | foo: bar\n       ^\n   2 | instance: my-instance\n   3 | name: my-bigtable-instance\n   4 | project: my-project\n   5 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-bigtable-instance
			type: bigtable
			project: my-project
			`,
			err: "error unmarshaling source: unable to parse source \"my-bigtable-instance\" as \"bigtable\": Key: 'Config.Instance' Error:Field validation for 'Instance' failed on the 'required' tag",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if errStr != tc.err {
				t.Fatalf("unexpected error: got %q, want %q", errStr, tc.err)
			}
		})
	}
}
