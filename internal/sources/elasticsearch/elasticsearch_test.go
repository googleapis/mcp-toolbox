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

package elasticsearch_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/elasticsearch"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlElasticsearch(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-es-instance
			type: elasticsearch
			addresses:
				- http://localhost:9200
			apikey: somekey
			`,
			want: map[string]sources.SourceConfig{
				"my-es-instance": elasticsearch.Config{
					Name:      "my-es-instance",
					Type:      elasticsearch.SourceType,
					Addresses: []string{"http://localhost:9200"},
					APIKey:    "somekey",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("failed to parse yaml: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("unexpected config diff (-want +got):\n%s", diff)
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
			name: my-es-instance
			type: elasticsearch
			addresses:
				- http://localhost:9200
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-es-instance\" as \"elasticsearch\": [3:1] unknown field \"foo\"\n   1 | addresses:\n   2 | - http://localhost:9200\n>  3 | foo: bar\n       ^\n   4 | name: my-es-instance\n   5 | type: elasticsearch",
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

func TestTool_esqlToMap(t1 *testing.T) {
	tests := []struct {
		name   string
		result elasticsearch.EsqlResult
		want   []map[string]any
	}{
		{
			name: "simple case with two rows",
			result: elasticsearch.EsqlResult{
				Columns: []elasticsearch.EsqlColumn{
					{Name: "first_name", Type: "text"},
					{Name: "last_name", Type: "text"},
				},
				Values: [][]any{
					{"John", "Doe"},
					{"Jane", "Smith"},
				},
			},
			want: []map[string]any{
				{"first_name": "John", "last_name": "Doe"},
				{"first_name": "Jane", "last_name": "Smith"},
			},
		},
		{
			name: "different data types",
			result: elasticsearch.EsqlResult{
				Columns: []elasticsearch.EsqlColumn{
					{Name: "id", Type: "integer"},
					{Name: "active", Type: "boolean"},
					{Name: "score", Type: "float"},
				},
				Values: [][]any{
					{1, true, 95.5},
					{2, false, 88.0},
				},
			},
			want: []map[string]any{
				{"id": 1, "active": true, "score": 95.5},
				{"id": 2, "active": false, "score": 88.0},
			},
		},
		{
			name: "no rows",
			result: elasticsearch.EsqlResult{
				Columns: []elasticsearch.EsqlColumn{
					{Name: "id", Type: "integer"},
					{Name: "name", Type: "text"},
				},
				Values: [][]any{},
			},
			want: []map[string]any{},
		},
		{
			name: "null values",
			result: elasticsearch.EsqlResult{
				Columns: []elasticsearch.EsqlColumn{
					{Name: "id", Type: "integer"},
					{Name: "name", Type: "text"},
				},
				Values: [][]any{
					{1, nil},
					{2, "Alice"},
				},
			},
			want: []map[string]any{
				{"id": 1, "name": nil},
				{"id": 2, "name": "Alice"},
			},
		},
		{
			name: "missing values in a row",
			result: elasticsearch.EsqlResult{
				Columns: []elasticsearch.EsqlColumn{
					{Name: "id", Type: "integer"},
					{Name: "name", Type: "text"},
					{Name: "age", Type: "integer"},
				},
				Values: [][]any{
					{1, "Bob"},
					{2, "Charlie", 30},
				},
			},
			want: []map[string]any{
				{"id": 1, "name": "Bob", "age": nil},
				{"id": 2, "name": "Charlie", "age": 30},
			},
		},
		{
			name: "all null row",
			result: elasticsearch.EsqlResult{
				Columns: []elasticsearch.EsqlColumn{
					{Name: "id", Type: "integer"},
					{Name: "name", Type: "text"},
				},
				Values: [][]any{
					nil,
				},
			},
			want: []map[string]any{
				{},
			},
		},
		{
			name: "empty columns",
			result: elasticsearch.EsqlResult{
				Columns: []elasticsearch.EsqlColumn{},
				Values: [][]any{
					{},
					{},
				},
			},
			want: []map[string]any{
				{},
				{},
			},
		},
		{
			name: "more values than columns",
			result: elasticsearch.EsqlResult{
				Columns: []elasticsearch.EsqlColumn{
					{Name: "id", Type: "integer"},
				},
				Values: [][]any{
					{1, "extra"},
				},
			},
			want: []map[string]any{
				{"id": 1},
			},
		},
		{
			name: "no columns but with values",
			result: elasticsearch.EsqlResult{
				Columns: []elasticsearch.EsqlColumn{},
				Values: [][]any{
					{1, "data"},
				},
			},
			want: []map[string]any{
				{},
			},
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			if got := elasticsearch.EsqlToMap(tt.result); !reflect.DeepEqual(got, tt.want) {
				t1.Errorf("esqlToMap() = %v, want %v", got, tt.want)
			}
		})
	}
}
