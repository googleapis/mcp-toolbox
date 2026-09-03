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

package firestorelistcollections_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/firestore/firestorelistcollections"
)

func TestParseFromYamlFirestoreListCollections(t *testing.T) {
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
			name: list_collections_tool
			type: firestore-list-collections
			source: my-firestore-instance
			description: List collections in Firestore
			`,
			want: server.ToolConfigs{
				"list_collections_tool": firestorelistcollections.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "list_collections_tool",
						Description:  "List collections in Firestore",
						AuthRequired: []string{},
					},
					Type:   "firestore-list-collections",
					Source: "my-firestore-instance",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_list_collections
			type: firestore-list-collections
			source: prod-firestore
			description: List collections with authentication
			authRequired:
				- google-auth-service
				- api-key-service
			`,
			want: server.ToolConfigs{
				"secure_list_collections": firestorelistcollections.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_list_collections",
						Description:  "List collections with authentication",
						AuthRequired: []string{"google-auth-service", "api-key-service"},
					},
					Type:   "firestore-list-collections",
					Source: "prod-firestore",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
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
	name: list_user_collections
	type: firestore-list-collections
	source: users-firestore
	description: List user-related collections
	authRequired:
		- user-auth
---
	kind: tool
	name: list_product_collections
	type: firestore-list-collections
	source: products-firestore
	description: List product-related collections
---
	kind: tool
	name: list_admin_collections
	type: firestore-list-collections
	source: admin-firestore
	description: List administrative collections
	authRequired:
		- user-auth
		- admin-auth
	`
	want := server.ToolConfigs{
		"list_user_collections": firestorelistcollections.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "list_user_collections",
				Description:  "List user-related collections",
				AuthRequired: []string{"user-auth"},
			},
			Type:   "firestore-list-collections",
			Source: "users-firestore",
		},
		"list_product_collections": firestorelistcollections.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "list_product_collections",
				Description:  "List product-related collections",
				AuthRequired: []string{},
			},
			Type:   "firestore-list-collections",
			Source: "products-firestore",
		},
		"list_admin_collections": firestorelistcollections.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "list_admin_collections",
				Description:  "List administrative collections",
				AuthRequired: []string{"user-auth", "admin-auth"},
			},
			Type:   "firestore-list-collections",
			Source: "admin-firestore",
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
