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

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	v20251125 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20251125"
)

// RunRequest is a helper function to send HTTP requests and return the response
func RunRequest(t *testing.T, method, url string, body io.Reader, headers map[string]string) (*http.Response, []byte) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("unable to create request: %s", err)
	}

	req.Header.Set("Content-type", "application/json")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unable to send request: %s", err)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("unable to read request body: %s", err)
	}

	defer resp.Body.Close()
	return resp, respBody
}

// RunInitialize runs the initialize lifecycle for mcp to set up client-server connection
func RunInitialize(t *testing.T, protocolVersion string) string {
	url := "http://127.0.0.1:5000/mcp"

	initializeRequestBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      "mcp-initialize",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": protocolVersion,
		},
	}
	reqMarshal, err := json.Marshal(initializeRequestBody)
	if err != nil {
		t.Fatalf("unexpected error during marshaling of body")
	}

	resp, _ := RunRequest(t, http.MethodPost, url, bytes.NewBuffer(reqMarshal), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("response status code is not 200")
	}

	if contentType := resp.Header.Get("Content-type"); contentType != "application/json" {
		t.Fatalf("unexpected content-type header: want %s, got %s", "application/json", contentType)
	}

	sessionId := resp.Header.Get("Mcp-Session-Id")

	header := map[string]string{}
	if sessionId != "" {
		header["Mcp-Session-Id"] = sessionId
	}

	initializeNotificationBody := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	notiMarshal, err := json.Marshal(initializeNotificationBody)
	if err != nil {
		t.Fatalf("unexpected error during marshaling of notifications body")
	}

	_, _ = RunRequest(t, http.MethodPost, url, bytes.NewBuffer(notiMarshal), header)
	return sessionId
}

// NewMCPRequestHeader takes custom headers and appends headers required for MCP.
func NewMCPRequestHeader(t *testing.T, customHeaders map[string]string) map[string]string {
	headers := make(map[string]string)
	for k, v := range customHeaders {
		headers[k] = v
	}
	headers["Content-Type"] = "application/json"
	headers["MCP-Protocol-Version"] = v20251125.PROTOCOL_VERSION
	return headers
}

// InvokeMCPTool is a transparent, native JSON-RPC execution harness for tests.
func InvokeMCPTool(t *testing.T, toolName string, arguments map[string]any, requestHeader map[string]string) (int, *MCPCallToolResponse, error) {
	headers := NewMCPRequestHeader(t, requestHeader)

	req := NewMCPCallToolRequest(uuid.New().String(), toolName, arguments)
	reqBody, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("error marshalling request body: %v", err)
	}

	resp, respBody := RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/mcp", bytes.NewBuffer(reqBody), headers)

	var mcpResp MCPCallToolResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		if resp.StatusCode != http.StatusOK {
			return resp.StatusCode, nil, fmt.Errorf("%s", string(respBody))
		}
		t.Fatalf("error parsing mcp response body: %v\nraw body: %s", err, string(respBody))
	}

	return resp.StatusCode, &mcpResp, nil
}

// getMCPResultText safely extracts the text from content blocks, unmarshaling them if they are valid JSON.
//
// TODO: For tests that need to strictly validate the exact schema or structure of the output,
// consider avoiding this helper and instead unmarshal the raw JSON directly into expected Go structs for comparison.
func getMCPResultText(t *testing.T, resp *MCPCallToolResponse) []any {
	if len(resp.Result.Content) == 0 {
		return []any{}
	}

	var res []any
	for _, content := range resp.Result.Content {
		var item any
		if err := json.Unmarshal([]byte(content.Text), &item); err != nil {
			res = append(res, content.Text)
		} else {
			if slice, ok := item.([]any); ok {
				res = append(res, slice...)
			} else {
				res = append(res, item)
			}
		}

	}
	if res == nil {
		return []any{}
	}
	return res
}

// GetMCPToolsList is a JSON-RPC harness that fetches the tools/list registry.
func GetMCPToolsList(t *testing.T, requestHeader map[string]string) (int, []any, error) {
	headers := NewMCPRequestHeader(t, requestHeader)

	req := MCPListToolsRequest{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      uuid.New().String(),
		Method:  v20251125.TOOLS_LIST,
	}
	reqBody, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("error marshalling tools/list request body: %v", err)
	}

	resp, respBody := RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/mcp", bytes.NewBuffer(reqBody), headers)

	var mcpResp jsonrpc.JSONRPCResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		if resp.StatusCode != http.StatusOK {
			return resp.StatusCode, nil, fmt.Errorf("%s", string(respBody))
		}
		t.Fatalf("error parsing tools/list response: %v\nraw body: %s", err, string(respBody))
	}

	resultMap, ok := mcpResp.Result.(map[string]any)
	if !ok {
		t.Fatalf("tools/list result is not a map: %v", mcpResp.Result)
	}

	toolsList, ok := resultMap["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list did not contain tools array: %v", resultMap)
	}

	return resp.StatusCode, toolsList, nil
}

