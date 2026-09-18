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

package google

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestInitialize_Validation(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantError bool
	}{
		{
			name: "only clientID, mcpEnabled false",
			config: Config{
				Name:       "google-auth",
				Type:       "google",
				ClientID:   "my-client-id",
				McpEnabled: false,
			},
			wantError: false,
		},
		{
			name: "only audience, mcpEnabled false (disallowed)",
			config: Config{
				Name:       "google-auth",
				Type:       "google",
				Audience:   "my-audience",
				McpEnabled: false,
			},
			wantError: true,
		},
		{
			name: "only audience, mcpEnabled true (allowed)",
			config: Config{
				Name:       "google-auth",
				Type:       "google",
				Audience:   "my-audience",
				McpEnabled: true,
			},
			wantError: false,
		},
		{
			name: "scopesRequired, mcpEnabled false (disallowed)",
			config: Config{
				Name:           "google-auth",
				Type:           "google",
				ScopesRequired: []string{"scope"},
				McpEnabled:     false,
			},
			wantError: true,
		},
		{
			name: "scopesRequired, mcpEnabled true, with audience (allowed)",
			config: Config{
				Name:           "google-auth",
				Type:           "google",
				ScopesRequired: []string{"scope"},
				Audience:       "my-audience",
				McpEnabled:     true,
			},
			wantError: false,
		},
		{
			name: "scopesRequired, mcpEnabled true, without audience or clientID (disallowed)",
			config: Config{
				Name:           "google-auth",
				Type:           "google",
				ScopesRequired: []string{"scope"},
				McpEnabled:     true,
			},
			wantError: true,
		},
		{
			name: "both clientID and audience, mcpEnabled true",
			config: Config{
				Name:       "google-auth",
				Type:       "google",
				ClientID:   "my-client-id",
				Audience:   "my-audience",
				McpEnabled: true,
			},
			wantError: false,
		},
		{
			name: "neither clientID nor audience, mcpEnabled false (disallowed)",
			config: Config{
				Name:       "google-auth",
				Type:       "google",
				McpEnabled: false,
			},
			wantError: true,
		},
		{
			name: "neither clientID nor audience, mcpEnabled true (disallowed)",
			config: Config{
				Name:       "google-auth",
				Type:       "google",
				McpEnabled: true,
			},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.config.Initialize()
			if (err != nil) != tc.wantError {
				t.Fatalf("Initialize() returned error: %v, wantError: %v", err, tc.wantError)
			}
		})
	}
}

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (f mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestValidateMCPAuth_Opaque_Fallback(t *testing.T) {
	tests := []struct {
		name         string
		audience     string
		clientID     string
		tokenInfoAud string
		tokenInfoAzp string
		wantError    bool
	}{
		{
			name:         "only audience matches",
			audience:     "my-aud",
			tokenInfoAud: "my-aud",
			wantError:    false,
		},
		{
			name:         "only clientID matches (fallback)",
			clientID:     "my-client-id",
			tokenInfoAud: "my-client-id",
			wantError:    false,
		},
		{
			name:         "only clientID, tokenInfo uses azp (fallback)",
			clientID:     "my-client-id",
			tokenInfoAzp: "my-client-id",
			wantError:    false,
		},
		{
			name:         "both audience and clientID, audience matches",
			audience:     "my-aud",
			clientID:     "my-client-id",
			tokenInfoAud: "my-aud",
			wantError:    false,
		},
		{
			name:         "both audience and clientID, clientID does not fall back if audience is specified",
			audience:     "my-aud",
			clientID:     "my-client-id",
			tokenInfoAud: "my-client-id",
			wantError:    true,
		},
		{
			name:         "neither audience nor clientID specified",
			tokenInfoAud: "any-aud",
			wantError:    false,
		},
		{
			name:         "audience mismatch",
			audience:     "my-aud",
			tokenInfoAud: "wrong-aud",
			wantError:    true,
		},
		{
			name:         "clientID mismatch (fallback)",
			clientID:     "my-client-id",
			tokenInfoAud: "wrong-aud",
			wantError:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &http.Client{
				Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
					if req.URL.String() != "https://oauth2.googleapis.com/tokeninfo" {
						return nil, fmt.Errorf("unexpected URL: %s", req.URL.String())
					}
					respBody := fmt.Sprintf(`{"aud": %q, "azp": %q, "scope": "openid email"}`, tc.tokenInfoAud, tc.tokenInfoAzp)
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(respBody)),
						Header:     make(http.Header),
					}, nil
				}),
			}

			a := AuthService{
				Config: Config{
					Audience: tc.audience,
					ClientID: tc.clientID,
				},
				client: mockClient,
			}

			header := make(http.Header)
			header.Set("Authorization", "Bearer some-opaque-token")

			_, err := a.ValidateMCPAuth(context.Background(), header)
			if (err != nil) != tc.wantError {
				t.Fatalf("ValidateMCPAuth() returned error: %v, wantError: %v", err, tc.wantError)
			}
		})
	}
}
