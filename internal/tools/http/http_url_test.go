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

package http

import (
	"maps"
	"net/url"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestGetHeadersCaseInsensitiveOverride(t *testing.T) {
	for _, tc := range []struct {
		name      string
		params    map[string]any
		want      map[string]string
		wantError bool
	}{
		{
			name:   "provided parameter overrides static header",
			params: map[string]any{"CONTENT-type": "application/json; charset=UTF-8"},
			want:   map[string]string{"Content-Type": "application/json; charset=UTF-8"},
		},
		{
			name: "omitted parameter retains static header",
			want: map[string]string{"Content-Type": "text/plain; charset=UTF-8"},
		},
		{
			name:   "nil parameter retains static header",
			params: map[string]any{"CONTENT-type": nil},
			want:   map[string]string{"Content-Type": "text/plain; charset=UTF-8"},
		},
		{
			name:   "empty string parameter retains static header",
			params: map[string]any{"CONTENT-type": ""},
			want:   map[string]string{"Content-Type": "text/plain; charset=UTF-8"},
		},
		{
			name:      "non-string parameter returns an error",
			params:    map[string]any{"CONTENT-type": 1},
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headerParams := parameters.Parameters{parameters.NewStringParameter("CONTENT-type", "content type", parameters.WithStringRequired(false))}
			got, err := getHeaders(headerParams, map[string]string{"content-type": "text/plain; charset=UTF-8"}, tc.params)
			if (err != nil) != tc.wantError {
				t.Fatalf("getHeaders() error = %v, wantError %v", err, tc.wantError)
			}
			if !tc.wantError && !maps.Equal(got, tc.want) {
				t.Fatalf("getHeaders() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGetURLHostOverride(t *testing.T) {
	testCases := []struct {
		name         string
		pathParam    string
		expectError  bool
		expectedHost string
	}{
		{
			name:         "at sign in path is not a host override",
			pathParam:    "@evil.com/v1",
			expectError:  false,
			expectedHost: "api.good.com",
		},
		{
			name:        "absolute url in path is rejected",
			pathParam:   "https://evil.com/v1",
			expectError: true,
		},
		{
			name:        "authority override in path is rejected",
			pathParam:   "//evil.com/v1",
			expectError: true,
		},
	}

	baseURL := "https://api.good.com"
	path := "{{.pathParam}}"
	pathParams := parameters.Parameters{parameters.NewStringParameter("pathParam", "path")}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			paramsMap := map[string]any{"pathParam": tc.pathParam}

			urlString, err := getURL(baseURL, path, pathParams, nil, nil, paramsMap)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			parsed, err := url.Parse(urlString)
			if err != nil {
				t.Fatalf("failed to parse URL: %v", err)
			}

			if parsed.Host != tc.expectedHost {
				t.Fatalf("expected host to be %q, got %q", tc.expectedHost, parsed.Host)
			}
		})
	}
}

func TestGetURLPathValidation(t *testing.T) {
	testCases := []struct {
		name         string
		baseURL      string
		pathParam    string
		expectError  bool
		expectedPath string
	}{
		{
			name:         "valid subpath stays within base path",
			baseURL:      "https://api.good.com/base/",
			pathParam:    "v1",
			expectError:  false,
			expectedPath: "/base/v1",
		},
		{
			name:        "path with dot segments is rejected",
			baseURL:     "https://api.good.com/base/",
			pathParam:   "../v1",
			expectError: true,
		},
		{
			name:        "absolute path escaping base path scope is rejected",
			baseURL:     "https://api.good.com/base/",
			pathParam:   "/v1",
			expectError: true,
		},
		{
			name:         "absolute path for root base path is allowed",
			baseURL:      "https://api.good.com/",
			pathParam:    "/v1",
			expectError:  false,
			expectedPath: "/v1",
		},
		{
			name:        "path with url-encoded dot segments is rejected",
			baseURL:     "https://api.good.com/base/",
			pathParam:   "%2e%2e/v1",
			expectError: true,
		},
		{
			name:        "sibling path traversal via simple prefix matching is rejected",
			baseURL:     "https://api.good.com/base",
			pathParam:   "/base-private",
			expectError: true,
		},
		{
			name:         "exact match of base path without trailing slash is allowed",
			baseURL:      "https://api.good.com/base",
			pathParam:    "",
			expectError:  false,
			expectedPath: "/base",
		},
		{
			name:         "double dots in query parameters are allowed",
			baseURL:      "https://api.good.com/base/",
			pathParam:    "v1?date=2023-01-01..2023-01-31",
			expectError:  false,
			expectedPath: "/base/v1",
		},
		{
			name:         "double dots in non-dot segments are allowed",
			baseURL:      "https://api.good.com/base/",
			pathParam:    "file..txt",
			expectError:  false,
			expectedPath: "/base/file..txt",
		},
	}

	path := "{{.pathParam}}"
	pathParams := parameters.Parameters{parameters.NewStringParameter("pathParam", "path")}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			paramsMap := map[string]any{"pathParam": tc.pathParam}

			urlString, err := getURL(tc.baseURL, path, pathParams, nil, nil, paramsMap)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			parsed, err := url.Parse(urlString)
			if err != nil {
				t.Fatalf("failed to parse URL: %v", err)
			}

			if parsed.Path != tc.expectedPath {
				t.Fatalf("expected path to be %q, got %q", tc.expectedPath, parsed.Path)
			}
		})
	}
}

func TestGetURLCustomFuncs(t *testing.T) {
	baseURL := "https://api.good.com/v1/"
	path := "users/{{pathEscape .name}}/details?q={{queryEscape .query}}"
	pathParams := parameters.Parameters{
		parameters.NewStringParameter("name", "user name"),
		parameters.NewStringParameter("query", "search query"),
	}

	paramsMap := map[string]any{
		"name":  "john/doe",
		"query": "hello world",
	}

	urlString, err := getURL(baseURL, path, pathParams, nil, nil, paramsMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "https://api.good.com/v1/users/john%2Fdoe/details?q=hello+world"
	if urlString != expected {
		t.Fatalf("expected %q, got %q", expected, urlString)
	}
}
