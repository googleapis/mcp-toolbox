// Copyright 2025 Google LLC
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

package lookerconversationalanalytics

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
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

const resourceType string = "looker-conversational-analytics"

const instructions = `**INSTRUCTIONS - FOLLOW THESE RULES:**
1. **CONTENT:** Your answer should present the supporting data and then provide a conclusion based on that data.
2. **OUTPUT FORMAT:** Your entire response MUST be in plain text format ONLY.
3. **NO CHARTS:** You are STRICTLY FORBIDDEN from generating any charts, graphs, images, or any other form of visualization.`

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
	GoogleCloudProject() string
	GoogleCloudLocation() string
	GoogleCloudQuotaProject() string
	UseClientAuthorization() bool
	GetAuthTokenHeaderName() string
	LookerApiSettings() *rtl.ApiSettings
	GetLookerSDK(context.Context, string) (*v4.LookerSDK, error)
	GetHostURL(context.Context, *v4.LookerSDK) (string, error)
}

// Structs for building the JSON payload
type UserMessage struct {
	Text string `json:"text"`
}
type Message struct {
	UserMessage UserMessage `json:"userMessage"`
}
type LookerExploreReference struct {
	LookerInstanceUri string `json:"lookerInstanceUri"`
	LookmlModel       string `json:"lookmlModel"`
	Explore           string `json:"explore"`
}
type LookerExploreReferences struct {
	ExploreReferences []LookerExploreReference `json:"exploreReferences"`
}
type SecretBased struct {
	ClientId     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}
type TokenBased struct {
	AccessToken string `json:"access_token"`
}
type OAuthCredentials struct {
	Secret SecretBased `json:"secret,omitzero"`
	Token  TokenBased  `json:"token,omitzero"`
}
type Credentials struct {
	OAuth OAuthCredentials `json:"oauth"`
}
type DatasourceReferences struct {
	Looker LookerExploreReferences `json:"looker"`
}
type ImageOptions struct {
	NoImage map[string]any `json:"noImage"`
}
type ChartOptions struct {
	Image ImageOptions `json:"image"`
}
type Python struct {
	Enabled bool `json:"enabled"`
}
type AnalysisOptions struct {
	Python Python `json:"python"`
}
type ConversationOptions struct {
	Chart    ChartOptions    `json:"chart,omitzero"`
	Analysis AnalysisOptions `json:"analysis,omitzero"`
}
type InlineContext struct {
	SystemInstruction    string               `json:"systemInstruction"`
	DatasourceReferences DatasourceReferences `json:"datasourceReferences"`
}
type CAPayload struct {
	Messages      []Message     `json:"messages"`
	InlineContext InlineContext `json:"inlineContext"`
	Credentials   *Credentials  `json:"credentials,omitempty"`
	ClientIdEnum  string        `json:"clientIdEnum"`
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

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	userQueryParameter := parameters.NewStringParameter("user_query_with_context", "The user's question, potentially including conversation history and system instructions for context.")

	exploreRefsDescription := `An Array of at least one and up to 5 explore references like [{'model': 'MODEL_NAME', 'explore': 'EXPLORE_NAME'}]`
	exploreRefsParameter := parameters.NewArrayParameter(
		"explore_references",
		exploreRefsDescription,
		parameters.NewMapParameter(
			"explore_reference",
			"An explore reference like {'model': 'MODEL_NAME', 'explore': 'EXPLORE_NAME'}",
			"",
		),
	)

	params := parameters.Parameters{userQueryParameter, exploreRefsParameter}

	// finish tool setup
	t := Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			lookercommon.ReadOnlyAnnotations(cfg.Annotations),
			tools.Manifest{Description: cfg.Description, Parameters: params.Manifest(), AuthRequired: cfg.AuthRequired},
			params,
		),
	}
	return t, nil
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

