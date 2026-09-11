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

package bigquerycommon

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	bigqueryrestapi "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/googleapi"
)

// validBQTableID matches BigQuery table identifiers in 'dataset.table' or
// 'project.dataset.table' form. Components are restricted to letters, digits,
// and underscores — the character set that BigQuery allows for dataset and
// table IDs and that is safe to interpolate inside a backtick-quoted SQL
// identifier.
var validBQTableID = regexp.MustCompile(`^[a-zA-Z0-9_-]+(\.([a-zA-Z0-9_]+)){1,2}$`)

// validBQColumnName matches BigQuery column names: a letter or underscore
// followed by letters, digits, or underscores.
var validBQColumnName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// ValidTableID returns true if s is a safe BigQuery table identifier of the
// form 'dataset.table' or 'project.dataset.table'. Values that fail this check
// must not be interpolated into backtick-quoted SQL.
func ValidTableID(s string) bool {
	return validBQTableID.MatchString(s)
}

// ValidColumnParam returns true if s (stripped of leading/trailing single quotes) is a safe column name.
func ValidColumnParam(s string) bool {
	return ValidColumnName(StripSingleQuotes(s))
}

// ValidContributionMetricParam returns true if s (stripped of leading/trailing single quotes) is a safe contribution metric (does not contain single quotes).
func ValidContributionMetricParam(s string) bool {
	return !strings.ContainsRune(StripSingleQuotes(s), '\'')
}

// StripSingleQuotes removes leading and trailing single quotes from a string if both are present.
func StripSingleQuotes(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

// ValidColumnName returns true if s is a safe BigQuery column name.
// Values that fail this check must not be interpolated as SQL identifiers
// or into single-quoted SQL string arguments that represent column references.
func ValidColumnName(s string) bool {
	return validBQColumnName.MatchString(s)
}

// DryRunQuery performs a dry run of the SQL query to validate it and get metadata.
func DryRunQuery(
	ctx context.Context,
	restService *bigqueryrestapi.Service,
	projectID string,
	location string,
	sql string,
	params []*bigqueryrestapi.QueryParameter,
	connProps []*bigqueryapi.ConnectionProperty,
	maximumBytesBilled int64,
	createSession bool,
) (*bigqueryrestapi.Job, error) {
	useLegacySql := false
	restConnProps := make([]*bigqueryrestapi.ConnectionProperty, len(connProps))
	for i, prop := range connProps {
		restConnProps[i] = &bigqueryrestapi.ConnectionProperty{Key: prop.Key, Value: prop.Value}
	}

	jobToInsert := &bigqueryrestapi.Job{
		JobReference: &bigqueryrestapi.JobReference{
			ProjectId: projectID,
			Location:  location,
		},
		Configuration: &bigqueryrestapi.JobConfiguration{
			DryRun: true,
			Query: &bigqueryrestapi.JobConfigurationQuery{
				Query:                sql,
				UseLegacySql:         &useLegacySql,
				ConnectionProperties: restConnProps,
				QueryParameters:      params,
				MaximumBytesBilled:   maximumBytesBilled,
				CreateSession:        createSession,
			},
		},
	}

	insertResponse, err := restService.Jobs.Insert(projectID, jobToInsert).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to insert dry run job: %w", err)
	}
	return insertResponse, nil
}

// DatasetValidator defines the interface for checking if a dataset is allowed.
type DatasetValidator interface {
	IsDatasetAllowed(projectID, datasetID string) bool
}

// tableReference represents a fully qualified reference to a BigQuery table or routine.
// Using a structured type avoids ambiguous string splitting on dots when project IDs
// contain domain prefixes (e.g. "google.com:project-id").
type tableReference struct {
	ProjectID string
	DatasetID string
	TableID   string
}

