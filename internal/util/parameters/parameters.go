// Copyright 2024 Google LLC
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

package parameters

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"text/template"

	embeddingmodels "github.com/googleapis/mcp-toolbox/internal/embeddingmodels"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

const (
	TypeString = "string"
	TypeInt    = "integer"
	TypeFloat  = "float"
	TypeBool   = "boolean"
	TypeArray  = "array"
	TypeMap    = "map"
)

// delimiters for string parameter escaping
const (
	escapeBackticks      = "backticks"
	escapeDoubleQuotes   = "double-quotes"
	escapeSingleQuotes   = "single-quotes"
	escapeSquareBrackets = "square-brackets"
)

// ParamValues is an ordered list of ParamValue
type ParamValues []ParamValue

// ParamValue represents the parameter's name and value.
type ParamValue struct {
	Name  string
	Value any
}

// AsSlice returns a slice of the Param's values (in order).
func (p ParamValues) AsSlice() []any {
	params := []any{}

	for _, p := range p {
		params = append(params, p.Value)
	}
	return params
}

// AsMap returns a map of ParamValue's names to values.
func (p ParamValues) AsMap() map[string]interface{} {
	params := make(map[string]interface{})
	for _, p := range p {
		params[p.Name] = p.Value
	}
	return params
}

// AsMapByOrderedKeys returns a map of a key's position to it's value, as necessary for Spanner PSQL.
// Example { $1 -> "value1", $2 -> "value2" }
func (p ParamValues) AsMapByOrderedKeys() map[string]interface{} {
	params := make(map[string]interface{})

	for i, p := range p {
		key := fmt.Sprintf("p%d", i+1)
		params[key] = p.Value
	}
	return params
}

// AsMapWithDollarPrefix ensures all keys are prefixed with a dollar sign for Dgraph.
// Example:
// Input:  {"role": "admin", "$age": 30}
// Output: {"$role": "admin", "$age": 30}
func (p ParamValues) AsMapWithDollarPrefix() map[string]interface{} {
	params := make(map[string]interface{})

	for _, param := range p {
		key := param.Name
		if !strings.HasPrefix(key, "$") {
			key = "$" + key
		}
		params[key] = param.Value
	}
	return params
}

func parseFromAuthService(paramAuthServices []ParamAuthService, claimsMap map[string]map[string]any) (any, error) {
	// parse a parameter from claims using its specified auth services
	for _, a := range paramAuthServices {
		claims, ok := claimsMap[a.Name]
		if !ok {
			// not validated for this authservice, skip to the next one
			continue
		}
		v, ok := claims[a.Field]
		if !ok {
			// claims do not contain specified field
			return nil, fmt.Errorf("no field named %s in claims", a.Field)
		}
		return v, nil
	}
	return nil, util.NewClientServerError("missing or invalid authentication header", http.StatusUnauthorized, nil)
}

// CheckParamRequired checks if a parameter is required based on the required and default field.
func CheckParamRequired(required bool, defaultV any) bool {
	return required && defaultV == nil
}

// ParseParams is a helper function for parsing Parameters from an arbitraryJSON object.
func ParseParams(ps Parameters, data map[string]any, claimsMap map[string]map[string]any) (ParamValues, error) {
	params := make([]ParamValue, 0, len(ps))
	for _, p := range ps {
		var v, newV any
		var err error
		paramAuthServices := p.GetAuthServices()
		name := p.GetName()

		sourceParamName := p.GetValueFromParam()
		if sourceParamName != "" {
			v = data[sourceParamName]

		} else if len(paramAuthServices) == 0 {
			// parse non auth-required parameter
			var ok bool
			v, ok = data[name]
			if !ok || v == nil {
				v = p.GetDefault()
				// if the parameter is required and no value given, throw an error
				if CheckParamRequired(p.GetRequired(), v) {
					return nil, util.NewAgentError(fmt.Sprintf("parameter %q is required", name), nil)
				}
			}
		} else {
			// parse authenticated parameter
			v, err = parseFromAuthService(paramAuthServices, claimsMap)
			if err != nil {
				return nil, util.NewClientServerError(fmt.Sprintf("error parsing authenticated parameter %q", name), http.StatusUnauthorized, err)
			}
		}
		if v != nil {
			newV, err = p.Parse(v)
			if err != nil {
				return nil, util.NewAgentError(fmt.Sprintf("unable to parse value for %q", name), err)
			}
		}
		params = append(params, ParamValue{Name: name, Value: newV})
	}
	return params, nil
}

// PrimitiveManagerI defines the minimal view of the primitive manager needed by parameters.
type PrimitiveManagerI interface {
	GetEmbeddingModel(string) (embeddingmodels.EmbeddingModel, bool)
}

