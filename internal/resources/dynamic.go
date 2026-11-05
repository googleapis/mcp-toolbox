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

package resources

import (
	"context"
	"fmt"
	"sync"
)

// DynamicUIResource represents an interactive MCP Apps UI resource that fetches or generates content dynamically.
type DynamicUIResource struct {
	ResourceConfigBase
	ReadFunc func(ctx context.Context) (string, error)
	cspMu    sync.RWMutex
}

var _ Resource = &DynamicUIResource{}

// NewDynamicUIResource constructs a DynamicUIResource configured for MCP Apps UI.
func NewDynamicUIResource(name, uri, description string, csp *CSPConfig, prefersBorder *bool, readFunc func(ctx context.Context) (string, error)) *DynamicUIResource {
	return &DynamicUIResource{
		ResourceConfigBase: ResourceConfigBase{
			ConfigBase: ConfigBase{
				Name:          name,
				Type:          "dynamic_ui",
				Description:   description,
				MimeType:      "text/html;profile=mcp-app",
				UI:            true,
				PrefersBorder: prefersBorder,
				CSP:           csp.Clone(),
			},
			URI: uri,
		},
		ReadFunc: readFunc,
	}
}

// SetCSP atomically replaces the resource's CSP configuration with a deep copy.
func (r *DynamicUIResource) SetCSP(csp *CSPConfig) {
	r.cspMu.Lock()
	r.CSP = csp.Clone()
	r.cspMu.Unlock()
}

// GetCSP returns a thread-safe deep copy of the resource's CSP configuration.
func (r *DynamicUIResource) GetCSP() *CSPConfig {
	r.cspMu.RLock()
	defer r.cspMu.RUnlock()
	return r.CSP.Clone()
}

// GetResourceUIMetadata returns the UI metadata conforming to the MCP Ext-Apps
// specification using a thread-safe snapshot of the dynamic CSP configuration.
func (r *DynamicUIResource) GetResourceUIMetadata() any {
	if !r.UI {
		return nil
	}
	var permMeta map[string]any
	if r.Permissions != nil {
		permMeta = r.Permissions.ToUIMetadata()
	}
	return ResourceUIMetadata{
		CSP:           r.GetCSP(),
		Domain:        r.Domain,
		PrefersBorder: r.PrefersBorder,
		Permissions:   permMeta,
	}
}

// GetSize returns nil for dynamic resources.
func (r *DynamicUIResource) GetSize() *int64 {
	return nil
}

// Read executes the dynamic content generation function.
func (r *DynamicUIResource) Read(ctx context.Context, _ map[string]any) (any, error) {
	if r.ReadFunc == nil {
		return "", fmt.Errorf("dynamic UI resource %q has no ReadFunc configured", r.Name)
	}
	return r.ReadFunc(ctx)
}

// ToConfig returns nil for dynamic resources.
func (r *DynamicUIResource) ToConfig() ResourceConfig {
	return nil
}
