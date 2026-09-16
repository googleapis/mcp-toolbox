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

package falkordbexecutecypher

import (
	"context"
	"fmt"
	"net/http"

	"github.com/goccy/go-yaml"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "falkordb-execute-cypher"

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
	DefaultGraph() string // kept to ensure falkordb source
	RunQuery(context.Context, string, string, map[string]any, bool, bool) (any, error)
}

type Config struct {
	tools.ConfigBase   `yaml:",inline"`
	Type               string                 `yaml:"type" validate:"required"`
	Source             string                 `yaml:"source" validate:"required"`
	ReadOnly           bool                   `yaml:"readOnly"`
	AllowGraphOverride bool                   `yaml:"allowGraphOverride"`
	Annotations        *tools.ToolAnnotations `yaml:"annotations,omitempty"`
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

	cypherParameter := parameters.NewStringParameter("cypher", "The cypher to execute.")
	dryRunParameter := parameters.NewBooleanParameter(
		"dry_run",
		"If set to true, the query will be validated and its execution plan "+
			"returned without running the query. Defaults to false.", parameters.WithBooleanDefault(
			false))
	params := parameters.Parameters{cypherParameter, dryRunParameter}
	if cfg.AllowGraphOverride {
		graphParameter := parameters.NewStringParameter(
			"graph",
			"The name of the graph to query. Defaults to the source's configured graph.",
			parameters.WithStringDefault(""))
		params = append(params, graphParameter)
	}

	defaultAnnotations := tools.NewDestructiveAnnotations
	if cfg.ReadOnly {
		defaultAnnotations = tools.NewReadOnlyAnnotations
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, defaultAnnotations),
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
	cypherStr, ok := paramsMap["cypher"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast cypher parameter %s", paramsMap["cypher"]), nil)
	}

	if cypherStr == "" {
		return nil, util.NewAgentError("parameter 'cypher' must be a non-empty string", nil)
	}

	dryRun, _ := paramsMap["dry_run"].(bool)

	graphName := ""
	if t.Cfg.AllowGraphOverride {
		if g, ok := paramsMap["graph"].(string); ok {
			graphName = g
		}
	}

	resp, err := source.RunQuery(ctx, graphName, cypherStr, nil, t.Cfg.ReadOnly, dryRun)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	return resp, nil
}
