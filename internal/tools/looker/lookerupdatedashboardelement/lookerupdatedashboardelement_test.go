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

package lookerupdatedashboardelement_test

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
	"github.com/googleapis/mcp-toolbox/internal/sources/looker"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookerupdatedashboardelement"
	"github.com/googleapis/mcp-toolbox/internal/util"
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
            type: looker-update-dashboard-element
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
					Type:   "looker-update-dashboard-element",
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
            type: looker-update-dashboard-element
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
		Type:   "looker-update-dashboard-element",
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
		"model",
		"explore",
		"fields",
		"filters",
		"pivots",
		"sorts",
		"limit",
		"tz",
		"filter_expression",
		"dynamic_fields",
		"dashboard_id",
		"dashboard_element_id",
		"title",
		"vis_config",
		"dashboard_filters",
		"type",
		"body_text",
		"title_text",
		"subtitle_text",
		"rich_content_json",
		"note_text",
		"note_display",
		"note_state",
		"title_hidden",
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
		Type:   "looker-update-dashboard-element",
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
	if annotations.OpenWorldHint == nil {
		t.Fatal("mcp manifest OpenWorldHint is nil")
	}
	if *annotations.OpenWorldHint != false {
		t.Errorf("OpenWorldHint should be false, got %v", *annotations.OpenWorldHint)
	}
}

func TestInvokeLookerUpdateDashboardElement(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithUserAgent(ctx, "test-agent")

	var gotElement v4.WriteDashboardElement
	var gotElementID string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/api/4.0/dashboard_elements/") {
			parts := strings.Split(r.URL.Path, "/")
			gotElementID = parts[len(parts)-1]
			if err := json.NewDecoder(r.Body).Decode(&gotElement); err != nil {
				t.Fatalf("failed to decode WriteDashboardElement: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "1486"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	srcCfg := looker.Config{
		Name:            "test-looker",
		Type:            "looker",
		BaseURL:         ts.URL,
		UseClientOAuth:  "true",
		Timeout:         "5s",
		SslVerification: false,
	}
	src, err := srcCfg.Initialize(ctx, nil)
	if err != nil {
		t.Fatalf("failed to initialize source: %v", err)
	}

	toolCfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "update_dashboard_element",
			Description: "test description",
		},
		Type:   "looker-update-dashboard-element",
		Source: "test-looker",
	}
	tool, err := toolCfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	toolParams, err := tool.GetParameters(nil)
	if err != nil {
		t.Fatalf("failed to get tool parameters: %v", err)
	}

	strPtr := func(s string) *string { return &s }

	rawArgs := map[string]any{
		"dashboard_id":         "78",
		"dashboard_element_id": "1486",
		"type":                 "text",
		"body_text":            "<b>Updated Text</b>",
		"title_text":           "Updated Title",
		"subtitle_text":        "Updated Subtitle",
	}
	params, err := parameters.ParseParams(toolParams, rawArgs, nil)
	if err != nil {
		t.Fatalf("ParseParams failed: %v", err)
	}

	_, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
	if toolboxErr != nil {
		t.Fatalf("unexpected invoke error: %v", toolboxErr)
	}

	if gotElementID != "1486" {
		t.Errorf("expected element ID '1486', got %q", gotElementID)
	}
	want := v4.WriteDashboardElement{
		DashboardId:  strPtr("78"),
		Type:         strPtr("text"),
		BodyText:     strPtr("<b>Updated Text</b>"),
		TitleText:    strPtr("Updated Title"),
		SubtitleText: strPtr("Updated Subtitle"),
	}
	if diff := cmp.Diff(want, gotElement); diff != "" {
		t.Fatalf("WriteDashboardElement mismatch (-want +got):\n%s", diff)
	}
}

func TestInvokeDashboardFilters(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tcs := []struct {
		desc          string
		dashFilters   any
		wantListen    []map[string]any
		wantNilFilter bool
	}{
		{
			desc: "groups multiple dashboard filters into a single filterable listen array",
			dashFilters: []any{
				map[string]any{"dashboard_filter_name": "Status Filter", "field": "orders.status"},
				map[string]any{"dashboard_filter_name": "State Filter", "field": "users.state"},
			},
			wantListen: []map[string]any{
				{"dashboard_filter_name": "Status Filter", "field": "orders.status"},
				{"dashboard_filter_name": "State Filter", "field": "users.state"},
			},
		},
		{
			desc:          "omits filterables entries when no dashboard filters are specified",
			dashFilters:   []any{},
			wantNilFilter: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			var capturedBody map[string]any
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/4.0/queries":
					_, _ = w.Write([]byte(`{"id":"77"}`))
				case r.Method == http.MethodPatch && r.URL.Path == "/api/4.0/dashboard_elements/12":
					bodyBytes, readErr := io.ReadAll(r.Body)
					if readErr != nil {
						t.Errorf("failed reading body: %v", readErr)
					}
					if uErr := json.Unmarshal(bodyBytes, &capturedBody); uErr != nil {
						t.Errorf("failed unmarshaling body: %v", uErr)
					}
					_, _ = w.Write([]byte(`{"id":"12","title":"Updated Tile"}`))
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
					Name:        "update_dashboard_element",
					Description: "Update dashboard element",
				},
				Type:   "looker-update-dashboard-element",
				Source: "my-instance",
			}
			tool, err := cfg.Initialize(ctx)
			if err != nil {
				t.Fatalf("Initialize failed: %v", err)
			}

			toolParams, err := tool.GetParameters(nil)
			if err != nil {
				t.Fatalf("GetParameters failed: %v", err)
			}
			params, pErr := parameters.ParseParams(toolParams, map[string]any{
				"dashboard_id":         "5",
				"dashboard_element_id": "12",
				"title":                "Updated Tile",
				"model":                "thelook",
				"explore":              "orders",
				"fields":               []any{"orders.status", "users.state", "orders.count"},
				"dashboard_filters":    tc.dashFilters,
			}, nil)
			if pErr != nil {
				t.Fatalf("ParseParams failed: %v", pErr)
			}

			_, tbErr := tool.Invoke(ctx, fakeLookerSource{settings: settings, sdk: sdk}, params, "")
			if tbErr != nil {
				t.Fatalf("Invoke failed: %v", tbErr)
			}

			rm, ok := capturedBody["result_maker"].(map[string]any)
			if !ok {
				t.Fatalf("expected result_maker in patch body, got %v", capturedBody)
			}
			if tc.wantNilFilter {
				if rm["filterables"] != nil {
					t.Fatalf("expected nil filterables when no dashboard_filters are provided, got %v", rm["filterables"])
				}
				return
			}

			filterables, ok := rm["filterables"].([]any)
			if !ok || len(filterables) != 1 {
				t.Fatalf("expected exactly 1 ResultMakerFilterables entry, got %v", rm["filterables"])
			}
			fb0, ok := filterables[0].(map[string]any)
			if !ok {
				t.Fatalf("expected filterables[0] to be map, got %T", filterables[0])
			}
			listenRaw, ok := fb0["listen"].([]any)
			if !ok {
				t.Fatalf("expected filterables[0].listen to be slice, got %v", fb0["listen"])
			}
			gotListen := make([]map[string]any, 0, len(listenRaw))
			for _, item := range listenRaw {
				m, _ := item.(map[string]any)
				gotListen = append(gotListen, m)
			}
			if diff := cmp.Diff(tc.wantListen, gotListen); diff != "" {
				t.Errorf("filterables[0].listen mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
