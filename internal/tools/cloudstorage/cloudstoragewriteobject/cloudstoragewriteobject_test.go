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

package cloudstoragewriteobject_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragewriteobject"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlCloudStorageWriteObject(t *testing.T) {
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
			name: write_tool
			type: cloud-storage-write-object
			source: my-gcs
			description: Write content to Cloud Storage
			`,
			want: server.ToolConfigs{
				"write_tool": cloudstoragewriteobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "write_tool",
						Description:  "Write content to Cloud Storage",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-write-object",
					Source: "my-gcs",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_write
			type: cloud-storage-write-object
			source: prod-gcs
			description: Write with authentication
			authRequired:
				- google-auth-service
			`,
			want: server.ToolConfigs{
				"secure_write": cloudstoragewriteobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_write",
						Description:  "Write with authentication",
						AuthRequired: []string{"google-auth-service"},
					},
					Type:   "cloud-storage-write-object",
					Source: "prod-gcs",
				},
			},
		},
		{
			desc: "with configurable bucket",
			in: `
			kind: tool
			name: configured_write
			type: cloud-storage-write-object
			source: prod-gcs
			description: Write configured object
			bucket: baked-bucket
			`,
			want: server.ToolConfigs{
				"configured_write": cloudstoragewriteobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "configured_write",
						Description:  "Write configured object",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-write-object",
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
	called         bool
	gotBucket      string
	gotObject      string
	gotContent     string
	gotContentType string
}

func TestConfiguredBucketHiddenAndForwarded(t *testing.T) {
	cfg := cloudstoragewriteobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "write_tool",
			Description: "Write",
		},
		Type:   "cloud-storage-write-object",
		Source: "my-gcs",
		Bucket: strPtr("baked-bucket"),
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"object", "content", "content_type"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}

	src := &mockSource{}
	params := parameters.ParamValues{
		{Name: "object", Value: "o"},
		{Name: "content", Value: "body"},
		{Name: "content_type", Value: "text/plain"},
	}
	if _, err := tool.Invoke(context.Background(), src, params, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.gotBucket != "baked-bucket" || src.gotObject != "o" || src.gotContent != "body" || src.gotContentType != "text/plain" {
		t.Fatalf("forwarded params = %q/%q/%q/%q, want baked-bucket/o/body/text/plain", src.gotBucket, src.gotObject, src.gotContent, src.gotContentType)
	}
}

func TestUnsetBucketRemainsVisible(t *testing.T) {
	cfg := cloudstoragewriteobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "write_tool",
			Description: "Write",
		},
		Type:   "cloud-storage-write-object",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"bucket", "object", "content", "content_type"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}
}

func TestEmptyConfiguredBucketRejected(t *testing.T) {
	cfg := cloudstoragewriteobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "write_tool",
			Description: "Write",
		},
		Type:   "cloud-storage-write-object",
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

func (m *mockSource) WriteObject(ctx context.Context, bucket, object, content, contentType string) (map[string]any, error) {
	m.called = true
	m.gotBucket = bucket
	m.gotObject = object
	m.gotContent = content
	m.gotContentType = contentType
	return map[string]any{"bucket": bucket, "object": object, "bytes": len(content), "contentType": contentType}, nil
}

func TestInvokeValidation(t *testing.T) {
	cfg := cloudstoragewriteobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "write_tool",
			Description: "Write",
		},
		Type:   "cloud-storage-write-object",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	tcs := []struct {
		desc        string
		bucket      any
		object      any
		content     any
		contentType any
		wantErr     bool
		wantSubstr  string
		wantCalled  bool
	}{
		{desc: "missing bucket", bucket: "", object: "o", content: "body", wantErr: true, wantSubstr: "bucket"},
		{desc: "missing object", bucket: "b", object: "", content: "body", wantErr: true, wantSubstr: "object"},
		{desc: "missing content", bucket: "b", object: "o", content: nil, wantErr: true, wantSubstr: "content"},
		{desc: "empty content is valid", bucket: "b", object: "o", content: "", contentType: "text/plain", wantCalled: true},
		{desc: "empty content_type passes through", bucket: "b", object: "o", content: "body", contentType: "", wantCalled: true},
		{desc: "happy path", bucket: "b", object: "o", content: "body", contentType: "text/plain", wantCalled: true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			src := &mockSource{}
			ct, _ := tc.contentType.(string)
			params := parameters.ParamValues{
				{Name: "bucket", Value: tc.bucket},
				{Name: "object", Value: tc.object},
				{Name: "content", Value: tc.content},
				{Name: "content_type", Value: ct},
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
			if src.gotBucket != tc.bucket || src.gotObject != tc.object || src.gotContent != tc.content || src.gotContentType != ct {
				t.Errorf("forwarded params = (%q, %q, %q, %q)", src.gotBucket, src.gotObject, src.gotContent, src.gotContentType)
			}
		})
	}
}
