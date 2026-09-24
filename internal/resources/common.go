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

package resources

import (
	"fmt"
	"mime"
	"net/url"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	// DefaultMaxFileSize is the default maximum number of bytes read from a file or GCS resource (5MB).
	DefaultMaxFileSize int64 = 5 * 1024 * 1024
	// MaxAllowedFileSize is the maximum configurable maxSize for a file or GCS resource (1GB).
	MaxAllowedFileSize int64 = 1024 * 1024 * 1024
)

var allowedExts = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".json": true,
	".yaml": true, ".yml": true, ".xml": true, ".sql": true,
	".html": true, ".htm": true, ".js": true, ".css": true, ".svg": true,
	".py": true,
}

// ValidateExtension checks if a file or object extension is in the safe text allowlist.
func ValidateExtension(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if !allowedExts[ext] {
		return fmt.Errorf("file extension %q is not allowed", ext)
	}
	return nil
}

// ValidateMaxSize validates that maxSize (when set) is positive and does not exceed MaxAllowedFileSize (1GB).
func ValidateMaxSize(maxSize *int64, entityLabel, name string) error {
	if maxSize == nil {
		return nil
	}
	if *maxSize <= 0 {
		return fmt.Errorf("%s %q maxSize must be greater than 0", entityLabel, name)
	}
	if *maxSize > MaxAllowedFileSize {
		return fmt.Errorf("%s %q maxSize cannot exceed 1GB", entityLabel, name)
	}
	return nil
}

// ContainsTraversal checks if any component of the path is a backward traversal (".."),
// including URL-encoded variants (%2e%2e) and backslashes.
func ContainsTraversal(p string) bool {
	// Check for URL-encoded traversal attempts to prevent evasion
	decoded, err := url.PathUnescape(p)
	if err == nil {
		p = decoded
	}

	// Convert any backslashes to forward slashes for unified checking
	p = strings.ReplaceAll(p, "\\", "/")

	parts := strings.Split(p, "/")
	for _, part := range parts {
		if part == ".." {
			return true
		}
	}
	return false
}

// ValidateTemplatePathParam validates a dynamic {path} template parameter for traversal (".."),
// URL-encoded traversal ("%2e%2e"), backslashes ("\"), and double slashes ("//").
func ValidateTemplatePathParam(p string) error {
	decoded := p
	if unescaped, err := url.PathUnescape(p); err == nil {
		decoded = unescaped
	}

	if ContainsTraversal(p) {
		return fmt.Errorf("security violation: path %q contains backward traversal components (..)", p)
	}

	if strings.Contains(p, "\\") || strings.Contains(decoded, "\\") {
		return fmt.Errorf("security violation: path %q contains backslashes", p)
	}

	if strings.Contains(p, "//") || strings.Contains(decoded, "//") {
		return fmt.Errorf("security violation: path %q contains double slashes (//)", p)
	}

	return nil
}

// ContainsHiddenSegment checks if any segment of the path starts with a dot (e.g. ".env", ".git", ".secrets").
func ContainsHiddenSegment(p string) bool {
	parts := strings.Split(strings.ReplaceAll(p, "\\", "/"), "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
	}
	return false
}

// InferMimeType resolves the MIME type from the path's extension, falling back to "text/plain".
func InferMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if mt := mime.TypeByExtension(ext); mt != "" {
		return mt
	}
	return "text/plain"
}

// TruncateUTF8 truncates content exceeding limit at a valid UTF-8 rune boundary
// and appends a server truncation warning.
func TruncateUTF8(content []byte, limit int64) string {
	if int64(len(content)) > limit {
		truncated := content[:limit]
		for len(truncated) > 0 {
			r, size := utf8.DecodeLastRune(truncated)
			if r == utf8.RuneError && size == 1 {
				truncated = truncated[:len(truncated)-1]
			} else {
				break
			}
		}
		warning := fmt.Sprintf("\n\n...[TRUNCATED BY SERVER: Payload exceeded %d byte safety limit]...", limit)
		return string(truncated) + warning
	}
	return string(content)
}
