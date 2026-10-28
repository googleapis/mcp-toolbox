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

package cloudstorageuploadobject_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstorageuploadobject"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlCloudStorageUploadObject(t *testing.T) {
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
			name: upload_tool
			type: cloud-storage-upload-object
			source: my-gcs
			description: Upload a local file to Cloud Storage
			`,
			want: server.ToolConfigs{
				"upload_tool": cloudstorageuploadobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "upload_tool",
						Description:  "Upload a local file to Cloud Storage",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-upload-object",
					Source: "my-gcs",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_upload
			type: cloud-storage-upload-object
			source: prod-gcs
			description: Upload with authentication
			authRequired:
				- google-auth-service
			`,
			want: server.ToolConfigs{
				"secure_upload": cloudstorageuploadobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_upload",
						Description:  "Upload with authentication",
						AuthRequired: []string{"google-auth-service"},
					},
					Type:   "cloud-storage-upload-object",
					Source: "prod-gcs",
				},
			},
		},
		{
			desc: "with configurable bucket",
			in: `
			kind: tool
			name: configured_upload
			type: cloud-storage-upload-object
			source: prod-gcs
			description: Upload configured object
			bucket: baked-bucket
			`,
			want: server.ToolConfigs{
				"configured_upload": cloudstorageuploadobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "configured_upload",
						Description:  "Upload configured object",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-upload-object",
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
	gotSource      string
	gotContentType string
}

func (m *mockSource) UploadObject(ctx context.Context, bucket, object, source, contentType string) (map[string]any, error) {
	m.called = true
	m.gotBucket = bucket
	m.gotObject = object
	m.gotSource = source
	m.gotContentType = contentType
	return map[string]any{"bucket": bucket, "object": object, "bytes": int64(0), "contentType": contentType}, nil
}

func TestInvokeValidation(t *testing.T) {
	cfg := cloudstorageuploadobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "upload_tool",
			Description: "Upload",
		},
		Type:   "cloud-storage-upload-object",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	validSource := filepath.Join(t.TempDir(), "in.csv")

	tcs := []struct {
		desc           string
		bucket         any
		object         any
		src            any
		contentType    any
		wantErr        bool
		wantSubstr     string
		wantCalled     bool
		wantContentTyp string
	}{
		{desc: "missing bucket", bucket: "", object: "o", src: "/tmp/in", wantErr: true, wantSubstr: "bucket"},
		{desc: "missing object", bucket: "b", object: "", src: "/tmp/in", wantErr: true, wantSubstr: "object"},
		{desc: "missing source", bucket: "b", object: "o", src: "", wantErr: true, wantSubstr: "source"},
		{desc: "relative source", bucket: "b", object: "o", src: "relative/path", wantErr: true, wantSubstr: "source"},
		{desc: "source with traversal", bucket: "b", object: "o", src: "/tmp/../etc/passwd", wantErr: true, wantSubstr: "source"},
		{desc: "happy path, explicit content_type", bucket: "b", object: "o", src: validSource, contentType: "text/csv", wantCalled: true, wantContentTyp: "text/csv"},
		{desc: "happy path, empty content_type forwarded", bucket: "b", object: "o", src: validSource, contentType: "", wantCalled: true, wantContentTyp: ""},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			src := &mockSource{}
			ct := ""
			if s, ok := tc.contentType.(string); ok {
				ct = s
			}
			params := parameters.ParamValues{
				{Name: "bucket", Value: tc.bucket},
				{Name: "object", Value: tc.object},
				{Name: "source", Value: tc.src},
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
			if src.gotContentType != tc.wantContentTyp {
				t.Errorf("content_type forwarded = %q, want %q", src.gotContentType, tc.wantContentTyp)
			}
		})
	}
}

func TestConfiguredBucketHiddenAndForwarded(t *testing.T) {
	cfg := cloudstorageuploadobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "upload_tool",
			Description: "Upload",
		},
		Type:   "cloud-storage-upload-object",
		Source: "my-gcs",
		Bucket: strPtr("baked-bucket"),
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"object", "source", "content_type"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}

	localSource := filepath.Join(t.TempDir(), "in.csv")
	src := &mockSource{}
	params := parameters.ParamValues{
		{Name: "object", Value: "o"},
		{Name: "source", Value: localSource},
		{Name: "content_type", Value: "text/csv"},
	}
	if _, err := tool.Invoke(context.Background(), src, params, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.gotBucket != "baked-bucket" || src.gotObject != "o" || src.gotSource != localSource || src.gotContentType != "text/csv" {
		t.Fatalf("forwarded params = %q/%q/%q/%q, want baked-bucket/o/%s/text/csv", src.gotBucket, src.gotObject, src.gotSource, src.gotContentType, localSource)
	}
}

func TestUnsetBucketRemainsVisible(t *testing.T) {
	cfg := cloudstorageuploadobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "upload_tool",
			Description: "Upload",
		},
		Type:   "cloud-storage-upload-object",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"bucket", "object", "source", "content_type"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}
}

func TestEmptyConfiguredBucketRejected(t *testing.T) {
	cfg := cloudstorageuploadobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "upload_tool",
			Description: "Upload",
		},
		Type:   "cloud-storage-upload-object",
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
