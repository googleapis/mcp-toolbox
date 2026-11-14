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
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

func TestHttpToolSendsGoogleAccessToken(t *testing.T) {
	const accessToken = "test-adc-access-token"
	var tokenRequests atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenRequests.Add(1)
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if got, want := r.Form.Get("grant_type"), "refresh_token"; got != want {
				http.Error(w, "unexpected grant type", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			expiresIn := 3600
			if tokenRequests.Load() == 1 {
				expiresIn = 11
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": accessToken,
				"token_type":   "Bearer",
				"expires_in":   expiresIn,
			})
		case "/protected":
			if got, want := r.Header.Get("Authorization"), "Bearer "+accessToken; got != want {
				http.Error(w, "unexpected Authorization header", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte("authenticated"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	credentials := map[string]string{
		"type":          "authorized_user",
		"client_id":     "test-client-id",
		"client_secret": "test-client-secret",
		"refresh_token": "test-refresh-token",
		"token_uri":     upstream.URL + "/token",
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

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-http-source": map[string]any{
				"type":                 "http",
				"baseUrl":              upstream.URL,
				"allowPrivateNetworks": true,
			},
		},
		"tools": map[string]any{
			"my-google-api-tool": map[string]any{
				"type":                  "http",
				"source":                "my-http-source",
				"method":                "GET",
				"path":                  "/protected",
				"description":           "Call an API with a Google access token.",
				"sendGoogleAccessToken": true,
				"headers": map[string]string{
					"Authorization": "Bearer configured-token",
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, "--enable-api")
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()
	defer cmd.Close()

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	for invocation := 1; invocation <= 2; invocation++ {
		if invocation == 2 {
			// The OAuth library treats tokens expiring within ten seconds as
			// invalid. Let the first token enter that window so this invocation
			// exercises refresh after the first request context is cancelled.
			time.Sleep(2 * time.Second)
		}
		req, err := http.NewRequest(
			http.MethodPost,
			"http://127.0.0.1:5000/api/tool/my-google-api-tool/invoke",
			bytes.NewBufferString(`{}`),
		)
		if err != nil {
			t.Fatalf("unable to create invocation request: %s", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("unable to invoke tool: %s", err)
		}
		if resp.StatusCode != http.StatusOK {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				t.Fatalf("unexpected invocation status %d and unable to read response: %s", resp.StatusCode, readErr)
			}
			t.Fatalf("unexpected invocation status: got %d, want %d, body: %s", resp.StatusCode, http.StatusOK, body)
		}

		var body struct {
			Result string `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			_ = resp.Body.Close()
			t.Fatalf("unable to decode invocation response: %s", err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("unable to close invocation response: %s", err)
		}
		if got, want := body.Result, `"authenticated"`; got != want {
			t.Fatalf("unexpected invocation result: got %q, want %q", got, want)
		}
	}

	if got, want := tokenRequests.Load(), int32(2); got != want {
		t.Fatalf("unexpected OAuth token request count: got %d, want %d", got, want)
	}
}
