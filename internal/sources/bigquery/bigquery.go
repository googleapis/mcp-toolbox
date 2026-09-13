// Copyright 2025 Google LLC
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

package bigquery

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	bigqueryapi "cloud.google.com/go/bigquery"
	dataplexapi "cloud.google.com/go/dataplex/apiv1"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/dataplex/searchcatalog"
	"github.com/googleapis/mcp-toolbox/internal/sources/sqlcommenter"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	bigqueryrestapi "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const SourceType string = "bigquery"

// CloudPlatformScope is a broad scope for Google Cloud Platform services.
const CloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

const (
	// No write operations are allowed.
	WriteModeBlocked string = "blocked"
	// Only protected write operations are allowed in a BigQuery session.
	WriteModeProtected string = "protected"
	// All write operations are allowed.
	WriteModeAllowed string = "allowed"
)

// validate interface
var _ sources.SourceConfig = Config{}

type BigqueryClientCreator func(tokenString string, wantRestService bool) (*bigqueryapi.Client, *bigqueryrestapi.Service, error)

type BigQuerySessionProvider func(ctx context.Context) (*Session, error)

type DataplexClientCreator func(tokenString string) (*dataplexapi.CatalogClient, error)

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name, MaxQueryResultRows: 50}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	if actual.WriteMode == "" {
		actual.WriteMode = WriteModeAllowed
		if actual.ReadOnly != nil && *actual.ReadOnly {
			actual.WriteMode = WriteModeBlocked
		}
	}
	return actual, nil
}

type Config struct {
	// BigQuery configs
	Name                      string              `yaml:"name" validate:"required"`
	Type                      string              `yaml:"type" validate:"required"`
	Project                   string              `yaml:"project" validate:"required"`
	Location                  string              `yaml:"location"`
	ReadOnly                  *bool               `yaml:"readOnly,omitempty"`
	WriteMode                 string              `yaml:"writeMode" validate:"omitempty,oneof=blocked protected allowed"`
	AllowedDatasets           StringOrStringSlice `yaml:"allowedDatasets"`
	UseClientOAuth            string              `yaml:"useClientOAuth"`
	QuotaProject              string              `yaml:"quotaProject"`
	ImpersonateServiceAccount string              `yaml:"impersonateServiceAccount"`
	Scopes                    StringOrStringSlice `yaml:"scopes"`
	MaxQueryResultRows        int                 `yaml:"maxQueryResultRows"`
	MaximumBytesBilled        int64               `yaml:"maximumBytesBilled" validate:"gte=0"`
	APIEndpoint               string              `yaml:"apiEndpoint"`
	SQLCommenter              *bool               `yaml:"sqlCommenter"`
}

// StringOrStringSlice is a custom type that can unmarshal both a single string
// (which it splits by comma) and a sequence of strings into a string slice.
type StringOrStringSlice []string

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *StringOrStringSlice) UnmarshalYAML(unmarshal func(any) error) error {
	var v any
	if err := unmarshal(&v); err != nil {
		return err
	}
	switch val := v.(type) {
	case string:
		*s = strings.Split(val, ",")
		return nil
	case []any:
		for _, item := range val {
			if str, ok := item.(string); ok {
				*s = append(*s, str)
			} else {
				return fmt.Errorf("element in sequence is not a string: %v", item)
			}
		}
		return nil
	}
	return fmt.Errorf("cannot unmarshal %T into StringOrStringSlice", v)
}

