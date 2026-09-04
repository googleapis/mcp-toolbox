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

package tidb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"

	_ "github.com/go-sql-driver/mysql"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "tidb"
const TiDBCloudHostPattern string = `gateway\d{2}\.(.+)\.(prod|dev|staging)\.(.+)\.tidbcloud\.com`

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

	// If the host is a TiDB Cloud instance, force to use SSL
	if IsTiDBCloudHost(actual.Host) {
		actual.UseSSL = true
	}

	return actual, nil
}

type Config struct {
	Name     string `yaml:"name" validate:"required"`
	Type     string `yaml:"type" validate:"required"`
	Host     string `yaml:"host" validate:"required"`
	Port     string `yaml:"port" validate:"required"`
	User     string `yaml:"user" validate:"required"`
	Password string `yaml:"password" validate:"required"`
	Database string `yaml:"database" validate:"required"`
	UseSSL   bool   `yaml:"ssl"`
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
		conn:   sources.NewConnectOnce[*sql.DB](ctx, r.Name, SourceType, tracer),
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[*sql.DB]
}

func (s *Source) pool(ctx context.Context) (*sql.DB, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*sql.DB, error) {
		pool, err := initTiDBConnectionPool(ctx, s.Host, s.Port, s.User, s.Password, s.Database, s.UseSSL)
		if err != nil {
			return nil, fmt.Errorf("unable to create pool: %w", err)
		}

		err = pool.PingContext(ctx)
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

// TiDBPool reports the pool once connected.
func (s *Source) TiDBPool() *sql.DB {
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

			// mysql driver return []uint8 type for "TEXT", "VARCHAR", and "NVARCHAR"
			// we'll need to cast it back to string
			switch colTypes[i].DatabaseTypeName() {
			case "JSON":
				// unmarshal JSON data before storing to prevent double
				// marshaling
				byteVal, ok := val.([]byte)
				if !ok {
					return nil, fmt.Errorf("expected []byte for JSON column, but got %T", val)
				}
				var unmarshaledData any
				if err := json.Unmarshal(byteVal, &unmarshaledData); err != nil {
					return nil, fmt.Errorf("unable to unmarshal json data %s", val)
				}
				vMap[name] = unmarshaledData
			case "TEXT", "VARCHAR", "NVARCHAR":
				byteVal, ok := val.([]byte)
				if !ok {
					return nil, fmt.Errorf("expected []byte for text-like column, but got %T", val)
				}
				vMap[name] = string(byteVal)
			default:
				vMap[name] = val
			}
		}
		out = append(out, vMap)
	}

	if err := results.Err(); err != nil {
		return nil, fmt.Errorf("errors encountered during row iteration: %w", err)
	}

	return out, nil
}

func IsTiDBCloudHost(host string) bool {
	pattern := `gateway\d{2}\.(.+)\.(prod|dev|staging)\.(.+)\.tidbcloud\.com`
	match, err := regexp.MatchString(pattern, host)
	if err != nil {
		return false
	}
	return match
}

func initTiDBConnectionPool(ctx context.Context, host, port, user, pass, dbname string, useSSL bool) (*sql.DB, error) {
	// Configure the driver to connect to the database
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&tls=%t", user, pass, host, port, dbname, useSSL)

	// Interact with the driver directly as you normally would
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	return pool, nil
}
