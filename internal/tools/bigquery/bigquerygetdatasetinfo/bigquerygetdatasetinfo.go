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

package bigquerygetdatasetinfo

import (
	"context"
	"fmt"
	"net/http"

	bigqueryapi "cloud.google.com/go/bigquery"
	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	bqutil "github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerycommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	bigqueryrestapi "google.golang.org/api/bigquery/v2"
)

const resourceType string = "bigquery-get-dataset-info"
const projectKey string = "project"
const datasetKey string = "dataset"

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
	BigQueryProject() string
	UseClientAuthorization() bool
	GetAuthTokenHeaderName() string
	IsDatasetAllowed(projectID, datasetID string) bool
	BigQueryAllowedDatasets() []string
	RetrieveClientAndService(context.Context, tools.AccessToken) (*bigqueryapi.Client, *bigqueryrestapi.Service, error)
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

	params := buildParams(nil, "")
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
	mapParams := params.AsMap()
	projectId, ok := mapParams[projectKey].(string)
	if !ok {
		// Updated: Use fmt.Sprintf for formatting, pass nil as cause
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a string", projectKey), nil)
	}

	datasetId, ok := mapParams[datasetKey].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a string", datasetKey), nil)
	}

	bqClient, _, err := source.RetrieveClientAndService(ctx, accessToken)
	if err != nil {
		return nil, util.NewClientServerError("failed to retrieve BigQuery client", http.StatusInternalServerError, err)
	}

	if !source.IsDatasetAllowed(projectId, datasetId) {
		return nil, util.NewAgentError(fmt.Sprintf("access denied to dataset '%s' because it is not in the configured list of allowed datasets for project '%s'", datasetId, projectId), nil)
	}

	dsHandle := bqClient.DatasetInProject(projectId, datasetId)

	metadata, err := dsHandle.Metadata(ctx)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	return metadata, nil
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}

func (t Tool) GetAuthTokenHeaderName(source sources.Source) (string, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return "", fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.GetAuthTokenHeaderName(), nil
}

// buildParams builds the tool's parameters from the source's allowed-dataset configuration.
// A nil allow-list and empty default project yield the plain skeleton.
func buildParams(allowedDatasets []string, defaultProject string) parameters.Parameters {
	projectDescription := "The Google Cloud project ID containing the dataset."
	datasetDescription := "The dataset to get metadata information. Can be in `project.dataset` format."
	projectParameter, datasetParameter := bqutil.InitializeDatasetParameters(allowedDatasets, defaultProject, projectKey, datasetKey, projectDescription, datasetDescription)
	return parameters.Parameters{projectParameter, datasetParameter}
}

// resolveParams builds the tool's parameters using the source's allowed-dataset configuration.
func (t Tool) resolveParams(source sources.Source) (parameters.Parameters, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return nil, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return buildParams(s.BigQueryAllowedDatasets(), s.BigQueryProject()), nil
}

// GetParameters returns the tool's parameters, resolved against the source.
func (t Tool) GetParameters(source sources.Source) (parameters.Parameters, error) {
	return t.resolveParams(source)
}

// Manifest returns the tool's manifest, resolved against the source.
func (t Tool) Manifest(source sources.Source) (tools.Manifest, error) {
	params, err := t.resolveParams(source)
	if err != nil {
		return tools.Manifest{}, err
	}
	return tools.Manifest{Description: t.Cfg.Description, Parameters: params.Manifest(), AuthRequired: t.Cfg.AuthRequired}, nil
}
