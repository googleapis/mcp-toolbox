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
package lookerrenderdashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerrenderdashboard"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

type mockDashLookerSource struct {
	settings *rtl.ApiSettings
	sdk      *v4.LookerSDK
}

func (m mockDashLookerSource) SourceType() string                  { return "looker" }
func (m mockDashLookerSource) ToConfig() sources.SourceConfig      { return nil }
func (m mockDashLookerSource) IsReadOnly() bool                    { return false }
func (m mockDashLookerSource) UseClientAuthorization() bool        { return true }
func (m mockDashLookerSource) GetAuthTokenHeaderName() string      { return "X-Looker-Token" }
func (m mockDashLookerSource) LookerApiSettings() *rtl.ApiSettings { return m.settings }
func (m mockDashLookerSource) GetLookerSDK(context.Context, string) (*v4.LookerSDK, error) {
	return m.sdk, nil
}

type incompatibleSource struct{}

func (incompatibleSource) SourceType() string             { return "other" }
func (incompatibleSource) ToConfig() sources.SourceConfig { return nil }
func (incompatibleSource) IsReadOnly() bool               { return false }

func TestParseFromYamlLookerRenderDashboard(t *testing.T) {
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
			name: render_dashboard
			type: looker-render-dashboard
			source: my-instance
			description: Render Looker dashboard
			`,
			want: server.ToolConfigs{
				"render_dashboard": lkr.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "render_dashboard",
						Description:  "Render Looker dashboard",
						AuthRequired: []string{},
					},
					Type:   "looker-render-dashboard",
					Source: "my-instance",
				},
			},
		},
		{
			desc: "example with custom UI metadata",
			in: `
			kind: tool
			name: render_dashboard
			type: looker-render-dashboard
			source: my-instance
			description: Render Looker dashboard
			ui:
			  resource: custom_dash
			  visibility:
			    - model
			    - app
			`,
			want: server.ToolConfigs{
				"render_dashboard": lkr.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "render_dashboard",
						Description:  "Render Looker dashboard",
						AuthRequired: []string{},
						UI: &tools.ToolUIMetadata{
							Resource:   "custom_dash",
							Visibility: []tools.ToolVisibility{tools.VisibilityModel, tools.VisibilityApp},
						},
					},
					Type:   "looker-render-dashboard",
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

func TestInitializeAndResourceProvider(t *testing.T) {
	ctx := context.Background()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Looker Dashboard</body></html>"))
	}))
	defer ts.Close()

	t.Setenv("LOOKER_UI_URL", ts.URL)

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_dashboard",
			Description: "Render dashboard tool",
		},
		Type:   "looker-render-dashboard",
		Source: "my-instance",
	}

	toolInstance, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if toolInstance.GetToolUIMetadata() == nil {
		t.Fatalf("expected UI meta to be populated, got nil")
	}
	if toolInstance.GetToolUIMetadata().Resource != "looker_render_dashboard_ui" {
		t.Errorf("expected default resource name looker_render_dashboard_ui, got %s", toolInstance.GetToolUIMetadata().Resource)
	}

	rp, ok := toolInstance.(resources.ResourceProvider)
	if !ok {
		t.Fatalf("expected toolInstance to implement ResourceProvider")
	}

	resList := rp.GetResources()
	if len(resList) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resList))
	}
	if resList[0].GetURI() != "ui://looker/render_dashboard.html" {
		t.Errorf("expected URI ui://looker/render_dashboard.html, got %s", resList[0].GetURI())
	}
	if resList[0].GetMimeType() != "text/html;profile=mcp-app" {
		t.Errorf("expected MimeType text/html;profile=mcp-app, got %s", resList[0].GetMimeType())
	}

	content, err := resList[0].Read(ctx, nil)
	if err != nil {
		t.Fatalf("error reading resource: %v", err)
	}
	contentStr, ok := content.(string)
	if !ok || !strings.Contains(contentStr, "Looker Dashboard") {
		t.Errorf("expected HTML content to contain Looker Dashboard, got %v", content)
	}
}

func TestRemoteUIPublicOriginDiscovery(t *testing.T) {
	ctx := context.Background()

	// Simulates the managed MCP server: toolbox fetches the UI over an internal
	// address, but Looker declares its public Host URL in the entry script src.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><head>` +
			`<style>@font-face { src: url('/fonts/roboto.woff2'); }</style>` +
			`<script nonce="n" src="https://looker.example.com/webpack/en/mcp-render-dashboard.webpack.js?v=1"></script>` +
			`</head><body></body></html>`))
	}))
	defer ts.Close()

	t.Setenv("LOOKER_UI_URL", ts.URL)
	t.Setenv("LOOKER_BASE_URL", "")

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{Name: "render_dashboard", Description: "Render dashboard tool"},
		Type:       "looker-render-dashboard",
		Source:     "my-instance",
	}
	toolInstance, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	res, ok := toolInstance.(resources.ResourceProvider).GetResources()[0].(*resources.DynamicUIResource)
	if !ok {
		t.Fatalf("expected *resources.DynamicUIResource")
	}

	content, err := res.Read(ctx, nil)
	if err != nil {
		t.Fatalf("failed to read resource: %v", err)
	}
	html := content.(string)
	if !strings.Contains(html, `url('https://looker.example.com/fonts/roboto.woff2')`) {
		t.Errorf("expected relative font URL to be rewritten to the public origin, got %s", html)
	}
	if strings.Contains(html, "nonce=") {
		t.Errorf("expected nonce attributes to be stripped, got %s", html)
	}

	if res.PrefersBorder == nil || !*res.PrefersBorder {
		t.Errorf("expected PrefersBorder to be true, got %v", res.PrefersBorder)
	}
	csp := res.CSP
	for _, want := range []string{
		"https://looker.example.com",
		"https://maps.googleapis.com",
		"https://mapsresources-pa.googleapis.com",
		"https://streetviewpixels-pa.googleapis.com",
		"https://khms0.googleapis.com",
		"https://khms1.googleapis.com",
		"https://maps.gstatic.com",
		"https://*.gstatic.com",
		"https://*.ggpht.com",
		"https://fonts.googleapis.com",
		"https://fonts.gstatic.com",
	} {
		if !contains(csp.ResourceDomains, want) {
			t.Errorf("expected ResourceDomains to contain %q, got %v", want, csp.ResourceDomains)
		}
	}
	for _, forbidden := range []string{
		"https://*.looker.com",
		"https://*.googleapis.com",
		"https://*.googleusercontent.com",
	} {
		if contains(csp.ResourceDomains, forbidden) {
			t.Errorf("expected ResourceDomains NOT to contain wildcard %q, got %v", forbidden, csp.ResourceDomains)
		}
		if contains(csp.ConnectDomains, forbidden) {
			t.Errorf("expected ConnectDomains NOT to contain wildcard %q, got %v", forbidden, csp.ConnectDomains)
		}
	}
	for _, want := range []string{
		"https://looker.example.com",
		"wss://looker.example.com",
		"https://maps.googleapis.com",
		"https://mapsresources-pa.googleapis.com",
		"https://streetviewpixels-pa.googleapis.com",
		"https://khms0.googleapis.com",
		"https://khms1.googleapis.com",
		"https://maps.gstatic.com",
		"https://*.ggpht.com",
		"https://fonts.googleapis.com",
		"https://fonts.gstatic.com",
	} {
		if !contains(csp.ConnectDomains, want) {
			t.Errorf("expected ConnectDomains to contain %q, got %v", want, csp.ConnectDomains)
		}
	}
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

