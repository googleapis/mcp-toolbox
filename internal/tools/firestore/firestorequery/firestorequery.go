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

package firestorequery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
	resourceType = "firestore-query"
	defaultLimit = 100
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

// Config represents the configuration for the Firestore query tool
type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`

	// Template fields
	CollectionPath string         `yaml:"collectionPath" validate:"required"`
	Filters        string         `yaml:"filters"`      // JSON string template
	Select         []string       `yaml:"select"`       // Fields to select
	OrderBy        map[string]any `yaml:"orderBy"`      // Order by configuration
	Limit          string         `yaml:"limit"`        // Limit template (can be a number or template)
	AnalyzeQuery   bool           `yaml:"analyzeQuery"` // Analyze query (boolean, not parameterizable)

	// Parameters for template substitution
	Parameters parameters.Parameters `yaml:"parameters"`
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

	// Set default limit if not specified
	if cfg.Limit == "" {
		cfg.Limit = fmt.Sprintf("%d", defaultLimit)
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: cfg.Parameters.Manifest(), AuthRequired: cfg.AuthRequired},
			cfg.Parameters,
		),
	}, nil
}

// validate interface
var _ tools.Tool = Tool{}

// Tool represents the Firestore query tool
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

// OrderByConfig represents ordering configuration
type OrderByConfig struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

// GetDirection returns the Firestore direction constant
func (o *OrderByConfig) GetDirection() firestoreapi.Direction {
	if strings.EqualFold(o.Direction, "DESCENDING") || strings.EqualFold(o.Direction, "DESC") {
		return firestoreapi.Desc
	}
	return firestoreapi.Asc
}

// SimplifiedFilter represents the simplified filter format
type SimplifiedFilter struct {
	And   []SimplifiedFilter `json:"and,omitempty"`
	Or    []SimplifiedFilter `json:"or,omitempty"`
	Field string             `json:"field,omitempty"`
	Op    string             `json:"op,omitempty"`
	Value interface{}        `json:"value,omitempty"`
}

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

// Invoke executes the Firestore query based on the provided parameters
func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramsMap := params.AsMap()
	// Process collection path with template substitution
	collectionPath, err := parameters.PopulateTemplate("collectionPath", t.Cfg.CollectionPath, paramsMap)
	if err != nil {
		return nil, util.NewAgentError(fmt.Sprintf("failed to process collection path: %v", err), err)
	}

	var filter firestoreapi.EntityFilter
	// Process and apply filters if template is provided
	if t.Cfg.Filters != "" {
		simplifiedFilter, filterErr := parseAndPopulateFilters(t.Cfg.Filters, paramsMap)
		if filterErr != nil {
			return nil, filterErr
		}

		// Convert simplified filter to Firestore filter
		filter = t.convertToFirestoreFilter(source, simplifiedFilter)
	}
	// Process and apply ordering
	orderBy, err := t.getOrderBy(paramsMap)
	if err != nil {
		return nil, util.NewAgentError(fmt.Sprintf("failed to process order by: %v", err), err)
	}
	// Process select fields
	selectFields, err := t.processSelectFields(paramsMap)
	if err != nil {
		return nil, util.NewAgentError(fmt.Sprintf("failed to process select fields: %v", err), err)
	}
	// Process and apply limit
	limit, err := t.getLimit(paramsMap)
	if err != nil {
		return nil, util.NewAgentError(fmt.Sprintf("failed to process limit: %v", err), err)
	}

	// prevent panic when accessing orderBy incase it is nil
	var orderByField string
	var orderByDirection firestoreapi.Direction
	if orderBy != nil {
		orderByField = orderBy.Field
		orderByDirection = orderBy.GetDirection()
	}

	// Build the query
	query, err := source.BuildQuery(collectionPath, filter, selectFields, orderByField, orderByDirection, limit, t.Cfg.AnalyzeQuery)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	// Execute the query and return results
	resp, err := source.ExecuteQuery(ctx, query, t.Cfg.AnalyzeQuery)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}

// convertToFirestoreFilter converts simplified filter format to Firestore EntityFilter
func (t Tool) convertToFirestoreFilter(source compatibleSource, filter SimplifiedFilter) firestoreapi.EntityFilter {
	// Handle AND filters
	if len(filter.And) > 0 {
		filters := make([]firestoreapi.EntityFilter, 0, len(filter.And))
		for _, f := range filter.And {
			if converted := t.convertToFirestoreFilter(source, f); converted != nil {
				filters = append(filters, converted)
			}
		}
		if len(filters) > 0 {
			return firestoreapi.AndFilter{Filters: filters}
		}
		return nil
	}

	// Handle OR filters
	if len(filter.Or) > 0 {
		filters := make([]firestoreapi.EntityFilter, 0, len(filter.Or))
		for _, f := range filter.Or {
			if converted := t.convertToFirestoreFilter(source, f); converted != nil {
				filters = append(filters, converted)
			}
		}
		if len(filters) > 0 {
			return firestoreapi.OrFilter{Filters: filters}
		}
		return nil
	}

	// Handle simple property filter
	if filter.Field != "" && filter.Op != "" && filter.Value != nil {
		if validOperators[filter.Op] {
			// Convert the value using the Firestore native JSON converter
			convertedValue, err := fsUtil.JSONToFirestoreValue(filter.Value, source.FirestoreClient())
			if err != nil {
				// If conversion fails, use the original value
				convertedValue = filter.Value
			}

			return firestoreapi.PropertyFilter{
				Path:     filter.Field,
				Operator: filter.Op,
				Value:    convertedValue,
			}
		}
	}

	return nil
}

// processSelectFields processes the select fields with parameter substitution
func (t Tool) processSelectFields(params map[string]any) ([]string, error) {
	var selectFields []string

	// Process configured select fields with template substitution
	for _, field := range t.Cfg.Select {
		// Check if it's a template
		if strings.Contains(field, "{{") {
			processed, err := parameters.PopulateTemplate("selectField", field, params)
			if err != nil {
				return nil, err
			}
			if processed != "" {
				// The processed field might be an array format [a b c] or a single value
				trimmedProcessed := strings.TrimSpace(processed)

				// Check if it's in array format [a b c]
				if strings.HasPrefix(trimmedProcessed, "[") && strings.HasSuffix(trimmedProcessed, "]") {
					// Remove brackets and split by spaces
					arrayContent := strings.TrimPrefix(trimmedProcessed, "[")
					arrayContent = strings.TrimSuffix(arrayContent, "]")
					fields := strings.Fields(arrayContent) // Fields splits by any whitespace
					for _, f := range fields {
						if f != "" {
							selectFields = append(selectFields, f)
						}
					}
				} else {
					selectFields = append(selectFields, processed)
				}
			}
		} else {
			selectFields = append(selectFields, field)
		}
	}

	return selectFields, nil
}

// getOrderBy processes the orderBy configuration with parameter substitution
func (t Tool) getOrderBy(params map[string]any) (*OrderByConfig, error) {
	if t.Cfg.OrderBy == nil {
		return nil, nil
	}

	orderBy := &OrderByConfig{}

	// Process field
	field, err := t.getOrderByForKey("field", params)
	if err != nil {
		return nil, err
	}
	orderBy.Field = field

	// Process direction
	direction, err := t.getOrderByForKey("direction", params)
	if err != nil {
		return nil, err
	}
	orderBy.Direction = direction

	if orderBy.Field == "" {
		return nil, nil
	}

	return orderBy, nil
}

func (t Tool) getOrderByForKey(key string, params map[string]any) (string, error) {
	value, ok := t.Cfg.OrderBy[key].(string)
	if !ok {
		return "", nil
	}

	processedValue, err := parameters.PopulateTemplate(fmt.Sprintf("orderBy%s", key), value, params)
	if err != nil {
		return "", err
	}

	return processedValue, nil
}

// processLimit processes the limit field with parameter substitution
func (t Tool) getLimit(params map[string]any) (int, error) {
	limit := defaultLimit
	if t.Cfg.Limit != "" {
		processedValue, err := parameters.PopulateTemplate("limit", t.Cfg.Limit, params)
		if err != nil {
			return 0, err
		}

		// Try to parse as integer
		if processedValue != "" {
			parsedLimit, err := strconv.Atoi(processedValue)
			if err != nil {
				return 0, err
			}
			limit = parsedLimit
		}
	}
	return limit, nil
}

// parseAndPopulateFilters parses the filter JSON structure before substituting
// runtime parameter values so parameter content cannot alter the filter tree.
func parseAndPopulateFilters(rawFilters string, paramsMap map[string]any) (SimplifiedFilter, util.ToolboxError) {
	normalizedJSON, placeholders, err := replaceUnquotedFilterTemplates(rawFilters)
	if err != nil {
		return SimplifiedFilter{}, util.NewAgentError(fmt.Sprintf("failed to process filters template: %v", err), err)
	}

	var simplifiedFilter SimplifiedFilter
	if err := json.Unmarshal([]byte(normalizedJSON), &simplifiedFilter); err != nil {
		return SimplifiedFilter{}, util.NewAgentError(fmt.Sprintf("failed to parse filters: %v", err), err)
	}

	if toolErr := populateSimplifiedFilter(&simplifiedFilter, paramsMap, placeholders); toolErr != nil {
		return SimplifiedFilter{}, toolErr
	}
	return simplifiedFilter, nil
}

func replaceUnquotedFilterTemplates(rawFilters string) (string, map[string]string, error) {
	prefix := "__firestore_filter_tmpl_"
	for strings.Contains(rawFilters, prefix) {
		prefix += "_"
	}

	placeholders := make(map[string]string)
	var out strings.Builder
	out.Grow(len(rawFilters))

	inString := false
	escaped := false
	for i := 0; i < len(rawFilters); i++ {
		ch := rawFilters[i]
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			out.WriteByte(ch)
			continue
		}

		if ch == '{' && i+1 < len(rawFilters) && rawFilters[i+1] == '{' {
			end, ok := findTemplateActionEnd(rawFilters, i)
			if !ok {
				return "", nil, fmt.Errorf("unclosed template action in filters")
			}
			token := fmt.Sprintf("%s%d__", prefix, len(placeholders))
			placeholders[token] = rawFilters[i:end]
			out.WriteByte('"')
			out.WriteString(token)
			out.WriteByte('"')
			i = end - 1
			continue
		}

		out.WriteByte(ch)
	}

	return out.String(), placeholders, nil
}

func findTemplateActionEnd(s string, start int) (int, bool) {
	var quote byte
	escaped := false
	for i := start + 2; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if quote == '`' {
				if ch == '`' {
					quote = 0
				}
				continue
			}
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '/' && i+1 < len(s) && s[i+1] == '*' {
			commentEnd := strings.Index(s[i+2:], "*/")
			if commentEnd == -1 {
				return 0, false
			}
			i += 2 + commentEnd + 1
			continue
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			quote = ch
			continue
		}
		if ch == '}' && i+1 < len(s) && s[i+1] == '}' {
			return i + 2, true
		}
	}
	return 0, false
}

