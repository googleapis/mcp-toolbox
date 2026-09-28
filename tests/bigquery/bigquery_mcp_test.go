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

package bigquery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// mcpEndpoint is the MCP endpoint of the Toolbox server started on the default port.
const mcpEndpoint = "http://127.0.0.1:5000/mcp"

// bigQueryInvokeTestCase describes a tool invocation and its expected outcome on either endpoint.
type bigQueryInvokeTestCase struct {
	name          string
	toolName      string
	requestHeader map[string]string
	args          map[string]any
	want          string // exact match on the result
	wantMCP       string // exact match on the MCP result, when it differs from want
	wantContains  string // substring of the result
	wantErr       string // substring of the error message
	isErr         bool   // any failure is expected
}

// runBigQueryInvokeTestCases invokes each test case and checks its outcome. By default the
// cases run against the legacy /api endpoint; pass tests.WithMCPExec() to run them over MCP.
func runBigQueryInvokeTestCases(t *testing.T, tcs []bigQueryInvokeTestCase, opts ...tests.ToolExecOption) {
	t.Helper()
	config := &tests.ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if config.IsMCP() {
				result := invokeMCPTool(t, tc.toolName, tc.args, tc.requestHeader)
				want := tc.want
				if tc.wantMCP != "" {
					want = tc.wantMCP
				}
				checkBigQueryResult(t, tc, result.failed(), result.errorText(), result.resultJSON(), want, result.String())
				return
			}

			// Legacy REST path
			api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
			reqBytes, err := json.Marshal(tc.args)
			if err != nil {
				t.Fatalf("error marshalling request body: %v", err)
			}
			resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer(reqBytes), tc.requestHeader)
			if resp.StatusCode != http.StatusOK {
				if tc.isErr || tc.wantErr != "" {
					return
				}
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
			}
			var body map[string]any
			if err := json.Unmarshal(respBody, &body); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			got, _ := body["result"].(string)
			// The /api endpoint reports tool errors as a JSON object inside the result string.
			failed := strings.Contains(got, `{"error":`)
			errText := got
			var inner map[string]any
			if err := json.Unmarshal([]byte(got), &inner); err == nil {
				if msg, ok := inner["error"].(string); ok {
					errText = msg
				}
			}
			checkBigQueryResult(t, tc, failed, errText, got, tc.want, string(respBody))
		})
	}
}

// checkBigQueryResult asserts one endpoint-neutral expectation against a tool result.
func checkBigQueryResult(t *testing.T, tc bigQueryInvokeTestCase, failed bool, errText, got, want, describe string) {
	t.Helper()
	switch {
	case tc.isErr:
		if !failed {
			t.Fatalf("expected %s to fail, got %s", tc.toolName, describe)
		}
	case tc.wantErr != "":
		if !failed || !strings.Contains(errText, tc.wantErr) {
			t.Fatalf("expected %s to fail with %q, got %s", tc.toolName, tc.wantErr, describe)
		}
	default:
		if failed {
			t.Fatalf("%s failed: %s", tc.toolName, describe)
		}
		if want != "" && got != want {
			t.Fatalf("unexpected value: got %q, want %q", got, want)
		}
		if !strings.Contains(got, tc.wantContains) {
			t.Fatalf("expected %q to contain %q, but it did not", got, tc.wantContains)
		}
	}
}

// mcpToolResult is the outcome of an MCP tools/call request.
type mcpToolResult struct {
	statusCode int
	resp       *tests.MCPCallToolResponse
	err        error
}