func TestRemoteUIFetching(t *testing.T) {
	ctx := context.Background()

	// 1. Success with Remote Server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Looker Dashboard Scene</body></html>"))
	}))
	defer ts.Close()

	t.Setenv("LOOKER_UI_URL", ts.URL)

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_dashboard",
			Description: "Render dashboard tool",
		},
		Type:   "looker-render-dashboard",
		Source: "my-instance",
	}

	toolInstance, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	rp := toolInstance.(resources.ResourceProvider)
	content, err := rp.GetResources()[0].Read(ctx, nil)
	if err != nil {
		t.Fatalf("failed to read resource: %v", err)
	}
	contentStr, ok := content.(string)
	if !ok || !strings.Contains(contentStr, "Looker Dashboard Scene") {
		t.Errorf("expected fetched content, got %v", content)
	}

	// 2. Admin disabled (403 Forbidden)
	tsForbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer tsForbidden.Close()

	t.Setenv("LOOKER_UI_URL", tsForbidden.URL)

	cfgForbidden := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_dashboard",
			Description: "Render dashboard tool",
		},
		Type:   "looker-render-dashboard",
		Source: "my-instance",
	}

	toolForbidden, err := cfgForbidden.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	rpForbidden := toolForbidden.(resources.ResourceProvider)
	_, err = rpForbidden.GetResources()[0].Read(ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "disabled by administrator") {
		t.Errorf("expected disabled by administrator error, got %v", err)
	}

	// 3. Error on 404
	tsNotFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tsNotFound.Close()

	t.Setenv("LOOKER_UI_URL", tsNotFound.URL)

	cfgNotFound := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_dashboard",
			Description: "Render dashboard tool",
		},
		Type:   "looker-render-dashboard",
		Source: "my-instance",
	}

	toolNotFound, err := cfgNotFound.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	rpNotFound := toolNotFound.(resources.ResourceProvider)
	_, err = rpNotFound.GetResources()[0].Read(ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("expected HTTP 404 error, got %v", err)
	}

	// 4. Error when no URL configured
	t.Setenv("LOOKER_UI_URL", "")
	t.Setenv("LOOKER_BASE_URL", "")

	cfgNoURL := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_dashboard",
			Description: "Render dashboard tool",
		},
		Type:   "looker-render-dashboard",
		Source: "my-instance",
	}
	toolNoURL, err := cfgNoURL.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	rpNoURL := toolNoURL.(resources.ResourceProvider)
	_, err = rpNoURL.GetResources()[0].Read(ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "no Looker instance URL configured") {
		t.Errorf("expected no Looker instance URL configured error, got %v", err)
	}
}

