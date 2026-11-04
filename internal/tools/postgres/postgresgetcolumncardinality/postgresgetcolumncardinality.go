// Copyright 2025 Google LLC
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

package postgresgetcolumncardinality

import (
	"context"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/jackc/pgx/v5/pgxpool"
)

const resourceType string = "postgres-get-column-cardinality"

const getColumnCardinality = `
    SELECT
          s.attname AS column_name,
          ROUND(
              CASE
                  WHEN s.n_distinct < 0 THEN ABS(s.n_distinct) * c.reltuples
                  ELSE s.n_distinct
              END
          ) AS estimated_cardinality
      FROM pg_stats s
      JOIN pg_class c ON s.tablename = c.relname
      JOIN pg_namespace n ON c.relnamespace = n.oid
      WHERE s.schemaname = $1 -- Parameter for schema
        AND s.tablename = $2 -- Parameter for table
        AND n.nspname = $1  -- (same parameter as schema)
        AND s.attname = COALESCE($3, s.attname)
      ORDER BY estimated_cardinality DESC;
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
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	allParameters := parameters.Parameters{
		parameters.NewStringParameter("schema_name", "Optional: The schema name in which the table is present.", parameters.WithStringDefault("public")),
		parameters.NewStringParameter("table_name", "Required: The table name in which the column is present.", parameters.WithStringRequired(true)),
		parameters.NewStringParameter("column_name", "Optional: The column name for which the cardinality is to be found. If not provided, cardinality for all columns will be returned.", parameters.WithStringRequired(false)),
	}
	paramManifest := allParameters.Manifest()

	if cfg.Description == "" {
		cfg.Description = "Estimates the number of unique values (cardinality) quickly for one or all columns in a specific PostgreSQL table by using the database's internal statistics, returning the results in descending order of estimated cardinality. Please run ANALYZE on the table before using this tool to get accurate results. The tool returns the column_name and the estimated_cardinality. If the column_name is not provided, the tool returns all columns along with their estimated cardinality."
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

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramsMap := params.AsMap()

	newParams, err := parameters.GetParams(t.StaticParameters, paramsMap)
	if err != nil {
		return nil, util.NewAgentError("unable to extract standard params", err)
	}
	sliceParams := newParams.AsSlice()
	resp, err := source.RunSQL(ctx, getColumnCardinality, sliceParams)
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
