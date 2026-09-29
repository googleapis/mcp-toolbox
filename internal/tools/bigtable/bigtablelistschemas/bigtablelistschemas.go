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

package bigtablelistschemas

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"cloud.google.com/go/bigtable"
	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "bigtable-list-schemas"

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
	ListTables(context.Context) (any, error)
	GetTable(context.Context, string) (any, error)
	ListLogicalViews(context.Context, string) (any, error)
	ListMaterializedViews(context.Context, string) (any, error)
	InstanceID() string
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
		cfg.Description = "List all Bigtable schemas, including tables with column family definitions, logical views, and materialized views."
	}

	allParameters := parameters.Parameters{
		parameters.NewIntParameter("limit", "Optional: The maximum number of tables to return. Default is 20", parameters.WithIntDefault(20)),
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

func (t Tool) ValidateSource(src sources.Source) error {
	_, ok := src.(compatibleSource)
	if !ok {
		return fmt.Errorf("source is not compatible with the tool")
	}
	return nil
}

func (t Tool) ToConfig() tools.ToolConfig {
	return t.Cfg
}

type TableSchema struct {
	TableName string              `json:"table_name"`
	Info      *bigtable.TableInfo `json:"info,omitempty"`
}

type Column struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

type LogicalViewSchema struct {
	bigtable.LogicalViewInfo
	Columns []Column `json:"columns"`
}

type MaterializedViewSchema struct {
	bigtable.MaterializedViewInfo
	Columns []Column `json:"columns"`
}

type SchemaList struct {
	Tables            []TableSchema            `json:"tables"`
	LogicalViews      []LogicalViewSchema      `json:"logical_views"`
	MaterializedViews []MaterializedViewSchema `json:"materialized_views"`
}

var (
	castRe   = regexp.MustCompile(`(?i)\b(?:SAFE_)?CAST\s*\(\s*.*?\s+AS\s+([a-zA-Z0-9_.]+)\s*\)(?:\s+AS\s+[a-zA-Z0-9_]+)?$`)
	funcRe   = regexp.MustCompile(`(?i)\b([A-Z0-9_]+)\s*\(.*?\)(?:\s+AS\s+[a-zA-Z0-9_]+)?$`)
	selectRe = regexp.MustCompile(`(?is)\bSELECT\s+(.+?)(?:\s+FROM\b|;|$)`)
)

func parseColumnsFromQuery(query string) []Column {
	// Heuristically extracts the output column list from a Bigtable GoogleSQL view query.
	// Matches `SELECT <cols> FROM` or `SELECT <cols>` if no FROM is specified.
	matches := selectRe.FindStringSubmatch(query)
	if len(matches) < 2 {
		return nil
	}

	selectClause := matches[1]

	var columns []Column
	var currentSeg strings.Builder
	depth := 0
	inSingleQuote := false
	inDoubleQuote := false
	var prevCh rune

	for _, ch := range selectClause {
		if ch == '\'' && prevCh != '\\' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
		} else if ch == '"' && prevCh != '\\' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
		}

		if !inSingleQuote && !inDoubleQuote {
			switch ch {
			case '(', '[':
				depth++
			case ')', ']':
				depth--
			}
		}

		if ch == ',' && depth == 0 && !inSingleQuote && !inDoubleQuote {
			columns = append(columns, extractColumn(currentSeg.String()))
			currentSeg.Reset()
		} else {
			currentSeg.WriteRune(ch)
		}
		prevCh = ch
	}
	if currentSeg.Len() > 0 {
		columns = append(columns, extractColumn(currentSeg.String()))
	}

	return columns
}

