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

package falkordb

import (
	"context"
	"testing"

	falkordbgo "github.com/FalkorDB/falkordb-go/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlFalkorDB(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-falkordb-instance
			type: falkordb
			host: my-host
			port: "6379"
			graph: my_graph
			`,
			want: map[string]sources.SourceConfig{
				"my-falkordb-instance": Config{
					Name:  "my-falkordb-instance",
					Type:  SourceType,
					Host:  "my-host",
					Port:  "6379",
					Graph: "my_graph",
				},
			},
		},
		{
			desc: "with auth, timeout and TLS",
			in: `
			kind: source
			name: my-falkordb-instance
			type: falkordb
			host: my-host
			port: "6380"
			username: my_user
			password: my_pass
			graph: my_graph
			queryTimeoutMs: 5000
			tls:
			    enabled: true
			    insecureSkipVerify: true
			`,
			want: map[string]sources.SourceConfig{
				"my-falkordb-instance": Config{
					Name:           "my-falkordb-instance",
					Type:           SourceType,
					Host:           "my-host",
					Port:           "6380",
					Username:       "my_user",
					Password:       "my_pass",
					Graph:          "my_graph",
					QueryTimeoutMs: 5000,
					TLS: TLSConfig{
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
			desc: "extra field",
			in: `
			kind: source
			name: my-falkordb-instance
			type: falkordb
			host: my-host
			port: "6379"
			graph: my_graph
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-falkordb-instance\" as \"falkordb\": [1:1] unknown field \"foo\"\n>  1 | foo: bar\n       ^\n   2 | graph: my_graph\n   3 | host: my-host\n   4 | name: my-falkordb-instance\n   5 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-falkordb-instance
			type: falkordb
			host: my-host
			port: "6379"
			`,
			err: "error unmarshaling source: unable to parse source \"my-falkordb-instance\" as \"falkordb\": Key: 'Config.Graph' Error:Field validation for 'Graph' failed on the 'required' tag",
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

func TestValidateTLS(t *testing.T) {
	tcs := []struct {
		desc               string
		enabled            bool
		insecureSkipVerify bool
		wantErr            bool
	}{
		{
			desc:               "insecureSkipVerify without tls is rejected",
			enabled:            false,
			insecureSkipVerify: true,
			wantErr:            true,
		},
		{
			desc:               "tls disabled without insecureSkipVerify is accepted",
			enabled:            false,
			insecureSkipVerify: false,
			wantErr:            false,
		},
		{
			desc:               "insecureSkipVerify with tls enabled is accepted",
			enabled:            true,
			insecureSkipVerify: true,
			wantErr:            false,
		},
		{
			desc:               "tls enabled with verification is accepted",
			enabled:            true,
			insecureSkipVerify: false,
			wantErr:            false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := Config{
				Name: "my-falkordb-instance",
				TLS: TLSConfig{
					Enabled:            tc.enabled,
					InsecureSkipVerify: tc.insecureSkipVerify,
				},
			}
			err := cfg.validateTLS()
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateTLS() error = %v, wantErr %t", err, tc.wantErr)
			}
		})
	}
}

func TestConvertValue(t *testing.T) {
	// Edges returned by falkordb-go's result parser carry their endpoint IDs
	// in unexported fields, so tests populate Source/Destination instead;
	// SourceNodeID/DestNodeID read the exported nodes when they are set.
	node := &falkordbgo.Node{
		ID:         1,
		Labels:     []string{"Person"},
		Properties: map[string]any{"name": "Ada"},
	}
	edge := &falkordbgo.Edge{
		ID:          7,
		Relation:    "KNOWS",
		Source:      &falkordbgo.Node{ID: 1},
		Destination: &falkordbgo.Node{ID: 2},
		Properties:  map[string]any{"since": int64(2020)},
	}

	wantNode := map[string]any{
		"id":         uint64(1),
		"labels":     []string{"Person"},
		"properties": map[string]any{"name": "Ada"},
	}
	wantEdge := map[string]any{
		"id":            uint64(7),
		"type":          "KNOWS",
		"sourceId":      uint64(1),
		"destinationId": uint64(2),
		"properties":    map[string]any{"since": int64(2020)},
	}

	tcs := []struct {
		desc string
		in   any
		want any
	}{
		{desc: "nil", in: nil, want: nil},
		{desc: "nil node pointer", in: (*falkordbgo.Node)(nil), want: nil},
		{desc: "nil edge pointer", in: (*falkordbgo.Edge)(nil), want: nil},
		{desc: "string passes through", in: "hello", want: "hello"},
		{desc: "int64 passes through", in: int64(42), want: int64(42)},
		{desc: "float64 passes through", in: 1.5, want: 1.5},
		{desc: "bool passes through", in: true, want: true},
		{desc: "node pointer", in: node, want: wantNode},
		{desc: "node value", in: *node, want: wantNode},
		{desc: "edge pointer", in: edge, want: wantEdge},
		{desc: "edge value", in: *edge, want: wantEdge},
		{
			desc: "node without properties yields an empty map",
			in:   &falkordbgo.Node{ID: 3, Labels: []string{"Empty"}},
			want: map[string]any{
				"id":         uint64(3),
				"labels":     []string{"Empty"},
				"properties": map[string]any{},
			},
		},
		{
			desc: "path",
			in: falkordbgo.Path{
				Nodes: []*falkordbgo.Node{node},
				Edges: []*falkordbgo.Edge{edge},
			},
			want: map[string]any{
				"nodes": []any{wantNode},
				"edges": []any{wantEdge},
			},
		},
		{
			desc: "list is converted element-wise",
			in:   []any{int64(1), node},
			want: []any{int64(1), wantNode},
		},
		{
			desc: "map is converted value-wise",
			in:   map[string]any{"n": node, "count": int64(2)},
			want: map[string]any{"n": wantNode, "count": int64(2)},
		},
		{
			desc: "nested list inside map is converted recursively",
			in:   map[string]any{"rows": []any{map[string]any{"n": node}}},
			want: map[string]any{"rows": []any{map[string]any{"n": wantNode}}},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got := ConvertValue(tc.in)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect conversion (-want +got):\n%s", diff)
			}
		})
	}
}
