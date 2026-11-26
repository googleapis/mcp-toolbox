// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mongodblistcollections_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/mongodb/mongodbcommon"
	"github.com/googleapis/mcp-toolbox/internal/tools/mongodb/mongodblistcollections"
)

func TestParseFromYamlMongoListCollections(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	in := `
	        kind: tool
	        name: list_collections
	        type: mongodb-list-collections
	        source: my-instance
	        description: List the collections available in the database.
	        database: test_db
	`
	want := server.ToolConfigs{
		"list_collections": mongodblistcollections.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "list_collections",
				AuthRequired: []string{},
				Description:  "List the collections available in the database.",
			},
			Type:     "mongodb-list-collections",
			Source:   "my-instance",
			Database: "test_db",
		},
	}
	_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(in))
	if err != nil {
		t.Fatalf("unable to unmarshal: %s", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("incorrect parse: diff %v", diff)
	}
}

func TestInitialize(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	t.Run("description is required", func(t *testing.T) {
		cfg := mongodblistcollections.Config{
			ConfigBase: tools.ConfigBase{Name: "list_collections"},
			Database:   "test_db",
		}
		if _, err := cfg.Initialize(ctx); err == nil {
			t.Fatal("expected an error when description is missing")
		}
	})

	cfg := mongodblistcollections.Config{
		ConfigBase: tools.ConfigBase{Name: "list_collections", Description: "some description"},
		Type:       "mongodb-list-collections",
		Source:     "my-instance",
		Database:   "test_db",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unable to initialize tool: %s", err)
	}

	t.Run("exposes no parameters", func(t *testing.T) {
		params, err := tool.GetParameters(&mongodbcommon.MockSource{})
		if err != nil {
			t.Fatalf("unable to get parameters: %s", err)
		}
		if len(params) != 0 {
			t.Errorf("expected no parameters, got %d", len(params))
		}
	})

	t.Run("is annotated read-only", func(t *testing.T) {
		annotations := tool.GetAnnotations(&mongodbcommon.MockSource{})
		if annotations.ReadOnlyHint == nil || !*annotations.ReadOnlyHint {
			t.Error("expected readOnlyHint to be true")
		}
	})

	t.Run("rejects an incompatible source", func(t *testing.T) {
		if err := tool.ValidateSource(nil); err == nil {
			t.Error("expected an error for an incompatible source")
		}
	})

	validateTcs := []struct {
		desc    string
		allowed map[string][]string
		wantErr bool
	}{
		{"unrestricted source", nil, false},
		{"source allows collections in the database", map[string][]string{"test_db": {"orders"}}, false},
		{"source allows nothing in the database", map[string][]string{"other_db": {"orders"}}, true},
	}
	for _, tc := range validateTcs {
		t.Run(tc.desc, func(t *testing.T) {
			err := tool.ValidateSource(&mongodbcommon.MockSource{AllowedCollections: tc.allowed})
			if tc.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
		})
	}
}
