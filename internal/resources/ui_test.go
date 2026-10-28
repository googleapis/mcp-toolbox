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

package resources_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/resources"
)

func TestUIConfig_YAML(t *testing.T) {
	yamlStr := `
name: ui-res
type: mock
ui: true
domain: "https://example.com"
prefersBorder: true
csp:
  connectDomains:
    - https://api.example.com
    - http://localhost:3000
  resourceDomains:
    - https://cdn.example.com
    - https://*.example.com
  frameDomains:
    - https://embed.example.com
  baseUriDomains:
    - https://base.example.com
permissions:
  - camera
  - microphone
  - clipboardWrite
`
	var cfg resources.ConfigBase
	if err := yaml.Unmarshal([]byte(yamlStr), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal ConfigBase: %v", err)
	}

	if !cfg.UI {
		t.Errorf("Expected UI to be true")
	}

	if cfg.Domain != "https://example.com" {
		t.Errorf("Expected Domain 'https://example.com', got %v", cfg.Domain)
	}

	if cfg.PrefersBorder == nil || !*cfg.PrefersBorder {
		t.Errorf("Expected PrefersBorder to be true")
	}

	csp := cfg.CSP
	if csp == nil {
		t.Fatalf("Expected CSP config to be populated")
	}
	if len(csp.ConnectDomains) != 2 || csp.ConnectDomains[0] != "https://api.example.com" {
		t.Errorf("Unexpected ConnectDomains: %v", csp.ConnectDomains)
	}
	if len(csp.ResourceDomains) != 2 || csp.ResourceDomains[0] != "https://cdn.example.com" {
		t.Errorf("Unexpected ResourceDomains: %v", csp.ResourceDomains)
	}
	if len(csp.FrameDomains) != 1 || csp.FrameDomains[0] != "https://embed.example.com" {
		t.Errorf("Unexpected FrameDomains: %v", csp.FrameDomains)
	}
	if len(csp.BaseUriDomains) != 1 || csp.BaseUriDomains[0] != "https://base.example.com" {
		t.Errorf("Unexpected BaseUriDomains: %v", csp.BaseUriDomains)
	}

	perms := cfg.Permissions
	if perms == nil || perms.Camera == nil || !*perms.Camera || perms.Microphone == nil || !*perms.Microphone || perms.ClipboardWrite == nil || !*perms.ClipboardWrite {
		t.Errorf("Expected camera, microphone, and clipboardWrite to be true, got %+v", perms)
	}

	meta := cfg.GetResourceUIMetadata()
	if meta == nil {
		t.Fatalf("Expected GetResourceUIMetadata() to not be nil")
	}

	tVal := true
	expectedMeta := resources.ResourceUIMetadata{
		Domain:        "https://example.com",
		PrefersBorder: &tVal,
		CSP: &resources.CSPConfig{
			ConnectDomains:  []string{"https://api.example.com", "http://localhost:3000"},
			ResourceDomains: []string{"https://cdn.example.com", "https://*.example.com"},
			FrameDomains:    []string{"https://embed.example.com"},
			BaseUriDomains:  []string{"https://base.example.com"},
		},
		Permissions: map[string]any{
			"camera":         map[string]any{},
			"microphone":     map[string]any{},
			"clipboardWrite": map[string]any{},
		},
	}

	if diff := cmp.Diff(expectedMeta, meta); diff != "" {
		t.Errorf("GetResourceUIMetadata() mismatch (-want +got):\n%s", diff)
	}
}

