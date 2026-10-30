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
	"maps"
	"net/http"
	"regexp"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/log"
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

type compatibleSource = lookercommon.LookerUISource

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
	baseParams := lookercommon.GetQueryParameters()
	allParams := make(parameters.Parameters, 0, len(baseParams)+2)
	allParams = append(allParams, parameters.NewStringParameter(
		"query_id",
		"Optional Looker query ID (or slug) to fetch and render the saved visualization directly. If provided, model, explore, and fields are not required.",
		parameters.WithStringRequired(false),
	))

	for _, p := range baseParams {
		switch p.GetName() {
		case "model":
			allParams = append(allParams, parameters.NewStringParameter(
				"model",
				"The model containing the explore (from get_models). Required if query_id is not provided.",
				parameters.WithStringRequired(false),
			))
		case "explore":
			allParams = append(allParams, parameters.NewStringParameter(
				"explore",
				"The explore to be queried (from get_explores). Required if query_id is not provided.",
				parameters.WithStringRequired(false),
			))
		case "fields":
			allParams = append(allParams, parameters.NewArrayParameter(
				"fields",
				"The fields to be retrieved (from get_dimensions, get_measures, get_filters, or get_parameters). Required if query_id is not provided. For charts comparing a metric across categories or time (such as bar, column, line, area, pie, or scatter charts), `fields` MUST include BOTH the grouping dimension(s) AND the measure(s) in a single call (for example, [\"products.department\", \"order_items.count\"]). Passing only a single field to a bar/column/line chart will fail with 'Not Enough Fields'.",
				parameters.NewStringParameter("field", "A field to be returned in the query"),
				parameters.WithArrayRequired(false),
			))
		case "pivots":
			allParams = append(allParams, parameters.NewArrayParameter(
				"pivots",
				"The query pivots. Any pivot field omitted from `fields` is automatically appended to `fields`.",
				parameters.NewStringParameter("pivot_field", "A field to be used as a pivot in the query"),
				parameters.WithArrayDefault([]any{}),
			))
		case "limit":
			// Omit WithIntDefault(500) so saved query_id calls preserve the saved query's own limit unless explicitly overridden.
			allParams = append(allParams, parameters.NewIntParameter("limit", "The row limit.", parameters.WithIntRequired(false)))
		default:
			allParams = append(allParams, p)
		}
	}

	allParams = append(allParams, parameters.NewStringParameter(
		"vis_config",
		"Optional JSON string specifying the visualization configuration (e.g. chart type, options). Use \"looker_column\" for vertical column charts and \"looker_bar\" for horizontal bar charts.",
		parameters.WithStringRequired(false),
	))
	return allParams
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	lookercommon.EnsureDefaultToolUIMetadata(&cfg.UI, defaultResourceName)

	allParameters := getVisualizationParameters()
	uiMgr := lookercommon.NewLookerUIResourceManager(lookercommon.LookerUIResourceConfig{
		ResourceName: cfg.UI.Resource,
		ResourceURI:  defaultResourceURI,
		Description:  "Looker visualization rendering interface",
		SceneName:    "render_visualization",
		UILabel:      "visualization",
	})

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
	t.uiMgr.SetSource(cs)
	return nil
}

var dynamicFieldExprRefRe = regexp.MustCompile(`\$\{\s*([a-zA-Z0-9_.]+)\s*\}`)

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

