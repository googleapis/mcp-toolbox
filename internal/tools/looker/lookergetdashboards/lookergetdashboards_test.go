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

package lookergetdashboards_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources/looker"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetdashboards"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

func TestParseFromYamlLookerGetDashboards(t *testing.T) {
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
            type: looker-get-dashboards
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
					Type:   "looker-get-dashboards",
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

func TestFailParseFromYamlLookerGetDashboards(t *testing.T) {
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
            type: looker-get-dashboards
            source: my-instance
            method: GOT
            description: some description
			`,
			err: "error unmarshaling tool: unable to parse tool \"example_tool\" as type \"looker-get-dashboards\": [3:1] unknown field \"method\"\n   1 | authRequired: []\n   2 | description: some description\n>  3 | method: GOT\n       ^\n   4 | name: example_tool\n   5 | source: my-instance\n   6 | type: looker-get-dashboards",
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

func TestInvokeLookerGetDashboards(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithUserAgent(ctx, "test-agent")

	var requestedFields string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/4.0/dashboards/search") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		requestedFields = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{
				"id": "101",
				"title": "Executive Sales Dashboard",
				"description": "Official Q3 revenue numbers",
				"certification_metadata": {
					"certification_status": "certified",
					"user_name": "Jane Doe",
					"notes": "Verified by Finance team",
					"updated_at": "2026-09-15T10:00:00Z"
				}
			},
			{
				"id": "102",
				"title": "Scratch Dashboard",
				"description": "Draft analysis"
			}
		]`))
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
			Name:        "get_dashboards",
			Description: "test description",
		},
		Type:   "looker-get-dashboards",
		Source: "test-looker",
	}
	tool, err := toolCfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	params := parameters.ParamValues{
		{Name: "title", Value: "%"},
		{Name: "desc", Value: ""},
		{Name: "limit", Value: 100},
		{Name: "offset", Value: 0},
	}

	got, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
	if toolboxErr != nil {
		t.Fatalf("unexpected invoke error: %v", toolboxErr)
	}

	if requestedFields != "id,title,description,certification_metadata" {
		t.Errorf("expected fields query param 'id,title,description,certification_metadata', got %q", requestedFields)
	}

	gotList, ok := got.([]any)
	if !ok || len(gotList) != 2 {
		t.Fatalf("expected 2 results, got %#v", got)
	}

	first, ok := gotList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map for first result, got %T", gotList[0])
	}
	if first["id"] != "101" || first["title"] != "Executive Sales Dashboard" {
		t.Errorf("unexpected first dashboard fields: %v", first)
	}
	cert, ok := first["certification_metadata"].(*v4.Certification)
	if !ok || cert == nil {
		t.Fatalf("expected certification_metadata in first result, got %#v", first["certification_metadata"])
	}
	if cert.CertificationStatus == nil || *cert.CertificationStatus != v4.CertificationStatus_Certified {
		t.Errorf("expected certification_status 'certified', got %v", cert.CertificationStatus)
	}
	if cert.UserName == nil || *cert.UserName != "Jane Doe" {
		t.Errorf("expected user_name 'Jane Doe', got %v", cert.UserName)
	}
	if cert.Notes == nil || *cert.Notes != "Verified by Finance team" {
		t.Errorf("expected notes 'Verified by Finance team', got %v", cert.Notes)
	}
	wantTime, _ := time.Parse(time.RFC3339, "2026-09-15T10:00:00Z")
	if cert.UpdatedAt == nil || !cert.UpdatedAt.Equal(wantTime) {
		t.Errorf("expected updated_at %v, got %v", wantTime, cert.UpdatedAt)
	}

	second, ok := gotList[1].(map[string]any)
	if !ok {
		t.Fatalf("expected map for second result, got %T", gotList[1])
	}
	if _, exists := second["certification_metadata"]; exists {
		t.Errorf("expected certification_metadata to be omitted when nil, got %v", second["certification_metadata"])
	}
}

