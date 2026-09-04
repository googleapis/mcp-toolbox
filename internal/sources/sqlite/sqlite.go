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

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/sqlcommenter"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"go.opentelemetry.io/otel/trace"
	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

const SourceType string = "sqlite"

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
	Database     string `yaml:"database" validate:"required"` // Path to SQLite database file
	SQLCommenter *bool  `yaml:"sqlCommenter"`
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
		db, err := initSQLiteConnection(ctx, s.Database)
		if err != nil {
			return nil, fmt.Errorf("unable to create db connection: %w", err)
		}

		// Opening a local file takes no deadline; the ping must not inherit the connect's.
		if err := db.PingContext(context.Background()); err != nil {
			db.Close()
			return nil, fmt.Errorf("unable to connect successfully: %w", err)
		}
		return db, nil
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

// SQLiteDB reports the database handle once opened.
func (s *Source) SQLiteDB() *sql.DB {
	db, _ := s.conn.Get()
	return db
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	db, err := s.pool(ctx)
	if err != nil {
		return nil, err
	}
	// Execute the SQL query with parameters
	statement = sqlcommenter.PrependComment(ctx, statement, SourceType, s.SQLCommenter)
	rows, err := db.QueryContext(ctx, statement, params...)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	defer rows.Close()

	// Get column names
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("unable to get column names: %w", err)
	}

	// The sqlite driver does not support ColumnTypes, so we can't get the
	// underlying database type of the columns. We'll have to rely on the
	// generic `any` type and then handle the JSON data separately.
	rawValues := make([]any, len(cols))
	values := make([]any, len(cols))
	for i := range rawValues {
		values[i] = &rawValues[i]
	}

	// Prepare the result slice
	out := []any{}
	for rows.Next() {
		if err := rows.Scan(values...); err != nil {
			return nil, fmt.Errorf("unable to scan row: %w", err)
		}

		// Create a map for this row
		row := orderedmap.Row{}
		for i, name := range cols {
			val := rawValues[i]
			// Handle nil values
			if val == nil {
				row.Add(name, nil)
				continue
			}
			// Handle JSON data
			if jsonString, ok := val.(string); ok {
				var unmarshaledData any
				if json.Unmarshal([]byte(jsonString), &unmarshaledData) == nil {
					row.Add(name, unmarshaledData)
					continue
				}
			}
			// Store the value in the map
			row.Add(name, val)
		}
		out = append(out, row)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return out, nil
}

func initSQLiteConnection(ctx context.Context, dbPath string) (*sql.DB, error) {
	// Open database connection
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	// Set some reasonable defaults for SQLite
	db.SetMaxOpenConns(1) // SQLite only supports one writer at a time
	db.SetMaxIdleConns(1)

	return db, nil
}
