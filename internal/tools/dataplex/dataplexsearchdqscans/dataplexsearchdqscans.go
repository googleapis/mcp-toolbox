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

package dataplexsearchdqscans

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"cloud.google.com/go/dataplex/apiv1/dataplexpb"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "dataplex-search-dq-scans"

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
	SearchDataQualityScans(context.Context, string, int, string) ([]*dataplexpb.DataScan, error)
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

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	filter := parameters.NewStringParameter("filter", "Optional. Filter string to search/filter data quality scans. E.g. \"display_name = \\\"my-scan\\\"\"", parameters.WithStringDefault(""))
	dataScanID := parameters.NewStringParameter("dataScanId", "Optional. The resource name of the data scan to filter by: projects/{project}/locations/{locationId}/dataScans/{dataScanId}.", parameters.WithStringDefault(""))
	resourcePath := parameters.NewStringParameter("resourcePath", "Optional. The resource path of the table or storage bucket to filter by. Maps to data.entity in the filter string. E.g. \"//bigquery.googleapis.com/projects/P/datasets/D/tables/T\"", parameters.WithStringDefault(""))
	pageSize := parameters.NewIntParameter("pageSize", "Number of returned data quality scans in the page.", parameters.WithIntDefault(10))
	orderBy := parameters.NewStringParameter("orderBy", "Specifies the ordering of results.", parameters.WithStringDefault(""))
	allParameters := parameters.Parameters{filter, dataScanID, resourcePath, pageSize, orderBy}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
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
	paramsMap := params.AsMap()
	filter, _ := paramsMap["filter"].(string)
	dataScanID, _ := paramsMap["dataScanId"].(string)
	resourcePath, _ := paramsMap["resourcePath"].(string)
	pageSize, _ := paramsMap["pageSize"].(int)
	orderBy, _ := paramsMap["orderBy"].(string)

	var filters []string
	if filter != "" {
		filters = append(filters, filter)
	}
	if dataScanID != "" {
		filters = append(filters, fmt.Sprintf("name = %q", dataScanID))
	}
	if resourcePath != "" {
		filters = append(filters, fmt.Sprintf("data.resource = %q", resourcePath))
	}

	finalFilter := strings.Join(filters, " AND ")

	res, err := source.SearchDataQualityScans(ctx, finalFilter, pageSize, orderBy)
	if err != nil {
		return nil, util.NewClientServerError("failed to search for dq scans", http.StatusInternalServerError, err)
	}
	return res, nil
}