func TestPruneDashboardMapPreservesComponentGroupIDs(t *testing.T) {
	rawDashboard := map[string]any{
		"id":    "30",
		"title": "Dashboard with Tile Group",
		"dashboard_layouts": []any{
			map[string]any{
				"id": "1",
				"dashboard_layout_components": []any{
					map[string]any{
						"id":                   "100",
						"dashboard_element_id": "50",
						"row":                  float64(0),
						"column":               float64(0),
						"width":                float64(24),
						"height":               float64(9),
					},
					map[string]any{
						"id":                   "101",
						"dashboard_element_id": "51",
						"group_id":             "100",
						"row":                  float64(0),
						"column":               float64(0),
						"width":                float64(12),
						"height":               float64(8),
					},
				},
			},
		},
	}

	lkr.PruneDashboardMap(rawDashboard)

	layouts := rawDashboard["dashboard_layouts"].([]any)
	comps := layouts[0].(map[string]any)["dashboard_layout_components"].([]any)
	comp0 := comps[0].(map[string]any)
	comp1 := comps[1].(map[string]any)
	if _, hasGroup := comp0["group_id"]; hasGroup {
		t.Errorf("expected comp 100 not to have group_id, got %v", comp0["group_id"])
	}
	if comp1["group_id"] != "100" {
		t.Errorf("expected comp 101 to preserve group_id \"100\", got %v", comp1["group_id"])
	}
}

