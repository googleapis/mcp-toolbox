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
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

func generateRSAPrivateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to create RSA private key: %v", err)
	}
	return key
}

func setupJWKSMockServer(t *testing.T, key *rsa.PrivateKey, keyID string) *httptest.Server {
	t.Helper()

	jwksHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"issuer":   "https://example.com",
				"jwks_uri": "http://" + r.Host + "/jwks",
			})
			return
		}

		if r.URL.Path == "/jwks" {
			options := jwkset.JWKOptions{
				Metadata: jwkset.JWKMetadataOptions{
					KID: keyID,
				},
			}
			jwk, err := jwkset.NewJWKFromKey(key.Public(), options)
			if err != nil {
				t.Fatalf("failed to create JWK: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"keys": []jwkset.JWKMarshal{jwk.Marshal()},
			})
			return
		}

		http.NotFound(w, r)
	})

	return httptest.NewServer(jwksHandler)
}

func generateValidToken(t *testing.T, key *rsa.PrivateKey, keyID string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	signedString, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signedString
}

func TestInitialize_Validation(t *testing.T) {
	key := generateRSAPrivateKey(t)
	mockOIDC := setupJWKSMockServer(t, key, "my-key-id")
	defer mockOIDC.Close()

	tests := []struct {
		name      string
		config    Config
		wantError bool
		errString string
	}{
		{
			name: "valid mcpEnabled true",
			config: Config{
				Name:                "generic-auth",
				Type:                "generic",
				Audience:            "my-audience",
				AuthorizationServer: mockOIDC.URL,
				McpEnabled:          true,
				ScopesRequired:      []string{"email"},
			},
			wantError: false,
		},
		{
			name: "valid mcpEnabled false",
			config: Config{
				Name:                "generic-auth",
				Type:                "generic",
				Audience:            "my-audience",
				AuthorizationServer: mockOIDC.URL,
				McpEnabled:          false,
			},
			wantError: false,
		},
		{
			name: "introspectionEndpoint disallowed with mcpEnabled false",
			config: Config{
				Name:                  "generic-auth",
				Type:                  "generic",
				Audience:              "my-audience",
				AuthorizationServer:   mockOIDC.URL,
				McpEnabled:            false,
				IntrospectionEndpoint: "http://example.com/introspect",
			},
			wantError: true,
			errString: "`introspectionEndpoint` is not allowed when `mcpEnabled` is false",
		},
		{
			name: "introspectionMethod disallowed with mcpEnabled false",
			config: Config{
				Name:                "generic-auth",
				Type:                "generic",
				Audience:            "my-audience",
				AuthorizationServer: mockOIDC.URL,
				McpEnabled:          false,
				IntrospectionMethod: "POST",
			},
			wantError: true,
			errString: "`introspectionMethod` is not allowed when `mcpEnabled` is false",
		},
		{
			name: "introspectionParamName disallowed with mcpEnabled false",
			config: Config{
				Name:                   "generic-auth",
				Type:                   "generic",
				Audience:               "my-audience",
				AuthorizationServer:    mockOIDC.URL,
				McpEnabled:             false,
				IntrospectionParamName: "token",
			},
			wantError: true,
			errString: "`introspectionParamName` is not allowed when `mcpEnabled` is false",
		},
		{
			name: "scopesRequired disallowed with mcpEnabled false",
			config: Config{
				Name:                "generic-auth",
				Type:                "generic",
				Audience:            "my-audience",
				AuthorizationServer: mockOIDC.URL,
				McpEnabled:          false,
				ScopesRequired:      []string{"email"},
			},
			wantError: true,
			errString: "`scopesRequired` is not allowed when `mcpEnabled` is false",
		},
		{
			name: "forceIntrospection disallowed with mcpEnabled false",
			config: Config{
				Name:                "generic-auth",
				Type:                "generic",
				Audience:            "my-audience",
				AuthorizationServer: mockOIDC.URL,
				McpEnabled:          false,
				ForceIntrospection:  true,
			},
			wantError: true,
			errString: "`forceIntrospection` is not allowed when `mcpEnabled` is false",
		},
		{
			name: "forceIntrospection allowed with mcpEnabled true",
			config: Config{
				Name:                "generic-auth",
				Type:                "generic",
				Audience:            "my-audience",
				AuthorizationServer: mockOIDC.URL,
				McpEnabled:          true,
				ForceIntrospection:  true,
			},
			wantError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.config.Initialize()
			if (err != nil) != tc.wantError {
				t.Fatalf("Initialize() returned error: %v, wantError: %v", err, tc.wantError)
			}
			if tc.wantError && err != nil && !strings.Contains(err.Error(), tc.errString) {
				t.Errorf("expected error containing %q, got %v", tc.errString, err)
			}
		})
	}
}

