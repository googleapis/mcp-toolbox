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

package resources

import (
	"fmt"
	"net/url"
	"strings"

	yaml "github.com/goccy/go-yaml"
)

// CSPConfig defines allowed external origins for the UI Resource.
type CSPConfig struct {
	ConnectDomains  []string `yaml:"connectDomains,omitempty" json:"connectDomains,omitempty"`
	ResourceDomains []string `yaml:"resourceDomains,omitempty" json:"resourceDomains,omitempty"`
	FrameDomains    []string `yaml:"frameDomains,omitempty" json:"frameDomains,omitempty"`
	BaseUriDomains  []string `yaml:"baseUriDomains,omitempty" json:"baseUriDomains,omitempty"`
}

// validateHTTPOrigin checks that a string is a valid absolute URI with an http or https scheme and a host.
func validateHTTPOrigin(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("must be a valid absolute URI with scheme and host")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	return nil
}

// Validate validates that every configured domain is a valid origin with a supported scheme.
func (c *CSPConfig) Validate() error {
	if c == nil {
		return nil
	}
	validateList := func(list []string, field string) error {
		for _, d := range list {
			trimmed := strings.TrimSpace(d)
			if trimmed == "" {
				return fmt.Errorf("csp %s cannot contain empty domain", field)
			}
			if err := validateHTTPOrigin(trimmed); err != nil {
				return fmt.Errorf("invalid csp %s origin %q: %w", field, d, err)
			}
		}
		return nil
	}
	if err := validateList(c.ConnectDomains, "connectDomains"); err != nil {
		return err
	}
	if err := validateList(c.ResourceDomains, "resourceDomains"); err != nil {
		return err
	}
	if err := validateList(c.FrameDomains, "frameDomains"); err != nil {
		return err
	}
	if err := validateList(c.BaseUriDomains, "baseUriDomains"); err != nil {
		return err
	}
	return nil
}

// PermissionsConfig defines browser device permissions requested by the UI Resource.
type PermissionsConfig struct {
	Camera         *bool `yaml:"camera,omitempty"`
	Microphone     *bool `yaml:"microphone,omitempty"`
	Geolocation    *bool `yaml:"geolocation,omitempty"`
	ClipboardWrite *bool `yaml:"clipboardWrite,omitempty"`
}

// UnmarshalYAML parses a list of permission names: [camera, microphone, geolocation, clipboardWrite]
// and maps each entry to true on the PermissionsConfig struct.
func (p *PermissionsConfig) UnmarshalYAML(b []byte) error {
	var list []string
	if err := yaml.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("permissions must be a list of permission names (e.g. [camera, microphone]): %w", err)
	}
	seen := make(map[string]bool)
	for _, name := range list {
		if seen[name] {
			return fmt.Errorf("duplicate permission %q is not allowed", name)
		}
		seen[name] = true
		t := true
		switch name {
		case "camera":
			p.Camera = &t
		case "microphone":
			p.Microphone = &t
		case "geolocation":
			p.Geolocation = &t
		case "clipboardWrite":
			p.ClipboardWrite = &t
		default:
			return fmt.Errorf("unknown permission %q: supported permissions are camera, microphone, geolocation, clipboardWrite", name)
		}
	}
	return nil
}

// ToUIMetadata converts PermissionsConfig to _meta.ui.permissions format.
func (p *PermissionsConfig) ToUIMetadata() map[string]any {
	if p == nil {
		return nil
	}
	meta := make(map[string]any)
	add := func(key string, enabled *bool) {
		if enabled != nil && *enabled {
			meta[key] = map[string]any{}
		}
	}
	add("camera", p.Camera)
	add("microphone", p.Microphone)
	add("geolocation", p.Geolocation)
	add("clipboardWrite", p.ClipboardWrite)
	if len(meta) == 0 {
		return nil
	}
	return meta
}

// ResourceUIMetadata represents the metadata for a UI Resource.
type ResourceUIMetadata struct {
	CSP           *CSPConfig     `yaml:"csp,omitempty" json:"csp,omitempty"`
	Permissions   map[string]any `yaml:"permissions,omitempty" json:"permissions,omitempty"`
	Domain        string         `yaml:"domain,omitempty" json:"domain,omitempty"`
	PrefersBorder *bool          `yaml:"prefersBorder,omitempty" json:"prefersBorder,omitempty"`
}
