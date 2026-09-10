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

package cloudstoragedeleteobject_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragedeleteobject"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlCloudStorageDeleteObject(t *testing.T) {
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
			name: delete_tool
			type: cloud-storage-delete-object
			source: my-gcs
			description: Delete a Cloud Storage object
			`,
			want: server.ToolConfigs{
				"delete_tool": cloudstoragedeleteobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "delete_tool",
						Description:  "Delete a Cloud Storage object",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-delete-object",
					Source: "my-gcs",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_delete
			type: cloud-storage-delete-object
			source: prod-gcs
			description: Delete with authentication
			authRequired:
				- google-auth-service
			`,
			want: server.ToolConfigs{
				"secure_delete": cloudstoragedeleteobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_delete",
						Description:  "Delete with authentication",
						AuthRequired: []string{"google-auth-service"},
					},
					Type:   "cloud-storage-delete-object",
					Source: "prod-gcs",
				},
			},
		},
		{
			desc: "with configurable bucket",
			in: `
			kind: tool
			name: configured_delete
			type: cloud-storage-delete-object
			source: prod-gcs
			description: Delete configured object
			bucket: baked-bucket
			`,
			want: server.ToolConfigs{
				"configured_delete": cloudstoragedeleteobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "configured_delete",
						Description:  "Delete configured object",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-delete-object",
					Source: "prod-gcs",
					Bucket: strPtr("baked-bucket"),
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
	called    bool
	gotBucket string
	gotObject string
}

func TestConfiguredBucketHiddenAndForwarded(t *testing.T) {
	cfg := cloudstoragedeleteobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "delete_tool",
			Description: "Delete",
		},
		Type:   "cloud-storage-delete-object",
		Source: "my-gcs",
		Bucket: strPtr("baked-bucket"),
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"object"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}

	src := &mockSource{}
	params := parameters.ParamValues{{Name: "object", Value: "o"}}
	if _, err := tool.Invoke(context.Background(), src, params, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.gotBucket != "baked-bucket" || src.gotObject != "o" {
		t.Fatalf("forwarded params = %q/%q, want baked-bucket/o", src.gotBucket, src.gotObject)
	}
}

func TestUnsetBucketRemainsVisible(t *testing.T) {
	cfg := cloudstoragedeleteobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "delete_tool",
			Description: "Delete",
		},
		Type:   "cloud-storage-delete-object",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"bucket", "object"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}
}

func TestEmptyConfiguredBucketRejected(t *testing.T) {
	cfg := cloudstoragedeleteobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "delete_tool",
			Description: "Delete",
		},
		Type:   "cloud-storage-delete-object",
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

func (m *mockSource) DeleteObject(ctx context.Context, bucket, object string) (map[string]any, error) {
	m.called = true
	m.gotBucket = bucket
	m.gotObject = object
	return map[string]any{"bucket": bucket, "object": object, "deleted": true}, nil
}

func TestInvokeValidation(t *testing.T) {
	cfg := cloudstoragedeleteobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "delete_tool",
			Description: "Delete",
		},
		Type:   "cloud-storage-delete-object",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	tcs := []struct {
		desc       string
		bucket     any
		object     any
		wantErr    bool
		wantSubstr string
		wantCalled bool
	}{
		{desc: "missing bucket", bucket: "", object: "o", wantErr: true, wantSubstr: "bucket"},
		{desc: "missing object", bucket: "b", object: "", wantErr: true, wantSubstr: "object"},
		{desc: "happy path", bucket: "b", object: "o", wantCalled: true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			src := &mockSource{}
			params := parameters.ParamValues{
				{Name: "bucket", Value: tc.bucket},
				{Name: "object", Value: tc.object},
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
			if src.gotBucket != tc.bucket || src.gotObject != tc.object {
				t.Errorf("forwarded params = (%q, %q)", src.gotBucket, src.gotObject)
			}
		})
	}
}
