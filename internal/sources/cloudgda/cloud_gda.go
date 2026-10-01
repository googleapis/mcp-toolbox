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
package cloudgda

import (
	"context"
	"fmt"

	geminidataanalytics "cloud.google.com/go/geminidataanalytics/apiv1beta"
	"cloud.google.com/go/geminidataanalytics/apiv1beta/geminidataanalyticspb"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

const SourceType string = "cloud-gemini-data-analytics"
const CloudPlatformScope string = "https://www.googleapis.com/auth/cloud-platform"

// NewDataChatClient can be overridden for testing.
var NewDataChatClient = geminidataanalytics.NewDataChatClient

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
	ProjectID      string `yaml:"projectId" validate:"required"`
	UseClientOAuth bool   `yaml:"useClientOAuth"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

// Initialize initializes a Gemini Data Analytics Source instance.
func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	ua, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("error in User Agent retrieval: %s", err)
	}

	s := &Source{
		Config:    r,
		userAgent: ua,
	}

	if !r.UseClientOAuth {
		client, err := NewDataChatClient(ctx, option.WithUserAgent(ua))
		if err != nil {
			return nil, fmt.Errorf("failed to create DataChatClient: %w", err)
		}
		s.Client = client
	}

	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	Client    *geminidataanalytics.DataChatClient
	userAgent string
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

func (s *Source) GetProjectID() string {
	return s.ProjectID
}

func (s *Source) GoogleCloudTokenSourceWithScope(ctx context.Context, scope string) (oauth2.TokenSource, error) {
	if scope == "" {
		scope = CloudPlatformScope
	}

	creds, err := google.FindDefaultCredentials(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default credentials: %w", err)
	}
	return creds.TokenSource, nil
}

func (s *Source) UseClientAuthorization() bool {
	return s.UseClientOAuth
}

func (s *Source) GetClient(ctx context.Context, tokenStr string) (*geminidataanalytics.DataChatClient, func(), error) {
	if s.UseClientOAuth {
		if tokenStr == "" {
			return nil, nil, fmt.Errorf("client-side OAuth is enabled but no access token was provided")
		}
		token := &oauth2.Token{AccessToken: tokenStr}
		opts := []option.ClientOption{
			option.WithUserAgent(s.userAgent),
			option.WithTokenSource(oauth2.StaticTokenSource(token)),
		}

		client, err := NewDataChatClient(ctx, opts...)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create per-request DataChatClient: %w", err)
		}
		return client, func() { client.Close() }, nil
	}
	return s.Client, func() {}, nil
}

func (s *Source) RunQuery(ctx context.Context, tokenStr string, req *geminidataanalyticspb.QueryDataRequest) (*geminidataanalyticspb.QueryDataResponse, error) {
	client, cleanup, err := s.GetClient(ctx, tokenStr)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	return client.QueryData(ctx, req)
}
