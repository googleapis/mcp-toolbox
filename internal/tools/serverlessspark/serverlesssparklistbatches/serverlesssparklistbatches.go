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

package serverlesssparklistbatches

import (
	"context"
	"fmt"
	"net/http"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType = "serverless-spark-list-batches"

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
	ListBatches(context.Context, *int, string, string) (any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

// validate interface
var _ tools.ToolConfig = Config{}

// ToolConfigType returns the unique name for this tool.
func (cfg Config) ToolConfigType() string {
	return resourceType
}

// Initialize creates a new Tool instance.
func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	desc := cfg.Description
	if desc == "" {
		desc = "Lists available Serverless Spark (aka Dataproc Serverless) batches"
	}

	allParameters := parameters.Parameters{
		parameters.NewStringParameter("filter", `Filter expression to limit the batches. Filters are case sensitive, and may contain multiple clauses combined with logical operators (AND/OR, case sensitive). Supported fields are batch_id, batch_uuid, state, create_time, and labels. e.g. state = RUNNING AND create_time < "2023-01-01T00:00:00Z" filters for batches in state RUNNING that were created before 2023-01-01. state = RUNNING AND labels.environment=production filters for batches in state in a RUNNING state that have a production environment label. Valid states are STATE_UNSPECIFIED, PENDING, RUNNING, CANCELLING, CANCELLED, SUCCEEDED, FAILED. Valid operators are < > <= >= = !=, and : as "has" for labels, meaning any non-empty value)`, parameters.WithStringRequired(false)),
		parameters.NewIntParameter("pageSize", "The maximum number of batches to return in a single page (default 20)", parameters.WithIntDefault(20)),
		parameters.NewStringParameter("pageToken", "A page token, received from a previous `ListBatches` call", parameters.WithStringRequired(false)),
	}
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: desc, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
		),
	}, nil
}

// validate interface
var _ tools.Tool = Tool{}

// Tool is the implementation of the tool.
type Tool struct {
	tools.BaseTool[Config]
}

// Invoke executes the tool's operation.
func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramMap := params.AsMap()
	var pageSize *int
	if ps, ok := paramMap["pageSize"]; ok && ps != nil {
		pageSizeV, ok := ps.(int)
		if !ok {
			// Handle float64 case if unmarshaled from JSON usually
			if f, ok := ps.(float64); ok {
				pageSizeV = int(f)
			} else {
				return nil, util.NewAgentError("pageSize must be an integer", nil)
			}
		}

		if pageSizeV <= 0 {
			return nil, util.NewAgentError(fmt.Sprintf("pageSize must be positive: %d", pageSizeV), nil)
		}
		pageSize = &pageSizeV
	}

	pt, _ := paramMap["pageToken"].(string)
	filter, _ := paramMap["filter"].(string)

	resp, err := source.ListBatches(ctx, pageSize, pt, filter)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
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
