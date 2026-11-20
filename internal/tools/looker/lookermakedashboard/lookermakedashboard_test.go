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

package lookermakedashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookermakedashboard"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

func TestParseFromYamlLookerMakeDashboard(t *testing.T) {
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
			type: looker-make-dashboard
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
					Type:   "looker-make-dashboard",
					Source: "my-instance",
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

func TestFailParseFromYamlMakeDashboard(t *testing.T) {
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
			type: looker-make-dashboard
			source: my-instance
			method: GOT
			description: some description
			`,
			err: "error unmarshaling tool: unable to parse tool \"example_tool\" as type \"looker-make-dashboard\": [3:1] unknown field \"method\"\n   1 | authRequired: []\n   2 | description: some description\n>  3 | method: GOT\n       ^\n   4 | name: example_tool\n   5 | source: my-instance\n   6 | type: looker-make-dashboard",
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

type mockSource struct {
	sources.Source
	baseURL string
	hostURL string
}

func (m mockSource) UseClientAuthorization() bool   { return false }
func (m mockSource) GetAuthTokenHeaderName() string { return "Authorization" }
func (m mockSource) LookerApiSettings() *rtl.ApiSettings {
	return &rtl.ApiSettings{BaseUrl: m.baseURL, ApiVersion: "4.0"}
}
func (m mockSource) GetLookerSDK(ctx context.Context, s string) (*v4.LookerSDK, error) {
	return v4.NewLookerSDK(rtl.NewAuthSession(rtl.ApiSettings{
		BaseUrl:      m.baseURL,
		ApiVersion:   "4.0",
		ClientId:     "test-id",
		ClientSecret: "test-secret",
	})), nil
}
func (m mockSource) GetHostURL(ctx context.Context, sdk *v4.LookerSDK) (string, error) {
	return m.hostURL, nil
}

func TestInvokeMakeDashboardReturnsLayoutID(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	var layoutsResp []map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/4.0/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/4.0/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "1", "personal_folder_id": "10"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/4.0/folders/10/dashboards":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/4.0/dashboards":
			resp := map[string]any{
				"id":  "dash-42",
				"url": "/dashboards/dash-42",
			}
			if layoutsResp != nil {
				resp["dashboard_layouts"] = layoutsResp
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "make_dashboard",
			Description: "test description",
		},
		Type:   "looker-make-dashboard",
		Source: "my-instance",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	src := mockSource{baseURL: ts.URL, hostURL: "https://looker.example.com"}

	// Case 1: Active layout preferred when multiple layouts returned
	layoutsResp = []map[string]any{
		{"id": "layout-inactive", "active": false, "type": "newspaper"},
		{"id": "layout-active", "active": true, "type": "newspaper"},
	}
	res, invokeErr := tool.Invoke(ctx, src, parameters.ParamValues{
		{Name: "title", Value: "Test Dash"},
		{Name: "description", Value: "desc"},
		{Name: "folder", Value: ""},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected error: %v", invokeErr)
	}
	want := map[string]any{
		"id":                  "dash-42",
		"url":                 "https://looker.example.com/dashboards/dash-42",
		"dashboard_layout_id": "layout-active",
	}
	if diff := cmp.Diff(want, res); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	// Case 2: Omitted when dashboard_layouts is empty
	layoutsResp = nil
	res, invokeErr = tool.Invoke(ctx, src, parameters.ParamValues{
		{Name: "title", Value: "Test Dash 2"},
		{Name: "description", Value: ""},
		{Name: "folder", Value: ""},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected error: %v", invokeErr)
	}
	wantNoLayout := map[string]any{
		"id":  "dash-42",
		"url": "https://looker.example.com/dashboards/dash-42",
	}
	if diff := cmp.Diff(wantNoLayout, res); diff != "" {
		t.Errorf("result mismatch without layouts (-want +got):\n%s", diff)
	}
}
