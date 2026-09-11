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

package firestorequerycollection_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/firestore/firestorequerycollection"
)

func TestParseFromYamlFirestoreQueryCollection(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		want server.ToolConfigs
	}{
		{
			desc: "basic example",
			in: `
            kind: tool
            name: query_users_tool
            type: firestore-query-collection
            source: my-firestore-instance
            description: Query users collection with filters and ordering
			`,
			want: server.ToolConfigs{
				"query_users_tool": firestorequerycollection.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "query_users_tool",
						Description:  "Query users collection with filters and ordering",
						AuthRequired: []string{},
					},
					Type:   "firestore-query-collection",
					Source: "my-firestore-instance",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
            kind: tool
            name: secure_query_tool
            type: firestore-query-collection
            source: prod-firestore
            description: Query collections with authentication
            authRequired:
                - google-auth-service
                - api-key-service
			`,
			want: server.ToolConfigs{
				"secure_query_tool": firestorequerycollection.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_query_tool",
						Description:  "Query collections with authentication",
						AuthRequired: []string{"google-auth-service", "api-key-service"},
					},
					Type:   "firestore-query-collection",
					Source: "prod-firestore",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}

func TestParseFromYamlMultipleTools(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	in := `
	kind: tool
	name: query_users
	type: firestore-query-collection
	source: users-firestore
	description: Query user documents with filtering
	authRequired:
		- user-auth
---
	kind: tool
	name: query_products
	type: firestore-query-collection
	source: products-firestore
	description: Query product catalog
---
	kind: tool
	name: query_orders
	type: firestore-query-collection
	source: orders-firestore
	description: Query customer orders with complex filters
	authRequired:
		- user-auth
		- admin-auth
	`
	want := server.ToolConfigs{
		"query_users": firestorequerycollection.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "query_users",
				Description:  "Query user documents with filtering",
				AuthRequired: []string{"user-auth"},
			},
			Type:   "firestore-query-collection",
			Source: "users-firestore",
		},
		"query_products": firestorequerycollection.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "query_products",
				Description:  "Query product catalog",
				AuthRequired: []string{},
			},
			Type:   "firestore-query-collection",
			Source: "products-firestore",
		},
		"query_orders": firestorequerycollection.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "query_orders",
				Description:  "Query customer orders with complex filters",
				AuthRequired: []string{"user-auth", "admin-auth"},
			},
			Type:   "firestore-query-collection",
			Source: "orders-firestore",
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
