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

package cloudstoragecopyobject_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragecopyobject"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlCloudStorageCopyObject(t *testing.T) {
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
			name: copy_tool
			type: cloud-storage-copy-object
			source: my-gcs
			description: Copy a Cloud Storage object
			`,
			want: server.ToolConfigs{
				"copy_tool": cloudstoragecopyobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "copy_tool",
						Description:  "Copy a Cloud Storage object",
						AuthRequired: []string{},
					},
					Type:   "cloud-storage-copy-object",
					Source: "my-gcs",
				},
			},
		},
		{
			desc: "with auth requirements",
			in: `
			kind: tool
			name: secure_copy
			type: cloud-storage-copy-object
			source: prod-gcs
			description: Copy with authentication
			authRequired:
				- google-auth-service
			`,
			want: server.ToolConfigs{
				"secure_copy": cloudstoragecopyobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "secure_copy",
						Description:  "Copy with authentication",
						AuthRequired: []string{"google-auth-service"},
					},
					Type:   "cloud-storage-copy-object",
					Source: "prod-gcs",
				},
			},
		},
		{
			desc: "with configurable buckets",
			in: `
			kind: tool
			name: configured_copy
			type: cloud-storage-copy-object
			source: prod-gcs
			description: Copy configured object
			source_bucket: source-bucket
			destination_bucket: destination-bucket
			`,
			want: server.ToolConfigs{
				"configured_copy": cloudstoragecopyobject.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "configured_copy",
						Description:  "Copy configured object",
						AuthRequired: []string{},
					},
					Type:              "cloud-storage-copy-object",
					Source:            "prod-gcs",
					SourceBucket:      strPtr("source-bucket"),
					DestinationBucket: strPtr("destination-bucket"),
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
	called               bool
	gotSourceBucket      string
	gotSourceObject      string
	gotDestinationBucket string
	gotDestinationObject string
}

func TestConfiguredBucketsHiddenAndForwarded(t *testing.T) {
	cfg := cloudstoragecopyobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "copy_tool",
			Description: "Copy",
		},
		Type:              "cloud-storage-copy-object",
		Source:            "my-gcs",
		SourceBucket:      strPtr("source-bucket"),
		DestinationBucket: strPtr("destination-bucket"),
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"source_object", "destination_object"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}

	src := &mockSource{}
	params := parameters.ParamValues{
		{Name: "source_object", Value: "src"},
		{Name: "destination_object", Value: "dst"},
	}
	if _, err := tool.Invoke(context.Background(), src, params, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.gotSourceBucket != "source-bucket" || src.gotSourceObject != "src" || src.gotDestinationBucket != "destination-bucket" || src.gotDestinationObject != "dst" {
		t.Fatalf("forwarded params = %q/%q/%q/%q, want source-bucket/src/destination-bucket/dst", src.gotSourceBucket, src.gotSourceObject, src.gotDestinationBucket, src.gotDestinationObject)
	}
}

func TestUnsetBucketsRemainVisible(t *testing.T) {
	cfg := cloudstoragecopyobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "copy_tool",
			Description: "Copy",
		},
		Type:   "cloud-storage-copy-object",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	gotNames := manifestParamNames(tool.StaticManifest().Parameters)
	wantNames := []string{"source_bucket", "source_object", "destination_bucket", "destination_object"}
	if diff := cmp.Diff(wantNames, gotNames); diff != "" {
		t.Fatalf("manifest parameters mismatch (-want +got):\n%s", diff)
	}
}

func TestEmptyConfiguredBucketsRejected(t *testing.T) {
	tcs := []struct {
		desc              string
		sourceBucket      *string
		destinationBucket *string
		wantSubstr        string
	}{
		{desc: "empty source bucket", sourceBucket: strPtr(""), wantSubstr: "source_bucket"},
		{desc: "empty destination bucket", destinationBucket: strPtr(""), wantSubstr: "destination_bucket"},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := cloudstoragecopyobject.Config{
				ConfigBase: tools.ConfigBase{
					Name:        "copy_tool",
					Description: "Copy",
				},
				Type:              "cloud-storage-copy-object",
				Source:            "my-gcs",
				SourceBucket:      tc.sourceBucket,
				DestinationBucket: tc.destinationBucket,
			}
			if _, err := cfg.Initialize(context.Background()); err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("Initialize() error = %v, want %q", err, tc.wantSubstr)
			}
		})
	}
}

func manifestParamNames(params []parameters.ParameterManifest) []string {
	names := make([]string, 0, len(params))
	for _, p := range params {
		names = append(names, p.Name)
	}
	return names
}

func (m *mockSource) CopyObject(ctx context.Context, sourceBucket, sourceObject, destinationBucket, destinationObject string) (map[string]any, error) {
	m.called = true
	m.gotSourceBucket = sourceBucket
	m.gotSourceObject = sourceObject
	m.gotDestinationBucket = destinationBucket
	m.gotDestinationObject = destinationObject
	return map[string]any{"sourceBucket": sourceBucket, "sourceObject": sourceObject, "destinationBucket": destinationBucket, "destinationObject": destinationObject}, nil
}

func TestInvokeValidation(t *testing.T) {
	cfg := cloudstoragecopyobject.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "copy_tool",
			Description: "Copy",
		},
		Type:   "cloud-storage-copy-object",
		Source: "my-gcs",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	tcs := []struct {
		desc              string
		sourceBucket      any
		sourceObject      any
		destinationBucket any
		destinationObject any
		wantErr           bool
		wantSubstr        string
		wantCalled        bool
	}{
		{desc: "missing source bucket", sourceBucket: "", sourceObject: "src", destinationBucket: "db", destinationObject: "dst", wantErr: true, wantSubstr: "source_bucket"},
		{desc: "missing source object", sourceBucket: "sb", sourceObject: "", destinationBucket: "db", destinationObject: "dst", wantErr: true, wantSubstr: "source_object"},
		{desc: "missing destination bucket", sourceBucket: "sb", sourceObject: "src", destinationBucket: "", destinationObject: "dst", wantErr: true, wantSubstr: "destination_bucket"},
		{desc: "missing destination object", sourceBucket: "sb", sourceObject: "src", destinationBucket: "db", destinationObject: "", wantErr: true, wantSubstr: "destination_object"},
		{desc: "happy path", sourceBucket: "sb", sourceObject: "src", destinationBucket: "db", destinationObject: "dst", wantCalled: true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			src := &mockSource{}
			params := parameters.ParamValues{
				{Name: "source_bucket", Value: tc.sourceBucket},
				{Name: "source_object", Value: tc.sourceObject},
				{Name: "destination_bucket", Value: tc.destinationBucket},
				{Name: "destination_object", Value: tc.destinationObject},
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
			if src.gotSourceBucket != tc.sourceBucket || src.gotSourceObject != tc.sourceObject || src.gotDestinationBucket != tc.destinationBucket || src.gotDestinationObject != tc.destinationObject {
				t.Errorf("forwarded params = (%q, %q, %q, %q)", src.gotSourceBucket, src.gotSourceObject, src.gotDestinationBucket, src.gotDestinationObject)
			}
		})
	}
}
