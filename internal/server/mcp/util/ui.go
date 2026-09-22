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
	"bytes"
	"encoding/json"
	"fmt"
	"mime"

	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

const (
	// UIExtensionURI is the extension URI for MCP Apps UI support.
	UIExtensionURI = "io.modelcontextprotocol/ui"
	// UIMimeType is the required MIME type for MCP Apps UI resources.
	UIMimeType = "text/html;profile=mcp-app"
)

// McpUiClientCapabilities represents MCP Apps capability settings advertised by clients to servers.
type McpUiClientCapabilities struct {
	// Array of supported MIME types for UI resources.
	// Must include "text/html;profile=mcp-app" for MCP Apps support.
	MimeTypes []string `yaml:"mimeTypes,omitempty" json:"mimeTypes,omitempty"`
}

// ValidateUISupport checks whether the capability payload advertises valid MCP Apps UI support.
func ValidateUISupport(cap McpUiClientCapabilities) bool {
	for _, mt := range cap.MimeTypes {
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

// CheckUISupport checks whether the provided extensions map contains valid MCP Apps UI capability.
func CheckUISupport(extensions map[string]any) bool {
	if len(extensions) == 0 {
		return false
	}
	extVal, ok := extensions[UIExtensionURI]
	if !ok || extVal == nil {
		return false
	}
	data, err := json.Marshal(extVal)
	if err != nil {
		return false
	}
	var uiCaps McpUiClientCapabilities
	if err := json.Unmarshal(data, &uiCaps); err != nil {
		return false
	}
	return ValidateUISupport(uiCaps)
}

func checkMetaForUI(meta any) bool {
	if meta == nil {
		return false
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	// Check "io.modelcontextprotocol/clientCapabilities".extensions
	if clientCaps, ok := m["io.modelcontextprotocol/clientCapabilities"].(map[string]any); ok {
		if exts, ok := clientCaps["extensions"].(map[string]any); ok && CheckUISupport(exts) {
			return true
		}
	}
	// Check "clientCapabilities".extensions
	if clientCaps, ok := m["clientCapabilities"].(map[string]any); ok {
		if exts, ok := clientCaps["extensions"].(map[string]any); ok && CheckUISupport(exts) {
			return true
		}
	}
	// Check "extensions"
	if exts, ok := m["extensions"].(map[string]any); ok && CheckUISupport(exts) {
		return true
	}
	// Check direct UIExtensionURI key in meta
	if extVal, ok := m[UIExtensionURI].(map[string]any); ok {
		if CheckUISupport(map[string]any{UIExtensionURI: extVal}) {
			return true
		}
	}
	return false
}

func checkCapsForUI(caps any) bool {
	if caps == nil {
		return false
	}
	data, err := json.Marshal(caps)
	if err != nil {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	if exts, ok := m["extensions"].(map[string]any); ok && CheckUISupport(exts) {
		return true
	}
	return false
}

func checkBodyForUI(body []byte) bool {
	var raw struct {
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(body, &raw); err != nil || raw.Params == nil {
		return false
	}
	if meta, ok := raw.Params["_meta"]; ok && checkMetaForUI(meta) {
		return true
	}
	if caps, ok := raw.Params["capabilities"]; ok && checkCapsForUI(caps) {
		return true
	}
	return false
}

// CheckUISupportFromRequest checks whether the request indicates support for MCP Apps UI.
// It inspects meta, capabilities, and falls back to raw body inspection if needed.
func CheckUISupportFromRequest(meta any, capabilities any, body []byte) bool {
	if checkMetaForUI(meta) {
		return true
	}
	if checkCapsForUI(capabilities) {
		return true
	}
	if len(body) > 0 && bytes.Contains(body, []byte(UIExtensionURI)) {
		if checkBodyForUI(body) {
			return true
		}
	}
	return false
}

// ResolveToolUIMetadata resolves a tool's UI resource and returns the UI metadata map.
func ResolveToolUIMetadata(pMgr *primitives.PrimitiveManager, tool tools.Tool) (map[string]any, error) {
	uiMetaOrig := tool.GetToolUIMetadata()
	if uiMetaOrig == nil || uiMetaOrig.Resource == "" {
		return nil, nil
	}

	var uri string
	if res, hasRes := pMgr.GetResource(uiMetaOrig.Resource); hasRes {
		uri = res.GetURI()
	} else if tmpl, hasTmpl := pMgr.GetResourceTemplate(uiMetaOrig.Resource); hasTmpl {
		uri = tmpl.GetURITemplate()
	} else if res, hasRes := pMgr.GetUIResourceFromURI(uiMetaOrig.Resource); hasRes {
		uri = res.GetURI()
	} else {
		return nil, fmt.Errorf("UI resource %q for tool %q is not registered", uiMetaOrig.Resource, tool.GetName())
	}

	uiMeta := map[string]any{
		"resourceUri": uri,
	}
	if len(uiMetaOrig.Visibility) > 0 {
		vis := make([]string, len(uiMetaOrig.Visibility))
		for i, v := range uiMetaOrig.Visibility {
			vis[i] = string(v)
		}
		uiMeta["visibility"] = vis
	}
	return uiMeta, nil
}
