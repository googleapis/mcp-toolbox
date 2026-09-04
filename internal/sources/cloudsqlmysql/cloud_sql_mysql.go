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

package cloudsqlmysql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"slices"

	"cloud.google.com/go/cloudsqlconn/mysql/mysql"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/sqlcommenter"
	"github.com/googleapis/mcp-toolbox/internal/tools/mysql/mysqlcommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "cloud-sql-mysql"

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
	IPType       sources.IPType `yaml:"ipType"`
	User         string         `yaml:"user"`
	Password     string         `yaml:"password"`
	Database     string         `yaml:"database"`
	ReadOnly     bool           `yaml:"readOnly"`
	SQLCommenter *bool          `yaml:"sqlCommenter"`
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
		pool, err := initCloudSQLMySQLConnectionPool(ctx, s.Name, s.Project, s.Region, s.Instance, s.IPType.String(), s.User, s.Password, s.Database, s.ReadOnly)
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
	return s.ReadOnly
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

// MySQLPool reports the pool once connected; use MySQLPoolContext to guarantee one.
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

func getConnectionConfig(ctx context.Context, user, pass string) (string, string, bool, error) {
	useIAM := true

	// If username and password both provided, use password authentication
	if user != "" && pass != "" {
		useIAM = false
		return user, pass, useIAM, nil
	}

	// If username is empty, fetch email from ADC
	// otherwise, use username as IAM email
	if user == "" {
		if pass != "" {
			return "", "", useIAM, fmt.Errorf("password is provided without a username. Please provide both a username and password, or leave both fields empty")
		}
		email, err := sources.GetIAMPrincipalEmailFromADC(ctx, "mysql")
		if err != nil {
			return "", "", useIAM, fmt.Errorf("error getting email from ADC: %v", err)
		}
		user = email
	}

	// Pass the user, empty password and useIAM set to true
	return user, pass, useIAM, nil
}

func buildDSN(user, pass, driverName, project, region, instance, dbname, userAgent string, useIAM, readOnly bool) string {
	connAttrs := fmt.Sprintf("program_name:%s", url.QueryEscape(userAgent))
	if readOnly {
		connAttrs += ",read_only_connection:true"
	}

	if useIAM {
		return fmt.Sprintf("%s@%s(%s:%s:%s)/%s?connectionAttributes=%s",
			user,
			driverName,
			project,
			region,
			instance,
			dbname,
			connAttrs,
		)
	}
	return fmt.Sprintf("%s:%s@%s(%s:%s:%s)/%s?connectionAttributes=%s",
		user,
		pass,
		driverName,
		project,
		region,
		instance,
		dbname,
		connAttrs,
	)
}

func initCloudSQLMySQLConnectionPool(ctx context.Context, name, project, region, instance, ipType, user, pass, dbname string, readOnly bool) (*sql.DB, error) {
	// Configure the driver to connect to the database
	user, pass, useIAM, err := getConnectionConfig(ctx, user, pass)
	if err != nil {
		return nil, fmt.Errorf("unable to get Cloud SQL connection config: %w", err)
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

	// Use a unique driver name based on the source name.
	driverName := fmt.Sprintf("cloudsql-mysql-%s", name)

	if !slices.Contains(sql.Drivers(), driverName) {
		if _, err := mysql.RegisterDriver(driverName, opts...); err != nil {
			return nil, fmt.Errorf("unable to register driver: %w", err)
		}
	}

	dsn := buildDSN(user, pass, driverName, project, region, instance, dbname, userAgent, useIAM, readOnly)

	db, err := sql.Open(
		driverName,
		dsn,
	)
	if err != nil {
		return nil, err
	}
	return db, nil
}
