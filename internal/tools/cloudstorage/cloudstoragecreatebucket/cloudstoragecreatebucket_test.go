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

package cloudstoragecreatebucket_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragecreatebucket"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlCloudStorageCreateBucket(t *testing.T) {
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
			name: create_bucket_tool
			type: cloud-storage-create-bucket
			source: my-gcs
			description: Create a Cloud Storage bucket
			`,
			want: server.ToolConfigs{
				"create_bucket_tool": cloudstoragecreatebucket.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "create_bucket_tool",
						Description:  "Create a Cloud Storage bucket",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-create-bucket",
					Source: "my-gcs",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_create_bucket
			type: cloud-storage-create-bucket
			source: prod-gcs
			description: Create bucket with authentication
			authRequired:
				- google-auth-service
			`,
			want: server.ToolConfigs{
				"secure_create_bucket": cloudstoragecreatebucket.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_create_bucket",
						Description:  "Create bucket with authentication",
						AuthRequired: []string{"google-auth-service"},
					},
					Type:   "cloud-storage-create-bucket",
					Source: "prod-gcs",
				},
			},
		},
		{
			desc: "with configurable parameters",
			in: `
			kind: tool
			name: configured_create_bucket
			type: cloud-storage-create-bucket
			source: prod-gcs
			description: Create configured bucket
			project: baked-project
			location: US
			uniform_bucket_level_access: true
			`,
			want: server.ToolConfigs{
				"configured_create_bucket": cloudstoragecreatebucket.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "configured_create_bucket",
						Description:  "Create configured bucket",
						AuthRequired: []string{},
					},
					Type:                     "cloud-storage-create-bucket",
					Source:                   "prod-gcs",
					Project:                  strPtr("baked-project"),
					Location:                 strPtr("US"),
					UniformBucketLevelAccess: boolPtr(true),
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

func boolPtr(b bool) *bool {
	return &b
}

type mockSource struct {
	sources.Source
	called                      bool
	gotBucket                   string
	gotProject                  string
	gotLocation                 string
	gotUniformBucketLevelAccess bool
}

func (m *mockSource) CreateBucket(ctx context.Context, bucket, project, location string, uniformBucketLevelAccess bool) (map[string]any, error) {
	m.called = true
	m.gotBucket = bucket
	m.gotProject = project
	m.gotLocation = location
	m.gotUniformBucketLevelAccess = uniformBucketLevelAccess
	return map[string]any{"bucket": bucket, "created": true}, nil
}

func initTool(t *testing.T) tools.Tool {
	t.Helper()
	cfg := cloudstoragecreatebucket.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "create_bucket_tool",
			Description: "Create bucket",
		},
		Type:   "cloud-storage-create-bucket",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	return tool
}

func TestInvokeValidationAndForwarding(t *testing.T) {
	tcs := []struct {
		desc        string
		bucket      any
		project     any
		location    any
		uniform     any
		wantErr     bool
		wantCalled  bool
		wantBucket  string
		wantProject string
		wantLoc     string
		wantUniform bool
		wantSubstr  string
	}{
		{desc: "missing bucket", bucket: "", location: "US", uniform: false, wantErr: true, wantSubstr: "bucket"},
		{desc: "happy path", bucket: "b", project: "override-project", location: "EU", uniform: true, wantCalled: true, wantBucket: "b", wantProject: "override-project", wantLoc: "EU", wantUniform: true},
		{desc: "omitted location", bucket: "b", uniform: false, wantCalled: true, wantBucket: "b"},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			tool := initTool(t)
			src := &mockSource{}
			params := parameters.ParamValues{
				{Name: "bucket", Value: tc.bucket},
				{Name: "project", Value: tc.project},
				{Name: "uniform_bucket_level_access", Value: tc.uniform},
			}
			if tc.location != nil {
				params = append(params, parameters.ParamValue{Name: "location", Value: tc.location})
			}
			_, toolErr := tool.Invoke(context.Background(), src, params, "")
			if tc.wantErr {
				if toolErr == nil {
					t.Fatalf("expected error, got nil")
				}
				if _, ok := toolErr.(*util.AgentError); !ok {
					t.Fatalf("expected *AgentError, got %T: %v", toolErr, toolErr)
				}
				if !strings.Contains(toolErr.Error(), tc.wantSubstr) {
					t.Errorf("error %q does not contain %q", toolErr, tc.wantSubstr)
				}
				if src.called {
					t.Errorf("expected source not to be called on validation failure")
				}
				return
			}
			if toolErr != nil {
				t.Fatalf("unexpected error: %v", toolErr)
			}
			if src.called != tc.wantCalled {
				t.Errorf("called = %v, want %v", src.called, tc.wantCalled)
			}
			if src.gotBucket != tc.wantBucket || src.gotProject != tc.wantProject || src.gotLocation != tc.wantLoc || src.gotUniformBucketLevelAccess != tc.wantUniform {
				t.Errorf("forwarded params = (%q, %q, %q, %v)", src.gotBucket, src.gotProject, src.gotLocation, src.gotUniformBucketLevelAccess)
			}
		})
	}
}

func TestConfiguredParametersHiddenAndForwarded(t *testing.T) {
	cfg := cloudstoragecreatebucket.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "create_bucket_tool",
			Description: "Create bucket",
		},
		Type:                     "cloud-storage-create-bucket",
		Source:                   "my-gcs",
		Project:                  strPtr("baked-project"),
		Location:                 strPtr("US"),
		UniformBucketLevelAccess: boolPtr(true),
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"bucket"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}

	src := &mockSource{}
	params := parameters.ParamValues{{Name: "bucket", Value: "b"}}
	if _, err := tool.Invoke(context.Background(), src, params, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.gotProject != "baked-project" || src.gotLocation != "US" || !src.gotUniformBucketLevelAccess {
		t.Fatalf("forwarded project/location/uniform = %q/%q/%v, want baked-project/US/true", src.gotProject, src.gotLocation, src.gotUniformBucketLevelAccess)
	}
}

func TestUnsetParametersRemainVisible(t *testing.T) {
	tool := initTool(t)
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"bucket", "project", "location", "uniform_bucket_level_access"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}
}

func TestEmptyConfiguredProjectHiddenAndForwarded(t *testing.T) {
	cfg := cloudstoragecreatebucket.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "create_bucket_tool",
			Description: "Create bucket",
		},
		Type:    "cloud-storage-create-bucket",
		Source:  "my-gcs",
		Project: strPtr(""),
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"bucket", "location", "uniform_bucket_level_access"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}

	src := &mockSource{}
	params := parameters.ParamValues{
		{Name: "bucket", Value: "b"},
		{Name: "location", Value: ""},
		{Name: "uniform_bucket_level_access", Value: false},
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
