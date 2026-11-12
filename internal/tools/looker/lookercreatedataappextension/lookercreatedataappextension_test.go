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

package lookercreatedataappextension_test

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
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercreatedataappextension"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

func TestParseFromYamlCreateDataAppExtension(t *testing.T) {
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
			name: create_data_app_extension
			type: looker-create-data-app-extension
			source: my-instance
			description: some description
			`,
			want: server.ToolConfigs{
				"create_data_app_extension": lkr.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "create_data_app_extension",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "looker-create-data-app-extension",
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

func TestFailParseFromYamlCreateDataAppExtension(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = testutils.ContextWithUserAgent(ctx, "test-user-agent")

	in := `
	kind: tool
	name: example_tool
	type: looker-create-data-app-extension
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

func TestInvokeCreateDataAppExtensionValidation(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "create_data_app_extension",
			Description: "test description",
		},
		Type:   "looker-create-data-app-extension",
		Source: "my-instance",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	mockSrc := MockSource{}

	tcs := []struct {
		desc    string
		params  parameters.ParamValues
		wantErr string
	}{
		{
			desc: "empty label",
			params: parameters.ParamValues{
				{Name: "label", Value: "   "},
				{Name: "code", Value: "export default () => null;"},
				{Name: "name", Value: ""},
			},
			wantErr: "'label' cannot be empty",
		},
		{
			desc: "empty code",
			params: parameters.ParamValues{
				{Name: "label", Value: "Sales App"},
				{Name: "code", Value: "   "},
				{Name: "name", Value: ""},
			},
			wantErr: "'code' cannot be empty",
		},
		{
			desc: "oversized code",
			params: parameters.ParamValues{
				{Name: "label", Value: "Sales App"},
				{Name: "code", Value: strings.Repeat("x", lookercommon.MaxDataAppExtensionCodeBytes+1)},
				{Name: "name", Value: ""},
			},
			wantErr: "cannot exceed",
		},
		{
			desc: "invalid explicit name",
			params: parameters.ParamValues{
				{Name: "label", Value: "Sales App"},
				{Name: "code", Value: "export default () => null;"},
				{Name: "name", Value: "bad/name!"},
			},
			wantErr: "'name' may only contain",
		},
		{
			desc: "non-string description",
			params: parameters.ParamValues{
				{Name: "label", Value: "Sales App"},
				{Name: "code", Value: "export default () => null;"},
				{Name: "description", Value: 42},
				{Name: "name", Value: ""},
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

func TestInvokeCreateDataAppExtensionSuccessAndErrors(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	var receivedBody map[string]any
	statusToReturn := http.StatusOK
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/4.0/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/api/4.0/data_app_extension":
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if statusToReturn != http.StatusOK {
				w.WriteHeader(statusToReturn)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "error from looker"})
				return
			}
			receivedBody = nil
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "ext-uuid-1",
				"name":        receivedBody["name"],
				"label":       receivedBody["label"],
				"description": receivedBody["description"],
				"code_size":   26,
				"mount_points": map[string]any{
					"dashboard_tile": true,
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "create_data_app_extension",
			Description: "test description",
		},
		Type:   "looker-create-data-app-extension",
		Source: "my-instance",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	mockSrc := MockSource{baseURL: ts.URL}

	// Case 1: Auto-generated name when omitted/empty
	res, invokeErr := tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "label", Value: "Executive KPI App"},
		{Name: "code", Value: "export default () => null;"},
		{Name: "description", Value: nil},
		{Name: "name", Value: ""},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected invoke error: %v", invokeErr)
	}
	gotExt, ok := res.(*lookercommon.DataAppExtension)
	if !ok {
		t.Fatalf("expected *lookercommon.DataAppExtension, got %T", res)
	}
	if gotExt.ExtensionId != "db::ext-uuid-1" {
		t.Errorf("expected ExtensionId 'db::ext-uuid-1', got %q", gotExt.ExtensionId)
	}
	genName, _ := receivedBody["name"].(string)
	if !strings.HasPrefix(genName, "data-app-executive-kpi-app-") {
		t.Errorf("expected auto-generated name prefix, got %q", genName)
	}
	if _, hasDesc := receivedBody["description"]; hasDesc {
		t.Errorf("expected omitted description when nil, got %v", receivedBody["description"])
	}

	// Case 2: Explicit name and description
	_, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "label", Value: "Executive KPI App"},
		{Name: "code", Value: "export default () => null;"},
		{Name: "description", Value: "Governed KPI overview"},
		{Name: "name", Value: "custom_kpi_app_1"},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected invoke error: %v", invokeErr)
	}
	if receivedBody["name"] != "custom_kpi_app_1" || receivedBody["description"] != "Governed KPI overview" {
		t.Errorf("unexpected body: %v", receivedBody)
	}

	// Case 3: Unauthorized error mapping (401)
	statusToReturn = http.StatusUnauthorized
	_, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "label", Value: "Executive KPI App"},
		{Name: "code", Value: "export default () => null;"},
		{Name: "name", Value: "custom_kpi_app_2"},
	}, "")
	if invokeErr == nil || !strings.Contains(invokeErr.Error(), "unauthorized error") {
		t.Fatalf("expected unauthorized error, got: %v", invokeErr)
	}
}
