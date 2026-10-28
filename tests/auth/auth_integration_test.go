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

package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// TestMcpAuth test for MCP Authorization
func TestMcpAuth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Set up generic auth mock server
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to create RSA private key: %v", err)
	}
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
					KID: "test-key-id",
				},
			}
			jwk, _ := jwkset.NewJWKFromKey(privateKey.Public(), options)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"keys": []jwkset.JWKMarshal{jwk.Marshal()},
			})
			return
		}
		if r.URL.Path == "/introspect" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"active": true,
				"scope":  "read:files execute:sql",
				"aud":    "test-audience",
				"exp":    time.Now().Add(time.Hour).Unix(),
				"iss":    "https://example.com",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer jwksServer.Close()

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-sqlite": map[string]any{
				"type":     "sqlite",
				"database": ":memory:",
			},
		},
		"authServices": map[string]any{
			"my-generic-auth": map[string]any{
				"type":                "generic",
				"audience":            "test-audience",
				"authorizationServer": jwksServer.URL,
				"scopesRequired":      []string{"read:files"},
				"mcpEnabled":          true,
			},
		},
		"tools": map[string]any{
			"my-tool": map[string]any{
				"type":           "sqlite-execute-sql",
				"source":         "my-sqlite",
				"description":    "Execute SQL on SQLite",
				"scopesRequired": []string{"execute:sql"},
				"authRequired":   []string{"my-generic-auth"},
			},
		},
	}
	args := []string{"--toolbox-url=http://127.0.0.1:5000"}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	apiSSE := "http://127.0.0.1:5000/mcp/sse"
	apiMCP := "http://127.0.0.1:5000/mcp"

	// Generate invalid token (wrong scopes)
	invalidToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   "https://example.com",
		"aud":   "test-audience",
		"scope": "wrong:scope",
		"sub":   "test-user",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	invalidToken.Header["kid"] = "test-key-id"
	invalidSignedString, _ := invalidToken.SignedString(privateKey)

	// Generate valid token
	validToken := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   "https://example.com",
		"aud":   "test-audience",
		"scope": "read:files",
		"sub":   "test-user",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	validToken.Header["kid"] = "test-key-id"
	validSignedString, _ := validToken.SignedString(privateKey)

	// Generate token with only read:files scope
	tokenOnlyRead := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   "https://example.com",
		"aud":   "test-audience",
		"scope": "read:files",
		"sub":   "test-user",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	tokenOnlyRead.Header["kid"] = "test-key-id"
	tokenOnlyReadStr, _ := tokenOnlyRead.SignedString(privateKey)

	// Generate token with BOTH scopes
	tokenBoth := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   "https://example.com",
		"aud":   "test-audience",
		"scope": "read:files execute:sql",
		"sub":   "test-user",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	tokenBoth.Header["kid"] = "test-key-id"
	tokenBothStr, _ := tokenBoth.SignedString(privateKey)

	tests := []struct {
		name           string
		token          string
		method         string
		url            string
		body           map[string]any
		wantStatusCode int
		checkWWWAuth   func(t *testing.T, authHeader string)
	}{
		{
			name:           "401 Unauthorized without token",
			token:          "",
			wantStatusCode: http.StatusUnauthorized,
			checkWWWAuth: func(t *testing.T, authHeader string) {
				if !strings.Contains(authHeader, `resource_metadata="http://127.0.0.1:5000/.well-known/oauth-protected-resource"`) || !strings.Contains(authHeader, `scope="read:files"`) {
					t.Fatalf("expected WWW-Authenticate header to contain resource_metadata and scope, got: %s", authHeader)
				}
			},
		},
		{
			name:           "403 Forbidden with insufficient scopes",
			token:          invalidSignedString,
			wantStatusCode: http.StatusForbidden,
			checkWWWAuth: func(t *testing.T, authHeader string) {
				if !strings.Contains(authHeader, `resource_metadata="http://127.0.0.1:5000/.well-known/oauth-protected-resource"`) || !strings.Contains(authHeader, `scope="read:files"`) || !strings.Contains(authHeader, `error="insufficient_scope"`) {
					t.Fatalf("expected WWW-Authenticate header to contain error, scope, and resource_metadata, got: %s", authHeader)
				}
			},
		},
		{
			name:           "200 OK with valid token",
			token:          validSignedString,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "200 OK with valid opaque token",
			token:          "this-is-an-opaque-token",
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "200 OK with valid opaque token containing two dots",
			token:          "this.is.opaque",
			wantStatusCode: http.StatusOK,
		},
		{
			name:   "403 Forbidden with insufficient tool scopes",
			token:  tokenOnlyReadStr,
			method: http.MethodPost,
			url:    apiMCP,
			body: map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "tools/call",
				"params": map[string]any{
					"name": "my-tool",
					"arguments": map[string]any{
						"sql": "SELECT 1;",
					},
				},
			},
			wantStatusCode: http.StatusForbidden,
			checkWWWAuth: func(t *testing.T, authHeader string) {
				if !strings.Contains(authHeader, `error="insufficient_scope"`) || !strings.Contains(authHeader, `scope="execute:sql"`) {
					t.Fatalf("expected WWW-Authenticate header to contain error and tool scope, got: %s", authHeader)
				}
			},
		},
		{
			name:   "200 OK with sufficient tool scopes",
			token:  tokenBothStr,
			method: http.MethodPost,
			url:    apiMCP,
			body: map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "tools/call",
				"params": map[string]any{
					"name": "my-tool",
					"arguments": map[string]any{
						"sql": "SELECT 1;",
					},
				},
			},
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			url := tc.url
			if url == "" {
				url = apiSSE
			}

			runTest := func(t *testing.T, protocolVersion string) {
				var body io.Reader
				if tc.body != nil {
					if protocolVersion == "2026-07-28" {
						if params, ok := tc.body["params"].(map[string]any); ok {
							params["_meta"] = map[string]any{
								"io.modelcontextprotocol/protocolVersion": protocolVersion,
								"io.modelcontextprotocol/clientInfo": map[string]string{
									"name":    "testClient",
									"version": "0.0.0",
								},
								"io.modelcontextprotocol/clientCapabilities": struct{}{},
							}
						}
					}
					b, err := json.Marshal(tc.body)
					if err != nil {
						t.Fatalf("error marshaling body: %s", err)
					}
					body = bytes.NewBuffer(b)
				}
				req, _ := http.NewRequest(method, url, body)
				if tc.token != "" {
					req.Header.Add("Authorization", "Bearer "+tc.token)
				}
				if method == http.MethodPost {
					req.Header.Set("Content-Type", "application/json")
					if protocolVersion != "" {
						req.Header.Set("MCP-Protocol-Version", protocolVersion)
					}
					if protocolVersion == "2026-07-28" {
						req.Header.Set("Mcp-Method", "tools/call")
						req.Header.Set("Mcp-Name", "my-tool")
					}
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("unable to send request: %s", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != tc.wantStatusCode {
					bodyBytes, _ := io.ReadAll(resp.Body)
					t.Fatalf("expected %d, got %d: %s", tc.wantStatusCode, resp.StatusCode, string(bodyBytes))
				}

				if tc.checkWWWAuth != nil {
					authHeader := resp.Header.Get("WWW-Authenticate")
					tc.checkWWWAuth(t, authHeader)
				}
			}

			if method == http.MethodPost {
				versions := []string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25", "2026-07-28"}
				for _, v := range versions {
					t.Run("version_"+v, func(t *testing.T) {
						runTest(t, v)
					})
				}
			} else {
				runTest(t, "")
			}
		})
	}
}

