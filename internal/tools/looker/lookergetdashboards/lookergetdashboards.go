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
package lookergetdashboards

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
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

const resourceType string = "looker-get-dashboards"

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

	titleParameter := parameters.NewStringParameter("title", "The title of the dashboard.", parameters.WithStringDefault(""))
	descParameter := parameters.NewStringParameter("desc", "The description of the dashboard.", parameters.WithStringDefault(""))
	limitParameter := parameters.NewIntParameter("limit", "The number of dashboards to fetch. Default 100", parameters.WithIntDefault(100))
	offsetParameter := parameters.NewIntParameter("offset", "The number of dashboards to skip before fetching. Default 0", parameters.WithIntDefault(0))
	params := parameters.Parameters{
		titleParameter,
		descParameter,
		limitParameter,
		offsetParameter,
	}

	// finish tool setup
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			lookercommon.ReadOnlyAnnotations(cfg.Annotations),
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

func dashboardToMap(v v4.Dashboard) map[string]any {
	vMap := make(map[string]any)
	if v.Id != nil {
		vMap["id"] = *v.Id
	}
	if v.Title != nil {
		vMap["title"] = *v.Title
	}
	if v.Description != nil {
		vMap["description"] = *v.Description
	}
	if v.CertificationMetadata != nil {
		vMap["certification_metadata"] = v.CertificationMetadata
	}
	return vMap
}

type lookerFieldMatcher struct {
	raw     string
	isNull  bool
	notNull bool
	re      *regexp.Regexp
}

// compileLookerFieldMatcher pre-compiles a Looker search pattern
// (case-insensitive, supporting '%' and '_' SQL LIKE wildcards, "IS NULL",
// and "NOT NULL"; if no '%' wildcard is specified, leaves the regex unanchored
// so partial title, underscore wildcard, or LookML dashboard ID/slug substring
// searches work in a single pass).
func compileLookerFieldMatcher(pattern string) lookerFieldMatcher {
	if pattern == "" {
		return lookerFieldMatcher{}
	}
	trimmed := strings.TrimSpace(pattern)
	if strings.EqualFold(trimmed, "IS NULL") {
		return lookerFieldMatcher{raw: pattern, isNull: true}
	}
	if strings.EqualFold(trimmed, "NOT NULL") {
		return lookerFieldMatcher{raw: pattern, notNull: true}
	}
	if pattern == "%" {
		return lookerFieldMatcher{raw: pattern}
	}
	patLower := strings.ToLower(pattern)
	hasPercent := strings.Contains(patLower, "%")

	var sb strings.Builder
	if hasPercent {
		sb.WriteString("(?s)^")
	} else {
		sb.WriteString("(?s)")
	}
	for _, ch := range patLower {
		switch ch {
		case '%':
			sb.WriteString(".*")
		case '_':
			sb.WriteString(".")
		default:
			sb.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	if hasPercent {
		sb.WriteString("$")
	}
	re, _ := regexp.Compile(sb.String())
	return lookerFieldMatcher{raw: pattern, re: re}
}

func (m lookerFieldMatcher) matches(val *string) bool {
	if m.raw == "" {
		return true
	}
	if m.isNull {
		return val == nil || *val == ""
	}
	if m.notNull {
		return val != nil && *val != ""
	}
	if val == nil {
		return false
	}
	if m.re == nil {
		return true
	}
	return m.re.MatchString(strings.ToLower(*val))
}

func matchesLookmlDashboard(v v4.Dashboard, titleMatcher, descMatcher lookerFieldMatcher) bool {
	if titleMatcher.raw != "" {
		if titleMatcher.isNull || titleMatcher.notNull {
			if !titleMatcher.matches(v.Title) {
				return false
			}
		} else if !titleMatcher.matches(v.Title) && !titleMatcher.matches(v.Id) {
			return false
		}
	}
	if descMatcher.raw != "" {
		if !descMatcher.matches(v.Description) {
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
	title := paramsMap["title"].(string)
	title_ptr := &title
	if *title_ptr == "" {
		title_ptr = nil
	}
	desc := paramsMap["desc"].(string)
	desc_ptr := &desc
	if *desc_ptr == "" {
		desc_ptr = nil
	}
	limit := int64(paramsMap["limit"].(int))
	if limit < 0 {
		limit = 0
	}
	offset := int64(paramsMap["offset"].(int))
	if offset < 0 {
		offset = 0
	}

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}
	fields := "id,title,description,certification_metadata"
	fetchLimit := limit
	if limit > 0 && offset > 0 {
		fetchLimit = offset + limit
	}
	zeroOffset := int64(0)
	req := v4.RequestSearchDashboards{
		Title:       title_ptr,
		Description: desc_ptr,
		Limit:       &fetchLimit,
		Offset:      &zeroOffset,
		Fields:      &fields,
	}
	logger.DebugContext(ctx, "Making request", "request", req)
	resp, err := sdk.SearchDashboards(req, source.LookerApiSettings())
	if err != nil {
		if strings.Contains(err.Error(), "status=401") {
			return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
		}
		return nil, util.ProcessGeneralError(err)
	}
	logger.DebugContext(ctx, "Got response", "response", resp)
	var data []any
	seenIDs := make(map[string]bool)
	for _, v := range resp {
		logger.DebugContext(ctx, "Got response element", "element", v)
		if v.Id != nil {
			seenIDs[*v.Id] = true
		}
		vMap := dashboardToMap(v)
		logger.DebugContext(ctx, "Converted to map", "map", vMap)
		data = append(data, vMap)
	}

	if fetchLimit <= 0 || int64(len(data)) < fetchLimit {
		lookmlResp, lookmlErr := lookercommon.SearchLookmlDashboards(sdk, map[string]any{"fields": fields}, source.LookerApiSettings())
		if lookmlErr != nil {
			logger.DebugContext(ctx, "error searching lookml dashboards", "error", lookmlErr)
		} else {
			titleMatcher := compileLookerFieldMatcher(title)
			descMatcher := compileLookerFieldMatcher(desc)
			for _, v := range lookmlResp {
				if v.Id != nil && seenIDs[*v.Id] {
					continue
				}
				if !matchesLookmlDashboard(v, titleMatcher, descMatcher) {
					continue
				}
				if v.Id != nil {
					seenIDs[*v.Id] = true
				}
				vMap := dashboardToMap(v)
				logger.DebugContext(ctx, "Converted LookML dashboard", "dashboard", vMap)
				data = append(data, vMap)
				if fetchLimit > 0 && int64(len(data)) >= fetchLimit {
					break
				}
			}
		}
	}

	if offset > 0 {
		if int(offset) >= len(data) {
			data = []any{}
		} else {
			data = data[offset:]
		}
	}
	if limit > 0 && int64(len(data)) > limit {
		data = data[:limit]
	}

	logger.DebugContext(ctx, "Final dashboard results", "data", data)

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
