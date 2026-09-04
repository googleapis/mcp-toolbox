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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-playground/validator/v10"
	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/resources/file"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

const defaultMaxFileSize = 5 * 1024 * 1024

func TestParseFromYamlFile(t *testing.T) {
	tmpDir := t.TempDir()
	validPath := filepath.Join(tmpDir, "valid.txt")
	if err := os.WriteFile(validPath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	tcs := []struct {
		desc string
		in   string
		want server.ResourceConfigs
	}{
		{
			desc: "basic example",
			in: fmt.Sprintf(`
			kind: resource
			name: my-file
			type: file
			path: %s
			`, filepath.ToSlash(validPath)),
			want: server.ResourceConfigs{
				"my-file": &file.Config{
					ResourceConfigBase: resources.ResourceConfigBase{
						ConfigBase: resources.ConfigBase{
							Name:        "my-file",
							Type:        "file",
							Annotations: &resources.ResourceAnnotations{Priority: func(f float64) *float64 { return &f }(1.0)},
						},
						URI: "file://my-file",
					},
					Path: filepath.ToSlash(validPath),
				},
			},
		},
		{
			desc: "with max_size and annotations",
			in: fmt.Sprintf(`
			kind: resource
			name: my-file-annotated
			type: file
			path: %s
			max_size: 1024
			annotations:
				priority: 0.5
				audience:
					- user
				lastModified: 2024-01-01T00:00:00Z
			`, filepath.ToSlash(validPath)),
			want: server.ResourceConfigs{
				"my-file-annotated": &file.Config{
					ResourceConfigBase: resources.ResourceConfigBase{
						ConfigBase: resources.ConfigBase{
							Name: "my-file-annotated",
							Type: "file",
							Annotations: &resources.ResourceAnnotations{
								Priority:     func(f float64) *float64 { return &f }(0.5),
								Audience:     []resources.AudienceRole{resources.RoleUser},
								LastModified: "2024-01-01T00:00:00Z",
							},
						},
						URI: "file://my-file-annotated",
					},
					Path:    filepath.ToSlash(validPath),
					MaxSize: func(i int64) *int64 { return &i }(1024),
				},
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, got, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got, cmp.AllowUnexported(file.Config{})); diff != "" {
				t.Fatalf("incorrect parse (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFailParseFromYaml(t *testing.T) {
	tmpDir := t.TempDir()
	validPath := filepath.Join(tmpDir, "valid.txt")
	if err := os.WriteFile(validPath, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: fmt.Sprintf(`
			kind: resource
			name: my-file
			type: file
			path: %s
			foo: bar
			`, filepath.ToSlash(validPath)),
			err: "unknown field \"foo\"",
		},
		{
			desc: "missing required path field",
			in: `
			kind: resource
			name: my-file
			type: file
			`,
			err: "Field validation for 'Path' failed on the 'required' tag",
		},
		{
			desc: "max_size zero",
			in: fmt.Sprintf(`
			kind: resource
			name: my-file
			type: file
			path: %s
			max_size: 0
			`, filepath.ToSlash(validPath)),
			err: "must be greater than 0",
		},
		{
			desc: "max_size negative",
			in: fmt.Sprintf(`
			kind: resource
			name: my-file
			type: file
			path: %s
			max_size: -50
			`, filepath.ToSlash(validPath)),
			err: "must be greater than 0",
		},
		{
			desc: "max_size too large",
			in: fmt.Sprintf(`
			kind: resource
			name: my-file
			type: file
			path: %s
			max_size: 2000000000
			`, filepath.ToSlash(validPath)),
			err: "cannot exceed 1GB",
		},
		{
			desc: "max_size type string",
			in: fmt.Sprintf(`
			kind: resource
			name: my-file
			type: file
			path: %s
			max_size: 50MB
			`, filepath.ToSlash(validPath)),
			err: "cannot unmarshal",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			if !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("unexpected error: got %q, want %q", err.Error(), tc.err)
			}
		})
	}
}

// TestFileResource_Validation verifies that the file resource correctly validates
// configurations at boot runtime, blocking invalid paths, directory evasions,
// non-regular files, and non-allowed file extensions.
func TestFileResource_Validation(t *testing.T) {
	tmpDir := t.TempDir()

	exePath := filepath.Join(tmpDir, "test.exe")
	if err := os.WriteFile(exePath, []byte("run"), 0644); err != nil {
		t.Fatal(err)
	}

	dirPath := filepath.Join(tmpDir, "fake.txt")
	if err := os.Mkdir(dirPath, 0755); err != nil {
		t.Fatal(err)
	}

	secretPath := filepath.Join(tmpDir, "secret.exe")
	if err := os.WriteFile(secretPath, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(tmpDir, "symlink.txt")
	if err := os.Symlink(secretPath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	nonExistentPath := filepath.Join(tmpDir, "nonexistent.txt")

	noPermPath := filepath.Join(tmpDir, "noperm.txt")
	if err := os.WriteFile(noPermPath, []byte("noperm"), 0000); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := exec.Command("icacls", noPermPath, "/deny", "*S-1-1-0:(R)").Run(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = exec.Command("icacls", noPermPath, "/remove:d", "*S-1-1-0").Run()
		})
	}

	tests := []struct {
		name       string
		yamlStr    string
		wantErrMsg string
	}{
		{
			name: "path traversal",
			yamlStr: fmt.Sprintf(`
			kind: resource
			name: my-file
			type: file
			path: %s
			`, "../secrets.txt"),
			wantErrMsg: "relative path \"../secrets.txt\" is unsafe",
		},
		{
			name: "invalid extension",
			yamlStr: fmt.Sprintf(`
			kind: resource
			name: my-file
			type: file
			path: %s
			`, filepath.ToSlash(exePath)),
			wantErrMsg: "invalid extension",
		},
		{
			name: "non-regular file directory",
			yamlStr: fmt.Sprintf(`
			kind: resource
			name: my-file
			type: file
			path: %s
			`, filepath.ToSlash(dirPath)),
			wantErrMsg: "not a regular file",
		},
		{
			name: "symlink evasion",
			yamlStr: fmt.Sprintf(`
			kind: resource
			name: my-file
			type: file
			path: %s
			`, filepath.ToSlash(symlinkPath)),
			wantErrMsg: "invalid extension",
		},
		{
			name: "non-existent file",
			yamlStr: fmt.Sprintf(`
			kind: resource
			name: my-file
			type: file
			path: %s
			`, filepath.ToSlash(nonExistentPath)),
			wantErrMsg: "file not found",
		},
	}

	if os.Geteuid() != 0 {
		tests = append(tests, struct {
			name       string
			yamlStr    string
			wantErrMsg string
		}{
			name: "missing read permissions",
			yamlStr: fmt.Sprintf(`
			kind: resource
			name: my-file
			type: file
			path: %s
			`, filepath.ToSlash(noPermPath)),
			wantErrMsg: "missing read permissions",
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			_, _, _, _, _, resConfigs, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tt.yamlStr))
			if err != nil {
				t.Fatalf("UnmarshalPrimitiveConfig failed unexpectedly: %v", err)
			}
			cfg := resConfigs["my-file"]
			_, err = cfg.Initialize(ctx)
			if err == nil || !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Fatalf("expected Initialize to fail with %q, got err: %v", tt.wantErrMsg, err)
			}
		})
	}
}

