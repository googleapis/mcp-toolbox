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

package postgreslisttriggers

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

const resourceType string = "postgres-list-triggers"

const listTriggersStatement = `
    WITH
    trigger_list AS (
        SELECT
            t.tgname AS trigger_name,
            n.nspname AS schema_name,
            c.relname AS table_name,
            CASE t.tgenabled
                WHEN 'O' THEN 'ENABLED'
                WHEN 'D' THEN 'DISABLED'
                WHEN 'R' THEN 'REPLICA'
                WHEN 'A' THEN 'ALWAYS'
                END AS status,
            CASE
                WHEN (t.tgtype::int & 2) = 2 THEN 'BEFORE'
                WHEN (t.tgtype::int & 64) = 64 THEN 'INSTEAD OF'
                ELSE 'AFTER'
                END AS timing,
            concat_ws(
                ', ',
                CASE WHEN (t.tgtype::int & 4) = 4 THEN 'INSERT' END,
                CASE WHEN (t.tgtype::int & 16) = 16 THEN 'UPDATE' END,
                CASE WHEN (t.tgtype::int & 8) = 8 THEN 'DELETE' END,
                CASE WHEN (t.tgtype::int & 32) = 32 THEN 'TRUNCATE' END) AS events,
            CASE WHEN (t.tgtype::int & 1) = 1 THEN 'ROW' ELSE 'STATEMENT' END AS activation_level,
            p.proname AS function_name,
            pg_get_triggerdef(t.oid) AS definition
        FROM pg_trigger t
        JOIN pg_class c
            ON t.tgrelid = c.oid
        JOIN pg_namespace n
            ON c.relnamespace = n.oid
        LEFT JOIN pg_proc p
            ON t.tgfoid = p.oid
        WHERE NOT t.tgisinternal
    )
    SELECT *
    FROM trigger_list
    WHERE
        ($1::text IS NULL OR trigger_name LIKE '%' || $1::text || '%')
        AND ($2::text IS NULL OR schema_name LIKE '%' || $2::text || '%')
        AND ($3::text IS NULL OR table_name LIKE '%' || $3::text || '%')
    ORDER BY schema_name, table_name, trigger_name
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
		parameters.NewStringParameter("trigger_name", "Optional: A specific trigger name pattern to search for.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("schema_name", "Optional: A specific schema name pattern to search for.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("table_name", "Optional: A specific table name pattern to search for.", parameters.WithStringDefault("")),
		parameters.NewIntParameter("limit", "Optional: The maximum number of rows to return.", parameters.WithIntDefault(50)),
	}

	if cfg.Description == "" {
		cfg.Description = "Lists all non-internal triggers in a database. Returns trigger name, schema name, table name, whether its enabled or disabled, timing (e.g BEFORE/AFTER of the event), the  events that cause the trigger to fire such as INSERT, UPDATE, or DELETE, whether the trigger activates per ROW or per STATEMENT, the handler function executed by the trigger and full definition."
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
	resp, err := source.RunSQL(ctx, listTriggersStatement, sliceParams)
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
