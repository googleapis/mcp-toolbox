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

package cloudsqlmssql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"slices"

	"cloud.google.com/go/cloudsqlconn/sqlserver/mssql"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "cloud-sql-mssql"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name, IPType: "public"} // Default IPType
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type Config struct {
	// Cloud SQL MSSQL configs
	Name     string         `yaml:"name" validate:"required"`
	Type     string         `yaml:"type" validate:"required"`
	Project  string         `yaml:"project" validate:"required"`
	Region   string         `yaml:"region" validate:"required"`
	Instance string         `yaml:"instance" validate:"required"`
	IPType   sources.IPType `yaml:"ipType" validate:"required"`
	User     string         `yaml:"user" validate:"required"`
	Password string         `yaml:"password" validate:"required"`
	Database string         `yaml:"database" validate:"required"`
}

func (r Config) SourceConfigType() string {
	// Returns Cloud SQL MSSQL source type
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	// Initializes a Cloud SQL MSSQL source
	db, err := initCloudSQLMssqlConnection(ctx, tracer, r.Name, r.Project, r.Region, r.Instance, r.IPType.String(), r.User, r.Password, r.Database)
	if err != nil {
		return nil, fmt.Errorf("unable to create db connection: %w", err)
	}

	// Verify db connection
	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("unable to connect successfully: %w", err)
	}

	s := &Source{
		Config: r,
		Db:     db,
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	Db *sql.DB
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	// Returns Cloud SQL MSSQL source type
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) MSSQLDB() *sql.DB {
	// Returns a Cloud SQL MSSQL database connection pool
	return s.Db
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	results, err := s.MSSQLDB().QueryContext(ctx, statement, params...)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	defer results.Close()

	cols, err := results.Columns()
	// If Columns() errors, it might be a DDL/DML without an OUTPUT clause.
	// We proceed, and results.Err() will catch actual query execution errors.
	// 'out' will remain an empty slice if cols is empty or err is not nil here.
	out := []any{}
	if err == nil && len(cols) > 0 {
		// create an array of values for each column, which can be re-used to scan each row
		rawValues := make([]any, len(cols))
		values := make([]any, len(cols))
		for i := range rawValues {
			values[i] = &rawValues[i]
		}

		for results.Next() {
			scanErr := results.Scan(values...)
			if scanErr != nil {
				return nil, fmt.Errorf("unable to parse row: %w", scanErr)
			}
			row := orderedmap.Row{}
			for i, name := range cols {
				row.Add(name, rawValues[i])
			}
			out = append(out, row)
		}
	}

	// Check for errors from iterating over rows or from the query execution itself.
	// results.Close() is handled by defer.
	if err := results.Err(); err != nil {
		return nil, fmt.Errorf("errors encountered during query execution or row processing: %w", err)
	}

	return out, nil
}

func initCloudSQLMssqlConnection(ctx context.Context, tracer trace.Tracer, name, project, region, instance, ipType, user, pass, dbname string) (*sql.DB, error) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Create dsn
	query := url.Values{}
	query.Add("app name", userAgent)
	query.Add("database", dbname)
	query.Add("cloudsql", fmt.Sprintf("%s:%s:%s", project, region, instance))

	url := &url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(user, pass),
		RawQuery: query.Encode(),
	}

	// Get dial options
	opts, err := sources.GetCloudSQLOpts(ipType, userAgent, false)
	if err != nil {
		return nil, err
	}

	// Register sql server driver
	if !slices.Contains(sql.Drivers(), "cloudsql-sqlserver-driver") {
		_, err := mssql.RegisterDriver("cloudsql-sqlserver-driver", opts...)
		if err != nil {
			return nil, err
		}
	}

	// Open database connection
	db, err := sql.Open(
		"cloudsql-sqlserver-driver",
		url.String(),
	)
	if err != nil {
		return nil, err
	}
	return db, nil
}
