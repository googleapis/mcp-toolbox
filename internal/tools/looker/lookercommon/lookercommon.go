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
package lookercommon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
	"github.com/thlib/go-timezone-local/tzlocal"
)

const (
	DimensionsFields = "fields(dimensions(name,type,label,label_short,description,synonyms,tags,hidden,suggestable,suggestions,suggest_dimension,suggest_explore))"
	FiltersFields    = "fields(filters(name,type,label,label_short,description,synonyms,tags,hidden,suggestable,suggestions,suggest_dimension,suggest_explore))"
	MeasuresFields   = "fields(measures(name,type,label,label_short,description,synonyms,tags,hidden,suggestable,suggestions,suggest_dimension,suggest_explore))"
	ParametersFields = "fields(parameters(name,type,label,label_short,description,synonyms,tags,hidden,suggestable,suggestions,suggest_dimension,suggest_explore))"
)

// ExtractLookerFieldProperties extracts common properties from Looker field objects.
func ExtractLookerFieldProperties(ctx context.Context, fields *[]v4.LookmlModelExploreField, showHiddenFields bool) ([]any, error) {
	data := make([]any, 0)

	// Handle nil fields pointer
	if fields == nil {
		return data, nil
	}

	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		// This should ideally not happen if the context is properly set up.
		// Log and return an empty map or handle as appropriate for your error strategy.
		return data, fmt.Errorf("error getting logger from context in ExtractLookerFieldProperties: %v", err)
	}

	for _, v := range *fields {
		logger.DebugContext(ctx, "Got response element of %v\n", v)
		if v.Name != nil && strings.HasSuffix(*v.Name, "_raw") {
			continue
		}
		if !showHiddenFields && v.Hidden != nil && *v.Hidden {
			continue
		}
		vMap := make(map[string]any)
		if v.Name != nil {
			vMap["name"] = *v.Name
		}
		if v.Type != nil {
			vMap["type"] = *v.Type
		}
		if v.Label != nil {
			vMap["label"] = *v.Label
		}
		if v.LabelShort != nil {
			vMap["label_short"] = *v.LabelShort
		}
		if v.Description != nil {
			vMap["description"] = *v.Description
		}
		if v.Tags != nil && len(*v.Tags) > 0 {
			vMap["tags"] = *v.Tags
		}
		if v.Synonyms != nil && len(*v.Synonyms) > 0 {
			vMap["synonyms"] = *v.Synonyms
		}
		if v.Suggestable != nil {
			vMap["suggestable"] = *v.Suggestable
		}
		if v.Suggestable != nil && *v.Suggestable {
			if v.Suggestions != nil && len(*v.Suggestions) > 0 {
				vMap["suggestions"] = *v.Suggestions
			}
			if v.SuggestExplore != nil && v.SuggestDimension != nil {
				vMap["suggest_explore"] = *v.SuggestExplore
				vMap["suggest_dimension"] = *v.SuggestDimension
			}
		}
		logger.DebugContext(ctx, "Converted to %v\n", vMap)
		data = append(data, vMap)
	}

	return data, nil
}

// CheckLookerExploreFields checks if the Fields object in LookmlModelExplore is nil before accessing its sub-fields.
func CheckLookerExploreFields(resp *v4.LookmlModelExplore) error {
	if resp == nil || resp.Fields == nil {
		return fmt.Errorf("looker API response or its fields object is nil")
	}
	return nil
}

func GetFieldParameters() parameters.Parameters {
	modelParameter := parameters.NewStringParameter("model", "The model containing the explore.")
	exploreParameter := parameters.NewStringParameter("explore", "The explore containing the fields.")
	return parameters.Parameters{modelParameter, exploreParameter}
}

