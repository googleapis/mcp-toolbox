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

package firestoregetdocuments_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/firestore/firestoregetdocuments"
)

func TestParseFromYamlFirestoreGetDocuments(t *testing.T) {
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
			name: get_docs_tool
			type: firestore-get-documents
			source: my-firestore-instance
			description: Retrieve documents from Firestore by paths
			`,
			want: server.ToolConfigs{
				"get_docs_tool": firestoregetdocuments.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "get_docs_tool",
						Description:  "Retrieve documents from Firestore by paths",
						AuthRequired: []string{},
					},
					Type:   "firestore-get-documents",
					Source: "my-firestore-instance",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_get_docs
			type: firestore-get-documents
			source: prod-firestore
			description: Get documents with authentication
			authRequired:
				- google-auth-service
				- api-key-service
			`,
			want: server.ToolConfigs{
				"secure_get_docs": firestoregetdocuments.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_get_docs",
						Description:  "Get documents with authentication",
						AuthRequired: []string{"google-auth-service", "api-key-service"},
					},
					Type:   "firestore-get-documents",
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
	name: get_user_docs
	type: firestore-get-documents
	source: users-firestore
	description: Get user documents
	authRequired:
		- user-auth
---
	kind: tool
	name: get_product_docs
	type: firestore-get-documents
	source: products-firestore
	description: Get product documents
---
	kind: tool
	name: get_order_docs
	type: firestore-get-documents
	source: orders-firestore
	description: Get order documents
	authRequired:
		- user-auth
		- admin-auth
	`
	want := server.ToolConfigs{
		"get_user_docs": firestoregetdocuments.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "get_user_docs",
				Description:  "Get user documents",
				AuthRequired: []string{"user-auth"},
			},
			Type:   "firestore-get-documents",
			Source: "users-firestore",
		},
		"get_product_docs": firestoregetdocuments.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "get_product_docs",
				Description:  "Get product documents",
				AuthRequired: []string{},
			},
			Type:   "firestore-get-documents",
			Source: "products-firestore",
		},
		"get_order_docs": firestoregetdocuments.Config{
			ConfigBase: tools.ConfigBase{
				Name:         "get_order_docs",
				Description:  "Get order documents",
				AuthRequired: []string{"user-auth", "admin-auth"},
			},
			Type:   "firestore-get-documents",
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
