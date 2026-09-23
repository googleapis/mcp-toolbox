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

package postgreslisttablestats

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

const resourceType string = "postgres-list-table-stats"

const listTableStats = `
    WITH table_stats AS (
        SELECT
            s.schemaname AS schema_name,
            s.relname AS table_name,
            pg_catalog.pg_get_userbyid(c.relowner) AS owner,
            pg_total_relation_size(s.relid) AS total_size_bytes,
            s.seq_scan,
            s.idx_scan,
            -- Ratio of index scans to total scans
            CASE
                WHEN (s.seq_scan + s.idx_scan) = 0 THEN 0
                ELSE round((s.idx_scan * 100.0) / (s.seq_scan + s.idx_scan), 2)
            END AS idx_scan_ratio_percent,
            s.n_live_tup AS live_rows,
            s.n_dead_tup AS dead_rows,
            -- Percentage of rows that are "dead" (bloat)
            CASE
                WHEN (s.n_live_tup + s.n_dead_tup) = 0 THEN 0
                ELSE round((s.n_dead_tup * 100.0) / (s.n_live_tup + s.n_dead_tup), 2)
            END AS dead_row_ratio_percent,
            s.n_tup_ins,
            s.n_tup_upd,
            s.n_tup_del,
            s.last_vacuum,
            s.last_autovacuum,
            s.last_autoanalyze
        FROM pg_stat_all_tables s
        JOIN pg_catalog.pg_class c ON s.relid = c.oid
      )
      SELECT *
      FROM table_stats
      WHERE
        ($1::text IS NULL OR schema_name LIKE '%' || $1::text || '%')
        AND ($2::text IS NULL OR table_name LIKE '%' || $2::text || '%')
        AND ($3::text IS NULL OR owner LIKE '%' || $3::text || '%')
      ORDER BY
        CASE
          WHEN $4::text = 'size' THEN total_size_bytes
          WHEN $4::text = 'dead_rows' THEN dead_rows
          WHEN $4::text = 'seq_scan' THEN seq_scan
          WHEN $4::text = 'idx_scan' THEN idx_scan
          ELSE seq_scan
        END DESC
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
	PostgresPoolContext(context.Context) (*pgxpool.Pool, error)
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
		parameters.NewStringParameter("schema_name", "Optional: A specific schema name to filter by", parameters.WithStringDefault("public")),
		parameters.NewStringParameter("table_name", "Optional: A specific table name to filter by", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("owner", "Optional: A specific owner to filter by", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("sort_by", "Optional: The column to sort by", parameters.WithStringRequired(false)),
		parameters.NewIntParameter("limit", "Optional: The maximum number of results to return", parameters.WithIntDefault(50)),
	}

	if cfg.Description == "" {
		cfg.Description = `Lists the user table statistics in the database ordered by number of
        sequential scans with a default limit of 50 rows. Returns the following
        columns: schema name, table name, table size in bytes, number of
        sequential scans, number of index scans, idx_scan_ratio_percent (showing
        the percentage of total scans that utilized an index, where a low ratio
        indicates missing or ineffective indexes), number of live rows, number
        of dead rows, dead_row_ratio_percent (indicating potential table bloat),
        total number of rows inserted, updated, and deleted, the timestamps
        for the last_vacuum, last_autovacuum, and last_autoanalyze operations.`
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

	resp, err := source.RunSQL(ctx, listTableStats, sliceParams)
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