func EmbedParams(ctx context.Context, ps Parameters, paramValues ParamValues, pMgr PrimitiveManagerI, formatter embeddingmodels.VectorFormatter) (ParamValues, error) {

	type ParamToEmbed struct {
		OriginalValue string
		Index         int // The index in the original Parameters slice
	}

	// Map: modelName -> list of ParamToEmbed
	parametersToEmbed := make(map[string][]ParamToEmbed)

	for i, p := range ps {
		modelName := p.GetEmbeddedBy()
		if modelName == "" {
			continue
		}

		// Get parameter's value to be embedded
		valueStr, ok := paramValues[i].Value.(string)
		if !ok {
			return nil, fmt.Errorf("parameter '%s' is marked for embedding but has a non-string value (type: %T)", p.GetName(), paramValues[i].Value)
		}

		parametersToEmbed[modelName] = append(parametersToEmbed[modelName], ParamToEmbed{
			OriginalValue: valueStr,
			Index:         i,
		})
	}

	// Batch embedding request sent to each model
	for modelName, params := range parametersToEmbed {
		model, ok := pMgr.GetEmbeddingModel(modelName)
		if !ok {
			return nil, fmt.Errorf("embedding model does not exist: %s", modelName)
		}

		// Extract only the string values for the API call
		stringBatch := make([]string, len(params))
		for i, paramStr := range params {
			stringBatch[i] = paramStr.OriginalValue
		}

		embeddings, err := model.EmbedParameters(ctx, stringBatch)
		if err != nil {
			return nil, fmt.Errorf("error embedding parameters with model %s: %w", modelName, err)
		}

		if len(embeddings) != len(stringBatch) {
			return nil, fmt.Errorf("model %s returned %d embeddings for %d inputs", modelName, len(embeddings), len(stringBatch))
		}

		for i, rawVector := range embeddings {

			item := params[i]

			// Call vector formatter
			var finalValue any = rawVector

			if formatter == nil {
				paramValues[item.Index].Value = finalValue
				continue
			}

			formattedVector := formatter(rawVector)
			finalValue = formattedVector
			paramValues[item.Index].Value = finalValue
		}
	}
	return paramValues, nil
}

// helper function to convert a string array parameter to a comma separated string
func ConvertArrayParamToString(param any) (string, error) {
	switch v := param.(type) {
	case []any:
		var stringValues []string
		for _, item := range v {
			stringVal, ok := item.(string)
			if !ok {
				return "", fmt.Errorf("templateParameter only supports string arrays")
			}
			stringValues = append(stringValues, stringVal)
		}
		return strings.Join(stringValues, ", "), nil
	default:
		return "", fmt.Errorf("invalid parameter type, expected array of type string")
	}
}

// GetParams return the ParamValues that are associated with the Parameters.
func GetParams(params Parameters, paramValuesMap map[string]any) (ParamValues, error) {
	resultParamValues := make(ParamValues, 0)
	for _, p := range params {
		k := p.GetName()
		v, ok := paramValuesMap[k]
		if !ok {
			return nil, fmt.Errorf("missing parameter %s", k)
		}
		resultParamValues = append(resultParamValues, ParamValue{Name: k, Value: v})
	}
	return resultParamValues, nil
}

func ResolveTemplateParams(templateParams Parameters, originalStatement string, paramsMap map[string]any) (string, error) {
	templateParamsValues, err := GetParams(templateParams, paramsMap)
	templateParamsMap := templateParamsValues.AsMap()
	if err != nil {
		return "", fmt.Errorf("error getting template params %s", err)
	}

	funcMap := template.FuncMap{
		"array": ConvertArrayParamToString,
	}
	t, err := template.New("statement").Funcs(funcMap).Parse(originalStatement)
	if err != nil {
		return "", fmt.Errorf("error creating go template %s", err)
	}
	var result bytes.Buffer
	err = t.Execute(&result, templateParamsMap)
	if err != nil {
		return "", fmt.Errorf("error executing go template %s", err)
	}

	modifiedStatement := result.String()
	return modifiedStatement, nil
}

// ProcessParameters concatenate templateParameters and parameters from a tool.
// It returns a list of concatenated parameters, concatenated Toolbox manifest, and concatenated MCP Manifest.
func ProcessParameters(templateParams Parameters, params Parameters) (Parameters, []ParameterManifest, error) {
	allParameters := slices.Concat(params, templateParams)

	// verify no duplicate parameter names
	err := CheckDuplicateParameters(allParameters)
	if err != nil {
		return nil, nil, err
	}

	// verify parameter restrictions
	for _, p := range allParameters {
		if err := validateParameter(p); err != nil {
			return nil, nil, err
		}
	}

	// create Toolbox manifest
	paramManifest := allParameters.Manifest()
	if paramManifest == nil {
		paramManifest = make([]ParameterManifest, 0)
	}

	return allParameters, paramManifest, nil
}

// validateParameter validates that parameter configuration adheres to system constraints.
func validateParameter(p Parameter) error {
	if p.GetSecure() {
		if len(p.GetAuthServices()) > 0 {
			return fmt.Errorf("parameter %q cannot have both 'secure' set to true and 'authServices' specified", p.GetName())
		}
		if p.GetDefault() != nil {
			return fmt.Errorf("parameter %q cannot have both 'secure' set to true and 'default' specified", p.GetName())
		}
		if !p.GetRequired() {
			return fmt.Errorf("parameter %q cannot have both 'secure' set to true and 'required' set to false", p.GetName())
		}
	}
	return nil
}

type Parameter interface {
	// Note: It's typically not idiomatic to include "Get" in the function name,
	// but this is done to differentiate it from the fields in CommonParameter.
	GetName() string
	GetDesc() string
	GetType() string
	GetDefault() any
	GetRequired() bool
	GetAuthServices() []ParamAuthService
	GetEmbeddedBy() string
	GetValueFromParam() string
	Parse(any) (any, error)
	Manifest() ParameterManifest
	McpManifest() (ParameterMcpManifest, []string)
	GetSecure() bool
}

// Parameters is a type used to allow unmarshal a list of parameters
type Parameters []Parameter

func (c *Parameters) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	*c = make(Parameters, 0)
	var rawList []util.DelayedUnmarshaler
	if err := unmarshal(&rawList); err != nil {
		return err
	}
	for _, u := range rawList {
		p, err := parseParamFromDelayedUnmarshaler(ctx, &u)
		if err != nil {
			return err
		}
		(*c) = append((*c), p)
	}
	return nil
}

