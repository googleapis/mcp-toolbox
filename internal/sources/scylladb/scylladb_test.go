// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scylladb_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/scylladb"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlScyllaDB(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example (without optional fields)",
			in: `
			kind: source
			name: my-scylladb-instance
			type: scylladb
			hosts:
				- "my-host1"
				- "my-host2"
			`,
			want: map[string]sources.SourceConfig{
				"my-scylladb-instance": scylladb.Config{
					Name:                     "my-scylladb-instance",
					Type:                     scylladb.SourceType,
					Hosts:                    []string{"my-host1", "my-host2"},
					Username:                 "",
					Password:                 "",
					LocalDC:                  "",
					ProtoVersion:             0,
					DisableInitialHostLookup: false,
					CAPath:                   "",
					CertPath:                 "",
					KeyPath:                  "",
					Keyspace:                 "",
					EnableHostVerification:   false,
				},
			},
		},
		{
			desc: "with optional fields",
			in: `
			kind: source
			name: my-scylladb-instance
			type: scylladb
			hosts:
				- "my-host1"
				- "my-host2"
			username: "user"
			password: "pass"
			keyspace: "example_keyspace"
			protoVersion: 4
			localDC: "AWS_US_EAST_1"
			caPath: "path/to/ca.crt"
			certPath: "path/to/cert"
			keyPath: "path/to/key"
			enableHostVerification: true
			`,
			want: map[string]sources.SourceConfig{
				"my-scylladb-instance": scylladb.Config{
					Name:                     "my-scylladb-instance",
					Type:                     scylladb.SourceType,
					Hosts:                    []string{"my-host1", "my-host2"},
					Username:                 "user",
					Password:                 "pass",
					Keyspace:                 "example_keyspace",
					ProtoVersion:             4,
					LocalDC:                  "AWS_US_EAST_1",
					DisableInitialHostLookup: false,
					CAPath:                   "path/to/ca.crt",
					CertPath:                 "path/to/cert",
					KeyPath:                  "path/to/key",
					EnableHostVerification:   true,
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
			name: my-scylladb-instance
			type: scylladb
			hosts:
				- "my-host"
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-scylladb-instance\" as \"scylladb\": [1:1] unknown field \"foo\"\n>  1 | foo: bar\n       ^\n   2 | hosts:\n   3 | - my-host\n   4 | name: my-scylladb-instance\n   5 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-scylladb-instance
			type: scylladb
			`,
			err: "error unmarshaling source: unable to parse source \"my-scylladb-instance\" as \"scylladb\": Key: 'Config.Hosts' Error:Field validation for 'Hosts' failed on the 'required' tag",
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