// ValidateQueryAgainstAllowedDatasets validates a SQL query against a list of allowed datasets.
//
// Why custom SQL parsing (TableParserDetailed & IsAnyTableExplicitlyReferenced) is needed
// alongside BigQuery dry run statistics:
//
//  1. Supporting Authorized Views:
//     When a query accesses an Authorized View (e.g. `SELECT * FROM allowed_ds.my_view`),
//     BigQuery's dry run resolves the view and includes ALL underlying base tables in
//     Statistics.Query.ReferencedTables, even if they reside in restricted datasets
//     (e.g. `restricted_ds.base_table`). If we relied solely on dry run statistics, queries
//     against legitimate authorized views would be erroneously blocked.
//     To distinguish legitimate authorized view access from unauthorized direct access:
//     - If a restricted dataset appears in dry run statistics, IsAnyTableExplicitlyReferenced
//     performs a lexical scan of the SQL to check whether the restricted table is
//     explicitly referenced by name in the query text.
//     - If the user explicitly typed the restricted table name, the query is blocked.
//     - If the restricted table was NOT named in the query (and all other references are
//     fully qualified), the access is via an authorized view and is granted exemption.
//
//  2. Catching Unanalyzable or Ambiguous Operations:
//     Dry run statistics alone cannot detect every security boundary issue:
//     - Unqualified table references (e.g. `SELECT * FROM my_table`): In the presence of
//     dataset restrictions, table names must be fully qualified (dataset.table) to prevent
//     accidental or ambiguous resolution based on session state or search paths.
//     TableParserDetailed detects unqualified references.
//     - Procedural operations (SET session variables, CALL, etc.) whose dynamic behavior
//     cannot be safely analyzed.
func ValidateQueryAgainstAllowedDatasets(
	ctx context.Context,
	restService *bigqueryrestapi.Service,
	projectID string,
	location string,
	sql string,
	params []*bigqueryrestapi.QueryParameter,
	connProps []*bigqueryapi.ConnectionProperty,
	validator DatasetValidator,
	maximumBytesBilled int64,
	createSession bool,
) (*bigqueryrestapi.Job, util.ToolboxError) {
	dryRunJob, err := DryRunQuery(ctx, restService, projectID, location, sql, params, connProps, maximumBytesBilled, createSession)
	if err != nil {
		var gErr *googleapi.Error
		if errors.As(err, &gErr) {
			return nil, util.ProcessGcpError(err)
		}
		return nil, util.NewAgentError("query validation failed", err)
	}

	if dryRunJob.Statistics == nil || dryRunJob.Statistics.Query == nil {
		return nil, util.NewAgentError("dry run failed to return query statistics", nil)
	}

	queryStats := dryRunJob.Statistics.Query

	// Statement types whose accessed objects cannot be determined statically must be
	// rejected outright. The dry run reports these reliably; the lexical fallback in
	// TableParser does not (e.g. it cannot see CREATE TABLE FUNCTION).
	switch queryStats.StatementType {
	case "CREATE_SCHEMA", "DROP_SCHEMA", "ALTER_SCHEMA":
		return nil, util.NewAgentError(fmt.Sprintf(
			"dataset-level operations like '%s' are not allowed when dataset restrictions are in place",
			queryStats.StatementType), nil)
	case "CREATE_FUNCTION", "CREATE_TABLE_FUNCTION", "CREATE_PROCEDURE":
		return nil, util.NewAgentError(fmt.Sprintf(
			"creating stored routines ('%s') is not allowed when dataset restrictions are in place, as their contents cannot be safely analyzed",
			queryStats.StatementType), nil)
	case "CALL":
		return nil, util.NewAgentError(fmt.Sprintf(
			"calling stored procedures ('%s') is not allowed when dataset restrictions are in place, as their contents cannot be safely analyzed",
			queryStats.StatementType), nil)
	case "SET":
		return nil, util.NewAgentError(
			"session variable assignment ('SET') is not allowed when dataset restrictions are in place, "+
				"as it can change how unqualified table names are resolved", nil)
	}

	// Use a structured map to avoid duplicate table names from the dry run result.
	// Storing typed tableReference objects avoids issues with domain-scoped project IDs (e.g. "google.com:project-id").
	tableRefSet := make(map[tableReference]struct{})
	addTableRef := func(pID, dsID, tID string) {
		if pID != "" && dsID != "" && tID != "" {
			tableRefSet[tableReference{
				ProjectID: pID,
				DatasetID: dsID,
				TableID:   tID,
			}] = struct{}{}
		}
	}
	for _, tableRef := range queryStats.ReferencedTables {
		if tableRef != nil {
			addTableRef(tableRef.ProjectId, tableRef.DatasetId, tableRef.TableId)
		}
	}
	if tableRef := queryStats.DdlTargetTable; tableRef != nil {
		addTableRef(tableRef.ProjectId, tableRef.DatasetId, tableRef.TableId)
	}
	if tableRef := queryStats.DdlDestinationTable; tableRef != nil {
		addTableRef(tableRef.ProjectId, tableRef.DatasetId, tableRef.TableId)
	}
	// DROP FUNCTION / DROP PROCEDURE etc. target a routine, not a table.
	if routineRef := queryStats.DdlTargetRoutine; routineRef != nil {
		addTableRef(routineRef.ProjectId, routineRef.DatasetId, routineRef.RoutineId)
	}

	var violatingTables []string
	var violatingRefs []tableReference
	for ref := range tableRefSet {
		// Skip validation for specific system functions that BigQuery reports as referenced tables.
		if IsSystemResource(ref.DatasetID, ref.TableID) {
			continue
		}
		if !validator.IsDatasetAllowed(ref.ProjectID, ref.DatasetID) {
			violatingTables = append(violatingTables, fmt.Sprintf("%s.%s.%s", ref.ProjectID, ref.DatasetID, ref.TableID))
			violatingRefs = append(violatingRefs, ref)
		}
	}

	parsed, parseErr := TableParserDetailed(sql, projectID)
	if parseErr != nil {
		return nil, util.NewAgentError("could not safely analyze query with dataset restrictions", parseErr)
	}

	// 1) Direct lexically visible violating datasets - unconditionally reject (consistent with main)
	var parsedViolatingDatasets []string
	seenParsedDatasets := make(map[string]struct{})
	for _, tableID := range parsed.TableIDs {
		parts := strings.Split(tableID, ".")
		if len(parts) == 4 && strings.Contains(parts[1], ":") {
			parts = []string{parts[0] + "." + parts[1], parts[2], parts[3]}
		}
		if len(parts) == 3 {
			if IsSystemResource(parts[1], parts[2]) {
				continue
			}
			if !validator.IsDatasetAllowed(parts[0], parts[1]) {
				datasetFQN := fmt.Sprintf("%s.%s", parts[0], parts[1])
				if _, seen := seenParsedDatasets[datasetFQN]; !seen {
					parsedViolatingDatasets = append(parsedViolatingDatasets, datasetFQN)
					seenParsedDatasets[datasetFQN] = struct{}{}
				}
			}
		}
	}
	if len(parsedViolatingDatasets) > 0 {
		return nil, notAllowedDatasetsErr(parsedViolatingDatasets)
	}

	// 2) Dry run violating tables
	if len(violatingTables) > 0 {
		explicitlyReferenced, err := IsAnyTableExplicitlyReferenced(sql, projectID, violatingTables)
		if err != nil {
			return nil, util.NewAgentError("failed to analyze query for explicit table references", err)
		}
		if explicitlyReferenced {
			var violatingDatasets []string
			seenDatasets := make(map[string]struct{})
			for _, ref := range violatingRefs {
				datasetFQN := fmt.Sprintf("%s.%s", ref.ProjectID, ref.DatasetID)
				if _, seen := seenDatasets[datasetFQN]; !seen {
					violatingDatasets = append(violatingDatasets, datasetFQN)
					seenDatasets[datasetFQN] = struct{}{}
				}
			}
			return nil, notAllowedDatasetsErr(violatingDatasets)
		}

		// Only when all table references in the query are explicit and fully-qualified
		// do we consider this an authorized view and grant exemption.
		if len(parsed.UnqualifiedRefs) > 0 {
			return nil, util.NewAgentError(fmt.Sprintf(
				"query references table %q without a dataset qualifier; "+
					"fully qualify all table names (dataset.table) when dataset restrictions are in place",
				parsed.UnqualifiedRefs[0]), nil)
		}
	}

	return dryRunJob, nil
}

