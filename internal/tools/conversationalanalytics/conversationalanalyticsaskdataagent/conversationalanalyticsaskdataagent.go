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

package conversationalanalyticsaskdataagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

const resourceType string = "conversational-analytics-ask-data-agent"

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

type BQTableReference struct {
	ProjectID string `json:"projectId"`
	DatasetID string `json:"datasetId"`
	TableID   string `json:"tableId"`
}

// Structs for building the JSON payload
type UserMessage struct {
	Text string `json:"text"`
}
type Message struct {
	UserMessage UserMessage `json:"userMessage"`
}

type DataAgentContext struct {
	DataAgent string `json:"dataAgent"`
}

type CAPayload struct {
	Messages         []Message        `json:"messages"`
	DataAgentContext DataAgentContext `json:"dataAgentContext"`
	ClientIdEnum     string           `json:"clientIdEnum"`
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
	Location         string `yaml:"location"`
	MaxResults       int    `yaml:"maxResults"`
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
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 50
	}

	dataAgentIdDescription := `The ID of the data agent to ask.`
	userQueryParameter := parameters.NewStringParameter("user_query_with_context", "The question to ask the agent, potentially including conversation history for context.")
	dataAgentIdParameter := parameters.NewStringParameter("data_agent_id", dataAgentIdDescription)
	params := parameters.Parameters{dataAgentIdParameter, userQueryParameter}

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

	// Extract parameters from the map
	mapParams := params.AsMap()
	dataAgentId, _ := mapParams["data_agent_id"].(string)
	userQuery, _ := mapParams["user_query_with_context"].(string)

	// Construct URL, headers, and payload
	projectID := source.GetProjectID()
	caURL := fmt.Sprintf("%s/v1/projects/%s/locations/%s:chat", util.GetGDAEndpoint(), projectID, t.Cfg.Location)

	headers := map[string]string{
		"Content-Type":      "application/json",
		"X-Goog-API-Client": util.GDAClientID,
	}

	dataAgentName := fmt.Sprintf("projects/%s/locations/%s/dataAgents/%s", projectID, t.Cfg.Location, dataAgentId)

	payload := CAPayload{
		Messages: []Message{{UserMessage: UserMessage{Text: userQuery}}},
		DataAgentContext: DataAgentContext{
			DataAgent: dataAgentName,
		},
		ClientIdEnum: util.GDAClientID,
	}

	client, err := util.NewGDAClient(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, util.NewClientServerError("failed to create GDA client", http.StatusInternalServerError, err)
	}
	client.Timeout = 330 * time.Second

	// Call the streaming API
	response, err := getStream(client, caURL, payload, headers, t.Cfg.MaxResults)
	if err != nil {
		return nil, util.NewAgentError("failed to get response from conversational analytics API", err)
	}

	return response, nil
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}

func getStream(client *http.Client, url string, payload CAPayload, headers map[string]string, maxRows int) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned non-200 status: %d %s", resp.StatusCode, string(body))
	}

	var messages []map[string]any
	decoder := json.NewDecoder(resp.Body)
	dataMsgIdx := -1

	// The response is a JSON array, so we read the opening bracket.
	if _, err := decoder.Token(); err != nil {
		if err == io.EOF {
			return "", nil // Empty response is valid
		}
		return "", fmt.Errorf("error reading start of json array: %w", err)
	}

	for decoder.More() {
		var rawMsg json.RawMessage
		if err := decoder.Decode(&rawMsg); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("error decoding raw message: %w", err)
		}

		var msg map[string]any
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			return "", fmt.Errorf("error unmarshaling raw message: %w", err)
		}

		var processedMsg map[string]any
		if dataResult := extractDataResult(msg); dataResult != nil {
			// 1. If it's a data result, format it.
			processedMsg = formatDataRetrieved(dataResult, maxRows)
			if dataMsgIdx >= 0 {
				// Replace previous data with a placeholder. Intermediate data results in a
				// stream are redundant and consume unnecessary tokens.
				messages[dataMsgIdx] = map[string]any{"Data Retrieved": "Intermediate result omitted"}
			}
			dataMsgIdx = len(messages)
		} else if sm, ok := msg["systemMessage"].(map[string]any); ok {
			// 2. If it's a system message, unwrap it.
			processedMsg = sm
		} else {
			// 3. Otherwise (e.g. error), pass it through raw.
			processedMsg = msg
		}

		if processedMsg != nil {
			messages = append(messages, processedMsg)
		}
	}

	var acc strings.Builder
	for i, msg := range messages {
		jsonBytes, err := json.Marshal(msg)
		if err != nil {
			return "", fmt.Errorf("error marshalling message: %w", err)
		}
		acc.Write(jsonBytes)
		if i < len(messages)-1 {
			acc.WriteString("\n")
		}
	}

	return acc.String(), nil
}

// extractDataResult attempts to find the result.data deep inside the generic map.
func extractDataResult(msg map[string]any) map[string]any {
	sm, ok := msg["systemMessage"].(map[string]any)
	if !ok {
		return nil
	}
	data, ok := sm["data"].(map[string]any)
	if !ok {
		return nil
	}
	result, ok := data["result"].(map[string]any)
	if !ok {
		return nil
	}
	if _, hasData := result["data"].([]any); hasData {
		return result
	}
	return nil
}

// formatDataRetrieved transforms the raw result map into the simplified Toolbox format.
func formatDataRetrieved(result map[string]any, maxRows int) map[string]any {
	rawData, _ := result["data"].([]any)

	var fields []any
	if schema, ok := result["schema"].(map[string]any); ok {
		if f, ok := schema["fields"].([]any); ok {
			fields = f
		}
	}

	var headers []string
	for _, f := range fields {
		if fm, ok := f.(map[string]any); ok {
			if name, ok := fm["name"].(string); ok {
				headers = append(headers, name)
			}
		}
	}

	totalRows := len(rawData)
	numToDisplay := totalRows
	if numToDisplay > maxRows {
		numToDisplay = maxRows
	}

	var rows [][]any
	for _, r := range rawData[:numToDisplay] {
		if rm, ok := r.(map[string]any); ok {
			var row []any
			for _, h := range headers {
				row = append(row, rm[h])
			}
			rows = append(rows, row)
		}
	}

	summary := fmt.Sprintf("Showing all %d rows.", totalRows)
	if totalRows > maxRows {
		summary = fmt.Sprintf("Showing the first %d of %d total rows.", numToDisplay, totalRows)
	}

	return map[string]any{
		"Data Retrieved": map[string]any{
			"headers": headers,
			"rows":    rows,
			"summary": summary,
		},
	}
}
