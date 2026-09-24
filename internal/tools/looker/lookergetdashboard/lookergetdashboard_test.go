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

package lookergetdashboard_test

import (
	"context"
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
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookergetdashboard"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

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
            type: looker-get-dashboard
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
					Type:   "looker-get-dashboard",
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
            type: looker-get-dashboard
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
		Type:   "looker-get-dashboard",
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

	expectedParams := []string{"dashboard_id"}
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
	readOnlyTrue := true
	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test_tool",
			Description: "test description",
			Annotations: &tools.ToolAnnotations{
				ReadOnlyHint: &readOnlyTrue,
			},
		},
		Type:   "looker-get-dashboard",
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
	if *annotations.ReadOnlyHint != true {
		t.Errorf("ReadOnlyHint should be true, got %v", *annotations.ReadOnlyHint)
	}
	if annotations.DestructiveHint == nil {
		t.Fatal("mcp manifest DestructiveHint is nil")
	}
	if *annotations.DestructiveHint != false {
		t.Errorf("DestructiveHint should be false, got %v", *annotations.DestructiveHint)
	}
	if annotations.OpenWorldHint == nil {
		t.Fatal("mcp manifest OpenWorldHint is nil")
	}
	if *annotations.OpenWorldHint != false {
		t.Errorf("OpenWorldHint should be false, got %v", *annotations.OpenWorldHint)
	}
}

func TestInvokeLookerGetDashboard(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithUserAgent(ctx, "test-agent")

	var requestedFields string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/4.0/dashboards/123") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		requestedFields = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "123",
			"title": "Certified Dashboard",
			"description": "Board certified dashboard",
			"view_count": 42,
			"certification_metadata": {
				"certification_status": "certified",
				"user_name": "Data Steward",
				"notes": "Certified for executive reporting",
				"updated_at": "2026-09-15T10:00:00Z"
			}
		}`))
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
	src, err := srcCfg.Initialize(ctx, nil, false)
	if err != nil {
		t.Fatalf("failed to initialize source: %v", err)
	}

	toolCfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "get_dashboard",
			Description: "test description",
		},
		Type:   "looker-get-dashboard",
		Source: "test-looker",
	}
	tool, err := toolCfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	params := parameters.ParamValues{
		{Name: "dashboard_id", Value: "123"},
	}

	got, toolboxErr := tool.Invoke(ctx, src, params, "mock-token")
	if toolboxErr != nil {
		t.Fatalf("unexpected invoke error: %v", toolboxErr)
	}

	if !strings.Contains(requestedFields, "certification_metadata") {
		t.Errorf("expected requested fields to include 'certification_metadata', got %q", requestedFields)
	}

	dash, ok := got.(v4.Dashboard)
	if !ok {
		t.Fatalf("expected v4.Dashboard, got %T", got)
	}
	if dash.CertificationMetadata == nil {
		t.Fatalf("expected CertificationMetadata to be populated, got nil")
	}
	if dash.CertificationMetadata.CertificationStatus == nil || *dash.CertificationMetadata.CertificationStatus != v4.CertificationStatus_Certified {
		t.Errorf("expected certification_status 'certified', got %v", dash.CertificationMetadata.CertificationStatus)
	}
	if dash.CertificationMetadata.UserName == nil || *dash.CertificationMetadata.UserName != "Data Steward" {
		t.Errorf("expected user_name 'Data Steward', got %v", dash.CertificationMetadata.UserName)
	}
	if dash.CertificationMetadata.Notes == nil || *dash.CertificationMetadata.Notes != "Certified for executive reporting" {
		t.Errorf("expected notes 'Certified for executive reporting', got %v", dash.CertificationMetadata.Notes)
	}
	wantTime, _ := time.Parse(time.RFC3339, "2026-09-15T10:00:00Z")
	if dash.CertificationMetadata.UpdatedAt == nil || !dash.CertificationMetadata.UpdatedAt.Equal(wantTime) {
		t.Errorf("expected updated_at %v, got %v", wantTime, dash.CertificationMetadata.UpdatedAt)
	}
}
