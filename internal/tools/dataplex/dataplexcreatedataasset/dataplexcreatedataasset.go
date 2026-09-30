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

package dataplexcreatedataasset

import (
	"context"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "dataplex-create-data-asset"

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
	CreateDataAsset(
		ctx context.Context,
		locationID string,
		dataProductID string,
		dataAssetID string,
		resourceURI string,
		labels map[string]string,
		accessGroupConfigs map[string][]string,
	) (map[string]string, error)
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

func (cfg Config) Initialize(ctx context.Context) (tools.Tool, error) {
	locationID := parameters.NewStringParameter("locationId", "The location ID (e.g. 'us', 'us-central1') where the parent Data Product is located.")
	dataProductID := parameters.NewStringParameter("dataProductId", "The unique ID of the parent Data Product.")
	dataAssetID := parameters.NewStringParameter("dataAssetId", "The unique ID of the Data Asset to create.")
	resourceURI := parameters.NewStringParameter("resourceUri", "The URI of the physical resource associated with the Data Asset (e.g. '//bigquery.googleapis.com/projects/my-project/datasets/my-dataset/tables/my-table').")
	labels := parameters.NewMapParameter("labels", "Optional. The labels associated with the Data Asset.", "string", parameters.WithMapRequired(false))
	accessGroupConfigs := parameters.NewMapParameter("accessGroupConfigs", "Optional. Map of access group configurations to associate with the Data Asset. Each key represents the access group ID, and the value is a list of string IAM role names (e.g. {'test-group': ['roles/bigquery.dataViewer']}). To find the list of supported roles that can be granted on the resource, refer to the IAM Roles documentation or use the roles:queryGrantableRoles API method (https://cloud.google.com/iam/docs/reference/rest/v1/roles/queryGrantableRoles).", "", parameters.WithMapRequired(false))

	params := parameters.Parameters{locationID, dataProductID, dataAssetID, resourceURI, labels, accessGroupConfigs}

	t := Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
			tools.Manifest{
				Description:  cfg.Description,
				Parameters:   params.Manifest(),
				AuthRequired: cfg.AuthRequired,
			},
			params,
		),
	}
	return t, nil
}

type Tool struct {
	tools.BaseTool[Config]
}

var _ tools.Tool = Tool{}

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
	locationID, ok := paramsMap["locationId"].(string)
	if !ok || locationID == "" {
		return nil, util.NewAgentError("locationId is required and must be a string", nil)
	}

	dataProductID, ok := paramsMap["dataProductId"].(string)
	if !ok || dataProductID == "" {
		return nil, util.NewAgentError("dataProductId is required and must be a string", nil)
	}

	dataAssetID, ok := paramsMap["dataAssetId"].(string)
	if !ok || dataAssetID == "" {
		return nil, util.NewAgentError("dataAssetId is required and must be a string", nil)
	}

	resourceURI, ok := paramsMap["resourceUri"].(string)
	if !ok || resourceURI == "" {
		return nil, util.NewAgentError("resourceUri is required and must be a string", nil)
	}

	var labels map[string]string
	if rawLabels, ok := paramsMap["labels"].(map[string]any); ok {
		labels = make(map[string]string, len(rawLabels))
		for k, v := range rawLabels {
			labels[k], _ = v.(string)
		}
	}

	var accessGroupConfigs map[string][]string
	if rawConfigs, ok := paramsMap["accessGroupConfigs"].(map[string]any); ok {
		accessGroupConfigs = make(map[string][]string, len(rawConfigs))
		for k, v := range rawConfigs {
			if rawRoles, ok := v.([]any); ok {
				var roles []string
				for _, r := range rawRoles {
					if sRole, _ := r.(string); sRole != "" {
						roles = append(roles, sRole)
					}
				}
				accessGroupConfigs[k] = roles
			}
		}
	}

	resp, err := source.CreateDataAsset(ctx, locationID, dataProductID, dataAssetID, resourceURI, labels, accessGroupConfigs)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	return resp, nil
}
