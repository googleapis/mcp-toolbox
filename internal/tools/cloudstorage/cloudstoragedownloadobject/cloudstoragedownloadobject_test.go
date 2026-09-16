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

package cloudstoragedownloadobject_test

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
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragedownloadobject"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlCloudStorageDownloadObject(t *testing.T) {
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
			name: download_tool
			type: cloud-storage-download-object
			source: my-gcs
			description: Download a Cloud Storage object
			`,
			want: server.ToolConfigs{
				"download_tool": cloudstoragedownloadobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "download_tool",
						Description:  "Download a Cloud Storage object",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-download-object",
					Source: "my-gcs",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_download
			type: cloud-storage-download-object
			source: prod-gcs
			description: Download with authentication
			authRequired:
				- google-auth-service
			`,
			want: server.ToolConfigs{
				"secure_download": cloudstoragedownloadobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_download",
						Description:  "Download with authentication",
						AuthRequired: []string{"google-auth-service"},
					},
					Type:   "cloud-storage-download-object",
					Source: "prod-gcs",
				},
			},
		},
		{
			desc: "with configurable parameters",
			in: `
			kind: tool
			name: configured_download
			type: cloud-storage-download-object
			source: prod-gcs
			description: Download configured object
			bucket: baked-bucket
			destination_dir: /tmp/downloads
			overwrite: false
			`,
			want: server.ToolConfigs{
				"configured_download": cloudstoragedownloadobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "configured_download",
						Description:  "Download configured object",
						AuthRequired: []string{},
					},
					Type:           "cloud-storage-download-object",
					Source:         "prod-gcs",
					Bucket:         strPtr("baked-bucket"),
					DestinationDir: strPtr("/tmp/downloads"),
					Overwrite:      boolPtr(false),
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
	called       bool
	gotBucket    string
	gotObject    string
	gotDest      string
	gotOverwrite bool
}

func (m *mockSource) DownloadObject(ctx context.Context, bucket, object, destination string, overwrite bool) (map[string]any, error) {
	m.called = true
	m.gotBucket = bucket
	m.gotObject = object
	m.gotDest = destination
	m.gotOverwrite = overwrite
	return map[string]any{"destination": destination, "bytes": int64(11), "contentType": "text/plain"}, nil
}

func TestInvokeValidation(t *testing.T) {
	cfg := cloudstoragedownloadobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "download_tool",
			Description: "Download",
		},
		Type:   "cloud-storage-download-object",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	validDest := filepath.Join(t.TempDir(), "out.bin")

	tcs := []struct {
		desc       string
		bucket     any
		object     any
		dest       any
		overwrite  any
		wantErr    bool
		wantSubstr string
		wantCalled bool
	}{
		{desc: "missing bucket", bucket: "", object: "o", dest: "/tmp/x", wantErr: true, wantSubstr: "bucket"},
		{desc: "missing object", bucket: "b", object: "", dest: "/tmp/x", wantErr: true, wantSubstr: "object"},
		{desc: "missing destination", bucket: "b", object: "o", dest: "", wantErr: true, wantSubstr: "destination"},
		{desc: "relative destination", bucket: "b", object: "o", dest: "relative/path", wantErr: true, wantSubstr: "destination"},
		{desc: "destination with traversal", bucket: "b", object: "o", dest: "/tmp/../etc/passwd", wantErr: true, wantSubstr: "destination"},
		{desc: "happy path", bucket: "b", object: "o", dest: validDest, overwrite: true, wantCalled: true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			src := &mockSource{}
			ov := false
			if b, ok := tc.overwrite.(bool); ok {
				ov = b
			}
			params := parameters.ParamValues{
				{Name: "bucket", Value: tc.bucket},
				{Name: "object", Value: tc.object},
				{Name: "destination", Value: tc.dest},
				{Name: "overwrite", Value: ov},
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
			if tc.wantCalled && !src.gotOverwrite {
				t.Errorf("overwrite flag not forwarded: got %v", src.gotOverwrite)
			}
		})
	}
}

func TestConfiguredParametersHiddenAndForwarded(t *testing.T) {
	destDir := t.TempDir()
	cfg := cloudstoragedownloadobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "download_tool",
			Description: "Download",
		},
		Type:           "cloud-storage-download-object",
		Source:         "my-gcs",
		Bucket:         strPtr("baked-bucket"),
		DestinationDir: strPtr(destDir),
		Overwrite:      boolPtr(false),
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"object", "destination"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}

	src := &mockSource{}
	params := parameters.ParamValues{
		{Name: "object", Value: "path/to/object.txt"},
		{Name: "destination", Value: filepath.Join("nested", "out.txt")},
	}
	if _, err := tool.Invoke(context.Background(), src, params, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantDest := filepath.Join(destDir, "nested", "out.txt")
	if src.gotBucket != "baked-bucket" || src.gotObject != "path/to/object.txt" || src.gotDest != wantDest || src.gotOverwrite {
		t.Fatalf("forwarded params = bucket %q object %q dest %q overwrite %v, want baked-bucket/path/to/object.txt/%s/false", src.gotBucket, src.gotObject, src.gotDest, src.gotOverwrite, wantDest)
	}
}

func TestUnsetParametersRemainVisible(t *testing.T) {
	cfg := cloudstoragedownloadobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "download_tool",
			Description: "Download",
		},
		Type:   "cloud-storage-download-object",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"bucket", "object", "destination", "overwrite"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}
}

func TestConfiguredParameterValidation(t *testing.T) {
	tcs := []struct {
		desc       string
		bucket     *string
		destDir    *string
		wantSubstr string
	}{
		{desc: "empty bucket", bucket: strPtr(""), wantSubstr: "bucket"},
		{desc: "empty destination dir", destDir: strPtr(""), wantSubstr: "destination_dir"},
		{desc: "relative destination dir", destDir: strPtr("relative/dir"), wantSubstr: "destination_dir"},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := cloudstoragedownloadobject.Config{
				ConfigBase: tools.ConfigBase{
					Name:        "download_tool",
					Description: "Download",
				},
				Type:           "cloud-storage-download-object",
				Source:         "my-gcs",
				Bucket:         tc.bucket,
				DestinationDir: tc.destDir,
			}
			if _, err := cfg.Initialize(context.Background()); err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("Initialize() error = %v, want %q", err, tc.wantSubstr)
			}
		})
	}
}

func TestConfiguredDestinationDirRejectsEscapingDestination(t *testing.T) {
	destDir := t.TempDir()
	cfg := cloudstoragedownloadobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "download_tool",
			Description: "Download",
		},
		Type:           "cloud-storage-download-object",
		Source:         "my-gcs",
		Bucket:         strPtr("baked-bucket"),
		DestinationDir: strPtr(destDir),
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	src := &mockSource{}
	params := parameters.ParamValues{
		{Name: "object", Value: "o"},
		{Name: "destination", Value: filepath.Join("..", "escape.txt")},
		{Name: "overwrite", Value: false},
	}
	_, toolErr := tool.Invoke(context.Background(), src, params, "")
	if toolErr == nil || !strings.Contains(toolErr.Error(), "destination") {
		t.Fatalf("Invoke() error = %v, want destination error", toolErr)
	}
	if src.called {
		t.Fatalf("source called despite invalid destination")
	}
}

func manifestParamNames(params []parameters.ParameterManifest) []string {
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Name)
	}
	return names
}
