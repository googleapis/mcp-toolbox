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

package file_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/resources/file"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestFileTemplate(t *testing.T) {
	// Create a temporary directory structure for testing
	tempDir := t.TempDir()
	var err error
	tempDir, err = filepath.EvalSymlinks(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Allowed area
	sandboxDir := filepath.Join(tempDir, "sandbox")
	if err := os.Mkdir(sandboxDir, 0755); err != nil {
		t.Fatal(err)
	}

	// External area
	externalDir := filepath.Join(tempDir, "external")
	if err := os.Mkdir(externalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test files
	fileEnc := filepath.Join(sandboxDir, "file..txt")
	if err := os.WriteFile(fileEnc, []byte("safe"), 0644); err != nil {
		t.Fatal(err)
	}
	validFile := filepath.Join(sandboxDir, "valid.txt")
	if err := os.WriteFile(validFile, []byte("valid content"), 0644); err != nil {
		t.Fatal(err)
	}

	secretFile := filepath.Join(externalDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("super secret"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlinks
	symlinkToSecret := filepath.Join(sandboxDir, "symlink_to_secret.txt")
	if err := os.Symlink(secretFile, symlinkToSecret); err != nil {
		t.Fatal(err)
	}

	largeFile := filepath.Join(sandboxDir, "large.txt")
	if err := os.WriteFile(largeFile, make([]byte, 1024*1024*5+100), 0644); err != nil {
		t.Fatal(err)
	}

	// Binary file for extension validation
	binFile := filepath.Join(sandboxDir, "image.png")
	if err := os.WriteFile(binFile, []byte("fake png content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Hidden directory for hidden files test
	hiddenDir := filepath.Join(sandboxDir, ".secrets")
	if err := os.Mkdir(hiddenDir, 0755); err != nil {
		t.Fatal(err)
	}
	hiddenDirFile := filepath.Join(hiddenDir, "data.txt")
	if err := os.WriteFile(hiddenDirFile, []byte("hidden dir"), 0644); err != nil {
		t.Fatal(err)
	}

	hiddenFile := filepath.Join(sandboxDir, ".hidden.txt")
	if err := os.WriteFile(hiddenFile, []byte("hidden"), 0644); err != nil {
		t.Fatal(err)
	}

	nonExistentFile := filepath.Join(sandboxDir, "nonexistent.txt")

	noPermFile := filepath.Join(sandboxDir, "noperm.txt")
	if err := os.WriteFile(noPermFile, []byte("no permission"), 0000); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := exec.Command("icacls", noPermFile, "/deny", "*S-1-1-0:(R)").Run(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = exec.Command("icacls", noPermFile, "/remove:d", "*S-1-1-0").Run()
		})
	}

	subDir := filepath.Join(sandboxDir, "subdir.txt")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name         string
		allowedPaths []string
		requestPath  string
		uriTemplate  string
		maxSize      *int64
		wantErr      bool
		errContains  string
	}{
		{
			name:         "allowed absolute path",
			allowedPaths: []string{sandboxDir},
			requestPath:  validFile,
			uriTemplate:  "file://{path}",
			wantErr:      false,
		},
		{
			name:         "denied path outside sandbox",
			allowedPaths: []string{sandboxDir},
			requestPath:  secretFile,
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "security violation",
		},
		{
			name:         "symlink escape attempt",
			allowedPaths: []string{sandboxDir},
			requestPath:  symlinkToSecret,
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "security violation",
		},
		{
			name:         "file size limit exceeded",
			allowedPaths: []string{sandboxDir},
			requestPath:  largeFile,
			uriTemplate:  "file://{path}",
			wantErr:      false, // just truncates, no error
		},
		{
			name:         "custom file size limit exceeded",
			allowedPaths: []string{sandboxDir},
			requestPath:  largeFile,
			uriTemplate:  "file://{path}",
			maxSize:      func() *int64 { i := int64(10); return &i }(),
			wantErr:      false,
		},
		{
			name:         "hidden file without allowed paths",
			allowedPaths: nil,
			requestPath:  hiddenFile,
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "security violation",
		},

		{
			name:         "hidden file succeeds when inside explicitly allowed paths",
			allowedPaths: []string{sandboxDir},
			requestPath:  hiddenFile,
			uriTemplate:  "file://{path}",
			wantErr:      false,
		},
		{
			name:         "visible file succeeds when no allowed paths",
			allowedPaths: nil,
			requestPath:  validFile,
			uriTemplate:  "file://{path}",
			wantErr:      false,
		},
		{
			name:         "file inside hidden directory fails without allowed paths",
			allowedPaths: nil,
			requestPath:  hiddenDirFile,
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "security violation",
		},
		{
			name:         "unsupported binary extension rejected",
			allowedPaths: []string{sandboxDir},
			requestPath:  binFile,
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "file extension not allowed",
		},

		{
			name:         "traversal: basic parent",
			allowedPaths: []string{sandboxDir},
			requestPath:  "../secret.txt",
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "security violation",
		},
		{
			name:         "traversal: windows mixed slashes",
			allowedPaths: []string{sandboxDir},
			requestPath:  "..\\secret.txt",
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "security violation",
		},
		{
			name:         "traversal: url encoded",
			allowedPaths: []string{sandboxDir},
			requestPath:  "%2e%2e/secret.txt",
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "security violation",
		},
		{
			name:         "traversal: url encoded slash",
			allowedPaths: []string{sandboxDir},
			requestPath:  "%2f..%2fsecret.txt",
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "security violation",
		},
		{
			name:         "traversal: url encoded backslash",
			allowedPaths: []string{sandboxDir},
			requestPath:  "..%5csecret.txt",
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "security violation",
		},
		{
			name:         "traversal: url encoded fully",
			allowedPaths: []string{sandboxDir},
			requestPath:  "%2e%2e%2fsecret.txt",
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "security violation",
		},
		{
			name:         "traversal: embedded in filename is safe",
			allowedPaths: []string{sandboxDir},
			requestPath:  filepath.Join(sandboxDir, "file..txt"),
			uriTemplate:  "file://{path}",
			wantErr:      false,
		},
		{
			name:         "non-existent file returns error",
			allowedPaths: []string{sandboxDir},
			requestPath:  nonExistentFile,
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "file not found",
		},
		{
			name:         "directory target rejected",
			allowedPaths: []string{sandboxDir},
			requestPath:  subDir,
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "non-regular file",
		},
		{
			name:         "relative path middle uri template",
			allowedPaths: []string{sandboxDir},
			requestPath:  "valid.txt",
			uriTemplate:  "file://" + filepath.ToSlash(sandboxDir) + "/{path}",
			wantErr:      false,
		},
	}

	if os.Geteuid() != 0 {
		tests = append(tests, struct {
			name         string
			allowedPaths []string
			requestPath  string
			uriTemplate  string
			maxSize      *int64
			wantErr      bool
			errContains  string
		}{
			name:         "missing read permissions returns error",
			allowedPaths: []string{sandboxDir},
			requestPath:  noPermFile,
			uriTemplate:  "file://{path}",
			wantErr:      true,
			errContains:  "failed to open file",
		})
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := file.TemplateConfig{
				AllowedPaths: tc.allowedPaths,
			}
			cfg.Name = "test-template"
			cfg.URITemplate = tc.uriTemplate

			resTmpl, err := cfg.Initialize(context.Background())
			if err != nil {
				t.Fatalf("Initialize failed: %v", err)
			}

			content, err := resTmpl.Read(context.Background(), map[string]any{"path": tc.requestPath})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got none", tc.errContains)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing %q, got: %v", tc.errContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tc.name == "file size limit exceeded" || tc.name == "custom file size limit exceeded" {
					contentStr := content.(string)
					if !strings.Contains(contentStr, "[TRUNCATED BY SERVER") {
						t.Errorf("expected truncation warning, got %q", contentStr[len(contentStr)-200:])
					}
				}
			}
		})
	}
}

