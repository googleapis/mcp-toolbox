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

package yugabytedb

import (
	"context"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/yugabyte/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "yugabytedb"

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
	Name                            string `yaml:"name" validate:"required"`
	Type                            string `yaml:"type" validate:"required"`
	Host                            string `yaml:"host" validate:"required"`
	Port                            string `yaml:"port" validate:"required"`
	User                            string `yaml:"user" validate:"required"`
	Password                        string `yaml:"password" validate:"required"`
	Database                        string `yaml:"database" validate:"required"`
	LoadBalance                     string `yaml:"loadBalance"`
	TopologyKeys                    string `yaml:"topologyKeys"`
	YBServersRefreshInterval        string `yaml:"ybServersRefreshInterval"`
	FallBackToTopologyKeysOnly      string `yaml:"fallbackToTopologyKeysOnly"`
	FailedHostReconnectDelaySeconds string `yaml:"failedHostReconnectDelaySecs"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s := r.newSource(ctx, tracer)
	if deferConnect {
		return s, nil
	}
	if _, err := s.pool(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r Config) newSource(ctx context.Context, tracer trace.Tracer) *Source {
	return &Source{
		Config: r,
		conn:   sources.NewConnectOnce[*pgxpool.Pool](ctx, r.Name, SourceType, tracer),
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[*pgxpool.Pool]
}

func (s *Source) pool(ctx context.Context) (*pgxpool.Pool, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*pgxpool.Pool, error) {
		pool, err := initYugabyteDBConnectionPool(ctx, s.Host, s.Port, s.User, s.Password, s.Database, s.LoadBalance, s.TopologyKeys, s.YBServersRefreshInterval, s.FallBackToTopologyKeysOnly, s.FailedHostReconnectDelaySeconds)
		if err != nil {
			return nil, fmt.Errorf("unable to create pool: %w", err)
		}

		err = pool.Ping(ctx)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("unable to connect successfully: %w", err)
		}
		return pool, nil
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

// YugabyteDBPool reports the pool once connected.
func (s *Source) YugabyteDBPool() *pgxpool.Pool {
	pool, _ := s.conn.Get()
	return pool
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	pool, err := s.pool(ctx)
	if err != nil {
		return nil, err
	}
	results, err := pool.Query(ctx, statement, params...)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}

	fields := results.FieldDescriptions()

	out := []any{}
	for results.Next() {
		v, err := results.Values()
		if err != nil {
			return nil, fmt.Errorf("unable to parse row: %w", err)
		}
		vMap := make(map[string]any)
		for i, f := range fields {
			val := sources.NormalizeValue(v[i], f.DataTypeOID)
			vMap[f.Name] = val
		}
		out = append(out, vMap)
	}

	// this will catch actual query execution errors
	if err := results.Err(); err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}

	return out, nil
}

func initYugabyteDBConnectionPool(ctx context.Context, host, port, user, pass, dbname, loadBalance, topologyKeys, refreshInterval, explicitFallback, failedHostTTL string) (*pgxpool.Pool, error) {
	// urlExample := "postgres://username:password@localhost:5433/database_name"
	i := fmt.Sprintf("postgres://%s:%s@%s:%s/%s", user, pass, host, port, dbname)
	if loadBalance == "true" {
		i = fmt.Sprintf("%s?load_balance=%s", i, loadBalance)
		if topologyKeys != "" {
			i = fmt.Sprintf("%s&topology_keys=%s", i, topologyKeys)
			if explicitFallback == "true" {
				i = fmt.Sprintf("%s&fallback_to_topology_keys_only=%s", i, explicitFallback)
			}
		}
		if refreshInterval != "" {
			i = fmt.Sprintf("%s&yb_servers_refresh_interval=%s", i, refreshInterval)
		}
		if failedHostTTL != "" {
			i = fmt.Sprintf("%s&failed_host_reconnect_delay_secs=%s", i, failedHostTTL)
		}
	}
	pool, err := pgxpool.New(ctx, i)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	return pool, nil
}
