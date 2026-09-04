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

package clickhouse

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "clickhouse"

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
	Host     string `yaml:"host" validate:"required"`
	Port     string `yaml:"port" validate:"required"`
	Database string `yaml:"database" validate:"required"`
	User     string `yaml:"user" validate:"required"`
	Password string `yaml:"password"`
	Protocol string `yaml:"protocol"`
	Secure   bool   `yaml:"secure"`
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
	protocol, err := r.protocol()
	if err != nil {
		return nil, err
	}
	return &Source{
		Config:   r,
		protocol: protocol,
		conn:     sources.NewConnectOnce[*sql.DB](ctx, r.Name, SourceType, tracer),
	}, nil
}

// protocol resolves the configured protocol; an unsupported value fails at startup, not at connect.
func (r Config) protocol() (string, error) {
	if r.Protocol == "" {
		return "https", nil
	}
	if err := validateConfig(r.Protocol); err != nil {
		return "", err
	}
	return r.Protocol, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	protocol string
	conn     *sources.ConnectOnce[*sql.DB]
}

func (s *Source) pool(ctx context.Context) (*sql.DB, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*sql.DB, error) {
		pool, err := initClickHouseConnectionPool(ctx, s.Host, s.Port, s.User, s.Password, s.Database, s.protocol, s.Secure)
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

// ClickHousePool reports the pool once connected.
func (s *Source) ClickHousePool() *sql.DB {
	pool, _ := s.conn.Get()
	return pool
}

func (s *Source) RunSQL(ctx context.Context, statement string, params parameters.ParamValues) (any, error) {
	pool, err := s.pool(ctx)
	if err != nil {
		return nil, err
	}
	var sliceParams []any
	if params != nil {
		sliceParams = params.AsSlice()
	}
	results, err := pool.QueryContext(ctx, statement, sliceParams...)
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
		vMap := make(map[string]any)
		for i, name := range cols {
			// ClickHouse driver may return specific types that need handling
			switch colTypes[i].DatabaseTypeName() {
			case "String", "FixedString":
				if rawValues[i] != nil {
					// Handle potential []byte to string conversion if needed
					if b, ok := rawValues[i].([]byte); ok {
						vMap[name] = string(b)
					} else {
						vMap[name] = rawValues[i]
					}
				} else {
					vMap[name] = nil
				}
			default:
				vMap[name] = rawValues[i]
			}
		}
		out = append(out, vMap)
	}

	if err := results.Err(); err != nil {
		return nil, fmt.Errorf("errors encountered by results.Scan: %w", err)
	}

	return out, nil
}

func validateConfig(protocol string) error {
	validProtocols := map[string]bool{"http": true, "https": true}

	if protocol != "" && !validProtocols[protocol] {
		return fmt.Errorf("invalid protocol: %s, must be one of: http, https", protocol)
	}
	return nil
}

func initClickHouseConnectionPool(ctx context.Context, host, port, user, pass, dbname, protocol string, secure bool) (*sql.DB, error) {
	if protocol == "" {
		protocol = "https"
	}

	if err := validateConfig(protocol); err != nil {
		return nil, err
	}

	encodedUser := url.QueryEscape(user)
	encodedPass := url.QueryEscape(pass)

	var dsn string
	scheme := protocol
	if protocol == "http" && secure {
		scheme = "https"
	}
	dsn = fmt.Sprintf("%s://%s:%s@%s:%s/%s", scheme, encodedUser, encodedPass, host, port, dbname)
	if scheme == "https" {
		dsn += "?secure=true&skip_verify=false"
	}

	pool, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	pool.SetMaxOpenConns(25)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(5 * time.Minute)

	return pool, nil
}
