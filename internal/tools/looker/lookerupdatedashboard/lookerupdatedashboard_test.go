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

package lookerupdatedashboard_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerupdatedashboard"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

type fakeLookerSource struct {
	settings *rtl.ApiSettings
	sdk      *v4.LookerSDK
}

func (f fakeLookerSource) SourceType() string                  { return "looker" }
func (f fakeLookerSource) ToConfig() sources.SourceConfig      { return nil }
func (f fakeLookerSource) IsReadOnly() bool                    { return false }
func (f fakeLookerSource) UseClientAuthorization() bool        { return false }
func (f fakeLookerSource) GetAuthTokenHeaderName() string      { return "Authorization" }
func (f fakeLookerSource) LookerApiSettings() *rtl.ApiSettings { return f.settings }
func (f fakeLookerSource) GetLookerSDK(context.Context, string) (*v4.LookerSDK, error) {
	return f.sdk, nil
}

func TestParseFromYaml(t *testing.T) {
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
            name: test_tool
            type: looker-update-dashboard
            source: my-instance
            description: some description
                                `,
			want: server.ToolConfigs{
				"test_tool": lkr.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "test_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "looker-update-dashboard",
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

func TestFailParseFromYaml(t *testing.T) {
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
            name: test_tool
            type: looker-update-dashboard
            source: my-instance
            method: GOT
            description: some description
                        `,
			err: "unknown field \"method\"",
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

func TestManifest(t *testing.T) {
	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test_tool",
			Description: "test description",
		},
		Type:   "looker-update-dashboard",
		Source: "my-instance",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	manifest, err := tool.Manifest(nil)
	if err != nil {
		t.Fatalf("Manifest() returned unexpected error: %v", err)
	}
	if manifest.Description != cfg.Description {
		t.Errorf("manifest description mismatch: got %q, want %q", manifest.Description, cfg.Description)
	}

	expectedParams := []string{
		"dashboard_id",
		"title",
		"description",
		"show_dashboard_header",
		"page_side_margins",
		"tile_space_between",
		"tile_shadow",
		"appearance",
		"hidden",
		"refresh_interval",
	}
	for _, p := range expectedParams {
		found := false
		for _, mp := range manifest.Parameters {
			if mp.Name == p {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected parameter %q not found in manifest", p)
		}
	}
}

func TestAnnotations(t *testing.T) {
	readOnlyFalse := false
	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test_tool",
			Description: "test description",
			Annotations: &tools.ToolAnnotations{
				ReadOnlyHint: &readOnlyFalse,
			},
		},
		Type:   "looker-update-dashboard",
		Source: "my-instance",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	annotations := tool.GetAnnotations(nil)
	if annotations == nil {
		t.Fatal("mcp manifest annotations is nil")
	}
	if annotations.ReadOnlyHint == nil {
		t.Fatal("mcp manifest ReadOnlyHint is nil")
	}
	if *annotations.ReadOnlyHint != false {
		t.Errorf("ReadOnlyHint should be false, got %v", *annotations.ReadOnlyHint)
	}
	if annotations.DestructiveHint == nil {
		t.Fatal("mcp manifest DestructiveHint is nil")
	}
	if *annotations.DestructiveHint != true {
		t.Errorf("DestructiveHint should be true, got %v", *annotations.DestructiveHint)
	}
}

func TestInvokeLookerUpdateDashboard(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	var capturedBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/4.0/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "mock-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case r.Method == http.MethodPatch && r.URL.Path == "/api/4.0/dashboards/233":
			bodyBytes, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(bodyBytes, &capturedBody)
			resp := map[string]any{
				"id":    "233",
				"title": "Updated Title",
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	settings := rtl.ApiSettings{
		BaseUrl:      ts.URL,
		ApiVersion:   "4.0",
		ClientId:     "id",
		ClientSecret: "secret",
	}
	sdk := v4.NewLookerSDK(rtl.NewAuthSession(settings))
	src := fakeLookerSource{settings: &settings, sdk: sdk}

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test_tool",
			Description: "test description",
		},
		Type:   "looker-update-dashboard",
		Source: "my-instance",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	toolParams, err := tool.GetParameters(nil)
	if err != nil {
		t.Fatalf("failed to get tool parameters: %v", err)
	}

	rawParams := map[string]any{
		"dashboard_id":          "233",
		"title":                 "Updated Title",
		"show_dashboard_header": false,
		"page_side_margins":     0,
		"tile_space_between":    0,
		"tile_shadow":           false,
	}
	paramValues, err := parameters.ParseParams(toolParams, rawParams, nil)
	if err != nil {
		t.Fatalf("failed to parse params: %v", err)
	}

	res, tbErr := tool.Invoke(ctx, src, paramValues, "")
	if tbErr != nil {
		t.Fatalf("Invoke() failed: %v", tbErr)
	}
	resMap, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", res)
	}
	if resMap["id"] != "233" || resMap["title"] != "Updated Title" {
		t.Errorf("unexpected invoke result: %v", resMap)
	}

	if capturedBody["title"] != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %v", capturedBody["title"])
	}
	appMap, ok := capturedBody["appearance"].(map[string]any)
	if !ok {
		t.Fatalf("expected appearance object in captured body, got %v", capturedBody)
	}
	if appMap["show_dashboard_header"] != false {
		t.Errorf("expected show_dashboard_header false, got %v", appMap["show_dashboard_header"])
	}
	if appMap["page_side_margins"] != float64(0) {
		t.Errorf("expected page_side_margins 0, got %v", appMap["page_side_margins"])
	}
	if appMap["tile_space_between"] != float64(0) {
		t.Errorf("expected tile_space_between 0, got %v", appMap["tile_space_between"])
	}
	if appMap["tile_shadow"] != false {
		t.Errorf("expected tile_shadow false, got %v", appMap["tile_shadow"])
	}
}

func TestInvokeLookerUpdateDashboardNoFields(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	settings := rtl.ApiSettings{
		BaseUrl:      "http://localhost",
		ApiVersion:   "4.0",
		ClientId:     "id",
		ClientSecret: "secret",
	}
	sdk := v4.NewLookerSDK(rtl.NewAuthSession(settings))
	src := fakeLookerSource{settings: &settings, sdk: sdk}

	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test_tool",
			Description: "test description",
		},
		Type:   "looker-update-dashboard",
		Source: "my-instance",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	toolParams, err := tool.GetParameters(nil)
	if err != nil {
		t.Fatalf("failed to get tool parameters: %v", err)
	}

	rawParams := map[string]any{
		"dashboard_id": "233",
	}
	paramValues, err := parameters.ParseParams(toolParams, rawParams, nil)
	if err != nil {
		t.Fatalf("failed to parse params: %v", err)
	}

	_, tbErr := tool.Invoke(ctx, src, paramValues, "")
	if tbErr == nil {
		t.Fatal("expected error when no dashboard fields provided, got nil")
	}
	if !strings.Contains(tbErr.Error(), "no dashboard fields provided to update") {
		t.Errorf("unexpected error: %v", tbErr)
	}
}
