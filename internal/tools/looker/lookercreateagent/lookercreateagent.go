// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package lookercreateagent

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"

	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

const resourceType string = "looker-create-agent"

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
	UseClientAuthorization() bool
	GetAuthTokenHeaderName() string
	LookerApiSettings() *rtl.ApiSettings
	GetLookerSDK(context.Context, string) (*v4.LookerSDK, error)
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

	nameParameter := parameters.NewStringParameter("name", "The name of the agent.", parameters.WithStringDefault(""))
	descriptionParameter := parameters.NewStringParameter("description", "The description of the agent.", parameters.WithStringDefault(""))
	instructionsParameter := parameters.NewStringParameter("instructions", "The instructions (system prompt) for the agent.", parameters.WithStringDefault(""))
	sourcesParameter := parameters.NewArrayParameter(
		"sources",
		"Optional. A list of JSON-encoded data sources for the agent (e.g., [{\"model\": \"my_model\", \"explore\": \"my_explore\"}]).",
		parameters.NewMapParameter(
			"source",
			"A JSON-encoded source object with 'model' and 'explore' keys.",
			"string",
		),
		parameters.WithArrayRequired(false),
	)

	codeInterpreterParameter := parameters.NewBooleanParameter("code_interpreter", "Optional. Enables Code Interpreter for this Agent.", parameters.WithBooleanDefault(false))
	params := parameters.Parameters{nameParameter, descriptionParameter, instructionsParameter, sourcesParameter, codeInterpreterParameter}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			lookercommon.WriteAnnotations(cfg.Annotations),
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
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError("unable to get logger from ctx", http.StatusInternalServerError, err)
	}

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError(fmt.Sprintf("error getting sdk: %v", err), http.StatusInternalServerError, err)
	}

	mapParams := params.AsMap()
	logger.DebugContext(ctx, fmt.Sprintf("%s params = ", t.Cfg.Name), mapParams)

	var name, description, instructions string
	if v, ok := mapParams["name"].(string); ok {
		name = v
	}
	if v, ok := mapParams["description"].(string); ok {
		description = v
	}
	if v, ok := mapParams["instructions"].(string); ok {
		instructions = v
	}

	agentSources := make([]v4.Source, 0)
	if sources, ok := mapParams["sources"].([]any); ok {
		for _, s := range sources {
			source := s.(map[string]any)
			model, ok := source["model"].(string)
			if !ok {
				return nil, util.NewClientServerError("invalid source format: expected model of type string", http.StatusBadRequest, nil)
			}
			explore, ok := source["explore"].(string)
			if !ok {
				return nil, util.NewClientServerError("invalid source format: expected explore of type string", http.StatusBadRequest, nil)
			}
			agentSources = append(agentSources, v4.Source{
				Model:   &model,
				Explore: &explore,
			})
		}
	} else {
		return nil, util.NewClientServerError(fmt.Sprintf("invalid sources. got %T, expected []any", mapParams["sources"]), http.StatusBadRequest, nil)
	}

	codeInterpreter, hasCodeInterpreter := mapParams["code_interpreter"].(bool)

	if name == "" {
		return nil, util.NewClientServerError(fmt.Sprintf("%s operation: name must be specified", t.Cfg.Type), http.StatusBadRequest, nil)
	}
	body := v4.WriteAgent{
		Name: &name,
	}
	if description != "" {
		body.Description = &description
	}
	if instructions != "" {
		context := v4.Context{
			Instructions: &instructions,
		}
		body.Context = &context
	}
	if len(agentSources) > 0 {
		body.Sources = &agentSources
	}
	if hasCodeInterpreter {
		body.CodeInterpreter = &codeInterpreter
	}
	resp, err := sdk.CreateAgent(body, "", source.LookerApiSettings())
	if err != nil {
		if strings.Contains(err.Error(), "status=401") {
			return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
		}
		return nil, util.ProcessGeneralError(err)
	}
	return resp, nil

}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}

func (t Tool) GetAuthTokenHeaderName(source sources.Source) (string, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return "", fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.GetAuthTokenHeaderName(), nil
}