// parseParamFromDelayedUnmarshaler is a helper function that is required to parse
// parameters because there are multiple different types
func parseParamFromDelayedUnmarshaler(ctx context.Context, u *util.DelayedUnmarshaler) (Parameter, error) {
	var p map[string]any
	err := u.Unmarshal(&p)
	if err != nil {
		return nil, fmt.Errorf("error parsing parameters: %w", err)
	}

	t, ok := p["type"]
	if !ok {
		return nil, fmt.Errorf("parameter is missing 'type' field")
	}

	typeStr, ok := t.(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'type' field must be a string, got %T", t)
	}

	param, err := ParseParameter(ctx, p, typeStr)
	if err != nil {
		return nil, err
	}

	if err := validateParameter(param); err != nil {
		return nil, err
	}

	return param, nil
}

// ParseParameter parses a raw map into a Parameter object based on its "type" field.
func ParseParameter(ctx context.Context, p map[string]any, paramType string) (Parameter, error) {
	dec, err := util.NewStrictDecoder(p)
	if err != nil {
		return nil, fmt.Errorf("error creating decoder: %w", err)
	}
	switch paramType {
	case TypeString:
		a := &StringParameter{}
		if err := dec.DecodeContext(ctx, a); err != nil {
			return nil, fmt.Errorf("unable to parse as %q: %w", paramType, err)
		}
		return a, nil
	case TypeInt:
		a := &IntParameter{}
		if err := dec.DecodeContext(ctx, a); err != nil {
			return nil, fmt.Errorf("unable to parse as %q: %w", paramType, err)
		}
		if a.GetEmbeddedBy() != "" {
			return nil, fmt.Errorf("parameter type %q cannot specify 'embeddedBy'", paramType)
		}
		return a, nil
	case TypeFloat:
		a := &FloatParameter{}
		if err := dec.DecodeContext(ctx, a); err != nil {
			return nil, fmt.Errorf("unable to parse as %q: %w", paramType, err)
		}
		if a.GetEmbeddedBy() != "" {
			return nil, fmt.Errorf("parameter type %q cannot specify 'embeddedBy'", paramType)
		}
		return a, nil
	case TypeBool:
		a := &BooleanParameter{}
		if err := dec.DecodeContext(ctx, a); err != nil {
			return nil, fmt.Errorf("unable to parse as %q: %w", paramType, err)
		}
		if a.GetEmbeddedBy() != "" {
			return nil, fmt.Errorf("parameter type %q cannot specify 'embeddedBy'", paramType)
		}
		return a, nil
	case TypeArray:
		a := &ArrayParameter{}
		if err := dec.DecodeContext(ctx, a); err != nil {
			return nil, fmt.Errorf("unable to parse as %q: %w", paramType, err)
		}
		if a.GetEmbeddedBy() != "" {
			return nil, fmt.Errorf("parameter type %q cannot specify 'embeddedBy'", paramType)
		}
		return a, nil
	case TypeMap:
		a := &MapParameter{}
		if err := dec.DecodeContext(ctx, a); err != nil {
			return nil, fmt.Errorf("unable to parse as %q: %w", paramType, err)
		}
		if a.GetEmbeddedBy() != "" {
			return nil, fmt.Errorf("parameter type %q cannot specify 'embeddedBy'", paramType)
		}
		return a, nil
	}
	return nil, fmt.Errorf("%q is not valid type for a parameter", paramType)
}

func (ps Parameters) Manifest() []ParameterManifest {
	rtn := make([]ParameterManifest, 0, len(ps))
	for _, p := range ps {
		if p.GetValueFromParam() != "" {
			continue
		}
		rtn = append(rtn, p.Manifest())
	}
	return rtn
}

// ParameterManifest represents parameters when served as part of a ToolManifest.
type ParameterManifest struct {
	Name                 string             `json:"name"`
	Type                 string             `json:"type"`
	Required             bool               `json:"required"`
	Description          string             `json:"description"`
	AuthServices         []string           `json:"authServices"`
	Items                *ParameterManifest `json:"items,omitempty"`
	Default              any                `json:"default,omitempty"`
	AdditionalProperties any                `json:"additionalProperties,omitempty"`
	EmbeddedBy           string             `json:"embeddedBy,omitempty"`
	ValueFromParam       string             `json:"valueFromParam,omitempty"`
}

// ParameterMcpManifest represents properties when served as part of a ToolMcpManifest.
type ParameterMcpManifest struct {
	Type                 string                `json:"type"`
	Description          string                `json:"description"`
	Items                *ParameterMcpManifest `json:"items,omitempty"`
	Default              any                   `json:"default,omitempty"`
	AdditionalProperties any                   `json:"additionalProperties,omitempty"`
}

// CommonParameter are default fields that are emebdding in most Parameter implementations. Embedding this stuct will give the object Name() and Type() functions.
type CommonParameter struct {
	Name           string             `yaml:"name" validate:"required"`
	Type           string             `yaml:"type" validate:"required"`
	Desc           string             `yaml:"description" validate:"required"`
	Required       *bool              `yaml:"required"`
	AllowedValues  []any              `yaml:"allowedValues"`
	ExcludedValues []any              `yaml:"excludedValues"`
	AuthServices   []ParamAuthService `yaml:"authServices"`
	EmbeddedBy     string             `yaml:"embeddedBy"`
	ValueFromParam string             `yaml:"valueFromParam"`
	Secure         bool               `yaml:"secure"`
}

// GetName returns the name specified for the Parameter.
func (p *CommonParameter) GetName() string {
	return p.Name
}

