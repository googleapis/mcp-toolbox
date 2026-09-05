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

package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/telemetry"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

const jsonrpcVersion = "2.0"
const protocolVersion20241105 = "2024-11-05"
const protocolVersion20250326 = "2025-03-26"
const protocolVersion20250618 = "2025-06-18"
const protocolVersion20251125 = "2025-11-25"
const protocolVersion20260728 = "2026-07-28"
const serverName = "Toolbox"

var basicInputSchema = map[string]any{
	"type":       "object",
	"properties": map[string]any{},
	"required":   []any{},
}

var tool2InputSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"param1": map[string]any{"type": "integer", "description": "This is the first parameter."},
		"param2": map[string]any{"type": "integer", "description": "This is the second parameter."},
	},
	"required": []any{"param1", "param2"},
}

var tool3InputSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"my_array": map[string]any{
			"type":        "array",
			"description": "this param is an array of strings",
			"items":       map[string]any{"type": "string", "description": "string item"},
		},
	},
	"required": []any{"my_array"},
}

var urlBindingToolInputSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"param1": map[string]any{"type": "string", "description": "A bound string param"},
		"param2": map[string]any{"type": "integer", "description": "A bound int param"},
		"param3": map[string]any{"type": "boolean", "description": "A bound bool param"},
		"param4": map[string]any{"type": "number", "description": "A bound float param"},
		"param5": map[string]any{"type": "string", "description": "An unbound string param"},
		"param6": map[string]any{
			"type":        "array",
			"description": "A bound array param",
			"items": map[string]any{
				"type":        "string",
				"description": "item",
			},
		},
		"param7": map[string]any{
			"type":        "object",
			"description": "A bound map param",
			"additionalProperties": map[string]any{
				"type": "string",
			},
		},
	},
	"required": []any{"param1", "param2", "param3", "param4", "param5", "param6", "param7"},
}

var prompt2Args = []any{
	map[string]any{
		"name":        "arg1",
		"description": "This is the first argument.",
		"required":    true,
	},
}

// TestMcpEndpointWithoutInitialized is expecting Toolbox to response with the
// v2024-11-05 version. This was a customs transport that we implemented during
// the initial integration of MCP within Toolbox.
func TestMcpEndpointWithoutInitialized(t *testing.T) {
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2, testutils.MockTool3, testutils.MockTool4, testutils.MockTool5}
	mockPrompts := []testutils.MockPrompt{testutils.MockPrompt1, testutils.MockPrompt2}
	toolsMap, promptsMap, groups := testutils.SetUpResources(t, mockTools, mockPrompts)
	r, shutdown := setUpServer(t, "mcp", toolsMap, promptsMap, groups)
	defer shutdown()
	ts := runServer(r, false)
	defer ts.Close()

	testCases := []struct {
		name  string
		url   string
		isErr bool
		body  jsonrpc.JSONRPCRequest
		want  map[string]any
	}{
		{
			name: "ping",
			url:  "/",
			body: jsonrpc.JSONRPCRequest{
				Jsonrpc: jsonrpcVersion,
				Id:      "ping-test-123",
				Request: jsonrpc.Request{
					Method: "ping",
				},
			},
			isErr: false,
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "ping-test-123",
				"result":  map[string]any{},
			},
		},
		{
			name: "tools/list",
			url:  "/",
			body: jsonrpc.JSONRPCRequest{
				Jsonrpc: jsonrpcVersion,
				Id:      "tools-list",
				Request: jsonrpc.Request{
					Method: "tools/list",
				},
			},
			isErr: false,
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "tools-list",
				"result": map[string]any{
					"tools": []any{
						map[string]any{
							"name":        "no_params",
							"inputSchema": basicInputSchema,
						},
						map[string]any{
							"name":        "some_params",
							"inputSchema": tool2InputSchema,
						},
						map[string]any{
							"name":        "array_param",
							"description": "some description",
							"inputSchema": tool3InputSchema,
						},
						map[string]any{
							"name":        "unauthorized_tool",
							"inputSchema": basicInputSchema,
						},
						map[string]any{
							"name":        "require_client_auth_tool",
							"inputSchema": basicInputSchema,
						},
					},
				},
			},
		},
		{
			name:  "missing method",
			url:   "/",
			isErr: true,
			body: jsonrpc.JSONRPCRequest{
				Jsonrpc: jsonrpcVersion,
				Id:      "missing-method",
				Request: jsonrpc.Request{},
			},
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "missing-method",
				"error": map[string]any{
					"code":    -32601.0,
					"message": "method not found",
				},
			},
		},
		{
			name:  "invalid jsonrpc version",
			url:   "/",
			isErr: true,
			body: jsonrpc.JSONRPCRequest{
				Jsonrpc: "1.0",
				Id:      "invalid-jsonrpc-version",
				Request: jsonrpc.Request{
					Method: "foo",
				},
			},
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "invalid-jsonrpc-version",
				"error": map[string]any{
					"code":    -32600.0,
					"message": "invalid json-rpc version",
				},
			},
		},
		{
			name: "call tool1 unauthorized tool",
			url:  "/",
			body: jsonrpc.JSONRPCRequest{
				Jsonrpc: jsonrpcVersion,
				Id:      "tools-call-tool1",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name": "no_params",
				},
			},
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "tools-call-tool1",
				"result": map[string]any{
					"content": []any{
						map[string]any{
							"type": "text",
							"text": `"no_params"`,
						},
					},
				},
			},
		},
		{
			name: "call tool4 unauthorized tool",
			url:  "/",
			body: jsonrpc.JSONRPCRequest{
				Jsonrpc: jsonrpcVersion,
				Id:      "tools-call-tool4",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name": "unauthorized_tool",
				},
			},
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "tools-call-tool4",
				"error": map[string]any{
					"code":    -32600.0,
					"message": "unauthorized Tool call: Please make sure you specify correct auth headers",
				},
			},
		},
		{
			name: "call tool5 unauthorized tool",
			url:  "/",
			body: jsonrpc.JSONRPCRequest{
				Jsonrpc: jsonrpcVersion,
				Id:      "tools-call-tool5",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name": "require_client_auth_tool",
				},
			},
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "tools-call-tool5",
				"error": map[string]any{
					"code":    -32600.0,
					"message": "missing access token in the 'Authorization' header",
				},
			},
		},
		{
			name: "prompts/list",
			url:  "/",
			body: jsonrpc.JSONRPCRequest{
				Jsonrpc: jsonrpcVersion,
				Id:      "prompts-list-uninitialized",
				Request: jsonrpc.Request{
					Method: "prompts/list",
				},
			},
			isErr: false,
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "prompts-list-uninitialized",
				"result": map[string]any{
					"prompts": []any{
						map[string]any{
							"name": "prompt1",
						},
						map[string]any{
							"name":      "prompt2",
							"arguments": prompt2Args,
						},
					},
				},
			},
		},
		{
			name:  "prompts/get non-existent prompt",
			url:   "/",
			isErr: true,
			body: jsonrpc.JSONRPCRequest{
				Jsonrpc: jsonrpcVersion,
				Id:      "prompts-get-non-existent",
				Request: jsonrpc.Request{
					Method: "prompts/get",
				},
				Params: map[string]any{
					"name": "non_existent_prompt",
				},
			},
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "prompts-get-non-existent",
				"error": map[string]any{
					"code":    -32602.0,
					"message": `prompt with name "non_existent_prompt" does not exist`,
				},
			},
		},
		{
			name:  "prompts/get with invalid arguments",
			url:   "/",
			isErr: true,
			body: jsonrpc.JSONRPCRequest{
				Jsonrpc: jsonrpcVersion,
				Id:      "prompts-get-invalid-args",
				Request: jsonrpc.Request{
					Method: "prompts/get",
				},
				Params: map[string]any{
					"name": "prompt2",
					"arguments": map[string]any{
						"arg1": 42,
					},
				},
			},
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "prompts-get-invalid-args",
				"error": map[string]any{
					"code":    -32602.0,
					"message": `invalid arguments for prompt "prompt2": unable to parse value for "arg1": %!q(float64=42) not type "string"`,
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reqMarshal, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("unexpected error during marshaling of body")
			}
			resp, body, err := runRequest(ts, http.MethodPost, tc.url, bytes.NewBuffer(reqMarshal), nil)
			if err != nil {
				t.Fatalf("unexpected error during request: %s", err)
			}

			// Notifications don't expect a response.
			if tc.want != nil {
				if contentType := resp.Header.Get("Content-type"); contentType != "application/json" {
					t.Fatalf("unexpected content-type header: want %s, got %s", "application/json", contentType)
				}

				var got map[string]any
				if err := json.Unmarshal(body, &got); err != nil {
					t.Fatalf("unexpected error unmarshalling body: %s", err)
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("unexpected response: got %+v, want %+v", got, tc.want)
				}
			}
		})
	}
}

