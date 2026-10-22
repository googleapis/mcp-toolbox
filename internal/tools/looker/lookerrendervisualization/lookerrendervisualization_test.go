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
package lookerrendervisualization_test

import (
	"context"
	"encoding/json"
	"io"
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
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerrendervisualization"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

type mockVisLookerSource struct {
	settings *rtl.ApiSettings
	sdk      *v4.LookerSDK
}

func (m mockVisLookerSource) SourceType() string                  { return "looker" }
func (m mockVisLookerSource) ToConfig() sources.SourceConfig      { return nil }
func (m mockVisLookerSource) IsReadOnly() bool                    { return false }
func (m mockVisLookerSource) UseClientAuthorization() bool        { return false }
func (m mockVisLookerSource) GetAuthTokenHeaderName() string      { return "Authorization" }
func (m mockVisLookerSource) LookerApiSettings() *rtl.ApiSettings { return m.settings }
func (m mockVisLookerSource) GetLookerSDK(context.Context, string) (*v4.LookerSDK, error) {
	return m.sdk, nil
}

func TestParse(t *testing.T) {
	ctx := context.Background()
	tcs := []struct {
		desc string
		in   string
		want server.ToolConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: tool
			name: render_visualization
			type: looker-render-visualization
			source: my-instance
			description: Render Looker visualization
			`,
			want: server.ToolConfigs{
				"render_visualization": lkr.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "render_visualization",
						Description:  "Render Looker visualization",
						AuthRequired: []string{},
					},
					Type:   "looker-render-visualization",
					Source: "my-instance",
				},
			},
		},
		{
			desc: "example with custom UI metadata",
			in: `
			kind: tool
			name: render_visualization
			type: looker-render-visualization
			source: my-instance
			description: Render Looker visualization
			ui:
			  resource: custom_vis
			  visibility:
			    - model
			    - app
			`,
			want: server.ToolConfigs{
				"render_visualization": lkr.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "render_visualization",
						Description:  "Render Looker visualization",
						AuthRequired: []string{},
						UI: &tools.ToolUIMetadata{
							Resource:   "custom_vis",
							Visibility: []tools.ToolVisibility{tools.VisibilityModel, tools.VisibilityApp},
						},
					},
					Type:   "looker-render-visualization",
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
		w.Write([]byte("<html><body>Looker Visualization</body></html>"))
	}))
	defer ts.Close()

	t.Setenv("LOOKER_UI_URL", ts.URL)

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_visualization",
			Description: "Render visualization tool",
		},
		Type:   "looker-render-visualization",
		Source: "my-instance",
	}

	toolInstance, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if toolInstance.GetToolUIMetadata() == nil {
		t.Fatalf("expected UI meta to be populated, got nil")
	}
	if toolInstance.GetToolUIMetadata().Resource != "looker_render_visualization_ui" {
		t.Errorf("expected default resource name looker_render_visualization_ui, got %s", toolInstance.GetToolUIMetadata().Resource)
	}

	rp, ok := toolInstance.(resources.ResourceProvider)
	if !ok {
		t.Fatalf("expected toolInstance to implement ResourceProvider")
	}

	resList := rp.GetResources()
	if len(resList) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resList))
	}
	if resList[0].GetURI() != "ui://looker/render_visualization.html" {
		t.Errorf("expected URI ui://looker/render_visualization.html, got %s", resList[0].GetURI())
	}
	if resList[0].GetMimeType() != "text/html;profile=mcp-app" {
		t.Errorf("expected MimeType text/html;profile=mcp-app, got %s", resList[0].GetMimeType())
	}

	content, err := resList[0].Read(ctx, nil)
	if err != nil {
		t.Fatalf("error reading resource: %v", err)
	}
	contentStr, ok := content.(string)
	if !ok || !strings.Contains(contentStr, "Looker Visualization") {
		t.Errorf("expected HTML content to contain Looker Visualization, got %v", content)
	}

	dynRes, ok := resList[0].(*resources.DynamicUIResource)
	if !ok {
		t.Fatalf("expected *resources.DynamicUIResource")
	}
	if dynRes.PrefersBorder == nil || !*dynRes.PrefersBorder {
		t.Errorf("expected PrefersBorder to be true, got %v", dynRes.PrefersBorder)
	}
	for _, want := range []string{
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
		if !containsStr(dynRes.CSP.ResourceDomains, want) {
			t.Errorf("expected ResourceDomains to contain %q, got %v", want, dynRes.CSP.ResourceDomains)
		}
	}
	for _, forbidden := range []string{
		"https://*.looker.com",
		"https://*.googleapis.com",
		"https://*.googleusercontent.com",
	} {
		if containsStr(dynRes.CSP.ResourceDomains, forbidden) {
			t.Errorf("expected ResourceDomains NOT to contain wildcard %q, got %v", forbidden, dynRes.CSP.ResourceDomains)
		}
		if containsStr(dynRes.CSP.ConnectDomains, forbidden) {
			t.Errorf("expected ConnectDomains NOT to contain wildcard %q, got %v", forbidden, dynRes.CSP.ConnectDomains)
		}
	}
	for _, want := range []string{
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
		if !containsStr(dynRes.CSP.ConnectDomains, want) {
			t.Errorf("expected ConnectDomains to contain %q, got %v", want, dynRes.CSP.ConnectDomains)
		}
	}

	manifest, err := toolInstance.Manifest(mockVisLookerSource{})
	if err != nil {
		t.Fatalf("Manifest failed: %v", err)
	}
	foundFieldsParam := false
	for _, p := range manifest.Parameters {
		switch p.Name {
		case "model":
			if !strings.Contains(p.Description, "from get_models") || !strings.Contains(p.Description, "Required if query_id is not provided") {
				t.Errorf("expected model parameter description to reference get_models and Required if query_id is not provided, got %q", p.Description)
			}
		case "explore":
			if !strings.Contains(p.Description, "from get_explores") || !strings.Contains(p.Description, "Required if query_id is not provided") {
				t.Errorf("expected explore parameter description to reference get_explores and Required if query_id is not provided, got %q", p.Description)
			}
		case "fields":
			foundFieldsParam = true
			if !strings.Contains(p.Description, "Not Enough Fields") {
				t.Errorf("expected fields parameter description to mention 'Not Enough Fields', got %q", p.Description)
			}
			if !strings.Contains(p.Description, "BOTH the grouping dimension(s) AND the measure(s)") {
				t.Errorf("expected fields parameter description to require BOTH dimension(s) and measure(s), got %q", p.Description)
			}
			if !strings.Contains(p.Description, "Required if query_id is not provided") {
				t.Errorf("expected fields parameter description to mention 'Required if query_id is not provided', got %q", p.Description)
			}
		case "pivots":
			if !strings.Contains(p.Description, "automatically appended") {
				t.Errorf("expected pivots parameter description to mention automatic appending, got %q", p.Description)
			}
		}
	}
	if !foundFieldsParam {
		t.Errorf("expected fields parameter in render_visualization manifest")
	}
}

func containsStr(list []string, value string) bool {
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
		_, _ = w.Write([]byte("<html><body>Looker Visualization Scene</body></html>"))
	}))
	defer ts.Close()

	t.Setenv("LOOKER_UI_URL", ts.URL)

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_visualization",
			Description: "Render visualization tool",
		},
		Type:   "looker-render-visualization",
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
	if !ok || !strings.Contains(contentStr, "Looker Visualization Scene") {
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
			Name:        "render_visualization",
			Description: "Render visualization tool",
		},
		Type:   "looker-render-visualization",
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
			Name:        "render_visualization",
			Description: "Render visualization tool",
		},
		Type:   "looker-render-visualization",
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
			Name:        "render_visualization",
			Description: "Render visualization tool",
		},
		Type:   "looker-render-visualization",
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

func TestInvokeQueryIdOverridesWithoutCreateQuery(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	createQueryCalled := false
	inlineQueryCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/4.0/queries/42":
			_, _ = w.Write([]byte(`{"id":"42","client_id":"slug42","model":"thelook","view":"orders","fields":["orders.status","orders.count"],"vis_config":{"type":"looker_column"},"can":{"explore":true,"run":true}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/4.0/queries":
			createQueryCalled = true
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"CreateQuery should not be called on query_id override path"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/4.0/queries/run/json_detail":
			inlineQueryCalled = true
			_, _ = w.Write([]byte(`{"data":[{"orders.status":{"value":"complete"},"orders.count":{"value":10}}],"fields":{"dimensions":[{"name":"orders.status"}],"measures":[{"name":"orders.count"}]}}`))
		case r.URL.Path == "/api/4.0/user":
			_, _ = w.Write([]byte(`{"id":"1","locale":"en"}`))
		case r.URL.Path == "/api/4.0/color_collections":
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	settings := &rtl.ApiSettings{BaseUrl: server.URL}
	session := rtl.NewAuthSession(*settings)
	session.Client = *server.Client()
	sdk := v4.NewLookerSDK(session)

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_visualization",
			Description: "Render visualization tool",
		},
		Type:   "looker-render-visualization",
		Source: "my-instance",
	}
	toolInstance, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	params := parameters.ParamValues{
		{Name: "query_id", Value: "42"},
		{Name: "limit", Value: 25},
		{Name: "vis_config", Value: `{"type":"looker_bar"}`},
	}
	res, tbErr := toolInstance.Invoke(ctx, mockVisLookerSource{settings: settings, sdk: sdk}, params, "")
	if tbErr != nil {
		t.Fatalf("Invoke failed: %v", tbErr)
	}
	if createQueryCalled {
		t.Errorf("expected CreateQuery NOT to be called when overriding saved query_id")
	}
	if !inlineQueryCalled {
		t.Errorf("expected RunInlineQuery (json_detail) to be called when overriding saved query_id")
	}
	structured, ok := res.(tools.StructuredToolResult)
	if !ok {
		t.Fatalf("expected StructuredToolResult, got %T", res)
	}
	contentMap, ok := structured.Content.(map[string]any)
	if !ok || contentMap["row_count"] != 1 {
		t.Errorf("expected row_count=1 in summary content, got %v", structured.Content)
	}
	if contentMap["model"] != "thelook" || contentMap["explore"] != "orders" {
		t.Errorf("expected model=thelook and explore=orders in summary content, got %v", contentMap)
	}
	if structured.StructuredContent["model"] != "thelook" || structured.StructuredContent["explore"] != "orders" {
		t.Errorf("expected top-level model=thelook and explore=orders in structuredContent, got %v", structured.StructuredContent)
	}
	if visData, ok := structured.StructuredContent["visualizationData"].(map[string]any); ok {
		if _, hasDuplicateAppState := visData["app_state"]; hasDuplicateAppState {
			t.Errorf("expected app_state not to be duplicated inside visualizationData")
		}
		if qMeta, ok := visData["query"].(map[string]any); ok {
			if canMap, ok := qMeta["can"].(map[string]bool); !ok || !canMap["explore"] {
				t.Errorf("expected visualizationData.query.can.explore=true to be preserved, got %v", qMeta["can"])
			}
		} else {
			t.Errorf("expected visualizationData.query map, got %v", visData["query"])
		}
	}
	if topCan, ok := structured.StructuredContent["can"].(map[string]bool); !ok || !topCan["explore"] {
		t.Errorf("expected top-level structuredContent.can.explore=true to be preserved, got %v", structured.StructuredContent["can"])
	}
}

func TestInvokeMissingRequiredInlineParams(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_visualization",
			Description: "Render visualization tool",
		},
		Type:   "looker-render-visualization",
		Source: "my-instance",
	}
	toolInstance, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	tcs := []struct {
		name    string
		params  parameters.ParamValues
		wantErr string
	}{
		{
			name: "missing model",
			params: parameters.ParamValues{
				{Name: "explore", Value: "order_items"},
				{Name: "fields", Value: []any{"products.department", "order_items.count"}},
			},
			wantErr: "model is required when query_id is not provided; use get_models",
		},
		{
			name: "missing explore",
			params: parameters.ParamValues{
				{Name: "model", Value: "cypress_thelook"},
				{Name: "fields", Value: []any{"products.department", "order_items.count"}},
			},
			wantErr: "explore is required when query_id is not provided; use get_explores",
		},
		{
			name: "missing fields",
			params: parameters.ParamValues{
				{Name: "model", Value: "cypress_thelook"},
				{Name: "explore", Value: "order_items"},
			},
			wantErr: "fields is required when query_id is not provided; use get_dimensions and get_measures",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			_, tbErr := toolInstance.Invoke(ctx, mockVisLookerSource{}, tc.params, "")
			if tbErr == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(tbErr.Error(), tc.wantErr) {
				t.Errorf("expected error to contain %q, got %q", tc.wantErr, tbErr.Error())
			}
		})
	}
}

