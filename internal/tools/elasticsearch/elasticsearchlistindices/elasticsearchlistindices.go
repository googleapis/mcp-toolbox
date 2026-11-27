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

package elasticsearchlistindices

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "elasticsearch-list-indices"

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
	ListIndices(ctx context.Context, patterns []string, columns []string) ([]map[string]any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}
	allParameters := parameters.Parameters{
		parameters.NewStringParameter("patterns", "Optional: One or more index names or wildcard patterns, comma-separated, e.g. 'logs-*,metrics-*'. Defaults to '*' (all indices). Indices whose names start with '.' are only listed when at least one pattern starts with '.'.", parameters.WithStringDefault("*")),
		parameters.NewStringParameter("output_format", "Optional: 'simple' (default) returns index, health, status, docs_count, store_size. 'detailed' also returns pri, rep, docs_deleted, creation_date, uuid.", parameters.WithStringDefault("simple")),
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
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

	pattern, ok := paramsMap["patterns"].(string)
	if !ok {
		return nil, util.NewAgentError("invalid 'patterns' parameter; expected a string", nil)
	}
	patterns := splitPatterns(pattern)
	outputFormat, _ := paramsMap["output_format"].(string)
	cols, catColumns, ok := columnsFor(outputFormat)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("invalid value for output_format: must be 'simple' or 'detailed', but got %q", outputFormat), nil)
	}

	rows, err := source.ListIndices(ctx, patterns, catColumns)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}

	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		name, _ := row["index"].(string)
		if !includeIndex(patterns, name) {
			continue
		}
		out := make(map[string]any, len(cols))
		for _, c := range cols {
			out[c.key] = row[c.cat]
		}
		result = append(result, out)
	}
	return result, nil
}

func splitPatterns(pattern string) []string {
	var patterns []string
	for _, p := range strings.Split(pattern, ",") {
		if p = strings.TrimSpace(p); p != "" {
			patterns = append(patterns, p)
		}
	}
	if len(patterns) == 0 {
		return []string{"*"}
	}
	return patterns
}

type column struct {
	cat string
	key string
}

var simpleColumns = []column{
	{"index", "index"},
	{"health", "health"},
	{"status", "status"},
	{"docs.count", "docs_count"},
	{"store.size", "store_size"},
}

var detailedColumns = slices.Concat(simpleColumns, []column{
	{"pri", "pri"},
	{"rep", "rep"},
	{"docs.deleted", "docs_deleted"},
	{"creation.date.string", "creation_date"},
	{"uuid", "uuid"},
})

var (
	simpleCatColumns   = catNames(simpleColumns)
	detailedCatColumns = catNames(detailedColumns)
)

func catNames(cols []column) []string {
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.cat
	}
	return names
}

func columnsFor(outputFormat string) ([]column, []string, bool) {
	switch outputFormat {
	case "simple":
		return simpleColumns, simpleCatColumns, true
	case "detailed":
		return detailedColumns, detailedCatColumns, true
	}
	return nil, nil, false
}

func includeIndex(patterns []string, name string) bool {
	if !strings.HasPrefix(name, ".") {
		return true
	}
	return slices.ContainsFunc(patterns, func(p string) bool {
		return strings.HasPrefix(p, ".")
	})
}
