// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scylladb

import (
	"context"
	"fmt"

	"github.com/goccy/go-yaml"
	gocql "github.com/gocql/gocql"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "scylladb"

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

// Config holds the parameters for establishing a ScyllaDB connection.
// Use LocalDC to enable DC-aware load balancing, which is required when
// connecting to ScyllaDB Cloud.
type Config struct {
	Name         string   `yaml:"name" validate:"required"`
	Type         string   `yaml:"type" validate:"required"`
	Hosts        []string `yaml:"hosts" validate:"required"`
	Keyspace     string   `yaml:"keyspace"`
	ProtoVersion int      `yaml:"protoVersion"`
	Username     string   `yaml:"username"`
	Password     string   `yaml:"password"`
	// LocalDC enables DC-aware token-aware load balancing. Required for
	// ScyllaDB Cloud connections (e.g. "AWS_US_EAST_1").
	LocalDC string `yaml:"localDC"`
	// DisableInitialHostLookup disables the initial host discovery step.
	// Set to true when connecting through a proxy, port-forward, or in
	// containerized environments where the cluster's internal IPs are not
	// reachable from the client.
	DisableInitialHostLookup bool   `yaml:"disableInitialHostLookup"`
	CAPath                   string `yaml:"caPath"`
	CertPath                 string `yaml:"certPath"`
	KeyPath                  string `yaml:"keyPath"`
	EnableHostVerification   bool   `yaml:"enableHostVerification"`
}

// Initialize implements sources.SourceConfig.
func (c Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	session, err := initScyllaDBSession(ctx, tracer, c)
	if err != nil {
		return nil, fmt.Errorf("unable to create session: %v", err)
	}
	s := &Source{
		Config:  c,
		Session: session,
	}
	return s, nil
}

// SourceConfigType implements sources.SourceConfig.
func (c Config) SourceConfigType() string {
	return SourceType
}

var _ sources.SourceConfig = Config{}

type Source struct {
	Config
	Session *gocql.Session
}

// ScyllaDBSession returns the underlying ScyllaDB session.
func (s *Source) ScyllaDBSession() *gocql.Session {
	return s.Session
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

// SourceType implements sources.Source.
func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) RunSQL(ctx context.Context, statement string, params parameters.ParamValues) (any, error) {
	sliceParams := params.AsSlice()
	iter := s.ScyllaDBSession().Query(statement, sliceParams...).WithContext(ctx).Iter()

	// Create a slice to store the output
	var out []map[string]interface{}

	// Scan results into a map and append to the slice
	for {
		row := make(map[string]interface{}) // Create a new map for each row
		if !iter.MapScan(row) {
			break // No more rows
		}
		out = append(out, row)
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("failed to execute ScyllaDB query: %w", err)
	}
	return out, nil
}

var _ sources.Source = &Source{}

func initScyllaDBSession(ctx context.Context, tracer trace.Tracer, c Config) (*gocql.Session, error) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, c.Name)
	defer span.End()

	// Validate authentication configuration
	if c.Password != "" && c.Username == "" {
		return nil, fmt.Errorf("invalid ScyllaDB configuration: password provided without a username")
	}

	cluster := gocql.NewCluster(c.Hosts...)
	cluster.ProtoVersion = c.ProtoVersion
	cluster.Keyspace = c.Keyspace
	cluster.DisableInitialHostLookup = c.DisableInitialHostLookup

	// Configure DC-aware token-aware host selection policy.
	// This is required for ScyllaDB Cloud and recommended for all multi-DC
	// deployments to ensure queries are routed to the correct datacenter.
	if c.LocalDC != "" {
		cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(
			gocql.DCAwareRoundRobinPolicy(c.LocalDC),
		)
	}

	// Configure authentication if username is provided
	if c.Username != "" {
		cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: c.Username,
			Password: c.Password,
		}
	}

	// Configure SSL options if any are specified
	if c.CAPath != "" || c.CertPath != "" || c.KeyPath != "" || c.EnableHostVerification {
		cluster.SslOpts = &gocql.SslOptions{
			CaPath:                 c.CAPath,
			CertPath:               c.CertPath,
			KeyPath:                c.KeyPath,
			EnableHostVerification: c.EnableHostVerification,
		}
	}

	// Create session
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create ScyllaDB session: %w", err)
	}
	return session, nil
}
