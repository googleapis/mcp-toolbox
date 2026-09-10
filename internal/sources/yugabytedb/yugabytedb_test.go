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

package yugabytedb_test

import (
	"context"
	"testing"

	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/yugabytedb"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

// Basic config parse
func TestParseFromYamlYugabyteDB(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "only required fields",
			in: `
			kind: source
			name: my-yb-instance
			type: yugabytedb
			host: yb-host
			port: yb-port
			user: yb_user
			password: yb_pass
			database: yb_db
			`,
			want: map[string]sources.SourceConfig{
				"my-yb-instance": yugabytedb.Config{
					Name:     "my-yb-instance",
					Type:     "yugabytedb",
					Host:     "yb-host",
					Port:     "yb-port",
					User:     "yb_user",
					Password: "yb_pass",
					Database: "yb_db",
				},
			},
		},
		{
			desc: "with loadBalance only",
			in: `
			kind: source
			name: my-yb-instance
			type: yugabytedb
			host: yb-host
			port: yb-port
			user: yb_user
			password: yb_pass
			database: yb_db
			loadBalance: true
			`,
			want: map[string]sources.SourceConfig{
				"my-yb-instance": yugabytedb.Config{
					Name:        "my-yb-instance",
					Type:        "yugabytedb",
					Host:        "yb-host",
					Port:        "yb-port",
					User:        "yb_user",
					Password:    "yb_pass",
					Database:    "yb_db",
					LoadBalance: "true",
				},
			},
		},
		{
			desc: "loadBalance with topologyKeys",
			in: `
			kind: source
			name: my-yb-instance
			type: yugabytedb
			host: yb-host
			port: yb-port
			user: yb_user
			password: yb_pass
			database: yb_db
			loadBalance: true
			topologyKeys: zone1,zone2
			`,
			want: map[string]sources.SourceConfig{
				"my-yb-instance": yugabytedb.Config{
					Name:         "my-yb-instance",
					Type:         "yugabytedb",
					Host:         "yb-host",
					Port:         "yb-port",
					User:         "yb_user",
					Password:     "yb_pass",
					Database:     "yb_db",
					LoadBalance:  "true",
					TopologyKeys: "zone1,zone2",
				},
			},
		},
		{
			desc: "with fallback only",
			in: `
			kind: source
			name: my-yb-instance
			type: yugabytedb
			host: yb-host
			port: yb-port
			user: yb_user
			password: yb_pass
			database: yb_db
			loadBalance: true
			topologyKeys: zone1
			fallbackToTopologyKeysOnly: true
			`,
			want: map[string]sources.SourceConfig{
				"my-yb-instance": yugabytedb.Config{
					Name:                       "my-yb-instance",
					Type:                       "yugabytedb",
					Host:                       "yb-host",
					Port:                       "yb-port",
					User:                       "yb_user",
					Password:                   "yb_pass",
					Database:                   "yb_db",
					LoadBalance:                "true",
					TopologyKeys:               "zone1",
					FallBackToTopologyKeysOnly: "true",
				},
			},
		},
		{
			desc: "with refresh interval and reconnect delay",
			in: `
			kind: source
			name: my-yb-instance
			type: yugabytedb
			host: yb-host
			port: yb-port
			user: yb_user
			password: yb_pass
			database: yb_db
			loadBalance: true
			ybServersRefreshInterval: 20
			failedHostReconnectDelaySecs: 5
			`,
			want: map[string]sources.SourceConfig{
				"my-yb-instance": yugabytedb.Config{
					Name:                            "my-yb-instance",
					Type:                            "yugabytedb",
					Host:                            "yb-host",
					Port:                            "yb-port",
					User:                            "yb_user",
					Password:                        "yb_pass",
					Database:                        "yb_db",
					LoadBalance:                     "true",
					YBServersRefreshInterval:        "20",
					FailedHostReconnectDelaySeconds: "5",
				},
			},
		},
		{
			desc: "all fields set",
			in: `
			kind: source
			name: my-yb-instance
			type: yugabytedb
			host: yb-host
			port: yb-port
			user: yb_user
			password: yb_pass
			database: yb_db
			loadBalance: true
			topologyKeys: zone1,zone2
			fallbackToTopologyKeysOnly: true
			ybServersRefreshInterval: 30
			failedHostReconnectDelaySecs: 10
			`,
			want: map[string]sources.SourceConfig{
				"my-yb-instance": yugabytedb.Config{
					Name:                            "my-yb-instance",
					Type:                            "yugabytedb",
					Host:                            "yb-host",
					Port:                            "yb-port",
					User:                            "yb_user",
					Password:                        "yb_pass",
					Database:                        "yb_db",
					LoadBalance:                     "true",
					TopologyKeys:                    "zone1,zone2",
					FallBackToTopologyKeysOnly:      "true",
					YBServersRefreshInterval:        "30",
					FailedHostReconnectDelaySeconds: "10",
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
				t.Fatalf("incorrect parse (-want +got):\n%s", cmp.Diff(tc.want, got))
			}
		})
	}
}

func TestFailParseFromYamlYugabyteDB(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-yb-source
			type: yugabytedb
			host: yb-host
			port: yb-port
			database: yb_db
			user: yb_user
			password: yb_pass
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-yb-source\" as \"yugabytedb\": [2:1] unknown field \"foo\"\n   1 | database: yb_db\n>  2 | foo: bar\n       ^\n   3 | host: yb-host\n   4 | name: my-yb-source\n   5 | password: yb_pass\n   6 | ",
		},
		{
			desc: "missing required field (password)",
			in: `
			kind: source
			name: my-yb-source
			type: yugabytedb
			host: yb-host
			port: yb-port
			database: yb_db
			user: yb_user
			`,
			err: "error unmarshaling source: unable to parse source \"my-yb-source\" as \"yugabytedb\": Key: 'Config.Password' Error:Field validation for 'Password' failed on the 'required' tag",
		},
		{
			desc: "missing required field (host)",
			in: `
			kind: source
			name: my-yb-source
			type: yugabytedb
			port: yb-port
			database: yb_db
			user: yb_user
			password: yb_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-yb-source\" as \"yugabytedb\": Key: 'Config.Host' Error:Field validation for 'Host' failed on the 'required' tag",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expected parsing to fail")
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.err) {
				t.Fatalf("unexpected error:\nGot:  %q\nWant: %q", errStr, tc.err)
			}
		})
	}
}
