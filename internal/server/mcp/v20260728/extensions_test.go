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

package v20260728

import (
	"slices"
	"testing"
)

const testExtURI = "com.google.cloud/test-extension"

func TestParseSupportedExtensions(t *testing.T) {
	orig := ServerExtensions
	t.Cleanup(func() {
		ServerExtensions = orig
	})
	tests := []struct {
		name        string
		extensions  map[string]any
		serverExts  map[string]any
		expectedUri string
		expectedVal bool
	}{
		{
			name:        "nil map",
			extensions:  nil,
			serverExts:  nil,
			expectedUri: testExtURI,
			expectedVal: false,
		},
		{
			name: "enabled extension via empty settings object in extensions",
			extensions: map[string]any{
				testExtURI: map[string]any{},
			},
			serverExts:  nil,
			expectedUri: testExtURI,
			expectedVal: true,
		},
		{
			name: "enabled extension via settings object with values in extensions",
			extensions: map[string]any{
				testExtURI: map[string]any{"setting": "val"},
			},
			serverExts:  map[string]any{testExtURI: map[string]any{}},
			expectedUri: testExtURI,
			expectedVal: true,
		},
		{
			name: "server does not support extension",
			extensions: map[string]any{
				testExtURI: map[string]any{},
			},
			serverExts:  map[string]any{"other-extension": map[string]any{}},
			expectedUri: testExtURI,
			expectedVal: false,
		},
		{
			name: "nil value in client extensions",
			extensions: map[string]any{
				testExtURI: nil,
			},
			serverExts:  nil,
			expectedUri: testExtURI,
			expectedVal: true,
		},
		{
			name: "server extensions empty",
			extensions: map[string]any{
				testExtURI: map[string]any{},
			},
			serverExts:  map[string]any{},
			expectedUri: testExtURI,
			expectedVal: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			origSupported := SupportedExtensions
			t.Cleanup(func() {
				SupportedExtensions = origSupported
			})
			if tc.serverExts != nil {
				SupportedExtensions = tc.serverExts
			} else {
				SupportedExtensions = map[string]any{testExtURI: map[string]any{}}
			}
			Initialize(nil)
			exts := ParseSupportedExtensions(tc.extensions)
			_, ok := exts[tc.expectedUri]
			if ok != tc.expectedVal {
				t.Errorf("ParseSupportedExtensions() value for %s = %v, want %v", tc.expectedUri, ok, tc.expectedVal)
			}
		})
	}
}

func TestServerExtensions(t *testing.T) {
	origServer := ServerExtensions
	t.Cleanup(func() {
		ServerExtensions = origServer
	})

	tests := []struct {
		name        string
		setup       func()
		expectedUri string
		expectedVal bool
	}{
		{
			name: "unregistered / nil server extensions",
			setup: func() {
				ServerExtensions = nil
			},
			expectedUri: testExtURI,
			expectedVal: false,
		},
		{
			name: "manually registered server extension",
			setup: func() {
				ServerExtensions = map[string]any{testExtURI: map[string]any{}}
			},
			expectedUri: testExtURI,
			expectedVal: true,
		},
		{
			name: "default supported extension registered after Initialize",
			setup: func() {
				Initialize(nil)
			},
			expectedUri: "com.google.cloud/toolbox.v1",
			expectedVal: true,
		},
		{
			name: "extension disabled after Initialize",
			setup: func() {
				Initialize([]string{"com.google.cloud/toolbox.v1"})
			},
			expectedUri: "com.google.cloud/toolbox.v1",
			expectedVal: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup()
			_, ok := ServerExtensions[tc.expectedUri]
			if ok != tc.expectedVal {
				t.Errorf("ServerExtensions[%q] presence = %v, want %v", tc.expectedUri, ok, tc.expectedVal)
			}
		})
	}
}

func TestGetUiCapability(t *testing.T) {
	tests := []struct {
		name       string
		clientCaps *ClientCapabilities
		wantNil    bool
		wantMimes  []string
		supportsUI bool
	}{
		{
			name:       "nil client capabilities",
			clientCaps: nil,
			wantNil:    true,
			supportsUI: false,
		},
		{
			name:       "empty extensions",
			clientCaps: &ClientCapabilities{},
			wantNil:    true,
			supportsUI: false,
		},
		{
			name: "other extension only",
			clientCaps: &ClientCapabilities{
				Extensions: map[string]any{
					"com.google.cloud/toolbox.v1": map[string]any{},
				},
			},
			wantNil:    true,
			supportsUI: false,
		},
		{
			name: "valid UI extension with correct mimeType",
			clientCaps: &ClientCapabilities{
				Extensions: map[string]any{
					UIExtensionURI: map[string]any{
						"mimeTypes": []any{"text/html;profile=mcp-app"},
					},
				},
			},
			wantNil:    false,
			wantMimes:  []string{"text/html;profile=mcp-app"},
			supportsUI: true,
		},
		{
			name: "valid UI extension with space in mimeType",
			clientCaps: &ClientCapabilities{
				Extensions: map[string]any{
					UIExtensionURI: map[string]any{
						"mimeTypes": []any{"text/html; profile=mcp-app"},
					},
				},
			},
			wantNil:    false,
			wantMimes:  []string{"text/html; profile=mcp-app"},
			supportsUI: true,
		},
		{
			name: "UI extension with unsupported mimeType",
			clientCaps: &ClientCapabilities{
				Extensions: map[string]any{
					UIExtensionURI: map[string]any{
						"mimeTypes": []any{"application/json"},
					},
				},
			},
			wantNil:    false,
			wantMimes:  []string{"application/json"},
			supportsUI: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetUiCapability(tc.clientCaps)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil McpUiClientCapabilities, got %+v", got)
				}
			} else {
				if got == nil {
					t.Fatalf("expected non-nil McpUiClientCapabilities, got nil")
				}
				if !slices.Equal(got.MimeTypes, tc.wantMimes) {
					t.Errorf("expected mimeTypes %v, got %v", tc.wantMimes, got.MimeTypes)
				}
			}
			if ClientSupportsUI(tc.clientCaps) != tc.supportsUI {
				t.Errorf("ClientSupportsUI() = %v, want %v", ClientSupportsUI(tc.clientCaps), tc.supportsUI)
			}
		})
	}
}
