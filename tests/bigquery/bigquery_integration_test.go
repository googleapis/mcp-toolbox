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

package bigquery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

var (
	BigquerySourceType = "bigquery"
	BigqueryToolType   = "bigquery-sql"
	BigqueryProject    = os.Getenv("BIGQUERY_PROJECT")
)

func getBigQueryVars(t *testing.T) map[string]any {
	switch "" {
	case BigqueryProject:
		t.Fatal("'BIGQUERY_PROJECT' not set")
	}

	return map[string]any{
		"type":    BigquerySourceType,
		"project": BigqueryProject,
	}
}

// Copied over from bigquery.go
func initBigQueryConnection(project string) (*bigqueryapi.Client, error) {
	ctx := context.Background()
	cred, err := google.FindDefaultCredentials(ctx, bigqueryapi.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials with scope %q: %w", bigqueryapi.Scope, err)
	}

	client, err := bigqueryapi.NewClient(ctx, project, option.WithCredentials(cred))
	if err != nil {
		return nil, fmt.Errorf("failed to create BigQuery client for project %q: %w", project, err)
	}
	return client, nil
}

func TestBigQueryToolEndpoints(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Logf("Starting test with uniqueID: %s", uniqueID)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create Cloud SQL connection pool: %s", err)
	}

	// create table name with UUID
	datasetName := fmt.Sprintf("temp_toolbox_test_%s", uniqueID)
	tableName := fmt.Sprintf("param_table_%s", uniqueID)
	tableNameParam := fmt.Sprintf("`%s.%s.%s`",
		BigqueryProject,
		datasetName,
		tableName,
	)
	tableNameAuth := fmt.Sprintf("`%s.%s.auth_table_%s`",
		BigqueryProject,
		datasetName,
		uniqueID,
	)
	tableNameTemplateParam := fmt.Sprintf("`%s.%s.template_param_table_%s`",
		BigqueryProject,
		datasetName,
		uniqueID,
	)
	tableNameDataType := fmt.Sprintf("`%s.%s.datatype_table_%s`",
		BigqueryProject,
		datasetName,
		uniqueID,
	)
	tableNameForecast := fmt.Sprintf("`%s.%s.forecast_table_%s`",
		BigqueryProject,
		datasetName,
		uniqueID,
	)

	tableNameAnalyzeContribution := fmt.Sprintf("`%s.%s.analyze_contribution_table_%s`",
		BigqueryProject,
		datasetName,
		uniqueID,
	)

	// global cleanup for this test run
	t.Cleanup(func() {
		tests.CleanupBigQueryDatasets(t, context.Background(), client, []string{datasetName})
	})

	// set up data for param tool
	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getBigQueryParamToolInfo(tableNameParam)
	setupBigQueryTable(t, ctx, client, createParamTableStmt, insertParamTableStmt, datasetName, tableNameParam, paramTestParams)

	// set up data for auth tool
	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getBigQueryAuthToolInfo(tableNameAuth)
	setupBigQueryTable(t, ctx, client, createAuthTableStmt, insertAuthTableStmt, datasetName, tableNameAuth, authTestParams)

	// set up data for data type test tool
	createDataTypeTableStmt, insertDataTypeTableStmt, dataTypeToolStmt, arrayDataTypeToolStmt, dataTypeTestParams := getBigQueryDataTypeTestInfo(tableNameDataType)
	setupBigQueryTable(t, ctx, client, createDataTypeTableStmt, insertDataTypeTableStmt, datasetName, tableNameDataType, dataTypeTestParams)

	// set up data for forecast tool
	createForecastTableStmt, insertForecastTableStmt, forecastTestParams := getBigQueryForecastToolInfo(tableNameForecast)
	setupBigQueryTable(t, ctx, client, createForecastTableStmt, insertForecastTableStmt, datasetName, tableNameForecast, forecastTestParams)

	// set up data for analyze contribution tool
	createAnalyzeContributionTableStmt, insertAnalyzeContributionTableStmt, analyzeContributionTestParams := getBigQueryAnalyzeContributionToolInfo(tableNameAnalyzeContribution)
	setupBigQueryTable(t, ctx, client, createAnalyzeContributionTableStmt, insertAnalyzeContributionTableStmt, datasetName, tableNameAnalyzeContribution, analyzeContributionTestParams)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, BigqueryToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addClientAuthSourceConfig(t, toolsFile)
	toolsFile = addBigQuerySqlToolConfig(t, toolsFile, dataTypeToolStmt, arrayDataTypeToolStmt)
	toolsFile = addBigQueryPrebuiltToolsConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := getBigQueryTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, BigqueryToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

	// Set up table for semantic search
	vectorTableName, teardownVectorTable := setupBigQueryVectorTable(t, ctx, client, datasetName)
	defer teardownVectorTable(t)

	// Add semantic search tool config
	insertStmt, searchStmt := getBigQueryVectorSearchStmts(vectorTableName)
	toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, BigqueryToolType, insertStmt, searchStmt)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Get configs for tests
	select1Want := "[{\"f0_\":1}]"
	invokeParamWant := "[{\"id\":1,\"name\":\"Alice\"},{\"id\":3,\"name\":\"Sid\"}]"
	datasetInfoWant := "\"Location\":\"US\",\"DefaultTableExpiration\":0,\"Labels\":null,\"Access\":"
	tableInfoWant := "{\"Name\":\"\",\"Location\":\"US\",\"Description\":\"\",\"Schema\":[{\"Name\":\"id\""
	ddlWant := `"Query executed successfully and returned no content."`
	dataInsightsWant := `FINAL_RESPONSE`
	// Partial message; the full error message is too long.
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing GCP request: failed to insert dry run job: googleapi: Error 400: Syntax error: Unexpected identifier \"SELEC\" at [1:1]`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"f0_\":1}"}]}}`
	createColArray := `["id INT64", "name STRING", "age INT64"]`
	selectEmptyWant := `"The query returned 0 rows."`

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want, tests.DisableOptionalNullParamTest(), tests.EnableClientAuthTest())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want, tests.EnableMcpClientAuthTest())
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam,
		tests.WithCreateColArray(createColArray),
		tests.WithDdlWant(ddlWant),
		tests.WithSelectEmptyWant(selectEmptyWant),
		tests.WithInsert1Want(ddlWant),
	)

	runBigQueryExecuteSqlToolInvokeTest(t, select1Want, invokeParamWant, tableNameParam, ddlWant)
	runBigQueryExecuteSqlToolInvokeDryRunTest(t, datasetName)
	runBigQueryForecastToolInvokeTest(t, tableNameForecast)
	runBigQueryAnalyzeContributionToolInvokeTest(t, tableNameAnalyzeContribution)
	runBigQueryDataTypeTests(t)
	runBigQueryListDatasetToolInvokeTest(t, datasetName)
	runBigQueryGetDatasetInfoToolInvokeTest(t, datasetName, datasetInfoWant)
	runBigQueryListTableIdsToolInvokeTest(t, datasetName, tableName)
	runBigQueryGetTableInfoToolInvokeTest(t, datasetName, tableName, tableInfoWant)
	runBigQueryConversationalAnalyticsInvokeTest(t, datasetName, tableName, dataInsightsWant)
	tests.RunSearchCatalogToolTest(t, tests.SearchCatalogTestParams{
		ContainerParamName: "datasetIds",
		ContainerName:      datasetName,
		ProjectID:          BigqueryProject,
		TargetName:         tableName,
		WantKey:            "DisplayName",
		AllowEmpty:         false,
		CheckValue:         true,
	})
	tests.RunSemanticSearchToolInvokeTest(t, ddlWant, "", "The quick brown fox")
}

func TestBigQueryToolWithDatasetRestriction(t *testing.T) {
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Logf("Starting restriction test with uniqueID: %s", uniqueID)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery client: %s", err)
	}

	allowedDatasetName1 := fmt.Sprintf("allowed_dataset_1_%s", uniqueID)
	allowedDatasetName2 := fmt.Sprintf("allowed_dataset_2_%s", uniqueID)
	disallowedDatasetName := fmt.Sprintf("disallowed_dataset_%s", uniqueID)
	allowedTableName1 := "allowed_table_1"
	allowedTableName2 := "allowed_table_2"
	disallowedTableName := "disallowed_table"
	allowedForecastTableName1 := "allowed_forecast_table_1"
	allowedForecastTableName2 := "allowed_forecast_table_2"
	disallowedForecastTableName := "disallowed_forecast_table"

	allowedAnalyzeContributionTableName1 := "allowed_analyze_contribution_table_1"
	allowedAnalyzeContributionTableName2 := "allowed_analyze_contribution_table_2"
	disallowedAnalyzeContributionTableName := "disallowed_analyze_contribution_table"

	// global cleanup for this test run
	t.Cleanup(func() {
		tests.CleanupBigQueryDatasets(t, context.Background(), client, []string{allowedDatasetName1, allowedDatasetName2, disallowedDatasetName})
	})

	// Setup allowed table
	allowedTableNameParam1 := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName1, allowedTableName1)
	createAllowedTableStmt1 := fmt.Sprintf("CREATE TABLE %s (id INT64)", allowedTableNameParam1)
	setupBigQueryTable(t, ctx, client, createAllowedTableStmt1, "", allowedDatasetName1, allowedTableNameParam1, nil)

	allowedTableNameParam2 := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName2, allowedTableName2)
	createAllowedTableStmt2 := fmt.Sprintf("CREATE TABLE %s (id INT64)", allowedTableNameParam2)
	setupBigQueryTable(t, ctx, client, createAllowedTableStmt2, "", allowedDatasetName2, allowedTableNameParam2, nil)

	// Setup allowed forecast table
	allowedForecastTableFullName1 := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName1, allowedForecastTableName1)
	createForecastStmt1, insertForecastStmt1, forecastParams1 := getBigQueryForecastToolInfo(allowedForecastTableFullName1)
	setupBigQueryTable(t, ctx, client, createForecastStmt1, insertForecastStmt1, allowedDatasetName1, allowedForecastTableFullName1, forecastParams1)

	allowedForecastTableFullName2 := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName2, allowedForecastTableName2)
	createForecastStmt2, insertForecastStmt2, forecastParams2 := getBigQueryForecastToolInfo(allowedForecastTableFullName2)
	setupBigQueryTable(t, ctx, client, createForecastStmt2, insertForecastStmt2, allowedDatasetName2, allowedForecastTableFullName2, forecastParams2)

	// Setup disallowed table
	disallowedTableNameParam := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, disallowedDatasetName, disallowedTableName)
	createDisallowedTableStmt := fmt.Sprintf("CREATE TABLE %s (id INT64)", disallowedTableNameParam)
	setupBigQueryTable(t, ctx, client, createDisallowedTableStmt, "", disallowedDatasetName, disallowedTableNameParam, nil)

	// Setup disallowed forecast table
	disallowedForecastTableFullName := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, disallowedDatasetName, disallowedForecastTableName)
	createDisallowedForecastStmt, insertDisallowedForecastStmt, disallowedForecastParams := getBigQueryForecastToolInfo(disallowedForecastTableFullName)
	setupBigQueryTable(t, ctx, client, createDisallowedForecastStmt, insertDisallowedForecastStmt, disallowedDatasetName, disallowedForecastTableFullName, disallowedForecastParams)

	// Setup allowed analyze contribution table
	allowedAnalyzeContributionTableFullName1 := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName1, allowedAnalyzeContributionTableName1)
	createAnalyzeContributionStmt1, insertAnalyzeContributionStmt1, analyzeContributionParams1 := getBigQueryAnalyzeContributionToolInfo(allowedAnalyzeContributionTableFullName1)
	setupBigQueryTable(t, ctx, client, createAnalyzeContributionStmt1, insertAnalyzeContributionStmt1, allowedDatasetName1, allowedAnalyzeContributionTableFullName1, analyzeContributionParams1)

	allowedAnalyzeContributionTableFullName2 := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName2, allowedAnalyzeContributionTableName2)
	createAnalyzeContributionStmt2, insertAnalyzeContributionStmt2, analyzeContributionParams2 := getBigQueryAnalyzeContributionToolInfo(allowedAnalyzeContributionTableFullName2)
	setupBigQueryTable(t, ctx, client, createAnalyzeContributionStmt2, insertAnalyzeContributionStmt2, allowedDatasetName2, allowedAnalyzeContributionTableFullName2, analyzeContributionParams2)

	// Setup disallowed analyze contribution table
	disallowedAnalyzeContributionTableFullName := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, disallowedDatasetName, disallowedAnalyzeContributionTableName)
	createDisallowedAnalyzeContributionStmt, insertDisallowedAnalyzeContributionStmt, disallowedAnalyzeContributionParams := getBigQueryAnalyzeContributionToolInfo(disallowedAnalyzeContributionTableFullName)
	setupBigQueryTable(t, ctx, client, createDisallowedAnalyzeContributionStmt, insertDisallowedAnalyzeContributionStmt, disallowedDatasetName, disallowedAnalyzeContributionTableFullName, disallowedAnalyzeContributionParams)

	// Configure source with dataset restriction.
	sourceConfig := getBigQueryVars(t)
	sourceConfig["allowedDatasets"] = []string{allowedDatasetName1, allowedDatasetName2}

	// Configure tool
	toolsConfig := map[string]any{
		"list-dataset-ids-restricted": map[string]any{
			"type":        "bigquery-list-dataset-ids",
			"source":      "my-instance",
			"description": "Tool to list dataset ids",
		},
		"list-table-ids-restricted": map[string]any{
			"type":        "bigquery-list-table-ids",
			"source":      "my-instance",
			"description": "Tool to list table within a dataset",
		},
		"get-dataset-info-restricted": map[string]any{
			"type":        "bigquery-get-dataset-info",
			"source":      "my-instance",
			"description": "Tool to get dataset info",
		},
		"get-table-info-restricted": map[string]any{
			"type":        "bigquery-get-table-info",
			"source":      "my-instance",
			"description": "Tool to get table info",
		},
		"execute-sql-restricted": map[string]any{
			"type":        "bigquery-execute-sql",
			"source":      "my-instance",
			"description": "Tool to execute SQL",
		},
		"conversational-analytics-restricted": map[string]any{
			"type":        "bigquery-conversational-analytics",
			"source":      "my-instance",
			"description": "Tool to ask BigQuery conversational analytics",
		},
		"forecast-restricted": map[string]any{
			"type":        "bigquery-forecast",
			"source":      "my-instance",
			"description": "Tool to forecast",
		},
		"analyze-contribution-restricted": map[string]any{
			"type":        "bigquery-analyze-contribution",
			"source":      "my-instance",
			"description": "Tool to analyze contribution",
		},
	}

	// Create config file
	config := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": toolsConfig,
	}

	// Start server
	args := []string{"--enable-api"}
	cmd, cleanup, err := tests.StartCmd(ctx, config, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()
	defer cmd.Close()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// FIX: Background goroutine to drain server logs and prevent pipe buffer deadlock.
	go func() {
		_, _ = io.Copy(io.Discard, cmd.Out)
	}()

	// Run tests
	runListDatasetIdsWithRestriction(t, allowedDatasetName1, allowedDatasetName2)
	runListTableIdsWithRestriction(t, allowedDatasetName1, disallowedDatasetName, allowedTableName1, allowedForecastTableName1, allowedAnalyzeContributionTableName1)
	runListTableIdsWithRestriction(t, allowedDatasetName2, disallowedDatasetName, allowedTableName2, allowedForecastTableName2, allowedAnalyzeContributionTableName2)
	runGetDatasetInfoWithRestriction(t, allowedDatasetName1, disallowedDatasetName)
	runGetDatasetInfoWithRestriction(t, allowedDatasetName2, disallowedDatasetName)
	runGetTableInfoWithRestriction(t, allowedDatasetName1, disallowedDatasetName, allowedTableName1, disallowedTableName)
	runGetTableInfoWithRestriction(t, allowedDatasetName2, disallowedDatasetName, allowedTableName2, disallowedTableName)
	runExecuteSqlWithRestriction(t, allowedTableNameParam1, disallowedTableNameParam)
	runExecuteSqlWithRestriction(t, allowedTableNameParam2, disallowedTableNameParam)
	runConversationalAnalyticsWithRestriction(t, allowedDatasetName1, disallowedDatasetName, allowedTableName1, disallowedTableName)
	runConversationalAnalyticsWithRestriction(t, allowedDatasetName2, disallowedDatasetName, allowedTableName2, disallowedTableName)
	runForecastWithRestriction(t, allowedForecastTableFullName1, disallowedForecastTableFullName)
	runForecastWithRestriction(t, allowedForecastTableFullName2, disallowedForecastTableFullName)
	runAnalyzeContributionWithRestriction(t, allowedAnalyzeContributionTableFullName1, disallowedAnalyzeContributionTableFullName)
	runAnalyzeContributionWithRestriction(t, allowedAnalyzeContributionTableFullName2, disallowedAnalyzeContributionTableFullName)
}

func TestBigQueryWriteModeAllowed(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	sourceConfig["writeMode"] = "allowed"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	datasetName := fmt.Sprintf("temp_toolbox_test_allowed_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))

	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery connection: %s", err)
	}

	dataset := client.Dataset(datasetName)
	if err := dataset.Create(ctx, &bigqueryapi.DatasetMetadata{Name: datasetName}); err != nil {
		t.Fatalf("Failed to create dataset %q: %v", datasetName, err)
	}
	defer func() {
		if err := dataset.DeleteWithContents(context.WithoutCancel(ctx)); err != nil {
			t.Logf("failed to cleanup dataset %s: %v", datasetName, err)
		}
	}()

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{
			"my-exec-sql-tool": map[string]any{
				"type":        "bigquery-execute-sql",
				"source":      "my-instance",
				"description": "Tool to execute sql",
			},
		},
	}

	args := []string{"--enable-api"}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()
	defer cmd.Close()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	runBigQueryWriteModeAllowedTest(t, datasetName)
}

func TestBigQueryWriteModeBlocked(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	sourceConfig["writeMode"] = "blocked"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	datasetName := fmt.Sprintf("temp_toolbox_test_blocked_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	tableName := fmt.Sprintf("param_table_blocked_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	tableNameParam := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, datasetName, tableName)

	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery connection: %s", err)
	}
	createParamTableStmt, insertParamTableStmt, _, _, _, _, paramTestParams := getBigQueryParamToolInfo(tableNameParam)
	teardownTable := setupBigQueryTable(t, ctx, client, createParamTableStmt, insertParamTableStmt, datasetName, tableNameParam, paramTestParams)
	defer teardownTable(t)

	toolsFile := map[string]any{
		"sources": map[string]any{"my-instance": sourceConfig},
		"tools": map[string]any{
			"my-exec-sql-tool": map[string]any{"type": "bigquery-execute-sql", "source": "my-instance", "description": "Tool to execute sql"},
		},
	}

	args := []string{"--enable-api"}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()
	defer cmd.Close()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	runBigQueryWriteModeBlockedTest(t, tableNameParam, datasetName)
}

func TestBigQueryWriteModeProtected(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	sourceConfig["writeMode"] = "protected"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	permanentDatasetName := fmt.Sprintf("perm_dataset_protected_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery connection: %s", err)
	}
	dataset := client.Dataset(permanentDatasetName)
	if err := dataset.Create(ctx, &bigqueryapi.DatasetMetadata{Name: permanentDatasetName}); err != nil {
		t.Fatalf("Failed to create dataset %q: %v", permanentDatasetName, err)
	}
	defer func() {
		if err := dataset.DeleteWithContents(context.WithoutCancel(ctx)); err != nil {
			t.Logf("failed to cleanup dataset %s: %v", permanentDatasetName, err)
		}
	}()

	toolsFile := map[string]any{
		"sources": map[string]any{"my-instance": sourceConfig},
		"tools": map[string]any{
			"my-exec-sql-tool": map[string]any{"type": "bigquery-execute-sql", "source": "my-instance", "description": "Tool to execute sql"},
			"my-sql-tool-protected": map[string]any{
				"type":        "bigquery-sql",
				"source":      "my-instance",
				"description": "Tool to query from the session",
				"statement":   "SELECT * FROM my_shared_temp_table",
				"annotations": map[string]any{
					"readOnlyHint": true,
				},
			},
			"my-forecast-tool-protected": map[string]any{
				"type":        "bigquery-forecast",
				"source":      "my-instance",
				"description": "Tool to forecast from session temp table",
			},
			"my-analyze-contribution-tool-protected": map[string]any{
				"type":        "bigquery-analyze-contribution",
				"source":      "my-instance",
				"description": "Tool to analyze contribution from session temp table",
			},
		},
	}

	args := []string{"--enable-api"}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()
	defer cmd.Close()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	runBigQueryWriteModeProtectedTest(t, permanentDatasetName)
}

// TestBigQuery_ReadOnlyVulnerabilityBlock verifies that if a custom tool is falsely annotated
// as readOnlyHint: true (thus bypassing server-level suppression) on a readOnly: true source,
// BigQuery dry-run validation will still catch and reject write queries while allowing valid reads.
func TestBigQuery_ReadOnlyVulnerabilityBlock(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	sourceConfig["readOnly"] = true

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	args := []string{"--enable-api", "--port", "5002"}

	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	datasetName := fmt.Sprintf("temp_toolbox_test_vuln_%s", uniqueID)
	tableName := fmt.Sprintf("vulnerability_test_%s", uniqueID)

	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery connection: %s", err)
	}

	dataset := client.Dataset(datasetName)
	if err := dataset.Create(ctx, &bigqueryapi.DatasetMetadata{Name: datasetName}); err != nil {
		t.Fatalf("Failed to create dataset %q: %v", datasetName, err)
	}
	defer func() {
		if err := dataset.DeleteWithContents(context.WithoutCancel(ctx)); err != nil {
			t.Logf("failed to cleanup dataset %s: %v", datasetName, err)
		}
	}()

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-readonly-bigquery-instance": sourceConfig,
		},
		"tools": map[string]any{
			"vulnerable_write_tool": map[string]any{
				"type":        "bigquery-execute-sql",
				"source":      "my-readonly-bigquery-instance",
				"description": "I am a tool that tries to write but falsely claims to be read-only!",
				"annotations": map[string]any{
					"readOnlyHint": true,
				},
			},
		},
	}

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()
	defer cmd.Close()

	waitCtx, cancelWait := context.WithTimeout(ctx, 30*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	api := "http://127.0.0.1:5002/api/tool/vulnerable_write_tool/invoke"

	// 1. Verify falsely annotated DDL write is rejected by dry-run defense
	requestBody := strings.NewReader(fmt.Sprintf(`{"sql": "CREATE TABLE %s.%s (id INT64);"}`, datasetName, tableName))
	resp, respBody := tests.RunRequest(t, "POST", api, requestBody, map[string]string{})

	bodyLower := strings.ToLower(string(respBody))
	if !strings.Contains(bodyLower, "blocked") && !strings.Contains(bodyLower, "only select statements are allowed") {
		t.Fatalf("Vulnerability check failed! Expected BigQuery dry-run validation to reject the write query, but got response code %d and body:\n%s", resp.StatusCode, string(respBody))
	}

	// 2. Verify valid SELECT read query succeeds on read-only source
	readBody := strings.NewReader(`{"sql": "SELECT 1 AS result;"}`)
	resp, respBody = tests.RunRequest(t, "POST", api, readBody, map[string]string{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected valid SELECT query to succeed on read-only source, got response code %d and body:\n%s", resp.StatusCode, string(respBody))
	}
}

// getBigQueryParamToolInfo returns statements and param for my-tool for bigquery type
func getBigQueryParamToolInfo(tableName string) (string, string, string, string, string, string, []bigqueryapi.QueryParameter) {
	createStatement := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (id INT64, name STRING);`, tableName)
	insertStatement := fmt.Sprintf(`
		INSERT INTO %s (id, name) VALUES (?, ?), (?, ?), (?, ?), (?, NULL);`, tableName)
	toolStatement := fmt.Sprintf(`SELECT * FROM %s WHERE id = ? OR name = ? ORDER BY id;`, tableName)
	idToolStatement := fmt.Sprintf(`SELECT * FROM %s WHERE id = ? ORDER BY id;`, tableName)
	nameToolStatement := fmt.Sprintf(`SELECT * FROM %s WHERE name = ? ORDER BY id;`, tableName)
	arrayToolStatememt := fmt.Sprintf(`SELECT * FROM %s WHERE id IN UNNEST(@idArray) AND name IN UNNEST(@nameArray) ORDER BY id;`, tableName)
	params := []bigqueryapi.QueryParameter{
		{Value: int64(1)}, {Value: "Alice"},
		{Value: int64(2)}, {Value: "Jane"},
		{Value: int64(3)}, {Value: "Sid"},
		{Value: int64(4)},
	}
	return createStatement, insertStatement, toolStatement, idToolStatement, nameToolStatement, arrayToolStatememt, params
}