func TestPruneDashboardMap(t *testing.T) {
	rawDash := map[string]any{
		"id":    "30",
		"title": "Large Dashboard",
		"can":   map[string]any{"show": true, "update": true},
		"dashboard_elements": []any{
			map[string]any{
				"id":    "1",
				"title": "Revenue Tile",
				"can":   map[string]any{"explore": true},
				"query": map[string]any{
					"id":            "999",
					"filter_config": map[string]any{"huge": "blob"},
				},
				"result_maker": map[string]any{
					"id": "500",
					"vis_config": map[string]any{
						"type":         "looker_column",
						"query_fields": map[string]any{"measures": []any{"huge"}},
					},
					"query": map[string]any{
						"id":                 "999",
						"client_id":          "abc123slug",
						"model":              "thelook",
						"view":               "order_items",
						"vis_config":         map[string]any{"type": "looker_column"},
						"filter_config":      map[string]any{"huge": "blob"},
						"expanded_share_url": "https://looker.example.com/explore/thelook/order_items?fields=...",
						"url":                "https://looker.example.com/x/abc123slug",
						"can":                map[string]any{"run": true},
					},
				},
			},
		},
		"dashboard_filters": []any{
			map[string]any{
				"id":    "10",
				"name":  "Date",
				"title": "Date",
				"can":   map[string]any{"index": true},
				"field": map[string]any{
					"name":         "orders.created_date",
					"type":         "date_date",
					"sql":          "${TABLE}.created_at",
					"drill_fields": []any{"orders.id"},
					"can":          map[string]any{"filter": true},
				},
			},
		},
	}

	lkr.PruneDashboardMap(rawDash)
	pruned := rawDash
	if _, hasCan := pruned["can"]; !hasCan {
		t.Errorf("expected top-level dashboard can to be preserved")
	}

	el := pruned["dashboard_elements"].([]any)[0].(map[string]any)
	if _, hasElCan := el["can"]; !hasElCan {
		t.Errorf("expected element.can to be preserved")
	}
	if _, hasQuery := el["query"]; hasQuery {
		t.Errorf("expected duplicate element.query to be pruned when result_maker.query exists")
	}
	rm := el["result_maker"].(map[string]any)
	rmVisCfg := rm["vis_config"].(map[string]any)
	if _, hasQueryFields := rmVisCfg["query_fields"]; hasQueryFields {
		t.Errorf("expected result_maker.vis_config.query_fields to be pruned")
	}
	rmQuery := rm["query"].(map[string]any)
	for _, stripped := range []string{"filter_config", "vis_config", "expanded_share_url", "url"} {
		if _, exists := rmQuery[stripped]; exists {
			t.Errorf("expected result_maker.query.%s to be pruned", stripped)
		}
	}
	if _, hasQueryCan := rmQuery["can"]; !hasQueryCan {
		t.Errorf("expected result_maker.query.can to be preserved")
	}
	if rmQuery["client_id"] != "abc123slug" {
		t.Errorf("expected result_maker.query.client_id to be preserved, got %v", rmQuery["client_id"])
	}

	filter := pruned["dashboard_filters"].([]any)[0].(map[string]any)
	field := filter["field"].(map[string]any)
	for _, stripped := range []string{"sql", "drill_fields", "can"} {
		if _, exists := field[stripped]; exists {
			t.Errorf("expected filter.field.%s to be pruned", stripped)
		}
	}
	if field["name"] != "orders.created_date" || field["type"] != "date_date" {
		t.Errorf("expected essential filter.field properties to be preserved, got %v", field)
	}
}

