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

package trino

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	trinogo "github.com/trinodb/trino-go-client/trino"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "trino"

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
	Name                   string `yaml:"name" validate:"required"`
	Type                   string `yaml:"type" validate:"required"`
	Host                   string `yaml:"host" validate:"required"`
	Port                   string `yaml:"port" validate:"required"`
	User                   string `yaml:"user"`
	Password               string `yaml:"password"`
	Catalog                string `yaml:"catalog" validate:"required"`
	Schema                 string `yaml:"schema" validate:"required"`
	QueryTimeout           string `yaml:"queryTimeout"`
	AccessToken            string `yaml:"accessToken"`
	KerberosEnabled        bool   `yaml:"kerberosEnabled"`
	SSLEnabled             bool   `yaml:"sslEnabled"`
	SSLCertPath            string `yaml:"sslCertPath"`
	SSLCert                string `yaml:"sslCert"`
	DisableSslVerification bool   `yaml:"disableSslVerification"`
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
	// The ping issues a query, which Trino bounds by queryTimeout, so the
	// connect must not be capped tighter. Trino owns this value's format, so a
	// string it does not recognise as a duration just leaves the default.
	var opts []sources.Option
	if timeout, err := time.ParseDuration(r.QueryTimeout); err == nil && timeout > 0 {
		opts = append(opts, sources.WithMinConnectTimeout(timeout))
	}
	return &Source{
		Config: r,
		conn:   sources.NewConnectOnce[*sql.DB](ctx, r.Name, SourceType, tracer, opts...),
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[*sql.DB]
}

func (s *Source) pool(ctx context.Context) (*sql.DB, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*sql.DB, error) {
		pool, err := initTrinoConnectionPool(ctx, s.Name, s.Host, s.Port, s.User, s.Password, s.Catalog, s.Schema, s.QueryTimeout, s.AccessToken, s.KerberosEnabled, s.SSLEnabled, s.SSLCertPath, s.SSLCert, s.DisableSslVerification)
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

// TrinoDB reports the pool once connected.
func (s *Source) TrinoDB() *sql.DB {
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
	defer results.Close()

	cols, err := results.Columns()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve column names: %w", err)
	}

	// create an array of values for each column, which can be re-used to scan each row
	rawValues := make([]any, len(cols))
	values := make([]any, len(cols))
	for i := range rawValues {
		values[i] = &rawValues[i]
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

			// Convert byte arrays to strings for text fields
			if b, ok := val.([]byte); ok {
				vMap[name] = string(b)
			} else {
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

func initTrinoConnectionPool(ctx context.Context, name, host, port, user, password, catalog, schema, queryTimeout, accessToken string, kerberosEnabled, sslEnabled bool, sslCertPath, sslCert string, disableSslVerification bool) (*sql.DB, error) {
	// Build Trino DSN
	dsn, err := buildTrinoDSN(host, port, user, password, catalog, schema, queryTimeout, accessToken, kerberosEnabled, sslEnabled, sslCertPath, sslCert)
	if err != nil {
		return nil, fmt.Errorf("failed to build DSN: %w", err)
	}

	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get logger from ctx: %s", err)
	}

	if disableSslVerification {
		logger.WarnContext(ctx, "SSL verification is disabled for trino source %s. This is an insecure setting and should not be used in production.\n", name)
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{Transport: tr}
		clientName := fmt.Sprintf("insecure_trino_client_%s", name)
		if err := trinogo.RegisterCustomClient(clientName, client); err != nil {
			return nil, fmt.Errorf("failed to register custom client: %w", err)
		}
		dsn = fmt.Sprintf("%s&custom_client=%s", dsn, clientName)
	}

	db, err := sql.Open("trino", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	return db, nil
}

func buildTrinoDSN(host, port, user, password, catalog, schema, queryTimeout, accessToken string, kerberosEnabled, sslEnabled bool, sslCertPath, sslCert string) (string, error) {
	// Build query parameters
	query := url.Values{}
	query.Set("catalog", catalog)
	query.Set("schema", schema)
	if queryTimeout != "" {
		query.Set("queryTimeout", queryTimeout)
	}
	if accessToken != "" {
		query.Set("accessToken", accessToken)
	}
	if kerberosEnabled {
		query.Set("KerberosEnabled", "true")
	}
	if sslCertPath != "" {
		query.Set("sslCertPath", sslCertPath)
	}
	if sslCert != "" {
		query.Set("sslCert", sslCert)
	}

	// Build URL
	scheme := "http"
	if sslEnabled {
		scheme = "https"
	}

	u := &url.URL{
		Scheme:   scheme,
		Host:     fmt.Sprintf("%s:%s", host, port),
		RawQuery: query.Encode(),
	}

	// Only set user and password if not empty
	if user != "" && password != "" {
		u.User = url.UserPassword(user, password)
	} else if user != "" {
		u.User = url.User(user)
	}

	return u.String(), nil
}
