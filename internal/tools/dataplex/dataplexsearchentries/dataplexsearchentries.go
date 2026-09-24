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

package dataplexsearchentries

import (
	"context"
	"fmt"
	"net/http"

	"cloud.google.com/go/dataplex/apiv1/dataplexpb"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "dataplex-search-entries"

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
	SearchEntries(context.Context, string, int, string, string) ([]*dataplexpb.SearchEntriesResult, error)
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
	query := parameters.NewStringParameter("query",
		"A query string for searching entries, following Dataplex search syntax. "+
			"Supports logical operators (AND, OR, NOT) and grouping. "+
			"For example, to find a table that might have been renamed, you could use 'type:table (name:books OR fiction)'. "+
			"This can be more efficient than multiple separate calls."+
			"Warning: Performing broad searches without specific filters (e.g., type:table) can be slow and consume significant resources. When performing exploratory searches, always use the pageSize parameter to limit the number of results returned.")
	scope := parameters.NewStringParameter("scope", "Optional. A scope limits the search space to a particular project or organization. It must be in the format: organizations/<org_id> or projects/<project_id> or projects/<project_number>.", parameters.WithStringDefault(""))
	pageSize := parameters.NewIntParameter("pageSize", "Optional. Number of results in the search page.", parameters.WithIntDefault(5))
	orderBy := parameters.NewStringParameter("orderBy", "Optional. Specifies the ordering of results. Supported values are: relevance, last_modified_timestamp, last_modified_timestamp asc", parameters.WithStringDefault("relevance"))
	allParameters := parameters.Parameters{query, scope, pageSize, orderBy}

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
	query, ok := paramsMap["query"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'query' parameter: %v", paramsMap["query"]), nil)
	}
	pageSize, ok := paramsMap["pageSize"].(int)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'pageSize' parameter: %v", paramsMap["pageSize"]), nil)
	}
	orderBy, ok := paramsMap["orderBy"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'orderBy' parameter: %v", paramsMap["orderBy"]), nil)
	}
	scope, ok := paramsMap["scope"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'scope' parameter: %v", paramsMap["scope"]), nil)
	}
	resp, err := source.SearchEntries(ctx, query, pageSize, orderBy, scope)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}
