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
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
	"golang.org/x/sync/singleflight"
)

const (
	ColorCollectionsCacheTTL   = 10 * time.Minute
	MapApiKeyCacheTTL          = 10 * time.Minute
	UIHTMLCacheTTL             = 5 * time.Minute
	ClientUserLocaleCacheTTL   = 5 * time.Minute
	StaticUserLocaleCacheTTL   = 10 * time.Second
	mcpUIRequestTimeout        = 5 * time.Second
	mcpUIRequestTimeoutSeconds = int32(5)
	maxTTLCacheEntries         = 1024
	mcpUserFields              = "id,email,display_name,locale,abilities,can"
	maxMapApiKeyResponseBytes  = 64 * 1024        // 64 KiB
	maxUIHTMLResponseBytes     = 10 * 1024 * 1024 // 10 MiB
)

type ttlEntry[V any] struct {
	value     V
	expiresAt time.Time
}

type ttlCache[V any] struct {
	mu      sync.RWMutex
	entries map[string]ttlEntry[V]
}

func newTTLCache[V any]() *ttlCache[V] {
	return &ttlCache[V]{entries: make(map[string]ttlEntry[V])}
}

func (c *ttlCache[V]) get(key string) (V, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		var zero V
		return zero, false
	}
	return entry.value, true
}

func (c *ttlCache[V]) set(key string, value V, ttl time.Duration) {
	now := time.Now()
	c.mu.Lock()
	for k, v := range c.entries {
		if now.After(v.expiresAt) {
			delete(c.entries, k)
		}
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= maxTTLCacheEntries {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range c.entries {
			if oldestKey == "" || v.expiresAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.expiresAt
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}
	c.entries[key] = ttlEntry[V]{
		value:     value,
		expiresAt: now.Add(ttl),
	}
	c.mu.Unlock()
}

func (c *ttlCache[V]) clear() {
	c.mu.Lock()
	clear(c.entries)
	c.mu.Unlock()
}

var (
	colorCollectionsCache = newTTLCache[map[string]any]()
	mapApiKeyCache        = newTTLCache[string]()
	userLocaleCache       = newTTLCache[string]()
	strictLookerLocaleRe  = regexp.MustCompile(`^[a-zA-Z]{2,3}(?:[-_][a-zA-Z]{2,4})?$`)
)

func cloneJSONMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = cloneJSONValue(v)
	}
	return dst
}

func cloneJSONValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return cloneJSONMap(val)
	case []any:
		out := make([]any, len(val))
		for i, elem := range val {
			out[i] = cloneJSONValue(elem)
		}
		return out
	case []string:
		return slices.Clone(val)
	default:
		return val
	}
}

func withShortSDKTimeout(settings *rtl.ApiSettings) *rtl.ApiSettings {
	var copySettings rtl.ApiSettings
	if settings != nil {
		copySettings = *settings
	}
	if copySettings.Timeout <= 0 || copySettings.Timeout > mcpUIRequestTimeoutSeconds {
		copySettings.Timeout = mcpUIRequestTimeoutSeconds
	}
	return &copySettings
}

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

func parseURLWithDefaultScheme(rawURL string) (*url.URL, bool) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, false
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return nil, false
	}
	return parsed, true
}

