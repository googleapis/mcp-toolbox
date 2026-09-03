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

package cloudstoragedeletebucket_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragedeletebucket"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlCloudStorageDeleteBucket(t *testing.T) {
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
			name: delete_bucket_tool
			type: cloud-storage-delete-bucket
			source: my-gcs
			description: Delete a Cloud Storage bucket
			`,
			want: server.ToolConfigs{
				"delete_bucket_tool": cloudstoragedeletebucket.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "delete_bucket_tool",
						Description:  "Delete a Cloud Storage bucket",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-delete-bucket",
					Source: "my-gcs",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_delete_bucket
			type: cloud-storage-delete-bucket
			source: prod-gcs
			description: Delete bucket with authentication
			authRequired:
				- google-auth-service
			`,
			want: server.ToolConfigs{
				"secure_delete_bucket": cloudstoragedeletebucket.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_delete_bucket",
						Description:  "Delete bucket with authentication",
						AuthRequired: []string{"google-auth-service"},
					},
					Type:   "cloud-storage-delete-bucket",
					Source: "prod-gcs",
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

type mockSource struct {
	sources.Source
	called    bool
	gotBucket string
}

func (m *mockSource) DeleteBucket(ctx context.Context, bucket string) (map[string]any, error) {
	m.called = true
	m.gotBucket = bucket
	return map[string]any{"bucket": bucket, "deleted": true}, nil
}

func TestInvokeValidation(t *testing.T) {
	cfg := cloudstoragedeletebucket.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "delete_bucket_tool",
			Description: "Delete bucket",
		},
		Type:   "cloud-storage-delete-bucket",
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