func (r Config) SourceConfigType() string {
	// Returns BigQuery source type
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	if r.WriteMode == "" {
		r.WriteMode = WriteModeAllowed
		if r.ReadOnly != nil && *r.ReadOnly {
			r.WriteMode = WriteModeBlocked
		}
	}

	if r.WriteMode != WriteModeAllowed && r.WriteMode != WriteModeBlocked && r.WriteMode != WriteModeProtected {
		return nil, fmt.Errorf("invalid writeMode %q: must be one of %q, %q, or %q", r.WriteMode, WriteModeAllowed, WriteModeProtected, WriteModeBlocked)
	}

	if r.ReadOnly != nil {
		// A writeMode is considered a read-only mode if it is Blocked or Protected.
		isReadOnlyMode := (r.WriteMode == WriteModeBlocked || r.WriteMode == WriteModeProtected)

		// The declared readOnly boolean must match the writeMode's behavior.
		if *r.ReadOnly != isReadOnlyMode {
			return nil, fmt.Errorf("conflicting source configuration: readOnly is %v, but writeMode is %q", *r.ReadOnly, r.WriteMode)
		}
	}

	if r.MaxQueryResultRows == 0 {
		r.MaxQueryResultRows = 50
	}

	if r.WriteMode == WriteModeProtected && strings.ToLower(r.UseClientOAuth) != "false" && r.UseClientOAuth != "" {
		// The protected mode only allows write operations to the session's temporary datasets.
		// when using client OAuth, a new session is created every
		// time a BigQuery tool is invoked. Therefore, no session data can
		// be preserved as needed by the protected mode.
		return nil, fmt.Errorf("writeMode 'protected' cannot be used with useClientOAuth enabled")
	}

	if strings.ToLower(r.UseClientOAuth) != "false" && r.UseClientOAuth != "" && r.ImpersonateServiceAccount != "" {
		return nil, fmt.Errorf("useClientOAuth cannot be used with impersonateServiceAccount")
	}

	endpoint := NormalizeEndpoint(r.APIEndpoint)

	var client *bigqueryapi.Client
	var restService *bigqueryrestapi.Service
	var tokenSource oauth2.TokenSource
	var clientCreator BigqueryClientCreator
	var err error

	s := &Source{
		Config:              r,
		Client:              client,
		RestService:         restService,
		TokenSource:         tokenSource,
		ClientCreator:       clientCreator,
		AuthTokenHeaderName: "Authorization",
	}

	if strings.ToLower(r.UseClientOAuth) == "false" || r.UseClientOAuth == "" {
		// Initializes a BigQuery Google SQL source
		client, restService, tokenSource, err = initBigQueryConnection(ctx, tracer, r.Name, r.Project, r.Location, r.QuotaProject, r.ImpersonateServiceAccount, r.Scopes, endpoint)
		if err != nil {
			return nil, fmt.Errorf("error creating client from ADC: %w", err)
		}
		s.Client = client
		s.RestService = restService
		s.TokenSource = tokenSource

		if r.WriteMode == WriteModeProtected {
			// session-based connections
			s.SessionProvider = s.newBigQuerySessionProvider()
		}
	} else {
		if strings.ToLower(r.UseClientOAuth) != "true" {
			s.AuthTokenHeaderName = r.UseClientOAuth
		}
		// use client OAuth
		baseClientCreator, err := newBigQueryClientCreator(ctx, tracer, r.Project, r.Location, r.QuotaProject, r.Name, endpoint)
		if err != nil {
			return nil, fmt.Errorf("error constructing client creator: %w", err)
		}
		setupClientCaching(s, baseClientCreator)
	}

	allowedDatasets := make(map[string]struct{})
	// Get full id of allowed datasets and verify they exist.
	if len(r.AllowedDatasets) > 0 {
		for _, allowed := range r.AllowedDatasets {
			var projectID, datasetID, allowedFullID string
			if strings.Contains(allowed, ".") {
				parts := strings.Split(allowed, ".")
				if len(parts) != 2 {
					return nil, fmt.Errorf("invalid allowedDataset format: %q, expected 'project.dataset' or 'dataset'", allowed)
				}
				projectID = parts[0]
				datasetID = parts[1]
				allowedFullID = allowed
			} else {
				projectID = r.Project
				datasetID = allowed
				allowedFullID = fmt.Sprintf("%s.%s", projectID, datasetID)
			}

			if s.Client != nil {
				dataset := s.Client.DatasetInProject(projectID, datasetID)
				_, err := dataset.Metadata(ctx)
				if err != nil {
					s.Client.Close()
					if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == http.StatusNotFound {
						return nil, fmt.Errorf("allowedDataset '%s' not found in project '%s'", datasetID, projectID)
					}
					return nil, fmt.Errorf("failed to verify allowedDataset '%s' in project '%s': %w", datasetID, projectID, err)
				}
			}
			allowedDatasets[allowedFullID] = struct{}{}
		}
	}

	s.AllowedDatasets = allowedDatasets
	s.SessionProvider = s.newBigQuerySessionProvider()
	s.makeDataplexCatalogClient = s.lazyInitDataplexClient(ctx, tracer)
	return s, nil
}

