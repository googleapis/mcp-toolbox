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

package generic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/googleapis/mcp-toolbox/internal/auth"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

const AuthServiceType string = "generic"

const (
	IntrospectionClientIDEnvVar     = "INTROSPECTION_CLIENT_ID"
	IntrospectionClientSecretEnvVar = "INTROSPECTION_CLIENT_SECRET"
)

// validate interface
var _ auth.AuthServiceConfig = Config{}

// Auth service configuration
type Config struct {
	Name                   string   `yaml:"name" validate:"required"`
	Type                   string   `yaml:"type" validate:"required"`
	Audience               string   `yaml:"audience" validate:"required"`
	McpEnabled             bool     `yaml:"mcpEnabled"`
	AuthorizationServer    string   `yaml:"authorizationServer" validate:"required"`
	ScopesRequired         []string `yaml:"scopesRequired"`
	IntrospectionEndpoint  string   `yaml:"introspectionEndpoint"`
	IntrospectionMethod    string   `yaml:"introspectionMethod"`
	IntrospectionParamName string   `yaml:"introspectionParamName"`
	ForceIntrospection     bool     `yaml:"forceIntrospection"`
}

// Returns the auth service type
func (cfg Config) AuthServiceConfigType() string {
	return AuthServiceType
}

func (cfg Config) IsMCPEnabled() bool {
	return cfg.McpEnabled
}

// Initialize a generic auth service
func (cfg Config) Initialize() (auth.AuthService, error) {
	if !cfg.McpEnabled {
		if cfg.IntrospectionEndpoint != "" {
			return nil, fmt.Errorf("`introspectionEndpoint` is not allowed when `mcpEnabled` is false")
		}
		if cfg.IntrospectionMethod != "" {
			return nil, fmt.Errorf("`introspectionMethod` is not allowed when `mcpEnabled` is false")
		}
		if cfg.IntrospectionParamName != "" {
			return nil, fmt.Errorf("`introspectionParamName` is not allowed when `mcpEnabled` is false")
		}
		if len(cfg.ScopesRequired) > 0 {
			return nil, fmt.Errorf("`scopesRequired` is not allowed when `mcpEnabled` is false")
		}
		if cfg.ForceIntrospection {
			return nil, fmt.Errorf("`forceIntrospection` is not allowed when `mcpEnabled` is false")
		}
	}
	httpClient := newSecureHTTPClient()

	// Discover OIDC endpoints
	jwksURL, introspectionURL, issuer, err := discoverOIDCConfig(httpClient, cfg.AuthorizationServer)
	if err != nil {
		return nil, fmt.Errorf("failed to discover OIDC config: %w", err)
	}

	// Override introspection URL if configured
	if cfg.IntrospectionEndpoint != "" {
		introspectionURL = cfg.IntrospectionEndpoint
	}

	var kf keyfunc.Keyfunc
	if !cfg.ForceIntrospection {
		// Create the keyfunc to fetch and cache the JWKS in the background
		kf, err = keyfunc.NewDefault([]string{jwksURL})
		if err != nil {
			return nil, fmt.Errorf("failed to create keyfunc from JWKS URL %s: %w", jwksURL, err)
		}
	}

	introspectionClientID := os.Getenv(IntrospectionClientIDEnvVar)
	introspectionClientSecret := os.Getenv(IntrospectionClientSecretEnvVar)
	if introspectionClientID == "" || introspectionClientSecret == "" {
		log.Printf("WARNING: Introspection client ID or secret not set in environment variables %s and %s. Will not be used until both are set.", IntrospectionClientIDEnvVar, IntrospectionClientSecretEnvVar)
		introspectionClientID = ""
		introspectionClientSecret = ""
	}

	a := &AuthService{
		Config:                    cfg,
		kf:                        kf,
		client:                    httpClient,
		introspectionURL:          introspectionURL,
		issuer:                    issuer,
		introspectionClientID:     introspectionClientID,
		introspectionClientSecret: introspectionClientSecret,
	}
	return a, nil
}

func newSecureHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func discoverOIDCConfig(client *http.Client, AuthorizationServer string) (jwksURI string, introspectionEndpoint string, issuer string, err error) {
	u, err := url.Parse(AuthorizationServer)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid auth URL")
	}
	if u.Scheme != "https" {
		log.Printf("WARNING: HTTP instead of HTTPS is being used for AuthorizationServer: %s", AuthorizationServer)
	}

	oidcConfigURL, err := url.JoinPath(AuthorizationServer, ".well-known/openid-configuration")
	if err != nil {
		return "", "", "", err
	}

	resp, err := client.Get(oidcConfigURL)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch OIDC config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Limit read size to 1MB to prevent memory exhaustion
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", "", err
	}

	var config struct {
		Issuer                string `json:"issuer"`
		JwksUri               string `json:"jwks_uri"`
		IntrospectionEndpoint string `json:"introspection_endpoint"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return "", "", "", err
	}

	if config.Issuer == "" {
		return "", "", "", fmt.Errorf("issuer not found in config")
	}

	if config.JwksUri == "" {
		return "", "", "", fmt.Errorf("jwks_uri not found in config")
	}

	// Sanitize the resulting JWKS URI before returning it
	parsedJWKS, err := url.Parse(config.JwksUri)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid jwks_uri detected")
	}
	if parsedJWKS.Scheme != "https" {
		log.Printf("WARNING: HTTP instead of HTTPS is being used for JWKS URI: %s", config.JwksUri)
	}

	return config.JwksUri, config.IntrospectionEndpoint, config.Issuer, nil
}

var _ auth.MCPAuthService = AuthService{}

// struct used to store auth service info
type AuthService struct {
	Config
	kf                        keyfunc.Keyfunc
	client                    *http.Client
	introspectionURL          string
	issuer                    string
	introspectionClientID     string
	introspectionClientSecret string
}

// Returns the auth service type
func (a AuthService) AuthServiceType() string {
	return AuthServiceType
}

func (a AuthService) ToConfig() auth.AuthServiceConfig {
	return a.Config
}

// Returns the name of the auth service
func (a AuthService) GetName() string {
	return a.Name
}

func (a AuthService) IsMCPEnabled() bool {
	return a.McpEnabled
}

func (a AuthService) GetScopesRequired() []string {
	return a.ScopesRequired
}

func (a AuthService) GetAuthorizationServer() string {
	return a.AuthorizationServer
}

// Verifies generic JWT access token inside the Authorization header
func (a AuthService) GetClaimsFromHeader(ctx context.Context, h http.Header) (map[string]any, error) {
	if a.McpEnabled {
		return nil, nil
	}

	tokenString := h.Get(a.Name + "_token")
	if tokenString == "" {
		return nil, nil
	}

	// Parse and verify the token signature
	token, err := jwt.Parse(tokenString, a.kf.Keyfunc)
	if err != nil {
		return nil, fmt.Errorf("failed to parse and verify JWT token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid JWT token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid JWT claims format")
	}

	// Validate 'aud' (audience) claim
	aud, err := claims.GetAudience()
	if err != nil {
		return nil, fmt.Errorf("could not parse audience from token: %w", err)
	}

	isAudValid := false
	for _, audItem := range aud {
		if audItem == a.Audience {
			isAudValid = true
			break
		}
	}

	if !isAudValid {
		return nil, fmt.Errorf("audience validation failed: expected %s, got %v", a.Audience, aud)
	}

	return claims, nil
}

// MCPAuthError represents an error during MCP authentication validation.
type MCPAuthError = auth.MCPAuthError

// ValidateMCPAuth handles MCP auth token validation
func (a AuthService) ValidateMCPAuth(ctx context.Context, h http.Header) (map[string]any, error) {
	tokenString := h.Get("Authorization")
	if tokenString == "" {
		return nil, &MCPAuthError{Code: http.StatusUnauthorized, Message: "missing access token", ScopesRequired: a.ScopesRequired}
	}

	headerParts := strings.Split(tokenString, " ")
	if len(headerParts) != 2 || strings.ToLower(headerParts[0]) != "bearer" {
		return nil, &MCPAuthError{Code: http.StatusUnauthorized, Message: "authorization header must be in the format 'Bearer <token>'", ScopesRequired: a.ScopesRequired}
	}

	tokenStr := headerParts[1]

	if !a.ForceIntrospection && isJWTFormat(tokenStr) {
		return a.validateJwtToken(ctx, tokenStr)
	}
	return a.validateOpaqueToken(ctx, tokenStr)
}

func isJWTFormat(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var header map[string]any
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return false
	}
	_, hasAlg := header["alg"]
	return hasAlg
}

// validateJwtToken validates a JWT token locally
func (a AuthService) validateJwtToken(ctx context.Context, tokenStr string) (map[string]any, error) {
	token, err := jwt.Parse(tokenStr, a.kf.Keyfunc)
	if err != nil || !token.Valid {
		return nil, &MCPAuthError{Code: http.StatusUnauthorized, Message: "invalid or expired token", ScopesRequired: a.ScopesRequired}
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, &MCPAuthError{Code: http.StatusUnauthorized, Message: "invalid JWT claims format", ScopesRequired: a.ScopesRequired}
	}

	// Validate issuer
	iss, err := claims.GetIssuer()
	if err != nil {
		return nil, &MCPAuthError{Code: http.StatusUnauthorized, Message: "could not parse issuer from token", ScopesRequired: a.ScopesRequired}
	}
	if iss == "" {
		return nil, &MCPAuthError{Code: http.StatusUnauthorized, Message: "missing issuer claim in token", ScopesRequired: a.ScopesRequired}
	}

	// Validate audience
	aud, err := claims.GetAudience()
	if err != nil {
		return nil, &MCPAuthError{Code: http.StatusUnauthorized, Message: "could not parse audience from token", ScopesRequired: a.ScopesRequired}
	}

	scopeClaim, _ := claims["scope"].(string)

	err = a.validateClaims(ctx, iss, aud, scopeClaim)
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// validateOpaqueToken validates an opaque token by calling the introspection endpoint
func (a AuthService) validateOpaqueToken(ctx context.Context, tokenStr string) (map[string]any, error) {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get logger from context: %w", err)
	}

	introspectionURL := a.introspectionURL
	if introspectionURL == "" {
		introspectionURL, err = url.JoinPath(a.AuthorizationServer, "introspect")
		if err != nil {
			return nil, fmt.Errorf("failed to construct introspection URL: %w", err)
		}
	}

	paramName := a.IntrospectionParamName
	if paramName == "" {
		paramName = "token"
	}

	var req *http.Request
	if a.IntrospectionMethod == "GET" {
		u, err := url.Parse(introspectionURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse introspection URL: %w", err)
		}
		q := u.Query()
		q.Set(paramName, tokenStr)
		u.RawQuery = q.Encode()
		req, err = http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create introspection request: %w", err)
		}
	} else {
		data := url.Values{}
		data.Set(paramName, tokenStr)
		req, err = http.NewRequestWithContext(ctx, "POST", introspectionURL, strings.NewReader(data.Encode()))
		if err != nil {
			return nil, fmt.Errorf("failed to create introspection request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Accept", "application/json")

	if a.introspectionClientID != "" {
		req.SetBasicAuth(a.introspectionClientID, a.introspectionClientSecret)
	}

	// Send request to auth server's introspection endpoint
	resp, err := a.client.Do(req)
	if err != nil {
		logger.ErrorContext(ctx, "failed to call introspection endpoint: %v", err)
		return nil, &MCPAuthError{Code: http.StatusInternalServerError, Message: fmt.Sprintf("failed to call introspection endpoint: %v", err), ScopesRequired: a.ScopesRequired}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.WarnContext(ctx, "introspection failed with status: %d", resp.StatusCode)
		return nil, &MCPAuthError{Code: http.StatusUnauthorized, Message: fmt.Sprintf("introspection failed with status: %d", resp.StatusCode), ScopesRequired: a.ScopesRequired}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read introspection response: %w", err)
	}

	var introspectResp struct {
		Active   *bool           `json:"active"`
		Scope    string          `json:"scope"`
		Aud      json.RawMessage `json:"aud"`
		Audience json.RawMessage `json:"audience"`
		Exp      json.Number     `json:"exp"`
		Iss      string          `json:"iss"`
	}

	if err := json.Unmarshal(body, &introspectResp); err != nil {
		return nil, fmt.Errorf("failed to parse introspection response: %w", err)
	}

	if introspectResp.Active == nil || !*introspectResp.Active {
		logger.InfoContext(ctx, "token is not active")
		return nil, &MCPAuthError{Code: http.StatusUnauthorized, Message: "token is not active", ScopesRequired: a.ScopesRequired}
	}

	var expVal int64
	if introspectResp.Exp != "" {
		expVal, err = introspectResp.Exp.Int64()
		if err != nil {
			logger.WarnContext(ctx, "failed to parse exp claim in introspection response: %v", err)
			return nil, &MCPAuthError{Code: http.StatusUnauthorized, Message: "invalid exp claim", ScopesRequired: a.ScopesRequired}
		}
	}

	// Verify expiration (with 1 minute leeway)
	const leeway = 60
	if expVal > 0 && time.Now().Unix() > (expVal+leeway) {
		logger.WarnContext(ctx, "token has expired: exp=%d, now=%d", expVal, time.Now().Unix())
		return nil, &MCPAuthError{Code: http.StatusUnauthorized, Message: "token has expired", ScopesRequired: a.ScopesRequired}
	}

	// Extract audience
	// According to RFC 7662, the aud claim can be a string or an array of strings
	// Fallback to "audience" for Google tokeninfo
	audData := introspectResp.Aud
	if len(audData) == 0 {
		audData = introspectResp.Audience
	}

	var aud []string
	if len(audData) > 0 {
		var audStr string
		var audArr []string
		if err := json.Unmarshal(audData, &audStr); err == nil {
			aud = []string{audStr}
		} else if err := json.Unmarshal(audData, &audArr); err == nil {
			aud = audArr
		} else {
			logger.WarnContext(ctx, "failed to parse aud or audience claim in introspection response")
			return nil, &MCPAuthError{Code: http.StatusUnauthorized, Message: "invalid aud claim", ScopesRequired: a.ScopesRequired}
		}
	}

	err = a.validateClaims(ctx, introspectResp.Iss, aud, introspectResp.Scope)
	if err != nil {
		return nil, err
	}
	claims := map[string]any{
		"active": introspectResp.Active,
		"scope":  introspectResp.Scope,
		"aud":    aud,
		"exp":    expVal,
		"iss":    introspectResp.Iss,
	}
	return claims, nil
}

// validateClaims validates the audience and scopes of a token
func (a AuthService) validateClaims(ctx context.Context, iss string, aud []string, scopeStr string) error {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to get logger from context: %w", err)
	}

	// Validate issuer
	if iss == "" {
		logger.WarnContext(ctx, "issuer validation failed: missing issuer in token")
		return &MCPAuthError{Code: http.StatusUnauthorized, Message: "missing issuer in token validation", ScopesRequired: a.ScopesRequired}
	}
	if iss != a.issuer {
		logger.WarnContext(ctx, "issuer validation failed: expected %s, got %s", a.issuer, iss)
		return &MCPAuthError{Code: http.StatusUnauthorized, Message: "issuer validation failed", ScopesRequired: a.ScopesRequired}
	}

	// Validate audience
	if a.Audience != "" {
		isAudValid := false
		for _, audItem := range aud {
			if audItem == a.Audience {
				isAudValid = true
				break
			}
		}

		if !isAudValid {
			logger.WarnContext(ctx, "audience validation failed: expected %s", a.Audience)
			return &MCPAuthError{Code: http.StatusUnauthorized, Message: "audience validation failed", ScopesRequired: a.ScopesRequired}
		}
	}

	// Check scopes
	if len(a.ScopesRequired) > 0 {
		tokenScopes := strings.Fields(scopeStr)
		scopeMap := make(map[string]bool)
		for _, s := range tokenScopes {
			scopeMap[s] = true
		}

		for _, requiredScope := range a.ScopesRequired {
			if !scopeMap[requiredScope] {
				logger.WarnContext(ctx, "insufficient scopes: missing %s", requiredScope)
				return &MCPAuthError{Code: http.StatusForbidden, Message: "insufficient scopes", ScopesRequired: a.ScopesRequired}
			}
		}
	}

	return nil
}
