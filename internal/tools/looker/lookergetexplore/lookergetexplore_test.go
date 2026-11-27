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

package lookergetexplore_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources/looker"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetexplore"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

func TestParseFromYamlLookerGetExplore(t *testing.T) {
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
            type: looker-get-explore
            source: my-instance
            description: some description
				`,
			want: server.ToolConfigs{
				"example_tool": lkr.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "looker-get-explore",
					Source: "my-instance",
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

func TestFailParseFromYamlLookerGetExplore(t *testing.T) {
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
            type: looker-get-explore
            source: my-instance
            method: GOT
            description: some description
			`,
			err: "error unmarshaling tool: unable to parse tool \"example_tool\" as type \"looker-get-explore\": [3:1] unknown field \"method\"\n   1 | authRequired: []\n   2 | description: some description\n>  3 | method: GOT\n       ^\n   4 | name: example_tool\n   5 | source: my-instance\n   6 | type: looker-get-explore",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
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

func TestInvokeLookerGetExplore(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithUserAgent(ctx, "test-agent")

	var requestedFields string
	var requestedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		requestedFields = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/api/4.0/lookml_models/sales_model/explores/orders":
			_, _ = w.Write([]byte(`{
				"name": "orders",
				"description": "Certified orders explore",
				"label": "Orders",
				"group_label": "Sales",
				"hidden": false,
				"tags": ["golden", "finance"],
				"always_filter": [{"name": "orders.created_date", "value": "30 days"}],
				"certification_metadata": {
					"certification_status": "certified",
					"user_name": "Jane Doe",
					"notes": "Verified by Data Platform",
					"updated_at": "2026-09-15T10:00:00Z"
				}
			}`))
		case "/api/4.0/lookml_models/sales_model/explores/legacy_orders":
			_, _ = w.Write([]byte(`{
				"name": "legacy_orders",
				"description": "Unverified legacy explore",
				"hidden": false
			}`))
		case "/api/4.0/lookml_models/sales_model/explores/hidden_orders":
			_, _ = w.Write([]byte(`{
				"name": "hidden_orders",
				"hidden": true
			}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	srcCfg := looker.Config{
		Name:            "test-looker",
		Type:            "looker",
		BaseURL:         ts.URL,
		UseClientOAuth:  "true",
		Timeout:         "5s",
		SslVerification: false,
	}
	src, err := srcCfg.Initialize(ctx, nil)
	if err != nil {
		t.Fatalf("failed to initialize source: %v", err)
	}

	toolCfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "get_explore",
			Description: "test description",
		},
		Type:   "looker-get-explore",
		Source: "test-looker",
	}
	tool, err := toolCfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	t.Run("certified explore includes certification_metadata", func(t *testing.T) {
		params := parameters.ParamValues{
			{Name: "model", Value: "sales_model"},
			{Name: "explore", Value: "orders"},
		}
		got, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr != nil {
			t.Fatalf("unexpected invoke error: %v", toolboxErr)
		}

		wantPath := "/api/4.0/lookml_models/sales_model/explores/orders"
		if requestedPath != wantPath {
			t.Errorf("expected path %q, got %q", wantPath, requestedPath)
		}
		wantFields := "name,description,label,group_label,hidden,tags,always_filter,conditionally_filter,certification_metadata"
		if requestedFields != wantFields {
			t.Errorf("expected fields query param %q, got %q", wantFields, requestedFields)
		}

		gotMap, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any, got %T", got)
		}
		if gotMap["name"] != "orders" || gotMap["label"] != "Orders" {
			t.Errorf("unexpected explore fields: %v", gotMap)
		}

		cert, ok := gotMap["certification_metadata"].(*v4.Certification)
		if !ok || cert == nil {
			t.Fatalf("expected certification_metadata in result, got %#v", gotMap["certification_metadata"])
		}
		if cert.CertificationStatus == nil || *cert.CertificationStatus != v4.CertificationStatus_Certified {
			t.Errorf("expected certification_status 'certified', got %v", cert.CertificationStatus)
		}
		if cert.UserName == nil || *cert.UserName != "Jane Doe" {
			t.Errorf("expected user_name 'Jane Doe', got %v", cert.UserName)
		}
		if cert.Notes == nil || *cert.Notes != "Verified by Data Platform" {
			t.Errorf("expected notes 'Verified by Data Platform', got %v", cert.Notes)
		}
		wantTime, _ := time.Parse(time.RFC3339, "2026-09-15T10:00:00Z")
		if cert.UpdatedAt == nil || !cert.UpdatedAt.Equal(wantTime) {
			t.Errorf("expected updated_at %v, got %v", wantTime, cert.UpdatedAt)
		}
	})

	t.Run("explore without certification_metadata omits key", func(t *testing.T) {
		params := parameters.ParamValues{
			{Name: "model", Value: "sales_model"},
			{Name: "explore", Value: "legacy_orders"},
		}
		got, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr != nil {
			t.Fatalf("unexpected invoke error: %v", toolboxErr)
		}

		gotMap, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any, got %T", got)
		}
		if gotMap["name"] != "legacy_orders" {
			t.Errorf("expected name 'legacy_orders', got %v", gotMap["name"])
		}
		if _, exists := gotMap["certification_metadata"]; exists {
			t.Errorf("expected certification_metadata to be omitted when nil, got %v", gotMap["certification_metadata"])
		}
	})

	t.Run("hidden explore returns agent error when show_hidden_explores is false", func(t *testing.T) {
		params := parameters.ParamValues{
			{Name: "model", Value: "sales_model"},
			{Name: "explore", Value: "hidden_orders"},
		}
		_, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr == nil {
			t.Fatal("expected error for hidden explore, got nil")
		}
	})
}