func TestInvokeRenderDashboardSuccessAndFilterNormalization(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dashJSON := `{
		"id": "30",
		"title": "Executive Overview",
		"can": {"show": true, "explore": true},
		"dashboard_filters": [
			{"id": "1", "name": "State", "title": "State", "dimension": "users.state", "field": {"name": "users.state", "type": "string", "sql": "${TABLE}.state"}},
			{"id": "2", "name": "Region", "title": "Customer Region", "dimension": "users.region"},
			{"id": "3", "name": "Created Date", "title": "Order Date", "dimension": "orders.created_date"},
			{"id": "4", "name": "Country", "title": "Country", "dimension": "users.country"}
		],
		"dashboard_elements": [
			{
				"id": "10",
				"title": "Orders by Status",
				"can": {"explore": true},
				"result_maker": {
					"id": "500",
					"query": {
						"id": "999",
						"model": "thelook",
						"view": "orders",
						"fields": ["orders.status", "orders.traffic_source", "orders.count"],
						"filter_config": {"unused": "blob"}
					},
					"filterables": [
						{
							"listen": [
								{"dashboard_filter_name": "Status", "field": "orders.status"}
							]
						}
					]
				}
			}
		]
	}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/4.0/dashboards/30":
			_, _ = w.Write([]byte(dashJSON))
		case r.URL.Path == "/api/4.0/user":
			_, _ = w.Write([]byte(`{"id":"1","locale":"ja_JP"}`))
		case r.URL.Path == "/api/4.0/color_collections/active":
			_, _ = w.Write([]byte(`{"id":"default-collection"}`))
		case r.URL.Path == "/public/mcp/ui/map_api_key":
			_, _ = w.Write([]byte(`{"api_key":"test-maps-key"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	settings := &rtl.ApiSettings{BaseUrl: ts.URL}
	session := rtl.NewAuthSession(*settings)
	session.Client = *ts.Client()
	sdk := v4.NewLookerSDK(session)

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_dashboard",
			Description: "Render dashboard tool",
		},
		Type:   "looker-render-dashboard",
		Source: "my-instance",
	}
	toolInstance, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	params := parameters.ParamValues{
		{Name: "dashboard_id", Value: "  30  "},
		{Name: "filters", Value: map[string]any{
			"state":                 "California",
			"Customer Region":       "West",
			"order_date":            "last 30 days",
			"users.country":         "USA",
			"orders_status":         "complete",
			"orders_traffic_source": "Search",
		}},
	}
	res, tbErr := toolInstance.Invoke(ctx, mockDashLookerSource{settings: settings, sdk: sdk}, params, "")
	if tbErr != nil {
		t.Fatalf("Invoke failed: %v", tbErr)
	}

	structured, ok := res.(tools.StructuredToolResult)
	if !ok {
		t.Fatalf("expected StructuredToolResult, got %T", res)
	}

	wantFilters := map[string]any{
		"State":                 "California",
		"Region":                "West",
		"Created Date":          "last 30 days",
		"Country":               "USA",
		"Status":                "complete",
		"orders.traffic_source": "Search",
	}

	summary, ok := structured.Content.(map[string]any)
	if !ok {
		t.Fatalf("expected summary Content map, got %T", structured.Content)
	}
	if summary["dashboard_id"] != "30" || summary["title"] != "Executive Overview" || summary["tile_count"] != 1 || summary["filter_count"] != 4 || summary["status"] != "rendered_in_ui" {
		t.Errorf("unexpected summary content: %v", summary)
	}
	if diff := cmp.Diff(wantFilters, summary["applied_filters"]); diff != "" {
		t.Errorf("applied_filters mismatch (-want +got):\n%s", diff)
	}

	dashData, ok := structured.StructuredContent["dashboardData"].(map[string]any)
	if !ok {
		t.Fatalf("expected dashboardData map in StructuredContent, got %v", structured.StructuredContent)
	}
	if diff := cmp.Diff(wantFilters, dashData["filters"]); diff != "" {
		t.Errorf("dashboardData.filters mismatch (-want +got):\n%s", diff)
	}
	if structured.StructuredContent["dashboard_id"] != "30" {
		t.Errorf("expected dashboard_id=\"30\", got %v", structured.StructuredContent["dashboard_id"])
	}
	appState, ok := structured.StructuredContent["app_state"].(map[string]any)
	if !ok || appState["map_api_key"] != "test-maps-key" {
		t.Errorf("expected app_state with map_api_key, got %v", structured.StructuredContent["app_state"])
	}
	if userMap, ok := appState["user"].(map[string]any); !ok || userMap["locale"] != "ja_JP" {
		t.Errorf("expected app_state.user.locale=ja_JP, got %v", appState["user"])
	}
}

func TestInvokeValidationAndAPIErrors(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not found"}`))
	}))
	defer ts.Close()

	settings := &rtl.ApiSettings{BaseUrl: ts.URL}
	session := rtl.NewAuthSession(*settings)
	session.Client = *ts.Client()
	sdk := v4.NewLookerSDK(session)
	src := mockDashLookerSource{settings: settings, sdk: sdk}

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_dashboard",
			Description: "Render dashboard tool",
		},
		Type:   "looker-render-dashboard",
		Source: "my-instance",
	}
	toolInstance, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	tcs := []struct {
		name    string
		source  sources.Source
		params  parameters.ParamValues
		wantErr string
	}{
		{
			name:    "incompatible source",
			source:  incompatibleSource{},
			params:  parameters.ParamValues{{Name: "dashboard_id", Value: "1"}},
			wantErr: "source used is not compatible with the tool",
		},
		{
			name:    "missing dashboard_id",
			source:  src,
			params:  parameters.ParamValues{},
			wantErr: "dashboard_id parameter missing or invalid",
		},
		{
			name:    "whitespace-only dashboard_id",
			source:  src,
			params:  parameters.ParamValues{{Name: "dashboard_id", Value: "   "}},
			wantErr: "dashboard_id parameter missing or invalid",
		},
		{
			name:   "invalid non-map filters",
			source: src,
			params: parameters.ParamValues{
				{Name: "dashboard_id", Value: "1"},
				{Name: "filters", Value: "not-a-map"},
			},
			wantErr: "'filters' must be a map",
		},
		{
			name:    "dashboard not found 404",
			source:  src,
			params:  parameters.ParamValues{{Name: "dashboard_id", Value: "9999"}},
			wantErr: "error processing request: Not found",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, tbErr := toolInstance.Invoke(ctx, tc.source, tc.params, "")
			if tbErr == nil || !strings.Contains(tbErr.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %v", tc.wantErr, tbErr)
			}
		})
	}
}

