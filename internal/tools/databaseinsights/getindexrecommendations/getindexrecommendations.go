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

package getindexrecommendations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/databaseinsights"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "databaseinsights-get-index-recommendations"

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
	BatchQueryIndexRecommendations(ctx context.Context, req *databaseinsights.BatchQueryIndexRecommendationsRequest) (*databaseinsights.BatchQueryIndexRecommendationsResponse, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		cfg.Description = `Fetches index advisor suggestions to optimize performance for a requested AlloyDB instance. Supports requesting recommendations for specific databases and a list of query IDs. Returns index recommendations including SQL commands (CREATE INDEX), target schema, relation, and columns, estimated storage size, and predicted query performance improvements (current vs estimated execution duration). Requires advanced query insights to be enabled.
Supported Engines:
  AlloyDB only.
Use Cases:
  - Targeted Optimization: "I found a slow query with ID <query_id> in my 'sales' database. What index should I add to fix it?"
  - Multi-Database Tuning: "Analyze performance recommendations for query <id_1> in the 'orders' database and query <id_2> in the 'shipping' database."
  - ROI Validation: "How much faster will my query become if I apply the suggested index, and how much storage will it consume?"
Response Fields:
  The response is structured under 'database_index_recommendations', which includes:
  1. **index_recommendations (List of Objects)**:
     - 'sql_command': The exact DDL statement to execute to create the index.
     - 'schema': The schema name for the recommended index.
     - 'relation': The table name for the recommended index.
     - 'columns': A list of column names the index targets.
     - 'estimated_storage_size_bytes': Predicted disk space consumption.
     - 'impacted_query_ids': List of query IDs that benefit from THIS specific index.
     - 'impacted_queries_count': Total number of queries that would be improved by this index.
     - 'index_recommendation_id': Unique ID for mapping back to query improvements.
  2. **query_improvements (Map of Query ID to Improvement Object)**:
     - 'query_id': The specific query hash being analyzed.
     - 'index_recommendation_ids': A list of IDs matching 'index_recommendation_id' above that contribute to this query's gain.
     - 'current_total_execution_duration': Baseline performance metric.
     - 'estimated_new_total_execution_duration': Predicted performance after index application.
**UNIT AND FORMATTING RULES:**
  1. **Execution Durations**: The 'current_total_execution_duration' and 'estimated_new_total_execution_duration' are in seconds.
     ALWAYS report these values, rounded to two decimal places, in **milliseconds** (ms) if less than 1 second, or in **seconds** (s) if 1 second or more.
     Example: A duration of 0.00111s should be reported as 1.11 ms.
  2. **Storage Size**: The 'estimated_storage_size_bytes' is in bytes.
     Use the 1024-based binary conversion for byte units (e.g., KiB, MiB, GiB).
     Example: A size of 26124288 bytes should be reported as 24.91 MB.
**Important**:
- Recommendations may only be available for a subset of the requested IDs or no IDs at all.
- Always cross-reference the 'query_improvements' map and 'impacted_query_ids' to verify which requested IDs are actually valid for optimization.
- If 'query_ids' was empty in the request, the response provides aggregated instance-level recommendations.`
	}
	params := buildParams()
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: params.Manifest(), AuthRequired: cfg.AuthRequired},
			params,
		),
	}, nil
}

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

	paramsMap := params.AsMap()

	parent, ok := paramsMap["parent"].(string)
	if !ok || parent == "" {
		return nil, util.NewAgentError("invalid or missing 'parent' parameter; expected a non-empty string", nil)
	}

	fullResourceName, ok := paramsMap["full_resource_name"].(string)
	if !ok || fullResourceName == "" {
		return nil, util.NewAgentError("invalid or missing 'full_resource_name' parameter; expected a non-empty string", nil)
	}

	var dbQueryIds []databaseinsights.DatabaseQueryIds
	if rawDbQueryIdsVal, exists := paramsMap["database_query_ids"]; exists && rawDbQueryIdsVal != nil {
		rawDatabaseQueryIds, ok := rawDbQueryIdsVal.([]any)
		if !ok {
			return nil, util.NewAgentError("invalid 'database_query_ids' parameter; expected a list of database-query configurations", nil)
		}

		for i, rawItem := range rawDatabaseQueryIds {
			itemMap, ok := rawItem.(map[string]any)
			if !ok {
				return nil, util.NewAgentError(fmt.Sprintf("invalid item at index %d in 'database_query_ids'; expected a map", i), nil)
			}

			database, ok := itemMap["database"].(string)
			if !ok || database == "" {
				return nil, util.NewAgentError(fmt.Sprintf("invalid or missing 'database' field in 'database_query_ids' item at index %d", i), nil)
			}

			var qids []string
			if rawQidsVal, exists := itemMap["query_ids"]; exists && rawQidsVal != nil {
				rawQids, ok := rawQidsVal.([]any)
				if !ok {
					return nil, util.NewAgentError(fmt.Sprintf("invalid 'query_ids' field in 'database_query_ids' item at index %d; expected a list of query IDs", i), nil)
				}
				for j, rawQid := range rawQids {
					qid, err := parseQueryID(rawQid)
					if err != nil {
						return nil, util.NewAgentError(fmt.Sprintf("invalid query ID at index %d under 'query_ids' at item %d: %v", j, i, err), nil)
					}
					qids = append(qids, qid)
				}
			}

			dbQueryIds = append(dbQueryIds, databaseinsights.DatabaseQueryIds{
				Database: database,
				QueryIDs: qids,
			})
		}
	}

	req := &databaseinsights.BatchQueryIndexRecommendationsRequest{
		Parent:           parent,
		FullResourceName: fullResourceName,
		DatabaseQueryIds: dbQueryIds,
	}

	resp, err := source.BatchQueryIndexRecommendations(ctx, req)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	return resp, nil
}

func parseQueryID(v any) (string, error) {
	switch val := v.(type) {
	case string:
		return val, nil
	case json.Number:
		return val.String(), nil
	case float64:
		return fmt.Sprintf("%.0f", val), nil
	case int:
		return fmt.Sprintf("%d", val), nil
	case int64:
		return fmt.Sprintf("%d", val), nil
	default:
		return "", fmt.Errorf("invalid type %T", v)
	}
}

func buildParams() parameters.Parameters {
	return parameters.Parameters{
		parameters.NewStringParameter("parent", "Required. Project and location. Format: projects/{project_id}/locations/{location}", parameters.WithStringRequired(true)),
		parameters.NewStringParameter("full_resource_name", "Required. The full identifier for the AlloyDB instance. Provide the full resource name ONLY in the following format: //alloydb.googleapis.com/projects/{project_id}/locations/{location}/clusters/{cluster_id}/instances/{instance_id}", parameters.WithStringRequired(true)),
		parameters.NewArrayParameter(
			"database_query_ids",
			"Optional. A list of objects used to target specific queries. Example schema: [{'database': 'dbname', 'query_ids': [12345]}]",
			parameters.NewMapParameter("", "", ""),
			parameters.WithArrayRequired(false),
		),
	}
}
