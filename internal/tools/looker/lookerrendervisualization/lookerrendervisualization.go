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
package lookerrendervisualization

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"

	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

const (
	resourceType        string = "looker-render-visualization"
	defaultResourceName string = "looker_render_visualization_ui"
	defaultResourceURI  string = "ui://looker/render_visualization.html"
)

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

func getVisualizationParameters() parameters.Parameters {
	queryIdParameter := parameters.NewStringParameter(
		"query_id",
		"Optional Looker query ID (or slug) to fetch and render the saved visualization directly. If provided, model, explore, and fields are not required.",
		parameters.WithStringRequired(false),
	)
	modelParameter := parameters.NewStringParameter(
		"model",
		"The model containing the explore (from get_models). Required if query_id is not provided.",
		parameters.WithStringRequired(false),
	)
	exploreParameter := parameters.NewStringParameter(
		"explore",
		"The explore to be queried (from get_explores). Required if query_id is not provided.",
		parameters.WithStringRequired(false),
	)
	fieldsParameter := parameters.NewArrayParameter(
		"fields",
		"The fields to be retrieved (from get_dimensions, get_measures, get_filters, or get_parameters). Required if query_id is not provided. For charts comparing a metric across categories or time (such as bar, column, line, area, pie, or scatter charts), `fields` MUST include BOTH the grouping dimension(s) AND the measure(s) in a single call (for example, [\"products.department\", \"order_items.count\"]). Passing only a single field to a bar/column/line chart will fail with 'Not Enough Fields'.",
		parameters.NewStringParameter("field", "A field to be returned in the query"),
		parameters.WithArrayRequired(false),
	)
	filtersParameter := parameters.NewMapParameter(
		"filters",
		"The filters for the query.",
		"",
		parameters.WithMapDefault(map[string]any{}),
	)
	pivotsParameter := parameters.NewArrayParameter(
		"pivots",
		"The query pivots. Any pivot field omitted from `fields` is automatically appended to `fields`.",
		parameters.NewStringParameter("pivot_field", "A field to be used as a pivot in the query"),
		parameters.WithArrayDefault([]any{}),
	)
	sortsParameter := parameters.NewArrayParameter(
		"sorts",
		"The sorts like \"field.id desc 0\".",
		parameters.NewStringParameter("sort_field", "A field to be used as a sort in the query"),
		parameters.WithArrayDefault([]any{}),
	)
	limitParameter := parameters.NewIntParameter("limit", "The row limit.", parameters.WithIntRequired(false))
	tzParameter := parameters.NewStringParameter("tz", "The query timezone.", parameters.WithStringDefault(""))
	filterExpressionParameter := parameters.NewStringParameter("filter_expression", "An optional filter expression string.", parameters.WithStringDefault(""))
	dynamicFieldsParameter := parameters.NewArrayParameter(
		"dynamic_fields",
		"An optional array of dynamic fields (table calculations, custom measures, custom dimensions).",
		parameters.NewMapParameter("dynamic_field", "A dynamic field definition", ""),
		parameters.WithArrayDefault([]any{}),
	)
	visConfigParameter := parameters.NewStringParameter(
		"vis_config",
		"Optional JSON string specifying the visualization configuration (e.g. chart type, options). Use \"looker_column\" for vertical column charts and \"looker_bar\" for horizontal bar charts.",
		parameters.WithStringDefault(""),
	)

	return parameters.Parameters{
		queryIdParameter,
		modelParameter,
		exploreParameter,
		fieldsParameter,
		filtersParameter,
		pivotsParameter,
		sortsParameter,
		limitParameter,
		tzParameter,
		filterExpressionParameter,
		dynamicFieldsParameter,
		visConfigParameter,
	}
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	lookercommon.EnsureDefaultToolUIMetadata(&cfg.UI, defaultResourceName)

	allParameters := getVisualizationParameters()
	uiMgr := lookercommon.NewLookerUIResourceManager(
		cfg.UI.Resource,
		defaultResourceURI,
		"Looker visualization rendering interface",
		"render_visualization",
		"render_dashboard",
		"visualization",
	)

	toolInstance := Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			lookercommon.ReadOnlyAnnotations(cfg.Annotations),
			tools.Manifest{
				Description:  cfg.Description,
				Parameters:   allParameters.Manifest(),
				AuthRequired: cfg.AuthRequired,
			},
			allParameters,
		),
		uiMgr: uiMgr,
	}

	return toolInstance, nil
}

