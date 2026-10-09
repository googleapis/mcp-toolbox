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
	"context"
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/mongodb/mongodblistcollections"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

type fakeSource struct {
	database string
}

func (s *fakeSource) SourceType() string {
	return "mongodb"
}

func (s *fakeSource) ToConfig() sources.SourceConfig {
	return nil
}

func (s *fakeSource) ListCollections(_ context.Context, database string) ([]string, error) {
	s.database = database
	return []string{"customers", "orders"}, nil
}

type nilCollectionsSource struct{}

func (s *nilCollectionsSource) SourceType() string {
	return "mongodb"
}

func (s *nilCollectionsSource) ToConfig() sources.SourceConfig {
	return nil
}

func (s *nilCollectionsSource) ListCollections(context.Context, string) ([]string, error) {
	return nil, nil
}

func TestParseFromYamlMongoDBListCollections(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	in := `
kind: tool
name: list_collections
type: mongodb-list-collections
source: my-mongo-source
database: app
description: List collections in MongoDB
`
	want := server.ToolConfigs{
		"list_collections": mongodblistcollections.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "list_collections",
				Description:  "List collections in MongoDB",
				AuthRequired: []string{},
			},
			Type:     "mongodb-list-collections",
			Source:   "my-mongo-source",
			Database: "app",
		},
	}

	_, _, _, got, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(in))
	if err != nil {
		t.Fatalf("unable to unmarshal: %s", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("incorrect parse: diff %v", diff)
	}
}

func TestInvokeListsConfiguredDatabaseCollections(t *testing.T) {
	ctx := context.Background()
	cfg := mongodblistcollections.Config{
		ConfigBase: tools.ConfigBase{Name: "list_collections", Description: "List collections"},
		Type:       "mongodb-list-collections",
		Source:     "mongo",
		Database:   "app",
	}

	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unable to initialize tool: %s", err)
	}
	params, err := tool.GetParameters(nil)
	if err != nil {
		t.Fatalf("unable to get parameters: %s", err)
	}
	if len(params) != 0 {
		t.Fatalf("expected no invocation parameters, got %d", len(params))
	}

	source := &fakeSource{}
	got, tbErr := tool.Invoke(ctx, source, parameters.ParamValues{}, "")
	if tbErr != nil {
		t.Fatalf("unexpected invocation error: %s", tbErr)
	}
	if diff := cmp.Diff([]string{"customers", "orders"}, got); diff != "" {
		t.Fatalf("unexpected collections (-want +got):\n%s", diff)
	}
	if source.database != "app" {
		t.Fatalf("expected database %q, got %q", "app", source.database)
	}
}

func TestInvokeNormalizesNilCollectionList(t *testing.T) {
	ctx := context.Background()
	cfg := mongodblistcollections.Config{
		ConfigBase: tools.ConfigBase{Name: "list_collections", Description: "List collections"},
		Type:       "mongodb-list-collections",
		Source:     "mongo",
		Database:   "app",
	}

	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unable to initialize tool: %s", err)
	}

	got, tbErr := tool.Invoke(ctx, &nilCollectionsSource{}, parameters.ParamValues{}, "")
	if tbErr != nil {
		t.Fatalf("unexpected invocation error: %s", tbErr)
	}
	collections, ok := got.([]string)
	if !ok {
		t.Fatalf("expected []string result, got %T", got)
	}
	if collections == nil {
		t.Fatal("expected an initialized empty collection list")
	}
	encoded, err := json.Marshal(collections)
	if err != nil {
		t.Fatalf("unable to marshal collections: %s", err)
	}
	if string(encoded) != `[]` {
		t.Fatalf("expected empty collection list to marshal as [], got %s", encoded)
	}
}
