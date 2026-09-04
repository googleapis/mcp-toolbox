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

package databaseinsights

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const SourceKind string = "databaseinsights"

const defaultEndpoint string = "https://databaseinsights.googleapis.com"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceKind, newConfig) {
		panic(fmt.Sprintf("source kind %q already registered", SourceKind))
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
	Name     string `yaml:"name" validate:"required"`
	Type     string `yaml:"type" validate:"required"`
	Project  string `yaml:"project"`  // Optional: fallback billing project
	Endpoint string `yaml:"endpoint"` // Optional: override endpoint for staging/testing
}

func (cfg Config) SourceConfigType() string {
	return SourceKind
}

func (cfg Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s := cfg.newSource(ctx, tracer)
	if deferConnect {
		return s, nil
	}
	if _, err := s.client(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (cfg Config) newSource(ctx context.Context, tracer trace.Tracer) *Source {
	// Derived from config alone so it resolves on an unconnected source.
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return &Source{
		Config:   cfg,
		tracer:   tracer,
		endpoint: endpoint,
		conn:     sources.NewConnectOnce[*http.Client](ctx, cfg.Name, SourceKind, tracer),
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	tracer   trace.Tracer
	endpoint string
	conn     *sources.ConnectOnce[*http.Client]
}

func (s *Source) client(ctx context.Context) (*http.Client, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*http.Client, error) {
		return initConnection(ctx, s.Project)
	})
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	return SourceKind
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

// HTTPClient reports the client once connected.
func (s *Source) HTTPClient() *http.Client {
	client, _ := s.conn.Get()
	return client
}

func (s *Source) APIEndpoint() string {
	return s.endpoint
}

func (s *Source) ProjectID() string {
	return s.Project
}

func initConnection(
	ctx context.Context,
	project string,
) (*http.Client, error) {
	cred, err := google.FindDefaultCredentials(ctx, sources.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials with scope %q: %w", sources.CloudPlatformScope, err)
	}

	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Create authenticated HTTP client using the credentials token source
	httpClient := oauth2.NewClient(ctx, cred.TokenSource)
	httpClient.Transport = &authHeadersRoundTripper{
		configProject: project,
		adcProject:    cred.ProjectID,
		userAgent:     userAgent,
		next:          httpClient.Transport,
	}

	return httpClient, nil
}

type authHeadersRoundTripper struct {
	configProject string
	adcProject    string
	userAgent     string
	next          http.RoundTripper
}

func (rt *authHeadersRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	newReq := req.Clone(req.Context())

	// Inject User-Agent
	ua := newReq.Header.Get("User-Agent")
	if ua == "" {
		newReq.Header.Set("User-Agent", rt.userAgent)
	} else {
		newReq.Header.Set("User-Agent", ua+" "+rt.userAgent)
	}

	// Determine the billing/quota project to use with correct precedence:
	// 1. YAML Config (rt.configProject)
	// 2. Extracted URL Path
	// 3. ADC credentials (rt.adcProject)
	var quotaProject string
	if rt.configProject != "" {
		quotaProject = rt.configProject
	} else if extracted := extractProjectFromPath(newReq.URL.Path); extracted != "" {
		quotaProject = extracted
	} else {
		quotaProject = rt.adcProject
	}

	// Inject billing/quota project header required for ADC authentication if not already set
	if quotaProject != "" && newReq.Header.Get("X-Goog-User-Project") == "" {
		newReq.Header.Set("X-Goog-User-Project", quotaProject)
	}

	return rt.next.RoundTrip(newReq)
}

func extractProjectFromPath(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "projects" {
			return parts[i+1]
		}
	}
	return ""
}

func extractLocationFromParent(parent string) string {
	parts := strings.Split(strings.TrimPrefix(parent, "/"), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "locations" {
			return parts[i+1]
		}
	}
	return ""
}

func (s *Source) getEndpointForParent(parent string) string {
	if s.endpoint != "" && s.endpoint != defaultEndpoint {
		return s.endpoint
	}
	location := extractLocationFromParent(parent)
	if location != "" && location != "global" {
		return fmt.Sprintf("https://%s-databaseinsights.googleapis.com", location)
	}
	return defaultEndpoint
}

// FetchQueryStatsRequest is the payload for fetching query execution stats.
type FetchQueryStatsRequest struct {
	Parent           string `json:"parent"`
	FullResourceName string `json:"fullResourceName"`
	StartTime        string `json:"startTime,omitempty"`
	EndTime          string `json:"endTime,omitempty"`
	Database         string `json:"database,omitempty"`
	Username         string `json:"username,omitempty"`
	QueryID          string `json:"queryId,omitempty"`
	PageSize         int32  `json:"pageSize,omitempty"`
	PageToken        string `json:"pageToken,omitempty"`
}

// Field represents a column schema in a result set metadata.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ResultSetMetadata contains column definitions.
type ResultSetMetadata struct {
	Fields []Field `json:"fields"`
}

// FetchQueryStatsResponse contains the results.
type FetchQueryStatsResponse struct {
	Results   [][]any           `json:"results,omitempty"`
	Metadata  ResultSetMetadata `json:"metadata,omitempty"`
	PageToken string            `json:"nextPageToken,omitempty"`
}

// FetchWaitEventStatsRequest is the payload for wait event stats.
type FetchWaitEventStatsRequest struct {
	Parent           string `json:"parent"`
	FullResourceName string `json:"fullResourceName"`
	StartTime        string `json:"startTime,omitempty"`
	EndTime          string `json:"endTime,omitempty"`
	Database         string `json:"database,omitempty"`
	Username         string `json:"username,omitempty"`
	QueryID          string `json:"queryId,omitempty"`
	PageSize         int32  `json:"pageSize,omitempty"`
	PageToken        string `json:"pageToken,omitempty"`
	View             string `json:"view,omitempty"`
}

// FetchWaitEventStatsResponse contains wait event stats results.
type FetchWaitEventStatsResponse struct {
	Results   [][]any           `json:"results,omitempty"`
	Metadata  ResultSetMetadata `json:"metadata,omitempty"`
	PageToken string            `json:"nextPageToken,omitempty"`
}

// FetchQueryTimeSeriesRequest payload.
type FetchQueryTimeSeriesRequest struct {
	Parent           string `json:"parent"`
	FullResourceName string `json:"fullResourceName"`
	StartTime        string `json:"startTime,omitempty"`
	EndTime          string `json:"endTime,omitempty"`
	Database         string `json:"database,omitempty"`
	Username         string `json:"username,omitempty"`
	QueryID          string `json:"queryId,omitempty"`
}

// FetchQueryTimeSeriesResponse contains the time-series stats.
type FetchQueryTimeSeriesResponse struct {
	TimeSeries []TimeSeries      `json:"timeseries,omitempty"`
	Metadata   ResultSetMetadata `json:"metadata,omitempty"`
}

type TimeSeries struct {
	GroupbyFieldValues []string                 `json:"groupbyFieldValues,omitempty"`
	Values             []TimeSeriesMetricValues `json:"values,omitempty"`
}

type TimeSeriesMetricValues struct {
	Interval Interval       `json:"interval,omitempty"`
	Value    any            `json:"value,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Interval struct {
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`
}

// FetchWaitEventTimeSeriesRequest payload.
type FetchWaitEventTimeSeriesRequest struct {
	Parent           string `json:"parent"`
	FullResourceName string `json:"fullResourceName"`
	StartTime        string `json:"startTime,omitempty"`
	EndTime          string `json:"endTime,omitempty"`
	Database         string `json:"database,omitempty"`
	Username         string `json:"username,omitempty"`
	QueryID          string `json:"queryId,omitempty"`
	View             string `json:"view,omitempty"`
}

// FetchWaitEventTimeSeriesResponse contains wait event time series.
type FetchWaitEventTimeSeriesResponse struct {
	TimeSeries []TimeSeries      `json:"timeseries,omitempty"`
	Metadata   ResultSetMetadata `json:"metadata,omitempty"`
}

// DatabaseQueryIds represents a database name and a list of query IDs.
type DatabaseQueryIds struct {
	Database string   `json:"database"`
	QueryIDs []string `json:"queryIds"`
}

// BatchQueryIndexRecommendationsRequest is the payload for fetching index recommendations.
type BatchQueryIndexRecommendationsRequest struct {
	Parent           string             `json:"parent"`
	FullResourceName string             `json:"fullResourceName"`
	DatabaseQueryIds []DatabaseQueryIds `json:"databaseQueryIds"`
}

// IndexRecommendation represents a single index suggestion.
type IndexRecommendation struct {
	SQLCommand                string   `json:"sqlCommand"`
	Schema                    string   `json:"schema"`
	Relation                  string   `json:"relation"`
	Columns                   []string `json:"columns"`
	EstimatedStorageSizeBytes int64    `json:"estimatedStorageSizeBytes,string"`
	ImpactedQueryIds          []string `json:"impactedQueryIds"`
}

// QueryImprovement details performance gains.
type QueryImprovement struct {
	QueryID                            string   `json:"queryId"`
	IndexRecommendationIds             []string `json:"indexRecommendationIds"`
	CurrentTotalExecutionDuration      string   `json:"currentTotalExecutionDuration"`
	EstimatedNewTotalExecutionDuration string   `json:"estimatedNewTotalExecutionDuration"`
}

// DatabaseIndexRecommendation represents recommendations for a specific database.
type DatabaseIndexRecommendation struct {
	Database             string                      `json:"database"`
	IndexRecommendations []IndexRecommendation       `json:"indexRecommendations"`
	QueryImprovements    map[string]QueryImprovement `json:"queryImprovements"`
}

// BatchQueryIndexRecommendationsResponse contains index recommendations.
type BatchQueryIndexRecommendationsResponse struct {
	DatabaseIndexRecommendations []DatabaseIndexRecommendation `json:"databaseIndexRecommendations"`
}

// FetchQueryStats executes the FetchQueryStats REST API method.
func (s *Source) FetchQueryStats(ctx context.Context, req *FetchQueryStatsRequest) (*FetchQueryStatsResponse, error) {
	url := fmt.Sprintf("%s/v1beta/%s/queryStats:fetch", s.getEndpointForParent(req.Parent), req.Parent)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %s: %s", resp.Status, string(respBody))
	}

	var fetchResp FetchQueryStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&fetchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &fetchResp, nil
}

