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

// Package falkordbschema provides a tool that extracts the schema of a
// FalkorDB graph: node labels, relationship types, observed property shapes,
// indexes, constraints, and statistics.
package falkordbschema

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/falkordb/falkordbschema/helpers"
	"github.com/googleapis/mcp-toolbox/internal/tools/falkordb/falkordbschema/types"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "falkordb-schema"

// defaultSampleSize is the number of entities sampled per label or
// relationship type when deriving property shapes, used when the tool does
// not configure a sampleSize.
const defaultSampleSize = 100

func init() {
	if !tools.Register(resourceType, newConfig) {
		panic(fmt.Sprintf("tool type %q already registered", resourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (tools.ToolConfig, error) {
	actual := Config{ConfigBase: tools.ConfigBase{Name: name}}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type compatibleSource interface {
	DefaultGraph() string
	RunQuery(context.Context, string, string, map[string]any, bool, bool) (any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	SampleSize       int                    `yaml:"sampleSize"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

// validate interface
var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	params := parameters.Parameters{}

	if cfg.SampleSize <= 0 {
		cfg.SampleSize = defaultSampleSize
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: params.Manifest(), AuthRequired: cfg.AuthRequired},
			params,
		),
	}, nil
}

// validate interface
var _ tools.Tool = Tool{}

type Tool struct {
	tools.BaseTool[Config]
}

func (t Tool) GetSourceName() string {
	return t.Cfg.Source
}

func (t Tool) ToConfig() tools.ToolConfig {
	return t.Cfg
}

func (t Tool) ValidateSource(source sources.Source) error {
	_, ok := source.(compatibleSource)
	if !ok {
		return fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return nil
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}

	schema, err := t.extractSchema(ctx, source)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}

	return schema, nil
}

// runReadQuery executes a read-only query against the source's default graph.
func runReadQuery(ctx context.Context, source compatibleSource, cypher string) (any, error) {
	return source.RunQuery(ctx, "", cypher, nil, true, false)
}

// escapeIdentifier makes a label or relationship type safe to embed between
// backticks in a Cypher pattern. Cypher escapes a backtick inside a quoted
// identifier by doubling it, so doubling preserves the identifier's meaning
// where stripping would silently target a different label.
func escapeIdentifier(identifier string) string {
	return strings.ReplaceAll(identifier, "`", "``")
}

// extractSchema orchestrates the concurrent extraction of the graph schema.
func (t Tool) extractSchema(ctx context.Context, source compatibleSource) (*types.SchemaInfo, error) {
	schema := &types.SchemaInfo{
		GraphInfo: types.GraphInfo{Name: source.DefaultGraph()},
		Statistics: types.Statistics{
			NodesByLabel:        make(map[string]int64),
			RelationshipsByType: make(map[string]int64),
		},
	}

	// FalkorDB creates graph keys lazily, so a graph nothing has written to
	// yet does not exist and every query on it fails. Report it as an empty
	// schema instead.
	if _, err := runReadQuery(ctx, source, "RETURN 1"); err != nil {
		if strings.Contains(err.Error(), "empty key") {
			return schema, nil
		}
		return nil, fmt.Errorf("failed to reach graph %q: %w", source.DefaultGraph(), err)
	}

	var mu sync.Mutex

	tasks := []struct {
		name string
		fn   func() error
	}{
		{
			name: "node-schema",
			fn: func() error {
				nodeLabels, err := t.extractNodeLabels(ctx, source)
				if err != nil {
					return fmt.Errorf("failed to extract node schema: %w", err)
				}
				mu.Lock()
				defer mu.Unlock()
				schema.NodeLabels = nodeLabels
				for _, label := range nodeLabels {
					schema.Statistics.NodesByLabel[label.Name] = label.Count
				}
				return nil
			},
		},
		{
			name: "relationship-schema",
			fn: func() error {
				relationships, err := t.extractRelationships(ctx, source)
				if err != nil {
					return fmt.Errorf("failed to extract relationship schema: %w", err)
				}
				mu.Lock()
				defer mu.Unlock()
				schema.Relationships = relationships
				for _, relationship := range relationships {
					schema.Statistics.RelationshipsByType[relationship.Type] = relationship.Count
				}
				return nil
			},
		},
		{
			name: "counts",
			fn: func() error {
				nodeResult, err := runReadQuery(ctx, source, "MATCH (n) RETURN count(n) AS count")
				if err != nil {
					return fmt.Errorf("failed to count nodes: %w", err)
				}
				edgeResult, err := runReadQuery(ctx, source, "MATCH ()-[r]->() RETURN count(r) AS count")
				if err != nil {
					return fmt.Errorf("failed to count relationships: %w", err)
				}
				mu.Lock()
				defer mu.Unlock()
				schema.GraphInfo.NodeCount = helpers.FirstRowInt64(nodeResult)
				schema.GraphInfo.EdgeCount = helpers.FirstRowInt64(edgeResult)
				schema.Statistics.TotalNodes = schema.GraphInfo.NodeCount
				schema.Statistics.TotalRelationships = schema.GraphInfo.EdgeCount
				return nil
			},
		},
		{
			name: "indexes",
			fn: func() error {
				result, err := runReadQuery(ctx, source, "CALL db.indexes()")
				if err != nil {
					return fmt.Errorf("failed to list indexes: %w", err)
				}
				mu.Lock()
				defer mu.Unlock()
				schema.Indexes = helpers.Rows(result)
				return nil
			},
		},
		{
			name: "constraints",
			fn: func() error {
				result, err := runReadQuery(ctx, source, "CALL db.constraints()")
				if err != nil {
					// Constraint listing varies across FalkorDB versions;
					// report the failure in the schema rather than failing
					// the whole extraction.
					mu.Lock()
					defer mu.Unlock()
					schema.Errors = append(schema.Errors, fmt.Sprintf("failed to list constraints: %s", err))
					return nil
				}
				mu.Lock()
				defer mu.Unlock()
				schema.Constraints = helpers.Rows(result)
				return nil
			},
		},
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(tasks))
	for _, task := range tasks {
		wg.Add(1)
		go func(fn func() error) {
			defer wg.Done()
			if err := fn(); err != nil {
				errs <- err
			}
		}(task.fn)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		return nil, err
	}

	return schema, nil
}

// extractNodeLabels lists the node labels and derives each label's count and
// property shapes from a sample of nodes.
func (t Tool) extractNodeLabels(ctx context.Context, source compatibleSource) ([]types.NodeLabel, error) {
	labelsResult, err := runReadQuery(ctx, source, "CALL db.labels()")
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	var nodeLabels []types.NodeLabel
	for _, label := range helpers.FirstColumnStrings(labelsResult) {
		escaped := escapeIdentifier(label)

		countResult, err := runReadQuery(ctx, source, fmt.Sprintf("MATCH (n:`%s`) RETURN count(n) AS count", escaped))
		if err != nil {
			return nil, fmt.Errorf("failed to count label %q: %w", label, err)
		}

		sampleResult, err := runReadQuery(ctx, source, fmt.Sprintf("MATCH (n:`%s`) RETURN n LIMIT %d", escaped, t.Cfg.SampleSize))
		if err != nil {
			return nil, fmt.Errorf("failed to sample label %q: %w", label, err)
		}
		accumulator := make(map[string]map[string]bool)
		for _, row := range helpers.Rows(sampleResult) {
			node, ok := row["n"].(map[string]any)
			if !ok {
				continue
			}
			if properties, ok := node["properties"].(map[string]any); ok {
				helpers.MergeProperties(accumulator, properties)
			}
		}

		nodeLabels = append(nodeLabels, types.NodeLabel{
			Name:       label,
			Count:      helpers.FirstRowInt64(countResult),
			Properties: helpers.CollectProperties(accumulator),
		})
	}
	helpers.SortNodeLabels(nodeLabels)
	return nodeLabels, nil
}

// extractRelationships lists the relationship types and derives each type's
// count, most common connectivity pattern, and property shapes from a sample
// of relationships.
func (t Tool) extractRelationships(ctx context.Context, source compatibleSource) ([]types.Relationship, error) {
	typesResult, err := runReadQuery(ctx, source, "CALL db.relationshipTypes()")
	if err != nil {
		return nil, fmt.Errorf("failed to list relationship types: %w", err)
	}

	var relationships []types.Relationship
	for _, relType := range helpers.FirstColumnStrings(typesResult) {
		escaped := escapeIdentifier(relType)

		countResult, err := runReadQuery(ctx, source, fmt.Sprintf("MATCH ()-[r:`%s`]->() RETURN count(r) AS count", escaped))
		if err != nil {
			return nil, fmt.Errorf("failed to count relationship type %q: %w", relType, err)
		}

		sampleResult, err := runReadQuery(ctx, source, fmt.Sprintf(
			"MATCH (a)-[r:`%s`]->(b) RETURN labels(a) AS startLabels, labels(b) AS endLabels, r LIMIT %d", escaped, t.Cfg.SampleSize))
		if err != nil {
			return nil, fmt.Errorf("failed to sample relationship type %q: %w", relType, err)
		}

		accumulator := make(map[string]map[string]bool)
		connectivity := make(map[types.RelConnectivityInfo]int64)
		for _, row := range helpers.Rows(sampleResult) {
			if edge, ok := row["r"].(map[string]any); ok {
				if properties, ok := edge["properties"].(map[string]any); ok {
					helpers.MergeProperties(accumulator, properties)
				}
			}
			pattern := types.RelConnectivityInfo{
				StartNode: firstString(row["startLabels"]),
				EndNode:   firstString(row["endLabels"]),
			}
			connectivity[pattern]++
		}

		relationship := types.Relationship{
			Type:       relType,
			Count:      helpers.FirstRowInt64(countResult),
			Properties: helpers.CollectProperties(accumulator),
		}
		var best int64
		for pattern, count := range connectivity {
			if count > best {
				best = count
				relationship.StartNode = pattern.StartNode
				relationship.EndNode = pattern.EndNode
			}
		}
		relationships = append(relationships, relationship)
	}
	helpers.SortRelationships(relationships)
	return relationships, nil
}

// firstString returns the first string of a converted list value, such as
// the result of the Cypher labels() function.
func firstString(value any) string {
	list, ok := value.([]any)
	if !ok || len(list) == 0 {
		return ""
	}
	s, _ := list[0].(string)
	return s
}
