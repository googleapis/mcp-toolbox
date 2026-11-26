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

package mongodb

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestCheckPipelineScope(t *testing.T) {
	scoped := &Source{
		AllowedCollections: map[string]map[string]struct{}{
			"crm":     {"orders": {}, "customers": {}},
			"archive": {"orders": {}},
		},
	}
	tcs := []struct {
		desc     string
		source   *Source
		pipeline string
		wantErr  bool
	}{
		{"unrestricted source allows any reference", &Source{}, `[{"$lookup": {"from": "secrets", "localField": "a", "foreignField": "b", "as": "c"}}]`, false},
		{"stages without references", scoped, `[{"$match": {"status": "open"}}, {"$limit": 5}]`, false},
		{"lookup from an allowed collection", scoped, `[{"$lookup": {"from": "customers", "localField": "a", "foreignField": "b", "as": "c"}}]`, false},
		{"lookup from a disallowed collection", scoped, `[{"$lookup": {"from": "secrets", "localField": "a", "foreignField": "b", "as": "c"}}]`, true},
		{"nested lookup inside a lookup pipeline", scoped, `[{"$lookup": {"from": "customers", "as": "c", "pipeline": [{"$lookup": {"from": "secrets", "as": "d", "pipeline": []}}]}}]`, true},
		{"graphLookup from a disallowed collection", scoped, `[{"$graphLookup": {"from": "secrets", "startWith": "$a", "connectFromField": "a", "connectToField": "b", "as": "c"}}]`, true},
		{"unionWith by name", scoped, `[{"$unionWith": "secrets"}]`, true},
		{"unionWith document", scoped, `[{"$unionWith": {"coll": "customers", "pipeline": [{"$match": {}}]}}]`, false},
		{"unionWith pipeline reaching a disallowed collection", scoped, `[{"$unionWith": {"coll": "customers", "pipeline": [{"$lookup": {"from": "secrets", "as": "d", "pipeline": []}}]}}]`, true},
		{"out by name", scoped, `[{"$out": "secrets"}]`, true},
		{"out to another allowed database", scoped, `[{"$out": {"db": "archive", "coll": "orders"}}]`, false},
		{"out to another database that is not allowed", scoped, `[{"$out": {"db": "archive", "coll": "customers"}}]`, true},
		{"merge by name", scoped, `[{"$merge": "orders"}]`, false},
		{"merge into a disallowed collection", scoped, `[{"$merge": {"into": "secrets"}}]`, true},
		{"merge into another database", scoped, `[{"$merge": {"into": {"db": "other", "coll": "orders"}}}]`, true},
		{"facet with a disallowed lookup", scoped, `[{"$facet": {"a": [{"$match": {}}], "b": [{"$lookup": {"from": "secrets", "as": "d", "pipeline": []}}]}}]`, true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			var pipeline []bson.M
			if err := bson.UnmarshalExtJSON([]byte(tc.pipeline), false, &pipeline); err != nil {
				t.Fatalf("unable to parse pipeline: %s", err)
			}
			err := tc.source.checkPipelineScope(pipeline, "crm")
			if tc.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
		})
	}
}
