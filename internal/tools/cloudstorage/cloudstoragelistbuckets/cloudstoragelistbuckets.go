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

package cloudstoragelistbuckets

import (
	"context"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragecommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "cloud-storage-list-buckets"

// maxResultsLimit matches the GCS per-page cap. Values above this are rejected
// in Invoke so callers see an explicit error instead of a silently-clamped page.
const maxResultsLimit = 1000

const (
	projectKey    = "project"
	prefixKey     = "prefix"
	maxResultsKey = "max_results"
	pageTokenKey  = "page_token"
)

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
	ListBuckets(ctx context.Context, project, prefix string, maxResults int, pageToken string) (map[string]any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string  `yaml:"type" validate:"required"`
	Source           string  `yaml:"source" validate:"required"`
	Project          *string `yaml:"project,omitempty"`
	Prefix           *string `yaml:"prefix,omitempty"`
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

	maxResultsParam := parameters.NewIntParameter(maxResultsKey, "Maximum number of buckets to return per page. A value of 0 uses the API default (1000); negative values and values above 1000 are rejected.", parameters.WithIntDefault(0))
	pageTokenParam := parameters.NewStringParameter(pageTokenKey, "A previously-returned page token for retrieving the next page of results.", parameters.WithStringDefault(""))
	allParameters := parameters.Parameters{}
	if cfg.Project == nil {
		allParameters = append(allParameters, parameters.NewStringParameter(projectKey, "Project ID to list buckets in. When empty, the source's configured project is used.", parameters.WithStringDefault("")))
	}
	if cfg.Prefix == nil {
		allParameters = append(allParameters, parameters.NewStringParameter(prefixKey, "Filter results to buckets whose names begin with this prefix.", parameters.WithStringDefault("")))
	}
	allParameters = append(allParameters, maxResultsParam, pageTokenParam)

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
	mapParams := params.AsMap()
	project := cloudstoragecommon.ResolveString(t.Cfg.Project, mapParams, projectKey)
	prefix := cloudstoragecommon.ResolveString(t.Cfg.Prefix, mapParams, prefixKey)
	pageToken, _ := mapParams[pageTokenKey].(string)
	maxResults, _ := mapParams[maxResultsKey].(int)
	if maxResults < 0 {
		return nil, util.NewAgentError(fmt.Sprintf("invalid '%s' parameter: %d must be >= 0 (use 0 for the API default)", maxResultsKey, maxResults), nil)
	}
	if maxResults > maxResultsLimit {
		return nil, util.NewAgentError(fmt.Sprintf("invalid '%s' parameter: %d exceeds the maximum of %d", maxResultsKey, maxResults, maxResultsLimit), nil)
	}

	resp, err := source.ListBuckets(ctx, project, prefix, maxResults, pageToken)
	if err != nil {
		return nil, cloudstoragecommon.ProcessGCSError(err)
	}
	return resp, nil
}
