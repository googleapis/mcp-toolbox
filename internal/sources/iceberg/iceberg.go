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

package iceberg

import (
	"context"
	"fmt"

	"github.com/apache/iceberg-go/catalog"
	"github.com/apache/iceberg-go/catalog/rest"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"

	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "iceberg"

// RESTCatalogType is the only catalog flavor the source currently supports.
const RESTCatalogType string = "rest"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name, Catalog: RESTCatalogType} // Default catalog flavor
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type Config struct {
	Name        string `yaml:"name" validate:"required"`
	Type        string `yaml:"type" validate:"required"`
	Catalog     string `yaml:"catalog" validate:"required"`
	Uri         string `yaml:"uri" validate:"required"`
	Warehouse   string `yaml:"warehouse"`
	AccessToken string `yaml:"accessToken"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	if r.Catalog != RESTCatalogType {
		return nil, fmt.Errorf("unsupported catalog type %q: only %q is supported", r.Catalog, RESTCatalogType)
	}

	cat, err := initRESTCatalog(ctx, tracer, r)
	if err != nil {
		return nil, fmt.Errorf("unable to create catalog client: %w", err)
	}

	// Cheap read to verify the endpoint and credentials before serving tools.
	if _, err := cat.ListNamespaces(ctx, nil); err != nil {
		return nil, fmt.Errorf("unable to connect successfully: %w", err)
	}

	s := &Source{
		Config: r,
		Cat:    cat,
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	Cat catalog.Catalog
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) IcebergCatalog() catalog.Catalog {
	return s.Cat
}

func initRESTCatalog(ctx context.Context, tracer trace.Tracer, r Config) (catalog.Catalog, error) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, r.Name)
	defer span.End()

	var opts []rest.Option
	if r.Warehouse != "" {
		opts = append(opts, rest.WithWarehouseLocation(r.Warehouse))
	}
	if r.AccessToken != "" {
		opts = append(opts, rest.WithOAuthToken(r.AccessToken))
	}
	return rest.NewCatalog(ctx, r.Name, r.Uri, opts...)
}
