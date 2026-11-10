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

package server

import (
	"testing"
)

func TestGetPRMURL(t *testing.T) {
	tests := []struct {
		name       string
		toolboxUrl string
		want       string
		wantErr    bool
	}{
		{
			name:       "empty url",
			toolboxUrl: "",
			want:       "/.well-known/oauth-protected-resource",
		},
		{
			name:       "root url without trailing slash",
			toolboxUrl: "https://toolbox.example.com",
			want:       "https://toolbox.example.com/.well-known/oauth-protected-resource",
		},
		{
			name:       "root url with trailing slash",
			toolboxUrl: "https://toolbox.example.com/",
			want:       "https://toolbox.example.com/.well-known/oauth-protected-resource",
		},
		{
			name:       "url with port without path",
			toolboxUrl: "http://127.0.0.1:5000",
			want:       "http://127.0.0.1:5000/.well-known/oauth-protected-resource",
		},
		{
			name:       "url with single segment path",
			toolboxUrl: "https://toolbox.example.com/mcp",
			want:       "https://toolbox.example.com/.well-known/oauth-protected-resource/mcp",
		},
		{
			name:       "url with single segment path and trailing slash",
			toolboxUrl: "https://toolbox.example.com/mcp/",
			want:       "https://toolbox.example.com/.well-known/oauth-protected-resource/mcp",
		},
		{
			name:       "url with multi segment path",
			toolboxUrl: "https://toolbox.example.com/api/v1/mcp",
			want:       "https://toolbox.example.com/.well-known/oauth-protected-resource/api/v1/mcp",
		},
		{
			name:       "url with port and path",
			toolboxUrl: "http://127.0.0.1:5000/mcp",
			want:       "http://127.0.0.1:5000/.well-known/oauth-protected-resource/mcp",
		},
		{
			name:       "relative path with leading slash",
			toolboxUrl: "/mcp",
			want:       "/.well-known/oauth-protected-resource/mcp",
		},
		{
			name:       "relative path without leading slash",
			toolboxUrl: "mcp",
			want:       "/.well-known/oauth-protected-resource/mcp",
		},
		{
			name:       "relative path with trailing slash",
			toolboxUrl: "/mcp/",
			want:       "/.well-known/oauth-protected-resource/mcp",
		},
		{
			name:       "relative root slash",
			toolboxUrl: "/",
			want:       "/.well-known/oauth-protected-resource",
		},
		{
			name:       "relative multi-segment path with leading slash",
			toolboxUrl: "/api/v1/mcp",
			want:       "/.well-known/oauth-protected-resource/api/v1/mcp",
		},
		{
			name:       "relative multi-segment path without leading slash",
			toolboxUrl: "api/v1/mcp",
			want:       "/.well-known/oauth-protected-resource/api/v1/mcp",
		},
		{
			name:       "invalid url with scheme but no host",
			toolboxUrl: "http://",
			wantErr:    true,
		},
		{
			name:       "malformed url",
			toolboxUrl: "://invalid",
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePRMURL(tc.toolboxUrl)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parsePRMURL(%q) error = %v, wantErr %v", tc.toolboxUrl, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("parsePRMURL(%q) = %q, want %q", tc.toolboxUrl, got, tc.want)
			}
			if !tc.wantErr {
				s := &Server{prmURL: got}
				if s.getPRMURL() != got {
					t.Errorf("s.getPRMURL() = %q, want %q", s.getPRMURL(), got)
				}
			}
		})
	}
}
