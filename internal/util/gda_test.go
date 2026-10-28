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

package util

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/option"
)

func TestGetGDAEndpoint(t *testing.T) {
	tests := []struct {
		name             string
		useClientCert    string
		useMtlsEndpoint  string
		expectedEndpoint string
	}{
		{
			name:             "default behavior (no env vars)",
			useClientCert:    "",
			useMtlsEndpoint:  "",
			expectedEndpoint: gdaDefaultEndpoint,
		},
		{
			name:             "client cert enabled, mtls auto",
			useClientCert:    "true",
			useMtlsEndpoint:  "auto",
			expectedEndpoint: gdaMTLSEndpoint,
		},
		{
			name:             "client cert enabled, mtls unset",
			useClientCert:    "true",
			useMtlsEndpoint:  "",
			expectedEndpoint: gdaMTLSEndpoint,
		},
		{
			name:             "client cert disabled, mtls auto",
			useClientCert:    "false",
			useMtlsEndpoint:  "auto",
			expectedEndpoint: gdaDefaultEndpoint,
		},
		{
			name:             "client cert disabled, mtls always",
			useClientCert:    "false",
			useMtlsEndpoint:  "always",
			expectedEndpoint: gdaMTLSEndpoint,
		},
		{
			name:             "client cert enabled, mtls never",
			useClientCert:    "true",
			useMtlsEndpoint:  "never",
			expectedEndpoint: gdaDefaultEndpoint,
		},
		{
			name:             "client cert enabled, mtls invalid",
			useClientCert:    "true",
			useMtlsEndpoint:  "invalid-mode",
			expectedEndpoint: gdaMTLSEndpoint, // defaults to auto, so mtls endpoint
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set environment variables
			if tc.useClientCert != "" {
				os.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", tc.useClientCert)
			} else {
				os.Unsetenv("GOOGLE_API_USE_CLIENT_CERTIFICATE")
			}

			if tc.useMtlsEndpoint != "" {
				os.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", tc.useMtlsEndpoint)
			} else {
				os.Unsetenv("GOOGLE_API_USE_MTLS_ENDPOINT")
			}

			// Clean up env vars
			defer func() {
				os.Unsetenv("GOOGLE_API_USE_CLIENT_CERTIFICATE")
				os.Unsetenv("GOOGLE_API_USE_MTLS_ENDPOINT")
			}()

			if endpoint := GetGDAEndpoint(); endpoint != tc.expectedEndpoint {
				t.Errorf("expected endpoint %q, got %q", tc.expectedEndpoint, endpoint)
			}
		})
	}
}

func TestNewGDAClient(t *testing.T) {
	ctx := context.Background()

	// Should be able to create a client with no options (uses ADC)
	client, err := NewGDAClient(ctx, option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestValidateGDAPathSegment(t *testing.T) {
	t.Parallel()

	if err := ValidateGDAPathSegment("valid_id-1.2", "data_agent_id"); err != nil {
		t.Fatalf("expected valid path segment, got error: %v", err)
	}

	err := ValidateGDAPathSegment("", "data_agent_id")
	if err == nil || !strings.Contains(err.Error(), "is required and must be a non-empty string") {
		t.Fatalf("expected empty segment error, got: %v", err)
	}
	var agentErr *AgentError
	if !errors.As(err, &agentErr) {
		t.Fatalf("expected AgentError, got %T", err)
	}

	err = ValidateGDAPathSegment("../bad", "location")
	if err == nil || !strings.Contains(err.Error(), "contains disallowed characters") {
		t.Fatalf("expected disallowed characters error, got: %v", err)
	}
	if !errors.As(err, &agentErr) {
		t.Fatalf("expected AgentError, got %T", err)
	}
}

func TestAwaitGDAOperation(t *testing.T) {
	t.Parallel()

	t.Run("direct non-operation resource returned without done field", func(t *testing.T) {
		t.Parallel()
		initial := map[string]any{
			"name":        "projects/p1/locations/global/dataAgents/agent-1",
			"displayName": "Agent 1",
		}
		res, err := AwaitGDAOperation(context.Background(), http.DefaultClient, "https://example.com", initial, "creation", 5*time.Millisecond, 50*time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resMap, ok := res.(map[string]any)
		if !ok || resMap["name"] != "projects/p1/locations/global/dataAgents/agent-1" {
			t.Fatalf("unexpected result: %v", res)
		}
	})

	t.Run("context cancellation during polling", func(t *testing.T) {
		t.Parallel()
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"projects/p1/locations/global/operations/op-ctx","done":false}`))
		}))
		defer ts.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
		defer cancel()

		initial := map[string]any{
			"name": "projects/p1/locations/global/operations/op-ctx",
			"done": false,
		}
		_, err := AwaitGDAOperation(ctx, ts.Client(), ts.URL, initial, "creation", 10*time.Millisecond, 5*time.Second)
		if err == nil || !strings.Contains(err.Error(), "context cancelled") {
			t.Fatalf("expected context cancelled error, got: %v", err)
		}
		var csErr *ClientServerError
		if !errors.As(err, &csErr) {
			t.Fatalf("expected ClientServerError on context cancel, got %T: %v", err, err)
		}
	})

	t.Run("403 forbidden poll response returns ClientServerError", func(t *testing.T) {
		t.Parallel()
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"message":"permission denied"}}`))
		}))
		defer ts.Close()

		initial := map[string]any{
			"name": "projects/p1/locations/global/operations/op-403",
			"done": false,
		}
		_, err := AwaitGDAOperation(context.Background(), ts.Client(), ts.URL, initial, "update", 5*time.Millisecond, 100*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "polling failed with 403") {
			t.Fatalf("expected 403 error, got: %v", err)
		}
		var csErr *ClientServerError
		if !errors.As(err, &csErr) || csErr.Code != http.StatusForbidden {
			t.Fatalf("expected ClientServerError(403), got %T: %v", err, err)
		}
	})

	t.Run("malformed JSON poll response returns ClientServerError immediately", func(t *testing.T) {
		t.Parallel()
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{not-valid-json`))
		}))
		defer ts.Close()

		initial := map[string]any{
			"name": "projects/p1/locations/global/operations/op-bad-json",
			"done": false,
		}
		_, err := AwaitGDAOperation(context.Background(), ts.Client(), ts.URL, initial, "deletion", 5*time.Millisecond, 100*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "error decoding operation poll response") {
			t.Fatalf("expected malformed JSON error, got: %v", err)
		}
		var csErr *ClientServerError
		if !errors.As(err, &csErr) {
			t.Fatalf("expected ClientServerError on malformed JSON, got %T: %v", err, err)
		}
	})
}
