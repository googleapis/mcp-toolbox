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

package lookerpublishdataapp_test

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
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerpublishdataapp"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

func TestParseFromYamlPublishDataApp(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = testutils.ContextWithUserAgent(ctx, "test-user-agent")

	tcs := []struct {
		desc string
		in   string
		want server.ToolConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: tool
			name: publish_data_app
			type: looker-publish-data-app
			source: my-instance
			description: some description
			`,
			want: server.ToolConfigs{
				"publish_data_app": lkr.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "publish_data_app",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "looker-publish-data-app",
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

func TestFailParseFromYamlPublishDataApp(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = testutils.ContextWithUserAgent(ctx, "test-user-agent")

	in := `
	kind: tool
	name: example_tool
	type: looker-publish-data-app
	source: my-instance
	unknown_field: bad
	description: some description
	`
	_, _, _, _, _, _, _, _, err = server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(in))
	if err == nil || !strings.Contains(err.Error(), `unknown field "unknown_field"`) {
		t.Fatalf("expected unknown field error, got: %v", err)
	}
}

type MockSource struct {
	sources.Source
	baseURL string
	hostURL string
}

func (m MockSource) UseClientAuthorization() bool {
	return false
}

func (m MockSource) GetAuthTokenHeaderName() string {
	return "Authorization"
}

func (m MockSource) LookerApiSettings() *rtl.ApiSettings {
	return &rtl.ApiSettings{
		BaseUrl:    m.baseURL,
		ApiVersion: "4.0",
	}
}

func (m MockSource) GetLookerSDK(ctx context.Context, s string) (*v4.LookerSDK, error) {
	settings := rtl.ApiSettings{
		BaseUrl:      m.baseURL,
		ApiVersion:   "4.0",
		ClientId:     "test-id",
		ClientSecret: "test-secret",
	}
	return v4.NewLookerSDK(rtl.NewAuthSession(settings)), nil
}

func (m MockSource) GetHostURL(ctx context.Context, sdk *v4.LookerSDK) (string, error) {
	return m.hostURL, nil
}

func newTestTool(t *testing.T) tools.Tool {
	t.Helper()
	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "publish_data_app",
			Description: "test description",
		},
		Type:   "looker-publish-data-app",
		Source: "my-instance",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	return tool
}

func TestInvokePublishDataAppValidation(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tool := newTestTool(t)
	mockSrc := MockSource{}

	tcs := []struct {
		desc    string
		params  parameters.ParamValues
		wantErr string
	}{
		{
			desc: "empty title",
			params: parameters.ParamValues{
				{Name: "title", Value: "   "},
				{Name: "code", Value: "export default () => null;"},
			},
			wantErr: "'title' cannot be empty",
		},
		{
			desc: "empty code",
			params: parameters.ParamValues{
				{Name: "title", Value: "Revenue App"},
				{Name: "code", Value: "   "},
			},
			wantErr: "'code' cannot be empty",
		},
		{
			desc: "oversized code",
			params: parameters.ParamValues{
				{Name: "title", Value: "Revenue App"},
				{Name: "code", Value: strings.Repeat("a", lookercommon.MaxDataAppExtensionCodeBytes+1)},
			},
			wantErr: "cannot exceed",
		},
		{
			desc: "invalid rich_content_json",
			params: parameters.ParamValues{
				{Name: "title", Value: "Revenue App"},
				{Name: "code", Value: "export default () => null;"},
				{Name: "rich_content_json", Value: "{not valid json"},
			},
			wantErr: "'rich_content_json' must be a valid JSON string",
		},
		{
			desc: "non-string description",
			params: parameters.ParamValues{
				{Name: "title", Value: "Revenue App"},
				{Name: "code", Value: "export default () => null;"},
				{Name: "description", Value: 123},
			},
			wantErr: "'description' must be a string",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, invokeErr := tool.Invoke(ctx, mockSrc, tc.params, "")
			if invokeErr == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(invokeErr.Error(), tc.wantErr) {
				t.Fatalf("unexpected error: got %q, want substring %q", invokeErr.Error(), tc.wantErr)
			}
		})
	}
}

func TestInvokePublishDataAppCreateFlow(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	var callOrder []string
	var extPayload map[string]any
	var dashPatchPayload map[string]any
	var elemPayload map[string]any
	var layoutPayload map[string]any
	failExtCreate := false
	personalFolderID := "306"
	existingTitles := []map[string]any{{"title": "Existing Dashboard"}}

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
			callOrder = append(callOrder, "GET /user")
			resp := map[string]any{"id": "1"}
			if personalFolderID != "" {
				resp["personal_folder_id"] = personalFolderID
			}
			_ = json.NewEncoder(w).Encode(resp)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/4.0/folders/") && strings.HasSuffix(r.URL.Path, "/dashboards"):
			callOrder = append(callOrder, "GET "+strings.TrimPrefix(r.URL.Path, "/api/4.0"))
			_ = json.NewEncoder(w).Encode(existingTitles)
		case r.Method == http.MethodPost && r.URL.Path == "/api/4.0/data_app_extension":
			callOrder = append(callOrder, "POST /data_app_extension")
			if failExtCreate {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "invalid extension code"})
				return
			}
			extPayload = nil
			_ = json.NewDecoder(r.Body).Decode(&extPayload)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "ext-uuid-1",
				"name":  extPayload["name"],
				"label": extPayload["label"],
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/4.0/dashboards":
			callOrder = append(callOrder, "POST /dashboards")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":  "dash-101",
				"url": "/dashboards/dash-101",
				"dashboard_layouts": []map[string]any{
					{"id": "layout-101", "active": true, "type": "newspaper"},
				},
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/4.0/dashboards/dash-101":
			callOrder = append(callOrder, "PATCH /dashboards/dash-101")
			dashPatchPayload = nil
			_ = json.NewDecoder(r.Body).Decode(&dashPatchPayload)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "dash-101"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/4.0/dashboard_elements":
			callOrder = append(callOrder, "POST /dashboard_elements")
			elemPayload = nil
			_ = json.NewDecoder(r.Body).Decode(&elemPayload)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "elem-101"})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/4.0/dashboard_layouts/layout-101":
			callOrder = append(callOrder, "PATCH /dashboard_layouts/layout-101")
			layoutPayload = nil
			_ = json.NewDecoder(r.Body).Decode(&layoutPayload)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "layout-101", "type": "extension", "active": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	tool := newTestTool(t)
	mockSrc := MockSource{baseURL: ts.URL, hostURL: "https://looker.example.com"}

	// Case 1: Happy path with default personal folder and rich_content_json
	richJSON := `{"data_app_queries_version":1,"data_app_queries":[]}`
	res, invokeErr := tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "title", Value: "Executive KPI App"},
		{Name: "code", Value: "export default class App {}"},
		{Name: "description", Value: "Full-bleed KPI overview"},
		{Name: "folder", Value: ""},
		{Name: "dashboard_id", Value: ""},
		{Name: "rich_content_json", Value: richJSON},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected invoke error: %v", invokeErr)
	}

	gotMap, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", res)
	}
	wantMap := map[string]any{
		"dashboard_id":         "dash-101",
		"url":                  "https://looker.example.com/dashboards/dash-101",
		"extension_id":         "db::ext-uuid-1",
		"dashboard_element_id": "elem-101",
		"dashboard_layout_id":  "layout-101",
	}
	if diff := cmp.Diff(wantMap, gotMap); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	wantOrder := []string{
		"GET /user",
		"GET /folders/306/dashboards",
		"POST /data_app_extension",
		"POST /dashboards",
		"PATCH /dashboards/dash-101",
		"POST /dashboard_elements",
		"PATCH /dashboard_layouts/layout-101",
	}
	if diff := cmp.Diff(wantOrder, callOrder); diff != "" {
		t.Errorf("call order mismatch (-want +got):\n%s", diff)
	}

	// Verify EnsureDataAppMeta stamped the code
	stampedCode, _ := extPayload["code"].(string)
	if !strings.Contains(stampedCode, "__lookerDataAppMeta") {
		t.Errorf("expected stamped __lookerDataAppMeta in code, got %q", stampedCode)
	}

	// Verify full-bleed appearance
	appMap, _ := dashPatchPayload["appearance"].(map[string]any)
	if appMap["show_dashboard_header"] != false || appMap["tile_shadow"] != false || appMap["page_side_margins"] != float64(0) || appMap["tile_space_between"] != float64(0) {
		t.Errorf("unexpected full-bleed appearance payload: %v", appMap)
	}

	// Verify extension element payload
	if elemPayload["type"] != "extension" || elemPayload["extension_id"] != "db::ext-uuid-1" || elemPayload["title_hidden"] != true || elemPayload["rich_content_json"] != richJSON {
		t.Errorf("unexpected element payload: %v", elemPayload)
	}

	// Verify layout switched to extension
	if layoutPayload["type"] != "extension" || layoutPayload["active"] != true {
		t.Errorf("unexpected layout payload: %v", layoutPayload)
	}

	// Case 2: Extension creation failure must NOT create an orphaned dashboard
	callOrder = nil
	failExtCreate = true
	_, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "title", Value: "Bad App"},
		{Name: "code", Value: "export default class Bad {}"},
		{Name: "folder", Value: "1"},
		{Name: "dashboard_id", Value: ""},
	}, "")
	if invokeErr == nil {
		t.Fatal("expected error when extension creation fails, got nil")
	}
	for _, call := range callOrder {
		if call == "POST /dashboards" {
			t.Fatalf("POST /dashboards should not have been called after extension failure; calls: %v", callOrder)
		}
	}
	failExtCreate = false

	// Case 3: Duplicate title rejected before creating extension or dashboard
	callOrder = nil
	_, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "title", Value: "Existing Dashboard"},
		{Name: "code", Value: "export default class Dup {}"},
		{Name: "folder", Value: "1"},
		{Name: "dashboard_id", Value: ""},
	}, "")
	if invokeErr == nil || !strings.Contains(invokeErr.Error(), "already used in folder") {
		t.Fatalf("expected duplicate title error, got: %v", invokeErr)
	}
	if len(callOrder) != 1 || callOrder[0] != "GET /folders/1/dashboards" {
		t.Fatalf("expected only folder check on duplicate title, got calls: %v", callOrder)
	}

	// Case 4: Missing personal folder error when folder is omitted
	personalFolderID = ""
	_, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "title", Value: "No Folder App"},
		{Name: "code", Value: "export default class App {}"},
		{Name: "folder", Value: ""},
		{Name: "dashboard_id", Value: ""},
	}, "")
	if invokeErr == nil || !strings.Contains(invokeErr.Error(), "does not have a personal folder") {
		t.Fatalf("expected missing personal folder error, got: %v", invokeErr)
	}
}

func TestInvokePublishDataAppUpdateFlow(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	var callOrder []string
	var extPatchQuery string
	var elemPatchPayload map[string]any
	extPatchStatus := http.StatusOK
	existingElements := []map[string]any{
		{"id": "elem-200", "type": "extension", "extension_id": "db::ext-old"},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/4.0/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/4.0/dashboards/dash-200":
			callOrder = append(callOrder, "GET /dashboards/dash-200")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                 "dash-200",
				"url":                "/dashboards/dash-200",
				"title":              "Old Title",
				"dashboard_elements": existingElements,
				"dashboard_layouts": []map[string]any{
					{"id": "layout-200", "active": true, "type": "extension"},
				},
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/4.0/data_app_extension/ext-old":
			callOrder = append(callOrder, "PATCH /data_app_extension/ext-old")
			extPatchQuery = r.URL.RawQuery
			if extPatchStatus != http.StatusOK {
				w.WriteHeader(extPatchStatus)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "not found"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ext-old", "label": "Updated App"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/4.0/data_app_extension":
			callOrder = append(callOrder, "POST /data_app_extension")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ext-replacement", "label": "Updated App"})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/4.0/dashboards/dash-200":
			callOrder = append(callOrder, "PATCH /dashboards/dash-200")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "dash-200"})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/4.0/dashboard_elements/elem-200":
			callOrder = append(callOrder, "PATCH /dashboard_elements/elem-200")
			elemPatchPayload = nil
			_ = json.NewDecoder(r.Body).Decode(&elemPatchPayload)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "elem-200"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/4.0/dashboard_elements":
			callOrder = append(callOrder, "POST /dashboard_elements")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "elem-new"})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/4.0/dashboard_layouts/layout-200":
			callOrder = append(callOrder, "PATCH /dashboard_layouts/layout-200")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "layout-200", "type": "extension", "active": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	tool := newTestTool(t)
	mockSrc := MockSource{baseURL: ts.URL, hostURL: "https://looker.example.com"}

	// Case 1: In-place update of existing extension
	res, invokeErr := tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "title", Value: "Updated App"},
		{Name: "code", Value: "export default class V2 {}"},
		{Name: "dashboard_id", Value: "dash-200"},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected invoke error: %v", invokeErr)
	}
	wantMap := map[string]any{
		"dashboard_id":         "dash-200",
		"url":                  "https://looker.example.com/dashboards/dash-200",
		"extension_id":         "db::ext-old",
		"dashboard_element_id": "elem-200",
		"dashboard_layout_id":  "layout-200",
	}
	if diff := cmp.Diff(wantMap, res); diff != "" {
		t.Errorf("update result mismatch (-want +got):\n%s", diff)
	}
	if !strings.Contains(extPatchQuery, "dashboard_id=dash-200") {
		t.Errorf("expected dashboard_id query param on PATCH, got %q", extPatchQuery)
	}

	// Case 2: Referenced extension deleted (404) -> falls back to creating new extension and repointing element
	callOrder = nil
	extPatchStatus = http.StatusNotFound
	res, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "title", Value: "Recovered App"},
		{Name: "code", Value: "export default class V3 {}"},
		{Name: "dashboard_id", Value: "dash-200"},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected invoke error on 404 fallback: %v", invokeErr)
	}
	gotMap := res.(map[string]any)
	if gotMap["extension_id"] != "db::ext-replacement" {
		t.Errorf("expected replacement extension_id 'db::ext-replacement', got %v", gotMap["extension_id"])
	}
	if elemPatchPayload["extension_id"] != "db::ext-replacement" {
		t.Errorf("expected element repointed to 'db::ext-replacement', got %v", elemPatchPayload["extension_id"])
	}

	// Case 3: Unauthorized error (401)
	extPatchStatus = http.StatusUnauthorized
	_, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "title", Value: "Unauth App"},
		{Name: "code", Value: "export default class V4 {}"},
		{Name: "dashboard_id", Value: "dash-200"},
	}, "")
	if invokeErr == nil || !strings.Contains(invokeErr.Error(), "unauthorized error") {
		t.Fatalf("expected unauthorized error, got: %v", invokeErr)
	}
}
