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
	ReadOnly       bool `yaml:"readOnly"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	pool, err := initPostgresConnectionPool(ctx, tracer, r.Name, r.Host, r.Port, r.User, r.Password, r.Database, r.QueryParams, r.QueryExecMode, r.ConnectTimeout)
	if err != nil {
		return nil, fmt.Errorf("unable to create pool: %w", err)
	}

	err = pool.Ping(ctx)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to connect successfully: %w", err)
	}

	if r.ReadOnly {
		if err := VerifyReadOnlyPermissions(ctx, pool, r.Name, r.User); err != nil {
			pool.Close()
			return nil, err
		}
	}

	s := &Source{
		Config: r,
		Pool:   pool,
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	Pool *pgxpool.Pool
}

func (s *Source) IsReadOnly() bool {
	return s.ReadOnly
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) PostgresPool() *pgxpool.Pool {
	return s.Pool
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	statement = sqlcommenter.PrependComment(ctx, statement, SourceType, s.SQLCommenter)
	results, err := s.PostgresPool().Query(ctx, statement, params...)
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

type readOnlyQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const verifyReadOnlyPermissionsSQL = `
SELECT
  COALESCE((SELECT rolsuper FROM pg_roles WHERE rolname = $1), false) AS is_superuser,
  EXISTS (
    SELECT 1
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
      AND n.nspname NOT LIKE 'pg_temp_%'
      AND n.nspname NOT LIKE 'pg_toast_temp_%'
      AND c.relkind IN ('r', 'v', 'm', 'p', 'f')
      AND (
        c.relowner = (SELECT oid FROM pg_roles WHERE rolname = $1) OR
        has_table_privilege($1, c.oid, 'INSERT') OR
        has_table_privilege($1, c.oid, 'UPDATE') OR
        has_table_privilege($1, c.oid, 'DELETE') OR
        has_table_privilege($1, c.oid, 'TRUNCATE')
      )
  ) AS has_table_write,
  EXISTS (
    SELECT 1 FROM pg_namespace
    WHERE nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
      AND nspname NOT LIKE 'pg_temp_%'
      AND nspname NOT LIKE 'pg_toast_temp_%'
      AND has_schema_privilege($1, nspname, 'CREATE')
  ) AS has_schema_create;
`

// VerifyReadOnlyPermissions checks that the connected PostgreSQL role does not possess
// superuser privileges, table mutation privileges (INSERT, UPDATE, DELETE, TRUNCATE),
// or schema CREATE privileges. If any are present, it fails closed with an actionable error.
func VerifyReadOnlyPermissions(ctx context.Context, q readOnlyQuerier, sourceName, user string) error {
	var isSuper, hasTableWrite, hasSchemaCreate bool
	err := q.QueryRow(ctx, verifyReadOnlyPermissionsSQL, user).Scan(&isSuper, &hasTableWrite, &hasSchemaCreate)
	if err != nil {
		return fmt.Errorf("unable to verify read-only permissions for user %q on source %q: %w", user, sourceName, err)
	}
	if isSuper {
		return fmt.Errorf("source %q is configured with readOnly: true, but user %q is a superuser; to secure PostgreSQL in read-only mode, connect with a dedicated non-superuser role with only SELECT privileges", sourceName, user)
	}
	if hasTableWrite {
		return fmt.Errorf("source %q is configured with readOnly: true, but user %q has table write privileges (INSERT, UPDATE, DELETE, or TRUNCATE); connect with a dedicated role with only SELECT privileges", sourceName, user)
	}
	if hasSchemaCreate {
		return fmt.Errorf("source %q is configured with readOnly: true, but user %q has CREATE privilege on one or more schemas; connect with a dedicated role with only SELECT privileges", sourceName, user)
	}
	return nil
}

func initPostgresConnectionPool(ctx context.Context, tracer trace.Tracer, name, host, port, user, pass, dbname string, queryParams map[string]string, queryExecMode string, connectTimeout *int) (*pgxpool.Pool, error) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()
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
