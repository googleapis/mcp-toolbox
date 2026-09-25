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

package mysqlshowquerystats

import (
	"context"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "mysql-show-query-stats"

const showQueryStatsStatement = `
SELECT 
    schema_name AS 'db',
    digest_text AS 'query',
    count_star AS 'execution_count',
    ROUND(sum_timer_wait / 1000000000, 2) AS 'total_latency_ms',
    ROUND(avg_timer_wait / 1000000000, 2) AS 'average_latency_ms',
    ROUND(max_timer_wait / 1000000000, 2) AS 'max_latency_ms',
    sum_rows_sent AS 'total_rows_sent',
    sum_rows_examined AS 'total_rows_examined',
    sum_no_index_used AS 'full_table_scan_count',
    sum_no_good_index_used AS 'inefficient_index_used_count',
    last_seen AS 'last_executed'
FROM performance_schema.events_statements_summary_by_digest
WHERE schema_name NOT IN ('information_schema', 'performance_schema', 'mysql', 'sys')
AND (schema_name = COALESCE(NULLIF(?, ''), NULLIF(DATABASE(), '')) OR COALESCE(NULLIF(?, ''), NULLIF(DATABASE(), '')) IS NULL)
ORDER BY sum_timer_wait DESC
LIMIT ?;
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
	RunSQL(context.Context, string, []any) (any, error)
	MySQLDatabase() string
	PerformanceSchemaEnabled(context.Context) (bool, error)
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
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	allParameters := parameters.Parameters{
		parameters.NewStringParameter("table_schema", "(Optional) The database where query statistics is to be executed. Check all queries visible to the current user if not specified", parameters.WithStringDefault("")),
		parameters.NewIntParameter("limit", "(Optional) Max rows to return, default is 10", parameters.WithIntDefault(10)),
		parameters.NewStringParameter("connected_schema", "(Optional) The database user is connected to, the value is set from env variable CLOUD_SQL_MYSQL_DATABASE or MYSQL_DATABASE", parameters.WithStringRequired(false)),
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
	// Check performance schema
	enabled, err := source.PerformanceSchemaEnabled(ctx)
	if err != nil {
		return nil, util.NewClientServerError("failed to check performance_schema", http.StatusInternalServerError, err)
	}
	if !enabled {
		return nil, util.NewClientServerError("enable performance_schema to run this tool", http.StatusInternalServerError, nil)
	}

	paramsMap := params.AsMap()

	table_schema, ok := paramsMap["table_schema"].(string)
	if !ok {
		return nil, util.NewAgentError("invalid 'table_schema' parameter; expected a string", nil)
	}
	limit, ok := paramsMap["limit"].(int)
	if !ok {
		return nil, util.NewAgentError("invalid 'limit' parameter; expected an integer", nil)
	}
	// Validate connected schema is either skipped or same as queried schema
	connected_schema, _ := paramsMap["connected_schema"].(string)
	if connected_schema == "" {
		connected_schema = source.MySQLDatabase()
	}
	if table_schema != connected_schema && connected_schema != "" && table_schema != "" {
		err := fmt.Errorf("error: connected schema '%s' does not match queried schema '%s'", connected_schema, table_schema)
		return nil, util.NewAgentError("SCHEMA_MATCH_FAILED", err)
	}

	// Log the query executed for debugging.
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting logger", http.StatusInternalServerError, err)
	}
	logger.DebugContext(ctx, fmt.Sprintf("executing `%s` tool query: %s", resourceType, showQueryStatsStatement))
	sliceParams := []any{table_schema, table_schema, limit}
	resp, err := source.RunSQL(ctx, showQueryStatsStatement, sliceParams)
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