func TestInvokeLookerGetDashboardsWithLookmlDashboards(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithUserAgent(ctx, "test-agent")

	var requestedLookmlFields string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/4.0/dashboards/search"):
			titleParam := r.URL.Query().Get("title")
			if titleParam != "" && titleParam != "%" && !strings.Contains(strings.ToLower("Executive Sales Dashboard"), strings.ToLower(strings.Trim(titleParam, "%"))) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`[]`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": "101",
					"title": "Executive Sales Dashboard",
					"description": "Official Q3 revenue numbers"
				}
			]`))
		case strings.HasSuffix(r.URL.Path, "/api/4.0/dashboards/lookml/search"):
			requestedLookmlFields = r.URL.Query().Get("fields")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[
				{
					"id": "thelook::business_pulse",
					"title": "Business Pulse",
					"description": "Core e-commerce KPIs"
				},
				{
					"id": "thelook::customer_overview",
					"title": "Customer Overview",
					"description": "Cohort retention metrics"
				}
			]`))
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
			Name:        "get_dashboards",
			Description: "test description",
		},
		Type:   "looker-get-dashboards",
		Source: "test-looker",
	}
	tool, err := toolCfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	t.Run("returns both user-defined and LookML dashboards", func(t *testing.T) {
		params := parameters.ParamValues{
			{Name: "title", Value: "%"},
			{Name: "desc", Value: ""},
			{Name: "limit", Value: 100},
			{Name: "offset", Value: 0},
		}
		got, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr != nil {
			t.Fatalf("unexpected invoke error: %v", toolboxErr)
		}
		if requestedLookmlFields != "id,title,description,certification_metadata" {
			t.Errorf("expected LookML fields query param 'id,title,description,certification_metadata', got %q", requestedLookmlFields)
		}
		gotList, ok := got.([]any)
		if !ok || len(gotList) != 3 {
			t.Fatalf("expected 3 results (1 user-defined + 2 LookML), got %#v", got)
		}
		second := gotList[1].(map[string]any)
		if second["id"] != "thelook::business_pulse" || second["title"] != "Business Pulse" {
			t.Errorf("unexpected LookML dashboard: %v", second)
		}
	})

	t.Run("matches LookML dashboard by LookML ID (model::dashboard_name)", func(t *testing.T) {
		params := parameters.ParamValues{
			{Name: "title", Value: "thelook::business_pulse"},
			{Name: "desc", Value: ""},
			{Name: "limit", Value: 100},
			{Name: "offset", Value: 0},
		}
		got, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr != nil {
			t.Fatalf("unexpected invoke error: %v", toolboxErr)
		}
		gotList, ok := got.([]any)
		if !ok || len(gotList) != 1 {
			t.Fatalf("expected 1 LookML dashboard matching ID, got %#v", got)
		}
		first := gotList[0].(map[string]any)
		if first["id"] != "thelook::business_pulse" {
			t.Errorf("expected id 'thelook::business_pulse', got %v", first["id"])
		}
	})

	t.Run("matches LookML dashboard by display title wildcard and description", func(t *testing.T) {
		params := parameters.ParamValues{
			{Name: "title", Value: "%Customer%"},
			{Name: "desc", Value: "%Cohort%"},
			{Name: "limit", Value: 100},
			{Name: "offset", Value: 0},
		}
		got, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr != nil {
			t.Fatalf("unexpected invoke error: %v", toolboxErr)
		}
		gotList, ok := got.([]any)
		if !ok || len(gotList) != 1 {
			t.Fatalf("expected 1 LookML dashboard matching title and desc, got %#v", got)
		}
		first := gotList[0].(map[string]any)
		if first["id"] != "thelook::customer_overview" {
			t.Errorf("expected id 'thelook::customer_overview', got %v", first["id"])
		}
	})

	t.Run("respects limit across combined dashboards", func(t *testing.T) {
		params := parameters.ParamValues{
			{Name: "title", Value: "%"},
			{Name: "desc", Value: ""},
			{Name: "limit", Value: 2},
			{Name: "offset", Value: 0},
		}
		got, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr != nil {
			t.Fatalf("unexpected invoke error: %v", toolboxErr)
		}
		gotList, ok := got.([]any)
		if !ok || len(gotList) != 2 {
			t.Fatalf("expected 2 results due to limit=2, got %#v", got)
		}
	})

	t.Run("respects offset across combined dashboards", func(t *testing.T) {
		params := parameters.ParamValues{
			{Name: "title", Value: "%"},
			{Name: "desc", Value: ""},
			{Name: "limit", Value: 2},
			{Name: "offset", Value: 1},
		}
		got, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr != nil {
			t.Fatalf("unexpected invoke error: %v", toolboxErr)
		}
		gotList, ok := got.([]any)
		if !ok || len(gotList) != 2 {
			t.Fatalf("expected 2 LookML results after skipping 1 UDD with offset=1, got %#v", got)
		}
		first := gotList[0].(map[string]any)
		second := gotList[1].(map[string]any)
		if first["id"] != "thelook::business_pulse" || second["id"] != "thelook::customer_overview" {
			t.Errorf("expected LookML dashboards after offset=1, got %v and %v", first["id"], second["id"])
		}
	})

	t.Run("matches LookML dashboard with single-character underscore wildcard without percent", func(t *testing.T) {
		params := parameters.ParamValues{
			{Name: "title", Value: "P_lse"},
			{Name: "desc", Value: ""},
			{Name: "limit", Value: 100},
			{Name: "offset", Value: 0},
		}
		got, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr != nil {
			t.Fatalf("unexpected invoke error: %v", toolboxErr)
		}
		gotList, ok := got.([]any)
		if !ok || len(gotList) != 1 {
			t.Fatalf("expected 1 LookML dashboard matching 'P_lse', got %#v", got)
		}
		first := gotList[0].(map[string]any)
		if first["id"] != "thelook::business_pulse" {
			t.Errorf("expected id 'thelook::business_pulse', got %v", first["id"])
		}
	})

	t.Run("normalizes negative limit and offset to zero", func(t *testing.T) {
		params := parameters.ParamValues{
			{Name: "title", Value: "%"},
			{Name: "desc", Value: ""},
			{Name: "limit", Value: -5},
			{Name: "offset", Value: -2},
		}
		got, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
		if toolboxErr != nil {
			t.Fatalf("unexpected invoke error: %v", toolboxErr)
		}
		gotList, ok := got.([]any)
		if !ok || len(gotList) != 3 {
			t.Fatalf("expected all 3 dashboards when limit < 0 is normalized to 0, got %#v", got)
		}
	})
}
