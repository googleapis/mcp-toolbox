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

package mysqlgetqueryplan

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "mysql-get-query-plan"

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
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	sqlParameter := parameters.NewStringParameter("sql_statement", "The sql statement to explain.")
	params := parameters.Parameters{sqlParameter}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: params.Manifest(), AuthRequired: cfg.AuthRequired},
			params,
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
	sqlStr, ok := paramsMap["sql_statement"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to get cast %s", paramsMap["sql_statement"]), nil)
	}

	// Log the query executed for debugging.
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting logger", http.StatusInternalServerError, err)
	}
	logger.DebugContext(ctx, fmt.Sprintf("executing `%s` tool query: %s", resourceType, sqlStr))

	// EXPLAIN (without ANALYZE) only computes the plan; it never executes the
	// wrapped statement. The hardcoded FORMAT=JSON prefix also makes EXPLAIN
	// ANALYZE unreachable, since MySQL's grammar requires ANALYZE to precede
	// FORMAT=. Multi-statement input is rejected by the driver (multiStatements
	// is off by default). Limiting the source to a least-privilege database user
	// is the recommended control for the statements that EXPLAIN does plan.
	query := fmt.Sprintf("EXPLAIN FORMAT=JSON %s", sqlStr)
	result, err := source.RunSQL(ctx, query, nil)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	// extract and return only the query plan object
	resSlice, ok := result.([]any)
	if !ok || len(resSlice) == 0 {
		return nil, util.NewClientServerError("no query plan returned", http.StatusInternalServerError, nil)
	}
	row, ok := resSlice[0].(orderedmap.Row)
	if !ok || len(row.Columns) == 0 {
		return nil, util.NewClientServerError("no query plan returned in row", http.StatusInternalServerError, nil)
	}
	plan, ok := row.Columns[0].Value.(string)
	if !ok {
		return nil, util.NewClientServerError("unable to convert plan object to string", http.StatusInternalServerError, nil)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(plan), &out); err != nil {
		return nil, util.NewClientServerError("failed to unmarshal query plan json", http.StatusInternalServerError, err)
	}
	return out, nil
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