// getBigQueryAuthToolInfo returns statements and param of my-auth-tool for bigquery type
func getBigQueryAuthToolInfo(tableName string) (string, string, string, []bigqueryapi.QueryParameter) {
	createStatement := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (id INT64, name STRING, email STRING)`, tableName)
	insertStatement := fmt.Sprintf(`
		INSERT INTO %s (id, name, email) VALUES (?, ?, ?), (?, ?, ?)`, tableName)
	toolStatement := fmt.Sprintf(`
		SELECT name FROM %s WHERE email = ?`, tableName)
	params := []bigqueryapi.QueryParameter{
		{Value: int64(1)}, {Value: "Alice"}, {Value: tests.ServiceAccountEmail},
		{Value: int64(2)}, {Value: "Jane"}, {Value: "janedoe@gmail.com"},
	}
	return createStatement, insertStatement, toolStatement, params
}

// getBigQueryDataTypeTestInfo returns statements and params for data type tests.
func getBigQueryDataTypeTestInfo(tableName string) (string, string, string, string, []bigqueryapi.QueryParameter) {
	createStatement := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (id INT64, int_val INT64, string_val STRING, float_val FLOAT64, bool_val BOOL);`, tableName)
	insertStatement := fmt.Sprintf(`
		INSERT INTO %s (id, int_val, string_val, float_val, bool_val) VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?), (?, ?, ?, ?, ?);`, tableName)
	toolStatement := fmt.Sprintf(`SELECT * FROM %s WHERE int_val = ? AND string_val = ? AND float_val = ? AND bool_val = ?;`, tableName)
	arrayToolStatement := fmt.Sprintf(`SELECT * FROM %s WHERE int_val IN UNNEST(@int_array) AND string_val IN UNNEST(@string_array) AND float_val IN UNNEST(@float_array) AND bool_val IN UNNEST(@bool_array) ORDER BY id;`, tableName)
	params := []bigqueryapi.QueryParameter{
		{Value: int64(1)}, {Value: int64(123)}, {Value: "hello"}, {Value: 3.14}, {Value: true},
		{Value: int64(2)}, {Value: int64(-456)}, {Value: "world"}, {Value: -0.55}, {Value: false},
		{Value: int64(3)}, {Value: int64(789)}, {Value: "test"}, {Value: 100.1}, {Value: true},
	}
	return createStatement, insertStatement, toolStatement, arrayToolStatement, params
}