// AssertMCPError asserts that the response contains an error covering the expected message.
func AssertMCPError(t *testing.T, mcpResp *MCPCallToolResponse, wantErrMsg string) {
	t.Helper()
	var errText string
	if mcpResp.Error != nil {
		errText = mcpResp.Error.Message
	} else if mcpResp.Result.IsError {
		for _, content := range mcpResp.Result.Content {
			if content.Type == "text" {
				errText += content.Text
			}
		}
	} else {
		t.Fatalf("expected error containing %q, but got success result: %v", wantErrMsg, mcpResp.Result)
	}

	if !strings.Contains(errText, wantErrMsg) {
		t.Fatalf("expected error text containing %q, got %q", wantErrMsg, errText)
	}
}

// RunMCPToolsListMethod calls tools/list and verifies that the returned tools match the expected list.
func RunMCPToolsListMethod(t *testing.T, expectedOutput []MCPToolManifest) {
	t.Helper()
	statusCodeList, toolsList, errList := GetMCPToolsList(t, nil)
	if errList != nil {
		t.Fatalf("native error executing tools/list: %s", errList)
	}
	if statusCodeList != http.StatusOK {
		t.Fatalf("expected status 200 for tools/list, got %d", statusCodeList)
	}

	// Unmarshal toolsList into []MCPToolManifest
	toolsJSON, err := json.Marshal(toolsList)
	if err != nil {
		t.Fatalf("error marshalling tools list: %v", err)
	}

	var actualTools []MCPToolManifest
	if err := json.Unmarshal(toolsJSON, &actualTools); err != nil {
		t.Fatalf("error unmarshalling tools into MCPToolManifest: %v", err)
	}

	if len(actualTools) != len(expectedOutput) {
		t.Fatalf("expected %d tools, got %d. Actual tools: %+v", len(expectedOutput), len(actualTools), actualTools)
	}

	for _, expected := range expectedOutput {
		found := false
		for _, actual := range actualTools {
			if actual.Name == expected.Name {
				found = true
				// Use reflect.DeepEqual to check all fields (description, parameters, etc.)
				if !reflect.DeepEqual(actual, expected) {
					t.Fatalf("tool %s mismatch:\nwant: %+v\ngot: %+v", expected.Name, expected, actual)
				}
				break
			}
		}
		if !found {
			t.Fatalf("tool %s was not found in the tools/list registry", expected.Name)
		}
	}
}

// RunMCPCustomToolCallMethod invokes a tool and compares the result with expected output.
func RunMCPCustomToolCallMethod(t *testing.T, toolName string, arguments map[string]any, want string) {
	t.Helper()
	statusCode, mcpResp, err := InvokeMCPTool(t, toolName, arguments, nil)
	if err != nil {
		t.Fatalf("native error executing %s: %s", toolName, err)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", statusCode)
	}
	if mcpResp.Result.IsError {
		t.Fatalf("%s returned error result: %v", toolName, mcpResp.Result)
	}
	got := getMCPResultText(t, mcpResp)
	gotBytes, _ := json.Marshal(got)
	gotStr := string(gotBytes)
	if !strings.Contains(gotStr, want) {
		t.Fatalf(`expected %q to contain %q`, gotStr, want)
	}
}

