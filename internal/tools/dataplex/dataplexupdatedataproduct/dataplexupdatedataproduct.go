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

package dataplexupdatedataproduct

import (
	"context"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/dataplex"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "dataplex-update-data-product"

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
	UpdateDataProduct(
		ctx context.Context,
		locationId string,
		dataProductId string,
		description string,
		displayName string,
		ownerEmails []string,
		accessGroups []dataplex.AccessGroup,
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
	locationId := parameters.NewStringParameter("locationId", "The location to update the data product in.")
	dataProductId := parameters.NewStringParameter("dataProductId", "The data product ID.")
	description := parameters.NewStringParameter(
		"description",
		"Optional. Description of the data product.",
		parameters.WithStringRequired(false),
	)
	displayName := parameters.NewStringParameter(
		"displayName",
		"Optional. Display name of the data product.",
		parameters.WithStringRequired(false),
	)
	ownerEmails := parameters.NewArrayParameter(
		"ownerEmails",
		"Optional. The email addresses of the owners of the data product.",
		parameters.NewStringParameter("email", "Owner email address"),
		parameters.WithArrayRequired(false),
	)
	accessGroups := parameters.NewArrayParameter(
		"accessGroups",
		"Optional. List of access groups to associate with the Data Product.",
		parameters.NewMapParameter("accessGroup", "Access Group details (id, displayName, description, googleGroup, serviceAccount)", ""),
		parameters.WithArrayRequired(false),
	)
	updateMask := parameters.NewArrayParameter(
		"updateMask",
		"Optional. The fields to update. If not specified, all fields provided will be updated.",
		parameters.NewStringParameter("field", "Field path to update"),
		parameters.WithArrayRequired(false),
	)

	params := parameters.Parameters{locationId, dataProductId, description, displayName, ownerEmails, accessGroups, updateMask}

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
		return nil, util.NewAgentError("locationId is required and must be a non-empty string", nil)
	}

	dataProductId, ok := paramsMap["dataProductId"].(string)
	if !ok || dataProductId == "" {
		return nil, util.NewAgentError("dataProductId is required and must be a non-empty string", nil)
	}

	description, _ := paramsMap["description"].(string)
	displayName, _ := paramsMap["displayName"].(string)

	var ownerEmails []string
	if rawOwners, ok := paramsMap["ownerEmails"].([]any); ok {
		for _, o := range rawOwners {
			if email, _ := o.(string); email != "" {
				ownerEmails = append(ownerEmails, email)
			}
		}
	}

	var accessGroups []dataplex.AccessGroup
	if rawGroups, ok := paramsMap["accessGroups"].([]any); ok {
		for _, rawG := range rawGroups {
			gMap, ok := rawG.(map[string]any)
			if !ok {
				return nil, util.NewAgentError("each access group in accessGroups must be an object", nil)
			}
			id, ok := gMap["id"].(string)
			if !ok || id == "" {
				return nil, util.NewAgentError("access group 'id' is required and must be a non-empty string", nil)
			}
			dispName, ok := gMap["displayName"].(string)
			if !ok || dispName == "" {
				return nil, util.NewAgentError("access group 'displayName' is required and must be a non-empty string", nil)
			}

			desc, _ := gMap["description"].(string)
			googleGroup, _ := gMap["googleGroup"].(string)
			serviceAccount, _ := gMap["serviceAccount"].(string)

			if googleGroup == "" && serviceAccount == "" {
				return nil, util.NewAgentError("at least one of access group 'googleGroup' or 'serviceAccount' must be a non-empty string", nil)
			}

			accessGroups = append(accessGroups, dataplex.AccessGroup{
				ID:             id,
				DisplayName:    dispName,
				Description:    desc,
				GoogleGroup:    googleGroup,
				ServiceAccount: serviceAccount,
			})
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

	resp, err := source.UpdateDataProduct(ctx, locationId, dataProductId, description, displayName, ownerEmails, accessGroups, updateMask)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	return resp, nil
}