func runInitializeLifecycle(t *testing.T, ts *httptest.Server, protocolVersion string, initializeWant map[string]any, idHeader bool) string {
	initializeRequestBody := map[string]any{
		"jsonrpc": jsonrpcVersion,
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

	resp, body, err := runRequest(ts, http.MethodPost, "/", bytes.NewBuffer(reqMarshal), nil)
	if err != nil {
		t.Fatalf("unexpected error during request: %s", err)
	}

	if contentType := resp.Header.Get("Content-type"); contentType != "application/json" {
		t.Fatalf("unexpected content-type header: want %s, got %s", "application/json", contentType)
	}

	sessionId := resp.Header.Get("Mcp-Session-Id")
	if idHeader && sessionId == "" {
		t.Fatalf("Mcp-Session-Id header is expected")
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unexpected error unmarshalling body: %s", err)
	}
	if !reflect.DeepEqual(got, initializeWant) {
		t.Fatalf("unexpected response: got %+v, want %+v", got, initializeWant)
	}

	header := map[string]string{}
	if sessionId != "" {
		header["Mcp-Session-Id"] = sessionId
	}

	initializeNotificationBody := map[string]any{
		"jsonrpc": jsonrpcVersion,
		"method":  "notifications/initialized",
	}
	notiMarshal, err := json.Marshal(initializeNotificationBody)
	if err != nil {
		t.Fatalf("unexpected error during marshaling of notifications body")
	}

	_, _, err = runRequest(ts, http.MethodPost, "/", bytes.NewBuffer(notiMarshal), header)
	if err != nil {
		t.Fatalf("unexpected error during request: %s", err)
	}
	return sessionId
}

func TestMcpEndpoint(t *testing.T) {
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2, testutils.MockTool3, testutils.MockTool4, testutils.MockTool5, testutils.MockToolUrlBinding}
	mockPrompts := []testutils.MockPrompt{testutils.MockPrompt1, testutils.MockPrompt2}
	toolsMap, promptsMap, groups := testutils.SetUpResources(t, mockTools, mockPrompts)
	r, shutdown := setUpServer(t, "mcp", toolsMap, promptsMap, groups, withEnableDraftSpecs())
	defer shutdown()
	ts := runServer(r, false)
	defer ts.Close()

	versTestCases := []struct {
		name                                   string
		protocol                               string
		idHeader                               bool
		reqHeader                              []string
		initWant                               map[string]any
		invalidMethods                         []string
		meta                                   map[string]any
		wantToolsList                          map[string]any
		wantPromptsList                        map[string]any
		wantPromptsGet                         map[string]any
		wantToolsListOnTool1                   map[string]any
		wantToolsCallOnTool1                   map[string]any
		wantToolsListWithURLParam              map[string]any
		wantToolsCallWithURLParam              map[string]any
		wantToolsCallWithURLParamOverrideError map[string]any
		wantToolsCallWithParamError            map[string]any
	}{
		{
			name:     "version 2024-11-05",
			protocol: protocolVersion20241105,
			idHeader: false,
			initWant: map[string]any{
				"jsonrpc": "2.0",
				"id":      "mcp-initialize",
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities": map[string]any{
						"tools":   map[string]any{"listChanged": false},
						"prompts": map[string]any{"listChanged": false},
					},
					"serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
				},
			},

			invalidMethods: []string{"server/discover"},
		},
		{
			name:     "version 2025-03-26",
			protocol: protocolVersion20250326,
			idHeader: true,
			initWant: map[string]any{
				"jsonrpc": "2.0",
				"id":      "mcp-initialize",
				"result": map[string]any{
					"protocolVersion": "2025-03-26",
					"capabilities": map[string]any{
						"tools":   map[string]any{"listChanged": false},
						"prompts": map[string]any{"listChanged": false},
					},
					"serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
				},
			},
			invalidMethods: []string{"server/discover"},
		},
		{
			name:      "version 2025-06-18",
			protocol:  protocolVersion20250618,
			idHeader:  false,
			reqHeader: []string{"Mcp-Protocol-Version"},
			initWant: map[string]any{
				"jsonrpc": "2.0",
				"id":      "mcp-initialize",
				"result": map[string]any{
					"protocolVersion": "2025-06-18",
					"capabilities": map[string]any{
						"tools":   map[string]any{"listChanged": false},
						"prompts": map[string]any{"listChanged": false},
					},
					"serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
				},
			},
			invalidMethods: []string{"server/discover"},
		},
		{
			name:      "version 2025-11-25",
			protocol:  protocolVersion20251125,
			idHeader:  false,
			reqHeader: []string{"Mcp-Protocol-Version"},
			initWant: map[string]any{
				"jsonrpc": "2.0",
				"id":      "mcp-initialize",
				"result": map[string]any{
					"protocolVersion": "2025-11-25",
					"capabilities": map[string]any{
						"tools":   map[string]any{"listChanged": false},
						"prompts": map[string]any{"listChanged": false},
					},
					"serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
				},
			},
			invalidMethods: []string{"server/discover"},
		},
		{
			name:           "version 2026-07-28",
			protocol:       protocolVersion20260728,
			idHeader:       false,
			reqHeader:      []string{"Mcp-Protocol-Version", "Mcp-Method", "Mcp-Name"},
			invalidMethods: []string{"ping"},
			meta: map[string]any{
				"io.modelcontextprotocol/protocolVersion": protocolVersion20260728,
				"io.modelcontextprotocol/clientInfo": map[string]any{
					"version": "client-temp-version",
					"name":    "client-name",
				},
				"io.modelcontextprotocol/clientCapabilities": map[string]any{},
			},
			wantToolsList: map[string]any{
				"jsonrpc": "2.0",
				"id":      "tools-list",
				"result": map[string]any{
					"resultType": "complete",
					"tools": []any{
						map[string]any{
							"name":        "no_params",
							"inputSchema": basicInputSchema,
						},
						map[string]any{
							"name":        "some_params",
							"inputSchema": tool2InputSchema,
						},
						map[string]any{
							"name":        "array_param",
							"description": "some description",
							"inputSchema": tool3InputSchema,
						},
						map[string]any{
							"name":        "unauthorized_tool",
							"inputSchema": basicInputSchema,
						},
						map[string]any{
							"name":        "require_client_auth_tool",
							"inputSchema": basicInputSchema,
						},
						map[string]any{
							"name":        "url_binding_tool",
							"description": "A tool for testing URL param binding",
							"inputSchema": urlBindingToolInputSchema,
						},
					},
					"ttlMs":      300000.0,
					"cacheScope": "public",
					"_meta": map[string]any{
						"io.modelcontextprotocol/serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
					},
				},
			},
			wantPromptsList: map[string]any{
				"jsonrpc": "2.0",
				"id":      "prompts-list",
				"result": map[string]any{
					"resultType": "complete",
					"prompts": []any{
						map[string]any{
							"name": "prompt1",
						},
						map[string]any{
							"name":      "prompt2",
							"arguments": prompt2Args,
						},
					},
					"ttlMs":      300000.0,
					"cacheScope": "public",
					"_meta": map[string]any{
						"io.modelcontextprotocol/serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
					},
				},
			},
			wantPromptsGet: map[string]any{
				"jsonrpc": "2.0",
				"id":      "prompts-get-prompt2",
				"result": map[string]any{
					"resultType": "complete",
					"messages": []any{
						map[string]any{
							"role": "user",
							"content": map[string]any{
								"type": "text",
								"text": "substituted prompt2",
							},
						},
					},
					"_meta": map[string]any{
						"io.modelcontextprotocol/serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
					},
				},
			},
			wantToolsListOnTool1: map[string]any{
				"jsonrpc": "2.0",
				"id":      "tools-list-tool1",
				"result": map[string]any{
					"resultType": "complete",
					"tools": []any{
						map[string]any{
							"name":        "no_params",
							"inputSchema": basicInputSchema,
						},
					},
					"ttlMs":      300000.0,
					"cacheScope": "public",
					"_meta": map[string]any{
						"io.modelcontextprotocol/serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
					},
				},
			},
			wantToolsCallOnTool1: map[string]any{
				"jsonrpc": "2.0",
				"id":      "tools-call-tool1",
				"result": map[string]any{
					"resultType": "complete",
					"content": []any{
						map[string]any{
							"type": "text",
							"text": `"no_params"`,
						},
					},
					"_meta": map[string]any{
						"io.modelcontextprotocol/serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
					},
				},
			},
			wantToolsListWithURLParam: map[string]any{
				"jsonrpc": "2.0",
				"id":      "tools-list-url-binding",
				"result": map[string]any{
					"resultType": "complete",
					"tools": []any{
						map[string]any{
							"name":        "no_params",
							"inputSchema": basicInputSchema,
						},
						map[string]any{
							"name":        "some_params",
							"inputSchema": basicInputSchema,
						},
						map[string]any{
							"name":        "array_param",
							"description": "some description",
							"inputSchema": tool3InputSchema,
						},
						map[string]any{
							"name":        "unauthorized_tool",
							"inputSchema": basicInputSchema,
						},
						map[string]any{
							"name":        "require_client_auth_tool",
							"inputSchema": basicInputSchema,
						},
						map[string]any{
							"name":        "url_binding_tool",
							"description": "A tool for testing URL param binding",
							"inputSchema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"param5": map[string]any{"type": "string", "description": "An unbound string param"},
								},
								"required": []any{"param5"},
							},
						},
					},
					"_meta": map[string]any{
						"io.modelcontextprotocol/serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
					},
					"ttlMs":      300000.0,
					"cacheScope": "public",
				},
			},
			wantToolsCallWithURLParam: map[string]any{
				"jsonrpc": "2.0",
				"id":      "tools-call-url-binding",
				"result": map[string]any{
					"resultType": "complete",
					"content": []any{
						map[string]any{
							"type": "text",
							"text": `"url_binding_tool"`,
						},
						map[string]any{
							"type": "text",
							"text": `"bound-string"`,
						},
						map[string]any{
							"type": "text",
							"text": `42`,
						},
						map[string]any{
							"type": "text",
							"text": `true`,
						},
						map[string]any{
							"type": "text",
							"text": `3.14`,
						},
						map[string]any{
							"type": "text",
							"text": `"unbound-value"`,
						},
						map[string]any{
							"type": "text",
							"text": `["a","b"]`,
						},
						map[string]any{
							"type": "text",
							"text": `{"k":"v"}`,
						},
					},
					"_meta": map[string]any{
						"io.modelcontextprotocol/serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
					},
				},
			},
			wantToolsCallWithURLParamOverrideError: map[string]any{
				"jsonrpc": "2.0",
				"id":      "tools-call-url-binding-override",
				"result": map[string]any{
					"resultType": "complete",
					"content": []any{
						map[string]any{
							"type": "text",
							"text": `parameter "param1" is bound by URL and cannot be provided in client arguments`,
						},
					},
					"isError": true,
					"_meta": map[string]any{
						"io.modelcontextprotocol/serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
					},
				},
			},
			wantToolsCallWithParamError: map[string]any{
				"jsonrpc": "2.0",
				"id":      "tools-call-param-error",
				"result": map[string]any{
					"resultType": "complete",
					"content": []any{
						map[string]any{
							"type": "text",
							"text": `provided parameters were invalid: parameter "param1" is required`,
						},
					},
					"isError": true,
					"_meta": map[string]any{
						"io.modelcontextprotocol/serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
					},
				},
			},
		},
	}
	for _, vtc := range versTestCases {
		t.Run(vtc.name, func(t *testing.T) {
			sessionId := ""
			if len(vtc.initWant) != 0 {
				sessionId = runInitializeLifecycle(t, ts, vtc.protocol, vtc.initWant, vtc.idHeader)
			}
			testCases := []*struct {
				name           string
				url            string
				isErr          bool
				body           jsonrpc.JSONRPCRequest
				batchBody      []jsonrpc.JSONRPCRequest
				methodName     string
				wantStatusCode int
				want           map[string]any
				wantOverwrite  map[string]any
			}{
				{
					name: "basic notification",
					url:  "/",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Request: jsonrpc.Request{
							Method: "notification",
						},
					},
					methodName:     "notification",
					wantStatusCode: http.StatusAccepted,
				},
				{
					name: "ping",
					url:  "/",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "ping-test-123",
						Request: jsonrpc.Request{
							Method: "ping",
						},
					},
					methodName:     "ping",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "ping-test-123",
						"result":  map[string]any{},
					},
				},
				{
					name: "server/discover",
					url:  "/",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "server-discover",
						Request: jsonrpc.Request{
							Method: "server/discover",
						},
					},
					methodName:     "server/discover",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "server-discover",
						"result": map[string]any{
							"resultType":        "complete",
							"supportedVersions": []any{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25", "2026-07-28"},
							"capabilities": map[string]any{
								"extensions": map[string]any{
									"com.google.cloud/toolbox.v1": map[string]any{},
								},
								"tools":   map[string]any{"listChanged": false},
								"prompts": map[string]any{"listChanged": false},
							},
							"_meta": map[string]any{
								"io.modelcontextprotocol/serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
							},
						},
					},
				},
				{
					name: "tools/list",
					url:  "/",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "tools-list",
						Request: jsonrpc.Request{
							Method: "tools/list",
						},
					},
					methodName:     "tools/list",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "tools-list",
						"result": map[string]any{
							"tools": []any{
								map[string]any{
									"name":        "no_params",
									"inputSchema": basicInputSchema,
								},
								map[string]any{
									"name":        "some_params",
									"inputSchema": tool2InputSchema,
								},
								map[string]any{
									"name":        "array_param",
									"description": "some description",
									"inputSchema": tool3InputSchema,
								},
								map[string]any{
									"name":        "unauthorized_tool",
									"inputSchema": basicInputSchema,
								},
								map[string]any{
									"name":        "require_client_auth_tool",
									"inputSchema": basicInputSchema,
								},
								map[string]any{
									"name":        "url_binding_tool",
									"description": "A tool for testing URL param binding",
									"inputSchema": urlBindingToolInputSchema,
								},
							},
						},
					},
					wantOverwrite: vtc.wantToolsList,
				},
				{
					name: "prompts/list",
					url:  "/",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "prompts-list",
						Request: jsonrpc.Request{
							Method: "prompts/list",
						},
					},
					methodName:     "prompts/list",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "prompts-list",
						"result": map[string]any{
							"prompts": []any{
								map[string]any{
									"name": "prompt1",
								},
								map[string]any{
									"name":      "prompt2",
									"arguments": prompt2Args,
								},
							},
						},
					},
					wantOverwrite: vtc.wantPromptsList,
				},
				{
					name: "prompts/get",
					url:  "/",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "prompts-get-prompt2",
						Request: jsonrpc.Request{
							Method: "prompts/get",
						},
						Params: map[string]any{
							"name": "prompt2",
							"arguments": map[string]any{
								"arg1": "value1",
							},
						},
					},
					methodName:     "prompts/get",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "prompts-get-prompt2",
						"result": map[string]any{
							"messages": []any{
								map[string]any{
									"role": "user",
									"content": map[string]any{
										"type": "text",
										"text": "substituted prompt2",
									},
								},
							},
						},
					},
					wantOverwrite: vtc.wantPromptsGet,
				},
				{
					name: "tools/list on tool1_only",
					url:  "/tool1_only",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "tools-list-tool1",
						Request: jsonrpc.Request{
							Method: "tools/list",
						},
					},
					methodName:     "tools/list",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "tools-list-tool1",
						"result": map[string]any{
							"tools": []any{
								map[string]any{
									"name":        "no_params",
									"inputSchema": basicInputSchema,
								},
							},
						},
					},
					wantOverwrite: vtc.wantToolsListOnTool1,
				},
				{
					name:  "tools/list on invalid tool set",
					url:   "/foo",
					isErr: true,
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "tools-list-invalid-toolset",
						Request: jsonrpc.Request{
							Method: "tools/list",
						},
					},
					methodName:     "tools/list",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "tools-list-invalid-toolset",
						"error": map[string]any{
							"code":    -32600.0,
							"message": "toolset does not exist",
						},
					},
				},
				{
					name:  "missing method",
					url:   "/",
					isErr: true,
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "missing-method",
						Request: jsonrpc.Request{},
					},
					methodName:     "",
					wantStatusCode: http.StatusNotFound,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "missing-method",
						"error": map[string]any{
							"code":    -32601.0,
							"message": "method not found",
						},
					},
				},
				{
					name:  "invalid method",
					url:   "/",
					isErr: true,
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "invalid-method",
						Request: jsonrpc.Request{
							Method: "foo",
						},
					},
					methodName:     "foo",
					wantStatusCode: http.StatusNotFound,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "invalid-method",
						"error": map[string]any{
							"code":    -32601.0,
							"message": "invalid method foo",
						},
					},
				},
				{
					name:  "invalid jsonrpc version",
					url:   "/",
					isErr: true,
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: "1.0",
						Id:      "invalid-jsonrpc-version",
						Request: jsonrpc.Request{
							Method: "foo",
						},
					},
					methodName:     "foo",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "invalid-jsonrpc-version",
						"error": map[string]any{
							"code":    -32600.0,
							"message": "invalid json-rpc version",
						},
					},
				},
				{
					name:  "batch requests",
					url:   "/",
					isErr: true,
					batchBody: []jsonrpc.JSONRPCRequest{
						jsonrpc.JSONRPCRequest{
							Jsonrpc: "1.0",
							Id:      "batch-requests1",
							Request: jsonrpc.Request{
								Method: "foo",
							},
						},
						jsonrpc.JSONRPCRequest{
							Jsonrpc: jsonrpcVersion,
							Id:      "batch-requests2",
							Request: jsonrpc.Request{
								Method: "tools/list",
							},
						},
					},
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      nil,
						"error": map[string]any{
							"code":    -32600.0,
							"message": "not supporting batch requests",
						},
					},
				},
				{
					name: "call tool1 unauthorized tool",
					url:  "/",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "tools-call-tool1",
						Request: jsonrpc.Request{
							Method: "tools/call",
						},
						Params: map[string]any{
							"name": "no_params",
						},
					},
					methodName:     "tools/call",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "tools-call-tool1",
						"result": map[string]any{
							"content": []any{
								map[string]any{
									"type": "text",
									"text": `"no_params"`,
								},
							},
						},
					},
					wantOverwrite: vtc.wantToolsCallOnTool1,
				},
				{
					name: "call tool4 unauthorized tool",
					url:  "/",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "tools-call-tool4",
						Request: jsonrpc.Request{
							Method: "tools/call",
						},
						Params: map[string]any{
							"name": "unauthorized_tool",
						},
					},
					methodName:     "tools/call",
					wantStatusCode: http.StatusUnauthorized,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "tools-call-tool4",
						"error": map[string]any{
							"code":    -32600.0,
							"message": "unauthorized Tool call: Please make sure you specify correct auth headers",
						},
					},
				},
				{
					name: "call tool5 unauthorized tool",
					url:  "/",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "tools-call-tool5",
						Request: jsonrpc.Request{
							Method: "tools/call",
						},
						Params: map[string]any{
							"name": "require_client_auth_tool",
						},
					},
					methodName:     "tools/call",
					wantStatusCode: http.StatusUnauthorized,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "tools-call-tool5",
						"error": map[string]any{
							"code":    -32600.0,
							"message": "missing access token in the 'Authorization' header",
						},
					},
				},
				{
					name: "tools/list with URL param binding",
					url:  "/?param1=bound-string&param2=42&param3=true&param4=3.14&param6=%5B%22a%22%2C%22b%22%5D&param7=%7B%22k%22%3A%22v%22%7D",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "tools-list-url-binding",
						Request: jsonrpc.Request{
							Method: "tools/list",
						},
					},
					methodName:     "tools/list",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "tools-list-url-binding",
						"result": map[string]any{
							"tools": []any{
								map[string]any{
									"name":        "no_params",
									"inputSchema": basicInputSchema,
								},
								map[string]any{
									"name":        "some_params",
									"inputSchema": basicInputSchema,
								},
								map[string]any{
									"name":        "array_param",
									"description": "some description",
									"inputSchema": tool3InputSchema,
								},
								map[string]any{
									"name":        "unauthorized_tool",
									"inputSchema": basicInputSchema,
								},
								map[string]any{
									"name":        "require_client_auth_tool",
									"inputSchema": basicInputSchema,
								},
								map[string]any{
									"name":        "url_binding_tool",
									"description": "A tool for testing URL param binding",
									"inputSchema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"param5": map[string]any{"type": "string", "description": "An unbound string param"},
										},
										"required": []any{"param5"},
									},
								},
							},
						},
					},
					wantOverwrite: vtc.wantToolsListWithURLParam,
				},
				{
					name: "tools/call with URL param binding",
					url:  "/?param1=bound-string&param2=42&param3=true&param4=3.14&param6=%5B%22a%22%2C%22b%22%5D&param7=%7B%22k%22%3A%22v%22%7D",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "tools-call-url-binding",
						Request: jsonrpc.Request{
							Method: "tools/call",
						},
						Params: map[string]any{
							"name": "url_binding_tool",
							"arguments": map[string]any{
								"param5": "unbound-value",
							},
						},
					},
					methodName:     "tools/call",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "tools-call-url-binding",
						"result": map[string]any{
							"content": []any{
								map[string]any{
									"type": "text",
									"text": `"url_binding_tool"`,
								},
								map[string]any{
									"type": "text",
									"text": `"bound-string"`,
								},
								map[string]any{
									"type": "text",
									"text": `42`,
								},
								map[string]any{
									"type": "text",
									"text": `true`,
								},
								map[string]any{
									"type": "text",
									"text": `3.14`,
								},
								map[string]any{
									"type": "text",
									"text": `"unbound-value"`,
								},
								map[string]any{
									"type": "text",
									"text": `["a","b"]`,
								},
								map[string]any{
									"type": "text",
									"text": `{"k":"v"}`,
								},
							},
						},
					},
					wantOverwrite: vtc.wantToolsCallWithURLParam,
				},
				{
					name: "tools/call with URL param override returns error",
					url:  "/?param1=bound-string&param2=42&param3=true&param4=3.14&param6=%5B%22a%22%2C%22b%22%5D&param7=%7B%22k%22%3A%22v%22%7D",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "tools-call-url-binding-override",
						Request: jsonrpc.Request{
							Method: "tools/call",
						},
						Params: map[string]any{
							"name": "url_binding_tool",
							"arguments": map[string]any{
								"param1": "client-override",
								"param5": "unbound-value",
							},
						},
					},
					methodName:     "tools/call",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "tools-call-url-binding-override",
						"result": map[string]any{
							"content": []any{
								map[string]any{
									"type": "text",
									"text": `parameter "param1" is bound by URL and cannot be provided in client arguments`,
								},
							},
							"isError": true,
						},
					},
					wantOverwrite: vtc.wantToolsCallWithURLParamOverrideError,
				},
				{
					name: "tools/call with insufficient parameters returns tool error",
					url:  "/",
					body: jsonrpc.JSONRPCRequest{
						Jsonrpc: jsonrpcVersion,
						Id:      "tools-call-param-error",
						Request: jsonrpc.Request{
							Method: "tools/call",
						},
						Params: map[string]any{
							"name":      "some_params",
							"arguments": map[string]any{},
						},
					},
					methodName:     "tools/call",
					wantStatusCode: http.StatusOK,
					want: map[string]any{
						"jsonrpc": "2.0",
						"id":      "tools-call-param-error",
						"result": map[string]any{
							"content": []any{
								map[string]any{
									"type": "text",
									"text": `provided parameters were invalid: parameter "param1" is required`,
								},
							},
							"isError": true,
						},
					},
					wantOverwrite: vtc.wantToolsCallWithParamError,
				},
			}
			for i := range testCases {
				tc := *testCases[i]
				t.Run(tc.name, func(t *testing.T) {
					// add required header
					header := map[string]string{}
					if sessionId != "" {
						header["Mcp-Session-Id"] = sessionId
					}
					if slices.Contains(vtc.reqHeader, "Mcp-Protocol-Version") {
						header["Mcp-Protocol-Version"] = vtc.protocol
					}
					if slices.Contains(vtc.reqHeader, "Mcp-Method") {
						header["Mcp-Method"] = tc.methodName
					}
					if slices.Contains(vtc.reqHeader, "Mcp-Name") && (tc.methodName == "tools/call" || tc.methodName == "prompts/get") {
						params := tc.body.Params.(map[string]any)
						header["Mcp-Name"] = params["name"].(string)
					}
					if vtc.meta != nil {
						body := tc.body
						if body.Params != nil {
							body.Params.(map[string]any)["_meta"] = vtc.meta
						} else {
							body.Params = map[string]any{
								"_meta": vtc.meta,
							}
						}
						tc.body = body
					}
					reqMarshal, err := json.Marshal(tc.body)
					if err != nil {
						t.Fatalf("unexpected error during marshaling of body")
					}
					if tc.batchBody != nil {
						reqMarshal, err = json.Marshal(tc.batchBody)
						if err != nil {
							t.Fatalf("unexpected error during marshaling of body")
						}
					}

					if vtc.protocol != protocolVersion20241105 && len(header) == 0 {
						t.Fatalf("header is missing")
					}

					resp, body, err := runRequest(ts, http.MethodPost, tc.url, bytes.NewBuffer(reqMarshal), header)

					if slices.Contains(vtc.invalidMethods, tc.methodName) {
						return
					}

					if err != nil {
						t.Fatalf("unexpected error during request: %s", err)
					}

					if resp.StatusCode != tc.wantStatusCode {
						t.Errorf("StatusCode mismatch: got %d, want %d", resp.StatusCode, tc.wantStatusCode)
					}

					want := tc.want
					if tc.wantOverwrite != nil {
						want = tc.wantOverwrite
					}
					// Notifications don't expect a response.
					if want != nil {
						if contentType := resp.Header.Get("Content-type"); contentType != "application/json" {
							t.Fatalf("unexpected content-type header: want %s, got %s", "application/json", contentType)
						}

						var got map[string]any
						if err := json.Unmarshal(body, &got); err != nil {
							t.Fatalf("unexpected error unmarshalling body: %s", err)
						}
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("unexpected response: got %#v, want %#v", got, want)
						}
					}
				})
			}
		})
	}
}

