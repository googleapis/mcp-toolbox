// Copyright 2025 Google LLC
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
package looker

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	geminidataanalytics "cloud.google.com/go/geminidataanalytics/apiv1"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/sync/singleflight"

	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

const SourceType string = "looker"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{
		Name:               name,
		SslVerification:    true,
		Timeout:            "600s",
		UseClientOAuth:     "false",
		ShowHiddenModels:   true,
		ShowHiddenExplores: true,
		ShowHiddenFields:   true,
		Location:           "us",
		SessionLength:      1200,
	} // Default Ssl,timeout, ShowHidden
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type Config struct {
	Name               string `yaml:"name" validate:"required"`
	Type               string `yaml:"type" validate:"required"`
	BaseURL            string `yaml:"base_url" validate:"required"`
	ClientId           string `yaml:"client_id"`
	ClientSecret       string `yaml:"client_secret"`
	SslVerification    bool   `yaml:"verify_ssl"`
	UseClientOAuth     string `yaml:"use_client_oauth"`
	Timeout            string `yaml:"timeout"`
	ShowHiddenModels   bool   `yaml:"show_hidden_models"`
	ShowHiddenExplores bool   `yaml:"show_hidden_explores"`
	ShowHiddenFields   bool   `yaml:"show_hidden_fields"`
	Project            string `yaml:"project"`
	Location           string `yaml:"location"`
	QuotaProject       string `yaml:"quotaProject"`
	SessionLength      int64  `yaml:"sessionLength"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

// Initialize initializes a Looker Source instance.
func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s, err := r.newSource(ctx, tracer)
	if err != nil {
		return nil, err
	}
	if deferConnect {
		return s, nil
	}
	if _, err := s.clients(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r Config) newSource(ctx context.Context, tracer trace.Tracer) (*Source, error) {
	authTokenHeaderName := "Authorization"
	if useOAuth := strings.ToLower(r.UseClientOAuth); useOAuth != "false" && useOAuth != "true" {
		authTokenHeaderName = r.UseClientOAuth
	}
	// A malformed timeout needs no network to detect, so it fails at startup.
	timeout, err := time.ParseDuration(r.Timeout)
	if err != nil {
		return nil, fmt.Errorf("unable to parse Timeout string as time.Duration: %s", err)
	}
	// The connect logs in through the SDK, so it needs the full configured timeout.
	opts := []sources.Option{sources.WithMinConnectTimeout(timeout)}
	return &Source{
		Config:              r,
		AuthTokenHeaderName: authTokenHeaderName,
		timeout:             timeout,
		tracer:              tracer,
		conn:                sources.NewConnectOnce[*clientSet](ctx, r.Name, SourceType, tracer, opts...),
	}, nil
}

var _ sources.Source = &Source{}

// clientSet holds the API settings, whose user agent comes from the connect context, and the API-key sdk, nil under client OAuth.
type clientSet struct {
	apiSettings *rtl.ApiSettings
	sdk         *v4.LookerSDK
	tokenSource oauth2.TokenSource
}

type Source struct {
	Config
	AuthTokenHeaderName string

	timeout time.Duration
	tracer  trace.Tracer
	conn    *sources.ConnectOnce[*clientSet]

	hostURLMu       sync.RWMutex
	cachedHostURL   string
	lastFetchTime   time.Time
	lastFetchFailed bool
	lastFetchErr    error
	hostURLGroup    singleflight.Group
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) clients(ctx context.Context) (*clientSet, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*clientSet, error) {
		logger, err := util.LoggerFromContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to get logger from ctx: %s", err)
		}

		userAgent, err := util.UserAgentFromContext(ctx)
		if err != nil {
			return nil, err
		}

		if !s.SslVerification {
			logger.WarnContext(ctx, "Insecure HTTP is enabled for Looker source %s. TLS certificate verification is skipped.\n", s.Name)
		}
		cfg := rtl.ApiSettings{
			AgentTag:     userAgent,
			BaseUrl:      s.BaseURL,
			ApiVersion:   "4.0",
			VerifySsl:    s.SslVerification,
			Timeout:      int32(s.timeout.Seconds()),
			ClientId:     s.ClientId,
			ClientSecret: s.ClientSecret,
		}

		tokenSource, _ := initGoogleCloudConnection(ctx)
		cs := &clientSet{apiSettings: &cfg, tokenSource: tokenSource}

		if strings.ToLower(s.UseClientOAuth) == "false" {
			if s.ClientId == "" || s.ClientSecret == "" {
				return nil, fmt.Errorf("client_id and client_secret need to be specified")
			}
			sdk := v4.NewLookerSDK(rtl.NewAuthSession(cfg))
			resp, err := sdk.Me("", cs.apiSettings)
			if err != nil {
				return nil, fmt.Errorf("incorrect settings: %w", err)
			}
			logger.DebugContext(ctx, fmt.Sprintf("logged in as %s %s", *resp.FirstName, *resp.LastName))
			cs.sdk = sdk
		} else {
			logger.DebugContext(ctx, fmt.Sprintf("Using AuthTokenHeaderName: %s", s.AuthTokenHeaderName))
		}

		return cs, nil
	})
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) UseClientAuthorization() bool {
	return strings.ToLower(s.UseClientOAuth) != "false"
}

func (s *Source) GetAuthTokenHeaderName() string {
	return s.AuthTokenHeaderName
}

func (s *Source) GoogleCloudProject() string {
	return s.Project
}

func (s *Source) GoogleCloudLocation() string {
	return s.Location
}

func (s *Source) GoogleCloudQuotaProject() string {
	return s.QuotaProject
}

// GoogleCloudTokenSource reports the ADC token source once connected.
func (s *Source) GoogleCloudTokenSource() oauth2.TokenSource {
	cs, ok := s.conn.Get()
	if !ok {
		return nil
	}
	return cs.tokenSource
}

func (s *Source) GoogleCloudTokenSourceWithScope(ctx context.Context, scope string) (oauth2.TokenSource, error) {
	return google.DefaultTokenSource(ctx, scope)
}

// LookerClient reports the API-key SDK once connected; use GetLookerSDK for a guaranteed-live one.
func (s *Source) LookerClient() *v4.LookerSDK {
	cs, ok := s.conn.Get()
	if !ok {
		return nil
	}
	return cs.sdk
}

// LookerApiSettings is only populated after GetLookerSDK has connected; otherwise use LookerApiSettingsContext.
func (s *Source) LookerApiSettings() *rtl.ApiSettings {
	cs, ok := s.conn.Get()
	if !ok {
		return nil
	}
	return cs.apiSettings
}

// LookerApiSettingsContext returns the API settings, connecting on first use so the user agent is filled in.
func (s *Source) LookerApiSettingsContext(ctx context.Context) (*rtl.ApiSettings, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	return cs.apiSettings, nil
}

func (s *Source) LookerShowHiddenFields() bool {
	return s.ShowHiddenFields
}

func (s *Source) LookerShowHiddenModels() bool {
	return s.ShowHiddenModels
}

func (s *Source) LookerShowHiddenExplores() bool {
	return s.ShowHiddenExplores
}

func (s *Source) LookerSessionLength() int64 {
	return s.SessionLength
}

// Make types for RoundTripper
type transportWithAuthHeader struct {
	Base      http.RoundTripper
	AuthToken string
	clientIP  string
}

func (t *transportWithAuthHeader) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("x-looker-appid", "go-sdk")
	req.Header.Set("Authorization", t.AuthToken)
	if t.clientIP != "" {
		req.Header.Set("X-Forwarded-For", t.clientIP)
		req.Header.Set("X-Real-IP", t.clientIP)
	}
	return t.Base.RoundTrip(req)
}

func (s *Source) GetLookerSDK(ctx context.Context, accessToken string) (*v4.LookerSDK, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}

	if s.UseClientAuthorization() {
		if accessToken == "" {
			return nil, fmt.Errorf("no access token supplied with request")
		}

		clientIP, _ := util.ClientIPFromContext(ctx)

		session := rtl.NewAuthSession(*cs.apiSettings)
		// Configure base transport with TLS
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: !cs.apiSettings.VerifySsl,
			},
		}

		// Build transport for end user token
		session.Client = http.Client{
			Transport: &transportWithAuthHeader{
				Base:      transport,
				AuthToken: accessToken,
				clientIP:  clientIP,
			},
		}
		// return SDK with new Transport
		return v4.NewLookerSDK(session), nil
	}

	if cs.sdk == nil {
		return nil, fmt.Errorf("client id or client secret not valid")
	}
	return cs.sdk, nil
}

func initGoogleCloudConnection(ctx context.Context) (oauth2.TokenSource, error) {
	cred, err := google.FindDefaultCredentials(ctx, geminidataanalytics.DefaultAuthScopes()...)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials with scope %q: %w", geminidataanalytics.DefaultAuthScopes(), err)
	}

	return cred.TokenSource, nil
}

func (s *Source) GetHostURL(ctx context.Context, sdk *v4.LookerSDK) (string, error) {
	defaultURL := strings.TrimSuffix(s.BaseURL, "/")

	if sdk == nil {
		return defaultURL, nil
	}

	// 1. Fast path: Read lock to check cache TTL
	s.hostURLMu.RLock()
	if !s.lastFetchFailed && !s.lastFetchTime.IsZero() && time.Since(s.lastFetchTime) < 10*time.Minute {
		urlStr := s.cachedHostURL
		s.hostURLMu.RUnlock()
		return urlStr, nil
	}
	if s.lastFetchFailed && time.Since(s.lastFetchTime) < time.Minute {
		urlStr := s.cachedHostURL
		if urlStr == "" {
			urlStr = defaultURL
		}
		err := s.lastFetchErr
		s.hostURLMu.RUnlock()
		return urlStr, err
	}
	s.hostURLMu.RUnlock()

	// 2. Slow path: Use singleflight to deduplicate concurrent network calls
	res, err, _ := s.hostURLGroup.Do("get_host_url", func() (any, error) {
		// Double check within singleflight callback if another thread updated cache just before us
		s.hostURLMu.RLock()
		if !s.lastFetchFailed && !s.lastFetchTime.IsZero() && time.Since(s.lastFetchTime) < 10*time.Minute {
			urlStr := s.cachedHostURL
			s.hostURLMu.RUnlock()
			return urlStr, nil
		}
		if s.lastFetchFailed && time.Since(s.lastFetchTime) < time.Minute {
			urlStr := s.cachedHostURL
			if urlStr == "" {
				urlStr = defaultURL
			}
			err := s.lastFetchErr
			s.hostURLMu.RUnlock()
			return urlStr, err
		}
		s.hostURLMu.RUnlock()

		logger, err := util.LoggerFromContext(ctx)
		if err != nil {
			return defaultURL, err
		}

		apiSettings, err := s.LookerApiSettingsContext(ctx)
		if err != nil {
			return defaultURL, err
		}

		// Perform network call outside of locks to prevent blocking concurrent readers
		versionInfo, err := sdk.Versions("", apiSettings)
		if err != nil || versionInfo.WebServerUrl == nil || *versionInfo.WebServerUrl == "" {
			fetchErr := err
			if fetchErr == nil {
				fetchErr = fmt.Errorf("empty web_server_url in versions payload")
			}
			s.hostURLMu.Lock()
			s.lastFetchFailed = true
			s.lastFetchTime = time.Now()
			s.lastFetchErr = fetchErr
			// DO NOT overwrite s.cachedHostURL on transient errors; keep the stale valid URL for stale fallbacks
			s.hostURLMu.Unlock()

			if err != nil {
				logger.WarnContext(ctx, fmt.Sprintf("unable to retrieve host_url via versions, using stale/default fallback: %v", err))
			} else {
				logger.DebugContext(ctx, "versions web_server_url is empty, using stale/default fallback")
			}
			return "", fetchErr
		}

		// Validate the retrieved host_url is a valid absolute HTTP/HTTPS URL
		valURL := *versionInfo.WebServerUrl
		u, err := url.Parse(valURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			parseErr := fmt.Errorf("invalid web_server_url format: %q", valURL)
			s.hostURLMu.Lock()
			s.lastFetchFailed = true
			s.lastFetchTime = time.Now()
			s.lastFetchErr = parseErr
			s.hostURLMu.Unlock()
			logger.WarnContext(ctx, fmt.Sprintf("invalid host_url setting retrieved %q, using stale/default fallback: %v", valURL, err))
			return "", parseErr
		}

		resolvedURL := strings.TrimSuffix(valURL, "/")

		s.hostURLMu.Lock()
		s.cachedHostURL = resolvedURL
		s.lastFetchTime = time.Now()
		s.lastFetchFailed = false
		s.lastFetchErr = nil
		s.hostURLMu.Unlock()

		logger.DebugContext(ctx, "successfully fetched and cached host_url: "+resolvedURL)
		return resolvedURL, nil
	})

	if err != nil {
		// If singleflight failed, return last known good cache, else defaultURL
		s.hostURLMu.RLock()
		defer s.hostURLMu.RUnlock()
		if s.cachedHostURL != "" {
			return s.cachedHostURL, err
		}
		return defaultURL, err
	}

	return res.(string), nil
}