// RunMCPToolInvokeTest runs the tool invoke test cases over MCP protocol.
func RunMCPToolInvokeTest(t *testing.T, select1Want string, options ...InvokeTestOption) {
	t.Helper()
	// Resolve options using existing InvokeTestOption and InvokeTestConfig from option.go
	configs := &InvokeTestConfig{
		myToolId3NameAliceWant:   "[{\"id\":1,\"name\":\"Alice\"},{\"id\":3,\"name\":\"Sid\"}]",
		myToolById4Want:          "[{\"id\":4,\"name\":null}]",
		myArrayToolWant:          "[{\"id\":1,\"name\":\"Alice\"},{\"id\":3,\"name\":\"Sid\"}]",
		nullWant:                 "[null]",
		supportOptionalNullParam: true,
		supportArrayParam:        true,
		supportClientAuth:        false,
		supportSelect1Want:       true,
		supportSelect1Auth:       true,
	}

	for _, option := range options {
		option(configs)
	}

	invokeTcs := []struct {
		name       string
		toolName   string
		args       map[string]any
		headers    map[string]string
		enabled    bool
		wantResult string // for success cases
		wantError  string // for failure cases
	}{
		{
			name:       "invoke my-simple-tool",
			toolName:   "my-simple-tool",
			args:       map[string]any{},
			enabled:    configs.supportSelect1Want,
			wantResult: select1Want,
		},
		{
			name:       "invoke my-tool",
			toolName:   "my-tool",
			args:       map[string]any{"id": 3, "name": "Alice"},
			enabled:    true,
			wantResult: configs.myToolId3NameAliceWant,
		},
		{
			name:       "invoke my-tool-by-id with nil response",
			toolName:   "my-tool-by-id",
			args:       map[string]any{"id": 4},
			enabled:    true,
			wantResult: configs.myToolById4Want,
		},
		{
			name:       "invoke my-tool-by-name with nil response",
			toolName:   "my-tool-by-name",
			args:       map[string]any{},
			enabled:    configs.supportOptionalNullParam,
			wantResult: configs.nullWant,
		},
		{
			name:      "Invoke my-tool without parameters",
			toolName:  "my-tool",
			args:      map[string]any{},
			enabled:   true,
			wantError: `parameter "id" is required`,
		},
		{
			name:      "Invoke my-tool with insufficient parameters",
			toolName:  "my-tool",
			args:      map[string]any{"id": 1},
			enabled:   true,
			wantError: `parameter "name" is required`,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.enabled {
				t.Skip("skipping disabled test case")
			}
			statusCode, mcpResp, err := InvokeMCPTool(t, tc.toolName, tc.args, tc.headers)
			if err != nil {
				t.Fatalf("native error executing %s: %s", tc.toolName, err)
			}
			if statusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", statusCode)
			}
			if tc.wantError != "" {
				AssertMCPError(t, mcpResp, tc.wantError)
				return
			}
			if mcpResp.Result.IsError {
				t.Fatalf("%s returned error result: %v", tc.toolName, mcpResp.Result)
			}
			got := getMCPResultText(t, mcpResp)
			gotBytes, _ := json.Marshal(got)
			gotStr := string(gotBytes)
			if !strings.Contains(gotStr, tc.wantResult) {
				t.Fatalf(`expected %q to contain %q`, gotStr, tc.wantResult)
			}
		})
	}
}

// GetBaseMCPExpectedTools returns the MCP manifests for the base tools loaded by GetToolsConfig.
func GetBaseMCPExpectedTools() []MCPToolManifest {
	return []MCPToolManifest{
		{
			Name:        "my-simple-tool",
			Description: "Simple tool to test end to end functionality.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-tool",
			Description: "Tool to test invocation with params.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":   map[string]any{"type": "integer", "description": "user ID"},
					"name": map[string]any{"type": "string", "description": "user name"},
				},
				"required": []any{"id", "name"},
			},
		},
		{
			Name:        "my-tool-by-id",
			Description: "Tool to test invocation with params.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"id": map[string]any{"type": "integer", "description": "user ID"}},
				"required":   []any{"id"},
			},
		},
		{
			Name:        "my-tool-by-name",
			Description: "Tool to test invocation with params.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"name": map[string]any{"type": "string", "description": "user name"}},
				"required":   []any{},
			},
		},
		{
			Name:        "my-array-tool",
			Description: "Tool to test invocation with array params.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"idArray":   map[string]any{"type": "array", "description": "ID array", "items": map[string]any{"type": "integer", "description": "ID"}},
					"nameArray": map[string]any{"type": "array", "description": "user name array", "items": map[string]any{"type": "string", "description": "user name"}},
				},
				"required": []any{"idArray", "nameArray"},
			},
		},
		{
			Name:        "my-auth-tool",
			Description: "Tool to test authenticated parameters.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"email": map[string]any{"type": "string", "description": "user email"}},
				"required":   []any{"email"},
			},
		},
		{
			Name:        "my-auth-required-tool",
			Description: "Tool to test auth required invocation.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-fail-tool",
			Description: "Tool to test statement with incorrect syntax.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
	}
}

// GetExecuteSQLMCPExpectedTools returns the MCP manifests for the tools loaded by AddExecuteSqlConfig.
func GetExecuteSQLMCPExpectedTools() []MCPToolManifest {
	return []MCPToolManifest{
		{
			Name:        "my-exec-sql-tool",
			Description: "Tool to execute sql",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"sql": map[string]any{"type": "string", "description": "The sql to execute."}},
				"required":   []any{"sql"},
			},
		},
		{
			Name:        "my-auth-exec-sql-tool",
			Description: "Tool to execute sql",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"sql": map[string]any{"type": "string", "description": "The sql to execute."}},
				"required":   []any{"sql"},
			},
		},
	}
}

