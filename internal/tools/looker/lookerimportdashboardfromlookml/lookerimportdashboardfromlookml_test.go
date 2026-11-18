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

package lookerimportdashboardfromlookml_test

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
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerimportdashboardfromlookml"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

func TestParseFromYamlLookerImportDashboardFromLookml(t *testing.T) {
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
			type: looker-import-dashboard-from-lookml
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
					Type:   "looker-import-dashboard-from-lookml",
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

func TestFailParseFromYamlImportDashboardFromLookml(t *testing.T) {
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
			type: looker-import-dashboard-from-lookml
			source: my-instance
			method: GOT
			description: some description
			`,
			err: "error unmarshaling tool: unable to parse tool \"example_tool\" as type \"looker-import-dashboard-from-lookml\": [3:1] unknown field \"method\"\n   1 | authRequired: []\n   2 | description: some description\n>  3 | method: GOT\n       ^\n   4 | name: example_tool\n   5 | source: my-instance\n   6 | type: looker-import-dashboard-from-lookml",
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

func TestInvokeValidation(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test_tool",
			Description: "test description",
		},
		Type:   "looker-import-dashboard-from-lookml",
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
			desc: "empty lookml",
			params: parameters.ParamValues{
				{Name: "lookml", Value: "   "},
				{Name: "folder", Value: ""},
			},
			wantErr: "'lookml' cannot be empty",
		},
		{
			desc: "non-string lookml",
			params: parameters.ParamValues{
				{Name: "lookml", Value: 123},
				{Name: "folder", Value: ""},
			},
			wantErr: "'lookml' must be a string",
		},
		{
			desc: "non-string folder",
			params: parameters.ParamValues{
				{Name: "lookml", Value: "- dashboard: test"},
				{Name: "folder", Value: 123},
			},
			wantErr: "'folder' must be a string",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := tool.Invoke(ctx, mockSrc, tc.params, "")
			if err == nil {
				t.Fatalf("expect error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("unexpected error: got %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestInvokeSuccess(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	var receivedBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/4.0/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/api/4.0/dashboards/lookml":
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":  "42",
				"url": "/dashboards/42",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "import_dashboard_from_lookml",
			Description: "test description",
		},
		Type:   "looker-import-dashboard-from-lookml",
		Source: "my-instance",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	mockSrc := MockSource{
		baseURL: ts.URL,
		hostURL: "https://looker.example.com",
	}

	// Case 1: folder omitted (empty string) -> folder_id should not be sent in request body
	res, invokeErr := tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "lookml", Value: "- dashboard: test_dash\n  title: Test Dash\n"},
		{Name: "folder", Value: ""},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected invoke error: %v", invokeErr)
	}
	if _, hasFolder := receivedBody["folder_id"]; hasFolder {
		t.Errorf("expected folder_id to be omitted when folder is empty, got %v", receivedBody["folder_id"])
	}
	want := map[string]any{
		"id":  "42",
		"url": "https://looker.example.com/dashboards/42",
	}
	if diff := cmp.Diff(want, res); diff != "" {
		t.Errorf("unexpected response diff: %s", diff)
	}

	// Case 2: folder specified -> folder_id should be sent in request body
	receivedBody = nil
	_, invokeErr = tool.Invoke(ctx, mockSrc, parameters.ParamValues{
		{Name: "lookml", Value: "- dashboard: test_dash\n  title: Test Dash\n"},
		{Name: "folder", Value: "99"},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected invoke error: %v", invokeErr)
	}
	if gotFolder, _ := receivedBody["folder_id"].(string); gotFolder != "99" {
		t.Errorf("expected folder_id '99', got %v", receivedBody["folder_id"])
	}
}
