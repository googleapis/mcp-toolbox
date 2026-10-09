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

package helpers

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/tools/neo4j/neo4jschema/types"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

func TestHelperFunctions(t *testing.T) {
	t.Run("ConvertToStringSlice", func(t *testing.T) {
		tests := []struct {
			name  string
			input []any
			want  []string
		}{
			{
				name:  "empty slice",
				input: []any{},
				want:  []string{},
			},
			{
				name:  "string values",
				input: []any{"a", "b", "c"},
				want:  []string{"a", "b", "c"},
			},
			{
				name:  "mixed types",
				input: []any{"string", 123, true, 45.67},
				want:  []string{"string", "123", "true", "45.67"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := ConvertToStringSlice(tt.input)
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("ConvertToStringSlice() mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})

	t.Run("GetStringValue", func(t *testing.T) {
		tests := []struct {
			name  string
			input any
			want  string
		}{
			{
				name:  "nil value",
				input: nil,
				want:  "",
			},
			{
				name:  "string value",
				input: "test",
				want:  "test",
			},
			{
				name:  "int value",
				input: 42,
				want:  "42",
			},
			{
				name:  "bool value",
				input: true,
				want:  "true",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := GetStringValue(tt.input)
				if got != tt.want {
					t.Errorf("GetStringValue() got %q, want %q", got, tt.want)
				}
			})
		}
	})
}

func TestMapToAPOCSchema(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]any
		want    *types.APOCSchemaResult
		wantErr bool
	}{
		{
			name: "simple node schema",
			input: map[string]any{
				"Person": map[string]any{
					"type":  "node",
					"count": int64(150),
					"properties": map[string]any{
						"name": map[string]any{
							"type":      "STRING",
							"unique":    false,
							"indexed":   true,
							"existence": false,
						},
					},
				},
			},
			want: &types.APOCSchemaResult{
				Value: map[string]types.APOCEntity{
					"Person": {
						Type:  "node",
						Count: 150,
						Properties: map[string]types.APOCProperty{
							"name": {
								Type:      "STRING",
								Unique:    false,
								Indexed:   true,
								Existence: false,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "empty input",
			input:   map[string]any{},
			want:    &types.APOCSchemaResult{Value: map[string]types.APOCEntity{}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MapToAPOCSchema(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapToAPOCSchema() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("MapToAPOCSchema() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestProcessAPOCSchema(t *testing.T) {
	tests := []struct {
		name          string
		input         *types.APOCSchemaResult
		wantNodes     []types.NodeLabel
		wantRels      []types.Relationship
		wantStats     *types.Statistics
		statsAreEmpty bool
	}{
		{
			name: "empty schema",
			input: &types.APOCSchemaResult{
				Value: map[string]types.APOCEntity{},
			},
			wantNodes:     nil,
			wantRels:      nil,
			statsAreEmpty: true,
		},
		{
			name: "simple node only",
			input: &types.APOCSchemaResult{
				Value: map[string]types.APOCEntity{
					"Person": {
						Type:  "node",
						Count: 100,
						Properties: map[string]types.APOCProperty{
							"name": {Type: "STRING", Indexed: true},
							"age":  {Type: "INTEGER"},
						},
					},
				},
			},
			wantNodes: []types.NodeLabel{
				{
					Name:  "Person",
					Count: 100,
					Properties: []types.PropertyInfo{
						{Name: "age", Types: []string{"INTEGER"}},
						{Name: "name", Types: []string{"STRING"}, Indexed: true},
					},
				},
			},
			wantRels: nil,
			wantStats: &types.Statistics{
				NodesByLabel:      map[string]int64{"Person": 100},
				PropertiesByLabel: map[string]int64{"Person": 2},
				TotalNodes:        100,
				TotalProperties:   200,
			},
		},
		{
			name: "nodes and relationships",
			input: &types.APOCSchemaResult{
				Value: map[string]types.APOCEntity{
					"Person": {
						Type:  "node",
						Count: 100,
						Properties: map[string]types.APOCProperty{
							"name": {Type: "STRING", Unique: true, Indexed: true, Existence: true},
						},
						Relationships: map[string]types.APOCRelationshipInfo{
							"KNOWS": {
								Direction: "out",
								Count:     50,
								Labels:    []string{"Person"},
								Properties: map[string]types.APOCProperty{
									"since": {Type: "INTEGER"},
								},
							},
						},
					},
					"Post": {
						Type:       "node",
						Count:      200,
						Properties: map[string]types.APOCProperty{"content": {Type: "STRING"}},
					},
					"FOLLOWS": {Type: "relationship", Count: 80},
				},
			},
			wantNodes: []types.NodeLabel{
				{
					Name:  "Post",
					Count: 200,
					Properties: []types.PropertyInfo{
						{Name: "content", Types: []string{"STRING"}},
					},
				},
				{
					Name:  "Person",
					Count: 100,
					Properties: []types.PropertyInfo{
						{Name: "name", Types: []string{"STRING"}, Unique: true, Indexed: true, Mandatory: true},
					},
				},
			},
			wantRels: []types.Relationship{
				{
					Type:      "KNOWS",
					StartNode: "Person",
					EndNode:   "Person",
					Count:     50,
					Properties: []types.PropertyInfo{
						{Name: "since", Types: []string{"INTEGER"}},
					},
				},
			},
			wantStats: &types.Statistics{
				NodesByLabel:        map[string]int64{"Person": 100, "Post": 200},
				RelationshipsByType: map[string]int64{"KNOWS": 50},
				PropertiesByLabel:   map[string]int64{"Person": 1, "Post": 1},
				PropertiesByRelType: map[string]int64{"KNOWS": 1},
				TotalNodes:          300,
				TotalRelationships:  50,
				TotalProperties:     350, // (100*1 + 200*1) for nodes + (50*1) for rels
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNodes, gotRels, gotStats := ProcessAPOCSchema(tt.input)

			if diff := cmp.Diff(tt.wantNodes, gotNodes); diff != "" {
				t.Errorf("ProcessAPOCSchema() node labels mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantRels, gotRels); diff != "" {
				t.Errorf("ProcessAPOCSchema() relationships mismatch (-want +got):\n%s", diff)
			}
			if tt.statsAreEmpty {
				tt.wantStats = &types.Statistics{}
			}

			if diff := cmp.Diff(tt.wantStats, gotStats); diff != "" {
				t.Errorf("ProcessAPOCSchema() statistics mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestProcessNonAPOCSchema(t *testing.T) {
	t.Run("full schema processing", func(t *testing.T) {
		nodeCounts := map[string]int64{"Person": 10, "City": 5}
		nodePropsMap := map[string]map[string]map[string]bool{
			"Person": {"name": {"STRING": true}, "age": {"INTEGER": true}},
			"City":   {"name": {"STRING": true, "TEXT": true}},
		}
		relCounts := map[string]int64{"LIVES_IN": 8}
		relPropsMap := map[string]map[string]map[string]bool{
			"LIVES_IN": {"since": {"DATE": true}},
		}
		relConnectivity := map[string]types.RelConnectivityInfo{
			"LIVES_IN": {StartNode: "Person", EndNode: "City", Count: 8},
		}

		wantNodes := []types.NodeLabel{
			{
				Name:  "Person",
				Count: 10,
				Properties: []types.PropertyInfo{
					{Name: "age", Types: []string{"INTEGER"}},
					{Name: "name", Types: []string{"STRING"}},
				},
			},
			{
				Name:  "City",
				Count: 5,
				Properties: []types.PropertyInfo{
					{Name: "name", Types: []string{"STRING", "TEXT"}},
				},
			},
		}
		wantRels := []types.Relationship{
			{
				Type:      "LIVES_IN",
				Count:     8,
				StartNode: "Person",
				EndNode:   "City",
				Properties: []types.PropertyInfo{
					{Name: "since", Types: []string{"DATE"}},
				},
			},
		}
		wantStats := &types.Statistics{
			TotalNodes:          15,
			TotalRelationships:  8,
			TotalProperties:     33, // (10*2 + 5*1) for nodes + (8*1) for rels
			NodesByLabel:        map[string]int64{"Person": 10, "City": 5},
			RelationshipsByType: map[string]int64{"LIVES_IN": 8},
			PropertiesByLabel:   map[string]int64{"Person": 2, "City": 1},
			PropertiesByRelType: map[string]int64{"LIVES_IN": 1},
		}

		gotNodes, gotRels, gotStats := ProcessNonAPOCSchema(nodeCounts, nodePropsMap, relCounts, relPropsMap, relConnectivity)

		if diff := cmp.Diff(wantNodes, gotNodes); diff != "" {
			t.Errorf("ProcessNonAPOCSchema() nodes mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(wantRels, gotRels); diff != "" {
			t.Errorf("ProcessNonAPOCSchema() relationships mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(wantStats, gotStats); diff != "" {
			t.Errorf("ProcessNonAPOCSchema() stats mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("empty schema", func(t *testing.T) {
		gotNodes, gotRels, gotStats := ProcessNonAPOCSchema(
			map[string]int64{},
			map[string]map[string]map[string]bool{},
			map[string]int64{},
			map[string]map[string]map[string]bool{},
			map[string]types.RelConnectivityInfo{},
		)

		if len(gotNodes) != 0 {
			t.Errorf("expected 0 nodes, got %d", len(gotNodes))
		}
		if len(gotRels) != 0 {
			t.Errorf("expected 0 relationships, got %d", len(gotRels))
		}
		if diff := cmp.Diff(&types.Statistics{}, gotStats); diff != "" {
			t.Errorf("ProcessNonAPOCSchema() stats mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestConvertValue(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  any
	}{
		{
			name:  "nil value",
			input: nil,
			want:  nil,
		},
		{
			name:  "neo4j.InvalidValue",
			input: neo4j.InvalidValue{},
			want:  nil,
		},
		{
			name:  "primitive bool",
			input: true,
			want:  true,
		},
		{
			name:  "primitive int",
			input: int64(42),
			want:  int64(42),
		},
		{
			name:  "primitive float",
			input: 3.14,
			want:  3.14,
		},
		{
			name:  "primitive string",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "neo4j.Date",
			input: neo4j.Date(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)),
			want:  "2024-06-01",
		},
		{
			name:  "neo4j.LocalTime",
			input: neo4j.LocalTime(time.Date(0, 0, 0, 12, 34, 56, 0, time.Local)),
			want:  "12:34:56",
		},
		{
			name:  "neo4j.Time",
			input: neo4j.Time(time.Date(0, 0, 0, 1, 2, 3, 0, time.UTC)),
			want:  "01:02:03Z",
		},
		{
			name:  "neo4j.LocalDateTime",
			input: neo4j.LocalDateTime(time.Date(2024, 6, 1, 10, 20, 30, 0, time.Local)),
			want:  "2024-06-01T10:20:30",
		},
		{
			name:  "neo4j.Duration",
			input: neo4j.Duration{Months: 1, Days: 2, Seconds: 3, Nanos: 4},
			want:  "P1M2DT3.000000004S",
		},
		{
			name:  "neo4j.Point2D",
			input: neo4j.Point2D{X: 1.1, Y: 2.2, SpatialRefId: 1234},
			want:  map[string]any{"x": 1.1, "y": 2.2, "srid": uint32(1234)},
		},
		{
			name:  "neo4j.Point3D",
			input: neo4j.Point3D{X: 1.1, Y: 2.2, Z: 3.3, SpatialRefId: 5467},
			want:  map[string]any{"x": 1.1, "y": 2.2, "z": 3.3, "srid": uint32(5467)},
		},
		{
			name: "neo4j.Node",
			input: neo4j.Node{
				ElementId: "element-1",
				Labels:    []string{"Person"},
				Props:     map[string]any{"name": "Alice"},
			},
			want: map[string]any{
				"elementId":  "element-1",
				"labels":     []string{"Person"},
				"properties": map[string]any{"name": "Alice"},
			},
		},
		{
			name: "neo4j.Relationship",
			input: neo4j.Relationship{
				ElementId:      "element-2",
				StartElementId: "start-1",
				EndElementId:   "end-1",
				Type:           "KNOWS",
				Props:          map[string]any{"since": 2024},
			},
			want: map[string]any{
				"elementId":      "element-2",
				"properties":     map[string]any{"since": 2024},
				"startElementId": "start-1",
				"endElementId":   "end-1",
				"type":           "KNOWS",
			},
		},
		{
			name: "neo4j.Path",
			input: func() neo4j.Path {
				node1 := neo4j.Node{ElementId: "n10", Labels: []string{"A"}, Props: map[string]any{"p1": "v1"}}
				node2 := neo4j.Node{ElementId: "n11", Labels: []string{"B"}, Props: map[string]any{"p2": "v2"}}
				rel1 := neo4j.Relationship{ElementId: "r12", StartElementId: "n10", EndElementId: "n11", Type: "REL", Props: map[string]any{"p3": "v3"}}
				return neo4j.Path{
					Nodes:         []neo4j.Node{node1, node2},
					Relationships: []neo4j.Relationship{rel1},
				}
			}(),
			want: map[string]any{
				"nodes": []any{
					map[string]any{
						"elementId":  "n10",
						"properties": map[string]any{"p1": "v1"},
						"labels":     []string{"A"},
					},
					map[string]any{
						"elementId":  "n11",
						"properties": map[string]any{"p2": "v2"},
						"labels":     []string{"B"},
					},
				},
				"relationships": []any{
					map[string]any{
						"elementId":      "r12",
						"properties":     map[string]any{"p3": "v3"},
						"startElementId": "n10",
						"endElementId":   "n11",
						"type":           "REL",
					},
				},
			},
		},
		{
			name:  "slice of primitives",
			input: []any{"a", 1, true},
			want:  []any{"a", 1, true},
		},
		{
			name:  "slice of mixed types",
			input: []any{"a", neo4j.Date(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))},
			want:  []any{"a", "2024-06-01"},
		},
		{
			name:  "map of primitives",
			input: map[string]any{"foo": 1, "bar": "baz"},
			want:  map[string]any{"foo": 1, "bar": "baz"},
		},
		{
			name:  "map with nested neo4j type",
			input: map[string]any{"date": neo4j.Date(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))},
			want:  map[string]any{"date": "2024-06-01"},
		},
		{
			name:  "unhandled type",
			input: struct{ X int }{X: 5},
			want:  "{5}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertValue(tt.input)
			if !cmp.Equal(got, tt.want) {
				t.Errorf("ConvertValue() mismatch (-want +got):\n%s", cmp.Diff(tt.want, got))
			}
		})
	}
}

func TestParseAPOCRelationshipRecord(t *testing.T) {
	tests := []struct {
		name   string
		values []any
		want   types.Relationship
		wantOK bool
	}{
		{
			name:   "well-formed record",
			values: []any{"KNOWS", "Person", "Company", map[string]any{"since": "INTEGER"}, int64(5)},
			want: types.Relationship{
				Type:       "KNOWS",
				StartNode:  "Person",
				EndNode:    "Company",
				Count:      5,
				Properties: []types.PropertyInfo{{Name: "since", Types: []string{"INTEGER"}}},
			},
			wantOK: true,
		},
		{
			// Go map iteration order is unspecified, so several properties must be
			// sorted by name for the extracted schema to be stable across runs.
			name: "multiple properties are sorted by name",
			values: []any{
				"KNOWS", "Person", "Company",
				map[string]any{"since": "INTEGER", "active": "BOOLEAN", "role": "STRING"},
				int64(2),
			},
			want: types.Relationship{
				Type:      "KNOWS",
				StartNode: "Person",
				EndNode:   "Company",
				Count:     2,
				Properties: []types.PropertyInfo{
					{Name: "active", Types: []string{"BOOLEAN"}},
					{Name: "role", Types: []string{"STRING"}},
					{Name: "since", Types: []string{"INTEGER"}},
				},
			},
			wantOK: true,
		},
		{
			// labels(n)[0] is null for a label-less node, so the start/end node
			// value arrives as nil. The previous bare values[i].(string) panicked
			// on it; ParseAPOCRelationshipRecord must instead yield an empty label.
			name:   "label-less node yields nil endpoint, no panic",
			values: []any{"KNOWS", nil, "Company", map[string]any{}, int64(3)},
			want: types.Relationship{
				Type:       "KNOWS",
				StartNode:  "",
				EndNode:    "Company",
				Count:      3,
				Properties: []types.PropertyInfo{},
			},
			wantOK: true,
		},
		{
			name:   "too few values is skipped",
			values: []any{"KNOWS", "Person"},
			wantOK: false,
		},
		{
			name:   "non-int64 count is skipped",
			values: []any{"KNOWS", "Person", "Company", map[string]any{}, "5"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseAPOCRelationshipRecord(tt.values)
			if ok != tt.wantOK {
				t.Fatalf("ParseAPOCRelationshipRecord() ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ParseAPOCRelationshipRecord() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
