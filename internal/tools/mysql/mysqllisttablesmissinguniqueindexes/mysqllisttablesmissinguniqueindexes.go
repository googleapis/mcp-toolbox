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

package mysqllisttablesmissinguniqueindexes

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

const resourceType string = "mysql-list-tables-missing-unique-indexes"

const listTablesMissingUniqueIndexesStatement = `
    SELECT
        tab.table_schema AS table_schema,
        tab.table_name AS table_name
    FROM
        information_schema.tables tab
        LEFT JOIN
        information_schema.table_constraints tco
        ON
            tab.table_schema = tco.table_schema
            AND tab.table_name = tco.table_name
            AND tco.constraint_type IN ('PRIMARY KEY', 'UNIQUE')
    WHERE
        tco.constraint_type IS NULL
        AND tab.table_schema NOT IN('mysql', 'information_schema', 'performance_schema', 'sys')
        AND tab.table_type = 'BASE TABLE'
        AND (COALESCE(?, '') = '' OR tab.table_schema = ?)
    ORDER BY
        tab.table_schema,
        tab.table_name
    LIMIT ?;
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
	MySQLPoolContext(context.Context) (*sql.DB, error)
	RunSQL(context.Context, string, []any) (any, error)
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
		parameters.NewStringParameter("table_schema", "(Optional) The database where the check is to be performed. Check all tables visible to the current user if not specified", parameters.WithStringDefault("")),
		parameters.NewIntParameter("limit", "(Optional) Max rows to return, default is 50", parameters.WithIntDefault(50)),
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

	table_schema, ok := paramsMap["table_schema"].(string)
	if !ok {
		return nil, util.NewAgentError("invalid 'table_schema' parameter; expected a string", nil)
	}
	limit, ok := paramsMap["limit"].(int)
	if !ok {
		return nil, util.NewAgentError("invalid 'limit' parameter; expected an integer", nil)
	}

	// Log the query executed for debugging.
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting logger", http.StatusInternalServerError, err)
	}
	logger.DebugContext(ctx, fmt.Sprintf("executing `%s` tool query: %s", resourceType, listTablesMissingUniqueIndexesStatement))
	resp, err := source.RunSQL(ctx, listTablesMissingUniqueIndexesStatement, []any{table_schema, table_schema, limit})
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