// TestMcpGroupsMethods checks that groups/list and groups/get are served only
// on the latest protocol version, and only to clients that declared the
// com.google.cloud/toolbox.v1 extension.
func TestMcpGroupsMethods(t *testing.T) {
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2}
	toolsMap, promptsMap, groups := testutils.SetUpResources(t, mockTools, nil)
	r, shutdown := setUpServer(t, "mcp", toolsMap, promptsMap, groups)
	defer shutdown()
	ts := runServer(r, false)
	defer ts.Close()

	meta := func(extensions map[string]any) map[string]any {
		return map[string]any{
			"io.modelcontextprotocol/protocolVersion": protocolVersion20260728,
			"io.modelcontextprotocol/clientInfo": map[string]any{
				"version": "client-temp-version",
				"name":    "client-name",
			},
			"io.modelcontextprotocol/clientCapabilities": map[string]any{
				"extensions": extensions,
			},
		}
	}
	toolboxExt := map[string]any{"com.google.cloud/toolbox.v1": map[string]any{}}
	serverInfoMeta := map[string]any{
		"io.modelcontextprotocol/serverInfo": map[string]any{"name": serverName, "version": testutils.MockVersionString},
	}

	testCases := []struct {
		name     string
		protocol string
		method   string
		params   map[string]any
		want     map[string]any
	}{
		{
			name:     "groups/list with extension",
			protocol: protocolVersion20260728,
			method:   "groups/list",
			params:   map[string]any{"_meta": meta(toolboxExt)},
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "groups-req",
				"result": map[string]any{
					"resultType": "complete",
					"groups": []any{
						map[string]any{"name": "tool1_only"},
						map[string]any{"name": "tool2_only"},
					},
					"_meta": serverInfoMeta,
				},
			},
		},
		{
			name:     "groups/get with extension",
			protocol: protocolVersion20260728,
			method:   "groups/get",
			params:   map[string]any{"name": "tool1_only", "_meta": meta(toolboxExt)},
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "groups-req",
				"result": map[string]any{
					"resultType": "complete",
					"name":       "tool1_only",
					"tools": []any{
						map[string]any{"name": "no_params", "inputSchema": basicInputSchema},
					},
					"prompts":    []any{},
					"ttlMs":      300000.0,
					"cacheScope": "public",
					"_meta":      serverInfoMeta,
				},
			},
		},
		{
			name:     "groups/list without extension",
			protocol: protocolVersion20260728,
			method:   "groups/list",
			params:   map[string]any{"_meta": meta(nil)},
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "groups-req",
				"error": map[string]any{
					"code":    -32021.0,
					"message": `missing required client capability: method "groups/list" requires com.google.cloud/toolbox.v1 extension which is not supported by the client`,
				},
			},
		},
		{
			name:     "groups/get without extension",
			protocol: protocolVersion20260728,
			method:   "groups/get",
			params:   map[string]any{"name": "tool1_only", "_meta": meta(nil)},
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "groups-req",
				"error": map[string]any{
					"code":    -32021.0,
					"message": `missing required client capability: method "groups/get" requires com.google.cloud/toolbox.v1 extension which is not supported by the client`,
				},
			},
		},
		{
			name:     "groups/list is not served on an earlier protocol version",
			protocol: protocolVersion20251125,
			method:   "groups/list",
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "groups-req",
				"error": map[string]any{
					"code":    -32601.0,
					"message": "invalid method groups/list",
				},
			},
		},
		{
			name:     "groups/get is not served on an earlier protocol version",
			protocol: protocolVersion20251125,
			method:   "groups/get",
			params:   map[string]any{"name": "tool1_only"},
			want: map[string]any{
				"jsonrpc": "2.0",
				"id":      "groups-req",
				"error": map[string]any{
					"code":    -32601.0,
					"message": "invalid method groups/get",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body := jsonrpc.JSONRPCRequest{
				Jsonrpc: jsonrpcVersion,
				Id:      "groups-req",
				Request: jsonrpc.Request{Method: tc.method},
			}
			if tc.params != nil {
				body.Params = tc.params
			}
			reqMarshal, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("unexpected error during marshaling of body: %s", err)
			}

			header := map[string]string{
				"Mcp-Protocol-Version": tc.protocol,
				"Mcp-Method":           tc.method,
			}
			if tc.method == "groups/get" {
				header["Mcp-Name"] = tc.params["name"].(string)
			}

			_, respBody, err := runRequest(ts, http.MethodPost, "/", bytes.NewBuffer(reqMarshal), header)
			if err != nil {
				t.Fatalf("unexpected error during request: %s", err)
			}
			var got map[string]any
			if err := json.Unmarshal(respBody, &got); err != nil {
				t.Fatalf("unexpected error unmarshalling body: %s", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unexpected response: got %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestMcpEndpointWithoutEnablingDraftSpecs checks a method on draft specs
// without enabling draft specs in server. The server should response with
// unsupported protocol version errror.
func TestMcpEndpointWithoutEnablingDraftSpecs(t *testing.T) {
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2, testutils.MockTool3, testutils.MockTool4, testutils.MockTool5}
	mockPrompts := []testutils.MockPrompt{testutils.MockPrompt1, testutils.MockPrompt2}
	toolsMap, promptsMap, groups := testutils.SetUpResources(t, mockTools, mockPrompts)
	r, shutdown := setUpServer(t, "mcp", toolsMap, promptsMap, groups)
	defer shutdown()
	ts := runServer(r, false)
	defer ts.Close()

	protocol := "DRAFT"
	meta := map[string]any{
		"io.modelcontextprotocol/protocolVersion": protocol,
		"io.modelcontextprotocol/clientInfo": map[string]any{
			"version": "client-temp-version",
			"name":    "client-name",
		},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}

	url := "/"
	body := jsonrpc.JSONRPCRequest{
		Jsonrpc: jsonrpcVersion,
		Id:      "server-discover",
		Request: jsonrpc.Request{
			Method: "server/discover",
		},
	}
	methodName := "server/discover"
	wantStatusCode := http.StatusBadRequest
	want := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      "server-discover",
		"error": map[string]interface{}{
			"code": float64(-32022),
			"data": map[string]interface{}{
				"requested": "DRAFT",
				"supported": []interface{}{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25", "2026-07-28"},
			},
			"message": "unsupported protocol version",
		},
	}
	// add required header
	header := map[string]string{}
	header["Mcp-Protocol-Version"] = protocol
	header["Mcp-Method"] = methodName
	if body.Params != nil {
		body.Params.(map[string]any)["_meta"] = meta
	} else {
		body.Params = map[string]any{
			"_meta": meta,
		}
	}
	reqMarshal, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("unexpected error during marshaling of body")
	}

	if protocol != protocolVersion20241105 && len(header) == 0 {
		t.Fatalf("header is missing")
	}

	resp, resBody, err := runRequest(ts, http.MethodPost, url, bytes.NewBuffer(reqMarshal), header)

	if err != nil {
		t.Fatalf("unexpected error during request: %s", err)
	}

	if resp.StatusCode != wantStatusCode {
		t.Errorf("StatusCode mismatch: got %d, want %d", resp.StatusCode, wantStatusCode)
	}

	if contentType := resp.Header.Get("Content-type"); contentType != "application/json" {
		t.Fatalf("unexpected content-type header: want %s, got %s", "application/json", contentType)
	}

	var got map[string]any
	if err := json.Unmarshal(resBody, &got); err != nil {
		t.Fatalf("unexpected error unmarshalling body: %s", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected response: got %#v, want %#v", got, want)
	}
}

func TestInvalidProtocolVersionHeader(t *testing.T) {
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2, testutils.MockTool3, testutils.MockTool4, testutils.MockTool5}
	mockPrompts := []testutils.MockPrompt{testutils.MockPrompt1}
	toolsMap, promptsMap, groups := testutils.SetUpResources(t, mockTools, mockPrompts)
	r, shutdown := setUpServer(t, "mcp", toolsMap, promptsMap, groups)
	defer shutdown()
	ts := runServer(r, false)
	defer ts.Close()

	reqBody := jsonrpc.JSONRPCRequest{
		Jsonrpc: jsonrpcVersion,
		Id:      "tools-list",
		Request: jsonrpc.Request{
			Method: "tools/list",
		},
	}
	reqMarshal, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("unexpected error during marshaling of body")
	}
	header := map[string]string{}
	header["MCP-Protocol-Version"] = "foo"

	resp, body, err := runRequest(ts, http.MethodPost, "/", bytes.NewBuffer(reqMarshal), header)
	if resp.Status != "400 Bad Request" {
		t.Fatalf("unexpected status: %s; %s", resp.Status, body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unexpected error unmarshalling body: %s", err)
	}
	errMap, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'error' field to be a map, got %T", got["error"])
	}
	msg, ok := errMap["message"].(string)
	if !ok {
		t.Fatalf("expected 'message' field to be a string, got %T", errMap["message"])
	}
	want := "unsupported protocol version"
	if msg != want {
		t.Fatalf("unexpected error message: got %s, want %s", got["error"], want)
	}
	if err != nil {
		t.Fatalf("unexpected error during request: %s", err)
	}
}

func TestDeleteEndpoint(t *testing.T) {
	r, shutdown := setUpServer(t, "mcp", nil, nil, nil)
	defer shutdown()
	ts := runServer(r, false)
	defer ts.Close()

	resp, _, err := runRequest(ts, http.MethodDelete, "/", nil, nil)
	if resp.Status != "200 OK" {
		t.Fatalf("unexpected status: %s", resp.Status)
	}
	if err != nil {
		t.Fatalf("unexpected error during request: %s", err)
	}
}

func TestGetEndpoint(t *testing.T) {
	r, shutdown := setUpServer(t, "mcp", nil, nil, nil)
	defer shutdown()
	ts := runServer(r, false)
	defer ts.Close()

	resp, body, err := runRequest(ts, http.MethodGet, "/", nil, nil)
	if resp.Status != "405 Method Not Allowed" {
		t.Fatalf("unexpected status: %s", resp.Status)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unexpected error unmarshalling body: %s", err)
	}
	want := "toolbox does not support streaming in streamable HTTP transport"
	if got["error"] != want {
		t.Fatalf("unexpected error message: %s", got["error"])
	}
	if err != nil {
		t.Fatalf("unexpected error during request: %s", err)
	}
}

func TestMcpRequestBodyLimit(t *testing.T) {
	r, shutdown := setUpServer(t, "mcp", nil, nil, nil)
	defer shutdown()
	ts := runServer(r, false)
	defer ts.Close()

	limit := int(DefaultHTTPMaxRequestBytes)
	tooLarge := bytes.Repeat([]byte("x"), limit+1)
	resp, body, err := runRequest(ts, http.MethodPost, "/", bytes.NewReader(tooLarge), nil)
	if err != nil {
		t.Fatalf("unexpected error during request: %s", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unexpected error unmarshalling body: %s", err)
	}
	errBody, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("response missing error payload: %v", got)
	}
	wantMessage := fmt.Sprintf("request body exceeds %d bytes", DefaultHTTPMaxRequestBytes)
	if errBody["message"] != wantMessage {
		t.Fatalf("unexpected error message: got %v, want %s", errBody["message"], wantMessage)
	}
}

func TestMcpRequestBodyLimitOverride(t *testing.T) {
	customLimit := int64(1 << 20)
	r, shutdown := setUpServer(t, "mcp", nil, nil, nil, withHTTPMaxRequestBytes(customLimit))
	defer shutdown()
	ts := runServer(r, false)
	defer ts.Close()

	tooLarge := bytes.Repeat([]byte("x"), int(customLimit)+1)
	resp, body, err := runRequest(ts, http.MethodPost, "/", bytes.NewReader(tooLarge), nil)
	if err != nil {
		t.Fatalf("unexpected error during request: %s", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unexpected error unmarshalling body: %s", err)
	}
	errBody, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("response missing error payload: %v", got)
	}
	wantMessage := fmt.Sprintf("request body exceeds %d bytes", customLimit)
	if errBody["message"] != wantMessage {
		t.Fatalf("unexpected error message: got %v, want %s", errBody["message"], wantMessage)
	}
}

func TestSseEndpoint(t *testing.T) {
	r, shutdown := setUpServer(t, "mcp", nil, nil, nil)
	defer shutdown()
	ts := runServer(r, false)
	defer ts.Close()
	if !strings.Contains(ts.URL, "http://127.0.0.1") {
		t.Fatalf("unexpected url, got %s", ts.URL)
	}
	tsPort := strings.TrimPrefix(ts.URL, "http://127.0.0.1:")
	tls := runServer(r, true)
	defer tls.Close()
	if !strings.Contains(tls.URL, "https://127.0.0.1") {
		t.Fatalf("unexpected url, got %s", tls.URL)
	}
	tlsPort := strings.TrimPrefix(tls.URL, "https://127.0.0.1:")

	contentType := "text/event-stream"
	cacheControl := "no-cache"
	connection := "keep-alive"

	testCases := []struct {
		name   string
		server *httptest.Server
		path   string
		proto  string
		event  string
	}{
		{
			name:   "basic",
			server: ts,
			path:   "/sse",
			event:  fmt.Sprintf("event: endpoint\ndata: %s/mcp?sessionId=", ts.URL),
		},
		{
			name:   "toolset1",
			server: ts,
			path:   "/tool1_only/sse",
			event:  fmt.Sprintf("event: endpoint\ndata: http://127.0.0.1:%s/mcp/tool1_only?sessionId=", tsPort),
		},
		{
			name:   "promptset1",
			server: ts,
			path:   "/prompt1_only/sse",
			event:  fmt.Sprintf("event: endpoint\ndata: http://127.0.0.1:%s/mcp/prompt1_only?sessionId=", tsPort),
		},
		{
			name:   "basic with http proto",
			server: ts,
			path:   "/sse",
			proto:  "http",
			event:  fmt.Sprintf("event: endpoint\ndata: http://127.0.0.1:%s/mcp?sessionId=", tsPort),
		},
		{
			name:   "basic tls with https proto",
			server: ts,
			path:   "/sse",
			proto:  "https",
			event:  fmt.Sprintf("event: endpoint\ndata: https://127.0.0.1:%s/mcp?sessionId=", tsPort),
		},
		{
			name:   "basic tls",
			server: tls,
			path:   "/sse",
			event:  fmt.Sprintf("event: endpoint\ndata: https://127.0.0.1:%s/mcp?sessionId=", tlsPort),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := runSseRequest(tc.server, tc.path, tc.proto)
			if err != nil {
				t.Fatalf("unable to run sse request: %s", err)
			}
			defer resp.Body.Close()

			if gotContentType := resp.Header.Get("Content-type"); gotContentType != contentType {
				t.Fatalf("unexpected content-type header: want %s, got %s", contentType, gotContentType)
			}
			if gotCacheControl := resp.Header.Get("Cache-Control"); gotCacheControl != cacheControl {
				t.Fatalf("unexpected cache-control header: want %s, got %s", cacheControl, gotCacheControl)
			}
			if gotConnection := resp.Header.Get("Connection"); gotConnection != connection {
				t.Fatalf("unexpected content-type header: want %s, got %s", connection, gotConnection)
			}

			buffer := make([]byte, 1024)
			n, err := resp.Body.Read(buffer)
			if err != nil {
				t.Fatalf("unable to read response: %s", err)
			}
			endpointEvent := string(buffer[:n])
			if !strings.Contains(endpointEvent, tc.event) {
				t.Fatalf("unexpected event: got %s, want to contain %s", endpointEvent, tc.event)
			}
		})
	}
}

