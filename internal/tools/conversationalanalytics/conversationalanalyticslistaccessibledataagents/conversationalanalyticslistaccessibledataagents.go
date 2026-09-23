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

package conversationalanalyticslistaccessibledataagents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	cloudgdads "github.com/googleapis/mcp-toolbox/internal/sources/cloudgda"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

const resourceType string = "conversational-analytics-list-accessible-data-agents"

const (
	minPageSize       = 1
	maxAutoDataAgents = 1000
	maxAutoPages      = 100
	// maxAutoDuration bounds the total time an automatic drain may spend.
	maxAutoDuration = 5 * time.Minute
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
	GoogleCloudTokenSourceWithScope(ctx context.Context, scope string) (oauth2.TokenSource, error)
	GetProjectID() string
	UseClientAuthorization() bool
}

// validate compatible sources are still compatible
var _ compatibleSource = &cloudgdads.Source{}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
	Location         string `yaml:"location"`
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

	if cfg.Location == "" {
		cfg.Location = "global"
	}

	minPageSizeValue := minPageSize
	params := parameters.Parameters{
		parameters.NewIntParameter(
			"page_size",
			"Optional. The maximum number of data agents to return in this call. Must be a positive integer. Only set this to page through the results manually: when both `page_size` and `page_token` are omitted, every page is fetched automatically and all accessible data agents are returned.",
			parameters.WithIntRequired(false),
			parameters.WithIntMinValue(&minPageSizeValue),
		),
		parameters.NewStringParameter(
			"page_token",
			"Optional. A page token returned as `nextPageToken` by a previous call, used to fetch the next page. Only set this to page through the results manually; omit it to fetch all accessible data agents at once.",
			parameters.WithStringRequired(false),
		),
	}

	// finish tool setup
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			nil,
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

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}

	pageSize, pageToken, tErr := parsePaginationParams(params)
	if tErr != nil {
		return nil, tErr
	}

	var tokenSource oauth2.TokenSource
	var err error
	// Get credentials for the API call
	if source.UseClientAuthorization() {
		// Use client-side access token
		if accessToken == "" {
			return nil, util.NewClientServerError("tool is configured for client OAuth but no token was provided in the request header", http.StatusUnauthorized, nil)
		}
		tokenStr, err := accessToken.ParseBearerToken()
		if err != nil {
			return nil, util.NewClientServerError("error parsing access token", http.StatusUnauthorized, err)
		}
		tokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: tokenStr})
	} else {
		// Get a token source for the Gemini Data Analytics API.
		tokenSource, err = source.GoogleCloudTokenSourceWithScope(ctx, "")
		if err != nil {
			return nil, util.NewClientServerError("failed to get token source", http.StatusInternalServerError, err)
		}

		// Use cloud-platform token source for Gemini Data Analytics API
		if tokenSource == nil {
			return nil, util.NewClientServerError("cloud-platform token source is missing", http.StatusInternalServerError, nil)
		}
	}

	client, err := util.NewGDAClient(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, util.NewClientServerError("failed to create GDA client", http.StatusInternalServerError, err)
	}
	client.Timeout = 30 * time.Second

	return listAccessibleDataAgents(ctx, client, util.GetGDAEndpoint(), source.GetProjectID(), t.Cfg.Location, pageSize, pageToken)
}

// parsePaginationParams extracts optional page_size and page_token parameters.
func parsePaginationParams(params parameters.ParamValues) (*int, string, util.ToolboxError) {
	paramsMap := params.AsMap()

	var pageSize *int
	if ps, ok := paramsMap["page_size"]; ok && ps != nil {
		size, ok := ps.(int)
		if !ok {
			return nil, "", util.NewAgentError(fmt.Sprintf("error casting 'page_size' parameter: %v", ps), nil)
		}
		if size < minPageSize {
			return nil, "", util.NewAgentError(fmt.Sprintf("'page_size' must be positive, got %d", size), nil)
		}
		pageSize = &size
	}

	var pageToken string
	if pt, ok := paramsMap["page_token"]; ok && pt != nil {
		s, ok := pt.(string)
		if !ok {
			return nil, "", util.NewAgentError(fmt.Sprintf("error casting 'page_token' parameter: %v", pt), nil)
		}
		pageToken = s
	}
	return pageSize, pageToken, nil
}

// listAccessibleDataAgents returns a single page when pagination parameters are provided, or all pages by default.
func listAccessibleDataAgents(ctx context.Context, client *http.Client, endpoint, projectID, location string, pageSize *int, pageToken string) (any, util.ToolboxError) {
	if pageSize != nil || pageToken != "" {
		body, tErr := listAccessibleDataAgentsPage(ctx, client, endpoint, projectID, location, pageSize, pageToken)
		if tErr != nil {
			return nil, tErr
		}
		var result any
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, util.NewClientServerError("failed to decode response", http.StatusInternalServerError, err)
		}
		return result, nil
	}
	return listAllAccessibleDataAgents(ctx, client, endpoint, projectID, location, maxAutoDuration)
}