func populateSimplifiedFilter(f *SimplifiedFilter, paramsMap map[string]any, placeholders map[string]string) util.ToolboxError {
	for i := range f.And {
		if err := populateSimplifiedFilter(&f.And[i], paramsMap, placeholders); err != nil {
			return err
		}
	}
	for i := range f.Or {
		if err := populateSimplifiedFilter(&f.Or[i], paramsMap, placeholders); err != nil {
			return err
		}
	}

	field, err := populateFilterString(f.Field, paramsMap, placeholders)
	if err != nil {
		return err
	}
	f.Field = field

	op, err := populateFilterString(f.Op, paramsMap, placeholders)
	if err != nil {
		return err
	}
	f.Op = op

	if f.Value != nil {
		val, err := populateFilterValue(f.Value, paramsMap, placeholders)
		if err != nil {
			return err
		}
		f.Value = val
	}
	return nil
}

func populateFilterString(s string, paramsMap map[string]any, placeholders map[string]string) (string, util.ToolboxError) {
	if expr, ok := placeholders[s]; ok {
		rendered, err := parameters.PopulateTemplateWithJSON("filters", expr, paramsMap)
		if err != nil {
			return "", util.NewAgentError(fmt.Sprintf("failed to process filters template: %v", err), err)
		}
		var parsed string
		if err := json.Unmarshal([]byte(rendered), &parsed); err != nil {
			return "", util.NewAgentError(fmt.Sprintf("failed to parse filters: %v", err), err)
		}
		return parsed, nil
	}
	if strings.Contains(s, "{{") {
		rendered, err := parameters.PopulateTemplateWithJSON("filters", s, paramsMap)
		if err != nil {
			return "", util.NewAgentError(fmt.Sprintf("failed to process filters template: %v", err), err)
		}
		return rendered, nil
	}
	return s, nil
}

