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

package mongodbfind_test

import (
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/mongodb/mongodbcommon"
	"github.com/googleapis/mcp-toolbox/internal/tools/mongodb/mongodbfind"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlMongoQuery(t *testing.T) {
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
            name: example_tool
            type: mongodb-find
            source: my-instance
            description: some description
            database: test_db
            collection: test_coll
            filterPayload: |
                { name: {{json .name}} }
            filterParams:
                - name: name 
                  type: string
                  description: small description
            projectPayload: |
              { name: 1, age: 1 }
            projectParams: []
            sortPayload: |
              { timestamp: -1 }
            sortParams: []
			`,
			want: server.ToolConfigs{
				"example_tool": mongodbfind.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						AuthRequired: []string{},
						Description:  "some description",
					},
					Type:          "mongodb-find",
					Source:        "my-instance",
					Database:      "test_db",
					Collection:    "test_coll",
					FilterPayload: "{ name: {{json .name}} }\n",
					FilterParams: parameters.Parameters{
						&parameters.StringParameter{
							CommonParameter: parameters.CommonParameter{
								Name: "name",
								Type: "string",
								Desc: "small description",
							},
						},
					},
					ProjectPayload: "{ name: 1, age: 1 }\n",
					ProjectParams:  parameters.Parameters{},
					SortPayload:    "{ timestamp: -1 }\n",
					SortParams:     parameters.Parameters{},
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
	// Test default annotations for read-only tool
	t.Run("default annotations", func(t *testing.T) {
		annotations := tools.GetAnnotationsOrDefault(nil, tools.NewReadOnlyAnnotations)
		if annotations == nil {
			t.Fatal("expected non-nil annotations")
		}
		if annotations.ReadOnlyHint == nil || *annotations.ReadOnlyHint != true {
			t.Error("expected readOnlyHint to be true")
		}
	})

	// Test custom annotations override default
	t.Run("custom annotations", func(t *testing.T) {
		customReadOnly := false
		custom := &tools.ToolAnnotations{ReadOnlyHint: &customReadOnly}
		annotations := tools.GetAnnotationsOrDefault(custom, tools.NewReadOnlyAnnotations)
		if annotations.ReadOnlyHint == nil || *annotations.ReadOnlyHint != false {
			t.Error("expected custom readOnlyHint to be false")
		}
	})
}

func TestFailParseFromYamlMongoQuery(t *testing.T) {
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
			desc: "Invalid method",
			in: `
            kind: tool
            name: example_tool
            type: mongodb-find
            source: my-instance
            description: some description
            collection: test_coll
            filterPayload: |
              { name : {{json .name}} }
			`,
			err: `unable to parse tool "example_tool" as type "mongodb-find"`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.err) {
				t.Fatalf("unexpected error string: got %q, want substring %q", errStr, tc.err)
			}
		})
	}

}

func collectionParam(params parameters.Parameters) *parameters.StringParameter {
	for _, p := range params {
		if sp, ok := p.(*parameters.StringParameter); ok && sp.GetName() == "collection" {
			return sp
		}
	}
	return nil
}

var noCollectionConfig = `
            kind: tool
            name: example_tool
            type: mongodb-find
            source: my-instance
            description: some description
            database: test_db
            filterPayload: |
                { name: {{json .name}} }
`

func TestRuntimeCollection(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// collection is optional now, so a config without it should still parse.
	if _, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(noCollectionConfig)); err != nil {
		t.Fatalf("expected config without collection to parse, got: %s", err)
	}

	tcs := []struct {
		desc          string
		collection    string
		allowedValues []string
		wantParam     bool
		wantAllowed   int
		wantErr       bool
	}{
		{"omitted exposes a required runtime param", "", nil, true, 0, false},
		{"omitted with allowed values restricts the param", "", []string{"orders", "customers"}, true, 2, false},
		{"set in config exposes no runtime param", "test_coll", nil, false, 0, false},
		{"collection and allowedValues together is an error", "test_coll", []string{"orders"}, false, 0, true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := mongodbfind.Config{
				ConfigBase:              tools.ConfigBase{Name: "example_tool", Description: "some description"},
				Collection:              tc.collection,
				CollectionAllowedValues: tc.allowedValues,
				Limit:                   1,
			}
			tool, err := cfg.Initialize(ctx)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error when collection and collectionAllowedValues are both set")
				}
				return
			}
			if err != nil {
				t.Fatalf("unable to initialize tool: %s", err)
			}
			params, err := tool.GetParameters(&mongodbcommon.MockSource{})
			if err != nil {
				t.Fatalf("unable to get parameters: %s", err)
			}
			p := collectionParam(params)
			if !tc.wantParam {
				if p != nil {
					t.Error("did not expect a collection parameter when collection is set in config")
				}
				return
			}
			if p == nil {
				t.Fatal("expected a runtime collection parameter when collection is omitted")
			}
			if p.Required == nil || !*p.Required {
				t.Error("expected the runtime collection parameter to be required")
			}
			if len(p.AllowedValues) != tc.wantAllowed {
				t.Errorf("expected %d allowed values, got %d", tc.wantAllowed, len(p.AllowedValues))
			}
		})
	}
}

func TestCollectionScoping(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	scoped := &mongodbcommon.MockSource{AllowedCollections: map[string][]string{"crm": {"customers", "orders"}}}

	newTool := func(t *testing.T, collection string, allowedValues []string) tools.Tool {
		t.Helper()
		cfg := mongodbfind.Config{
			ConfigBase:              tools.ConfigBase{Name: "example_tool", Description: "some description"},
			Database:                "crm",
			Collection:              collection,
			CollectionAllowedValues: allowedValues,
			Limit:                   1,
		}
		tool, err := cfg.Initialize(ctx)
		if err != nil {
			t.Fatalf("unable to initialize tool: %s", err)
		}
		return tool
	}

	t.Run("fixed collection inside the source allow-list is accepted", func(t *testing.T) {
		if err := newTool(t, "orders", nil).ValidateSource(scoped); err != nil {
			t.Fatalf("expected no error, got: %s", err)
		}
	})

	t.Run("fixed collection outside the source allow-list is rejected", func(t *testing.T) {
		if err := newTool(t, "secrets", nil).ValidateSource(scoped); err == nil {
			t.Fatal("expected ValidateSource to reject a collection outside the source allow-list")
		}
	})

	t.Run("tool values disjoint from the source allow-list are rejected", func(t *testing.T) {
		if err := newTool(t, "", []string{"secrets"}).ValidateSource(scoped); err == nil {
			t.Fatal("expected ValidateSource to reject a tool scope outside the source allow-list")
		}
	})

	t.Run("runtime parameter is narrowed to the source allow-list", func(t *testing.T) {
		params, err := newTool(t, "", nil).GetParameters(scoped)
		if err != nil {
			t.Fatalf("unable to get parameters: %s", err)
		}
		if diff := cmp.Diff([]any{"customers", "orders"}, collectionParam(params).AllowedValues); diff != "" {
			t.Fatalf("unexpected allowed values (-want +got):\n%s", diff)
		}
	})

	t.Run("runtime parameter is narrowed to the intersection", func(t *testing.T) {
		params, err := newTool(t, "", []string{"orders", "secrets"}).GetParameters(scoped)
		if err != nil {
			t.Fatalf("unable to get parameters: %s", err)
		}
		if diff := cmp.Diff([]any{"orders"}, collectionParam(params).AllowedValues); diff != "" {
			t.Fatalf("unexpected allowed values (-want +got):\n%s", diff)
		}
	})

	t.Run("manifest carries the narrowed parameter", func(t *testing.T) {
		manifest, err := newTool(t, "", nil).Manifest(scoped)
		if err != nil {
			t.Fatalf("unable to get manifest: %s", err)
		}
		for _, p := range manifest.Parameters {
			if p.Name == "collection" {
				return
			}
		}
		t.Fatal("expected the collection parameter in the manifest")
	})

	t.Run("unrestricted source leaves the tool scope untouched", func(t *testing.T) {
		params, err := newTool(t, "", []string{"anything"}).GetParameters(&mongodbcommon.MockSource{})
		if err != nil {
			t.Fatalf("unable to get parameters: %s", err)
		}
		if diff := cmp.Diff([]any{"anything"}, collectionParam(params).AllowedValues); diff != "" {
			t.Fatalf("unexpected allowed values (-want +got):\n%s", diff)
		}
	})
}