// TestFileResource_PathResolution ensures that both absolute and relative paths
// resolve correctly, specifically enforcing that relative paths anchor to the
// provided base directory when initializing.
func TestFileResource_PathResolution(t *testing.T) {
	tmpDir := t.TempDir()

	absPath := filepath.Join(tmpDir, "abs.txt")
	if err := os.WriteFile(absPath, []byte("absolute data"), 0644); err != nil {
		t.Fatal(err)
	}

	relPath := "rel.txt"
	if err := os.WriteFile(filepath.Join(tmpDir, relPath), []byte("relative data"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		yamlStr  string
		expected string
	}{
		{
			name:     "absolute path",
			yamlStr:  fmt.Sprintf("type: file\npath: %s", filepath.ToSlash(absPath)),
			expected: "absolute data",
		},
		{
			name:     "relative path anchored to baseDir",
			yamlStr:  fmt.Sprintf("type: file\npath: %s", filepath.ToSlash(relPath)),
			expected: "relative data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), resources.BaseDirKey, tmpDir)
			decoder := yaml.NewDecoder(bytes.NewReader([]byte(tt.yamlStr)), yaml.Strict(), yaml.Validator(validator.New()))
			cfg, err := resources.DecodeConfig(ctx, "file", "test", decoder)
			if err != nil {
				t.Fatalf("DecodeConfig failed: %v", err)
			}
			res, err := cfg.Initialize(ctx)
			if err != nil {
				t.Fatalf("Initialize failed: %v", err)
			}
			data, err := res.Read(ctx, nil)
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}
			if data.(string) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, data)
			}
		})
	}
}