// setupClientCaching initializes caches and wraps the base client creator with caching logic.
func setupClientCaching(s *Source, baseCreator BigqueryClientCreator) {
	// Define eviction handlers
	onBqEvict := func(key string, value interface{}) {
		if client, ok := value.(*bigqueryapi.Client); ok && client != nil {
			client.Close()
		}
	}
	onDataplexEvict := func(key string, value interface{}) {
		if client, ok := value.(*dataplexapi.CatalogClient); ok && client != nil {
			client.Close()
		}
	}

	// Initialize caches
	s.bqClientCache = sources.NewCache(onBqEvict)
	s.bqRestCache = sources.NewCache(nil)
	s.dataplexCache = sources.NewCache(onDataplexEvict)

	// Create the caching wrapper for the client creator
	s.ClientCreator = func(tokenString string, wantRestService bool) (*bigqueryapi.Client, *bigqueryrestapi.Service, error) {
		// Check cache
		bqClientVal, bqFound := s.bqClientCache.Get(tokenString)

		if wantRestService {
			restServiceVal, restFound := s.bqRestCache.Get(tokenString)
			if bqFound && restFound {
				// Cache hit for both
				return bqClientVal.(*bigqueryapi.Client), restServiceVal.(*bigqueryrestapi.Service), nil
			}
		} else {
			if bqFound {
				return bqClientVal.(*bigqueryapi.Client), nil, nil
			}
		}

		// Cache miss - call the client creator
		client, restService, err := baseCreator(tokenString, wantRestService)
		if err != nil {
			return nil, nil, err
		}

		// Set in cache
		s.bqClientCache.Set(tokenString, client)
		if wantRestService && restService != nil {
			s.bqRestCache.Set(tokenString, restService)
		}

		return client, restService, nil
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	Client                    *bigqueryapi.Client
	RestService               *bigqueryrestapi.Service
	TokenSource               oauth2.TokenSource
	AuthTokenHeaderName       string
	ClientCreator             BigqueryClientCreator
	AllowedDatasets           map[string]struct{}
	sessionMutex              sync.Mutex
	makeDataplexCatalogClient func() (*dataplexapi.CatalogClient, DataplexClientCreator, error)
	SessionProvider           BigQuerySessionProvider
	Session                   *Session

	// Caches for OAuth clients
	bqClientCache *sources.Cache
	bqRestCache   *sources.Cache
	dataplexCache *sources.Cache
}

type Session struct {
	ID           string
	ProjectID    string
	DatasetID    string
	CreationTime time.Time
	LastUsed     time.Time
}

func (s *Source) IsReadOnly() bool {
	return s.WriteMode == WriteModeBlocked || s.WriteMode == WriteModeProtected
}

func (s *Source) SourceType() string {
	// Returns BigQuery Google SQL source type
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	cfg := s.Config
	readOnly := s.IsReadOnly()
	cfg.ReadOnly = &readOnly
	return cfg
}

func (s *Source) BigQueryClient() *bigqueryapi.Client {
	return s.Client
}

func (s *Source) BigQueryRestService() *bigqueryrestapi.Service {
	return s.RestService
}

func (s *Source) BigQueryWriteMode() string {
	return s.WriteMode
}

func (s *Source) BigQuerySession() BigQuerySessionProvider {
	return s.SessionProvider
}

func (s *Source) newBigQuerySessionProvider() BigQuerySessionProvider {
	return func(ctx context.Context) (*Session, error) {
		if s.WriteMode != WriteModeProtected {
			return nil, nil
		}

		s.sessionMutex.Lock()
		defer s.sessionMutex.Unlock()

		logger, err := util.LoggerFromContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get logger from context: %w", err)
		}

		if s.Session != nil {
			// Absolute 7-day lifetime check.
			const sessionMaxLifetime = 7 * 24 * time.Hour
			// This assumes a single task will not exceed 30 minutes, preventing it from failing mid-execution.
			const refreshThreshold = 30 * time.Minute
			if time.Since(s.Session.CreationTime) > (sessionMaxLifetime - refreshThreshold) {
				logger.DebugContext(ctx, "Session is approaching its 7-day maximum lifetime. Creating a new one.")
			} else {
				job := &bigqueryrestapi.Job{
					Configuration: &bigqueryrestapi.JobConfiguration{
						DryRun: true,
						Query: &bigqueryrestapi.JobConfigurationQuery{
							Query:                "SELECT 1",
							UseLegacySql:         new(bool),
							ConnectionProperties: []*bigqueryrestapi.ConnectionProperty{{Key: "session_id", Value: s.Session.ID}},
						},
					},
				}
				_, err := s.RestService.Jobs.Insert(s.Project, job).Do()
				if err == nil {
					s.Session.LastUsed = time.Now()
					return s.Session, nil
				}
				logger.DebugContext(ctx, "Session validation failed (likely expired), creating a new one.", "error", err)
			}
		}

		// Create a new session if one doesn't exist, it has passed its 7-day lifetime,
		// or it failed the validation dry run.

		creationTime := time.Now()
		job := &bigqueryrestapi.Job{
			JobReference: &bigqueryrestapi.JobReference{
				ProjectId: s.Project,
				Location:  s.Location,
			},
			Configuration: &bigqueryrestapi.JobConfiguration{
				DryRun: true,
				Query: &bigqueryrestapi.JobConfigurationQuery{
					Query:         "SELECT 1",
					CreateSession: true,
				},
			},
		}

		createdJob, err := s.RestService.Jobs.Insert(s.Project, job).Do()
		if err != nil {
			return nil, fmt.Errorf("failed to create new session: %w", err)
		}

		var sessionID, sessionDatasetID, projectID string
		if createdJob.Status != nil && createdJob.Statistics.SessionInfo != nil {
			sessionID = createdJob.Statistics.SessionInfo.SessionId
		} else {
			return nil, fmt.Errorf("failed to get session ID from new session job")
		}

		if createdJob.Configuration != nil && createdJob.Configuration.Query != nil && createdJob.Configuration.Query.DestinationTable != nil {
			sessionDatasetID = createdJob.Configuration.Query.DestinationTable.DatasetId
			projectID = createdJob.Configuration.Query.DestinationTable.ProjectId
		} else {
			return nil, fmt.Errorf("failed to get session dataset ID from new session job")
		}

		s.Session = &Session{
			ID:           sessionID,
			ProjectID:    projectID,
			DatasetID:    sessionDatasetID,
			CreationTime: creationTime,
			LastUsed:     creationTime,
		}
		return s.Session, nil
	}
}

