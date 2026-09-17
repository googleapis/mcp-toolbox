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

package postgreslistpublicationtables

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

const resourceType string = "postgres-list-publication-tables"

const listPublicationTablesStatement = `
    WITH
    publication_details AS (
        SELECT
            pt.pubname AS publication_name,
            pt.schemaname AS schema_name,
            pt.tablename AS table_name,
            -- Definition details
            p.puballtables AS publishes_all_tables,
            p.pubinsert AS publishes_inserts,
            p.pubupdate AS publishes_updates,
            p.pubdelete AS publishes_deletes,
            p.pubtruncate AS publishes_truncates,
            -- Owner information
            pg_catalog.pg_get_userbyid(p.pubowner) AS publication_owner
        FROM pg_catalog.pg_publication_tables pt
        JOIN pg_catalog.pg_publication p
            ON pt.pubname = p.pubname
    )
    SELECT *
    FROM publication_details
    WHERE
        (NULLIF(TRIM($1::text), '') IS NULL OR table_name = ANY(regexp_split_to_array(TRIM($1::text), '\s*,\s*')))
        AND (NULLIF(TRIM($2::text), '') IS NULL OR publication_name = ANY(regexp_split_to_array(TRIM($2::text), '\s*,\s*')))
        AND (NULLIF(TRIM($3::text), '') IS NULL OR schema_name = ANY(regexp_split_to_array(TRIM($3::text), '\s*,\s*')))
    ORDER BY
        publication_name, schema_name, table_name
    LIMIT COALESCE($4::int, 50);
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
		parameters.NewStringParameter("table_names", "Optional: Filters by a comma-separated list of table names.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("publication_names", "Optional: Filters by a comma-separated list of publication names.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("schema_names", "Optional: Filters by a comma-separated list of schema names.", parameters.WithStringDefault("")),
		parameters.NewIntParameter("limit", "Optional: The maximum number of rows to return.", parameters.WithIntDefault(50)),
	}
	if cfg.Description == "" {
		cfg.Description = "Lists all publication tables in the database. Returns the publication name, schema name, and table name, along with definition details indicating if it publishes all tables, whether it replicates inserts, updates, deletes, or truncates, and the publication owner."
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
	resp, err := source.RunSQL(ctx, listPublicationTablesStatement, sliceParams)
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