// GetTemplateParamMCPExpectedTools returns the MCP manifests for the tools loaded by AddTemplateParamConfig.
func GetTemplateParamMCPExpectedTools() []MCPToolManifest {
	return []MCPToolManifest{
		{
			Name:        "create-table-templateParams-tool",
			Description: "Create table tool with template parameters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tableName": map[string]any{"type": "string", "description": "some description"},
					"columns":   map[string]any{"type": "array", "description": "The columns to create", "items": map[string]any{"type": "string", "description": "A column name that will be created"}},
				},
				"required": []any{"tableName", "columns"},
			},
		},
		{
			Name:        "insert-table-templateParams-tool",
			Description: "Insert tool with template parameters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tableName": map[string]any{"type": "string", "description": "some description"},
					"columns":   map[string]any{"type": "array", "description": "The columns to insert into", "items": map[string]any{"type": "string", "description": "A column name that will be returned from the query."}},
					"values":    map[string]any{"type": "string", "description": "The values to insert as a comma separated string"},
				},
				"required": []any{"tableName", "columns", "values"},
			},
		},
		{
			Name:        "select-templateParams-tool",
			Description: "Create table tool with template parameters",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"tableName": map[string]any{"type": "string", "description": "some description"}},
				"required":   []any{"tableName"},
			},
		},
		{
			Name:        "select-templateParams-combined-tool",
			Description: "Create table tool with template parameters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":        map[string]any{"type": "integer", "description": "the id of the user"},
					"tableName": map[string]any{"type": "string", "description": "some description"},
				},
				"required": []any{"id", "tableName"},
			},
		},
		{
			Name:        "select-fields-templateParams-tool",
			Description: "Create table tool with template parameters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tableName": map[string]any{"type": "string", "description": "some description"},
					"fields":    map[string]any{"type": "array", "description": "The fields to select from", "items": map[string]any{"type": "string", "description": "A field that will be returned from the query."}},
				},
				"required": []any{"tableName", "fields"},
			},
		},
		{
			Name:        "select-filter-templateParams-combined-tool",
			Description: "Create table tool with template parameters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":         map[string]any{"type": "string", "description": "the name of the user"},
					"tableName":    map[string]any{"type": "string", "description": "some description"},
					"columnFilter": map[string]any{"type": "string", "description": "some description"},
				},
				"required": []any{"name", "tableName", "columnFilter"},
			},
		},
		{
			Name:        "drop-table-templateParams-tool",
			Description: "Drop table tool with template parameters",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"tableName": map[string]any{"type": "string", "description": "some description"}},
				"required":   []any{"tableName"},
			},
		},
	}
}