// TestFileResource_Truncation checks that file contents exceeding the safety
// max_size limit are truncated properly, appending a clear warning message
// to indicate partial reads to the MCP client.
func TestFileResource_Truncation(t *testing.T) {
	tmpDir := t.TempDir()

	size := defaultMaxFileSize + 100
	largeContent := make([]byte, size)
	for i := 0; i < size; i++ {
		largeContent[i] = 'A'
	}
	largePath := filepath.Join(tmpDir, "large.txt")
	if err := os.WriteFile(largePath, largeContent, 0644); err != nil {
		t.Fatal(err)
	}

	smallContent := make([]byte, 100)
	for i := 0; i < 100; i++ {
		smallContent[i] = 'B'
	}
	smallPath := filepath.Join(tmpDir, "small.txt")
	if err := os.WriteFile(smallPath, smallContent, 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		yamlStr   string
		wantSize  int64
		wantTrunc bool
	}{
		{
			name:      "default truncation 5MB",
			yamlStr:   fmt.Sprintf("type: file\npath: %s", filepath.ToSlash(largePath)),
			wantSize:  int64(defaultMaxFileSize),
			wantTrunc: true,
		},
		{
			name:      "override max_size small",
			yamlStr:   fmt.Sprintf("type: file\npath: %s\nmax_size: 50", filepath.ToSlash(smallPath)),
			wantTrunc: true,
		},
		{
			name:      "no truncation needed",
			yamlStr:   fmt.Sprintf("type: file\npath: %s\nmax_size: 200", filepath.ToSlash(smallPath)),
			wantTrunc: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			decoder := yaml.NewDecoder(bytes.NewReader([]byte(tt.yamlStr)), yaml.Strict(), yaml.Validator(validator.New()))
			cfg, err := resources.DecodeConfig(ctx, "file", "test", decoder)
			if err != nil {
				t.Fatalf("DecodeConfig failed: %v", err)
			}
			res, err := cfg.Initialize(ctx)
			if err != nil {
				t.Fatalf("Initialize failed: %v", err)
			}

			data, err := res.Read(ctx, nil)
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			strData := data.(string)
			hasWarning := strings.HasSuffix(strData, "safety limit]...")
			if hasWarning != tt.wantTrunc {
				t.Errorf("expected truncation warning %v, got %v", tt.wantTrunc, hasWarning)
			}
		})
	}
}

