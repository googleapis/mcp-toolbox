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

package dataplexgetrunstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"cloud.google.com/go/dataplex/apiv1/dataplexpb"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/protobuf/encoding/protojson"
)

const resourceType string = "dataplex-get-run-status"

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
	ProjectID() string
	GetJobStatus(ctx context.Context, location, scanID, jobID string) (*dataplexpb.DataScanJob, error)
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
	scanID := parameters.NewStringParameter("scanId", "The unique ID of the Dataplex DataScan (e.g. 'nq-doc-12345...').")
	location := parameters.NewStringParameter("location", "The Google Cloud region where the Dataplex scan was created (e.g. 'us-central1').")
	jobID := parameters.NewStringParameter("jobId", "Optional. The specific unique ID of the background job run (DataScanJob) to track. If not provided, tracks the latest execution run.", parameters.WithStringRequired(false))

	allParameters := parameters.Parameters{scanID, location, jobID}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
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
	scanId, _ := paramsMap["scanId"].(string)
	location, _ := paramsMap["location"].(string)
	jobId, _ := paramsMap["jobId"].(string)

	if scanId == "" {
		return nil, util.NewAgentError("scanId parameter is required", nil)
	}
	if location == "" {
		return nil, util.NewAgentError("location parameter is required", nil)
	}

	job, err := source.GetJobStatus(ctx, location, scanId, jobId)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	if job == nil {
		return json.RawMessage(`{"status": "NO_JOBS"}`), nil
	}

	jsonBytes, err := protojson.Marshal(job)
	if err != nil {
		return nil, util.NewClientServerError("failed to marshal response to JSON", http.StatusInternalServerError, err)
	}

	return json.RawMessage(jsonBytes), nil
}