func TestGetClaimsFromHeader(t *testing.T) {
	privateKey := generateRSAPrivateKey(t)
	keyID := "test-key-id"
	server := setupJWKSMockServer(t, privateKey, keyID)
	defer server.Close()

	cfg := Config{
		Name:                "test-generic-auth",
		Type:                "generic",
		Audience:            "my-audience",
		McpEnabled:          false,
		AuthorizationServer: server.URL,
	}

	authService, err := cfg.Initialize()
	if err != nil {
		t.Fatalf("failed to initialize auth service: %v", err)
	}

	genericAuth, ok := authService.(*AuthService)
	if !ok {
		t.Fatalf("expected *AuthService, got %T", authService)
	}

	ctx := context.Background()

	tests := []struct {
		name        string
		setupHeader func() http.Header
		wantError   bool
		errContains string
		validate    func(claims map[string]any)
	}{
		{
			name: "valid token",
			setupHeader: func() http.Header {
				token := generateValidToken(t, privateKey, keyID, jwt.MapClaims{
					"iss":   "https://example.com",
					"aud":   "my-audience",
					"scope": "read:files write:files",
					"sub":   "test-user",
					"exp":   time.Now().Add(time.Hour).Unix(),
				})
				header := http.Header{}
				header.Set("test-generic-auth_token", token)
				return header
			},
			wantError: false,
			validate: func(claims map[string]any) {
				if sub, ok := claims["sub"].(string); !ok || sub != "test-user" {
					t.Errorf("expected sub=test-user, got %v", claims["sub"])
				}
			},
		},
		{
			name: "no header",
			setupHeader: func() http.Header {
				return http.Header{}
			},
			wantError: false,
			validate: func(claims map[string]any) {
				if claims != nil {
					t.Errorf("expected nil claims on missing header, got %v", claims)
				}
			},
		},
		{
			name: "wrong audience",
			setupHeader: func() http.Header {
				token := generateValidToken(t, privateKey, keyID, jwt.MapClaims{
					"iss":   "https://example.com",
					"aud":   "wrong-audience",
					"scope": "read:files",
					"exp":   time.Now().Add(time.Hour).Unix(),
				})
				header := http.Header{}
				header.Set("test-generic-auth_token", token)
				return header
			},
			wantError:   true,
			errContains: "audience validation failed",
		},
		{
			name: "expired token",
			setupHeader: func() http.Header {
				token := generateValidToken(t, privateKey, keyID, jwt.MapClaims{
					"iss":   "https://example.com",
					"aud":   "my-audience",
					"scope": "read:files",
					"exp":   time.Now().Add(-1 * time.Hour).Unix(),
				})
				header := http.Header{}
				header.Set("test-generic-auth_token", token)
				return header
			},
			wantError:   true,
			errContains: "token has invalid claims: token is expired", // Custom JWT err string
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			header := tc.setupHeader()
			claims, err := genericAuth.GetClaimsFromHeader(ctx, header)

			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing %q, got: %v", tc.errContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tc.validate != nil {
					tc.validate(claims)
				}
			}
		})
	}
}

