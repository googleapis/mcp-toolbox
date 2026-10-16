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

package clickhouse

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ClickHouse/clickhouse-go/v2"
	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const executeSQLType string = "clickhouse-execute-sql"

func init() {
	if !tools.Register(executeSQLType, newExecuteSQLConfig) {
		panic(fmt.Sprintf("tool type %q already registered", executeSQLType))
	}
}

func newExecuteSQLConfig(ctx context.Context, name string, decoder *yaml.Decoder) (tools.ToolConfig, error) {
	actual := Config{ConfigBase: tools.ConfigBase{Name: name}}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type compatibleSource interface {
	RunSQL(context.Context, string, parameters.ParamValues) (any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return executeSQLType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	sqlParameter := parameters.NewStringParameter("sql", "The SQL statement to execute.")
	queryParamsParameter := parameters.NewMapParameter(
		"params",
		"Optional query parameters for {name:Type} placeholders in the SQL statement, by name. ClickHouse binds them "+
			"on the server. Each value is a string in ClickHouse's text format for its type (e.g. \"42\", "+
			"\"2024-01-31\", \"['a','b']\").",
		"string",
		parameters.WithMapRequired(false),
	)
	params := parameters.Parameters{sqlParameter, queryParamsParameter}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: params.Manifest(), AuthRequired: cfg.AuthRequired},
			params,
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
	sql, ok := paramsMap["sql"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast sql parameter %s", paramsMap["sql"]), nil)
	}
	queryParams, err := toQueryParameters(paramsMap["params"])
	if err != nil {
		return nil, util.NewAgentError(err.Error(), nil)
	}
	if len(queryParams) > 0 {
		// Sent as param_<name>: the server parses each value as its declared
		// type, so a value is never read as SQL.
		ctx = clickhouse.Context(ctx, clickhouse.WithParameters(queryParams))
	}
	resp, err := source.RunSQL(ctx, sql, nil)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	return resp, nil
}

// toQueryParameters converts the optional params argument, a map of parameter
// names to values in ClickHouse's text format, to query parameters.
func toQueryParameters(value any) (clickhouse.Parameters, error) {
	if value == nil {
		return nil, nil
	}
	values, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unable to cast params parameter %v: want an object of strings", value)
	}
	queryParams := make(clickhouse.Parameters, len(values))
	for name, v := range values {
		text, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid value for params.%s: want a string in ClickHouse's text format, got %T", name, v)
		}
		queryParams[name] = text
	}
	return queryParams, nil
}
