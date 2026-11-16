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

package lookerupdatedataappextension_test

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
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerupdatedataappextension"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

func TestParseFromYamlUpdateDataAppExtension(t *testing.T) {
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
			name: update_data_app_extension
			type: looker-update-data-app-extension
			source: my-instance
			description: some description
			`,
			want: server.ToolConfigs{
				"update_data_app_extension": lkr.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "update_data_app_extension",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "looker-update-data-app-extension",
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

func TestFailParseFromYamlUpdateDataAppExtension(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = testutils.ContextWithUserAgent(ctx, "test-user-agent")

	in := `
	kind: tool
	name: example_tool
	type: looker-update-data-app-extension
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

func TestInvokeUpdateDataAppExtensionValidation(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "update_data_app_extension",
			Description: "test description",
		},
		Type:   "looker-update-data-app-extension",
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
			desc: "empty id",
			params: parameters.ParamValues{
				{Name: "data_app_extension_id", Value: "db::"},
				{Name: "label", Value: "Updated"},
			},
			wantErr: "'data_app_extension_id' cannot be empty",
		},
		{
			desc: "path traversal id rejected",
			params: parameters.ParamValues{
				{Name: "data_app_extension_id", Value: "../inline_extension/123"},
				{Name: "label", Value: "Updated"},
			},
			wantErr: "invalid 'data_app_extension_id'",
		},
		{
			desc: "no updatable fields provided",
			params: parameters.ParamValues{
				{Name: "data_app_extension_id", Value: "db::abc-123"},
				{Name: "label", Value: nil},
				{Name: "code", Value: nil},
				{Name: "description", Value: nil},
				{Name: "name", Value: nil},
				{Name: "dashboard_id", Value: "42"},
			},
			wantErr: "at least one of 'label', 'code', 'description', or 'name' must be provided",
		},
		{
			desc: "empty code rejected when provided",
			params: parameters.ParamValues{
				{Name: "data_app_extension_id", Value: "abc-123"},
				{Name: "code", Value: "   "},
			},
			wantErr: "'code' cannot be empty",
		},
		{
			desc: "invalid name rejected when provided",
			params: parameters.ParamValues{
				{Name: "data_app_extension_id", Value: "abc-123"},
				{Name: "name", Value: "invalid space"},
			},
			wantErr: "'name' may only contain",
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

func TestInvokeUpdateDataAppExtensionSuccessAndErrors(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	var lastPath, lastRawQuery string
	var receivedBody map[string]any
	statusToReturn := http.StatusOK

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/4.0/login" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if statusToReturn != http.StatusOK {
			w.WriteHeader(statusToReturn)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "error from looker"})
			return
		}
		lastPath = r.URL.Path
		lastRawQuery = r.URL.RawQuery
		receivedBody = nil
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        "ext-uuid-9",
			"name":      "data-app-sales-ext-uuid-9",
			"label":     "Updated Sales App",
			"code_size": 31,
		})
	}))
	defer ts.Close()

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "update_data_app_extension",
			Description: "test description",
		},
		Type:   "looker-update-data-app-extension",
		Source: "my-instance",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	mockSrc := MockSource{baseURL: ts.URL}

	// Case 1: Partial PATCH with db:: prefix and dashboard_id query param
	res, invokeErr := tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "data_app_extension_id", Value: "db::ext-uuid-9"},
		{Name: "label", Value: "Updated Sales App"},
		{Name: "code", Value: "export default () => 'updated';"},
		{Name: "description", Value: nil},
		{Name: "name", Value: nil},
		{Name: "dashboard_id", Value: "123"},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected invoke error: %v", invokeErr)
	}
	if lastPath != "/api/4.0/data_app_extension/ext-uuid-9" || lastRawQuery != "dashboard_id=123" {
		t.Fatalf("unexpected PATCH target: %s?%s", lastPath, lastRawQuery)
	}
	if _, hasName := receivedBody["name"]; hasName {
		t.Errorf("expected name to be omitted from PATCH body, got %v", receivedBody["name"])
	}
	if _, hasDesc := receivedBody["description"]; hasDesc {
		t.Errorf("expected description to be omitted from PATCH body, got %v", receivedBody["description"])
	}
	gotExt, ok := res.(*lookercommon.DataAppExtension)
	if !ok {
		t.Fatalf("expected *lookercommon.DataAppExtension, got %T", res)
	}
	if gotExt.ExtensionId != "db::ext-uuid-9" {
		t.Errorf("expected ExtensionId 'db::ext-uuid-9', got %q", gotExt.ExtensionId)
	}

	// Case 2: Unauthorized error mapping (401)
	statusToReturn = http.StatusUnauthorized
	_, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "data_app_extension_id", Value: "ext-uuid-9"},
		{Name: "label", Value: "Updated Sales App"},
		{Name: "dashboard_id", Value: ""},
	}, "")
	if invokeErr == nil || !strings.Contains(invokeErr.Error(), "unauthorized error") {
		t.Fatalf("expected unauthorized error, got: %v", invokeErr)
	}
}
