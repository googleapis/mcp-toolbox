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

package postgres

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/sqlcommenter"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "postgres"

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
	Name          string            `yaml:"name" validate:"required"`
	Type          string            `yaml:"type" validate:"required"`
	Host          string            `yaml:"host" validate:"required"`
	Port          string            `yaml:"port" validate:"required"`
	User          string            `yaml:"user" validate:"required"`
	Password      string            `yaml:"password" validate:"required"`
	Database      string            `yaml:"database" validate:"required"`
	QueryParams   map[string]string `yaml:"queryParams"`
	QueryExecMode string            `yaml:"queryExecMode" validate:"omitempty,oneof=cache_statement cache_describe describe_exec exec simple_protocol"`
	SQLCommenter  *bool             `yaml:"sqlCommenter"`
	// ConnectTimeout optionally bounds how long a single connection attempt may
	// take, in seconds. When unset, no timeout is applied and connection behavior
	// is unchanged.
	ConnectTimeout *int `yaml:"connectTimeout" validate:"omitempty,gte=1"`
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
	var opts []sources.Option
	if r.ConnectTimeout != nil {
		opts = append(opts, sources.WithMinConnectTimeout(time.Duration(*r.ConnectTimeout)*time.Second))
	}
	return &Source{
		Config: r,
		conn:   sources.NewConnectOnce[*pgxpool.Pool](ctx, r.Name, SourceType, tracer, opts...),
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[*pgxpool.Pool]
}

func (s *Source) pool(ctx context.Context) (*pgxpool.Pool, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*pgxpool.Pool, error) {
		pool, err := initPostgresConnectionPool(ctx, s.Host, s.Port, s.User, s.Password, s.Database, s.QueryParams, s.QueryExecMode, s.ConnectTimeout)
		if err != nil {
			return nil, fmt.Errorf("unable to create pool: %w", err)
		}
		if err := pool.Ping(ctx); err != nil {
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

// PostgresPool reports the pool once connected; use PostgresPoolContext to guarantee one.
func (s *Source) PostgresPool() *pgxpool.Pool {
	pool, _ := s.conn.Get()
	return pool
}

// PostgresPoolContext returns the pool, connecting on first use.
func (s *Source) PostgresPoolContext(ctx context.Context) (*pgxpool.Pool, error) {
	return s.pool(ctx)
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	pool, err := s.pool(ctx)
	if err != nil {
		return nil, err
	}
	statement = sqlcommenter.PrependComment(ctx, statement, SourceType, s.SQLCommenter)
	results, err := pool.Query(ctx, statement, params...)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	defer results.Close()

	fields := results.FieldDescriptions()
	out := []any{}
	for results.Next() {
		values, err := results.Values()
		if err != nil {
			return nil, fmt.Errorf("unable to parse row: %w", err)
		}
		row := orderedmap.Row{}
		for i, f := range fields {
			val := sources.NormalizeValue(values[i], f.DataTypeOID)
			row.Add(f.Name, val)
		}
		out = append(out, row)
	}
	// this will catch actual query execution errors
	if err := results.Err(); err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	return out, nil
}

func initPostgresConnectionPool(ctx context.Context, host, port, user, pass, dbname string, queryParams map[string]string, queryExecMode string, connectTimeout *int) (*pgxpool.Pool, error) {
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		userAgent = "genai-toolbox"
	}
	if queryParams == nil {
		// Initialize the map before using it
		queryParams = make(map[string]string)
	}
	if _, ok := queryParams["application_name"]; !ok {
		queryParams["application_name"] = userAgent
	}

	config, err := pgxpool.ParseConfig(BuildPostgresURL(host, port, user, pass, dbname, queryParams))
	if err != nil {
		return nil, fmt.Errorf("unable to parse connection uri: %w", err)
	}

	execMode, err := ParseQueryExecMode(queryExecMode)
	if err != nil {
		return nil, err
	}
	config.ConnConfig.DefaultQueryExecMode = execMode

	if connectTimeout != nil {
		config.ConnConfig.ConnectTimeout = time.Duration(*connectTimeout) * time.Second
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	return pool, nil
}

// BuildPostgresURL assembles a postgres connection URL from its components.
// It uses net.JoinHostPort so IPv6 host literals are wrapped in brackets as
// required by RFC 3986 (e.g. "[::1]:5432"); IPv4 addresses and hostnames are
// left unchanged. Query parameters are encoded with url.Values so special
// characters are escaped correctly and the output is deterministic.
func BuildPostgresURL(host, port, user, pass, dbname string, queryParams map[string]string) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   net.JoinHostPort(host, port),
		Path:   dbname,
	}
	if len(queryParams) > 0 {
		q := url.Values{}
		for k, v := range queryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func ParseQueryExecMode(queryExecMode string) (pgx.QueryExecMode, error) {
	switch queryExecMode {
	case "", "cache_statement":
		return pgx.QueryExecModeCacheStatement, nil
	case "cache_describe":
		return pgx.QueryExecModeCacheDescribe, nil
	case "describe_exec":
		return pgx.QueryExecModeDescribeExec, nil
	case "exec":
		return pgx.QueryExecModeExec, nil
	case "simple_protocol":
		return pgx.QueryExecModeSimpleProtocol, nil
	default:
		return 0, fmt.Errorf("invalid queryExecMode %q: must be one of %q, %q, %q, %q, or %q", queryExecMode, "cache_statement", "cache_describe", "describe_exec", "exec", "simple_protocol")
	}
}
