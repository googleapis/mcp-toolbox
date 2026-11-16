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
package lookerupdatedashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

const resourceType string = "looker-update-dashboard"

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

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	dashboardIdParam := parameters.NewStringParameter(
		"dashboard_id",
		"The ID of the dashboard to update.",
	)
	titleParam := parameters.NewStringParameter(
		"title",
		"Optional updated title for the dashboard.",
		parameters.WithStringRequired(false),
	)
	descParam := parameters.NewStringParameter(
		"description",
		"Optional updated description for the dashboard.",
		parameters.WithStringRequired(false),
	)
	showHeaderParam := parameters.NewBooleanParameter(
		"show_dashboard_header",
		"Whether to display Looker's native top header bar (title, filter bar, reload icon, menu). Set to false for full-bleed Data Apps.",
		parameters.WithBooleanRequired(false),
	)
	sideMarginsParam := parameters.NewIntParameter(
		"page_side_margins",
		"The page margin width in pixels (e.g. 0 for full bleed).",
		parameters.WithIntRequired(false),
	)
	spaceBetweenParam := parameters.NewIntParameter(
		"tile_space_between",
		"The spacing between dashboard tiles in pixels (e.g. 0 for full bleed).",
		parameters.WithIntRequired(false),
	)
	tileShadowParam := parameters.NewBooleanParameter(
		"tile_shadow",
		"Whether dashboard tiles display a shadow border (e.g. false for full bleed).",
		parameters.WithBooleanRequired(false),
	)
	appearanceParam := parameters.NewMapParameter(
		"appearance",
		"Optional appearance settings map to customize colors and styling.",
		"",
		parameters.WithMapRequired(false),
	)
	hiddenParam := parameters.NewBooleanParameter(
		"hidden",
		"Whether to hide the dashboard in navigation.",
		parameters.WithBooleanRequired(false),
	)
	refreshIntervalParam := parameters.NewStringParameter(
		"refresh_interval",
		"Auto-refresh interval phrase (e.g. '1 hour', '30 minutes').",
		parameters.WithStringRequired(false),
	)

	params := parameters.Parameters{
		dashboardIdParam,
		titleParam,
		descParam,
		showHeaderParam,
		sideMarginsParam,
		spaceBetweenParam,
		tileShadowParam,
		appearanceParam,
		hiddenParam,
		refreshIntervalParam,
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			lookercommon.DestructiveAnnotations(cfg.Annotations),
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

	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError("unable to get logger from ctx", http.StatusInternalServerError, err)
	}
	logger.DebugContext(ctx, "invoking tool", "params", params)
	paramsMap := params.AsMap()

	dashboardId, ok := paramsMap["dashboard_id"].(string)
	if !ok || dashboardId == "" {
		return nil, util.NewAgentError("dashboard_id parameter missing or invalid", nil)
	}

	appearanceMap := make(map[string]any)
	if rawApp, ok := paramsMap["appearance"].(map[string]any); ok && rawApp != nil {
		for k, v := range rawApp {
			appearanceMap[k] = v
		}
	}
	if showHeader, ok := paramsMap["show_dashboard_header"].(bool); ok {
		appearanceMap["show_dashboard_header"] = showHeader
	}
	if margins, ok := paramsMap["page_side_margins"].(int); ok {
		appearanceMap["page_side_margins"] = margins
	}
	if spaceBetween, ok := paramsMap["tile_space_between"].(int); ok {
		appearanceMap["tile_space_between"] = spaceBetween
	}
	if shadow, ok := paramsMap["tile_shadow"].(bool); ok {
		appearanceMap["tile_shadow"] = shadow
	}

	body := make(map[string]any)
	if title, ok := paramsMap["title"].(string); ok && title != "" {
		body["title"] = title
	}
	if desc, ok := paramsMap["description"].(string); ok && desc != "" {
		body["description"] = desc
	}
	if hidden, ok := paramsMap["hidden"].(bool); ok {
		body["hidden"] = hidden
	}
	if refresh, ok := paramsMap["refresh_interval"].(string); ok && refresh != "" {
		body["refresh_interval"] = refresh
	}
	if len(appearanceMap) > 0 {
		body["appearance"] = appearanceMap
	}

	if len(body) == 0 {
		return nil, util.NewAgentError("no dashboard fields provided to update", nil)
	}

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}

	dashPath := fmt.Sprintf("/dashboards/%s", url.PathEscape(dashboardId))
	var resp map[string]any
	err = sdk.AuthSession.Do(&resp, "PATCH", "/4.0", dashPath, nil, body, source.LookerApiSettings())
	if err != nil {
		if strings.Contains(err.Error(), "status=401") {
			return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
		}
		return nil, util.ProcessGeneralError(err)
	}
	logger.DebugContext(ctx, "dashboard updated", "resp", resp)

	data := make(map[string]any)
	data["result"] = fmt.Sprintf("Dashboard %s updated", dashboardId)
	if id, ok := resp["id"]; ok {
		data["id"] = id
	}
	if title, ok := resp["title"]; ok {
		data["title"] = title
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