func populateFilterValue(v any, paramsMap map[string]any, placeholders map[string]string) (any, util.ToolboxError) {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, elem := range val {
			resolvedKey, err := populateFilterString(k, paramsMap, placeholders)
			if err != nil {
				return nil, err
			}
			if _, exists := out[resolvedKey]; exists {
				dupErr := fmt.Errorf("duplicate filter value key %q", resolvedKey)
				return nil, util.NewAgentError(fmt.Sprintf("failed to parse filters: %v", dupErr), dupErr)
			}
			resolvedElem, err := populateFilterValue(elem, paramsMap, placeholders)
			if err != nil {
				return nil, err
			}
			out[resolvedKey] = resolvedElem
		}
		return out, nil
	case []any:
		out := make([]any, len(val))
		for i, elem := range val {
			resolvedElem, err := populateFilterValue(elem, paramsMap, placeholders)
			if err != nil {
				return nil, err
			}
			out[i] = resolvedElem
		}
		return out, nil
	case string:
		if expr, ok := placeholders[val]; ok {
			rendered, err := parameters.PopulateTemplateWithJSON("filters", expr, paramsMap)
			if err != nil {
				return nil, util.NewAgentError(fmt.Sprintf("failed to process filters template: %v", err), err)
			}
			var parsed any
			if err := json.Unmarshal([]byte(rendered), &parsed); err != nil {
				return nil, util.NewAgentError(fmt.Sprintf("failed to parse filters: %v", err), err)
			}
			return parsed, nil
		}
		if strings.Contains(val, "{{") {
			rendered, err := parameters.PopulateTemplateWithJSON("filters", val, paramsMap)
			if err != nil {
				return nil, util.NewAgentError(fmt.Sprintf("failed to process filters template: %v", err), err)
			}
			return rendered, nil
		}
		return val, nil
	default:
		return v, nil
	}
}
