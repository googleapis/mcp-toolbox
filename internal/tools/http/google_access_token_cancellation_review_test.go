// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
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
	"testing"
	"time"
)

// Cancellation during token retrieval must not wait for the OAuth endpoint.
func TestGoogleAccessTokenHonorsActiveInvocationCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
			return
		case <-release:
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "local-test-token", "token_type": "Bearer", "expires_in": 3600,
		})
	}))
	defer server.Close()
	defer close(release)
	credentials, err := json.Marshal(map[string]string{
		"type": "authorized_user", "client_id": "test-client", "client_secret": "test-secret",
		"refresh_token": "test-refresh", "token_uri": server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	credentialPath := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(credentialPath, credentials, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credentialPath)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := &adcTokenProvider{}
	result := make(chan error, 1)
	go func() {
		_, err := provider.Token(ctx)
		result <- err
	}()
	select {
	case <-started:
	case err := <-result:
		t.Fatalf("token retrieval ended before local endpoint was reached: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("local token endpoint was not reached")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("token retrieval ignored active invocation cancellation while OAuth endpoint was blocked")
	}
}
