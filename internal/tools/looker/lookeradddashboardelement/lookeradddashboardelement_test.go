// Copyright 2025 Google LLC
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

package lookeradddashboardelement_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources/looker"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookeradddashboardelement"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

func TestParseFromYamlLookerAddDashboardElement(t *testing.T) {
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
            type: looker-add-dashboard-element
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
					Type:   "looker-add-dashboard-element",
					Source: "my-instance",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
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

func TestFailParseFromYamlLookerAddDashboardElement(t *testing.T) {
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
            type: looker-add-dashboard-element
            source: my-instance
            method: GOT
            description: some description
			`,
			err: "error unmarshaling tool: unable to parse tool \"example_tool\" as type \"looker-add-dashboard-element\": [3:1] unknown field \"method\"\n   1 | authRequired: []\n   2 | description: some description\n>  3 | method: GOT\n       ^\n   4 | name: example_tool\n   5 | source: my-instance\n   6 | type: looker-add-dashboard-element",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
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

func TestInvokeLookerAddDashboardElement(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithUserAgent(ctx, "test-agent")

	var gotElement v4.WriteDashboardElement
	var queryCreated bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/4.0/queries"):
			queryCreated = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "999"}`))
		case strings.HasSuffix(r.URL.Path, "/api/4.0/dashboard_elements"):
			if err := json.NewDecoder(r.Body).Decode(&gotElement); err != nil {
				t.Fatalf("failed to decode WriteDashboardElement: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": "1485"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
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
			Name:        "add_dashboard_element",
			Description: "test description",
		},
		Type:   "looker-add-dashboard-element",
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
	boolPtr := func(b bool) *bool { return &b }

	t.Run("slate rich text element", func(t *testing.T) {
		gotElement = v4.WriteDashboardElement{}
		queryCreated = false

		rawArgs := map[string]any{
			"dashboard_id":      "78",
			"type":              "text",
			"body_text":         `[{"type":"h1","children":[{"text":"H1 Heading"}],"align":"center"}]`,
			"rich_content_json": `{"format":"slate"}`,
			"title_text":        "",
			"subtitle_text":     "",
			"title_hidden":      false,
		}
		params, err := parameters.ParseParams(toolParams, rawArgs, nil)
		if err != nil {
			t.Fatalf("ParseParams failed without model/explore/fields: %v", err)
		}

		_, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr != nil {
			t.Fatalf("unexpected invoke error: %v", toolboxErr)
		}
		if queryCreated {
			t.Errorf("expected no query to be created for text element")
		}

		want := v4.WriteDashboardElement{
			DashboardId:     strPtr("78"),
			Type:            strPtr("text"),
			BodyText:        strPtr(`[{"type":"h1","children":[{"text":"H1 Heading"}],"align":"center"}]`),
			RichContentJson: strPtr(`{"format":"slate"}`),
			TitleText:       strPtr(""),
			SubtitleText:    strPtr(""),
			TitleHidden:     boolPtr(false),
		}
		if diff := cmp.Diff(want, gotElement); diff != "" {
			t.Fatalf("WriteDashboardElement mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("markdown/html text element", func(t *testing.T) {
		gotElement = v4.WriteDashboardElement{}
		queryCreated = false

		rawArgs := map[string]any{
			"dashboard_id":  "78",
			"type":          "text",
			"body_text":     "<b>MyText</b>",
			"title_text":    "myTitle",
			"subtitle_text": "mySubtitle",
			"title_hidden":  false,
		}
		params, err := parameters.ParseParams(toolParams, rawArgs, nil)
		if err != nil {
			t.Fatalf("ParseParams failed: %v", err)
		}

		_, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr != nil {
			t.Fatalf("unexpected invoke error: %v", toolboxErr)
		}
		if queryCreated {
			t.Errorf("expected no query to be created for text element")
		}

		want := v4.WriteDashboardElement{
			DashboardId:  strPtr("78"),
			Type:         strPtr("text"),
			BodyText:     strPtr("<b>MyText</b>"),
			TitleText:    strPtr("myTitle"),
			SubtitleText: strPtr("mySubtitle"),
			TitleHidden:  boolPtr(false),
		}
		if diff := cmp.Diff(want, gotElement); diff != "" {
			t.Fatalf("WriteDashboardElement mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("query vis element with notes and subtitle", func(t *testing.T) {
		gotElement = v4.WriteDashboardElement{}
		queryCreated = false

		rawArgs := map[string]any{
			"dashboard_id":     "78",
			"model":            "system__activity",
			"explore":          "look",
			"fields":           []any{"look.count"},
			"title":            "Look Count",
			"subtitle_text":    "Last 30 days",
			"note_text":        "Excludes deleted looks",
			"note_display":     "below",
			"note_state":       "expanded",
			"title_hidden":     true,
			"refresh_interval": "1 hour",
			"vis_config":       map[string]any{"type": "single_value"},
		}
		params, err := parameters.ParseParams(toolParams, rawArgs, nil)
		if err != nil {
			t.Fatalf("ParseParams failed: %v", err)
		}

		_, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr != nil {
			t.Fatalf("unexpected invoke error: %v", toolboxErr)
		}
		if !queryCreated {
			t.Errorf("expected query to be created for vis element")
		}
		if gotElement.Type == nil || *gotElement.Type != "vis" {
			t.Errorf("expected type 'vis', got %v", gotElement.Type)
		}
		if gotElement.QueryId == nil || *gotElement.QueryId != "999" {
			t.Errorf("expected query_id '999', got %v", gotElement.QueryId)
		}
		if gotElement.SubtitleText == nil || *gotElement.SubtitleText != "Last 30 days" {
			t.Errorf("expected subtitle_text 'Last 30 days', got %v", gotElement.SubtitleText)
		}
		if gotElement.NoteText == nil || *gotElement.NoteText != "Excludes deleted looks" {
			t.Errorf("expected note_text 'Excludes deleted looks', got %v", gotElement.NoteText)
		}
		if gotElement.NoteDisplay == nil || *gotElement.NoteDisplay != "below" {
			t.Errorf("expected note_display 'below', got %v", gotElement.NoteDisplay)
		}
		if gotElement.NoteState == nil || *gotElement.NoteState != "expanded" {
			t.Errorf("expected note_state 'expanded', got %v", gotElement.NoteState)
		}
		if gotElement.TitleHidden == nil || *gotElement.TitleHidden != true {
			t.Errorf("expected title_hidden true, got %v", gotElement.TitleHidden)
		}
		if gotElement.RefreshInterval == nil || *gotElement.RefreshInterval != "1 hour" {
			t.Errorf("expected refresh_interval '1 hour', got %v", gotElement.RefreshInterval)
		}
	})

	t.Run("missing query params for non-text element fails", func(t *testing.T) {
		rawArgs := map[string]any{
			"dashboard_id": "78",
			"type":         "vis",
		}
		params, err := parameters.ParseParams(toolParams, rawArgs, nil)
		if err != nil {
			t.Fatalf("ParseParams failed: %v", err)
		}
		_, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr == nil {
			t.Fatalf("expected error when model/explore/fields are missing for 'vis' element")
		}
	})
}
