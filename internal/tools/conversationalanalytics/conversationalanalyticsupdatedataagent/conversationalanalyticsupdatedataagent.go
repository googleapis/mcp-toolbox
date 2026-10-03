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

package conversationalanalyticsupdatedataagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

const toolType string = "conversational-analytics-update-data-agent"

func init() {
	if !tools.Register(toolType, newConfig) {
		panic(fmt.Sprintf("tool type %q already registered", toolType))
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
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	Location         string                 `yaml:"location"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

// validate interface
var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return toolType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for %q tool", toolType)
	}

	if cfg.Location == "" {
		cfg.Location = "global"
	}
	if err := util.ValidateGDAPathSegment(cfg.Location, "location"); err != nil {
		return nil, fmt.Errorf("invalid location for %q tool: %w", toolType, err)
	}

	dataAgentIDParameter := parameters.NewStringParameter("data_agent_id", "The ID of the data agent to update.")
	agentConfigParameter := parameters.NewMapParameter("agent_config", `The JSON representation of the DataAgent resource fields to update. Example:
{
  "displayName": "Updated Support Agent",
  "description": "Updated description",
  "dataAnalyticsAgent": {
    "publishedContext": {
      "systemInstruction": "You are a helpful support data analyst."
    }
  }
}`, "")
	updateMaskParameter := parameters.NewStringParameter("update_mask", `Comma-separated list of fields to update, using the API's camelCase JSON field names (e.g. "displayName,description" or "dataAnalyticsAgent.publishedContext.systemInstruction"). Every field listed here must also be present in agent_config.`)
	params := parameters.Parameters{dataAgentIDParameter, agentConfigParameter, updateMaskParameter}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: params.Manifest(), AuthRequired: cfg.AuthRequired},
			params,
		),
	}, nil
}

// validate interface
var _ tools.Tool = Tool{}

type Tool struct {
	tools.BaseTool[Config]
	endpoint     string
	httpClient   *http.Client
	pollInterval time.Duration
	pollTimeout  time.Duration
}

func (t Tool) GetSourceName() string {
	return t.Cfg.Source
}

func (t Tool) ValidateSource(source sources.Source) error {
	_, ok := source.(compatibleSource)
	if !ok {
		return fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return nil
}

func (t Tool) ToConfig() tools.ToolConfig {
	return t.Cfg
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}

	mapParams := params.AsMap()
	dataAgentID, _ := mapParams["data_agent_id"].(string)
	if err := util.ValidateGDAPathSegment(dataAgentID, "data_agent_id"); err != nil {
		return nil, err
	}

	// projectID and location come from operator config rather than the LLM:
	// location is validated in Initialize, and project IDs may be domain-scoped
	// (e.g. "google.com:my-project"), so both are only path-escaped in the URL.
	projectID := source.GetProjectID()

	agentConfigMap, ok := mapParams["agent_config"].(map[string]any)
	if !ok || len(agentConfigMap) == 0 {
		return nil, util.NewAgentError("agent_config must be a non-empty JSON object", nil)
	}

	rawUpdateMask, _ := mapParams["update_mask"].(string)
	var maskFields []string
	for _, f := range strings.Split(rawUpdateMask, ",") {
		trimmed := strings.TrimSpace(f)
		if trimmed != "" {
			maskFields = append(maskFields, trimmed)
		}
	}
	if len(maskFields) == 0 {
		return nil, util.NewAgentError("update_mask is required and must list at least one field to update", nil)
	}

	var missingFields []string
	for _, f := range maskFields {
		if !maskFieldPresent(agentConfigMap, f) {
			missingFields = append(missingFields, f)
		}
	}
	if len(missingFields) > 0 {
		return nil, util.NewAgentError(fmt.Sprintf("update_mask field(s) %v are not present in agent_config; the API clears masked fields that are omitted from the body, so every masked field must be provided in agent_config", missingFields), nil)
	}

	normalizedMask := strings.Join(maskFields, ",")

	agentConfigBytes, err := json.Marshal(agentConfigMap)
	if err != nil {
		return nil, util.NewClientServerError("invalid agent config", http.StatusBadRequest, err)
	}

	var tokenSource oauth2.TokenSource
	if source.UseClientAuthorization() {
		if accessToken == "" {
			return nil, util.NewClientServerError("tool is configured for client OAuth but no token was provided in the request header", http.StatusUnauthorized, nil)
		}
		tokenStr, err := accessToken.ParseBearerToken()
		if err != nil {
			return nil, util.NewClientServerError("error parsing access token", http.StatusUnauthorized, err)
		}
		tokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: tokenStr})
	} else {
		tokenSource, err = source.GoogleCloudTokenSourceWithScope(ctx, "")
		if err != nil {
			return nil, util.NewClientServerError("failed to get token source", http.StatusInternalServerError, err)
		}
		if tokenSource == nil {
			return nil, util.NewClientServerError("cloud-platform token source is missing", http.StatusInternalServerError, nil)
		}
	}

	endpoint := t.endpoint
	if endpoint == "" {
		endpoint = util.GetGDAEndpoint()
	}

	client := t.httpClient
	if client == nil {
		client, err = util.NewGDAClient(ctx, option.WithTokenSource(tokenSource))
		if err != nil {
			return nil, util.NewClientServerError("failed to create GDA client", http.StatusInternalServerError, err)
		}
		client.Timeout = 30 * time.Second
	}

	return updateDataAgent(ctx, client, endpoint, projectID, t.Cfg.Location, dataAgentID, normalizedMask, agentConfigBytes, t.pollInterval, t.pollTimeout)
}

func updateDataAgent(
	ctx context.Context,
	client *http.Client,
	endpoint, projectID, location, dataAgentID, normalizedMask string,
	agentConfigBytes []byte,
	pollInterval, pollTimeout time.Duration,
) (any, util.ToolboxError) {
	caURL := fmt.Sprintf(
		"%s/v1/projects/%s/locations/%s/dataAgents/%s?updateMask=%s",
		endpoint,
		url.PathEscape(projectID),
		url.PathEscape(location),
		url.PathEscape(dataAgentID),
		url.QueryEscape(normalizedMask),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, caURL, bytes.NewReader(agentConfigBytes))
	if err != nil {
		return nil, util.NewClientServerError("failed to create request", http.StatusInternalServerError, err)
	}
	req.Header.Set("X-Goog-API-Client", util.GDAClientID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, util.NewClientServerError("failed to send request", http.StatusInternalServerError, err)
	}
	respBody, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, util.NewAgentError(fmt.Sprintf("API returned error status: %d %s", resp.StatusCode, string(respBody)), nil)
	}
	if readErr != nil {
		return nil, util.NewClientServerError("failed to read response body", http.StatusInternalServerError, readErr)
	}

	var result map[string]any
	if len(bytes.TrimSpace(respBody)) == 0 {
		result = map[string]any{"done": true}
	} else if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, util.NewClientServerError("failed to decode response", http.StatusInternalServerError, err)
	}

	return util.AwaitGDAOperation(ctx, client, endpoint, result, "update", pollInterval, pollTimeout)
}

func (t Tool) RequiresClientAuthorization(s sources.Source) (bool, error) {
	source, ok := s.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return source.UseClientAuthorization(), nil
}

func maskFieldPresent(config map[string]any, fieldPath string) bool {
	var current any = config
	for _, part := range strings.Split(fieldPath, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return false
		}
		val, exists := m[part]
		if !exists {
			return false
		}
		current = val
	}
	return true
}
