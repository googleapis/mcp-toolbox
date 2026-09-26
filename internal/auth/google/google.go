// Copyright 2024 Google LLC
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

package google

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/auth"
	"google.golang.org/api/idtoken"
)

const AuthServiceType string = "google"

// validate interface
var _ auth.AuthServiceConfig = Config{}

// Auth service configuration
type Config struct {
	Name           string   `yaml:"name" validate:"required"`
	Type           string   `yaml:"type" validate:"required"`
	ClientID       string   `yaml:"clientId"`
	Audience       string   `yaml:"audience"`
	McpEnabled     bool     `yaml:"mcpEnabled"`
	ScopesRequired []string `yaml:"scopesRequired"`
}

// Returns the auth service type
func (cfg Config) AuthServiceConfigType() string {
	return AuthServiceType
}

func (cfg Config) IsMCPEnabled() bool {
	return cfg.McpEnabled
}

// Initialize a Google auth service
func (cfg Config) Initialize() (auth.AuthService, error) {
	if cfg.McpEnabled {
		if cfg.Audience == "" && cfg.ClientID == "" {
			return nil, fmt.Errorf("`audience` or `clientId` is required when `mcpEnabled` is true")
		}
	} else {
		if cfg.ClientID == "" {
			return nil, fmt.Errorf("`clientId` is required when `mcpEnabled` is false")
		}
		if cfg.Audience != "" {
			return nil, fmt.Errorf("`audience` is not allowed when `mcpEnabled` is false")
		}
		if len(cfg.ScopesRequired) > 0 {
			return nil, fmt.Errorf("`scopesRequired` is not allowed when `mcpEnabled` is false")
		}
	}
	httpClient := &http.Client{
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
	a := &AuthService{
		Config: cfg,
		client: httpClient,
	}
	return a, nil
}

var _ auth.MCPAuthService = AuthService{}

// struct used to store auth service info
type AuthService struct {
	Config
	client *http.Client
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
	return "https://accounts.google.com"
}

// Verifies Google ID token and return claims
func (a AuthService) GetClaimsFromHeader(ctx context.Context, h http.Header) (map[string]any, error) {
	if token := h.Get(a.Name + "_token"); token != "" {
		payload, err := idtoken.Validate(ctx, token, a.ClientID)
		if err != nil {
			return nil, fmt.Errorf("google ID token verification failure: %w", err)
		}
		return payload.Claims, nil
	}
	return nil, nil
}

// ValidateMCPAuth handles MCP auth token validation for Google
func (a AuthService) ValidateMCPAuth(ctx context.Context, h http.Header) (map[string]any, error) {
	tokenString := h.Get("Authorization")
	if tokenString == "" {
		return nil, &auth.MCPAuthError{Code: http.StatusUnauthorized, Message: "missing access token", ScopesRequired: a.ScopesRequired}
	}

	headerParts := strings.Split(tokenString, " ")
	if len(headerParts) != 2 || strings.ToLower(headerParts[0]) != "bearer" {
		return nil, &auth.MCPAuthError{Code: http.StatusUnauthorized, Message: "authorization header must be in the format 'Bearer <token>'", ScopesRequired: a.ScopesRequired}
	}

	tokenStr := headerParts[1]

	if isJWTFormat(tokenStr) {
		aud := a.Audience
		if aud == "" {
			aud = a.ClientID
		}
		if aud == "" {
			return nil, &auth.MCPAuthError{Code: http.StatusUnauthorized, Message: "audience or client ID is required for ID token validation", ScopesRequired: a.ScopesRequired}
		}
		payload, err := idtoken.Validate(ctx, tokenStr, aud)
		if err != nil {
			return nil, &auth.MCPAuthError{Code: http.StatusUnauthorized, Message: fmt.Sprintf("Google ID token verification failure: %v", err), ScopesRequired: a.ScopesRequired}
		}

		scopeClaim, _ := payload.Claims["scope"].(string)
		if len(a.ScopesRequired) > 0 {
			tokenScopes := strings.Fields(scopeClaim)
			scopeMap := make(map[string]bool)
			for _, s := range tokenScopes {
				scopeMap[s] = true
			}

			for _, requiredScope := range a.ScopesRequired {
				if !scopeMap[requiredScope] {
					return nil, &auth.MCPAuthError{Code: http.StatusForbidden, Message: "insufficient scopes", ScopesRequired: a.ScopesRequired}
				}
			}
		}
		return payload.Claims, nil
	}

	// Validate opaque Google access token via tokeninfo
	data := url.Values{}
	data.Set("access_token", tokenStr)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/tokeninfo", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create Google tokeninfo request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := a.client
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, &auth.MCPAuthError{Code: http.StatusInternalServerError, Message: fmt.Sprintf("failed to call Google tokeninfo: %v", err), ScopesRequired: a.ScopesRequired}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &auth.MCPAuthError{Code: http.StatusUnauthorized, Message: fmt.Sprintf("Google token validation failed with status: %d", resp.StatusCode), ScopesRequired: a.ScopesRequired}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read Google tokeninfo response: %w", err)
	}

	var tokenInfo struct {
		Aud   string `json:"aud"`
		Azp   string `json:"azp"`
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(body, &tokenInfo); err != nil {
		return nil, fmt.Errorf("failed to decode Google tokeninfo response: %w", err)
	}

	aud := tokenInfo.Aud
	if aud == "" {
		aud = tokenInfo.Azp
	}

	audLimit := a.Audience
	if audLimit == "" {
		audLimit = a.ClientID
	}

	if audLimit != "" && aud != audLimit {
		return nil, &auth.MCPAuthError{Code: http.StatusUnauthorized, Message: "audience validation failed", ScopesRequired: a.ScopesRequired}
	}

	if len(a.ScopesRequired) > 0 {
		tokenScopes := strings.Fields(tokenInfo.Scope)
		scopeMap := make(map[string]bool)
		for _, s := range tokenScopes {
			scopeMap[s] = true
		}

		for _, requiredScope := range a.ScopesRequired {
			if !scopeMap[requiredScope] {
				return nil, &auth.MCPAuthError{Code: http.StatusForbidden, Message: "insufficient scopes", ScopesRequired: a.ScopesRequired}
			}
		}
	}

	claims := map[string]any{
		"aud":   aud,
		"scope": tokenInfo.Scope,
	}
	return claims, nil
}

func isJWTFormat(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	return strings.HasPrefix(parts[0], "eyJ")
}
