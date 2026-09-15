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

package dgraph_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/dgraph"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlDgraph(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-dgraph-instance
			type: dgraph
			dgraphUrl: https://localhost:8080
			apiKey: abc123
			password: pass@123
			namespace: 0
			user: user123
			`,
			want: map[string]sources.SourceConfig{
				"my-dgraph-instance": dgraph.Config{
					Name:      "my-dgraph-instance",
					Type:      dgraph.SourceType,
					DgraphUrl: "https://localhost:8080",
					ApiKey:    "abc123",
					Password:  "pass@123",
					Namespace: 0,
					User:      "user123",
				},
			},
		},
		{
			desc: "basic example minimal field",
			in: `
			kind: source
			name: my-dgraph-instance
			type: dgraph
			dgraphUrl: https://localhost:8080
			`,
			want: map[string]sources.SourceConfig{
				"my-dgraph-instance": dgraph.Config{
					Name:      "my-dgraph-instance",
					Type:      dgraph.SourceType,
					DgraphUrl: "https://localhost:8080",
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

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
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
			name: my-dgraph-instance
			type: dgraph
			dgraphUrl: https://localhost:8080
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-dgraph-instance\" as \"dgraph\": [2:1] unknown field \"foo\"\n   1 | dgraphUrl: https://localhost:8080\n>  2 | foo: bar\n       ^\n   3 | name: my-dgraph-instance\n   4 | type: dgraph",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-dgraph-instance
			type: dgraph
			`,
			err: "error unmarshaling source: unable to parse source \"my-dgraph-instance\" as \"dgraph\": Key: 'Config.DgraphUrl' Error:Field validation for 'DgraphUrl' failed on the 'required' tag",
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
