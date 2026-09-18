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

package mysqllisttables

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

const resourceType string = "mysql-list-tables"

const listTablesStatement = `
    SELECT
        T.TABLE_SCHEMA AS schema_name,
        T.TABLE_NAME AS object_name,
        CASE
            WHEN @output_format = 'simple' THEN
                JSON_OBJECT('name', T.TABLE_NAME)
            ELSE
                CONVERT(
                    JSON_OBJECT(
                        'schema_name', T.TABLE_SCHEMA,
                        'object_name', T.TABLE_NAME,
                        'object_type', 'TABLE',
                        'owner', (
                            SELECT
                                IFNULL(U.GRANTEE, 'N/A')
                            FROM
                                INFORMATION_SCHEMA.SCHEMA_PRIVILEGES U
                            WHERE
                                U.TABLE_SCHEMA = T.TABLE_SCHEMA
                            LIMIT 1
                        ),
                        'comment', IFNULL(T.TABLE_COMMENT, ''),
                        'columns', (
                            SELECT
                                IFNULL(
                                    JSON_ARRAYAGG(
                                        JSON_OBJECT(
                                            'column_name', C.COLUMN_NAME,
                                            'data_type', C.COLUMN_TYPE,
                                            'ordinal_position', C.ORDINAL_POSITION,
                                            'is_not_nullable', IF(C.IS_NULLABLE = 'NO', TRUE, FALSE),
                                            'column_default', C.COLUMN_DEFAULT,
                                            'column_comment', IFNULL(C.COLUMN_COMMENT, '')
                                        )
                                    ),
                                    JSON_ARRAY()
                                )
                            FROM
                                INFORMATION_SCHEMA.COLUMNS C
                            WHERE
                                C.TABLE_SCHEMA = T.TABLE_SCHEMA AND C.TABLE_NAME = T.TABLE_NAME
                            ORDER BY C.ORDINAL_POSITION
                        ),
                        'constraints', (
                            SELECT
                                IFNULL(
                                    JSON_ARRAYAGG(
                                        JSON_OBJECT(
                                            'constraint_name', TC.CONSTRAINT_NAME,
                                            'constraint_type',
                                            CASE TC.CONSTRAINT_TYPE
                                                WHEN 'PRIMARY KEY' THEN 'PRIMARY KEY'
                                                WHEN 'FOREIGN KEY' THEN 'FOREIGN KEY'
                                                WHEN 'UNIQUE' THEN 'UNIQUE'
                                                ELSE TC.CONSTRAINT_TYPE
                                            END,
                                            'constraint_definition', '',
                                            'constraint_columns', (
                                                SELECT
                                                    IFNULL(JSON_ARRAYAGG(KCU.COLUMN_NAME), JSON_ARRAY())
                                                FROM
                                                    INFORMATION_SCHEMA.KEY_COLUMN_USAGE KCU
                                                WHERE
                                                    KCU.CONSTRAINT_SCHEMA = TC.CONSTRAINT_SCHEMA
                                                    AND KCU.CONSTRAINT_NAME = TC.CONSTRAINT_NAME
                                                    AND KCU.TABLE_NAME = TC.TABLE_NAME
                                                ORDER BY KCU.ORDINAL_POSITION
                                            ),
                                            'foreign_key_referenced_table', IF(TC.CONSTRAINT_TYPE = 'FOREIGN KEY', RC.REFERENCED_TABLE_NAME, NULL),
                                            'foreign_key_referenced_columns', IF(TC.CONSTRAINT_TYPE = 'FOREIGN KEY',
                                                (SELECT IFNULL(JSON_ARRAYAGG(FKCU.REFERENCED_COLUMN_NAME), JSON_ARRAY())
                                                FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE FKCU
                                                WHERE FKCU.CONSTRAINT_SCHEMA = TC.CONSTRAINT_SCHEMA
                                                    AND FKCU.CONSTRAINT_NAME = TC.CONSTRAINT_NAME
                                                    AND FKCU.TABLE_NAME = TC.TABLE_NAME
                                                    AND FKCU.REFERENCED_TABLE_NAME IS NOT NULL
                                                ORDER BY FKCU.ORDINAL_POSITION),
                                                NULL
                                            )
                                        )
                                    ),
                                    JSON_ARRAY()
                                )
                            FROM
                                INFORMATION_SCHEMA.TABLE_CONSTRAINTS TC
                            LEFT JOIN
                                INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS RC
                                ON TC.CONSTRAINT_SCHEMA = RC.CONSTRAINT_SCHEMA
                                AND TC.CONSTRAINT_NAME = RC.CONSTRAINT_NAME
                                AND TC.TABLE_NAME = RC.TABLE_NAME
                            WHERE
                                TC.TABLE_SCHEMA = T.TABLE_SCHEMA AND TC.TABLE_NAME = T.TABLE_NAME
                        ),
                        'indexes', (
                            SELECT
                                IFNULL(
                                    JSON_ARRAYAGG(
                                        JSON_OBJECT(
                                            'index_name', IndexData.INDEX_NAME,
                                            'is_unique', IF(IndexData.NON_UNIQUE = 0, TRUE, FALSE),
                                            'is_primary', IF(IndexData.INDEX_NAME = 'PRIMARY', TRUE, FALSE),
                                            'index_columns', IFNULL(IndexData.INDEX_COLUMNS_ARRAY, JSON_ARRAY())
                                        )
                                    ),
                                    JSON_ARRAY()
                                )
                            FROM (
                                SELECT
                                    S.TABLE_SCHEMA,
                                    S.TABLE_NAME,
                                    S.INDEX_NAME,
                                    MIN(S.NON_UNIQUE) AS NON_UNIQUE,
                                    JSON_ARRAYAGG(S.COLUMN_NAME) AS INDEX_COLUMNS_ARRAY
                                FROM
                                    INFORMATION_SCHEMA.STATISTICS S
                                GROUP BY
                                    S.TABLE_SCHEMA, S.TABLE_NAME, S.INDEX_NAME
                            ) AS IndexData
                            WHERE IndexData.TABLE_SCHEMA = T.TABLE_SCHEMA AND IndexData.TABLE_NAME = T.TABLE_NAME
                            ORDER BY IndexData.INDEX_NAME
                        ),
                        'triggers', (
                            SELECT
                                IFNULL(
                                    JSON_ARRAYAGG(
                                        JSON_OBJECT(
                                            'trigger_name', TR.TRIGGER_NAME,
                                            'trigger_definition', TR.ACTION_STATEMENT
                                        )
                                    ),
                                    JSON_ARRAY()
                                )
                            FROM
                                INFORMATION_SCHEMA.TRIGGERS TR
                            WHERE
                                TR.EVENT_OBJECT_SCHEMA = T.TABLE_SCHEMA AND TR.EVENT_OBJECT_TABLE = T.TABLE_NAME
                            ORDER BY TR.TRIGGER_NAME
                        )
                    )
                USING utf8mb4)
        END AS object_details
    FROM
        INFORMATION_SCHEMA.TABLES T
    CROSS JOIN (SELECT @table_names := ?, @output_format := ?, @connected_schema := ?) AS variables
    WHERE
        T.TABLE_SCHEMA NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')
        AND (NULLIF(TRIM(@connected_schema), '') IS NULL OR T.TABLE_SCHEMA = TRIM(@connected_schema))
        AND (NULLIF(TRIM(@table_names), '') IS NULL OR FIND_IN_SET(T.TABLE_NAME, @table_names))
        AND T.TABLE_TYPE = 'BASE TABLE'
    ORDER BY
        T.TABLE_SCHEMA, T.TABLE_NAME;
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
	MySQLPool() *sql.DB
	RunSQL(context.Context, string, []any) (any, error)
	MySQLDatabase() string
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
		parameters.NewStringParameter("table_names", "Optional: A comma-separated list of table names. If empty, details for all tables will be listed.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("output_format", "Optional: Use 'simple' for names only or 'detailed' for full info.", parameters.WithStringDefault("detailed")),
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

	tableNames, ok := paramsMap["table_names"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("invalid '%s' parameter; expected a string", tableNames), nil)
	}
	outputFormat, _ := paramsMap["output_format"].(string)
	if outputFormat != "simple" && outputFormat != "detailed" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid value for output_format: must be 'simple' or 'detailed', but got %q", outputFormat), nil)
	}
	resp, err := source.RunSQL(ctx, listTablesStatement, []any{tableNames, outputFormat, source.MySQLDatabase()})
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	// if there's no results, return empty list instead of null
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