func notAllowedDatasetsErr(datasetFQNs []string) util.ToolboxError {
	sort.Strings(datasetFQNs)
	formatted := make([]string, len(datasetFQNs))
	for i, ds := range datasetFQNs {
		formatted[i] = fmt.Sprintf("'%s'", ds)
	}
	plural := ""
	if len(formatted) > 1 {
		plural = "s"
	}
	return util.NewAgentError(fmt.Sprintf("access to dataset%s %s is not allowed", plural, strings.Join(formatted, ", ")), nil)
}

// IsSystemResource checks if a given dataset and table/function ID refer to a BigQuery system resource
// (like a built-in AI or ML function) that should be exempted from dataset restriction checks.
// Note: This function exempts system resources by dataset and resource name prefix (e.g. AI.FORECAST, ML.PREDICT).
// User-created datasets with these identical names will also be exempted; this is a known, acceptable tradeoff.
func IsSystemResource(datasetID, resourceID string) bool {
	datasetID = strings.ToUpper(datasetID)
	resourceID = strings.ToUpper(resourceID)

	if datasetID == "AI" {
		switch resourceID {
		case "FORECAST", "GENERATE_TEXT", "EXTRACT_ENTITY", "SUMMARIZE",
			"GENERATE", "GENERATE_BOOL", "GENERATE_INT", "GENERATE_DOUBLE",
			"GENERATE_TABLE", "IF", "CLASSIFY", "SCORE":
			return true
		}
	}
	if datasetID == "ML" {
		switch resourceID {
		case "GET_INSIGHTS", "EXPLAIN_PREDICT", "GENERATE_TEXT", "DISTANCE", "PREDICT",
			"GENERATE_EMBEDDING", "TRANSLATE", "UNDERSTAND_TEXT", "ANNOTATE_IMAGE", "TRANSCRIBE",
			"PROCESS_DOCUMENT", "DETECT_ANOMALIES", "FORECAST", "EVALUATE", "TRAINING_INFO",
			"FEATURE_INFO", "WEIGHTS", "CENTROIDS", "CONFUSION_MATRIX", "ROC_CURVE",
			"ARIMA_EVALUATE", "ARIMA_COEFFICIENTS", "GLOBAL_EXPLAIN", "RECOMMEND",
			"TRANSFORM", "PRINCIPAL_COMPONENTS", "HOLIDAY":
			return true
		}
	}
	return false
}

