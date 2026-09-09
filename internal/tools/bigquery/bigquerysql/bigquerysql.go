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

package bigquerysql

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strconv"

	bigqueryapi "cloud.google.com/go/bigquery"
	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	bigqueryds "github.com/googleapis/mcp-toolbox/internal/sources/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	bqutil "github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerycommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	bigqueryrestapi "google.golang.org/api/bigquery/v2"
)

const resourceType string = "bigquery-sql"

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
	BigQuerySession() bigqueryds.BigQuerySessionProvider
	UseClientAuthorization() bool
	GetAuthTokenHeaderName() string
	GetMaximumBytesBilled() int64
	RetrieveClientAndService(context.Context, tools.AccessToken) (*bigqueryapi.Client, *bigqueryrestapi.Service, error)
	RunSQL(context.Context, *bigqueryapi.Client, string, string, []bigqueryapi.QueryParameter, []*bigqueryapi.ConnectionProperty, map[string]string) (any, error)
}

type Config struct {
	tools.ConfigBase   `yaml:",inline"`
	Type               string                 `yaml:"type" validate:"required"`
	Source             string                 `yaml:"source" validate:"required"`
	Statement          string                 `yaml:"statement" validate:"required"`
	Parameters         parameters.Parameters  `yaml:"parameters"`
	TemplateParameters parameters.Parameters  `yaml:"templateParameters"`
	Annotations        *tools.ToolAnnotations `yaml:"annotations,omitempty"`
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

	allParameters, paramManifest, err := parameters.ProcessParameters(cfg.TemplateParameters, cfg.Parameters)
	if err != nil {
		return nil, err
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: paramManifest, AuthRequired: cfg.AuthRequired},
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
	newStatement, err := parameters.ResolveTemplateParams(t.Cfg.TemplateParameters, t.Cfg.Statement, paramsMap)
	if err != nil {
		return nil, util.NewAgentError("unable to extract template params", err)
	}

	highLevelParams, lowLevelParams, tbErr := buildQueryParameters(t.Cfg.Parameters, paramsMap, newStatement)
	if tbErr != nil {
		return nil, tbErr
	}

	connProps := []*bigqueryapi.ConnectionProperty{}
	if source.BigQuerySession() != nil {
		session, err := source.BigQuerySession()(ctx)
		if err != nil {
			return nil, util.NewClientServerError("failed to get BigQuery session", http.StatusInternalServerError, err)
		}
		if session != nil {
			// Add session ID to the connection properties for subsequent calls.
			connProps = append(connProps, &bigqueryapi.ConnectionProperty{Key: "session_id", Value: session.ID})
		}
	}

	bqClient, restService, err := source.RetrieveClientAndService(ctx, accessToken)
	if err != nil {
		return nil, util.NewClientServerError("failed to retrieve BigQuery client", http.StatusInternalServerError, err)
	}

	dryRunJob, err := bqutil.DryRunQuery(ctx, restService, bqClient.Project(), bqClient.Location, newStatement, lowLevelParams, connProps, source.GetMaximumBytesBilled())
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	statementType := dryRunJob.Statistics.Query.StatementType
	resp, err := source.RunSQL(ctx, bqClient, newStatement, statementType, highLevelParams, connProps, map[string]string{"mcp-toolbox-tool": resourceType})
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}

