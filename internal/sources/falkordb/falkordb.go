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

package falkordb

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	falkordb "github.com/FalkorDB/falkordb-go/v2"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "falkordb"

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
	Name           string    `yaml:"name" validate:"required"`
	Type           string    `yaml:"type" validate:"required"`
	Host           string    `yaml:"host" validate:"required"`
	Port           string    `yaml:"port" validate:"required"`
	Username       string    `yaml:"username"`
	Password       string    `yaml:"password"`
	Graph          string    `yaml:"graph" validate:"required"`
	QueryTimeoutMs int       `yaml:"queryTimeoutMs"`
	TLS            TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	Enabled            bool `yaml:"enabled"`
	InsecureSkipVerify bool `yaml:"insecureSkipVerify"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

// validateTLS rejects a TLS configuration whose settings contradict each
// other. Without TLS there is no certificate to verify, so insecureSkipVerify
// would otherwise be accepted and silently ignored.
func (r Config) validateTLS() error {
	if !r.TLS.Enabled && r.TLS.InsecureSkipVerify {
		return fmt.Errorf("tls.insecureSkipVerify is set on source %q but tls.enabled is false; enable TLS or remove the setting", r.Name)
	}
	return nil
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	if err := r.validateTLS(); err != nil {
		return nil, err
	}

	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get logger from ctx: %s", err)
	}
	if r.TLS.InsecureSkipVerify {
		logger.WarnContext(ctx, fmt.Sprintf("TLS certificate verification is skipped (insecureSkipVerify: true) for FalkorDB source %s. This exposes traffic for this source to man-in-the-middle attacks. Do not use in production.", r.Name))
	}

	s := r.newSource(ctx, tracer)
	if deferConnect {
		return s, nil
	}
	if _, err := s.client(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r Config) newSource(ctx context.Context, tracer trace.Tracer) *Source {
	return &Source{
		Config: r,
		conn:   sources.NewConnectOnce[*falkordb.FalkorDB](ctx, r.Name, SourceType, tracer),
	}
}

func initFalkorDBClient(ctx context.Context, r Config) (*falkordb.FalkorDB, error) {
	opts := &falkordb.ConnectionOption{
		Addr:     net.JoinHostPort(r.Host, r.Port),
		Username: r.Username,
		Password: r.Password,
	}
	if r.TLS.Enabled {
		opts.TLSConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: r.TLS.InsecureSkipVerify,
		}
	}

	client, err := falkordb.FalkorDBNew(opts)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection client: %w", err)
	}
	return client, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[*falkordb.FalkorDB]
}

func (s *Source) client(ctx context.Context) (*falkordb.FalkorDB, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*falkordb.FalkorDB, error) {
		client, err := initFalkorDBClient(ctx, s.Config)
		if err != nil {
			return nil, fmt.Errorf("unable to create client: %w", err)
		}
		if err := client.Conn.Ping(ctx).Err(); err != nil {
			client.Conn.Close()
			return nil, fmt.Errorf("unable to connect successfully: %w", err)
		}
		return client, nil
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

// FalkorDBClient reports the client once connected; use FalkorDBClientContext to guarantee one.
func (s *Source) FalkorDBClient() *falkordb.FalkorDB {
	client, _ := s.conn.Get()
	return client
}

// FalkorDBClientContext returns the client, connecting on first use.
func (s *Source) FalkorDBClientContext(ctx context.Context) (*falkordb.FalkorDB, error) {
	return s.client(ctx)
}

func (s *Source) DefaultGraph() string {
	return s.Graph
}

// RunQuery executes a Cypher query against a graph in the FalkorDB instance.
// An empty graphName targets the source's default graph. readOnly queries are
// dispatched as GRAPH.RO_QUERY, so the server itself rejects write operations.
// dryRun returns the GRAPH.EXPLAIN execution plan without running the query.
func (s *Source) RunQuery(ctx context.Context, graphName, cypherStr string, params map[string]any, readOnly, dryRun bool) (any, error) {
	if graphName == "" {
		graphName = s.Graph
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	graph := client.SelectGraph(graphName)

	if dryRun {
		// GRAPH.EXPLAIN replies with an array of plan lines, which
		// falkordb-go's ExecutionPlan does not handle; issue the command
		// directly instead.
		plan, err := client.Conn.Do(ctx, "GRAPH.EXPLAIN", graphName, cypherStr).StringSlice()
		if err != nil {
			return nil, fmt.Errorf("unable to explain query: %w", err)
		}
		return map[string]any{"plan": plan}, nil
	}

	var opts *falkordb.QueryOptions
	if s.QueryTimeoutMs > 0 {
		opts = falkordb.NewQueryOptions().SetTimeout(s.QueryTimeoutMs)
	}

	var results *falkordb.QueryResult
	if readOnly {
		results, err = graph.ROQuery(cypherStr, params, opts)
	} else {
		results, err = graph.Query(cypherStr, params, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}

	out := convertRecords(results)
	if len(out) == 0 {
		if stats := mutationStats(results); len(stats) > 0 {
			return map[string]any{"stats": stats}, nil
		}
	}
	return out, nil
}

// convertRecords converts a query result set into JSON-compatible rows.
func convertRecords(results *falkordb.QueryResult) []map[string]any {
	var out []map[string]any
	for results.Next() {
		record := results.Record()
		vMap := make(map[string]any)
		keys := record.Keys()
		for col, value := range record.Values() {
			vMap[keys[col]] = ConvertValue(value)
		}
		out = append(out, vMap)
	}
	return out
}

// mutationStats collects the non-zero mutation counters of a query result, so
// write queries without a RETURN clause still yield useful feedback.
func mutationStats(results *falkordb.QueryResult) map[string]any {
	stats := make(map[string]any)
	counters := map[string]int{
		"labelsAdded":          results.LabelsAdded(),
		"nodesCreated":         results.NodesCreated(),
		"nodesDeleted":         results.NodesDeleted(),
		"propertiesSet":        results.PropertiesSet(),
		"relationshipsCreated": results.RelationshipsCreated(),
		"relationshipsDeleted": results.RelationshipsDeleted(),
		"indicesCreated":       results.IndicesCreated(),
		"indicesDeleted":       results.IndicesDeleted(),
	}
	for name, count := range counters {
		if count != 0 {
			stats[name] = count
		}
	}
	if len(stats) > 0 {
		stats["executionTimeMs"] = results.InternalExecutionTime()
	}
	return stats
}

// ConvertValue converts a FalkorDB value to a JSON-compatible value.
func ConvertValue(value any) any {
	switch v := value.(type) {
	case nil:
		return nil
	case falkordb.Node:
		return ConvertValue(&v)
	case falkordb.Edge:
		return ConvertValue(&v)
	case *falkordb.Node:
		if v == nil {
			return nil
		}
		return map[string]any{
			"id":         v.ID,
			"labels":     v.Labels,
			"properties": ConvertValue(v.Properties),
		}
	case *falkordb.Edge:
		if v == nil {
			return nil
		}
		return map[string]any{
			"id":            v.ID,
			"type":          v.Relation,
			"sourceId":      v.SourceNodeID(),
			"destinationId": v.DestNodeID(),
			"properties":    ConvertValue(v.Properties),
		}
	case falkordb.Path:
		var nodes, edges []any
		for _, n := range v.Nodes {
			nodes = append(nodes, ConvertValue(n))
		}
		for _, e := range v.Edges {
			edges = append(edges, ConvertValue(e))
		}
		return map[string]any{
			"nodes": nodes,
			"edges": edges,
		}
	case []any:
		arr := make([]any, len(v))
		for i, elem := range v {
			arr[i] = ConvertValue(elem)
		}
		return arr
	case map[string]any:
		m := make(map[string]any)
		for key, val := range v {
			m[key] = ConvertValue(val)
		}
		return m
	default:
		return v
	}
}
