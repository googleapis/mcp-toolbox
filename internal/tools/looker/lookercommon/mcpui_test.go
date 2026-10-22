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

package lookercommon_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

func TestDiscoverMCPUIPublicOrigin(t *testing.T) {
	tcs := []struct {
		desc string
		html string
		want string
	}{
		{
			desc: "dashboard entry script",
			html: `<script type="text/javascript" nonce="abc" src="https://looker.example.com/webpack/en/mcp-render-dashboard.webpack.js?v=26.18.0"></script>`,
			want: "https://looker.example.com",
		},
		{
			desc: "visualization entry script with port and single quotes",
			html: `<script src='https://Looker.Example.com:8443/webpack/en/mcp-render-visualization.webpack.js'></script>`,
			want: "https://looker.example.com:8443",
		},
		{
			desc: "ignores unrelated scripts before the entry script",
			html: `<script src="https://cdn.example.com/other.js"></script>` +
				`<script src="https://looker.example.com/webpack/en/mcp-render-dashboard.webpack.js"></script>`,
			want: "https://looker.example.com",
		},
		{
			desc: "rejects http origins",
			html: `<script src="http://looker:19999/webpack/en/mcp-render-dashboard.webpack.js"></script>`,
			want: "",
		},
		{
			desc: "rejects relative src",
			html: `<script src="/webpack/en/mcp-render-dashboard.webpack.js"></script>`,
			want: "",
		},
		{
			desc: "no entry script",
			html: `<html><body>Looker Dashboard</body></html>`,
			want: "",
		},
		{
			desc: "rejects userinfo in https origin",
			html: `<script src="https://user:pass@looker.example.com/webpack/en/mcp-render-dashboard.webpack.js"></script>`,
			want: "",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if got := lookercommon.DiscoverMCPUIPublicOrigin(tc.html); got != tc.want {
				t.Errorf("DiscoverMCPUIPublicOrigin() = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("allowedOrigins filter", func(t *testing.T) {
		html := `<script src="https://looker.example.com/webpack/en/mcp-render-dashboard.webpack.js"></script>`
		if got := lookercommon.DiscoverMCPUIPublicOrigin(html, "https://looker.example.com"); got != "https://looker.example.com" {
			t.Errorf("expected allowed origin to match, got %q", got)
		}
		if got := lookercommon.DiscoverMCPUIPublicOrigin(html, "https://other.example.com"); got != "" {
			t.Errorf("expected mismatched allowed origin to be rejected, got %q", got)
		}
	})
}

func TestAddCSPOrigin(t *testing.T) {
	csp := &resources.CSPConfig{
		ResourceDomains: []string{"https://looker.example.com/"},
		ConnectDomains:  []string{"https://maps.googleapis.com"},
	}

	lookercommon.AddCSPOrigin(csp, "https://looker.example.com")
	lookercommon.AddCSPOrigin(csp, "https://looker.example.com")
	lookercommon.AddCSPOrigin(csp, "http://localhost:9999/")
	lookercommon.AddCSPOrigin(csp, "")
	lookercommon.AddCSPOrigin(nil, "https://ignored.example.com")

	want := &resources.CSPConfig{
		ResourceDomains: []string{"https://looker.example.com/", "http://localhost:9999"},
		ConnectDomains: []string{
			"https://maps.googleapis.com",
			"https://looker.example.com",
			"wss://looker.example.com",
			"http://localhost:9999",
			"ws://localhost:9999",
		},
	}
	if diff := cmp.Diff(want, csp); diff != "" {
		t.Errorf("AddCSPOrigin() mismatch (-want +got):\n%s", diff)
	}
}

func TestGoogleMapsCSPDomains(t *testing.T) {
	csp := lookercommon.DefaultMCPUICSPConfig()
	if csp == nil {
		t.Fatalf("expected non-nil CSPConfig")
	}

	requiredGoogleMapsResourceDomains := []string{
		"https://maps.googleapis.com",
		"https://mapsresources-pa.googleapis.com",
		"https://streetviewpixels-pa.googleapis.com",
		"https://khms0.googleapis.com",
		"https://khms1.googleapis.com",
		"https://maps.gstatic.com",
		"https://*.gstatic.com",
		"https://*.ggpht.com",
		"https://fonts.googleapis.com",
		"https://fonts.gstatic.com",
	}

	requiredGoogleMapsConnectDomains := []string{
		"https://maps.googleapis.com",
		"https://mapsresources-pa.googleapis.com",
		"https://streetviewpixels-pa.googleapis.com",
		"https://khms0.googleapis.com",
		"https://khms1.googleapis.com",
		"https://maps.gstatic.com",
		"https://*.ggpht.com",
		"https://fonts.googleapis.com",
		"https://fonts.gstatic.com",
	}

	containsDomain := func(list []string, target string) bool {
		for _, item := range list {
			if item == target {
				return true
			}
		}
		return false
	}

	for _, domain := range requiredGoogleMapsResourceDomains {
		if !containsDomain(lookercommon.GoogleMapsResourceDomains, domain) {
			t.Errorf("expected GoogleMapsResourceDomains to contain %q", domain)
		}
		if !containsDomain(csp.ResourceDomains, domain) {
			t.Errorf("expected DefaultMCPUICSPConfig().ResourceDomains to contain %q", domain)
		}
	}

	for _, domain := range requiredGoogleMapsConnectDomains {
		if !containsDomain(lookercommon.GoogleMapsConnectDomains, domain) {
			t.Errorf("expected GoogleMapsConnectDomains to contain %q", domain)
		}
		if !containsDomain(csp.ConnectDomains, domain) {
			t.Errorf("expected DefaultMCPUICSPConfig().ConnectDomains to contain %q", domain)
		}
	}

	for _, disallowed := range []string{"https://*.googleapis.com", "https://*.googleusercontent.com"} {
		if containsDomain(lookercommon.GoogleMapsResourceDomains, disallowed) || containsDomain(csp.ResourceDomains, disallowed) {
			t.Errorf("did not expect ResourceDomains to contain wildcard %q", disallowed)
		}
		if containsDomain(lookercommon.GoogleMapsConnectDomains, disallowed) || containsDomain(csp.ConnectDomains, disallowed) {
			t.Errorf("did not expect ConnectDomains to contain wildcard %q", disallowed)
		}
	}
	if containsDomain(lookercommon.GoogleMapsConnectDomains, "https://*.gstatic.com") || containsDomain(csp.ConnectDomains, "https://*.gstatic.com") {
		t.Errorf("did not expect ConnectDomains to contain wildcard https://*.gstatic.com")
	}
}

func TestBuildLookerUICSPHostnameCheck(t *testing.T) {
	t.Setenv("LOOKER_BASE_URL", "https://localhost.evil.example.com")
	t.Setenv("LOOKER_UI_URL", "https://looker.example.com/localhost")

	csp := lookercommon.BuildLookerUICSP()
	for _, d := range csp.ResourceDomains {
		if d == "http://localhost:9999" || d == "http://127.0.0.1:9999" {
			t.Errorf("did not expect local dev ports for non-loopback hostname, got %q", d)
		}
	}

	t.Setenv("LOOKER_BASE_URL", "http://localhost:19999")
	t.Setenv("LOOKER_UI_URL", "http://localhost:9999")
	localCsp := lookercommon.BuildLookerUICSP()
	foundLocal := false
	for _, d := range localCsp.ResourceDomains {
		if d == "http://localhost:3035" {
			foundLocal = true
			break
		}
	}
	if !foundLocal {
		t.Errorf("expected http://localhost:3035 in local dev CSP ResourceDomains")
	}
}


func TestFetchMcpAppStateCachesColorCollectionsPerInstanceOnly(t *testing.T) {
	lookercommon.ClearMcpAppStateCacheForTesting()
	defer lookercommon.ClearMcpAppStateCacheForTesting()

	var userCalls int32
	var colorCalls int32
	var mapKeyCalls int32
	var lastAssetRawQuery atomic.Value

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/4.0/login":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/api/4.0/user":
			w.Header().Set("Content-Type", "application/json")
			callNum := atomic.AddInt32(&userCalls, 1)
			locale := "en"
			if callNum == 2 {
				locale = "ja_JP"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "1",
				"locale": locale,
				"abilities": map[string]any{
					"objects": map[string]any{
						"query": map[string]any{"can": map[string]any{"explore": true, "run": true}},
					},
				},
			})
		case "/api/4.0/color_collections/active":
			w.Header().Set("Content-Type", "application/json")
			atomic.AddInt32(&colorCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"default_collection_id": "b43731d5-dc87-4a8e-b807-635bef3948e7",
				"default_colors":        []string{"#FBBC04", "#EA4335"},
				"custom":                []any{},
				"standard":              []any{map[string]any{"id": "b43731d5-dc87-4a8e-b807-635bef3948e7", "label": "Boardwalk"}},
			})
		case "/public/mcp/ui/map_api_key":
			w.Header().Set("Content-Type", "application/json")
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": "Unauthorized"})
				return
			}
			atomic.AddInt32(&mapKeyCalls, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"api_key": "test-gmaps-key-123",
			})
		case "/public/mcp/ui/render_visualization/assets":
			lastAssetRawQuery.Store(r.URL.RawQuery)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><div id="root"></div></body></html>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	settings := rtl.ApiSettings{
		BaseUrl:      ts.URL,
		ApiVersion:   "4.0",
		ClientId:     "id",
		ClientSecret: "secret",
	}
	sdk := v4.NewLookerSDK(rtl.NewAuthSession(settings))

	// First call fetches /user, /color_collections/active, and /public/mcp/ui/map_api_key
	state1 := lookercommon.FetchMcpAppState(context.Background(), sdk, &settings)
	if state1 == nil {
		t.Fatalf("expected non-nil app_state on first call")
	}
	if got := atomic.LoadInt32(&userCalls); got != 1 {
		t.Errorf("userCalls after call 1 = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&colorCalls); got != 1 {
		t.Errorf("colorCalls after call 1 = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&mapKeyCalls); got != 1 {
		t.Errorf("mapKeyCalls after call 1 = %d, want 1", got)
	}
	if got, _ := state1["map_api_key"].(string); got != "test-gmaps-key-123" {
		t.Errorf("state1[map_api_key] = %q, want test-gmaps-key-123", got)
	}
	if user1, ok := state1["user"].(map[string]any); !ok || user1["locale"] != "en" {
		t.Errorf("state1[user][locale] = %v, want en", state1["user"])
	}

	// Second call fetches /user again (not cached) but reuses cached /color_collections/active and /public/mcp/ui/map_api_key
	state2 := lookercommon.FetchMcpAppState(context.Background(), sdk, &settings)
	if state2 == nil {
		t.Fatalf("expected non-nil app_state on second call")
	}
	if got := atomic.LoadInt32(&userCalls); got != 2 {
		t.Errorf("userCalls after call 2 = %d, want 2", got)
	}
	if got := atomic.LoadInt32(&colorCalls); got != 1 {
		t.Errorf("colorCalls after call 2 = %d, want 1 (should be cached)", got)
	}
	if got := atomic.LoadInt32(&mapKeyCalls); got != 1 {
		t.Errorf("mapKeyCalls after call 2 = %d, want 1 (should be cached)", got)
	}
	if got, _ := state2["map_api_key"].(string); got != "test-gmaps-key-123" {
		t.Errorf("state2[map_api_key] = %q, want test-gmaps-key-123", got)
	}
	if user2, ok := state2["user"].(map[string]any); !ok || user2["locale"] != "ja_JP" {
		t.Errorf("state2[user][locale] = %v, want ja_JP", state2["user"])
	}

	// Verify FetchRemoteUI does not leak User 2's locale into shared resources/read asset fetches
	t.Setenv("LOOKER_UI_URL", ts.URL)
	mgr := lookercommon.NewLookerUIResourceManager(
		"looker_vis",
		"ui://looker/render_visualization.html",
		"desc",
		"render_visualization",
		"render_dashboard",
		"Looker Visualization",
	)
	if _, err := mgr.FetchRemoteUI(context.Background()); err != nil {
		t.Fatalf("FetchRemoteUI() failed: %v", err)
	}
	if gotQuery, _ := lastAssetRawQuery.Load().(string); gotQuery != "" {
		t.Errorf("expected FetchRemoteUI not to leak cross-user locale query param, got %q", gotQuery)
	}
}

func TestCacheConstantsAndResolveVerifySSL(t *testing.T) {
	if lookercommon.ColorCollectionsCacheTTL != 10*time.Minute {
		t.Errorf("ColorCollectionsCacheTTL = %v, want 10m", lookercommon.ColorCollectionsCacheTTL)
	}
	if lookercommon.MapApiKeyCacheTTL != 10*time.Minute {
		t.Errorf("MapApiKeyCacheTTL = %v, want 10m", lookercommon.MapApiKeyCacheTTL)
	}

	ctx := context.Background()
	t.Setenv("LOOKER_VERIFY_SSL", "")
	if !lookercommon.ResolveVerifySSL(ctx, nil, "test") {
		t.Errorf("expected default VerifySSL=true when env var is empty")
	}
	t.Setenv("LOOKER_VERIFY_SSL", "false")
	if lookercommon.ResolveVerifySSL(ctx, nil, "test") {
		t.Errorf("expected VerifySSL=false when LOOKER_VERIFY_SSL=false")
	}
	t.Setenv("LOOKER_VERIFY_SSL", "0")
	if lookercommon.ResolveVerifySSL(ctx, nil, "test") {
		t.Errorf("expected VerifySSL=false when LOOKER_VERIFY_SSL=0")
	}
	if !lookercommon.ResolveVerifySSL(ctx, &rtl.ApiSettings{VerifySsl: true}, "test") {
		t.Errorf("expected apiSettings.VerifySsl=true to take precedence over env var")
	}
}

func TestRenderHTMLStripsSingleAndDoubleQuotedNonces(t *testing.T) {
	mgr := lookercommon.NewLookerUIResourceManager(
		"looker_vis",
		"ui://looker/render_visualization.html",
		"desc",
		"render_visualization",
		"render_dashboard",
		"Looker Visualization",
	)
	rawHTML := `<!DOCTYPE html><html><head><base href="https://evil.example.com/"><script nonce='single-nonce-123'>window.__LOOKER_BASE_URL__="https://looker.example.com";</script></head><body><script type="text/javascript" nonce="double-nonce-456" src="/webpack/en/mcp-render-visualization.webpack.js"></script></body></html>`
	rendered := mgr.RenderHTML(rawHTML, "https://looker.example.com")

	if got := rendered; got != `<!DOCTYPE html><html><head><script>window.__LOOKER_BASE_URL__="https://looker.example.com";</script></head><body><script type="text/javascript" src="https://looker.example.com/webpack/en/mcp-render-visualization.webpack.js"></script></body></html>` {
		t.Errorf("RenderHTML() = %q", got)
	}
}

func TestFetchMapApiKeyDoesNotLeakTokenToCrossHostLookerUIURL(t *testing.T) {
	lookercommon.ClearMcpAppStateCacheForTesting()
	defer lookercommon.ClearMcpAppStateCacheForTesting()

	var crossHostCalls int32
	crossHostServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&crossHostCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"api_key": "evil-key"})
	}))
	defer crossHostServer.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/4.0/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "secret-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/api/4.0/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "1", "locale": "en"})
		case "/api/4.0/color_collections/active":
			_ = json.NewEncoder(w).Encode(map[string]any{"default_collection_id": "default"})
		case "/public/mcp/ui/map_api_key":
			_ = json.NewEncoder(w).Encode(map[string]any{"api_key": "trusted-base-key"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// Set LOOKER_UI_URL to an external non-loopback host that differs from ts.URL (127.0.0.1)
	t.Setenv("LOOKER_UI_URL", "https://untrusted-external.example.com")

	settings := rtl.ApiSettings{
		BaseUrl:      ts.URL,
		ApiVersion:   "4.0",
		ClientId:     "id",
		ClientSecret: "secret",
	}
	sdk := v4.NewLookerSDK(rtl.NewAuthSession(settings))

	state := lookercommon.FetchMcpAppState(context.Background(), sdk, &settings)
	if got := atomic.LoadInt32(&crossHostCalls); got != 0 {
		t.Errorf("expected 0 calls to cross-host server, got %d", got)
	}
	if got, _ := state["map_api_key"].(string); got != "trusted-base-key" {
		t.Errorf("state[map_api_key] = %q, want trusted-base-key", got)
	}
}

func TestDiscoverMCPUIPublicOriginWildcardAllowlist(t *testing.T) {
	cdnHTML := `<script src="https://static-a.lookercdn.com/webpack/en/mcp-render-dashboard.webpack.js"></script>`
	if got := lookercommon.DiscoverMCPUIPublicOrigin(cdnHTML, "https://looker.example.com", "https://*.lookercdn.com"); got != "https://static-a.lookercdn.com" {
		t.Errorf("expected wildcard CDN allowlist to match, got %q", got)
	}

	appCDNHTML := `<script src="https://assets.cdn.looker.app/webpack/en/mcp-render-visualization.webpack.js"></script>`
	if got := lookercommon.DiscoverMCPUIPublicOrigin(appCDNHTML, "https://looker.example.com", "https://*.cdn.looker.app"); got != "https://assets.cdn.looker.app" {
		t.Errorf("expected *.cdn.looker.app wildcard allowlist to match, got %q", got)
	}

	evilHTML := `<script src="https://evil-lookercdn.com/webpack/en/mcp-render-dashboard.webpack.js"></script>`
	if got := lookercommon.DiscoverMCPUIPublicOrigin(evilHTML, "https://looker.example.com", "https://*.lookercdn.com"); got != "" {
		t.Errorf("expected evil-lookercdn.com to be rejected, got %q", got)
	}

	bareRootHTML := `<script src="https://lookercdn.com/webpack/en/mcp-render-dashboard.webpack.js"></script>`
	if got := lookercommon.DiscoverMCPUIPublicOrigin(bareRootHTML, "https://looker.example.com", "https://*.lookercdn.com"); got != "" {
		t.Errorf("expected bare root lookercdn.com not to match *.lookercdn.com wildcard, got %q", got)
	}
}

func TestFetchRemoteUIOriginValidationAndConcurrentCSPSafety(t *testing.T) {
	// Case 1: Untrusted script src is rejected when LOOKER_BASE_URL is configured;
	// authoritative LOOKER_BASE_URL origin is added to both ResourceDomains and ConnectDomains.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><script>window.__LOOKER_BASE_URL__="https://tenant.looker.com";</script></head><body><script src="https://evil.attacker.example/webpack/en/mcp-render-visualization.webpack.js"></script></body></html>`))
	}))
	defer ts.Close()

	t.Setenv("LOOKER_BASE_URL", ts.URL)
	t.Setenv("LOOKER_UI_URL", ts.URL)

	mgr := lookercommon.NewLookerUIResourceManager(
		"looker_vis_test",
		"ui://looker/render_visualization.html",
		"desc",
		"render_visualization",
		"render_dashboard",
		"Looker Visualization",
	)
	resList := mgr.GetResources()
	if len(resList) != 1 {
		t.Fatalf("expected 1 UI resource, got %d", len(resList))
	}
	res, ok := resList[0].(*resources.DynamicUIResource)
	if !ok {
		t.Fatalf("expected *resources.DynamicUIResource")
	}

	// Run concurrent FetchRemoteUI and GetResourceUIMetadata / CSP reads to verify race-free CSP updates
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = mgr.FetchRemoteUI(context.Background())
		}()
		go func() {
			defer wg.Done()
			_ = res.GetResourceUIMetadata()
			_ = mgr.CSP()
		}()
	}
	wg.Wait()

	csp := mgr.CSP()
	for _, domain := range csp.ResourceDomains {
		if domain == "https://evil.attacker.example" {
			t.Errorf("untrusted script origin was added to ResourceDomains: %v", csp.ResourceDomains)
		}
	}
	for _, domain := range csp.ConnectDomains {
		if domain == "https://evil.attacker.example" {
			t.Errorf("untrusted script origin was added to ConnectDomains: %v", csp.ConnectDomains)
		}
	}

	foundConnectTenant := false
	for _, domain := range csp.ConnectDomains {
		if domain == "https://tenant.looker.com" {
			foundConnectTenant = true
			break
		}
	}
	if !foundConnectTenant {
		t.Errorf("expected authoritative window.__LOOKER_BASE_URL__ origin https://tenant.looker.com in ConnectDomains, got %v", csp.ConnectDomains)
	}
}

