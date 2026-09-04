// Copyright 2024 Google LLC
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

package spanner

import (
	"context"
	"encoding/json"
	"fmt"

	dataplexapi "cloud.google.com/go/dataplex/apiv1"
	"cloud.google.com/go/spanner"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/dataplex/searchcatalog"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/iterator"
)

const SourceType string = "spanner"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name, Dialect: "googlesql"} // Default dialect
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type Config struct {
	Name           string          `yaml:"name" validate:"required"`
	Type           string          `yaml:"type" validate:"required"`
	Project        string          `yaml:"project" validate:"required"`
	Instance       string          `yaml:"instance" validate:"required"`
	Dialect        sources.Dialect `yaml:"dialect" validate:"required"`
	Database       string          `yaml:"database" validate:"required"`
	UseClientOAuth bool            `yaml:"useClientOAuth"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s := r.newSource(ctx, tracer)
	if deferConnect {
		return s, nil
	}
	if _, err := s.client(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r Config) newSource(ctx context.Context, tracer trace.Tracer) *Source {
	onDataplexEvict := func(key string, value interface{}) {
		if client, ok := value.(*dataplexapi.CatalogClient); ok && client != nil {
			client.Close()
		}
	}

	return &Source{
		Config: r,
		tracer: tracer,
		conn:   sources.NewConnectOnce[*spanner.Client](ctx, r.Name, SourceType, tracer),
		// Builds nothing until a catalog call arrives, so it is free to hold here.
		dataplexMgr: &searchcatalog.DataplexClientManager{
			UseClientOAuth: r.UseClientOAuth,
			Cache:          sources.NewCache(onDataplexEvict),
		},
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	tracer      trace.Tracer
	conn        *sources.ConnectOnce[*spanner.Client]
	dataplexMgr *searchcatalog.DataplexClientManager
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) client(ctx context.Context) (*spanner.Client, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*spanner.Client, error) {
		client, err := initSpannerClient(ctx, s.Project, s.Instance, s.Database)
		if err != nil {
			return nil, fmt.Errorf("unable to create client: %w", err)
		}
		return client, nil
	})
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

// SpannerClient reports the client once connected.
func (s *Source) SpannerClient() *spanner.Client {
	client, _ := s.conn.Get()
	return client
}

func (s *Source) DatabaseDialect() string {
	return s.Dialect.String()
}

func (s *Source) ProjectID() string {
	return s.Project
}

func (s *Source) UseClientAuthorization() bool {
	return s.UseClientOAuth
}

func (s *Source) GetCatalogClient(ctx context.Context, tokenString string) (*dataplexapi.CatalogClient, error) {
	return s.dataplexMgr.GetCatalogClient(ctx, tokenString)
}

// processRows iterates over the spanner.RowIterator and converts each row to a map[string]any.
func processRows(iter *spanner.RowIterator) ([]any, error) {
	out := []any{}
	defer iter.Stop()

	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("unable to parse row: %w", err)
		}

		rowMap := orderedmap.Row{}
		cols := row.ColumnNames()
		for i, c := range cols {
			if c == "object_details" { // for list graphs or list tables
				val := row.ColumnValue(i)
				if val == nil { // ColumnValue returns the Cloud Spanner Value of column i, or nil for invalid column.
					rowMap.Add(c, nil)
				} else {
					jsonString, ok := val.AsInterface().(string)
					if !ok {
						return nil, fmt.Errorf("column 'object_details' is not a string, but %T", val.AsInterface())
					}
					var details map[string]any
					if err := json.Unmarshal([]byte(jsonString), &details); err != nil {
						return nil, fmt.Errorf("unable to unmarshal JSON: %w", err)
					}
					rowMap.Add(c, details)
				}
			} else {
				rowMap.Add(c, row.ColumnValue(i))
			}
		}
		out = append(out, rowMap)
	}
	return out, nil
}

func (s *Source) RunSQL(ctx context.Context, readOnly bool, statement string, params map[string]any) (any, error) {
	var results []any
	var err error
	var opErr error
	stmt := spanner.Statement{
		SQL: statement,
	}
	if params != nil {
		stmt.Params = params
	}

	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}

	if readOnly {
		iter := client.Single().Query(ctx, stmt)
		results, opErr = processRows(iter)
	} else {
		_, opErr = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			iter := txn.Query(ctx, stmt)
			results, err = processRows(iter)
			if err != nil {
				return err
			}
			return nil
		})
	}

	if opErr != nil {
		return nil, fmt.Errorf("unable to execute client: %w", opErr)
	}

	return results, nil
}

func initSpannerClient(ctx context.Context, project, instance, dbname string) (*spanner.Client, error) {
	// Configure the connection to the database
	db := fmt.Sprintf("projects/%s/instances/%s/databases/%s", project, instance, dbname)

	// Create spanner client
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}
	client, err := spanner.NewClientWithConfig(ctx, db, spanner.ClientConfig{UserAgent: userAgent})
	if err != nil {
		return nil, fmt.Errorf("unable to create new client: %w", err)
	}

	return client, nil
}

func (s *Source) InvokeSearchCatalog(ctx context.Context, params map[string]any, tokenStr string) ([]searchcatalog.DataplexSearchResponse, error) {
	typeMap := map[string]string{
		"cloud-spanner-instance": "SERVICE",
		"cloud-spanner-database": "DATABASE",
		"cloud-spanner-table":    "TABLE",
		"cloud-spanner-view":     "VIEW",
	}
	return searchcatalog.InvokeSearchCatalog(
		ctx,
		params,
		tokenStr,
		"CLOUD_SPANNER",
		"databaseIds",
		typeMap,
		s.ProjectID(),
		func(ctx context.Context, token string) (*dataplexapi.CatalogClient, error) {
			return s.GetCatalogClient(ctx, token)
		},
	)
}