func runSseRequest(ts *httptest.Server, path string, proto string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}
	if proto != "" {
		req.Header.Set("X-Forwarded-Proto", proto)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to send request: %w", err)
	}
	return resp, nil
}

// nonFlusherResponseWriter is an http.ResponseWriter that deliberately does not
// implement http.Flusher, used to exercise sseHandler's missing-flusher path.
type nonFlusherResponseWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *nonFlusherResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *nonFlusherResponseWriter) Write(b []byte) (int, error) { return w.body.Write(b) }

func (w *nonFlusherResponseWriter) WriteHeader(status int) { w.status = status }

// TestSseHandlerWriterWithoutFlusher checks that when the ResponseWriter does
// not implement http.Flusher, sseHandler reports a clean 500 instead of falling
// through and dereferencing a nil flusher.
func TestSseHandlerWriterWithoutFlusher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testLogger, err := log.NewStdLogger(os.Stderr, os.Stderr, "warn")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}

	otelShutdown, err := telemetry.SetupOTel(ctx, testutils.MockVersionString, "", false, "", "toolbox")
	if err != nil {
		t.Fatalf("unable to setup otel: %s", err)
	}
	defer func() {
		if err := otelShutdown(ctx); err != nil {
			t.Fatalf("error shutting down OpenTelemetry: %s", err)
		}
	}()

	instrumentation, err := telemetry.CreateTelemetryInstrumentation(testutils.MockVersionString)
	if err != nil {
		t.Fatalf("unable to create custom metrics: %s", err)
	}

	server := &Server{
		version:         testutils.MockVersionString,
		logger:          testLogger,
		instrumentation: instrumentation,
		sseManager:      newSseManager(ctx),
		PrimitiveMgr:    primitives.NewPrimitiveManager(nil, nil, nil, nil, nil, nil),
	}

	req := httptest.NewRequest(http.MethodGet, "/sse", nil).WithContext(ctx)
	w := &nonFlusherResponseWriter{}

	sseHandler(server, w, req)

	if w.status != http.StatusInternalServerError {
		t.Fatalf("expected status %d for a writer without a flusher, got %d", http.StatusInternalServerError, w.status)
	}
}

func TestStdioSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2, testutils.MockTool3}
	mockPrompts := []testutils.MockPrompt{testutils.MockPrompt1, testutils.MockPrompt2}
	toolsMap, promptsMap, groups := testutils.SetUpResources(t, mockTools, mockPrompts)

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("error with Pipe: %s", err)
	}

	testLogger, err := log.NewStdLogger(pw, os.Stderr, "warn")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}

	otelShutdown, err := telemetry.SetupOTel(ctx, testutils.MockVersionString, "", false, "", "toolbox")
	if err != nil {
		t.Fatalf("unable to setup otel: %s", err)
	}
	defer func() {
		err := otelShutdown(ctx)
		if err != nil {
			t.Fatalf("error shutting down OpenTelemetry: %s", err)
		}
	}()

	instrumentation, err := telemetry.CreateTelemetryInstrumentation(testutils.MockVersionString)
	if err != nil {
		t.Fatalf("unable to create custom metrics: %s", err)
	}

	sseManager := newSseManager(ctx)

	primitiveManager := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, groups)

	server := &Server{
		version:         testutils.MockVersionString,
		logger:          testLogger,
		instrumentation: instrumentation,
		sseManager:      sseManager,
		PrimitiveMgr:    primitiveManager,
	}

	in := bufio.NewReader(pr)
	stdioSession := NewStdioSession(server, in, pw)

	// test stdioSession.readLine()
	input := "test readLine function\n"
	_, err = fmt.Fprintf(pw, "%s", input)
	if err != nil {
		t.Fatalf("error writing into pipe w: %s", err)
	}

	line, err := stdioSession.readLine(ctx)
	if err != nil {
		t.Fatalf("error with stdioSession.readLine: %s", err)
	}
	if line != input {
		t.Fatalf("unexpected line: got %s, want %s", line, input)
	}

	// test stdioSession.write()
	write := "test write function"
	err = stdioSession.write(ctx, write)
	if err != nil {
		t.Fatalf("error with stdioSession.write: %s", err)
	}

	read, err := in.ReadString('\n')
	if err != nil {
		t.Fatalf("error reading: %s", err)
	}
	want := fmt.Sprintf(`"%s"`, write) + "\n"
	if read != want {
		t.Fatalf("unexpected read: got %s, want %s", read, want)
	}
}

func TestSseManagerGetNonExistentSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := newSseManager(ctx)

	// Must not panic when session ID doesn't exist in the map.
	session, ok := m.get("non-existent-id")
	if ok {
		t.Error("expected ok to be false for non-existent session")
	}
	if session != nil {
		t.Error("expected nil session for non-existent ID")
	}
}

func TestSseManagerGetNilSessionValue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := newSseManager(ctx)
	m.sseSessions["nil-session-id"] = nil

	session, ok := m.get("nil-session-id")
	if ok {
		t.Error("expected ok to be false for nil session value")
	}
	if session != nil {
		t.Error("expected nil session for nil session value")
	}
}

func TestExtractMeta(t *testing.T) {
	// withTraceContextPropagator registers the W3C trace-context propagator globally
	// for the duration of the test. extractMeta delegates to otel.GetTextMapPropagator,
	// and the default global propagator is a no-op — so without this helper the
	// "extracted" trace context would always be invalid.
	t.Helper()
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	testCases := []struct {
		name                 string
		body                 []byte
		wantEmptyCtx         bool
		wantValidSpanContext bool
		wantTraceId          string
		wantProtocolVersion  string
		wantTelemetryAttr    *util.TelemetryAttributes
		wantClientName       string
		wantClientVersion    string
	}{
		{
			name:         "empty meta",
			body:         []byte(""),
			wantEmptyCtx: true,
		},
		{
			name:         "not json meta",
			body:         []byte("not json"),
			wantEmptyCtx: true,
		},
		{
			name:         "no _meta",
			body:         []byte(`{"params":{}}`),
			wantEmptyCtx: true,
		},
		{
			name:         "no params",
			body:         []byte(`{"method":"tools/call"}`),
			wantEmptyCtx: true,
		},
		{
			name:         "empty meta",
			body:         []byte(`{"params":{"_meta":{}}}`),
			wantEmptyCtx: true,
		},
		{
			name:                 "traceparent only",
			body:                 []byte(`{"params":{"_meta":{"traceparent":"00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"}}}`),
			wantValidSpanContext: true,
			wantEmptyCtx:         true,
		},
		{
			name: "telemetry attributes only",
			body: []byte(`{"params":{"_meta":{"dev.mcp-toolbox/telemetry":{` +
				`"client.name":"toolbox-langchain-python",` +
				`"client.version":"v0.1.0",` +
				`"client.model":"gemini-2.5-flash",` +
				`"client.user.id":"user-123",` +
				`"client.agent.id":"agent-456"}}}}`),
			wantTelemetryAttr: &util.TelemetryAttributes{
				ClientName: "toolbox-langchain-python", ClientVersion: "v0.1.0",
				ClientModel: "gemini-2.5-flash", ClientUserID: "user-123", ClientAgentID: "agent-456",
			},
		},
		{
			name: "traceparent, telemetry and protocol version",
			body: []byte(`{"params":{"_meta":{` +
				`"traceparent":"00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",` +
				`"dev.mcp-toolbox/telemetry":{"client.name":"foo","client.version":"v1"},` +
				`"io.modelcontextprotocol/protocolVersion":"v999"}}}`),
			wantTelemetryAttr: &util.TelemetryAttributes{
				ClientName: "foo", ClientVersion: "v1",
				ClientModel: "", ClientUserID: "", ClientAgentID: "",
			},
			wantValidSpanContext: true,
			wantClientName:       "foo",
			wantClientVersion:    "v1",
			wantProtocolVersion:  "v999",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			metaProtocolVersion, ctx := extractMeta(context.Background(), tc.body)
			ta := util.TelemetryAttributesFromContext(ctx)
			if tc.wantEmptyCtx {
				if ta != nil {
					t.Fatalf("expected no telemetry attributes")
				}
			}

			if !reflect.DeepEqual(ta, tc.wantTelemetryAttr) {
				t.Fatalf("got %#v, want %#v", ta, tc.wantTelemetryAttr)
				if tc.wantClientName != "" && ta.ClientName != tc.wantClientName {
					t.Fatalf("invalid telemetry attr client name: got %s, want %s", ta.ClientName, tc.wantClientName)
				}
				if tc.wantClientVersion != "" && ta.ClientVersion != tc.wantClientVersion {
					t.Fatalf("invalid telemetry attr client version: got %s, want %s", ta.ClientVersion, tc.wantClientVersion)
				}
			}

			if tc.wantValidSpanContext {
				sc := trace.SpanContextFromContext(ctx)
				if !sc.IsValid() {
					t.Fatal("expected valid span context")
				}
				if got := sc.TraceID().String(); got != "0af7651916cd43dd8448eb211c80319c" {
					t.Errorf("trace id mismatch: got %s", got)
				}
			}

			if tc.wantProtocolVersion != metaProtocolVersion {
				t.Fatalf("meta protocol version mismatch: got %s, want %s", metaProtocolVersion, tc.wantProtocolVersion)
			}
		})
	}
}

