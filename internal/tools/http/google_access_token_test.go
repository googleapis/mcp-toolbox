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

package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/tools"
	"golang.org/x/oauth2"
)

func TestInitializeGoogleAccessTokenIsLazy(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "missing-credentials.json"))

	initialized, err := (Config{
		ConfigBase:            tools.ConfigBase{Name: "google-api", Description: "Call a Google API."},
		Type:                  "http",
		Source:                "my-http-source",
		Path:                  "/resource",
		Method:                tools.HTTPMethod(http.MethodGet),
		SendGoogleAccessToken: true,
	}).Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize returned an error before the tool was invoked: %s", err)
	}
	httpTool, ok := initialized.(Tool)
	if !ok {
		t.Fatalf("Initialize returned %T, want Tool", initialized)
	}
	if httpTool.googleAccessTokenProvider == nil {
		t.Fatal("Google access token provider was not configured")
	}
	if httpTool.googleAccessTokenProvider.tokenSource != nil {
		t.Fatal("Google ADC was resolved during tool initialization")
	}
}

func TestSetGoogleAccessToken(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("unable to create request: %s", err)
	}
	req.Header.Set("Authorization", "Bearer configured-token")

	err = setGoogleAccessToken(
		context.Background(),
		req,
		&adcTokenProvider{
			tokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "adc-token"}),
		},
	)
	if err != nil {
		t.Fatalf("setGoogleAccessToken returned an error: %s", err)
	}
	if got, want := req.Header.Get("Authorization"), "Bearer adc-token"; got != want {
		t.Fatalf("unexpected Authorization header: got %q, want %q", got, want)
	}
}

func TestSetGoogleAccessTokenError(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("unable to create request: %s", err)
	}

	err = setGoogleAccessToken(
		context.Background(),
		req,
		&adcTokenProvider{
			tokenSource: errorTokenSource{err: errors.New("credentials unavailable")},
		},
	)
	if err == nil {
		t.Fatal("expected setGoogleAccessToken to return an error")
	}
	if !strings.Contains(err.Error(), "credentials unavailable") {
		t.Fatalf("unexpected error: %s", err)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization header was set after token retrieval failed: %q", got)
	}
}

func TestGoogleAccessTokenUsesInvocationContext(t *testing.T) {
	const accessToken = "invocation-context-token"
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": accessToken,
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	credentials := map[string]string{
		"type":          "authorized_user",
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"refresh_token": "test-refresh-token",
		"token_uri":     tokenServer.URL,
	}
	credentialsJSON, err := json.Marshal(credentials)
	if err != nil {
		t.Fatalf("unable to marshal credentials: %s", err)
	}
	credentialsPath := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(credentialsPath, credentialsJSON, 0o600); err != nil {
		t.Fatalf("unable to write credentials: %s", err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credentialsPath)

	initializationCtx, cancelInitialization := context.WithCancel(context.Background())
	initialized, err := (Config{
		ConfigBase:            tools.ConfigBase{Name: "google-api", Description: "Call a Google API."},
		Type:                  "http",
		Source:                "my-http-source",
		Path:                  "/resource",
		Method:                tools.HTTPMethod(http.MethodGet),
		SendGoogleAccessToken: true,
	}).Initialize(initializationCtx)
	if err != nil {
		t.Fatalf("Initialize returned an error: %s", err)
	}
	cancelInitialization()

	httpTool, ok := initialized.(Tool)
	if !ok {
		t.Fatalf("Initialize returned %T, want Tool", initialized)
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("unable to create request: %s", err)
	}
	if err := setGoogleAccessToken(context.Background(), req, httpTool.googleAccessTokenProvider); err != nil {
		t.Fatalf("setGoogleAccessToken returned an error after initialization context cancellation: %s", err)
	}
	if got, want := req.Header.Get("Authorization"), "Bearer "+accessToken; got != want {
		t.Fatalf("unexpected Authorization header: got %q, want %q", got, want)
	}
}

type errorTokenSource struct {
	err error
}

func (s errorTokenSource) Token() (*oauth2.Token, error) {
	return nil, s.err
}