// getBigQueryForecastToolInfo returns statements and params for the forecast tool.
func getBigQueryForecastToolInfo(tableName string) (string, string, []bigqueryapi.QueryParameter) {
	createStatement := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (ts TIMESTAMP, data FLOAT64, id STRING);`, tableName)
	insertStatement := fmt.Sprintf(`
		INSERT INTO %s (ts, data, id) VALUES
		(?, ?, ?), (?, ?, ?), (?, ?, ?), 
		(?, ?, ?), (?, ?, ?), (?, ?, ?);`, tableName)
	params := []bigqueryapi.QueryParameter{
		{Value: "2025-01-01T00:00:00Z"}, {Value: 10.0}, {Value: "a"},
		{Value: "2025-01-01T01:00:00Z"}, {Value: 11.0}, {Value: "a"},
		{Value: "2025-01-01T02:00:00Z"}, {Value: 12.0}, {Value: "a"},
		{Value: "2025-01-01T00:00:00Z"}, {Value: 20.0}, {Value: "b"},
		{Value: "2025-01-01T01:00:00Z"}, {Value: 21.0}, {Value: "b"},
		{Value: "2025-01-01T02:00:00Z"}, {Value: 22.0}, {Value: "b"},
	}
	return createStatement, insertStatement, params
}

// getBigQueryAnalyzeContributionToolInfo returns statements and params for the analyze-contribution tool.
func getBigQueryAnalyzeContributionToolInfo(tableName string) (string, string, []bigqueryapi.QueryParameter) {
	createStatement := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (dim1 STRING, dim2 STRING, is_test BOOL, metric FLOAT64);`, tableName)
	insertStatement := fmt.Sprintf(`
		INSERT INTO %s (dim1, dim2, is_test, metric) VALUES 
		(?, ?, ?, ?), (?, ?, ?, ?), (?, ?, ?, ?), (?, ?, ?, ?);`, tableName)
	params := []bigqueryapi.QueryParameter{
		{Value: "a"}, {Value: "x"}, {Value: true}, {Value: 100.0},
		{Value: "a"}, {Value: "x"}, {Value: false}, {Value: 110.0},
		{Value: "a"}, {Value: "y"}, {Value: true}, {Value: 120.0},
		{Value: "a"}, {Value: "y"}, {Value: false}, {Value: 100.0},
		{Value: "b"}, {Value: "x"}, {Value: true}, {Value: 40.0},
		{Value: "b"}, {Value: "x"}, {Value: false}, {Value: 100.0},
		{Value: "b"}, {Value: "y"}, {Value: true}, {Value: 60.0},
		{Value: "b"}, {Value: "y"}, {Value: false}, {Value: 60.0},
	}
	return createStatement, insertStatement, params
}

// getBigQueryTmplToolStatement returns statements for template parameter test cases for bigquery type
func getBigQueryTmplToolStatement() (string, string) {
	tmplSelectCombined := "SELECT * FROM {{.tableName}} WHERE id = ? ORDER BY id"
	tmplSelectFilterCombined := "SELECT * FROM {{.tableName}} WHERE {{.columnFilter}} = ? ORDER BY id"
	return tmplSelectCombined, tmplSelectFilterCombined
}

func setupBigQueryTable(t *testing.T, ctx context.Context, client *bigqueryapi.Client, createStatement, insertStatement, datasetName string, tableName string, params []bigqueryapi.QueryParameter) func(*testing.T) {
	// Create dataset
	dataset := client.Dataset(datasetName)
	_, err := dataset.Metadata(ctx)

	if err != nil {
		apiErr, ok := err.(*googleapi.Error)
		if !ok || apiErr.Code != 404 {
			t.Fatalf("Failed to check dataset %q existence: %v", datasetName, err)
		}
		metadataToCreate := &bigqueryapi.DatasetMetadata{Name: datasetName}
		if err := dataset.Create(ctx, metadataToCreate); err != nil {
			t.Fatalf("Failed to create dataset %q: %v", datasetName, err)
		}
	}

	// Create table
	createJob, err := client.Query(createStatement).Run(ctx)

	if err != nil {
		t.Fatalf("Failed to start create table job for %s: %v", tableName, err)
	}
	createStatus, err := createJob.Wait(ctx)
	if err != nil {
		t.Fatalf("Failed to wait for create table job for %s: %v", tableName, err)
	}
	if err := createStatus.Err(); err != nil {
		t.Fatalf("Create table job for %s failed: %v", tableName, err)
	}

	if len(params) > 0 {
		// Insert test data
		insertQuery := client.Query(insertStatement)
		insertQuery.Parameters = params
		insertJob, err := insertQuery.Run(ctx)
		if err != nil {
			t.Fatalf("Failed to start insert job for %s: %v", tableName, err)
		}
		insertStatus, err := insertJob.Wait(ctx)
		if err != nil {
			t.Fatalf("Failed to wait for insert job for %s: %v", tableName, err)
		}
		if err := insertStatus.Err(); err != nil {
			t.Fatalf("Insert job for %s failed: %v", tableName, err)
		}
	}

	return func(t *testing.T) {
		// tear down table
		dropSQL := fmt.Sprintf("drop table %s", tableName)
		dropJob, err := client.Query(dropSQL).Run(context.WithoutCancel(ctx))
		if err != nil {
			t.Errorf("Failed to start drop table job for %s: %v", tableName, err)
			return
		}
		dropStatus, err := dropJob.Wait(context.WithoutCancel(ctx))
		if err != nil {
			t.Errorf("Failed to wait for drop table job for %s: %v", tableName, err)
			return
		}
		if err := dropStatus.Err(); err != nil {
			t.Errorf("Error dropping table %s: %v", tableName, err)
		}

		// tear down dataset
		datasetToTeardown := client.Dataset(datasetName)
		tablesIterator := datasetToTeardown.Tables(context.WithoutCancel(ctx))
		_, err = tablesIterator.Next()

		if err == iterator.Done {
			if err := datasetToTeardown.Delete(context.WithoutCancel(ctx)); err != nil {
				t.Errorf("Failed to delete dataset %s: %v", datasetName, err)
			}
		} else if err != nil {
			t.Errorf("Failed to list tables in dataset %s to check emptiness: %v.", datasetName, err)
		}
	}
}

func addBigQueryPrebuiltToolsConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "bigquery-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "bigquery-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	tools["my-client-auth-exec-sql-tool"] = map[string]any{
		"type":        "bigquery-execute-sql",
		"source":      "my-client-auth-source",
		"description": "Tool to execute sql",
	}
	tools["my-forecast-tool"] = map[string]any{
		"type":        "bigquery-forecast",
		"source":      "my-instance",
		"description": "Tool to forecast time series data.",
	}
	tools["my-auth-forecast-tool"] = map[string]any{
		"type":        "bigquery-forecast",
		"source":      "my-instance",
		"description": "Tool to forecast time series data with auth.",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	tools["my-client-auth-forecast-tool"] = map[string]any{
		"type":        "bigquery-forecast",
		"source":      "my-client-auth-source",
		"description": "Tool to forecast time series data with auth.",
	}
	tools["my-analyze-contribution-tool"] = map[string]any{
		"type":        "bigquery-analyze-contribution",
		"source":      "my-instance",
		"description": "Tool to analyze contribution.",
	}
	tools["my-auth-analyze-contribution-tool"] = map[string]any{
		"type":        "bigquery-analyze-contribution",
		"source":      "my-instance",
		"description": "Tool to analyze contribution with auth.",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	tools["my-client-auth-analyze-contribution-tool"] = map[string]any{
		"type":        "bigquery-analyze-contribution",
		"source":      "my-client-auth-source",
		"description": "Tool to analyze contribution with auth.",
	}
	tools["my-list-dataset-ids-tool"] = map[string]any{
		"type":        "bigquery-list-dataset-ids",
		"source":      "my-instance",
		"description": "Tool to list dataset",
	}
	tools["my-auth-list-dataset-ids-tool"] = map[string]any{
		"type":        "bigquery-list-dataset-ids",
		"source":      "my-instance",
		"description": "Tool to list dataset",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	tools["my-client-auth-list-dataset-ids-tool"] = map[string]any{
		"type":        "bigquery-list-dataset-ids",
		"source":      "my-client-auth-source",
		"description": "Tool to list dataset",
	}
	tools["my-get-dataset-info-tool"] = map[string]any{
		"type":        "bigquery-get-dataset-info",
		"source":      "my-instance",
		"description": "Tool to show dataset metadata",
	}
	tools["my-auth-get-dataset-info-tool"] = map[string]any{
		"type":        "bigquery-get-dataset-info",
		"source":      "my-instance",
		"description": "Tool to show dataset metadata",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	tools["my-client-auth-get-dataset-info-tool"] = map[string]any{
		"type":        "bigquery-get-dataset-info",
		"source":      "my-client-auth-source",
		"description": "Tool to show dataset metadata",
	}
	tools["my-list-table-ids-tool"] = map[string]any{
		"type":        "bigquery-list-table-ids",
		"source":      "my-instance",
		"description": "Tool to list table within a dataset",
	}
	tools["my-auth-list-table-ids-tool"] = map[string]any{
		"type":        "bigquery-list-table-ids",
		"source":      "my-instance",
		"description": "Tool to list table within a dataset",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	tools["my-client-auth-list-table-ids-tool"] = map[string]any{
		"type":        "bigquery-list-table-ids",
		"source":      "my-client-auth-source",
		"description": "Tool to list table within a dataset",
	}
	tools["my-get-table-info-tool"] = map[string]any{
		"type":        "bigquery-get-table-info",
		"source":      "my-instance",
		"description": "Tool to show dataset metadata",
	}
	tools["my-auth-get-table-info-tool"] = map[string]any{
		"type":        "bigquery-get-table-info",
		"source":      "my-instance",
		"description": "Tool to show dataset metadata",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	tools["my-client-auth-get-table-info-tool"] = map[string]any{
		"type":        "bigquery-get-table-info",
		"source":      "my-client-auth-source",
		"description": "Tool to show dataset metadata",
	}
	tools["my-conversational-analytics-tool"] = map[string]any{
		"type":        "bigquery-conversational-analytics",
		"source":      "my-instance",
		"description": "Tool to ask BigQuery conversational analytics",
	}
	tools["my-auth-conversational-analytics-tool"] = map[string]any{
		"type":        "bigquery-conversational-analytics",
		"source":      "my-instance",
		"description": "Tool to ask BigQuery conversational analytics",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	tools["my-client-auth-conversational-analytics-tool"] = map[string]any{
		"type":        "bigquery-conversational-analytics",
		"source":      "my-client-auth-source",
		"description": "Tool to ask BigQuery conversational analytics",
	}
	tools["my-search-catalog-tool"] = map[string]any{
		"type":        "bigquery-search-catalog",
		"source":      "my-instance",
		"description": "Tool to search the BiqQuery catalog",
	}
	tools["my-auth-search-catalog-tool"] = map[string]any{
		"type":        "bigquery-search-catalog",
		"source":      "my-instance",
		"description": "Tool to search the BiqQuery catalog",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	tools["my-client-auth-search-catalog-tool"] = map[string]any{
		"type":        "bigquery-search-catalog",
		"source":      "my-client-auth-source",
		"description": "Tool to search the BiqQuery catalog",
	}
	config["tools"] = tools
	return config
}

func addClientAuthSourceConfig(t *testing.T, config map[string]any) map[string]any {
	sources, ok := config["sources"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get sources from config")
	}
	sources["my-client-auth-source"] = map[string]any{
		"type":           BigquerySourceType,
		"project":        BigqueryProject,
		"useClientOAuth": true,
	}
	sources["my-custom-client-auth-source"] = map[string]any{
		"type":           BigquerySourceType,
		"project":        BigqueryProject,
		"useClientOAuth": "X-Custom-Auth",
	}
	config["sources"] = sources
	return config
}

func addBigQuerySqlToolConfig(t *testing.T, config map[string]any, toolStatement, arrayToolStatement string) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-scalar-datatype-tool"] = map[string]any{
		"type":        "bigquery-sql",
		"source":      "my-instance",
		"description": "Tool to test various scalar data types.",
		"statement":   toolStatement,
		"parameters": []any{
			map[string]any{"name": "int_val", "type": "integer", "description": "an integer value"},
			map[string]any{"name": "string_val", "type": "string", "description": "a string value"},
			map[string]any{"name": "float_val", "type": "float", "description": "a float value"},
			map[string]any{"name": "bool_val", "type": "boolean", "description": "a boolean value"},
		},
	}
	tools["my-array-datatype-tool"] = map[string]any{
		"type":        "bigquery-sql",
		"source":      "my-instance",
		"description": "Tool to test various array data types.",
		"statement":   arrayToolStatement,
		"parameters": []any{
			map[string]any{"name": "int_array", "type": "array", "description": "an array of integer values", "items": map[string]any{"name": "item", "type": "integer", "description": "desc"}},
			map[string]any{"name": "string_array", "type": "array", "description": "an array of string values", "items": map[string]any{"name": "item", "type": "string", "description": "desc"}},
			map[string]any{"name": "float_array", "type": "array", "description": "an array of float values", "items": map[string]any{"name": "item", "type": "float", "description": "desc"}},
			map[string]any{"name": "bool_array", "type": "array", "description": "an array of boolean values", "items": map[string]any{"name": "item", "type": "boolean", "description": "desc"}},
		},
	}
	tools["my-client-auth-tool"] = map[string]any{
		"type":        "bigquery-sql",
		"source":      "my-client-auth-source",
		"description": "Tool to test client authorization.",
		"statement":   "SELECT 1",
	}
	tools["my-custom-client-auth-tool"] = map[string]any{
		"type":        "bigquery-sql",
		"source":      "my-custom-client-auth-source",
		"description": "Tool to test custom client authorization header.",
		"statement":   "SELECT 1",
	}
	config["tools"] = tools
	return config
}

func runBigQueryExecuteSqlToolInvokeTest(t *testing.T, select1Want, invokeParamWant, tableNameParam, ddlWant string, opts ...tests.ToolExecOption) {
	// Get ID token
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Get access token
	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	unqualifiedTableErr := `error processing GCP request: failed to insert dry run job: googleapi: Error 400: Table "t" must be qualified with a dataset (e.g. dataset.table)., invalid`

	// Test tool invoke endpoint
	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:     "invoke my-exec-sql-tool without body",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{},
			wantErr:  `parameter "sql" is required`,
		},
		{
			name:     "invoke my-exec-sql-tool",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{"sql": "SELECT 1"},
			want:     select1Want,
		},
		{
			name:     "invoke my-exec-sql-tool create table",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{"sql": "CREATE TABLE t (id SERIAL PRIMARY KEY, name TEXT)"},
			wantErr:  unqualifiedTableErr,
		},
		{
			name:     "invoke my-exec-sql-tool with data present in table",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{"sql": fmt.Sprintf("SELECT id, name FROM %s WHERE id = 3 OR name = 'Alice' ORDER BY id", tableNameParam)},
			want:     invokeParamWant,
		},
		{
			name:     "invoke my-exec-sql-tool with no matching rows",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{"sql": fmt.Sprintf("SELECT * FROM %s WHERE id = 999", tableNameParam)},
			want:     `"The query returned 0 rows."`,
			wantMCP:  `["The query returned 0 rows."]`,
		},
		{
			name:     "invoke my-exec-sql-tool drop table",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{"sql": "DROP TABLE t"},
			wantErr:  unqualifiedTableErr,
		},
		{
			name:     "invoke my-exec-sql-tool insert entry",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{"sql": fmt.Sprintf("INSERT INTO %s (id, name) VALUES (4, 'test_name')", tableNameParam)},
			want:     ddlWant,
			wantMCP:  "[" + ddlWant + "]",
		},
		{
			name:     "invoke my-exec-sql-tool without body",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{},
			isErr:    true,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool with auth token",
			toolName:      "my-auth-exec-sql-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"sql": "SELECT 1"},
			want:          select1Want,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool with invalid auth token",
			toolName:      "my-auth-exec-sql-tool",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			args:          map[string]any{"sql": "SELECT 1"},
			isErr:         true,
		},
		{
			name:     "Invoke my-auth-exec-sql-tool without auth token",
			toolName: "my-auth-exec-sql-tool",
			args:     map[string]any{"sql": "SELECT 1"},
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-exec-sql-tool with auth token",
			toolName:      "my-client-auth-exec-sql-tool",
			requestHeader: map[string]string{"Authorization": accessToken},
			args:          map[string]any{"sql": "SELECT 1"},
			want:          "[{\"f0_\":1}]",
		},
		{
			name:     "Invoke my-client-auth-exec-sql-tool without auth token",
			toolName: "my-client-auth-exec-sql-tool",
			args:     map[string]any{"sql": "SELECT 1"},
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-exec-sql-tool with invalid auth token",
			toolName:      "my-client-auth-exec-sql-tool",
			requestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			args:          map[string]any{"sql": "SELECT 1"},
			isErr:         true,
		},
	}, opts...)
}

func runBigQueryWriteModeAllowedTest(t *testing.T, datasetName string, opts ...tests.ToolExecOption) {
	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:     "CREATE TABLE should succeed",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{"sql": fmt.Sprintf("CREATE TABLE %s.new_table (x INT64)", datasetName)},
			want:     `"Query executed successfully and returned no content."`,
			wantMCP:  `["Query executed successfully and returned no content."]`,
		},
	}, opts...)
}

