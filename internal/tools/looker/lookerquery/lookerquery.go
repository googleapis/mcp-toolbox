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
package lookerquery

import (
	"context"
	"encoding/json"
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

const resourceType string = "looker-query"

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

func getLookerQueryParameters() parameters.Parameters {
	queryIdParameter := parameters.NewStringParameter(
		"query_id",
		"Optional unique identifier (numeric ID or slug) of a saved Looker query to run. If provided, model, explore, and fields are not required.",
		parameters.WithStringRequired(false),
	)
	modelParameter := parameters.NewStringParameter(
		"model",
		"The model containing the explore (required if query_id is not provided).",
		parameters.WithStringRequired(false),
	)
	exploreParameter := parameters.NewStringParameter(
		"explore",
		"The explore to be queried (required if query_id is not provided).",
		parameters.WithStringRequired(false),
	)
	fieldsParameter := parameters.NewArrayParameter(
		"fields",
		"The fields to be retrieved (required if query_id is not provided).",
		parameters.NewStringParameter("field", "A field to be returned in the query"),
		parameters.WithArrayRequired(false),
	)

	commonParams := lookercommon.GetQueryParameters()

	resultFormatParameter := parameters.NewStringParameter(
		"result_format",
		"The output format: 'json' (raw row array) or 'json_detail' (includes fields, pivots, and data). Default is 'json'.",
		parameters.WithStringDefault("json"),
	)
	visConfigParameter := parameters.NewStringParameter(
		"vis_config",
		"Optional visualization configuration JSON string to associate with the query response.",
		parameters.WithStringDefault(""),
	)
	generateDrillLinksParameter := parameters.NewBooleanParameter(
		"generate_drill_links",
		"Optional flag indicating whether to generate drill links in the query result.",
		parameters.WithBooleanRequired(false),
	)

	allParams := parameters.Parameters{
		queryIdParameter,
		modelParameter,
		exploreParameter,
		fieldsParameter,
	}
	// Append filters, pivots, sorts, limit, tz, filter_expression, dynamic_fields from commonParams
	if len(commonParams) > 3 {
		allParams = append(allParams, commonParams[3:]...)
	}
	allParams = append(
		allParams,
		resultFormatParameter,
		visConfigParameter,
		generateDrillLinksParameter,
	)
	return allParams
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	allParameters := getLookerQueryParameters()

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

// BuildWriteQueryWithOverrides creates a v4.WriteQuery from a base v4.Query,
// merging any provided filterOverrides into the query's base filters and
// applying sortOverrides if non-empty. FilterConfig and ClientId are intentionally
// omitted so Looker does not allow explore UI filter_config to override Filters.
func BuildWriteQueryWithOverrides(baseQuery v4.Query, filterOverrides map[string]any, sortOverrides []string) v4.WriteQuery {
	mergedFilters := make(map[string]any)
	if baseQuery.Filters != nil {
		for k, v := range *baseQuery.Filters {
			mergedFilters[k] = v
		}
	}
	for k, v := range filterOverrides {
		mergedFilters[k] = v
	}

	var sortsPtr *[]string
	if len(sortOverrides) > 0 {
		sCopy := make([]string, len(sortOverrides))
		copy(sCopy, sortOverrides)
		sortsPtr = &sCopy
	} else {
		sortsPtr = baseQuery.Sorts
	}

	return v4.WriteQuery{
		Model:            baseQuery.Model,
		View:             baseQuery.View,
		Fields:           baseQuery.Fields,
		Pivots:           baseQuery.Pivots,
		FillFields:       baseQuery.FillFields,
		Filters:          &mergedFilters,
		FilterExpression: baseQuery.FilterExpression,
		Sorts:            sortsPtr,
		Limit:            baseQuery.Limit,
		ColumnLimit:      baseQuery.ColumnLimit,
		Total:            baseQuery.Total,
		RowTotal:         baseQuery.RowTotal,
		Subtotals:        baseQuery.Subtotals,
		VisConfig:        baseQuery.VisConfig,
		DynamicFields:    baseQuery.DynamicFields,
		QueryTimezone:    baseQuery.QueryTimezone,
	}
}

func sortsMatchBaseQuery(sortOverrides []string, baseSorts *[]string) bool {
	if len(sortOverrides) == 0 {
		return true
	}
	if baseSorts == nil || len(*baseSorts) != len(sortOverrides) {
		return false
	}
	for i := range sortOverrides {
		if strings.TrimSpace(sortOverrides[i]) != strings.TrimSpace((*baseSorts)[i]) {
			return false
		}
	}
	return true
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

	paramsMap := params.AsMap()
	queryId, _ := paramsMap["query_id"].(string)
	queryId = strings.TrimSpace(queryId)

	resultFormat := "json"
	if rf, ok := paramsMap["result_format"].(string); ok && strings.TrimSpace(rf) != "" {
		resultFormat = strings.TrimSpace(rf)
	}

	var visConfigObj any
	if vcStr, ok := paramsMap["vis_config"].(string); ok && strings.TrimSpace(vcStr) != "" {
		var parsed any
		if err := json.Unmarshal([]byte(vcStr), &parsed); err == nil {
			visConfigObj = parsed
		}
	}

	var generateDrillLinks *bool
	if gdl, ok := paramsMap["generate_drill_links"].(bool); ok {
		generateDrillLinks = &gdl
	}

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}

	if queryId != "" {
		var filterOverrides map[string]any
		if fMap, ok := paramsMap["filters"].(map[string]any); ok && len(fMap) > 0 {
			filterOverrides = make(map[string]any, len(fMap))
			for k, v := range fMap {
				newKey := k
				if len(k) >= 2 && (k[0] == '\'' || k[0] == '"') && k[0] == k[len(k)-1] {
					newKey = k[1 : len(k)-1]
				}
				newVal := v
				if s, ok := v.(string); ok && len(s) >= 2 &&
					(s[0] == '\'' || s[0] == '"') && s[0] == s[len(s)-1] {
					newVal = s[1 : len(s)-1]
				}
				filterOverrides[newKey] = newVal
			}
		}

		var sortOverrides []string
		if sSlice, ok := paramsMap["sorts"].([]any); ok && len(sSlice) > 0 {
			if converted, err := parameters.ConvertAnySliceToTyped(sSlice, "string"); err == nil {
				sortOverrides = converted.([]string)
			}
		}

		hasOverrides := len(filterOverrides) > 0 || len(sortOverrides) > 0

		var resp string
		var rErr error

		if hasOverrides {
			baseQuery, qErr := sdk.Query(queryId, "", source.LookerApiSettings())
			if qErr != nil {
				logger.DebugContext(ctx, "sdk.Query failed, attempting QueryForSlug", "query_id", queryId, "error", qErr)
				baseQuery, qErr = sdk.QueryForSlug(queryId, "", source.LookerApiSettings())
			}
			if qErr != nil {
				if strings.Contains(qErr.Error(), "status=401") {
					return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, qErr)
				}
				return nil, util.ProcessGeneralError(lookercommon.ExtractLookerErrorMessage(qErr))
			}

			if len(filterOverrides) == 0 && sortsMatchBaseQuery(sortOverrides, baseQuery.Sorts) {
				resp, rErr = lookercommon.RunSavedQueryWithDrillLinks(ctx, sdk, queryId, resultFormat, generateDrillLinks, source.LookerApiSettings())
				if rErr != nil {
					wq := BuildWriteQueryWithOverrides(baseQuery, nil, nil)
					resp, rErr = lookercommon.RunInlineQueryWithDrillLinks(ctx, sdk, &wq, resultFormat, generateDrillLinks, source.LookerApiSettings())
				}
			} else {
				wq := BuildWriteQueryWithOverrides(baseQuery, filterOverrides, sortOverrides)
				if escErr := lookercommon.EscapeUnquotedParameterFilters(ctx, sdk, &wq, source.LookerApiSettings()); escErr != nil {
					logger.WarnContext(ctx, "skipping unquoted-parameter escape, metadata lookup failed", "error", escErr)
				}
				resp, rErr = lookercommon.RunInlineQueryWithDrillLinks(ctx, sdk, &wq, resultFormat, generateDrillLinks, source.LookerApiSettings())
			}
		} else {
			resp, rErr = lookercommon.RunSavedQueryWithDrillLinks(ctx, sdk, queryId, resultFormat, generateDrillLinks, source.LookerApiSettings())
			if rErr != nil {
				logger.WarnContext(ctx, "RunSavedQuery failed, attempting slug lookup", "query_id", queryId, "error", rErr)

				slugQuery, qErr := sdk.QueryForSlug(queryId, "", source.LookerApiSettings())
				if qErr == nil {
					wq := BuildWriteQueryWithOverrides(slugQuery, nil, nil)
					resp, rErr = lookercommon.RunInlineQueryWithDrillLinks(ctx, sdk, &wq, resultFormat, generateDrillLinks, source.LookerApiSettings())
				}
			}
		}

		if rErr != nil {
			if strings.Contains(rErr.Error(), "status=401") {
				return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, rErr)
			}
			return nil, util.ProcessGeneralError(lookercommon.ExtractLookerErrorMessage(rErr))
		}

		if resultFormat == "json_detail" {
			var detailResp map[string]any
			if err := json.Unmarshal([]byte(resp), &detailResp); err != nil {
				return nil, util.NewClientServerError("error unmarshaling json_detail response", http.StatusInternalServerError, err)
			}
			if generateDrillLinks != nil && !*generateDrillLinks {
				lookercommon.StripDrillLinks(detailResp)
			}

			result := map[string]any{
				"data":        detailResp["data"],
				"fields":      detailResp["fields"],
				"pivots":      detailResp["pivots"],
				"totals_data": detailResp["totals_data"],
				"query_id":    queryId,
				"status":      "success",
			}
			if errs, ok := detailResp["errors"]; ok && errs != nil {
				result["errors"] = errs
			}
			if visConfigObj != nil {
				result["vis_config"] = visConfigObj
			}
			return result, nil
		}

		var rawData []any
		if err := json.Unmarshal([]byte(resp), &rawData); err != nil {
			return nil, util.NewClientServerError("error unmarshaling query response", http.StatusInternalServerError, err)
		}
		if visConfigObj != nil {
			return map[string]any{
				"data":       rawData,
				"query_id":   queryId,
				"status":     "success",
				"vis_config": visConfigObj,
			}, nil
		}
		return rawData, nil
	}

	// Inline query path (query_id is empty)
	modelVal, _ := paramsMap["model"].(string)
	if strings.TrimSpace(modelVal) == "" {
		return nil, util.NewAgentError("model is required when query_id is not provided", nil)
	}
	exploreVal, _ := paramsMap["explore"].(string)
	if strings.TrimSpace(exploreVal) == "" {
		return nil, util.NewAgentError("explore is required when query_id is not provided", nil)
	}
	fieldsSlice, _ := paramsMap["fields"].([]any)
	if len(fieldsSlice) == 0 {
		return nil, util.NewAgentError("fields is required when query_id is not provided", nil)
	}

	wq, err := lookercommon.ProcessQueryArgs(ctx, params)
	if err != nil {
		return nil, util.NewAgentError("error building WriteQuery request", err)
	}
	if escErr := lookercommon.EscapeUnquotedParameterFilters(ctx, sdk, wq, source.LookerApiSettings()); escErr != nil {
		logger.WarnContext(ctx, "skipping unquoted-parameter escape, metadata lookup failed", "error", escErr)
	}
	resp, err := lookercommon.RunInlineQueryWithDrillLinks(ctx, sdk, wq, resultFormat, generateDrillLinks, source.LookerApiSettings())
	if err != nil {
		if strings.Contains(err.Error(), "status=401") {
			return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
		}
		return nil, util.ProcessGeneralError(err)
	}

	logger.DebugContext(ctx, "resp = ", resp)

	if resultFormat == "json_detail" {
		var detailResp map[string]any
		if err := json.Unmarshal([]byte(resp), &detailResp); err != nil {
			return nil, util.NewClientServerError("error unmarshaling json_detail response", http.StatusInternalServerError, err)
		}
		if generateDrillLinks != nil && !*generateDrillLinks {
			lookercommon.StripDrillLinks(detailResp)
		}
		result := map[string]any{
			"data":        detailResp["data"],
			"fields":      detailResp["fields"],
			"pivots":      detailResp["pivots"],
			"totals_data": detailResp["totals_data"],
			"status":      "success",
		}
		if errs, ok := detailResp["errors"]; ok && errs != nil {
			result["errors"] = errs
		}
		if visConfigObj != nil {
			result["vis_config"] = visConfigObj
		}
		return result, nil
	}

	var data []any
	e := json.Unmarshal([]byte(resp), &data)
	if e != nil {
		return nil, util.NewClientServerError("error unmarshaling query response", http.StatusInternalServerError, e)
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

func (t Tool) GetAuthTokenHeaderName(source sources.Source) (string, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return "", fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.GetAuthTokenHeaderName(), nil
}
