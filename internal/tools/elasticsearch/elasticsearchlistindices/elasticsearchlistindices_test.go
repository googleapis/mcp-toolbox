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

package elasticsearchlistindices

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

func TestParseFromYamlElasticsearchListIndices(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc    string
		in      string
		want    server.ToolConfigs
		wantErr bool
	}{
		{
			desc: "basic list indices tool example",
			in: `
            kind: tool
            name: example_tool
            type: elasticsearch-list-indices
            source: my-elasticsearch-instance
            description: Elasticsearch list indices tool
		`,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "Elasticsearch list indices tool",
						AuthRequired: []string{},
					},
					Type:   "elasticsearch-list-indices",
					Source: "my-elasticsearch-instance",
				},
			},
		},
		{
			desc: "missing source fails",
			in: `
            kind: tool
            name: example_tool
            type: elasticsearch-list-indices
            description: Elasticsearch list indices tool
		`,
			wantErr: true,
		},
		{
			desc: "unknown field fails",
			in: `
            kind: tool
            name: example_tool
            type: elasticsearch-list-indices
            source: my-elasticsearch-instance
            description: Elasticsearch list indices tool
            format: csv
		`,
			wantErr: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}

func TestSplitPatterns(t *testing.T) {
	tcs := []struct {
		desc    string
		pattern string
		want    []string
	}{
		{"empty", "", []string{"*"}},
		{"whitespace", "  ", []string{"*"}},
		{"single", "people", []string{"people"}},
		{"two", "a,b", []string{"a", "b"}},
		{"trimmed with trailing comma", " a , b* ,", []string{"a", "b*"}},
		{"only commas", ",,", []string{"*"}},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, splitPatterns(tc.pattern)); diff != "" {
				t.Fatalf("splitPatterns(%q) diff: %v", tc.pattern, diff)
			}
		})
	}
}

func TestIncludeIndex(t *testing.T) {
	tcs := []struct {
		desc     string
		patterns []string
		name     string
		want     bool
	}{
		{"normal index with wildcard", []string{"*"}, "people", true},
		{"dot index with wildcard", []string{"*"}, ".internal.alerts-default", false},
		{"dot index with normal pattern", []string{"logs-*"}, ".hidden", false},
		{"dot index with dot pattern", []string{".kibana*"}, ".kibana_1", true},
		{"normal index with dot pattern", []string{".kibana*"}, "people", true},
		{"dot index with exact dot name", []string{".security-7"}, ".security-7", true},
		{"dot index kept by one dot pattern in list", []string{"people", ".kibana*"}, ".kibana_1", true},
		{"normal index kept with dot pattern in list", []string{"people", ".kibana*"}, "people", true},
		{"dot index dropped without dot pattern in list", []string{"people", "logs-*"}, ".kibana_1", false},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if got := includeIndex(tc.patterns, tc.name); got != tc.want {
				t.Fatalf("includeIndex(%q, %q) = %v, want %v", tc.patterns, tc.name, got, tc.want)
			}
		})
	}
}

func TestColumnsFor(t *testing.T) {
	tcs := []struct {
		desc    string
		format  string
		wantCat []string
		wantKey []string
		wantOk  bool
	}{
		{
			desc:    "simple",
			format:  "simple",
			wantCat: []string{"index", "health", "status", "docs.count", "store.size"},
			wantKey: []string{"index", "health", "status", "docs_count", "store_size"},
			wantOk:  true,
		},
		{
			desc:    "detailed",
			format:  "detailed",
			wantCat: []string{"index", "health", "status", "docs.count", "store.size", "pri", "rep", "docs.deleted", "creation.date.string", "uuid"},
			wantKey: []string{"index", "health", "status", "docs_count", "store_size", "pri", "rep", "docs_deleted", "creation_date", "uuid"},
			wantOk:  true,
		},
		{desc: "invalid", format: "bad"},
		{desc: "empty", format: ""},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cols, cats, ok := columnsFor(tc.format)
			if ok != tc.wantOk {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOk)
			}
			if !ok {
				return
			}
			var keys []string
			for _, c := range cols {
				keys = append(keys, c.key)
			}
			if !reflect.DeepEqual(cats, tc.wantCat) {
				t.Fatalf("cat columns = %v, want %v", cats, tc.wantCat)
			}
			if !reflect.DeepEqual(keys, tc.wantKey) {
				t.Fatalf("keys = %v, want %v", keys, tc.wantKey)
			}
		})
	}
}