// GetPostgresPrebuiltMCPExpectedTools returns the MCP manifests for the tools loaded by AddPostgresPrebuiltConfig.
func GetPostgresPrebuiltMCPExpectedTools() []MCPToolManifest {
	return []MCPToolManifest{
		{
			Name:        "database_overview",
			Description: "Fetches the current state of the PostgreSQL server, returning the version, whether it's a replica, uptime duration, maximum connection limit, number of current connections, number of active connections, and the percentage of connections in use.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "get_column_cardinality",
			Description: "Estimates the number of unique values (cardinality) quickly for one or all columns in a specific PostgreSQL table by using the database's internal statistics, returning the results in descending order of estimated cardinality. Please run ANALYZE on the table before using this tool to get accurate results. The tool returns the column_name and the estimated_cardinality. If the column_name is not provided, the tool returns all columns along with their estimated cardinality.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"column_name": map[string]any{"description": "Optional: The column name for which the cardinality is to be found. If not provided, cardinality for all columns will be returned.", "type": "string"},
					"schema_name": map[string]any{"default": "public", "description": "Optional: The schema name in which the table is present.", "type": "string"},
					"table_name":  map[string]any{"description": "Required: The table name in which the column is present.", "type": "string"},
				},
				"required": []any{"table_name"},
			},
		},
		{
			Name:        "list_active_queries",
			Description: "Lists active queries in the database.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"exclude_application_names": map[string]any{"default": "", "description": "Optional: A comma-separated list of application names to exclude from the query results. This is useful for filtering out queries from specific applications (e.g., 'psql', 'pgAdmin', 'DBeaver'). The match is case-sensitive. Whitespace around commas and names is automatically handled. If this parameter is omitted, no applications are excluded.", "type": "string"},
					"limit":                     map[string]any{"default": float64(50), "description": "Optional: The maximum number of rows to return.", "type": "integer"},
					"min_duration":              map[string]any{"default": "1 minute", "description": "Optional: Only show queries running at least this long (e.g., '1 minute', '1 second', '2 seconds').", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_available_extensions",
			Description: "Lists available extensions in the database.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "list_database_stats",
			Description: "Lists the key performance and activity statistics for each PostgreSQL databasein the instance, offering insights into cache efficiency, transaction throughputrow-level activity, temporary file usage, and contention. It returns: the database name, whether the database is connectable,  database owner, default tablespace name, the percentage of data blocks found in the buffer cache rather than being read from disk (a higher value indicates better cache performance), the total number of disk blocks read from disk, the total number of times disk blocks were found already in the cache; the total number of committed transactions, the total number of rolled back transactions, the percentage of rolled back transactions compared to the total number of completed transactions, the total number of rows returned by queries, the total number of live rows fetched by scans, the total number of rows inserted, the total number of rows updated, the total number of rows deleted, the number of temporary files created by queries, the total size of all temporary files created by queries in bytes, the number of query cancellations due to conflicts with recovery, the number of deadlocks detected, the current number of active connections to the database, the timestamp of the last statistics reset, and total database size in bytes.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database_name":      map[string]any{"default": "", "description": "Optional: A specific database name pattern to search for.", "type": "string"},
					"database_owner":     map[string]any{"default": "", "description": "Optional: A specific database owner name pattern to search for.", "type": "string"},
					"default_tablespace": map[string]any{"default": "", "description": "Optional: A specific default tablespace name pattern to search for.", "type": "string"},
					"include_templates":  map[string]any{"default": false, "description": "Optional: Whether to include template databases in the results.", "type": "boolean"},
					"limit":              map[string]any{"default": float64(10), "description": "Optional: The maximum number of rows to return.", "type": "integer"},
					"order_by":           map[string]any{"default": "", "description": "Optional: The field to order the results by. Valid values are 'size' and 'commit'.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_indexes",
			Description: "Lists available user indexes in the database, excluding system schemas (pg_catalog, information_schema). For each index, the following properties are returned: schema name, table name, index name, index type (access method), a boolean indicating if it's a unique index, a boolean indicating if it's for a primary key, the index definition, index size in bytes, the number of index scans, the number of index tuples read, the number of table tuples fetched via index scans, and a boolean indicating if the index has been used at least once.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"index_name":  map[string]any{"default": "", "description": "Optional: a text to filter results by index name. The input is used within a LIKE clause.", "type": "string"},
					"limit":       map[string]any{"default": float64(50), "description": "Optional: The maximum number of rows to return. Default is 50", "type": "integer"},
					"only_unused": map[string]any{"default": false, "description": "Optional: If true, only returns indexes that have never been used.", "type": "boolean"},
					"schema_name": map[string]any{"default": "", "description": "Optional: a text to filter results by schema name. The input is used within a LIKE clause.", "type": "string"},
					"table_name":  map[string]any{"default": "", "description": "Optional: a text to filter results by table name. The input is used within a LIKE clause.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_installed_extensions",
			Description: "Lists installed extensions in the database.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "list_locks",
			Description: "Identifies all locks held by active processes showing the process ID, user, query text, and an aggregated list of all transactions and specific locks (relation, mode, grant status) associated with each process.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "list_pg_settings",
			Description: "Lists configuration parameters for the postgres server ordered lexicographically, with a default limit of 50 rows. It returns the parameter name, its current setting, unit of measurement, a short description, the source of the current setting (e.g., default, configuration file, session), and whether a restart is required when the parameter value is changed.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":        map[string]any{"default": float64(50), "description": "Optional: The maximum number of rows to return.", "type": "integer"},
					"setting_name": map[string]any{"default": "", "description": "Optional: A specific configuration parameter name pattern to search for.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_publication_tables",
			Description: "Lists all publication tables in the database. Returns the publication name, schema name, and table name, along with definition details indicating if it publishes all tables, whether it replicates inserts, updates, deletes, or truncates, and the publication owner.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":             map[string]any{"default": float64(50), "description": "Optional: The maximum number of rows to return.", "type": "integer"},
					"publication_names": map[string]any{"default": "", "description": "Optional: Filters by a comma-separated list of publication names.", "type": "string"},
					"schema_names":      map[string]any{"default": "", "description": "Optional: Filters by a comma-separated list of schema names.", "type": "string"},
					"table_names":       map[string]any{"default": "", "description": "Optional: Filters by a comma-separated list of table names.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_query_stats",
			Description: "Lists performance statistics for executed queries ordered by total time, filtering by database name pattern if provided. This tool requires the pg_stat_statements extension to be installed. The tool returns the database name, query text, execution count, timing metrics (total, min, max, mean), rows affected, and buffer cache I/O statistics (hits and reads).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database_name": map[string]any{"default": "", "description": "Optional: The database name to list query stats for.", "type": "string"},
					"limit":         map[string]any{"default": float64(50), "description": "Optional: The maximum number of results to return. Defaults to 50.", "type": "integer"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_roles",
			Description: "Lists all the user-created roles in the instance . It returns the role name, Object ID, the maximum number of concurrent connections the role can make, along with boolean indicators for: superuser status, privilege inheritance from member roles, ability to create roles, ability to create databases, ability to log in, replication privilege, and the ability to bypass row-level security, the password expiration timestamp, a list of direct members belonging to this role, and a list of other roles/groups that this role is a member of.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":     map[string]any{"default": float64(50), "description": "Optional: The maximum number of rows to return. Default is 50", "type": "integer"},
					"role_name": map[string]any{"default": "", "description": "Optional: a text to filter results by role name. The input is used within a LIKE clause.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_schemas",
			Description: "Lists all schemas in the database ordered by schema name and excluding system and temporary schemas. It returns the schema name, schema owner, grants, number of functions, number of tables and number of views within each schema.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":       map[string]any{"default": float64(10), "description": "Optional: The maximum number of schemas to return.", "type": "integer"},
					"owner":       map[string]any{"default": "", "description": "Optional: A specific schema owner name pattern to search for.", "type": "string"},
					"schema_name": map[string]any{"default": "", "description": "Optional: A specific schema name pattern to search for.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_sequences",
			Description: "Lists sequences in the database. Returns sequence name, schema name, sequence owner, data type of the sequence, starting value, minimum value, maximum value of the sequence, the value by which the sequence is incremented, and the last value generated by the sequence in the current session",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":         map[string]any{"default": float64(50), "description": "Optional: The maximum number of rows to return. Default is 50", "type": "integer"},
					"schema_name":   map[string]any{"default": "", "description": "Optional: A specific schema name pattern to search for.", "type": "string"},
					"sequence_name": map[string]any{"default": "", "description": "Optional: A specific sequence name pattern to search for.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_stored_procedure",
			Description: "Retrieves stored procedure metadata returning schema name, procedure name, procedure owner, language, definition, and description, filtered by optional role name (procedure owner), schema name, and limit (default 20).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":       map[string]any{"default": float64(20), "description": "Optional: The maximum number of stored procedures to return. Defaults to 20.", "type": "integer"},
					"role_name":   map[string]any{"description": "Optional: The owner name to filter the stored procedures by. Defaults to NULL.", "type": "string"},
					"schema_name": map[string]any{"description": "Optional: The schema name to filter the stored procedures by. Defaults to NULL.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_table_stats",
			Description: "Lists the user table statistics in the database ordered by number of\n        sequential scans with a default limit of 50 rows. Returns the following\n        columns: schema name, table name, table size in bytes, number of\n        sequential scans, number of index scans, idx_scan_ratio_percent (showing\n        the percentage of total scans that utilized an index, where a low ratio\n        indicates missing or ineffective indexes), number of live rows, number\n        of dead rows, dead_row_ratio_percent (indicating potential table bloat),\n        total number of rows inserted, updated, and deleted, the timestamps\n        for the last_vacuum, last_autovacuum, and last_autoanalyze operations.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":       map[string]any{"default": float64(50), "description": "Optional: The maximum number of results to return", "type": "integer"},
					"owner":       map[string]any{"description": "Optional: A specific owner to filter by", "type": "string"},
					"schema_name": map[string]any{"default": "public", "description": "Optional: A specific schema name to filter by", "type": "string"},
					"sort_by":     map[string]any{"description": "Optional: The column to sort by", "type": "string"},
					"table_name":  map[string]any{"description": "Optional: A specific table name to filter by", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_tables",
			Description: "Lists tables in the database.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"output_format": map[string]any{"default": "detailed", "description": "Optional: Use 'simple' for names only or 'detailed' for full info.", "type": "string"},
					"table_names":   map[string]any{"default": "", "description": "Optional: A comma-separated list of table names. If empty, details for all tables will be listed.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_tablespaces",
			Description: "Lists all tablespaces in the database. Returns the tablespace name, owner name, size in bytes(if the current user has CREATE privileges on the tablespace, otherwise NULL), internal object ID, the access control list regarding permissions, and any specific tablespace options.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":           map[string]any{"default": float64(50), "description": "Optional: The maximum number of rows to return.", "type": "integer"},
					"tablespace_name": map[string]any{"default": "", "description": "Optional: a text to filter results by tablespace name. The input is used within a LIKE clause.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_triggers",
			Description: "Lists all non-internal triggers in a database. Returns trigger name, schema name, table name, whether its enabled or disabled, timing (e.g BEFORE/AFTER of the event), the  events that cause the trigger to fire such as INSERT, UPDATE, or DELETE, whether the trigger activates per ROW or per STATEMENT, the handler function executed by the trigger and full definition.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":        map[string]any{"default": float64(50), "description": "Optional: The maximum number of rows to return.", "type": "integer"},
					"schema_name":  map[string]any{"default": "", "description": "Optional: A specific schema name pattern to search for.", "type": "string"},
					"table_name":   map[string]any{"default": "", "description": "Optional: A specific table name pattern to search for.", "type": "string"},
					"trigger_name": map[string]any{"default": "", "description": "Optional: A specific trigger name pattern to search for.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "list_views",
			Description: "Lists views in the database from pg_views with a default limit of 50 rows. Returns schemaname, viewname, ownername and the definition.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":       map[string]any{"default": float64(50), "description": "Optional: The maximum number of rows to return.", "type": "integer"},
					"schema_name": map[string]any{"default": "", "description": "Optional: A specific schema name to search for.", "type": "string"},
					"view_name":   map[string]any{"default": "", "description": "Optional: A specific view name to search for.", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "long_running_transactions",
			Description: "Identifies and lists database transactions that exceed a specified time limit. For each of the long running transactions, the output contains the process id, database name, user name, application name, client address, state, connection age, transaction age, query age, last activity age, wait event type, wait event, and query string.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":        map[string]any{"default": float64(20), "description": "Optional: The maximum number of long-running transactions to return. Defaults to 20.", "type": "integer"},
					"min_duration": map[string]any{"default": "5 minutes", "description": "Optional: Only show transactions running at least this long (e.g., '1 minute', '15 minutes', '30 seconds').", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name:        "replication_stats",
			Description: "Lists each replica's process ID, user name, application name, backend_xmin (standby's xmin horizon reported by hot_standby_feedback), client IP address, connection state, and sync_state, along with lag sizes in bytes for sent_lag (primary to sent), write_lag (sent to written), flush_lag (written to flushed), replay_lag (flushed to replayed), and the overall total_lag (primary to replayed).",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
	}
}

// RunMCPSecureToolInvokeTest runs integration test cases verifying secure-params protocol constraints.
func RunMCPSecureToolInvokeTest(t *testing.T, options ...McpTestOption) {
	t.Helper()

	configs := &MCPTestConfig{}
	for _, o := range options {
		o(configs)
	}

	mcpVersion := "2026-07-28"

	header := map[string]string{}

	// Local helper to send the tools/call request
	invokeTool := func(toolName string, arguments, secureArguments map[string]any, supportsSecure bool) (int, *MCPCallToolResponse, error) {
		headers := map[string]string{
			"Content-Type":         "application/json",
			"MCP-Protocol-Version": mcpVersion,
			"Mcp-Method":           "tools/call",
			"Mcp-Name":             toolName,
		}
		for k, v := range header {
			headers[k] = v
		}

		params := map[string]any{
			"name": toolName,
		}
		if arguments != nil {
			params["arguments"] = arguments
		}
		if secureArguments != nil {
			params["secureArguments"] = secureArguments
		}
		meta := map[string]any{
			"io.modelcontextprotocol/protocolVersion": mcpVersion,
			"io.modelcontextprotocol/clientInfo": map[string]any{
				"name":    "TestClient",
				"version": "1.0",
			},
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		}
		if supportsSecure {
			meta["io.modelcontextprotocol/clientCapabilities"] = map[string]any{
				"extensions": map[string]any{
					"com.google.cloud/toolbox.v1": map[string]any{},
				},
			}
		}
		params["_meta"] = meta

		req := map[string]any{
			"jsonrpc": "2.0",
			"id":      uuid.New().String(),
			"method":  "tools/call",
			"params":  params,
		}

		reqBody, err := json.Marshal(req)
		if err != nil {
			return 0, nil, err
		}

		resp, respBody := RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/mcp", bytes.NewBuffer(reqBody), headers)

		var mcpResp MCPCallToolResponse
		if err := json.Unmarshal(respBody, &mcpResp); err != nil {
			if resp.StatusCode != http.StatusOK {
				return resp.StatusCode, nil, fmt.Errorf("%s", string(respBody))
			}
			return resp.StatusCode, nil, err
		}

		return resp.StatusCode, &mcpResp, nil
	}

	wantMatch := `[{"id":1,"name":"Alice"},{"id":3,"name":"Sid"}]`
	if configs.mySecureToolWant != "" {
		wantMatch = configs.mySecureToolWant
	}

	tcs := []struct {
		name             string
		arguments        map[string]any
		secureArguments  map[string]any
		supportsSecure   bool
		wantRpcErrorCode int
		wantIsError      bool
		wantErrorMsg     string
		wantBodyMatch    string
	}{
		{
			name:             "client doesn't support secure-params",
			arguments:        map[string]any{"id": 3},
			secureArguments:  map[string]any{"name": "Alice"},
			supportsSecure:   false,
			wantRpcErrorCode: jsonrpc.MISSING_REQUIRED_CLIENT_CAPABILITY,
			wantErrorMsg:     "requires com.google.cloud/toolbox.v1 extension which is not supported by the client",
		},
		{
			name:            "secure parameter passed in standard arguments",
			arguments:       map[string]any{"id": 3, "name": "Alice"},
			secureArguments: nil,
			supportsSecure:  true,
			wantIsError:     true,
			wantErrorMsg:    `parameter "name" is secure and must not be passed in standard arguments`,
		},
		{
			name:             "standard parameter passed in secureArguments",
			arguments:        nil,
			secureArguments:  map[string]any{"id": 3, "name": "Alice"},
			supportsSecure:   true,
			wantRpcErrorCode: jsonrpc.INVALID_PARAMS,
			wantErrorMsg:     `parameter "id" is not secure and must not be passed in secureArguments`,
		},
		{
			name:             "missing required secure parameter",
			arguments:        map[string]any{"id": 3},
			secureArguments:  map[string]any{},
			supportsSecure:   true,
			wantRpcErrorCode: jsonrpc.INVALID_PARAMS,
			wantErrorMsg:     `missing required secure parameter "name" in secureArguments`,
		},
		{
			name:            "successful invocation with correct routing",
			arguments:       map[string]any{"id": 3},
			secureArguments: map[string]any{"name": "Alice"},
			supportsSecure:  true,
			wantBodyMatch:   wantMatch,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := invokeTool("my-secure-tool", tc.arguments, tc.secureArguments, tc.supportsSecure)
			if err != nil {
				t.Fatalf("unexpected native error: %s", err)
			}
			if statusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", statusCode)
			}

			if tc.wantRpcErrorCode != 0 {
				if mcpResp.Error == nil {
					t.Fatalf("expected JSON-RPC error with code %d, but got success result: %v", tc.wantRpcErrorCode, mcpResp.Result)
				}
				if mcpResp.Error.Code != tc.wantRpcErrorCode {
					t.Fatalf("expected error code %d, got %d (msg: %q)", tc.wantRpcErrorCode, mcpResp.Error.Code, mcpResp.Error.Message)
				}
				if !strings.Contains(mcpResp.Error.Message, tc.wantErrorMsg) {
					t.Fatalf("expected error message containing %q, got %q", tc.wantErrorMsg, mcpResp.Error.Message)
				}
				return
			}

			if tc.wantIsError {
				if mcpResp.Error != nil {
					t.Fatalf("expected agent error (isError: true), but got protocol error: code %d, msg %q", mcpResp.Error.Code, mcpResp.Error.Message)
				}
				if !mcpResp.Result.IsError {
					t.Fatalf("expected mcpResp.Result.IsError = true, got false")
				}
				var contentText string
				for _, c := range mcpResp.Result.Content {
					if c.Type == "text" {
						contentText += c.Text
					}
				}
				if !strings.Contains(contentText, tc.wantErrorMsg) {
					t.Fatalf("expected agent error containing %q, got %q", tc.wantErrorMsg, contentText)
				}
				return
			}

			if mcpResp.Error != nil {
				t.Fatalf("unexpected JSON-RPC error: code %d, msg %q", mcpResp.Error.Code, mcpResp.Error.Message)
			}
			if mcpResp.Result.IsError {
				t.Fatalf("expected successful execution, got error: %v", mcpResp.Result)
			}
			got := getMCPResultText(t, mcpResp)
			gotBytes, _ := json.Marshal(got)
			gotStr := string(gotBytes)
			if !strings.Contains(gotStr, tc.wantBodyMatch) {
				t.Fatalf(`expected %q to contain %q`, gotStr, tc.wantBodyMatch)
			}
		})
	}
}