// parseExploreReferences converts the raw explore_references parameter into typed
// references. The parameter is declared as an array of free-form maps, so the values
// the model supplies are not schema-validated; guard every field access instead of
// letting an unexpected shape (a missing key, a non-string value, or a non-object
// element) panic the tool. Mirrors the validation in datalineagesearchlineage.
func parseExploreReferences(raw []any, lookerInstanceURI string) ([]LookerExploreReference, util.ToolboxError) {
	refs := make([]LookerExploreReference, 0, len(raw))
	for i, er := range raw {
		m, ok := er.(map[string]any)
		if !ok {
			return nil, util.NewAgentError(fmt.Sprintf("invalid explore reference at index %d in 'explore_references': expected object, got %T", i, er), nil)
		}
		model, ok := m["model"].(string)
		if !ok {
			return nil, util.NewAgentError(fmt.Sprintf("missing or invalid 'model' (expected string) in 'explore_references' at index %d", i), nil)
		}
		explore, ok := m["explore"].(string)
		if !ok {
			return nil, util.NewAgentError(fmt.Sprintf("missing or invalid 'explore' (expected string) in 'explore_references' at index %d", i), nil)
		}
		refs = append(refs, LookerExploreReference{
			LookerInstanceUri: lookerInstanceURI,
			LookmlModel:       model,
			Explore:           explore,
		})
	}
	return refs, nil
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

	if source.GoogleCloudProject() == "" {
		return nil, util.NewClientServerError(fmt.Sprintf("project must be defined for source to use with %q tool", resourceType), http.StatusInternalServerError, nil)
	}

	// Get credentials for the API call
	// Use cloud-platform token source for Gemini Data Analytics API
	tokenSource, err := source.GoogleCloudTokenSourceWithScope(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, util.NewClientServerError("failed to get cloud-platform token source", http.StatusInternalServerError, err)
	}

	// Extract parameters from the map
	mapParams := params.AsMap()
	userQuery, _ := mapParams["user_query_with_context"].(string)
	exploreReferences, _ := mapParams["explore_references"].([]any)

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}

	hostURL, err := source.GetHostURL(ctx, sdk)
	if err != nil {
		logger.WarnContext(ctx, "failed to dynamically resolve public host URL, utilizing fallback", "error", err)
	}

	ler, lerErr := parseExploreReferences(exploreReferences, hostURL)
	if lerErr != nil {
		return nil, lerErr
	}
	oauth_creds := OAuthCredentials{}
	if source.UseClientAuthorization() {
		rawToken, err := accessToken.ParseBearerToken()
		if err != nil {
			return nil, err.(util.ToolboxError)
		}
		oauth_creds.Token = TokenBased{AccessToken: rawToken}
	} else {
		oauth_creds.Secret = SecretBased{ClientId: source.LookerApiSettings().ClientId, ClientSecret: source.LookerApiSettings().ClientSecret}
	}

	lers := LookerExploreReferences{
		ExploreReferences: ler,
	}

	// Construct URL, headers, and payload
	projectID := source.GoogleCloudProject()
	location := source.GoogleCloudLocation()
	caURL := fmt.Sprintf("%s/v1/projects/%s/locations/%s:chat", util.GetGDAEndpoint(), url.PathEscape(projectID), url.PathEscape(location))

	headers := map[string]string{
		"Content-Type":      "application/json",
		"X-Goog-API-Client": util.GDAClientID,
	}
	if quotaProject := source.GoogleCloudQuotaProject(); quotaProject != "" {
		headers["X-Goog-User-Project"] = quotaProject
	}

	payload := CAPayload{
		Messages: []Message{{UserMessage: UserMessage{Text: userQuery}}},
		InlineContext: InlineContext{
			SystemInstruction: instructions,
			DatasourceReferences: DatasourceReferences{
				Looker: lers,
			},
		},
		Credentials: &Credentials{
			OAuth: oauth_creds,
		},
		ClientIdEnum: util.GDAClientID,
	}

	client, err := util.NewGDAClient(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, util.NewClientServerError("failed to create GDA client", http.StatusInternalServerError, err)
	}
	client.Timeout = 330 * time.Second

	// Call the streaming API
	response, err := getStream(ctx, client, caURL, payload, headers)
	if err != nil {
		return nil, util.NewClientServerError("failed to get response from conversational analytics API", http.StatusInternalServerError, err)
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

// StreamMessage represents a single message object from the streaming API response.
type StreamMessage struct {
	SystemMessage *SystemMessage `json:"systemMessage,omitempty"`
}

// SystemMessage contains different types of system-generated content.
type SystemMessage struct {
	Text     *TextMessage     `json:"text,omitempty"`
	Schema   *SchemaMessage   `json:"schema,omitempty"`
	Data     *DataMessage     `json:"data,omitempty"`
	Analysis *AnalysisMessage `json:"analysis,omitempty"`
	Error    *ErrorMessage    `json:"error,omitempty"`
}

// TextMessage contains textual parts of a message.
type TextMessage struct {
	Parts []string `json:"parts"`
}

// SchemaMessage contains schema-related information.
type SchemaMessage struct {
	Query  *SchemaQuery  `json:"query,omitempty"`
	Result *SchemaResult `json:"result,omitempty"`
}

// SchemaQuery holds the question that prompted a schema lookup.
type SchemaQuery struct {
	Question string `json:"question"`
}

// SchemaResult contains the datasources with their schemas.
type SchemaResult struct {
	Datasources []Datasource `json:"datasources"`
}

// Datasource represents a data source with its reference and schema.
type Datasource struct {
	LookerExploreReference LookerExploreReference `json:"lookerExploreReference"`
}

// DataMessage contains data-related information, like queries and results.
type DataMessage struct {
	GeneratedLookerQuery *LookerQuery `json:"generatedLookerQuery,omitempty"`
	Result               *DataResult  `json:"result,omitempty"`
}

type LookerQuery struct {
	Model   string   `json:"model"`
	Explore string   `json:"explore"`
	Fields  []string `json:"fields"`
	Filters []Filter `json:"filters,omitempty"`
	Sorts   []string `json:"sorts,omitempty"`
	Limit   string   `json:"limit,omitempty"`
}

type Filter struct {
	Field string `json:"field,omitempty"`
	Value string `json:"value,omitempty"`
}

// DataResult contains the schema and rows of a query result.
type DataResult struct {
	Data []map[string]any `json:"data"`
}

type AnalysisQuery struct {
	Question        string   `json:"question,omitempty"`
	DataResultNames []string `json:"dataResultNames,omitempty"`
}
type AnalysisEvent struct {
	PlannerReasoning      string `json:"plannerReasoning,omitempty"`
	CoderInstructions     string `json:"coderInstructions,omitempty"`
	Code                  string `json:"code,omitempty"`
	ExecutionOutput       string `json:"executionOutput,omitempty"`
	ExecutionError        string `json:"executionError,omitempty"`
	ResultVegaChartJson   string `json:"resultVegaChartJson,omitempty"`
	ResultNaturalLanguage string `json:"resultNaturalLanguage,omitempty"`
	ResultCsvData         string `json:"resultCsvData,omitempty"`
	ResultReferenceData   string `json:"resultReferenceData,omitempty"`
	Error                 string `json:"error,omitempty"`
}
type AnalysisMessage struct {
	Query         AnalysisQuery `json:"query,omitempty"`
	ProgressEvent AnalysisEvent `json:"progressEvent,omitempty"`
}

// ErrorResponse represents an error message from the API.
type ErrorMessage struct {
	Text string `json:"text"`
}

func getStream(ctx context.Context, client *http.Client, url string, payload CAPayload, headers map[string]string) ([]map[string]any, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned non-200 status: %d %s", resp.StatusCode, string(body))
	}

	var messages []map[string]any
	decoder := json.NewDecoder(resp.Body)

	// The response is a JSON array, so we read the opening bracket.
	if _, err := decoder.Token(); err != nil {
		if err == io.EOF {
			return nil, nil // Empty response is valid
		}
		return nil, fmt.Errorf("error reading start of json array: %w", err)
	}

	for decoder.More() {
		var msg StreamMessage
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("error decoding stream message: %w", err)
		}

		var newMessage map[string]any
		if msg.SystemMessage != nil {
			if msg.SystemMessage.Text != nil {
				newMessage = handleTextResponse(ctx, msg.SystemMessage.Text)
			} else if msg.SystemMessage.Schema != nil {
				newMessage = handleSchemaResponse(ctx, msg.SystemMessage.Schema)
			} else if msg.SystemMessage.Data != nil {
				newMessage = handleDataResponse(ctx, msg.SystemMessage.Data)
			} else if msg.SystemMessage.Analysis != nil {
				newMessage = handleAnalysisResponse(ctx, msg.SystemMessage.Analysis)
			} else if msg.SystemMessage.Error != nil {
				newMessage = handleError(ctx, msg.SystemMessage.Error)
			}
			messages = appendMessage(messages, newMessage)
		}
	}

	return messages, nil
}

