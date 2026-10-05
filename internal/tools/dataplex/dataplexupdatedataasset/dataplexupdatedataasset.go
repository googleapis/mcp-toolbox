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

package dataplexupdatedataasset

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

const resourceType string = "dataplex-update-data-asset"

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
	UpdateDataAsset(
		ctx context.Context,
		locationId string,
		dataProductId string,
		dataAssetId string,
		labels map[string]string,
		accessGroupConfigs map[string][]string,
		updateMask []string,
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
	locationId := parameters.NewStringParameter("locationId", "The location ID (e.g. 'us', 'us-central1') where the parent Data Product is located.")
	dataProductId := parameters.NewStringParameter("dataProductId", "The unique ID of the parent Data Product.")
	dataAssetId := parameters.NewStringParameter("dataAssetId", "The unique ID of the Data Asset to update.")
	labels := parameters.NewMapParameter("labels", "Optional. The labels associated with the Data Asset.", "string", parameters.WithMapRequired(false))
	accessGroupConfigs := parameters.NewMapParameter("accessGroupConfigs", "Optional. Map of access group configurations to associate with the Data Asset. Each key represents the access group ID, and the value is a list of string IAM role names (e.g. {'test-group': ['roles/bigquery.dataViewer']}). To find the list of supported roles that can be granted on the resource, refer to the IAM Roles documentation or use the roles:queryGrantableRoles API method (https://cloud.google.com/iam/docs/reference/rest/v1/roles/queryGrantableRoles).", "", parameters.WithMapRequired(false))
	updateMask := parameters.NewArrayParameter("updateMask", "Optional. The list of fields to update. If not specified, all non-empty fields will be updated.", parameters.NewStringParameter("", ""), parameters.WithArrayRequired(false))

	params := parameters.Parameters{locationId, dataProductId, dataAssetId, labels, accessGroupConfigs, updateMask}

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
	locationId, ok := paramsMap["locationId"].(string)
	if !ok || locationId == "" {
		return nil, util.NewAgentError("locationId is required and must be a string", nil)
	}

	dataProductId, ok := paramsMap["dataProductId"].(string)
	if !ok || dataProductId == "" {
		return nil, util.NewAgentError("dataProductId is required and must be a string", nil)
	}

	dataAssetId, ok := paramsMap["dataAssetId"].(string)
	if !ok || dataAssetId == "" {
		return nil, util.NewAgentError("dataAssetId is required and must be a string", nil)
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

	var updateMask []string
	if rawMask, ok := paramsMap["updateMask"].([]any); ok {
		for _, v := range rawMask {
			if s, _ := v.(string); s != "" {
				updateMask = append(updateMask, s)
			}
		}
	}

	resp, err := source.UpdateDataAsset(ctx, locationId, dataProductId, dataAssetId, labels, accessGroupConfigs, updateMask)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	return resp, nil
}
