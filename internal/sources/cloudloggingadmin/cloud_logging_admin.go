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
package cloudloggingadmin

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const SourceType string = "cloud-logging-admin"

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
	Name                      string `yaml:"name" validate:"required"`
	Type                      string `yaml:"type" validate:"required"`
	Project                   string `yaml:"project" validate:"required"`
	UseClientOAuth            bool   `yaml:"useClientOAuth"`
	ImpersonateServiceAccount string `yaml:"impersonateServiceAccount"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {

	if r.UseClientOAuth && r.ImpersonateServiceAccount != "" {
		return nil, fmt.Errorf("useClientOAuth cannot be used with impersonateServiceAccount")
	}

	var client *logadmin.Client
	var tokenSource oauth2.TokenSource
	var clientCreator LogAdminClientCreator
	var err error

	s := &Source{
		Config:        r,
		Client:        client,
		TokenSource:   tokenSource,
		ClientCreator: clientCreator,
	}

	if r.UseClientOAuth {
		// use client OAuth
		baseClientCreator, err := newLogAdminClientCreator(ctx, tracer, r.Project, r.Name)
		if err != nil {
			return nil, fmt.Errorf("error constructing client creator: %w", err)
		}
		setupClientCaching(s, baseClientCreator)
	} else {
		client, tokenSource, err = initLogAdminConnection(ctx, tracer, r.Name, r.Project, r.ImpersonateServiceAccount)
		if err != nil {
			return nil, fmt.Errorf("error creating client from ADC %w", err)
		}
		s.Client = client
		s.TokenSource = tokenSource
	}
	return s, nil
}

var _ sources.Source = &Source{}

type LogAdminClientCreator func(tokenString string) (*logadmin.Client, error)

type Source struct {
	Config
	Client        *logadmin.Client
	TokenSource   oauth2.TokenSource
	ClientCreator LogAdminClientCreator

	// Caches for OAuth clients
	logadminClientCache *sources.Cache
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	// Returns logadmin source type
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) UseClientAuthorization() bool {
	return s.UseClientOAuth
}

func (s *Source) LogAdminClient() *logadmin.Client {
	return s.Client
}

func (s *Source) LogAdminTokenSource() oauth2.TokenSource {
	return s.TokenSource
}

func (s *Source) LogAdminClientCreator() LogAdminClientCreator {
	return s.ClientCreator
}

func (s *Source) GetProject() string {
	return s.Project
}

// getClient returns the appropriate client based on authentication mode
func (s *Source) getClient(accessToken string) (*logadmin.Client, error) {
	if s.UseClientOAuth {
		if s.ClientCreator == nil {
			return nil, fmt.Errorf("client creator is not initialized")
		}
		return s.ClientCreator(accessToken)
	}
	if s.Client == nil {
		return nil, fmt.Errorf("source client is not initialized")
	}
	return s.Client, nil
}

// ListLogNames lists all log names in the project
func (s *Source) ListLogNames(ctx context.Context, limit int, accessToken string) ([]string, error) {
	client, err := s.getClient(accessToken)
	if err != nil {
		return nil, err
	}

	it := client.Logs(ctx)
	var logNames []string
	for len(logNames) < limit {
		logName, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		logNames = append(logNames, logName)
	}
	return logNames, nil
}

// ListResourceTypes lists all resource types in the project
func (s *Source) ListResourceTypes(ctx context.Context, accessToken string) ([]string, error) {
	client, err := s.getClient(accessToken)
	if err != nil {
		return nil, err
	}

	it := client.ResourceDescriptors(ctx)
	var types []string
	for {
		desc, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list resource descriptors: %w", err)
		}
		types = append(types, desc.Type)
	}
	slices.Sort(types)
	return types, nil
}

// QueryLogsParams contains the parameters for querying logs
type QueryLogsParams struct {
	Filter      string
	NewestFirst bool
	StartTime   string
	EndTime     string
	Verbose     bool
	Limit       int
}

// QueryLogs queries log entries based on the provided parameters
func (s *Source) QueryLogs(ctx context.Context, params QueryLogsParams, accessToken string) ([]map[string]any, error) {
	client, err := s.getClient(accessToken)
	if err != nil {
		return nil, err
	}

	// Build filter
	var filterParts []string
	if params.Filter != "" {
		filterParts = append(filterParts, params.Filter)
	}

	// Add timestamp filter
	startTime := params.StartTime
	if startTime != "" {
		filterParts = append(filterParts, fmt.Sprintf(`timestamp>="%s"`, startTime))
	}

	if params.EndTime != "" {
		filterParts = append(filterParts, fmt.Sprintf(`timestamp<="%s"`, params.EndTime))
	}

	combinedFilter := strings.Join(filterParts, " AND ")

	// Add opts
	opts := []logadmin.EntriesOption{
		logadmin.Filter(combinedFilter),
	}

	// Set order
	if params.NewestFirst {
		opts = append(opts, logadmin.NewestFirst())
	}

	// Set up iterator
	it := client.Entries(ctx, opts...)

	var results []map[string]any
	for len(results) < params.Limit {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate entries: %w", err)
		}

		result := map[string]any{
			"logName":   entry.LogName,
			"timestamp": entry.Timestamp.Format(time.RFC3339),
			"severity":  entry.Severity.String(),
			"resource": map[string]any{
				"type":   entry.Resource.Type,
				"labels": entry.Resource.Labels,
			},
		}

		if entry.Payload != nil {
			result["payload"] = entry.Payload
		}

		if params.Verbose {
			result["insertId"] = entry.InsertID

			if len(entry.Labels) > 0 {
				result["labels"] = entry.Labels
			}

			if entry.HTTPRequest != nil {
				httpRequestMap := map[string]any{
					"status":   entry.HTTPRequest.Status,
					"latency":  entry.HTTPRequest.Latency.String(),
					"remoteIp": entry.HTTPRequest.RemoteIP,
				}
				if req := entry.HTTPRequest.Request; req != nil {
					httpRequestMap["requestMethod"] = req.Method
					httpRequestMap["requestUrl"] = req.URL.String()
					httpRequestMap["userAgent"] = req.UserAgent()
				}
				result["httpRequest"] = httpRequestMap
			}

			if entry.Trace != "" {
				result["trace"] = entry.Trace
			}

			if entry.SpanID != "" {
				result["spanId"] = entry.SpanID
			}

			if entry.Operation != nil {
				result["operation"] = map[string]any{
					"id":       entry.Operation.Id,
					"producer": entry.Operation.Producer,
					"first":    entry.Operation.First,
					"last":     entry.Operation.Last,
				}
			}

			if entry.SourceLocation != nil {
				result["sourceLocation"] = map[string]any{
					"file":     entry.SourceLocation.File,
					"line":     entry.SourceLocation.Line,
					"function": entry.SourceLocation.Function,
				}
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func setupClientCaching(s *Source, baseCreator LogAdminClientCreator) {
	onEvict := func(key string, value interface{}) {
		if client, ok := value.(*logadmin.Client); ok && client != nil {
			client.Close()
		}
	}

	s.logadminClientCache = sources.NewCache(onEvict)

	s.ClientCreator = func(tokenString string) (*logadmin.Client, error) {
		if val, found := s.logadminClientCache.Get(tokenString); found {
			return val.(*logadmin.Client), nil
		}

		client, err := baseCreator(tokenString)
		if err != nil {
			return nil, err
		}
		s.logadminClientCache.Set(tokenString, client)
		return client, nil
	}
}

func initLogAdminConnection(
	ctx context.Context,
	tracer trace.Tracer,
	name string,
	project string,
	impersonateServiceAccount string,
) (*logadmin.Client, oauth2.TokenSource, error) {
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	var tokenSource oauth2.TokenSource
	var opts []option.ClientOption

	if impersonateServiceAccount != "" {
		// Create impersonated credentials token source with cloud-platform scope
		// This broader scope is needed for tools like conversational analytics
		cloudPlatformTokenSource, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: impersonateServiceAccount,
			Scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
		})

		if err != nil {
			return nil, nil, fmt.Errorf("failed to create impersonated credentials for %q: %w", impersonateServiceAccount, err)
		}

		tokenSource = cloudPlatformTokenSource
		opts = []option.ClientOption{
			option.WithUserAgent(userAgent),
			option.WithTokenSource(cloudPlatformTokenSource),
		}
	} else {
		// Use default credentials
		cred, err := google.FindDefaultCredentials(ctx, logging.AdminScope)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to find default Google Cloud credentials with scope %q: %w", logging.AdminScope, err)
		}
		tokenSource = cred.TokenSource
		opts = []option.ClientOption{
			option.WithUserAgent(userAgent),
			option.WithCredentials(cred),
		}
	}

	client, err := logadmin.NewClient(ctx, project, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Cloud Logging Admin client for project %q: %w", project, err)
	}
	return client, tokenSource, nil
}

func initLogAdminConnectionWithOAuthToken(
	ctx context.Context,
	tracer trace.Tracer,
	project, name, userAgent, tokenString string,
) (*logadmin.Client, error) {
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	token := &oauth2.Token{
		AccessToken: string(tokenString),
	}
	ts := oauth2.StaticTokenSource(token)

	// Initialize the logadmin client with tokenSource
	client, err := logadmin.NewClient(ctx, project, option.WithUserAgent(userAgent), option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create logadmin client for project %q: %w", project, err)
	}
	return client, nil
}

func newLogAdminClientCreator(
	ctx context.Context,
	tracer trace.Tracer,
	project, name string,
) (LogAdminClientCreator, error) {
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}

	return func(tokenString string) (*logadmin.Client, error) {
		return initLogAdminConnectionWithOAuthToken(ctx, tracer, project, name, userAgent, tokenString)
	}, nil
}