// GetSecure returns whether the parameter is secure.
func (p *CommonParameter) GetSecure() bool {
	return p.Secure
}

// GetDesc returns the description specified for the Parameter.
func (p *CommonParameter) GetDesc() string {
	return p.Desc
}

// GetType returns the type specified for the Parameter.
func (p *CommonParameter) GetType() string {
	return p.Type
}

// GetRequired returns the type specified for the Parameter.
func (p *CommonParameter) GetRequired() bool {
	// parameters are defaulted to required
	if p.Required == nil {
		return true
	}
	return *p.Required
}

// GetAllowedValues returns the allowed values for the Parameter.
func (p *CommonParameter) GetAllowedValues() []any {
	return p.AllowedValues
}

// IsAllowedValues checks if the value is allowed.
func (p *CommonParameter) IsAllowedValues(v any) bool {
	if len(p.AllowedValues) == 0 {
		return true
	}
	for _, av := range p.AllowedValues {
		if MatchStringOrRegex(v, av) {
			return true
		}
	}
	return false
}

// GetExcludedValues returns the excluded values for the Parameter.
func (p *CommonParameter) GetExcludedValues() []any {
	return p.ExcludedValues
}

// IsExcludedValues checks if the value is allowed.
func (p *CommonParameter) IsExcludedValues(v any) bool {
	if len(p.ExcludedValues) == 0 {
		return false
	}
	for _, av := range p.ExcludedValues {
		if MatchStringOrRegex(v, av) {
			return true
		}
	}
	return false
}

// GetEmbeddedBy returns the embedding model name for the Parameter.
func (p *CommonParameter) GetEmbeddedBy() string {
	return p.EmbeddedBy
}

// GetValueFromParam returns the param value to copy from.
func (p *CommonParameter) GetValueFromParam() string {
	return p.ValueFromParam
}

// MatchStringOrRegex checks if the input matches the target
func MatchStringOrRegex(input, target any) bool {
	targetS, ok := target.(string)
	if !ok {
		return input == target
	}
	re, err := regexp.Compile(targetS)
	if err != nil {
		// if target is not regex, run direct comparison
		return input == target
	}
	inputS := fmt.Sprintf("%v", input)
	return re.MatchString(inputS)
}

// McpManifest returns the MCP manifest for the Parameter.
func (p *CommonParameter) McpManifest() (ParameterMcpManifest, []string) {
	authServiceNames := getAuthServiceNames(p.AuthServices)
	return ParameterMcpManifest{
		Type:        p.Type,
		Description: p.Desc,
	}, authServiceNames
}

// getAuthServiceNames retrieves the list of auth services names
func getAuthServiceNames(authServices []ParamAuthService) []string {
	authServiceNames := make([]string, len(authServices))
	for i, a := range authServices {
		authServiceNames[i] = a.Name
	}
	return authServiceNames
}

// ParseTypeError is a custom error for incorrectly typed Parameters.
type ParseTypeError struct {
	Name  string
	Type  string
	Value any
}

func (e ParseTypeError) Error() string {
	return fmt.Sprintf("%q not type %q", e.Value, e.Type)
}

type ParamAuthService struct {
	Name  string `yaml:"name"`
	Field string `yaml:"field"`
}

// NewStringParameter is a convenience function for initializing a StringParameter.
type StringParameterOption func(*StringParameter)

func WithStringRequired(v bool) StringParameterOption {
	return func(p *StringParameter) { p.Required = &v }
}
func WithStringAuth(v []ParamAuthService) StringParameterOption {
	return func(p *StringParameter) { p.AuthServices = v }
}
func WithStringAllowedValues(v []any) StringParameterOption {
	return func(p *StringParameter) { p.AllowedValues = v }
}
func WithStringExcludedValues(v []any) StringParameterOption {
	return func(p *StringParameter) { p.ExcludedValues = v }
}
func WithStringDefault(v string) StringParameterOption {
	return func(p *StringParameter) { p.Default = &v }
}
func WithStringEscape(v string) StringParameterOption {
	return func(p *StringParameter) { p.Escape = &v }
}

