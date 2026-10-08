// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dataformcompilelocal

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// setupDataformMCPServer creates a minimal Dataform project and starts a Toolbox
// server serving the dataform-compile-local tool over the MCP endpoint. Every
// teardown step is registered with t.Cleanup, so nothing leaks if setup fails
// partway through.
func setupDataformMCPServer(t *testing.T, ctx context.Context) string {
	projectDir, cleanupProject := setupTestProject(t)
	t.Cleanup(cleanupProject)

	toolsFile := map[string]any{
		"tools": map[string]any{
			"my-dataform-compiler": map[string]any{
				"type":        "dataform-compile-local",
				"description": "Tool to compile dataform projects",
			},
		},
	}

	cmd, cleanupServer, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	// Registered last so it runs first. StartCmd's cleanup only removes the
	// temporary config file, so Close is what stops the server and closes its
	// pipes.
	t.Cleanup(func() {
		cmd.Close()
		cleanupServer()
	})

	waitCtx, cancelWait := context.WithTimeout(ctx, 30*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	return projectDir
}

func TestDataformMCPListTools(t *testing.T) {
	if _, err := exec.LookPath("dataform"); err != nil {
		t.Skip("dataform CLI not found in $PATH, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	setupDataformMCPServer(t, ctx)

	// This source has no database and no shared fixtures, so the manifest is
	// just the one tool the config registers.
	expectedTools := []tests.MCPToolManifest{
		{
			Name:        "my-dataform-compiler",
			Description: "Tool to compile dataform projects",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project_dir": map[string]any{"type": "string", "description": "The Dataform project directory."},
				},
				"required": []any{"project_dir"},
			},
		},
	}

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, expectedTools)
	})
}

func TestDataformMCPCallTool(t *testing.T) {
	if _, err := exec.LookPath("dataform"); err != nil {
		t.Skip("dataform CLI not found in $PATH, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	projectDir := setupDataformMCPServer(t, ctx)
	nonExistentDir := filepath.Join(os.TempDir(), "non-existent-dir")

	const toolName = "my-dataform-compiler"

	t.Run("success case", func(t *testing.T) {
		tests.RunMCPCustomToolCallMethod(t, toolName, map[string]any{"project_dir": projectDir}, "test_col")
	})

	// Over MCP the failures come back as a JSON-RPC error or an isError result
	// rather than an error string in a 200 body, so they are asserted with
	// AssertMCPError instead of a substring match on the whole response.
	errorTcs := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{
			name:    "missing parameter",
			args:    map[string]any{},
			wantErr: `parameter "project_dir" is required`,
		},
		{
			name:    "non-existent directory",
			args:    map[string]any{"project_dir": nonExistentDir},
			wantErr: "error executing dataform compile",
		},
	}

	for _, tc := range errorTcs {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, toolName, tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing %s: %s", toolName, err)
			}
			if statusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", statusCode)
			}
			tests.AssertMCPError(t, mcpResp, tc.wantErr)
		})
	}
}
