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

package conversationalanalyticsdeletedataagent

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/conversationalanalytics"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/api/option"
)

const toolType string = "conversational-analytics-delete-data-agent"

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

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
	Location         string `yaml:"location"`
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

	dataAgentIDParameter := parameters.NewStringParameter("data_agent_id", "The ID of the data agent to delete.")
	params := parameters.Parameters{dataAgentIDParameter}

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
	_, ok := source.(conversationalanalytics.CompatibleSource)
	if !ok {
		return fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return nil
}

func (t Tool) ToConfig() tools.ToolConfig {
	return t.Cfg
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(conversationalanalytics.CompatibleSource)
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

	tokenSource, tbErr := conversationalanalytics.GetTokenSource(ctx, source, accessToken)
	if tbErr != nil {
		return nil, tbErr
	}

	endpoint := t.endpoint
	if endpoint == "" {
		endpoint = util.GetGDAEndpoint()
	}

	client := t.httpClient
	if client == nil {
		var err error
		client, err = util.NewGDAClient(ctx, option.WithTokenSource(tokenSource))
		if err != nil {
			return nil, util.NewClientServerError("failed to create GDA client", http.StatusInternalServerError, err)
		}
		client.Timeout = 30 * time.Second
	}

	return deleteDataAgent(ctx, client, endpoint, projectID, t.Cfg.Location, dataAgentID, t.pollInterval, t.pollTimeout)
}

func deleteDataAgent(
	ctx context.Context,
	client *http.Client,
	endpoint, projectID, location, dataAgentID string,
	pollInterval, pollTimeout time.Duration,
) (any, util.ToolboxError) {
	caURL := fmt.Sprintf(
		"%s/v1/projects/%s/locations/%s/dataAgents/%s",
		endpoint,
		url.PathEscape(projectID),
		url.PathEscape(location),
		url.PathEscape(dataAgentID),
	)

	result, tbErr := conversationalanalytics.DoGDARequest(ctx, client, http.MethodDelete, caURL, nil)
	if tbErr != nil {
		return nil, tbErr
	}

	return util.AwaitGDAOperation(ctx, client, endpoint, result, "deletion", pollInterval, pollTimeout)
}

func (t Tool) RequiresClientAuthorization(s sources.Source) (bool, error) {
	source, ok := s.(conversationalanalytics.CompatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return source.UseClientAuthorization(), nil
}
