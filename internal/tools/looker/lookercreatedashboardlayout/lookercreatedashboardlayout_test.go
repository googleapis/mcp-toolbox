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

package lookercreatedashboardlayout_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	lkr "github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercreatedashboardlayout"
)

func TestParseFromYaml(t *testing.T) {
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
            name: test_tool
            type: looker-create-dashboard-layout
            source: my-instance
            description: some description
                                `,
			want: server.ToolConfigs{
				"test_tool": lkr.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "test_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "looker-create-dashboard-layout",
					Source: "my-instance",
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

func TestFailParseFromYaml(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "Invalid method",
			in: `
            kind: tool
            name: test_tool
            type: looker-create-dashboard-layout
            source: my-instance
            method: GOT
            description: some description
                        `,
			err: "unknown field \"method\"",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.err) {
				t.Fatalf("unexpected error string: got %q, want substring %q", errStr, tc.err)
			}
		})
	}
}

func TestManifest(t *testing.T) {
	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test_tool",
			Description: "test description",
		},
		Type:   "looker-create-dashboard-layout",
		Source: "my-instance",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	manifest, err := tool.Manifest(nil)
	if err != nil {
		t.Fatalf("Manifest() returned unexpected error: %v", err)
	}
	if manifest.Description != cfg.Description {
		t.Errorf("manifest description mismatch: got %q, want %q", manifest.Description, cfg.Description)
	}

	expectedParams := []string{
		"dashboard_id",
		"label",
		"type",
		"active",
	}
	for _, p := range expectedParams {
		found := false
		for _, mp := range manifest.Parameters {
			if mp.Name == p {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected parameter %q not found in manifest", p)
		}
	}
}

func TestAnnotations(t *testing.T) {
	readOnlyFalse := false
	cfg := lkr.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test_tool",
			Description: "test description",
		},
		Type:   "looker-create-dashboard-layout",
		Source: "my-instance",
		Annotations: &tools.ToolAnnotations{
			ReadOnlyHint: &readOnlyFalse,
		},
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	annotations := tool.GetAnnotations(nil)
	if annotations == nil {
		t.Fatal("mcp manifest annotations is nil")
	}
	if annotations.ReadOnlyHint == nil {
		t.Fatal("mcp manifest ReadOnlyHint is nil")
	}
	if *annotations.ReadOnlyHint != false {
		t.Errorf("ReadOnlyHint should be false, got %v", *annotations.ReadOnlyHint)
	}
}