func buildQueryParameters(paramsMetadata parameters.Parameters, paramsMap map[string]any, statement string) ([]bigqueryapi.QueryParameter, []*bigqueryrestapi.QueryParameter, util.ToolboxError) {
	highLevelParams := make([]bigqueryapi.QueryParameter, 0, len(paramsMetadata))
	lowLevelParams := make([]*bigqueryrestapi.QueryParameter, 0, len(paramsMetadata))

	for _, p := range paramsMetadata {
		name := p.GetName()
		value := paramsMap[name]

		// Handle array types: convert []any to typed slices if necessary.
		if arrayParam, ok := p.(*parameters.ArrayParameter); ok && value != nil {
			if arrayParamValue, ok := value.([]any); ok {
				itemType := arrayParam.GetItems().GetType()
				var err error
				value, err = parameters.ConvertAnySliceToTyped(arrayParamValue, itemType)
				if err != nil {
					return nil, nil, util.NewAgentError(fmt.Sprintf("unable to convert parameter `%s` from []any to typed slice", name), err)
				}
			}
		}

		// Determine if the parameter is named or positional for the high-level client.
		var paramNameForHighLevel string
		isNamed, _ := regexp.MatchString("@"+name+"\\b", statement)
		if isNamed {
			paramNameForHighLevel = name
		}

		// Handle nil values for optional parameters by providing typed NULLs.
		// BigQuery high-level client requires objects like NullString for NULLs.
		// BigQuery low-level REST client requires setting the Null fields.
		finalValue := value
		isNull := value == nil

		if isNull {
			if p.GetEmbeddedBy() != "" {
				finalValue = []float64(nil)
			} else {
				switch p.GetType() {
				case parameters.TypeString:
					finalValue = bigqueryapi.NullString{Valid: false}
				case parameters.TypeInt:
					finalValue = bigqueryapi.NullInt64{Valid: false}
				case parameters.TypeFloat:
					finalValue = bigqueryapi.NullFloat64{Valid: false}
				case parameters.TypeBool:
					finalValue = bigqueryapi.NullBool{Valid: false}
				case parameters.TypeArray:
					// For arrays, provide a typed nil slice based on items type.
					if arrayParam, ok := p.(*parameters.ArrayParameter); ok {
						switch arrayParam.GetItems().GetType() {
						case parameters.TypeString:
							finalValue = []string(nil)
						case parameters.TypeInt:
							finalValue = []int64(nil)
						case parameters.TypeFloat:
							finalValue = []float64(nil)
						case parameters.TypeBool:
							finalValue = []bool(nil)
						default:
							finalValue = []any(nil)
						}
					}
				case parameters.TypeMap:
					finalValue = map[string]any(nil)
				}
			}
		}

		// 1. Create the high-level parameter for the final query execution.
		highLevelParams = append(highLevelParams, bigqueryapi.QueryParameter{
			Name:  paramNameForHighLevel,
			Value: finalValue,
		})

		// 2. Create the low-level parameter for the dry run.
		lowLevelParam := &bigqueryrestapi.QueryParameter{
			Name:           paramNameForHighLevel,
			ParameterType:  &bigqueryrestapi.QueryParameterType{},
			ParameterValue: &bigqueryrestapi.QueryParameterValue{},
		}

		if isNull {
			lowLevelParam.ParameterValue.NullFields = []string{"Value"}
		}

		// Check if this parameter is an array type.
		// It is an array if its metadata type is Array, or if it is used for embedding,
		var isArray bool
		var itemType = "FLOAT64" // Default to FLOAT64 for embeddings
		if arrayParam, ok := p.(*parameters.ArrayParameter); ok {
			isArray = true
			if bqType, err := bqutil.BQTypeStringFromToolType(arrayParam.GetItems().GetType()); err == nil {
				itemType = bqType
			}
		} else if p.GetEmbeddedBy() != "" {
			isArray = true
		}

		if isArray {
			lowLevelParam.ParameterType.Type = "ARRAY"
			lowLevelParam.ParameterType.ArrayType = &bigqueryrestapi.QueryParameterType{Type: itemType}

			if !isNull {
				sliceVal := reflect.ValueOf(value)
				arrayValues := make([]*bigqueryrestapi.QueryParameterValue, sliceVal.Len())
				for i := 0; i < sliceVal.Len(); i++ {
					val := sliceVal.Index(i).Interface()

					// Prevent precision loss and scientific notation issues
					var valStr string
					switch v := val.(type) {
					case float64:
						valStr = strconv.FormatFloat(v, 'f', -1, 64)
					case float32:
						valStr = strconv.FormatFloat(float64(v), 'f', -1, 32)
					default:
						valStr = fmt.Sprintf("%v", val)
					}

					arrayValues[i] = &bigqueryrestapi.QueryParameterValue{
						Value: valStr,
					}
				}
				lowLevelParam.ParameterValue.ArrayValues = arrayValues
			}
		} else {
			bqType, err := bqutil.BQTypeStringFromToolType(p.GetType())
			if err != nil {
				return nil, nil, util.NewAgentError(fmt.Sprintf("unable to get BigQuery type for parameter %q", name), err)
			}
			lowLevelParam.ParameterType.Type = bqType
			if !isNull {
				lowLevelParam.ParameterValue.Value = fmt.Sprintf("%v", value)
			}
		}
		lowLevelParams = append(lowLevelParams, lowLevelParam)
	}
	return highLevelParams, lowLevelParams, nil
}

func formatVectorForBigQuery(vectorFloats []float32) any {
	if len(vectorFloats) == 0 {
		return []float64{}
	}

	res := make([]float64, len(vectorFloats))
	for i, f := range vectorFloats {
		// Convert to float64
		res[i] = float64(f)
	}
	return res
}

func (t Tool) EmbedParams(ctx context.Context, paramValues parameters.ParamValues, pMgr tools.PrimitiveManagerI) (parameters.ParamValues, error) {
	return parameters.EmbedParams(ctx, t.StaticParameters, paramValues, pMgr, formatVectorForBigQuery)
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}

func (t Tool) GetAuthTokenHeaderName(source sources.Source) (string, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return "", fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.GetAuthTokenHeaderName(), nil
}
