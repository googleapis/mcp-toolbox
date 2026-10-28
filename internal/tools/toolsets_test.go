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

package tools_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestToolsetConfig_Initialize(t *testing.T) {
	t.Parallel()

	var tool1 = testutils.NewMockTool("tool1", "some description", "", []parameters.Parameter{}, false, false)
	var tool2 = testutils.NewMockTool(
		"tool2",
		"some description", "",
		parameters.Parameters{
			parameters.NewIntParameter("param1", "This is the first parameter."),
			parameters.NewIntParameter("param2", "This is the second parameter."),
		}, false, false)

	toolsMap := map[string]tools.Tool{
		"tool1": tool1,
		"tool2": tool2,
	}
	serverVersion := "test-version"

	t1 := toolsMap["tool1"]
	t2 := toolsMap["tool2"]
	tool1Ptr := &t1
	tool2Ptr := &t2

	testCases := []struct {
		name    string
		config  tools.ToolsetConfig
		want    tools.Toolset
		wantErr string
	}{
		{
			name: "Success case",
			config: tools.ToolsetConfig{
				Name:      "default",
				ToolNames: []string{"tool1", "tool2"},
			},
			want: tools.Toolset{
				ToolsetConfig: tools.ToolsetConfig{
					Name:      "default",
					ToolNames: []string{"tool1", "tool2"},
				},
				Tools: []*tools.Tool{
					tool1Ptr,
					tool2Ptr,
				},
				Manifest: tools.ToolsetManifest{
					ServerVersion: serverVersion,
				},
			},
			wantErr: "",
		},
		{
			name: "Success case with one tool",
			config: tools.ToolsetConfig{
				Name:      "single",
				ToolNames: []string{"tool1"},
			},
			want: tools.Toolset{
				ToolsetConfig: tools.ToolsetConfig{
					Name:      "single",
					ToolNames: []string{"tool1"},
				},
				Tools: []*tools.Tool{
					tool1Ptr,
				},
				Manifest: tools.ToolsetManifest{
					ServerVersion: serverVersion,
				},
			},
			wantErr: "",
		},
		{
			name: "Failure case - invalid toolset name",
			config: tools.ToolsetConfig{
				Name:      "invalid name", // Contains a space
				ToolNames: []string{"tool1"},
			},
			want: tools.Toolset{
				ToolsetConfig: tools.ToolsetConfig{
					Name:      "invalid name",
					ToolNames: []string{"tool1"},
				},
				Tools: []*tools.Tool{},
				Manifest: tools.ToolsetManifest{
					ServerVersion: serverVersion,
				},
			},
			wantErr: "invalid toolset name",
		},
		{
			name: "Failure case - tool not found",
			config: tools.ToolsetConfig{
				Name:      "missing_tool",
				ToolNames: []string{"tool1", "tool_does_not_exist"},
			},
			// Expect partial struct with fields populated up to the error
			want: tools.Toolset{
				ToolsetConfig: tools.ToolsetConfig{
					Name:      "missing_tool",
					ToolNames: []string{"tool1", "tool_does_not_exist"},
				},
				Tools: []*tools.Tool{
					tool1Ptr,
				},
				Manifest: tools.ToolsetManifest{
					ServerVersion: serverVersion,
				},
			},
			wantErr: "tool does not exist",
		},
		{
			name: "Success case - empty tools list",
			config: tools.ToolsetConfig{
				Name:      "empty",
				ToolNames: []string{},
			},
			want: tools.Toolset{
				ToolsetConfig: tools.ToolsetConfig{
					Name:      "empty",
					ToolNames: []string{},
				},
				Tools: []*tools.Tool{},
				Manifest: tools.ToolsetManifest{
					ServerVersion: serverVersion,
				},
			},
			wantErr: "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.config.Initialize(serverVersion, toolsMap)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("Initialize() expected error but got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Initialize() error mismatch:\n  want to contain: %q\n  got: %q", tc.wantErr, err.Error())
				}
				// Also check that the partially populated struct matches
				if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(testutils.MockTool{}), cmp.AllowUnexported(tools.BaseTool[testutils.MockToolConfig]{}), cmpopts.IgnoreUnexported(tools.Toolset{})); diff != "" {
					t.Errorf("Initialize() partial result on error mismatch (-want +got):\n%s", diff)
				}
			} else {
				if err != nil {
					t.Fatalf("Initialize() returned unexpected error: %v", err)
				}
				// Using cmp.AllowUnexported because MockTool is unexported
				if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(testutils.MockTool{}), cmp.AllowUnexported(tools.BaseTool[testutils.MockToolConfig]{}), cmpopts.IgnoreUnexported(tools.Toolset{})); diff != "" {
					t.Errorf("Initialize() result mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestToolset_ContainsTool(t *testing.T) {
	t.Parallel()

	toolset := tools.Toolset{
		ToolsetConfig: tools.ToolsetConfig{
			Name:      "test-toolset",
			ToolNames: []string{"echo", "list_tables"},
		},
	}

	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		{
			name:     "tool exists in toolset",
			toolName: "echo",
			want:     true,
		},
		{
			name:     "another tool exists in toolset",
			toolName: "list_tables",
			want:     true,
		},
		{
			name:     "tool not in toolset",
			toolName: "admin_delete",
			want:     false,
		},
		{
			name:     "empty tool name",
			toolName: "",
			want:     false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := toolset.ContainsTool(tc.toolName)
			if got != tc.want {
				t.Errorf("ContainsTool(%q) = %v, want %v", tc.toolName, got, tc.want)
			}
		})
	}
}

func TestToolset_ContainsTool_EmptyToolset(t *testing.T) {
	t.Parallel()

	toolset := tools.Toolset{
		ToolsetConfig: tools.ToolsetConfig{
			Name:      "empty-toolset",
			ToolNames: []string{},
		},
	}

	if toolset.ContainsTool("anything") {
		t.Error("ContainsTool should return false for empty toolset")
	}
}
