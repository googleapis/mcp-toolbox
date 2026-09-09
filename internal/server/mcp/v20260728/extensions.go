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
	"encoding/json"
	"mime"
	"slices"
)

const (
	// UIExtensionURI is the extension URI for MCP Apps UI support.
	UIExtensionURI = "io.modelcontextprotocol/ui"
	// UIMimeType is the required MIME type for MCP Apps UI resources.
	UIMimeType = "text/html;profile=mcp-app"
)

// SupportedExtensions lists all MCP extension URIs supported by Toolbox by default.
var SupportedExtensions = map[string]any{
	"com.google.cloud/toolbox.v1": map[string]any{},
	UIExtensionURI:                map[string]any{},
}

// ServerExtensions is the map of extension URIs enabled on this server.
var ServerExtensions map[string]any

// Initialize performs version-specific protocol setup for v20260728.
func Initialize(disabledExts []string) {
	ServerExtensions = make(map[string]any)
	for ext, extConfig := range SupportedExtensions {
		if ext != "" && !slices.Contains(disabledExts, ext) {
			ServerExtensions[ext] = extConfig
		}
	}
}

// ParseSupportedExtensions returns a map of extension URIs that are supported by both the client and the server.
func ParseSupportedExtensions(clientExtensions map[string]any) map[string]any {
	supported := make(map[string]any)
	if len(clientExtensions) == 0 || len(ServerExtensions) == 0 {
		return supported
	}
	for uri, clientExtVal := range clientExtensions {
		if _, ok := ServerExtensions[uri]; ok {
			supported[uri] = clientExtVal
		}
	}
	return supported
}

// GetUiCapability extracts McpUiClientCapabilities from client capabilities if present.
// Returns nil if client capabilities are missing or do not contain the UI extension.
func GetUiCapability(clientCaps *ClientCapabilities) *McpUiClientCapabilities {
	if clientCaps == nil || len(clientCaps.Extensions) == 0 {
		return nil
	}
	extVal, ok := clientCaps.Extensions[UIExtensionURI]
	if !ok || extVal == nil {
		return nil
	}
	data, err := json.Marshal(extVal)
	if err != nil {
		return nil
	}
	var uiCaps McpUiClientCapabilities
	if err := json.Unmarshal(data, &uiCaps); err != nil {
		return nil
	}
	return &uiCaps
}

// ClientSupportsUI checks whether client capabilities advertise support for the MCP Apps UI extension.
func ClientSupportsUI(clientCaps *ClientCapabilities) bool {
	uiCaps := GetUiCapability(clientCaps)
	if uiCaps == nil {
		return false
	}
	for _, mt := range uiCaps.MimeTypes {
		if mt == UIMimeType {
			return true
		}
		mediaType, params, err := mime.ParseMediaType(mt)
		if err == nil && mediaType == "text/html" && params["profile"] == "mcp-app" {
			return true
		}
	}
	return false
}