func parseVisConfig(raw any) (map[string]any, util.ToolboxError) {
	if raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil, nil
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return nil, util.NewAgentError("invalid vis_config JSON: must be a valid JSON object string", err)
		}
		return parsed, nil
	case map[string]any:
		return maps.Clone(v), nil
	default:
		return nil, util.NewAgentError(fmt.Sprintf("invalid vis_config: expected JSON object string, got %T", raw), nil)
	}
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
func sanitizeWriteQueryFields(wq *v4.WriteQuery, queryDef *v4.Query, visConfig map[string]any) {
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

	extractHiddenFieldNames(visConfig, &requiredFields)
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
	queryID, _ := paramMap["query_id"].(string)
	queryID = strings.TrimSpace(queryID)

	visConfig, tbErr := parseVisConfig(paramMap["vis_config"])
	if tbErr != nil {
		return nil, tbErr
	}

	if queryID == "" {
		if modelVal, _ := paramMap["model"].(string); strings.TrimSpace(modelVal) == "" {
			return nil, util.NewAgentError("model is required when query_id is not provided; use get_models to discover available models and retry", nil)
		}
		if exploreVal, _ := paramMap["explore"].(string); strings.TrimSpace(exploreVal) == "" {
			return nil, util.NewAgentError("explore is required when query_id is not provided; use get_explores to discover available explores and retry", nil)
		}
		if fieldsSlice, _ := paramMap["fields"].([]any); len(fieldsSlice) == 0 {
			return nil, util.NewAgentError("fields is required when query_id is not provided; use get_dimensions and get_measures to discover available fields and retry", nil)
		}
	}

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}
	opts := source.LookerApiSettings()

	if queryID != "" {
		return t.invokeSavedQuery(ctx, logger, sdk, opts, queryID, paramMap, visConfig)
	}
	return t.invokeInlineQuery(ctx, logger, sdk, opts, params, visConfig)
}

func isNumericQueryID(queryID string) bool {
	if queryID == "" {
		return false
	}
	for i := 0; i < len(queryID); i++ {
		if queryID[i] < '0' || queryID[i] > '9' {
			return false
		}
	}
	return true
}

func fetchQueryByIDOrSlug(sdk *v4.LookerSDK, queryID string, opts *rtl.ApiSettings) (*v4.Query, error) {
	if isNumericQueryID(queryID) {
		q, err := sdk.Query(queryID, "", opts)
		if err == nil {
			return &q, nil
		}
		q, slugErr := sdk.QueryForSlug(queryID, "", opts)
		if slugErr == nil {
			return &q, nil
		}
		return nil, err
	}
	q, err := sdk.QueryForSlug(queryID, "", opts)
	if err == nil {
		return &q, nil
	}
	q, idErr := sdk.Query(queryID, "", opts)
	if idErr == nil {
		return &q, nil
	}
	return nil, err
}

