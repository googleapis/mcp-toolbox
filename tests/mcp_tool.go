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
