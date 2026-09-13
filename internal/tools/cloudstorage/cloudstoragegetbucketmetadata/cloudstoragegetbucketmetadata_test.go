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

package cloudstoragegetbucketmetadata_test

import (
	"context"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragegetbucketmetadata"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlCloudStorageGetBucketMetadata(t *testing.T) {
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
			name: bucket_metadata_tool
			type: cloud-storage-get-bucket-metadata
			source: my-gcs
			description: Get bucket metadata
			`,
			want: server.ToolConfigs{
				"bucket_metadata_tool": cloudstoragegetbucketmetadata.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "bucket_metadata_tool",
						Description:  "Get bucket metadata",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-get-bucket-metadata",
					Source: "my-gcs",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_bucket_metadata
			type: cloud-storage-get-bucket-metadata
			source: prod-gcs
			description: Get bucket metadata with authentication
			authRequired:
				- google-auth-service
			`,
			want: server.ToolConfigs{
				"secure_bucket_metadata": cloudstoragegetbucketmetadata.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_bucket_metadata",
						Description:  "Get bucket metadata with authentication",
						AuthRequired: []string{"google-auth-service"},
					},
					Type:   "cloud-storage-get-bucket-metadata",
					Source: "prod-gcs",
				},
			},
		},
		{
			desc: "with configurable bucket",
			in: `
			kind: tool
			name: configured_bucket_metadata
			type: cloud-storage-get-bucket-metadata
			source: prod-gcs
			description: Get configured bucket metadata
			bucket: baked-bucket
			`,
			want: server.ToolConfigs{
				"configured_bucket_metadata": cloudstoragegetbucketmetadata.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "configured_bucket_metadata",
						Description:  "Get configured bucket metadata",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-get-bucket-metadata",
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
}

func (m *mockSource) GetBucketMetadata(ctx context.Context, bucket string) (*storage.BucketAttrs, error) {
	m.called = true
	m.gotBucket = bucket
	return &storage.BucketAttrs{Name: bucket, Location: "US"}, nil
}

func TestInvokeValidation(t *testing.T) {
	cfg := cloudstoragegetbucketmetadata.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "bucket_metadata_tool",
			Description: "Get bucket metadata",
		},
		Type:   "cloud-storage-get-bucket-metadata",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	tcs := []struct {
		desc       string
		bucket     any
		wantErr    bool
		wantCalled bool
		wantSubstr string
	}{
		{desc: "missing bucket", bucket: "", wantErr: true, wantSubstr: "bucket"},
		{desc: "happy path", bucket: "b", wantCalled: true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			src := &mockSource{}
			params := parameters.ParamValues{{Name: "bucket", Value: tc.bucket}}
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
			if src.called != tc.wantCalled || src.gotBucket != tc.bucket {
				t.Errorf("called=%v bucket=%q, want called=%v bucket=%q", src.called, src.gotBucket, tc.wantCalled, tc.bucket)
			}
		})
	}
}

func TestConfiguredBucketHiddenAndForwarded(t *testing.T) {
	cfg := cloudstoragegetbucketmetadata.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "bucket_metadata_tool",
			Description: "Get bucket metadata",
		},
		Type:   "cloud-storage-get-bucket-metadata",
		Source: "my-gcs",
		Bucket: strPtr("baked-bucket"),
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	if len(gotNames) != 0 {
		t.Fatalf("manifest parameters = %v, want none", gotNames)
	}

	src := &mockSource{}
	if _, err := tool.Invoke(context.Background(), src, nil, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.gotBucket != "baked-bucket" {
		t.Fatalf("bucket forwarded = %q, want baked-bucket", src.gotBucket)
	}
}

func TestUnsetBucketRemainsVisible(t *testing.T) {
	cfg := cloudstoragegetbucketmetadata.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "bucket_metadata_tool",
			Description: "Get bucket metadata",
		},
		Type:   "cloud-storage-get-bucket-metadata",
		Source: "my-gcs",
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
}

func TestEmptyConfiguredBucketRejected(t *testing.T) {
	cfg := cloudstoragegetbucketmetadata.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "bucket_metadata_tool",
			Description: "Get bucket metadata",
		},
		Type:   "cloud-storage-get-bucket-metadata",
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
