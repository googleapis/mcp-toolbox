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

package mysqllisttablestats

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

const resourceType string = "mysql-list-table-stats"

const listTableStatsStatement = `
SELECT
  t.table_schema AS 'table_schema',
  t.table_name AS 'table_name',
  ROUND((t.data_length + t.index_length) / 1024 / 1024, 2) AS 'size_MB',
  t.TABLE_ROWS AS 'row_count',
  ROUND(ts.total_latency / 1000000000000, 2) AS 'total_latency_secs',
  ts.rows_fetched AS 'rows_fetched',
  ts.rows_inserted AS 'rows_inserted',
  ts.rows_updated AS 'rows_updated',
  ts.rows_deleted AS 'rows_deleted',
  ts.io_read_requests AS 'io_reads',
  ROUND(ts.io_read_latency / 1000000000000, 2) AS 'io_read_latency',
  ts.io_write_requests AS 'io_writes',
  ROUND(ts.io_write_latency / 1000000000000, 2) AS 'io_write_latency',
  ts.io_misc_requests AS 'io_misc_requests',
  ROUND(ts.io_misc_latency / 1000000000000, 2) AS 'io_misc_latency'
FROM
  information_schema.tables AS t
INNER JOIN
  sys.x$schema_table_statistics AS ts
  ON (t.table_schema = ts.table_schema AND t.table_name = ts.table_name)
WHERE
  t.table_schema NOT IN ('sys', 'information_schema', 'mysql', 'performance_schema')
  AND (t.table_schema = COALESCE(NULLIF(?, ''), NULLIF(DATABASE(), '')) OR COALESCE(NULLIF(?, ''), NULLIF(DATABASE(), '')) IS NULL)
  AND (COALESCE(?, '') = '' OR t.table_name = ?)
ORDER BY
  CASE ?
    WHEN 'row_count' THEN row_count
    WHEN 'rows_fetched' THEN rows_fetched
    WHEN 'rows_inserted' THEN rows_inserted
    WHEN 'rows_updated' THEN rows_updated
    WHEN 'rows_deleted' THEN rows_deleted
    ELSE ts.total_latency
    END DESC
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
		parameters.NewStringParameter("table_schema", "(Optional) The database where statistics  is to be executed. Check all tables visible to the current user if not specified", parameters.WithStringDefault("")),
		parameters.NewStringParameter("table_name", "(Optional) Name of the table to be checked. Check all tables visible to the current user if not specified.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("sort_by", "(Optional) The column to sort by", parameters.WithStringDefault("")),
		parameters.NewIntParameter("limit", "(Optional) Max rows to return, default is 10", parameters.WithIntDefault(10)),
		parameters.NewStringParameter("connected_schema", "(Optional) The connected db", parameters.WithStringRequired(false)),
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
	paramsMap := params.AsMap()

	table_schema, ok := paramsMap["table_schema"].(string)
	if !ok {
		return nil, util.NewAgentError("invalid 'table_schema' parameter; expected a string", nil)
	}
	table_name, ok := paramsMap["table_name"].(string)
	if !ok {
		return nil, util.NewAgentError("invalid 'table_name' parameter; expected a string", nil)
	}
	sort_by, ok := paramsMap["sort_by"].(string)
	if !ok {
		return nil, util.NewAgentError("invalid 'sort_by' parameter; expected a string", nil)
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
		return nil, util.NewClientServerError("schema match failed", http.StatusInternalServerError, err)
	}

	// Log the query executed for debugging.
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting logger", http.StatusInternalServerError, err)
	}
	logger.DebugContext(ctx, fmt.Sprintf("executing `%s` tool query: %s", resourceType, listTableStatsStatement))
	sliceParams := []any{table_schema, table_schema, table_name, table_name, sort_by, limit}
	resp, err := source.RunSQL(ctx, listTableStatsStatement, sliceParams)
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
