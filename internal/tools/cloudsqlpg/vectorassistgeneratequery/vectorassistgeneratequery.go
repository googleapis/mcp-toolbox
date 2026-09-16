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

package vectorassistgeneratequery

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

const resourceType string = "vector-assist-generate-query"

const generateQueryStatement = `
    SELECT vector_assist.generate_query(
        spec_id => @spec_id::TEXT, table_name => @table_name::TEXT,
        schema_name => @schema_name::TEXT, column_name => @column_name::TEXT,
        search_text => @search_text::TEXT, search_vector => @search_vector::vector,
        output_column_names => @output_column_names,
        top_k => @top_k::INTEGER,
        filter_expressions => @filter_expressions,
        target_recall => @target_recall::FLOAT,
        iterative_index_search => @iterative_index_search::BOOLEAN
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
	// parameters are marked required/ optional based on the vector assist function defintions
	allParameters := parameters.Parameters{
		parameters.NewStringParameter("spec_id", "Generate the vector query corresponding to this vector spec.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("table_name", "Generate the vector query corresponding to this table (in case of a single spec defined on the table).", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("schema_name", "Schema name for the table related to the vector query generation.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("column_name", "text_column_name or vector_column_name of the spec to identify the exact spec in case there are multiple specs defined on a table.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("search_text", "Text search for which query needs to be generated. Embeddings are generated using the model defined in the vector spec.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("search_vector", "Vector for which query needs to be generated. Only one of search_text or search_vector must be populated.", parameters.WithStringRequired(false)),
		parameters.NewArrayParameter("output_column_names", "Column names to retrieve in the output search query. Defaults to retrieving all columns.", parameters.NewStringParameter("output_column_name", "Output column name"), parameters.WithArrayRequired(false)),
		parameters.NewIntParameter("top_k", "Number of nearest neighbors to be returned in the vector search query. Defaults to 10.", parameters.WithIntRequired(false)),
		parameters.NewArrayParameter("filter_expressions", "Any filter expressions to be applied on the vector search query.", parameters.NewStringParameter("filter_expression", "Filter expression"), parameters.WithArrayRequired(false)),
		parameters.NewFloatParameter("target_recall", "The recall that the user would like to target with the given query. Overrides the spec-level target_recall.", parameters.WithFloatRequired(false)),
		parameters.NewBooleanParameter("iterative_index_search", "Perform iterative index search for filtered queries to ensure enough results are returned.", parameters.WithBooleanRequired(false)),
	}

	if cfg.Description == "" {
		cfg.Description = "This tool generates optimized SQL queries for vector search by leveraging the metadata and vector specifications defined in a specific spec_id. It may return a single query or a sequence of multiple SQL queries that can be executed sequentially. Use this tool when a user wants to perform semantic or similarity searches on their data. It serves as the primary actionable tool to invoke for generating the executable SQL required to retrieve relevant results based on vector similarity. The 'execute_sql' tool can be used as a follow-up action after invoking this tool."
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
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
	resp, err := source.RunSQL(ctx, generateQueryStatement, []any{namedArgs})
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