func TestValidateMCPAuth_Opaque(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		scopesRequired []string
		audience       string
		mockOidcConfig map[string]any
		mockResponse   map[string]any
		mockStatus     int
		wantError      bool
		errContains    string
	}{
		{
			name:           "valid opaque token",
			token:          "opaque-valid",
			scopesRequired: []string{"read:files"},
			audience:       "my-audience",
			mockResponse: map[string]any{
				"active": true,
				"scope":  "read:files write:files",
				"aud":    "my-audience",
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			},
			mockStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:           "valid opaque token with string exp",
			token:          "opaque-valid-string-exp",
			scopesRequired: []string{"read:files"},
			audience:       "my-audience",
			mockResponse: map[string]any{
				"active": true,
				"scope":  "read:files write:files",
				"aud":    "my-audience",
				"exp":    "2000000000",
				"iss":    "https://example.com",
			},
			mockStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:           "valid opaque token with custom introspection endpoint",
			token:          "opaque-valid-custom",
			scopesRequired: []string{"read:files"},
			audience:       "my-audience",
			mockOidcConfig: map[string]any{
				"introspection_endpoint": "http://SERVER_HOST/custom-introspect",
			},
			mockResponse: map[string]any{
				"active": true,
				"scope":  "read:files",
				"aud":    "my-audience",
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			},
			mockStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:           "valid opaque token with array aud",
			token:          "opaque-valid-array-aud",
			scopesRequired: []string{"read:files"},
			audience:       "my-audience",
			mockResponse: map[string]any{
				"active": true,
				"scope":  "read:files",
				"aud":    []string{"other-audience", "my-audience"},
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			},
			mockStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:           "inactive opaque token",
			token:          "opaque-inactive",
			scopesRequired: []string{"read:files"},
			mockResponse: map[string]any{
				"active": false,
			},
			mockStatus:  http.StatusOK,
			wantError:   true,
			errContains: "token is not active",
		},
		{
			name:           "missing active claim",
			token:          "opaque-missing-active",
			scopesRequired: []string{"read:files"},
			mockResponse: map[string]any{
				"scope": "read:files",
			},
			mockStatus:  http.StatusOK,
			wantError:   true,
			errContains: "token is not active",
		},
		{
			name:           "insufficient scopes",
			token:          "opaque-bad-scope",
			scopesRequired: []string{"read:files", "write:files"},
			mockResponse: map[string]any{
				"active": true,
				"scope":  "read:files",
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			},
			mockStatus:  http.StatusOK,
			wantError:   true,
			errContains: "insufficient scopes",
		},
		{
			name:     "audience mismatch",
			token:    "opaque-bad-aud",
			audience: "my-audience",
			mockResponse: map[string]any{
				"active": true,
				"aud":    "wrong-audience",
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			},
			mockStatus:  http.StatusOK,
			wantError:   true,
			errContains: "audience validation failed",
		},
		{
			name:  "expired token",
			token: "opaque-expired",
			mockResponse: map[string]any{
				"active": true,
				"exp":    time.Now().Add(-1 * time.Hour).Unix(),
				"iss":    "https://example.com",
			},
			mockStatus:  http.StatusOK,
			wantError:   true,
			errContains: "token has expired",
		},
		{
			name:  "missing issuer in opaque token response",
			token: "opaque-missing-iss",
			mockResponse: map[string]any{
				"active": true,
				"aud":    "my-audience",
				"exp":    time.Now().Add(time.Hour).Unix(),
			},
			mockStatus:  http.StatusOK,
			wantError:   true,
			errContains: "missing issuer in token validation",
		},
		{
			name:  "introspection error status",
			token: "opaque-error",
			mockResponse: map[string]any{
				"error": "server_error",
			},
			mockStatus:  http.StatusInternalServerError,
			wantError:   true,
			errContains: "introspection failed with status: 500",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/.well-known/openid-configuration" {
					w.Header().Set("Content-Type", "application/json")
					config := map[string]interface{}{
						"issuer":   "https://example.com",
						"jwks_uri": "http://" + r.Host + "/jwks",
					}
					if tc.mockOidcConfig != nil {
						for k, v := range tc.mockOidcConfig {
							valStr, ok := v.(string)
							if ok && strings.Contains(valStr, "SERVER_HOST") {
								config[k] = strings.Replace(valStr, "SERVER_HOST", r.Host, 1)
							} else {
								config[k] = v
							}
						}
					}
					_ = json.NewEncoder(w).Encode(config)
					return
				}
				if r.URL.Path == "/jwks" {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"keys": []any{},
					})
					return
				}
				if r.URL.Path == "/introspect" || r.URL.Path == "/custom-introspect" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.mockStatus)
					_ = json.NewEncoder(w).Encode(tc.mockResponse)
					return
				}
				http.NotFound(w, r)
			})
			server := httptest.NewServer(handler)
			defer server.Close()

			cfg := Config{
				Name:                "test-generic-auth",
				Type:                "generic",
				Audience:            tc.audience,
				AuthorizationServer: server.URL,
				McpEnabled:          true,
				ScopesRequired:      tc.scopesRequired,
			}

			authService, err := cfg.Initialize()
			if err != nil {
				t.Fatalf("failed to initialize auth service: %v", err)
			}

			genericAuth, ok := authService.(*AuthService)
			if !ok {
				t.Fatalf("expected *AuthService, got %T", authService)
			}

			logger, err := log.NewLogger("standard", log.Debug, &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("failed to create logger: %v", err)
			}
			ctx := util.WithLogger(context.Background(), logger)

			header := http.Header{}
			header.Set("Authorization", "Bearer "+tc.token)

			_, err = genericAuth.ValidateMCPAuth(ctx, header)

			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing %q, got: %v", tc.errContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateJwtToken(t *testing.T) {
	privateKey := generateRSAPrivateKey(t)
	keyID := "test-key-id"
	server := setupJWKSMockServer(t, privateKey, keyID)
	defer server.Close()

	cfg := Config{
		Name:                "test-generic-auth",
		Type:                "generic",
		Audience:            "my-audience",
		AuthorizationServer: server.URL,
		McpEnabled:          true,
		ScopesRequired:      []string{"read:files"},
	}

	authService, err := cfg.Initialize()
	if err != nil {
		t.Fatalf("failed to initialize auth service: %v", err)
	}

	genericAuth, ok := authService.(*AuthService)
	if !ok {
		t.Fatalf("expected *AuthService, got %T", authService)
	}

	tests := []struct {
		name        string
		token       string
		wantError   bool
		errContains string
	}{
		{
			name: "valid jwt",
			token: generateValidToken(t, privateKey, keyID, jwt.MapClaims{
				"iss":   "https://example.com",
				"aud":   "my-audience",
				"scope": "read:files",
				"exp":   time.Now().Add(time.Hour).Unix(),
			}),
			wantError: false,
		},
		{
			name:        "invalid token (wrong signature)",
			token:       "header.payload.signature",
			wantError:   true,
			errContains: "invalid or expired token",
		},
		{
			name: "audience mismatch",
			token: generateValidToken(t, privateKey, keyID, jwt.MapClaims{
				"iss":   "https://example.com",
				"aud":   "wrong-audience",
				"scope": "read:files",
				"exp":   time.Now().Add(time.Hour).Unix(),
			}),
			wantError:   true,
			errContains: "audience validation failed",
		},
		{
			name: "insufficient scopes",
			token: generateValidToken(t, privateKey, keyID, jwt.MapClaims{
				"iss":   "https://example.com",
				"aud":   "my-audience",
				"scope": "wrong:scope",
				"exp":   time.Now().Add(time.Hour).Unix(),
			}),
			wantError:   true,
			errContains: "insufficient scopes",
		},
		{
			name: "missing issuer",
			token: generateValidToken(t, privateKey, keyID, jwt.MapClaims{
				"aud":   "my-audience",
				"scope": "read:files",
				"exp":   time.Now().Add(time.Hour).Unix(),
			}),
			wantError:   true,
			errContains: "missing issuer claim in token",
		},
		{
			name: "issuer mismatch",
			token: generateValidToken(t, privateKey, keyID, jwt.MapClaims{
				"iss":   "https://wrong-issuer.com",
				"aud":   "my-audience",
				"scope": "read:files",
				"exp":   time.Now().Add(time.Hour).Unix(),
			}),
			wantError:   true,
			errContains: "issuer validation failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger, err := log.NewLogger("standard", log.Debug, &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("failed to create logger: %v", err)
			}
			ctx := util.WithLogger(context.Background(), logger)
			_, err = genericAuth.validateJwtToken(ctx, tc.token)
			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing %q, got: %v", tc.errContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateOpaqueToken(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		scopesRequired []string
		audience       string
		mockOidcConfig map[string]any
		mockResponse   map[string]any
		mockStatus     int
		wantError      bool
		errContains    string
	}{
		{
			name:           "valid opaque token",
			token:          "opaque-valid",
			scopesRequired: []string{"read:files"},
			audience:       "my-audience",
			mockResponse: map[string]any{
				"active": true,
				"scope":  "read:files write:files",
				"aud":    "my-audience",
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			},
			mockStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:           "valid opaque token with string exp",
			token:          "opaque-valid-string-exp",
			scopesRequired: []string{"read:files"},
			audience:       "my-audience",
			mockResponse: map[string]any{
				"active": true,
				"scope":  "read:files write:files",
				"aud":    "my-audience",
				"exp":    "2000000000",
				"iss":    "https://example.com",
			},
			mockStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:           "valid opaque token with custom introspection endpoint",
			token:          "opaque-valid-custom",
			scopesRequired: []string{"read:files"},
			audience:       "my-audience",
			mockOidcConfig: map[string]any{
				"introspection_endpoint": "http://SERVER_HOST/custom-introspect",
			},
			mockResponse: map[string]any{
				"active": true,
				"scope":  "read:files",
				"aud":    "my-audience",
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			},
			mockStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:           "valid opaque token with array aud",
			token:          "opaque-valid-array-aud",
			scopesRequired: []string{"read:files"},
			audience:       "my-audience",
			mockResponse: map[string]any{
				"active": true,
				"scope":  "read:files",
				"aud":    []string{"other-audience", "my-audience"},
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			},
			mockStatus: http.StatusOK,
			wantError:  false,
		},
		{
			name:           "inactive opaque token",
			token:          "opaque-inactive",
			scopesRequired: []string{"read:files"},
			mockResponse: map[string]any{
				"active": false,
			},
			mockStatus:  http.StatusOK,
			wantError:   true,
			errContains: "token is not active",
		},
		{
			name:           "missing active claim",
			token:          "opaque-missing-active",
			scopesRequired: []string{"read:files"},
			mockResponse: map[string]any{
				"scope": "read:files",
			},
			mockStatus:  http.StatusOK,
			wantError:   true,
			errContains: "token is not active",
		},
		{
			name:           "insufficient scopes",
			token:          "opaque-bad-scope",
			scopesRequired: []string{"read:files", "write:files"},
			mockResponse: map[string]any{
				"active": true,
				"scope":  "read:files",
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			},
			mockStatus:  http.StatusOK,
			wantError:   true,
			errContains: "insufficient scopes",
		},
		{
			name:     "audience mismatch",
			token:    "opaque-bad-aud",
			audience: "my-audience",
			mockResponse: map[string]any{
				"active": true,
				"aud":    "wrong-audience",
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			},
			mockStatus:  http.StatusOK,
			wantError:   true,
			errContains: "audience validation failed",
		},
		{
			name:  "expired token",
			token: "opaque-expired",
			mockResponse: map[string]any{
				"active": true,
				"exp":    time.Now().Add(-1 * time.Hour).Unix(),
				"iss":    "https://example.com",
			},
			mockStatus:  http.StatusOK,
			wantError:   true,
			errContains: "token has expired",
		},
		{
			name:  "missing issuer in opaque token response",
			token: "opaque-missing-iss",
			mockResponse: map[string]any{
				"active": true,
				"aud":    "my-audience",
				"exp":    time.Now().Add(time.Hour).Unix(),
			},
			mockStatus:  http.StatusOK,
			wantError:   true,
			errContains: "missing issuer in token validation",
		},
		{
			name:  "introspection error status",
			token: "opaque-error",
			mockResponse: map[string]any{
				"error": "server_error",
			},
			mockStatus:  http.StatusInternalServerError,
			wantError:   true,
			errContains: "introspection failed with status: 500",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/introspect" || r.URL.Path == "/custom-introspect" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.mockStatus)
					_ = json.NewEncoder(w).Encode(tc.mockResponse)
					return
				}
				http.NotFound(w, r)
			})
			server := httptest.NewServer(handler)
			defer server.Close()

			genericAuth := &AuthService{
				Config: Config{
					Audience:            tc.audience,
					AuthorizationServer: server.URL,
					ScopesRequired:      tc.scopesRequired,
				},
				client: newSecureHTTPClient(),
				issuer: "https://example.com",
			}

			logger, err := log.NewLogger("standard", log.Debug, &bytes.Buffer{}, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("failed to create logger: %v", err)
			}
			ctx := util.WithLogger(context.Background(), logger)

			_, err = genericAuth.validateOpaqueToken(ctx, tc.token)

			if tc.wantError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing %q, got: %v", tc.errContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestIsJWTFormat(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{
			name:  "valid JWT format with alg",
			token: "eyJhbGciOiJSUzI1NiJ9.payload.signature",
			want:  true,
		},
		{
			name:  "invalid base64 in header",
			token: "invalid_base64!@#.payload.signature",
			want:  false,
		},
		{
			name:  "valid base64 but invalid JSON",
			token: "aGVsbG8.payload.signature",
			want:  false,
		},
		{
			name:  "valid JSON but missing alg parameter",
			token: "eyJmb28iOiJiYXIifQ.payload.signature",
			want:  false,
		},
		{
			name:  "opaque token without dots",
			token: "opaque-token",
			want:  false,
		},
		{
			name:  "too many dots",
			token: "a.b.c.d",
			want:  false,
		},
		{
			name:  "too few dots",
			token: "a.b",
			want:  false,
		},
		{
			name:  "empty string",
			token: "",
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isJWTFormat(tc.token)
			if got != tc.want {
				t.Errorf("isJWTFormat(%q) = %v; want %v", tc.token, got, tc.want)
			}
		})
	}
}

