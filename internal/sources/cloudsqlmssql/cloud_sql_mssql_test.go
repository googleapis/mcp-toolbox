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

package cloudsqlmssql_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/cloudsqlmssql"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlCloudSQLMssql(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-instance
			type: cloud-sql-mssql
			project: my-project
			region: my-region
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": cloudsqlmssql.Config{
					Name:     "my-instance",
					Type:     cloudsqlmssql.SourceType,
					Project:  "my-project",
					Region:   "my-region",
					Instance: "my-instance",
					IPType:   "public",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
				},
			},
		},
		{
			desc: "psc ipType",
			in: `
			kind: source
			name: my-instance
			type: cloud-sql-mssql
			project: my-project
			region: my-region
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
			ipType: psc
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": cloudsqlmssql.Config{
					Name:     "my-instance",
					Type:     cloudsqlmssql.SourceType,
					Project:  "my-project",
					Region:   "my-region",
					Instance: "my-instance",
					IPType:   "psc",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
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
				t.Fatalf("incorrect psarse: want %v, got %v", tc.want, got)
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
			desc: "invalid ipType",
			in: `
			kind: source
			name: my-instance
			type: cloud-sql-mssql
			project: my-project
			region: my-region
			instance: my-instance
			ipType: fail
			database: my_db
			user: my_user
			password: my_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-instance\" as \"cloud-sql-mssql\": ipType invalid: must be one of \"public\", \"private\", or \"psc\"",
		},
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-instance
			type: cloud-sql-mssql
			project: my-project
			region: my-region
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-instance\" as \"cloud-sql-mssql\": [2:1] unknown field \"foo\"\n   1 | database: my_db\n>  2 | foo: bar\n       ^\n   3 | instance: my-instance\n   4 | name: my-instance\n   5 | password: my_pass\n   6 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-instance
			type: cloud-sql-mssql
			region: my-region
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-instance\" as \"cloud-sql-mssql\": Key: 'Config.Project' Error:Field validation for 'Project' failed on the 'required' tag",
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
