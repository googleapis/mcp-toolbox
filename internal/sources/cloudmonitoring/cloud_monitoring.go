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
package cloudmonitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	monitoring "google.golang.org/api/monitoring/v3"
)

const SourceType string = "cloud-monitoring"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type Config struct {
	Name           string `yaml:"name" validate:"required"`
	Type           string `yaml:"type" validate:"required"`
	UseClientOAuth bool   `yaml:"useClientOAuth"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

// Initialize initializes a Cloud Monitoring Source instance.
func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s := r.newSource(ctx, tracer)
	if deferConnect {
		return s, nil
	}
	if _, err := s.clients(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r Config) newSource(ctx context.Context, tracer trace.Tracer) *Source {
	return &Source{
		Config:  r,
		baseURL: "https://monitoring.googleapis.com",
		tracer:  tracer,
		conn:    sources.NewConnectOnce[*clientSet](ctx, r.Name, SourceType, tracer),
	}
}

// clientSet pairs the client with the user agent read from the same connect context.
type clientSet struct {
	client    *http.Client
	userAgent string
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	baseURL string
	tracer  trace.Tracer
	conn    *sources.ConnectOnce[*clientSet]
}

func (s *Source) clients(ctx context.Context) (*clientSet, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*clientSet, error) {
		ua, err := util.UserAgentFromContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("error in User Agent retrieval: %s", err)
		}

		var client *http.Client
		if s.UseClientOAuth {
			client = &http.Client{
				Transport: util.NewUserAgentRoundTripper(ua, http.DefaultTransport),
			}
		} else {
			// Use Application Default Credentials
			creds, err := google.FindDefaultCredentials(ctx, monitoring.MonitoringScope)
			if err != nil {
				return nil, fmt.Errorf("failed to find default credentials: %w", err)
			}
			baseClient := oauth2.NewClient(ctx, creds.TokenSource)
			baseClient.Transport = util.NewUserAgentRoundTripper(ua, baseClient.Transport)
			client = baseClient
		}

		return &clientSet{client: client, userAgent: ua}, nil
	})
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) BaseURL() string {
	return s.baseURL
}

// Client reports the HTTP client once connected.
func (s *Source) Client() *http.Client {
	cs, ok := s.conn.Get()
	if !ok {
		return nil
	}
	return cs.client
}

// UserAgent reports the user agent once connected.
func (s *Source) UserAgent() string {
	cs, ok := s.conn.Get()
	if !ok {
		return ""
	}
	return cs.userAgent
}

func (s *Source) GetClient(ctx context.Context, accessToken string) (*http.Client, error) {
	if s.UseClientOAuth {
		if accessToken == "" {
			return nil, fmt.Errorf("client-side OAuth is enabled but no access token was provided")
		}
		token := &oauth2.Token{AccessToken: accessToken}
		return oauth2.NewClient(ctx, oauth2.StaticTokenSource(token)), nil
	}
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	return cs.client, nil
}

func (s *Source) UseClientAuthorization() bool {
	return s.UseClientOAuth
}

func (s *Source) RunQuery(ctx context.Context, projectID, query string) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v1/projects/%s/location/global/prometheus/api/v1/query", s.BaseURL(), projectID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("query", query)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("User-Agent", cs.userAgent)

	resp, err := cs.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed: %s, body: %s", resp.Status, string(body))
	}

	if len(body) == 0 {
		return nil, nil
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %w, body: %s", err, string(body))
	}

	return result, nil
}
