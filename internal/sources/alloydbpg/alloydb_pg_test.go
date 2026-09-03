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

package alloydbpg_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/alloydbpg"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlAlloyDBPg(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-pg-instance
			type: alloydb-postgres
			project: my-project
			region: my-region
			cluster: my-cluster
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-pg-instance": alloydbpg.Config{
					Name:     "my-pg-instance",
					Type:     alloydbpg.SourceType,
					Project:  "my-project",
					Region:   "my-region",
					Cluster:  "my-cluster",
					Instance: "my-instance",
					IPType:   "public",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
				},
			},
		},
		{
			desc: "public ipType",
			in: `
			kind: source
			name: my-pg-instance
			type: alloydb-postgres
			project: my-project
			region: my-region
			cluster: my-cluster
			instance: my-instance
			ipType: Public
			database: my_db
			user: my_user
			password: my_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-pg-instance": alloydbpg.Config{
					Name:     "my-pg-instance",
					Type:     alloydbpg.SourceType,
					Project:  "my-project",
					Region:   "my-region",
					Cluster:  "my-cluster",
					Instance: "my-instance",
					IPType:   "public",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
				},
			},
		},
		{
			desc: "private ipType",
			in: `
			kind: source
			name: my-pg-instance
			type: alloydb-postgres
			project: my-project
			region: my-region
			cluster: my-cluster
			instance: my-instance
			ipType: private
			database: my_db
			user: my_user
			password: my_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-pg-instance": alloydbpg.Config{
					Name:     "my-pg-instance",
					Type:     alloydbpg.SourceType,
					Project:  "my-project",
					Region:   "my-region",
					Cluster:  "my-cluster",
					Instance: "my-instance",
					IPType:   "private",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
				},
			},
		},
		{
			desc: "readOnly set to true",
			in: `
			kind: source
			name: my-pg-instance
			type: alloydb-postgres
			project: my-project
			region: my-region
			cluster: my-cluster
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
			readOnly: true
			`,
			want: map[string]sources.SourceConfig{
				"my-pg-instance": alloydbpg.Config{
					Name:     "my-pg-instance",
					Type:     alloydbpg.SourceType,
					Project:  "my-project",
					Region:   "my-region",
					Cluster:  "my-cluster",
					Instance: "my-instance",
					IPType:   "public",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
					ReadOnly: true,
				},
			},
		},
		{
			desc: "readOnly set to false",
			in: `
			kind: source
			name: my-pg-instance
			type: alloydb-postgres
			project: my-project
			region: my-region
			cluster: my-cluster
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
			readOnly: false
			`,
			want: map[string]sources.SourceConfig{
				"my-pg-instance": alloydbpg.Config{
					Name:     "my-pg-instance",
					Type:     alloydbpg.SourceType,
					Project:  "my-project",
					Region:   "my-region",
					Cluster:  "my-cluster",
					Instance: "my-instance",
					IPType:   "public",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
					ReadOnly: false,
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			got, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse: want %v, got %v", tc.want, got)
			}
			for _, sc := range got {
				if cfg, ok := sc.(alloydbpg.Config); ok {
					src := &alloydbpg.Source{Config: cfg}
					if src.IsReadOnly() != cfg.ReadOnly {
						t.Errorf("IsReadOnly() = %v, want %v", src.IsReadOnly(), cfg.ReadOnly)
					}
				}
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
			name: my-pg-instance
			type: alloydb-postgres
			project: my-project
			region: my-region
			cluster: my-cluster
			instance: my-instance
			ipType: fail 
			database: my_db
			user: my_user
			password: my_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-pg-instance\" as \"alloydb-postgres\": ipType invalid: must be one of \"public\", \"private\", or \"psc\"",
		},
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-pg-instance
			type: alloydb-postgres
			project: my-project
			region: my-region
			cluster: my-cluster
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-pg-instance\" as \"alloydb-postgres\": [3:1] unknown field \"foo\"\n   1 | cluster: my-cluster\n   2 | database: my_db\n>  3 | foo: bar\n       ^\n   4 | instance: my-instance\n   5 | name: my-pg-instance\n   6 | password: my_pass\n   7 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-pg-instance
			type: alloydb-postgres
			region: my-region
			cluster: my-cluster
			instance: my-instance
			database: my_db
			user: my_user
			password: my_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-pg-instance\" as \"alloydb-postgres\": Key: 'Config.Project' Error:Field validation for 'Project' failed on the 'required' tag",
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