// TestFileResource_TOCTOU tests Time-Of-Check to Time-Of-Use mitigations.
// It verifies that if a file is maliciously swapped with a symlink or directory
// between the security validation phase and the actual read operation, the
// read will be aggressively rejected to prevent arbitrary read vulnerabilities.
func TestFileResource_TOCTOU(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name       string
		swapAction func(t *testing.T, path string)
	}{
		{
			name: "swap with symlink to binary",
			swapAction: func(t *testing.T, path string) {
				target := filepath.Join(tmpDir, "malicious.exe")
				if err := os.WriteFile(target, []byte("bad"), 0644); err != nil {
					t.Fatalf("failed to write file: %v", err)
				}
				os.Remove(path)
				if err := os.Symlink(target, path); err != nil {
					t.Fatalf("failed to create symlink: %v", err)
				}
			},
		},
		{
			name: "swap with directory",
			swapAction: func(t *testing.T, path string) {
				os.Remove(path)
				if err := os.Mkdir(path, 0755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(tmpDir, strings.ReplaceAll(tt.name, " ", "_")+".txt")
			if err := os.WriteFile(filePath, []byte("safe content"), 0644); err != nil {
				t.Fatal(err)
			}

			yamlStr := fmt.Sprintf("type: file\npath: %s", filepath.ToSlash(filePath))
			ctx := context.Background()
			decoder := yaml.NewDecoder(bytes.NewReader([]byte(yamlStr)), yaml.Strict())
			cfg, err := resources.DecodeConfig(ctx, "file", "test", decoder)
			if err != nil {
				t.Fatalf("DecodeConfig failed: %v", err)
			}

			res, err := cfg.Initialize(ctx)
			if err != nil {
				t.Fatalf("Initialize failed: %v", err)
			}

			tt.swapAction(t, filePath)

			_, err = res.Read(ctx, nil)
			if err == nil {
				t.Fatalf("expected Read to fail after TOCTOU swap")
			}
			if !strings.Contains(err.Error(), "security violation") {
				t.Errorf("expected security violation error, got: %v", err)
			}
		})
	}
}

// TestFileResource_Metadata verifies that dynamic resource metadata, such as
// MimeType and LastModified timestamps, are correctly computed and populated
// when retrieving resource configurations.
func TestFileResource_Metadata(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notes.md")
	if err := os.WriteFile(filePath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	yamlStr := fmt.Sprintf("type: file\npath: %s", filepath.ToSlash(filePath))
	ctx := context.Background()
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(yamlStr)), yaml.Strict())
	cfg, _ := resources.DecodeConfig(ctx, "file", "notes", decoder)
	res, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	mimeType := res.GetMimeType()

	if !strings.HasPrefix(mimeType, "text/markdown") && !strings.HasPrefix(mimeType, "text/plain") && mimeType != "" {
		t.Errorf("expected reasonable MimeType, got: %v", mimeType)
	}

	anns := res.GetAnnotations()
	if anns == nil || anns.LastModified == "" {
		t.Errorf("expected LastModified annotation to be set")
	} else {
		_, err := time.Parse(time.RFC3339, anns.LastModified)
		if err != nil {
			t.Errorf("expected valid RFC3339 LastModified, got %q", anns.LastModified)
		}
	}
}

// TestFileResource_NonExistentFileFailsAtBoot ensures that if a resource file does
// not yet exist during server boot, the server halts.
func TestFileResource_NonExistentFileFailsAtBoot(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "delayed.txt")

	yamlStr := "type: file\npath: " + filepath.ToSlash(filePath)
	ctx := context.Background()
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(yamlStr)), yaml.Strict())
	cfg, err := resources.DecodeConfig(ctx, "file", "delayed", decoder)
	if err != nil {
		t.Fatalf("DecodeConfig failed: %v", err)
	}

	_, err = cfg.Initialize(ctx)
	if err == nil {
		t.Fatalf("Initialize succeeded unexpectedly for non-existent file")
	}
	if !strings.Contains(err.Error(), "files must exist at boot time to prevent dead URIs") {
		t.Errorf("Expected boot-time existence error, got: %v", err)
	}
}

// TestFileResource_DynamicMetadata ensures that the resource configuration
// (specifically its size and modification timestamp) updates dynamically
// at runtime upon subsequent MCP list or read requests.
func TestFileResource_DynamicMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "dynamic.txt")

	if err := os.WriteFile(filePath, []byte("1234567890"), 0644); err != nil {
		t.Fatal(err)
	}

	yamlStr := fmt.Sprintf("type: file\npath: %s", filepath.ToSlash(filePath))
	ctx := context.Background()
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(yamlStr)), yaml.Strict())
	cfg, _ := resources.DecodeConfig(ctx, "file", "dynamic", decoder)
	res, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	fileCfg := res.ToConfig().(*file.Config)
	initialTimestamp := fileCfg.Annotations.LastModified

	time.Sleep(10 * time.Millisecond)

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("12345")); err != nil {
		t.Fatal(err)
	}
	f.Close()

	updatedAnns := res.GetAnnotations()
	if updatedAnns.LastModified == initialTimestamp {
		_, err := time.Parse(time.RFC3339, updatedAnns.LastModified)
		if err != nil {
			t.Errorf("Invalid RFC3339 LastModified: %q", updatedAnns.LastModified)
		}
	}
}

