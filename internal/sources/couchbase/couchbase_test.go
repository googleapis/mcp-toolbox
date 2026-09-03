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

package couchbase_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/couchbase"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlCouchbase(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-couchbase-instance
			type: couchbase
			connectionString: localhost
			username: Administrator
			password: password
			bucket: travel-sample
			scope: inventory
			`,
			want: map[string]sources.SourceConfig{
				"my-couchbase-instance": couchbase.Config{
					Name:             "my-couchbase-instance",
					Type:             couchbase.SourceType,
					ConnectionString: "localhost",
					Username:         "Administrator",
					Password:         "password",
					Bucket:           "travel-sample",
					Scope:            "inventory",
				},
			},
		},
		{
			desc: "with TLS configuration",
			in: `
			kind: source
			name: my-couchbase-instance
			type: couchbase
			connectionString: couchbases://localhost
			bucket: travel-sample
			scope: inventory
			clientCert: /path/to/cert.pem
			clientKey: /path/to/key.pem
			clientCertPassword: password
			clientKeyPassword: password
			caCert: /path/to/ca.pem
			noSslVerify: false
			queryScanConsistency: 2
			`,
			want: map[string]sources.SourceConfig{
				"my-couchbase-instance": couchbase.Config{
					Name:                 "my-couchbase-instance",
					Type:                 couchbase.SourceType,
					ConnectionString:     "couchbases://localhost",
					Bucket:               "travel-sample",
					Scope:                "inventory",
					ClientCert:           "/path/to/cert.pem",
					ClientKey:            "/path/to/key.pem",
					ClientCertPassword:   "password",
					ClientKeyPassword:    "password",
					CACert:               "/path/to/ca.pem",
					NoSSLVerify:          false,
					QueryScanConsistency: 2,
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
			name: my-couchbase-instance
			type: couchbase
			connectionString: localhost
			username: Administrator
			password: password
			bucket: travel-sample
			scope: inventory
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-couchbase-instance\" as \"couchbase\": [3:1] unknown field \"foo\"\n   1 | bucket: travel-sample\n   2 | connectionString: localhost\n>  3 | foo: bar\n       ^\n   4 | name: my-couchbase-instance\n   5 | password: password\n   6 | scope: inventory\n   7 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-couchbase-instance
			type: couchbase
			username: Administrator
			password: password
			bucket: travel-sample
			scope: inventory
			`,
			err: "error unmarshaling source: unable to parse source \"my-couchbase-instance\" as \"couchbase\": Key: 'Config.ConnectionString' Error:Field validation for 'ConnectionString' failed on the 'required' tag",
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
