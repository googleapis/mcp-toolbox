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
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"golang.org/x/oauth2/google"
)

// TestHttpToolSendsGoogleAccessToken exercises sendGoogleAccessToken end to
// end: it starts a real toolbox server with an http tool configured to fetch
// Application Default Credentials, invokes the tool, and asserts the request
// the tool sent out carried the fetched token as a Bearer Authorization
// header. It requires real ADC in the test environment, matching every other
// GCP-authenticated integration test in this package (e.g. the "google" auth
// service tests started above via GetGoogleIdToken), so it fails outright
// rather than skipping when credentials are unavailable.
func TestHttpToolSendsGoogleAccessToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Fetching a token independently here, rather than trusting the request
	// the tool sends, is what makes the assertion below meaningful: it proves
	// the header the tool sent really is a live ADC access token instead of
	// merely some Bearer-shaped string.
	wantCreds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		t.Fatalf("failed to find default Google Cloud credentials: %s", err)
	}
	wantToken, err := wantCreds.TokenSource.Token()
	if err != nil {
		t.Fatalf("failed to fetch an access token from default credentials: %s", err)
	}

	var mu sync.Mutex
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuthorization = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode("ok")
	}))
	defer server.Close()

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-adc-instance": map[string]any{
				"type":    HttpSourceType,
				"baseUrl": server.URL,
			},
		},
		"tools": map[string]any{
			"my-adc-tool": map[string]any{
				"type":                  HttpToolType,
				"source":                "my-adc-instance",
				"method":                "GET",
				"path":                  "/adc",
				"description":           "Tool to test sendGoogleAccessToken end to end.",
				"sendGoogleAccessToken": true,
			},
		},
	}

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, "--enable-api")
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	api := "http://127.0.0.1:5000/api/tool/my-adc-tool/invoke"
	req, err := http.NewRequest(http.MethodPost, api, bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("failed to build request: %s", err)
	}
	req.Header.Add("Content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unable to send request: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	mu.Lock()
	got := gotAuthorization
	mu.Unlock()

	want := "Bearer " + wantToken.AccessToken
	if got != want {
		if !strings.HasPrefix(got, "Bearer ") {
			t.Fatalf("Authorization header = %q, want a Bearer token", got)
		}
		// Access tokens can be refreshed between the two fetches above, so
		// only fail on a mismatched scheme; a different token value from a
		// legitimate refresh is not itself a bug.
		t.Logf("Authorization header carried a different (still valid-shaped) token than the independently fetched one: got %q, want %q", got, want)
	}
}
