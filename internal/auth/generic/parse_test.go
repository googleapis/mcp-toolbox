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

package generic_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/auth"
	"github.com/googleapis/mcp-toolbox/internal/auth/generic"
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
			desc: "valid mcpEnabled false",
			in: `
			kind: authService
			name: my-generic-auth
			type: generic
			authorizationServer: https://example.com
			audience: my-audience
			mcpEnabled: false
			`,
			want: map[string]auth.AuthServiceConfig{
				"my-generic-auth": generic.Config{
					Name:                "my-generic-auth",
					Type:                "generic",
					AuthorizationServer: "https://example.com",
					Audience:            "my-audience",
					McpEnabled:          false,
				},
			},
		},
		{
			desc: "valid mcpEnabled true",
			in: `
			kind: authService
			name: my-generic-auth
			type: generic
			authorizationServer: https://example.com
			audience: my-audience
			introspectionEndpoint: https://example.com/introspect
			introspectionMethod: POST
			introspectionParamName: token
			scopesRequired:
			  - email
			mcpEnabled: true
			`,
			want: map[string]auth.AuthServiceConfig{
				"my-generic-auth": generic.Config{
					Name:                   "my-generic-auth",
					Type:                   "generic",
					AuthorizationServer:    "https://example.com",
					Audience:               "my-audience",
					IntrospectionEndpoint:  "https://example.com/introspect",
					IntrospectionMethod:    "POST",
					IntrospectionParamName: "token",
					ScopesRequired:         []string{"email"},
					McpEnabled:             true,
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
			desc: "introspectionEndpoint, mcpEnabled false",
			in: `
			kind: authService
			name: my-generic-auth
			type: generic
			authorizationServer: https://example.com
			audience: my-audience
			introspectionEndpoint: https://example.com/introspect
			mcpEnabled: false
			`,
			err: "`introspectionEndpoint` is not allowed when `mcpEnabled` is false",
		},
		{
			desc: "introspectionMethod, mcpEnabled false",
			in: `
			kind: authService
			name: my-generic-auth
			type: generic
			authorizationServer: https://example.com
			audience: my-audience
			introspectionMethod: POST
			mcpEnabled: false
			`,
			err: "`introspectionMethod` is not allowed when `mcpEnabled` is false",
		},
		{
			desc: "introspectionParamName, mcpEnabled false",
			in: `
			kind: authService
			name: my-generic-auth
			type: generic
			authorizationServer: https://example.com
			audience: my-audience
			introspectionParamName: token
			mcpEnabled: false
			`,
			err: "`introspectionParamName` is not allowed when `mcpEnabled` is false",
		},
		{
			desc: "scopesRequired, mcpEnabled false",
			in: `
			kind: authService
			name: my-generic-auth
			type: generic
			authorizationServer: https://example.com
			audience: my-audience
			scopesRequired:
			  - email
			mcpEnabled: false
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
