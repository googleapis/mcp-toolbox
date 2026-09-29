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

package firestorequery_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/firestore/firestorequery"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlFirestoreQuery(t *testing.T) {
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
			desc: "basic example with parameterized collection path",
			in: `
            kind: tool
            name: query_users_tool
            type: firestore-query
            source: my-firestore-instance
            description: Query users collection with parameterized path
            collectionPath: "users/{{.userId}}/documents"
            parameters:
                - name: userId
                  type: string
                  description: The user ID to query documents for
                  required: true
			`,
			want: server.ToolConfigs{
				"query_users_tool": firestorequery.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "query_users_tool",
						Description:  "Query users collection with parameterized path",
						AuthRequired: []string{},
					},
					Type:           "firestore-query",
					Source:         "my-firestore-instance",
					CollectionPath: "users/{{.userId}}/documents",
					Parameters: parameters.Parameters{
						parameters.NewStringParameter("userId", "The user ID to query documents for", parameters.WithStringRequired(true)),
					},
				},
			},
		},
		{
			desc: "with parameterized filters",
			in: `
            kind: tool
            name: query_products_tool
            type: firestore-query
            source: prod-firestore
            description: Query products with dynamic filters
            collectionPath: "products"
            filters: |
                {
                    "and": [
                        {"field": "category", "op": "==", "value": {"stringValue": "{{.category}}"}},
                        {"field": "price", "op": "<=", "value": {"doubleValue": {{.maxPrice}}}}
                    ]
                }
            parameters:
                - name: category
                  type: string
                  description: Product category to filter by
                  required: true
                - name: maxPrice
                  type: float
                  description: Maximum price for products
                  required: true
			`,
			want: server.ToolConfigs{
				"query_products_tool": firestorequery.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "query_products_tool",
						Description:  "Query products with dynamic filters",
						AuthRequired: []string{},
					},
					Type:           "firestore-query",
					Source:         "prod-firestore",
					CollectionPath: "products",
					Filters: `{
    "and": [
        {"field": "category", "op": "==", "value": {"stringValue": "{{.category}}"}},
        {"field": "price", "op": "<=", "value": {"doubleValue": {{.maxPrice}}}}
    ]
}
`,
					Parameters: parameters.Parameters{
						parameters.NewStringParameter("category", "Product category to filter by", parameters.WithStringRequired(true)),
						parameters.NewFloatParameter("maxPrice", "Maximum price for products", parameters.WithFloatRequired(true)),
					},
				},
			},
		},
		{
			desc: "with select fields and orderBy",
			in: `
            kind: tool
            name: query_orders_tool
            type: firestore-query
            source: orders-firestore
            description: Query orders with field selection
            collectionPath: "orders"
            select:
                - orderId
                - customerName
                - totalAmount
            orderBy:
                field: "{{.sortField}}"
                direction: "DESCENDING"
            limit: 50
            parameters:
                - name: sortField
                  type: string
                  description: Field to sort by
                  required: true
			`,
			want: server.ToolConfigs{
				"query_orders_tool": firestorequery.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "query_orders_tool",
						Description:  "Query orders with field selection",
						AuthRequired: []string{},
					},
					Type:           "firestore-query",
					Source:         "orders-firestore",
					CollectionPath: "orders",
					Select:         []string{"orderId", "customerName", "totalAmount"},
					OrderBy: map[string]any{
						"field":     "{{.sortField}}",
						"direction": "DESCENDING",
					},
					Limit: "50",
					Parameters: parameters.Parameters{
						parameters.NewStringParameter("sortField", "Field to sort by", parameters.WithStringRequired(true)),
					},
				},
			},
		},
		{
			desc: "with auth requirements and complex filters",
			in: `
            kind: tool
            name: secure_query_tool
            type: firestore-query
            source: secure-firestore
            description: Query with authentication and complex filters
            collectionPath: "{{.collection}}"
            filters: |
                {
                    "or": [
                        {
                            "and": [
                                {"field": "status", "op": "==", "value": {"stringValue": "{{.status}}"}},
                                {"field": "priority", "op": ">=", "value": {"integerValue": "{{.minPriority}}"}}
                            ]
                        },
                        {"field": "urgent", "op": "==", "value": {"booleanValue": true}}
                    ]
                }
            analyzeQuery: true
            authRequired:
                - google-auth-service
                - api-key-service
            parameters:
                - name: collection
                  type: string
                  description: Collection name to query
                  required: true
                - name: status
                  type: string
                  description: Status to filter by
                  required: true
                - name: minPriority
                  type: integer
                  description: Minimum priority level
                  default: 1
			`,
			want: server.ToolConfigs{
				"secure_query_tool": firestorequery.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_query_tool",
						Description:  "Query with authentication and complex filters",
						AuthRequired: []string{"google-auth-service", "api-key-service"},
					},
					Type:           "firestore-query",
					Source:         "secure-firestore",
					CollectionPath: "{{.collection}}",
					Filters: `{
    "or": [
        {
            "and": [
                {"field": "status", "op": "==", "value": {"stringValue": "{{.status}}"}},
                {"field": "priority", "op": ">=", "value": {"integerValue": "{{.minPriority}}"}}
            ]
        },
        {"field": "urgent", "op": "==", "value": {"booleanValue": true}}
    ]
}
`,
					AnalyzeQuery: true,
					Parameters: parameters.Parameters{
						parameters.NewStringParameter("collection", "Collection name to query", parameters.WithStringRequired(true)),
						parameters.NewStringParameter("status", "Status to filter by", parameters.WithStringRequired(true)),
						parameters.NewIntParameter("minPriority", "Minimum priority level", parameters.WithIntDefault(1)),
					},
				},
			},
		},
		{
			desc: "with Firestore native JSON value types and template parameters",
			in: `
            kind: tool
            name: query_with_typed_values
            type: firestore-query
            source: typed-firestore
            description: Query with Firestore native JSON value types
            collectionPath: "countries"
            filters: |
                {
                    "or": [
                        {"field": "continent", "op": "==", "value": {"stringValue": "{{.continent}}"}},
                        {
                            "and": [
                                {"field": "area", "op": ">", "value": {"integerValue": "2000000"}},
                                {"field": "area", "op": "<", "value": {"integerValue": "3000000"}},
                                {"field": "population", "op": ">=", "value": {"integerValue": "{{.minPopulation}}"}},
                                {"field": "gdp", "op": ">", "value": {"doubleValue": {{.minGdp}}}},
                                {"field": "isActive", "op": "==", "value": {"booleanValue": {{.isActive}}}},
                                {"field": "lastUpdated", "op": ">=", "value": {"timestampValue": "{{.startDate}}"}}
                            ]
                        }
                    ]
                }
            parameters:
                - name: continent
                  type: string
                  description: Continent to filter by
                  required: true
                - name: minPopulation
                  type: string
                  description: Minimum population as string
                  required: true
                - name: minGdp
                  type: float
                  description: Minimum GDP value
                  required: true
                - name: isActive
                  type: boolean
                  description: Filter by active status
                  required: true
                - name: startDate
                  type: string
                  description: Start date in RFC3339 format
                  required: true
			`,
			want: server.ToolConfigs{
				"query_with_typed_values": firestorequery.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "query_with_typed_values",
						Description:  "Query with Firestore native JSON value types",
						AuthRequired: []string{},
					},
					Type:           "firestore-query",
					Source:         "typed-firestore",
					CollectionPath: "countries",
					Filters: `{
    "or": [
        {"field": "continent", "op": "==", "value": {"stringValue": "{{.continent}}"}},
        {
            "and": [
                {"field": "area", "op": ">", "value": {"integerValue": "2000000"}},
                {"field": "area", "op": "<", "value": {"integerValue": "3000000"}},
                {"field": "population", "op": ">=", "value": {"integerValue": "{{.minPopulation}}"}},
                {"field": "gdp", "op": ">", "value": {"doubleValue": {{.minGdp}}}},
                {"field": "isActive", "op": "==", "value": {"booleanValue": {{.isActive}}}},
                {"field": "lastUpdated", "op": ">=", "value": {"timestampValue": "{{.startDate}}"}}
            ]
        }
    ]
}
`,
					Parameters: parameters.Parameters{
						parameters.NewStringParameter("continent", "Continent to filter by", parameters.WithStringRequired(true)),
						parameters.NewStringParameter("minPopulation", "Minimum population as string", parameters.WithStringRequired(true)),
						parameters.NewFloatParameter("minGdp", "Minimum GDP value", parameters.WithFloatRequired(true)),
						parameters.NewBooleanParameter("isActive", "Filter by active status", parameters.WithBooleanRequired(true)),
						parameters.NewStringParameter("startDate", "Start date in RFC3339 format", parameters.WithStringRequired(true)),
					},
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

func TestParseFromYamlMultipleQueryTools(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	in := `
	kind: tool
	name: query_user_posts
	type: firestore-query
	source: social-firestore
	description: Query user posts with filtering
	collectionPath: "users/{{.userId}}/posts"
	filters: |
		{
			"and": [
				{"field": "visibility", "op": "==", "value": {"stringValue": "{{.visibility}}"}},
				{"field": "createdAt", "op": ">=", "value": {"timestampValue": "{{.startDate}}"}}
			]
		}
	select:
		- title
		- content
		- likes
	orderBy:
		field: createdAt
		direction: "{{.sortOrder}}"
	limit: 20
	parameters:
		- name: userId
		  type: string
		  description: User ID whose posts to query
		  required: true
		- name: visibility
		  type: string
		  description: Post visibility (public, private, friends)
		  required: true
		- name: startDate
		  type: string
		  description: Start date for posts
		  required: true
		- name: sortOrder
		  type: string
		  description: Sort order (ASCENDING or DESCENDING)
		  default: "DESCENDING"
---
	kind: tool
	name: query_inventory
	type: firestore-query
	source: inventory-firestore
	description: Query inventory items
	collectionPath: "warehouses/{{.warehouseId}}/inventory"
	filters: |
		{
			"field": "quantity", "op": "<", "value": {"integerValue": "{{.threshold}}"}}
	parameters:
		- name: warehouseId
		  type: string
		  description: Warehouse ID to check inventory
		  required: true
		- name: threshold
		  type: integer
		  description: Quantity threshold for low stock
		  required: true
---
	kind: tool
	name: query_transactions
	type: firestore-query
	source: finance-firestore
	description: Query financial transactions
	collectionPath: "accounts/{{.accountId}}/transactions"
	filters: |
		{
			"or": [
				{"field": "type", "op": "==", "value": {"stringValue": "{{.transactionType}}"}},
				{"field": "amount", "op": ">", "value": {"doubleValue": {{.minAmount}}}}
			]
		}
	analyzeQuery: true
	authRequired:
		- finance-auth
	parameters:
		- name: accountId
		  type: string
		  description: Account ID for transactions
		  required: true
		- name: transactionType
		  type: string
		  description: Type of transaction
		  default: "all"
		- name: minAmount
		  type: float
		  description: Minimum transaction amount
		  default: 0
	`
	want := server.ToolConfigs{
		"query_user_posts": firestorequery.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "query_user_posts",
				Description:  "Query user posts with filtering",
				AuthRequired: []string{},
			},
			Type:           "firestore-query",
			Source:         "social-firestore",
			CollectionPath: "users/{{.userId}}/posts",
			Filters: `{
  "and": [
    {"field": "visibility", "op": "==", "value": {"stringValue": "{{.visibility}}"}},
    {"field": "createdAt", "op": ">=", "value": {"timestampValue": "{{.startDate}}"}}
  ]
}
`,
			Select: []string{"title", "content", "likes"},
			OrderBy: map[string]any{
				"field":     "createdAt",
				"direction": "{{.sortOrder}}",
			},
			Limit: "20",
			Parameters: parameters.Parameters{
				parameters.NewStringParameter("userId", "User ID whose posts to query", parameters.WithStringRequired(true)),
				parameters.NewStringParameter("visibility", "Post visibility (public, private, friends)", parameters.WithStringRequired(true)),
				parameters.NewStringParameter("startDate", "Start date for posts", parameters.WithStringRequired(true)),
				parameters.NewStringParameter("sortOrder", "Sort order (ASCENDING or DESCENDING)", parameters.WithStringDefault("DESCENDING")),
			},
		},
		"query_inventory": firestorequery.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "query_inventory",
				Description:  "Query inventory items",
				AuthRequired: []string{},
			},
			Type:           "firestore-query",
			Source:         "inventory-firestore",
			CollectionPath: "warehouses/{{.warehouseId}}/inventory",
			Filters: `{
  "field": "quantity", "op": "<", "value": {"integerValue": "{{.threshold}}"}}
`,
			Parameters: parameters.Parameters{
				parameters.NewStringParameter("warehouseId", "Warehouse ID to check inventory", parameters.WithStringRequired(true)),
				parameters.NewIntParameter("threshold", "Quantity threshold for low stock", parameters.WithIntRequired(true)),
			},
		},
		"query_transactions": firestorequery.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "query_transactions",
				Description:  "Query financial transactions",
				AuthRequired: []string{"finance-auth"},
			},
			Type:           "firestore-query",
			Source:         "finance-firestore",
			CollectionPath: "accounts/{{.accountId}}/transactions",
			Filters: `{
  "or": [
    {"field": "type", "op": "==", "value": {"stringValue": "{{.transactionType}}"}},
    {"field": "amount", "op": ">", "value": {"doubleValue": {{.minAmount}}}}
  ]
}
`,
			AnalyzeQuery: true,
			Parameters: parameters.Parameters{
				parameters.NewStringParameter("accountId", "Account ID for transactions", parameters.WithStringRequired(true)),
				parameters.NewStringParameter("transactionType", "Type of transaction", parameters.WithStringDefault("all")),
				parameters.NewFloatParameter("minAmount", "Minimum transaction amount", parameters.WithFloatDefault(0)),
			},
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
