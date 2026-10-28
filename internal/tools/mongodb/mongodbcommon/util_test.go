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
	"testing"

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
