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

package arcadedb_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/arcadedb"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlArcadeDB(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-arcadedb-instance
			type: arcadedb
			uri: bolt://my-host:7687
			database: my_db
			user: my_user
			password: my_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-arcadedb-instance": arcadedb.Config{
					Name:     "my-arcadedb-instance",
					Type:     arcadedb.SourceType,
					Uri:      "bolt://my-host:7687",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
				},
			},
		},
		{
			desc: "http overrides",
			in: `
			kind: source
			name: my-arcadedb-instance
			type: arcadedb
			uri: bolt://my-host:7687
			database: my_db
			user: my_user
			password: my_pass
			httpUri: https://my-http-host:2481
			httpScheme: https
			httpPort: 2481
			`,
			want: map[string]sources.SourceConfig{
				"my-arcadedb-instance": arcadedb.Config{
					Name:       "my-arcadedb-instance",
					Type:       arcadedb.SourceType,
					Uri:        "bolt://my-host:7687",
					Database:   "my_db",
					User:       "my_user",
					Password:   "my_pass",
					HTTPUri:    "https://my-http-host:2481",
					HTTPScheme: "https",
					HTTPPort:   2481,
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

func TestFailParseFromYamlArcadeDB(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-arcadedb-instance
			type: arcadedb
			uri: bolt://my-host:7687
			database: my_db
			user: my_user
			password: my_pass
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-arcadedb-instance\" as \"arcadedb\": [2:1] unknown field \"foo\"\n   1 | database: my_db\n>  2 | foo: bar\n       ^\n   3 | name: my-arcadedb-instance\n   4 | password: my_pass\n   5 | type: arcadedb\n   6 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-arcadedb-instance
			type: arcadedb
			uri: bolt://my-host:7687
			database: my_db
			user: my_user
			`,
			err: "error unmarshaling source: unable to parse source \"my-arcadedb-instance\" as \"arcadedb\": Key: 'Config.Password' Error:Field validation for 'Password' failed on the 'required' tag",
		},
		{
			desc: "missing database",
			in: `
			kind: source
			name: my-arcadedb-instance
			type: arcadedb
			uri: bolt://my-host:7687
			user: my_user
			password: my_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-arcadedb-instance\" as \"arcadedb\": Key: 'Config.Database' Error:Field validation for 'Database' failed on the 'required' tag",
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
