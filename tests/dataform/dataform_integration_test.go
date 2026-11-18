// Copyright 2025 Google LLC
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
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// setupTestProject creates a minimal dataform project using the 'dataform init' CLI.
// It returns the path to the directory.
func setupTestProject(t *testing.T) string {
	tmpDir := t.TempDir()

	cmd := exec.Command("dataform", "init", tmpDir, "test-project-id", "US")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run 'dataform init': %v\nOutput: %s", err, string(output))
	}

	definitionsDir := filepath.Join(tmpDir, "definitions")
	exampleSQLX := `config { type: "table" } SELECT 1 AS test_col`
	err := os.WriteFile(filepath.Join(definitionsDir, "example.sqlx"), []byte(exampleSQLX), 0644)
	if err != nil {
		t.Fatalf("Failed to write example.sqlx: %v", err)
	}

	return tmpDir
}

func TestDataformCompileTool(t *testing.T) {
	if _, err := exec.LookPath("dataform"); err != nil {
		t.Skip("dataform CLI not found in $PATH, skipping integration test")
	}

	projectDir := setupTestProject(t)

	toolsFile := map[string]any{
		"tools": map[string]any{
			"my-dataform-compiler": map[string]any{
				"type":        "dataform-compile-local",
				"description": "Tool to compile dataform projects",
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}
	cmd, cleanupServer, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanupServer()

	waitCtx, cancelWait := context.WithTimeout(ctx, 30*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	nonExistentDir := filepath.Join(os.TempDir(), "non-existent-dir")

	testCases := []struct {
		name       string
		reqBody    string
		wantStatus int
		wantBody   string // Substring to check for in the response
	}{
		{
			name:       "success case",
			reqBody:    fmt.Sprintf(`{"project_dir":"%s"}`, projectDir),
			wantStatus: http.StatusOK,
			wantBody:   "test_col",
		},
		{
			name:       "missing parameter",
			reqBody:    `{}`,
			wantStatus: http.StatusOK,
			wantBody:   `error`,
		},
		{
			name:       "non-existent directory",
			reqBody:    fmt.Sprintf(`{"project_dir":"%s"}`, nonExistentDir),
			wantStatus: http.StatusOK,
			wantBody:   "error executing dataform compile",
		},
	}

	api := "http://127.0.0.1:5000/api/tool/my-dataform-compiler/invoke"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, api, strings.NewReader(tc.reqBody), nil)

			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("unexpected status: got %d, want %d. Body: %s", resp.StatusCode, tc.wantStatus, string(bodyBytes))
			}

			if tc.wantBody != "" && !strings.Contains(string(bodyBytes), tc.wantBody) {
				t.Fatalf("expected body to contain %q, got: %s", tc.wantBody, string(bodyBytes))
			}
		})
	}
}
