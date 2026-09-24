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

package fhirpatientsearch

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

const resourceType string = "cloud-healthcare-fhir-patient-search"
const (
	activeKey           = "active"
	cityKey             = "city"
	countryKey          = "country"
	postalCodeKey       = "postalcode"
	stateKey            = "state"
	addressSubstringKey = "addressSubstring"
	birthDateRangeKey   = "birthDateRange"
	deathDateRangeKey   = "deathDateRange"
	deceasedKey         = "deceased"
	emailKey            = "email"
	genderKey           = "gender"
	addressUseKey       = "addressUse"
	nameKey             = "name"
	givenNameKey        = "givenName"
	familyNameKey       = "familyName"
	phoneKey            = "phone"
	languageKey         = "language"
	identifierKey       = "identifier"
	summaryKey          = "summary"
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
	FHIRPatientSearch(string, string, []googleapi.CallOption) (any, error)
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
		return nil, util.NewAgentError("failed to validate store ID", err)
	}

	var tokenStr string
	if source.UseClientAuthorization() {
		tokenStr, err = accessToken.ParseBearerToken()
		if err != nil {
			return nil, util.NewClientServerError("error parsing access token", http.StatusUnauthorized, err)
		}
	}

	var summary bool
	var opts []googleapi.CallOption
	for k, v := range params.AsMap() {
		if k == common.StoreKey {
			continue
		}
		if k == summaryKey {
			var ok bool
			summary, ok = v.(bool)
			if !ok {
				return nil, util.NewAgentError(fmt.Sprintf("invalid '%s' parameter; expected a boolean", summaryKey), nil)
			}
			continue
		}

		val, ok := v.(string)
		if !ok {
			return nil, util.NewAgentError(fmt.Sprintf("invalid parameter '%s'; expected a string", k), nil)
		}
		if val == "" {
			continue
		}
		switch k {
		case activeKey, deceasedKey, emailKey, genderKey, phoneKey, languageKey, identifierKey:
			opts = append(opts, googleapi.QueryParameter(k, val))
		case cityKey, countryKey, postalCodeKey, stateKey:
			opts = append(opts, googleapi.QueryParameter("address-"+k, val))
		case addressSubstringKey:
			opts = append(opts, googleapi.QueryParameter("address", val))
		case birthDateRangeKey, deathDateRangeKey:
			key := "birthdate"
			if k == deathDateRangeKey {
				key = "death-date"
			}
			parts := strings.Split(val, "/")
			if len(parts) != 2 {
				return nil, util.NewAgentError(fmt.Sprintf("invalid '%s' format; expected YYYY-MM-DD/YYYY-MM-DD", k), nil)
			}
			var values []string
			if parts[0] != "" {
				values = append(values, "ge"+parts[0])
			}
			if parts[1] != "" {
				values = append(values, "le"+parts[1])
			}
			if len(values) != 0 {
				opts = append(opts, googleapi.QueryParameter(key, values...))
			}
		case addressUseKey:
			opts = append(opts, googleapi.QueryParameter("address-use", val))
		case nameKey:
			parts := strings.Split(val, " ")
			for _, part := range parts {
				opts = append(opts, googleapi.QueryParameter("name", part))
			}
		case givenNameKey:
			opts = append(opts, googleapi.QueryParameter("given", val))
		case familyNameKey:
			opts = append(opts, googleapi.QueryParameter("family", val))
		default:
			return nil, util.NewAgentError(fmt.Sprintf("unexpected parameter key %q", k), nil)
		}
	}
	if summary {
		opts = append(opts, googleapi.QueryParameter("_summary", "text"))
	}
	resp, err := source.FHIRPatientSearch(storeID, tokenStr, opts)
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
		parameters.NewStringParameter(activeKey, "Whether the patient record is active. Use true or false", parameters.WithStringDefault("")),
		parameters.NewStringParameter(cityKey, "The city of the patient's address", parameters.WithStringDefault("")),
		parameters.NewStringParameter(countryKey, "The country of the patient's address", parameters.WithStringDefault("")),
		parameters.NewStringParameter(postalCodeKey, "The postal code of the patient's address", parameters.WithStringDefault("")),
		parameters.NewStringParameter(stateKey, "The state of the patient's address", parameters.WithStringDefault("")),
		parameters.NewStringParameter(addressSubstringKey, "A substring to search for in any address field", parameters.WithStringDefault("")),
		parameters.NewStringParameter(birthDateRangeKey, "A date range for the patient's birthdate in the format YYYY-MM-DD/YYYY-MM-DD. Omit the first or second date to indicate open-ended ranges (e.g. '/2000-01-01' or '1950-01-01/')", parameters.WithStringDefault("")),
		parameters.NewStringParameter(deathDateRangeKey, "A date range for the patient's death date in the format YYYY-MM-DD/YYYY-MM-DD. Omit the first or second date to indicate open-ended ranges (e.g. '/2000-01-01' or '1950-01-01/')", parameters.WithStringDefault("")),
		parameters.NewStringParameter(deceasedKey, "Whether the patient is deceased. Use true or false", parameters.WithStringDefault("")),
		parameters.NewStringParameter(emailKey, "The patient's email address", parameters.WithStringDefault("")),
		parameters.NewStringParameter(genderKey, "The patient's gender. Must be one of 'male', 'female', 'other', or 'unknown'", parameters.WithStringDefault("")),
		parameters.NewStringParameter(addressUseKey, "The use of the patient's address. Must be one of 'home', 'work', 'temp', 'old', or 'billing'", parameters.WithStringDefault("")),
		parameters.NewStringParameter(nameKey, "The patient's name. Can be a family name, given name, or both", parameters.WithStringDefault("")),
		parameters.NewStringParameter(givenNameKey, "A portion of the given name of the patient", parameters.WithStringDefault("")),
		parameters.NewStringParameter(familyNameKey, "A portion of the family name of the patient", parameters.WithStringDefault("")),
		parameters.NewStringParameter(phoneKey, "The patient's phone number", parameters.WithStringDefault("")),
		parameters.NewStringParameter(languageKey, "The patient's preferred language. Must be a valid BCP-47 code (e.g. 'en-US', 'es')", parameters.WithStringDefault("")),
		parameters.NewStringParameter(identifierKey, "An identifier for the patient", parameters.WithStringDefault("")),
		parameters.NewBooleanParameter(summaryKey, "Requests the server to return a subset of the resource. Return a limited subset of elements from the resource. Enabled by default to reduce response size. Use get-fhir-resource tool to get full resource details (preferred) or set to false to disable.", parameters.WithBooleanDefault(true)),
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
