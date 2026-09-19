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

package mssqlgetdatabaseobjectsourcecode

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "mssql-get-database-object-source-code"

type queryPair struct {
	full      string
	namesOnly string
}

var objectTypeQueries = map[string]queryPair{
	"procedures": {
		full: `	SELECT 
		QUOTENAME(SCHEMA_NAME(schema_id)) + '.' + QUOTENAME(name), 0
	FROM 
		sys.procedures`,
		namesOnly: `	SELECT 
		QUOTENAME(SCHEMA_NAME(schema_id)) + '.' + QUOTENAME(name)
	FROM 
		sys.procedures`,
	},
	"views": {
		full: `	SELECT 
		QUOTENAME(SCHEMA_NAME(schema_id)) + '.' + QUOTENAME(name), 0
	FROM 
		sys.views`,
		namesOnly: `	SELECT 
		QUOTENAME(SCHEMA_NAME(schema_id)) + '.' + QUOTENAME(name)
	FROM 
		sys.views`,
	},
	"functions": {
		full: `	SELECT 
		QUOTENAME(SCHEMA_NAME(schema_id)) + '.' + QUOTENAME(name), 0
	FROM 
		sys.objects
	WHERE type IN ('FN', 'IF', 'TF', 'FS', 'FT')`,
		namesOnly: `	SELECT 
		QUOTENAME(SCHEMA_NAME(schema_id)) + '.' + QUOTENAME(name)
	FROM 
		sys.objects
	WHERE type IN ('FN', 'IF', 'TF', 'FS', 'FT')`,
	},
	"triggers": {
		full: `	SELECT 
		QUOTENAME(name), 0
	FROM
		sys.triggers`,
		namesOnly: `	SELECT 
		QUOTENAME(name)
	FROM
		sys.triggers`,
	},
	"linked_servers": {
		full: `	SELECT 
		QUOTENAME(name), 0
	FROM sys.servers 
	WHERE is_linked = 1`,
		namesOnly: `	SELECT 
		QUOTENAME(name)
	FROM sys.servers 
	WHERE is_linked = 1`,
	},
	"tables": {
		full: `	SELECT 
		QUOTENAME(SCHEMA_NAME(schema_id)) + '.' + QUOTENAME(name), 1
	FROM 
		sys.tables`,
		namesOnly: `	SELECT 
		QUOTENAME(SCHEMA_NAME(schema_id)) + '.' + QUOTENAME(name)
	FROM 
		sys.tables`,
	},
}

func getStatement(objectType string, namesOnly bool) (string, error) {
	var selectQueries []string
	order := []string{"procedures", "views", "functions", "triggers", "linked_servers", "tables"}

	if objectType == "all" || objectType == "" {
		for _, ot := range order {
			qp := objectTypeQueries[ot]
			if namesOnly {
				selectQueries = append(selectQueries, qp.namesOnly)
			} else {
				selectQueries = append(selectQueries, qp.full)
			}
		}
	} else if qp, ok := objectTypeQueries[objectType]; ok {
		if namesOnly {
			selectQueries = append(selectQueries, qp.namesOnly)
		} else {
			selectQueries = append(selectQueries, qp.full)
		}
	} else {
		return "", fmt.Errorf("unsupported object_type: %q", objectType)
	}

	unionSelects := strings.Join(selectQueries, "\nUNION ALL\n")

	if namesOnly {
		return unionSelects + ";", nil
	}

	stmt := fmt.Sprintf(`-- Create a temporary table to hold procedure names
CREATE TABLE #SqlResourceList (
    RowID INT IDENTITY(1,1),
    ResourceName NVARCHAR(500),
	ResourceType INT
);

-- Insert selected database objects into the temp table
INSERT INTO #SqlResourceList (ResourceName, ResourceType)
%s;

DECLARE @Counter INT = 1;
DECLARE @TotalCount INT = (SELECT COUNT(*) FROM #SqlResourceList);
DECLARE @CurrentResource NVARCHAR(500);
DECLARE @CurrentResourceType INT

-- Loop through each stored procedure
WHILE @Counter <= @TotalCount
BEGIN
    SELECT @CurrentResource = ResourceName,
		@CurrentResourceType = ResourceType
    FROM #SqlResourceList 
    WHERE RowID = @Counter;

	IF @CurrentResourceType = 0
		BEGIN
			-- Execute sp_helptext for the current stored procedure
			EXEC sp_helptext @CurrentResource;
		END
	ELSE
		BEGIN
			PRINT 'Table - ' + @CurrentResource
			SELECT 
				@CurrentResource as TABLE_NAME,
				COLUMN_NAME, 
				DATA_TYPE, 
				CHARACTER_MAXIMUM_LENGTH AS MAX_LENGTH, 
				IS_NULLABLE 
			FROM INFORMATION_SCHEMA.COLUMNS 
			WHERE TABLE_NAME = @CurrentResource;
			SELECT '-------------------------------------------------------'
		END

    SET @Counter = @Counter + 1;
END;

-- Clean up
DROP TABLE #SqlResourceList;
`, unionSelects)

	return stmt, nil
}

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
	MSSQLDB() *sql.DB
	RunSQL(context.Context, string, []any) (any, error)
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

	namesOnlyParam := parameters.NewBooleanParameter("namesOnly", "Optional: Set to true to return only object names instead of full object source code.", parameters.WithBooleanDefault(false))
	objectTypeParam := parameters.NewStringParameter(
		"object_type",
		"Optional: Specify database object type to retrieve. Allowed values: all, functions, tables, procedures, views, triggers, linked_servers.",
		parameters.WithStringDefault("all"),
		parameters.WithStringAllowedValues([]any{"all", "functions", "tables", "procedures", "views", "triggers", "linked_servers"}),
	)
	allParameters := parameters.Parameters{namesOnlyParam, objectTypeParam}

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

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}

	paramsMap := params.AsMap()
	namesOnly, _ := paramsMap["namesOnly"].(bool)
	objectType, _ := paramsMap["object_type"].(string)

	stmt, err := getStatement(objectType, namesOnly)
	if err != nil {
		return nil, util.NewClientServerError(err.Error(), http.StatusBadRequest, err)
	}

	results, err := source.MSSQLDB().QueryContext(ctx, stmt)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	defer results.Close()

	var allResults []any

	for {
		cols, err := results.Columns()
		if err == nil && len(cols) > 0 {
			rawValues := make([]any, len(cols))
			values := make([]any, len(cols))
			for i := range rawValues {
				values[i] = &rawValues[i]
			}

			for results.Next() {
				if scanErr := results.Scan(values...); scanErr != nil {
					return nil, util.ProcessGeneralError(scanErr)
				}
				row := orderedmap.Row{}
				for i, name := range cols {
					row.Add(name, rawValues[i])
				}
				allResults = append(allResults, row)
			}
		}

		if !results.NextResultSet() {
			break
		}
	}

	if err := results.Err(); err != nil {
		return nil, util.ProcessGeneralError(err)
	}

	return allResults, nil
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
