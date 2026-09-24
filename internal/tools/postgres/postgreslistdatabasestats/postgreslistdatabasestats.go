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

package postgreslistdatabasestats

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

const resourceType string = "postgres-list-database-stats"

const listDatabaseStats = `
    WITH database_stats AS (
        SELECT
            s.datname AS database_name,
            -- Database Metadata
            d.datallowconn AS is_connectable,
            pg_get_userbyid(d.datdba) AS database_owner,
            ts.spcname AS default_tablespace,

            -- Cache Performance
            CASE
                WHEN (s.blks_hit + s.blks_read) = 0 THEN 0
                ELSE round((s.blks_hit * 100.0) / (s.blks_hit + s.blks_read), 2)
            END AS cache_hit_ratio_percent,
            s.blks_read AS blocks_read_from_disk,
            s.blks_hit AS blocks_hit_in_cache,

            -- Transaction Throughput
            s.xact_commit,
            s.xact_rollback,
            round(s.xact_rollback * 100.0 / (s.xact_commit + s.xact_rollback + 1), 2) AS rollback_ratio_percent,

            -- Tuple Activity
            s.tup_returned AS rows_returned_by_queries,
            s.tup_fetched AS rows_fetched_by_scans,
            s.tup_inserted,
            s.tup_updated,
            s.tup_deleted,

            -- Temporary File Usage
            s.temp_files,
            s.temp_bytes AS temp_size_bytes,

            -- Conflicts & Deadlocks
            s.conflicts,
            s.deadlocks,

            -- General Info
            s.numbackends AS active_connections,
            s.stats_reset AS statistics_last_reset,
            pg_database_size(s.datid) AS database_size_bytes
        FROM
            pg_stat_database s
        JOIN
            pg_database d ON d.oid = s.datid
        JOIN
            pg_tablespace ts ON ts.oid = d.dattablespace
        WHERE
            -- Exclude cloudsql internal databases
            s.datname NOT IN ('cloudsqladmin')
            -- Exclude template databases if not requested
            AND ( $2::boolean IS TRUE OR d.datistemplate IS FALSE )
    )
    SELECT *
    FROM database_stats
    WHERE
        ($1::text IS NULL OR database_name LIKE '%' || $1::text || '%')
        AND ($3::text IS NULL OR database_owner LIKE '%' || $3::text || '%')
        AND ($4::text IS NULL OR default_tablespace LIKE '%' || $4::text || '%')
    ORDER BY
        CASE WHEN $5::text = 'size' THEN database_size_bytes END DESC,
        CASE WHEN $5::text = 'commit' THEN xact_commit END DESC,
        database_name
    LIMIT COALESCE($6::int, 10);
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
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	allParameters := parameters.Parameters{
		parameters.NewStringParameter("database_name", "Optional: A specific database name pattern to search for.", parameters.WithStringDefault("")),
		parameters.NewBooleanParameter("include_templates", "Optional: Whether to include template databases in the results.", parameters.WithBooleanDefault(false)),
		parameters.NewStringParameter("database_owner", "Optional: A specific database owner name pattern to search for.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("default_tablespace", "Optional: A specific default tablespace name pattern to search for.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("order_by", "Optional: The field to order the results by. Valid values are 'size' and 'commit'.", parameters.WithStringDefault("")),
		parameters.NewIntParameter("limit", "Optional: The maximum number of rows to return.", parameters.WithIntDefault(10)),
	}
	if cfg.Description == "" {
		cfg.Description =
			"Lists the key performance and activity statistics for each PostgreSQL database" +
				"in the instance, offering insights into cache efficiency, transaction throughput" +
				"row-level activity, temporary file " +
				"usage, and contention. " +
				"It returns: the database name, whether the database is connectable,  " +
				"database owner, default tablespace name, the percentage of data blocks " +
				"found in the buffer cache rather than being read from disk (a higher " +
				"value indicates better cache performance), the total number of disk " +
				"blocks read from disk, the total number of times disk blocks were found " +
				"already in the cache; the total number of committed transactions, the " +
				"total number of rolled back transactions, the percentage of rolled back " +
				"transactions compared to the total number of completed transactions, the " +
				"total number of rows returned by queries, the total number of live rows " +
				"fetched by scans, the total number of rows inserted, the total number " +
				"of rows updated, the total number of rows deleted, the number of " +
				"temporary files created by queries, the total size of all temporary " +
				"files created by queries in bytes, the number of query cancellations due " +
				"to conflicts with recovery, the number of deadlocks detected, the current " +
				"number of active connections to the database, the timestamp of the " +
				"last statistics reset, and total database size in bytes."
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

	resp, err := source.RunSQL(ctx, listDatabaseStats, sliceParams)
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
