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

package vectorassistimprovequeryrecall

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const resourceType string = "vector-assist-improve-query-recall"

// Query to check if the index exists and if it is an HNSW index.
const checkIndexQuery = `
	SELECT 
		(COUNT(1) > 0) AS index_present,
		COALESCE(BOOL_OR(indexdef ILIKE '%USING hnsw%'), false) AS is_hnsw
	FROM pg_indexes 
	WHERE schemaname = @schema_name::TEXT
		AND tablename = @table_name::TEXT
		AND indexname = @index_name::TEXT
		AND indexdef ILIKE '%' || @vector_column_name::TEXT || '%';
`

// Query to find the optimal index parameters
const improveRecallQuery = `
  SELECT output_ef_search AS ef_search
  FROM vector_assist.find_ef_search_for_target_recall(
    table_name => @table_name::TEXT,
    schema_name => @schema_name::TEXT,
    column_name => @vector_column_name::TEXT,
    top_k => @top_k::INT,
    target_recall => @target_recall::FLOAT,
    distance_func => @distance_func::TEXT
  );
`

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
	PostgresPool() *pgxpool.Pool
	RunSQL(context.Context, string, []any) (any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	allParameters := parameters.Parameters{
		parameters.NewStringParameter("schema_name", "Optional parameter: Schema name of the table.", parameters.WithStringDefault("public")),
		parameters.NewStringParameter("table_name", "Table name experiencing degraded vector search recall.", parameters.WithStringRequired(true)),
		parameters.NewStringParameter("vector_column_name", "Column name containing the vector embeddings.", parameters.WithStringRequired(true)),
		parameters.NewStringParameter("index_name", "Name of the vector index to tune.", parameters.WithStringRequired(true)),
		parameters.NewIntParameter("top_k", "Optional parameter: Top k value for the vector search.", parameters.WithIntDefault(10)),
		parameters.NewFloatParameter("target_recall", "Optional parameter: Target recall value for search results.", parameters.WithFloatDefault(0.95)),
		parameters.NewStringParameter("distance_func", "Optional parameter: Distance function used for the vector search similarity.", parameters.WithStringDefault("cosine")),
	}
	paramManifest := allParameters.Manifest()

	if cfg.Description == "" {
		cfg.Description = "Use this tool to troubleshoot and optimize existing vector search workloads when a user reports irrelevant results, poor accuracy, or degraded recall. It determines the optimal tuning parameter (such as ef_search) for an active vector index to improve the search results. The tool outputs an actionable SQL query recommendation to be executed as a next action using the 'execute_sql' tool."
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: paramManifest, AuthRequired: cfg.AuthRequired},
			allParameters,
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

	namedArgs := pgx.NamedArgs{}
	for key, value := range paramsMap {
		namedArgs[key] = value
	}

	// Check if the index exists and if it is an HNSW index.
	checkResp, err := source.RunSQL(ctx, checkIndexQuery, []any{namedArgs})
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}

	checkBytes, marshalErr := json.Marshal(checkResp)
	if marshalErr != nil {
		return nil, util.NewClientServerError("failed to process index check response", http.StatusInternalServerError, marshalErr)
	}

	var checkRows []map[string]interface{}
	if unmarshalErr := json.Unmarshal(checkBytes, &checkRows); unmarshalErr != nil || len(checkRows) == 0 {
		return nil, util.NewClientServerError("unexpected empty response from database", http.StatusInternalServerError, unmarshalErr)
	}

	row := checkRows[0]
	indexPresent, ok := row["index_present"].(bool)
	if !ok {
		// If the key is missing or isn't a boolean, it's likely a server-side/query issue.
		return nil, util.NewClientServerError("Internal error: 'index_present' is missing or has an invalid type.", http.StatusInternalServerError, nil)
	}
	if !indexPresent {
		return nil, util.NewClientServerError("Index not found for the given table and vector column. If the table lacks an existing vector setup, use the 'define_spec' tool to configure the database.", http.StatusBadRequest, nil)
	}

	isHnsw, ok := row["is_hnsw"].(bool)
	if !ok {
		return nil, util.NewClientServerError("Internal error: 'is_hnsw' is missing or has an invalid type.", http.StatusInternalServerError, nil)
	}
	if !isHnsw {
		return nil, util.NewClientServerError("Unsupported index type for recall optimization. Only HNSW index is supported.", http.StatusBadRequest, nil)
	}

	// Calculate the optimal index parameters to achieve the target recall.
	tuningResp, err := source.RunSQL(ctx, improveRecallQuery, []any{namedArgs})
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}

	tuningBytes, marshalErr := json.Marshal(tuningResp)
	if marshalErr != nil {
		return nil, util.NewClientServerError("failed to process tuning response", http.StatusInternalServerError, marshalErr)
	}

	var tuningRows []map[string]interface{}
	if unmarshalErr := json.Unmarshal(tuningBytes, &tuningRows); unmarshalErr != nil || len(tuningRows) == 0 {
		return nil, util.NewClientServerError("unexpected empty tuning response from database", http.StatusInternalServerError, unmarshalErr)
	}

	// Extract ef_search (JSON decoder defaults numbers to float64)
	efSearchVal, ok := tuningRows[0]["ef_search"].(float64)
	if !ok {
		return nil, util.NewClientServerError("Failed to calculate appropriate efSearch value", http.StatusInternalServerError, nil)
	}

	queryRecommendation := fmt.Sprintf("SET hnsw.ef_search = %d;", int(efSearchVal))
	return queryRecommendation, nil
}