// TestMcpPromptScopingByGroup is an end-to-end HTTP test that a `prompts/list`
// request sent to a group's MCP endpoint returns only the prompts belonging to
// that group. It stands up the real server with two groups (each scoped to a
// different prompt) and asserts each route surfaces just its own prompt.
func TestMcpPromptScopingByGroup(t *testing.T) {
	toolsMap := map[string]tools.Tool{}
	promptsMap := map[string]prompts.Prompt{
		testutils.MockPrompt1.Name: testutils.MockPrompt1,
		testutils.MockPrompt2.Name: testutils.MockPrompt2,
	}
	groupA, err := group.GroupConfig{Name: "group_a", PromptNames: []string{testutils.MockPrompt1.Name}}.Initialize(toolsMap, promptsMap)
	if err != nil {
		t.Fatalf("unable to initialize group_a: %s", err)
	}
	groupB, err := group.GroupConfig{Name: "group_b", PromptNames: []string{testutils.MockPrompt2.Name}}.Initialize(toolsMap, promptsMap)
	if err != nil {
		t.Fatalf("unable to initialize group_b: %s", err)
	}
	groups := map[string]group.Group{"group_a": groupA, "group_b": groupB}
	r, shutdown := setUpServer(t, "mcp", toolsMap, promptsMap, groups)
	defer shutdown()
	ts := runServer(r, false)
	defer ts.Close()

	testCases := []struct {
		name        string
		url         string
		wantPrompts []any
	}{
		{
			name:        "group_a scopes to its own prompt",
			url:         "/group_a",
			wantPrompts: []any{map[string]any{"name": "prompt1"}},
		},
		{
			name:        "group_b scopes to its own prompt",
			url:         "/group_b",
			wantPrompts: []any{map[string]any{"name": "prompt2", "arguments": prompt2Args}},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := jsonrpc.JSONRPCRequest{Jsonrpc: jsonrpcVersion, Id: "prompts-list", Request: jsonrpc.Request{Method: "prompts/list"}}
			reqMarshal, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("unexpected error marshaling body: %s", err)
			}
			resp, body, err := runRequest(ts, http.MethodPost, tc.url, bytes.NewBuffer(reqMarshal), nil)
			if err != nil {
				t.Fatalf("unexpected error during request: %s", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("StatusCode mismatch: got %d, want %d", resp.StatusCode, http.StatusOK)
			}
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("unexpected error unmarshalling body: %s", err)
			}
			want := map[string]any{"jsonrpc": "2.0", "id": "prompts-list", "result": map[string]any{"prompts": tc.wantPrompts}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("unexpected response: got %#v, want %#v", got, want)
			}
		})
	}
}
