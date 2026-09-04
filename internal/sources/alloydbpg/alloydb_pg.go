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

package alloydbpg

import (
	"context"
	"fmt"
	"net"
	"strings"

	"cloud.google.com/go/alloydbconn"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/sqlcommenter"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "alloydb-postgres"

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
	Cluster      string         `yaml:"cluster" validate:"required"`
	Instance     string         `yaml:"instance" validate:"required"`
	IPType       sources.IPType `yaml:"ipType" validate:"required"`
	User         string         `yaml:"user"`
	Password     string         `yaml:"password"`
	Database     string         `yaml:"database" validate:"required"`
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
		conn:   sources.NewConnectOnce[*pgxpool.Pool](ctx, r.Name, SourceType, tracer),
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[*pgxpool.Pool]
}

func (s *Source) pool(ctx context.Context) (*pgxpool.Pool, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*pgxpool.Pool, error) {
		pool, err := initAlloyDBPgConnectionPool(ctx, s.Project, s.Region, s.Cluster, s.Instance, s.IPType.String(), s.User, s.Password, s.Database, s.ReadOnly)
		if err != nil {
			return nil, fmt.Errorf("unable to create pool: %w", err)
		}

		err = pool.Ping(ctx)
		if err != nil {
			pool.Close()
			if s.ReadOnly &&
				strings.Contains(err.Error(), "unrecognized configuration parameter") &&
				strings.Contains(err.Error(), "alloydb_session_read_only") {
				return nil, fmt.Errorf("failed to initialize AlloyDB source in read-only mode: 'alloydb_session_read_only' is not supported on this instance version. See documentation for details: https://mcp-toolbox.dev/integrations/alloydb/source/#reference: %w", err)
			}
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

// PostgresPool reports the pool once connected; use PostgresPoolContext to guarantee one.
func (s *Source) PostgresPool() *pgxpool.Pool {
	pool, _ := s.conn.Get()
	return pool
}

// PostgresPoolContext returns the pool, connecting on first use.
func (s *Source) PostgresPoolContext(ctx context.Context) (*pgxpool.Pool, error) {
	return s.pool(ctx)
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	pool, err := s.pool(ctx)
	if err != nil {
		return nil, err
	}
	statement = sqlcommenter.PrependComment(ctx, statement, SourceType, s.SQLCommenter)
	results, err := pool.Query(ctx, statement, params...)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	defer results.Close()

	fields := results.FieldDescriptions()
	out := []any{}
	for results.Next() {
		v, err := results.Values()
		if err != nil {
			return nil, fmt.Errorf("unable to parse row: %w", err)
		}
		row := orderedmap.Row{}
		for i, f := range fields {
			val := sources.NormalizeValue(v[i], f.DataTypeOID)
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

func getOpts(ipType, userAgent string, useIAM bool) ([]alloydbconn.Option, error) {
	opts := []alloydbconn.Option{alloydbconn.WithUserAgent(userAgent)}
	switch strings.ToLower(ipType) {
	case "private":
		opts = append(opts, alloydbconn.WithDefaultDialOptions(alloydbconn.WithPrivateIP()))
	case "public":
		opts = append(opts, alloydbconn.WithDefaultDialOptions(alloydbconn.WithPublicIP()))
	case "psc":
		opts = append(opts, alloydbconn.WithDefaultDialOptions(alloydbconn.WithPSC()))
	default:
		return nil, fmt.Errorf("invalid ipType %s", ipType)
	}

	if useIAM {
		opts = append(opts, alloydbconn.WithIAMAuthN())
	}
	return opts, nil
}

const (
	passwordDSNFormat = "user=%s password=%s dbname=%s sslmode=disable application_name=%s"
	iamDSNFormat      = "user=%s dbname=%s sslmode=disable application_name=%s"
)

func getConnectionConfig(ctx context.Context, user, pass, dbname string, readOnly bool) (string, bool, error) {
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		userAgent = "genai-toolbox"
	}
	useIAM := true

	var dsn string
	// If username and password both provided, use password authentication
	if user != "" && pass != "" {
		dsn = fmt.Sprintf(passwordDSNFormat, user, pass, dbname, userAgent)
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
		dsn = fmt.Sprintf(iamDSNFormat, user, dbname, userAgent)
	} else {
		// Construct IAM connection string with username
		dsn = fmt.Sprintf(iamDSNFormat, user, dbname, userAgent)
	}

	if readOnly {
		// IMPORTANT: Must use underscore ('alloydb_session_read_only'), NOT a dot.
		// PostgreSQL treats dotted GUCs (e.g. 'alloydb.session_read_only') as custom placeholders
		// and silently ignores them at connection time, leaving the session in read-write mode.
		dsn += " options='-c alloydb_session_read_only=locked'"
	}

	return dsn, useIAM, nil
}

func initAlloyDBPgConnectionPool(ctx context.Context, project, region, cluster, instance, ipType, user, pass, dbname string, readOnly bool) (*pgxpool.Pool, error) {
	dsn, useIAM, err := getConnectionConfig(ctx, user, pass, dbname, readOnly)
	if err != nil {
		return nil, fmt.Errorf("unable to get AlloyDB connection config: %w", err)
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
	opts, err := getOpts(ipType, userAgent, useIAM)
	if err != nil {
		return nil, err
	}
	d, err := alloydbconn.NewDialer(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to parse connection uri: %w", err)
	}

	// Tell the driver to use the AlloyDB Go Connector to create connections
	i := fmt.Sprintf("projects/%s/locations/%s/clusters/%s/instances/%s", project, region, cluster, instance)
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
