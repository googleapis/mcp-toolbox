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

package cloudstoragelistbuckets_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragelistbuckets"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlCloudStorageListBuckets(t *testing.T) {
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
			name: list_buckets_tool
			type: cloud-storage-list-buckets
			source: my-gcs
			description: List Cloud Storage buckets
			`,
			want: server.ToolConfigs{
				"list_buckets_tool": cloudstoragelistbuckets.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "list_buckets_tool",
						Description:  "List Cloud Storage buckets",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-list-buckets",
					Source: "my-gcs",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_list_buckets
			type: cloud-storage-list-buckets
			source: prod-gcs
			description: List buckets with authentication
			authRequired:
				- google-auth-service
			`,
			want: server.ToolConfigs{
				"secure_list_buckets": cloudstoragelistbuckets.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_list_buckets",
						Description:  "List buckets with authentication",
						AuthRequired: []string{"google-auth-service"},
					},
					Type:   "cloud-storage-list-buckets",
					Source: "prod-gcs",
				},
			},
		},
		{
			desc: "with configurable parameters",
			in: `
			kind: tool
			name: configured_list_buckets
			type: cloud-storage-list-buckets
			source: prod-gcs
			description: List configured buckets
			project: baked-project
			prefix: logs-
			`,
			want: server.ToolConfigs{
				"configured_list_buckets": cloudstoragelistbuckets.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "configured_list_buckets",
						Description:  "List configured buckets",
						AuthRequired: []string{},
					},
					Type:    "cloud-storage-list-buckets",
					Source:  "prod-gcs",
					Project: strPtr("baked-project"),
					Prefix:  strPtr("logs-"),
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

func strPtr(s string) *string {
	return &s
}

type mockSource struct {
	sources.Source
	gotProject string
	gotPrefix  string
	listCalled bool
}

func (m *mockSource) ListBuckets(ctx context.Context, project, prefix string, maxResults int, pageToken string) (map[string]any, error) {
	m.listCalled = true
	m.gotProject = project
	m.gotPrefix = prefix
	return map[string]any{"buckets": []any{}, "nextPageToken": ""}, nil
}

func initTool(t *testing.T) tools.Tool {
	t.Helper()
	cfg := cloudstoragelistbuckets.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "list_buckets_tool",
			Description: "List buckets",
		},
		Type:   "cloud-storage-list-buckets",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	return tool
}

func TestInvokeMaxResultsValidation(t *testing.T) {
	tcs := []struct {
		desc        string
		maxResults  int
		wantSubstrs []string
	}{
		{desc: "above limit", maxResults: 1001, wantSubstrs: []string{"max_results", "1000"}},
		{desc: "negative", maxResults: -1, wantSubstrs: []string{"max_results", ">= 0"}},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			tool := initTool(t)
			src := &mockSource{}

			params := parameters.ParamValues{
				{Name: "project", Value: ""},
				{Name: "prefix", Value: ""},
				{Name: "max_results", Value: tc.maxResults},
				{Name: "page_token", Value: ""},
			}

			_, toolErr := tool.Invoke(context.Background(), src, params, "")
			if toolErr == nil {
				t.Fatalf("expected error for max_results=%d, got nil", tc.maxResults)
			}
			if _, ok := toolErr.(*util.AgentError); !ok {
				t.Fatalf("expected *util.AgentError, got %T: %v", toolErr, toolErr)
			}
			for _, s := range tc.wantSubstrs {
				if !strings.Contains(toolErr.Error(), s) {
					t.Fatalf("expected error to contain %q, got: %v", s, toolErr)
				}
			}
			if src.listCalled {
				t.Errorf("expected ListBuckets not to be called when validation fails")
			}
		})
	}
}

func TestInvokeProjectPassthrough(t *testing.T) {
	tool := initTool(t)

	tcs := []struct {
		desc       string
		project    string
		wantPassed string
	}{
		{desc: "empty project uses source fallback", project: "", wantPassed: ""},
		{desc: "explicit project overrides", project: "override-project", wantPassed: "override-project"},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			src := &mockSource{}
			params := parameters.ParamValues{
				{Name: "project", Value: tc.project},
				{Name: "prefix", Value: ""},
				{Name: "max_results", Value: 0},
				{Name: "page_token", Value: ""},
			}
			if _, err := tool.Invoke(context.Background(), src, params, ""); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !src.listCalled {
				t.Fatalf("expected ListBuckets to be called")
			}
			if src.gotProject != tc.wantPassed {
				t.Errorf("project forwarded = %q, want %q", src.gotProject, tc.wantPassed)
			}
		})
	}
}

func TestConfiguredParametersHiddenAndForwarded(t *testing.T) {
	cfg := cloudstoragelistbuckets.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "list_buckets_tool",
			Description: "List buckets",
		},
		Type:    "cloud-storage-list-buckets",
		Source:  "my-gcs",
		Project: strPtr("baked-project"),
		Prefix:  strPtr("logs-"),
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"max_results", "page_token"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}

	src := &mockSource{}
	params := parameters.ParamValues{
		{Name: "max_results", Value: 0},
		{Name: "page_token", Value: ""},
	}
	if _, err := tool.Invoke(context.Background(), src, params, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.gotProject != "baked-project" || src.gotPrefix != "logs-" {
		t.Fatalf("forwarded project/prefix = %q/%q, want baked-project/logs-", src.gotProject, src.gotPrefix)
	}
}

func TestUnsetParametersRemainVisible(t *testing.T) {
	tool := initTool(t)
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"project", "prefix", "max_results", "page_token"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}
}

func TestEmptyConfiguredProjectHiddenAndForwarded(t *testing.T) {
	cfg := cloudstoragelistbuckets.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "list_buckets_tool",
			Description: "List buckets",
		},
		Type:    "cloud-storage-list-buckets",
		Source:  "my-gcs",
		Project: strPtr(""),
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"prefix", "max_results", "page_token"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}

	src := &mockSource{}
	params := parameters.ParamValues{
		{Name: "prefix", Value: ""},
		{Name: "max_results", Value: 0},
		{Name: "page_token", Value: ""},
	}
	if _, err := tool.Invoke(context.Background(), src, params, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.gotProject != "" {
		t.Fatalf("project forwarded = %q, want empty source fallback marker", src.gotProject)
	}
}

func manifestParamNames(params []parameters.ParameterManifest) []string {
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Name)
	}
	return names
}
