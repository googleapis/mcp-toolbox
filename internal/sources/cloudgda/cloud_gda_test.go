// Copyright 2025 Google LLC
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

package cloudgda_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/cloudgda"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestParseFromYamlCloudGDA(t *testing.T) {
	t.Parallel()
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-gda-instance
			type: cloud-gemini-data-analytics
			projectId: test-project-id
			`,
			want: map[string]sources.SourceConfig{
				"my-gda-instance": cloudgda.Config{
					Name:           "my-gda-instance",
					Type:           cloudgda.SourceType,
					ProjectID:      "test-project-id",
					UseClientOAuth: false,
				},
			},
		},
		{
			desc: "use client auth example",
			in: `
			kind: source
			name: my-gda-instance
			type: cloud-gemini-data-analytics
			projectId: another-project
			useClientOAuth: true
			`,
			want: map[string]sources.SourceConfig{
				"my-gda-instance": cloudgda.Config{
					Name:           "my-gda-instance",
					Type:           cloudgda.SourceType,
					ProjectID:      "another-project",
					UseClientOAuth: true,
				},
			},
		},
	}
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			got, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse: want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestFailParseFromYaml(t *testing.T) {
	t.Parallel()
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "missing projectId",
			in: `
			kind: source
			name: my-gda-instance
			type: cloud-gemini-data-analytics
			`,
			err: "error unmarshaling source: unable to parse source \"my-gda-instance\" as \"cloud-gemini-data-analytics\": Key: 'Config.ProjectID' Error:Field validation for 'ProjectID' failed on the 'required' tag",
		},
	}
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if errStr != tc.err {
				t.Fatalf("unexpected error: got %q, want %q", errStr, tc.err)
			}
		})
	}
}

func TestInitialize(t *testing.T) {
	// Create a dummy credentials file for testing ADC
	credFile := filepath.Join(t.TempDir(), "application_default_credentials.json")
	dummyCreds := `{
		"client_id": "foo",
		"client_secret": "bar",
		"refresh_token": "baz",
		"type": "authorized_user"
	}`
	if err := os.WriteFile(credFile, []byte(dummyCreds), 0644); err != nil {
		t.Fatalf("failed to write dummy credentials file: %v", err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credFile)

	// Use ContextWithUserAgent to avoid "unable to retrieve user agent" error
	ctx := testutils.ContextWithUserAgent(context.Background(), "test-user-agent")
	tracer := noop.NewTracerProvider().Tracer("test")

	tcs := []struct {
		desc            string
		cfg             cloudgda.Config
		wantClientOAuth bool
	}{
		{
			desc:            "initialize with ADC",
			cfg:             cloudgda.Config{Name: "test-gda", Type: cloudgda.SourceType, ProjectID: "test-proj"},
			wantClientOAuth: false,
		},
		{
			desc:            "initialize with client OAuth",
			cfg:             cloudgda.Config{Name: "test-gda-oauth", Type: cloudgda.SourceType, ProjectID: "test-proj", UseClientOAuth: true},
			wantClientOAuth: true,
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			src, err := tc.cfg.Initialize(ctx, tracer)
			if err != nil {
				t.Fatalf("failed to initialize source: %v", err)
			}

			gdaSrc, ok := src.(*cloudgda.Source)
			if !ok {
				t.Fatalf("expected *cloudgda.Source, got %T", src)
			}

			// Check that the client is non-nil
			if gdaSrc.Client == nil && !tc.wantClientOAuth {
				t.Fatal("expected non-nil HTTP client for ADC, got nil")
			}
			// When client OAuth is true, the source's client should be nil.
			if gdaSrc.Client != nil && tc.wantClientOAuth {
				t.Fatal("expected nil HTTP client for client OAuth config, got non-nil")
			}

			// Test UseClientAuthorization method
			if gdaSrc.UseClientAuthorization() != tc.wantClientOAuth {
				t.Errorf("UseClientAuthorization mismatch: want %t, got %t", tc.wantClientOAuth, gdaSrc.UseClientAuthorization())
			}

			// Test GetClient with accessToken for client OAuth scenarios
			if tc.wantClientOAuth {
				client, cleanup, err := gdaSrc.GetClient(ctx, "dummy-token")
				if err != nil {
					t.Fatalf("GetClient with token failed: %v", err)
				}
				defer cleanup()
				if client == nil {
					t.Fatal("expected non-nil HTTP client from GetClient with token, got nil")
				}
				// Ensure passing empty token with UseClientOAuth enabled returns error
				_, _, err = gdaSrc.GetClient(ctx, "")
				if err == nil || err.Error() != "client-side OAuth is enabled but no access token was provided" {
					t.Errorf("expected 'client-side OAuth is enabled but no access token was provided' error, got: %v", err)
				}
			}
		})
	}
}
