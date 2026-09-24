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

package resources_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/googleapis/mcp-toolbox/internal/resources"
)

func TestValidateExtension(t *testing.T) {
	allowed := []string{
		"file.txt", "README.md", "data.csv", "config.json",
		"spec.yaml", "spec.yml", "feed.xml", "schema.sql",
		"index.html", "index.htm", "app.js", "style.css", "icon.svg",
		"script.py", "UPPER.TXT", "nested/dir/report.MD",
	}
	for _, p := range allowed {
		if err := resources.ValidateExtension(p); err != nil {
			t.Errorf("expected %q to be allowed, got error: %v", p, err)
		}
	}

	rejected := []string{
		"secret.key", "cert.pem", ".env", "config.env",
		"run.sh", "binary.exe", ".git",
		"image.png", "archive.tar.gz", "noext",
	}
	for _, p := range rejected {
		if err := resources.ValidateExtension(p); err == nil {
			t.Errorf("expected %q to be rejected, got nil error", p)
		}
	}
}

func TestValidateMaxSize(t *testing.T) {
	int64Ptr := func(v int64) *int64 { return &v }

	if err := resources.ValidateMaxSize(nil, "gcs resource", "test"); err != nil {
		t.Errorf("expected nil maxSize to succeed, got %v", err)
	}
	if err := resources.ValidateMaxSize(int64Ptr(1024), "gcs resource", "test"); err != nil {
		t.Errorf("expected valid maxSize to succeed, got %v", err)
	}
	if err := resources.ValidateMaxSize(int64Ptr(resources.MaxAllowedFileSize), "gcs resource", "test"); err != nil {
		t.Errorf("expected exact MaxAllowedFileSize to succeed, got %v", err)
	}
	if err := resources.ValidateMaxSize(int64Ptr(0), "gcs resource", "test"); err == nil || !strings.Contains(err.Error(), "must be greater than 0") {
		t.Errorf("expected zero maxSize to fail, got %v", err)
	}
	if err := resources.ValidateMaxSize(int64Ptr(-10), "gcs resource", "test"); err == nil || !strings.Contains(err.Error(), "must be greater than 0") {
		t.Errorf("expected negative maxSize to fail, got %v", err)
	}
	if err := resources.ValidateMaxSize(int64Ptr(resources.MaxAllowedFileSize+1), "gcs resource", "test"); err == nil || !strings.Contains(err.Error(), "cannot exceed 1GB") {
		t.Errorf("expected >1GB maxSize to fail, got %v", err)
	}
}

func TestContainsTraversalAndValidateTemplatePathParam(t *testing.T) {
	invalid := []struct {
		path        string
		errContains string
	}{
		{"../secret.txt", "backward traversal"},
		{"a/b/../../secret.txt", "backward traversal"},
		{"%2e%2e/secret.txt", "backward traversal"},
		{"%2e%2e%2fsecret.txt", "backward traversal"},
		{"..%5csecret.txt", "backward traversal"},
		{"a\\b.txt", "backslashes"},
		{"a%5cb.txt", "backslashes"},
		{"a//b.txt", "double slashes"},
		{"a%2f%2fb.txt", "double slashes"},
	}

	for _, tc := range invalid {
		err := resources.ValidateTemplatePathParam(tc.path)
		if err == nil || !strings.Contains(err.Error(), tc.errContains) {
			t.Errorf("ValidateTemplatePathParam(%q): expected error containing %q, got %v", tc.path, tc.errContains, err)
		}
	}

	valid := []string{
		"2025/report.md",
		"data_dictionary.csv",
		"nested/file..txt",
	}
	for _, p := range valid {
		if err := resources.ValidateTemplatePathParam(p); err != nil {
			t.Errorf("ValidateTemplatePathParam(%q): unexpected error: %v", p, err)
		}
	}
}

func TestContainsHiddenSegment(t *testing.T) {
	hidden := []string{
		".env",
		".git/config",
		"folder/.hidden.txt",
		"a/.secrets/data.txt",
		`dir\.git/config`,
	}
	for _, p := range hidden {
		if !resources.ContainsHiddenSegment(p) {
			t.Errorf("expected ContainsHiddenSegment(%q) to be true", p)
		}
	}

	visible := []string{
		"normal/file.txt",
		"file..txt",
		"2026/incident.md",
	}
	for _, p := range visible {
		if resources.ContainsHiddenSegment(p) {
			t.Errorf("expected ContainsHiddenSegment(%q) to be false", p)
		}
	}
}

func TestInferMimeType(t *testing.T) {
	if got := resources.InferMimeType("data.json"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("expected application/json for .json, got %q", got)
	}
	if got := resources.InferMimeType("unknown.unknownext"); got != "text/plain" {
		t.Errorf("expected default text/plain, got %q", got)
	}
}

func TestTruncateUTF8(t *testing.T) {
	input := []byte("a€b")
	got := resources.TruncateUTF8(input, 3)
	if !utf8.ValidString(got) {
		t.Fatalf("TruncateUTF8 returned invalid UTF-8 string: %q", got)
	}
	if !strings.HasPrefix(got, "a\n\n...[TRUNCATED BY SERVER:") {
		t.Errorf("expected prefix 'a' followed by truncation warning, got %q", got)
	}

	// No truncation when within limit
	gotNoTrunc := resources.TruncateUTF8(input, 10)
	if gotNoTrunc != "a€b" {
		t.Errorf("expected 'a€b', got %q", gotNoTrunc)
	}
}
