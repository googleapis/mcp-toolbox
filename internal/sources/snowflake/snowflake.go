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
	"crypto/rsa"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/jmoiron/sqlx"
	"github.com/snowflakedb/gosnowflake/v2"
	"github.com/youmark/pkcs8"
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
	Name                 string `yaml:"name" validate:"required"`
	Type                 string `yaml:"type" validate:"required"`
	Account              string `yaml:"account" validate:"required"`
	User                 string `yaml:"user" validate:"required"`
	Password             string `yaml:"password" validate:"required_without_all=PrivateKey PrivateKeyPath"`
	PrivateKey           string `yaml:"privateKey"`
	PrivateKeyPath       string `yaml:"privateKeyPath"`
	PrivateKeyPassphrase string `yaml:"privateKeyPassphrase"`
	Database             string `yaml:"database" validate:"required"`
	Schema               string `yaml:"schema" validate:"required"`
	Warehouse            string `yaml:"warehouse"`
	Role                 string `yaml:"role"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s := &Source{
		Config: r,
		conn:   sources.NewConnectOnce[*sqlx.DB](ctx, r.Name, SourceType, tracer),
	}
	if deferConnect {
		return s, nil
	}
	if _, err := s.SnowflakeDBContext(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[*sqlx.DB]
}

// SnowflakeDBContext returns the database handle, connecting on first use. It is the
// discriminator the snowflake tools assert on.
func (s *Source) SnowflakeDBContext(ctx context.Context) (*sqlx.DB, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*sqlx.DB, error) {
		r := s.Config
		db, err := initSnowflakeConnection(ctx, r)
		if err != nil {
			return nil, fmt.Errorf("unable to create connection: %w", err)
		}
		if err := db.PingContext(ctx); err != nil {
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

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	db, err := s.SnowflakeDBContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryxContext(ctx, statement, params...)
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

func initSnowflakeConnection(ctx context.Context, r Config) (*sqlx.DB, error) {
	dsn, err := r.dsn()
	if err != nil {
		return nil, err
	}
	db, err := sqlx.ConnectContext(ctx, "snowflake", dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection: %w", err)
	}

	return db, nil
}

// dsn builds the connection string for either password or key-pair (JWT) authentication.
func (r Config) dsn() (string, error) {
	// Set defaults for optional parameters
	warehouse, role := r.Warehouse, r.Role
	if warehouse == "" {
		warehouse = "COMPUTE_WH"
	}
	if role == "" {
		role = "ACCOUNTADMIN"
	}

	cfg := &gosnowflake.Config{
		Account:   r.Account,
		User:      r.User,
		Database:  r.Database,
		Schema:    r.Schema,
		Warehouse: warehouse,
		Role:      role,
	}
	if r.PrivateKey != "" || r.PrivateKeyPath != "" {
		if r.Password != "" {
			return "", errors.New("only one of 'password', 'privateKey' or 'privateKeyPath' can be set")
		}
		key, err := r.rsaPrivateKey()
		if err != nil {
			return "", err
		}
		cfg.Authenticator = gosnowflake.AuthTypeJwt
		cfg.PrivateKey = key
	} else {
		cfg.Password = r.Password
	}

	dsn, err := gosnowflake.DSN(cfg)
	if err != nil {
		return "", fmt.Errorf("unable to build dsn: %w", err)
	}
	return dsn, nil
}

// rsaPrivateKey loads the PKCS#8 key used for key-pair (JWT) authentication.
func (r Config) rsaPrivateKey() (*rsa.PrivateKey, error) {
	keyPEM := []byte(r.PrivateKey)
	if r.PrivateKeyPath != "" {
		if r.PrivateKey != "" {
			return nil, errors.New("only one of 'privateKey' or 'privateKeyPath' can be set")
		}
		b, err := os.ReadFile(r.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("unable to read private key file: %w", err)
		}
		keyPEM = b
	}

	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("unable to decode private key: no PEM block found")
	}
	key, err := pkcs8.ParsePKCS8PrivateKeyRSA(block.Bytes, []byte(r.PrivateKeyPassphrase))
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key: %w", err)
	}
	return key, nil
}
