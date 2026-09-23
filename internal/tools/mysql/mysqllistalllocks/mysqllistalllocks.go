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

package mysqllistalllocks

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

const resourceType string = "mysql-list-all-locks"

const listAllLocksStatement8plus = `
SELECT
  dl.thread_id AS thread_id,
  it.TRX_MYSQL_THREAD_ID AS process_id,
  dl.object_schema AS table_schema,
  dl.object_name AS table_name,
  dl.lock_type AS lock_type,
  dl.lock_mode AS lock_mode,
  dl.LOCK_STATUS AS lock_status,
  it.TRX_STATE AS transaction_state,
  it.TRX_OPERATION_STATE AS current_operation,
  substring(it.TRX_QUERY, 1, 100) AS query
FROM
  performance_schema.data_locks dl
INNER JOIN information_schema.innodb_trx it
  ON dl.ENGINE_TRANSACTION_ID = it.TRX_ID
WHERE
  (dl.object_schema = COALESCE(NULLIF(?, ''), NULLIF(DATABASE(), '')) OR COALESCE(NULLIF(?, ''), NULLIF(DATABASE(), '')) IS NULL)
  AND (COALESCE(?, '') = '' OR dl.object_name = ?)
ORDER BY TRX_STARTED
LIMIT ?;
`

const listAllLocksStatement57 = `
SELECT  
  th.THREAD_ID AS thread_id,
  it.TRX_MYSQL_THREAD_ID AS process_id,
  REPLACE(SUBSTRING_INDEX(il.lock_table, '.', 1), '` + "`" + `', '') AS table_schema,
  REPLACE(SUBSTRING_INDEX(il.lock_table, '.', -1), '` + "`" + `', '') AS table_name,
  il.lock_type AS lock_type, 
  il.lock_mode AS lock_mode,
  IF(w.requested_lock_id IS NOT NULL, 'WAITING', 'GRANTED') AS lock_status,
  it.TRX_STATE AS transaction_state,
  it.TRX_OPERATION_STATE AS current_operation,
  SUBSTRING(it.TRX_QUERY, 1, 100) AS query
FROM
  information_schema.innodb_locks il
INNER JOIN information_schema.innodb_trx it
  ON il.lock_trx_id = it.TRX_ID
LEFT JOIN performance_schema.threads th
  ON it.TRX_MYSQL_THREAD_ID = th.PROCESSLIST_ID
LEFT JOIN information_schema.innodb_lock_waits w
  ON il.lock_id = w.requested_lock_id
WHERE
  (REPLACE(SUBSTRING_INDEX(il.lock_table, '.', 1), '` + "`" + `', '') = COALESCE(NULLIF(?, ''), NULLIF(DATABASE(), '')) OR COALESCE(NULLIF(?, ''), NULLIF(DATABASE(), '')) IS NULL)
  AND (COALESCE(?, '') = '' OR REPLACE(SUBSTRING_INDEX(il.lock_table, '.', -1), '` + "`" + `', '') = ?)
ORDER BY it.TRX_STARTED
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
	RetrieveSourceVersion(context.Context) (string, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
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

	allParameters := parameters.Parameters{
		parameters.NewStringParameter("table_schema", "(Optional) The database where locked object is detected. Check all databases if not specified.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("table_name", "(Optional) Name of the table to be checked. Check all tables visible to the current user if not specified.", parameters.WithStringDefault("")),
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
	table_name, ok := paramsMap["table_name"].(string)
	if !ok {
		return nil, util.NewAgentError("invalid 'table_name' parameter; expected a string", nil)
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

	version, err := source.RetrieveSourceVersion(ctx)
	if err != nil {
		return nil, util.NewClientServerError("failed to get mysql version", http.StatusInternalServerError, err)
	}

	var listAllLocksStatement string
	if strings.HasPrefix(version, "5.7") {
		listAllLocksStatement = listAllLocksStatement57
	} else {
		listAllLocksStatement = listAllLocksStatement8plus
	}

	logger.DebugContext(ctx, fmt.Sprintf("executing `%s` tool query: %s", resourceType, listAllLocksStatement))
	sliceParams := []any{table_schema, table_schema, table_name, table_name, limit}
	resp, err := source.RunSQL(ctx, listAllLocksStatement, sliceParams)
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
