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
package lookerhealthvacuum

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

// =================================================================================================================
// START MCP SERVER CORE LOGIC
// =================================================================================================================
const resourceType string = "looker-health-vacuum"

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
	GetLookerSDK(context.Context, string) (*v4.LookerSDK, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	Parameters       map[string]any         `yaml:"parameters"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	actionParameter := parameters.NewStringParameter("action", "The vacuum action to run. Can be 'models', or 'explores'.", parameters.WithStringRequired(true))
	projectParameter := parameters.NewStringParameter("project", "The Looker project to vacuum (optional).", parameters.WithStringDefault(""))
	modelParameter := parameters.NewStringParameter("model", "The Looker model to vacuum (optional).", parameters.WithStringDefault(""))
	exploreParameter := parameters.NewStringParameter("explore", "The Looker explore to vacuum (optional).", parameters.WithStringDefault(""))
	timeframeParameter := parameters.NewIntParameter("timeframe", "The timeframe in days to analyze.", parameters.WithIntDefault(90))
	minQueriesParameter := parameters.NewIntParameter("min_queries", "The minimum number of queries for a model or explore to be considered used.", parameters.WithIntDefault(1))

	allParameters := parameters.Parameters{
		actionParameter,
		projectParameter,
		modelParameter,
		exploreParameter,
		timeframeParameter,
		minQueriesParameter,
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			lookercommon.ReadOnlyAnnotations(cfg.Annotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
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
	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}

	paramsMap := params.AsMap()
	timeframe, _ := paramsMap["timeframe"].(int)
	if timeframe == 0 {
		timeframe = 90
	}
	minQueries, _ := paramsMap["min_queries"].(int)
	if minQueries == 0 {
		minQueries = 1
	}

	vacuumTool := &vacuumTool{
		SdkClient:  sdk,
		timeframe:  timeframe,
		minQueries: minQueries,
	}

	action, ok := paramsMap["action"].(string)
	if !ok {
		return nil, util.NewAgentError("action parameter not found", nil)
	}

	var res []map[string]interface{}

	switch action {
	case "models":
		project, _ := paramsMap["project"].(string)
		model, _ := paramsMap["model"].(string)
		res, err = vacuumTool.models(ctx, project, model)
	case "explores":
		model, _ := paramsMap["model"].(string)
		explore, _ := paramsMap["explore"].(string)
		res, err = vacuumTool.explores(ctx, model, explore)
	default:
		return nil, util.NewAgentError(fmt.Sprintf("unknown action: %s", action), nil)
	}

	if err != nil {
		if strings.Contains(err.Error(), "status=401") {
			return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
		}
		return nil, util.ProcessGeneralError(err)
	}

	return res, nil
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}

// =================================================================================================================
// END MCP SERVER CORE LOGIC
// =================================================================================================================

// =================================================================================================================
// START LOOKER HEALTH VACUUM CORE LOGIC
// =================================================================================================================
type vacuumTool struct {
	SdkClient  *v4.LookerSDK
	timeframe  int
	minQueries int
}

func (t *vacuumTool) models(ctx context.Context, project, model string) ([]map[string]interface{}, error) {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get logger from ctx: %s", err)
	}
	logger.InfoContext(ctx, "Vacuuming models...")

	usedModels, err := t.getUsedModels(ctx)
	if err != nil {
		return nil, err
	}

	lookmlModels, err := t.SdkClient.AllLookmlModels(v4.RequestAllLookmlModels{}, nil)
	if err != nil {
		return nil, fmt.Errorf("error fetching LookML models: %w", err)
	}

	var results []map[string]interface{}
	for _, m := range lookmlModels {
		if (project == "" || (m.ProjectName != nil && *m.ProjectName == project)) &&
			(model == "" || (m.Name != nil && *m.Name == model)) {

			queryCount := 0
			if qc, ok := usedModels[*m.Name]; ok {
				queryCount = qc
			}

			unusedExplores, err := t.getUnusedExplores(ctx, *m.Name)
			if err != nil {
				return nil, err
			}

			results = append(results, map[string]interface{}{
				"Model":             *m.Name,
				"Unused Explores":   unusedExplores,
				"Model Query Count": queryCount,
			})
		}
	}
	return results, nil
}

func (t *vacuumTool) explores(ctx context.Context, model, explore string) ([]map[string]interface{}, error) {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get logger from ctx: %s", err)
	}
	logger.InfoContext(ctx, "Vacuuming explores...")

	lookmlModels, err := t.SdkClient.AllLookmlModels(v4.RequestAllLookmlModels{}, nil)
	if err != nil {
		return nil, fmt.Errorf("error fetching LookML models: %w", err)
	}

	var results []map[string]interface{}
	for _, m := range lookmlModels {
		if model != "" && (m.Name == nil || *m.Name != model) {
			continue
		}
		if m.Explores == nil {
			continue
		}

		for _, e := range *m.Explores {
			if explore != "" && (e.Name == nil || *e.Name != explore) {
				continue
			}
			if e.Name == nil {
				continue
			}

			exploreDetail, err := t.SdkClient.LookmlModelExplore(v4.RequestLookmlModelExplore{
				LookmlModelName: *m.Name,
				ExploreName:     *e.Name,
			}, nil)
			if err != nil {
				logger.ErrorContext(ctx, fmt.Sprintf("Error fetching detail for explore %s.%s: %v", *m.Name, *e.Name, err))
				continue
			}

			usedFields, err := t.getUsedExploreFields(ctx, *m.Name, *e.Name)
			if err != nil {
				logger.ErrorContext(ctx, fmt.Sprintf("Error fetching used fields for explore %s.%s: %v", *m.Name, *e.Name, err))
				continue
			}

			var allFields []string
			if exploreDetail.Fields != nil {
				for _, d := range *exploreDetail.Fields.Dimensions {
					if !*d.Hidden {
						allFields = append(allFields, *d.Name)
					}
				}
				for _, ms := range *exploreDetail.Fields.Measures {
					if !*ms.Hidden {
						allFields = append(allFields, *ms.Name)
					}
				}
			}

			var unusedFields []string
			for _, field := range allFields {
				if _, ok := usedFields[field]; !ok {
					unusedFields = append(unusedFields, field)
				}
			}

			joinStats := make(map[string]int)
			if exploreDetail.Joins != nil {
				for field, queryCount := range usedFields {
					join := strings.Split(field, ".")[0]
					joinStats[join] += queryCount
				}
				for _, join := range *exploreDetail.Joins {
					if _, ok := joinStats[*join.Name]; !ok {
						joinStats[*join.Name] = 0
					}
				}
			}

			var unusedJoins []string
			for join, count := range joinStats {
				if count == 0 {
					unusedJoins = append(unusedJoins, join)
				}
			}

			results = append(results, map[string]interface{}{
				"Model":         *m.Name,
				"Explore":       *e.Name,
				"Unused Joins":  unusedJoins,
				"Unused Fields": unusedFields,
			})
		}
	}
	return results, nil
}

func (t *vacuumTool) getUsedModels(ctx context.Context) (map[string]int, error) {
	limit := "5000"
	query := &v4.WriteQuery{
		Model:  "system__activity",
		View:   "history",
		Fields: &[]string{"history.query_run_count", "query.model"},
		Filters: &map[string]any{
			"history.created_date":    fmt.Sprintf("%d days", t.timeframe),
			"query.model":             "-system__activity, -i__looker",
			"history.query_run_count": fmt.Sprintf(">%d", t.minQueries-1),
			"user.dev_branch_name":    "NULL",
		},
		Limit: &limit,
	}
	raw, err := lookercommon.RunInlineQuery(ctx, t.SdkClient, query, "json", nil)
	if err != nil {
		return nil, err
	}

	var data []map[string]interface{}
	_ = json.Unmarshal([]byte(raw), &data)

	results := make(map[string]int)
	for _, row := range data {
		model, _ := row["query.model"].(string)
		count, _ := row["history.query_run_count"].(float64)
		results[model] = int(count)
	}
	return results, nil
}

func (t *vacuumTool) getUnusedExplores(ctx context.Context, modelName string) ([]string, error) {
	lookmlModel, err := t.SdkClient.LookmlModel(modelName, "", nil)
	if err != nil {
		return nil, fmt.Errorf("error fetching LookML model %s: %w", modelName, err)
	}

	var unusedExplores []string
	if lookmlModel.Explores != nil {
		for _, e := range *lookmlModel.Explores {
			limit := "1"
			queryCountQueryBody := &v4.WriteQuery{
				Model:  "system__activity",
				View:   "history",
				Fields: &[]string{"history.query_run_count"},
				Filters: &map[string]any{
					"query.model":             modelName,
					"query.view":              *e.Name,
					"history.created_date":    fmt.Sprintf("%d days", t.timeframe),
					"history.query_run_count": fmt.Sprintf(">%d", t.minQueries-1),
					"user.dev_branch_name":    "NULL",
				},
				Limit: &limit,
			}

			rawQueryCount, err := lookercommon.RunInlineQuery(ctx, t.SdkClient, queryCountQueryBody, "json", nil)
			if err != nil {
				// Log the error but continue
				continue
			}

			var data []map[string]interface{}
			_ = json.Unmarshal([]byte(rawQueryCount), &data)
			if len(data) == 0 {
				unusedExplores = append(unusedExplores, *e.Name)
			}
		}
	}
	return unusedExplores, nil
}

func (t *vacuumTool) getUsedExploreFields(ctx context.Context, model, explore string) (map[string]int, error) {
	limit := "5000"
	query := &v4.WriteQuery{
		Model:  "system__activity",
		View:   "history",
		Fields: &[]string{"query.formatted_fields", "query.filters", "history.query_run_count"},
		Filters: &map[string]any{
			"history.created_date":   fmt.Sprintf("%d days", t.timeframe),
			"query.model":            strings.ReplaceAll(model, "_", "^_"),
			"query.view":             strings.ReplaceAll(explore, "_", "^_"),
			"query.formatted_fields": "-NULL",
			"history.workspace_id":   "production",
		},
		Limit: &limit,
	}
	raw, err := lookercommon.RunInlineQuery(ctx, t.SdkClient, query, "json", nil)
	if err != nil {
		return nil, err
	}

	var data []map[string]interface{}
	_ = json.Unmarshal([]byte(raw), &data)

	results := make(map[string]int)
	fieldRegex := regexp.MustCompile(`(\w+\.\w+)`)

	for _, row := range data {
		count, _ := row["history.query_run_count"].(float64)
		formattedFields, _ := row["query.formatted_fields"].(string)
		filters, _ := row["query.filters"].(string)

		usedFields := make(map[string]bool)

		for _, field := range fieldRegex.FindAllString(formattedFields, -1) {
			results[field] += int(count)
			usedFields[field] = true
		}

		for _, field := range fieldRegex.FindAllString(filters, -1) {
			if _, ok := usedFields[field]; !ok {
				results[field] += int(count)
			}
		}
	}
	return results, nil
}

// =================================================================================================================
// END LOOKER HEALTH VACUUM CORE LOGIC
// =================================================================================================================

func (t Tool) GetAuthTokenHeaderName(source sources.Source) (string, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return "", fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.GetAuthTokenHeaderName(), nil
}