func TestForceIntrospection(t *testing.T) {
	const jwtToken = "eyJhbGciOiJSUzI1NiJ9.payload.signature"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 "https://example.com",
				"jwks_uri":               "http://" + r.Host + "/jwks",
				"introspection_endpoint": "http://" + r.Host + "/introspect",
			})
		case "/jwks":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
		case "/introspect":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active": true,
				"scope":  "read:files",
				"aud":    "my-audience",
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			})
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	logger, err := log.NewLogger("standard", log.Debug, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+jwtToken)

	t.Run("force introspection with JWT token", func(t *testing.T) {
		cfg := Config{
			Name:                "test-generic-auth",
			Type:                "generic",
			Audience:            "my-audience",
			AuthorizationServer: server.URL,
			McpEnabled:          true,
			ScopesRequired:      []string{"read:files"},
			ForceIntrospection:  true,
		}
		authService, err := cfg.Initialize()
		if err != nil {
			t.Fatalf("failed to initialize auth service: %v", err)
		}
		genericAuth := authService.(*AuthService)

		claims, err := genericAuth.ValidateMCPAuth(ctx, header)
		if err != nil {
			t.Fatalf("expected success via introspection, got error: %v", err)
		}
		if claims["iss"] != "https://example.com" {
			t.Errorf("expected claims from introspection, got %v", claims)
		}
	})
}

