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

package sqlite_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/sqlite"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlSQLite(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
            kind: source
            name: my-sqlite-db
            type: sqlite
            database: /path/to/database.db
            `,
			want: map[string]sources.SourceConfig{
				"my-sqlite-db": sqlite.Config{
					Name:     "my-sqlite-db",
					Type:     sqlite.SourceType,
					Database: "/path/to/database.db",
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
            name: my-sqlite-db
            type: sqlite
            database: /path/to/database.db
            foo: bar
            `,
			err: "error unmarshaling source: unable to parse source \"my-sqlite-db\" as \"sqlite\": [2:1] unknown field \"foo\"\n   1 | database: /path/to/database.db\n>  2 | foo: bar\n       ^\n   3 | name: my-sqlite-db\n   4 | type: sqlite",
		},
		{
			desc: "missing required field",
			in: `
            kind: source
            name: my-sqlite-db
            type: sqlite
            `,
			err: "error unmarshaling source: unable to parse source \"my-sqlite-db\" as \"sqlite\": Key: 'Config.Database' Error:Field validation for 'Database' failed on the 'required' tag",
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
