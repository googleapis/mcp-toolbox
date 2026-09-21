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

package firestoredeletedocuments_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/firestore/firestoredeletedocuments"
)

func TestParseFromYamlFirestoreDeleteDocuments(t *testing.T) {
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
			name: delete_docs_tool
			type: firestore-delete-documents
			source: my-firestore-instance
			description: Delete documents from Firestore by paths
			`,
			want: server.ToolConfigs{
				"delete_docs_tool": firestoredeletedocuments.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "delete_docs_tool",
						Description:  "Delete documents from Firestore by paths",
						AuthRequired: []string{},
					},
					Type:   "firestore-delete-documents",
					Source: "my-firestore-instance",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_delete_docs
			type: firestore-delete-documents
			source: prod-firestore
			description: Delete documents with authentication
			authRequired:
				- google-auth-service
				- api-key-service
			`,
			want: server.ToolConfigs{
				"secure_delete_docs": firestoredeletedocuments.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_delete_docs",
						Description:  "Delete documents with authentication",
						AuthRequired: []string{"google-auth-service", "api-key-service"},
					},
					Type:   "firestore-delete-documents",
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
	name: delete_user_docs
	type: firestore-delete-documents
	source: users-firestore
	description: Delete user documents
	authRequired:
		- user-auth
---
	kind: tool
	name: delete_product_docs
	type: firestore-delete-documents
	source: products-firestore
	description: Delete product documents
---
	kind: tool
	name: delete_order_docs
	type: firestore-delete-documents
	source: orders-firestore
	description: Delete order documents
	authRequired:
		- user-auth
		- admin-auth
	`
	want := server.ToolConfigs{
		"delete_user_docs": firestoredeletedocuments.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "delete_user_docs",
				Description:  "Delete user documents",
				AuthRequired: []string{"user-auth"},
			},
			Type:   "firestore-delete-documents",
			Source: "users-firestore",
		},
		"delete_product_docs": firestoredeletedocuments.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "delete_product_docs",
				Description:  "Delete product documents",
				AuthRequired: []string{},
			},
			Type:   "firestore-delete-documents",
			Source: "products-firestore",
		},
		"delete_order_docs": firestoredeletedocuments.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "delete_order_docs",
				Description:  "Delete order documents",
				AuthRequired: []string{"user-auth", "admin-auth"},
			},
			Type:   "firestore-delete-documents",
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
