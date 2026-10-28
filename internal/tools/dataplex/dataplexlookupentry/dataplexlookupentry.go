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

package dataplexlookupentry

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	dataplexpb "cloud.google.com/go/dataplex/apiv1/dataplexpb"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "dataplex-lookup-entry"

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
	LookupEntry(context.Context, string, int, []string, string) (*dataplexpb.Entry, error)
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
	viewDesc := `
				## Argument: view

				**Type:** Integer

				**Description:** Optional. Specifies the parts of the entry and its aspects to return.

				**Possible Values:**

				*   1 (BASIC): Returns entry without aspects.
				*   2 (FULL): Return all required aspects and the keys of non-required aspects. (Default)
				*   3 (CUSTOM): Return the entry and aspects requested in aspect_types field (at most 100 aspects). Always use this view when aspect_types is not empty.
				*   4 (ALL): Return the entry and both required and optional aspects (at most 100 aspects)
				`

	view := parameters.NewIntParameter("view", viewDesc, parameters.WithIntDefault(2))
	aspectTypes := parameters.NewArrayParameter("aspectTypes", "Optional. Limits the aspects returned to the provided aspect types. It only works when used together with CUSTOM view.", parameters.NewStringParameter("aspectType", "The types of aspects to be included in the response in the format `projects/{project}/locations/{location}/aspectTypes/{aspectType}`."), parameters.WithArrayDefault([]any{}))
	entry := parameters.NewStringParameter("entry", "Required. The resource name of the Entry in the following form: projects/{project}/locations/{location}/entryGroups/{entryGroup}/entries/{entry}.")
	allParameters := parameters.Parameters{entry, view, aspectTypes}

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
	entry, _ := paramsMap["entry"].(string)
	view, _ := paramsMap["view"].(int)
	aspectTypeSlice, err := parameters.ConvertAnySliceToTyped(paramsMap["aspectTypes"].([]any), "string")
	if err != nil {
		return nil, util.NewAgentError(fmt.Sprintf("can't convert aspectTypes to array of strings: %s", err), err)
	}
	parts := strings.Split(entry, "/")
	if len(parts) < 4 || parts[0] != "projects" || parts[2] != "locations" {
		err = fmt.Errorf("invalid entry format: must be in the form projects/{project}/locations/{location}/entryGroups/{entryGroup}/entries/{entry}")
		return nil, util.NewAgentError(err.Error(), err)
	}
	name := strings.Join(parts[:4], "/")
	aspectTypes := aspectTypeSlice.([]string)
	resp, err := source.LookupEntry(ctx, name, view, aspectTypes, entry)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}