func (s *Source) UseClientAuthorization() bool {
	return strings.ToLower(s.UseClientOAuth) != "false" && s.UseClientOAuth != ""
}

func (s *Source) GetAuthTokenHeaderName() string {
	return s.AuthTokenHeaderName
}

func (s *Source) BigQueryProject() string {
	return s.Project
}

func (s *Source) BigQueryLocation() string {
	return s.Location
}

func (s *Source) BigQueryQuotaProject() string {
	return s.QuotaProject
}

func (s *Source) BigQueryTokenSource() oauth2.TokenSource {
	return s.TokenSource
}

func (s *Source) BigQueryTokenSourceWithScope(ctx context.Context, scopes []string) (oauth2.TokenSource, error) {
	if len(scopes) == 0 {
		scopes = s.Scopes
		if len(scopes) == 0 {
			scopes = []string{CloudPlatformScope}
		}
	}

	if s.ImpersonateServiceAccount != "" {
		// Create impersonated credentials token source with the requested scopes
		ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: s.ImpersonateServiceAccount,
			Scopes:          scopes,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create impersonated credentials for %q with scopes %v: %w", s.ImpersonateServiceAccount, scopes, err)
		}
		return ts, nil
	}
	return google.DefaultTokenSource(ctx, scopes...)
}

func (s *Source) GetMaxQueryResultRows() int {
	return s.MaxQueryResultRows
}

