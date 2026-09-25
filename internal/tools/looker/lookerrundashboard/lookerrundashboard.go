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
package lookerrundashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"

	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

const resourceType string = "looker-run-dashboard"

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

	dashboardidParameter := parameters.NewStringParameter("dashboard_id", "The id of the dashboard to run.")

	allParameters := parameters.Parameters{
		dashboardidParameter,
	}

	// finish tool setup
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			lookercommon.ReadOnlyAnnotations(cfg.Annotations),
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

	dashboard_id := paramsMap["dashboard_id"].(string)

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}

	apiSettings, err := source.LookerApiSettings(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting api settings", http.StatusInternalServerError, err)
	}
	dashboard, err := sdk.Dashboard(dashboard_id, "", apiSettings)
	if err != nil {
		if strings.Contains(err.Error(), "status=401") {
			return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
		}
		return nil, util.ProcessGeneralError(err)
	}

	data := make(map[string]any)
	data["tiles"] = make([]any, 0)
	if dashboard.Title != nil {
		data["title"] = *dashboard.Title
	}
	if dashboard.Description != nil {
		data["description"] = *dashboard.Description
	}

	channels := make([]<-chan map[string]any, len(*dashboard.DashboardElements))
	for i, element := range *dashboard.DashboardElements {
		channels[i] = tileQueryWorker(ctx, sdk, apiSettings, i, element)
	}

	for resp := range merge(channels...) {
		data["tiles"] = append(data["tiles"].([]any), resp)
	}

	logger.DebugContext(ctx, "data = ", data)

	return data, nil
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}

func tileQueryWorker(ctx context.Context, sdk *v4.LookerSDK, options *rtl.ApiSettings, index int, element v4.DashboardElement) <-chan map[string]any {
	out := make(chan map[string]any)

	go func() {
		defer close(out)

		data := make(map[string]any)
		data["index"] = index
		if element.Title != nil {
			data["title"] = *element.Title
		}
		if element.TitleText != nil {
			data["title_text"] = *element.TitleText
		}
		if element.SubtitleText != nil {
			data["subtitle_text"] = *element.SubtitleText
		}
		if element.BodyText != nil {
			data["body_text"] = *element.BodyText
		}

		// Check for SQL query
		if element.ResultMaker != nil && element.ResultMaker.SqlQueryId != nil && *element.ResultMaker.SqlQueryId != "" {
			sqlQueryId := *element.ResultMaker.SqlQueryId
			data["element_type"] = "sql_query"
			queryResult, err := sdk.RunSqlQuery(sqlQueryId, "json", "", options)
			if err != nil {
				data["query_status"] = fmt.Sprintf("error running SQL query %s: %s", sqlQueryId, err)
				out <- data
				return
			}

			var resp []any
			if err := json.Unmarshal([]byte(queryResult), &resp); err != nil {
				data["query_status"] = fmt.Sprintf("error unmarshaling SQL query %s result: %s", sqlQueryId, err)
				out <- data
				return
			}

			data["query_status"] = "success"
			data["query_result"] = resp
			out <- data
			return
		}

		// Check for Merge query
		if element.ResultMaker != nil && element.ResultMaker.MergeResultId != nil && *element.ResultMaker.MergeResultId != "" {
			data["element_type"] = "merge_result"
			mergeQuery, err := sdk.MergeQuery(*element.ResultMaker.MergeResultId, "", options)
			if err != nil {
				data["query_status"] = fmt.Sprintf("error getting merge query %s: %s", *element.ResultMaker.MergeResultId, err)
				out <- data
				return
			}
			data["parts"] = make([]any, 0)

			for i, sourceQuery := range *mergeQuery.SourceQueries {
				partData := make(map[string]any)
				partData["index"] = i
				if sourceQuery.Name != nil {
					partData["name"] = *sourceQuery.Name
				}
				if sourceQuery.MergeFields != nil {
					mar, err := json.Marshal(*sourceQuery.MergeFields)
					if err == nil {
						mergeFields := make([]any, 0)
						if err := json.Unmarshal(mar, &mergeFields); err == nil {
							partData["merge_fields"] = mergeFields
						} else {
							partData["merge_fields"] = fmt.Sprintf("error marshaling merge fields: %s", err)
						}
					} else {
						partData["merge_fields"] = fmt.Sprintf("error unmarshaling merge fields: %s", err)
					}
				}
				q, err := sdk.QueryForSlug(*sourceQuery.QuerySlug, "", options)
				if err != nil {
					partData["query_status"] = fmt.Sprintf("error getting query for slug %s: %s", *sourceQuery.QuerySlug, err)
				} else {
					runQuery(ctx, sdk, q, options, partData)
				}
				data["parts"] = append(data["parts"].([]any), partData)
			}
			out <- data
			return
		}

		var q v4.Query
		if element.ResultMaker != nil && element.ResultMaker.Query != nil {
			data["element_type"] = "query"
			q = *element.ResultMaker.Query
		} else if element.Query != nil {
			data["element_type"] = "query"
			q = *element.Query
		} else if element.Look != nil {
			data["element_type"] = "look"
			q = *element.Look.Query
		} else {
			// Just a text element
			data["element_type"] = "text"
			out <- data
			return
		}

		runQuery(ctx, sdk, q, options, data)
		out <- data
	}()
	return out
}

func runQuery(ctx context.Context, sdk *v4.LookerSDK, q v4.Query, options *rtl.ApiSettings, data map[string]any) {
	wq := v4.WriteQuery{
		Model:         q.Model,
		View:          q.View,
		Fields:        q.Fields,
		Pivots:        q.Pivots,
		Filters:       q.Filters,
		Sorts:         q.Sorts,
		QueryTimezone: q.QueryTimezone,
		Limit:         q.Limit,
	}
	query_result, err := lookercommon.RunInlineQuery(ctx, sdk, &wq, "json", options)
	if err != nil {
		data["query_status"] = fmt.Sprintf("error running query: %s", err)
		return
	}
	var resp []any
	e := json.Unmarshal([]byte(query_result), &resp)
	if e != nil {
		data["query_status"] = fmt.Sprintf("error parsing query result: %s", e)
		return
	}
	data["query_status"] = "success"
	data["query_result"] = resp
}

func merge(channels ...<-chan map[string]any) <-chan map[string]any {
	var wg sync.WaitGroup
	out := make(chan map[string]any)

	output := func(c <-chan map[string]any) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}
	wg.Add(len(channels))
	for _, c := range channels {
		go output(c)
	}

	// Start a goroutine to close out once all the output goroutines are
	// done.  This must start after the wg.Add call.
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func (t Tool) GetAuthTokenHeaderName(source sources.Source) (string, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return "", fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.GetAuthTokenHeaderName(), nil
}