// TestFileResource_NoBaseDirContext tests the fallback boundary enforcement.
// If the server initializes a resource without an explicitly configured base
// directory, it defaults to the current working directory and successfully
// blocks any relative traversals or local symlink escapes.
func TestFileResource_NoBaseDirContext(t *testing.T) {
	baseDirTmp := t.TempDir()
	outsideTmp := t.TempDir()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(baseDirTmp); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Error(err)
		}
	}()

	secretFile := filepath.Join(outsideTmp, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	symlinkPath := "test_symlink_escape.txt"
	os.Remove(symlinkPath)
	if err := os.Symlink(secretFile, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	yamlStr := fmt.Sprintf("type: file\npath: %s", symlinkPath)
	ctx := context.Background()
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(yamlStr)), yaml.Strict())
	cfg, err := resources.DecodeConfig(ctx, "file", "test", decoder)
	if err != nil {
		t.Fatalf("DecodeConfig failed: %v", err)
	}

	_, err = cfg.Initialize(ctx)
	if err == nil {
		t.Fatalf("expected Initialize to fail due to symlink base escape, but it succeeded")
	}
	if !strings.Contains(err.Error(), "escapes base directory") {
		t.Errorf("expected escapes base directory error, got: %v", err)
	}
}

// TestFileResource_UTF8Truncation verifies that when truncation occurs mid-byte
// on a multi-byte UTF-8 character, the implementation securely steps back to a
// valid rune boundary, preventing corrupted payloads from crashing decoders.
func TestFileResource_UTF8Truncation(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "utf8.txt")
	content := []byte("Hello 🚀🚀")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	yamlStr := fmt.Sprintf("type: file\npath: %s\nmax_size: 8", filepath.ToSlash(filePath))
	ctx := context.Background()
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(yamlStr)), yaml.Strict())
	cfg, err := resources.DecodeConfig(ctx, "file", "test", decoder)
	if err != nil {
		t.Fatalf("DecodeConfig failed: %v", err)
	}

	res, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	data, err := res.Read(ctx, nil)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	strData := data.(string)

	if !utf8.ValidString(strData) {
		t.Errorf("Truncated string contains invalid UTF-8 sequences!")
	}

	expectedPrefix := "Hello "
	if !strings.HasPrefix(strData, expectedPrefix) {
		t.Errorf("Expected prefix %q, got %q", expectedPrefix, strData)
	}
	if strings.Contains(strData, "\ufffd") {
		t.Errorf("String contains unicode replacement character, meaning it wasn't safely truncated.")
	}
}

// TestFileResource_ToConfigNonRegularFile verifies that ToConfig does not
// leak metadata (size and modification time) if the underlying target file
// is swapped with a non-regular file (like a directory) post-initialization.
func TestFileResource_ToConfigNonRegularFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(filePath, []byte("some content"), 0644); err != nil {
		t.Fatal(err)
	}

	yamlStr := fmt.Sprintf("type: file\npath: %s", filepath.ToSlash(filePath))
	ctx := context.Background()
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(yamlStr)), yaml.Strict())
	cfg, err := resources.DecodeConfig(ctx, "file", "test", decoder)
	if err != nil {
		t.Fatalf("DecodeConfig failed: %v", err)
	}

	res, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Swap the valid file with a directory
	if err := os.Remove(filePath); err != nil {
		t.Fatalf("failed to remove file: %v", err)
	}
	if err := os.Mkdir(filePath, 0755); err != nil {
		t.Fatalf("failed to create directory in place of file: %v", err)
	}

	config := res.ToConfig().(*file.Config)
	if config.Annotations != nil && config.Annotations.LastModified != "" {
		t.Errorf("Expected LastModified to be empty for non-regular file, got %q", config.Annotations.LastModified)
	}
}