// validate interfaces
var (
	_ tools.Tool                 = Tool{}
	_ resources.ResourceProvider = Tool{}
)

type Tool struct {
	tools.BaseTool[Config]
	uiMgr *lookercommon.LookerUIResourceManager
}

func (t Tool) GetResources() []resources.Resource {
	if t.uiMgr == nil {
		return nil
	}
	return t.uiMgr.GetResources()
}

func (t Tool) GetSourceName() string {
	return t.Cfg.Source
}

func (t Tool) ToConfig() tools.ToolConfig {
	return t.Cfg
}

func (t Tool) ValidateSource(source sources.Source) error {
	cs, ok := source.(compatibleSource)
	if !ok {
		return fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	if t.uiMgr != nil {
		t.uiMgr.SetSource(cs)
	}
	return nil
}

var dynamicFieldExprRefRe = regexp.MustCompile(`\$\{([a-zA-Z0-9_.]+)\}`)

func parseDynamicFieldsForMeta(raw *string) []any {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	var parsed []any
	if err := json.Unmarshal([]byte(*raw), &parsed); err == nil && len(parsed) > 0 {
		return parsed
	}
	return nil
}

func extractHiddenFieldNames(rawVisConfig any, out *[]string) {
	m, ok := rawVisConfig.(map[string]any)
	if !ok || m == nil {
		return
	}
	rawHidden, exists := m["hidden_fields"]
	if !exists || rawHidden == nil {
		return
	}
	switch v := rawHidden.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				*out = append(*out, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				*out = append(*out, strings.TrimSpace(s))
			}
		}
	}
}

