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

package firestorequerycollection

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	firestoreapi "cloud.google.com/go/firestore"
	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	fsUtil "github.com/googleapis/mcp-toolbox/internal/tools/firestore/util"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// Constants for tool configuration
const (
	resourceType    = "firestore-query-collection"
	defaultLimit    = 100
	defaultAnalyze  = false
	maxFilterLength = 100 // Maximum filters to prevent abuse
)

// Parameter keys
const (
	collectionPathKey = "collectionPath"
	filtersKey        = "filters"
	orderByKey        = "orderBy"
	limitKey          = "limit"
	analyzeQueryKey   = "analyzeQuery"
)

// Firestore operators
var validOperators = map[string]bool{
	"<":                  true,
	"<=":                 true,
	">":                  true,
	">=":                 true,
	"==":                 true,
	"!=":                 true,
	"array-contains":     true,
	"array-contains-any": true,
	"in":                 true,
	"not-in":             true,
}

// Error messages
const (
	errMissingCollectionPath = "invalid or missing '%s' parameter"
	errInvalidFilters        = "invalid '%s' parameter; expected an array"
	errFilterNotString       = "filter at index %d is not a string"
	errFilterParseFailed     = "failed to parse filter at index %d: %w"
	errInvalidOperator       = "unsupported operator: %s. Valid operators are: %v"
	errMissingFilterValue    = "no value specified for filter on field '%s'"
	errOrderByParseFailed    = "failed to parse orderBy: %w"
	errTooManyFilters        = "too many filters provided: %d (maximum: %d)"
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

// compatibleSource defines the interface for sources that can provide a Firestore client
type compatibleSource interface {
	FirestoreClient() *firestoreapi.Client
	BuildQuery(string, firestoreapi.EntityFilter, []string, string, firestoreapi.Direction, int, bool) (*firestoreapi.Query, error)
	ExecuteQuery(context.Context, *firestoreapi.Query, bool) (any, error)
}

// Config represents the configuration for the Firestore query collection tool
type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

// validate interface
var _ tools.ToolConfig = Config{}

// ToolConfigType returns the type of tool configuration
func (cfg Config) ToolConfigType() string {
	return resourceType
}

// Initialize creates a new Tool instance from the configuration
func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	// Create parameters
	params := createParameters()

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: params.Manifest(), AuthRequired: cfg.AuthRequired},
			params,
		),
	}, nil
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

// createParameters creates the parameter definitions for the tool
func createParameters() parameters.Parameters {
	collectionPathParameter := parameters.NewStringParameter(
		collectionPathKey,
		"The relative path to the Firestore collection to query (e.g., 'users' or 'users/userId/posts'). Note: This is a relative path, NOT an absolute path like 'projects/{project_id}/databases/{database_id}/documents/...'",
	)

	filtersDescription := `Array of filter objects to apply to the query. Each filter is a JSON string with:
- field: The field name to filter on
- op: The operator to use ("<", "<=", ">", ">=", "==", "!=", "array-contains", "array-contains-any", "in", "not-in")
- value: The value to compare against (can be string, number, boolean, or array)
Example: {"field": "age", "op": ">", "value": 18}`

	filtersParameter := parameters.NewArrayParameter(
		filtersKey,
		filtersDescription,
		parameters.NewStringParameter("item", "JSON string representation of a filter object"),
	)

	orderByParameter := parameters.NewStringParameter(
		orderByKey,
		"JSON string specifying the field and direction to order by (e.g., {\"field\": \"name\", \"direction\": \"ASCENDING\"}). Leave empty if not specified",
	)

	limitParameter := parameters.NewIntParameter(
		limitKey,
		"The maximum number of documents to return", parameters.WithIntDefault(
			defaultLimit))

	analyzeQueryParameter := parameters.NewBooleanParameter(
		analyzeQueryKey,
		"If true, returns query explain metrics including execution statistics", parameters.WithBooleanDefault(
			defaultAnalyze))

	return parameters.Parameters{
		collectionPathParameter,
		filtersParameter,
		orderByParameter,
		limitParameter,
		analyzeQueryParameter,
	}
}

// validate interface
var _ tools.Tool = Tool{}

// Tool represents the Firestore query collection tool
type Tool struct {
	tools.BaseTool[Config]
}

// FilterConfig represents a filter for the query
type FilterConfig struct {
	Field string      `json:"field"`
	Op    string      `json:"op"`
	Value interface{} `json:"value"`
}

// Validate checks if the filter configuration is valid
func (f *FilterConfig) Validate() error {
	if f.Field == "" {
		return fmt.Errorf("filter field cannot be empty")
	}

	if !validOperators[f.Op] {
		ops := make([]string, 0, len(validOperators))
		for op := range validOperators {
			ops = append(ops, op)
		}
		return fmt.Errorf(errInvalidOperator, f.Op, ops)
	}

	if f.Value == nil {
		return fmt.Errorf(errMissingFilterValue, f.Field)
	}

	return nil
}

// OrderByConfig represents ordering configuration
type OrderByConfig struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