// BQTypeStringFromToolType converts a tool parameter type string to a BigQuery standard SQL type string.
func BQTypeStringFromToolType(toolType string) (string, error) {
	switch toolType {
	case parameters.TypeString:
		return "STRING", nil
	case parameters.TypeInt:
		return "INT64", nil
	case parameters.TypeFloat:
		return "FLOAT64", nil
	case parameters.TypeBool:
		return "BOOL", nil
	case parameters.TypeMap:
		return "STRUCT", nil
	default:
		return "", fmt.Errorf("unsupported tool parameter type for BigQuery: %s", toolType)
	}
}

// InitializeDatasetParameters generates project and dataset tool parameters based on allowedDatasets.
func InitializeDatasetParameters(
	allowedDatasets []string,
	defaultProjectID string,
	projectKey, datasetKey string,
	projectDescription, datasetDescription string,
) (projectParam, datasetParam parameters.Parameter) {
	if len(allowedDatasets) > 0 {
		if len(allowedDatasets) == 1 {
			parts := strings.Split(allowedDatasets[0], ".")
			defaultProjectID = parts[0]
			datasetID := parts[1]
			projectDescription += fmt.Sprintf(" Must be `%s`.", defaultProjectID)
			datasetDescription += fmt.Sprintf(" Must be `%s`.", datasetID)
			datasetParam = parameters.NewStringParameter(datasetKey, datasetDescription, parameters.WithStringDefault(datasetID))
		} else {
			datasetIDsByProject := make(map[string][]string)
			for _, ds := range allowedDatasets {
				parts := strings.Split(ds, ".")
				project := parts[0]
				dataset := parts[1]
				datasetIDsByProject[project] = append(datasetIDsByProject[project], fmt.Sprintf("`%s`", dataset))
			}

			var datasetDescriptions, projectIDList []string
			for project, datasets := range datasetIDsByProject {
				sort.Strings(datasets)
				projectIDList = append(projectIDList, fmt.Sprintf("`%s`", project))
				datasetList := strings.Join(datasets, ", ")
				datasetDescriptions = append(datasetDescriptions, fmt.Sprintf("%s from project `%s`", datasetList, project))
			}
			sort.Strings(projectIDList)
			sort.Strings(datasetDescriptions)
			projectDescription += fmt.Sprintf(" Must be one of the following: %s.", strings.Join(projectIDList, ", "))
			datasetDescription += fmt.Sprintf(" Must be one of the allowed datasets: %s.", strings.Join(datasetDescriptions, "; "))
			datasetParam = parameters.NewStringParameter(datasetKey, datasetDescription)
		}
	} else {
		datasetParam = parameters.NewStringParameter(datasetKey, datasetDescription)
	}

	projectParam = parameters.NewStringParameter(projectKey, projectDescription, parameters.WithStringDefault(defaultProjectID))

	return projectParam, datasetParam
}
