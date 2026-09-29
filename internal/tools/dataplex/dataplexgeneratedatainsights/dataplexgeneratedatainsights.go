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

package dataplexgeneratedatainsights

import (
	"context"
	"fmt"
	"net/http"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/dataplex/dataplexcommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "dataplex-generate-data-insights"

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
	GenerateDataInsights(ctx context.Context, location, resourcePath string, publish bool) (string, error)
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
	resourcePath := parameters.NewStringParameter("resourcePath", "The fully-qualified BigQuery resource path of the dataset or table to analyze. Format: //bigquery.googleapis.com/projects/{project}/datasets/{dataset} or //bigquery.googleapis.com/projects/{project}/datasets/{dataset}/tables/{table}.")
	location := parameters.NewStringParameter("location", "The Google Cloud region where the Dataplex scan should be created and executed (e.g., 'us-central1'). This should match the location of the BigQuery resource or the published location of the asset in the catalog.")
	publish := parameters.NewBooleanParameter("publish", "Optional. If true, automatically publishes the results to the Knowledge Catalog upon successful completion. False by default.", parameters.WithBooleanDefault(false))

	allParameters := parameters.Parameters{resourcePath, location, publish}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
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
	resourcePath, _ := paramsMap["resourcePath"].(string)
	location, _ := paramsMap["location"].(string)
	publish, _ := paramsMap["publish"].(bool)

	if resourcePath == "" {
		return nil, util.NewAgentError("resourcePath parameter is required", nil)
	}
	if location == "" {
		return nil, util.NewAgentError("location parameter is required", nil)
	}

	resourcePath = dataplexcommon.NormalizeResourcePath(resourcePath, source.ProjectID())

	opName, err := source.GenerateDataInsights(ctx, location, resourcePath, publish)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	return map[string]string{
		"operation_id": opName,
	}, nil
}
