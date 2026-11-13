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

package cockroachdblisttables

import (
	"context"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/cockroachdb"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/jackc/pgx/v5"
)

const resourceType string = "cockroachdb-list-tables"

const listTablesStatement = `
	WITH desired_relkinds AS (
		SELECT ARRAY['r', 'p']::char[] AS kinds
	),
	table_info AS (
		SELECT
			t.oid AS table_oid,
			ns.nspname AS schema_name,
			t.relname AS table_name,
			pg_get_userbyid(t.relowner) AS table_owner,
			obj_description(t.oid, 'pg_class') AS table_comment,
			t.relkind AS object_kind
		FROM
			pg_class t
		JOIN
			pg_namespace ns ON ns.oid = t.relnamespace
		CROSS JOIN desired_relkinds dk
		WHERE
			t.relkind = ANY(dk.kinds)
			AND (NULLIF(TRIM($1), '') IS NULL OR t.relname = ANY(string_to_array($1,',')))
			AND ns.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast', 'crdb_internal', 'pg_extension')
			AND ns.nspname NOT LIKE 'pg_temp_%' AND ns.nspname NOT LIKE 'pg_toast_temp_%'
	),
	columns_info AS (
		SELECT
			att.attrelid AS table_oid, att.attname AS column_name, format_type(att.atttypid, att.atttypmod) AS data_type,
			att.attnum AS column_ordinal_position, att.attnotnull AS is_not_nullable,
			pg_get_expr(ad.adbin, ad.adrelid) AS column_default, col_description(att.attrelid, att.attnum) AS column_comment
		FROM pg_attribute att LEFT JOIN pg_attrdef ad ON att.attrelid = ad.adrelid AND att.attnum = ad.adnum
		JOIN table_info ti ON att.attrelid = ti.table_oid WHERE att.attnum > 0 AND NOT att.attisdropped
	),
	constraints_info AS (
		SELECT
			con.conrelid AS table_oid, con.conname AS constraint_name, pg_get_constraintdef(con.oid) AS constraint_definition,
			CASE con.contype WHEN 'p' THEN 'PRIMARY KEY' WHEN 'f' THEN 'FOREIGN KEY' WHEN 'u' THEN 'UNIQUE' WHEN 'c' THEN 'CHECK' ELSE con.contype::text END AS constraint_type,
			(SELECT array_agg(att.attname ORDER BY u.attposition) FROM unnest(con.conkey) WITH ORDINALITY AS u(attnum, attposition) JOIN pg_attribute att ON att.attrelid = con.conrelid AND att.attnum = u.attnum) AS constraint_columns,
			NULLIF(con.confrelid, 0)::regclass AS foreign_key_referenced_table,
			(SELECT array_agg(att.attname ORDER BY u.attposition) FROM unnest(con.confkey) WITH ORDINALITY AS u(attnum, attposition) JOIN pg_attribute att ON att.attrelid = con.confrelid AND att.attnum = u.attnum WHERE con.contype = 'f') AS foreign_key_referenced_columns
		FROM pg_constraint con JOIN table_info ti ON con.conrelid = ti.table_oid
	),
	indexes_info AS (
		SELECT
			idx.indrelid AS table_oid, ic.relname AS index_name, pg_get_indexdef(idx.indexrelid) AS index_definition,
			idx.indisunique AS is_unique, idx.indisprimary AS is_primary, am.amname AS index_method,
			(SELECT array_agg(att.attname ORDER BY u.ord) FROM unnest(idx.indkey::int[]) WITH ORDINALITY AS u(colidx, ord) LEFT JOIN pg_attribute att ON att.attrelid = idx.indrelid AND att.attnum = u.colidx WHERE u.colidx <> 0) AS index_columns
		FROM pg_index idx JOIN pg_class ic ON ic.oid = idx.indexrelid JOIN pg_am am ON am.oid = ic.relam JOIN table_info ti ON idx.indrelid = ti.table_oid
	)
	SELECT
		ti.schema_name,
		ti.table_name AS object_name,
		CASE
		  WHEN $2 = 'simple' THEN
			  json_build_object('name', ti.table_name)
		  ELSE
			json_build_object(
				'schema_name', ti.schema_name,
				'object_name', ti.table_name,
				'object_type', CASE ti.object_kind
								WHEN 'r' THEN 'TABLE'
								WHEN 'p' THEN 'PARTITIONED TABLE'
								ELSE ti.object_kind::text
							END,
				'owner', ti.table_owner,
				'comment', ti.table_comment,
				'columns', COALESCE((SELECT json_agg(json_build_object('column_name',ci.column_name,'data_type',ci.data_type,'ordinal_position',ci.column_ordinal_position,'is_not_nullable',ci.is_not_nullable,'column_default',ci.column_default,'column_comment',ci.column_comment) ORDER BY ci.column_ordinal_position) FROM columns_info ci WHERE ci.table_oid = ti.table_oid), '[]'::json),
				'constraints', COALESCE((SELECT json_agg(json_build_object('constraint_name',cons.constraint_name,'constraint_type',cons.constraint_type,'constraint_definition',cons.constraint_definition,'constraint_columns',cons.constraint_columns,'foreign_key_referenced_table',cons.foreign_key_referenced_table,'foreign_key_referenced_columns',cons.foreign_key_referenced_columns)) FROM constraints_info cons WHERE cons.table_oid = ti.table_oid), '[]'::json),
				'indexes', COALESCE((SELECT json_agg(json_build_object('index_name',ii.index_name,'index_definition',ii.index_definition,'is_unique',ii.is_unique,'is_primary',ii.is_primary,'index_method',ii.index_method,'index_columns',ii.index_columns)) FROM indexes_info ii WHERE ii.table_oid = ti.table_oid), '[]'::json)
			) 
		END AS object_details
	FROM table_info ti ORDER BY ti.schema_name, ti.table_name;
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
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
}

var _ compatibleSource = &cockroachdb.Source{}

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
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	allParameters := parameters.Parameters{
		parameters.NewStringParameter("table_names", "Optional: A comma-separated list of table names. If empty, details for all tables will be listed.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("output_format", "Optional: Use 'simple' for names only or 'detailed' for full info.", parameters.WithStringDefault("detailed")),
	}
	paramManifest := allParameters.Manifest()

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

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramsMap := params.AsMap()

	tableNames, ok := paramsMap["table_names"].(string)
	if !ok {
		return nil, util.NewAgentError("invalid 'table_names' parameter; expected a string", nil)
	}
	outputFormat, _ := paramsMap["output_format"].(string)
	if outputFormat != "simple" && outputFormat != "detailed" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid value for output_format: must be 'simple' or 'detailed', but got %q", outputFormat), nil)
	}

	results, err := source.Query(ctx, listTablesStatement, tableNames, outputFormat)
	if err != nil {
		return nil, util.ProcessGeneralError(fmt.Errorf("unable to execute query: %w", err))
	}
	defer results.Close()

	fields := results.FieldDescriptions()
	var out []map[string]any

	for results.Next() {
		values, err := results.Values()
		if err != nil {
			return nil, util.NewClientServerError("unable to parse row", http.StatusInternalServerError, err)
		}
		rowMap := make(map[string]any)
		for i, field := range fields {
			rowMap[string(field.Name)] = values[i]
		}
		out = append(out, rowMap)
	}

	if err := results.Err(); err != nil {
		return nil, util.ProcessGeneralError(fmt.Errorf("error reading query results: %w", err))
	}

	return out, nil
}
