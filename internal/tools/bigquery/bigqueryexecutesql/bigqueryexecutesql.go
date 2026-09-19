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

package bigqueryexecutesql

import (
	"context"
	"encoding/json"
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

const resourceType string = "bigquery-execute-sql"

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
	BigQueryClient() *bigqueryapi.Client
	BigQuerySession() bigqueryds.BigQuerySessionProvider
	BigQueryWriteMode() string
	UseClientAuthorization() bool
	GetAuthTokenHeaderName() string
	GetMaximumBytesBilled() int64
	IsDatasetAllowed(projectID, datasetID string) bool
	BigQueryAllowedDatasets() []string
	RetrieveClientAndService(tools.AccessToken) (*bigqueryapi.Client, *bigqueryrestapi.Service, error)
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

	params, err := buildParams("", nil)
	if err != nil {
		return nil, err
	}
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
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
	sql, ok := paramsMap["sql"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast sql parameter %s", paramsMap["sql"]), nil)
	}
	dryRun, ok := paramsMap["dry_run"].(bool)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to cast dry_run parameter %s", paramsMap["dry_run"]), nil)
	}

	bqClient, restService, err := source.RetrieveClientAndService(accessToken)
	if err != nil {
		return nil, util.NewClientServerError("failed to retrieve BigQuery client", http.StatusInternalServerError, err)
	}

	var connProps []*bigqueryapi.ConnectionProperty
	var session *bigqueryds.Session
	if source.BigQueryWriteMode() == bigqueryds.WriteModeProtected {
		session, err = source.BigQuerySession()(ctx)
		if err != nil {
			return nil, util.NewClientServerError("failed to get BigQuery session for protected mode", http.StatusInternalServerError, err)
		}
		connProps = []*bigqueryapi.ConnectionProperty{
			{Key: "session_id", Value: session.ID},
		}
	}

	if len(source.BigQueryAllowedDatasets()) > 0 {
		// Check the allowlist from a static parse of the SQL text before the
		// dry run. A query naming a disallowed dataset that doesn't exist
		// (or the caller can't see) would otherwise have its dry run fail
		// with an opaque BigQuery 404/403 before the clearer allowlist
		// AgentError is ever reached. The dry-run-based check below still
		// runs afterward, since it catches tables a view resolves to that
		// aren't visible in the raw SQL text.
		preDryRunTables, parseErr := bqutil.TableParser(sql, bqClient.Project())
		if parseErr != nil {
			return nil, util.NewAgentError("could not parse tables from query to validate against allowed datasets", parseErr)
		}
		if err := checkDatasetsAllowed(preDryRunTables, source); err != nil {
			return nil, err
		}
	}

	dryRunJob, err := bqutil.DryRunQuery(ctx, restService, bqClient.Project(), bqClient.Location, sql, nil, connProps, source.GetMaximumBytesBilled())
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}

	statementType := dryRunJob.Statistics.Query.StatementType

	switch source.BigQueryWriteMode() {
	case bigqueryds.WriteModeBlocked:
		if statementType != "SELECT" {
			return nil, util.NewAgentError("write mode is 'blocked', only SELECT statements are allowed", nil)
		}
	case bigqueryds.WriteModeProtected:
		if dryRunJob.Configuration != nil && dryRunJob.Configuration.Query != nil {
			if dest := dryRunJob.Configuration.Query.DestinationTable; dest != nil && dest.DatasetId != session.DatasetID {
				return nil, util.NewAgentError(fmt.Sprintf("protected write mode only supports SELECT statements, or write operations in the anonymous "+
					"dataset of a BigQuery session, but destination was %q", dest.DatasetId), nil)
			}
		}
	}

	if len(source.BigQueryAllowedDatasets()) > 0 {
		switch statementType {
		case "CREATE_SCHEMA", "DROP_SCHEMA", "ALTER_SCHEMA":
			return nil, util.NewAgentError(fmt.Sprintf("dataset-level operations like '%s' are not allowed when dataset restrictions are in place", statementType), nil)
		case "CREATE_FUNCTION", "CREATE_TABLE_FUNCTION", "CREATE_PROCEDURE":
			return nil, util.NewAgentError(fmt.Sprintf("creating stored routines ('%s') is not allowed when dataset restrictions are in place, as their contents cannot be safely analyzed", statementType), nil)
		case "CALL":
			return nil, util.NewAgentError(fmt.Sprintf("calling stored procedures ('%s') is not allowed when dataset restrictions are in place, as their contents cannot be safely analyzed", statementType), nil)
		}

		// Use a map to avoid duplicate table names.
		tableIDSet := make(map[string]struct{})

		// Get all tables from the dry run result. This is the most reliable method.
		queryStats := dryRunJob.Statistics.Query
		if queryStats != nil {
			for _, tableRef := range queryStats.ReferencedTables {
				tableIDSet[fmt.Sprintf("%s.%s.%s", tableRef.ProjectId, tableRef.DatasetId, tableRef.TableId)] = struct{}{}
			}
			if tableRef := queryStats.DdlTargetTable; tableRef != nil {
				tableIDSet[fmt.Sprintf("%s.%s.%s", tableRef.ProjectId, tableRef.DatasetId, tableRef.TableId)] = struct{}{}
			}
			if tableRef := queryStats.DdlDestinationTable; tableRef != nil {
				tableIDSet[fmt.Sprintf("%s.%s.%s", tableRef.ProjectId, tableRef.DatasetId, tableRef.TableId)] = struct{}{}
			}
		}

		// Always run the parser to ensure we catch views/tables that the dry run might bypass
		parsedTables, parseErr := bqutil.TableParser(sql, bqClient.Project())
		if parseErr != nil {
			return nil, util.NewAgentError("could not parse tables from query to validate against allowed datasets", parseErr)
		}
		for _, tableID := range parsedTables {
			tableIDSet[tableID] = struct{}{}
		}

		tableIDs := make([]string, 0, len(tableIDSet))
		for tableID := range tableIDSet {
			tableIDs = append(tableIDs, tableID)
		}
		if err := checkDatasetsAllowed(tableIDs, source); err != nil {
			return nil, err
		}
	}

	if dryRun {
		if dryRunJob != nil {
			jobJSON, err := json.MarshalIndent(dryRunJob, "", "  ")
			if err != nil {
				return nil, util.NewClientServerError("failed to marshal dry run job to JSON", http.StatusInternalServerError, err)
			}
			return string(jobJSON), nil
		}
		// This case should not be reached, but as a fallback, we return a message.
		return "Dry run was requested, but no job information was returned.", nil
	}

	// Log the query executed for debugging.
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting logger", http.StatusInternalServerError, err)
	}
	logger.DebugContext(ctx, fmt.Sprintf("executing `%s` tool query: %s", resourceType, sql))
	resp, err := source.RunSQL(ctx, bqClient, sql, statementType, nil, connProps, map[string]string{"mcp-toolbox-tool": resourceType})
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}

