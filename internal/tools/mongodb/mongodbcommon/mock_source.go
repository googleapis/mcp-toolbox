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

package mongodbcommon

import (
	"context"
	"slices"
	"sort"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MockSource is a reusable mock implementation of sources.Source for MongoDB tool tests.
// It satisfies every MongoDB tool's compatibleSource, so one mock serves all of them.
type MockSource struct {
	sources.Source
	// AllowedCollections maps a database name to its allowed collections; nil means unrestricted.
	AllowedCollections map[string][]string
}

func (m *MockSource) MongoClient() *mongo.Client { return nil }

func (m *MockSource) IsCollectionAllowed(database, collection string) bool {
	if m.AllowedCollections == nil {
		return true
	}
	return slices.Contains(m.AllowedCollections[database], collection)
}

func (m *MockSource) MongoDBAllowedCollections(database string) []string {
	if m.AllowedCollections == nil {
		return nil
	}
	names := slices.Clone(m.AllowedCollections[database])
	if names == nil {
		names = []string{}
	}
	sort.Strings(names)
	return names
}

func (m *MockSource) Aggregate(context.Context, string, bool, bool, string, string) ([]any, error) {
	return nil, nil
}

func (m *MockSource) Find(context.Context, string, string, string, *options.FindOptionsBuilder) ([]any, error) {
	return nil, nil
}

func (m *MockSource) FindOne(context.Context, string, string, string, *options.FindOneOptionsBuilder) ([]any, error) {
	return nil, nil
}

func (m *MockSource) InsertOne(context.Context, string, bool, string, string) (any, error) {
	return nil, nil
}

func (m *MockSource) InsertMany(context.Context, string, bool, string, string) ([]any, error) {
	return nil, nil
}

func (m *MockSource) UpdateOne(context.Context, string, bool, string, string, string, bool) (any, error) {
	return nil, nil
}

func (m *MockSource) UpdateMany(context.Context, string, bool, string, string, string, bool) ([]any, error) {
	return nil, nil
}

func (m *MockSource) DeleteOne(context.Context, string, string, string) (any, error) {
	return nil, nil
}

func (m *MockSource) DeleteMany(context.Context, string, string, string) (any, error) {
	return nil, nil
}