func formatDatasourceAsDict(ctx context.Context, datasource *Datasource) map[string]any {
	logger, _ := util.LoggerFromContext(ctx)
	logger.DebugContext(ctx, "Datasource %s", *datasource)
	ds := make(map[string]any)
	ds["model"] = datasource.LookerExploreReference.LookmlModel
	ds["explore"] = datasource.LookerExploreReference.Explore
	ds["lookerInstanceUri"] = datasource.LookerExploreReference.LookerInstanceUri
	return map[string]any{"Datasource": ds}
}

func handleAnalysisResponse(ctx context.Context, resp *AnalysisMessage) map[string]any {
	logger, _ := util.LoggerFromContext(ctx)
	jsonData, err := json.Marshal(*resp)
	if err != nil {
		logger.ErrorContext(ctx, "error marshaling struct: %w", err)
		return map[string]any{"Analysis": "error"}
	}
	return map[string]any{"Analysis": jsonData}
}

func handleTextResponse(ctx context.Context, resp *TextMessage) map[string]any {
	logger, _ := util.LoggerFromContext(ctx)
	logger.DebugContext(ctx, "Text Response: %s", strings.Join(resp.Parts, ""))
	return map[string]any{"Answer": strings.Join(resp.Parts, "")}
}

func handleSchemaResponse(ctx context.Context, resp *SchemaMessage) map[string]any {
	if resp.Query != nil {
		return map[string]any{"Question": resp.Query.Question}
	}
	if resp.Result != nil {
		var formattedSources []map[string]any
		for _, ds := range resp.Result.Datasources {
			formattedSources = append(formattedSources, formatDatasourceAsDict(ctx, &ds))
		}
		return map[string]any{"Schema Resolved": formattedSources}
	}
	return nil
}

