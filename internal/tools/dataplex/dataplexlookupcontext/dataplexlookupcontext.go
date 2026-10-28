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

package dataplexlookupcontext

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

const resourceType string = "dataplex-lookup-context"

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
	LookupContext(context.Context, string, []string) (*dataplexpb.LookupContextResponse, error)
	ProjectID() string
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                `yaml:"type" validate:"required"`
	Source           string                `yaml:"source" validate:"required"`
	Parameters       parameters.Parameters `yaml:"parameters"`
}

// validate interface
var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	resources := parameters.NewArrayParameter("resources",
		"A list of up to 10 resource names. Resources may belong to different projects, but all must belong to the same location.",
		parameters.NewStringParameter("resource",
			"Name of a resource in the following format: projects/{project_id_or_number}/locations/{location}/entryGroups/{group}/entries/{entry}."+
				" Example for a BigQuery table: 'projects/{project_id_or_number}/locations/{location}/entryGroups/@bigquery/entries/bigquery.googleapis.com/projects/{project_id}/datasets/{dataset_id}/tables/{table_id}'."+
				" This is the same value which is returned by the search_entries tool's response in the dataplexEntry.name field."))
	allParameters := parameters.Parameters{resources}

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
	resourcesSlice, err := parameters.ConvertAnySliceToTyped(paramsMap["resources"].([]any), "string")
	if err != nil {
		return nil, util.NewAgentError(fmt.Sprintf("can't convert resources to array of strings: %s", err), err)
	}
	resources := resourcesSlice.([]string)

	if len(resources) == 0 {
		err := fmt.Errorf("resources cannot be empty")
		return nil, util.NewAgentError(err.Error(), err)
	}
	var name string
	var firstLocation string
	for i, resource := range resources {
		parts := strings.Split(resource, "/")
		if len(parts) < 4 || parts[0] != "projects" || parts[2] != "locations" {
			err := fmt.Errorf("invalid resource format at index %d, must be in the format of projects/{project_id_or_number}/locations/{location}/entryGroups/{group}/entries/{entry}", i)
			return nil, util.NewAgentError(err.Error(), err)
		}

		location := parts[3]
		if i == 0 {
			firstLocation = location
			project := source.ProjectID()
			name = fmt.Sprintf("projects/%s/locations/%s", project, location)
		} else if location != firstLocation {
			err := fmt.Errorf("all resources must belong to the same location. Please make separate calls for each distinct location")
			return nil, util.NewAgentError(err.Error(), err)
		}
	}

	resp, err := source.LookupContext(ctx, name, resources)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}
