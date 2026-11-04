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
package lookerupdatedashboardelement

import (
	"context"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"

	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

const resourceType string = "looker-update-dashboard-element"

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

	params := lookercommon.GetDashboardElementQueryParameters()

	dashboardIdParam := parameters.NewStringParameter("dashboard_id", "The ID of the dashboard containing the element.")
	params = append(params, dashboardIdParam)
	elementIdParam := parameters.NewStringParameter("dashboard_element_id", "The ID of the dashboard element to update.")
	params = append(params, elementIdParam)
	titleParam := parameters.NewStringParameter("title", "The new title of the element.", parameters.WithStringDefault(""))
	params = append(params, titleParam)
	visConfigParam := parameters.NewMapParameter("vis_config", "The new visualization configuration.", "", parameters.WithMapDefault(map[string]any{}))
	params = append(params, visConfigParam)

	dashFilters := parameters.NewArrayParameter(
		"dashboard_filters",
		`An array of dashboard filters like [{"dashboard_filter_name": "name", "field": "view_name.field_name"}, ...]`,
		parameters.NewMapParameter(
			"dashboard_filter",
			`A dashboard filter like {"dashboard_filter_name": "name", "field": "view_name.field_name"}`,
			"",
			parameters.WithMapDefault(map[string]any{}),
		),
		parameters.WithArrayRequired(false),
	)
	params = append(params, dashFilters)
	params = append(params, lookercommon.GetDashboardElementParameters()...)

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			lookercommon.DestructiveAnnotations(cfg.Annotations),
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

	dashboardElementId, ok := paramsMap["dashboard_element_id"].(string)
	if !ok || dashboardElementId == "" {
		return nil, util.NewAgentError("dashboard_element_id parameter missing or invalid", nil)
	}

	dashboardId, ok := paramsMap["dashboard_id"].(string)
	if !ok || dashboardId == "" {
		return nil, util.NewAgentError("dashboard_id parameter missing or invalid", nil)
	}

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}

	wde, toolboxErr := lookercommon.BuildWriteDashboardElement(ctx, sdk, dashboardId, params, source.LookerApiSettings())
	if toolboxErr != nil {
		return nil, toolboxErr
	}

	resp, err := sdk.UpdateDashboardElement(dashboardElementId, wde, "", source.LookerApiSettings())
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	logger.DebugContext(ctx, "resp = %v", resp)

	data := make(map[string]any)
	data["result"] = fmt.Sprintf("Dashboard element %s updated", dashboardElementId)
	if resp.Id != nil {
		data["id"] = *resp.Id
	}
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
