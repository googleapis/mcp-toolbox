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

package mssqllistindexes

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "mssql-list-indexes"

const listIndexesStatement = `
    WITH IndexDetails AS (
        SELECT
            s.name AS schema_name,
            t.name AS table_name,
            i.name AS index_name,
            i.type_desc AS index_type,
            i.is_unique,
            i.is_primary_key AS is_primary,
            i.is_disabled,
            i.filter_definition,
            STUFF((SELECT ', ' + c.name + CASE WHEN ic.is_descending_key = 1 THEN ' DESC' ELSE ' ASC' END
                   FROM sys.index_columns ic
                   JOIN sys.columns c ON ic.object_id = c.object_id AND ic.column_id = c.column_id
                   WHERE ic.object_id = i.object_id AND ic.index_id = i.index_id AND ic.is_included_column = 0
                   ORDER BY ic.key_ordinal
                   FOR XML PATH(''), TYPE).value('.', 'NVARCHAR(MAX)'), 1, 2, '') AS key_columns,
            STUFF((SELECT ', ' + c.name
                   FROM sys.index_columns ic
                   JOIN sys.columns c ON ic.object_id = c.object_id AND ic.column_id = c.column_id
                   WHERE ic.object_id = i.object_id AND ic.index_id = i.index_id AND ic.is_included_column = 1
                   ORDER BY ic.index_column_id
                   FOR XML PATH(''), TYPE).value('.', 'NVARCHAR(MAX)'), 1, 2, '') AS included_columns,
            ISNULL(us.user_seeks, 0) + ISNULL(us.user_scans, 0) + ISNULL(us.user_lookups, 0) AS user_reads,
            ISNULL(us.user_updates, 0) AS user_updates,
            us.last_user_seek,
            us.last_user_scan,
            CASE
                WHEN (ISNULL(us.user_seeks, 0) + ISNULL(us.user_scans, 0) + ISNULL(us.user_lookups, 0)) > 0
                THEN CAST(1 AS BIT) ELSE CAST(0 AS BIT)
            END AS is_used
        FROM sys.indexes i
        JOIN sys.tables t ON i.object_id = t.object_id
        JOIN sys.schemas s ON t.schema_id = s.schema_id
        LEFT JOIN sys.dm_db_index_usage_stats us
            ON us.object_id = i.object_id AND us.index_id = i.index_id AND us.database_id = DB_ID()
        WHERE
            t.type = 'U'          -- user tables only
            AND i.type <> 0       -- exclude heaps
            AND i.name IS NOT NULL
            AND s.name NOT IN ('sys', 'INFORMATION_SCHEMA')
    )
    SELECT *
    FROM IndexDetails
    WHERE
        (@schema_name IS NULL OR @schema_name = '' OR schema_name LIKE '%' + @schema_name + '%')
        AND (@table_name IS NULL OR @table_name = '' OR table_name LIKE '%' + @table_name + '%')
        AND (@index_name IS NULL OR @index_name = '' OR index_name LIKE '%' + @index_name + '%')
        AND (@only_unused = 0 OR is_used = 0)
    ORDER BY schema_name, table_name, index_name
    OFFSET 0 ROWS FETCH NEXT @limit ROWS ONLY;
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
	MSSQLDB() *sql.DB
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
		parameters.NewStringParameter("schema_name", "Optional: a text to filter results by schema name. The input is used within a LIKE clause.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("table_name", "Optional: a text to filter results by table name. The input is used within a LIKE clause.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("index_name", "Optional: a text to filter results by index name. The input is used within a LIKE clause.", parameters.WithStringDefault("")),
		parameters.NewBooleanParameter("only_unused", "Optional: If true, only returns indexes with no recorded reads since the last SQL Server restart.", parameters.WithBooleanDefault(false)),
		parameters.NewIntParameter("limit", "Optional: The maximum number of rows to return. Default is 50.", parameters.WithIntDefault(50)),
	}

	if cfg.Description == "" {
		cfg.Description = "Lists user indexes in a SQL Server database, excluding system schemas. For each index returns: schema name, table name, index name, index type (e.g. CLUSTERED/NONCLUSTERED), whether it is unique, whether it backs a primary key, whether it is disabled, the filter definition, the key columns, the included columns, the number of user reads (seeks + scans + lookups) and user updates recorded by sys.dm_db_index_usage_stats since the last restart, and a boolean indicating whether the index has been used at least once. Index usage statistics reset on SQL Server restart."
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
	namedArgs := []any{
		sql.Named("schema_name", paramsMap["schema_name"]),
		sql.Named("table_name", paramsMap["table_name"]),
		sql.Named("index_name", paramsMap["index_name"]),
		sql.Named("only_unused", paramsMap["only_unused"]),
		sql.Named("limit", paramsMap["limit"]),
	}

	resp, err := source.RunSQL(ctx, listIndexesStatement, namedArgs)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}

	// Return an empty list instead of null when there are no rows.
	resSlice, ok := resp.([]any)
	if !ok || len(resSlice) == 0 {
		return []any{}, nil
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
