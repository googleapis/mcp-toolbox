// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package lookercreateviewfromtable

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"

	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

const resourceType string = "looker-create-view-from-table"

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
	UseClientAuthorization() bool
	GetAuthTokenHeaderName() string
	LookerApiSettings(context.Context) (*rtl.ApiSettings, error)
	GetLookerSDK(context.Context, string) (*v4.LookerSDK, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
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

	projectIdParameter := parameters.NewStringParameter("project_id", "The id of the project to create the view in.")
	connectionParameter := parameters.NewStringParameter("connection", "The database connection name.")

	tableDef := parameters.NewMapParameter("table", "Table definition.", "")
	tablesParameter := parameters.NewArrayParameter("tables", `The tables to generate views for.
		Each item must be a map with:
		- schema (string, required)
		- table_name (string, required)
		- primary_key (string, optional)
		- base_view (boolean, optional)
		- columns (array of objects, optional): Each object must have 'column_name' (string).`, tableDef)

	folderNameParameter := parameters.NewStringParameter("folder_name", "The folder to place the view files in (e.g., 'views').", parameters.WithStringDefault("views"))

	params := parameters.Parameters{projectIdParameter, connectionParameter, tablesParameter, folderNameParameter}

	// finish tool setup
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			lookercommon.WriteAnnotations(cfg.Annotations),
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
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError(fmt.Sprintf("error getting logger from context: %s", err), http.StatusInternalServerError, err)
	}

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError(fmt.Sprintf("error getting sdk: %v", err), http.StatusInternalServerError, err)
	}

	apiSettings, err := source.LookerApiSettings(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting api settings", http.StatusInternalServerError, err)
	}

	mapParams := params.AsMap()
	projectId, ok := mapParams["project_id"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("'project_id' must be a string, got %T", mapParams["project_id"]), nil)
	}
	connection, ok := mapParams["connection"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("'connection' must be a string, got %T", mapParams["connection"]), nil)
	}
	folderName, ok := mapParams["folder_name"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("'folder_name' must be a string, got %T", mapParams["folder_name"]), nil)
	}

	tablesSlice, ok := mapParams["tables"].([]any)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("'tables' must be an array, got %T", mapParams["tables"]), nil)
	}

	logger.DebugContext(ctx, "generating views with request", "tables", tablesSlice)

	var generatorTables []lookercommon.ProjectGeneratorTable
	for _, tRaw := range tablesSlice {
		t, ok := tRaw.(map[string]any)
		if !ok {
			return nil, util.NewClientServerError(fmt.Sprintf("expected map in tables list, got %T", tRaw), http.StatusInternalServerError, nil)
		}

		var schema, tableName string
		var primaryKey *string
		var baseView *bool
		var columns []lookercommon.ProjectGeneratorColumn

		if s, ok := t["schema"].(string); ok {
			schema = s
		}
		if tn, ok := t["table_name"].(string); ok {
			tableName = tn
		}
		// Enforce required fields for map input
		if schema == "" || tableName == "" {
			return nil, util.NewClientServerError("schema and table_name are required in table map", http.StatusInternalServerError, nil)
		}

		if pk, ok := t["primary_key"].(string); ok {
			primaryKey = &pk
		}
		if bv, ok := t["base_view"].(bool); ok {
			baseView = &bv
		}
		if colsRaw, ok := t["columns"].([]any); ok {
			for _, cRaw := range colsRaw {
				if cMap, ok := cRaw.(map[string]any); ok {
					if cName, ok := cMap["column_name"].(string); ok {
						columns = append(columns, lookercommon.ProjectGeneratorColumn{ColumnName: cName})
					}
				}
			}
		}

		if tableName == "" {
			continue // Skip invalid entries
		}

		generatorTables = append(generatorTables, lookercommon.ProjectGeneratorTable{
			Schema:     schema,
			TableName:  tableName,
			PrimaryKey: primaryKey,
			BaseView:   baseView,
			Columns:    columns,
		})
	}

	queryParams := lookercommon.ProjectGeneratorQueryParams{
		Connection:          connection,
		FileTypeForExplores: "none",
		FolderName:          folderName,
	}

	reqBody := lookercommon.ProjectGeneratorRequestBody{
		Tables: generatorTables,
	}

	logger.DebugContext(ctx, "generating views with request", "query", queryParams, "body", reqBody)

	err = lookercommon.CreateViewsFromTables(ctx, sdk, projectId, queryParams, reqBody, apiSettings)
	if err != nil {
		if strings.Contains(err.Error(), "status=401") {
			return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
		}
		return nil, util.ProcessGeneralError(err)
	}

	return map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Triggered view generation for project %s in folder %s", projectId, folderName),
	}, nil
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
