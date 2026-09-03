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

package spanner_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/dataplex/searchcatalog"
	"github.com/googleapis/mcp-toolbox/internal/sources/spanner"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlSpannerDb(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-spanner-instance
			type: spanner
			project: my-project
			instance: my-instance
			database: my_db
			`,
			want: map[string]sources.SourceConfig{
				"my-spanner-instance": spanner.Config{
					Name:     "my-spanner-instance",
					Type:     spanner.SourceType,
					Project:  "my-project",
					Instance: "my-instance",
					Dialect:  "googlesql",
					Database: "my_db",
				},
			},
		},
		{
			desc: "gsql dialect",
			in: `
			kind: source
			name: my-spanner-instance
			type: spanner
			project: my-project
			instance: my-instance
			dialect: Googlesql 
			database: my_db
			`,
			want: map[string]sources.SourceConfig{
				"my-spanner-instance": spanner.Config{
					Name:     "my-spanner-instance",
					Type:     spanner.SourceType,
					Project:  "my-project",
					Instance: "my-instance",
					Dialect:  "googlesql",
					Database: "my_db",
				},
			},
		},
		{
			desc: "postgresql dialect",
			in: `
			kind: source
			name: my-spanner-instance
			type: spanner
			project: my-project
			instance: my-instance
			dialect: postgresql
			database: my_db
			`,
			want: map[string]sources.SourceConfig{
				"my-spanner-instance": spanner.Config{
					Name:     "my-spanner-instance",
					Type:     spanner.SourceType,
					Project:  "my-project",
					Instance: "my-instance",
					Dialect:  "postgresql",
					Database: "my_db",
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
			desc: "invalid dialect",
			in: `
			kind: source
			name: my-spanner-instance
			type: spanner
			project: my-project
			instance: my-instance
			dialect: fail
			database: my_db
			`,
			err: "error unmarshaling source: unable to parse source \"my-spanner-instance\" as \"spanner\": dialect invalid: must be one of \"googlesql\", or \"postgresql\"",
		},
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-spanner-instance
			type: spanner
			project: my-project
			instance: my-instance
			database: my_db
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-spanner-instance\" as \"spanner\": [2:1] unknown field \"foo\"\n   1 | database: my_db\n>  2 | foo: bar\n       ^\n   3 | instance: my-instance\n   4 | name: my-spanner-instance\n   5 | project: my-project\n   6 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-spanner-instance
			type: spanner
			project: my-project
			instance: my-instance
			`,
			err: "error unmarshaling source: unable to parse source \"my-spanner-instance\" as \"spanner\": Key: 'Config.Database' Error:Field validation for 'Database' failed on the 'required' tag",
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

func TestExtractType(t *testing.T) {
	typeMap := map[string]string{
		"cloud-spanner-instance": "SERVICE",
		"cloud-spanner-database": "DATABASE",
		"cloud-spanner-table":    "TABLE",
		"cloud-spanner-view":     "VIEW",
	}
	tcs := []struct {
		desc string
		in   string
		want string
	}{
		{
			desc: "mapped type instance",
			in:   "projects/my-project/locations/global/entryTypes/cloud-spanner-instance",
			want: "SERVICE",
		},
		{
			desc: "mapped type database",
			in:   "projects/my-project/locations/global/entryTypes/cloud-spanner-database",
			want: "DATABASE",
		},
		{
			desc: "mapped type table",
			in:   "projects/my-project/locations/global/entryTypes/cloud-spanner-table",
			want: "TABLE",
		},
		{
			desc: "mapped type view",
			in:   "projects/my-project/locations/global/entryTypes/cloud-spanner-view",
			want: "VIEW",
		},
		{
			desc: "no slash returns input",
			in:   "cloud-spanner-table",
			want: "cloud-spanner-table",
		},
		{
			desc: "unknown type with slash returns empty",
			in:   "projects/my-project/locations/global/entryTypes/unknown-type",
			want: "",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got := searchcatalog.ExtractType(tc.in, typeMap)
			if got != tc.want {
				t.Errorf("ExtractType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