// sanitizeWriteQueryFields removes table_calculation names from wq.Fields (since
// Looker's CreateQuery and RunInlineQuery reject table calculation names inside
// fields) while ensuring any underlying LookML or custom SQL fields referenced
// by wq.DynamicFields expressions (${view.field}) or hidden in visConfig.hidden_fields
// remain present in wq.Fields.
func sanitizeWriteQueryFields(wq *v4.WriteQuery, queryDef *v4.Query, visConfigObj any) {
	if wq == nil || wq.Fields == nil {
		return
	}

	tableCalcNames := make(map[string]bool)
	customSQLFieldNames := make(map[string]bool)
	var requiredFields []string

	if wq.DynamicFields != nil && strings.TrimSpace(*wq.DynamicFields) != "" {
		var parsedDyn []map[string]any
		if err := json.Unmarshal([]byte(*wq.DynamicFields), &parsedDyn); err == nil {
			for _, entry := range parsedDyn {
				if tcName, ok := entry["table_calculation"].(string); ok && strings.TrimSpace(tcName) != "" {
					tableCalcNames[strings.TrimSpace(tcName)] = true
					continue
				}
				if dimName, ok := entry["dimension"].(string); ok && strings.TrimSpace(dimName) != "" {
					customSQLFieldNames[strings.TrimSpace(dimName)] = true
				}
				if measName, ok := entry["measure"].(string); ok && strings.TrimSpace(measName) != "" {
					customSQLFieldNames[strings.TrimSpace(measName)] = true
				}
			}
			for _, entry := range parsedDyn {
				if expr, ok := entry["expression"].(string); ok && expr != "" {
					matches := dynamicFieldExprRefRe.FindAllStringSubmatch(expr, -1)
					for _, match := range matches {
						if len(match) >= 2 {
							ref := strings.TrimSpace(match[1])
							if ref != "" && !tableCalcNames[ref] && (strings.Contains(ref, ".") || customSQLFieldNames[ref]) {
								requiredFields = append(requiredFields, ref)
							}
						}
					}
				}
			}
		}
	}

	extractHiddenFieldNames(visConfigObj, &requiredFields)
	if queryDef != nil && queryDef.VisConfig != nil {
		extractHiddenFieldNames(*queryDef.VisConfig, &requiredFields)
	}

	seen := make(map[string]bool)
	cleaned := make([]string, 0, len(*wq.Fields)+len(requiredFields))
	for _, field := range *wq.Fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" || tableCalcNames[trimmed] || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		cleaned = append(cleaned, trimmed)
	}
	for _, reqField := range requiredFields {
		if reqField == "" || tableCalcNames[reqField] || seen[reqField] {
			continue
		}
		seen[reqField] = true
		cleaned = append(cleaned, reqField)
	}
	wq.Fields = &cleaned
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
	paramMap := params.AsMap()
	queryIdVal, _ := paramMap["query_id"].(string)
	queryIdVal = strings.TrimSpace(queryIdVal)
	visConfigVal := paramMap["vis_config"]

	if queryIdVal == "" {
		modelVal, _ := paramMap["model"].(string)
		if strings.TrimSpace(modelVal) == "" {
			return nil, util.NewAgentError("model is required when query_id is not provided; use get_models to discover available models and retry", nil)
		}
		exploreVal, _ := paramMap["explore"].(string)
		if strings.TrimSpace(exploreVal) == "" {
			return nil, util.NewAgentError("explore is required when query_id is not provided; use get_explores to discover available explores and retry", nil)
		}
		fieldsSlice, _ := paramMap["fields"].([]any)
		if len(fieldsSlice) == 0 {
			return nil, util.NewAgentError("fields is required when query_id is not provided; use get_dimensions and get_measures to discover available fields and retry", nil)
		}
	}

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}

	var (
		appState   map[string]any
		appStateWg sync.WaitGroup
	)
	appStateWg.Add(1)
	go func() {
		defer appStateWg.Done()
		appState = lookercommon.FetchMcpAppState(ctx, sdk, source.LookerApiSettings())
	}()

	var visConfigObj any
	if visConfigVal != nil && visConfigVal != "" {
		if str, ok := visConfigVal.(string); ok {
			var parsed any
			if json.Unmarshal([]byte(str), &parsed) == nil {
				visConfigObj = parsed
			} else {
				visConfigObj = str
			}
		} else {
			visConfigObj = visConfigVal
		}
	}

	// 1. If query_id is provided, fetch query by ID/slug and run it (merging parameter overrides if provided)
	if queryIdVal != "" {
		queryDef, qErr := lookercommon.GetQuery(ctx, sdk, queryIdVal, source.LookerApiSettings())
		if qErr != nil {
			queryDef, qErr = lookercommon.GetQueryBySlug(ctx, sdk, queryIdVal, source.LookerApiSettings())
			if qErr != nil {
				logger.WarnContext(ctx, "could not fetch query definition by id or slug", "error", qErr)
			}
		}

		var wq *v4.WriteQuery
		hasOverrides := false

		if queryDef != nil && queryDef.Model != "" && queryDef.View != "" {
			wq = &v4.WriteQuery{
				Model:            queryDef.Model,
				View:             queryDef.View,
				Fields:           queryDef.Fields,
				Filters:          queryDef.Filters,
				Pivots:           queryDef.Pivots,
				Sorts:            queryDef.Sorts,
				Limit:            queryDef.Limit,
				FilterExpression: queryDef.FilterExpression,
				DynamicFields:    queryDef.DynamicFields,
				QueryTimezone:    queryDef.QueryTimezone,
			}

			// Apply parameter overrides if provided by caller
			if limitVal, ok := paramMap["limit"].(int); ok && limitVal > 0 {
				limitStr := fmt.Sprintf("%d", limitVal)
				wq.Limit = &limitStr
				hasOverrides = true
			}
			if sortsSlice, ok := paramMap["sorts"].([]any); ok && len(sortsSlice) > 0 {
				if s, sErr := parameters.ConvertAnySliceToTyped(sortsSlice, "string"); sErr == nil {
					sorts := s.([]string)
					wq.Sorts = &sorts
					hasOverrides = true
				}
			}
			if fieldsSlice, ok := paramMap["fields"].([]any); ok && len(fieldsSlice) > 0 {
				if f, fErr := parameters.ConvertAnySliceToTyped(fieldsSlice, "string"); fErr == nil {
					fields := f.([]string)
					wq.Fields = &fields
					hasOverrides = true
				}
			}
			if pivotsSlice, ok := paramMap["pivots"].([]any); ok && len(pivotsSlice) > 0 {
				if p, pErr := parameters.ConvertAnySliceToTyped(pivotsSlice, "string"); pErr == nil {
					pivots := p.([]string)
					var currentFields []string
					if wq.Fields != nil {
						currentFields = *wq.Fields
					}
					updatedFields, pivErr := lookercommon.EnsurePivotsInFields(currentFields, pivots)
					if pivErr != nil {
						return nil, util.NewAgentError(pivErr.Error(), pivErr)
					}
					wq.Fields = &updatedFields
					wq.Pivots = &pivots
					hasOverrides = true
				}
			}
			if modelVal, _ := paramMap["model"].(string); modelVal != "" {
				wq.Model = modelVal
				hasOverrides = true
			}
			if exploreVal, _ := paramMap["explore"].(string); exploreVal != "" {
				wq.View = exploreVal
				hasOverrides = true
			}
			if filtersMap, ok := paramMap["filters"].(map[string]any); ok && len(filtersMap) > 0 {
				var knownFields []string
				if wq.Fields != nil {
					knownFields = *wq.Fields
				}
				normalizedOverrides := lookercommon.NormalizeFilterKeys(filtersMap, knownFields, wq.View)
				mergedFilters := make(map[string]any)
				if queryDef.Filters != nil {
					for k, v := range *queryDef.Filters {
						mergedFilters[k] = v
					}
				}
				for k, v := range normalizedOverrides {
					mergedFilters[k] = v
				}
				wq.Filters = &mergedFilters
				hasOverrides = true
			}
			if tzVal, _ := paramMap["tz"].(string); tzVal != "" {
				wq.QueryTimezone = &tzVal
				hasOverrides = true
			}
			if feVal, _ := paramMap["filter_expression"].(string); feVal != "" {
				wq.FilterExpression = &feVal
				hasOverrides = true
			}
			if dfSlice, ok := paramMap["dynamic_fields"].([]any); ok && len(dfSlice) > 0 {
				if jsonBytes, mErr := json.Marshal(dfSlice); mErr == nil {
					jsonStr := string(jsonBytes)
					wq.DynamicFields = &jsonStr
					hasOverrides = true
				}
			}
		}

		var resp string
		var rErr error

		if wq != nil {
			if visConfigObj != nil {
				if m, ok := visConfigObj.(map[string]any); ok {
					mergedVisConfig := make(map[string]any)
					if queryDef != nil && queryDef.VisConfig != nil {
						for k, v := range *queryDef.VisConfig {
							mergedVisConfig[k] = v
						}
					}
					for k, v := range m {
						mergedVisConfig[k] = v
					}
					wq.VisConfig = &mergedVisConfig
					visConfigObj = mergedVisConfig
					hasOverrides = true
				}
			}

			sanitizeWriteQueryFields(wq, queryDef, visConfigObj)

			if escErr := lookercommon.EscapeUnquotedParameterFilters(ctx, sdk, wq, source.LookerApiSettings()); escErr != nil {
				logger.WarnContext(ctx, "skipping unquoted-parameter escape, metadata lookup failed", "error", escErr)
			}

			// Execute the modified query inline without calling sdk.CreateQuery so
			// read-only overrides do not persist new rows in Looker's database.
			generateDrillLinks := false
			resp, rErr = lookercommon.RunInlineQueryWithDrillLinks(ctx, sdk, wq, "json_detail", &generateDrillLinks, source.LookerApiSettings())
			if rErr != nil && !hasOverrides {
				logger.WarnContext(ctx, "error running inline query from query definition, falling back to saved query", "error", rErr)
				resp, rErr = lookercommon.RunSavedQuery(ctx, sdk, queryIdVal, "json_detail", &generateDrillLinks, source.LookerApiSettings())
			}
		} else {
			generateDrillLinks := false
			resp, rErr = lookercommon.RunSavedQuery(ctx, sdk, queryIdVal, "json_detail", &generateDrillLinks, source.LookerApiSettings())
		}
		if rErr != nil {
			if strings.Contains(rErr.Error(), "status=401") {
				return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, rErr)
			}
			return nil, util.ProcessGeneralError(lookercommon.ExtractLookerErrorMessage(rErr))
		}

		var detailResp map[string]any
		if err := json.Unmarshal([]byte(resp), &detailResp); err != nil {
			return nil, util.NewClientServerError("error unmarshaling json_detail response", http.StatusInternalServerError, err)
		}
		if wq != nil && wq.Pivots != nil && len(*wq.Pivots) > 0 {
			if pivErr := lookercommon.ValidatePivotsInDetailResponse(*wq.Pivots, detailResp); pivErr != nil {
				return nil, util.NewAgentError(pivErr.Error(), pivErr)
			}
		}
		lookercommon.StripDrillLinks(detailResp)

		if (visConfigObj == nil || visConfigObj == "") && queryDef != nil && queryDef.VisConfig != nil {
			visConfigObj = queryDef.VisConfig
		}
		if visConfigObj == nil || visConfigObj == "" {
			visConfigObj = map[string]any{"type": "table", "show_row_numbers": true}
		}

		modelVal, _ := paramMap["model"].(string)
		exploreVal, _ := paramMap["explore"].(string)

		title := "Looker Visualization"
		if wq != nil && wq.View != "" {
			title = wq.View
		} else if queryDef != nil && queryDef.View != "" {
			title = queryDef.View
		} else if exploreVal != "" {
			title = exploreVal
		}

		queryMeta := map[string]any{
			"id": queryIdVal,
		}
		if modelVal != "" {
			queryMeta["model"] = modelVal
		}
		if exploreVal != "" {
			queryMeta["view"] = exploreVal
		}
		if wq != nil {
			if wq.Model != "" {
				queryMeta["model"] = wq.Model
			}
			if wq.View != "" {
				queryMeta["view"] = wq.View
			}
			if wq.Fields != nil {
				queryMeta["fields"] = *wq.Fields
			}
			if wq.Filters != nil && len(*wq.Filters) > 0 {
				queryMeta["filters"] = *wq.Filters
			}
			if wq.Pivots != nil && len(*wq.Pivots) > 0 {
				queryMeta["pivots"] = *wq.Pivots
			}
			if wq.Limit != nil && *wq.Limit != "" {
				queryMeta["limit"] = *wq.Limit
			}
			if wq.Sorts != nil && len(*wq.Sorts) > 0 {
				queryMeta["sorts"] = *wq.Sorts
			}
			if wq.FilterExpression != nil && *wq.FilterExpression != "" {
				queryMeta["filter_expression"] = *wq.FilterExpression
			}
			if wq.QueryTimezone != nil && *wq.QueryTimezone != "" {
				queryMeta["query_timezone"] = *wq.QueryTimezone
			}
			if parsedDyn := parseDynamicFieldsForMeta(wq.DynamicFields); len(parsedDyn) > 0 {
				queryMeta["dynamic_fields"] = parsedDyn
			}
		}
		if queryDef != nil {
			if queryMeta["model"] == nil && queryDef.Model != "" {
				queryMeta["model"] = queryDef.Model
			}
			if queryMeta["view"] == nil && queryDef.View != "" {
				queryMeta["view"] = queryDef.View
			}
			if queryMeta["fields"] == nil && queryDef.Fields != nil {
				queryMeta["fields"] = *queryDef.Fields
			}
			if queryMeta["filter_expression"] == nil && queryDef.FilterExpression != nil && *queryDef.FilterExpression != "" {
				queryMeta["filter_expression"] = *queryDef.FilterExpression
			}
			if queryMeta["query_timezone"] == nil && queryDef.QueryTimezone != nil && *queryDef.QueryTimezone != "" {
				queryMeta["query_timezone"] = *queryDef.QueryTimezone
			}
			if queryMeta["dynamic_fields"] == nil {
				if parsedDyn := parseDynamicFieldsForMeta(queryDef.DynamicFields); len(parsedDyn) > 0 {
					queryMeta["dynamic_fields"] = parsedDyn
				}
			}
			if queryDef.Slug != nil && *queryDef.Slug != "" {
				queryMeta["slug"] = *queryDef.Slug
			}
			if queryDef.ClientId != nil && *queryDef.ClientId != "" {
				queryMeta["client_id"] = *queryDef.ClientId
				queryMeta["qid"] = *queryDef.ClientId
			}
			if queryDef.ShareUrl != nil && *queryDef.ShareUrl != "" {
				queryMeta["share_url"] = *queryDef.ShareUrl
			}
			if queryDef.ExpandedShareUrl != nil && *queryDef.ExpandedShareUrl != "" {
				queryMeta["expanded_share_url"] = *queryDef.ExpandedShareUrl
			}
			if queryDef.Url != nil && *queryDef.Url != "" {
				queryMeta["url"] = *queryDef.Url
			}
			if queryDef.Can != nil && len(*queryDef.Can) > 0 {
				queryMeta["can"] = *queryDef.Can
			}
		}

		queryResult := map[string]any{
			"data":        detailResp["data"],
			"fields":      detailResp["fields"],
			"pivots":      detailResp["pivots"],
			"totals_data": detailResp["totals_data"],
		}
		if errs, ok := detailResp["errors"]; ok && errs != nil {
			queryResult["errors"] = errs
		}

		appStateWg.Wait()

		visualizationData := map[string]any{
			"queryResult": queryResult,
			"visConfig":   visConfigObj,
			"title":       title,
			"query":       queryMeta,
		}

		resultPayload := map[string]any{
			"visualizationData":  visualizationData,
			"vis_config":         visConfigObj,
			"query_id":           queryIdVal,
			"slug":               queryMeta["slug"],
			"client_id":          queryMeta["client_id"],
			"share_url":          queryMeta["share_url"],
			"expanded_share_url": queryMeta["expanded_share_url"],
		}
		if canVal, ok := queryMeta["can"]; ok && canVal != nil {
			resultPayload["can"] = canVal
		}
		if qModel, ok := queryMeta["model"].(string); ok && qModel != "" {
			resultPayload["model"] = qModel
		}
		if qView, ok := queryMeta["view"].(string); ok && qView != "" {
			resultPayload["explore"] = qView
		}
		if appState != nil {
			resultPayload["app_state"] = appState
		}
		return applyVisualizationPayloadLimit(resultPayload, visualizationData), nil
	}

	// 2. Inline query path with json_detail
	wq, err := lookercommon.ProcessQueryArgs(ctx, params)
	if err != nil {
		return nil, util.NewAgentError("error building WriteQuery request", err)
	}
	if visConfigObj != nil {
		if m, ok := visConfigObj.(map[string]any); ok {
			wq.VisConfig = &m
		}
	}
	sanitizeWriteQueryFields(wq, nil, visConfigObj)
	if escErr := lookercommon.EscapeUnquotedParameterFilters(ctx, sdk, wq, source.LookerApiSettings()); escErr != nil {
		logger.WarnContext(ctx, "skipping unquoted-parameter escape, metadata lookup failed", "error", escErr)
	}

	createdQuery, cErr := sdk.CreateQuery(*wq, "id,slug,client_id,share_url,expanded_share_url,url,can", source.LookerApiSettings())
	var queryId string
	var slug string
	var shareUrl string
	var expandedShareUrl string
	var url string
	var queryCan map[string]bool
	if cErr == nil {
		// In Looker, Explore URLs require the 22-character client slug GUID (client_id) for ?qid= (stored in `slugs` table).
		if createdQuery.ClientId != nil && *createdQuery.ClientId != "" {
			slug = *createdQuery.ClientId
			queryId = *createdQuery.ClientId
		} else if createdQuery.Slug != nil && *createdQuery.Slug != "" {
			slug = *createdQuery.Slug
			queryId = *createdQuery.Slug
		} else if createdQuery.Id != nil {
			queryId = *createdQuery.Id
		}
		if createdQuery.ShareUrl != nil {
			shareUrl = *createdQuery.ShareUrl
		}
		if createdQuery.ExpandedShareUrl != nil {
			expandedShareUrl = *createdQuery.ExpandedShareUrl
		}
		if createdQuery.Url != nil {
			url = *createdQuery.Url
		}
		if createdQuery.Can != nil && len(*createdQuery.Can) > 0 {
			queryCan = *createdQuery.Can
		}
	} else {
		logger.WarnContext(ctx, "could not create query in looker", "error", cErr)
	}

	generateDrillLinks := false
	resp, err := lookercommon.RunInlineQueryWithDrillLinks(ctx, sdk, wq, "json_detail", &generateDrillLinks, source.LookerApiSettings())
	if err != nil {
		resp, err = lookercommon.RunInlineQueryWithDrillLinks(ctx, sdk, wq, "json", &generateDrillLinks, source.LookerApiSettings())
	}
	if err != nil {
		if strings.Contains(err.Error(), "status=401") {
			return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
		}
		return nil, util.ProcessGeneralError(lookercommon.ExtractLookerErrorMessage(err))
	}

	var rawParsed any
	if err := json.Unmarshal([]byte(resp), &rawParsed); err != nil {
		return nil, util.NewClientServerError("error unmarshaling query response", http.StatusInternalServerError, err)
	}

	var dataList []any
	var fieldsObj any
	var pivotsList any
	var totalsObj any
	var errorsObj any

	if detailMap, ok := rawParsed.(map[string]any); ok {
		if wq.Pivots != nil && len(*wq.Pivots) > 0 {
			if pivErr := lookercommon.ValidatePivotsInDetailResponse(*wq.Pivots, detailMap); pivErr != nil {
				return nil, util.NewAgentError(pivErr.Error(), pivErr)
			}
		}
		lookercommon.StripDrillLinks(detailMap)
		if d, ok := detailMap["data"].([]any); ok {
			dataList = d
		}
		fieldsObj = detailMap["fields"]
		pivotsList = detailMap["pivots"]
		totalsObj = detailMap["totals_data"]
		errorsObj = detailMap["errors"]
	} else if arr, ok := rawParsed.([]any); ok {
		dataList = arr
	}

	if visConfigObj == nil {
		visConfigObj = map[string]any{"type": "table", "show_row_numbers": true}
	}

	queryResult := map[string]any{
		"data":        dataList,
		"fields":      fieldsObj,
		"pivots":      pivotsList,
		"totals_data": totalsObj,
	}
	if errorsObj != nil {
		queryResult["errors"] = errorsObj
	}

	appStateWg.Wait()

	inlineQueryMeta := map[string]any{
		"id":                 queryId,
		"slug":               slug,
		"client_id":          slug,
		"qid":                slug,
		"share_url":          shareUrl,
		"expanded_share_url": expandedShareUrl,
		"url":                url,
		"model":              wq.Model,
		"view":               wq.View,
		"fields":             wq.Fields,
		"filters":            wq.Filters,
		"pivots":             wq.Pivots,
		"sorts":              wq.Sorts,
		"limit":              wq.Limit,
	}
	if wq.FilterExpression != nil && *wq.FilterExpression != "" {
		inlineQueryMeta["filter_expression"] = *wq.FilterExpression
	}
	if wq.QueryTimezone != nil && *wq.QueryTimezone != "" {
		inlineQueryMeta["query_timezone"] = *wq.QueryTimezone
	}
	if parsedDyn := parseDynamicFieldsForMeta(wq.DynamicFields); len(parsedDyn) > 0 {
		inlineQueryMeta["dynamic_fields"] = parsedDyn
	}
	if len(queryCan) > 0 {
		inlineQueryMeta["can"] = queryCan
	}

	visualizationData := map[string]any{
		"queryResult": queryResult,
		"visConfig":   visConfigObj,
		"title":       wq.View,
		"query":       inlineQueryMeta,
	}

	resultPayload := map[string]any{
		"visualizationData":  visualizationData,
		"model":              wq.Model,
		"explore":            wq.View,
		"vis_config":         visConfigObj,
		"query_id":           queryId,
		"slug":               slug,
		"client_id":          slug,
		"share_url":          shareUrl,
		"expanded_share_url": expandedShareUrl,
	}
	if len(queryCan) > 0 {
		resultPayload["can"] = queryCan
	}
	if appState != nil {
		resultPayload["app_state"] = appState
	}
	return applyVisualizationPayloadLimit(resultPayload, visualizationData), nil
}