func runBigQueryWriteModeBlockedTest(t *testing.T, tableNameParam, datasetName string, opts ...tests.ToolExecOption) {
	blockedErr := "write mode is 'blocked', only SELECT statements are allowed"
	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:     "SELECT statement should succeed",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{"sql": fmt.Sprintf("SELECT id, name FROM %s WHERE id = 1", tableNameParam)},
			want:     `[{"id":1,"name":"Alice"}]`,
		},
		{
			name:     "INSERT statement should fail",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{"sql": fmt.Sprintf("INSERT INTO %s (id, name) VALUES (10, 'test')", tableNameParam)},
			wantErr:  blockedErr,
		},
		{
			name:     "CREATE TABLE statement should fail",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{"sql": fmt.Sprintf("CREATE TABLE %s.new_table (x INT64)", datasetName)},
			wantErr:  blockedErr,
		},
	}, opts...)
}

func runBigQueryWriteModeProtectedTest(t *testing.T, permanentDatasetName string, opts ...tests.ToolExecOption) {
	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:     "CREATE TABLE to permanent dataset should fail",
			toolName: "my-exec-sql-tool",
			args:     map[string]any{"sql": fmt.Sprintf("CREATE TABLE %s.new_table (x INT64)", permanentDatasetName)},
			wantErr:  "protected write mode only supports SELECT statements, or write operations in the anonymous dataset",
		},
		{
			name:         "CREATE TEMP TABLE should succeed",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": "CREATE TEMP TABLE my_shared_temp_table (x INT64)"},
			wantContains: `"Query executed successfully and returned no content."`,
		},
		{
			name:         "INSERT into TEMP TABLE should succeed",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": "INSERT INTO my_shared_temp_table (x) VALUES (42)"},
			wantContains: `"Query executed successfully and returned no content."`,
		},
		{
			name:         "SELECT from TEMP TABLE with exec-sql should succeed",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": "SELECT * FROM my_shared_temp_table"},
			wantContains: `[{"x":42}]`,
		},
		{
			name:         "SELECT from TEMP TABLE with sql-tool should succeed",
			toolName:     "my-sql-tool-protected",
			args:         map[string]any{},
			wantContains: `[{"x":42}]`,
		},
		{
			name:         "CREATE TEMP TABLE for forecast should succeed",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": "CREATE TEMP TABLE forecast_temp_table (ts TIMESTAMP, data FLOAT64) AS SELECT TIMESTAMP('2025-01-01T00:00:00Z') AS ts, 10.0 AS data UNION ALL SELECT TIMESTAMP('2025-01-01T01:00:00Z'), 11.0 UNION ALL SELECT TIMESTAMP('2025-01-01T02:00:00Z'), 12.0 UNION ALL SELECT TIMESTAMP('2025-01-01T03:00:00Z'), 13.0"},
			wantContains: `"Query executed successfully and returned no content."`,
		},
		{
			name:         "Forecast from TEMP TABLE should succeed",
			toolName:     "my-forecast-tool-protected",
			args:         map[string]any{"history_data": "SELECT * FROM forecast_temp_table", "timestamp_col": "ts", "data_col": "data", "horizon": 1},
			wantContains: `"forecast_timestamp"`,
		},
		{
			name:         "CREATE TEMP TABLE for contribution analysis should succeed",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": "CREATE TEMP TABLE contribution_temp_table (dim1 STRING, is_test BOOL, metric FLOAT64) AS SELECT 'a' as dim1, true as is_test, 100.0 as metric UNION ALL SELECT 'b', false, 120.0"},
			wantContains: `"Query executed successfully and returned no content."`,
		},
		{
			name:         "Analyze contribution from TEMP TABLE should succeed",
			toolName:     "my-analyze-contribution-tool-protected",
			args:         map[string]any{"input_data": "SELECT * FROM contribution_temp_table", "contribution_metric": "SUM(metric)", "is_test_col": "is_test", "dimension_id_cols": []any{"dim1"}},
			wantContains: `"relative_difference"`,
		},
	}, opts...)
}

func runBigQueryExecuteSqlToolInvokeDryRunTest(t *testing.T, datasetName string, opts ...tests.ToolExecOption) {
	// Get ID token
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	newTableName := fmt.Sprintf("%s.new_dry_run_table_%s", datasetName, strings.ReplaceAll(uuid.New().String(), "-", ""))

	// Test tool invoke endpoint
	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:         "invoke my-exec-sql-tool with dryRun",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": "SELECT 1", "dry_run": true},
			wantContains: `\"statementType\": \"SELECT\"`,
		},
		{
			name:         "invoke my-exec-sql-tool with dryRun create table",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING)", newTableName), "dry_run": true},
			wantContains: `\"statementType\": \"CREATE_TABLE\"`,
		},
		{
			name:         "invoke my-exec-sql-tool with dryRun execute immediate",
			toolName:     "my-exec-sql-tool",
			args:         map[string]any{"sql": fmt.Sprintf(`EXECUTE IMMEDIATE "CREATE TABLE %s (id INT64, name STRING)"`, newTableName), "dry_run": true},
			wantContains: `\"statementType\": \"SCRIPT\"`,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool with dryRun and auth token",
			toolName:      "my-auth-exec-sql-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"sql": "SELECT 1", "dry_run": true},
			wantContains:  `\"statementType\": \"SELECT\"`,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool with dryRun and invalid auth token",
			toolName:      "my-auth-exec-sql-tool",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			args:          map[string]any{"sql": "SELECT 1", "dry_run": true},
			isErr:         true,
		},
		{
			name:     "Invoke my-auth-exec-sql-tool with dryRun and without auth token",
			toolName: "my-auth-exec-sql-tool",
			args:     map[string]any{"sql": "SELECT 1", "dry_run": true},
			isErr:    true,
		},
	}, opts...)
}

func runBigQueryForecastToolInvokeTest(t *testing.T, tableName string, opts ...tests.ToolExecOption) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Get access token
	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	historyDataTable := strings.ReplaceAll(tableName, "`", "")
	historyDataQuery := fmt.Sprintf("SELECT ts, data, id FROM %s", tableName)
	forecastArgs := func() map[string]any {
		return map[string]any{"history_data": historyDataTable, "timestamp_col": "ts", "data_col": "data"}
	}

	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:     "invoke my-forecast-tool without required params",
			toolName: "my-forecast-tool",
			args:     map[string]any{"history_data": historyDataTable},
			isErr:    true,
		},
		{
			name:         "invoke my-forecast-tool with table",
			toolName:     "my-forecast-tool",
			args:         forecastArgs(),
			wantContains: `"forecast_timestamp"`,
		},
		{
			name:         "invoke my-forecast-tool with query and horizon",
			toolName:     "my-forecast-tool",
			args:         map[string]any{"history_data": historyDataQuery, "timestamp_col": "ts", "data_col": "data", "horizon": 5},
			wantContains: `"forecast_timestamp"`,
		},
		{
			name:         "invoke my-forecast-tool with id_cols",
			toolName:     "my-forecast-tool",
			args:         map[string]any{"history_data": historyDataTable, "timestamp_col": "ts", "data_col": "data", "id_cols": []any{"id"}},
			wantContains: `"id"`,
		},
		{
			name:          "invoke my-auth-forecast-tool with auth token",
			toolName:      "my-auth-forecast-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          forecastArgs(),
			wantContains:  `"forecast_timestamp"`,
		},
		{
			name:          "invoke my-auth-forecast-tool with invalid auth token",
			toolName:      "my-auth-forecast-tool",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			args:          forecastArgs(),
			isErr:         true,
		},
		{
			name:          "Invoke my-client-auth-forecast-tool with auth token",
			toolName:      "my-client-auth-forecast-tool",
			requestHeader: map[string]string{"Authorization": accessToken},
			args:          forecastArgs(),
			wantContains:  `"forecast_timestamp"`,
		},
		{
			name:     "Invoke my-client-auth-forecast-tool without auth token",
			toolName: "my-client-auth-forecast-tool",
			args:     forecastArgs(),
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-forecast-tool with invalid auth token",
			toolName:      "my-client-auth-forecast-tool",
			requestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			args:          forecastArgs(),
			isErr:         true,
		},
	}, opts...)
}

func runBigQueryAnalyzeContributionToolInvokeTest(t *testing.T, tableName string, opts ...tests.ToolExecOption) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Get access token
	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	dataTable := strings.ReplaceAll(tableName, "`", "")
	contributionArgs := func() map[string]any {
		return map[string]any{"input_data": dataTable, "contribution_metric": "SUM(metric)", "is_test_col": "is_test", "dimension_id_cols": []any{"dim1", "dim2"}}
	}

	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:     "invoke my-analyze-contribution-tool without required params",
			toolName: "my-analyze-contribution-tool",
			args:     map[string]any{"input_data": dataTable},
			isErr:    true,
		},
		{
			name:         "invoke my-analyze-contribution-tool with table",
			toolName:     "my-analyze-contribution-tool",
			args:         contributionArgs(),
			wantContains: `"relative_difference"`,
		},
		{
			name:          "invoke my-auth-analyze-contribution-tool with auth token",
			toolName:      "my-auth-analyze-contribution-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          contributionArgs(),
			wantContains:  `"relative_difference"`,
		},
		{
			name:          "invoke my-auth-analyze-contribution-tool with invalid auth token",
			toolName:      "my-auth-analyze-contribution-tool",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			args:          contributionArgs(),
			isErr:         true,
		},
		{
			name:          "Invoke my-client-auth-analyze-contribution-tool with auth token",
			toolName:      "my-client-auth-analyze-contribution-tool",
			requestHeader: map[string]string{"Authorization": accessToken},
			args:          contributionArgs(),
			wantContains:  `"relative_difference"`,
		},
		{
			name:     "Invoke my-client-auth-analyze-contribution-tool without auth token",
			toolName: "my-client-auth-analyze-contribution-tool",
			args:     contributionArgs(),
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-analyze-contribution-tool with invalid auth token",
			toolName:      "my-client-auth-analyze-contribution-tool",
			requestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			args:          contributionArgs(),
			isErr:         true,
		},
	}, opts...)
}

