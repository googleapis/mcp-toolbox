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

package oceanbase_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/oceanbase"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

// Test parsing OceanBase source config from YAML.
func TestParseFromYamlOceanBase(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-oceanbase-instance
			type: oceanbase
			host: 0.0.0.0
			port: 2881
			database: ob_db
			user: ob_user
			password: ob_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-oceanbase-instance": oceanbase.Config{
					Name:     "my-oceanbase-instance",
					Type:     oceanbase.SourceType,
					Host:     "0.0.0.0",
					Port:     "2881",
					Database: "ob_db",
					User:     "ob_user",
					Password: "ob_pass",
				},
			},
		},
		{
			desc: "with query timeout",
			in: `
			kind: source
			name: my-oceanbase-instance
			type: oceanbase
			host: 0.0.0.0
			port: 2881
			database: ob_db
			user: ob_user
			password: ob_pass
			queryTimeout: 30s
			`,
			want: map[string]sources.SourceConfig{
				"my-oceanbase-instance": oceanbase.Config{
					Name:         "my-oceanbase-instance",
					Type:         oceanbase.SourceType,
					Host:         "0.0.0.0",
					Port:         "2881",
					Database:     "ob_db",
					User:         "ob_user",
					Password:     "ob_pass",
					QueryTimeout: "30s",
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

// Test parsing failure cases for OceanBase source config.
func TestFailParseFromYamlOceanBase(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-oceanbase-instance
			type: oceanbase
			host: 0.0.0.0
			port: 2881
			database: ob_db
			user: ob_user
			password: ob_pass
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-oceanbase-instance\" as \"oceanbase\": [2:1] unknown field \"foo\"\n   1 | database: ob_db\n>  2 | foo: bar\n       ^\n   3 | host: 0.0.0.0\n   4 | name: my-oceanbase-instance\n   5 | password: ob_pass\n   6 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-oceanbase-instance
			type: oceanbase
			port: 2881
			database: ob_db
			user: ob_user
			password: ob_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-oceanbase-instance\" as \"oceanbase\": Key: 'Config.Host' Error:Field validation for 'Host' failed on the 'required' tag",
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
