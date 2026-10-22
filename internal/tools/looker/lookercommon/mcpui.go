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

package lookercommon

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

const (
	ColorCollectionsCacheTTL  = 10 * time.Minute
	MapApiKeyCacheTTL         = 10 * time.Minute
	uiHTMLCacheTTL            = 5 * time.Minute
	mcpUserFields             = "id,email,display_name,locale,abilities,can"
	maxMapApiKeyResponseBytes = 64 * 1024        // 64 KiB
	maxUIHTMLResponseBytes    = 10 * 1024 * 1024 // 10 MiB
)

type cachedColorCollections struct {
	data      map[string]any
	fetchedAt time.Time
}

type cachedMapApiKey struct {
	apiKey    string
	fetchedAt time.Time
}

type cachedUserLocale struct {
	locale    string
	fetchedAt time.Time
}

type cachedUIHTML struct {
	html      string
	fetchedAt time.Time
}

const (
	clientUserLocaleCacheTTL = 5 * time.Minute
	staticUserLocaleCacheTTL = 10 * time.Second
)

var (
	mcpAppStateCacheMu    sync.RWMutex
	colorCollectionsCache = make(map[string]cachedColorCollections)
	mapApiKeyCache        = make(map[string]cachedMapApiKey)
	userLocaleCache       = make(map[string]cachedUserLocale)
	strictLookerLocaleRe  = regexp.MustCompile(`^[a-zA-Z]{2,3}(?:[-_][a-zA-Z]{2,4})?$`)
)

func resolveInstanceCacheKey(settings *rtl.ApiSettings) string {
	if settings != nil && strings.TrimSpace(settings.BaseUrl) != "" {
		return strings.TrimSuffix(strings.TrimSpace(settings.BaseUrl), "/")
	}
	if envURL := strings.TrimSpace(os.Getenv("LOOKER_BASE_URL")); envURL != "" {
		return strings.TrimSuffix(envURL, "/")
	}
	return "default"
}