func TestInvokeStripsTableCalculationsAndRestoresHiddenFields(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var capturedFields []string
	dynamicFieldsJSON := `[{"table_calculation":"pct_of_total","label":"Pct of Total","expression":"${orders.total_sale_price} / sum(${orders.total_sale_price})"},{"measure":"custom_avg","label":"Custom Avg","based_on":"orders.sale_price","type":"average"}]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/4.0/queries/77":
			_, _ = w.Write([]byte(`{"id":"77","client_id":"slug77","model":"thelook","view":"orders","fields":["orders.status","orders.count","orders.total_sale_price","custom_avg"],"dynamic_fields":` + `"` + strings.ReplaceAll(dynamicFieldsJSON, `"`, `\"`) + `"` + `,"vis_config":{"type":"looker_column","hidden_fields":["orders.count","pct_of_total"]},"can":{"explore":true,"run":true}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/4.0/queries/run/json_detail":
			bodyBytes, _ := io.ReadAll(r.Body)
			var payload struct {
				Fields []string `json:"fields"`
			}
			_ = json.Unmarshal(bodyBytes, &payload)
			capturedFields = payload.Fields
			for _, f := range payload.Fields {
				if f == "pct_of_total" {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"message":"table calculation pct_of_total cannot be passed in fields"}`))
					return
				}
			}
			_, _ = w.Write([]byte(`{"data":[{"orders.status":{"value":"complete"},"orders.count":{"value":10},"orders.total_sale_price":{"value":250},"custom_avg":{"value":25},"pct_of_total":{"value":1}}],"fields":{"dimensions":[{"name":"orders.status"}],"measures":[{"name":"orders.count"},{"name":"orders.total_sale_price"},{"name":"custom_avg"}],"table_calculations":[{"name":"pct_of_total"}]}}`))
		case r.URL.Path == "/api/4.0/user":
			_, _ = w.Write([]byte(`{"id":"1","locale":"en"}`))
		case r.URL.Path == "/api/4.0/color_collections":
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	settings := &rtl.ApiSettings{BaseUrl: server.URL}
	session := rtl.NewAuthSession(*settings)
	session.Client = *server.Client()
	sdk := v4.NewLookerSDK(session)

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "render_visualization",
			Description: "Render visualization tool",
		},
		Type:   "looker-render-visualization",
		Source: "my-instance",
	}
	toolInstance, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	params := parameters.ParamValues{
		{Name: "query_id", Value: "77"},
		{Name: "fields", Value: []any{"orders.status", "pct_of_total", "custom_avg"}},
		{Name: "vis_config", Value: `{"type":"looker_bar","hidden_fields":["orders.count","pct_of_total"]}`},
	}
	res, tbErr := toolInstance.Invoke(ctx, mockVisLookerSource{settings: settings, sdk: sdk}, params, "")
	if tbErr != nil {
		t.Fatalf("Invoke failed when switching chart type with hidden_fields + dynamic_fields: %v", tbErr)
	}
	if containsStr(capturedFields, "pct_of_total") {
		t.Errorf("expected table calculation pct_of_total to be stripped from fields, got %v", capturedFields)
	}
	for _, want := range []string{"orders.status", "custom_avg", "orders.total_sale_price", "orders.count"} {
		if !containsStr(capturedFields, want) {
			t.Errorf("expected field %q to be preserved/restored in WriteQuery.Fields, got %v", want, capturedFields)
		}
	}
	structured, ok := res.(tools.StructuredToolResult)
	if !ok {
		t.Fatalf("expected StructuredToolResult, got %T", res)
	}
	visData, _ := structured.StructuredContent["visualizationData"].(map[string]any)
	queryMeta, _ := visData["query"].(map[string]any)
	if dynSlice, ok := queryMeta["dynamic_fields"].([]any); !ok || len(dynSlice) != 2 {
		t.Errorf("expected queryMeta.dynamic_fields to preserve 2 parsed dynamic field entries, got %#v", queryMeta["dynamic_fields"])
	}
}



