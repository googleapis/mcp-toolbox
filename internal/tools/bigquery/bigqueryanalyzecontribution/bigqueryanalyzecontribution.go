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

package bigqueryanalyzecontribution

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	bigqueryapi "cloud.google.com/go/bigquery"
	yaml "github.com/goccy/go-yaml"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	bigqueryds "github.com/googleapis/mcp-toolbox/internal/sources/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	bqutil "github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerycommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	bigqueryrestapi "google.golang.org/api/bigquery/v2"
)

const resourceType string = "bigquery-analyze-contribution"

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

// Invoke runs the contribution analysis.
func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramsMap := params.AsMap()
	inputData, ok := paramsMap["input_data"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast input_data parameter %s", paramsMap["input_data"]), nil)
	}

	bqClient, _, err := source.RetrieveClientAndService(ctx, accessToken)
	if err != nil {
		return nil, util.NewClientServerError("failed to retrieve BigQuery client", http.StatusInternalServerError, err)
	}

	modelID := fmt.Sprintf("contribution_analysis_model_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))

	contributionMetric, ok := paramsMap["contribution_metric"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast contribution_metric parameter %v", paramsMap["contribution_metric"]), nil)
	}
	if !bqutil.ValidContributionMetricParam(contributionMetric) {
		return nil, util.NewAgentError("invalid 'contribution_metric': must not contain single quotes", nil)
	}

	isTestCol, ok := paramsMap["is_test_col"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast is_test_col parameter %v", paramsMap["is_test_col"]), nil)
	}
	if !bqutil.ValidColumnParam(isTestCol) {
		return nil, util.NewAgentError(fmt.Sprintf("invalid column name for 'is_test_col': %q; must match [a-zA-Z_][a-zA-Z0-9_]*", isTestCol), nil)
	}

	var options []string
	options = append(options, "MODEL_TYPE = 'CONTRIBUTION_ANALYSIS'")
	options = append(options, fmt.Sprintf("CONTRIBUTION_METRIC = %s", contributionMetric))
	options = append(options, fmt.Sprintf("IS_TEST_COL = %s", isTestCol))

	if val, ok := paramsMap["dimension_id_cols"]; ok {
		if cols, ok := val.([]any); ok {
			var strCols []string
			for _, c := range cols {
				colStr, ok := c.(string)
				if !ok {
					return nil, util.NewAgentError(fmt.Sprintf("dimension_id_cols contains non-string value: %v", c), nil)
				}
				if !bqutil.ValidColumnParam(colStr) {
					return nil, util.NewAgentError(fmt.Sprintf("invalid column name in 'dimension_id_cols': %q; must match [a-zA-Z_][a-zA-Z0-9_]*", colStr), nil)
				}
				strCols = append(strCols, colStr)
			}
			options = append(options, fmt.Sprintf("DIMENSION_ID_COLS = [%s]", strings.Join(strCols, ", ")))
		} else {
			return nil, util.NewAgentError(fmt.Sprintf("unable to cast dimension_id_cols parameter %s", paramsMap["dimension_id_cols"]), nil)
		}
	}
	if val, ok := paramsMap["top_k_insights_by_apriori_support"]; ok {
		options = append(options, fmt.Sprintf("TOP_K_INSIGHTS_BY_APRIORI_SUPPORT = %v", val))
	}
	if val, ok := paramsMap["pruning_method"].(string); ok {
		upperVal := strings.ToUpper(val)
		if upperVal != "NO_PRUNING" && upperVal != "PRUNE_REDUNDANT_INSIGHTS" {
			return nil, util.NewAgentError(fmt.Sprintf("invalid pruning_method: %s", val), nil)
		}
		options = append(options, fmt.Sprintf("PRUNING_METHOD = '%s'", upperVal))
	}

	var inputDataSource string
	trimmedUpperInputData := strings.TrimSpace(strings.ToUpper(inputData))
	if strings.HasPrefix(trimmedUpperInputData, "SELECT") || strings.HasPrefix(trimmedUpperInputData, "WITH") {
		inputDataSource = fmt.Sprintf("(%s)", inputData)
	} else {
		if !bqutil.ValidTableID(inputData) {
			return nil, util.NewAgentError(fmt.Sprintf("invalid table identifier for 'input_data': %q; expected 'dataset.table' or 'project.dataset.table'", inputData), nil)
		}
		if len(source.BigQueryAllowedDatasets()) > 0 {
			parts := strings.Split(inputData, ".")
			var projectID, datasetID string
			switch len(parts) {
			case 3: // project.dataset.table
				projectID, datasetID = parts[0], parts[1]
			case 2: // dataset.table
				projectID, datasetID = bqClient.Project(), parts[0]
			default:
				return nil, util.NewAgentError(fmt.Sprintf("invalid table ID format for 'input_data': %q. Expected 'dataset.table' or 'project.dataset.table'", inputData), nil)
			}
			if !source.IsDatasetAllowed(projectID, datasetID) {
				return nil, util.NewAgentError(fmt.Sprintf("access to dataset '%s.%s' (from table '%s') is not allowed", projectID, datasetID, inputData), nil)
			}
		}
		inputDataSource = fmt.Sprintf("SELECT * FROM `%s`", inputData)
	}

	// Use temp model to skip the clean up at the end. To use TEMP MODEL, queries have to be
	// in the same BigQuery session.
	createModelSQL := fmt.Sprintf("CREATE TEMP MODEL %s OPTIONS(%s) AS %s",
		modelID,
		strings.Join(options, ", "),
		inputDataSource,
	)

	createModelQuery := bqClient.Query(createModelSQL)
	createModelQuery.Labels = map[string]string{"mcp-toolbox-tool": resourceType}

	// Get session from provider if in protected mode.
	// Otherwise, a new session will be created by the first query.
	session, err := source.BigQuerySession()(ctx)
	if err != nil {
		return nil, util.NewClientServerError("failed to get BigQuery session", http.StatusInternalServerError, err)
	}

	if session != nil {
		createModelQuery.ConnectionProperties = []*bigqueryapi.ConnectionProperty{
			{Key: "session_id", Value: session.ID},
		}
	} else {
		// If not in protected mode, create a session for this invocation.
		createModelQuery.CreateSession = true
	}
	if len(source.BigQueryAllowedDatasets()) > 0 {
		createModelQuery.DryRun = true
		dryRunJob, err := createModelQuery.Run(ctx)
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
			return nil, util.NewAgentError("could not dry run model creation query to validate allowed datasets", nil)
		}
		createModelQuery.DryRun = false
	}

	createModelJob, err := createModelQuery.Run(ctx)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	status, err := createModelJob.Wait(ctx)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	if err := status.Err(); err != nil {
		return nil, util.ProcessGcpError(err)
	}

	// Determine the session ID to use for subsequent queries.
	// It's either from the pre-existing session (protected mode) or the one just created.
	var sessionID string
	if session != nil {
		sessionID = session.ID
	} else if status.Statistics != nil && status.Statistics.SessionInfo != nil {
		sessionID = status.Statistics.SessionInfo.SessionID
	} else {
		return nil, util.NewClientServerError("failed to get or create a BigQuery session ID", http.StatusInternalServerError, nil)
	}

	getInsightsSQL := fmt.Sprintf("SELECT * FROM ML.GET_INSIGHTS(MODEL %s)", modelID)
	connProps := []*bigqueryapi.ConnectionProperty{{Key: "session_id", Value: sessionID}}

	resp, err := source.RunSQL(ctx, bqClient, getInsightsSQL, "SELECT", nil, connProps, map[string]string{"mcp-toolbox-tool": resourceType})
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
	inputDataDescription := "The data that contain the test and control data to analyze. Can be a fully qualified BigQuery table ID or a SQL query."
	if len(allowedDatasets) > 0 {
		datasetIDs := []string{}
		for _, ds := range allowedDatasets {
			datasetIDs = append(datasetIDs, fmt.Sprintf("`%s`", ds))
		}
		inputDataDescription += fmt.Sprintf(" The query or table must only access datasets from the following list: %s.", strings.Join(datasetIDs, ", "))
	}

	inputDataParameter := parameters.NewStringParameter("input_data", inputDataDescription)
	contributionMetricParameter := parameters.NewStringParameter("contribution_metric", `The name of the column that contains the metric to analyze.
		Provides the expression to use to calculate the metric you are analyzing.
		To calculate a summable metric, the expression must be in the form SUM(metric_column_name),
		where metric_column_name is a numeric data type.
		To calculate a summable ratio metric, the expression must be in the form
		SUM(numerator_metric_column_name)/SUM(denominator_metric_column_name),
		where numerator_metric_column_name and denominator_metric_column_name are numeric data types.
		To calculate a summable by category metric, the expression must be in the form
		SUM(metric_sum_column_name)/COUNT(DISTINCT categorical_column_name). The summed column must be a numeric data type.
		The categorical column must have type BOOL, DATE, DATETIME, TIME, TIMESTAMP, STRING, or INT64.`, parameters.WithStringEscape(
		"single-quotes"))
	isTestColParameter := parameters.NewStringParameter("is_test_col", "The name of the column that identifies whether a row is in the test or control group.", parameters.WithStringEscape(
		"single-quotes"))
	dimensionIDColsParameter := parameters.NewArrayParameter("dimension_id_cols", "An array of column names that uniquely identify each dimension.", parameters.NewStringParameter("dimension_id_col", "A dimension column name.", parameters.WithStringEscape("single-quotes")), parameters.WithArrayRequired(
		false))
	topKInsightsParameter := parameters.NewIntParameter("top_k_insights_by_apriori_support", "The number of top insights to return, ranked by apriori support.", parameters.WithIntDefault(30))
	pruningMethodParameter := parameters.NewStringParameter("pruning_method", "The method to use for pruning redundant insights. Can be 'NO_PRUNING' or 'PRUNE_REDUNDANT_INSIGHTS'.", parameters.WithStringDefault("PRUNE_REDUNDANT_INSIGHTS"))

	return parameters.Parameters{
		inputDataParameter,
		contributionMetricParameter,
		isTestColParameter,
		dimensionIDColsParameter,
		topKInsightsParameter,
		pruningMethodParameter,
	}
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
