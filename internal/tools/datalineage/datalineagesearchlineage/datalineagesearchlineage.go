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

package datalineagesearchlineage

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	lineagepb "cloud.google.com/go/datacatalog/lineage/apiv1/lineagepb"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "datalineage-search-lineage"

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
	SearchLineageStreaming(
		ctx context.Context,
		parentLocation string,
		locations []string,
		rootEntities []*lineagepb.EntityReference,
		direction lineagepb.SearchLineageStreamingRequest_SearchDirection,
		maxDepth int32,
		maxResults int32,
		maxProcessPerLink int32,
		requestProcessDetails bool,
	) ([]*lineagepb.LineageLink, []string, error)
}

type SearchLineageResponse struct {
	Links       []*lineagepb.LineageLink `json:"links"`
	Unreachable []string                 `json:"unreachable,omitempty"`
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
	locations := parameters.NewArrayParameter(
		"locations",
		"Required. The locations to search in. Must contain at least 1 location. The first location will be used to initiate the search.",
		parameters.NewStringParameter("location", "A location to search in (e.g., 'us', 'eu', 'global')."),
	)

	// MapParameter for entity reference
	entityRef := parameters.NewMapParameter("entity", "Entity reference containing fully_qualified_name and optional fields.", "")

	rootEntities := parameters.NewArrayParameter(
		"root_entities",
		"Required. The starting entities for the search. Each object must have 'fully_qualified_name' (string) and optionally 'fields' (array of strings).",
		entityRef,
	)

	direction := parameters.NewStringParameter(
		"direction",
		"Required. Direction of the search.",
		parameters.WithStringAllowedValues([]any{"UPSTREAM", "DOWNSTREAM"}),
	)

	maxDepth := parameters.NewIntParameter(
		"max_depth",
		"Optional. The maximum depth of the search. Default is 5, max is 100.", parameters.WithIntRequired(
			false))

	maxResults := parameters.NewIntParameter(
		"max_results",
		"Optional. The maximum number of links to return in the response. Default is 1000, max is 10000.", parameters.WithIntRequired(
			false))

	maxProcessPerLink := parameters.NewIntParameter(
		"max_process_per_link",
		"Optional. The maximum number of processes to return per link. Default is 0, max is 100. Must be greater than 0 if request_process_details is true.", parameters.WithIntRequired(
			false))

	requestProcessDetails := parameters.NewBooleanParameter(
		"request_process_details",
		"Optional. If true, retrieves full process details (displayName, attributes, origin) for the links. Requires max_process_per_link to be greater than 0.", parameters.WithBooleanRequired(
			false))

	params := parameters.Parameters{locations, rootEntities, direction, maxDepth, maxResults, maxProcessPerLink, requestProcessDetails}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: params.Manifest(), AuthRequired: cfg.AuthRequired},
			params,
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

	// Parse locations
	locationsRaw, ok := paramsMap["locations"].([]any)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'locations' parameter: %v", paramsMap["locations"]), nil)
	}
	if len(locationsRaw) == 0 {
		return nil, util.NewAgentError("at least one location must be specified in 'locations'", nil)
	}
	var locations []string
	for _, loc := range locationsRaw {
		locStr, ok := loc.(string)
		if !ok {
			return nil, util.NewAgentError(fmt.Sprintf("invalid location value: %v", loc), nil)
		}
		locations = append(locations, locStr)
	}

	// Parse root_entities
	rootEntitiesRaw, ok := paramsMap["root_entities"].([]any)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'root_entities' parameter: %v", paramsMap["root_entities"]), nil)
	}
	if len(rootEntitiesRaw) == 0 {
		return nil, util.NewAgentError("at least one root entity must be specified in 'root_entities'", nil)
	}

	var rootEntities []*lineagepb.EntityReference
	for i, item := range rootEntitiesRaw {
		itemMap, ok := item.(map[string]any)
		if !ok {
			return nil, util.NewAgentError(fmt.Sprintf("invalid entity at index %d in 'root_entities': expected object, got %T", i, item), nil)
		}
		fqn, ok := itemMap["fully_qualified_name"].(string)
		if !ok {
			return nil, util.NewAgentError(fmt.Sprintf("missing or invalid 'fully_qualified_name' in 'root_entities' at index %d", i), nil)
		}

		var fields []string
		if fieldsRaw, ok := itemMap["fields"]; ok {
			fieldsList, ok := fieldsRaw.([]any)
			if !ok {
				return nil, util.NewAgentError(fmt.Sprintf("invalid 'fields' in 'root_entities' at index %d: expected array", i), nil)
			}
			for _, f := range fieldsList {
				fStr, ok := f.(string)
				if !ok {
					return nil, util.NewAgentError(fmt.Sprintf("invalid field value in 'root_entities' at index %d: expected string", i), nil)
				}
				fields = append(fields, fStr)
			}
		}

		rootEntities = append(rootEntities, &lineagepb.EntityReference{
			FullyQualifiedName: fqn,
			Field:              fields,
		})
	}

	// Parse direction
	directionStr, ok := paramsMap["direction"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'direction' parameter: %v", paramsMap["direction"]), nil)
	}

	var direction lineagepb.SearchLineageStreamingRequest_SearchDirection
	switch strings.ToUpper(directionStr) {
	case "UPSTREAM":
		direction = lineagepb.SearchLineageStreamingRequest_UPSTREAM
	case "DOWNSTREAM":
		direction = lineagepb.SearchLineageStreamingRequest_DOWNSTREAM
	default:
		return nil, util.NewAgentError(fmt.Sprintf("invalid direction %q: must be UPSTREAM or DOWNSTREAM", directionStr), nil)
	}

	// Parse limits
	var maxDepth int32
	if val, ok := paramsMap["max_depth"].(int); ok {
		maxDepth = int32(val)
	}

	var maxResults int32
	if val, ok := paramsMap["max_results"].(int); ok {
		maxResults = int32(val)
	}

	var maxProcessPerLink int32
	if val, ok := paramsMap["max_process_per_link"].(int); ok {
		maxProcessPerLink = int32(val)
	}

	var requestProcessDetails bool
	if val, ok := paramsMap["request_process_details"].(bool); ok {
		requestProcessDetails = val
	}

	// If process details are requested, ensure maxProcessPerLink is non-zero
	if requestProcessDetails && maxProcessPerLink == 0 {
		return nil, util.NewAgentError("max_process_per_link must be greater than 0 when request_process_details is true", nil)
	}

	// Use first location as parent location
	parentLocation := locations[0]

	links, unreachable, err := source.SearchLineageStreaming(ctx, parentLocation, locations, rootEntities, direction, maxDepth, maxResults, maxProcessPerLink, requestProcessDetails)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	return SearchLineageResponse{
		Links:       links,
		Unreachable: unreachable,
	}, nil
}