func runBigQueryDataTypeTests(t *testing.T, opts ...tests.ToolExecOption) {
	// Test tool invoke endpoint
	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:     "invoke my-scalar-datatype-tool with values",
			toolName: "my-scalar-datatype-tool",
			args:     map[string]any{"int_val": 123, "string_val": "hello", "float_val": 3.14, "bool_val": true},
			want:     `[{"id":1,"int_val":123,"string_val":"hello","float_val":3.14,"bool_val":true}]`,
		},
		{
			name:     "invoke my-scalar-datatype-tool with missing params",
			toolName: "my-scalar-datatype-tool",
			args:     map[string]any{"int_val": 123},
			wantErr:  `parameter "string_val" is required`,
		},
		{
			name:     "invoke my-array-datatype-tool",
			toolName: "my-array-datatype-tool",
			args:     map[string]any{"int_array": []any{123, 789}, "string_array": []any{"hello", "test"}, "float_array": []any{3.14, 100.1}, "bool_array": []any{true}},
			want:     `[{"id":1,"int_val":123,"string_val":"hello","float_val":3.14,"bool_val":true},{"id":3,"int_val":789,"string_val":"test","float_val":100.1,"bool_val":true}]`,
		},
	}, opts...)
}

func runBigQueryListDatasetToolInvokeTest(t *testing.T, datasetWant string, opts ...tests.ToolExecOption) {
	// Get ID token
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Get access token
	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	// Test tool invoke endpoint
	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:         "invoke my-list-dataset-ids-tool",
			toolName:     "my-list-dataset-ids-tool",
			args:         map[string]any{},
			wantContains: datasetWant,
		},
		{
			name:          "invoke my-list-dataset-ids-tool with project",
			toolName:      "my-auth-list-dataset-ids-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"project": BigqueryProject},
			wantContains:  datasetWant,
		},
		{
			name:          "invoke my-list-dataset-ids-tool with non-existent project",
			toolName:      "my-auth-list-dataset-ids-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"project": fmt.Sprintf("%s-%s", BigqueryProject, uuid.NewString())},
			isErr:         true,
		},
		{
			name:          "invoke my-auth-list-dataset-ids-tool",
			toolName:      "my-auth-list-dataset-ids-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{},
			wantContains:  datasetWant,
		},
		{
			name:          "Invoke my-client-auth-list-dataset-ids-tool with auth token",
			toolName:      "my-client-auth-list-dataset-ids-tool",
			requestHeader: map[string]string{"Authorization": accessToken},
			args:          map[string]any{},
			wantContains:  datasetWant,
		},
		{
			name:     "Invoke my-client-auth-list-dataset-ids-tool without auth token",
			toolName: "my-client-auth-list-dataset-ids-tool",
			args:     map[string]any{},
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-list-dataset-ids-tool with invalid auth token",
			toolName:      "my-client-auth-list-dataset-ids-tool",
			requestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			args:          map[string]any{},
			isErr:         true,
		},
	}, opts...)
}

func runBigQueryGetDatasetInfoToolInvokeTest(t *testing.T, datasetName, datasetInfoWant string, opts ...tests.ToolExecOption) {
	// Get ID token
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Get access token
	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	datasetArgs := func() map[string]any {
		return map[string]any{"dataset": datasetName}
	}

	// Test tool invoke endpoint
	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:     "invoke my-get-dataset-info-tool without body",
			toolName: "my-get-dataset-info-tool",
			args:     map[string]any{},
			isErr:    true,
		},
		{
			name:         "invoke my-get-dataset-info-tool",
			toolName:     "my-get-dataset-info-tool",
			args:         datasetArgs(),
			wantContains: datasetInfoWant,
		},
		{
			name:          "Invoke my-auth-get-dataset-info-tool with correct project",
			toolName:      "my-auth-get-dataset-info-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"project": BigqueryProject, "dataset": datasetName},
			wantContains:  datasetInfoWant,
		},
		{
			name:          "Invoke my-auth-get-dataset-info-tool with non-existent project",
			toolName:      "my-auth-get-dataset-info-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"project": fmt.Sprintf("%s-%s", BigqueryProject, uuid.NewString()), "dataset": datasetName},
			isErr:         true,
		},
		{
			name:     "invoke my-auth-get-dataset-info-tool without body",
			toolName: "my-get-dataset-info-tool",
			args:     map[string]any{},
			isErr:    true,
		},
		{
			name:          "Invoke my-auth-get-dataset-info-tool with auth token",
			toolName:      "my-auth-get-dataset-info-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          datasetArgs(),
			wantContains:  datasetInfoWant,
		},
		{
			name:          "Invoke my-auth-get-dataset-info-tool with invalid auth token",
			toolName:      "my-auth-get-dataset-info-tool",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			args:          datasetArgs(),
			isErr:         true,
		},
		{
			name:     "Invoke my-auth-get-dataset-info-tool without auth token",
			toolName: "my-auth-get-dataset-info-tool",
			args:     datasetArgs(),
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-get-dataset-info-tool with auth token",
			toolName:      "my-client-auth-get-dataset-info-tool",
			requestHeader: map[string]string{"Authorization": accessToken},
			args:          datasetArgs(),
			wantContains:  datasetInfoWant,
		},
		{
			name:     "Invoke my-client-auth-get-dataset-info-tool without auth token",
			toolName: "my-client-auth-get-dataset-info-tool",
			args:     datasetArgs(),
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-get-dataset-info-tool with invalid auth token",
			toolName:      "my-client-auth-get-dataset-info-tool",
			requestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			args:          datasetArgs(),
			isErr:         true,
		},
	}, opts...)
}

func runBigQueryListTableIdsToolInvokeTest(t *testing.T, datasetName, tablename_want string, opts ...tests.ToolExecOption) {
	// Get ID token
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Get access token
	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	datasetArgs := func() map[string]any {
		return map[string]any{"dataset": datasetName}
	}

	// Test tool invoke endpoint
	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:     "invoke my-list-table-ids-tool without body",
			toolName: "my-list-table-ids-tool",
			args:     map[string]any{},
			isErr:    true,
		},
		{
			name:         "invoke my-list-table-ids-tool",
			toolName:     "my-list-table-ids-tool",
			args:         datasetArgs(),
			wantContains: tablename_want,
		},
		{
			name:     "invoke my-list-table-ids-tool without body",
			toolName: "my-list-table-ids-tool",
			args:     map[string]any{},
			isErr:    true,
		},
		{
			name:          "Invoke my-auth-list-table-ids-tool with auth token",
			toolName:      "my-auth-list-table-ids-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          datasetArgs(),
			wantContains:  tablename_want,
		},
		{
			name:          "Invoke my-auth-list-table-ids-tool with correct project",
			toolName:      "my-auth-list-table-ids-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"project": BigqueryProject, "dataset": datasetName},
			wantContains:  tablename_want,
		},
		{
			name:          "Invoke my-auth-list-table-ids-tool with non-existent project",
			toolName:      "my-auth-list-table-ids-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"project": fmt.Sprintf("%s-%s", BigqueryProject, uuid.NewString()), "dataset": datasetName},
			isErr:         true,
		},
		{
			name:          "Invoke my-auth-list-table-ids-tool with invalid auth token",
			toolName:      "my-auth-list-table-ids-tool",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			args:          datasetArgs(),
			isErr:         true,
		},
		{
			name:     "Invoke my-auth-list-table-ids-tool without auth token",
			toolName: "my-auth-list-table-ids-tool",
			args:     datasetArgs(),
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-list-table-ids-tool with auth token",
			toolName:      "my-client-auth-list-table-ids-tool",
			requestHeader: map[string]string{"Authorization": accessToken},
			args:          datasetArgs(),
			wantContains:  tablename_want,
		},
		{
			name:     "Invoke my-client-auth-list-table-ids-tool without auth token",
			toolName: "my-client-auth-list-table-ids-tool",
			args:     datasetArgs(),
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-list-table-ids-tool with invalid auth token",
			toolName:      "my-client-auth-list-table-ids-tool",
			requestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			args:          datasetArgs(),
			isErr:         true,
		},
	}, opts...)
}

func runBigQueryGetTableInfoToolInvokeTest(t *testing.T, datasetName, tableName, tableInfoWant string, opts ...tests.ToolExecOption) {
	// Get ID token
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Get access token
	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	tableArgs := func() map[string]any {
		return map[string]any{"dataset": datasetName, "table": tableName}
	}

	// Test tool invoke endpoint
	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:     "invoke my-get-table-info-tool without body",
			toolName: "my-get-table-info-tool",
			args:     map[string]any{},
			isErr:    true,
		},
		{
			name:         "invoke my-get-table-info-tool",
			toolName:     "my-get-table-info-tool",
			args:         tableArgs(),
			wantContains: tableInfoWant,
		},
		{
			name:     "invoke my-auth-get-table-info-tool without body",
			toolName: "my-get-table-info-tool",
			args:     map[string]any{},
			isErr:    true,
		},
		{
			name:          "Invoke my-auth-get-table-info-tool with auth token",
			toolName:      "my-auth-get-table-info-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          tableArgs(),
			wantContains:  tableInfoWant,
		},
		{
			name:          "Invoke my-auth-get-table-info-tool with correct project",
			toolName:      "my-auth-get-table-info-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"project": BigqueryProject, "dataset": datasetName, "table": tableName},
			wantContains:  tableInfoWant,
		},
		{
			name:          "Invoke my-auth-get-table-info-tool with non-existent project",
			toolName:      "my-auth-get-table-info-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"project": fmt.Sprintf("%s-%s", BigqueryProject, uuid.NewString()), "dataset": datasetName, "table": tableName},
			isErr:         true,
		},
		{
			name:          "Invoke my-auth-get-table-info-tool with invalid auth token",
			toolName:      "my-auth-get-table-info-tool",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			args:          tableArgs(),
			isErr:         true,
		},
		{
			name:     "Invoke my-auth-get-table-info-tool without auth token",
			toolName: "my-auth-get-table-info-tool",
			args:     tableArgs(),
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-get-table-info-tool with auth token",
			toolName:      "my-client-auth-get-table-info-tool",
			requestHeader: map[string]string{"Authorization": accessToken},
			args:          tableArgs(),
			wantContains:  tableInfoWant,
		},
		{
			name:     "Invoke my-client-auth-get-table-info-tool without auth token",
			toolName: "my-client-auth-get-table-info-tool",
			args:     tableArgs(),
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-get-table-info-tool with invalid auth token",
			toolName:      "my-client-auth-get-table-info-tool",
			requestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			args:          tableArgs(),
			isErr:         true,
		},
	}, opts...)
}