// GetDirection returns the Firestore direction constant
func (o *OrderByConfig) GetDirection() firestoreapi.Direction {
	if strings.EqualFold(o.Direction, "DESCENDING") {
		return firestoreapi.Desc
	}
	return firestoreapi.Asc
}

// Invoke executes the Firestore query based on the provided parameters
func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	// Parse parameters
	queryParams, err := t.parseQueryParameters(params)
	if err != nil {
		return nil, util.NewAgentError(fmt.Sprintf("failed to parse query parameters: %v", err), err)
	}

	var filter firestoreapi.EntityFilter
	// Apply filters
	if len(queryParams.Filters) > 0 {
		filterConditions := make([]firestoreapi.EntityFilter, 0, len(queryParams.Filters))
		for _, filter := range queryParams.Filters {
			filterConditions = append(filterConditions, firestoreapi.PropertyFilter{
				Path:     filter.Field,
				Operator: filter.Op,
				Value:    filter.Value,
			})
		}

		filter = firestoreapi.AndFilter{
			Filters: filterConditions,
		}
	}

	// prevent panic incase queryParams.OrderBy is nil
	var orderByField string
	var orderByDirection firestoreapi.Direction
	if queryParams.OrderBy != nil {
		orderByField = queryParams.OrderBy.Field
		orderByDirection = queryParams.OrderBy.GetDirection()
	}

	// Build the query
	query, err := source.BuildQuery(queryParams.CollectionPath, filter, nil, orderByField, orderByDirection, queryParams.Limit, queryParams.AnalyzeQuery)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	resp, err := source.ExecuteQuery(ctx, query, queryParams.AnalyzeQuery)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}

// queryParameters holds all parsed query parameters
type queryParameters struct {
	CollectionPath string
	Filters        []FilterConfig
	OrderBy        *OrderByConfig
	Limit          int
	AnalyzeQuery   bool
}

// parseQueryParameters extracts and validates parameters from the input
func (t Tool) parseQueryParameters(params parameters.ParamValues) (*queryParameters, error) {
	mapParams := params.AsMap()

	// Get collection path
	collectionPath, ok := mapParams[collectionPathKey].(string)
	if !ok || collectionPath == "" {
		return nil, fmt.Errorf(errMissingCollectionPath, collectionPathKey)
	}

	// Validate collection path
	if err := fsUtil.ValidateCollectionPath(collectionPath); err != nil {
		return nil, fmt.Errorf("invalid collection path: %w", err)
	}

	result := &queryParameters{
		CollectionPath: collectionPath,
		Limit:          defaultLimit,
		AnalyzeQuery:   defaultAnalyze,
	}

	// Parse filters
	if filtersRaw, ok := mapParams[filtersKey]; ok && filtersRaw != nil {
		filters, err := t.parseFilters(filtersRaw)
		if err != nil {
			return nil, err
		}
		result.Filters = filters
	}

	// Parse orderBy
	if orderByRaw, ok := mapParams[orderByKey]; ok && orderByRaw != nil {
		orderBy, err := t.parseOrderBy(orderByRaw)
		if err != nil {
			return nil, err
		}
		result.OrderBy = orderBy
	}

	// Parse limit
	if limit, ok := mapParams[limitKey].(int); ok {
		result.Limit = limit
	}

	// Parse analyze
	if analyze, ok := mapParams[analyzeQueryKey].(bool); ok {
		result.AnalyzeQuery = analyze
	}

	return result, nil
}

// parseFilters parses and validates filter configurations
func (t Tool) parseFilters(filtersRaw interface{}) ([]FilterConfig, error) {
	filters, ok := filtersRaw.([]any)
	if !ok {
		return nil, fmt.Errorf(errInvalidFilters, filtersKey)
	}

	if len(filters) > maxFilterLength {
		return nil, fmt.Errorf(errTooManyFilters, len(filters), maxFilterLength)
	}

	result := make([]FilterConfig, 0, len(filters))
	for i, filterRaw := range filters {
		filterJSON, ok := filterRaw.(string)
		if !ok {
			return nil, fmt.Errorf(errFilterNotString, i)
		}

		var filter FilterConfig
		if err := json.Unmarshal([]byte(filterJSON), &filter); err != nil {
			return nil, fmt.Errorf(errFilterParseFailed, i, err)
		}

		if err := filter.Validate(); err != nil {
			return nil, fmt.Errorf("filter at index %d is invalid: %w", i, err)
		}

		result = append(result, filter)
	}

	return result, nil
}

// parseOrderBy parses the orderBy configuration
func (t Tool) parseOrderBy(orderByRaw interface{}) (*OrderByConfig, error) {
	orderByJSON, ok := orderByRaw.(string)
	if !ok || orderByJSON == "" {
		return nil, nil
	}

	var orderBy OrderByConfig
	if err := json.Unmarshal([]byte(orderByJSON), &orderBy); err != nil {
		return nil, fmt.Errorf(errOrderByParseFailed, err)
	}

	if orderBy.Field == "" {
		return nil, nil
	}

	return &orderBy, nil
}
