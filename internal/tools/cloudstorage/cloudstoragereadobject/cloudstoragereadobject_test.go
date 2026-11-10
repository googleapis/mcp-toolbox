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

package cloudstoragereadobject

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlCloudStorageReadObject(t *testing.T) {
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
			name: read_object_tool
			type: cloud-storage-read-object
			source: my-gcs
			description: Read a Cloud Storage object
			`,
			want: server.ToolConfigs{
				"read_object_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "read_object_tool",
						Description:  "Read a Cloud Storage object",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-read-object",
					Source: "my-gcs",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_read_object
			type: cloud-storage-read-object
			source: prod-gcs
			description: Read object with authentication
			authRequired:
				- google-auth-service
			`,
			want: server.ToolConfigs{
				"secure_read_object": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_read_object",
						Description:  "Read object with authentication",
						AuthRequired: []string{"google-auth-service"},
					},
					Type:   "cloud-storage-read-object",
					Source: "prod-gcs",
				},
			},
		},
		{
			desc: "with configurable bucket",
			in: `
			kind: tool
			name: configured_read_object
			type: cloud-storage-read-object
			source: prod-gcs
			description: Read configured object
			bucket: baked-bucket
			`,
			want: server.ToolConfigs{
				"configured_read_object": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "configured_read_object",
						Description:  "Read configured object",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-read-object",
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
	gotOffset int64
	gotLength int64
}

func (m *mockSource) ReadObject(ctx context.Context, bucket, object string, offset, length int64) (map[string]any, error) {
	m.called = true
	m.gotBucket = bucket
	m.gotObject = object
	m.gotOffset = offset
	m.gotLength = length
	return map[string]any{"content": "hello", "contentType": "text/plain", "size": 5}, nil
}

func TestConfiguredBucketHiddenAndForwarded(t *testing.T) {
	cfg := Config{
		ConfigBase: tools.ConfigBase{
			Name:        "read_object_tool",
			Description: "Read object",
		},
		Type:   "cloud-storage-read-object",
		Source: "my-gcs",
		Bucket: strPtr("baked-bucket"),
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"object", "range"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}

	src := &mockSource{}
	params := parameters.ParamValues{
		{Name: "object", Value: "o"},
		{Name: "range", Value: "bytes=5-9"},
	}
	if _, err := tool.Invoke(context.Background(), src, params, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.gotBucket != "baked-bucket" || src.gotObject != "o" || src.gotOffset != 5 || src.gotLength != 5 {
		t.Fatalf("forwarded bucket/object/range = %q/%q/%d/%d, want baked-bucket/o/5/5", src.gotBucket, src.gotObject, src.gotOffset, src.gotLength)
	}
}

func TestUnsetBucketRemainsVisible(t *testing.T) {
	cfg := Config{
		ConfigBase: tools.ConfigBase{
			Name:        "read_object_tool",
			Description: "Read object",
		},
		Type:   "cloud-storage-read-object",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"bucket", "object", "range"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}
}

func TestEmptyConfiguredBucketRejected(t *testing.T) {
	cfg := Config{
		ConfigBase: tools.ConfigBase{
			Name:        "read_object_tool",
			Description: "Read object",
		},
		Type:   "cloud-storage-read-object",
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

func TestParseRange(t *testing.T) {
	tcs := []struct {
		in         string
		wantOffset int64
		wantLength int64
		wantErr    bool
	}{
		{in: "", wantOffset: 0, wantLength: -1},
		{in: "bytes=0-9", wantOffset: 0, wantLength: 10},
		{in: "bytes=10-19", wantOffset: 10, wantLength: 10},
		{in: "bytes=10-", wantOffset: 10, wantLength: -1},
		{in: "bytes=-5", wantOffset: -5, wantLength: -1},
		{in: "bytes=0-0", wantOffset: 0, wantLength: 1},

		{in: "garbage", wantErr: true},
		{in: "bytes=", wantErr: true},
		{in: "bytes=a-b", wantErr: true},
		{in: "bytes=-", wantErr: true},
		{in: "bytes=-0", wantErr: true},
		{in: "bytes=5-2", wantErr: true},
		{in: "bytes=-1-2", wantErr: true},
	}
	for _, tc := range tcs {
		t.Run(tc.in, func(t *testing.T) {
			offset, length, err := parseRange(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got offset=%d length=%d", offset, length)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if offset != tc.wantOffset || length != tc.wantLength {
				t.Fatalf("got (%d, %d), want (%d, %d)", offset, length, tc.wantOffset, tc.wantLength)
			}
		})
	}
}
