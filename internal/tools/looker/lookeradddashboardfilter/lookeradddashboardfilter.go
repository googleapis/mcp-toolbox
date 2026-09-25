// Copyright 2025 Google LLC
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
package lookeradddashboardfilter

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

const resourceType string = "looker-add-dashboard-filter"

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
	LookerApiSettings(context.Context) (*rtl.ApiSettings, error)
	GetLookerSDK(context.Context, string) (*v4.LookerSDK, error)
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

	params := parameters.Parameters{}

	dashIdParameter := parameters.NewStringParameter("dashboard_id", "The id of the dashboard where this filter will exist")
	params = append(params, dashIdParameter)
	nameParameter := parameters.NewStringParameter("name", "The name of the Dashboard Filter")
	params = append(params, nameParameter)
	titleParameter := parameters.NewStringParameter("title", "The title of the Dashboard Filter")
	params = append(params, titleParameter)
	filterTypeParameter := parameters.NewStringParameter("filter_type", "The filter_type of the Dashboard Filter: date_filter, number_filter, string_filter, field_filter (default field_filter)", parameters.WithStringDefault("field_filter"))
	params = append(params, filterTypeParameter)
	defaultParameter := parameters.NewStringParameter("default_value", "The default_value of the Dashboard Filter (optional)", parameters.WithStringRequired(false))
	params = append(params, defaultParameter)
	modelParameter := parameters.NewStringParameter("model", "The model of a field type Dashboard Filter (required if type field)", parameters.WithStringRequired(false))
	params = append(params, modelParameter)
	exploreParameter := parameters.NewStringParameter("explore", "The explore of a field type Dashboard Filter (required if type field)", parameters.WithStringRequired(false))
	params = append(params, exploreParameter)
	dimensionParameter := parameters.NewStringParameter("dimension", "The dimension of a field type Dashboard Filter (required if type field)", parameters.WithStringRequired(false))
	params = append(params, dimensionParameter)
	multiValueParameter := parameters.NewBooleanParameter("allow_multiple_values", "The Dashboard Filter should allow multiple values (default true)", parameters.WithBooleanDefault(true))
	params = append(params, multiValueParameter)
	requiredParameter := parameters.NewBooleanParameter("required", "The Dashboard Filter is required to run dashboard (default false)", parameters.WithBooleanDefault(false))
	params = append(params, requiredParameter)

	// finish tool setup
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
	logger.DebugContext(ctx, "params = ", params)

	paramsMap := params.AsMap()
	dashboard_id, ok := paramsMap["dashboard_id"].(string)
	if !ok {
		return nil, util.NewAgentError("dashboard_id parameter missing or invalid", nil)
	}
	name, ok := paramsMap["name"].(string)
	if !ok {
		return nil, util.NewAgentError("name parameter missing or invalid", nil)
	}
	title, ok := paramsMap["title"].(string)
	if !ok {
		return nil, util.NewAgentError("title parameter missing or invalid", nil)
	}
	filterType, ok := paramsMap["filter_type"].(string)
	if !ok {
		return nil, util.NewAgentError("filter_type parameter missing or invalid", nil)
	}

	switch filterType {
	case "date_filter":
	case "number_filter":
	case "string_filter":
	case "field_filter":
	default:
		return nil, util.NewAgentError(fmt.Sprintf("invalid filter type: %s. Must be one of date_filter, number_filter, string_filter, field_filter", filterType), nil)
	}

	allowMultipleValues, ok := paramsMap["allow_multiple_values"].(bool)
	if !ok {
		// defaults should handle this, but safe fallback
		allowMultipleValues = true
	}
	required, ok := paramsMap["required"].(bool)
	if !ok {
		required = false
	}

	req := v4.WriteCreateDashboardFilter{
		DashboardId:         dashboard_id,
		Name:                name,
		Title:               title,
		Type:                filterType,
		AllowMultipleValues: &allowMultipleValues,
		Required:            &required,
	}

	if v, ok := paramsMap["default_value"]; ok && v != nil {
		if defaultValue, ok := v.(string); ok {
			req.DefaultValue = &defaultValue
		}
	}

	if filterType == "field_filter" {
		model, ok := paramsMap["model"].(string)
		if !ok || model == "" {
			return nil, util.NewAgentError("model must be specified for field_filter type", nil)
		}
		explore, ok := paramsMap["explore"].(string)
		if !ok || explore == "" {
			return nil, util.NewAgentError("explore must be specified for field_filter type", nil)
		}
		dimension, ok := paramsMap["dimension"].(string)
		if !ok || dimension == "" {
			return nil, util.NewAgentError("dimension must be specified for field_filter type", nil)
		}

		req.Model = &model
		req.Explore = &explore
		req.Dimension = &dimension
	}

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}

	apiSettings, err := source.LookerApiSettings(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting api settings", http.StatusInternalServerError, err)
	}

	resp, err := sdk.CreateDashboardFilter(req, "name", apiSettings)
	if err != nil {
		if strings.Contains(err.Error(), "status=401") {
			return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
		}
		return nil, util.ProcessGeneralError(err)
	}
	logger.DebugContext(ctx, "resp = %v", resp)

	data := make(map[string]any)

	data["result"] = fmt.Sprintf("Dashboard filter \"%s\" added to dashboard %s", *resp.Name, dashboard_id)

	return data, nil
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