func hasRequestedQueryOverrides(paramMap map[string]any, visConfig map[string]any) bool {
	if len(visConfig) > 0 {
		return true
	}
	if rawLimit, ok := paramMap["limit"]; ok && rawLimit != nil {
		if limitVal, ok := rawLimit.(int); !ok || limitVal != 0 {
			return true
		}
	}
	for _, sliceKey := range []string{"sorts", "fields", "pivots", "dynamic_fields"} {
		if s, ok := paramMap[sliceKey].([]any); ok && len(s) > 0 {
			return true
		}
	}
	for _, strKey := range []string{"model", "explore", "tz", "filter_expression"} {
		if s, ok := paramMap[strKey].(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
	}
	if fMap, ok := paramMap["filters"].(map[string]any); ok && len(fMap) > 0 {
		return true
	}
	return false
}

func applySavedQueryOverrides(
	queryDef *v4.Query,
	paramMap map[string]any,
	visConfig map[string]any,
) (*v4.WriteQuery, map[string]any, bool, util.ToolboxError) {
	if queryDef == nil || queryDef.Model == "" || queryDef.View == "" {
		return nil, visConfig, false, nil
	}

	baseWQ := lookercommon.WriteQueryFromQuery(*queryDef)
	wq := &baseWQ
	hasOverrides := false

	if rawLimit, ok := paramMap["limit"]; ok && rawLimit != nil {
		limitVal, ok := rawLimit.(int)
		if !ok || limitVal < -1 {
			return nil, nil, false, util.NewAgentError(fmt.Sprintf("invalid limit parameter: must be a positive integer or -1, got %v", rawLimit), nil)
		}
		if limitVal > 0 || limitVal == -1 {
			limitStr := fmt.Sprintf("%d", limitVal)
			wq.Limit = &limitStr
			hasOverrides = true
		}
	}
	if sortsSlice, ok := paramMap["sorts"].([]any); ok && len(sortsSlice) > 0 {
		s, err := parameters.ConvertAnySliceToTyped(sortsSlice, "string")
		if err != nil {
			return nil, nil, false, util.NewAgentError(fmt.Sprintf("invalid sorts parameter: %v", err), err)
		}
		sorts := s.([]string)
		wq.Sorts = &sorts
		hasOverrides = true
	}
	if fieldsSlice, ok := paramMap["fields"].([]any); ok && len(fieldsSlice) > 0 {
		f, err := parameters.ConvertAnySliceToTyped(fieldsSlice, "string")
		if err != nil {
			return nil, nil, false, util.NewAgentError(fmt.Sprintf("invalid fields parameter: %v", err), err)
		}
		fields := f.([]string)
		wq.Fields = &fields
		hasOverrides = true
	}
	if pivotsSlice, ok := paramMap["pivots"].([]any); ok && len(pivotsSlice) > 0 {
		p, err := parameters.ConvertAnySliceToTyped(pivotsSlice, "string")
		if err != nil {
			return nil, nil, false, util.NewAgentError(fmt.Sprintf("invalid pivots parameter: %v", err), err)
		}
		pivots := p.([]string)
		wq.Pivots = &pivots
		hasOverrides = true
	}
	if wq.Pivots != nil && len(*wq.Pivots) > 0 {
		var currentFields []string
		if wq.Fields != nil {
			currentFields = *wq.Fields
		}
		updatedFields, pivErr := lookercommon.EnsurePivotsInFields(currentFields, *wq.Pivots)
		if pivErr != nil {
			return nil, nil, false, util.NewAgentError(pivErr.Error(), pivErr)
		}
		wq.Fields = &updatedFields
	}
	if modelVal, _ := paramMap["model"].(string); strings.TrimSpace(modelVal) != "" {
		wq.Model = strings.TrimSpace(modelVal)
		hasOverrides = true
	}
	if exploreVal, _ := paramMap["explore"].(string); strings.TrimSpace(exploreVal) != "" {
		wq.View = strings.TrimSpace(exploreVal)
		hasOverrides = true
	}
	if filtersMap, ok := paramMap["filters"].(map[string]any); ok && len(filtersMap) > 0 {
		var knownFields []string
		if wq.Fields != nil {
			knownFields = *wq.Fields
		}
		hints := []string{wq.View}
		if wq.Pivots != nil {
			hints = append(hints, *wq.Pivots...)
		}
		if wq.Sorts != nil {
			hints = append(hints, *wq.Sorts...)
		}
		normalizedOverrides := lookercommon.NormalizeFilterKeys(filtersMap, knownFields, hints...)
		mergedFilters := make(map[string]any)
		if wq.Filters != nil {
			maps.Copy(mergedFilters, *wq.Filters)
		}
		maps.Copy(mergedFilters, normalizedOverrides)
		wq.Filters = &mergedFilters
		hasOverrides = true
	}
	if tzVal, _ := paramMap["tz"].(string); strings.TrimSpace(tzVal) != "" {
		trimmedTZ := strings.TrimSpace(tzVal)
		wq.QueryTimezone = &trimmedTZ
		hasOverrides = true
	}
	if feVal, _ := paramMap["filter_expression"].(string); strings.TrimSpace(feVal) != "" {
		trimmedFE := strings.TrimSpace(feVal)
		wq.FilterExpression = &trimmedFE
		hasOverrides = true
	}
	if dfSlice, ok := paramMap["dynamic_fields"].([]any); ok && len(dfSlice) > 0 {
		jsonBytes, err := json.Marshal(dfSlice)
		if err != nil {
			return nil, nil, false, util.NewAgentError(fmt.Sprintf("invalid dynamic_fields parameter: %v", err), err)
		}
		jsonStr := string(jsonBytes)
		wq.DynamicFields = &jsonStr
		hasOverrides = true
	}
	if visConfig != nil {
		mergedVisConfig := make(map[string]any)
		if wq.VisConfig != nil {
			maps.Copy(mergedVisConfig, *wq.VisConfig)
		}
		maps.Copy(mergedVisConfig, visConfig)
		wq.VisConfig = &mergedVisConfig
		visConfig = mergedVisConfig
		hasOverrides = true
	}

	return wq, visConfig, hasOverrides, nil
}

func resolveEffectiveVisConfig(visConfig map[string]any, queryDef *v4.Query) map[string]any {
	if len(visConfig) > 0 {
		return visConfig
	}
	if queryDef != nil && queryDef.VisConfig != nil && len(*queryDef.VisConfig) > 0 {
		return maps.Clone(*queryDef.VisConfig)
	}
	return map[string]any{"type": "table", "show_row_numbers": true}
}

func extractFatalDetailError(detailResp map[string]any) error {
	rawErrs, ok := detailResp["errors"].([]any)
	if !ok || len(rawErrs) == 0 {
		return nil
	}
	for _, e := range rawErrs {
		errMap, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if fatal, hasFatal := errMap["fatal"].(bool); hasFatal && !fatal {
			continue
		}
		msg, _ := errMap["message"].(string)
		if strings.TrimSpace(msg) == "" {
			msg, _ = errMap["message_details"].(string)
		}
		if strings.TrimSpace(msg) != "" {
			return fmt.Errorf("error running query: %s", strings.TrimSpace(msg))
		}
	}
	return nil
}

func parseVisualizationQueryResult(rawResp string, requestedPivots *[]string) (map[string]any, util.ToolboxError) {
	var rawParsed any
	if err := json.Unmarshal([]byte(rawResp), &rawParsed); err != nil {
		return nil, util.NewClientServerError("error unmarshaling query response", http.StatusInternalServerError, err)
	}

	var (
		dataVal      any
		fieldsObj    any
		pivotsObj    any
		totalsObj    any
		rowTotalsObj any
		subtotalsObj any
		errorsObj    any
	)

	switch parsed := rawParsed.(type) {
	case map[string]any:
		if requestedPivots != nil && len(*requestedPivots) > 0 {
			if pivErr := lookercommon.ValidatePivotsInDetailResponse(*requestedPivots, parsed); pivErr != nil {
				return nil, util.NewAgentError(pivErr.Error(), pivErr)
			}
		}
		dataRows, _ := parsed["data"].([]any)
		if len(dataRows) == 0 {
			if fatalErr := extractFatalDetailError(parsed); fatalErr != nil {
				return nil, util.NewAgentError(fatalErr.Error(), fatalErr)
			}
		}
		lookercommon.StripDrillLinks(parsed)
		dataVal = parsed["data"]
		fieldsObj = parsed["fields"]
		pivotsObj = parsed["pivots"]
		totalsObj = parsed["totals_data"]
		rowTotalsObj = parsed["row_totals"]
		subtotalsObj = parsed["subtotals_data"]
		errorsObj = parsed["errors"]
	case []any:
		dataVal = parsed
	default:
		return nil, util.NewClientServerError("unexpected query response format", http.StatusInternalServerError, nil)
	}

	queryResult := map[string]any{
		"data":        dataVal,
		"fields":      fieldsObj,
		"pivots":      pivotsObj,
		"totals_data": totalsObj,
	}
	if rowTotalsObj != nil {
		queryResult["row_totals"] = rowTotalsObj
	}
	if subtotalsObj != nil {
		queryResult["subtotals_data"] = subtotalsObj
	}
	if errorsObj != nil {
		queryResult["errors"] = errorsObj
	}
	return queryResult, nil
}

func populateQueryMetaFromWriteQuery(meta map[string]any, wq *v4.WriteQuery) {
	if wq == nil {
		return
	}
	if wq.Model != "" {
		meta["model"] = wq.Model
	}
	if wq.View != "" {
		meta["view"] = wq.View
	}
	if wq.Fields != nil {
		meta["fields"] = *wq.Fields
	}
	if wq.Filters != nil && len(*wq.Filters) > 0 {
		meta["filters"] = *wq.Filters
	}
	if wq.Pivots != nil && len(*wq.Pivots) > 0 {
		meta["pivots"] = *wq.Pivots
	}
	if wq.Sorts != nil && len(*wq.Sorts) > 0 {
		meta["sorts"] = *wq.Sorts
	}
	if wq.Limit != nil && *wq.Limit != "" {
		meta["limit"] = *wq.Limit
	}
	if wq.FilterExpression != nil && *wq.FilterExpression != "" {
		meta["filter_expression"] = *wq.FilterExpression
	}
	if wq.QueryTimezone != nil && *wq.QueryTimezone != "" {
		meta["query_timezone"] = *wq.QueryTimezone
	}
	if parsedDyn := parseDynamicFieldsForMeta(wq.DynamicFields); len(parsedDyn) > 0 {
		meta["dynamic_fields"] = parsedDyn
	}
}

func populateQueryMetaFromQueryDef(meta map[string]any, queryDef *v4.Query) {
	if queryDef == nil {
		return
	}
	if meta["model"] == nil && queryDef.Model != "" {
		meta["model"] = queryDef.Model
	}
	if meta["view"] == nil && queryDef.View != "" {
		meta["view"] = queryDef.View
	}
	if meta["fields"] == nil && queryDef.Fields != nil {
		meta["fields"] = *queryDef.Fields
	}
	if meta["filters"] == nil && queryDef.Filters != nil && len(*queryDef.Filters) > 0 {
		meta["filters"] = *queryDef.Filters
	}
	if meta["pivots"] == nil && queryDef.Pivots != nil && len(*queryDef.Pivots) > 0 {
		meta["pivots"] = *queryDef.Pivots
	}
	if meta["sorts"] == nil && queryDef.Sorts != nil && len(*queryDef.Sorts) > 0 {
		meta["sorts"] = *queryDef.Sorts
	}
	if meta["limit"] == nil && queryDef.Limit != nil && *queryDef.Limit != "" {
		meta["limit"] = *queryDef.Limit
	}
	if meta["filter_expression"] == nil && queryDef.FilterExpression != nil && *queryDef.FilterExpression != "" {
		meta["filter_expression"] = *queryDef.FilterExpression
	}
	if meta["query_timezone"] == nil && queryDef.QueryTimezone != nil && *queryDef.QueryTimezone != "" {
		meta["query_timezone"] = *queryDef.QueryTimezone
	}
	if meta["dynamic_fields"] == nil {
		if parsedDyn := parseDynamicFieldsForMeta(queryDef.DynamicFields); len(parsedDyn) > 0 {
			meta["dynamic_fields"] = parsedDyn
		}
	}
	if queryDef.Slug != nil && *queryDef.Slug != "" {
		meta["slug"] = *queryDef.Slug
	}
	if queryDef.ClientId != nil && *queryDef.ClientId != "" {
		meta["client_id"] = *queryDef.ClientId
		meta["qid"] = *queryDef.ClientId
	}
	if queryDef.ShareUrl != nil && *queryDef.ShareUrl != "" {
		meta["share_url"] = *queryDef.ShareUrl
	}
	if queryDef.ExpandedShareUrl != nil && *queryDef.ExpandedShareUrl != "" {
		meta["expanded_share_url"] = *queryDef.ExpandedShareUrl
	}
	if queryDef.Url != nil && *queryDef.Url != "" {
		meta["url"] = *queryDef.Url
	}
	if queryDef.Can != nil && len(*queryDef.Can) > 0 {
		meta["can"] = *queryDef.Can
	}
}

func buildVisualizationResult(
	ctx context.Context,
	sdk *v4.LookerSDK,
	opts *rtl.ApiSettings,
	title string,
	queryID string,
	visConfig map[string]any,
	queryResult map[string]any,
	queryMeta map[string]any,
) tools.StructuredToolResult {
	appState := lookercommon.FetchMcpAppState(ctx, sdk, opts)

	visualizationData := map[string]any{
		"queryResult": queryResult,
		"visConfig":   visConfig,
		"title":       title,
		"query":       queryMeta,
	}

	resultPayload := map[string]any{
		"visualizationData":  visualizationData,
		"vis_config":         visConfig,
		"query_id":           queryID,
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
	return applyVisualizationPayloadLimit(resultPayload, visualizationData)
}

func (t Tool) invokeSavedQuery(
	ctx context.Context,
	logger log.Logger,
	sdk *v4.LookerSDK,
	opts *rtl.ApiSettings,
	queryID string,
	paramMap map[string]any,
	visConfig map[string]any,
) (any, util.ToolboxError) {
	queryDef, qErr := fetchQueryByIDOrSlug(sdk, queryID, opts)
	if qErr != nil {
		logger.WarnContext(ctx, "could not fetch query definition by id or slug", "error", qErr)
		if hasRequestedQueryOverrides(paramMap, visConfig) {
			return nil, lookercommon.ProcessLookerAPIError(qErr)
		}
	}

	wq, mergedVisConfig, hasOverrides, tbErr := applySavedQueryOverrides(queryDef, paramMap, visConfig)
	if tbErr != nil {
		return nil, tbErr
	}

	savedNumericID := queryID
	if queryDef != nil && queryDef.Id != nil && *queryDef.Id != "" {
		savedNumericID = *queryDef.Id
	}

	var (
		resp               string
		rErr               error
		generateDrillLinks = false
		requestedPivots    *[]string
	)

	if wq != nil {
		sanitizeWriteQueryFields(wq, queryDef, mergedVisConfig)
		if escErr := lookercommon.EscapeUnquotedParameterFilters(ctx, sdk, wq, opts); escErr != nil {
			logger.WarnContext(ctx, "skipping unquoted-parameter escape, metadata lookup failed", "error", escErr)
		}
		requestedPivots = wq.Pivots
		// Execute the modified query inline without calling sdk.CreateQuery so
		// read-only overrides do not persist new rows in Looker's database.
		resp, rErr = lookercommon.RunInlineQueryWithDrillLinks(ctx, sdk, wq, "json_detail", &generateDrillLinks, opts)
		if rErr != nil && !hasOverrides {
			logger.WarnContext(ctx, "error running inline query from query definition, falling back to saved query", "error", rErr)
			resp, rErr = lookercommon.RunSavedQuery(ctx, sdk, savedNumericID, "json_detail", &generateDrillLinks, opts)
		}
	} else {
		resp, rErr = lookercommon.RunSavedQuery(ctx, sdk, savedNumericID, "json_detail", &generateDrillLinks, opts)
	}
	if rErr != nil {
		return nil, lookercommon.ProcessLookerAPIError(rErr)
	}

	queryResult, tbErr := parseVisualizationQueryResult(resp, requestedPivots)
	if tbErr != nil {
		return nil, tbErr
	}

	effectiveVisConfig := resolveEffectiveVisConfig(mergedVisConfig, queryDef)

	modelVal, _ := paramMap["model"].(string)
	exploreVal, _ := paramMap["explore"].(string)

	title := "Looker Visualization"
	switch {
	case wq != nil && wq.View != "":
		title = wq.View
	case queryDef != nil && queryDef.View != "":
		title = queryDef.View
	case strings.TrimSpace(exploreVal) != "":
		title = strings.TrimSpace(exploreVal)
	}

	queryMeta := map[string]any{"id": queryID}
	if strings.TrimSpace(modelVal) != "" {
		queryMeta["model"] = strings.TrimSpace(modelVal)
	}
	if strings.TrimSpace(exploreVal) != "" {
		queryMeta["view"] = strings.TrimSpace(exploreVal)
	}
	populateQueryMetaFromWriteQuery(queryMeta, wq)
	populateQueryMetaFromQueryDef(queryMeta, queryDef)

	return buildVisualizationResult(ctx, sdk, opts, title, queryID, effectiveVisConfig, queryResult, queryMeta), nil
}

func (t Tool) invokeInlineQuery(
	ctx context.Context,
	logger log.Logger,
	sdk *v4.LookerSDK,
	opts *rtl.ApiSettings,
	params parameters.ParamValues,
	visConfig map[string]any,
) (any, util.ToolboxError) {
	wq, err := lookercommon.ProcessQueryArgs(ctx, params)
	if err != nil {
		return nil, util.NewAgentError("error building WriteQuery request", err)
	}
	if visConfig != nil {
		wq.VisConfig = &visConfig
	}
	sanitizeWriteQueryFields(wq, nil, visConfig)
	if escErr := lookercommon.EscapeUnquotedParameterFilters(ctx, sdk, wq, opts); escErr != nil {
		logger.WarnContext(ctx, "skipping unquoted-parameter escape, metadata lookup failed", "error", escErr)
	}

	var (
		queryID          string
		slug             string
		clientID         string
		shareURL         string
		expandedShareURL string
		queryURL         string
		queryCan         map[string]bool
	)
	if createdQuery, cErr := sdk.CreateQuery(*wq, "id,slug,client_id,share_url,expanded_share_url,url,can", opts); cErr == nil {
		if createdQuery.ClientId != nil && *createdQuery.ClientId != "" {
			clientID = *createdQuery.ClientId
		}
		if createdQuery.Slug != nil && *createdQuery.Slug != "" {
			slug = *createdQuery.Slug
		}
		if slug == "" {
			slug = clientID
		}
		if clientID == "" {
			clientID = slug
		}
		switch {
		case clientID != "":
			queryID = clientID
		case slug != "":
			queryID = slug
		case createdQuery.Id != nil:
			queryID = *createdQuery.Id
		}
		if createdQuery.ShareUrl != nil {
			shareURL = *createdQuery.ShareUrl
		}
		if createdQuery.ExpandedShareUrl != nil {
			expandedShareURL = *createdQuery.ExpandedShareUrl
		}
		if createdQuery.Url != nil {
			queryURL = *createdQuery.Url
		}
		if createdQuery.Can != nil && len(*createdQuery.Can) > 0 {
			queryCan = *createdQuery.Can
		}
	} else {
		logger.WarnContext(ctx, "could not create query in looker", "error", cErr)
	}

	generateDrillLinks := false
	resp, err := lookercommon.RunInlineQueryWithDrillLinks(ctx, sdk, wq, "json_detail", &generateDrillLinks, opts)
	if err != nil {
		return nil, lookercommon.ProcessLookerAPIError(err)
	}

	queryResult, tbErr := parseVisualizationQueryResult(resp, wq.Pivots)
	if tbErr != nil {
		return nil, tbErr
	}

	effectiveVisConfig := resolveEffectiveVisConfig(visConfig, nil)

	queryMeta := map[string]any{
		"id":                 queryID,
		"slug":               slug,
		"client_id":          clientID,
		"qid":                clientID,
		"share_url":          shareURL,
		"expanded_share_url": expandedShareURL,
		"url":                queryURL,
	}
	populateQueryMetaFromWriteQuery(queryMeta, wq)
	if len(queryCan) > 0 {
		queryMeta["can"] = queryCan
	}

	return buildVisualizationResult(ctx, sdk, opts, wq.View, queryID, effectiveVisConfig, queryResult, queryMeta), nil
}

func applyVisualizationPayloadLimit(resultPayload, visualizationData map[string]any) tools.StructuredToolResult {
	var rowCount int
	if qrMap, ok := visualizationData["queryResult"].(map[string]any); ok {
		if rows, ok := qrMap["data"].([]any); ok {
			rowCount = len(rows)
		}
	}

	buildSummary := func(status string) map[string]any {
		summary := map[string]any{
			"query_id":  resultPayload["query_id"],
			"slug":      resultPayload["slug"],
			"client_id": resultPayload["client_id"],
			"share_url": resultPayload["share_url"],
			"title":     visualizationData["title"],
			"row_count": rowCount,
			"status":    status,
		}
		if modelVal, ok := resultPayload["model"]; ok && modelVal != nil && modelVal != "" {
			summary["model"] = modelVal
		}
		if exploreVal, ok := resultPayload["explore"]; ok && exploreVal != nil && exploreVal != "" {
			summary["explore"] = exploreVal
		}
		return summary
	}

	if _, tooLarge := lookercommon.ExceedsMCPPayloadLimit(resultPayload, lookercommon.MaxMCPPayloadBytes); tooLarge {
		errMsg := "This visualization's query result is too large to display here. Open it in Looker Explore to view the full result."
		if qrMap, ok := visualizationData["queryResult"].(map[string]any); ok {
			qrMap["data"] = []any{}
			delete(qrMap, "totals_data")
			delete(qrMap, "row_totals")
			delete(qrMap, "subtotals_data")
			delete(qrMap, "pivots")
		}
		visualizationData["response_too_large"] = true
		visualizationData["error"] = errMsg
		resultPayload["response_too_large"] = true
		resultPayload["error"] = errMsg
		summary := buildSummary("response_too_large")
		summary["response_too_large"] = true
		summary["error"] = errMsg
		return tools.StructuredToolResult{
			Content:           summary,
			StructuredContent: resultPayload,
		}
	}

	return tools.StructuredToolResult{
		Content:           buildSummary("rendered_in_ui"),
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