// runBigQueryConversationalAnalyticsInvokeTest runs the conversational analytics cases.
// By default they run against the legacy /api endpoint; pass tests.WithMCPExec() to run them over MCP.
func runBigQueryConversationalAnalyticsInvokeTest(t *testing.T, datasetName, tableName, dataInsightsWant string, opts ...tests.ToolExecOption) {
	config := &tests.ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	// Each test is expected to complete in under 10s, we set a 25s timeout with retries to avoid flaky tests.
	const maxRetries = 3
	const requestTimeout = 340 * time.Second
	// Get ID token
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Get access token
	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	tableRefsJSON := fmt.Sprintf(`[{"projectId":"%s","datasetId":"%s","tableId":"%s"}]`, BigqueryProject, datasetName, tableName)
	askArgs := func() map[string]any {
		return map[string]any{"user_query_with_context": "What are the names in the table?", "table_references": tableRefsJSON}
	}

	invokeTcs := []bigQueryInvokeTestCase{
		{
			name:     "invoke my-conversational-analytics-tool successfully",
			toolName: "my-conversational-analytics-tool",
			args:     askArgs(),
			want:     dataInsightsWant,
		},
		{
			name:          "invoke my-auth-conversational-analytics-tool with auth token",
			toolName:      "my-auth-conversational-analytics-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          askArgs(),
			want:          dataInsightsWant,
		},
		{
			name:     "invoke my-auth-conversational-analytics-tool without auth token",
			toolName: "my-auth-conversational-analytics-tool",
			args:     map[string]any{"user_query_with_context": "What are the names in the table?"},
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-conversational-analytics-tool with auth token",
			toolName:      "my-client-auth-conversational-analytics-tool",
			requestHeader: map[string]string{"Authorization": accessToken},
			args:          askArgs(),
			want:          dataInsightsWant,
		},
		{
			name:     "Invoke my-client-auth-conversational-analytics-tool without auth token",
			toolName: "my-client-auth-conversational-analytics-tool",
			args:     askArgs(),
			isErr:    true,
		},
		{
			name:          "Invoke my-client-auth-conversational-analytics-tool with invalid auth token",
			toolName:      "my-client-auth-conversational-analytics-tool",
			requestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			args:          askArgs(),
			isErr:         true,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var failed bool
			var errText, got, describe string

			for i := 0; i < maxRetries; i++ {
				ctx, cancel := context.WithTimeout(t.Context(), requestTimeout)
				statusCode, timedOut := 0, false

				if config.IsMCP() {
					result := callMCPTool(t, ctx, mcpEndpoint, tc.toolName, tc.args, tc.requestHeader)
					cancel()
					statusCode, timedOut = result.statusCode, result.resp == nil && os.IsTimeout(result.err)
					failed, errText, got, describe = result.failed(), result.errorText(), result.resultJSON(), result.String()
					if !timedOut && statusCode == 0 {
						t.Fatalf("unable to send request: %v", result.err)
					}
				} else {
					api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
					reqBytes, err := json.Marshal(tc.args)
					if err != nil {
						cancel()
						t.Fatalf("error marshalling request body: %v", err)
					}
					req, err := http.NewRequestWithContext(ctx, http.MethodPost, api, bytes.NewBuffer(reqBytes))
					if err != nil {
						cancel()
						t.Fatalf("unable to create request: %s", err)
					}
					req.Header.Set("Content-type", "application/json")
					for k, v := range tc.requestHeader {
						req.Header.Add(k, v)
					}
					resp, err := http.DefaultClient.Do(req)
					cancel()
					if err != nil {
						if os.IsTimeout(err) {
							timedOut = true
						} else {
							t.Fatalf("unable to send request: %s", err)
						}
					} else {
						statusCode = resp.StatusCode
						bodyBytes, readErr := io.ReadAll(resp.Body)
						resp.Body.Close()
						if readErr != nil {
							t.Fatalf("unable to read response: %s", readErr)
						}
						var body map[string]any
						if statusCode == http.StatusOK {
							if err := json.Unmarshal(bodyBytes, &body); err != nil {
								t.Fatalf("error parsing response body: %v", err)
							}
						}
						got, _ = body["result"].(string)
						failed = statusCode != http.StatusOK || strings.Contains(got, `{"error":`)
						errText, describe = string(bodyBytes), string(bodyBytes)
					}
				}

				if timedOut {
					t.Logf("Request timed out (attempt %d/%d), retrying...", i+1, maxRetries)
					time.Sleep(5 * time.Second)
					continue
				}
				if statusCode == http.StatusServiceUnavailable {
					t.Logf("Received 503 Service Unavailable (attempt %d/%d), retrying...", i+1, maxRetries)
					time.Sleep(15 * time.Second)
					continue
				}
				break
			}

			if tc.isErr {
				if !failed {
					t.Fatalf("expected %s to fail, got %s", tc.toolName, describe)
				}
				return
			}
			if failed {
				t.Fatalf("%s failed: %s", tc.toolName, errText)
			}
			if !regexp.MustCompile(tc.want).MatchString(got) {
				t.Fatalf("response did not match the expected pattern.\nFull response:\n%s", got)
			}
		})
	}
}

// runListDatasetIdsWithRestriction verifies that only the allowed datasets are listed.
// By default it runs against the legacy /api endpoint; pass tests.WithMCPExec() to run it over MCP.
func runListDatasetIdsWithRestriction(t *testing.T, allowedDatasetName1, allowedDatasetName2 string, opts ...tests.ToolExecOption) {
	config := &tests.ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	testCases := []struct {
		name         string
		wantElements []string
	}{
		{
			name: "invoke list-dataset-ids with restriction",
			wantElements: []string{
				fmt.Sprintf("%s.%s", BigqueryProject, allowedDatasetName1),
				fmt.Sprintf("%s.%s", BigqueryProject, allowedDatasetName2),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var gotJSON string
			if config.IsMCP() {
				result := invokeMCPTool(t, "list-dataset-ids-restricted", map[string]any{}, nil)
				if result.failed() {
					t.Fatalf("list-dataset-ids-restricted failed: %s", result)
				}
				gotJSON = result.resultJSON()
			} else {
				body := bytes.NewBuffer([]byte(`{}`))
				resp, bodyBytes := tests.RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/api/tool/list-dataset-ids-restricted/invoke", body, nil)
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("unexpected status code: got %d, want %d. Body: %s", resp.StatusCode, http.StatusOK, string(bodyBytes))
				}
				var respBody map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &respBody); err != nil {
					t.Fatalf("error parsing response body: %v", err)
				}
				str, ok := respBody["result"].(string)
				if !ok {
					t.Fatalf("unable to find 'result' as a string in response body: %s", string(bodyBytes))
				}
				gotJSON = str
			}

			// Unmarshal the result into a slice to compare contents.
			var gotElements []string
			if err := json.Unmarshal([]byte(gotJSON), &gotElements); err != nil {
				t.Fatalf("error parsing result field JSON %q: %v", gotJSON, err)
			}

			sort.Strings(gotElements)
			sort.Strings(tc.wantElements)
			if !reflect.DeepEqual(gotElements, tc.wantElements) {
				t.Errorf("unexpected result:\n got: %v\nwant: %v", gotElements, tc.wantElements)
			}
		})
	}
}

// runListTableIdsWithRestriction verifies that only tables in allowed datasets are listed.
// By default it runs against the legacy /api endpoint; pass tests.WithMCPExec() to run it over MCP.
func runListTableIdsWithRestriction(t *testing.T, allowedDatasetName, disallowedDatasetName string, allowedTableNames ...string) {
	runListTableIdsWithRestrictionOpts(t, allowedDatasetName, disallowedDatasetName, nil, allowedTableNames...)
}

// runListTableIdsWithRestrictionOpts is runListTableIdsWithRestriction with endpoint options.
func runListTableIdsWithRestrictionOpts(t *testing.T, allowedDatasetName, disallowedDatasetName string, opts []tests.ToolExecOption, allowedTableNames ...string) {
	config := &tests.ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	sort.Strings(allowedTableNames)
	var quotedNames []string
	for _, name := range allowedTableNames {
		quotedNames = append(quotedNames, fmt.Sprintf(`"%s"`, name))
	}
	wantResult := fmt.Sprintf(`[%s]`, strings.Join(quotedNames, ","))

	testCases := []struct {
		name         string
		dataset      string
		wantInResult string
		wantInError  string
	}{
		{
			name:         "invoke on allowed dataset",
			dataset:      allowedDatasetName,
			wantInResult: wantResult,
		},
		{
			name:        "invoke on disallowed dataset",
			dataset:     disallowedDatasetName,
			wantInError: fmt.Sprintf("access denied to dataset '%s'", disallowedDatasetName),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var gotJSON, errText string
			var failed bool
			if config.IsMCP() {
				result := invokeMCPTool(t, "list-table-ids-restricted", map[string]any{"dataset": tc.dataset}, nil)
				failed, errText, gotJSON = result.failed(), result.errorText(), result.resultJSON()
			} else {
				body := bytes.NewBuffer([]byte(fmt.Sprintf(`{"dataset":"%s"}`, tc.dataset)))
				resp, bodyBytes := tests.RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/api/tool/list-table-ids-restricted/invoke", body, nil)
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("unexpected status code: got %d, want %d. Body: %s", resp.StatusCode, http.StatusOK, string(bodyBytes))
				}
				var respBody map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &respBody); err != nil {
					t.Fatalf("error parsing response body: %v", err)
				}
				gotJSON, _ = respBody["result"].(string)
				errText = string(bodyBytes)
				failed = strings.Contains(gotJSON, `{"error":`)
			}

			if tc.wantInError != "" {
				if !failed || !strings.Contains(errText, tc.wantInError) {
					t.Errorf("unexpected error message: got %q, want to contain %q", errText, tc.wantInError)
				}
				return
			}

			if failed {
				t.Fatalf("list-table-ids-restricted failed: %s", errText)
			}
			var gotSlice []string
			if err := json.Unmarshal([]byte(gotJSON), &gotSlice); err != nil {
				t.Fatalf("error unmarshalling result: %v", err)
			}
			sort.Strings(gotSlice)
			sortedGotBytes, err := json.Marshal(gotSlice)
			if err != nil {
				t.Fatalf("error marshalling sorted result: %v", err)
			}

			if string(sortedGotBytes) != tc.wantInResult {
				t.Errorf("unexpected result: got %q, want %q", string(sortedGotBytes), tc.wantInResult)
			}
		})
	}
}

func runGetDatasetInfoWithRestriction(t *testing.T, allowedDatasetName, disallowedDatasetName string, opts ...tests.ToolExecOption) {
	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:     "invoke on allowed dataset",
			toolName: "get-dataset-info-restricted",
			args:     map[string]any{"dataset": allowedDatasetName},
		},
		{
			name:     "invoke on disallowed dataset",
			toolName: "get-dataset-info-restricted",
			args:     map[string]any{"dataset": disallowedDatasetName},
			wantErr:  fmt.Sprintf("access denied to dataset '%s'", disallowedDatasetName),
		},
	}, opts...)
}