func (s *Source) GetMaximumBytesBilled() int64 {
	return s.MaximumBytesBilled
}

func (s *Source) BigQueryClientCreator() BigqueryClientCreator {
	return s.ClientCreator
}

func (s *Source) BigQueryAllowedDatasets() []string {
	if len(s.AllowedDatasets) == 0 {
		return nil
	}
	datasets := make([]string, 0, len(s.AllowedDatasets))
	for d := range s.AllowedDatasets {
		datasets = append(datasets, d)
	}
	return datasets
}

// IsDatasetAllowed checks if a given dataset is accessible based on the source's configuration.
func (s *Source) IsDatasetAllowed(projectID, datasetID string) bool {
	// If the normalized map is empty, it means no restrictions were configured.
	if len(s.AllowedDatasets) == 0 {
		return true
	}

	targetDataset := fmt.Sprintf("%s.%s", projectID, datasetID)
	_, ok := s.AllowedDatasets[targetDataset]
	return ok
}

func (s *Source) MakeDataplexCatalogClient() func() (*dataplexapi.CatalogClient, DataplexClientCreator, error) {
	return s.makeDataplexCatalogClient
}

func (s *Source) lazyInitDataplexClient(ctx context.Context, tracer trace.Tracer) func() (*dataplexapi.CatalogClient, DataplexClientCreator, error) {
	var once sync.Once
	var client *dataplexapi.CatalogClient
	var clientCreator DataplexClientCreator
	var err error

	return func() (*dataplexapi.CatalogClient, DataplexClientCreator, error) {
		once.Do(func() {
			c, cc, e := initDataplexConnection(ctx, tracer, s.Name, s.Project, s.UseClientAuthorization(), s.ImpersonateServiceAccount, s.Scopes)
			if e != nil {
				err = fmt.Errorf("failed to initialize dataplex client: %w", e)
				return
			}
			client = c

			// If using OAuth, wrap the provided client creator (cc) with caching logic
			if s.UseClientAuthorization() && cc != nil {
				clientCreator = func(tokenString string) (*dataplexapi.CatalogClient, error) {
					// Check cache
					if val, found := s.dataplexCache.Get(tokenString); found {
						return val.(*dataplexapi.CatalogClient), nil
					}

					// Cache miss - call client creator
					dpClient, err := cc(tokenString)
					if err != nil {
						return nil, err
					}

					// Set in cache
					s.dataplexCache.Set(tokenString, dpClient)
					return dpClient, nil
				}
			} else {
				// Not using OAuth or no creator was returned
				clientCreator = cc
			}
		})
		return client, clientCreator, err
	}
}

func (s *Source) RetrieveClientAndService(accessToken tools.AccessToken) (*bigqueryapi.Client, *bigqueryrestapi.Service, error) {
	bqClient := s.BigQueryClient()
	restService := s.BigQueryRestService()

	// Initialize new client if using user OAuth token
	if s.UseClientAuthorization() {
		tokenStr, err := accessToken.ParseBearerToken()
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing access token: %w", err)
		}
		bqClient, restService, err = s.BigQueryClientCreator()(tokenStr, true)
		if err != nil {
			return nil, nil, fmt.Errorf("error creating client from OAuth access token: %w", err)
		}
	}
	return bqClient, restService, nil
}