// callMCPTool sends a tools/call request to the MCP endpoint at url. Unlike tests.InvokeMCPTool,
// it accepts a context and port, and reports transport and parse errors instead of failing the test.
func callMCPTool(t *testing.T, ctx context.Context, url, toolName string, args map[string]any, headers map[string]string) mcpToolResult {
	reqBody, err := json.Marshal(tests.NewMCPCallToolRequest(uuid.NewString(), toolName, args))
	if err != nil {
		return mcpToolResult{err: fmt.Errorf("unable to marshal request: %w", err)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return mcpToolResult{err: fmt.Errorf("unable to create request: %w", err)}
	}
	for k, v := range tests.NewMCPRequestHeader(t, headers) {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return mcpToolResult{err: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcpToolResult{statusCode: resp.StatusCode, err: fmt.Errorf("unable to read response: %w", err)}
	}
	var mcpResp tests.MCPCallToolResponse
	if err := json.Unmarshal(body, &mcpResp); err != nil {
		return mcpToolResult{statusCode: resp.StatusCode, err: fmt.Errorf("unable to parse response %q: %w", string(body), err)}
	}
	return mcpToolResult{statusCode: resp.StatusCode, resp: &mcpResp}
}

// invokeMCPTool calls toolName on the Toolbox server started on the default port.
func invokeMCPTool(t *testing.T, toolName string, args map[string]any, headers map[string]string) mcpToolResult {
	t.Helper()
	return callMCPTool(t, t.Context(), mcpEndpoint, toolName, args, headers)
}

// failed reports whether the call failed at the HTTP, JSON-RPC or tool level.
func (r mcpToolResult) failed() bool {
	return r.err != nil || r.statusCode != http.StatusOK || r.resp.Error != nil || r.resp.Result.IsError
}

// errorText returns the transport error, the JSON-RPC error message, or the tool error content.
func (r mcpToolResult) errorText() string {
	switch {
	case r.err != nil:
		return r.err.Error()
	case r.resp.Error != nil:
		return r.resp.Error.Message
	}
	texts := make([]string, 0, len(r.resp.Result.Content))
	for _, c := range r.resp.Result.Content {
		texts = append(texts, c.Text)
	}
	return strings.Join(texts, "\n")
}

// resultJSON returns the result as a JSON array, which matches the legacy /api result for row results.
// Each row arrives as a separate content block, and any other result arrives as a single block.
func (r mcpToolResult) resultJSON() string {
	if r.resp == nil {
		return ""
	}
	content := r.resp.Result.Content
	if len(content) == 1 && strings.HasPrefix(content[0].Text, "[") {
		return content[0].Text
	}
	texts := make([]string, 0, len(content))
	for _, c := range content {
		texts = append(texts, c.Text)
	}
	return "[" + strings.Join(texts, ",") + "]"
}

func (r mcpToolResult) String() string {
	if r.failed() {
		return fmt.Sprintf("status %d, error: %s", r.statusCode, r.errorText())
	}
	return fmt.Sprintf("status %d, result: %s", r.statusCode, r.resultJSON())
}

// setupBigQueryMCPServer seeds the test data and starts a Toolbox server serving the
// BigQuery tools over the MCP endpoint. It returns the dataset and table names the
// tests need, and a teardown function.
func setupBigQueryMCPServer(t *testing.T, ctx context.Context) (bigQueryMCPFixture, func()) {
	sourceConfig := getBigQueryVars(t)
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Logf("Starting BigQuery MCP test with uniqueID: %s", uniqueID)

	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery client: %s", err)
	}

	f := bigQueryMCPFixture{
		datasetName: fmt.Sprintf("temp_toolbox_test_%s", uniqueID),
		tableName:   fmt.Sprintf("param_table_%s", uniqueID),
	}
	f.tableNameParam = fmt.Sprintf("`%s.%s.%s`", BigqueryProject, f.datasetName, f.tableName)
	tableNameAuth := fmt.Sprintf("`%s.%s.auth_table_%s`", BigqueryProject, f.datasetName, uniqueID)
	f.tableNameTemplateParam = fmt.Sprintf("`%s.%s.template_param_table_%s`", BigqueryProject, f.datasetName, uniqueID)
	tableNameDataType := fmt.Sprintf("`%s.%s.datatype_table_%s`", BigqueryProject, f.datasetName, uniqueID)
	f.tableNameForecast = fmt.Sprintf("`%s.%s.forecast_table_%s`", BigqueryProject, f.datasetName, uniqueID)
	f.tableNameAnalyzeContribution = fmt.Sprintf("`%s.%s.analyze_contribution_table_%s`", BigqueryProject, f.datasetName, uniqueID)

	t.Cleanup(func() {
		tests.CleanupBigQueryDatasets(t, context.Background(), client, []string{f.datasetName})
	})

	createParamTableStmt, insertParamTableStmt, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getBigQueryParamToolInfo(f.tableNameParam)
	setupBigQueryTable(t, ctx, client, createParamTableStmt, insertParamTableStmt, f.datasetName, f.tableNameParam, paramTestParams)

	createAuthTableStmt, insertAuthTableStmt, authToolStmt, authTestParams := getBigQueryAuthToolInfo(tableNameAuth)
	setupBigQueryTable(t, ctx, client, createAuthTableStmt, insertAuthTableStmt, f.datasetName, tableNameAuth, authTestParams)

	createDataTypeTableStmt, insertDataTypeTableStmt, dataTypeToolStmt, arrayDataTypeToolStmt, dataTypeTestParams := getBigQueryDataTypeTestInfo(tableNameDataType)
	setupBigQueryTable(t, ctx, client, createDataTypeTableStmt, insertDataTypeTableStmt, f.datasetName, tableNameDataType, dataTypeTestParams)

	createForecastTableStmt, insertForecastTableStmt, forecastTestParams := getBigQueryForecastToolInfo(f.tableNameForecast)
	setupBigQueryTable(t, ctx, client, createForecastTableStmt, insertForecastTableStmt, f.datasetName, f.tableNameForecast, forecastTestParams)

	createAnalyzeContributionTableStmt, insertAnalyzeContributionTableStmt, analyzeContributionTestParams := getBigQueryAnalyzeContributionToolInfo(f.tableNameAnalyzeContribution)
	setupBigQueryTable(t, ctx, client, createAnalyzeContributionTableStmt, insertAnalyzeContributionTableStmt, f.datasetName, f.tableNameAnalyzeContribution, analyzeContributionTestParams)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, BigqueryToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = addClientAuthSourceConfig(t, toolsFile)
	toolsFile = addBigQuerySqlToolConfig(t, toolsFile, dataTypeToolStmt, arrayDataTypeToolStmt)
	toolsFile = addBigQueryPrebuiltToolsConfig(t, toolsFile)
	tmplSelectCombined, tmplSelectFilterCombined := getBigQueryTmplToolStatement()
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, BigqueryToolType, tmplSelectCombined, tmplSelectFilterCombined, "")

	vectorTableName, teardownVectorTable := setupBigQueryVectorTable(t, ctx, client, f.datasetName)
	insertStmt, searchStmt := getBigQueryVectorSearchStmts(vectorTableName)
	toolsFile = tests.AddSemanticSearchConfig(t, toolsFile, BigqueryToolType, insertStmt, searchStmt)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	return f, func() {
		cleanup()
		teardownVectorTable(t)
	}
}

