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

package bigqueryforecast

import (
	"context"
	"fmt"
	"net/http"
	"strings"

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

const resourceType string = "bigquery-forecast"

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
	GetMaximumBytesBilled() int64
	IsDatasetAllowed(projectID, datasetID string) bool
	BigQueryAllowedDatasets() []string
	BigQuerySession() bigqueryds.BigQuerySessionProvider
	RetrieveClientAndService(context.Context, tools.AccessToken) (*bigqueryapi.Client, *bigqueryrestapi.Service, error)
	RunSQL(context.Context, *bigqueryapi.Client, string, string, []bigqueryapi.QueryParameter, []*bigqueryapi.ConnectionProperty, map[string]string) (any, error)
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

	// params is the static skeleton (no source at init ⇒ no allowed-dataset restriction).
	// Manifest/GetParameters re-resolve against the source lazily.
	params := buildParams(nil)
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
	paramsMap := params.AsMap()
	historyData, ok := paramsMap["history_data"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast history_data parameter %v", paramsMap["history_data"]), nil)
	}
	timestampCol, ok := paramsMap["timestamp_col"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast timestamp_col parameter %v", paramsMap["timestamp_col"]), nil)
	}
	dataCol, ok := paramsMap["data_col"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast data_col parameter %v", paramsMap["data_col"]), nil)
	}
	idColsRaw, ok := paramsMap["id_cols"].([]any)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast id_cols parameter %v", paramsMap["id_cols"]), nil)
	}
	var idCols []string
	for _, v := range idColsRaw {
		s, ok := v.(string)
		if !ok {
			return nil, util.NewAgentError(fmt.Sprintf("id_cols contains non-string value: %v", v), nil)
		}
		idCols = append(idCols, s)
	}
	horizon, ok := paramsMap["horizon"].(int)
	if !ok {
		if h, ok := paramsMap["horizon"].(float64); ok {
			horizon = int(h)
		} else {
			return nil, util.NewAgentError(fmt.Sprintf("unable to cast horizon parameter %v", paramsMap["horizon"]), nil)
		}
	}

	if !bqutil.ValidColumnParam(dataCol) {
		return nil, util.NewAgentError(fmt.Sprintf("invalid column name for 'data_col': %q; must match [a-zA-Z_][a-zA-Z0-9_]*", dataCol), nil)
	}
	if !bqutil.ValidColumnParam(timestampCol) {
		return nil, util.NewAgentError(fmt.Sprintf("invalid column name for 'timestamp_col': %q; must match [a-zA-Z_][a-zA-Z0-9_]*", timestampCol), nil)
	}
	for _, col := range idCols {
		if !bqutil.ValidColumnParam(col) {
			return nil, util.NewAgentError(fmt.Sprintf("invalid column name in 'id_cols': %q; must match [a-zA-Z_][a-zA-Z0-9_]*", col), nil)
		}
	}

	bqClient, _, err := source.RetrieveClientAndService(ctx, accessToken)
	if err != nil {
		return nil, util.NewClientServerError("failed to retrieve BigQuery client", http.StatusInternalServerError, err)
	}

	var historyDataSource string
	trimmedUpperHistoryData := strings.TrimSpace(strings.ToUpper(historyData))
	if strings.HasPrefix(trimmedUpperHistoryData, "SELECT") || strings.HasPrefix(trimmedUpperHistoryData, "WITH") {
		historyDataSource = fmt.Sprintf("(%s)", historyData)
	} else {
		if !bqutil.ValidTableID(historyData) {
			return nil, util.NewAgentError(fmt.Sprintf("invalid table identifier for 'history_data': %q; expected 'dataset.table' or 'project.dataset.table'", historyData), nil)
		}
		if len(source.BigQueryAllowedDatasets()) > 0 {
			parts := strings.Split(historyData, ".")
			var projectID, datasetID string

			switch len(parts) {
			case 3: // project.dataset.table
				projectID = parts[0]
				datasetID = parts[1]
			case 2: // dataset.table
				projectID = bqClient.Project()
				datasetID = parts[0]
			default:
				return nil, util.NewAgentError(fmt.Sprintf("invalid table ID format for 'history_data': %q. Expected 'dataset.table' or 'project.dataset.table'", historyData), nil)
			}

			if !source.IsDatasetAllowed(projectID, datasetID) {
				return nil, util.NewAgentError(fmt.Sprintf("access to dataset '%s.%s' (from table '%s') is not allowed", projectID, datasetID, historyData), nil)
			}
		}
		historyDataSource = fmt.Sprintf("TABLE `%s`", historyData)
	}

	idColsArg := ""
	if len(idCols) > 0 {
		idColsFormatted := fmt.Sprintf("[%s]", strings.Join(idCols, ", "))
		idColsArg = fmt.Sprintf(", id_cols => %s", idColsFormatted)
	}
	sql := fmt.Sprintf(`SELECT * 
		FROM AI.FORECAST(
            %s,
            data_col => %s,
            timestamp_col => %s,
            horizon => %d%s)`,
		historyDataSource, dataCol, timestampCol, horizon, idColsArg)

	session, err := source.BigQuerySession()(ctx)
	if err != nil {
		return nil, util.NewClientServerError("failed to get BigQuery session", http.StatusInternalServerError, err)
	}
	var connProps []*bigqueryapi.ConnectionProperty
	if session != nil {
		// Add session ID to the connection properties for subsequent calls.
		connProps = []*bigqueryapi.ConnectionProperty{
			{Key: "session_id", Value: session.ID},
		}
	}

	if len(source.BigQueryAllowedDatasets()) > 0 {
		dryRunQuery := bqClient.Query(sql)
		dryRunQuery.Location = bqClient.Location
		if connProps != nil {
			dryRunQuery.ConnectionProperties = connProps
		}
		dryRunQuery.DryRun = true
		dryRunJob, err := dryRunQuery.Run(ctx)
		if err != nil {
			return nil, util.ProcessGcpError(err)
		}
		status := dryRunJob.LastStatus()
		if status.Statistics != nil {
			if qStats, ok := status.Statistics.Details.(*bigqueryapi.QueryStatistics); ok {
				for _, tableRef := range qStats.ReferencedTables {
					if !source.IsDatasetAllowed(tableRef.ProjectID, tableRef.DatasetID) {
						return nil, util.NewAgentError(fmt.Sprintf("query accesses dataset '%s.%s', which is not in the allowed list", tableRef.ProjectID, tableRef.DatasetID), nil)
					}
				}
			} else {
				return nil, util.NewAgentError("could not get query statistics details during dry run validation", nil)
			}
		} else {
			return nil, util.NewAgentError("could not dry run final query to validate allowed datasets", nil)
		}
	}

	// Log the query executed for debugging.
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting logger", http.StatusInternalServerError, err)
	}
	logger.DebugContext(ctx, fmt.Sprintf("executing `%s` tool query: %s", resourceType, sql))

	resp, err := source.RunSQL(ctx, bqClient, sql, "SELECT", nil, connProps, map[string]string{"mcp-toolbox-tool": resourceType})
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