func TestIntrospectionClientAuth(t *testing.T) {
	var gotUser, gotPass string
	var gotOK bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 "https://example.com",
				"jwks_uri":               "http://" + r.Host + "/jwks",
				"introspection_endpoint": "http://" + r.Host + "/introspect",
			})
		case "/jwks":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{}})
		case "/introspect":
			gotUser, gotPass, gotOK = r.BasicAuth()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active": true,
				"scope":  "read:files",
				"aud":    "my-audience",
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			})
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	logger, err := log.NewLogger("standard", log.Debug, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	t.Run("only client ID set is treated as unconfigured, not an error", func(t *testing.T) {
		t.Setenv(IntrospectionClientIDEnvVar, "my-client")
		t.Setenv(IntrospectionClientSecretEnvVar, "")
		gotOK = false

		cfg := Config{
			Name:                "test-generic-auth",
			Type:                "generic",
			Audience:            "my-audience",
			AuthorizationServer: server.URL,
			McpEnabled:          true,
			ForceIntrospection:  true,
		}
		authService, err := cfg.Initialize()
		if err != nil {
			t.Fatalf("expected partial env config to be tolerated, got error: %v", err)
		}
		genericAuth := authService.(*AuthService)

		header := http.Header{}
		header.Set("Authorization", "Bearer opaque-token")
		if _, err := genericAuth.ValidateMCPAuth(ctx, header); err != nil {
			t.Fatalf("expected success via introspection, got error: %v", err)
		}
		if gotOK {
			t.Errorf("expected no basic auth to be sent, but got user=%q, pass=%q", gotUser, gotPass)
		}
	})

	t.Run("client ID and secret are sent via basic auth", func(t *testing.T) {
		t.Setenv(IntrospectionClientIDEnvVar, "my-client")
		t.Setenv(IntrospectionClientSecretEnvVar, "my-secret")

		cfg := Config{
			Name:                "test-generic-auth",
			Type:                "generic",
			Audience:            "my-audience",
			AuthorizationServer: server.URL,
			McpEnabled:          true,
			ForceIntrospection:  true,
		}
		authService, err := cfg.Initialize()
		if err != nil {
			t.Fatalf("failed to initialize auth service: %v", err)
		}
		genericAuth := authService.(*AuthService)

		header := http.Header{}
		header.Set("Authorization", "Bearer opaque-token")
		if _, err := genericAuth.ValidateMCPAuth(ctx, header); err != nil {
			t.Fatalf("expected success via introspection, got error: %v", err)
		}

		if !gotOK {
			t.Errorf("expected basic auth to be sent, but got none")
		}
		if gotUser != "my-client" || gotPass != "my-secret" {
			t.Errorf("expected basic auth user=my-client, pass=my-secret, got user=%q, pass=%q", gotUser, gotPass)
		}
	})

	t.Run("no client ID or secret are sent via basic auth", func(t *testing.T) {
		t.Setenv(IntrospectionClientIDEnvVar, "")
		t.Setenv(IntrospectionClientSecretEnvVar, "")

		cfg := Config{
			Name:                "test-generic-auth",
			Type:                "generic",
			Audience:            "my-audience",
			AuthorizationServer: server.URL,
			McpEnabled:          true,
			ForceIntrospection:  true,
		}
		authService, err := cfg.Initialize()
		if err != nil {
			t.Fatalf("failed to initialize auth service: %v", err)
		}
		genericAuth := authService.(*AuthService)

		header := http.Header{}
		header.Set("Authorization", "Bearer opaque-token")
		if _, err := genericAuth.ValidateMCPAuth(ctx, header); err != nil {
			t.Fatalf("expected success via introspection, got error: %v", err)
		}
		if gotOK {
			t.Errorf("expected no basic auth to be sent, but got user=%q, pass=%q", gotUser, gotPass)
		}
	})
}
