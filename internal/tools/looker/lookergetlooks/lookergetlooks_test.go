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

package lookergetlooks_test

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
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetlooks"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

func TestParseFromYamlLookerGetLooks(t *testing.T) {
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
            type: looker-get-looks
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
					Type:   "looker-get-looks",
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

func TestFailParseFromYamlLookerGetLooks(t *testing.T) {
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
            type: looker-get-looks
            source: my-instance
            method: GOT
            description: some description
			`,
			err: "error unmarshaling tool: unable to parse tool \"example_tool\" as type \"looker-get-looks\": [3:1] unknown field \"method\"\n   1 | authRequired: []\n   2 | description: some description\n>  3 | method: GOT\n       ^\n   4 | name: example_tool\n   5 | source: my-instance\n   6 | type: looker-get-looks",
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

func TestInvokeLookerGetLooks(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithUserAgent(ctx, "test-agent")

	var requestedFields string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/4.0/looks/search") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		requestedFields = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{
				"id": "201",
				"title": "Monthly Active Users",
				"description": "Certified MAU look",
				"model": {"id": "growth_model"},
				"certification_metadata": {
					"certification_status": "certified",
					"user_name": "Alex Smith",
					"notes": "Approved by Analytics Engineering",
					"updated_at": "2026-09-15T10:00:00Z"
				}
			},
			{
				"id": "202",
				"title": "Test Look",
				"description": "Unverified look",
				"model": {"id": "growth_model"}
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
			Name:        "get_looks",
			Description: "test description",
		},
		Type:   "looker-get-looks",
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

	if requestedFields != "id,title,description,model,certification_metadata" {
		t.Errorf("expected fields query param 'id,title,description,model,certification_metadata', got %q", requestedFields)
	}

	gotList, ok := got.([]any)
	if !ok || len(gotList) != 2 {
		t.Fatalf("expected 2 results, got %#v", got)
	}

	first, ok := gotList[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map for first result, got %T", gotList[0])
	}
	if first["id"] != "201" || first["model_id"] != "growth_model" {
		t.Errorf("unexpected first look fields: %v", first)
	}
	cert, ok := first["certification_metadata"].(*v4.Certification)
	if !ok || cert == nil {
		t.Fatalf("expected certification_metadata in first result, got %#v", first["certification_metadata"])
	}
	if cert.CertificationStatus == nil || *cert.CertificationStatus != v4.CertificationStatus_Certified {
		t.Errorf("expected certification_status 'certified', got %v", cert.CertificationStatus)
	}
	if cert.UserName == nil || *cert.UserName != "Alex Smith" {
		t.Errorf("expected user_name 'Alex Smith', got %v", cert.UserName)
	}
	if cert.Notes == nil || *cert.Notes != "Approved by Analytics Engineering" {
		t.Errorf("expected notes 'Approved by Analytics Engineering', got %v", cert.Notes)
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