func (s *Source) RunSQL(ctx context.Context, bqClient *bigqueryapi.Client, statement, statementType string, params []bigqueryapi.QueryParameter, connProps []*bigqueryapi.ConnectionProperty, labels map[string]string) (any, error) {
	query := bqClient.Query(statement)
	query.Location = bqClient.Location
	if params != nil {
		query.Parameters = params
	}
	if connProps != nil {
		query.ConnectionProperties = connProps
	}
	// BigQuery attaches SQLCommenter attributes as native job labels rather
	// than SQL-text comments, so they surface in INFORMATION_SCHEMA.JOBS and
	// billing exports without query text parsing.
	labels = sqlcommenter.AppendLabels(ctx, labels, SourceType, s.SQLCommenter)
	if labels != nil {
		query.Labels = labels
	}
	if s.MaximumBytesBilled > 0 {
		query.MaxBytesBilled = s.MaximumBytesBilled
	}

	// This block handles SELECT statements, which return a row set.
	// We iterate through the results, convert each row into a map of
	// column names to values, and return the collection of rows.
	job, err := query.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to read query results: %w", err)
	}

	out := []any{}
	for s.MaxQueryResultRows <= 0 || len(out) < s.MaxQueryResultRows {
		var val []bigqueryapi.Value
		err = it.Next(&val)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("unable to iterate through query results: %w", err)
		}
		schema := it.Schema
		row := orderedmap.Row{}
		for i, field := range schema {
			row.Add(field.Name, NormalizeValue(val[i]))
		}
		out = append(out, row)
	}
	// If the query returned any rows, return them directly.
	if len(out) > 0 {
		return out, nil
	}

	// This handles the standard case for a SELECT query that successfully
	// executes but returns zero rows.
	if statementType == "SELECT" {
		return "The query returned 0 rows.", nil
	}
	// This is the fallback for a successful query that doesn't return content.
	// In most cases, this will be for DML/DDL statements like INSERT, UPDATE, CREATE, etc.
	// However, it is also possible that this was a query that was expected to return rows
	// but returned none, a case that we cannot distinguish here.
	return "Query executed successfully and returned no content.", nil
}

// NormalizeValue converts BigQuery specific types to standard JSON-compatible types.
// Specifically, it handles *big.Rat (used for NUMERIC/BIGNUMERIC) by converting
// them to decimal strings with up to 38 digits of precision, trimming trailing zeros.
// It recursively handles slices (arrays) and maps (structs) using reflection.
func NormalizeValue(v any) any {
	if v == nil {
		return nil
	}

	// Handle *big.Rat specifically.
	if rat, ok := v.(*big.Rat); ok {
		// Convert big.Rat to a decimal string.
		// Use a precision of 38 digits (enough for BIGNUMERIC and NUMERIC)
		// and trim trailing zeros to match BigQuery's behavior.
		s := rat.FloatString(38)
		if strings.Contains(s, ".") {
			s = strings.TrimRight(s, "0")
			s = strings.TrimRight(s, ".")
		}
		return s
	}

	// Use reflection for slices and maps to handle various underlying types.
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		// Preserve []byte as is, so json.Marshal encodes it as Base64 string (BigQuery BYTES behavior).
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return v
		}
		newSlice := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			newSlice[i] = NormalizeValue(rv.Index(i).Interface())
		}
		return newSlice
	case reflect.Map:
		// Ensure keys are strings to produce a JSON-compatible map.
		if rv.Type().Key().Kind() != reflect.String {
			return v
		}
		newMap := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			newMap[iter.Key().String()] = NormalizeValue(iter.Value().Interface())
		}
		return newMap
	}

	return v
}

// NormalizeEndpoint canonicalizes a raw endpoint string for use with option.WithEndpoint.
// Empty or whitespace-only input returns "" (no override). If no scheme is present,
// "https://" is assumed. A default port (:443, or :80 for http) is appended when the
// host carries none. Trailing slashes are stripped.
func NormalizeEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	if u.Port() == "" {
		if u.Scheme == "http" {
			u.Host = u.Hostname() + ":80"
		} else {
			u.Host = u.Hostname() + ":443"
		}
	}
	if u.Path == "/" {
		u.Path = ""
	}
	return u.String()
}