// TestGoogleOAuthValidation tests validation of Google access token using type: google
func TestGoogleOAuthValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Get access token
	accessToken, err := sources.GetIAMAccessToken(ctx)
	if err != nil {
		t.Errorf("error getting access token from ADC: %s", err)
	}

	// Call tokeninfo to get audience
	resp, err := http.Get("https://www.googleapis.com/oauth2/v1/tokeninfo?access_token=" + accessToken)
	if err != nil {
		t.Fatalf("failed to call tokeninfo: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("tokeninfo returned non-200 status %d: %s", resp.StatusCode, string(body))
	}

	var tokenInfo struct {
		Audience string `json:"audience"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenInfo); err != nil {
		t.Fatalf("failed to decode tokeninfo response: %v", err)
	}

	aud := tokenInfo.Audience
	if aud == "" {
		t.Fatalf("audience is empty in tokeninfo response")
	}

	toolsFile := map[string]any{
		"sources": map[string]any{},
		"authServices": map[string]any{
			"google-auth": map[string]any{
				"type":       "google",
				"audience":   aud,
				"mcpEnabled": true,
			},
		},
		"tools": map[string]any{},
	}

	args := []string{"--toolbox-url=http://127.0.0.1:5006", "--port=5006"}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	api := "http://127.0.0.1:5006/mcp/sse"

	req, _ := http.NewRequest(http.MethodGet, api, nil)
	req.Header.Add("Authorization", "Bearer "+accessToken)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unable to send request: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(bodyBytes))
	}
}

// TestGoogleOAuthValidationNoClientIDOrAudienceFails tests that initialization fails when neither clientId nor audience is configured with mcpEnabled: true.
func TestGoogleOAuthValidationNoClientIDOrAudienceFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	toolsFile := map[string]any{
		"sources": map[string]any{},
		"authServices": map[string]any{
			"google-auth": map[string]any{
				"type":       "google",
				"mcpEnabled": true,
			},
		},
		"tools": map[string]any{},
	}

	args := []string{"--toolbox-url=http://127.0.0.1:5007", "--port=5007"}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	expectedErrPattern := regexp.MustCompile(`audience.*or.*clientId.*is required when.*mcpEnabled.*is true`)
	out, err := testutils.WaitForString(waitCtx, expectedErrPattern, cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("expected toolbox to fail with validation error, but it did not: %s", err)
	}
}

// TestMcpAuthPRMURLWithPath tests that when --toolbox-url contains a path component
func TestMcpAuthPRMURLWithPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to create RSA private key: %v", err)
	}
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
					KID: "test-key-id",
				},
			}
			jwk, _ := jwkset.NewJWKFromKey(privateKey.Public(), options)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"keys": []jwkset.JWKMarshal{jwk.Marshal()},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer jwksServer.Close()

	toolsFile := map[string]any{
		"sources": map[string]any{},
		"authServices": map[string]any{
			"my-generic-auth": map[string]any{
				"type":                "generic",
				"audience":            "test-audience",
				"mcpEnabled":          true,
				"authorizationServer": jwksServer.URL,
				"scopesRequired":      []string{"read:files"},
			},
		},
		"tools": map[string]any{},
	}
	args := []string{"--toolbox-url=http://127.0.0.1:5008/mcp", "--port=5008"}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Verify PRM document is served at the RFC 9728 §3.1 path
	prmURL := "http://127.0.0.1:5008/.well-known/oauth-protected-resource/mcp"
	prmResp, err := http.Get(prmURL)
	if err != nil {
		t.Fatalf("failed to fetch PRM document: %v", err)
	}
	defer prmResp.Body.Close()
	if prmResp.StatusCode != http.StatusOK {
		t.Fatalf("expected PRM status 200, got %d", prmResp.StatusCode)
	}

	// Verify WWW-Authenticate header challenge advertises the RFC 9728 §3.1 PRM URL
	mcpURL := "http://127.0.0.1:5008/mcp"
	mcpResp, err := http.Get(mcpURL)
	if err != nil {
		t.Fatalf("failed to send unauthenticated MCP request: %v", err)
	}
	defer mcpResp.Body.Close()
	if mcpResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected MCP status 401, got %d", mcpResp.StatusCode)
	}
	authHeader := mcpResp.Header.Get("WWW-Authenticate")
	expectedMetadata := `resource_metadata="http://127.0.0.1:5008/.well-known/oauth-protected-resource/mcp"`
	if !strings.Contains(authHeader, expectedMetadata) {
		t.Fatalf("expected WWW-Authenticate header to contain %q, got: %s", expectedMetadata, authHeader)
	}
}
