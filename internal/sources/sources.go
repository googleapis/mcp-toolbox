// Copyright 2024 Google LLC
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

package sources

import (
	"context"

	"fmt"

	"github.com/goccy/go-yaml"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CloudPlatformScope is the OAuth2 scope for Google Cloud Platform services.
const CloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

// SourceConfigFactory defines the function signature for creating a SourceConfig.
type SourceConfigFactory func(ctx context.Context, name string, decoder *yaml.Decoder) (SourceConfig, error)

var sourceRegistry = make(map[string]SourceConfigFactory)

// Register registers a new source type with its factory.
// It returns false if the type is already registered.
func Register(sourceType string, factory SourceConfigFactory) bool {
	if _, exists := sourceRegistry[sourceType]; exists {
		// Source with this type already exists, do not overwrite.
		return false
	}
	sourceRegistry[sourceType] = factory
	return true
}

// DecodeConfig decodes a source configuration using the registered factory for the given type.
func DecodeConfig(ctx context.Context, sourceType string, name string, decoder *yaml.Decoder) (SourceConfig, error) {
	factory, found := sourceRegistry[sourceType]
	if !found {
		return nil, fmt.Errorf("unknown source type: %q", sourceType)
	}
	sourceConfig, err := factory(ctx, name, decoder)
	if err != nil {
		return nil, fmt.Errorf("unable to parse source %q as %q: %w", name, sourceType, err)
	}
	return sourceConfig, err
}

// SourceConfig is the interface for configuring a source.
type SourceConfig interface {
	SourceConfigType() string
	Initialize(ctx context.Context, tracer trace.Tracer) (Source, error)
}

// Source is the interface for the source itself.
type Source interface {
	SourceType() string
	ToConfig() SourceConfig
	IsReadOnly() bool
}

// OAuthProxySource is implemented by a Source whose OAuth authorization
// server does not send CORS headers on its discovery document or token
// endpoint. Browsers enforce CORS on both fetch/XHR requests, so a
// browser-based MCP client cannot complete an OAuth Authorization Code +
// PKCE flow directly against such a server, even though the same requests
// work fine from a non-browser HTTP client (curl, a backend service, etc).
//
// A Source that implements this interface, and reports
// ClientOAuthEnabled() == true, has its OAuth discovery document and token
// endpoint proxied by the toolbox itself (see
// internal/server/oauth_proxy.go), at:
//
//	GET  /.well-known/oauth-authorization-server/oauth-proxy/{sourceName}
//	POST /oauth-proxy/{sourceName}/token
//
// The browser then only ever talks to the toolbox's own origin, which
// already sends the CORS headers configured via --allowed-origins, and the
// toolbox forwards the token exchange to the authorization server
// server-side, where CORS does not apply.
//
// A source supplies its own full endpoint URLs rather than a bare base URL
// plus an assumed path suffix, because different authorization servers use
// different conventions and the toolbox should not need to guess any
// source's URL layout.
type OAuthProxySource interface {
	Source
	// ClientOAuthEnabled reports whether this source instance expects the
	// caller to supply a per-request, browser-obtained OAuth access token,
	// as opposed to a single credential configured for the whole process.
	ClientOAuthEnabled() bool
	// OAuthAuthorizationEndpoint returns the full URL of the authorization
	// server's authorization endpoint (where the browser is redirected to
	// authenticate). This is returned as-is in the proxied discovery
	// document: the initial redirect happens via top-level browser
	// navigation, which isn't subject to CORS, so it does not need to be
	// proxied.
	OAuthAuthorizationEndpoint() string
	// OAuthTokenEndpoint returns the full URL of the authorization server's
	// own token endpoint. The toolbox's token proxy forwards requests here
	// server-side; browser clients never call this URL directly.
	OAuthTokenEndpoint() string
}

// OAuthProxyCapabilities can optionally be implemented by an
// OAuthProxySource to override the default OAuth capabilities advertised in
// its proxied discovery document (response_types_supported: ["code"],
// grant_types_supported: ["authorization_code", "refresh_token"],
// code_challenge_methods_supported: ["S256"]). Most authorization servers
// matching OAuthProxySource use these defaults and don't need to implement
// this.
type OAuthProxyCapabilities interface {
	OAuthResponseTypesSupported() []string
	OAuthGrantTypesSupported() []string
	OAuthCodeChallengeMethodsSupported() []string
}

// InitConnectionSpan adds a span for database pool connection initialization
func InitConnectionSpan(ctx context.Context, tracer trace.Tracer, sourceType, sourceName string) (context.Context, trace.Span) {
	ctx, span := tracer.Start(
		ctx,
		"toolbox/server/source/connect",
		trace.WithAttributes(attribute.String("source_type", sourceType)),
		trace.WithAttributes(attribute.String("source_name", sourceName)),
	)
	return ctx, span
}
