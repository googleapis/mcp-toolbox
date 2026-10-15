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

package mongodb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "mongodb"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type Config struct {
	Name string `yaml:"name" validate:"required"`
	Type string `yaml:"type" validate:"required"`
	Uri  string `yaml:"uri" validate:"required"` // MongoDB Atlas connection URI
	// AllowedCollections restricts every tool on this source to these "database.collection" entries.
	AllowedCollections []string `yaml:"allowedCollections"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	allowedCollections, err := normalizeAllowedCollections(r.AllowedCollections)
	if err != nil {
		return nil, err
	}

	client, err := initMongoDBClient(ctx, tracer, r.Name, r.Uri)
	if err != nil {
		return nil, fmt.Errorf("unable to create MongoDB client: %w", err)
	}

	// Verify the connection
	err = client.Ping(ctx, nil)
	if err != nil {
		_ = client.Disconnect(ctx)
		return nil, fmt.Errorf("unable to connect successfully: %w", err)
	}

	s := &Source{
		Config:             r,
		Client:             client,
		AllowedCollections: allowedCollections,
	}
	return s, nil
}

// normalizeAllowedCollections indexes "database.collection" entries by database; a database name cannot contain a dot, so the split is on the first one.
func normalizeAllowedCollections(entries []string) (map[string]map[string]struct{}, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	allowed := make(map[string]map[string]struct{})
	for _, e := range entries {
		database, collection, found := strings.Cut(e, ".")
		if !found || database == "" || collection == "" {
			return nil, fmt.Errorf("invalid allowedCollections entry %q, expected 'database.collection'", e)
		}
		if allowed[database] == nil {
			allowed[database] = make(map[string]struct{})
		}
		allowed[database][collection] = struct{}{}
	}
	return allowed, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	Client *mongo.Client
	// AllowedCollections maps a database name to the set of collections allowed in it; empty means unrestricted.
	AllowedCollections map[string]map[string]struct{}
}

// IsCollectionAllowed reports whether a collection may be used by a tool on this source.
func (s *Source) IsCollectionAllowed(database, collection string) bool {
	if len(s.AllowedCollections) == 0 {
		return true
	}
	_, ok := s.AllowedCollections[database][collection]
	return ok
}

// MongoDBAllowedCollections returns the sorted collections allowed in a database, or nil when unrestricted.
func (s *Source) MongoDBAllowedCollections(database string) []string {
	if len(s.AllowedCollections) == 0 {
		return nil
	}
	names := make([]string, 0, len(s.AllowedCollections[database]))
	for c := range s.AllowedCollections[database] {
		names = append(names, c)
	}
	sort.Strings(names)
	return names
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) MongoClient() *mongo.Client {
	return s.Client
}

func parseData(ctx context.Context, cur *mongo.Cursor) ([]any, error) {
	var data = []any{}
	err := cur.All(ctx, &data)
	if err != nil {
		return nil, err
	}
	final := []any{}
	for _, item := range data {
		tmp, _ := bson.MarshalExtJSON(item, false, false)
		var tmp2 any
		err = json.Unmarshal(tmp, &tmp2)
		if err != nil {
			return nil, err
		}
		final = append(final, tmp2)
	}
	return final, err
}

// collectionRef is a collection that a pipeline stage reads from or writes to.
type collectionRef struct {
	database, collection string
}

// checkPipelineScope rejects a pipeline whose stages reach a collection outside allowedCollections.
func (s *Source) checkPipelineScope(pipeline []bson.M, database string) error {
	if len(s.AllowedCollections) == 0 {
		return nil
	}
	stages := make([]any, len(pipeline))
	for i, stage := range pipeline {
		stages[i] = stage
	}
	for _, ref := range pipelineCollections(stages, database) {
		if !s.IsCollectionAllowed(ref.database, ref.collection) {
			return fmt.Errorf("pipeline references collection %q in database %q, which is not in the allowedCollections of this source", ref.collection, ref.database)
		}
	}
	return nil
}

// pipelineCollections returns the collections referenced by $lookup, $graphLookup, $unionWith, $out, $merge and $facet stages, including nested pipelines.
func pipelineCollections(stages []any, database string) []collectionRef {
	var refs []collectionRef
	for _, stage := range stages {
		for op, spec := range asDocument(stage) {
			doc := asDocument(spec)
			switch op {
			case "$lookup", "$graphLookup":
				refs = appendCollectionRef(refs, doc["from"], database)
				refs = append(refs, pipelineCollections(asArray(doc["pipeline"]), database)...)
			case "$unionWith":
				if doc == nil {
					refs = appendCollectionRef(refs, spec, database)
					continue
				}
				refs = appendCollectionRef(refs, doc["coll"], database)
				refs = append(refs, pipelineCollections(asArray(doc["pipeline"]), database)...)
			case "$out":
				refs = appendCollectionRef(refs, spec, database)
			case "$merge":
				if doc == nil {
					refs = appendCollectionRef(refs, spec, database)
					continue
				}
				refs = appendCollectionRef(refs, doc["into"], database)
			case "$facet":
				for _, sub := range doc {
					refs = append(refs, pipelineCollections(asArray(sub), database)...)
				}
			}
		}
	}
	return refs
}

// appendCollectionRef adds a collection given either by name or as a {db, coll} document.
func appendCollectionRef(refs []collectionRef, v any, database string) []collectionRef {
	if name, ok := v.(string); ok {
		return append(refs, collectionRef{database, name})
	}
	doc := asDocument(v)
	coll, _ := doc["coll"].(string)
	if coll == "" {
		return refs
	}
	if db, _ := doc["db"].(string); db != "" {
		database = db
	}
	return append(refs, collectionRef{database, coll})
}

// asDocument returns v as a map, accepting the bson.M and bson.D forms that extended JSON decodes into.
func asDocument(v any) map[string]any {
	switch d := v.(type) {
	case bson.M:
		return d
	case bson.D:
		m := make(map[string]any, len(d))
		for _, e := range d {
			m[e.Key] = e.Value
		}
		return m
	}
	return nil
}

// asArray returns v as a slice, accepting the bson.A form that extended JSON decodes into.
func asArray(v any) []any {
	if a, ok := v.(bson.A); ok {
		return a
	}
	return nil
}

// ListCollectionNames returns the collections in a database, restricted to the source's allowedCollections when one is set.
func (s *Source) ListCollectionNames(ctx context.Context, database string) ([]string, error) {
	names, err := s.MongoClient().Database(database).ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	allowed := make([]string, 0, len(names))
	for _, n := range names {
		if s.IsCollectionAllowed(database, n) {
			allowed = append(allowed, n)
		}
	}
	sort.Strings(allowed)
	return allowed, nil
}

func (s *Source) Aggregate(ctx context.Context, pipelineString string, canonical, readOnly bool, database, collection string) ([]any, error) {
	var pipeline = []bson.M{}
	err := bson.UnmarshalExtJSON([]byte(pipelineString), canonical, &pipeline)
	if err != nil {
		return nil, err
	}

	if readOnly {
		//fail if we do a merge or an out
		for _, stage := range pipeline {
			for key := range stage {
				if key == "$merge" || key == "$out" {
					return nil, fmt.Errorf("this is not a read-only pipeline: %+v", stage)
				}
			}
		}
	}

	if err := s.checkPipelineScope(pipeline, database); err != nil {
		return nil, err
	}

	cur, err := s.MongoClient().Database(database).Collection(collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	res, err := parseData(ctx, cur)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return []any{}, nil
	}
	return res, err
}

func (s *Source) Find(ctx context.Context, filterString, database, collection string, opts *options.FindOptionsBuilder) ([]any, error) {
	var filter = bson.D{}
	err := bson.UnmarshalExtJSON([]byte(filterString), false, &filter)
	if err != nil {
		return nil, err
	}

	cur, err := s.MongoClient().Database(database).Collection(collection).Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	return parseData(ctx, cur)
}

func (s *Source) FindOne(ctx context.Context, filterString, database, collection string, opts *options.FindOneOptionsBuilder) ([]any, error) {
	var filter = bson.D{}
	err := bson.UnmarshalExtJSON([]byte(filterString), false, &filter)
	if err != nil {
		return nil, err
	}

	res := s.MongoClient().Database(database).Collection(collection).FindOne(ctx, filter, opts)
	if res.Err() != nil {
		return nil, res.Err()
	}

	var data any
	err = res.Decode(&data)
	if err != nil {
		return nil, err
	}

	var final []any
	tmp, _ := bson.MarshalExtJSON(data, false, false)
	var tmp2 any
	err = json.Unmarshal(tmp, &tmp2)
	if err != nil {
		return nil, err
	}
	final = append(final, tmp2)

	return final, err
}

func (s *Source) InsertMany(ctx context.Context, jsonData string, canonical bool, database, collection string) ([]any, error) {
	var data = []any{}
	err := bson.UnmarshalExtJSON([]byte(jsonData), canonical, &data)
	if err != nil {
		return nil, err
	}

	res, err := s.MongoClient().Database(database).Collection(collection).InsertMany(ctx, data, options.InsertMany())
	if err != nil {
		return nil, err
	}
	return res.InsertedIDs, nil
}

func (s *Source) InsertOne(ctx context.Context, jsonData string, canonical bool, database, collection string) (any, error) {
	var data any
	err := bson.UnmarshalExtJSON([]byte(jsonData), canonical, &data)
	if err != nil {
		return nil, err
	}

	res, err := s.MongoClient().Database(database).Collection(collection).InsertOne(ctx, data, options.InsertOne())
	if err != nil {
		return nil, err
	}
	return res.InsertedID, nil
}

func (s *Source) UpdateMany(ctx context.Context, filterString string, canonical bool, updateString, database, collection string, upsert bool) ([]any, error) {
	var filter = bson.D{}
	err := bson.UnmarshalExtJSON([]byte(filterString), canonical, &filter)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal filter string: %w", err)
	}
	var update = bson.D{}
	err = bson.UnmarshalExtJSON([]byte(updateString), false, &update)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal update string: %w", err)
	}

	res, err := s.MongoClient().Database(database).Collection(collection).UpdateMany(ctx, filter, update, options.UpdateMany().SetUpsert(upsert))
	if err != nil {
		return nil, fmt.Errorf("error updating collection: %w", err)
	}
	return []any{res.ModifiedCount, res.UpsertedCount, res.MatchedCount}, nil
}

func (s *Source) UpdateOne(ctx context.Context, filterString string, canonical bool, updateString, database, collection string, upsert bool) (any, error) {
	var filter = bson.D{}
	err := bson.UnmarshalExtJSON([]byte(filterString), false, &filter)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal filter string: %w", err)
	}
	var update = bson.D{}
	err = bson.UnmarshalExtJSON([]byte(updateString), canonical, &update)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal update string: %w", err)
	}

	res, err := s.MongoClient().Database(database).Collection(collection).UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(upsert))
	if err != nil {
		return nil, fmt.Errorf("error updating collection: %w", err)
	}
	return res.ModifiedCount, nil
}

func (s *Source) DeleteMany(ctx context.Context, filterString, database, collection string) (any, error) {
	var filter = bson.D{}
	err := bson.UnmarshalExtJSON([]byte(filterString), false, &filter)
	if err != nil {
		return nil, err
	}

	res, err := s.MongoClient().Database(database).Collection(collection).DeleteMany(ctx, filter, options.DeleteMany())
	if err != nil {
		return nil, err
	}

	if res.DeletedCount == 0 {
		return nil, errors.New("no document found")
	}
	return res.DeletedCount, nil
}

func (s *Source) DeleteOne(ctx context.Context, filterString, database, collection string) (any, error) {
	var filter = bson.D{}
	err := bson.UnmarshalExtJSON([]byte(filterString), false, &filter)
	if err != nil {
		return nil, err
	}

	res, err := s.MongoClient().Database(database).Collection(collection).DeleteOne(ctx, filter, options.DeleteOne())
	if err != nil {
		return nil, err
	}
	return res.DeletedCount, nil
}

func initMongoDBClient(ctx context.Context, tracer trace.Tracer, name, uri string) (*mongo.Client, error) {
	// Start a tracing span
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Create a new MongoDB client
	clientOpts := options.Client().ApplyURI(uri).SetAppName(userAgent)
	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("unable to create MongoDB client: %w", err)
	}

	return client, nil
}
