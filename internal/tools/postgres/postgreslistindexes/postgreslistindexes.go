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

package postgreslistindexes

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

const resourceType string = "postgres-list-indexes"

const listIndexesStatement = `
    WITH IndexDetails AS (
        SELECT
            s.schemaname AS schema_name,
            t.relname AS table_name,
            i.relname AS index_name,
            am.amname AS index_type,
            ix.indisunique AS is_unique,
            ix.indisprimary AS is_primary,
            pg_get_indexdef(i.oid) AS index_definition,
            pg_relation_size(i.oid) AS index_size_bytes,
            s.idx_scan AS index_scans,
            s.idx_tup_read AS tuples_read,
            s.idx_tup_fetch AS tuples_fetched,
            CASE
                WHEN s.idx_scan > 0 THEN true
                ELSE false
            END AS is_used
        FROM pg_catalog.pg_class t
        JOIN pg_catalog.pg_index ix
            ON t.oid = ix.indrelid
        JOIN pg_catalog.pg_class i
            ON i.oid = ix.indexrelid
        JOIN pg_catalog.pg_am am
            ON i.relam = am.oid
        JOIN pg_catalog.pg_stat_all_indexes s
            ON i.oid = s.indexrelid
        WHERE
            t.relkind = 'r'
            AND s.schemaname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
            AND s.schemaname NOT LIKE 'pg_temp_%'
    )
    SELECT *
    FROM IndexDetails
    WHERE
        ($1::text IS NULL OR schema_name LIKE '%' || $1 || '%')
        AND ($2::text IS NULL OR table_name LIKE '%' || $2 || '%')
        AND ($3::text IS NULL OR index_name LIKE '%' || $3 || '%')
        AND ($4::boolean IS NOT TRUE OR is_used IS FALSE)
    ORDER BY
        schema_name,
        table_name,
        index_name
    LIMIT COALESCE($5::int, 50);
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
		parameters.NewStringParameter("schema_name", "Optional: a text to filter results by schema name. The input is used within a LIKE clause.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("table_name", "Optional: a text to filter results by table name. The input is used within a LIKE clause.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("index_name", "Optional: a text to filter results by index name. The input is used within a LIKE clause.", parameters.WithStringDefault("")),
		parameters.NewBooleanParameter("only_unused", "Optional: If true, only returns indexes that have never been used.", parameters.WithBooleanDefault(false)),
		parameters.NewIntParameter("limit", "Optional: The maximum number of rows to return. Default is 50", parameters.WithIntDefault(50)),
	}

	if cfg.Description == "" {
		cfg.Description = "Lists available user indexes in the database, excluding system schemas (pg_catalog, information_schema). For each index, the following properties are returned: schema name, table name, index name, index type (access method), a boolean indicating if it's a unique index, a boolean indicating if it's for a primary key, the index definition, index size in bytes, the number of index scans, the number of index tuples read, the number of table tuples fetched via index scans, and a boolean indicating if the index has been used at least once."
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

	newParams, err := parameters.GetParams(t.StaticParameters, paramsMap)
	if err != nil {
		return nil, util.NewAgentError("unable to extract standard params", err)
	}
	sliceParams := newParams.AsSlice()

	resp, err := source.RunSQL(ctx, listIndexesStatement, sliceParams)
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
