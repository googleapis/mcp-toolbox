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

package postgreslistschemas

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

const resourceType string = "postgres-list-schemas"

const listSchemasStatement = `
    WITH
    schema_grants AS (
        SELECT schema_oid, jsonb_object_agg(grantee, privileges) AS grants
        FROM
        (
            SELECT
            n.oid AS schema_oid,
            CASE
                WHEN p.grantee = 0 THEN 'PUBLIC'
                ELSE pg_catalog.pg_get_userbyid(p.grantee)
                END
                AS grantee,
            jsonb_agg(p.privilege_type ORDER BY p.privilege_type) AS privileges
            FROM pg_catalog.pg_namespace n, aclexplode(n.nspacl) p
            WHERE n.nspacl IS NOT NULL
            GROUP BY n.oid, grantee
        ) permissions_by_grantee
        GROUP BY schema_oid
    ),
    all_schemas AS (
        SELECT
        n.nspname AS schema_name,
        pg_catalog.pg_get_userbyid(n.nspowner) AS owner,
        COALESCE(sg.grants, '{}'::jsonb) AS grants,
        (
            SELECT COUNT(*)
            FROM pg_catalog.pg_class c
            WHERE c.relnamespace = n.oid AND c.relkind = 'r'
        ) AS tables,
        (
            SELECT COUNT(*)
            FROM pg_catalog.pg_class c
            WHERE c.relnamespace = n.oid AND c.relkind = 'v'
        ) AS views,
        (SELECT COUNT(*) FROM pg_catalog.pg_proc p WHERE p.pronamespace = n.oid)
            AS functions
        FROM pg_catalog.pg_namespace n
        LEFT JOIN schema_grants sg
        ON n.oid = sg.schema_oid
    )
    SELECT *
    FROM all_schemas
    -- Exclude system and temporary schemas created per session.
    WHERE
        schema_name NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
        AND schema_name NOT LIKE 'pg_temp_%'
        AND schema_name NOT LIKE 'pg_toast_temp_%'
        AND ($1::text IS NULL OR schema_name ILIKE '%' || $1::text || '%')
        AND ($2::text IS NULL OR owner ILIKE '%' || $2::text || '%')
    ORDER BY schema_name
    LIMIT COALESCE($3::int, NULL);
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
		parameters.NewStringParameter("schema_name", "Optional: A specific schema name pattern to search for.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("owner", "Optional: A specific schema owner name pattern to search for.", parameters.WithStringDefault("")),
		parameters.NewIntParameter("limit", "Optional: The maximum number of schemas to return.", parameters.WithIntDefault(10)),
	}

	if cfg.Description == "" {
		cfg.Description = "Lists all schemas in the database ordered by schema name and excluding system and temporary schemas. It returns the schema name, schema owner, grants, number of functions, number of tables and number of views within each schema."
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
	resp, err := source.RunSQL(ctx, listSchemasStatement, sliceParams)
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
