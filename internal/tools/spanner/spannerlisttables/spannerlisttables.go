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

package spannerlisttables

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"cloud.google.com/go/spanner"
	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "spanner-list-tables"

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
	SpannerClient() *spanner.Client
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
			"table_names",
			"Optional: A comma-separated list of table names. If empty, details for all tables in user-accessible schemas will be listed.", parameters.WithStringDefault("")),
		parameters.NewStringParameter(
			"output_format",
			"Optional: Use 'simple' to return table names only or use 'detailed' to return the full information schema.", parameters.WithStringDefault("detailed")),
	}

	if cfg.Description == "" {
		cfg.Description = "Lists detailed schema information (object type, columns, constraints, indexes) as JSON for user-created tables (ordinary or partitioned). Filters by a comma-separated list of names. If names are omitted, lists all tables in user schemas. The output can be 'simple' (table names only) or 'detailed' (full schema)."
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

func getStatement(dialect string) string {
	switch strings.ToLower(dialect) {
	case "postgresql":
		return postgresqlStatement
	case "googlesql":
		return googleSQLStatement
	default:
		// Default to GoogleSQL
		return googleSQLStatement
	}
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramsMap := params.AsMap()

	// Get the appropriate SQL statement based on dialect
	statement := getStatement(source.DatabaseDialect())

	// Prepare parameters based on dialect
	var stmtParams map[string]interface{}

	tableNames, ok := paramsMap["table_names"].(string)
	if !ok {
		return nil, util.NewAgentError("unable to get cast table_names", nil)
	}
	outputFormat, ok := paramsMap["output_format"].(string)
	if !ok {
		return nil, util.NewAgentError("unable to get cast output_format", nil)
	}
	if outputFormat == "" {
		outputFormat = "detailed"
	}

	switch strings.ToLower(source.DatabaseDialect()) {
	case "postgresql":
		// PostgreSQL uses positional parameters ($1, $2)
		stmtParams = map[string]interface{}{
			"p1": tableNames,
			"p2": outputFormat,
		}
	case "googlesql":
		// GoogleSQL uses named parameters (@table_names, @output_format)
		stmtParams = map[string]interface{}{
			"table_names":   tableNames,
			"output_format": outputFormat,
		}
	default:
		return nil, util.NewAgentError(fmt.Sprintf("unsupported dialect: %s", source.DatabaseDialect()), nil)
	}

	resp, err := source.RunSQL(ctx, true, statement, stmtParams)
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

// PostgreSQL statement for listing tables
const postgresqlStatement = `
WITH table_info_cte AS (
    SELECT
      T.TABLE_SCHEMA,
      T.TABLE_NAME,
      T.TABLE_TYPE,
      T.PARENT_TABLE_NAME,
      T.ON_DELETE_ACTION
    FROM INFORMATION_SCHEMA.TABLES AS T
    WHERE
      T.TABLE_SCHEMA = 'public'
      AND T.TABLE_TYPE = 'BASE TABLE'
      AND (
      NULLIF(TRIM($1), '') IS NULL OR
      T.TABLE_NAME IN (
        SELECT table_name
        FROM UNNEST(regexp_split_to_array($1, '\s*,\s*')) AS table_name)
      )
  ),

  columns_info_cte AS (
    SELECT
      C.TABLE_SCHEMA,
      C.TABLE_NAME,
      ARRAY_AGG(
        CONCAT(
          '{',
          '"column_name":"', COALESCE(REPLACE(C.COLUMN_NAME, '"', '\"'), ''), '",',
          '"data_type":"', COALESCE(REPLACE(C.SPANNER_TYPE, '"', '\"'), ''), '",',
          '"ordinal_position":', C.ORDINAL_POSITION::TEXT, ',',
          '"is_not_nullable":', CASE WHEN C.IS_NULLABLE = 'NO' THEN 'true' ELSE 'false' END, ',',
          '"column_default":', CASE WHEN C.COLUMN_DEFAULT IS NULL THEN 'null' ELSE CONCAT('"', REPLACE(C.COLUMN_DEFAULT::text, '"', '\"'), '"') END,
          '}'
        ) ORDER BY C.ORDINAL_POSITION
      ) AS columns_json_array_elements
    FROM INFORMATION_SCHEMA.COLUMNS AS C
    WHERE C.TABLE_SCHEMA = 'public'
      AND EXISTS (SELECT 1 FROM table_info_cte TI WHERE C.TABLE_SCHEMA = TI.TABLE_SCHEMA AND C.TABLE_NAME = TI.TABLE_NAME)
    GROUP BY C.TABLE_SCHEMA, C.TABLE_NAME
  ),

  constraint_columns_agg_cte AS (
    SELECT
      CONSTRAINT_CATALOG,
      CONSTRAINT_SCHEMA,
      CONSTRAINT_NAME,
      ARRAY_AGG(REPLACE(COLUMN_NAME, '"', '\"') ORDER BY ORDINAL_POSITION) AS column_names_json_list
    FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
    WHERE CONSTRAINT_SCHEMA = 'public'
    GROUP BY CONSTRAINT_CATALOG, CONSTRAINT_SCHEMA, CONSTRAINT_NAME
  ),

  constraints_info_cte AS (
    SELECT
      TC.TABLE_SCHEMA,
      TC.TABLE_NAME,
      ARRAY_AGG(
        CONCAT(
          '{',
          '"constraint_name":"', COALESCE(REPLACE(TC.CONSTRAINT_NAME, '"', '\"'), ''), '",',
          '"constraint_type":"', COALESCE(REPLACE(TC.CONSTRAINT_TYPE, '"', '\"'), ''), '",',
          '"constraint_definition":',
            CASE TC.CONSTRAINT_TYPE
              WHEN 'CHECK' THEN CASE WHEN CC.CHECK_CLAUSE IS NULL THEN 'null' ELSE CONCAT('"', REPLACE(CC.CHECK_CLAUSE, '"', '\"'), '"') END
              WHEN 'PRIMARY KEY' THEN CONCAT('"', 'PRIMARY KEY (', array_to_string(COALESCE(KeyCols.column_names_json_list, ARRAY[]::text[]), ', '), ')', '"')
              WHEN 'UNIQUE' THEN CONCAT('"', 'UNIQUE (', array_to_string(COALESCE(KeyCols.column_names_json_list, ARRAY[]::text[]), ', '), ')', '"')
              WHEN 'FOREIGN KEY' THEN CONCAT('"', 'FOREIGN KEY (', array_to_string(COALESCE(KeyCols.column_names_json_list, ARRAY[]::text[]), ', '), ') REFERENCES ',
                                      COALESCE(REPLACE(RefKeyTable.TABLE_NAME, '"', '\"'), ''),
                                      ' (', array_to_string(COALESCE(RefKeyCols.column_names_json_list, ARRAY[]::text[]), ', '), ')', '"')
              ELSE 'null'
            END, ',',
          '"constraint_columns":["', array_to_string(COALESCE(KeyCols.column_names_json_list, ARRAY[]::text[]), ','), '"],',
          '"foreign_key_referenced_table":', CASE WHEN RefKeyTable.TABLE_NAME IS NULL THEN 'null' ELSE CONCAT('"', REPLACE(RefKeyTable.TABLE_NAME, '"', '\"'), '"') END, ',',
          '"foreign_key_referenced_columns":["', array_to_string(COALESCE(RefKeyCols.column_names_json_list, ARRAY[]::text[]), ','), '"]',
          '}'
        ) ORDER BY TC.CONSTRAINT_NAME
      ) AS constraints_json_array_elements
    FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS AS TC
    LEFT JOIN INFORMATION_SCHEMA.CHECK_CONSTRAINTS AS CC
      ON TC.CONSTRAINT_CATALOG = CC.CONSTRAINT_CATALOG AND TC.CONSTRAINT_SCHEMA = CC.CONSTRAINT_SCHEMA AND TC.CONSTRAINT_NAME = CC.CONSTRAINT_NAME
    LEFT JOIN INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS AS RC
      ON TC.CONSTRAINT_CATALOG = RC.CONSTRAINT_CATALOG AND TC.CONSTRAINT_SCHEMA = RC.CONSTRAINT_SCHEMA AND TC.CONSTRAINT_NAME = RC.CONSTRAINT_NAME
    LEFT JOIN INFORMATION_SCHEMA.TABLE_CONSTRAINTS AS RefConstraint
      ON RC.UNIQUE_CONSTRAINT_CATALOG = RefConstraint.CONSTRAINT_CATALOG AND RC.UNIQUE_CONSTRAINT_SCHEMA = RefConstraint.CONSTRAINT_SCHEMA AND RC.UNIQUE_CONSTRAINT_NAME = RefConstraint.CONSTRAINT_NAME
    LEFT JOIN INFORMATION_SCHEMA.TABLES AS RefKeyTable
      ON RefConstraint.TABLE_CATALOG = RefKeyTable.TABLE_CATALOG AND RefConstraint.TABLE_SCHEMA = RefKeyTable.TABLE_SCHEMA AND RefConstraint.TABLE_NAME = RefKeyTable.TABLE_NAME
    LEFT JOIN constraint_columns_agg_cte AS KeyCols
      ON TC.CONSTRAINT_CATALOG = KeyCols.CONSTRAINT_CATALOG AND TC.CONSTRAINT_SCHEMA = KeyCols.CONSTRAINT_SCHEMA AND TC.CONSTRAINT_NAME = KeyCols.CONSTRAINT_NAME
    LEFT JOIN constraint_columns_agg_cte AS RefKeyCols
      ON RC.UNIQUE_CONSTRAINT_CATALOG = RefKeyCols.CONSTRAINT_CATALOG AND RC.UNIQUE_CONSTRAINT_SCHEMA = RefKeyCols.CONSTRAINT_SCHEMA AND RC.UNIQUE_CONSTRAINT_NAME = RefKeyCols.CONSTRAINT_NAME AND TC.CONSTRAINT_TYPE = 'FOREIGN KEY'
    WHERE TC.TABLE_SCHEMA = 'public'
      AND EXISTS (SELECT 1 FROM table_info_cte TI WHERE TC.TABLE_SCHEMA = TI.TABLE_SCHEMA AND TC.TABLE_NAME = TI.TABLE_NAME)
    GROUP BY TC.TABLE_SCHEMA, TC.TABLE_NAME
  ),

  index_key_columns_agg_cte AS (
    SELECT
      TABLE_CATALOG,
      TABLE_SCHEMA,
      TABLE_NAME,
      INDEX_NAME,
      ARRAY_AGG(
        CONCAT(
          '{"column_name":"', COALESCE(REPLACE(COLUMN_NAME, '"', '\"'), ''), '",',
          '"ordering":"', COALESCE(REPLACE(COLUMN_ORDERING, '"', '\"'), ''), '"}'
        ) ORDER BY ORDINAL_POSITION
      ) AS key_column_json_details
    FROM INFORMATION_SCHEMA.INDEX_COLUMNS
    WHERE ORDINAL_POSITION IS NOT NULL
      AND TABLE_SCHEMA = 'public'
    GROUP BY TABLE_CATALOG, TABLE_SCHEMA, TABLE_NAME, INDEX_NAME
  ),

  index_storing_columns_agg_cte AS (
    SELECT
      TABLE_CATALOG,
      TABLE_SCHEMA,
      TABLE_NAME,
      INDEX_NAME,
      ARRAY_AGG(CONCAT('"', REPLACE(COLUMN_NAME, '"', '\"'), '"') ORDER BY COLUMN_NAME) AS storing_column_json_names
    FROM INFORMATION_SCHEMA.INDEX_COLUMNS
    WHERE ORDINAL_POSITION IS NULL
      AND TABLE_SCHEMA = 'public'
    GROUP BY TABLE_CATALOG, TABLE_SCHEMA, TABLE_NAME, INDEX_NAME
  ),

  indexes_info_cte AS (
    SELECT
      I.TABLE_SCHEMA,
      I.TABLE_NAME,
      ARRAY_AGG(
        CONCAT(
          '{',
          '"index_name":"', COALESCE(REPLACE(I.INDEX_NAME, '"', '\"'), ''), '",',
          '"index_type":"', COALESCE(REPLACE(I.INDEX_TYPE, '"', '\"'), ''), '",',
          '"is_unique":', CASE WHEN I.IS_UNIQUE = 'YES' THEN 'true' ELSE 'false' END, ',',
          '"is_null_filtered":', CASE WHEN I.IS_NULL_FILTERED = 'YES' THEN 'true' ELSE 'false' END, ',',
          '"interleaved_in_table":', CASE WHEN I.PARENT_TABLE_NAME IS NULL OR I.PARENT_TABLE_NAME = '' THEN 'null' ELSE CONCAT('"', REPLACE(I.PARENT_TABLE_NAME, '"', '\"'), '"') END, ',',
          '"index_key_columns":[', COALESCE(array_to_string(KeyIndexCols.key_column_json_details, ','), ''), '],',
          '"storing_columns":[', COALESCE(array_to_string(StoringIndexCols.storing_column_json_names, ','), ''), ']',
          '}'
        ) ORDER BY I.INDEX_NAME
      ) AS indexes_json_array_elements
    FROM INFORMATION_SCHEMA.INDEXES AS I
    LEFT JOIN index_key_columns_agg_cte AS KeyIndexCols
      ON I.TABLE_CATALOG = KeyIndexCols.TABLE_CATALOG AND I.TABLE_SCHEMA = KeyIndexCols.TABLE_SCHEMA AND I.TABLE_NAME = KeyIndexCols.TABLE_NAME AND I.INDEX_NAME = KeyIndexCols.INDEX_NAME
    LEFT JOIN index_storing_columns_agg_cte AS StoringIndexCols
      ON I.TABLE_CATALOG = StoringIndexCols.TABLE_CATALOG AND I.TABLE_SCHEMA = StoringIndexCols.TABLE_SCHEMA AND I.TABLE_NAME = StoringIndexCols.TABLE_NAME AND I.INDEX_NAME = StoringIndexCols.INDEX_NAME
    AND I.INDEX_TYPE IN ('LOCAL', 'GLOBAL')
    WHERE I.TABLE_SCHEMA = 'public'
      AND EXISTS (SELECT 1 FROM table_info_cte TI WHERE I.TABLE_SCHEMA = TI.TABLE_SCHEMA AND I.TABLE_NAME = TI.TABLE_NAME)
    GROUP BY I.TABLE_SCHEMA, I.TABLE_NAME
  )

SELECT
  TI.TABLE_SCHEMA AS schema_name,
  TI.TABLE_NAME AS object_name,
  CASE
    WHEN $2 = 'simple' THEN
      -- IF format is 'simple', return basic JSON
          CONCAT('{"name":"', COALESCE(REPLACE(TI.TABLE_NAME, '"', '\"'), ''), '"}')
    ELSE
      CONCAT(
        '{',
        '"schema_name":"', COALESCE(REPLACE(TI.TABLE_SCHEMA, '"', '\"'), ''), '",',
        '"object_name":"', COALESCE(REPLACE(TI.TABLE_NAME, '"', '\"'), ''), '",',
        '"object_type":"', COALESCE(REPLACE(TI.TABLE_TYPE, '"', '\"'), ''), '",',
        '"columns":[', COALESCE(array_to_string(CI.columns_json_array_elements, ','), ''), '],',
        '"constraints":[', COALESCE(array_to_string(CONSI.constraints_json_array_elements, ','), ''), '],',
        '"indexes":[', COALESCE(array_to_string(II.indexes_json_array_elements, ','), ''), ']',
        '}'
      )
  END AS object_details
FROM table_info_cte AS TI
LEFT JOIN columns_info_cte AS CI
  ON TI.TABLE_SCHEMA = CI.TABLE_SCHEMA AND TI.TABLE_NAME = CI.TABLE_NAME
LEFT JOIN constraints_info_cte AS CONSI
  ON TI.TABLE_SCHEMA = CONSI.TABLE_SCHEMA AND TI.TABLE_NAME = CONSI.TABLE_NAME
LEFT JOIN indexes_info_cte AS II
  ON TI.TABLE_SCHEMA = II.TABLE_SCHEMA AND TI.TABLE_NAME = II.TABLE_NAME
ORDER BY TI.TABLE_SCHEMA, TI.TABLE_NAME`

// GoogleSQL statement for listing tables
const googleSQLStatement = `
WITH FilterTableNames AS (
  SELECT DISTINCT TRIM(name) AS TABLE_NAME
  FROM UNNEST(IF(@table_names = '' OR @table_names IS NULL, ['%'], SPLIT(@table_names, ','))) AS name
),

-- 1. Table Information
table_info_cte AS (
  SELECT
    T.TABLE_SCHEMA,
    T.TABLE_NAME,
    T.TABLE_TYPE,
    T.PARENT_TABLE_NAME, -- For interleaved tables
    T.ON_DELETE_ACTION -- For interleaved tables
  FROM INFORMATION_SCHEMA.TABLES AS T
  WHERE
    T.TABLE_SCHEMA = ''
    AND T.TABLE_TYPE = 'BASE TABLE'
    AND (EXISTS (SELECT 1 FROM FilterTableNames WHERE FilterTableNames.TABLE_NAME = '%') OR T.TABLE_NAME IN (SELECT TABLE_NAME FROM FilterTableNames))
),

-- 2. Column Information (with JSON string for each column)
columns_info_cte AS (
  SELECT
    C.TABLE_SCHEMA,
    C.TABLE_NAME,
    ARRAY_AGG(
      CONCAT(
        '{',
        '"column_name":"', IFNULL(C.COLUMN_NAME, ''), '",',
        '"data_type":"', IFNULL(C.SPANNER_TYPE, ''), '",',
        '"ordinal_position":', CAST(C.ORDINAL_POSITION AS STRING), ',',
        '"is_not_nullable":', IF(C.IS_NULLABLE = 'NO', 'true', 'false'), ',',
        '"column_default":', IF(C.COLUMN_DEFAULT IS NULL, 'null', CONCAT('"', C.COLUMN_DEFAULT, '"')),
        '}'
      ) ORDER BY C.ORDINAL_POSITION
    ) AS columns_json_array_elements
  FROM INFORMATION_SCHEMA.COLUMNS AS C
  WHERE EXISTS (SELECT 1 FROM table_info_cte TI WHERE C.TABLE_SCHEMA = TI.TABLE_SCHEMA AND C.TABLE_NAME = TI.TABLE_NAME)
  GROUP BY C.TABLE_SCHEMA, C.TABLE_NAME
),

-- Helper CTE for aggregating constraint columns
constraint_columns_agg_cte AS (
  SELECT
    CONSTRAINT_CATALOG,
    CONSTRAINT_SCHEMA,
    CONSTRAINT_NAME,
    ARRAY_AGG(REPLACE(COLUMN_NAME, '"', '\"') ORDER BY ORDINAL_POSITION) AS column_names_json_list
  FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
  GROUP BY CONSTRAINT_CATALOG, CONSTRAINT_SCHEMA, CONSTRAINT_NAME
),

-- 3. Constraint Information (with JSON string for each constraint)
constraints_info_cte AS (
  SELECT
    TC.TABLE_SCHEMA,
    TC.TABLE_NAME,
    ARRAY_AGG(
      CONCAT(
        '{',
        '"constraint_name":"', IFNULL(TC.CONSTRAINT_NAME, ''), '",',
        '"constraint_type":"', IFNULL(TC.CONSTRAINT_TYPE, ''), '",',
        '"constraint_definition":',
          CASE TC.CONSTRAINT_TYPE
            WHEN 'CHECK' THEN IF(CC.CHECK_CLAUSE IS NULL, 'null', CONCAT('"', CC.CHECK_CLAUSE, '"'))
            WHEN 'PRIMARY KEY' THEN CONCAT('"', 'PRIMARY KEY (', ARRAY_TO_STRING(COALESCE(KeyCols.column_names_json_list, []), ', '), ')', '"')
            WHEN 'UNIQUE' THEN CONCAT('"', 'UNIQUE (', ARRAY_TO_STRING(COALESCE(KeyCols.column_names_json_list, []), ', '), ')', '"')
            WHEN 'FOREIGN KEY' THEN CONCAT('"', 'FOREIGN KEY (', ARRAY_TO_STRING(COALESCE(KeyCols.column_names_json_list, []), ', '), ') REFERENCES ',
                                    IFNULL(RefKeyTable.TABLE_NAME, ''),
                                    ' (', ARRAY_TO_STRING(COALESCE(RefKeyCols.column_names_json_list, []), ', '), ')', '"')
            ELSE 'null'
          END, ',',
        '"constraint_columns":["', ARRAY_TO_STRING(COALESCE(KeyCols.column_names_json_list, []), ','), '"],',
        '"foreign_key_referenced_table":', IF(RefKeyTable.TABLE_NAME IS NULL, 'null', CONCAT('"', RefKeyTable.TABLE_NAME, '"')), ',',
        '"foreign_key_referenced_columns":["', ARRAY_TO_STRING(COALESCE(RefKeyCols.column_names_json_list, []), ','), '"]',
        '}'
      ) ORDER BY TC.CONSTRAINT_NAME
    ) AS constraints_json_array_elements
  FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS AS TC
  LEFT JOIN INFORMATION_SCHEMA.CHECK_CONSTRAINTS AS CC
    ON TC.CONSTRAINT_CATALOG = CC.CONSTRAINT_CATALOG AND TC.CONSTRAINT_SCHEMA = CC.CONSTRAINT_SCHEMA AND TC.CONSTRAINT_NAME = CC.CONSTRAINT_NAME
  LEFT JOIN INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS AS RC
    ON TC.CONSTRAINT_CATALOG = RC.CONSTRAINT_CATALOG AND TC.CONSTRAINT_SCHEMA = RC.CONSTRAINT_SCHEMA AND TC.CONSTRAINT_NAME = RC.CONSTRAINT_NAME
  LEFT JOIN INFORMATION_SCHEMA.TABLE_CONSTRAINTS AS RefConstraint
    ON RC.UNIQUE_CONSTRAINT_CATALOG = RefConstraint.CONSTRAINT_CATALOG AND RC.UNIQUE_CONSTRAINT_SCHEMA = RefConstraint.CONSTRAINT_SCHEMA AND RC.UNIQUE_CONSTRAINT_NAME = RefConstraint.CONSTRAINT_NAME
  LEFT JOIN INFORMATION_SCHEMA.TABLES AS RefKeyTable
    ON RefConstraint.TABLE_CATALOG = RefKeyTable.TABLE_CATALOG AND RefConstraint.TABLE_SCHEMA = RefKeyTable.TABLE_SCHEMA AND RefConstraint.TABLE_NAME = RefKeyTable.TABLE_NAME
  LEFT JOIN constraint_columns_agg_cte AS KeyCols
    ON TC.CONSTRAINT_CATALOG = KeyCols.CONSTRAINT_CATALOG AND TC.CONSTRAINT_SCHEMA = KeyCols.CONSTRAINT_SCHEMA AND TC.CONSTRAINT_NAME = KeyCols.CONSTRAINT_NAME
  LEFT JOIN constraint_columns_agg_cte AS RefKeyCols
    ON RC.UNIQUE_CONSTRAINT_CATALOG = RefKeyCols.CONSTRAINT_CATALOG AND RC.UNIQUE_CONSTRAINT_SCHEMA = RefKeyCols.CONSTRAINT_SCHEMA AND RC.UNIQUE_CONSTRAINT_NAME = RefKeyCols.CONSTRAINT_NAME AND TC.CONSTRAINT_TYPE = 'FOREIGN KEY'
  WHERE EXISTS (SELECT 1 FROM table_info_cte TI WHERE TC.TABLE_SCHEMA = TI.TABLE_SCHEMA AND TC.TABLE_NAME = TI.TABLE_NAME)
  GROUP BY TC.TABLE_SCHEMA, TC.TABLE_NAME
),

-- Helper CTE for aggregating index key columns (as JSON strings)
index_key_columns_agg_cte AS (
  SELECT
    TABLE_CATALOG,
    TABLE_SCHEMA,
    TABLE_NAME,
    INDEX_NAME,
    ARRAY_AGG(
      CONCAT(
        '{"column_name":"', IFNULL(COLUMN_NAME, ''), '",',
        '"ordering":"', IFNULL(COLUMN_ORDERING, ''), '"}'
      ) ORDER BY ORDINAL_POSITION
    ) AS key_column_json_details
  FROM INFORMATION_SCHEMA.INDEX_COLUMNS
  WHERE ORDINAL_POSITION IS NOT NULL -- Key columns
  GROUP BY TABLE_CATALOG, TABLE_SCHEMA, TABLE_NAME, INDEX_NAME
),

-- Helper CTE for aggregating index storing columns (as JSON strings)
index_storing_columns_agg_cte AS (
  SELECT
    TABLE_CATALOG,
    TABLE_SCHEMA,
    TABLE_NAME,
    INDEX_NAME,
    ARRAY_AGG(CONCAT('"', COLUMN_NAME, '"') ORDER BY COLUMN_NAME) AS storing_column_json_names
  FROM INFORMATION_SCHEMA.INDEX_COLUMNS
  WHERE ORDINAL_POSITION IS NULL -- Storing columns
  GROUP BY TABLE_CATALOG, TABLE_SCHEMA, TABLE_NAME, INDEX_NAME
),

-- 4. Index Information (with JSON string for each index)
indexes_info_cte AS (
  SELECT
    I.TABLE_SCHEMA,
    I.TABLE_NAME,
    ARRAY_AGG(
      CONCAT(
        '{',
        '"index_name":"', IFNULL(I.INDEX_NAME, ''), '",',
        '"index_type":"', IFNULL(I.INDEX_TYPE, ''), '",',
        '"is_unique":', IF(I.IS_UNIQUE, 'true', 'false'), ',',
        '"is_null_filtered":', IF(I.IS_NULL_FILTERED, 'true', 'false'), ',',
        '"interleaved_in_table":', IF(I.PARENT_TABLE_NAME IS NULL, 'null', CONCAT('"', I.PARENT_TABLE_NAME, '"')), ',',
        '"index_key_columns":[', ARRAY_TO_STRING(COALESCE(KeyIndexCols.key_column_json_details, []), ','), '],',
        '"storing_columns":[', ARRAY_TO_STRING(COALESCE(StoringIndexCols.storing_column_json_names, []), ','), ']',
        '}'
      ) ORDER BY I.INDEX_NAME
    ) AS indexes_json_array_elements
  FROM INFORMATION_SCHEMA.INDEXES AS I
  LEFT JOIN index_key_columns_agg_cte AS KeyIndexCols
    ON I.TABLE_CATALOG = KeyIndexCols.TABLE_CATALOG AND I.TABLE_SCHEMA = KeyIndexCols.TABLE_SCHEMA AND I.TABLE_NAME = KeyIndexCols.TABLE_NAME AND I.INDEX_NAME = KeyIndexCols.INDEX_NAME
  LEFT JOIN index_storing_columns_agg_cte AS StoringIndexCols
    ON I.TABLE_CATALOG = StoringIndexCols.TABLE_CATALOG AND I.TABLE_SCHEMA = StoringIndexCols.TABLE_SCHEMA AND I.TABLE_NAME = StoringIndexCols.TABLE_NAME AND I.INDEX_NAME = StoringIndexCols.INDEX_NAME AND I.INDEX_TYPE = 'INDEX'
  WHERE EXISTS (SELECT 1 FROM table_info_cte TI WHERE I.TABLE_SCHEMA = TI.TABLE_SCHEMA AND I.TABLE_NAME = TI.TABLE_NAME)
  GROUP BY I.TABLE_SCHEMA, I.TABLE_NAME
)

-- Final SELECT to build the JSON output
SELECT
  TI.TABLE_SCHEMA AS schema_name,
  TI.TABLE_NAME AS object_name,
  CASE
    WHEN @output_format = 'simple' THEN
      -- IF format is 'simple', return basic JSON
          CONCAT('{"name":"', IFNULL(REPLACE(TI.TABLE_NAME, '"', '\"'), ''), '"}')
    ELSE
      CONCAT(
        '{',
        '"schema_name":"', IFNULL(TI.TABLE_SCHEMA, ''), '",',
        '"object_name":"', IFNULL(TI.TABLE_NAME, ''), '",',
        '"object_type":"', IFNULL(TI.TABLE_TYPE, ''), '",',
        '"columns":[', ARRAY_TO_STRING(COALESCE(CI.columns_json_array_elements, []), ','), '],',
        '"constraints":[', ARRAY_TO_STRING(COALESCE(CONSI.constraints_json_array_elements, []), ','), '],',
        '"indexes":[', ARRAY_TO_STRING(COALESCE(II.indexes_json_array_elements, []), ','), ']',
        '}'
      )
  END AS object_details
FROM table_info_cte AS TI
LEFT JOIN columns_info_cte AS CI
  ON TI.TABLE_SCHEMA = CI.TABLE_SCHEMA AND TI.TABLE_NAME = CI.TABLE_NAME
LEFT JOIN constraints_info_cte AS CONSI
  ON TI.TABLE_SCHEMA = CONSI.TABLE_SCHEMA AND TI.TABLE_NAME = CONSI.TABLE_NAME
LEFT JOIN indexes_info_cte AS II
  ON TI.TABLE_SCHEMA = II.TABLE_SCHEMA AND TI.TABLE_NAME = II.TABLE_NAME
ORDER BY TI.TABLE_SCHEMA, TI.TABLE_NAME`
