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

package helpers

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/tools/falkordb/falkordbschema/types"
)

func TestFirstColumnStrings(t *testing.T) {
	in := any([]map[string]any{
		{"label": "Person"},
		{"label": "City"},
	})
	// The rows are returned in the order FalkorDB produced them.
	want := []string{"Person", "City"}
	if diff := cmp.Diff(want, FirstColumnStrings(in)); diff != "" {
		t.Fatalf("incorrect result: diff %v", diff)
	}
	if got := FirstColumnStrings(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
}

func TestFirstRowInt64(t *testing.T) {
	tcs := []struct {
		desc string
		in   any
		want int64
	}{
		{desc: "int64 count", in: []map[string]any{{"count": int64(42)}}, want: 42},
		{desc: "float count", in: []map[string]any{{"count": float64(7)}}, want: 7},
		{desc: "empty result", in: []map[string]any{}, want: 0},
		{desc: "not rows", in: "unexpected", want: 0},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if got := FirstRowInt64(tc.in); got != tc.want {
				t.Fatalf("incorrect result: got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestTypeName(t *testing.T) {
	tcs := []struct {
		in   any
		want string
	}{
		{in: nil, want: "NULL"},
		{in: true, want: "BOOLEAN"},
		{in: "a", want: "STRING"},
		{in: int64(1), want: "INTEGER"},
		{in: 1.5, want: "FLOAT"},
		{in: []any{1}, want: "LIST"},
		{in: map[string]any{}, want: "MAP"},
		{in: struct{}{}, want: "UNKNOWN"},
	}
	for _, tc := range tcs {
		if got := TypeName(tc.in); got != tc.want {
			t.Fatalf("incorrect type for %v: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMergeAndCollectProperties(t *testing.T) {
	accumulator := make(map[string]map[string]bool)
	MergeProperties(accumulator, map[string]any{"name": "a", "age": int64(3)})
	MergeProperties(accumulator, map[string]any{"name": "b", "age": 1.5})

	want := []types.PropertyInfo{
		{Name: "age", Types: []string{"FLOAT", "INTEGER"}},
		{Name: "name", Types: []string{"STRING"}},
	}
	if diff := cmp.Diff(want, CollectProperties(accumulator)); diff != "" {
		t.Fatalf("incorrect result: diff %v", diff)
	}
}

func TestSortNodeLabelsAndRelationships(t *testing.T) {
	nodeLabels := []types.NodeLabel{
		{Name: "B", Count: 1},
		{Name: "A", Count: 1},
		{Name: "C", Count: 5},
	}
	SortNodeLabels(nodeLabels)
	if nodeLabels[0].Name != "C" || nodeLabels[1].Name != "A" {
		t.Fatalf("incorrect node label order: %v", nodeLabels)
	}

	relationships := []types.Relationship{
		{Type: "Y", Count: 2},
		{Type: "X", Count: 9},
	}
	SortRelationships(relationships)
	if relationships[0].Type != "X" {
		t.Fatalf("incorrect relationship order: %v", relationships)
	}
}
