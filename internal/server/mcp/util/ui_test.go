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

package util

import (
	"testing"
)

func TestValidateUISupport(t *testing.T) {
	tests := []struct {
		name       string
		cap        McpUiClientCapabilities
		supportsUI bool
	}{
		{
			name:       "empty mime types",
			cap:        McpUiClientCapabilities{MimeTypes: []string{}},
			supportsUI: false,
		},
		{
			name: "exact mimeType match",
			cap: McpUiClientCapabilities{
				MimeTypes: []string{"text/html;profile=mcp-app"},
			},
			supportsUI: true,
		},
		{
			name: "mimeType with space",
			cap: McpUiClientCapabilities{
				MimeTypes: []string{"text/html; profile=mcp-app"},
			},
			supportsUI: true,
		},
		{
			name: "unsupported mimeType",
			cap: McpUiClientCapabilities{
				MimeTypes: []string{"application/json"},
			},
			supportsUI: false,
		},
		{
			name: "multiple mimeTypes with supported one",
			cap: McpUiClientCapabilities{
				MimeTypes: []string{"application/json", "text/html;profile=mcp-app"},
			},
			supportsUI: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateUISupport(tc.cap)
			if got != tc.supportsUI {
				t.Errorf("ValidateUISupport() = %v, want %v", got, tc.supportsUI)
			}
		})
	}
}

func TestCheckUISupport(t *testing.T) {
	tests := []struct {
		name       string
		exts       map[string]any
		supportsUI bool
	}{
		{
			name:       "nil extensions",
			exts:       nil,
			supportsUI: false,
		},
		{
			name:       "empty extensions",
			exts:       map[string]any{},
			supportsUI: false,
		},
		{
			name: "missing ui extension",
			exts: map[string]any{
				"com.example/other": map[string]any{},
			},
			supportsUI: false,
		},
		{
			name: "valid ui extension ([]string)",
			exts: map[string]any{
				UIExtensionURI: map[string]any{
					"mimeTypes": []string{"text/html;profile=mcp-app"},
				},
			},
			supportsUI: true,
		},
		{
			name: "valid ui extension ([]any from json.Unmarshal)",
			exts: map[string]any{
				UIExtensionURI: map[string]any{
					"mimeTypes": []any{"text/html;profile=mcp-app"},
				},
			},
			supportsUI: true,
		},
		{
			name: "valid ui extension (McpUiClientCapabilities struct)",
			exts: map[string]any{
				UIExtensionURI: McpUiClientCapabilities{
					MimeTypes: []string{"text/html;profile=mcp-app"},
				},
			},
			supportsUI: true,
		},
		{
			name: "valid ui extension (*McpUiClientCapabilities pointer)",
			exts: map[string]any{
				UIExtensionURI: &McpUiClientCapabilities{
					MimeTypes: []string{"text/html;profile=mcp-app"},
				},
			},
			supportsUI: true,
		},
		{
			name: "invalid mimeType in ui extension",
			exts: map[string]any{
				UIExtensionURI: map[string]any{
					"mimeTypes": []string{"image/png"},
				},
			},
			supportsUI: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckUISupport(tc.exts)
			if got != tc.supportsUI {
				t.Errorf("CheckUISupport() = %v, want %v", got, tc.supportsUI)
			}
		})
	}
}

func TestCheckUISupportFromRequest(t *testing.T) {
	tests := []struct {
		name       string
		body       []byte
		supportsUI bool
	}{
		{
			name:       "all nil/empty",
			body:       nil,
			supportsUI: false,
		},
		{
			name:       "meta with io.modelcontextprotocol/clientCapabilities",
			body:       []byte(`{"params": {"_meta": {"io.modelcontextprotocol/clientCapabilities": {"extensions": {"io.modelcontextprotocol/ui": {"mimeTypes": ["text/html;profile=mcp-app"]}}}}}}`),
			supportsUI: true,
		},
		{
			name:       "meta with clientCapabilities",
			body:       []byte(`{"params": {"_meta": {"clientCapabilities": {"extensions": {"io.modelcontextprotocol/ui": {"mimeTypes": ["text/html;profile=mcp-app"]}}}}}}`),
			supportsUI: true,
		},
		{
			name:       "meta with direct extensions",
			body:       []byte(`{"params": {"_meta": {"extensions": {"io.modelcontextprotocol/ui": {"mimeTypes": ["text/html;profile=mcp-app"]}}}}}`),
			supportsUI: true,
		},
		{
			name:       "meta with direct extension uri key",
			body:       []byte(`{"params": {"_meta": {"io.modelcontextprotocol/ui": {"mimeTypes": ["text/html;profile=mcp-app"]}}}}`),
			supportsUI: true,
		},
		{
			name:       "capabilities struct with extensions",
			body:       []byte(`{"params": {"capabilities": {"extensions": {"io.modelcontextprotocol/ui": {"mimeTypes": ["text/html;profile=mcp-app"]}}}}}`),
			supportsUI: true,
		},
		{
			name: "raw body with params._meta",
			body: []byte(`{
				"jsonrpc": "2.0",
				"id": 1,
				"method": "tools/list",
				"params": {
					"_meta": {
						"io.modelcontextprotocol/clientCapabilities": {
							"extensions": {
								"io.modelcontextprotocol/ui": {
									"mimeTypes": ["text/html;profile=mcp-app"]
								}
							}
						}
					}
				}
			}`),
			supportsUI: true,
		},
		{
			name: "raw body with params.capabilities",
			body: []byte(`{
				"jsonrpc": "2.0",
				"id": 1,
				"method": "tools/list",
				"params": {
					"capabilities": {
						"extensions": {
							"io.modelcontextprotocol/ui": {
								"mimeTypes": ["text/html;profile=mcp-app"]
							}
						}
					}
				}
			}`),
			supportsUI: true,
		},
		{
			name: "raw body without UI capability",
			body: []byte(`{
				"jsonrpc": "2.0",
				"id": 1,
				"method": "tools/list",
				"params": {
					"cursor": "123"
				}
			}`),
			supportsUI: false,
		},
		{
			name: "raw body with unrelated extension",
			body: []byte(`{
				"jsonrpc": "2.0",
				"id": 1,
				"method": "tools/list",
				"params": {
					"_meta": {
						"extensions": {
							"com.example/other": {}
						}
					}
				}
			}`),
			supportsUI: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckUISupportFromRequest(tc.body)
			if got != tc.supportsUI {
				t.Errorf("CheckUISupportFromRequest() = %v, want %v", got, tc.supportsUI)
			}
		})
	}
}
