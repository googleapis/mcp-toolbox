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

package mysqllistactivequeries

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/cloudsqlmysql"
	"github.com/googleapis/mcp-toolbox/internal/sources/mysql"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "mysql-list-active-queries"

const listActiveQueriesStatementMySQL = `
    SELECT
        p.id AS processlist_id,
        substring(IFNULL(p.info, t.trx_query), 1, 100) AS query,
        t.trx_started AS trx_started,
        (UNIX_TIMESTAMP(UTC_TIMESTAMP()) - UNIX_TIMESTAMP(t.trx_started)) AS trx_duration_seconds,
        (UNIX_TIMESTAMP(UTC_TIMESTAMP()) - UNIX_TIMESTAMP(t.trx_wait_started)) AS trx_wait_duration_seconds,
        p.time AS query_time,
        t.trx_state AS trx_state,
        p.state AS process_state,
        IF(p.host IS NULL OR p.host = '', p.user, concat(p.user, '@', SUBSTRING_INDEX(p.host, ':', 1))) AS user,
        t.trx_rows_locked AS trx_rows_locked,
        t.trx_rows_modified AS trx_rows_modified,
        p.db AS db
    FROM
        information_schema.processlist p
        LEFT OUTER JOIN
        information_schema.innodb_trx t
        ON p.id = t.trx_mysql_thread_id
    WHERE
        (? IS NULL OR p.time >= ?)
        AND p.id != CONNECTION_ID()
        AND Command NOT IN ('Binlog Dump', 'Binlog Dump GTID', 'Connect', 'Connect Out', 'Register Slave')
        AND User NOT IN ('system user', 'event_scheduler')
        AND (t.trx_id is NOT NULL OR command != 'Sleep')
    ORDER BY
        t.trx_started
    LIMIT ?;
`

const listActiveQueriesStatementCloudSQLMySQL = `
    SELECT
        p.id AS processlist_id,
        substring(IFNULL(p.info, t.trx_query), 1, 100) AS query,
        t.trx_started AS trx_started,
        (UNIX_TIMESTAMP(UTC_TIMESTAMP()) - UNIX_TIMESTAMP(t.trx_started)) AS trx_duration_seconds,
        (UNIX_TIMESTAMP(UTC_TIMESTAMP()) - UNIX_TIMESTAMP(t.trx_wait_started)) AS trx_wait_duration_seconds,
        p.time AS query_time,
        t.trx_state AS trx_state,
        p.state AS process_state,
        IF(p.host IS NULL OR p.host = '', p.user, concat(p.user, '@', SUBSTRING_INDEX(p.host, ':', 1))) AS user,
        t.trx_rows_locked AS trx_rows_locked,
        t.trx_rows_modified AS trx_rows_modified,
        p.db AS db
    FROM
        information_schema.processlist p
        LEFT OUTER JOIN
        information_schema.innodb_trx t
        ON p.id = t.trx_mysql_thread_id
    WHERE
        (? IS NULL OR p.time >= ?)
        AND p.id != CONNECTION_ID()
        AND SUBSTRING_INDEX(IFNULL(p.host,''), ':', 1) NOT IN ('localhost', '127.0.0.1')
        AND IFNULL(p.host,'') NOT LIKE '::1%'
        AND Command NOT IN ('Binlog Dump', 'Binlog Dump GTID', 'Connect', 'Connect Out', 'Register Slave')
        AND User NOT IN ('system user', 'event_scheduler')
        AND (t.trx_id is NOT NULL OR command != 'sleep')
    ORDER BY
        t.trx_started
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
	SourceType() string
	MySQLPoolContext(context.Context) (*sql.DB, error)
	RunSQL(context.Context, string, []any) (any, error)
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
		parameters.NewIntParameter("min_duration_secs", "Optional: Only show queries running for at least this long in seconds", parameters.WithIntDefault(0)),
		parameters.NewIntParameter("limit", "Optional: The maximum number of rows to return.", parameters.WithIntDefault(100)),
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
	var statement string
	switch source.SourceType() {
	case mysql.SourceType:
		statement = listActiveQueriesStatementMySQL
	case cloudsqlmysql.SourceType:
		statement = listActiveQueriesStatementCloudSQLMySQL
	default:
		return nil, util.NewClientServerError(fmt.Sprintf("unsupported source type: %s", source.SourceType()), http.StatusInternalServerError, nil)
	}

	paramsMap := params.AsMap()

	duration, ok := paramsMap["min_duration_secs"].(int)
	if !ok {
		return nil, util.NewAgentError("invalid 'min_duration_secs' parameter; expected an integer", nil)
	}
	limit, ok := paramsMap["limit"].(int)
	if !ok {
		return nil, util.NewAgentError("invalid 'limit' parameter; expected an integer", nil)
	}

	// Log the query executed for debugging.
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting logger", http.StatusInternalServerError, err)
	}
	logger.DebugContext(ctx, fmt.Sprintf("executing `%s` tool query: %s", resourceType, statement))
	resp, err := source.RunSQL(ctx, statement, []any{duration, duration, limit})
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
