// Copyright 2026 Google LLC
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

package snowflake

import (
	"context"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/jmoiron/sqlx"
	_ "github.com/snowflakedb/gosnowflake/v2"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "snowflake"

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
	Name      string `yaml:"name" validate:"required"`
	Type      string `yaml:"type" validate:"required"`
	Account   string `yaml:"account" validate:"required"`
	User      string `yaml:"user" validate:"required"`
	Password  string `yaml:"password" validate:"required"`
	Database  string `yaml:"database" validate:"required"`
	Schema    string `yaml:"schema" validate:"required"`
	Warehouse string `yaml:"warehouse"`
	Role      string `yaml:"role"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	db, err := initSnowflakeConnection(ctx, tracer, r.Name, r.Account, r.User, r.Password, r.Database, r.Schema, r.Warehouse, r.Role)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection: %w", err)
	}

	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("unable to connect successfully: %w", err)
	}

	s := &Source{
		Config: r,
		DB:     db,
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	DB *sqlx.DB
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

func (s *Source) SnowflakeDB() *sqlx.DB {
	return s.DB
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	rows, err := s.DB.QueryxContext(ctx, statement, params...)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	defer rows.Close()

	out := []any{}
	for rows.Next() {
		cols, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("unable to get columns: %w", err)
		}

		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("unable to scan row: %w", err)
		}

		vMap := make(map[string]any)
		for i, col := range cols {
			vMap[col] = values[i]
		}
		out = append(out, vMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return out, nil
}

func initSnowflakeConnection(ctx context.Context, tracer trace.Tracer, name, account, user, password, database, schema, warehouse, role string) (*sqlx.DB, error) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	// Set defaults for optional parameters
	if warehouse == "" {
		warehouse = "COMPUTE_WH"
	}
	if role == "" {
		role = "ACCOUNTADMIN"
	}

	// Snowflake DSN format: user:password@account/database/schema?warehouse=warehouse&role=role
	dsn := fmt.Sprintf("%s:%s@%s/%s/%s?warehouse=%s&role=%s", user, password, account, database, schema, warehouse, role)
	db, err := sqlx.ConnectContext(ctx, "snowflake", dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection: %w", err)
	}

	return db, nil
}