func TestUIConfig_Validation(t *testing.T) {
	tests := []struct {
		name      string
		cfg       resources.ConfigBase
		errSubstr string
	}{
		// UIFalse tests
		{
			name: "CSPWithoutUIRejected",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: false,
				CSP: &resources.CSPConfig{ConnectDomains: []string{"https://example.com"}},
			},
			errSubstr: "csp cannot be configured when 'ui' is false or omitted",
		},
		{
			name: "PermissionsWithoutUIRejected",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: false,
				Permissions: &resources.PermissionsConfig{Camera: func(b bool) *bool { return &b }(true)},
			},
			errSubstr: "permissions cannot be configured when 'ui' is false or omitted",
		},
		// CSP Validation tests
		{
			name: "ValidCSP",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: true,
				CSP: &resources.CSPConfig{
					ConnectDomains:  []string{"https://api.example.com", "http://localhost:3000"},
					ResourceDomains: []string{"https://cdn.example.com", "http://localhost:8080", "https://*.cloudflare.com"},
					FrameDomains:    []string{"https://frame.example.com"},
				},
			},
			errSubstr: "",
		},
		{
			name: "InvalidURLInConnectDomains",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: true,
				CSP: &resources.CSPConfig{ConnectDomains: []string{"://invalid-url"}},
			},
			errSubstr: "invalid csp",
		},
		{
			name: "InvalidURLInResourceDomains",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: true,
				CSP: &resources.CSPConfig{ResourceDomains: []string{"invalid url with spaces"}},
			},
			errSubstr: "invalid csp",
		},
		{
			name: "InvalidURLInFrameDomains",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: true,
				CSP: &resources.CSPConfig{FrameDomains: []string{"://invalid-frame"}},
			},
			errSubstr: "invalid csp",
		},
		{
			name: "InvalidURLInBaseUriDomains",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: true,
				CSP: &resources.CSPConfig{BaseUriDomains: []string{"://invalid-base-uri"}},
			},
			errSubstr: "invalid csp",
		},
		{
			name: "UnsupportedSchemeInDomains",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: true,
				CSP: &resources.CSPConfig{ConnectDomains: []string{"ftp://ftp.example.com"}},
			},
			errSubstr: "scheme must be",
		},
		{
			name: "EmptyDomainInList",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: true,
				CSP: &resources.CSPConfig{ConnectDomains: []string{"   "}},
			},
			errSubstr: "cannot contain empty domain",
		},
		// Domain validation tests
		{
			name: "ValidDomainHTTP",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: true,
				Domain: "http://localhost:8080",
			},
			errSubstr: "",
		},
		{
			name: "ValidDomainHTTPS",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: true,
				Domain: "https://example.com",
			},
			errSubstr: "",
		},
		{
			name: "InvalidDomainNoScheme",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: true,
				Domain: "example.com",
			},
			errSubstr: "must be a valid absolute URI with scheme and host",
		},
		{
			name: "InvalidDomainUnsupportedScheme",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: true,
				Domain: "ftp://example.com",
			},
			errSubstr: "scheme must be http or https",
		},
		{
			name: "InvalidDomainMalformed",
			cfg: resources.ConfigBase{
				Name: "test", Type: "mock", UI: true,
				Domain: "://invalid-domain",
			},
			errSubstr: "must be a valid absolute URI with scheme and host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.errSubstr == "" {
				if err != nil {
					t.Errorf("Expected valid config, got %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Expected error containing %q, got %v", tt.errSubstr, err)
				}
			}
		})
	}
}

func TestUIConfig_URISchemeValidation(t *testing.T) {
	t.Run("ResourceConfigBase", func(t *testing.T) {
		tests := []struct {
			name      string
			cfg       resources.ResourceConfigBase
			errSubstr string
			checkFunc func(*testing.T, *resources.ResourceConfigBase)
		}{
			{
				name: "UIEnabledValidScheme",
				cfg: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{Name: "app", Type: "mock", UI: true},
					URI:        "ui://app",
				},
			},

			{
				name: "UIEnabledInvalidScheme",
				cfg: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{Name: "app", Type: "mock", UI: true},
					URI:        "file://app",
				},
				errSubstr: "must be 'ui'",
			},
			{
				name: "UIDisabledUISchemeRejected",
				cfg: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{Name: "app", Type: "mock", UI: false},
					URI:        "ui://app",
				},
				errSubstr: "scheme 'ui' is only permitted when 'ui' is true",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := tt.cfg.Validate()
				if tt.errSubstr == "" {
					if err != nil {
						t.Errorf("Expected no error, got %v", err)
					}
					if tt.checkFunc != nil {
						tt.checkFunc(t, &tt.cfg)
					}
				} else {
					if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
						t.Errorf("Expected error containing %q, got %v", tt.errSubstr, err)
					}
				}
			})
		}
	})

	t.Run("ResourceTemplateConfigBase", func(t *testing.T) {
		tests := []struct {
			name      string
			cfg       resources.ResourceTemplateConfigBase
			errSubstr string
			checkFunc func(*testing.T, *resources.ResourceTemplateConfigBase)
		}{
			{
				name: "UIEnabledValidTemplate",
				cfg: resources.ResourceTemplateConfigBase{
					ConfigBase:  resources.ConfigBase{Name: "app-tmpl", Type: "mock", UI: true},
					URITemplate: "ui://app-tmpl/{path}",
				},
			},

			{
				name: "UIEnabledInvalidScheme",
				cfg: resources.ResourceTemplateConfigBase{
					ConfigBase:  resources.ConfigBase{Name: "app-tmpl", Type: "mock", UI: true},
					URITemplate: "file://app-tmpl/{path}",
				},
				errSubstr: "must be 'ui'",
			},
			{
				name: "UIDisabledUISchemeRejected",
				cfg: resources.ResourceTemplateConfigBase{
					ConfigBase:  resources.ConfigBase{Name: "app-tmpl", Type: "mock", UI: false},
					URITemplate: "ui://app-tmpl/{path}",
				},
				errSubstr: "scheme 'ui' is only permitted when 'ui' is true",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := tt.cfg.Validate()
				if tt.errSubstr == "" {
					if err != nil {
						t.Errorf("Expected no error, got %v", err)
					}
					if tt.checkFunc != nil {
						tt.checkFunc(t, &tt.cfg)
					}
				} else {
					if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
						t.Errorf("Expected error containing %q, got %v", tt.errSubstr, err)
					}
				}
			})
		}
	})
}

