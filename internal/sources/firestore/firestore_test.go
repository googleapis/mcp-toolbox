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

package firestore

import (
	"context"
	"testing"
	"time"

	firestoreapi "cloud.google.com/go/firestore"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"google.golang.org/genproto/googleapis/type/latlng"
)

func TestParseFromYamlFirestore(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example with default database",
			in: `
			kind: source
			name: my-firestore
			type: firestore
			project: my-project
			`,
			want: map[string]sources.SourceConfig{
				"my-firestore": Config{
					Name:     "my-firestore",
					Type:     SourceType,
					Project:  "my-project",
					Database: "",
				},
			},
		},
		{
			desc: "with custom database",
			in: `
			kind: source
			name: my-firestore
			type: firestore
			project: my-project
			database: my-database
			`,
			want: map[string]sources.SourceConfig{
				"my-firestore": Config{
					Name:     "my-firestore",
					Type:     SourceType,
					Project:  "my-project",
					Database: "my-database",
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
			name: my-firestore
			type: firestore
			project: my-project
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-firestore\" as \"firestore\": [1:1] unknown field \"foo\"\n>  1 | foo: bar\n       ^\n   2 | name: my-firestore\n   3 | project: my-project\n   4 | type: firestore",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-firestore
			type: firestore
			`,
			err: "error unmarshaling source: unable to parse source \"my-firestore\" as \"firestore\": Key: 'Config.Project' Error:Field validation for 'Project' failed on the 'required' tag",
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

func TestFirestoreValueToJSON_RoundTrip(t *testing.T) {
	// Test round-trip conversion
	original := map[string]any{
		"name":   "Test",
		"count":  int64(42),
		"price":  19.99,
		"active": true,
		"tags":   []any{"tag1", "tag2"},
		"metadata": map[string]any{
			"created": time.Now(),
		},
		"nullField": nil,
	}

	// Convert to JSON representation
	jsonRepresentation := FirestoreValueToJSON(original)

	// Verify types are simplified
	jsonMap, ok := jsonRepresentation.(map[string]any)
	if !ok {
		t.Fatalf("Expected map, got %T", jsonRepresentation)
	}

	// Time should be converted to string
	metadata, ok := jsonMap["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata should be a map, got %T", jsonMap["metadata"])
	}
	_, ok = metadata["created"].(string)
	if !ok {
		t.Errorf("created should be a string, got %T", metadata["created"])
	}
}

func TestGetDatabaseId(t *testing.T) {
	t.Run("default database id", func(t *testing.T) {
		s := &Source{
			Config: Config{
				Project:  "my-project",
				Database: "",
			},
		}
		if got := s.GetDatabaseId(); got != "(default)" {
			t.Errorf("GetDatabaseId() = %q, want %q", got, "(default)")
		}
	})

	t.Run("custom database id", func(t *testing.T) {
		s := &Source{
			Config: Config{
				Project:  "my-project",
				Database: "custom-db",
			},
		}
		if got := s.GetDatabaseId(); got != "custom-db" {
			t.Errorf("GetDatabaseId() = %q, want %q", got, "custom-db")
		}
	})
}

func TestGetTypeName(t *testing.T) {
	testCases := []struct {
		in   any
		want string
	}{
		{in: nil, want: "null"},
		{in: "hello", want: "string"},
		{in: true, want: "boolean"},
		{in: int(10), want: "integer"},
		{in: int8(10), want: "integer"},
		{in: int16(10), want: "integer"},
		{in: int32(10), want: "integer"},
		{in: int64(10), want: "integer"},
		{in: uint(10), want: "integer"},
		{in: uint8(10), want: "integer"},
		{in: uint16(10), want: "integer"},
		{in: uint32(10), want: "integer"},
		{in: uint64(10), want: "integer"},
		{in: float32(3.14), want: "double"},
		{in: float64(3.14), want: "double"},
		{in: time.Now(), want: "timestamp"},
		{in: map[string]any{"a": 1}, want: "map"},
		{in: []any{"a", "b"}, want: "array"},
		{in: &latlng.LatLng{Latitude: 10, Longitude: 20}, want: "geopoint"},
		{in: &firestoreapi.DocumentRef{}, want: "reference"},
		{in: []byte{1, 2, 3}, want: "bytes"},
		{in: struct{}{}, want: "struct {}"},
	}

	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			got := getTypeName(tc.in)
			if got != tc.want {
				t.Errorf("getTypeName(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFlattenFieldsFromMap(t *testing.T) {
	t.Run("simple fields with stringValue", func(t *testing.T) {
		input := map[string]any{
			"name": map[string]any{"stringValue": "string"},
			"age":  map[string]any{"stringValue": "long"},
		}
		got := flattenFieldsFromMap("", input)
		want := []FieldSchema{
			{Name: "name", Types: []string{"string"}},
			{Name: "age", Types: []string{"long"}},
		}
		if len(got) != len(want) {
			t.Fatalf("len(got) = %d, want %d", len(got), len(want))
		}
	})

	t.Run("nested mapValue fields", func(t *testing.T) {
		input := map[string]any{
			"address": map[string]any{
				"mapValue": map[string]any{
					"fields": map[string]any{
						"city": map[string]any{"stringValue": "string"},
						"zip":  map[string]any{"stringValue": "int"},
					},
				},
			},
		}
		got := flattenFieldsFromMap("", input)
		want := []FieldSchema{
			{Name: "address.city", Types: []string{"string"}},
			{Name: "address.zip", Types: []string{"int"}},
		}
		if len(got) != len(want) {
			t.Fatalf("len(got) = %d, want %d", len(got), len(want))
		}
	})

	t.Run("plain string type mapping", func(t *testing.T) {
		input := map[string]any{
			"status": "string",
		}
		got := flattenFieldsFromMap("", input)
		want := []FieldSchema{
			{Name: "status", Types: []string{"string"}},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("unexpected result: diff %v", diff)
		}
	})

	t.Run("generic Value suffix trimming", func(t *testing.T) {
		input := map[string]any{
			"active": map[string]any{"booleanValue": true},
		}
		got := flattenFieldsFromMap("", input)
		want := []FieldSchema{
			{Name: "active", Types: []string{"boolean"}},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("unexpected result: diff %v", diff)
		}
	})
}

func TestParseFieldsFromPipelineResponse(t *testing.T) {
	t.Run("array with result.fields", func(t *testing.T) {
		raw := []any{
			map[string]any{
				"result": map[string]any{
					"fields": map[string]any{
						"title": map[string]any{"stringValue": "string"},
					},
				},
			},
		}
		got := parseFieldsFromPipelineResponse(raw)
		want := []FieldSchema{
			{Name: "title", Types: []string{"string"}},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("unexpected result: diff %v", diff)
		}
	})

	t.Run("array with document.fields", func(t *testing.T) {
		raw := []any{
			map[string]any{
				"document": map[string]any{
					"fields": map[string]any{
						"email": map[string]any{"stringValue": "string"},
					},
				},
			},
		}
		got := parseFieldsFromPipelineResponse(raw)
		want := []FieldSchema{
			{Name: "email", Types: []string{"string"}},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("unexpected result: diff %v", diff)
		}
	})

	t.Run("array with top-level fields", func(t *testing.T) {
		raw := []any{
			map[string]any{
				"fields": map[string]any{
					"score": map[string]any{"stringValue": "double"},
				},
			},
		}
		got := parseFieldsFromPipelineResponse(raw)
		want := []FieldSchema{
			{Name: "score", Types: []string{"double"}},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("unexpected result: diff %v", diff)
		}
	})

	t.Run("map with results array", func(t *testing.T) {
		raw := map[string]any{
			"results": []any{
				map[string]any{
					"fields": map[string]any{
						"item": map[string]any{"stringValue": "string"},
					},
				},
			},
		}
		got := parseFieldsFromPipelineResponse(raw)
		want := []FieldSchema{
			{Name: "item", Types: []string{"string"}},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("unexpected result: diff %v", diff)
		}
	})

	t.Run("map with direct fields", func(t *testing.T) {
		raw := map[string]any{
			"fields": map[string]any{
				"category": map[string]any{"stringValue": "string"},
			},
		}
		got := parseFieldsFromPipelineResponse(raw)
		want := []FieldSchema{
			{Name: "category", Types: []string{"string"}},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("unexpected result: diff %v", diff)
		}
	})
}

func TestExtractFieldTypes(t *testing.T) {
	data := map[string]any{
		"username": "alice",
		"age":      30,
		"profile": map[string]any{
			"bio":     "developer",
			"website": "https://example.com",
		},
	}
	fieldsMap := make(map[string]map[string]bool)

	extractFieldTypes("", data, fieldsMap)

	if !fieldsMap["username"]["string"] {
		t.Errorf("expected fieldsMap[username] to have 'string'")
	}
	if !fieldsMap["age"]["integer"] {
		t.Errorf("expected fieldsMap[age] to have 'integer'")
	}
	if !fieldsMap["profile.bio"]["string"] {
		t.Errorf("expected fieldsMap[profile.bio] to have 'string'")
	}
	if !fieldsMap["profile.website"]["string"] {
		t.Errorf("expected fieldsMap[profile.website] to have 'string'")
	}
}