func runGetTableInfoWithRestriction(t *testing.T, allowedDatasetName, disallowedDatasetName, allowedTableName, disallowedTableName string, opts ...tests.ToolExecOption) {
	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:     "invoke on allowed table",
			toolName: "get-table-info-restricted",
			args:     map[string]any{"dataset": allowedDatasetName, "table": allowedTableName},
		},
		{
			name:     "invoke on disallowed table",
			toolName: "get-table-info-restricted",
			args:     map[string]any{"dataset": disallowedDatasetName, "table": disallowedTableName},
			wantErr:  fmt.Sprintf("access denied to dataset '%s'", disallowedDatasetName),
		},
	}, opts...)
}

func runExecuteSqlWithRestriction(t *testing.T, allowedTableFullName, disallowedTableFullName string, opts ...tests.ToolExecOption) {
	allowedTableParts := strings.Split(strings.Trim(allowedTableFullName, "`"), ".")
	if len(allowedTableParts) != 3 {
		t.Fatalf("invalid allowed table name format: %s", allowedTableFullName)
	}
	allowedDatasetID := allowedTableParts[1]

	sqlCase := func(name, sql, wantErr string) bigQueryInvokeTestCase {
		return bigQueryInvokeTestCase{name: name, toolName: "execute-sql-restricted", args: map[string]any{"sql": sql}, wantErr: wantErr}
	}

	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		sqlCase("invoke on allowed table", fmt.Sprintf("SELECT * FROM %s", allowedTableFullName), ""),
		sqlCase("invoke on disallowed table", fmt.Sprintf("SELECT * FROM %s", disallowedTableFullName),
			fmt.Sprintf("query accesses dataset '%s', which is not in the allowed list",
				strings.Join(
					strings.Split(strings.Trim(disallowedTableFullName, "`"), ".")[0:2],
					"."))),
		sqlCase("disallowed create schema", "CREATE SCHEMA another_dataset",
			"dataset-level operations like 'CREATE_SCHEMA' are not allowed"),
		sqlCase("disallowed alter schema", fmt.Sprintf("ALTER SCHEMA %s SET OPTIONS(description='new one')", allowedDatasetID),
			"dataset-level operations like 'ALTER_SCHEMA' are not allowed"),
		sqlCase("disallowed create function", fmt.Sprintf("CREATE FUNCTION %s.my_func() RETURNS INT64 AS (1)", allowedDatasetID),
			"creating stored routines ('CREATE_FUNCTION') is not allowed"),
		sqlCase("disallowed create procedure", fmt.Sprintf("CREATE PROCEDURE %s.my_proc() BEGIN SELECT 1; END", allowedDatasetID),
			"unanalyzable statements like 'CREATE PROCEDURE' are not allowed"),
		sqlCase("disallowed execute immediate", "EXECUTE IMMEDIATE 'SELECT 1'",
			"EXECUTE IMMEDIATE is not allowed when dataset restrictions are in place"),
	}, opts...)
}

func runConversationalAnalyticsWithRestriction(t *testing.T, allowedDatasetName, disallowedDatasetName, allowedTableName, disallowedTableName string, opts ...tests.ToolExecOption) {
	allowedTableRefsJSON := fmt.Sprintf(`[{"projectId":"%s","datasetId":"%s","tableId":"%s"}]`, BigqueryProject, allowedDatasetName, allowedTableName)
	disallowedTableRefsJSON := fmt.Sprintf(`[{"projectId":"%s","datasetId":"%s","tableId":"%s"}]`, BigqueryProject, disallowedDatasetName, disallowedTableName)

	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:         "invoke with allowed table",
			toolName:     "conversational-analytics-restricted",
			args:         map[string]any{"user_query_with_context": "What is in the table?", "table_references": allowedTableRefsJSON},
			wantContains: `FINAL_RESPONSE`,
		},
		{
			name:     "invoke with disallowed table",
			toolName: "conversational-analytics-restricted",
			args:     map[string]any{"user_query_with_context": "What is in the table?", "table_references": disallowedTableRefsJSON},
			wantErr:  fmt.Sprintf("access to dataset '%s.%s' (from table '%s') is not allowed", BigqueryProject, disallowedDatasetName, disallowedTableName),
		},
	}, opts...)
}

func runForecastWithRestriction(t *testing.T, allowedTableFullName, disallowedTableFullName string, opts ...tests.ToolExecOption) {
	allowedTableUnquoted := strings.ReplaceAll(allowedTableFullName, "`", "")
	disallowedTableUnquoted := strings.ReplaceAll(disallowedTableFullName, "`", "")
	disallowedDatasetFQN := strings.Join(strings.Split(disallowedTableUnquoted, ".")[0:2], ".")

	forecastArgs := func(historyData, timestampCol, dataCol string) map[string]any {
		return map[string]any{"history_data": historyData, "timestamp_col": timestampCol, "data_col": dataCol}
	}

	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:         "invoke with allowed table name",
			toolName:     "forecast-restricted",
			args:         forecastArgs(allowedTableUnquoted, "ts", "data"),
			wantContains: `"forecast_timestamp"`,
		},
		{
			name:     "invoke with disallowed table name",
			toolName: "forecast-restricted",
			args:     forecastArgs(disallowedTableUnquoted, "ts", "data"),
			wantErr:  fmt.Sprintf("access to dataset '%s' (from table '%s') is not allowed", disallowedDatasetFQN, disallowedTableUnquoted),
		},
		{
			name:         "invoke with query on allowed table",
			toolName:     "forecast-restricted",
			args:         forecastArgs(fmt.Sprintf("SELECT * FROM %s", allowedTableFullName), "ts", "data"),
			wantContains: `"forecast_timestamp"`,
		},
		{
			name:     "invoke with query on disallowed table",
			toolName: "forecast-restricted",
			args:     forecastArgs(fmt.Sprintf("SELECT * FROM %s", disallowedTableFullName), "ts", "data"),
			wantErr:  fmt.Sprintf("query accesses dataset '%s', which is not in the allowed list", disallowedDatasetFQN),
		},
		{
			name:     "invoke with SQL injection in timestamp_col",
			toolName: "forecast-restricted",
			args:     forecastArgs(allowedTableUnquoted, "ts', horizon => 5) --", "data"),
			wantErr:  `invalid column name for 'timestamp_col': "'ts'', horizon => 5) --'"; must match [a-zA-Z_][a-zA-Z0-9_]*`,
		},
		{
			name:     "invoke with SQL injection in data_col",
			toolName: "forecast-restricted",
			args:     forecastArgs(allowedTableUnquoted, "ts", "data', horizon => 5) --"),
			wantErr:  `invalid column name for 'data_col': "'data'', horizon => 5) --'"; must match [a-zA-Z_][a-zA-Z0-9_]*`,
		},
	}, opts...)
}

func runAnalyzeContributionWithRestriction(t *testing.T, allowedTableFullName, disallowedTableFullName string, opts ...tests.ToolExecOption) {
	allowedTableUnquoted := strings.ReplaceAll(allowedTableFullName, "`", "")
	disallowedTableUnquoted := strings.ReplaceAll(disallowedTableFullName, "`", "")
	disallowedDatasetFQN := strings.Join(strings.Split(disallowedTableUnquoted, ".")[0:2], ".")

	contributionArgs := func(inputData, contributionMetric, isTestCol string, dimensionIdCols []any) map[string]any {
		return map[string]any{
			"input_data":          inputData,
			"contribution_metric": contributionMetric,
			"is_test_col":         isTestCol,
			"dimension_id_cols":   dimensionIdCols,
		}
	}
	defaultDims := []any{"dim1", "dim2"}

	runBigQueryInvokeTestCases(t, []bigQueryInvokeTestCase{
		{
			name:         "invoke with allowed table name",
			toolName:     "analyze-contribution-restricted",
			args:         contributionArgs(allowedTableUnquoted, "SUM(metric)", "is_test", defaultDims),
			wantContains: `"relative_difference"`,
		},
		{
			name:     "invoke with disallowed table name",
			toolName: "analyze-contribution-restricted",
			args:     contributionArgs(disallowedTableUnquoted, "SUM(metric)", "is_test", defaultDims),
			wantErr:  fmt.Sprintf("access to dataset '%s' (from table '%s') is not allowed", disallowedDatasetFQN, disallowedTableUnquoted),
		},
		{
			name:         "invoke with query on allowed table",
			toolName:     "analyze-contribution-restricted",
			args:         contributionArgs(fmt.Sprintf("SELECT * FROM %s", allowedTableFullName), "SUM(metric)", "is_test", defaultDims),
			wantContains: `"relative_difference"`,
		},
		{
			name:     "invoke with query on disallowed table",
			toolName: "analyze-contribution-restricted",
			args:     contributionArgs(fmt.Sprintf("SELECT * FROM %s", disallowedTableFullName), "SUM(metric)", "is_test", defaultDims),
			wantErr:  fmt.Sprintf("query accesses dataset '%s', which is not in the allowed list", disallowedDatasetFQN),
		},
		{
			name:     "invoke with SQL injection in is_test_col",
			toolName: "analyze-contribution-restricted",
			args:     contributionArgs(allowedTableUnquoted, "SUM(metric)", "is_test; drop table x", defaultDims),
			wantErr:  `invalid column name for 'is_test_col': "'is_test; drop table x'"; must match [a-zA-Z_][a-zA-Z0-9_]*`,
		},
		{
			name:     "invoke with SQL injection in dimension_id_cols",
			toolName: "analyze-contribution-restricted",
			args:     contributionArgs(allowedTableUnquoted, "SUM(metric)", "is_test", []any{"dim1", "dim2; drop table x"}),
			wantErr:  `invalid column name in 'dimension_id_cols': "'dim2; drop table x'"; must match [a-zA-Z_][a-zA-Z0-9_]*`,
		},
		{
			name:     "invoke with single quote in contribution_metric",
			toolName: "analyze-contribution-restricted",
			args:     contributionArgs(allowedTableUnquoted, "SUM('metric')", "is_test", defaultDims),
			wantErr:  `invalid 'contribution_metric': must not contain single quotes`,
		},
	}, opts...)
}

// setupBigQueryVectorTable creates a vector table in BigQuery for semantic search testing
func setupBigQueryVectorTable(t *testing.T, ctx context.Context, client *bigqueryapi.Client, datasetName string) (string, func(*testing.T)) {
	tableName := fmt.Sprintf("vector_table_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	fullTableName := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, datasetName, tableName)
	createStatement := fmt.Sprintf(`CREATE TABLE %s (
		id INT64,
		content STRING,
		embedding ARRAY<FLOAT64>
	)`, fullTableName)

	teardownTable := setupBigQueryTable(t, ctx, client, createStatement, "", datasetName, fullTableName, nil)
	return fullTableName, teardownTable
}

// getBigQueryVectorSearchStmts returns statements for bigquery semantic search
func getBigQueryVectorSearchStmts(vectorTableName string) (string, string) {
	insertStmt := fmt.Sprintf("INSERT INTO %s (id, content, embedding) VALUES (1, @content, @text_to_embed)", vectorTableName)
	searchStmt := fmt.Sprintf("SELECT id, content, ML.DISTANCE(embedding, @query, 'COSINE') AS distance FROM %s ORDER BY distance LIMIT 1", vectorTableName)
	return insertStmt, searchStmt
}
