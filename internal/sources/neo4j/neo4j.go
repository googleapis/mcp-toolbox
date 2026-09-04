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

package neo4j

import (
	"context"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools/neo4j/neo4jexecutecypher/classifier"
	"github.com/googleapis/mcp-toolbox/internal/tools/neo4j/neo4jschema/helpers"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	neo4jconf "github.com/neo4j/neo4j-go-driver/v6/neo4j/config"

	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "neo4j"

var sourceClassifier *classifier.QueryClassifier = classifier.NewQueryClassifier()

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name, Database: "neo4j"} // Default database
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type Config struct {
	Name     string `yaml:"name" validate:"required"`
	Type     string `yaml:"type" validate:"required"`
	Uri      string `yaml:"uri" validate:"required"`
	User     string `yaml:"user" validate:"required"`
	Password string `yaml:"password" validate:"required"`
	Database string `yaml:"database" validate:"required"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s := r.newSource(ctx, tracer)
	if deferConnect {
		return s, nil
	}
	if _, err := s.driver(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r Config) newSource(ctx context.Context, tracer trace.Tracer) *Source {
	if r.Database == "" {
		r.Database = "neo4j"
	}
	return &Source{
		Config: r,
		conn:   sources.NewConnectOnce[neo4j.Driver](ctx, r.Name, SourceType, tracer),
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[neo4j.Driver]
}

func (s *Source) driver(ctx context.Context) (neo4j.Driver, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (neo4j.Driver, error) {
		driver, err := initNeo4jDriver(ctx, s.Uri, s.User, s.Password)
		if err != nil {
			return nil, fmt.Errorf("unable to create driver: %w", err)
		}

		if err := driver.VerifyConnectivity(ctx); err != nil {
			driver.Close(ctx)
			return nil, fmt.Errorf("unable to connect successfully: %w", err)
		}
		return driver, nil
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

// Neo4jDriver reports the driver once connected; use Neo4jDriverContext to guarantee one.
func (s *Source) Neo4jDriver() neo4j.Driver {
	driver, _ := s.conn.Get()
	return driver
}

// Neo4jDriverContext returns the driver, connecting on first use.
func (s *Source) Neo4jDriverContext(ctx context.Context) (neo4j.Driver, error) {
	return s.driver(ctx)
}

func (s *Source) Neo4jDatabase() string {
	return s.Database
}

func (s *Source) RunQuery(ctx context.Context, cypherStr string, params map[string]any, readOnly, dryRun bool) (any, error) {
	// validate the cypher query before executing
	cf := sourceClassifier.Classify(cypherStr)
	if cf.Error != nil {
		return nil, cf.Error
	}

	if cf.Type == classifier.WriteQuery && readOnly {
		return nil, fmt.Errorf("this tool is read-only and cannot execute write queries")
	}

	if dryRun {
		// Add EXPLAIN to the beginning of the query to validate it without executing
		cypherStr = "EXPLAIN " + cypherStr
	}

	driver, err := s.driver(ctx)
	if err != nil {
		return nil, err
	}

	config := neo4j.ExecuteQueryWithDatabase(s.Neo4jDatabase())
	results, err := neo4j.ExecuteQuery[*neo4j.EagerResult](ctx, driver, cypherStr, params,
		neo4j.EagerResultTransformer, config)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}

	// If dry run, return the summary information only
	if dryRun {
		summary := results.Summary
		plan := summary.Plan()
		execPlan := map[string]any{
			"queryType":     cf.Type.String(),
			"statementType": summary.QueryType(),
			"operator":      plan.Operator(),
			"arguments":     plan.Arguments(),
			"identifiers":   plan.Identifiers(),
			"childrenCount": len(plan.Children()),
		}
		if len(plan.Children()) > 0 {
			execPlan["children"] = addPlanChildren(plan)
		}
		return []map[string]any{execPlan}, nil
	}

	var out []map[string]any
	keys := results.Keys
	records := results.Records
	for _, record := range records {
		vMap := make(map[string]any)
		for col, value := range record.Values {
			vMap[keys[col]] = helpers.ConvertValue(value)
		}
		out = append(out, vMap)
	}

	return out, nil
}

// Recursive function to add plan children
func addPlanChildren(p neo4j.Plan) []map[string]any {
	var children []map[string]any
	for _, child := range p.Children() {
		childMap := map[string]any{
			"operator":       child.Operator(),
			"arguments":      child.Arguments(),
			"identifiers":    child.Identifiers(),
			"children_count": len(child.Children()),
		}
		if len(child.Children()) > 0 {
			childMap["children"] = addPlanChildren(child)
		}
		children = append(children, childMap)
	}
	return children
}

func initNeo4jDriver(ctx context.Context, uri, user, password string) (neo4j.Driver, error) {
	auth := neo4j.BasicAuth(user, password, "")
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}
	driver, err := neo4j.NewDriver(uri, auth, func(config *neo4jconf.Config) {
		config.UserAgent = userAgent
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create connection driver: %w", err)
	}
	return driver, nil
}