func extractType(seg string) string {
	seg = strings.TrimSpace(seg)

	// Check for CAST(...) at the outermost level
	if match := castRe.FindStringSubmatch(seg); len(match) > 1 {
		return strings.ToUpper(match[1])
	}

	// Check for other known functions as the outer expression
	if match := funcRe.FindStringSubmatch(seg); len(match) > 1 {
		fn := strings.ToUpper(match[1])
		switch fn {
		case "TO_INT64", "UNIX_MILLIS", "UNIX_MICROS", "UNIX_SECONDS":
			return "INT64"
		case "TO_FLOAT64":
			return "FLOAT64"
		case "TO_FLOAT32":
			return "FLOAT32"
		case "TO_VECTOR32":
			return "VECTOR32"
		case "TO_HEX", "TO_BASE64", "SAFE_CONVERT_BYTES_TO_STRING", "FORMAT_TIMESTAMP", "ARRAY_TO_STRING", "CODE_POINTS_TO_STRING", "TO_JSON_STRING":
			return "STRING"
		case "TIMESTAMP_MILLIS", "TIMESTAMP_MICROS", "TIMESTAMP_SECONDS", "PARSE_TIMESTAMP":
			return "TIMESTAMP"
		case "PARSE_DATE":
			return "DATE"
		case "FROM_BASE64", "FROM_HEX":
			return "BYTES"
		case "MAP_KEYS":
			return "ARRAY<BYTES>"
		case "NULLIF":
			return "" // Unknown usually
		}
	}

	if strings.HasPrefix(seg, "_key") && !strings.Contains(seg, "(") {
		return "BYTES"
	}
	return ""
}

func extractColumn(seg string) Column {
	seg = strings.TrimSpace(seg)
	upperSeg := strings.ToUpper(seg)

	// Find last " AS " that is not inside parentheses
	depth := 0
	asIdx := -1
	for i := len(upperSeg) - 1; i >= 3; i-- {
		if upperSeg[i] == ')' {
			depth++
		} else if upperSeg[i] == '(' {
			depth--
		} else if depth == 0 && upperSeg[i-3:i+1] == " AS " {
			asIdx = i - 3
			break
		}
	}

	t := extractType(seg)
	if asIdx != -1 {
		return Column{Name: strings.TrimSpace(seg[asIdx+4:]), Type: t}
	}

	// Fallback for unaliased columns (e.g. `_key`, `cf['my_col']`, `(CAST(...)).product_name`)
	// We grab the last word-like token optionally surrounded by brackets or quotes
	// as an approximation if no AS is provided.
	parts := strings.FieldsFunc(seg, func(c rune) bool {
		return unicode.IsSpace(c) || c == '.' || c == ')'
	})
	if len(parts) > 0 {
		return Column{Name: parts[len(parts)-1], Type: t}
	}
	return Column{Name: seg, Type: t}
}

func (t Tool) Invoke(ctx context.Context, src sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := src.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}

	schemaList := SchemaList{
		Tables:            []TableSchema{},
		LogicalViews:      []LogicalViewSchema{},
		MaterializedViews: []MaterializedViewSchema{},
	}

	// List tables and get their info
	tablesAny, err := source.ListTables(ctx)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	tableNames, _ := tablesAny.([]string)

	paramsMap := params.AsMap()
	limit := 20
	if val, ok := paramsMap["limit"].(int); ok && val > 0 {
		limit = val
	}
	if len(tableNames) > limit {
		tableNames = tableNames[:limit]
	}

	for _, tableName := range tableNames {
		tableInfoAny, err := source.GetTable(ctx, tableName)
		if err != nil {
			schemaList.Tables = append(schemaList.Tables, TableSchema{TableName: tableName})
		} else {
			if tableInfo, ok := tableInfoAny.(*bigtable.TableInfo); ok {
				schemaList.Tables = append(schemaList.Tables, TableSchema{TableName: tableName, Info: tableInfo})
			} else {
				schemaList.Tables = append(schemaList.Tables, TableSchema{TableName: tableName})
			}
		}
	}

	// Column schemas for SQL views are dynamically resolved by analyzing the SELECT expressions in the view query.

	// List Logical Views
	logicalViewsAny, err := source.ListLogicalViews(ctx, source.InstanceID())
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	if logicalViews, ok := logicalViewsAny.([]bigtable.LogicalViewInfo); ok {
		for _, v := range logicalViews {
			schemaList.LogicalViews = append(schemaList.LogicalViews, LogicalViewSchema{
				LogicalViewInfo: v,
				Columns:         parseColumnsFromQuery(v.Query),
			})
		}
	}

	// List Materialized Views
	materializedViewsAny, err := source.ListMaterializedViews(ctx, source.InstanceID())
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	if materializedViews, ok := materializedViewsAny.([]bigtable.MaterializedViewInfo); ok {
		for _, v := range materializedViews {
			schemaList.MaterializedViews = append(schemaList.MaterializedViews, MaterializedViewSchema{
				MaterializedViewInfo: v,
				Columns:              parseColumnsFromQuery(v.Query),
			})
		}
	}

	return schemaList, nil
}
