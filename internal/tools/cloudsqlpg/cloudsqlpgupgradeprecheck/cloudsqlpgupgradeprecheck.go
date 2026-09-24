// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloudsqlpgupgradeprecheck

import (
	"context"
	"fmt"
	"net/http"
	"time"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	sqladmin "google.golang.org/api/sqladmin/v1"
)

const resourceType string = "postgres-upgrade-precheck"

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
	GetService(context.Context, string) (*sqladmin.Service, error)
	UseClientAuthorization() bool
}

// Config defines the configuration for the precheck-upgrade tool.
type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

// validate interface
var _ tools.ToolConfig = Config{}

// ToolConfigType returns the type of the tool.
func (cfg Config) ToolConfigType() string {
	return resourceType
}

// Initialize initializes the tool from the configuration.
func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	allParameters := parameters.Parameters{
		parameters.NewStringParameter("project", "The project ID"),
		parameters.NewStringParameter("instance", "The name of the instance to check"),
		parameters.NewStringParameter("targetDatabaseVersion", "The target PostgreSQL version for the upgrade (e.g., POSTGRES_18). If not specified, defaults to the PostgreSQL 18.", parameters.WithStringDefault("POSTGRES_18")),
	}

	if cfg.Description == "" {
		cfg.Description = "Analyzes a Cloud SQL PostgreSQL instance for major version upgrade readiness. Results are provided to guide customer actions:\n" +
			"ERROR: Action Required. These are critical issues blocking the upgrade. Customers must resolve these using the provided actions_required steps before attempting the upgrade.\n" +
			"WARNING: Review Recommended. These are potential issues. Customers should review the message and actions_required. While not blocking, addressing these is advised to prevent future problems or unexpected behavior post-upgrade.\n" +
			"INFO: No Action Needed. Informational messages only. This pre-check helps customers proactively fix problems, preventing upgrade failures and ensuring a smoother transition."
	}
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
		),
	}, nil
}

// Tool represents the precheck-upgrade tool.
type Tool struct {
	tools.BaseTool[Config]
}

// PreCheckResultItem holds the details of a single check result.
type PreCheckResultItem struct {
	Message         string   `json:"message"`
	MessageType     string   `json:"messageType"` // INFO, WARNING, ERROR
	ActionsRequired []string `json:"actionsRequired"`
}

// PreCheckAPIResponse holds the array of pre-check results.
type PreCheckAPIResponse struct {
	Items []PreCheckResultItem `json:"preCheckResponse"`
}

// Helper function to convert from []*sqladmin.PreCheckResponse to []PreCheckResultItem
func convertResults(items []*sqladmin.PreCheckResponse) []PreCheckResultItem {
	if len(items) == 0 { // Handle nil or empty slice
		return []PreCheckResultItem{}
	}
	results := make([]PreCheckResultItem, len(items))
	for i, item := range items {
		results[i] = PreCheckResultItem{
			Message:         item.Message,
			MessageType:     item.MessageType,
			ActionsRequired: item.ActionsRequired,
		}
	}
	return results
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

// Invoke executes the tool's logic.
func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramsMap := params.AsMap()

	project, ok := paramsMap["project"].(string)
	if !ok || project == "" {
		return nil, util.NewAgentError("missing or empty 'project' parameter", nil)
	}
	instanceName, ok := paramsMap["instance"].(string)
	if !ok || instanceName == "" {
		return nil, util.NewAgentError("missing or empty 'instance' parameter", nil)
	}
	targetVersion, ok := paramsMap["targetDatabaseVersion"].(string)
	if !ok || targetVersion == "" {
		// This should not happen due to the default value
		return nil, util.NewAgentError("missing or empty 'targetDatabaseVersion' parameter", nil)
	}

	service, err := source.GetService(ctx, string(accessToken))
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	reqBody := &sqladmin.InstancesPreCheckMajorVersionUpgradeRequest{
		PreCheckMajorVersionUpgradeContext: &sqladmin.PreCheckMajorVersionUpgradeContext{
			TargetDatabaseVersion: targetVersion,
		},
	}

	call := service.Instances.PreCheckMajorVersionUpgrade(project, instanceName, reqBody).Context(ctx)
	op, err := call.Do()
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	const pollTimeout = 20 * time.Second
	cutoffTime := time.Now().Add(pollTimeout)

	for time.Now().Before(cutoffTime) {
		currentOp, err := service.Operations.Get(project, op.Name).Context(ctx).Do()
		if err != nil {
			return nil, util.ProcessGcpError(err)
		}

		if currentOp.Status == "DONE" {
			if currentOp.Error != nil && len(currentOp.Error.Errors) > 0 {
				errMsg := fmt.Sprintf("pre-check operation LRO failed: %s", currentOp.Error.Errors[0].Message)
				if currentOp.Error.Errors[0].Code != "" {
					errMsg = fmt.Sprintf("%s (Code: %s)", errMsg, currentOp.Error.Errors[0].Code)
				}
				return nil, util.NewClientServerError(errMsg, http.StatusInternalServerError, fmt.Errorf("pre-check operation failed with error: %s", errMsg))
			}

			var preCheckItems []*sqladmin.PreCheckResponse
			if currentOp.PreCheckMajorVersionUpgradeContext != nil {
				preCheckItems = currentOp.PreCheckMajorVersionUpgradeContext.PreCheckResponse
			}
			// convertResults handles nil or empty preCheckItems
			return PreCheckAPIResponse{Items: convertResults(preCheckItems)}, nil
		}

		select {
		case <-ctx.Done():
			return nil, util.NewClientServerError("timed out waiting for operation", http.StatusRequestTimeout, ctx.Err())
		case <-time.After(5 * time.Second):
		}
	}
	return op, nil
}

// Authorized checks if the tool is authorized.
func (t Tool) Authorized(verifiedAuthServices []string) bool {
	return true
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}
