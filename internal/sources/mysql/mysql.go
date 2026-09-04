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

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/sqlcommenter"
	"github.com/googleapis/mcp-toolbox/internal/tools/mysql/mysqlcommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "mysql"

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
	Name         string            `yaml:"name" validate:"required"`
	Type         string            `yaml:"type" validate:"required"`
	Host         string            `yaml:"host" validate:"required"`
	Port         string            `yaml:"port" validate:"required"`
	User         string            `yaml:"user"`
	Password     string            `yaml:"password"`
	Database     string            `yaml:"database"`
	QueryTimeout string            `yaml:"queryTimeout"`
	QueryParams  map[string]string `yaml:"queryParams"`
	SQLCommenter *bool             `yaml:"sqlCommenter"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s, err := r.newSource(ctx, tracer)
	if err != nil {
		return nil, err
	}
	if deferConnect {
		return s, nil
	}
	if _, err := s.pool(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r Config) newSource(ctx context.Context, tracer trace.Tracer) (*Source, error) {
	queryTimeout, err := r.queryTimeout()
	if err != nil {
		return nil, err
	}
	// The ping honours queryTimeout as the read timeout, so the connect must not
	// be capped tighter than the config allows.
	var opts []sources.Option
	if queryTimeout > 0 {
		opts = append(opts, sources.WithMinConnectTimeout(queryTimeout))
	}
	return &Source{
		Config:       r,
		queryTimeout: queryTimeout,
		conn:         sources.NewConnectOnce[*sql.DB](ctx, r.Name, SourceType, tracer, opts...),
	}, nil
}

// queryTimeout needs no network to validate, so it is resolved at startup.
func (r Config) queryTimeout() (time.Duration, error) {
	if r.QueryTimeout == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(r.QueryTimeout)
	if err != nil {
		return 0, fmt.Errorf("invalid queryTimeout %q: %w", r.QueryTimeout, err)
	}
	return timeout, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	queryTimeout time.Duration
	conn         *sources.ConnectOnce[*sql.DB]
}

func (s *Source) pool(ctx context.Context) (*sql.DB, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*sql.DB, error) {
		pool, err := initMySQLConnectionPool(ctx, s.Host, s.Port, s.User, s.Password, s.Database, s.queryTimeout, s.QueryParams)
		if err != nil {
			return nil, fmt.Errorf("unable to create pool: %w", err)
		}
		if err := pool.PingContext(ctx); err != nil {
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

// MySQLPool reports the pool once connected; use MySQLPoolContext for a guaranteed-live one.
func (s *Source) MySQLPool() *sql.DB {
	pool, _ := s.conn.Get()
	return pool
}

// MySQLPoolContext returns the pool, connecting on first use.
func (s *Source) MySQLPoolContext(ctx context.Context) (*sql.DB, error) {
	return s.pool(ctx)
}

func (s *Source) MySQLDatabase() string {
	return s.Database
}

func (s *Source) PerformanceSchemaEnabled(ctx context.Context) (bool, error) {
	pool, err := s.pool(ctx)
	if err != nil {
		return false, err
	}
	var name, value string
	if err := pool.QueryRowContext(ctx, "SHOW VARIABLES LIKE 'performance_schema'").Scan(&name, &value); err != nil {
		return false, err
	}
	return value == "ON", nil
}

func (s *Source) RetrieveSourceVersion(ctx context.Context) (string, error) {
	pool, err := s.pool(ctx)
	if err != nil {
		return "", err
	}
	var version string
	if err := pool.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return "", err
	}
	return version, nil
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	pool, err := s.pool(ctx)
	if err != nil {
		return nil, err
	}
	statement = sqlcommenter.PrependComment(ctx, statement, SourceType, s.SQLCommenter)
	results, err := pool.QueryContext(ctx, statement, params...)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	defer results.Close()

	cols, err := results.Columns()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve rows column name: %w", err)
	}

	// create an array of values for each column, which can be re-used to scan each row
	rawValues := make([]any, len(cols))
	values := make([]any, len(cols))
	for i := range rawValues {
		values[i] = &rawValues[i]
	}

	colTypes, err := results.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("unable to get column types: %w", err)
	}

	out := []any{}
	for results.Next() {
		err := results.Scan(values...)
		if err != nil {
			return nil, fmt.Errorf("unable to parse row: %w", err)
		}
		row := orderedmap.Row{}
		for i, name := range cols {
			val := rawValues[i]
			if val == nil {
				row.Add(name, nil)
				continue
			}

			convertedValue, err := mysqlcommon.ConvertToType(colTypes[i], val)
			if err != nil {
				return nil, fmt.Errorf("errors encountered when converting values: %w", err)
			}
			row.Add(name, convertedValue)
		}
		out = append(out, row)
	}

	if err := results.Err(); err != nil {
		return nil, fmt.Errorf("errors encountered during row iteration: %w", err)
	}

	return out, nil
}

func initMySQLConnectionPool(ctx context.Context, host, port, user, pass, dbname string, queryTimeout time.Duration, queryParams map[string]string) (*sql.DB, error) {
	config := driver.NewConfig()
	config.Addr = fmt.Sprintf("%s:%s", host, port)
	config.Net = "tcp"
	if user != "" {
		config.User = user
		// password will require user
		if pass != "" {
			config.Passwd = pass
		}
	}
	if dbname != "" {
		config.DBName = dbname
	}
	if queryTimeout != 0 {
		config.ReadTimeout = queryTimeout
	}

	// Custom user parameters
	params := map[string]string{"parseTime": "true"}
	for k, v := range queryParams {
		if v == "" {
			continue // skip empty values
		}
		params[k] = v
	}
	config.Params = params

	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}
	config.ConnectionAttributes = fmt.Sprintf("program_name:%s", userAgent)
	dsn := config.FormatDSN()

	// Interact with the driver directly as you normally would
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return pool, nil
}
