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

package bigtable

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigtable"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/option"
)

const SourceType string = "bigtable"

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
	Name     string `yaml:"name" validate:"required"`
	Type     string `yaml:"type" validate:"required"`
	Project  string `yaml:"project" validate:"required"`
	Instance string `yaml:"instance" validate:"required"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

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
		Config: r,
		conn:   sources.NewConnectOnce[*clientSet](ctx, r.Name, SourceType, tracer),
	}
}

var _ sources.Source = &Source{}

type clientSet struct {
	client        *bigtable.Client
	instanceAdmin *bigtable.InstanceAdminClient
	admin         *bigtable.AdminClient
}

type Source struct {
	Config
	conn *sources.ConnectOnce[*clientSet]
}

func (s *Source) clients(ctx context.Context) (*clientSet, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*clientSet, error) {
		client, err := initBigtableClient(ctx, s.Project, s.Instance)
		if err != nil {
			return nil, fmt.Errorf("unable to create client: %w", err)
		}

		instanceAdminClient, err := initBigtableInstanceAdminClient(ctx, s.Project)
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("unable to create instance admin client: %w", err)
		}

		adminClient, err := initBigtableAdminClient(ctx, s.Project, s.Instance)
		if err != nil {
			client.Close()
			instanceAdminClient.Close()
			return nil, fmt.Errorf("unable to create admin client: %w", err)
		}

		return &clientSet{
			client:        client,
			instanceAdmin: instanceAdminClient,
			admin:         adminClient,
		}, nil
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

// BigtableClient reports the data client once connected.
func (s *Source) BigtableClient() *bigtable.Client {
	cs, ok := s.conn.Get()
	if !ok {
		return nil
	}
	return cs.client
}

// BigtableInstanceAdminClient reports the instance admin client once connected.
func (s *Source) BigtableInstanceAdminClient() *bigtable.InstanceAdminClient {
	cs, ok := s.conn.Get()
	if !ok {
		return nil
	}
	return cs.instanceAdmin
}

// BigtableAdminClient reports the admin client once connected.
func (s *Source) BigtableAdminClient() *bigtable.AdminClient {
	cs, ok := s.conn.Get()
	if !ok {
		return nil
	}
	return cs.admin
}

func (s *Source) ProjectID() string {
	return s.Project
}

func (s *Source) InstanceID() string {
	return s.Instance
}

func getBigtableType(paramType string) (bigtable.SQLType, error) {
	switch paramType {
	case "boolean":
		return bigtable.BoolSQLType{}, nil
	case "string":
		return bigtable.StringSQLType{}, nil
	case "integer":
		return bigtable.Int64SQLType{}, nil
	case "float":
		return bigtable.Float64SQLType{}, nil
	case "array":
		return bigtable.ArraySQLType{}, nil
	default:
		return nil, fmt.Errorf("unknow param type %s", paramType)
	}
}

func getMapParamsType(tparams parameters.Parameters) (map[string]bigtable.SQLType, error) {
	btParamTypes := make(map[string]bigtable.SQLType)
	for _, p := range tparams {
		if p.GetType() == "array" {
			itemType, err := getBigtableType(p.Manifest().Items.Type)
			if err != nil {
				return nil, err
			}
			btParamTypes[p.GetName()] = bigtable.ArraySQLType{
				ElemType: itemType,
			}
			continue
		}
		paramType, err := getBigtableType(p.GetType())
		if err != nil {
			return nil, err
		}
		btParamTypes[p.GetName()] = paramType
	}
	return btParamTypes, nil
}

func (s *Source) RunSQL(ctx context.Context, statement string, configParam parameters.Parameters, params parameters.ParamValues) (any, error) {
	cs, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}

	mapParamsType, err := getMapParamsType(configParam)
	if err != nil {
		return nil, fmt.Errorf("fail to get map params: %w", err)
	}

	ps, err := cs.client.PrepareStatement(
		ctx,
		statement,
		mapParamsType,
	)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare statement: %w", err)
	}

	bs, err := ps.Bind(params.AsMap())
	if err != nil {
		return nil, fmt.Errorf("unable to bind: %w", err)
	}

	out := []any{}
	var rowErr error
	err = bs.Execute(ctx, func(resultRow bigtable.ResultRow) bool {
		vMap := make(map[string]any)
		cols := resultRow.Metadata.Columns

		for _, c := range cols {
			var columValue any
			if err = resultRow.GetByName(c.Name, &columValue); err != nil {
				rowErr = err
				return false
			}
			vMap[c.Name] = columValue
		}

		out = append(out, vMap)

		return true
	})
	if err != nil {
		return nil, fmt.Errorf("unable to execute client: %w", err)
	}
	if rowErr != nil {
		return nil, fmt.Errorf("error processing row: %w", rowErr)
	}

	return out, nil
}

func initBigtableClient(ctx context.Context, project, instance string) (*bigtable.Client, error) {
	// Set up Bigtable data operations client.
	poolSize := 10
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}

	client, err := bigtable.NewClient(ctx, project, instance, option.WithUserAgent(userAgent), option.WithGRPCConnectionPool(poolSize))

	if err != nil {
		return nil, fmt.Errorf("unable to create bigtable.NewClient: %w", err)
	}

	return client, nil
}

func initBigtableInstanceAdminClient(ctx context.Context, project string) (*bigtable.InstanceAdminClient, error) {
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}

	client, err := bigtable.NewInstanceAdminClient(ctx, project, option.WithUserAgent(userAgent))
	if err != nil {
		return nil, fmt.Errorf("unable to create bigtable.NewInstanceAdminClient: %w", err)
	}

	return client, nil
}

func initBigtableAdminClient(ctx context.Context, project, instance string) (*bigtable.AdminClient, error) {
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}

	client, err := bigtable.NewAdminClient(ctx, project, instance, option.WithUserAgent(userAgent))
	if err != nil {
		return nil, fmt.Errorf("unable to create bigtable.NewAdminClient: %w", err)
	}

	return client, nil
}