func initBigQueryConnection(
	ctx context.Context,
	tracer trace.Tracer,
	name string,
	project string,
	location string,
	quotaProject string,
	impersonateServiceAccount string,
	scopes []string,
	endpoint string,
) (*bigqueryapi.Client, *bigqueryrestapi.Service, oauth2.TokenSource, error) {
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	var tokenSource oauth2.TokenSource
	var opts []option.ClientOption

	var credScopes []string
	if len(scopes) > 0 {
		credScopes = scopes
	} else if impersonateServiceAccount != "" {
		credScopes = []string{CloudPlatformScope}
	} else {
		credScopes = []string{bigqueryapi.Scope}
	}

	if impersonateServiceAccount != "" {
		// Create impersonated credentials token source
		// This broader scope is needed for tools like conversational analytics
		cloudPlatformTokenSource, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
			TargetPrincipal: impersonateServiceAccount,
			Scopes:          credScopes,
		})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to create impersonated credentials for %q: %w", impersonateServiceAccount, err)
		}
		tokenSource = cloudPlatformTokenSource
		opts = []option.ClientOption{
			option.WithUserAgent(userAgent),
			option.WithTokenSource(cloudPlatformTokenSource),
		}
	} else {
		// Use default credentials
		cred, err := google.FindDefaultCredentials(ctx, credScopes...)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to find default Google Cloud credentials with scopes %v: %w", credScopes, err)
		}
		tokenSource = cred.TokenSource
		opts = []option.ClientOption{
			option.WithUserAgent(userAgent),
			option.WithCredentials(cred),
		}
	}

	if endpoint != "" {
		opts = append(opts, option.WithEndpoint(endpoint))
	}
	if quotaProject != "" {
		opts = append(opts, option.WithQuotaProject(quotaProject))
	}

	// Initialize the high-level BigQuery client
	client, err := bigqueryapi.NewClient(ctx, project, opts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create BigQuery client for project %q: %w", project, err)
	}
	client.Location = location

	// Initialize the low-level BigQuery REST service using the same credentials
	restService, err := bigqueryrestapi.NewService(ctx, opts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create BigQuery v2 service: %w", err)
	}

	return client, restService, tokenSource, nil
}

// initBigQueryConnectionWithOAuthToken initialize a BigQuery client with an
// OAuth access token.
func initBigQueryConnectionWithOAuthToken(
	ctx context.Context,
	tracer trace.Tracer,
	project string,
	location string,
	quotaProject string,
	name string,
	userAgent string,
	tokenString string,
	wantRestService bool,
	endpoint string,
) (*bigqueryapi.Client, *bigqueryrestapi.Service, error) {
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()
	// Construct token source
	token := &oauth2.Token{
		AccessToken: string(tokenString),
	}
	ts := oauth2.StaticTokenSource(token)

	opts := []option.ClientOption{
		option.WithUserAgent(userAgent),
		option.WithTokenSource(ts),
	}
	if endpoint != "" {
		opts = append(opts, option.WithEndpoint(endpoint))
	}
	if quotaProject != "" {
		opts = append(opts, option.WithQuotaProject(quotaProject))
	}

	// Initialize the BigQuery client with tokenSource
	client, err := bigqueryapi.NewClient(ctx, project, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create BigQuery client for project %q: %w", project, err)
	}
	client.Location = location

	if wantRestService {
		// Initialize the low-level BigQuery REST service using the same credentials
		restService, err := bigqueryrestapi.NewService(ctx, opts...)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create BigQuery v2 service: %w", err)
		}
		return client, restService, nil
	}

	return client, nil, nil
}