// parseDataAgentsPage splits a page into its data agents, its page token, and
// any other top-level fields, which are kept as raw JSON.
func parseDataAgentsPage(body []byte) ([]json.RawMessage, string, map[string]json.RawMessage, error) {
	var page map[string]json.RawMessage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, "", nil, err
	}
	if page == nil {
		return nil, "", nil, fmt.Errorf("response is not a JSON object")
	}

	var dataAgents []json.RawMessage
	if raw, ok := page["dataAgents"]; ok {
		if err := json.Unmarshal(raw, &dataAgents); err != nil {
			return nil, "", nil, fmt.Errorf("invalid 'dataAgents' field: %w", err)
		}
	}

	var nextPageToken string
	if raw, ok := page["nextPageToken"]; ok {
		if err := json.Unmarshal(raw, &nextPageToken); err != nil {
			return nil, "", nil, fmt.Errorf("invalid 'nextPageToken' field: %w", err)
		}
	}

	delete(page, "dataAgents")
	delete(page, "nextPageToken")
	return dataAgents, nextPageToken, page, nil
}

// mergeExtras folds a page's extra top-level fields into the ones collected so
// far, concatenating JSON arrays so lists such as `unreachable` keep every page.
func mergeExtras(extras, pageExtras map[string]json.RawMessage) {
	for field, value := range pageExtras {
		extras[field] = concatJSONArrays(extras[field], value)
	}
}

// concatJSONArrays joins two JSON arrays. When the values cannot be merged it
// keeps whichever side holds real data: a non-array `prev` is overwritten by
// `next` (last page wins for scalars), while an unmergeable `next` leaves the
// already aggregated `prev` untouched so earlier pages are never dropped.
func concatJSONArrays(prev, next json.RawMessage) json.RawMessage {
	if len(prev) == 0 {
		return next
	}
	if len(next) == 0 {
		return prev
	}
	var prevItems, nextItems []json.RawMessage
	if json.Unmarshal(prev, &prevItems) != nil || prevItems == nil {
		return next
	}
	if json.Unmarshal(next, &nextItems) != nil || nextItems == nil {
		// Keep what was aggregated so far instead of discarding earlier pages.
		return prev
	}
	merged, err := json.Marshal(append(prevItems, nextItems...))
	if err != nil {
		return prev
	}
	return merged
}

// drainResult assembles a drained response. `nextPageToken` is omitted once the
// drain reached the end of the list.
func drainResult(extras map[string]json.RawMessage, dataAgents []json.RawMessage, pageToken string) map[string]any {
	result := make(map[string]any, len(extras)+2)
	for field, value := range extras {
		result[field] = value
	}
	result["dataAgents"] = dataAgents
	if pageToken != "" {
		result["nextPageToken"] = pageToken
	}
	return result
}

// listAllAccessibleDataAgents fetches pages until completion or safety limits.
// budget caps the total time spent draining pages; callers pass maxAutoDuration.
func listAllAccessibleDataAgents(ctx context.Context, client *http.Client, endpoint, projectID, location string, budget time.Duration) (any, util.ToolboxError) {
	drainCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	dataAgents := []json.RawMessage{}
	pageToken := ""
	extras := map[string]json.RawMessage{}

	for page := 0; page < maxAutoPages; page++ {
		body, tErr := listAccessibleDataAgentsPage(drainCtx, client, endpoint, projectID, location, nil, pageToken)
		if tErr != nil {
			if ctx.Err() == nil && errors.Is(tErr, context.DeadlineExceeded) {
				return nil, util.NewAgentError("timed out while fetching accessible data agents; set 'page_size' and 'page_token' to page through them instead", tErr)
			}
			return nil, tErr
		}

		pageAgents, nextPageToken, pageExtras, err := parseDataAgentsPage(body)
		if err != nil {
			return nil, util.NewClientServerError("failed to decode response", http.StatusInternalServerError, err)
		}

		dataAgents = append(dataAgents, pageAgents...)
		mergeExtras(extras, pageExtras)

		// A repeated token means the API is not advancing; stop rather than loop.
		if nextPageToken == pageToken {
			nextPageToken = ""
		}
		pageToken = nextPageToken

		if pageToken == "" || len(dataAgents) >= maxAutoDataAgents {
			break
		}
	}

	return drainResult(extras, dataAgents, pageToken), nil
}

// listAccessibleDataAgentsPage sends a GET request for a single page of data agents.
func listAccessibleDataAgentsPage(ctx context.Context, client *http.Client, endpoint, projectID, location string, pageSize *int, pageToken string) ([]byte, util.ToolboxError) {
	caURL := fmt.Sprintf("%s/v1/projects/%s/locations/%s/dataAgents:listAccessible", endpoint, projectID, location)

	query := url.Values{}
	if pageSize != nil {
		query.Set("pageSize", strconv.Itoa(*pageSize))
	}
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	if len(query) > 0 {
		caURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, caURL, nil)
	if err != nil {
		return nil, util.NewClientServerError("failed to create request", http.StatusInternalServerError, err)
	}
	req.Header.Set("X-Goog-API-Client", util.GDAClientID)

	resp, err := client.Do(req)
	if err != nil {
		return nil, util.NewClientServerError("failed to send request", http.StatusInternalServerError, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, util.NewClientServerError("failed to read response", http.StatusInternalServerError, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, util.NewAgentError(fmt.Sprintf("API returned non-200 status: %d %s", resp.StatusCode, string(body)), nil)
	}
	return body, nil
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}
