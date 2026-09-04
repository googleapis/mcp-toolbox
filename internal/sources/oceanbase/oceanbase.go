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

package oceanbase

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools/mysql/mysqlcommon"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "oceanbase"

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
	Name         string `yaml:"name" validate:"required"`
	Type         string `yaml:"type" validate:"required"`
	Host         string `yaml:"host" validate:"required"`
	Port         string `yaml:"port" validate:"required"`
	User         string `yaml:"user" validate:"required"`
	Password     string `yaml:"password" validate:"required"`
	Database     string `yaml:"database" validate:"required"`
	QueryTimeout string `yaml:"queryTimeout"`
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

// queryTimeout parses the configured timeout; a malformed value fails at startup, not at connect.
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
		pool, err := initOceanBaseConnectionPool(ctx, s.Host, s.Port, s.User, s.Password, s.Database, s.queryTimeout)
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

// OceanBasePool reports the pool once connected.
func (s *Source) OceanBasePool() *sql.DB {
	pool, _ := s.conn.Get()
	return pool
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	pool, err := s.pool(ctx)
	if err != nil {
		return nil, err
	}
	results, err := pool.QueryContext(ctx, statement, params...)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}

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
	defer results.Close()

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
		vMap := make(map[string]any)
		for i, name := range cols {
			val := rawValues[i]
			if val == nil {
				vMap[name] = nil
				continue
			}

			// oceanbase uses mysql driver
			vMap[name], err = mysqlcommon.ConvertToType(colTypes[i], val)
			if err != nil {
				return nil, fmt.Errorf("errors encountered when converting values: %w", err)
			}
		}
		out = append(out, vMap)
	}

	if err := results.Err(); err != nil {
		return nil, fmt.Errorf("errors encountered during row iteration: %w", err)
	}

	return out, nil
}

func initOceanBaseConnectionPool(ctx context.Context, host, port, user, pass, dbname string, queryTimeout time.Duration) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", user, pass, host, port, dbname)

	if queryTimeout != 0 {
		dsn += "&readTimeout=" + queryTimeout.String()
	}

	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return pool, nil
}
