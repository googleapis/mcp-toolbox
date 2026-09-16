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

package vectorassistmodifyspec

import (
	"context"
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

const resourceType string = "vector-assist-modify-spec"

const modifySpecQuery = `
    SELECT recommendation_id, vector_spec_id, table_name, schema_name, query, recommendation, applied, modified, created_at
    FROM vector_assist.modify_spec(spec_id => @spec_id::TEXT, table_name => @table_name::TEXT, schema_name => @schema_name::TEXT,
        vector_column_name => @vector_column_name::TEXT, text_column_name => @text_column_name::TEXT,
        vector_index_type => @vector_index_type::TEXT, embeddings_available => @embeddings_available::BOOLEAN,
        num_vectors => @num_vectors::INTEGER, dimensionality => @dimensionality::INTEGER,
        embedding_model => @embedding_model::TEXT, prefilter_column_names => @prefilter_column_names,
        distance_func => @distance_func::TEXT, quantization => @quantization::TEXT,
        memory_budget_kb => @memory_budget_kb::INTEGER, target_recall => @target_recall::FLOAT,
        target_top_k => @target_top_k::INTEGER, tune_vector_index => @tune_vector_index::BOOLEAN);
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
	// parameters are marked required/ optional based on the vector assist function defintions
	allParameters := parameters.Parameters{
		parameters.NewStringParameter("spec_id", "Unique ID for the vector spec you want to modify.", parameters.WithStringRequired(true)),
		parameters.NewStringParameter("table_name", "Modify the table name on which vector workload needs to be set up.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("schema_name", "Modify the schema containing the given table.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("vector_column_name", "Modify the column name for the column with vector embeddings.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("text_column_name", "Modify the column name for the column with text on which vector search needs to be set up.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("vector_index_type", "Modify the type of the vector index to be created (Allowed inputs: 'hnsw', 'ivfflat', 'scann').", parameters.WithStringRequired(false)),
		parameters.NewBooleanParameter("embeddings_available", "Modify whether vector embeddings are already available in the table.", parameters.WithBooleanRequired(false)),
		parameters.NewIntParameter("num_vectors", "Modify the number of vectors expected in the dataset.", parameters.WithIntRequired(false)),
		parameters.NewIntParameter("dimensionality", "Modify the dimensionality of the vectors or embedding model.", parameters.WithIntRequired(false)),
		parameters.NewStringParameter("embedding_model", "Modify the model used for generating embeddings.", parameters.WithStringRequired(false)),
		parameters.NewArrayParameter("prefilter_column_names", "Modify the column(s) based on which prefiltering will happen in vector search queries.", parameters.NewStringParameter("prefilter_column_name", "Pre filter column name"), parameters.WithArrayRequired(false)),
		parameters.NewStringParameter("distance_func", "Modify the distance function to be used for comparing vectors (Allowed inputs: 'cosine', 'ip', 'l2', 'l1').", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("quantization", "Modify the quantization to be used for creating the vector indexes (Allowed inputs: 'none', 'halfvec', 'bit').", parameters.WithStringRequired(false)),
		parameters.NewIntParameter("memory_budget_kb", "Modify the maximum size that the index can consume in memory while building.", parameters.WithIntRequired(false)),
		parameters.NewFloatParameter("target_recall", "Modify the recall that the user would like to target with the given index.", parameters.WithFloatRequired(false)),
		parameters.NewIntParameter("target_top_k", "Modify the Top-K matching values that need to be retrieved for the given query.", parameters.WithIntRequired(false)),
		parameters.NewBooleanParameter("tune_vector_index", "Modify whether to tune vector index build and search parameters.", parameters.WithBooleanRequired(false)),
	}

	if cfg.Description == "" {
		cfg.Description = "This tool modifies an existing vector specification (identified by a required spec_id) with new parameters or overrides. Upon modification, it automatically recalculates and refreshes the list of generated SQL recommendations to match the updated requirements. This tool provides a way to modify column(s) in the vector spec before applying and taking action on the recommendations. While highly customizable, any optional parameters left unspecified will use internally determined defaults optimized for the specific workload. Use this tool to modify configurations established via 'define_spec' tool such as adjusting target recall, embedding models, or quantization settings, etc."
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewWriteAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
		),
	}, nil
}

var _ tools.Tool = Tool{}

type Tool struct {
	tools.BaseTool[Config]
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramsMap := params.AsMap()

	// Convert our parsed parameters directly into pgx.NamedArgs
	namedArgs := pgx.NamedArgs{}
	for key, value := range paramsMap {
		namedArgs[key] = value
	}

	// As long as source.RunSQL unwraps args into pgx.Query(ctx, sql, args...), pgx handles the mapping of @param to the named parameter.
	resp, err := source.RunSQL(ctx, modifySpecQuery, []any{namedArgs})
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	return resp, nil
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
