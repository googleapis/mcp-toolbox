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

package mongodbinsertone_test

import (
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/mongodb/mongodbinsertone"
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
            type: mongodb-insert-one
            source: my-instance
            description: some description
            database: test_db
            collection: test_coll
			`,
			want: server.ToolConfigs{
				"example_tool": mongodbinsertone.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						AuthRequired: []string{},
						Description:  "some description",
					},
					Type:       "mongodb-insert-one",
					Source:     "my-instance",
					Database:   "test_db",
					Collection: "test_coll",
					Canonical:  false,
				},
			},
		},
		{
			desc: "true canonical",
			in: `
            kind: tool
            name: example_tool
            type: mongodb-insert-one
            source: my-instance
            description: some description
            database: test_db
            collection: test_coll
            canonical: true
			`,
			want: server.ToolConfigs{
				"example_tool": mongodbinsertone.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						AuthRequired: []string{},
						Description:  "some description",
					},
					Type:       "mongodb-insert-one",
					Source:     "my-instance",
					Database:   "test_db",
					Collection: "test_coll",
					Canonical:  true,
				},
			},
		},
		{
			desc: "false canonical",
			in: `
            kind: tool
            name: example_tool
            type: mongodb-insert-one
            source: my-instance
            description: some description
            database: test_db
            collection: test_coll
            canonical: false
			`,
			want: server.ToolConfigs{
				"example_tool": mongodbinsertone.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						AuthRequired: []string{},
						Description:  "some description",
					},
					Type:       "mongodb-insert-one",
					Source:     "my-instance",
					Database:   "test_db",
					Collection: "test_coll",
					Canonical:  false,
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
	// Test default annotations for destructive tool
	t.Run("default annotations", func(t *testing.T) {
		annotations := tools.GetAnnotationsOrDefault(nil, tools.NewDestructiveAnnotations)
		if annotations == nil {
			t.Fatal("expected non-nil annotations")
		}
		if annotations.DestructiveHint == nil || *annotations.DestructiveHint != true {
			t.Error("expected destructiveHint to be true")
		}
		if annotations.ReadOnlyHint == nil || *annotations.ReadOnlyHint != false {
			t.Error("expected readOnlyHint to be false")
		}
	})

	// Test custom annotations override default
	t.Run("custom annotations", func(t *testing.T) {
		customDestructive := false
		custom := &tools.ToolAnnotations{DestructiveHint: &customDestructive}
		annotations := tools.GetAnnotationsOrDefault(custom, tools.NewDestructiveAnnotations)
		if annotations.DestructiveHint == nil || *annotations.DestructiveHint != false {
			t.Error("expected custom destructiveHint to be false")
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
            type: mongodb-insert-one
            source: my-instance
            description: some description
            collection: test_coll
            canonical: true
			`,
			err: `unable to parse tool "example_tool" as type "mongodb-insert-one"`,
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
            type: mongodb-insert-one
            source: my-instance
            description: some description
            database: test_db
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
			cfg := mongodbinsertone.Config{
				ConfigBase:              tools.ConfigBase{Name: "example_tool", Description: "some description"},
				Collection:              tc.collection,
				CollectionAllowedValues: tc.allowedValues,
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
			params, err := tool.GetParameters(nil)
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
