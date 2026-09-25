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

package fhirpatienteverything

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudhealthcare/common"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/api/googleapi"
)

const resourceType string = "cloud-healthcare-fhir-patient-everything"
const (
	patientIDKey   = "patientID"
	typeFilterKey  = "resourceTypesFilter"
	sinceFilterKey = "sinceFilter"
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
	AllowedFHIRStores() map[string]struct{}
	UseClientAuthorization() bool
	FHIRPatientEverything(string, string, string, []googleapi.CallOption) (any, error)
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
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	params := buildParams(false)
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

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	storeID, err := common.ValidateAndFetchStoreID(params, source.AllowedFHIRStores())
	if err != nil {
		// ValidateAndFetchStoreID usually returns input validation errors
		return nil, util.NewAgentError("failed to validate store ID", err)
	}
	patientID, ok := params.AsMap()[patientIDKey].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a string", patientIDKey), nil)
	}

	var tokenStr string
	if source.UseClientAuthorization() {
		tokenStr, err = accessToken.ParseBearerToken()
		if err != nil {
			return nil, util.NewClientServerError("error parsing access token", http.StatusUnauthorized, err)
		}
	}

	var opts []googleapi.CallOption
	if val, ok := params.AsMap()[typeFilterKey]; ok {
		types, ok := val.([]any)
		if !ok {
			return nil, util.NewAgentError(fmt.Sprintf("invalid '%s' parameter; expected a string array", typeFilterKey), nil)
		}
		typeFilterSlice, err := parameters.ConvertAnySliceToTyped(types, "string")
		if err != nil {
			return nil, util.NewAgentError(fmt.Sprintf("can't convert '%s' to array of strings: %s", typeFilterKey, err), err)
		}
		if len(typeFilterSlice.([]string)) != 0 {
			opts = append(opts, googleapi.QueryParameter("_type", strings.Join(typeFilterSlice.([]string), ",")))
		}
	}
	if since, ok := params.AsMap()[sinceFilterKey]; ok {
		sinceStr, ok := since.(string)
		if !ok {
			return nil, util.NewAgentError(fmt.Sprintf("invalid '%s' parameter; expected a string", sinceFilterKey), nil)
		}
		if sinceStr != "" {
			opts = append(opts, googleapi.QueryParameter("_since", sinceStr))
		}
	}

	resp, err := source.FHIRPatientEverything(storeID, patientID, tokenStr, opts)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}

// buildParams builds the tool's parameters. When the source pins exactly one store
// (singleStore), the store param is omitted; otherwise it is included.
func buildParams(singleStore bool) parameters.Parameters {
	params := parameters.Parameters{
		parameters.NewStringParameter(patientIDKey, "The ID of the patient FHIR resource for which the information is required"),
		parameters.NewArrayParameter(typeFilterKey, "List of FHIR resource types. If provided, only resources of the specified resource type(s) are returned.", parameters.NewStringParameter("resourceType", "A FHIR resource type"), parameters.WithArrayDefault([]any{})),
		parameters.NewStringParameter(sinceFilterKey, "If provided, only resources updated after this time are returned. The time uses the format YYYY-MM-DDThh:mm:ss.sss+zz:zz. The time must be specified to the second and include a time zone. For example, 2015-02-07T13:28:17.239+02:00 or 2017-01-01T00:00:00Z", parameters.WithStringDefault("")),
	}
	if !singleStore {
		params = append(params, parameters.NewStringParameter(common.StoreKey, "The FHIR store ID to retrieve the resource from."))
	}
	return params
}

// resolveParams builds the tool's parameters using the source's configured FHIR/DICOM stores.
func (t Tool) resolveParams(source sources.Source) (parameters.Parameters, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return nil, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return buildParams(len(s.AllowedFHIRStores()) == 1), nil
}

// GetParameters returns the tool's parameters, resolved against the source.
func (t Tool) GetParameters(source sources.Source) (parameters.Parameters, error) {
	return t.resolveParams(source)
}

// Manifest returns the tool's manifest, resolved against the source.
func (t Tool) Manifest(source sources.Source) (tools.Manifest, error) {
	allParameters, err := t.resolveParams(source)
	if err != nil {
		return tools.Manifest{}, err
	}
	return tools.Manifest{Description: t.Cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: t.Cfg.AuthRequired}, nil
}
