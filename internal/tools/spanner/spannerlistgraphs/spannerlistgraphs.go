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

package spannerlistgraphs

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "spanner-list-graphs"

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
	DatabaseDialect() string
	RunSQL(context.Context, bool, string, map[string]any) (any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

// validate interface
var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	// Define parameters for the tool
	allParameters := parameters.Parameters{
		parameters.NewStringParameter(
			"graph_names",
			"Optional: A comma-separated list of graph names. If empty, details for all graphs in user-accessible schemas will be listed.", parameters.WithStringDefault(
				"")),
		parameters.NewStringParameter(
			"output_format",
			"Optional: Use 'simple' to return graph names only or use 'detailed' to return the full information schema.", parameters.WithStringDefault(
				"detailed")),
	}

	if cfg.Description == "" {
		cfg.Description = "Lists detailed graph schema information (node tables, edge tables, labels and property declarations) as JSON for user-created graphs. Filters by a comma-separated list of graph names. If names are omitted, lists all graphs. The output can be 'simple' (graph names only) or 'detailed' (full schema)."
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

// validate interface
var _ tools.Tool = Tool{}

type Tool struct {
	tools.BaseTool[Config]
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	// Check dialect here at RUNTIME instead of startup
	if strings.ToLower(source.DatabaseDialect()) != "googlesql" {
		return nil, util.NewAgentError(fmt.Sprintf("operation not supported: The 'spanner-list-graphs' tool is only available for GoogleSQL dialect databases. Your current database dialect is '%s'", source.DatabaseDialect()), nil)
	}

	paramsMap := params.AsMap()

	graphNames, _ := paramsMap["graph_names"].(string)
	outputFormat, _ := paramsMap["output_format"].(string)
	if outputFormat == "" {
		outputFormat = "detailed"
	}

	stmtParams := map[string]interface{}{
		"graph_names":   graphNames,
		"output_format": outputFormat,
	}
	resp, err := source.RunSQL(ctx, true, googleSQLStatement, stmtParams)
	if err != nil {
		return nil, util.ProcessGcpError(err)
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

// GoogleSQL statement for listing graphs
const googleSQLStatement = `
WITH FilterGraphNames AS (
  SELECT DISTINCT TRIM(name) AS GRAPH_NAME
  FROM UNNEST(IF(@graph_names = '' OR @graph_names IS NULL, ['%'], SPLIT(@graph_names, ','))) AS name
)

SELECT
	PG.PROPERTY_GRAPH_SCHEMA AS schema_name,
  PG.PROPERTY_GRAPH_NAME AS object_name,
  CASE
    WHEN @output_format = 'simple' THEN
      -- IF format is 'simple', return basic JSON
          CONCAT('{"name":"', IFNULL(REPLACE(PG.PROPERTY_GRAPH_NAME, '"', '\"'), ''), '"}')
    ELSE
      CONCAT(
        '{',
        '"schema_name":"', IFNULL(PG.PROPERTY_GRAPH_SCHEMA, ''), '",',
        '"object_name":"', IFNULL(PG.PROPERTY_GRAPH_NAME, ''), '",',
				'"catalog":"', IFNULL(JSON_VALUE(PG.PROPERTY_GRAPH_METADATA_JSON,"$.catalog"), ''), '",',
        '"node_tables":', TO_JSON_STRING(PG.PROPERTY_GRAPH_METADATA_JSON.nodeTables), ',',
				'"edge_tables":', TO_JSON_STRING(PG.PROPERTY_GRAPH_METADATA_JSON.edgeTables), ',',
				'"labels":', TO_JSON_STRING(PG.PROPERTY_GRAPH_METADATA_JSON.labels), ',',
				'"property_declarations":', TO_JSON_STRING(PG.PROPERTY_GRAPH_METADATA_JSON.propertyDeclarations),
        '}'
      )
  END AS object_details
FROM INFORMATION_SCHEMA.PROPERTY_GRAPHS PG
WHERE
	EXISTS (SELECT 1 FROM FilterGraphNames WHERE FilterGraphNames.GRAPH_NAME = '%') OR PG.PROPERTY_GRAPH_NAME IN (SELECT GRAPH_NAME FROM FilterGraphNames)
`
