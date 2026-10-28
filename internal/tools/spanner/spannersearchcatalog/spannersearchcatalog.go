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

package spannersearchcatalog

import (
	"context"
	"fmt"
	"net/http"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/dataplex/searchcatalog"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "spanner-search-catalog"

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
	UseClientAuthorization() bool
	InvokeSearchCatalog(ctx context.Context, params map[string]any, tokenStr string) ([]searchcatalog.DataplexSearchResponse, error)
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
	prompt := parameters.NewStringParameter("prompt", "Prompt representing search intention. Do not rewrite the prompt.")

	databaseIds := parameters.NewArrayParameter(
		"databaseIds",
		"Array of database IDs.",
		parameters.NewStringParameter("databaseId", "The IDs of the spanner database."), parameters.WithArrayDefault(
			[]any{}))

	projectIds := parameters.NewArrayParameter(
		"projectIds",
		"Array of project IDs.",
		parameters.NewStringParameter("projectId", "The IDs of the GCP project."), parameters.WithArrayDefault(
			[]any{}))

	types := parameters.NewArrayParameter(
		"types",
		"Array of data types to filter by.",
		parameters.NewStringParameter("type", "The type of the data. Accepted values are: SERVICE, DATABASE, TABLE, VIEW."), parameters.WithArrayDefault(
			[]any{}))

	pageSize := parameters.NewIntParameter("pageSize", "Number of results in the search page.", parameters.WithIntDefault(5))

	params := parameters.Parameters{prompt, databaseIds, projectIds, types, pageSize}

	if cfg.Description == "" {
		cfg.Description = "Searches for data assets (eg. Spanner tables, views, or databases) in catalog based on the provided search query"
	}

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

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	var tokenStr string
	var err error
	if source.UseClientAuthorization() {
		tokenStr, err = accessToken.ParseBearerToken()
		if err != nil {
			return nil, util.NewClientServerError("failed to parse access token", http.StatusInternalServerError, err)
		}
	}

	results, err := source.InvokeSearchCatalog(ctx, params.AsMap(), tokenStr)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	return results, nil
}

func (t Tool) GetAuthTokenHeaderName(source sources.Source) (string, error) {
	return "", nil
}