func applyVisualizationPayloadLimit(resultPayload, visualizationData map[string]any) tools.StructuredToolResult {
	if _, tooLarge := lookercommon.ExceedsMCPPayloadLimit(resultPayload, lookercommon.MaxMCPPayloadBytes); tooLarge {
		errMsg := "This visualization's query result is too large to display here. Open it in Looker Explore to view the full result."
		if qrMap, ok := visualizationData["queryResult"].(map[string]any); ok {
			qrMap["data"] = []any{}
			delete(qrMap, "totals_data")
			delete(qrMap, "pivots")
		}
		visualizationData["response_too_large"] = true
		visualizationData["error"] = errMsg
		resultPayload["response_too_large"] = true
		resultPayload["error"] = errMsg
		return tools.StructuredToolResult{
			Content:           resultPayload,
			StructuredContent: resultPayload,
		}
	}

	var rowCount int
	if qrMap, ok := visualizationData["queryResult"].(map[string]any); ok {
		if rows, ok := qrMap["data"].([]any); ok {
			rowCount = len(rows)
		}
	}
	summary := map[string]any{
		"query_id":  resultPayload["query_id"],
		"slug":      resultPayload["slug"],
		"client_id": resultPayload["client_id"],
		"share_url": resultPayload["share_url"],
		"title":     visualizationData["title"],
		"row_count": rowCount,
		"status":    "rendered_in_ui",
	}
	if modelVal, ok := resultPayload["model"]; ok && modelVal != nil && modelVal != "" {
		summary["model"] = modelVal
	}
	if exploreVal, ok := resultPayload["explore"]; ok && exploreVal != nil && exploreVal != "" {
		summary["explore"] = exploreVal
	}

	return tools.StructuredToolResult{
		Content:           summary,
		StructuredContent: resultPayload,
	}
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
