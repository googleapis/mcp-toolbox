// Copyright 2026 Google LLC
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

package dataproclistjobs

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

const kind = "dataproc-list-jobs"

func init() {
	if !tools.Register(kind, newConfig) {
		panic(fmt.Sprintf("tool kind %q already registered", kind))
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
}

// validate interface
var _ tools.ToolConfig = Config{}

// ToolConfigType returns the unique name for this tool.
func (cfg Config) ToolConfigType() string {
	return kind
}

// Initialize creates a new Tool instance.
func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	desc := cfg.Description
	if desc == "" {
		desc = "Lists and filters Dataproc jobs"
	}

	allParameters := parameters.Parameters{
		parameters.NewStringParameter("filter", `A filter constraining the jobs to list. Filters are case-sensitive and have the following syntax: field = value [AND [field = value]] ... where field is clusterName, status.state, or labels.[KEY], and [KEY] is a label key. value can be * to match all values. status.state can be one of the following: PENDING, RUNNING, CANCEL_PENDING, JOB_STATE_CANCELLED, DONE, ERROR, or ATTEMPT_FAILURE. Only the logical AND operator is supported; space-separated items are treated as having an implicit AND operator. Filtering by clusterName is recommended to improve query performance.`, parameters.WithStringRequired(false)),
		parameters.NewStringParameter("jobStateMatcher", "Specifies if the job state matcher should match ALL jobs, only ACTIVE jobs, or only NON_ACTIVE jobs. Defaults to ALL. Supported values: ALL, ACTIVE, NON_ACTIVE.", parameters.WithStringRequired(false)),
		parameters.NewIntParameter("pageSize", "The maximum number of jobs to return in a single page (default 20)", parameters.WithIntDefault(20)),
		parameters.NewStringParameter("pageToken", "A page token, received from a previous `ListJobs` call", parameters.WithStringRequired(false)),
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

type compatibleSource interface {
	ListJobs(context.Context, *int, string, string, string) (any, error)
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
		pageSizeV := ps.(int)
		if pageSizeV <= 0 {
			return nil, util.NewAgentError(fmt.Sprintf("pageSize must be positive: %d", pageSizeV), nil)
		}
		pageSize = &pageSizeV
	}
	pt, _ := paramMap["pageToken"].(string)
	filter, _ := paramMap["filter"].(string)
	matcher, _ := paramMap["jobStateMatcher"].(string)

	res, err := source.ListJobs(ctx, pageSize, pt, filter, matcher)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return res, nil
}