// checkDatasetsAllowed returns an AgentError naming the first table ID (in
// "project.dataset.table" form) whose dataset is not in source's configured
// allowlist, or nil if every table is allowed.
func checkDatasetsAllowed(tableIDs []string, source compatibleSource) util.ToolboxError {
	for _, tableID := range tableIDs {
		parts := strings.Split(tableID, ".")
		if len(parts) == 3 {
			projectID, datasetID := parts[0], parts[1]
			if !source.IsDatasetAllowed(projectID, datasetID) {
				return util.NewAgentError(fmt.Sprintf("query accesses dataset '%s.%s', which is not in the allowed list", projectID, datasetID), nil)
			}
		}
	}
	return nil
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

// buildParams builds the tool's parameters from the source's write mode and allowed-dataset
// configuration. Empty writeMode and a nil allow-list yield the plain skeleton.
func buildParams(writeMode string, allowedDatasets []string) (parameters.Parameters, error) {
	var sqlDescriptionBuilder strings.Builder
	switch writeMode {
	case bigqueryds.WriteModeBlocked:
		sqlDescriptionBuilder.WriteString("The SQL to execute. In 'blocked' mode, only SELECT statements are allowed; other statement types will fail.")
	case bigqueryds.WriteModeProtected:
		sqlDescriptionBuilder.WriteString("The SQL to execute. Only SELECT statements and writes to the session's temporary dataset are allowed (e.g., `CREATE TEMP TABLE ...`).")
	default: // WriteModeAllowed
		sqlDescriptionBuilder.WriteString("The SQL to execute.")
	}

	if len(allowedDatasets) > 0 {
		if len(allowedDatasets) == 1 {
			datasetFQN := allowedDatasets[0]
			parts := strings.Split(datasetFQN, ".")
			if len(parts) < 2 {
				return nil, fmt.Errorf("expected allowedDataset to have at least 2 parts (project.dataset): %s", datasetFQN)
			}
			datasetID := parts[1]
			fmt.Fprintf(&sqlDescriptionBuilder, " The query must only access the `%s` dataset. "+
				"To query a table within this dataset (e.g., `my_table`), "+
				"qualify it with the dataset id (e.g., `%s.my_table`).", datasetFQN, datasetID)
		} else {
			datasetIDs := []string{}
			for _, ds := range allowedDatasets {
				datasetIDs = append(datasetIDs, fmt.Sprintf("`%s`", ds))
			}
			fmt.Fprintf(&sqlDescriptionBuilder, " The query must only access datasets from the following list: %s.", strings.Join(datasetIDs, ", "))
		}
	}

	sqlParameter := parameters.NewStringParameter("sql", sqlDescriptionBuilder.String())
	dryRunParameter := parameters.NewBooleanParameter(
		"dry_run",
		"If set to true, the query will be validated and information about the execution will be returned "+
			"without running the query. Defaults to false.", parameters.WithBooleanDefault(
			false))
	return parameters.Parameters{sqlParameter, dryRunParameter}, nil
}

// resolveParams builds the tool's parameters using the source's allowed-dataset configuration.
func (t Tool) resolveParams(source sources.Source) (parameters.Parameters, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return nil, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return buildParams(s.BigQueryWriteMode(), s.BigQueryAllowedDatasets())
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

// GetAnnotations dynamically returns readOnlyHint: true and destructiveHint: false
// when the connected database source is in read-only mode.
func (t Tool) GetAnnotations(src sources.Source) *tools.ToolAnnotations {
	base := t.BaseTool.GetAnnotations(src)
	if src == nil || !src.IsReadOnly() {
		return base
	}

	res := tools.NewReadOnlyAnnotations()
	if base != nil {
		copied := *base
		copied.ReadOnlyHint = res.ReadOnlyHint
		copied.DestructiveHint = res.DestructiveHint
		return &copied
	}
	return res
}
