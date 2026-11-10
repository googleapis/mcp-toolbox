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

package arcadedbexecutesql

import (
	"context"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "arcadedb-execute-sql"

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
	ArcadeDBDatabase() string
	RunSQL(context.Context, string, map[string]any, bool) (any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
	ReadOnly         bool   `yaml:"readOnly"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	sqlParameter := parameters.NewStringParameter("sql", "The SQL statement to execute.")
	queryParamsParameter := parameters.NewMapParameter(
		"params",
		"Optional query parameters to use with the SQL statement.",
		"",
		parameters.WithMapDefault(map[string]any{}),
	)
	dryRunParameter := parameters.NewBooleanParameter(
		"dry_run",
		"If set to true, the SQL will be validated and execution plan metadata will be returned without running it. Defaults to false.",
		parameters.WithBooleanDefault(false),
	)
	params := parameters.Parameters{sqlParameter, queryParamsParameter, dryRunParameter}

	allParameters, paramManifest, err := parameters.ProcessParameters(nil, params)
	if err != nil {
		return nil, err
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: paramManifest, AuthRequired: cfg.AuthRequired},
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
	sqlStr, ok := paramsMap["sql"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast sql parameter %v", paramsMap["sql"]), nil)
	}
	if sqlStr == "" {
		return nil, util.NewAgentError("parameter 'sql' must be a non-empty string", nil)
	}

	queryParams, ok := paramsMap["params"].(map[string]any)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast params parameter %v", paramsMap["params"]), nil)
	}

	dryRun, ok := paramsMap["dry_run"].(bool)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast dry_run parameter %v", paramsMap["dry_run"]), nil)
	}

	query := sqlStr
	if dryRun {
		query = "EXPLAIN " + sqlStr
	}

	resp, err := source.RunSQL(ctx, query, queryParams, t.Cfg.ReadOnly)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	return resp, nil
}

func (t Tool) EmbedParams(ctx context.Context, paramValues parameters.ParamValues, pMgr tools.PrimitiveManagerI) (parameters.ParamValues, error) {
	return parameters.EmbedParams(ctx, t.StaticParameters, paramValues, pMgr, nil)
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
