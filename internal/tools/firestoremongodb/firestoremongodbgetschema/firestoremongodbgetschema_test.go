// Copyright 2026 Google LLC
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

package firestoremongodbgetschema_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/firestoremongodb/firestoremongodbgetschema"
)

func TestParseFromYamlFirestoreGetSchema(t *testing.T) {
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
			name: get_schema_tool
			type: firestore-mongodb-get-schema
			source: my-firestore-database
			description: Get schema from Firestore collections
			`,
			want: server.ToolConfigs{
				"get_schema_tool": firestoremongodbgetschema.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "get_schema_tool",
						Description:  "Get schema from Firestore collections",
						AuthRequired: []string{},
					},
					Type:   "firestore-mongodb-get-schema",
					Source: "my-firestore-database",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_get_schema
			type: firestore-mongodb-get-schema
			source: prod-firestore
			description: Get schema with authentication
			authRequired:
				- google-auth-service
			`,
			want: server.ToolConfigs{
				"secure_get_schema": firestoremongodbgetschema.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_get_schema",
						Description:  "Get schema with authentication",
						AuthRequired: []string{"google-auth-service"},
					},
					Type:   "firestore-mongodb-get-schema",
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

func TestAnnotations(t *testing.T) {
	t.Run("default read only annotations", func(t *testing.T) {
		annotations := tools.GetAnnotationsOrDefault(nil, tools.NewReadOnlyAnnotations)
		if annotations == nil {
			t.Fatal("expected non-nil annotations")
		}
		if annotations.ReadOnlyHint == nil || *annotations.ReadOnlyHint != true {
			t.Error("expected readOnlyHint to be true")
		}
	})

	t.Run("custom annotations override", func(t *testing.T) {
		readOnly := false
		custom := &tools.ToolAnnotations{ReadOnlyHint: &readOnly}
		annotations := tools.GetAnnotationsOrDefault(custom, tools.NewReadOnlyAnnotations)
		if annotations.ReadOnlyHint == nil || *annotations.ReadOnlyHint != false {
			t.Error("expected custom readOnlyHint to be false")
		}
	})
}

func TestFailParseFromYaml(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "missing source",
			in: `
			kind: tool
			name: get_schema_tool
			type: firestore-mongodb-get-schema
			description: Get schema
			`,
			err: "Field validation for 'Source' failed on the 'required' tag",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			if !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("unexpected error: got %q, want substring %q", err.Error(), tc.err)
			}
		})
	}
}