func TestPermissions_Validation(t *testing.T) {
	tests := []struct {
		name      string
		yamlStr   string
		errSubstr string
	}{
		{
			name:    "ValidPermissionsListMappedToStruct",
			yamlStr: `permissions: ["camera", "clipboardWrite"]`,
		},
		{
			name:      "UnknownPermissionRejected",
			yamlStr:   `permissions: ["camera", "unknownPerm"]`,
			errSubstr: "unknown permission",
		},
		{
			name:      "DuplicatePermissionRejected",
			yamlStr:   `permissions: ["camera", "camera"]`,
			errSubstr: "duplicate permission",
		},
		{
			name:      "NonListFormatRejected",
			yamlStr:   `permissions: 123`,
			errSubstr: "must be a list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var holder struct {
				Permissions resources.PermissionsConfig `yaml:"permissions"`
			}
			err := yaml.Unmarshal([]byte(tt.yamlStr), &holder)
			if tt.errSubstr == "" {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
				if tt.name == "ValidPermissionsListMappedToStruct" {
					perms := holder.Permissions
					if perms.Camera == nil || !*perms.Camera || perms.ClipboardWrite == nil || !*perms.ClipboardWrite {
						t.Errorf("Expected camera and clipboardWrite to be true")
					}
					if perms.Microphone != nil {
						t.Errorf("Expected microphone to be nil")
					}
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Expected error containing %q, got %v", tt.errSubstr, err)
				}
			}
		})
	}
}

func TestGetResourceUIMetadata_OmittedFields(t *testing.T) {
	tests := []struct {
		name      string
		cfg       resources.ConfigBase
		checkFunc func(*testing.T, any)
	}{
		{
			name: "UIFalseReturnsNil",
			cfg:  resources.ConfigBase{Name: "test", Type: "mock", UI: false},
			checkFunc: func(t *testing.T, meta any) {
				if meta != nil {
					t.Errorf("Expected nil for UI: false, got %v", meta)
				}
			},
		},
		{
			name: "UITrueEmptyMeta",
			cfg:  resources.ConfigBase{Name: "test", Type: "mock", UI: true},
			checkFunc: func(t *testing.T, meta any) {
				if meta == nil {
					t.Fatalf("Expected non-nil map for UI: true")
				}
				if !reflect.DeepEqual(meta, resources.ResourceUIMetadata{}) {
					t.Errorf("Expected empty struct when no optional fields are set, got %v", meta)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := tt.cfg.GetResourceUIMetadata()
			tt.checkFunc(t, meta)
		})
	}
}

func TestUIResource_MimeTypeValidation(t *testing.T) {
	tests := []struct {
		name      string
		cfg       resources.ConfigBase
		wantError string
	}{
		{
			name: "default mimeType when omitted",
			cfg: resources.ConfigBase{
				Name: "test-res",
				UI:   true,
			},
		},
		{
			name: "explicit valid mimeType",
			cfg: resources.ConfigBase{
				Name:     "test-res",
				UI:       true,
				MimeType: "text/html;profile=mcp-app",
			},
		},
		{
			name: "explicit valid mimeType with space",
			cfg: resources.ConfigBase{
				Name:     "test-res",
				UI:       true,
				MimeType: "text/html; profile=mcp-app",
			},
		},
		{
			name: "invalid mimeType for UI resource",
			cfg: resources.ConfigBase{
				Name:     "test-res",
				UI:       true,
				MimeType: "text/plain",
			},
			wantError: `invalid mimeType "text/plain" for UI resource "test-res": must be "text/html;profile=mcp-app"`,
		},
		{
			name: "invalid mimeType application/json for UI resource",
			cfg: resources.ConfigBase{
				Name:     "test-res",
				UI:       true,
				MimeType: "application/json",
			},
			wantError: `invalid mimeType "application/json" for UI resource "test-res": must be "text/html;profile=mcp-app"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cfg.SetDefaults()
			err := tt.cfg.Validate()
			if tt.wantError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantError)
				}
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected error %q, got %q", tt.wantError, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tt.cfg.MimeType != resources.UIMimeType {
					t.Errorf("expected MimeType %q, got %q", resources.UIMimeType, tt.cfg.MimeType)
				}
			}
		})
	}
}