func NewStringParameter(name string, desc string, opts ...StringParameterOption) *StringParameter {
	p := &StringParameter{
		CommonParameter: CommonParameter{
			Name:         name,
			Type:         TypeString,
			Desc:         desc,
			AuthServices: nil,
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

var _ Parameter = &StringParameter{}

// StringParameter is a parameter representing the "string" type.
type StringParameter struct {
	CommonParameter `yaml:",inline"`
	Default         *string `yaml:"default"`
	Escape          *string `yaml:"escape"`
}

// Parse casts the value "v" as a "string".
func (p *StringParameter) Parse(v any) (any, error) {
	newV, ok := v.(string)
	if !ok {
		return nil, &ParseTypeError{p.Name, p.Type, v}
	}
	if !p.IsAllowedValues(newV) {
		return nil, fmt.Errorf("%s is not an allowed value", newV)
	}
	if p.IsExcludedValues(newV) {
		return nil, fmt.Errorf("%s is an excluded value", newV)
	}
	if p.Escape != nil {
		return applyEscape(*p.Escape, newV)
	}
	return newV, nil
}

func applyEscape(escape, v string) (any, error) {
	switch escape {
	case escapeBackticks:
		escaped := strings.ReplaceAll(v, "`", "``")
		return fmt.Sprintf("`%s`", escaped), nil
	case escapeDoubleQuotes:
		escaped := strings.ReplaceAll(v, `"`, `""`)
		return fmt.Sprintf(`"%s"`, escaped), nil
	case escapeSingleQuotes:
		escaped := strings.ReplaceAll(v, `'`, `''`)
		return fmt.Sprintf(`'%s'`, escaped), nil
	case escapeSquareBrackets:
		escaped := strings.ReplaceAll(v, "]", "]]")
		return fmt.Sprintf("[%s]", escaped), nil
	default:
		return nil, fmt.Errorf("%s is not an allowed escaping delimiter", escape)
	}
}

func (p *StringParameter) GetAuthServices() []ParamAuthService {
	return p.AuthServices
}

func (p *StringParameter) GetDefault() any {
	if p.Default == nil {
		return nil
	}
	return *p.Default
}

// Manifest returns the manifest for the StringParameter.
func (p *StringParameter) Manifest() ParameterManifest {
	// only list ParamAuthService names (without fields) in manifest
	authServiceNames := getAuthServiceNames(p.AuthServices)
	r := CheckParamRequired(p.GetRequired(), p.GetDefault())
	return ParameterManifest{
		Name:         p.Name,
		Type:         p.Type,
		Required:     r,
		Description:  p.Desc,
		AuthServices: authServiceNames,
		Default:      p.GetDefault(),
	}
}

// NewIntParameter is a convenience function for initializing a IntParameter.
type IntParameterOption func(*IntParameter)

func WithIntRequired(v bool) IntParameterOption { return func(p *IntParameter) { p.Required = &v } }
func WithIntAuth(v []ParamAuthService) IntParameterOption {
	return func(p *IntParameter) { p.AuthServices = v }
}
func WithIntAllowedValues(v []any) IntParameterOption {
	return func(p *IntParameter) { p.AllowedValues = v }
}
func WithIntExcludedValues(v []any) IntParameterOption {
	return func(p *IntParameter) { p.ExcludedValues = v }
}
func WithIntDefault(v int) IntParameterOption { return func(p *IntParameter) { p.Default = &v } }

func NewIntParameter(name string, desc string, opts ...IntParameterOption) *IntParameter {
	p := &IntParameter{
		CommonParameter: CommonParameter{
			Name:         name,
			Type:         TypeInt,
			Desc:         desc,
			AuthServices: nil,
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

var _ Parameter = &IntParameter{}

func WithIntMinValue(v *int) IntParameterOption { return func(p *IntParameter) { p.MinValue = v } }
func WithIntMaxValue(v *int) IntParameterOption { return func(p *IntParameter) { p.MaxValue = v } }

// IntParameter is a parameter representing the "int" type.
type IntParameter struct {
	CommonParameter `yaml:",inline"`
	Default         *int `yaml:"default"`
	MinValue        *int `yaml:"minValue"`
	MaxValue        *int `yaml:"maxValue"`
}

func (p *IntParameter) Parse(v any) (any, error) {
	var out int
	switch newV := v.(type) {
	default:
		return nil, &ParseTypeError{p.Name, p.Type, v}
	case int:
		out = int(newV)
	case int32:
		out = int(newV)
	case int64:
		out = int(newV)
	case json.Number:
		newI, err := newV.Int64()
		if err != nil {
			return nil, &ParseTypeError{p.Name, p.Type, v}
		}
		out = int(newI)
	}
	if !p.IsAllowedValues(out) {
		return nil, fmt.Errorf("%d is not an allowed value", out)
	}
	if p.IsExcludedValues(out) {
		return nil, fmt.Errorf("%d is an excluded value", out)
	}
	if p.MinValue != nil && out < *p.MinValue {
		return nil, fmt.Errorf("%d is under the minimum value", out)
	}
	if p.MaxValue != nil && out > *p.MaxValue {
		return nil, fmt.Errorf("%d is above the maximum value", out)
	}
	return out, nil
}

func (p *IntParameter) GetAuthServices() []ParamAuthService {
	return p.AuthServices
}

func (p *IntParameter) GetDefault() any {
	if p.Default == nil {
		return nil
	}
	return *p.Default
}

// Manifest returns the manifest for the IntParameter.
func (p *IntParameter) Manifest() ParameterManifest {
	// only list ParamAuthService names (without fields) in manifest
	authServiceNames := getAuthServiceNames(p.AuthServices)
	r := CheckParamRequired(p.GetRequired(), p.GetDefault())
	return ParameterManifest{
		Name:         p.Name,
		Type:         p.Type,
		Required:     r,
		Description:  p.Desc,
		AuthServices: authServiceNames,
		Default:      p.GetDefault(),
	}
}

// NewFloatParameter is a convenience function for initializing a FloatParameter.
type FloatParameterOption func(*FloatParameter)

func WithFloatRequired(v bool) FloatParameterOption {
	return func(p *FloatParameter) { p.Required = &v }
}
func WithFloatAuth(v []ParamAuthService) FloatParameterOption {
	return func(p *FloatParameter) { p.AuthServices = v }
}
func WithFloatAllowedValues(v []any) FloatParameterOption {
	return func(p *FloatParameter) { p.AllowedValues = v }
}
func WithFloatExcludedValues(v []any) FloatParameterOption {
	return func(p *FloatParameter) { p.ExcludedValues = v }
}
func WithFloatDefault(v float64) FloatParameterOption {
	return func(p *FloatParameter) { p.Default = &v }
}

func NewFloatParameter(name string, desc string, opts ...FloatParameterOption) *FloatParameter {
	p := &FloatParameter{
		CommonParameter: CommonParameter{
			Name:         name,
			Type:         TypeFloat,
			Desc:         desc,
			AuthServices: nil,
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

var _ Parameter = &FloatParameter{}

func WithFloatMinValue(v *float64) FloatParameterOption {
	return func(p *FloatParameter) { p.MinValue = v }
}
func WithFloatMaxValue(v *float64) FloatParameterOption {
	return func(p *FloatParameter) { p.MaxValue = v }
}

// FloatParameter is a parameter representing the "float" type.
type FloatParameter struct {
	CommonParameter `yaml:",inline"`
	Default         *float64 `yaml:"default"`
	MinValue        *float64 `yaml:"minValue"`
	MaxValue        *float64 `yaml:"maxValue"`
}

func (p *FloatParameter) Parse(v any) (any, error) {
	var out float64
	switch newV := v.(type) {
	default:
		return nil, &ParseTypeError{p.Name, p.Type, v}
	case float32:
		out = float64(newV)
	case float64:
		out = newV
	case json.Number:
		newI, err := newV.Float64()
		if err != nil {
			return nil, &ParseTypeError{p.Name, p.Type, v}
		}
		out = float64(newI)
	}
	if !p.IsAllowedValues(out) {
		return nil, fmt.Errorf("%g is not an allowed value", out)
	}
	if p.IsExcludedValues(out) {
		return nil, fmt.Errorf("%g is an excluded value", out)
	}
	if p.MinValue != nil && out < *p.MinValue {
		return nil, fmt.Errorf("%g is under the minimum value", out)
	}
	if p.MaxValue != nil && out > *p.MaxValue {
		return nil, fmt.Errorf("%g is above the maximum value", out)
	}
	return out, nil
}

func (p *FloatParameter) GetAuthServices() []ParamAuthService {
	return p.AuthServices
}

func (p *FloatParameter) GetDefault() any {
	if p.Default == nil {
		return nil
	}
	return *p.Default
}

// Manifest returns the manifest for the FloatParameter.
func (p *FloatParameter) Manifest() ParameterManifest {
	// only list ParamAuthService names (without fields) in manifest
	authServiceNames := getAuthServiceNames(p.AuthServices)
	r := CheckParamRequired(p.GetRequired(), p.GetDefault())
	return ParameterManifest{
		Name:         p.Name,
		Type:         p.Type,
		Required:     r,
		Description:  p.Desc,
		AuthServices: authServiceNames,
		Default:      p.GetDefault(),
	}
}

// McpManifest returns the MCP manifest for the FloatParameter.
// json schema only allow numeric types of 'integer' and 'number'.
func (p *FloatParameter) McpManifest() (ParameterMcpManifest, []string) {
	authServiceNames := getAuthServiceNames(p.AuthServices)
	return ParameterMcpManifest{
		Type:        "number",
		Description: p.Desc,
	}, authServiceNames
}

// NewBooleanParameter is a convenience function for initializing a BooleanParameter.
type BooleanParameterOption func(*BooleanParameter)

func WithBooleanRequired(v bool) BooleanParameterOption {
	return func(p *BooleanParameter) { p.Required = &v }
}
func WithBooleanAuth(v []ParamAuthService) BooleanParameterOption {
	return func(p *BooleanParameter) { p.AuthServices = v }
}
func WithBooleanAllowedValues(v []any) BooleanParameterOption {
	return func(p *BooleanParameter) { p.AllowedValues = v }
}
func WithBooleanExcludedValues(v []any) BooleanParameterOption {
	return func(p *BooleanParameter) { p.ExcludedValues = v }
}
func WithBooleanDefault(v bool) BooleanParameterOption {
	return func(p *BooleanParameter) { p.Default = &v }
}

func NewBooleanParameter(name string, desc string, opts ...BooleanParameterOption) *BooleanParameter {
	p := &BooleanParameter{
		CommonParameter: CommonParameter{
			Name:         name,
			Type:         TypeBool,
			Desc:         desc,
			AuthServices: nil,
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

var _ Parameter = &BooleanParameter{}

// BooleanParameter is a parameter representing the "boolean" type.
type BooleanParameter struct {
	CommonParameter `yaml:",inline"`
	Default         *bool `yaml:"default"`
}

func (p *BooleanParameter) Parse(v any) (any, error) {
	newV, ok := v.(bool)
	if !ok {
		return nil, &ParseTypeError{p.Name, p.Type, v}
	}
	if !p.IsAllowedValues(newV) {
		return nil, fmt.Errorf("%t is not an allowed value", newV)
	}
	if p.IsExcludedValues(newV) {
		return nil, fmt.Errorf("%t is an excluded value", newV)
	}
	return newV, nil
}

func (p *BooleanParameter) GetAuthServices() []ParamAuthService {
	return p.AuthServices
}

func (p *BooleanParameter) GetDefault() any {
	if p.Default == nil {
		return nil
	}
	return *p.Default
}

// Manifest returns the manifest for the BooleanParameter.
func (p *BooleanParameter) Manifest() ParameterManifest {
	// only list ParamAuthService names (without fields) in manifest
	authServiceNames := getAuthServiceNames(p.AuthServices)
	r := CheckParamRequired(p.GetRequired(), p.GetDefault())
	return ParameterManifest{
		Name:         p.Name,
		Type:         p.Type,
		Required:     r,
		Description:  p.Desc,
		AuthServices: authServiceNames,
		Default:      p.GetDefault(),
	}
}

// NewArrayParameter is a convenience function for initializing a ArrayParameter.
type ArrayParameterOption func(*ArrayParameter)

func WithArrayRequired(v bool) ArrayParameterOption {
	return func(p *ArrayParameter) { p.Required = &v }
}
func WithArrayAuth(v []ParamAuthService) ArrayParameterOption {
	return func(p *ArrayParameter) { p.AuthServices = v }
}
func WithArrayAllowedValues(v []any) ArrayParameterOption {
	return func(p *ArrayParameter) { p.AllowedValues = v }
}
func WithArrayExcludedValues(v []any) ArrayParameterOption {
	return func(p *ArrayParameter) { p.ExcludedValues = v }
}
func WithArrayDefault(v []any) ArrayParameterOption {
	return func(p *ArrayParameter) { p.Default = &v }
}

func NewArrayParameter(name string, desc string, items Parameter, opts ...ArrayParameterOption) *ArrayParameter {
	p := &ArrayParameter{
		CommonParameter: CommonParameter{
			Name:         name,
			Type:         TypeArray,
			Desc:         desc,
			AuthServices: nil,
		},
		Items: items,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

var _ Parameter = &ArrayParameter{}

// ArrayParameter is a parameter representing the "array" type.
type ArrayParameter struct {
	CommonParameter `yaml:",inline"`
	Default         *[]any    `yaml:"default"`
	Items           Parameter `yaml:"items"`
}

func (p *ArrayParameter) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	var rawItem struct {
		CommonParameter `yaml:",inline"`
		Default         *[]any                  `yaml:"default"`
		Items           util.DelayedUnmarshaler `yaml:"items"`
	}
	if err := unmarshal(&rawItem); err != nil {
		return err
	}
	p.CommonParameter = rawItem.CommonParameter
	p.Default = rawItem.Default
	i, err := parseParamFromDelayedUnmarshaler(ctx, &rawItem.Items)
	if err != nil {
		return fmt.Errorf("unable to parse 'items' field: %w", err)
	}
	if i.GetAuthServices() != nil && len(i.GetAuthServices()) != 0 {
		return fmt.Errorf("nested items should not have auth services")
	}
	p.Items = i

	return nil
}

func (p *ArrayParameter) IsAllowedValues(v []any) bool {
	a := p.GetAllowedValues()
	if len(a) == 0 {
		return true
	}
	for _, av := range a {
		if reflect.DeepEqual(v, av) {
			return true
		}
	}
	return false
}

func (p *ArrayParameter) IsExcludedValues(v []any) bool {
	a := p.GetExcludedValues()
	if len(a) == 0 {
		return false
	}
	for _, av := range a {
		if reflect.DeepEqual(v, av) {
			return true
		}
	}
	return false
}

func (p *ArrayParameter) Parse(v any) (any, error) {
	arrVal, ok := v.([]any)
	if !ok {
		return nil, &ParseTypeError{p.Name, p.Type, v}
	}
	if !p.IsAllowedValues(arrVal) {
		return nil, fmt.Errorf("%s is not an allowed value", arrVal)
	}
	if p.IsExcludedValues(arrVal) {
		return nil, fmt.Errorf("%s is an excluded value", arrVal)
	}
	rtn := make([]any, 0, len(arrVal))
	for idx, val := range arrVal {
		val, err := p.Items.Parse(val)
		if err != nil {
			return nil, fmt.Errorf("unable to parse element #%d: %w", idx, err)
		}
		rtn = append(rtn, val)
	}
	return rtn, nil
}

func (p *ArrayParameter) GetAuthServices() []ParamAuthService {
	return p.AuthServices
}

func (p *ArrayParameter) GetDefault() any {
	if p.Default == nil {
		return nil
	}
	return *p.Default
}

func (p *ArrayParameter) GetItems() Parameter {
	return p.Items
}

// Manifest returns the manifest for the ArrayParameter.
func (p *ArrayParameter) Manifest() ParameterManifest {
	// only list ParamAuthService names (without fields) in manifest
	authServiceNames := getAuthServiceNames(p.AuthServices)
	items := p.Items.Manifest()
	// if required value is true, or there's no default value
	r := CheckParamRequired(p.GetRequired(), p.GetDefault())
	items.Required = r
	return ParameterManifest{
		Name:         p.Name,
		Type:         p.Type,
		Required:     r,
		Description:  p.Desc,
		AuthServices: authServiceNames,
		Items:        &items,
		Default:      p.GetDefault(),
	}
}

// McpManifest returns the MCP manifest for the ArrayParameter.
func (p *ArrayParameter) McpManifest() (ParameterMcpManifest, []string) {
	// only list ParamAuthService names (without fields) in manifest
	authServiceNames := getAuthServiceNames(p.AuthServices)
	items, _ := p.Items.McpManifest()
	return ParameterMcpManifest{
		Type:        p.Type,
		Description: p.Desc,
		Items:       &items,
	}, authServiceNames
}

// MapParameter is a parameter representing a map with string keys. If ValueType is
// specified (e.g., "string"), values are validated against that type. If ValueType
// is empty, it is treated as a generic map[string]any.
type MapParameter struct {
	CommonParameter `yaml:",inline"`
	Default         *map[string]any `yaml:"default,omitempty"`
	ValueType       string          `yaml:"valueType,omitempty"`
}

// Ensure MapParameter implements the Parameter interface.
var _ Parameter = &MapParameter{}

// NewMapParameter is a convenience function for initializing a MapParameter.
type MapParameterOption func(*MapParameter)

func WithMapRequired(v bool) MapParameterOption { return func(p *MapParameter) { p.Required = &v } }
func WithMapAuth(v []ParamAuthService) MapParameterOption {
	return func(p *MapParameter) { p.AuthServices = v }
}
func WithMapAllowedValues(v []any) MapParameterOption {
	return func(p *MapParameter) { p.AllowedValues = v }
}
func WithMapExcludedValues(v []any) MapParameterOption {
	return func(p *MapParameter) { p.ExcludedValues = v }
}
func WithMapDefault(v map[string]any) MapParameterOption {
	return func(p *MapParameter) { p.Default = &v }
}

func NewMapParameter(name string, desc string, valueType string, opts ...MapParameterOption) *MapParameter {
	p := &MapParameter{
		CommonParameter: CommonParameter{
			Name: name,
			Type: "map",
			Desc: desc,
		},
		ValueType: valueType,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// UnmarshalYAML handles parsing the MapParameter from YAML input.
func (p *MapParameter) UnmarshalYAML(ctx context.Context, unmarshal func(interface{}) error) error {
	var rawItem struct {
		CommonParameter `yaml:",inline"`
		Default         *map[string]any `yaml:"default"`
		ValueType       string          `yaml:"valueType"`
	}
	if err := unmarshal(&rawItem); err != nil {
		return err
	}

	// Validate `ValueType` to be one of the supported basic types
	if rawItem.ValueType != "" {
		if _, err := getPrototypeParameter(rawItem.ValueType); err != nil {
			return err
		}
	}

	p.CommonParameter = rawItem.CommonParameter
	p.Default = rawItem.Default
	p.ValueType = rawItem.ValueType
	return nil
}

// getPrototypeParameter is a helper factory to create a temporary parameter
// based on a type string for parsing and manifest generation.
func getPrototypeParameter(typeName string) (Parameter, error) {
	switch typeName {
	case "string":
		return NewStringParameter("", ""), nil
	case "integer":
		return NewIntParameter("", ""), nil
	case "boolean":
		return NewBooleanParameter("", ""), nil
	case "float":
		return NewFloatParameter("", ""), nil
	default:
		return nil, fmt.Errorf("unsupported valueType %q for map parameter", typeName)
	}
}

func (p *MapParameter) IsAllowedValues(v map[string]any) bool {
	a := p.GetAllowedValues()
	if len(a) == 0 {
		return true
	}
	for _, av := range a {
		if reflect.DeepEqual(v, av) {
			return true
		}
	}
	return false
}

func (p *MapParameter) IsExcludedValues(v map[string]any) bool {
	a := p.GetExcludedValues()
	if len(a) == 0 {
		return false
	}
	for _, av := range a {
		if reflect.DeepEqual(v, av) {
			return true
		}
	}
	return false
}

// Parse validates and parses an incoming value for the map parameter.
func (p *MapParameter) Parse(v any) (any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, &ParseTypeError{p.Name, p.Type, v}
	}
	if !p.IsAllowedValues(m) {
		return nil, fmt.Errorf("%s is not an allowed value", m)
	}
	if p.IsExcludedValues(m) {
		return nil, fmt.Errorf("%s is an excluded value", m)
	}
	// for generic maps, convert json.Numbers to their corresponding types
	if p.ValueType == "" {
		convertedData, err := util.ConvertNumbers(m)
		if err != nil {
			return nil, fmt.Errorf("failed to parse integer or float values in map: %s", err)
		}
		convertedMap, ok := convertedData.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("internal error: ConvertNumbers should return a map, but got type %T", convertedData)
		}
		return convertedMap, nil
	}

	// Otherwise, get a prototype and parse each value in the map.
	prototype, err := getPrototypeParameter(p.ValueType)
	if err != nil {
		return nil, err
	}

	rtn := make(map[string]any, len(m))
	for key, val := range m {
		parsedVal, err := prototype.Parse(val)
		if err != nil {
			return nil, fmt.Errorf("unable to parse value for key %q: %w", key, err)
		}
		rtn[key] = parsedVal
	}
	return rtn, nil
}

func (p *MapParameter) GetAuthServices() []ParamAuthService {
	return p.AuthServices
}

func (p *MapParameter) GetDefault() any {
	if p.Default == nil {
		return nil
	}
	return *p.Default
}

func (p *MapParameter) GetValueType() string {
	return p.ValueType
}

// Manifest returns the manifest for the MapParameter.
func (p *MapParameter) Manifest() ParameterManifest {
	authServiceNames := getAuthServiceNames(p.AuthServices)
	r := CheckParamRequired(p.GetRequired(), p.GetDefault())

	var additionalProperties any
	if p.ValueType != "" {
		_, err := getPrototypeParameter(p.ValueType)
		if err != nil {
			panic(err)
		}
		valueSchema := map[string]any{"type": p.ValueType}
		additionalProperties = valueSchema
	} else {
		// If no valueType is given, allow any properties.
		additionalProperties = true
	}
	var defaultV any
	if p.Default != nil {
		defaultV = *p.Default
	}
	return ParameterManifest{
		Name:                 p.Name,
		Type:                 "object",
		Required:             r,
		Description:          p.Desc,
		AuthServices:         authServiceNames,
		AdditionalProperties: additionalProperties,
		Default:              defaultV,
	}
}

// McpManifest returns the MCP manifest for the MapParameter.
func (p *MapParameter) McpManifest() (ParameterMcpManifest, []string) {
	authServiceNames := getAuthServiceNames(p.AuthServices)
	var additionalProperties any
	if p.ValueType != "" {
		_, err := getPrototypeParameter(p.ValueType)
		if err != nil {
			panic(err)
		}
		valueSchema := map[string]any{"type": p.ValueType}
		additionalProperties = valueSchema
	} else {
		// If no valueType is given, allow any properties.
		additionalProperties = true
	}

	return ParameterMcpManifest{
		Type:                 "object",
		Description:          p.Desc,
		AdditionalProperties: additionalProperties,
	}, authServiceNames
}
