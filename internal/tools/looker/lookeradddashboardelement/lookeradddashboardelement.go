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
package lookeradddashboardelement

import (
	"context"
	"fmt"
	"net/http"
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

const resourceType string = "looker-add-dashboard-element"

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

	params := lookercommon.GetQueryParameters()

	dashIdParameter := parameters.NewStringParameter("dashboard_id", "The id of the dashboard where this tile will exist")
	params = append(params, dashIdParameter)
	titleParameter := parameters.NewStringParameter("title", "The title of the Dashboard Element", parameters.WithStringDefault(""))
	params = append(params, titleParameter)
	vizParameter := parameters.NewMapParameter(
		"vis_config",
		"The visualization config for the query",
		"",
		parameters.WithMapDefault(map[string]any{}),
	)

	params = append(params, vizParameter)
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

var (
	dataType string = "data"
	visType  string = "vis"
)

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

	wq, err := lookercommon.ProcessQueryArgs(ctx, params)
	if err != nil {
		return nil, util.NewAgentError("error building query request", err)
	}

	paramsMap := params.AsMap()

	dashboard_id, ok := paramsMap["dashboard_id"].(string)
	if !ok {
		return nil, util.NewAgentError("dashboard_id parameter missing or invalid", nil)
	}

	title, ok := paramsMap["title"].(string)
	if !ok {
		title = ""
	}

	visConfig, ok := paramsMap["vis_config"].(map[string]any)
	if !ok {
		visConfig = make(map[string]any)
	}
	wq.VisConfig = &visConfig

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}

	apiSettings, err := source.LookerApiSettings(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting api settings", http.StatusInternalServerError, err)
	}
	if escErr := lookercommon.EscapeUnquotedParameterFilters(ctx, sdk, wq, apiSettings); escErr != nil {
		logger.WarnContext(ctx, "skipping unquoted-parameter escape, metadata lookup failed", "error", escErr)
	}

	qresp, err := sdk.CreateQuery(*wq, "id", apiSettings)
	if err != nil {
		if strings.Contains(err.Error(), "status=401") {
			return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
		}
		return nil, util.ProcessGeneralError(err)
	}

	dashFilters := []any{}
	if v, ok := paramsMap["dashboard_filters"]; ok {
		if v != nil {
			if df, ok := v.([]any); ok {
				dashFilters = df
			}
		}
	}

	var filterables []v4.ResultMakerFilterables
	for _, m := range dashFilters {
		f, ok := m.(map[string]any)
		if !ok {
			return nil, util.NewAgentError("invalid dashboard filter structure", nil)
		}
		name, ok := f["dashboard_filter_name"].(string)
		if !ok {
			return nil, util.NewAgentError("error processing dashboard filter: missing dashboard_filter_name", nil)
		}
		field, ok := f["field"].(string)
		if !ok {
			return nil, util.NewAgentError("error processing dashboard filter: missing field", nil)
		}
		listener := v4.ResultMakerFilterablesListen{
			DashboardFilterName: &name,
			Field:               &field,
		}
		listeners := []v4.ResultMakerFilterablesListen{listener}

		filter := v4.ResultMakerFilterables{
			Listen: &listeners,
		}

		filterables = append(filterables, filter)
	}

	if len(filterables) == 0 {
		filterables = nil
	}

	wrm := v4.WriteResultMakerWithIdVisConfigAndDynamicFields{
		Query:       wq,
		VisConfig:   &visConfig,
		Filterables: &filterables,
	}
	wde := v4.WriteDashboardElement{
		DashboardId: &dashboard_id,
		Title:       &title,
		ResultMaker: &wrm,
		Query:       wq,
		QueryId:     qresp.Id,
	}

	switch len(visConfig) {
	case 0:
		wde.Type = &dataType
	default:
		wde.Type = &visType
	}

	fields := ""

	req := v4.RequestCreateDashboardElement{
		Body:   wde,
		Fields: &fields,
	}

	resp, err := sdk.CreateDashboardElement(req, apiSettings)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	logger.DebugContext(ctx, "resp = %v", resp)

	data := make(map[string]any)

	data["result"] = fmt.Sprintf("Dashboard element added to dashboard %s", dashboard_id)

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