func TestInvokeResponseTooLarge(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hugeText := strings.Repeat("x", 950*1024)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/4.0/dashboards/huge":
			_, _ = w.Write([]byte(`{"id":"huge","title":"Huge Dashboard","description":"` + hugeText + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	settings := &rtl.ApiSettings{BaseUrl: ts.URL}
	session := rtl.NewAuthSession(*settings)
	session.Client = *ts.Client()
	sdk := v4.NewLookerSDK(session)

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_dashboard",
			Description: "Render dashboard tool",
		},
		Type:   "looker-render-dashboard",
		Source: "my-instance",
	}
	toolInstance, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	res, tbErr := toolInstance.Invoke(ctx, mockDashLookerSource{settings: settings, sdk: sdk}, parameters.ParamValues{
		{Name: "dashboard_id", Value: "huge"},
	}, "")
	if tbErr != nil {
		t.Fatalf("expected structured too-large response without error, got %v", tbErr)
	}
	structured, ok := res.(tools.StructuredToolResult)
	if !ok {
		t.Fatalf("expected StructuredToolResult, got %T", res)
	}
	if structured.StructuredContent["response_too_large"] != true {
		t.Errorf("expected response_too_large=true, got %v", structured.StructuredContent)
	}
}

func TestToolMetadataAndSourceMethods(t *testing.T) {
	ctx := context.Background()
	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_dashboard",
			Description: "Render dashboard tool",
		},
		Type:   "looker-render-dashboard",
		Source: "my-instance",
	}
	if cfg.ToolConfigType() != "looker-render-dashboard" {
		t.Errorf("expected ToolConfigType looker-render-dashboard, got %s", cfg.ToolConfigType())
	}
	toolInstance, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	dashTool := toolInstance.(lkr.Tool)
	if dashTool.GetSourceName() != "my-instance" {
		t.Errorf("expected source my-instance, got %s", dashTool.GetSourceName())
	}
	if dashTool.ToConfig().ToolConfigType() != "looker-render-dashboard" {
		t.Errorf("unexpected ToConfig type: %v", dashTool.ToConfig())
	}
	if err := dashTool.ValidateSource(mockDashLookerSource{}); err != nil {
		t.Errorf("ValidateSource failed for compatible source: %v", err)
	}
	if err := dashTool.ValidateSource(incompatibleSource{}); err == nil {
		t.Errorf("expected ValidateSource to fail for incompatible source")
	}
	if reqAuth, err := dashTool.RequiresClientAuthorization(mockDashLookerSource{}); err != nil || !reqAuth {
		t.Errorf("expected RequiresClientAuthorization=true, got %v, %v", reqAuth, err)
	}
	if _, err := dashTool.RequiresClientAuthorization(incompatibleSource{}); err == nil {
		t.Errorf("expected RequiresClientAuthorization error for incompatible source")
	}
	if hdr, err := dashTool.GetAuthTokenHeaderName(mockDashLookerSource{}); err != nil || hdr != "X-Looker-Token" {
		t.Errorf("expected header X-Looker-Token, got %q, %v", hdr, err)
	}
	if _, err := dashTool.GetAuthTokenHeaderName(incompatibleSource{}); err == nil {
		t.Errorf("expected GetAuthTokenHeaderName error for incompatible source")
	}
}
