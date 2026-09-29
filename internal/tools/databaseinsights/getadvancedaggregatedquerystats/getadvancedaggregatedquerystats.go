// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package getadvancedaggregatedquerystats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/databaseinsights"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "databaseinsights-get-advanced-aggregated-query-stats"

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
	FetchQueryStats(ctx context.Context, req *databaseinsights.FetchQueryStatsRequest) (*databaseinsights.FetchQueryStatsResponse, error)
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
		cfg.Description = `Fetches aggregated query execution statistics for a requested AlloyDB instance within a specified time period. Supports filtering by database name, database user, and a specific query ID, as well as pagination. Returns statistics including query ID, database name, total execution time (sum(execution_time)), average execution time (avg(execution_time)), total execution count (sum(count)), total wait time (sum(wait_time)), and normalized query text. Requires advanced query insights to be enabled.
**IMPORTANT:**
  Use these parameters to narrow down the results as requested by the user.
  - If the user asks for a specific number of results (e.g., "Top 10", "First 5", "List 50"), map that number directly to page_size.
  - If they ask for a specific user or database, use the corresponding arguments.
  - When fetching results for a time range (e.g., "last 24 hours"), you MUST calculate and provide EXPLICIT 'start_time' AND 'end_time' in RFC3339 format. This pins the data window so that 'page_token' remains valid for subsequent requests. If 'page_token' is present, 'start_time' and 'end_time' MUST match the initial request.
Use Cases:
  - Identify top queries: "I want execution details about top queries executed on database <database_name> on instance <instance_id> in the last 6 hours."
  - Monitor specific user activity: "What were the top queries run by user <user_name> in the last hour?"
Response:
  Returns a ranked list of queries with metrics: sum(execution_time), sum(count), avg(execution_time), sum(wait_time), and normalized_query_text.
**UNIT AND FORMATTING RULES:**
  1. **Execution and Wait times**: Metrics like 'sum(execution_time)', 'avg(execution_time)', and 'sum(wait_time)' are in microseconds.
     ALWAYS report these values, rounded to two decimal places, in **milliseconds** (ms) if less than 1 second, or in **seconds** (s) if 1 second or more.
     Example: 1112.3 microseconds should be reported as 1.11 ms.`
	}
	params := buildParams()
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
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

	paramsMap := params.AsMap()

	parent, ok := paramsMap["parent"].(string)
	if !ok || parent == "" {
		return nil, util.NewAgentError("invalid or missing 'parent' parameter; expected a non-empty string", nil)
	}

	fullResourceName, ok := paramsMap["full_resource_name"].(string)
	if !ok || fullResourceName == "" {
		return nil, util.NewAgentError("invalid or missing 'full_resource_name' parameter; expected a non-empty string", nil)
	}

	req := &databaseinsights.FetchQueryStatsRequest{
		Parent:           parent,
		FullResourceName: fullResourceName,
	}

	if startTime, ok := paramsMap["start_time"].(string); ok && startTime != "" {
		req.StartTime = startTime
	}
	if endTime, ok := paramsMap["end_time"].(string); ok && endTime != "" {
		req.EndTime = endTime
	}
	if database, ok := paramsMap["database"].(string); ok && database != "" {
		req.Database = database
	}
	if username, ok := paramsMap["username"].(string); ok && username != "" {
		req.Username = username
	}
	if queryId, ok := paramsMap["query_id"].(string); ok && queryId != "" {
		req.QueryID = queryId
	}
	if pageToken, ok := paramsMap["page_token"].(string); ok && pageToken != "" {
		req.PageToken = pageToken
	}

	if pageSizeVal, ok := paramsMap["page_size"]; ok {
		switch val := pageSizeVal.(type) {
		case int:
			req.PageSize = int32(val)
		case int64:
			req.PageSize = int32(val)
		case float64:
			req.PageSize = int32(val)
		case json.Number:
			if i, err := val.Int64(); err == nil {
				req.PageSize = int32(i)
			}
		}
	}

	resp, err := source.FetchQueryStats(ctx, req)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	return resp, nil
}

func buildParams() parameters.Parameters {
	return parameters.Parameters{
		parameters.NewStringParameter("parent", "Required. Project and location. Format: projects/{project_id}/locations/{location}", parameters.WithStringRequired(true)),
		parameters.NewStringParameter("full_resource_name", "Required. The full identifier for the AlloyDB instance. Provide the full resource name ONLY in the following format: //alloydb.googleapis.com/projects/{project_id}/locations/{location}/clusters/{cluster_id}/instances/{instance_id}", parameters.WithStringRequired(true)),
		parameters.NewStringParameter("start_time", "Optional. Beginning of the interval for fetching stats in RFC3339 format (Defaults to 1 hour ago).", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("end_time", "Optional. End of the interval for fetching stats in RFC3339 format (Defaults to 'now').", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("database", "Optional. Filter results to a specific database name.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("username", "Optional. Filter results to a specific database user.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("query_id", "Optional. Fetch aggregated statistics for a single, specific query hash.", parameters.WithStringRequired(false)),
		parameters.NewIntParameter("page_size", "Optional. Maximum number of query stats to return (Default: 20).", parameters.WithIntRequired(false)),
		parameters.NewStringParameter("page_token", "Optional. Token for fetching the next set of results.", parameters.WithStringRequired(false)),
	}
}
