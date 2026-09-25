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

package bigquery_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/sources/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"go.opentelemetry.io/otel/trace/noop"
)

// fakeADC points ADC at an authorized_user credentials file whose token_uri is
// tokenURL. That is the shape `gcloud auth application-default login` writes,
// and the one that makes this test meaningful: for authorized_user, x/oauth2
// builds a tokenRefresher that keeps its construction context and attaches it
// to every refresh request. A service-account file would not fail the same way,
// since its jwtSource uses the context only to select an HTTP client.
func fakeADC(t *testing.T, tokenURL string) {
	t.Helper()
	blob, err := json.Marshal(map[string]string{
		"type":             "authorized_user",
		"client_id":        "fake-client-id.apps.googleusercontent.com",
		"client_secret":    "fake-client-secret",
		"refresh_token":    "fake-refresh-token",
		"token_uri":        tokenURL,
		"quota_project_id": "test-project",
	})
	if err != nil {
		t.Fatalf("marshalling creds: %s", err)
	}
	path := filepath.Join(t.TempDir(), "adc.json")
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("writing creds: %s", err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", path)
}

// The ADC client is cached and reused for the life of the process, but its
// token has to be refreshed periodically. Building it from the connect context
// — which ConnectOnce cancels as soon as the connect returns — leaves a client
// that cannot refresh, so the first API call fails with "context canceled" and
// every one after it does too.
func TestADCClientRefreshesAfterTheConnectReturns(t *testing.T) {
	var tokenFetches atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		tokenFetches.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"fake-token","token_type":"Bearer","expires_in":3600}`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"test-project:d.t"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	fakeADC(t, srv.URL+"/token")

	cfg := bigquery.Config{
		Name:           "bq",
		Type:           "bigquery",
		Project:        "test-project",
		UseClientOAuth: "false",
		APIEndpoint:    srv.URL,
	}

	// deferConnect mirrors --defer-source-connect: the connect runs on first
	// use rather than at startup.
	startupCtx := testutils.ContextWithUserAgent(context.Background(), "1.2.3")
	src, err := cfg.Initialize(startupCtx, noop.NewTracerProvider().Tracer("test"), true)
	if err != nil {
		t.Fatalf("initializing source: %s", err)
	}
	bqSrc, ok := src.(*bigquery.Source)
	if !ok {
		t.Fatalf("expected *bigquery.Source, got %T", src)
	}

	// Triggers the connect and caches the client. The connect context is
	// cancelled as soon as this returns.
	_, restService, err := bqSrc.RetrieveClientAndService(context.Background(), "")
	if err != nil {
		t.Fatalf("connecting: %s", err)
	}

	if _, err := restService.Tables.Get("test-project", "d", "t").Do(); err != nil {
		if strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("the cached client cannot refresh its token once the connect has returned: %s", err)
		}
		t.Fatalf("unexpected error calling the API: %s", err)
	}
	if tokenFetches.Load() == 0 {
		t.Fatal("no token was fetched, so this test did not exercise a refresh")
	}
}
