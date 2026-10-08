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

package google_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/auth"
	"github.com/googleapis/mcp-toolbox/internal/auth/google"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYaml(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.AuthServiceConfigs
	}{
		{
			desc: "only clientId, mcpEnabled false",
			in: `
			kind: authService
			name: my-google-auth
			type: google
			clientId: my-client-id
			`,
			want: map[string]auth.AuthServiceConfig{
				"my-google-auth": google.Config{
					Name:       "my-google-auth",
					Type:       "google",
					ClientID:   "my-client-id",
					McpEnabled: false,
				},
			},
		},
		{
			desc: "only audience, mcpEnabled true",
			in: `
			kind: authService
			name: my-google-auth
			type: google
			audience: my-audience
			mcpEnabled: true
			`,
			want: map[string]auth.AuthServiceConfig{
				"my-google-auth": google.Config{
					Name:       "my-google-auth",
					Type:       "google",
					Audience:   "my-audience",
					McpEnabled: true,
				},
			},
		},
		{
			desc: "scopesRequired, mcpEnabled true",
			in: `
			kind: authService
			name: my-google-auth
			type: google
			scopesRequired:
			  - email
			mcpEnabled: true
			`,
			want: map[string]auth.AuthServiceConfig{
				"my-google-auth": google.Config{
					Name:           "my-google-auth",
					Type:           "google",
					ScopesRequired: []string{"email"},
					McpEnabled:     true,
				},
			},
		},
		{
			desc: "both clientId and audience, mcpEnabled true",
			in: `
			kind: authService
			name: my-google-auth
			type: google
			clientId: my-client-id
			audience: my-audience
			mcpEnabled: true
			`,
			want: map[string]auth.AuthServiceConfig{
				"my-google-auth": google.Config{
					Name:       "my-google-auth",
					Type:       "google",
					ClientID:   "my-client-id",
					Audience:   "my-audience",
					McpEnabled: true,
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, got, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
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
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "only audience, mcpEnabled false",
			in: `
			kind: authService
			name: my-google-auth
			type: google
			audience: my-audience
			`,
			err: "`audience` is not allowed when `mcpEnabled` is false",
		},
		{
			desc: "scopesRequired, mcpEnabled false",
			in: `
			kind: authService
			name: my-google-auth
			type: google
			scopesRequired:
			  - email
			`,
			err: "`scopesRequired` is not allowed when `mcpEnabled` is false",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.err) {
				t.Fatalf("unexpected error: got %q, want it to contain %q", errStr, tc.err)
			}
		})
	}
}