type perUserLocaleMockSource struct {
	settings  *rtl.ApiSettings
	sdkByAuth map[string]*v4.LookerSDK
}

func (m perUserLocaleMockSource) UseClientAuthorization() bool        { return true }
func (m perUserLocaleMockSource) GetAuthTokenHeaderName() string      { return "Authorization" }
func (m perUserLocaleMockSource) LookerApiSettings() *rtl.ApiSettings { return m.settings }
func (m perUserLocaleMockSource) GetLookerSDK(_ context.Context, token string) (*v4.LookerSDK, error) {
	return m.sdkByAuth[token], nil
}

func TestFetchRemoteUIPerUserLocaleResolutionAndIsolation(t *testing.T) {
	lookercommon.ClearMcpAppStateCacheForTesting()
	defer lookercommon.ClearMcpAppStateCacheForTesting()

	var userAPICalls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/4.0/login":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": r.FormValue("client_id"),
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case "/api/4.0/user":
			atomic.AddInt32(&userAPICalls, 1)
			w.Header().Set("Content-Type", "application/json")
			locale := "en"
			if r.Header.Get("Authorization") == "Bearer ja-session" {
				locale = "ja_JP"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "1", "locale": locale})
		case "/public/mcp/ui/render_visualization/assets":
			w.Header().Set("Content-Type", "text/html")
			loc := r.URL.Query().Get("locale")
			if loc == "" {
				loc = "en"
			}
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><script>window.__LOOKER_LOCALE__="` + loc + `";</script></head><body><script src="/webpack/` + loc + `/mcp-render-visualization.webpack.js"></script></body></html>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	t.Setenv("LOOKER_UI_URL", ts.URL)

	jaSettings := rtl.ApiSettings{BaseUrl: ts.URL, ApiVersion: "4.0", ClientId: "ja-session", ClientSecret: "s"}
	enSettings := rtl.ApiSettings{BaseUrl: ts.URL, ApiVersion: "4.0", ClientId: "en-session", ClientSecret: "s"}

	mgr := lookercommon.NewLookerUIResourceManager(
		"looker_vis_locale_test",
		"ui://looker/render_visualization.html",
		"desc",
		"render_visualization",
		"render_dashboard",
		"Looker Visualization",
	)
	mgr.SetSource(perUserLocaleMockSource{
		settings: &jaSettings,
		sdkByAuth: map[string]*v4.LookerSDK{
			"Bearer token-ja": v4.NewLookerSDK(rtl.NewAuthSession(jaSettings)),
			"Bearer token-en": v4.NewLookerSDK(rtl.NewAuthSession(enSettings)),
		},
	})

	jaCtx := util.WithRequestHeader(context.Background(), http.Header{"Authorization": []string{"Bearer token-ja"}})
	enCtx := util.WithRequestHeader(context.Background(), http.Header{"Authorization": []string{"Bearer token-en"}})

	jaHTML, err := mgr.FetchRemoteUI(jaCtx)
	if err != nil {
		t.Fatalf("FetchRemoteUI(jaCtx) failed: %v", err)
	}
	if want := `/webpack/ja_JP/mcp-render-visualization.webpack.js`; !cmp.Equal(true, json.Valid([]byte(`"ok"`)) && len(jaHTML) > 0 && containsSubstr(jaHTML, want)) {
		t.Errorf("expected jaHTML to contain %q, got %q", want, jaHTML)
	}

	// Second call for same user uses cached locale + cached per-locale HTML (zero extra /api/4.0/user calls)
	if _, err := mgr.FetchRemoteUI(jaCtx); err != nil {
		t.Fatalf("second FetchRemoteUI(jaCtx) failed: %v", err)
	}
	if got := atomic.LoadInt32(&userAPICalls); got != 1 {
		t.Errorf("expected 1 /api/4.0/user call after cached repeat read, got %d", got)
	}

	// Concurrent English user receives English HTML rather than cached Japanese HTML
	enHTML, err := mgr.FetchRemoteUI(enCtx)
	if err != nil {
		t.Fatalf("FetchRemoteUI(enCtx) failed: %v", err)
	}
	if want := `/webpack/en/mcp-render-visualization.webpack.js`; !containsSubstr(enHTML, want) {
		t.Errorf("expected enHTML to contain %q, got %q", want, enHTML)
	}
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}