// newBigQueryClientCreator sets the project parameters for the init helper
// function. The returned function takes in an OAuth access token and uses it to
// create a BQ client.
func newBigQueryClientCreator(
	ctx context.Context,
	tracer trace.Tracer,
	project string,
	location string,
	quotaProject string,
	name string,
	endpoint string,
) (func(string, bool) (*bigqueryapi.Client, *bigqueryrestapi.Service, error), error) {
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}

	return func(tokenString string, wantRestService bool) (*bigqueryapi.Client, *bigqueryrestapi.Service, error) {
		return initBigQueryConnectionWithOAuthToken(ctx, tracer, project, location, quotaProject, name, userAgent, tokenString, wantRestService, endpoint)
	}, nil
}

func initDataplexConnection(
	ctx context.Context,
	tracer trace.Tracer,
	name string,
	project string,
	useClientOAuth bool,
	impersonateServiceAccount string,
	scopes []string,
) (*dataplexapi.CatalogClient, DataplexClientCreator, error) {
	var client *dataplexapi.CatalogClient
	var clientCreator DataplexClientCreator
	var err error

	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	if useClientOAuth {
		clientCreator = newDataplexClientCreator(ctx, project, userAgent)
	} else {
		var opts []option.ClientOption

		credScopes := scopes
		if len(credScopes) == 0 {
			credScopes = []string{CloudPlatformScope}
		}

		if impersonateServiceAccount != "" {
			// Create impersonated credentials token source
			ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
				TargetPrincipal: impersonateServiceAccount,
				Scopes:          credScopes,
			})
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create impersonated credentials for %q: %w", impersonateServiceAccount, err)
			}
			opts = []option.ClientOption{
				option.WithUserAgent(userAgent),
				option.WithTokenSource(ts),
			}
		} else {
			// Use default credentials
			cred, err := google.FindDefaultCredentials(ctx, credScopes...)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to find default Google Cloud credentials: %w", err)
			}
			opts = []option.ClientOption{
				option.WithUserAgent(userAgent),
				option.WithCredentials(cred),
			}
		}

		client, err = dataplexapi.NewCatalogClient(ctx, opts...)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create Dataplex client for project %q: %w", project, err)
		}
	}

	return client, clientCreator, nil
}

func initDataplexConnectionWithOAuthToken(
	ctx context.Context,
	project string,
	userAgent string,
	tokenString string,
) (*dataplexapi.CatalogClient, error) {
	// Construct token source
	token := &oauth2.Token{
		AccessToken: string(tokenString),
	}
	ts := oauth2.StaticTokenSource(token)

	client, err := dataplexapi.NewCatalogClient(ctx, option.WithUserAgent(userAgent), option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create Dataplex client for project %q: %w", project, err)
	}
	return client, nil
}

func newDataplexClientCreator(
	ctx context.Context,
	project string,
	userAgent string,
) func(string) (*dataplexapi.CatalogClient, error) {
	return func(tokenString string) (*dataplexapi.CatalogClient, error) {
		return initDataplexConnectionWithOAuthToken(ctx, project, userAgent, tokenString)
	}
}

func (s *Source) InvokeSearchCatalog(ctx context.Context, params map[string]any, tokenStr string) ([]searchcatalog.DataplexSearchResponse, error) {
	typeMap := map[string]string{
		"bigquery-connection":  "CONNECTION",
		"bigquery-data-policy": "POLICY",
		"bigquery-dataset":     "DATASET",
		"bigquery-model":       "MODEL",
		"bigquery-routine":     "ROUTINE",
		"bigquery-table":       "TABLE",
		"bigquery-view":        "VIEW",
	}
	catalogClient, dataplexClientCreator, err := s.MakeDataplexCatalogClient()()
	if err != nil {
		return nil, fmt.Errorf("failed to get dataplex client: %w", err)
	}
	return searchcatalog.InvokeSearchCatalog(
		ctx,
		params,
		tokenStr,
		"bigquery",
		"datasetIds",
		typeMap,
		s.BigQueryProject(),
		func(ctx context.Context, token string) (*dataplexapi.CatalogClient, error) {
			if token != "" {
				return dataplexClientCreator(token)
			}
			return catalogClient, nil
		},
	)
}
