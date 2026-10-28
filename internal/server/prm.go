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
	"fmt"
	"net/url"
	"strings"
)

// ProtectedResourceMetadata represents the OAuth 2.0 Protected Resource Metadata document as defined in RFC 9728.
// Reference: https://datatracker.ietf.org/doc/html/rfc9728
type ProtectedResourceMetadata struct {
	// REQUIRED. The protected resource's resource identifier (a URL using the https scheme).
	Resource string `json:"resource"`

	// REQUIRED. Array containing a list of OAuth authorization server issuer identifiers.
	AuthorizationServers []string `json:"authorization_servers,omitempty"`

	// OPTIONAL. URL of the protected resource's JSON Web Key (JWK) Set document.
	JWKSURI string `json:"jwks_uri,omitempty"`

	// RECOMMENDED. Array containing a list of scope values used to request access.
	ScopesSupported []string `json:"scopes_supported,omitempty"`

	// OPTIONAL. Array containing a list of the supported methods of sending an
	// OAuth 2.0 bearer token (e.g., "header", "body", "query").
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`

	// OPTIONAL. Array containing a list of the JWS signing algorithms (alg values)
	// supported by the protected resource for signing resource responses.
	ResourceSigningAlgValuesSupported []string `json:"resource_signing_alg_values_supported,omitempty"`

	// RECOMMENDED. Human-readable name of the protected resource intended for display.
	ResourceName string `json:"resource_name,omitempty"`

	// OPTIONAL. URL of a page containing human-readable developer documentation.
	ResourceDocumentation string `json:"resource_documentation,omitempty"`

	// OPTIONAL. URL of a page containing human-readable policy requirements.
	ResourcePolicyURI string `json:"resource_policy_uri,omitempty"`

	// OPTIONAL. URL of a page containing human-readable terms of service.
	ResourceTOSURI string `json:"resource_tos_uri,omitempty"`

	// OPTIONAL. Boolean indicating support for mutual-TLS client certificate-bound
	// access tokens. If omitted, the default is false.
	TLSClientCertificateBoundAccessTokens *bool `json:"tls_client_certificate_bound_access_tokens,omitempty"`

	// OPTIONAL. Array containing a list of authorization details type values supported.
	AuthorizationDetailsTypesSupported []string `json:"authorization_details_types_supported,omitempty"`

	// OPTIONAL. Array containing a list of JWS alg values supported for DPoP proof JWTs.
	DPoPSigningAlgValuesSupported []string `json:"dpop_signing_alg_values_supported,omitempty"`

	// OPTIONAL. Boolean specifying whether the protected resource always requires
	// the use of DPoP-bound access tokens. If omitted, the default is false.
	DPoPBoundAccessTokensRequired *bool `json:"dpop_bound_access_tokens_required,omitempty"`

	// OPTIONAL. A JWT containing metadata parameters about the protected resource
	// as claims. Consists of the entire signed JWT string.
	SignedMetadata string `json:"signed_metadata,omitempty"`
}

// getPRMURL returns the full URL to advertise in WWW-Authenticate headers.
func (s *Server) getPRMURL() string {
	return s.prmURL
}

// parsePRMURL returns the full URL to advertise in WWW-Authenticate headers from a toolbox URL.
func parsePRMURL(toolboxUrl string) (string, error) {
	if toolboxUrl == "" {
		return "/.well-known/oauth-protected-resource", nil
	}
	u, err := url.Parse(toolboxUrl)
	if err != nil {
		return "", fmt.Errorf("invalid toolbox-url %q: %w", toolboxUrl, err)
	}
	if u.Scheme != "" || u.Host != "" {
		if u.Scheme == "" || u.Host == "" {
			return "", fmt.Errorf("invalid toolbox-url %q: must be a valid absolute URL with scheme and host", toolboxUrl)
		}
		path := strings.TrimSuffix(u.Path, "/")
		return u.Scheme + "://" + u.Host + "/.well-known/oauth-protected-resource" + path, nil
	}
	path := strings.TrimSuffix(u.Path, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if path == "/" {
		return "/.well-known/oauth-protected-resource", nil
	}
	return "/.well-known/oauth-protected-resource" + path, nil
}
