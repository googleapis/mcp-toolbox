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

package vectorassistapplyspec

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

const resourceType string = "vector-assist-apply-spec"

const applySpecQuery = `
    SELECT * FROM vector_assist.apply_spec(spec_id => @spec_id::TEXT, table_name => @table_name::TEXT,
    column_name => @column_name::TEXT, schema_name => @schema_name::TEXT);
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
		parameters.NewStringParameter("spec_id", "The unique ID of the vector specification to apply.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("table_name", "The name of the table to apply the vector specification to (in case of a single spec defined on the table).", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("column_name", "The text_column_name or vector_column_name of the spec to identify the exact spec in case there are multiple specs defined on a table.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("schema_name", "The schema name for the table.", parameters.WithStringRequired(false)),
	}

	if cfg.Description == "" {
		cfg.Description = "This tool automatically executes all the SQL recommendations associated with a specific vector specification (spec_id) or table. It runs the necessary commands in the correct sequence to provision the workload, marking each step as applied once successful. Use this tool when the user has reviewed the generated recommendations from a defined (or modified) spec and is ready to apply the changes directly to their database instance to finalize the vector search setup. This tool can be used as a follow-up action after invoking the 'define_spec' or 'modify_spec' tool."
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
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
	resp, err := source.RunSQL(ctx, applySpecQuery, []any{namedArgs})
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
