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

package conversationalanalytics

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"golang.org/x/oauth2"
)

type stubSource struct {
	projectID  string
	clientAuth bool
	tokenSrc   oauth2.TokenSource
	tokenErr   error
}

func (s *stubSource) GoogleCloudTokenSourceWithScope(ctx context.Context, scope string) (oauth2.TokenSource, error) {
	return s.tokenSrc, s.tokenErr
}
func (s *stubSource) GetProjectID() string         { return s.projectID }
func (s *stubSource) UseClientAuthorization() bool { return s.clientAuth }

func TestGetTokenSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("client authorization with valid Bearer token", func(t *testing.T) {
		t.Parallel()
		src := &stubSource{clientAuth: true}
		ts, tbErr := GetTokenSource(ctx, src, tools.AccessToken("Bearer my-oauth-token"))
		if tbErr != nil {
			t.Fatalf("unexpected error: %v", tbErr)
		}
		tok, err := ts.Token()
		if err != nil || tok.AccessToken != "my-oauth-token" {
			t.Fatalf("expected token 'my-oauth-token', got %v (err: %v)", tok, err)
		}
	})

	t.Run("client authorization with missing token", func(t *testing.T) {
		t.Parallel()
		src := &stubSource{clientAuth: true}
		_, tbErr := GetTokenSource(ctx, src, "")
		var csErr *util.ClientServerError
		if !errors.As(tbErr, &csErr) || csErr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 ClientServerError, got %T: %v", tbErr, tbErr)
		}
	})

	t.Run("client authorization with malformed token", func(t *testing.T) {
		t.Parallel()
		src := &stubSource{clientAuth: true}
		_, tbErr := GetTokenSource(ctx, src, tools.AccessToken("invalid-header"))
		var csErr *util.ClientServerError
		if !errors.As(tbErr, &csErr) || csErr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 ClientServerError, got %T: %v", tbErr, tbErr)
		}
	})

	t.Run("server credentials success", func(t *testing.T) {
		t.Parallel()
		src := &stubSource{
			tokenSrc: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "server-token"}),
		}
		ts, tbErr := GetTokenSource(ctx, src, "")
		if tbErr != nil {
			t.Fatalf("unexpected error: %v", tbErr)
		}
		tok, err := ts.Token()
		if err != nil || tok.AccessToken != "server-token" {
			t.Fatalf("expected token 'server-token', got %v (err: %v)", tok, err)
		}
	})

	t.Run("server credentials error", func(t *testing.T) {
		t.Parallel()
		src := &stubSource{tokenErr: errors.New("adc failed")}
		_, tbErr := GetTokenSource(ctx, src, "")
		var csErr *util.ClientServerError
		if !errors.As(tbErr, &csErr) || csErr.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 ClientServerError, got %T: %v", tbErr, tbErr)
		}
	})

	t.Run("server credentials nil token source", func(t *testing.T) {
		t.Parallel()
		src := &stubSource{}
		_, tbErr := GetTokenSource(ctx, src, "")
		var csErr *util.ClientServerError
		if !errors.As(tbErr, &csErr) || csErr.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 ClientServerError, got %T: %v", tbErr, tbErr)
		}
	})
}

func TestDoGDARequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("POST with JSON body sets headers and decodes JSON response", func(t *testing.T) {
		t.Parallel()
		var gotContentType, gotClientHeader, gotBody string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotContentType = r.Header.Get("Content-Type")
			gotClientHeader = r.Header.Get("X-Goog-API-Client")
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"op-1","done":true}`))
		}))
		defer srv.Close()

		res, tbErr := DoGDARequest(ctx, srv.Client(), http.MethodPost, srv.URL, []byte(`{"displayName":"test"}`))
		if tbErr != nil {
			t.Fatalf("unexpected error: %v", tbErr)
		}
		if gotContentType != "application/json" {
			t.Fatalf("expected Content-Type 'application/json', got %q", gotContentType)
		}
		if gotClientHeader != util.GDAClientID {
			t.Fatalf("expected X-Goog-API-Client %q, got %q", util.GDAClientID, gotClientHeader)
		}
		if gotBody != `{"displayName":"test"}` {
			t.Fatalf("unexpected request body: %q", gotBody)
		}
		if res["name"] != "op-1" || res["done"] != true {
			t.Fatalf("unexpected decoded response: %v", res)
		}
	})

	t.Run("DELETE with nil body omits Content-Type and handles 204 empty body", func(t *testing.T) {
		t.Parallel()
		var gotContentType string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotContentType = r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		res, tbErr := DoGDARequest(ctx, srv.Client(), http.MethodDelete, srv.URL, nil)
		if tbErr != nil {
			t.Fatalf("unexpected error: %v", tbErr)
		}
		if gotContentType != "" {
			t.Fatalf("expected empty Content-Type for nil body, got %q", gotContentType)
		}
		if res["done"] != true {
			t.Fatalf("expected done=true for empty response body, got %v", res)
		}
	})

	t.Run("non-2xx response returns AgentError", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"bad request"}`))
		}))
		defer srv.Close()

		_, tbErr := DoGDARequest(ctx, srv.Client(), http.MethodPost, srv.URL, []byte(`{}`))
		var agentErr *util.AgentError
		if !errors.As(tbErr, &agentErr) || !strings.Contains(tbErr.Error(), "400") {
			t.Fatalf("expected AgentError with 400 status, got %T: %v", tbErr, tbErr)
		}
	})

	t.Run("malformed JSON and literal null response return ClientServerError", func(t *testing.T) {
		t.Parallel()
		badJSONSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{not-json`))
		}))
		defer badJSONSrv.Close()

		_, tbErr := DoGDARequest(ctx, badJSONSrv.Client(), http.MethodGet, badJSONSrv.URL, nil)
		var csErr *util.ClientServerError
		if !errors.As(tbErr, &csErr) {
			t.Fatalf("expected ClientServerError on malformed JSON, got %T: %v", tbErr, tbErr)
		}

		nullJSONSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`null`))
		}))
		defer nullJSONSrv.Close()

		_, tbErr = DoGDARequest(ctx, nullJSONSrv.Client(), http.MethodGet, nullJSONSrv.URL, nil)
		if !errors.As(tbErr, &csErr) {
			t.Fatalf("expected ClientServerError on literal null JSON, got %T: %v", tbErr, tbErr)
		}
	})
}
