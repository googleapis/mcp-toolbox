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

package lookergetdataappextension_test

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
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetdataappextension"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

func TestParseFromYamlGetDataAppExtension(t *testing.T) {
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
			name: get_data_app_extension
			type: looker-get-data-app-extension
			source: my-instance
			description: some description
			`,
			want: server.ToolConfigs{
				"get_data_app_extension": lkr.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "get_data_app_extension",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "looker-get-data-app-extension",
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

func TestFailParseFromYamlGetDataAppExtension(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = testutils.ContextWithUserAgent(ctx, "test-user-agent")

	in := `
	kind: tool
	name: example_tool
	type: looker-get-data-app-extension
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

func TestInvokeGetDataAppExtensionValidation(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "get_data_app_extension",
			Description: "test description",
		},
		Type:   "looker-get-data-app-extension",
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
			},
			wantErr: "'data_app_extension_id' cannot be empty",
		},
		{
			desc: "path traversal id rejected",
			params: parameters.ParamValues{
				{Name: "data_app_extension_id", Value: "../inline_extension/123"},
			},
			wantErr: "invalid 'data_app_extension_id'",
		},
		{
			desc: "non-boolean include_code rejected",
			params: parameters.ParamValues{
				{Name: "data_app_extension_id", Value: "abc-123"},
				{Name: "include_code", Value: "yes"},
			},
			wantErr: "'include_code' must be a boolean",
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

func TestInvokeGetDataAppExtensionSuccessAndErrors(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	const expectedBundleJS = "window.__lookerDataAppMeta = {};\nexport default class MyApp {};"
	var metaCalls, bundleCalls int
	metaStatus := http.StatusOK
	bundleStatus := http.StatusOK

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/4.0/login" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/api/4.0/data_app_extension/ext-uuid-9":
			metaCalls++
			w.Header().Set("Content-Type", "application/json")
			if metaStatus != http.StatusOK {
				w.WriteHeader(metaStatus)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "error from looker"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "ext-uuid-9",
				"name":        "data-app-sales-ext-uuid-9",
				"label":       "Sales App",
				"description": "Executive sales data app",
				"user_id":     42,
				"code_size":   len(expectedBundleJS),
			})
		case "/api/4.0/inline_extension/ext-uuid-9/bundle.js":
			bundleCalls++
			if bundleStatus != http.StatusOK {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(bundleStatus)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "bundle error"})
				return
			}
			w.Header().Set("Content-Type", "application/javascript")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(expectedBundleJS))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "get_data_app_extension",
			Description: "test description",
		},
		Type:   "looker-get-data-app-extension",
		Source: "my-instance",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	mockSrc := MockSource{baseURL: ts.URL}

	// Case 1: Default include_code=true with db:: prefix
	res, invokeErr := tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "data_app_extension_id", Value: "db::ext-uuid-9"},
		{Name: "include_code", Value: true},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected invoke error: %v", invokeErr)
	}
	gotExt, ok := res.(*lookercommon.DataAppExtension)
	if !ok {
		t.Fatalf("expected *lookercommon.DataAppExtension, got %T", res)
	}
	if gotExt.ExtensionId != "db::ext-uuid-9" {
		t.Errorf("expected ExtensionId 'db::ext-uuid-9', got %q", gotExt.ExtensionId)
	}
	if gotExt.Code == nil || *gotExt.Code != expectedBundleJS {
		t.Errorf("expected Code %q, got %v", expectedBundleJS, gotExt.Code)
	}
	if metaCalls != 1 || bundleCalls != 1 {
		t.Errorf("expected 1 metadata call and 1 bundle call, got meta=%d bundle=%d", metaCalls, bundleCalls)
	}

	// Case 2: include_code=false skips bundle fetch
	res, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "data_app_extension_id", Value: "ext-uuid-9"},
		{Name: "include_code", Value: false},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected invoke error: %v", invokeErr)
	}
	gotExt, ok = res.(*lookercommon.DataAppExtension)
	if !ok {
		t.Fatalf("expected *lookercommon.DataAppExtension, got %T", res)
	}
	if gotExt.Code != nil {
		t.Errorf("expected nil Code when include_code=false, got %q", *gotExt.Code)
	}
	if metaCalls != 2 || bundleCalls != 1 {
		t.Errorf("expected 2 metadata calls and 1 bundle call, got meta=%d bundle=%d", metaCalls, bundleCalls)
	}

	// Case 3: Unauthorized error mapping (401)
	metaStatus = http.StatusUnauthorized
	_, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "data_app_extension_id", Value: "ext-uuid-9"},
		{Name: "include_code", Value: true},
	}, "")
	if invokeErr == nil || !strings.Contains(invokeErr.Error(), "unauthorized error") {
		t.Fatalf("expected unauthorized error, got: %v", invokeErr)
	}

	// Case 4: Not found error mapping (404)
	metaStatus = http.StatusNotFound
	_, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "data_app_extension_id", Value: "ext-uuid-9"},
		{Name: "include_code", Value: true},
	}, "")
	if invokeErr == nil || !strings.Contains(invokeErr.Error(), "404") {
		t.Fatalf("expected 404 error, got: %v", invokeErr)
	}

	// Case 5: Bundle fetch failure propagates error
	metaStatus = http.StatusOK
	bundleStatus = http.StatusNotFound
	_, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "data_app_extension_id", Value: "ext-uuid-9"},
		{Name: "include_code", Value: true},
	}, "")
	if invokeErr == nil || !strings.Contains(invokeErr.Error(), "404") {
		t.Fatalf("expected bundle 404 error, got: %v", invokeErr)
	}
}
