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

	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
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
			// CheckUISupportFromRequest is currently disabled to comply with capability negotiation spec.
			if got != false {
				t.Errorf("CheckUISupportFromRequest() = %v, want false (disabled)", got)
			}
		})
	}
}

func TestResolveToolUIMetadata(t *testing.T) {
	resByName := testutils.NewMockResource("res-by-name", "ui://app/res-name.html", "", "", "", nil, nil)
	uiResByURI := testutils.NewMockUIResource("ui-res", "ui://app/by-uri.html", "", "", "", nil, nil, nil, nil, "", nil)
	tmplByName := testutils.NewMockResourceTemplate("tmpl-by-name", "ui://app/tmpl/{path}", "", "", "", nil)
	uiTmplByURI := testutils.NewMockUIResourceTemplate("ui-tmpl", "ui://app/ui-tmpl/{path}", "", "", "", nil, nil, nil, "", nil)

	resourcesMap := map[string]resources.Resource{
		resByName.GetName():  resByName,
		uiResByURI.GetName(): uiResByURI,
	}
	templatesMap := map[string]resources.ResourceTemplate{
		tmplByName.GetName():  tmplByName,
		uiTmplByURI.GetName(): uiTmplByURI,
	}
	pMgr := primitives.NewPrimitiveManager(nil, nil, nil, nil, nil, resourcesMap, templatesMap, nil)

	tests := []struct {
		name    string
		tool    tools.Tool
		wantURI string
		wantErr bool
	}{
		{
			name:    "tool without UI metadata",
			tool:    testutils.NewMockTool("plain-tool", "", "", nil, false, false),
			wantURI: "",
			wantErr: false,
		},
		{
			name:    "resolve resource by name",
			tool:    testutils.NewMockToolWithUI("tool-res-name", "", "", nil, false, false, "res-by-name"),
			wantURI: "ui://app/res-name.html",
			wantErr: false,
		},
		{
			name:    "resolve resource template by name",
			tool:    testutils.NewMockToolWithUI("tool-tmpl-name", "", "", nil, false, false, "tmpl-by-name"),
			wantURI: "ui://app/tmpl/{path}",
			wantErr: false,
		},
		{
			name:    "resolve UI resource by URI",
			tool:    testutils.NewMockToolWithUI("tool-ui-res-uri", "", "", nil, false, false, "ui://app/by-uri.html"),
			wantURI: "ui://app/by-uri.html",
			wantErr: false,
		},
		{
			name:    "resolve UI resource template by URI",
			tool:    testutils.NewMockToolWithUI("tool-ui-tmpl-uri", "", "", nil, false, false, "ui://app/ui-tmpl/dashboard"),
			wantURI: "ui://app/ui-tmpl/{path}",
			wantErr: false,
		},
		{
			name:    "unregistered UI resource returns error",
			tool:    testutils.NewMockToolWithUI("tool-missing", "", "", nil, false, false, "nonexistent-res"),
			wantURI: "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveToolUIMetadata(pMgr, tc.tool)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ResolveToolUIMetadata() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantURI == "" {
				if got != nil {
					t.Errorf("ResolveToolUIMetadata() = %v, want nil", got)
				}
				return
			}
			if got == nil || got["resourceUri"] != tc.wantURI {
				t.Errorf("ResolveToolUIMetadata() resourceUri = %v, want %q", got, tc.wantURI)
			}
		})
	}
}