// FetchWaitEventStats executes the FetchWaitEventStats REST API method.
func (s *Source) FetchWaitEventStats(ctx context.Context, req *FetchWaitEventStatsRequest) (*FetchWaitEventStatsResponse, error) {
	url := fmt.Sprintf("%s/v1beta/%s/waitEventStats:fetch", s.getEndpointForParent(req.Parent), req.Parent)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %s: %s", resp.Status, string(respBody))
	}

	var fetchResp FetchWaitEventStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&fetchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &fetchResp, nil
}

// FetchQueryTimeSeries executes the FetchQueryTimeSeries REST API method.
func (s *Source) FetchQueryTimeSeries(ctx context.Context, req *FetchQueryTimeSeriesRequest) (*FetchQueryTimeSeriesResponse, error) {
	url := fmt.Sprintf("%s/v1beta/%s/queryTimeSeries:fetch", s.getEndpointForParent(req.Parent), req.Parent)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %s: %s", resp.Status, string(respBody))
	}

	var fetchResp FetchQueryTimeSeriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&fetchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &fetchResp, nil
}

// FetchWaitEventTimeSeries executes the FetchWaitEventTimeSeries REST API method.
func (s *Source) FetchWaitEventTimeSeries(ctx context.Context, req *FetchWaitEventTimeSeriesRequest) (*FetchWaitEventTimeSeriesResponse, error) {
	url := fmt.Sprintf("%s/v1beta/%s/waitEventTimeSeries:fetch", s.getEndpointForParent(req.Parent), req.Parent)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %s: %s", resp.Status, string(respBody))
	}

	var fetchResp FetchWaitEventTimeSeriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&fetchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &fetchResp, nil
}

// BatchQueryIndexRecommendations executes the BatchQueryIndexRecommendations REST API method.
func (s *Source) BatchQueryIndexRecommendations(ctx context.Context, req *BatchQueryIndexRecommendationsRequest) (*BatchQueryIndexRecommendationsResponse, error) {
	url := fmt.Sprintf("%s/v1beta/%s/indexRecommendations:batchQuery", s.getEndpointForParent(req.Parent), req.Parent)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %s: %s", resp.Status, string(respBody))
	}

	var fetchResp BatchQueryIndexRecommendationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&fetchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &fetchResp, nil
}
