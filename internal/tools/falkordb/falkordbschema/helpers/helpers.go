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

// Package helpers provides utilities to turn raw FalkorDB query results into
// the schema structures defined by the types package.
package helpers

import (
	"sort"

	"github.com/googleapis/mcp-toolbox/internal/tools/falkordb/falkordbschema/types"
)

// Rows casts the result of a source RunQuery call to its row form. A nil or
// non-row result yields an empty slice.
func Rows(result any) []map[string]any {
	rows, ok := result.([]map[string]any)
	if !ok {
		return nil
	}
	return rows
}

// FirstColumnStrings extracts the single string column of a procedure call
// result such as `CALL db.labels()`, regardless of the column's name. The
// order FalkorDB returned the rows in is preserved; callers that need a
// particular order sort the values they derive from it.
func FirstColumnStrings(result any) []string {
	var out []string
	for _, row := range Rows(result) {
		for _, value := range row {
			if s, ok := value.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// FirstRowInt64 extracts a count value from a single-row, single-column
// result such as `MATCH (n) RETURN count(n)`.
func FirstRowInt64(result any) int64 {
	rows := Rows(result)
	if len(rows) == 0 {
		return 0
	}
	for _, value := range rows[0] {
		switch v := value.(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v)
		}
	}
	return 0
}

// TypeName maps a JSON-compatible value produced by the falkordb source to a
// Cypher-style type name for schema output.
func TypeName(value any) string {
	switch value.(type) {
	case nil:
		return "NULL"
	case bool:
		return "BOOLEAN"
	case string:
		return "STRING"
	case int, int32, int64:
		return "INTEGER"
	case float32, float64:
		return "FLOAT"
	case []any:
		return "LIST"
	case map[string]any:
		return "MAP"
	default:
		return "UNKNOWN"
	}
}

// MergeProperties records the observed types of each property of a sampled
// entity into the accumulator map (property name -> set of type names).
func MergeProperties(accumulator map[string]map[string]bool, properties map[string]any) {
	for name, value := range properties {
		if accumulator[name] == nil {
			accumulator[name] = make(map[string]bool)
		}
		accumulator[name][TypeName(value)] = true
	}
}

// CollectProperties converts a property-type accumulator into a sorted
// PropertyInfo slice.
func CollectProperties(accumulator map[string]map[string]bool) []types.PropertyInfo {
	properties := make([]types.PropertyInfo, 0, len(accumulator))
	for name, typeSet := range accumulator {
		typeList := make([]string, 0, len(typeSet))
		for typeName := range typeSet {
			typeList = append(typeList, typeName)
		}
		sort.Strings(typeList)
		properties = append(properties, types.PropertyInfo{Name: name, Types: typeList})
	}
	sort.Slice(properties, func(i, j int) bool { return properties[i].Name < properties[j].Name })
	return properties
}

// SortNodeLabels orders node labels by count (descending) then name.
func SortNodeLabels(nodeLabels []types.NodeLabel) {
	sort.Slice(nodeLabels, func(i, j int) bool {
		if nodeLabels[i].Count != nodeLabels[j].Count {
			return nodeLabels[i].Count > nodeLabels[j].Count
		}
		return nodeLabels[i].Name < nodeLabels[j].Name
	})
}

// SortRelationships orders relationships by count (descending) then type.
func SortRelationships(relationships []types.Relationship) {
	sort.Slice(relationships, func(i, j int) bool {
		if relationships[i].Count != relationships[j].Count {
			return relationships[i].Count > relationships[j].Count
		}
		return relationships[i].Type < relationships[j].Type
	})
}
