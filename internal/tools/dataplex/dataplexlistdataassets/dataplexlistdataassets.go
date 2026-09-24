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

package dataplexlistdataassets

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

const resourceType string = "dataplex-list-data-assets"

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
	ListDataAssets(ctx context.Context, locationId string, dataProductId string, filter string, pageSize int, orderBy string) ([]*dataplex.DataAsset, error)
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

func (cfg Config) Initialize(ctx context.Context) (tools.Tool, error) {
	locationId := parameters.NewStringParameter("locationId", "The location ID (e.g., 'us', 'us-central1') where the Data Product is located.")
	dataProductId := parameters.NewStringParameter("dataProductId", "The unique ID of the parent Data Product.")
	filter := parameters.NewStringParameter(
		"filter",
		"Optional. Filter string to list data assets. Based on the AIP-160 proposal. Use '=' for exact, and ':' for contains matching. String literals must be enclosed within \"\". Matching across all fields at once is not yet supported.",
		parameters.WithStringRequired(false),
	)
	pageSize := parameters.NewIntParameter(
		"pageSize",
		"Optional. Number of returned data assets in the page.",
		parameters.WithIntDefault(10),
	)
	orderBy := parameters.NewStringParameter(
		"orderBy",
		"Optional. Specifies the ordering of results.",
		parameters.WithStringRequired(false),
	)
	params := parameters.Parameters{locationId, dataProductId, filter, pageSize, orderBy}

	t := Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
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
	paramsMap := params.AsMap()
	locationId, ok := paramsMap["locationId"].(string)
	if !ok || locationId == "" {
		return nil, util.NewAgentError("locationId is required and must be a non-empty string", nil)
	}
	dataProductId, ok := paramsMap["dataProductId"].(string)
	if !ok || dataProductId == "" {
		return nil, util.NewAgentError("dataProductId is required and must be a non-empty string", nil)
	}

	filter, _ := paramsMap["filter"].(string)
	pageSize, _ := paramsMap["pageSize"].(int)
	orderBy, _ := paramsMap["orderBy"].(string)

	resp, err := source.ListDataAssets(ctx, locationId, dataProductId, filter, pageSize, orderBy)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}