// bigQueryMCPFixture holds the names of the test data created for the MCP tests.
type bigQueryMCPFixture struct {
	datasetName                  string
	tableName                    string
	tableNameParam               string
	tableNameTemplateParam       string
	tableNameForecast            string
	tableNameAnalyzeContribution string
}

func TestBigQueryMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	_, teardown := setupBigQueryMCPServer(t, ctx)
	defer teardown()

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getBigQueryMCPExpectedTools())
	})
}

func TestBigQueryMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	f, teardown := setupBigQueryMCPServer(t, ctx)
	defer teardown()

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

	tests.RunToolInvokeTest(t, select1Want, tests.DisableOptionalNullParamTest(), tests.EnableClientAuthTest(), tests.WithMCP())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want, tests.EnableMcpClientAuthTest())
	// Over MCP, the template harness compares a non-row result as a single-element array.
	tests.RunToolInvokeWithTemplateParameters(t, f.tableNameTemplateParam,
		tests.WithCreateColArray(createColArray),
		tests.WithDdlWant("["+ddlWant+"]"),
		tests.WithSelectEmptyWant("["+selectEmptyWant+"]"),
		tests.WithInsert1Want("["+ddlWant+"]"),
		tests.WithMCPTemplate(),
	)

	runBigQueryExecuteSqlToolInvokeTest(t, select1Want, invokeParamWant, f.tableNameParam, ddlWant, tests.WithMCPExec())
	runBigQueryExecuteSqlToolInvokeDryRunTest(t, f.datasetName, tests.WithMCPExec())
	runBigQueryForecastToolInvokeTest(t, f.tableNameForecast, tests.WithMCPExec())
	runBigQueryAnalyzeContributionToolInvokeTest(t, f.tableNameAnalyzeContribution, tests.WithMCPExec())
	runBigQueryDataTypeTests(t, tests.WithMCPExec())
	runBigQueryListDatasetToolInvokeTest(t, f.datasetName, tests.WithMCPExec())
	runBigQueryGetDatasetInfoToolInvokeTest(t, f.datasetName, datasetInfoWant, tests.WithMCPExec())
	runBigQueryListTableIdsToolInvokeTest(t, f.datasetName, f.tableName, tests.WithMCPExec())
	runBigQueryGetTableInfoToolInvokeTest(t, f.datasetName, f.tableName, tableInfoWant, tests.WithMCPExec())
	runBigQueryConversationalAnalyticsInvokeTest(t, f.datasetName, f.tableName, dataInsightsWant, tests.WithMCPExec())
	tests.RunSearchCatalogToolTest(t, tests.SearchCatalogTestParams{
		ContainerParamName: "datasetIds",
		ContainerName:      f.datasetName,
		ProjectID:          BigqueryProject,
		TargetName:         f.tableName,
		WantKey:            "DisplayName",
		AllowEmpty:         false,
		CheckValue:         true,
	}, tests.WithMCPExec())
	tests.RunSemanticSearchToolInvokeTest(t, ddlWant, ddlWant, "The quick brown fox", tests.WithMCPExec())
}

