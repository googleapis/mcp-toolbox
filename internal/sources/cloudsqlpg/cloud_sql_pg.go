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

package cloudsqlpg

import (
	"context"
	"fmt"
	"net"

	"cloud.google.com/go/cloudsqlconn"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/sqlcommenter"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "cloud-sql-postgres"

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
	Name         string         `yaml:"name" validate:"required"`
	Type         string         `yaml:"type" validate:"required"`
	Project      string         `yaml:"project" validate:"required"`
	Region       string         `yaml:"region" validate:"required"`
	Instance     string         `yaml:"instance" validate:"required"`
	IPType       sources.IPType `yaml:"ipType" validate:"required"`
	Database     string         `yaml:"database" validate:"required"`
	User         string         `yaml:"user"`
	Password     string         `yaml:"password"`
	ReadOnly     bool           `yaml:"readOnly"`
	SQLCommenter *bool          `yaml:"sqlCommenter"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	pool, err := initCloudSQLPgConnectionPool(ctx, tracer, r.Name, r.Project, r.Region, r.Instance, r.IPType.String(), r.User, r.Password, r.Database, r.ReadOnly)
	if err != nil {
		return nil, fmt.Errorf("unable to create pool: %w", err)
	}

	err = pool.Ping(ctx)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to connect successfully: %w", err)
	}

	var res int
	err = pool.QueryRow(ctx, "SELECT 1").Scan(&res)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to execute 'SELECT 1' after connection: %w", err)
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

func getConnectionConfig(ctx context.Context, user, pass, dbname string, readOnly bool) (string, bool, error) {
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		userAgent = "genai-toolbox"
	}
	useIAM := true

	var dsn string
	// If username and password both provided, use password authentication
	if user != "" && pass != "" {
		dsn = fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable application_name=%s", user, pass, dbname, userAgent)
		useIAM = false
	} else if user == "" {
		// If username is empty, fetch email from ADC
		// otherwise, use username as IAM email
		if pass != "" {
			// If password is provided without an username, raise an error
			return "", useIAM, fmt.Errorf("password is provided without a username. Please provide both a username and password, or leave both fields empty")
		}
		email, err := sources.GetIAMPrincipalEmailFromADC(ctx, "postgres")
		if err != nil {
			return "", useIAM, fmt.Errorf("error getting email from ADC: %v", err)
		}
		user = email
		dsn = fmt.Sprintf("user=%s dbname=%s sslmode=disable application_name=%s", user, dbname, userAgent)
	} else {
		// Construct IAM connection string with username
		dsn = fmt.Sprintf("user=%s dbname=%s sslmode=disable application_name=%s", user, dbname, userAgent)
	}

	if readOnly {
		// IMPORTANT: Must use underscore ('cloudsql_session_read_only'), NOT a dot.
		// PostgreSQL treats dotted GUCs (e.g. 'cloudsql.session_read_only') as custom placeholders
		// and silently ignores them at connection time, leaving the session in read-write mode.
		dsn += " options='-c cloudsql_session_read_only=locked'"
	}

	return dsn, useIAM, nil
}

func initCloudSQLPgConnectionPool(ctx context.Context, tracer trace.Tracer, name, project, region, instance, ipType, user, pass, dbname string, readOnly bool) (*pgxpool.Pool, error) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	// Configure the driver to connect to the database
	dsn, useIAM, err := getConnectionConfig(ctx, user, pass, dbname, readOnly)
	if err != nil {
		return nil, fmt.Errorf("unable to get Cloud SQL connection config: %w", err)
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to parse connection uri: %w", err)
	}

	// Create a new dialer with options
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}
	opts, err := sources.GetCloudSQLOpts(ipType, userAgent, useIAM)
	if err != nil {
		return nil, err
	}
	d, err := cloudsqlconn.NewDialer(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to parse connection uri: %w", err)
	}

	// Tell the driver to use the Cloud SQL Go Connector to create connections
	i := fmt.Sprintf("%s:%s:%s", project, region, instance)
	config.ConnConfig.DialFunc = func(ctx context.Context, _ string, instance string) (net.Conn, error) {
		return d.Dial(ctx, i)
	}

	// Interact with the driver directly as you normally would
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	return pool, nil
}
