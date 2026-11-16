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
package cloudloggingadminquerylogs

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	cla "github.com/googleapis/mcp-toolbox/internal/sources/cloudloggingadmin"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const (
	resourceType string = "cloud-logging-admin-query-logs"

	defaultLimit               int = 200
	defaultStartTimeOffsetDays int = 30
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
	UseClientAuthorization() bool
	QueryLogs(ctx context.Context, params cla.QueryLogsParams, accessToken string) ([]map[string]any, error)
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

	startTimeDescription := fmt.Sprintf("Start time in RFC3339 format (e.g., 2025-12-09T00:00:00Z). Defaults to %d days ago.", defaultStartTimeOffsetDays)
	limitDescription := fmt.Sprintf("Maximum number of log entries to return. Default: %d.", defaultLimit)
	params := parameters.Parameters{
		parameters.NewStringParameter(
			"filter",
			"Cloud Logging filter query. Common fields: resource.type, resource.labels.*, logName, severity, textPayload, jsonPayload.*, protoPayload.*, labels.*, httpRequest.*. Operators: =, !=, <, <=, >, >=, :, =~, AND, OR, NOT.", parameters.WithStringRequired(false)),
		parameters.NewBooleanParameter("newestFirst", "Set to true for newest logs first. Defaults to oldest first.", parameters.WithBooleanRequired(false)),
		parameters.NewStringParameter("startTime", startTimeDescription, parameters.WithStringRequired(false)),
		parameters.NewStringParameter("endTime", "End time in RFC3339 format (e.g., 2025-12-09T23:59:59Z). Defaults to now.", parameters.WithStringRequired(false)),
		parameters.NewBooleanParameter("verbose", "Include additional fields (insertId, trace, spanId, httpRequest, labels, operation, sourceLocation). Defaults to false.", parameters.WithBooleanRequired(false)),
		parameters.NewIntParameter("limit", limitDescription, parameters.WithIntRequired(false)),
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
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

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	// Parse parameters
	limit := defaultLimit
	paramsMap := params.AsMap()
	newestFirst, _ := paramsMap["newestFirst"].(bool)

	// Check and set limit
	if val, ok := paramsMap["limit"].(int); ok && val > 0 {
		limit = val
	} else if ok && val < 0 {
		return nil, util.NewAgentError("limit must be greater than or equal to 1", nil)
	}

	// Check for verbosity of output
	verbose, _ := paramsMap["verbose"].(bool)

	// Build filter
	var filter string
	if f, ok := paramsMap["filter"].(string); ok {
		if len(f) == 0 {
			return nil, util.NewAgentError("filter cannot be empty if provided", nil)
		}
		filter = f
	}

	// Parse start time
	var startTime string
	if val, ok := paramsMap["startTime"].(string); ok && val != "" {
		if _, err := time.Parse(time.RFC3339, val); err != nil {
			return nil, util.NewAgentError(fmt.Sprintf("startTime must be in RFC3339 format (e.g., 2025-12-09T00:00:00Z): %v", err), err)
		}
		startTime = val
	} else {
		startTime = time.Now().AddDate(0, 0, -defaultStartTimeOffsetDays).Format(time.RFC3339)
	}

	// Parse end time
	var endTime string
	if val, ok := paramsMap["endTime"].(string); ok && val != "" {
		if _, err := time.Parse(time.RFC3339, val); err != nil {
			return nil, util.NewAgentError(fmt.Sprintf("endTime must be in RFC3339 format (e.g., 2025-12-09T23:59:59Z): %v", err), err)
		}
		endTime = val
	}

	tokenString := ""
	var err error
	if source.UseClientAuthorization() {
		tokenString, err = accessToken.ParseBearerToken()
		if err != nil {
			return nil, util.NewClientServerError("failed to parse access token", http.StatusUnauthorized, err)
		}
	}

	queryParams := cla.QueryLogsParams{
		Filter:      filter,
		NewestFirst: newestFirst,
		StartTime:   startTime,
		EndTime:     endTime,
		Verbose:     verbose,
		Limit:       limit,
	}

	resp, err := source.QueryLogs(ctx, queryParams, tokenString)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
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

func (t Tool) EmbedParams(ctx context.Context, paramValues parameters.ParamValues, pMgr tools.PrimitiveManagerI) (parameters.ParamValues, error) {
	return paramValues, nil
}
