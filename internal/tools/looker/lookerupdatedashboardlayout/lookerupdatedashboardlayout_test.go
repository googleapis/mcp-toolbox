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

package lookerupdatedashboardlayout_test

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
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerupdatedashboardlayout"
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
            type: looker-update-dashboard-layout
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
					Type:   "looker-update-dashboard-layout",
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
            type: looker-update-dashboard-layout
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
		Type:   "looker-update-dashboard-layout",
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
		"dashboard_layout_id",
		"type",
		"active",
		"label",
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
		Type:   "looker-update-dashboard-layout",
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

func TestInvokeLookerUpdateDashboardLayout(t *testing.T) {
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
		case r.Method == http.MethodPatch && r.URL.Path == "/api/4.0/dashboard_layouts/369":
			bodyBytes, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(bodyBytes, &capturedBody)
			resp := map[string]any{
				"id":     "369",
				"type":   "extension",
				"active": true,
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
		Type:   "looker-update-dashboard-layout",
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

	layoutId := "369"
	extType := "extension"
	active := true
	label := "Main App"
	rawParams := map[string]any{
		"dashboard_layout_id": layoutId,
		"type":                extType,
		"active":              active,
		"label":               label,
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
	if resMap["id"] != "369" || resMap["type"] != "extension" || resMap["active"] != true {
		t.Errorf("unexpected invoke result: %v", resMap)
	}
	if capturedBody["type"] != "extension" || capturedBody["active"] != true || capturedBody["label"] != "Main App" {
		t.Errorf("unexpected captured body: %v", capturedBody)
	}
}

func TestInvokeLookerUpdateDashboardLayoutNoFields(t *testing.T) {
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
		Type:   "looker-update-dashboard-layout",
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
		"dashboard_layout_id": "369",
	}
	paramValues, err := parameters.ParseParams(toolParams, rawParams, nil)
	if err != nil {
		t.Fatalf("failed to parse params: %v", err)
	}

	_, tbErr := tool.Invoke(ctx, src, paramValues, "")
	if tbErr == nil {
		t.Fatal("expected error when no updatable fields provided, got nil")
	}
	if !strings.Contains(tbErr.Error(), "at least one of 'type', 'active', or 'label' must be provided to update") {
		t.Errorf("unexpected error: %v", tbErr)
	}
}
