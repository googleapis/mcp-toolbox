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

package redis_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/redis"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlRedis(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "default setting",
			in: `
			kind: source
			name: my-redis-instance
			type: redis
			address:
			  - 127.0.0.1
			`,
			want: map[string]sources.SourceConfig{
				"my-redis-instance": redis.Config{
					Name:           "my-redis-instance",
					Type:           redis.SourceType,
					Address:        []string{"127.0.0.1"},
					ClusterEnabled: false,
					UseGCPIAM:      false,
				},
			},
		},
		{
			desc: "advanced example",
			in: `
			kind: source
			name: my-redis-instance
			type: redis
			address:
			  - 127.0.0.1
			password: my-pass
			database: 1
			useGCPIAM: true
			clusterEnabled: true
			tls:
			  enabled: true
			  insecureSkipVerify: true
			`,
			want: map[string]sources.SourceConfig{
				"my-redis-instance": redis.Config{
					Name:           "my-redis-instance",
					Type:           redis.SourceType,
					Address:        []string{"127.0.0.1"},
					Password:       "my-pass",
					Database:       1,
					ClusterEnabled: true,
					UseGCPIAM:      true,
					TLS: redis.TLSConfig{
						Enabled:            true,
						InsecureSkipVerify: true,
					},
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
			desc: "invalid database",
			in: `
			kind: source
			name: my-redis-instance
			type: redis
			address:
			- 127.0.0.1
			password: my-pass
			database: data
			`,
			err: "error unmarshaling source: unable to parse source \"my-redis-instance\" as \"redis\": [3:11] cannot unmarshal string into Go struct field Config.Database of type int\n   1 | address:\n   2 | - 127.0.0.1\n>  3 | database: data\n                 ^\n   4 | name: my-redis-instance\n   5 | password: my-pass\n   6 | type: redis",
		},
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-redis-instance
			type: redis
			project: my-project
			address:
			- 127.0.0.1
			password: my-pass
			database: 1
			`,
			err: "error unmarshaling source: unable to parse source \"my-redis-instance\" as \"redis\": [6:1] unknown field \"project\"\n   3 | database: 1\n   4 | name: my-redis-instance\n   5 | password: my-pass\n>  6 | project: my-project\n       ^\n   7 | type: redis",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-redis-instance
			type: redis
			`,
			err: "error unmarshaling source: unable to parse source \"my-redis-instance\" as \"redis\": Key: 'Config.Address' Error:Field validation for 'Address' failed on the 'required' tag",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.err) {
				t.Fatalf("unexpected error: got %q, want %q", errStr, tc.err)
			}
		})
	}
}