func isLoopbackHostname(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

func isSameOrLoopbackHost(hostA, hostB string) bool {
	if strings.EqualFold(hostA, hostB) {
		return true
	}
	return isLoopbackHostname(hostA) && isLoopbackHostname(hostB)
}

func resolveMapApiKeyEndpoint(settings *rtl.ApiSettings) string {
	var rawBase string
	if settings != nil {
		rawBase = strings.TrimSpace(settings.BaseUrl)
	}
	if rawBase == "" {
		rawBase = strings.TrimSpace(os.Getenv("LOOKER_BASE_URL"))
	}

	var baseParsed *url.URL
	if rawBase != "" {
		if p, err := url.Parse(rawBase); err == nil && p.Scheme != "" && p.Host != "" {
			baseParsed = p
		}
	}

	if rawUI := strings.TrimSpace(os.Getenv("LOOKER_UI_URL")); rawUI != "" {
		if uiParsed, err := url.Parse(rawUI); err == nil && uiParsed.Scheme != "" && uiParsed.Host != "" {
			// Only send authenticated map_api_key requests to LOOKER_UI_URL when it
			// targets the same hostname as the configured Looker API BaseUrl (e.g.
			// looker:9999 vs looker:19999) so bearer tokens are never sent to a
			// different host.
			if baseParsed == nil || isSameOrLoopbackHost(uiParsed.Hostname(), baseParsed.Hostname()) {
				return fmt.Sprintf("%s://%s/public/mcp/ui/map_api_key", uiParsed.Scheme, uiParsed.Host)
			}
		}
	}

	if baseParsed != nil {
		return fmt.Sprintf("%s://%s/public/mcp/ui/map_api_key", baseParsed.Scheme, baseParsed.Host)
	}
	return ""
}

// ClearMcpAppStateCacheForTesting clears the in-memory color collections,
// map API key, and user locale caches. Intended for unit tests.
func ClearMcpAppStateCacheForTesting() {
	mcpAppStateCacheMu.Lock()
	defer mcpAppStateCacheMu.Unlock()
	colorCollectionsCache = make(map[string]cachedColorCollections)
	mapApiKeyCache = make(map[string]cachedMapApiKey)
	userLocaleCache = make(map[string]cachedUserLocale)
}

func hashUserLocaleCacheKey(instanceKey, credentialIdentity string) string {
	sum := sha256.Sum256([]byte(instanceKey + "\x00" + credentialIdentity))
	return hex.EncodeToString(sum[:])
}

func sanitizeLookerLocaleParam(rawLocale string) string {
	trimmed := strings.TrimSpace(rawLocale)
	if trimmed == "" || !strictLookerLocaleRe.MatchString(trimmed) {
		return ""
	}
	return trimmed
}

func resolveUIUserLocale(ctx context.Context, src LookerUISource, settings *rtl.ApiSettings) string {
	if src == nil {
		return ""
	}

	instanceKey := resolveInstanceCacheKey(settings)
	var (
		accessToken string
		cacheKey    string
		ttl         time.Duration
	)

	if src.UseClientAuthorization() {
		header := util.RequestHeaderFromContext(ctx)
		if header == nil {
			return ""
		}
		headerName := src.GetAuthTokenHeaderName()
		if strings.TrimSpace(headerName) == "" {
			headerName = "Authorization"
		}
		accessToken = strings.TrimSpace(header.Get(headerName))
		if accessToken == "" {
			return ""
		}
		cacheKey = hashUserLocaleCacheKey(instanceKey, "client:"+accessToken)
		ttl = clientUserLocaleCacheTTL
	} else {
		cacheKey = hashUserLocaleCacheKey(instanceKey, "static-sdk")
		ttl = staticUserLocaleCacheTTL
	}

	mcpAppStateCacheMu.RLock()
	if entry, ok := userLocaleCache[cacheKey]; ok && time.Since(entry.fetchedAt) < ttl {
		loc := entry.locale
		mcpAppStateCacheMu.RUnlock()
		return loc
	}
	mcpAppStateCacheMu.RUnlock()

	sdk, err := src.GetLookerSDK(ctx, accessToken)
	if err != nil || sdk == nil || sdk.AuthSession == nil {
		return ""
	}

	var u map[string]any
	reqParams := map[string]any{"fields": "locale"}
	if err := sdk.AuthSession.Do(&u, "GET", "/4.0", "/user", reqParams, nil, settings); err != nil {
		if logger, lErr := util.LoggerFromContext(ctx); lErr == nil {
			logger.DebugContext(ctx, "failed to fetch Looker user locale for MCP UI resource", "error", err)
		}
		return ""
	}

	rawLocale, _ := u["locale"].(string)
	validatedLocale := sanitizeLookerLocaleParam(rawLocale)

	now := time.Now()
	mcpAppStateCacheMu.Lock()
	for k, v := range userLocaleCache {
		if now.Sub(v.fetchedAt) >= clientUserLocaleCacheTTL {
			delete(userLocaleCache, k)
		}
	}
	userLocaleCache[cacheKey] = cachedUserLocale{
		locale:    validatedLocale,
		fetchedAt: now,
	}
	mcpAppStateCacheMu.Unlock()

	return validatedLocale
}

func fetchActiveColorCollections(ctx context.Context, sdk *v4.LookerSDK, settings *rtl.ApiSettings) map[string]any {
	cacheKey := resolveInstanceCacheKey(settings)
	mcpAppStateCacheMu.RLock()
	if entry, ok := colorCollectionsCache[cacheKey]; ok && len(entry.data) > 0 && time.Since(entry.fetchedAt) < ColorCollectionsCacheTTL {
		data := entry.data
		mcpAppStateCacheMu.RUnlock()
		return data
	}
	mcpAppStateCacheMu.RUnlock()

	var ccMap map[string]any
	if err := sdk.AuthSession.Do(&ccMap, "GET", "/4.0", "/color_collections/active", nil, nil, settings); err != nil {
		if logger, lErr := util.LoggerFromContext(ctx); lErr == nil {
			logger.DebugContext(ctx, "failed to fetch active color collections for MCP app_state", "error", err)
		}
		return nil
	}
	if len(ccMap) == 0 {
		return nil
	}

	now := time.Now()
	mcpAppStateCacheMu.Lock()
	for k, v := range colorCollectionsCache {
		if now.Sub(v.fetchedAt) >= ColorCollectionsCacheTTL {
			delete(colorCollectionsCache, k)
		}
	}
	colorCollectionsCache[cacheKey] = cachedColorCollections{
		data:      ccMap,
		fetchedAt: now,
	}
	mcpAppStateCacheMu.Unlock()
	return ccMap
}

func fetchMapApiKey(ctx context.Context, sdk *v4.LookerSDK, settings *rtl.ApiSettings) string {
	endpoint := resolveMapApiKeyEndpoint(settings)
	if endpoint == "" {
		return ""
	}
	cacheKey := endpoint

	mcpAppStateCacheMu.RLock()
	if entry, ok := mapApiKeyCache[cacheKey]; ok && entry.apiKey != "" && time.Since(entry.fetchedAt) < MapApiKeyCacheTTL {
		apiKey := entry.apiKey
		mcpAppStateCacheMu.RUnlock()
		return apiKey
	}
	mcpAppStateCacheMu.RUnlock()

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		if logger, lErr := util.LoggerFromContext(ctx); lErr == nil {
			logger.DebugContext(ctx, "failed to create map_api_key request for MCP app_state", "error", err)
		}
		return ""
	}
	req.Header.Set("Accept", "application/json")
	if settings != nil && settings.Headers != nil {
		for k, v := range settings.Headers {
			req.Header.Set(k, v)
		}
	}

	authSess, ok := sdk.AuthSession.(*rtl.AuthSession)
	if !ok || authSess == nil {
		return ""
	}
	if authSess.Config.Headers != nil {
		for k, v := range authSess.Config.Headers {
			if req.Header.Get(k) == "" {
				req.Header.Set(k, v)
			}
		}
	}

	resp, err := authSess.Client.Do(req)
	if err != nil {
		if logger, lErr := util.LoggerFromContext(ctx); lErr == nil {
			logger.DebugContext(ctx, "failed to fetch map_api_key for MCP app_state", "error", err)
		}
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if logger, lErr := util.LoggerFromContext(ctx); lErr == nil {
			logger.DebugContext(ctx, "non-200 response from map_api_key endpoint for MCP app_state", "status", resp.StatusCode)
		}
		return ""
	}

	var payload struct {
		ApiKey string `json:"api_key"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxMapApiKeyResponseBytes)).Decode(&payload); err != nil {
		if logger, lErr := util.LoggerFromContext(ctx); lErr == nil {
			logger.DebugContext(ctx, "failed to decode map_api_key response for MCP app_state", "error", err)
		}
		return ""
	}

	apiKey := strings.TrimSpace(payload.ApiKey)
	if apiKey == "" {
		return ""
	}

	now := time.Now()
	mcpAppStateCacheMu.Lock()
	for k, v := range mapApiKeyCache {
		if now.Sub(v.fetchedAt) >= MapApiKeyCacheTTL {
			delete(mapApiKeyCache, k)
		}
	}
	mapApiKeyCache[cacheKey] = cachedMapApiKey{
		apiKey:    apiKey,
		fetchedAt: now,
	}
	mcpAppStateCacheMu.Unlock()
	return apiKey
}

// FetchMcpAppState concurrently fetches the authenticated user's profile,
// locale, and evaluated abilities alongside the Looker instance's active color
// collections and Google Maps API key (each cached per instance for 10 minutes).
// Errors are logged at debug level and omitted so older Looker instances
// degrade gracefully.
func FetchMcpAppState(ctx context.Context, sdk *v4.LookerSDK, settings *rtl.ApiSettings) map[string]any {
	if sdk == nil || sdk.AuthSession == nil {
		return nil
	}

	var (
		wg        sync.WaitGroup
		userMap   map[string]any
		colorsMap map[string]any
		mapApiKey string
	)

	wg.Add(3)
	go func() {
		defer wg.Done()
		var u map[string]any
		reqParams := map[string]any{"fields": mcpUserFields}
		if err := sdk.AuthSession.Do(&u, "GET", "/4.0", "/user", reqParams, nil, settings); err != nil {
			if logger, lErr := util.LoggerFromContext(ctx); lErr == nil {
				logger.DebugContext(ctx, "failed to fetch user abilities for MCP app_state", "error", err)
			}
			return
		}
		if len(u) > 0 {
			userMap = u
		}
	}()

	go func() {
		defer wg.Done()
		colorsMap = fetchActiveColorCollections(ctx, sdk, settings)
	}()

	go func() {
		defer wg.Done()
		mapApiKey = fetchMapApiKey(ctx, sdk, settings)
	}()

	wg.Wait()

	if len(userMap) == 0 && len(colorsMap) == 0 && mapApiKey == "" {
		return nil
	}

	appState := make(map[string]any)
	if len(userMap) > 0 {
		appState["user"] = userMap
	}
	if len(colorsMap) > 0 {
		appState["color_collections"] = colorsMap
	}
	if mapApiKey != "" {
		appState["map_api_key"] = mapApiKey
	}
	return appState
}

// mcpUIEntryScriptRe matches the <script src="..."> tag that loads a Looker
// MCP Apps entry bundle (mcp-render-dashboard / mcp-render-visualization) and
// captures the https origin of that src.
var mcpUIEntryScriptRe = regexp.MustCompile(
	`(?i)<script\b[^>]*\bsrc\s*=\s*["'](https://[^/"'\s?#]+)/[^"']*mcp-render-(?:dashboard|visualization)[^"']*["']`,
)

// lookerBaseURLScriptRe matches window.__LOOKER_BASE_URL__ = "https://..."
// injected by Looker's Soy template in the HTML head.
var lookerBaseURLScriptRe = regexp.MustCompile(
	`window\.__LOOKER_BASE_URL__\s*=\s*["'](https://[^/"'\s?#]+)(?:/[^"']*)?["']`,
)

// DefaultAllowedLookerCDNOrigins lists the trusted Looker CDN origins that may
// serve MCP Apps webpack bundles when CDN is enabled on a Looker instance.
var DefaultAllowedLookerCDNOrigins = []string{
	"https://*.lookercdn.com",
	"https://lookercdn.com",
	"https://*.cdn.looker.app",
	"https://cdn.looker.app",
}

// matchesAllowedOrigin checks whether a lowercased https candidate origin
// ("https://host[:port]") and its hostname ("host") match rawAllowed, which may
// be an https origin, a bare hostname, or a wildcard pattern ("https://*.example.com"
// or "*.example.com").
func matchesAllowedOrigin(candidate, candidateHost, rawAllowed string) bool {
	allowed := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(rawAllowed), "/"))
	if allowed == "" {
		return false
	}
	if candidate == allowed || candidateHost == allowed {
		return true
	}
	if strings.HasPrefix(allowed, "https://*.") {
		suffix := strings.TrimPrefix(allowed, "https://*")
		return strings.HasSuffix(candidateHost, suffix) && len(candidateHost) > len(suffix)
	}
	if strings.HasPrefix(allowed, "*.") {
		suffix := strings.TrimPrefix(allowed, "*")
		return strings.HasSuffix(candidateHost, suffix) && len(candidateHost) > len(suffix)
	}
	if allowedParsed, pErr := url.Parse(allowed); pErr == nil && allowedParsed.Scheme == "https" && allowedParsed.Host != "" {
		if candidate == fmt.Sprintf("https://%s", strings.ToLower(allowedParsed.Host)) {
			return true
		}
	}
	return false
}