func GetQueryParameters() parameters.Parameters {
	modelParameter := parameters.NewStringParameter("model", "The model containing the explore.")
	exploreParameter := parameters.NewStringParameter("explore", "The explore to be queried.")
	fieldsParameter := parameters.NewArrayParameter("fields",
		"The fields to be retrieved.",
		parameters.NewStringParameter("field", "A field to be returned in the query"),
	)
	filtersParameter := parameters.NewMapParameter(
		"filters",
		"The filters for the query. Keys are fully-qualified field names "+
			"(e.g. \"view.field\") and values are filter expressions or "+
			"parameter values. Pass values bare — do not wrap them in extra "+
			"quote characters. For LookML `parameter` fields, use the raw "+
			"allowed_value (e.g. `first_touch`), not `\"first_touch\"`."+
			" To retrieve valid filter values for a suggestible field, "+
			"use the 'get_field_value_suggestions' tool.",
		"",
		parameters.WithMapDefault(map[string]any{}),
	)
	pivotsParameter := parameters.NewArrayParameter(
		"pivots",
		"The query pivots (must be included in fields as well).",
		parameters.NewStringParameter("pivot_field", "A field to be used as a pivot in the query"),
		parameters.WithArrayDefault([]any{}),
	)
	sortsParameter := parameters.NewArrayParameter(
		"sorts",
		"The sorts like \"field.id desc 0\".",
		parameters.NewStringParameter("sort_field", "A field to be used as a sort in the query"),
		parameters.WithArrayDefault([]any{}),
	)
	limitParameter := parameters.NewIntParameter("limit", "The row limit.", parameters.WithIntDefault(500))
	tzParameter := parameters.NewStringParameter("tz", "The query timezone.", parameters.WithStringRequired(false))
	filterExpressionParameter := parameters.NewStringParameter("filter_expression", "An optional filter expression string.", parameters.WithStringRequired(false))
	dynamicFieldsParameter := parameters.NewArrayParameter(
		"dynamic_fields",
		"An optional array of dynamic fields (table calculations, custom measures, custom dimensions).",
		parameters.NewMapParameter("dynamic_field", "A dynamic field definition", ""),
		parameters.WithArrayDefault([]any{}),
	)

	return parameters.Parameters{
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
	}
}

func ProcessFieldArgs(ctx context.Context, params parameters.ParamValues) (*string, *string, error) {
	mapParams := params.AsMap()
	model, ok := mapParams["model"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("'model' must be a string, got %T", mapParams["model"])
	}
	explore, ok := mapParams["explore"].(string)
	if !ok {
		return nil, nil, fmt.Errorf("'explore' must be a string, got %T", mapParams["explore"])
	}
	return &model, &explore, nil
}

