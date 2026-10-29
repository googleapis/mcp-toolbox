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
	// MaxOpenConns, MaxIdleConns and ConnMaxLifetime optionally override the
	// connection pool defaults. When unset, the defaults below are used.
	MaxOpenConns    *int   `yaml:"maxOpenConns" validate:"omitempty,gte=1"`
	MaxIdleConns    *int   `yaml:"maxIdleConns" validate:"omitempty,gte=0"`
	ConnMaxLifetime string `yaml:"connMaxLifetime"`
}

const (
	defaultMaxOpenConns    = 25
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 5 * time.Minute
)

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	poolCfg, err := r.poolSettings()
	if err != nil {
		return nil, err
	}

	pool, err := initClickHouseConnectionPool(ctx, tracer, r.Name, r.Host, r.Port, r.User, r.Password, r.Database, r.Protocol, r.Secure, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("unable to create pool: %w", err)
	}

	err = pool.PingContext(ctx)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to connect successfully: %w", err)
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
	Pool *sql.DB
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

func (s *Source) ClickHousePool() *sql.DB {
	return s.Pool
}

func (s *Source) RunSQL(ctx context.Context, statement string, params parameters.ParamValues) (any, error) {
	var sliceParams []any
	if params != nil {
		sliceParams = params.AsSlice()
	}
	results, err := s.ClickHousePool().QueryContext(ctx, statement, sliceParams...)
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

// poolSettings holds the resolved connection pool limits for a source.
type poolSettings struct {
	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
}

// poolSettings resolves the pool limits from the config, falling back to the
// defaults for any field that is not set.
func (r Config) poolSettings() (poolSettings, error) {
	p := poolSettings{
		maxOpenConns:    defaultMaxOpenConns,
		maxIdleConns:    defaultMaxIdleConns,
		connMaxLifetime: defaultConnMaxLifetime,
	}
	if r.MaxOpenConns != nil {
		p.maxOpenConns = *r.MaxOpenConns
	}
	if r.MaxIdleConns != nil {
		p.maxIdleConns = *r.MaxIdleConns
		if p.maxIdleConns > p.maxOpenConns {
			return poolSettings{}, fmt.Errorf("maxIdleConns (%d) must not be greater than maxOpenConns (%d)", p.maxIdleConns, p.maxOpenConns)
		}
	} else if p.maxIdleConns > p.maxOpenConns {
		// keep the default idle count within a lower maxOpenConns
		p.maxIdleConns = p.maxOpenConns
	}
	if r.ConnMaxLifetime != "" {
		d, err := time.ParseDuration(r.ConnMaxLifetime)
		if err != nil {
			return poolSettings{}, fmt.Errorf("invalid connMaxLifetime %q: %w", r.ConnMaxLifetime, err)
		}
		if d <= 0 {
			return poolSettings{}, fmt.Errorf("invalid connMaxLifetime %q: must be positive", r.ConnMaxLifetime)
		}
		p.connMaxLifetime = d
	}
	return p, nil
}

func validateConfig(protocol string) error {
	validProtocols := map[string]bool{"http": true, "https": true}

	if protocol != "" && !validProtocols[protocol] {
		return fmt.Errorf("invalid protocol: %s, must be one of: http, https", protocol)
	}
	return nil
}

func initClickHouseConnectionPool(ctx context.Context, tracer trace.Tracer, name, host, port, user, pass, dbname, protocol string, secure bool, poolCfg poolSettings) (*sql.DB, error) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

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

	pool.SetMaxOpenConns(poolCfg.maxOpenConns)
	pool.SetMaxIdleConns(poolCfg.maxIdleConns)
	pool.SetConnMaxLifetime(poolCfg.connMaxLifetime)

	return pool, nil
}