func handleDataResponse(ctx context.Context, resp *DataMessage) map[string]any {
	if resp.GeneratedLookerQuery != nil {
		logger, _ := util.LoggerFromContext(ctx)
		jsonData, err := json.Marshal(resp.GeneratedLookerQuery)
		if err != nil {
			logger.ErrorContext(ctx, "error marshaling struct: %w", err)
			return map[string]any{"Retrieval Query": "error"}
		}
		return map[string]any{
			"Retrieval Query": jsonData,
		}
	}
	if resp.Result != nil {

		return map[string]any{
			"Data Retrieved": resp.Result.Data,
		}
	}
	return nil
}

func handleError(ctx context.Context, resp *ErrorMessage) map[string]any {
	logger, _ := util.LoggerFromContext(ctx)
	logger.DebugContext(ctx, "Error Response: %s", resp.Text)
	return map[string]any{
		"Error": map[string]any{
			"Message": resp.Text,
		},
	}
}

func appendMessage(messages []map[string]any, newMessage map[string]any) []map[string]any {
	if newMessage == nil {
		return messages
	}
	if len(messages) > 0 {
		if _, ok := messages[len(messages)-1]["Data Retrieved"]; ok {
			messages = messages[:len(messages)-1]
		}
	}
	return append(messages, newMessage)
}

func (t Tool) GetAuthTokenHeaderName(source sources.Source) (string, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return "", fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.GetAuthTokenHeaderName(), nil
}
