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

package tidb_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/tidb"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlTiDB(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-tidb-instance
			type: tidb
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-tidb-instance": tidb.Config{
					Name:     "my-tidb-instance",
					Type:     tidb.SourceType,
					Host:     "0.0.0.0",
					Port:     "my-port",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
					UseSSL:   false,
				},
			},
		},
		{
			desc: "with SSL enabled",
			in: `
			kind: source
			name: my-tidb-cloud
			type: tidb
			host: gateway01.us-west-2.prod.aws.tidbcloud.com
			port: 4000
			database: test_db
			user: cloud_user
			password: cloud_pass
			ssl: true
			`,
			want: map[string]sources.SourceConfig{
				"my-tidb-cloud": tidb.Config{
					Name:     "my-tidb-cloud",
					Type:     tidb.SourceType,
					Host:     "gateway01.us-west-2.prod.aws.tidbcloud.com",
					Port:     "4000",
					Database: "test_db",
					User:     "cloud_user",
					Password: "cloud_pass",
					UseSSL:   true,
				},
			},
		},
		{
			desc: "Change SSL enabled due to TiDB Cloud host",
			in: `
			kind: source
			name: my-tidb-cloud
			type: tidb
			host: gateway01.us-west-2.prod.aws.tidbcloud.com
			port: 4000
			database: test_db
			user: cloud_user
			password: cloud_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-tidb-cloud": tidb.Config{
					Name:     "my-tidb-cloud",
					Type:     tidb.SourceType,
					Host:     "gateway01.us-west-2.prod.aws.tidbcloud.com",
					Port:     "4000",
					Database: "test_db",
					User:     "cloud_user",
					Password: "cloud_pass",
					UseSSL:   true,
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
			name: my-tidb-instance
			type: tidb
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			ssl: false
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-tidb-instance\" as \"tidb\": [2:1] unknown field \"foo\"\n   1 | database: my_db\n>  2 | foo: bar\n       ^\n   3 | host: 0.0.0.0\n   4 | name: my-tidb-instance\n   5 | password: my_pass\n   6 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-tidb-instance
			type: tidb
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			ssl: false
			`,
			err: "error unmarshaling source: unable to parse source \"my-tidb-instance\" as \"tidb\": Key: 'Config.Host' Error:Field validation for 'Host' failed on the 'required' tag",
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

func TestIsTiDBCloudHost(t *testing.T) {
	tcs := []struct {
		desc string
		host string
		want bool
	}{
		{
			desc: "valid TiDB Cloud host - ap-southeast-1",
			host: "gateway01.ap-southeast-1.prod.aws.tidbcloud.com",
			want: true,
		},
		{
			desc: "invalid TiDB Cloud host - wrong domain",
			host: "gateway01.ap-southeast-1.prod.aws.tdbcloud.com",
			want: false,
		},
		{
			desc: "local IP address",
			host: "127.0.0.1",
			want: false,
		},
		{
			desc: "valid TiDB Cloud host - us-west-2",
			host: "gateway01.us-west-2.prod.aws.tidbcloud.com",
			want: true,
		},
		{
			desc: "valid TiDB Cloud host - dev environment",
			host: "gateway02.eu-west-1.dev.aws.tidbcloud.com",
			want: true,
		},
		{
			desc: "valid TiDB Cloud host - staging environment",
			host: "gateway03.us-east-1.staging.aws.tidbcloud.com",
			want: true,
		},
		{
			desc: "invalid - wrong gateway format",
			host: "gateway1.us-west-2.prod.aws.tidbcloud.com",
			want: false,
		},
		{
			desc: "invalid - missing environment",
			host: "gateway01.us-west-2.aws.tidbcloud.com",
			want: false,
		},
		{
			desc: "invalid - wrong subdomain",
			host: "gateway01.us-west-2.prod.aws.tidbcloud.org",
			want: false,
		},
		{
			desc: "invalid - localhost",
			host: "localhost",
			want: false,
		},
		{
			desc: "invalid - private IP",
			host: "192.168.1.1",
			want: false,
		},
		{
			desc: "invalid - empty string",
			host: "",
			want: false,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got := tidb.IsTiDBCloudHost(tc.host)
			if got != tc.want {
				t.Fatalf("isTiDBCloudHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}