// escapeUnquotedParameterValue escapes Looker filter-expression metacharacters
// so the value reaches a type: unquoted parameter without being interpreted as
// a wildcard pattern. Looker treats `_` as a single-character wildcard and `%`
// as a multi-character wildcard, and rejects either inside unquoted-parameter
// values; `,` is the filter-expression value separator; `^` is the escape
// character itself. Already-escaped sequences (`^_`, `^%`, `^,`, `^^`) pass
// through unchanged, which keeps the function idempotent for callers that pass
// pre-escaped forms (e.g. round-tripping `default_filter_value` from the
// explore metadata). Lone metacharacters get a `^` prefix; lone `^` is doubled
// to `^^`. Walking rune-by-rune (not whole-string scanning) means a value with
// both an already-escaped sequence and an unescaped metacharacter gets each
// half handled correctly — e.g. `first^_touch_v2` becomes `first^_touch^_v2`.
func escapeUnquotedParameterValue(value string) string {
	var sb strings.Builder
	sb.Grow(len(value))
	runes := []rune(value)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '^' {
			if i+1 < len(runes) {
				next := runes[i+1]
				if next == '_' || next == '%' || next == ',' || next == '^' {
					sb.WriteRune('^')
					sb.WriteRune(next)
					i++
					continue
				}
			}
			sb.WriteString("^^")
			continue
		}
		if r == '_' || r == '%' || r == ',' {
			sb.WriteRune('^')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// EscapeFiltersForUnquotedParameters mutates wq.Filters so every string value
// keyed to a fully-qualified parameter name listed in unquotedNames is escaped
// per Looker filter-expression syntax. Non-string values and filters targeting
// other fields are left untouched.
func EscapeFiltersForUnquotedParameters(wq *v4.WriteQuery, unquotedNames map[string]bool) {
	if wq == nil || wq.Filters == nil || len(unquotedNames) == 0 {
		return
	}
	for k, v := range *wq.Filters {
		if !unquotedNames[k] {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		(*wq.Filters)[k] = escapeUnquotedParameterValue(s)
	}
}

// resolveUnquotedParameterNames fetches the parameter metadata for the explore
// targeted by wq and returns the set of fully-qualified parameter names whose
// LookML type is `unquoted`. Returns an empty map (and no error) when the
// WriteQuery has no model/view set or no filters at all — the caller has
// nothing to escape in either case.
func resolveUnquotedParameterNames(_ context.Context, sdk *v4.LookerSDK, wq *v4.WriteQuery, opts *rtl.ApiSettings) (map[string]bool, error) {
	if wq == nil || wq.Filters == nil || len(*wq.Filters) == 0 || wq.Model == "" || wq.View == "" {
		return map[string]bool{}, nil
	}
	fields := ParametersFields
	req := v4.RequestLookmlModelExplore{
		LookmlModelName: wq.Model,
		ExploreName:     wq.View,
		Fields:          &fields,
	}
	resp, err := sdk.LookmlModelExplore(req, opts)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	if resp.Fields == nil || resp.Fields.Parameters == nil {
		return out, nil
	}
	for _, p := range *resp.Fields.Parameters {
		if p.Name != nil && p.Type != nil && *p.Type == "unquoted" {
			out[*p.Name] = true
		}
	}
	return out, nil
}

// EscapeUnquotedParameterFilters looks up the explore's parameter metadata and
// escapes filter-expression metacharacters in any filter value that targets a
// type: unquoted parameter. Looker's filter parser interprets `_` and `%` as
// wildcards and rejects them for unquoted parameters, so an unescaped value
// like `first_touch` is parsed as `first<single-char-wildcard>touch` and 400s
// with "The filter \"first_touch\" is not allowed." This is a no-op when no
// filters target unquoted parameters. Metadata-lookup failures are returned to
// the caller, which should log and proceed: callers without explore-read
// permission still need their non-parameter queries to succeed.
func EscapeUnquotedParameterFilters(ctx context.Context, sdk *v4.LookerSDK, wq *v4.WriteQuery, opts *rtl.ApiSettings) error {
	unquoted, err := resolveUnquotedParameterNames(ctx, sdk, wq, opts)
	if err != nil {
		return err
	}
	EscapeFiltersForUnquotedParameters(wq, unquoted)
	return nil
}

func ProcessQueryArgs(ctx context.Context, params parameters.ParamValues) (*v4.WriteQuery, error) {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get logger from ctx: %s", err)
	}

	logger.DebugContext(ctx, "params = ", params)
	paramsMap := params.AsMap()

	f, err := parameters.ConvertAnySliceToTyped(paramsMap["fields"].([]any), "string")
	if err != nil {
		return nil, fmt.Errorf("can't convert fields to array of strings: %s", err)
	}
	fields := f.([]string)
	filters := paramsMap["filters"].(map[string]any)
	// Strip a single layer of wrapping quotes from keys and string values.
	// Values matter for LookML `type: unquoted` parameters, where Looker
	// substitutes the value bare into SQL via {% parameter %}. Build a new map
	// rather than mutating during iteration, and avoid comparing `any` values
	// directly (non-comparable dynamic types like slices would panic).
	processedFilters := make(map[string]any, len(filters))
	for k, v := range filters {
		newKey := k
		if len(k) >= 2 && (k[0] == '\'' || k[0] == '"') && k[0] == k[len(k)-1] {
			newKey = k[1 : len(k)-1]
		}
		newVal := v
		if s, ok := v.(string); ok && len(s) >= 2 &&
			(s[0] == '\'' || s[0] == '"') && s[0] == s[len(s)-1] {
			newVal = s[1 : len(s)-1]
		}
		processedFilters[newKey] = newVal
	}
	filters = processedFilters
	p, err := parameters.ConvertAnySliceToTyped(paramsMap["pivots"].([]any), "string")
	if err != nil {
		return nil, fmt.Errorf("can't convert pivots to array of strings: %s", err)
	}
	pivots := p.([]string)
	s, err := parameters.ConvertAnySliceToTyped(paramsMap["sorts"].([]any), "string")
	if err != nil {
		return nil, fmt.Errorf("can't convert sorts to array of strings: %s", err)
	}
	sorts := s.([]string)
	limit := fmt.Sprintf("%v", paramsMap["limit"].(int))

	var tz string
	if paramsMap["tz"] != nil {
		tz = paramsMap["tz"].(string)
	} else {
		tzname, err := tzlocal.RuntimeTZ()
		if err != nil {
			logger.ErrorContext(ctx, fmt.Sprintf("Error getting local timezone: %s", err))
			tzname = "Etc/UTC"
		}
		tz = tzname
	}

	var filterExpressionPtr *string
	if val, ok := paramsMap["filter_expression"]; ok && val != nil {
		if strVal, ok := val.(string); ok {
			filterExpressionPtr = &strVal
		} else {
			return nil, fmt.Errorf("'filter_expression' must be a string, got %T", val)
		}
	}

	var dynamicFieldsPtr *string
	if val, ok := paramsMap["dynamic_fields"]; ok && val != nil {
		if sliceVal, ok := val.([]any); ok && len(sliceVal) > 0 {
			jsonBytes, err := json.Marshal(sliceVal)
			if err != nil {
				return nil, fmt.Errorf("error marshaling dynamic_fields: %w", err)
			}
			jsonStr := string(jsonBytes)
			dynamicFieldsPtr = &jsonStr
		}
	}

	wq := v4.WriteQuery{
		Model:            paramsMap["model"].(string),
		View:             paramsMap["explore"].(string),
		Fields:           &fields,
		Pivots:           &pivots,
		Filters:          &filters,
		Sorts:            &sorts,
		QueryTimezone:    &tz,
		Limit:            &limit,
		FilterExpression: filterExpressionPtr,
		DynamicFields:    dynamicFieldsPtr,
	}
	return &wq, nil
}

type QueryApiClientContext struct {
	Name            string            `json:"name"`
	Attributes      map[string]string `json:"attributes,omitempty"`
	ExtraAttributes map[string]string `json:"extra_attributes,omitempty"`
}

type RenderOptions struct {
	Format string `json:"format"`
}

type RequestRunInlineQuery2 struct {
	Query             v4.WriteQuery         `json:"query"`
	RenderOpts        RenderOptions         `json:"render_options"`
	QueryApiClientCtx QueryApiClientContext `json:"query_api_client_context"`
}

func RunInlineQuery2(l *v4.LookerSDK, request RequestRunInlineQuery2, options *rtl.ApiSettings) (string, error) {
	var result string
	err := l.AuthSession.Do(&result, "POST", "/4.0", "/queries/run_inline", nil, request, options)
	return result, err
}

func RunInlineQuery(ctx context.Context, sdk *v4.LookerSDK, wq *v4.WriteQuery, format string, options *rtl.ApiSettings) (string, error) {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to get logger from ctx: %s", err)
	}
	req := v4.RequestRunInlineQuery{
		Body:         *wq,
		ResultFormat: format,
	}
	req2 := RequestRunInlineQuery2{
		Query: *wq,
		RenderOpts: RenderOptions{
			Format: format,
		},
		QueryApiClientCtx: QueryApiClientContext{
			Name: "MCP Toolbox",
		},
	}
	resp, err := RunInlineQuery2(sdk, req2, options)
	if err != nil {
		logger.DebugContext(ctx, "error querying with new endpoint, trying again with original", err)
		resp, err = sdk.RunInlineQuery(req, options)
	}
	return resp, err
}

func GetProjectFileContent(l *v4.LookerSDK, projectId string, filePath string, options *rtl.ApiSettings) (string, error) {
	var result string
	path := fmt.Sprintf("/projects/%s/file/content", url.PathEscape(projectId))
	query := map[string]any{
		"file_path": filePath,
	}
	err := l.AuthSession.Do(&result, "GET", "/4.0", path, query, nil, options)
	return result, err
}

func DeleteProjectFile(l *v4.LookerSDK, projectId string, filePath string, options *rtl.ApiSettings) error {
	path := fmt.Sprintf("/projects/%s/files", url.PathEscape(projectId))
	query := map[string]any{
		"file_path": filePath,
	}
	err := l.AuthSession.Do(nil, "DELETE", "/4.0", path, query, nil, options)
	return err
}

type FileContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func CreateProjectFile(l *v4.LookerSDK, projectId string, fileContent FileContent, options *rtl.ApiSettings) error {
	path := fmt.Sprintf("/projects/%s/files", url.PathEscape(projectId))
	err := l.AuthSession.Do(nil, "POST", "/4.0", path, nil, fileContent, options)
	return err
}

func UpdateProjectFile(l *v4.LookerSDK, projectId string, fileContent FileContent, options *rtl.ApiSettings) error {
	path := fmt.Sprintf("/projects/%s/files", url.PathEscape(projectId))
	err := l.AuthSession.Do(nil, "PUT", "/4.0", path, nil, fileContent, options)
	return err
}

func GetProjectDirectories(l *v4.LookerSDK, projectId string, options *rtl.ApiSettings) (string, error) {
	var result string
	path := fmt.Sprintf("/projects/%s/directories", url.PathEscape(projectId))
	err := l.AuthSession.Do(&result, "GET", "/4.0", path, nil, nil, options)
	return result, err
}

type Directory struct {
	Path string `json:"path"`
}

func CreateProjectDirectory(l *v4.LookerSDK, projectId string, directoryPath string, options *rtl.ApiSettings) error {
	d := Directory{
		Path: directoryPath,
	}
	var result string
	path := fmt.Sprintf("/projects/%s/directories", url.PathEscape(projectId))
	return l.AuthSession.Do(&result, "POST", "/4.0", path, nil, d, options)
}

func DeleteProjectDirectory(l *v4.LookerSDK, projectId string, directoryPath string, options *rtl.ApiSettings) error {
	var query = map[string]any{
		"path": directoryPath,
	}
	var result string
	path := fmt.Sprintf("/projects/%s/directories", url.PathEscape(projectId))
	return l.AuthSession.Do(&result, "DELETE", "/4.0", path, query, nil, options)
}

type ProjectGeneratorColumn struct {
	ColumnName string `json:"column_name"`
}

type ProjectGeneratorTable struct {
	Schema     string                   `json:"schema"`
	TableName  string                   `json:"table_name"`
	PrimaryKey *string                  `json:"primary_key,omitempty"`
	BaseView   *bool                    `json:"base_view,omitempty"`
	Columns    []ProjectGeneratorColumn `json:"columns,omitempty"`
}

type ProjectGeneratorRequestBody struct {
	Tables []ProjectGeneratorTable `json:"tables"`
}

type ProjectGeneratorQueryParams struct {
	Connection          string `json:"connection"`
	FileTypeForExplores string `json:"file_type_for_explores"`
	FolderName          string `json:"folder_name,omitempty"`
}

func CreateViewsFromTables(ctx context.Context, l *v4.LookerSDK, projectId string, queryParams ProjectGeneratorQueryParams, reqBody ProjectGeneratorRequestBody, options *rtl.ApiSettings) error {
	path := fmt.Sprintf("/projects/%s/generate", url.PathEscape(projectId))

	// Construct query parameter map
	query := map[string]any{
		"connection":             queryParams.Connection,
		"file_type_for_explores": queryParams.FileTypeForExplores,
		"folder_name":            queryParams.FolderName,
	}

	// The API expects a JSON object with a `tables` property containing the array.
	err := l.AuthSession.Do(nil, "POST", "/4.0", path, query, reqBody, options)

	logger, _ := util.LoggerFromContext(ctx)
	logger.DebugContext(ctx, fmt.Sprintf("generating views with request: query=%v body=%v error=%v", query, reqBody.Tables, err))
	return err
}

type ProjectCommitRequest struct {
	Files   *[]string `json:"files,omitempty"`   // List of files to commit
	Message *string   `json:"message,omitempty"` // Commit message
	Amend   *bool     `json:"amend,omitempty"`   // Amend the last commit
}

func ProjectCommit(l *v4.LookerSDK, projectId string, request ProjectCommitRequest, options *rtl.ApiSettings) (string, error) {
	var result string
	path := fmt.Sprintf("/projects/%s/commit", url.PathEscape(projectId))
	err := l.AuthSession.Do(&result, "POST", "/4.0", path, nil, request, options)
	return result, err
}

func PushBranch(l *v4.LookerSDK, projectId string, options *rtl.ApiSettings) (string, error) {
	projectId = url.PathEscape(projectId)
	var result string
	path := fmt.Sprintf("/projects/%v/push_branch", projectId)
	err := l.AuthSession.Do(&result, "POST", "/4.0", path, nil, nil, options)
	return result, err
}