func TestBigQueryMCPToolWithDatasetRestriction(t *testing.T) {
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Logf("Starting MCP restriction test with uniqueID: %s", uniqueID)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	client, err := initBigQueryConnection(BigqueryProject)
	if err != nil {
		t.Fatalf("unable to create BigQuery client: %s", err)
	}

	allowedDatasetName := fmt.Sprintf("allowed_dataset_mcp_%s", uniqueID)
	disallowedDatasetName := fmt.Sprintf("disallowed_dataset_mcp_%s", uniqueID)
	allowedTableName := "allowed_table"
	disallowedTableName := "disallowed_table"
	allowedForecastTableName := "allowed_forecast_table"
	disallowedForecastTableName := "disallowed_forecast_table"
	allowedAnalyzeContributionTableName := "allowed_analyze_contribution_table"
	disallowedAnalyzeContributionTableName := "disallowed_analyze_contribution_table"

	t.Cleanup(func() {
		tests.CleanupBigQueryDatasets(t, context.Background(), client, []string{allowedDatasetName, disallowedDatasetName})
	})

	allowedTableNameParam := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName, allowedTableName)
	setupBigQueryTable(t, ctx, client, fmt.Sprintf("CREATE TABLE %s (id INT64)", allowedTableNameParam), "", allowedDatasetName, allowedTableNameParam, nil)

	disallowedTableNameParam := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, disallowedDatasetName, disallowedTableName)
	setupBigQueryTable(t, ctx, client, fmt.Sprintf("CREATE TABLE %s (id INT64)", disallowedTableNameParam), "", disallowedDatasetName, disallowedTableNameParam, nil)

	allowedForecastTableFullName := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName, allowedForecastTableName)
	createForecastStmt, insertForecastStmt, forecastParams := getBigQueryForecastToolInfo(allowedForecastTableFullName)
	setupBigQueryTable(t, ctx, client, createForecastStmt, insertForecastStmt, allowedDatasetName, allowedForecastTableFullName, forecastParams)

	disallowedForecastTableFullName := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, disallowedDatasetName, disallowedForecastTableName)
	createDisallowedForecastStmt, insertDisallowedForecastStmt, disallowedForecastParams := getBigQueryForecastToolInfo(disallowedForecastTableFullName)
	setupBigQueryTable(t, ctx, client, createDisallowedForecastStmt, insertDisallowedForecastStmt, disallowedDatasetName, disallowedForecastTableFullName, disallowedForecastParams)

	allowedAnalyzeContributionTableFullName := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, allowedDatasetName, allowedAnalyzeContributionTableName)
	createAnalyzeContributionStmt, insertAnalyzeContributionStmt, analyzeContributionParams := getBigQueryAnalyzeContributionToolInfo(allowedAnalyzeContributionTableFullName)
	setupBigQueryTable(t, ctx, client, createAnalyzeContributionStmt, insertAnalyzeContributionStmt, allowedDatasetName, allowedAnalyzeContributionTableFullName, analyzeContributionParams)

	disallowedAnalyzeContributionTableFullName := fmt.Sprintf("`%s.%s.%s`", BigqueryProject, disallowedDatasetName, disallowedAnalyzeContributionTableName)
	createDisallowedAnalyzeContributionStmt, insertDisallowedAnalyzeContributionStmt, disallowedAnalyzeContributionParams := getBigQueryAnalyzeContributionToolInfo(disallowedAnalyzeContributionTableFullName)
	setupBigQueryTable(t, ctx, client, createDisallowedAnalyzeContributionStmt, insertDisallowedAnalyzeContributionStmt, disallowedDatasetName, disallowedAnalyzeContributionTableFullName, disallowedAnalyzeContributionParams)

	// Configure source with dataset restriction.
	sourceConfig := getBigQueryVars(t)
	sourceConfig["allowedDatasets"] = []string{allowedDatasetName}

	config := map[string]any{
		"sources": map[string]any{"my-instance": sourceConfig},
		"tools": map[string]any{
			"list-dataset-ids-restricted":         map[string]any{"type": "bigquery-list-dataset-ids", "source": "my-instance", "description": "Tool to list dataset ids"},
			"list-table-ids-restricted":           map[string]any{"type": "bigquery-list-table-ids", "source": "my-instance", "description": "Tool to list table within a dataset"},
			"get-dataset-info-restricted":         map[string]any{"type": "bigquery-get-dataset-info", "source": "my-instance", "description": "Tool to get dataset info"},
			"get-table-info-restricted":           map[string]any{"type": "bigquery-get-table-info", "source": "my-instance", "description": "Tool to get table info"},
			"execute-sql-restricted":              map[string]any{"type": "bigquery-execute-sql", "source": "my-instance", "description": "Tool to execute SQL"},
			"conversational-analytics-restricted": map[string]any{"type": "bigquery-conversational-analytics", "source": "my-instance", "description": "Tool to ask BigQuery conversational analytics"},
			"forecast-restricted":                 map[string]any{"type": "bigquery-forecast", "source": "my-instance", "description": "Tool to forecast"},
			"analyze-contribution-restricted":     map[string]any{"type": "bigquery-analyze-contribution", "source": "my-instance", "description": "Tool to analyze contribution"},
		},
	}

	cleanup := startBigQueryMCPCmd(t, ctx, config)
	defer cleanup()

	runListDatasetIdsWithRestriction(t, allowedDatasetName, allowedDatasetName, tests.WithMCPExec())
	runListTableIdsWithRestrictionOpts(t, allowedDatasetName, disallowedDatasetName, []tests.ToolExecOption{tests.WithMCPExec()},
		allowedTableName, allowedForecastTableName, allowedAnalyzeContributionTableName)
	runGetDatasetInfoWithRestriction(t, allowedDatasetName, disallowedDatasetName, tests.WithMCPExec())
	runGetTableInfoWithRestriction(t, allowedDatasetName, disallowedDatasetName, allowedTableName, disallowedTableName, tests.WithMCPExec())
	runExecuteSqlWithRestriction(t, allowedTableNameParam, disallowedTableNameParam, tests.WithMCPExec())
	runConversationalAnalyticsWithRestriction(t, allowedDatasetName, disallowedDatasetName, allowedTableName, disallowedTableName, tests.WithMCPExec())
	runForecastWithRestriction(t, allowedForecastTableFullName, disallowedForecastTableFullName, tests.WithMCPExec())
	runAnalyzeContributionWithRestriction(t, allowedAnalyzeContributionTableFullName, disallowedAnalyzeContributionTableFullName, tests.WithMCPExec())
}

