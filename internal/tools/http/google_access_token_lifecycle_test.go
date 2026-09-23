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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func useLocalADC(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	credentials, err := json.Marshal(map[string]string{
		"type": "authorized_user", "client_id": "test-client", "client_secret": "test-secret",
		"refresh_token": "test-refresh", "token_uri": server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, credentials, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
}

func writeLocalToken(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"access_token":"local-token","token_type":"Bearer","expires_in":3600}`)
}

func TestGoogleAccessTokenRejectsCanceledContextWithCachedToken(t *testing.T) {
	var requests atomic.Int32
	useLocalADC(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		writeLocalToken(w)
	})
	p := &adcTokenProvider{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := p.Token(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := p.Token(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled invocation returned %v, want context.Canceled", err)
	}
	if token, err := p.Token(context.Background()); err != nil || token.AccessToken != "local-token" {
		t.Fatalf("healthy invocation after cancellation: token=%v, error=%v", token, err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("token requests = %d, want 1", got)
	}
}

func TestGoogleAccessTokenCancelsRefreshAndRetries(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	var requests atomic.Int32
	useLocalADC(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if requests.Add(1) == 1 {
			close(started)
			select {
			case <-r.Context().Done():
				close(cancelled)
			case <-release:
			}
			return
		}
		writeLocalToken(w)
	})
	t.Cleanup(func() { close(release) })
	p := &adcTokenProvider{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := p.Token(ctx); result <- err }()
	select {
	case <-started:
	case err := <-result:
		t.Fatalf("refresh did not reach endpoint: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled refresh did not return")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("OAuth request was not cancelled")
	}
	retryCtx, retryCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer retryCancel()
	if token, err := p.Token(retryCtx); err != nil || token.AccessToken != "local-token" {
		t.Fatalf("retry: token=%v, error=%v", token, err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("token requests = %d, want 2", got)
	}
}

func TestGoogleAccessTokenConcurrentWaiterCanCancel(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var requests atomic.Int32
	useLocalADC(t, func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			close(started)
		}
		<-release
		writeLocalToken(w)
	})
	t.Cleanup(unblock)
	p := &adcTokenProvider{}
	leader := make(chan error, 1)
	go func() { _, err := p.Token(context.Background()); leader <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	waiter := make(chan error, 1)
	go func() { _, err := p.Token(ctx); waiter <- err }()
	select {
	case err := <-waiter:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v, want deadline", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter ignored its deadline")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("token requests = %d, want 1", got)
	}
	unblock()
	select {
	case err := <-leader:
		if err != nil {
			t.Fatalf("waiter cancellation affected the active refresh: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("active refresh did not finish after release")
	}
}

func TestGoogleAccessTokenConcurrentCallsReuseToken(t *testing.T) {
	var requests atomic.Int32
	useLocalADC(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		writeLocalToken(w)
	})
	p := &adcTokenProvider{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const callers = 12
	results := make(chan error, callers)
	for range callers {
		go func() {
			token, err := p.Token(ctx)
			if err == nil && token.AccessToken != "local-token" {
				err = errors.New("unexpected token")
			}
			results <- err
		}()
	}
	for range callers {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("concurrent calls did not finish")
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("token requests = %d, want 1", got)
	}
}