func (t Tool) GetAuthTokenHeaderName(source sources.Source) (string, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return "", fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.GetAuthTokenHeaderName(), nil
}

// resolveParams builds the tool's parameters using the source's allowed-dataset configuration.
func buildParams(allowedDatasets []string) parameters.Parameters {
	historyDataDescription := "The table id or the query of the history time series data."
	if len(allowedDatasets) > 0 {
		datasetIDs := []string{}
		for _, ds := range allowedDatasets {
			datasetIDs = append(datasetIDs, fmt.Sprintf("`%s`", ds))
		}
		historyDataDescription += fmt.Sprintf(" The query or table must only access datasets from the following list: %s.", strings.Join(datasetIDs, ", "))
	}

	historyDataParameter := parameters.NewStringParameter("history_data", historyDataDescription)
	timestampColumnNameParameter := parameters.NewStringParameter("timestamp_col", "The name of the time series timestamp column.", parameters.WithStringEscape(
		"single-quotes"))
	dataColumnNameParameter := parameters.NewStringParameter("data_col", "The name of the time series data column.", parameters.WithStringEscape(
		"single-quotes"))
	idColumnNameParameter := parameters.NewArrayParameter("id_cols", "An array of the time series id column names.", parameters.NewStringParameter("id_col", "The name of time series id column.", parameters.WithStringEscape("single-quotes")), parameters.WithArrayDefault([]any{}))
	horizonParameter := parameters.NewIntParameter("horizon", "The number of forecasting steps.", parameters.WithIntDefault(10))
	return parameters.Parameters{historyDataParameter,
		timestampColumnNameParameter, dataColumnNameParameter, idColumnNameParameter, horizonParameter}
}

func (t Tool) resolveParams(source sources.Source) (parameters.Parameters, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return nil, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return buildParams(s.BigQueryAllowedDatasets()), nil
}

// GetParameters returns the tool's parameters, resolved against the source.
func (t Tool) GetParameters(source sources.Source) (parameters.Parameters, error) {
	return t.resolveParams(source)
}

// Manifest returns the tool's manifest, resolved against the source.
func (t Tool) Manifest(source sources.Source) (tools.Manifest, error) {
	params, err := t.resolveParams(source)
	if err != nil {
		return tools.Manifest{}, err
	}
	return tools.Manifest{Description: t.Cfg.Description, Parameters: params.Manifest(), AuthRequired: t.Cfg.AuthRequired}, nil
}