// DiscoverMCPUIPublicOrigin returns the public origin that Looker declared in
// the MCP Apps UI HTML it served, or "" if none was found.
//
// Toolbox may fetch the UI over an address the end user's browser cannot
// reach. For example, the Looker-managed MCP server fetches over
// http://looker:19999 inside the cluster. Looker builds the entry script URL
// from its Host URL setting, so the origin of that script is the origin the
// browser loads assets from, and therefore the one the CSP must allow.
//
// Only valid https origins without userinfo or paths are returned, so a page
// can't downgrade the CSP to plain http or inject credentials. If one or more
// non-empty allowedOrigins are provided, the discovered origin must match one
// of them.
func DiscoverMCPUIPublicOrigin(html string, allowedOrigins ...string) string {
	m := mcpUIEntryScriptRe.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	parsed, err := url.Parse(m[1])
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil {
		return ""
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	candidate := fmt.Sprintf("%s://%s", parsed.Scheme, strings.ToLower(parsed.Host))
	candidateHost := strings.ToLower(parsed.Hostname())

	hasAllowedFilter := false
	for _, rawAllowed := range allowedOrigins {
		if strings.TrimSpace(rawAllowed) == "" {
			continue
		}
		hasAllowedFilter = true
		if matchesAllowedOrigin(candidate, candidateHost, rawAllowed) {
			return candidate
		}
	}
	if hasAllowedFilter {
		return ""
	}
	return candidate
}

// GoogleMapsResourceDomains lists the explicit CSP resource domains required by
// the Google Maps JavaScript API (including vector maps, raster/satellite tiles,
// fonts, streetview/imagery, and static assets) without broad *.googleapis.com
// or *.googleusercontent.com wildcards.
var GoogleMapsResourceDomains = []string{
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

// GoogleMapsConnectDomains lists the explicit CSP connect-src domains required
// by the Google Maps JavaScript API (including vector featureMaps on
// mapsresources-pa.googleapis.com, tile servers, and imagery endpoints) without
// broad *.googleapis.com, *.googleusercontent.com, or *.gstatic.com wildcards.
var GoogleMapsConnectDomains = []string{
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

// DefaultMCPUICSPConfig returns a new CSPConfig initialized with the default
// Looker CDN and Google Maps resource and connect domains.
func DefaultMCPUICSPConfig() *resources.CSPConfig {
	resourceDomains := []string{
		"https://*.lookercdn.com",
		"https://lookercdn.com",
		"https://*.cdn.looker.app",
		"https://cdn.looker.app",
	}
	resourceDomains = append(resourceDomains, GoogleMapsResourceDomains...)

	connectDomains := []string{
		"https://*.lookercdn.com",
		"https://lookercdn.com",
		"https://*.cdn.looker.app",
		"https://cdn.looker.app",
	}
	connectDomains = append(connectDomains, GoogleMapsConnectDomains...)

	return &resources.CSPConfig{
		ResourceDomains: resourceDomains,
		ConnectDomains:  connectDomains,
	}
}

// AddCSPOrigin allows origin in the CSP's resource and connect domains and adds
// the matching WebSocket origin (ws:// for http, wss:// for https) to the
// connect domains. Duplicate entries are skipped. The caller must synchronize
// access to csp.
func AddCSPOrigin(csp *resources.CSPConfig, origin string) {
	origin = strings.TrimSuffix(strings.TrimSpace(origin), "/")
	if csp == nil || origin == "" {
		return
	}
	appendUnique(&csp.ResourceDomains, origin)
	appendUnique(&csp.ConnectDomains, origin)
	switch {
	case strings.HasPrefix(origin, "https://"):
		appendUnique(&csp.ConnectDomains, "wss://"+strings.TrimPrefix(origin, "https://"))
	case strings.HasPrefix(origin, "http://"):
		appendUnique(&csp.ConnectDomains, "ws://"+strings.TrimPrefix(origin, "http://"))
	}
}

func appendUnique(list *[]string, value string) {
	for _, existing := range *list {
		if strings.TrimSuffix(existing, "/") == value {
			return
		}
	}
	*list = append(*list, value)
}

// ResolveVerifySSL determines whether TLS certificate verification should be
// enabled based on apiSettings (if non-nil) or the LOOKER_VERIFY_SSL
// environment variable ("false" or "0" disables verification). When disabled,
// a warning is logged to ctx.
func ResolveVerifySSL(ctx context.Context, apiSettings *rtl.ApiSettings, component string) bool {
	verifySSL := true
	if apiSettings != nil {
		verifySSL = apiSettings.VerifySsl
	} else if envVerify := strings.TrimSpace(os.Getenv("LOOKER_VERIFY_SSL")); strings.EqualFold(envVerify, "false") || envVerify == "0" {
		verifySSL = false
	}
	if !verifySSL {
		if logger, lErr := util.LoggerFromContext(ctx); lErr == nil {
			logger.WarnContext(ctx, fmt.Sprintf("TLS certificate verification is disabled (VerifySsl=false) when fetching Looker %s", component))
		}
	}
	return verifySSL
}

// EnsureDefaultToolUIMetadata populates default Resource and Visibility
// ([model, app]) on a tool's UI metadata config.
func EnsureDefaultToolUIMetadata(ui **tools.ToolUIMetadata, defaultResourceName string) {
	if *ui == nil {
		*ui = &tools.ToolUIMetadata{
			Resource:   defaultResourceName,
			Visibility: []tools.ToolVisibility{tools.VisibilityModel, tools.VisibilityApp},
		}
		return
	}
	if (*ui).Resource == "" {
		(*ui).Resource = defaultResourceName
	}
	if len((*ui).Visibility) == 0 {
		(*ui).Visibility = []tools.ToolVisibility{tools.VisibilityModel, tools.VisibilityApp}
	}
}

// BuildLookerUICSP constructs a CSPConfig initialized with Looker CDN, Google
// Maps, LOOKER_BASE_URL, LOOKER_UI_URL, and local development origins.
func BuildLookerUICSP() *resources.CSPConfig {
	csp := DefaultMCPUICSPConfig()

	lookerBaseURL := os.Getenv("LOOKER_BASE_URL")
	lookerUIURL := os.Getenv("LOOKER_UI_URL")

	ensureDomain := func(list *[]string, domain string) {
		if domain == "" {
			return
		}
		domain = strings.TrimSuffix(domain, "/")
		for _, d := range *list {
			if strings.TrimSuffix(d, "/") == domain {
				return
			}
		}
		*list = append(*list, domain)
	}

	getOrigin := func(rawURL string) string {
		if rawURL == "" {
			return ""
		}
		rawURL = strings.TrimSpace(rawURL)
		if !strings.Contains(rawURL, "://") {
			rawURL = "http://" + rawURL
		}
		if parsed, err := url.Parse(rawURL); err == nil && parsed.Host != "" {
			return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
		}
		return strings.TrimSuffix(rawURL, "/")
	}

	addOrigin := func(rawURL string) {
		origin := getOrigin(rawURL)
		if origin == "" {
			return
		}
		ensureDomain(&csp.ResourceDomains, origin)
		ensureDomain(&csp.ConnectDomains, origin)

		// If origin is HTTP, also ensure corresponding WebSocket origin for local dev hot reloading
		if strings.HasPrefix(origin, "http://") {
			wsOrigin := "ws://" + strings.TrimPrefix(origin, "http://")
			ensureDomain(&csp.ConnectDomains, wsOrigin)
		} else if strings.HasPrefix(origin, "https://") {
			wssOrigin := "wss://" + strings.TrimPrefix(origin, "https://")
			ensureDomain(&csp.ConnectDomains, wssOrigin)
		}
	}

	addOrigin(lookerBaseURL)
	addOrigin(lookerUIURL)

	// In local development environments, permit all local dev ports for both resources and connect
	if isLocalDevURL(lookerBaseURL) || isLocalDevURL(lookerUIURL) {
		for _, host := range []string{"localhost", "127.0.0.1", "0.0.0.0"} {
			for _, port := range []string{"9998", "9999", "19999", "3035"} {
				ensureDomain(&csp.ResourceDomains, fmt.Sprintf("http://%s:%s", host, port))
				ensureDomain(&csp.ConnectDomains, fmt.Sprintf("http://%s:%s", host, port))
				ensureDomain(&csp.ConnectDomains, fmt.Sprintf("ws://%s:%s", host, port))
			}
			ensureDomain(&csp.ConnectDomains, fmt.Sprintf("ws://%s:*", host))
			ensureDomain(&csp.ConnectDomains, fmt.Sprintf("wss://%s:*", host))
		}
	}

	return csp
}

func isLocalDevURL(rawURL string) bool {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return false
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return false
	}
	return isLoopbackHostname(parsed.Hostname())
}

// LookerUISource represents a Looker source capable of providing API settings
// and client authorization configuration for MCP Apps UI tools.
type LookerUISource interface {
	UseClientAuthorization() bool
	GetAuthTokenHeaderName() string
	LookerApiSettings() *rtl.ApiSettings
	GetLookerSDK(context.Context, string) (*v4.LookerSDK, error)
}

var (
	baseTagRe   = regexp.MustCompile(`(?i)<base\b[^>]*\/?>`)
	nonceAttrRe = regexp.MustCompile(`(?i)\s+nonce\s*=\s*(?:"[^"]*"|'[^']*')`)
)

// LookerUIResourceManager manages dynamic MCP Apps UI HTML fetching, caching,
// asset origin rewriting, and CSP origin registration for a Looker UI scene.
type LookerUIResourceManager struct {
	mu                 sync.RWMutex
	src                LookerUISource
	sceneName          string
	otherSceneName     string
	uiLabel            string
	csp                *resources.CSPConfig
	uiResource         resources.Resource
	cachedHTMLByLocale map[string]cachedUIHTML
}

// NewLookerUIResourceManager constructs a LookerUIResourceManager with a
// DynamicUIResource backed by FetchRemoteUI.
func NewLookerUIResourceManager(resourceName, resourceURI, description, sceneName, otherSceneName, uiLabel string) *LookerUIResourceManager {
	csp := BuildLookerUICSP()
	m := &LookerUIResourceManager{
		sceneName:          sceneName,
		otherSceneName:     otherSceneName,
		uiLabel:            uiLabel,
		csp:                csp,
		cachedHTMLByLocale: make(map[string]cachedUIHTML),
	}
	prefersBorder := true
	m.uiResource = resources.NewDynamicUIResource(
		resourceName,
		resourceURI,
		description,
		csp,
		&prefersBorder,
		func(ctx context.Context) (string, error) {
			return m.FetchRemoteUI(ctx)
		},
	)
	return m
}

// SetSource binds the validated LookerUISource to the resource manager.
func (m *LookerUIResourceManager) SetSource(src LookerUISource) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.src = src
	m.mu.Unlock()
}

// GetResources returns the dynamic UI resource managed by m.
func (m *LookerUIResourceManager) GetResources() []resources.Resource {
	if m == nil || m.uiResource == nil {
		return nil
	}
	return []resources.Resource{m.uiResource}
}

// CSP returns a thread-safe deep copy of the underlying CSPConfig for the UI resource.
func (m *LookerUIResourceManager) CSP() *resources.CSPConfig {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.csp.Clone()
}

func parsePublicHTTPSOrigin(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil {
		return ""
	}
	if isLoopbackHostname(parsed.Hostname()) {
		return ""
	}
	return fmt.Sprintf("https://%s", strings.ToLower(parsed.Host))
}

// resolveAllowedPublicOrigins builds the allowlist of HTTPS origins permitted
// for DiscoverMCPUIPublicOrigin based on configured Looker URLs (LOOKER_UI_URL,
// apiSettings.BaseUrl, LOOKER_BASE_URL), window.__LOOKER_BASE_URL__ in the HTML,
// and Looker's official CDN domains.
func (m *LookerUIResourceManager) resolveAllowedPublicOrigins(apiSettings *rtl.ApiSettings, rawHTML string) []string {
	var configuredHTTPSOrigins []string
	addIfValid := func(raw string) {
		if origin := parsePublicHTTPSOrigin(raw); origin != "" {
			appendUnique(&configuredHTTPSOrigins, origin)
		}
	}

	addIfValid(os.Getenv("LOOKER_UI_URL"))
	if apiSettings != nil {
		addIfValid(apiSettings.BaseUrl)
	}
	addIfValid(os.Getenv("LOOKER_BASE_URL"))

	if len(configuredHTTPSOrigins) == 0 {
		if match := lookerBaseURLScriptRe.FindStringSubmatch(rawHTML); len(match) >= 2 {
			addIfValid(match[1])
		}
	}

	if len(configuredHTTPSOrigins) == 0 {
		return nil
	}

	allowed := make([]string, 0, len(configuredHTTPSOrigins)+len(DefaultAllowedLookerCDNOrigins))
	allowed = append(allowed, configuredHTTPSOrigins...)
	allowed = append(allowed, DefaultAllowedLookerCDNOrigins...)
	return allowed
}

// resolveAuthoritativePublicOrigin returns the browser-facing HTTPS origin for
// rewriting relative UI assets using the authoritative hierarchy:
//  1. Configured HTTPS LOOKER_UI_URL
//  2. Configured HTTPS apiSettings.BaseUrl / LOOKER_BASE_URL
//  3. Validated HTTPS window.__LOOKER_BASE_URL__ from the Looker HTML shell
//  4. Allowlist-validated discoveredOrigin from DiscoverMCPUIPublicOrigin
func (m *LookerUIResourceManager) resolveAuthoritativePublicOrigin(apiSettings *rtl.ApiSettings, rawHTML, discoveredOrigin string) string {
	if origin := parsePublicHTTPSOrigin(os.Getenv("LOOKER_UI_URL")); origin != "" {
		return origin
	}
	if apiSettings != nil {
		if origin := parsePublicHTTPSOrigin(apiSettings.BaseUrl); origin != "" {
			return origin
		}
	}
	if origin := parsePublicHTTPSOrigin(os.Getenv("LOOKER_BASE_URL")); origin != "" {
		return origin
	}
	if match := lookerBaseURLScriptRe.FindStringSubmatch(rawHTML); len(match) >= 2 {
		if origin := parsePublicHTTPSOrigin(match[1]); origin != "" {
			return origin
		}
	}
	return discoveredOrigin
}

// RenderHTML makes the fetched UI HTML loadable inside the MCP host's sandbox.
// publicOrigin is the browser-facing Looker origin resolved from configuration
// or DiscoverMCPUIPublicOrigin. If it is empty, relative asset URLs are resolved
// against LOOKER_UI_URL, then the source BaseUrl, then LOOKER_BASE_URL.
func (m *LookerUIResourceManager) RenderHTML(rawHTML, publicOrigin string) string {
	assetBaseURL := publicOrigin
	if lookerUIURL := os.Getenv("LOOKER_UI_URL"); assetBaseURL == "" && lookerUIURL != "" {
		lookerUIURL = strings.TrimSpace(lookerUIURL)
		if !strings.Contains(lookerUIURL, "://") {
			lookerUIURL = "http://" + lookerUIURL
		}
		if parsed, err := url.Parse(lookerUIURL); err == nil && parsed.Host != "" {
			assetBaseURL = fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
		}
	}
	if assetBaseURL == "" && m != nil {
		m.mu.RLock()
		src := m.src
		m.mu.RUnlock()
		if src != nil && src.LookerApiSettings() != nil && src.LookerApiSettings().BaseUrl != "" {
			assetBaseURL = strings.TrimSuffix(src.LookerApiSettings().BaseUrl, "/")
		}
	}
	if assetBaseURL == "" {
		if lookerBaseURL := os.Getenv("LOOKER_BASE_URL"); lookerBaseURL != "" {
			lookerBaseURL = strings.TrimSpace(lookerBaseURL)
			if !strings.Contains(lookerBaseURL, "://") {
				lookerBaseURL = "http://" + lookerBaseURL
			}
			if parsed, err := url.Parse(lookerBaseURL); err == nil && parsed.Host != "" {
				assetBaseURL = fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
			}
		}
	}

	res := strings.ReplaceAll(rawHTML, "__MCP_ASSET_BASE_URL__", assetBaseURL)
	if assetBaseURL != "" {
		res = strings.ReplaceAll(res, `src="/webpack/`, fmt.Sprintf(`src="%s/webpack/`, assetBaseURL))
		res = strings.ReplaceAll(res, `src="/public/mcp/ui/assets/`, fmt.Sprintf(`src="%s/public/mcp/ui/%s/assets/`, assetBaseURL, m.sceneName))
		res = strings.ReplaceAll(res, fmt.Sprintf(`src="/public/mcp/ui/%s/assets/`, m.sceneName), fmt.Sprintf(`src="%s/public/mcp/ui/%s/assets/`, assetBaseURL, m.sceneName))
		res = strings.ReplaceAll(res, `src="/mcp/ui/assets/`, fmt.Sprintf(`src="%s/public/mcp/ui/%s/assets/`, assetBaseURL, m.sceneName))
		res = strings.ReplaceAll(res, `url('/fonts/`, fmt.Sprintf(`url('%s/fonts/`, assetBaseURL))
		res = strings.ReplaceAll(res, `url("/fonts/`, fmt.Sprintf(`url("%s/fonts/`, assetBaseURL))
		res = strings.ReplaceAll(res, `url(/fonts/`, fmt.Sprintf(`url(%s/fonts/`, assetBaseURL))
	}
	// Strip any <base ...> tag to prevent CSP base-uri 'self' violation in sandboxed iframes
	res = baseTagRe.ReplaceAllString(res, "")
	// Strip any server-generated nonce attributes to prevent CSP nonce mismatch in sandboxed iframes
	res = nonceAttrRe.ReplaceAllString(res, "")
	return res
}

// FetchRemoteUI fetches and caches the scene's HTML shell from the Looker instance.
func (m *LookerUIResourceManager) FetchRemoteUI(ctx context.Context) (string, error) {
	if m == nil {
		return "", fmt.Errorf("tool state not initialized: unable to fetch UI")
	}

	m.mu.RLock()
	src := m.src
	var apiSettings *rtl.ApiSettings
	if src != nil {
		apiSettings = src.LookerApiSettings()
	}
	m.mu.RUnlock()

	requestedLocale := resolveUIUserLocale(ctx, src, apiSettings)
	localeCacheKey := requestedLocale
	if localeCacheKey == "" {
		localeCacheKey = "default"
	}

	// 1. Check in-memory HTML cache scoped by validated locale key (5-minute TTL)
	m.mu.RLock()
	if entry, ok := m.cachedHTMLByLocale[localeCacheKey]; ok && entry.html != "" && time.Since(entry.fetchedAt) < uiHTMLCacheTTL {
		html := entry.html
		m.mu.RUnlock()
		return html, nil
	}
	m.mu.RUnlock()

	var remoteURL string
	targetSuffix := fmt.Sprintf("/%s/assets", m.sceneName)
	otherSuffix := fmt.Sprintf("/%s/assets", m.otherSceneName)
	if lookerUIURL := os.Getenv("LOOKER_UI_URL"); lookerUIURL != "" {
		lookerUIURL = strings.TrimSpace(lookerUIURL)
		if !strings.Contains(lookerUIURL, "://") {
			lookerUIURL = "http://" + lookerUIURL
		}
		lookerUIURL = strings.TrimSuffix(lookerUIURL, "/")
		if strings.HasSuffix(lookerUIURL, targetSuffix) {
			remoteURL = lookerUIURL
		} else if m.otherSceneName != "" && strings.Contains(lookerUIURL, otherSuffix) {
			remoteURL = strings.ReplaceAll(lookerUIURL, otherSuffix, targetSuffix)
		} else if !strings.HasSuffix(lookerUIURL, "/assets") {
			remoteURL = fmt.Sprintf("%s/public/mcp/ui/%s/assets", lookerUIURL, m.sceneName)
		} else {
			remoteURL = lookerUIURL
		}
	} else if apiSettings != nil && apiSettings.BaseUrl != "" {
		baseURL := strings.TrimSuffix(apiSettings.BaseUrl, "/")
		remoteURL = fmt.Sprintf("%s/public/mcp/ui/%s/assets", baseURL, m.sceneName)
	}

	if remoteURL == "" {
		return "", fmt.Errorf("no Looker instance URL configured to fetch %s UI", m.uiLabel)
	}

	if requestedLocale != "" {
		if parsedRemote, pErr := url.Parse(remoteURL); pErr == nil {
			q := parsedRemote.Query()
			if q.Get("locale") == "" {
				q.Set("locale", requestedLocale)
				parsedRemote.RawQuery = q.Encode()
				remoteURL = parsedRemote.String()
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for %s UI at %s: %w", m.uiLabel, remoteURL, err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	if !ResolveVerifySSL(ctx, apiSettings, m.uiLabel+" UI") {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch %s UI from Looker at %s: %w", m.uiLabel, remoteURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("tool UI access denied (HTTP %d): disabled by administrator", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch %s UI from Looker at %s: HTTP %d", m.uiLabel, remoteURL, resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxUIHTMLResponseBytes+1))
	if err != nil || len(bodyBytes) == 0 {
		return "", fmt.Errorf("empty %s UI returned from Looker at %s", m.uiLabel, remoteURL)
	}
	if len(bodyBytes) > maxUIHTMLResponseBytes {
		return "", fmt.Errorf("%s UI response from Looker at %s exceeds maximum size (%d bytes)", m.uiLabel, remoteURL, maxUIHTMLResponseBytes)
	}

	rawHTML := string(bodyBytes)
	allowedOrigins := m.resolveAllowedPublicOrigins(apiSettings, rawHTML)
	discoveredOrigin := DiscoverMCPUIPublicOrigin(rawHTML, allowedOrigins...)
	publicOrigin := m.resolveAuthoritativePublicOrigin(apiSettings, rawHTML, discoveredOrigin)
	html := m.RenderHTML(rawHTML, publicOrigin)
	m.mu.Lock()
	if m.cachedHTMLByLocale == nil {
		m.cachedHTMLByLocale = make(map[string]cachedUIHTML)
	}
	m.allowUIOrigin(remoteURL, publicOrigin, discoveredOrigin)
	m.cachedHTMLByLocale[localeCacheKey] = cachedUIHTML{
		html:      html,
		fetchedAt: time.Now(),
	}
	m.mu.Unlock()

	return html, nil
}

// allowUIOrigin adds the origin(s) the browser loads UI assets from to a newly
// cloned CSPConfig and atomically updates the DynamicUIResource's CSP. If no
// HTTPS public origin was resolved or discovered (e.g. local development over
// http), the origin toolbox fetched the UI from is allowed in both
// ResourceDomains and ConnectDomains instead. The caller must hold m.mu.
func (m *LookerUIResourceManager) allowUIOrigin(remoteURL, publicOrigin, discoveredOrigin string) {
	if m.csp == nil {
		return
	}
	nextCSP := m.csp.Clone()
	addedAny := false
	if publicOrigin != "" {
		AddCSPOrigin(nextCSP, publicOrigin)
		addedAny = true
	}
	if discoveredOrigin != "" && discoveredOrigin != publicOrigin {
		AddCSPOrigin(nextCSP, discoveredOrigin)
		addedAny = true
	}
	if !addedAny {
		if parsed, err := url.Parse(remoteURL); err == nil && parsed.Host != "" {
			origin := fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
			AddCSPOrigin(nextCSP, origin)
		}
	}
	m.csp = nextCSP
	if dynRes, ok := m.uiResource.(*resources.DynamicUIResource); ok {
		dynRes.SetCSP(nextCSP)
	}
}

