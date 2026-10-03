// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mongodbcommon_test

import (
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/googleapis/mcp-toolbox/internal/tools/mongodb/mongodbcommon"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestValidateCollectionConfig(t *testing.T) {
	tcs := []struct {
		desc          string
		collection    string
		allowedValues []string
		wantErr       bool
	}{
		{"neither set", "", nil, false},
		{"only collection", "orders", nil, false},
		{"only allowedValues", "", []string{"orders", "customers"}, false},
		{"both set is an error", "orders", []string{"orders"}, true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			err := mongodbcommon.ValidateCollectionConfig(tc.collection, tc.allowedValues)
			if tc.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got: %s", err)
			}
		})
	}
}

func TestWithRuntimeCollectionParam(t *testing.T) {
	// When a collection is fixed in the config, no runtime parameter is added.
	if got := mongodbcommon.WithRuntimeCollectionParam("orders", nil, parameters.Parameters{}); len(got) != 0 {
		t.Fatalf("expected no injected parameter when collection is set, got %d", len(got))
	}

	// When collection is omitted, a required "collection" parameter is added.
	params := mongodbcommon.WithRuntimeCollectionParam("", nil, parameters.Parameters{})
	if len(params) != 1 {
		t.Fatalf("expected 1 injected parameter, got %d", len(params))
	}
	sp, ok := params[0].(*parameters.StringParameter)
	if !ok || sp.GetName() != "collection" {
		t.Fatalf("expected an injected string parameter named 'collection'")
	}
	if sp.Required == nil || !*sp.Required {
		t.Error("expected the injected collection parameter to be required")
	}
	if len(sp.AllowedValues) != 0 {
		t.Errorf("expected no allowed values, got %d", len(sp.AllowedValues))
	}

	// When allowedValues is provided, it is applied to the injected parameter.
	scoped := mongodbcommon.WithRuntimeCollectionParam("", []string{"orders", "customers"}, parameters.Parameters{})
	ssp, ok := scoped[0].(*parameters.StringParameter)
	if !ok {
		t.Fatal("expected an injected string parameter")
	}
	if len(ssp.AllowedValues) != 2 {
		t.Fatalf("expected 2 allowed values, got %d", len(ssp.AllowedValues))
	}
}

func TestResolveCollection(t *testing.T) {
	// Config value wins.
	got, err := mongodbcommon.ResolveCollection("orders", map[string]any{"collection": "ignored"})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got != "orders" {
		t.Fatalf("expected configured collection to win, got %q", got)
	}

	// Falls back to the runtime parameter.
	got, err = mongodbcommon.ResolveCollection("", map[string]any{"collection": "customers"})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got != "customers" {
		t.Fatalf("expected runtime collection, got %q", got)
	}

	// Errors when neither is available.
	if _, err := mongodbcommon.ResolveCollection("", map[string]any{}); err == nil {
		t.Fatal("expected an error when collection is missing, got nil")
	}
}

type fakeSource struct {
	allowed map[string]map[string]struct{}
}

func (f fakeSource) IsCollectionAllowed(database, collection string) bool {
	if len(f.allowed) == 0 {
		return true
	}
	_, ok := f.allowed[database][collection]
	return ok
}

func (f fakeSource) MongoDBAllowedCollections(database string) []string {
	if len(f.allowed) == 0 {
		return nil
	}
	names := make([]string, 0, len(f.allowed[database]))
	for c := range f.allowed[database] {
		names = append(names, c)
	}
	sort.Strings(names)
	return names
}

func TestEffectiveCollections(t *testing.T) {
	tcs := []struct {
		desc          string
		sourceAllowed []string
		toolAllowed   []string
		want          []string
	}{
		{"both unset", nil, nil, nil},
		{"tool only", nil, []string{"a", "b"}, []string{"a", "b"}},
		{"source only", []string{"a", "b", "c"}, nil, []string{"a", "b", "c"}},
		{"tool narrows source", []string{"a", "b", "c"}, []string{"a", "b"}, []string{"a", "b"}},
		{"disjoint", []string{"a", "b"}, []string{"c"}, []string{}},
		{"source allows nothing in this database", []string{}, []string{"a"}, []string{}},
		{"tool regex narrows source", []string{"customers", "order_1", "order_2"}, []string{"^order_.*$"}, []string{"order_1", "order_2"}},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, mongodbcommon.EffectiveCollections(tc.sourceAllowed, tc.toolAllowed)); diff != "" {
				t.Fatalf("unexpected collections (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateCollectionScope(t *testing.T) {
	unrestricted := fakeSource{}
	scoped := fakeSource{allowed: map[string]map[string]struct{}{
		"crm": {"a": {}, "b": {}, "c": {}},
	}}

	tcs := []struct {
		desc        string
		src         mongodbcommon.CollectionScopedSource
		database    string
		collection  string
		toolAllowed []string
		wantErr     bool
	}{
		{"unset source, fixed collection", unrestricted, "crm", "anything", nil, false},
		{"unset source, runtime collection", unrestricted, "crm", "", nil, false},
		{"unset source, runtime collection with tool values", unrestricted, "crm", "", []string{"a", "b"}, false},
		{"scoped source, fixed collection in list", scoped, "crm", "a", nil, false},
		{"scoped source, fixed collection outside list", scoped, "crm", "z", nil, true},
		{"scoped source, runtime collection", scoped, "crm", "", nil, false},
		{"scoped source, tool narrows to a subset", scoped, "crm", "", []string{"a", "b"}, false},
		{"scoped source, tool escapes the source scope", scoped, "crm", "", []string{"z"}, true},
		{"scoped source, database has no allowed collections", scoped, "other", "", nil, true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			err := mongodbcommon.ValidateCollectionScope(tc.src, tc.database, tc.collection, tc.toolAllowed)
			if tc.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got: %s", err)
			}
		})
	}
}

func TestScopeCollectionParam(t *testing.T) {
	base := mongodbcommon.WithRuntimeCollectionParam("", nil, parameters.Parameters{
		parameters.NewStringParameter("city", "a filter param"),
	})

	// An empty allow-list leaves the parameters untouched.
	if got := mongodbcommon.ScopeCollectionParam(base, nil); len(got[1].(*parameters.StringParameter).AllowedValues) != 0 {
		t.Error("expected no allowed values when the effective set is empty")
	}

	scoped := mongodbcommon.ScopeCollectionParam(base, []string{"orders", "customers"})
	sp, ok := scoped[1].(*parameters.StringParameter)
	if !ok || sp.GetName() != mongodbcommon.CollectionKey {
		t.Fatal("expected the collection parameter to be present")
	}
	if diff := cmp.Diff([]any{"orders", "customers"}, sp.AllowedValues); diff != "" {
		t.Fatalf("unexpected allowed values (-want +got):\n%s", diff)
	}
	if got := base[1].(*parameters.StringParameter).AllowedValues; len(got) != 0 {
		t.Error("expected the original parameters to be left unmodified")
	}
	if scoped[0] != base[0] {
		t.Error("expected non-collection parameters to be shared, not cloned")
	}
}