func extractURLOrigin(rawURL string) string {
	if parsed, ok := parseURLWithDefaultScheme(rawURL); ok {
		return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
	}
	return ""
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
		p, ok := parseURLWithDefaultScheme(rawBase)
		if !ok {
			return ""
		}
		baseParsed = p
	}

	if rawUI := strings.TrimSpace(os.Getenv("LOOKER_UI_URL")); rawUI != "" {
		if uiParsed, ok := parseURLWithDefaultScheme(rawUI); ok {
			// Only send authenticated map_api_key requests to LOOKER_UI_URL when it
			// targets the same hostname as the configured Looker API BaseUrl (e.g.
			// looker:9999 vs looker:19999) so bearer tokens are never sent to a
			// different host. Fail closed when no valid BaseUrl is configured.
			if baseParsed != nil && isSameOrLoopbackHost(uiParsed.Hostname(), baseParsed.Hostname()) {
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
	colorCollectionsCache.clear()
	mapApiKeyCache.clear()
	userLocaleCache.clear()
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

func debugLog(ctx context.Context, msg string, args ...any) {
	if logger, err := util.LoggerFromContext(ctx); err == nil {
		logger.DebugContext(ctx, msg, args...)
	}
}

func resolveUIUserLocale(ctx context.Context, src LookerUISource, settings *rtl.ApiSettings) string {
	if src == nil || ctx.Err() != nil {
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
		ttl = ClientUserLocaleCacheTTL
	} else {
		cacheKey = hashUserLocaleCacheKey(instanceKey, "static-sdk")
		ttl = StaticUserLocaleCacheTTL
	}

	if loc, ok := userLocaleCache.get(cacheKey); ok {
		return loc
	}

	sdk, err := src.GetLookerSDK(ctx, accessToken)
	if err != nil || sdk == nil || sdk.AuthSession == nil {
		return ""
	}

	var u map[string]any
	reqParams := map[string]any{"fields": "locale"}
	if err := sdk.AuthSession.Do(&u, "GET", "/4.0", "/user", reqParams, nil, withShortSDKTimeout(settings)); err != nil {
		debugLog(ctx, "failed to fetch Looker user locale for MCP UI resource", "error", err)
		return ""
	}

	rawLocale, _ := u["locale"].(string)
	validatedLocale := sanitizeLookerLocaleParam(rawLocale)
	userLocaleCache.set(cacheKey, validatedLocale, ttl)
	return validatedLocale
}

func fetchActiveColorCollections(ctx context.Context, sdk *v4.LookerSDK, settings *rtl.ApiSettings) map[string]any {
	cacheKey := resolveInstanceCacheKey(settings)
	if data, ok := colorCollectionsCache.get(cacheKey); ok && len(data) > 0 {
		return cloneJSONMap(data)
	}
	if ctx.Err() != nil {
		return nil
	}

	var ccMap map[string]any
	if err := sdk.AuthSession.Do(&ccMap, "GET", "/4.0", "/color_collections/active", nil, nil, withShortSDKTimeout(settings)); err != nil {
		debugLog(ctx, "failed to fetch active color collections for MCP app_state", "error", err)
		return nil
	}
	if len(ccMap) == 0 {
		return nil
	}

	colorCollectionsCache.set(cacheKey, cloneJSONMap(ccMap), ColorCollectionsCacheTTL)
	return cloneJSONMap(ccMap)
}

func fetchMapApiKey(ctx context.Context, sdk *v4.LookerSDK, settings *rtl.ApiSettings) string {
	endpoint := resolveMapApiKeyEndpoint(settings)
	if endpoint == "" {
		return ""
	}

	if apiKey, ok := mapApiKeyCache.get(endpoint); ok && apiKey != "" {
		return apiKey
	}

	reqCtx, cancel := context.WithTimeout(ctx, mcpUIRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		debugLog(ctx, "failed to create map_api_key request for MCP app_state", "error", err)
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
	for k, v := range authSess.Config.Headers {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := authSess.Client.Do(req)
	if err != nil {
		debugLog(ctx, "failed to fetch map_api_key for MCP app_state", "error", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		debugLog(ctx, "non-200 response from map_api_key endpoint for MCP app_state", "status", resp.StatusCode)
		return ""
	}

	var payload struct {
		ApiKey string `json:"api_key"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxMapApiKeyResponseBytes)).Decode(&payload); err != nil {
		debugLog(ctx, "failed to decode map_api_key response for MCP app_state", "error", err)
		return ""
	}

	apiKey := strings.TrimSpace(payload.ApiKey)
	if apiKey == "" {
		return ""
	}

	mapApiKeyCache.set(endpoint, apiKey, MapApiKeyCacheTTL)
	return apiKey
}

// FetchMcpAppState fetches the authenticated user's profile, locale, and
// evaluated abilities alongside the Looker instance's active color collections
// and Google Maps API key (each cached per instance for 10 minutes).
// Requests execute sequentially because the Looker Go SDK's AuthSession is not
// safe for concurrent token initialization or refresh. Errors are logged at
// debug level and omitted so older Looker instances degrade gracefully.
func FetchMcpAppState(ctx context.Context, sdk *v4.LookerSDK, settings *rtl.ApiSettings) map[string]any {
	if sdk == nil || sdk.AuthSession == nil || ctx.Err() != nil {
		return nil
	}

	shortSettings := withShortSDKTimeout(settings)
	var userMap map[string]any
	var u map[string]any
	reqParams := map[string]any{"fields": mcpUserFields}
	if err := sdk.AuthSession.Do(&u, "GET", "/4.0", "/user", reqParams, nil, shortSettings); err != nil {
		debugLog(ctx, "failed to fetch user abilities for MCP app_state", "error", err)
	} else if len(u) > 0 {
		userMap = u
	}

	colorsMap := fetchActiveColorCollections(ctx, sdk, shortSettings)
	mapApiKey := fetchMapApiKey(ctx, sdk, settings)

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
	return &resources.CSPConfig{
		ResourceDomains: slices.Concat(DefaultAllowedLookerCDNOrigins, GoogleMapsResourceDomains),
		ConnectDomains:  slices.Concat(DefaultAllowedLookerCDNOrigins, GoogleMapsConnectDomains),
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
// enabled. When apiSettings is non-nil (for example, from an initialized Looker
// source whose verify_ssl config may interpolate LOOKER_VERIFY_SSL),
// apiSettings.VerifySsl takes precedence; otherwise it falls back to the
// LOOKER_VERIFY_SSL environment variable ("false" or "0" disables verification).
// When disabled, a warning is logged to ctx.
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

func addLookerURLsToCSP(csp *resources.CSPConfig, rawURLs ...string) {
	if csp == nil {
		return
	}
	hasLocalDev := false
	for _, rawURL := range rawURLs {
		AddCSPOrigin(csp, extractURLOrigin(rawURL))
		if isLocalDevURL(rawURL) {
			hasLocalDev = true
		}
	}
	// In local development environments, permit all local dev ports on browser
	// loopback hostnames (including 0.0.0.0, which webpack-dev-server's HMR
	// WebSocket client connects to via ws://0.0.0.0:9999/ws).
	if hasLocalDev {
		for _, host := range []string{"localhost", "127.0.0.1", "0.0.0.0"} {
			for _, port := range []string{"9998", "9999", "19999", "3035"} {
				AddCSPOrigin(csp, fmt.Sprintf("http://%s:%s", host, port))
			}
			appendUnique(&csp.ConnectDomains, fmt.Sprintf("ws://%s:*", host))
			appendUnique(&csp.ConnectDomains, fmt.Sprintf("wss://%s:*", host))
		}
	}
}

func buildLookerUICSPForBaseURL(baseURL string) *resources.CSPConfig {
	csp := DefaultMCPUICSPConfig()
	if strings.TrimSpace(baseURL) == "" {
		baseURL = os.Getenv("LOOKER_BASE_URL")
	}
	addLookerURLsToCSP(csp, baseURL, os.Getenv("LOOKER_UI_URL"))
	return csp
}

// BuildLookerUICSP constructs a CSPConfig initialized with Looker CDN, Google
// Maps, LOOKER_BASE_URL, LOOKER_UI_URL, and local development origins.
func BuildLookerUICSP() *resources.CSPConfig {
	return buildLookerUICSPForBaseURL("")
}

func isLocalDevURL(rawURL string) bool {
	parsed, ok := parseURLWithDefaultScheme(rawURL)
	return ok && isLoopbackHostname(parsed.Hostname())
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
	baseTagRe          = regexp.MustCompile(`(?i)<base\b[^>]*\/?>`)
	htmlTagWithNonceRe = regexp.MustCompile(`(?i)<[a-zA-Z][^>]*\s+nonce\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>"'/]+)[^>]*>`)
	nonceAttrRe        = regexp.MustCompile(`(?i)\s+nonce\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>"'/]+)`)
)

// LookerUIResourceConfig configures a LookerUIResourceManager for an MCP Apps UI scene.
type LookerUIResourceConfig struct {
	ResourceName string
	ResourceURI  string
	Description  string
	SceneName    string
	UILabel      string
}

// LookerUIResourceManager manages dynamic MCP Apps UI HTML fetching, caching,
// asset origin rewriting, and CSP origin registration for a Looker UI scene.
type LookerUIResourceManager struct {
	mu         sync.RWMutex
	src        LookerUISource
	sceneName  string
	uiLabel    string
	csp        *resources.CSPConfig
	uiResource resources.Resource
	htmlCache  *ttlCache[string]
	fetchGroup singleflight.Group
}

// NewLookerUIResourceManager constructs a LookerUIResourceManager with a
// DynamicUIResource backed by FetchRemoteUI.
func NewLookerUIResourceManager(cfg LookerUIResourceConfig) *LookerUIResourceManager {
	csp := BuildLookerUICSP()
	m := &LookerUIResourceManager{
		sceneName: cfg.SceneName,
		uiLabel:   cfg.UILabel,
		csp:       csp,
		htmlCache: newTTLCache[string](),
	}
	prefersBorder := true
	m.uiResource = resources.NewDynamicUIResource(
		cfg.ResourceName,
		cfg.ResourceURI,
		cfg.Description,
		csp,
		&prefersBorder,
		func(ctx context.Context) (string, error) {
			return m.FetchRemoteUI(ctx)
		},
	)
	return m
}

// SetSource binds the validated LookerUISource to the resource manager and
// rebuilds its base CSP using the source's configured BaseUrl (isolating
// multi-source configs from global LOOKER_BASE_URL while still enabling local
// dev ports when BaseUrl targets loopback).
func (m *LookerUIResourceManager) SetSource(src LookerUISource) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.src = src
	if src != nil && m.csp != nil {
		if settings := src.LookerApiSettings(); settings != nil && strings.TrimSpace(settings.BaseUrl) != "" {
			nextCSP := buildLookerUICSPForBaseURL(settings.BaseUrl)
			m.csp = nextCSP
			if dynRes, ok := m.uiResource.(*resources.DynamicUIResource); ok {
				dynRes.SetCSP(nextCSP)
			}
		}
	}
	m.mu.Unlock()
	if m.htmlCache != nil {
		m.htmlCache.clear()
	}
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
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil {
		return ""
	}
	if isLoopbackHostname(parsed.Hostname()) {
		return ""
	}
	return fmt.Sprintf("https://%s", strings.ToLower(parsed.Host))
}

// resolveConfiguredPublicHTTPSOrigins returns the ordered list of browser-facing
// public HTTPS origins configured for this source:
//  1. LOOKER_UI_URL
//  2. apiSettings.BaseUrl (falling back to LOOKER_BASE_URL only when the source
//     does not declare a public HTTPS BaseUrl)
//  3. window.__LOOKER_BASE_URL__ from the Looker HTML shell when no configured
//     URL provides a public HTTPS origin
func (m *LookerUIResourceManager) resolveConfiguredPublicHTTPSOrigins(apiSettings *rtl.ApiSettings, rawHTML string) []string {
	var configuredHTTPSOrigins []string
	addIfValid := func(raw string) bool {
		if origin := parsePublicHTTPSOrigin(raw); origin != "" {
			appendUnique(&configuredHTTPSOrigins, origin)
			return true
		}
		return false
	}

	addIfValid(os.Getenv("LOOKER_UI_URL"))
	hasSourceHTTPSBase := false
	if apiSettings != nil && strings.TrimSpace(apiSettings.BaseUrl) != "" {
		hasSourceHTTPSBase = addIfValid(apiSettings.BaseUrl)
	}
	if !hasSourceHTTPSBase {
		addIfValid(os.Getenv("LOOKER_BASE_URL"))
	}

	if len(configuredHTTPSOrigins) == 0 {
		if match := lookerBaseURLScriptRe.FindStringSubmatch(rawHTML); len(match) >= 2 {
			addIfValid(match[1])
		}
	}
	return configuredHTTPSOrigins
}

// RenderHTML makes the fetched UI HTML loadable inside the MCP host's sandbox.
// publicOrigin is the browser-facing Looker origin resolved from configuration
// or DiscoverMCPUIPublicOrigin. If it is empty, relative asset URLs are resolved
// against LOOKER_UI_URL, then the source BaseUrl, then LOOKER_BASE_URL.
func (m *LookerUIResourceManager) RenderHTML(rawHTML, publicOrigin string) string {
	assetBaseURL := publicOrigin
	if assetBaseURL == "" {
		assetBaseURL = extractURLOrigin(os.Getenv("LOOKER_UI_URL"))
	}
	if assetBaseURL == "" && m != nil {
		m.mu.RLock()
		src := m.src
		m.mu.RUnlock()
		if src != nil && src.LookerApiSettings() != nil {
			assetBaseURL = extractURLOrigin(src.LookerApiSettings().BaseUrl)
		}
	}
	if assetBaseURL == "" {
		assetBaseURL = extractURLOrigin(os.Getenv("LOOKER_BASE_URL"))
	}

	res := strings.ReplaceAll(rawHTML, "__MCP_ASSET_BASE_URL__", assetBaseURL)
	if assetBaseURL != "" {
		for _, q := range []string{`"`, `'`} {
			res = strings.ReplaceAll(res, fmt.Sprintf(`src=%s/webpack/`, q), fmt.Sprintf(`src=%s%s/webpack/`, q, assetBaseURL))
			res = strings.ReplaceAll(res, fmt.Sprintf(`src=%s/public/mcp/ui/assets/`, q), fmt.Sprintf(`src=%s%s/public/mcp/ui/%s/assets/`, q, assetBaseURL, m.sceneName))
			res = strings.ReplaceAll(res, fmt.Sprintf(`src=%s/public/mcp/ui/%s/assets/`, q, m.sceneName), fmt.Sprintf(`src=%s%s/public/mcp/ui/%s/assets/`, q, assetBaseURL, m.sceneName))
			res = strings.ReplaceAll(res, fmt.Sprintf(`src=%s/mcp/ui/assets/`, q), fmt.Sprintf(`src=%s%s/public/mcp/ui/%s/assets/`, q, assetBaseURL, m.sceneName))
		}
		res = strings.ReplaceAll(res, `url('/fonts/`, fmt.Sprintf(`url('%s/fonts/`, assetBaseURL))
		res = strings.ReplaceAll(res, `url("/fonts/`, fmt.Sprintf(`url("%s/fonts/`, assetBaseURL))
		res = strings.ReplaceAll(res, `url(/fonts/`, fmt.Sprintf(`url(%s/fonts/`, assetBaseURL))
	}
	// Strip any <base ...> tag to prevent CSP base-uri 'self' violation in sandboxed iframes.
	res = baseTagRe.ReplaceAllString(res, "")
	// Strip server-generated nonce attributes from HTML opening tags to prevent CSP nonce mismatch in sandboxed iframes.
	res = htmlTagWithNonceRe.ReplaceAllStringFunc(res, func(tag string) string {
		return nonceAttrRe.ReplaceAllString(tag, "")
	})
	return res
}

func (m *LookerUIResourceManager) resolveRemoteUIURL(apiSettings *rtl.ApiSettings) string {
	targetSuffix := fmt.Sprintf("/%s/assets", m.sceneName)
	if lookerUIURL := strings.TrimSpace(os.Getenv("LOOKER_UI_URL")); lookerUIURL != "" {
		parsed, ok := parseURLWithDefaultScheme(lookerUIURL)
		if !ok {
			return ""
		}
		trimmedPath := strings.TrimSuffix(parsed.Path, "/")
		switch {
		case strings.HasSuffix(trimmedPath, targetSuffix):
			parsed.Path = trimmedPath
		case strings.Contains(trimmedPath, "/public/mcp/ui/"):
			idx := strings.Index(trimmedPath, "/public/mcp/ui/")
			parsed.Path = fmt.Sprintf("%s/public/mcp/ui/%s/assets", trimmedPath[:idx], m.sceneName)
		case strings.HasSuffix(trimmedPath, "/assets"):
			parsed.Path = trimmedPath
		default:
			parsed.Path = fmt.Sprintf("%s/public/mcp/ui/%s/assets", trimmedPath, m.sceneName)
		}
		parsed.RawPath = ""
		return parsed.String()
	}

	var rawBase string
	if apiSettings != nil {
		rawBase = strings.TrimSpace(apiSettings.BaseUrl)
	}
	if rawBase == "" {
		rawBase = strings.TrimSpace(os.Getenv("LOOKER_BASE_URL"))
	}
	if parsed, ok := parseURLWithDefaultScheme(rawBase); ok {
		parsed.Path = fmt.Sprintf("%s/public/mcp/ui/%s/assets", strings.TrimSuffix(parsed.Path, "/"), m.sceneName)
		parsed.RawPath = ""
		return parsed.String()
	}
	return ""
}

func newUIHTTPClient(verifySSL bool) (*http.Client, func()) {
	client := &http.Client{Timeout: mcpUIRequestTimeout}
	if verifySSL {
		return client, func() {}
	}
	var tr *http.Transport
	if defaultTr, ok := http.DefaultTransport.(*http.Transport); ok {
		tr = defaultTr.Clone()
	} else {
		tr = &http.Transport{}
	}
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	client.Transport = tr
	return client, tr.CloseIdleConnections
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

	if m.htmlCache != nil {
		if html, ok := m.htmlCache.get(localeCacheKey); ok && html != "" {
			return html, nil
		}
	}

	val, err, _ := m.fetchGroup.Do(localeCacheKey, func() (any, error) {
		if m.htmlCache != nil {
			if html, ok := m.htmlCache.get(localeCacheKey); ok && html != "" {
				return html, nil
			}
		}
		return m.fetchAndCacheRemoteUI(ctx, apiSettings, requestedLocale, localeCacheKey)
	})
	if err != nil {
		return "", err
	}
	html, _ := val.(string)
	return html, nil
}

func (m *LookerUIResourceManager) fetchAndCacheRemoteUI(ctx context.Context, apiSettings *rtl.ApiSettings, requestedLocale, localeCacheKey string) (string, error) {
	remoteURL := m.resolveRemoteUIURL(apiSettings)
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

	client, cleanup := newUIHTTPClient(ResolveVerifySSL(ctx, apiSettings, m.uiLabel+" UI"))
	defer cleanup()

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch %s UI from Looker at %s: %w", m.uiLabel, remoteURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("tool UI access denied (HTTP %d): disabled by administrator", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("tool UI access denied (HTTP %d): unauthorized response from %s (verify LOOKER_UI_URL points to the Looker web UI port rather than the API port)", resp.StatusCode, remoteURL)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch %s UI from Looker at %s: HTTP %d", m.uiLabel, remoteURL, resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxUIHTMLResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("failed to read %s UI response from Looker at %s: %w", m.uiLabel, remoteURL, err)
	}
	if len(bodyBytes) == 0 {
		return "", fmt.Errorf("empty %s UI returned from Looker at %s", m.uiLabel, remoteURL)
	}
	if len(bodyBytes) > maxUIHTMLResponseBytes {
		return "", fmt.Errorf("%s UI response from Looker at %s exceeds maximum size (%d bytes)", m.uiLabel, remoteURL, maxUIHTMLResponseBytes)
	}

	rawHTML := string(bodyBytes)
	configuredOrigins := m.resolveConfiguredPublicHTTPSOrigins(apiSettings, rawHTML)
	var allowedOrigins []string
	if len(configuredOrigins) > 0 {
		allowedOrigins = slices.Concat(configuredOrigins, DefaultAllowedLookerCDNOrigins)
	}
	discoveredOrigin := DiscoverMCPUIPublicOrigin(rawHTML, allowedOrigins...)
	publicOrigin := discoveredOrigin
	if len(configuredOrigins) > 0 {
		publicOrigin = configuredOrigins[0]
	}
	html := m.RenderHTML(rawHTML, publicOrigin)

	m.mu.Lock()
	m.allowUIOrigin(remoteURL, publicOrigin, discoveredOrigin)
	m.mu.Unlock()

	if m.htmlCache != nil {
		m.htmlCache.set(localeCacheKey, html, UIHTMLCacheTTL)
	}

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
		AddCSPOrigin(nextCSP, extractURLOrigin(remoteURL))
	}
	m.csp = nextCSP
	if dynRes, ok := m.uiResource.(*resources.DynamicUIResource); ok {
		dynRes.SetCSP(nextCSP)
	}
}
