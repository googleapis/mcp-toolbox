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

import "slices"

// SupportedExtensions lists all MCP extension URIs supported by Toolbox by default.
var SupportedExtensions = map[string]any{
	"com.google.cloud/toolbox.v1": map[string]any{},
	"io.modelcontextprotocol/ui": map[string]any{},
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
