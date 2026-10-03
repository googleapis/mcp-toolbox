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

package mongodb_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/mongodb"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlMongoDB(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: mongo-db
			type: "mongodb"
			uri: "mongodb+srv://username:password@host/dbname"
			`,
			want: map[string]sources.SourceConfig{
				"mongo-db": mongodb.Config{
					Name: "mongo-db",
					Type: mongodb.SourceType,
					Uri:  "mongodb+srv://username:password@host/dbname",
				},
			},
		},
		{
			desc: "with allowedCollections",
			in: `
			kind: source
			name: mongo-db
			type: "mongodb"
			uri: "mongodb+srv://username:password@host/dbname"
			allowedCollections: [crm.orders, crm.customers]
			`,
			want: map[string]sources.SourceConfig{
				"mongo-db": mongodb.Config{
					Name:               "mongo-db",
					Type:               mongodb.SourceType,
					Uri:                "mongodb+srv://username:password@host/dbname",
					AllowedCollections: []string{"crm.orders", "crm.customers"},
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
			name: mongo-db
			type: mongodb
			uri: "mongodb+srv://username:password@host/dbname"
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"mongo-db\" as \"mongodb\": [1:1] unknown field \"foo\"\n>  1 | foo: bar\n       ^\n   2 | name: mongo-db\n   3 | type: mongodb\n   4 | uri: mongodb+srv://username:password@host/dbname",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: mongo-db
			type: mongodb
			`,
			err: "error unmarshaling source: unable to parse source \"mongo-db\" as \"mongodb\": Key: 'Config.Uri' Error:Field validation for 'Uri' failed on the 'required' tag",
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
				t.Fatalf("unexpected error: got \n%q, want \n%q", errStr, tc.err)
			}
		})
	}
}

func TestInitializeRejectsInvalidAllowedCollections(t *testing.T) {
	tcs := []struct {
		desc string
		in   []string
	}{
		{"bare collection name", []string{"orders"}},
		{"empty database", []string{".orders"}},
		{"empty collection", []string{"crm."}},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := mongodb.Config{
				Name:               "mongo-db",
				Type:               mongodb.SourceType,
				Uri:                "mongodb://localhost:27017",
				AllowedCollections: tc.in,
			}
			if _, err := cfg.Initialize(context.Background(), nil); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestIsCollectionAllowed(t *testing.T) {
	scoped := &mongodb.Source{
		AllowedCollections: map[string]map[string]struct{}{
			"crm": {"orders": {}, "customers": {}},
		},
	}
	tcs := []struct {
		desc       string
		source     *mongodb.Source
		database   string
		collection string
		want       bool
	}{
		{"unrestricted source allows anything", &mongodb.Source{}, "crm", "secrets", true},
		{"allowed collection", scoped, "crm", "orders", true},
		{"collection not in the list", scoped, "crm", "secrets", false},
		{"database not in the list", scoped, "other", "orders", false},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if got := tc.source.IsCollectionAllowed(tc.database, tc.collection); got != tc.want {
				t.Fatalf("got %t, want %t", got, tc.want)
			}
		})
	}
}

func TestMongoDBAllowedCollections(t *testing.T) {
	scoped := &mongodb.Source{
		AllowedCollections: map[string]map[string]struct{}{
			"crm": {"orders": {}, "customers": {}},
		},
	}
	tcs := []struct {
		desc     string
		source   *mongodb.Source
		database string
		want     []string
	}{
		{"unrestricted source returns nil", &mongodb.Source{}, "crm", nil},
		{"sorted collections for the database", scoped, "crm", []string{"customers", "orders"}},
		{"database with no allowed collections", scoped, "other", []string{}},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, tc.source.MongoDBAllowedCollections(tc.database)); diff != "" {
				t.Fatalf("unexpected collections (-want +got):\n%s", diff)
			}
		})
	}
}
