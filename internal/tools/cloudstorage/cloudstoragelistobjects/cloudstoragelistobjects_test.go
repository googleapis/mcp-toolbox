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

package cloudstoragelistobjects_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragelistobjects"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlCloudStorageListObjects(t *testing.T) {
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
			name: list_objects_tool
			type: cloud-storage-list-objects
			source: my-gcs
			description: List objects in a Cloud Storage bucket
			`,
			want: server.ToolConfigs{
				"list_objects_tool": cloudstoragelistobjects.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "list_objects_tool",
						Description:  "List objects in a Cloud Storage bucket",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-list-objects",
					Source: "my-gcs",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_list_objects
			type: cloud-storage-list-objects
			source: prod-gcs
			description: List objects with authentication
			authRequired:
				- google-auth-service
				- api-key-service
			`,
			want: server.ToolConfigs{
				"secure_list_objects": cloudstoragelistobjects.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_list_objects",
						Description:  "List objects with authentication",
						AuthRequired: []string{"google-auth-service", "api-key-service"},
					},
					Type:   "cloud-storage-list-objects",
					Source: "prod-gcs",
				},
			},
		},
		{
			desc: "with configurable parameters",
			in: `
			kind: tool
			name: configured_list_objects
			type: cloud-storage-list-objects
			source: prod-gcs
			description: List configured objects
			bucket: baked-bucket
			prefix: logs/
			delimiter: /
			`,
			want: server.ToolConfigs{
				"configured_list_objects": cloudstoragelistobjects.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "configured_list_objects",
						Description:  "List configured objects",
						AuthRequired: []string{},
					},
					Type:      "cloud-storage-list-objects",
					Source:    "prod-gcs",
					Bucket:    strPtr("baked-bucket"),
					Prefix:    strPtr("logs/"),
					Delimiter: strPtr("/"),
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
	listCalled   bool
	gotBucket    string
	gotPrefix    string
	gotDelimiter string
}

func (m *mockSource) ListObjects(ctx context.Context, bucket, prefix, delimiter string, maxResults int, pageToken string) (map[string]any, error) {
	m.listCalled = true
	m.gotBucket = bucket
	m.gotPrefix = prefix
	m.gotDelimiter = delimiter
	return map[string]any{"objects": []any{}, "prefixes": []string{}, "nextPageToken": ""}, nil
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
			cfg := cloudstoragelistobjects.Config{
				ConfigBase: tools.ConfigBase{
					Name:        "list_objects_tool",
					Description: "List objects",
				},
				Type:   "cloud-storage-list-objects",
				Source: "my-gcs",
			}
			tool, err := cfg.Initialize(context.Background())
			if err != nil {
				t.Fatalf("failed to initialize tool: %v", err)
			}

			src := &mockSource{}

			params := parameters.ParamValues{
				{Name: "bucket", Value: "my-bucket"},
				{Name: "prefix", Value: ""},
				{Name: "delimiter", Value: ""},
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
				t.Errorf("expected ListObjects not to be called when validation fails")
			}
		})
	}
}

func TestConfiguredParametersHiddenAndForwarded(t *testing.T) {
	cfg := cloudstoragelistobjects.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "list_objects_tool",
			Description: "List objects",
		},
		Type:      "cloud-storage-list-objects",
		Source:    "my-gcs",
		Bucket:    strPtr("baked-bucket"),
		Prefix:    strPtr("logs/"),
		Delimiter: strPtr("/"),
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
	if src.gotBucket != "baked-bucket" || src.gotPrefix != "logs/" || src.gotDelimiter != "/" {
		t.Fatalf("forwarded bucket/prefix/delimiter = %q/%q/%q, want baked-bucket/logs//", src.gotBucket, src.gotPrefix, src.gotDelimiter)
	}
}

func TestUnsetParametersRemainVisible(t *testing.T) {
	cfg := cloudstoragelistobjects.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "list_objects_tool",
			Description: "List objects",
		},
		Type:   "cloud-storage-list-objects",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"bucket", "prefix", "delimiter", "max_results", "page_token"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}
}

func TestEmptyConfiguredBucketRejected(t *testing.T) {
	cfg := cloudstoragelistobjects.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "list_objects_tool",
			Description: "List objects",
		},
		Type:   "cloud-storage-list-objects",
		Source: "my-gcs",
		Bucket: strPtr(""),
	}
	if _, err := cfg.Initialize(context.Background()); err == nil || !strings.Contains(err.Error(), "bucket") {
		t.Fatalf("Initialize() error = %v, want bucket error", err)
	}
}

func manifestParamNames(params []parameters.ParameterManifest) []string {
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Name)
	}
	return names
}
