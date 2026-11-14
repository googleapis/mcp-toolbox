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

package looker_test

import (
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/looker"
)

// TestLookerSource_ImplementsOAuthProxySource is a regression guard: if
// looker.Source ever stops implementing sources.OAuthProxySource (e.g. a
// method gets renamed), the toolbox's OAuth discovery/token proxy silently
// stops applying to Looker sources with client-side OAuth enabled, with no
// other test catching it.
//
// This constructs a *looker.Source directly (rather than going through
// Config.Initialize) so the test exercises exactly the three delegate
// methods under test, with no dependency on a real or fake Looker backend.
func TestLookerSource_ImplementsOAuthProxySource(t *testing.T) {
	src := &looker.Source{
		Config: looker.Config{
			Name:           "test-looker",
			Type:           "looker",
			BaseURL:        "https://looker.example.com",
			UseClientOAuth: "true",
		},
	}

	proxySrc, ok := sources.Source(src).(sources.OAuthProxySource)
	if !ok {
		t.Fatalf("*looker.Source no longer implements sources.OAuthProxySource")
	}
	if !proxySrc.ClientOAuthEnabled() {
		t.Errorf("ClientOAuthEnabled() = false, want true for use_client_oauth: \"true\"")
	}
	if got, want := proxySrc.OAuthAuthorizationEndpoint(), "https://looker.example.com/auth"; got != want {
		t.Errorf("OAuthAuthorizationEndpoint() = %q, want %q", got, want)
	}
	if got, want := proxySrc.OAuthTokenEndpoint(), "https://looker.example.com/api/token"; got != want {
		t.Errorf("OAuthTokenEndpoint() = %q, want %q", got, want)
	}
}

// TestLookerSource_OAuthEndpoints_TrimsTrailingSlash guards against a
// double slash (https://host//auth) if the operator's configured base_url
// happens to end in "/".
func TestLookerSource_OAuthEndpoints_TrimsTrailingSlash(t *testing.T) {
	src := &looker.Source{
		Config: looker.Config{
			Name:           "test-looker",
			Type:           "looker",
			BaseURL:        "https://looker.example.com/",
			UseClientOAuth: "true",
		},
	}

	proxySrc := sources.Source(src).(sources.OAuthProxySource)
	if got, want := proxySrc.OAuthAuthorizationEndpoint(), "https://looker.example.com/auth"; got != want {
		t.Errorf("OAuthAuthorizationEndpoint() = %q, want %q (no double slash)", got, want)
	}
	if got, want := proxySrc.OAuthTokenEndpoint(), "https://looker.example.com/api/token"; got != want {
		t.Errorf("OAuthTokenEndpoint() = %q, want %q (no double slash)", got, want)
	}
}

func TestLookerSource_ClientOAuthDisabled(t *testing.T) {
	src := &looker.Source{
		Config: looker.Config{
			Name:           "test-looker",
			Type:           "looker",
			BaseURL:        "https://looker.example.com",
			UseClientOAuth: "false",
		},
	}

	proxySrc, ok := sources.Source(src).(sources.OAuthProxySource)
	if !ok {
		t.Fatalf("*looker.Source no longer implements sources.OAuthProxySource")
	}
	if proxySrc.ClientOAuthEnabled() {
		t.Errorf("ClientOAuthEnabled() = true, want false for use_client_oauth: \"false\"")
	}
}

// TestLookerSource_ClientOAuthCustomHeaderName covers the third
// use_client_oauth mode: a non-"true"/"false" value names the header the
// caller's token is expected in, rather than enabling or disabling client
// OAuth outright. This still counts as client-OAuth-enabled for the
// purposes of the proxy, since the caller still supplies a per-request
// token rather than the source using a shared credential.
func TestLookerSource_ClientOAuthCustomHeaderName(t *testing.T) {
	src := &looker.Source{
		Config: looker.Config{
			Name:           "test-looker",
			Type:           "looker",
			BaseURL:        "https://looker.example.com",
			UseClientOAuth: "X-Custom-Auth",
		},
	}

	proxySrc, ok := sources.Source(src).(sources.OAuthProxySource)
	if !ok {
		t.Fatalf("*looker.Source no longer implements sources.OAuthProxySource")
	}
	if !proxySrc.ClientOAuthEnabled() {
		t.Errorf("ClientOAuthEnabled() = false, want true for a custom header name value")
	}
}
