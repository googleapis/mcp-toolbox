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

package firestoreadddocuments_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/firestore/firestoreadddocuments"
)

func TestParseFromYamlFirestoreAddDocuments(t *testing.T) {
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
			name: add_docs_tool
			type: firestore-add-documents
			source: my-firestore-instance
			description: Add documents to Firestore collections
			`,
			want: server.ToolConfigs{
				"add_docs_tool": firestoreadddocuments.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "add_docs_tool",
						Description:  "Add documents to Firestore collections",
						AuthRequired: []string{},
					},
					Type:   "firestore-add-documents",
					Source: "my-firestore-instance",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_add_docs
			type: firestore-add-documents
			source: prod-firestore
			description: Add documents with authentication
			authRequired:
				- google-auth-service
				- api-key-service
			`,
			want: server.ToolConfigs{
				"secure_add_docs": firestoreadddocuments.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_add_docs",
						Description:  "Add documents with authentication",
						AuthRequired: []string{"google-auth-service", "api-key-service"},
					},
					Type:   "firestore-add-documents",
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
	name: add_user_docs
	type: firestore-add-documents
	source: users-firestore
	description: Add user documents
	authRequired:
		- user-auth
---
	kind: tool
	name: add_product_docs
	type: firestore-add-documents
	source: products-firestore
	description: Add product documents
---
	kind: tool
	name: add_order_docs
	type: firestore-add-documents
	source: orders-firestore
	description: Add order documents
	authRequired:
		- user-auth
		- admin-auth
	`
	want := server.ToolConfigs{
		"add_user_docs": firestoreadddocuments.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "add_user_docs",
				Description:  "Add user documents",
				AuthRequired: []string{"user-auth"},
			},
			Type:   "firestore-add-documents",
			Source: "users-firestore",
		},
		"add_product_docs": firestoreadddocuments.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "add_product_docs",
				Description:  "Add product documents",
				AuthRequired: []string{},
			},
			Type:   "firestore-add-documents",
			Source: "products-firestore",
		},
		"add_order_docs": firestoreadddocuments.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "add_order_docs",
				Description:  "Add order documents",
				AuthRequired: []string{"user-auth", "admin-auth"},
			},
			Type:   "firestore-add-documents",
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
