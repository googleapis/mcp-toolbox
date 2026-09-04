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

package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "elasticsearch"

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
	Name      string   `yaml:"name" validate:"required"`
	Type      string   `yaml:"type" validate:"required"`
	Addresses []string `yaml:"addresses" validate:"required"`
	Username  string   `yaml:"username"`
	Password  string   `yaml:"password"`
	APIKey    string   `yaml:"apikey"`
}

func (c Config) SourceConfigType() string {
	return SourceType
}

type EsClient interface {
	esapi.Transport
	elastictransport.Instrumented
}

type Source struct {
	Config
	tracer trace.Tracer
	conn   *sources.ConnectOnce[EsClient]
}

var _ sources.Source = &Source{}

// tracerProviderAdapter adapts a Tracer to implement the TracerProvider interface
type tracerProviderAdapter struct {
	trace.TracerProvider
	tracer trace.Tracer
}

// Tracer implements the TracerProvider interface
func (t *tracerProviderAdapter) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	return t.tracer
}

// Initialize creates a new Elasticsearch Source instance.
func (c Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s := c.newSource(ctx, tracer)
	if deferConnect {
		return s, nil
	}
	if _, err := s.client(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (c Config) newSource(ctx context.Context, tracer trace.Tracer) *Source {
	return &Source{
		Config: c,
		tracer: tracer,
		conn:   sources.NewConnectOnce[EsClient](ctx, c.Name, SourceType, tracer),
	}
}

func (s *Source) client(ctx context.Context) (EsClient, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (EsClient, error) {
		tracerProvider := &tracerProviderAdapter{tracer: s.tracer}

		ua, err := util.UserAgentFromContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("error getting user agent from context: %w", err)
		}

		// Create a new Elasticsearch client with the provided configuration
		cfg := elasticsearch.Config{
			Addresses:       s.Addresses,
			Instrumentation: elasticsearch.NewOpenTelemetryInstrumentation(tracerProvider, false),
			Header:          http.Header{"User-Agent": []string{ua + " go-elasticsearch/" + elasticsearch.Version}},
		}

		// Client need either username and password or an API key
		if s.Username != "" && s.Password != "" {
			cfg.Username = s.Username
			cfg.Password = s.Password
		} else if s.APIKey != "" {
			// API key will be set below
			cfg.APIKey = s.APIKey
		} else {
			// If neither username/password nor API key is provided, we throw an error
			return nil, fmt.Errorf("elasticsearch source %q requires either username/password or an API key", s.Name)
		}

		client, err := elasticsearch.NewBaseClient(cfg)
		if err != nil {
			return nil, err
		}

		// Test connection
		res, err := esapi.InfoRequest{
			Instrument: client.InstrumentationEnabled(),
		}.Do(ctx, client)

		if err != nil {
			return nil, err
		}
		defer res.Body.Close()

		if res.IsError() {
			return nil, fmt.Errorf("elasticsearch connection failed: status %d", res.StatusCode)
		}

		return client, nil
	})
}

// SourceType returns the resourceType string for this source.
func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

// ElasticsearchClient reports the client once connected; callers needing a live one must go through a context-taking method.
func (s *Source) ElasticsearchClient() EsClient {
	client, _ := s.conn.Get()
	return client
}

type EsqlColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type EsqlResult struct {
	Columns []EsqlColumn `json:"columns"`
	Values  [][]any      `json:"values"`
}

func (s *Source) RunSQL(ctx context.Context, format, query string, params []map[string]any) (any, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	bodyStruct := struct {
		Query  string           `json:"query"`
		Params []map[string]any `json:"params,omitempty"`
	}{
		Query:  query,
		Params: params,
	}
	body, err := json.Marshal(bodyStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query body: %w", err)
	}

	res, err := esapi.EsqlQueryRequest{
		Body:       bytes.NewReader(body),
		Format:     format,
		FilterPath: []string{"columns", "values"},
		Instrument: client.InstrumentationEnabled(),
	}.Do(ctx, client)

	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		// Try to extract error message from response
		var esErr json.RawMessage
		err = util.DecodeJSON(res.Body, &esErr)
		if err != nil {
			return nil, fmt.Errorf("elasticsearch error: status %s", res.Status())
		}
		return esErr, nil
	}

	var result EsqlResult
	err = util.DecodeJSON(res.Body, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
	}

	output := EsqlToMap(result)

	return output, nil
}

// EsqlToMap converts the esqlResult to a slice of maps.
func EsqlToMap(result EsqlResult) []map[string]any {
	output := make([]map[string]any, 0, len(result.Values))
	for _, value := range result.Values {
		row := make(map[string]any)
		if value == nil {
			output = append(output, row)
			continue
		}
		for i, col := range result.Columns {
			if i < len(value) {
				row[col.Name] = value[i]
			} else {
				row[col.Name] = nil
			}
		}
		output = append(output, row)
	}
	return output
}