func TestBigQueryMCPWriteModeAllowed(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	sourceConfig["writeMode"] = "allowed"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	datasetName := fmt.Sprintf("temp_toolbox_test_mcp_allowed_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
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
		"sources": map[string]any{"my-instance": sourceConfig},
		"tools": map[string]any{
			"my-exec-sql-tool": map[string]any{"type": "bigquery-execute-sql", "source": "my-instance", "description": "Tool to execute sql"},
		},
	}
	cleanup := startBigQueryMCPCmd(t, ctx, toolsFile)
	defer cleanup()

	runBigQueryWriteModeAllowedTest(t, datasetName, tests.WithMCPExec())
}

func TestBigQueryMCPWriteModeBlocked(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	sourceConfig["writeMode"] = "blocked"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	datasetName := fmt.Sprintf("temp_toolbox_test_mcp_blocked_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
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
	cleanup := startBigQueryMCPCmd(t, ctx, toolsFile)
	defer cleanup()

	runBigQueryWriteModeBlockedTest(t, tableNameParam, datasetName, tests.WithMCPExec())
}

func TestBigQueryMCPWriteModeProtected(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	sourceConfig["writeMode"] = "protected"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	permanentDatasetName := fmt.Sprintf("perm_dataset_mcp_protected_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
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
				"annotations": map[string]any{"readOnlyHint": true},
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
	cleanup := startBigQueryMCPCmd(t, ctx, toolsFile)
	defer cleanup()

	runBigQueryWriteModeProtectedTest(t, permanentDatasetName, tests.WithMCPExec())
}

// TestBigQueryMCPReadOnlyVulnerabilityBlock is the MCP counterpart of
// TestBigQuery_ReadOnlyVulnerabilityBlock: a tool falsely annotated as read-only on a
// readOnly source must still have its write rejected by BigQuery dry-run validation.
func TestBigQueryMCPReadOnlyVulnerabilityBlock(t *testing.T) {
	sourceConfig := getBigQueryVars(t)
	sourceConfig["readOnly"] = true

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	datasetName := fmt.Sprintf("temp_toolbox_test_mcp_vuln_%s", uniqueID)
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
		"sources": map[string]any{"my-readonly-bigquery-instance": sourceConfig},
		"tools": map[string]any{
			"vulnerable_write_tool": map[string]any{
				"type":        "bigquery-execute-sql",
				"source":      "my-readonly-bigquery-instance",
				"description": "I am a tool that tries to write but falsely claims to be read-only!",
				"annotations": map[string]any{"readOnlyHint": true},
			},
		},
	}
	cleanup := startBigQueryMCPCmd(t, ctx, toolsFile, "--port", "5003")
	defer cleanup()

	const url = "http://127.0.0.1:5003/mcp"

	// 1. Verify falsely annotated DDL write is rejected by dry-run defense
	writeResult := callMCPTool(t, t.Context(), url, "vulnerable_write_tool", map[string]any{"sql": fmt.Sprintf("CREATE TABLE %s.%s (id INT64);", datasetName, tableName)}, nil)
	errLower := strings.ToLower(writeResult.errorText())
	if !writeResult.failed() || (!strings.Contains(errLower, "blocked") && !strings.Contains(errLower, "only select statements are allowed")) {
		t.Fatalf("Vulnerability check failed! Expected BigQuery dry-run validation to reject the write query, but got %s", writeResult)
	}

	// 2. Verify valid SELECT read query succeeds on read-only source
	readResult := callMCPTool(t, t.Context(), url, "vulnerable_write_tool", map[string]any{"sql": "SELECT 1 AS result;"}, nil)
	if readResult.failed() {
		t.Fatalf("Expected valid SELECT query to succeed on read-only source, got %s", readResult)
	}
}

// startBigQueryMCPCmd starts a Toolbox server for the given config without --enable-api.
func startBigQueryMCPCmd(t *testing.T, ctx context.Context, toolsFile map[string]any, args ...string) func() {
	t.Helper()
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	waitCtx, cancelWait := context.WithTimeout(ctx, 30*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
	return func() {
		cleanup()
		cmd.Close()
	}
}

// getBigQueryMCPExpectedTools returns the MCP manifests for every tool loaded by TestBigQueryToolEndpoints.
func getBigQueryMCPExpectedTools() []tests.MCPToolManifest {
	expectedTools := tests.GetBaseMCPExpectedTools()
	expectedTools = append(expectedTools, tests.GetTemplateParamMCPExpectedTools()...)

	schema := func(properties map[string]any, required ...any) map[string]any {
		if required == nil {
			required = []any{}
		}
		return map[string]any{"type": "object", "properties": properties, "required": required}
	}
	typed := func(typ, description string) map[string]any {
		return map[string]any{"type": typ, "description": description}
	}
	withDefault := func(property map[string]any, value any) map[string]any {
		property["default"] = value
		return property
	}
	arrayOf := func(description, itemType, itemDescription string) map[string]any {
		return map[string]any{"type": "array", "description": description, "items": typed(itemType, itemDescription)}
	}
	projectParam := func(description string) map[string]any {
		return withDefault(typed("string", description), BigqueryProject)
	}

	expectedTools = append(expectedTools,
		tests.MCPToolManifest{
			Name:        "my-scalar-datatype-tool",
			Description: "Tool to test various scalar data types.",
			InputSchema: schema(map[string]any{
				"int_val":    typed("integer", "an integer value"),
				"string_val": typed("string", "a string value"),
				"float_val":  typed("number", "a float value"),
				"bool_val":   typed("boolean", "a boolean value"),
			}, "int_val", "string_val", "float_val", "bool_val"),
		},
		tests.MCPToolManifest{
			Name:        "my-array-datatype-tool",
			Description: "Tool to test various array data types.",
			InputSchema: schema(map[string]any{
				"int_array":    arrayOf("an array of integer values", "integer", "desc"),
				"string_array": arrayOf("an array of string values", "string", "desc"),
				"float_array":  arrayOf("an array of float values", "number", "desc"),
				"bool_array":   arrayOf("an array of boolean values", "boolean", "desc"),
			}, "int_array", "string_array", "float_array", "bool_array"),
		},
		tests.MCPToolManifest{
			Name:        "my-client-auth-tool",
			Description: "Tool to test client authorization.",
			InputSchema: schema(map[string]any{}),
		},
		tests.MCPToolManifest{
			Name:        "my-custom-client-auth-tool",
			Description: "Tool to test custom client authorization header.",
			InputSchema: schema(map[string]any{}),
		},
		tests.MCPToolManifest{
			Name:        "insert_docs",
			Description: "Stores content and its vector embedding into the documents table.",
			InputSchema: schema(map[string]any{
				"content": typed("string", "The text content associated with the vector."),
			}, "content"),
		},
		tests.MCPToolManifest{
			Name:        "search_docs",
			Description: "Finds the most semantically similar document to the query vector.",
			InputSchema: schema(map[string]any{
				"query": typed("string", "The text content to search for."),
			}, "query"),
		},
	)

	contributionMetricDescription := "The name of the column that contains the metric to analyze.\n" +
		"\t\tProvides the expression to use to calculate the metric you are analyzing.\n" +
		"\t\tTo calculate a summable metric, the expression must be in the form SUM(metric_column_name),\n" +
		"\t\twhere metric_column_name is a numeric data type.\n" +
		"\t\tTo calculate a summable ratio metric, the expression must be in the form\n" +
		"\t\tSUM(numerator_metric_column_name)/SUM(denominator_metric_column_name),\n" +
		"\t\twhere numerator_metric_column_name and denominator_metric_column_name are numeric data types.\n" +
		"\t\tTo calculate a summable by category metric, the expression must be in the form\n" +
		"\t\tSUM(metric_sum_column_name)/COUNT(DISTINCT categorical_column_name). The summed column must be a numeric data type.\n" +
		"\t\tThe categorical column must have type BOOL, DATE, DATETIME, TIME, TIMESTAMP, STRING, or INT64."

	// Each prebuilt tool is configured as my-<tool>, my-auth-<tool> and my-client-auth-<tool>.
	prebuiltTools := []struct {
		tool            string
		description     string
		authDescription string
		inputSchema     map[string]any
	}{
		{
			tool:            "exec-sql-tool",
			description:     "Tool to execute sql",
			authDescription: "Tool to execute sql",
			inputSchema: schema(map[string]any{
				"sql":     typed("string", "The SQL to execute."),
				"dry_run": withDefault(typed("boolean", "If set to true, the query will be validated and information about the execution will be returned without running the query. Defaults to false."), false),
			}, "sql"),
		},
		{
			tool:            "forecast-tool",
			description:     "Tool to forecast time series data.",
			authDescription: "Tool to forecast time series data with auth.",
			inputSchema: schema(map[string]any{
				"history_data":  typed("string", "The table id or the query of the history time series data."),
				"timestamp_col": typed("string", "The name of the time series timestamp column."),
				"data_col":      typed("string", "The name of the time series data column."),
				"id_cols":       withDefault(arrayOf("An array of the time series id column names.", "string", "The name of time series id column."), []any{}),
				"horizon":       withDefault(typed("integer", "The number of forecasting steps."), float64(10)),
			}, "history_data", "timestamp_col", "data_col"),
		},
		{
			tool:            "analyze-contribution-tool",
			description:     "Tool to analyze contribution.",
			authDescription: "Tool to analyze contribution with auth.",
			inputSchema: schema(map[string]any{
				"input_data":                        typed("string", "The data that contain the test and control data to analyze. Can be a fully qualified BigQuery table ID or a SQL query."),
				"contribution_metric":               typed("string", contributionMetricDescription),
				"is_test_col":                       typed("string", "The name of the column that identifies whether a row is in the test or control group."),
				"dimension_id_cols":                 arrayOf("An array of column names that uniquely identify each dimension.", "string", "A dimension column name."),
				"top_k_insights_by_apriori_support": withDefault(typed("integer", "The number of top insights to return, ranked by apriori support."), float64(30)),
				"pruning_method":                    withDefault(typed("string", "The method to use for pruning redundant insights. Can be 'NO_PRUNING' or 'PRUNE_REDUNDANT_INSIGHTS'."), "PRUNE_REDUNDANT_INSIGHTS"),
			}, "input_data", "contribution_metric", "is_test_col"),
		},
		{
			tool:            "list-dataset-ids-tool",
			description:     "Tool to list dataset",
			authDescription: "Tool to list dataset",
			inputSchema: schema(map[string]any{
				"project": projectParam("The Google Cloud project to list dataset ids."),
			}),
		},
		{
			tool:            "get-dataset-info-tool",
			description:     "Tool to show dataset metadata",
			authDescription: "Tool to show dataset metadata",
			inputSchema: schema(map[string]any{
				"project": projectParam("The Google Cloud project ID containing the dataset."),
				"dataset": typed("string", "The dataset to get metadata information. Can be in `project.dataset` format."),
			}, "dataset"),
		},
		{
			tool:            "list-table-ids-tool",
			description:     "Tool to list table within a dataset",
			authDescription: "Tool to list table within a dataset",
			inputSchema: schema(map[string]any{
				"project": projectParam("The Google Cloud project ID containing the dataset."),
				"dataset": typed("string", "The dataset to list table ids."),
			}, "dataset"),
		},
		{
			tool:            "get-table-info-tool",
			description:     "Tool to show dataset metadata",
			authDescription: "Tool to show dataset metadata",
			inputSchema: schema(map[string]any{
				"project": projectParam("The Google Cloud project ID containing the dataset and table."),
				"dataset": typed("string", "The table's parent dataset."),
				"table":   typed("string", "The table to get metadata information."),
			}, "dataset", "table"),
		},
		{
			tool:            "conversational-analytics-tool",
			description:     "Tool to ask BigQuery conversational analytics",
			authDescription: "Tool to ask BigQuery conversational analytics",
			inputSchema: schema(map[string]any{
				"user_query_with_context": typed("string", "The user's question, potentially including conversation history and system instructions for context."),
				"table_references":        typed("string", `A JSON string of a list of BigQuery tables to use as context. Each object in the list must contain 'projectId', 'datasetId', and 'tableId'. Example: '[{"projectId": "my-gcp-project", "datasetId": "my_dataset", "tableId": "my_table"}]'.`),
			}, "user_query_with_context", "table_references"),
		},
		{
			tool:            "search-catalog-tool",
			description:     "Tool to search the BiqQuery catalog",
			authDescription: "Tool to search the BiqQuery catalog",
			inputSchema: schema(map[string]any{
				"prompt":     typed("string", "Prompt representing search intention. Do not rewrite the prompt."),
				"datasetIds": withDefault(arrayOf("Array of dataset IDs.", "string", "The IDs of the bigquery dataset."), []any{}),
				"projectIds": withDefault(arrayOf("Array of project IDs.", "string", "The IDs of the bigquery project."), []any{}),
				"types":      withDefault(arrayOf("Array of data types to filter by.", "string", "The type of the data. Accepted values are: CONNECTION, POLICY, DATASET, MODEL, ROUTINE, TABLE, VIEW."), []any{}),
				"pageSize":   withDefault(typed("integer", "Number of results in the search page."), float64(5)),
			}, "prompt"),
		},
	}
	for _, p := range prebuiltTools {
		expectedTools = append(expectedTools,
			tests.MCPToolManifest{Name: "my-" + p.tool, Description: p.description, InputSchema: p.inputSchema},
			tests.MCPToolManifest{Name: "my-auth-" + p.tool, Description: p.authDescription, InputSchema: p.inputSchema},
			tests.MCPToolManifest{Name: "my-client-auth-" + p.tool, Description: p.authDescription, InputSchema: p.inputSchema},
		)
	}
	return expectedTools
}
