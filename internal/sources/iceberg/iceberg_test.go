// Copyright 2026 Google LLC
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

package iceberg_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/iceberg"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlIceberg(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-iceberg-instance
			type: iceberg
			uri: http://my-catalog:8181
			`,
			want: map[string]sources.SourceConfig{
				"my-iceberg-instance": iceberg.Config{
					Name:    "my-iceberg-instance",
					Type:    iceberg.SourceType,
					Catalog: iceberg.RESTCatalogType,
					Uri:     "http://my-catalog:8181",
				},
			},
		},
		{
			desc: "with all optional fields",
			in: `
			kind: source
			name: my-iceberg-instance
			type: iceberg
			catalog: rest
			uri: https://my-catalog.example.com
			warehouse: s3://my-warehouse/wh
			accessToken: my-token
			`,
			want: map[string]sources.SourceConfig{
				"my-iceberg-instance": iceberg.Config{
					Name:        "my-iceberg-instance",
					Type:        iceberg.SourceType,
					Catalog:     iceberg.RESTCatalogType,
					Uri:         "https://my-catalog.example.com",
					Warehouse:   "s3://my-warehouse/wh",
					AccessToken: "my-token",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
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
			name: my-iceberg-instance
			type: iceberg
			uri: http://my-catalog:8181
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-iceberg-instance\" as \"iceberg\": [1:1] unknown field \"foo\"\n>  1 | foo: bar\n       ^\n   2 | name: my-iceberg-instance\n   3 | type: iceberg\n   4 | uri: http://my-catalog:8181",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-iceberg-instance
			type: iceberg
			`,
			err: "error unmarshaling source: unable to parse source \"my-iceberg-instance\" as \"iceberg\": Key: 'Config.Uri' Error:Field validation for 'Uri' failed on the 'required' tag",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
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