func TestFileTemplate_Validation(t *testing.T) {
	tests := []struct {
		name       string
		yamlStr    string
		wantErrMsg string
	}{
		{
			name: "missing uriTemplate",
			yamlStr: `
			kind: resourceTemplate
			name: my-template
			type: file
			`,
			wantErrMsg: "Field validation for 'URITemplate' failed on the 'required' tag",
		},
		{
			name: "invalid max_size negative",
			yamlStr: `
			kind: resourceTemplate
			name: my-template
			type: file
			uriTemplate: "file://{path}"
			max_size: -50
			`,
			wantErrMsg: "must be greater than 0",
		},
		{
			name: "invalid max_size zero",
			yamlStr: `
			kind: resourceTemplate
			name: my-template
			type: file
			uriTemplate: "file://{path}"
			max_size: 0
			`,
			wantErrMsg: "must be greater than 0",
		},
		{
			name: "max_size too large",
			yamlStr: `
			kind: resourceTemplate
			name: my-template
			type: file
			uriTemplate: "file://{path}"
			max_size: 2000000000
			`,
			wantErrMsg: "cannot exceed 1GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tt.yamlStr))
			if err == nil || !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Fatalf("expected UnmarshalPrimitiveConfig to fail with %q, got err: %v", tt.wantErrMsg, err)
			}
		})
	}
}
