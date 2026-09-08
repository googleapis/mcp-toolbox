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

package v20260728

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// Dummy JSONRPC ID for testing
var (
	dummyID           jsonrpc.RequestId = 1
	fakeVersionString                   = "0.0.0"
)

// mustGroup fetches the default group from the resource manager.
func mustGroup(t *testing.T, rm *primitives.PrimitiveManager) group.Group {
	t.Helper()
	g, ok := rm.GetGroup("")
	if !ok {
		t.Fatal("default group not found")
	}
	return g
}

func TestValidateMetadata(t *testing.T) {
	var dummyId jsonrpc.RequestId
	clientCapabilities := &ClientCapabilities{}

	tests := []struct {
		name        string
		params      RequestParams
		stdio       bool
		wantErr     bool
		errContains string
	}{
		{
			name: "Missing Meta entirely",
			params: RequestParams{
				Meta: nil,
			},
			stdio:       true,
			wantErr:     true,
			errContains: "missing required fields in request metadata",
		},
		{
			name: "Missing Protocol Version",
			params: RequestParams{
				Meta: &RequestMetaObject{}, // ProtocolVersion defaults to ""
			},
			stdio:       true,
			wantErr:     true,
			errContains: "missing io.modelcontextprotocol/protocolVersion",
		},
		{
			name: "Protocol Version Mismatch (non-stdio)",
			params: RequestParams{
				Meta: &RequestMetaObject{
					ProtocolVersion: "invalid-version-999",
				},
			},
			stdio:       false,
			wantErr:     true,
			errContains: "header mismatch",
		},
		{
			name: "Missing ClientInfo Name",
			params: RequestParams{
				Meta: &RequestMetaObject{
					ProtocolVersion: PROTOCOL_VERSION,
					ClientInfo: Implementation{
						Version:      "1.0",
						BaseMetadata: BaseMetadata{Name: ""}, // Missing name
					},
				},
			},
			stdio:       true,
			wantErr:     true,
			errContains: "missing field from io.modelcontextprotocol/clientInfo",
		},
		{
			name: "Missing ClientInfo Version",
			params: RequestParams{
				Meta: &RequestMetaObject{
					ProtocolVersion: PROTOCOL_VERSION,
					ClientInfo: Implementation{
						BaseMetadata: BaseMetadata{Name: "TestClient"},
						Version:      "", // Missing version
					},
				},
			},
			stdio:       true,
			wantErr:     true,
			errContains: "missing field from io.modelcontextprotocol/clientInfo",
		},
		{
			name: "Missing Client Capabilities",
			params: RequestParams{
				Meta: &RequestMetaObject{
					ProtocolVersion: PROTOCOL_VERSION,
					ClientInfo: Implementation{
						BaseMetadata: BaseMetadata{Name: "TestClient"},
						Version:      "1.0",
					},
					MetaClientCapabilities: nil, // Missing capabilities
				},
			},
			stdio:       true,
			wantErr:     true,
			errContains: "missing field from io.modelcontextprotocol/clientCapabilities",
		},
		{
			name: "stdio transport",
			params: RequestParams{
				Meta: &RequestMetaObject{
					// ProtocolVersion can be anything if stdio is true
					// Technically it will be valid and would already be
					// verified during message processing
					ProtocolVersion: "any-version",
					ClientInfo: Implementation{
						BaseMetadata: BaseMetadata{Name: "TestClient"},
						Version:      "1.0",
					},
					MetaClientCapabilities: clientCapabilities,
				},
			},
			stdio:   true,
			wantErr: false,
		},
		{
			name: "Success request metadata",
			params: RequestParams{
				Meta: &RequestMetaObject{
					ProtocolVersion: PROTOCOL_VERSION, // Must match exactly when stdio=false
					ClientInfo: Implementation{
						BaseMetadata: BaseMetadata{Name: "TestClient"},
						Version:      "1.0",
					},
					MetaClientCapabilities: clientCapabilities,
				},
			},
			stdio:   false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := validateMetadata(dummyId, tt.params, tt.stdio)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateMetadata() expected an error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateMetadata() error = %v, want error containing %q", err, tt.errContains)
				}
				if res == nil {
					t.Errorf("validateMetadata() expected jsonrpc error response, got nil res")
				}
			} else {
				if err != nil {
					t.Errorf("validateMetadata() expected no error, got %v", err)
				}
				if res != nil {
					t.Errorf("validateMetadata() expected nil res on success, got %v", res)
				}
			}
		})
	}
}

func TestValidateHeader(t *testing.T) {
	tests := []struct {
		name    string
		header  http.Header
		method  string
		reqName string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil header (stdio transport)",
			header:  nil,
			method:  "test-method",
			reqName: "test-name",
			wantErr: false,
		},
		{
			name: "valid header matches body",
			header: http.Header{
				"Mcp-Method": []string{"test-method"},
				"Mcp-Name":   []string{"test-name"},
			},
			method:  "test-method",
			reqName: "test-name",
			wantErr: false,
		},
		{
			name: "mismatched method",
			header: http.Header{
				"Mcp-Method": []string{"wrong-method"},
				"Mcp-Name":   []string{"test-name"},
			},
			method:  "test-method",
			reqName: "test-name",
			wantErr: true,
			errMsg:  "Mcp-Method header value 'wrong-method' does not match body value 'test-method'",
		},
		{
			name: "mismatched name",
			header: http.Header{
				"Mcp-Method": []string{"test-method"},
				"Mcp-Name":   []string{"wrong-name"},
			},
			method:  "test-method",
			reqName: "test-name",
			wantErr: true,
			errMsg:  "Mcp-Name header value 'wrong-name' does not match body value 'test-name'",
		},
		{
			name: "missing method in header",
			header: http.Header{
				"Mcp-Name": []string{"test-name"},
			},
			method:  "test-method",
			reqName: "test-name",
			wantErr: true,
			errMsg:  "Mcp-Method header value '' does not match body value 'test-method'",
		},
		{
			name: "missing name in header",
			header: http.Header{
				"Mcp-Method": []string{"test-method"},
			},
			method:  "test-method",
			reqName: "test-name",
			wantErr: true,
			errMsg:  "Mcp-Name header value '' does not match body value 'test-name'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotObj, err := validateHeader(dummyID, tt.header, tt.method, tt.reqName)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateHeader() expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateHeader() error = %v, wantMsg %v", err, tt.errMsg)
				}
				if gotObj == nil {
					t.Errorf("validateHeader() expected an error object return value, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("validateHeader() unexpected error: %v", err)
				}
				if gotObj != nil {
					t.Errorf("validateHeader() expected nil object, got %v", gotObj)
				}
			}
		})
	}
}

func TestServerDiscoverHandler(t *testing.T) {
	origExts := ServerExtensions
	t.Cleanup(func() {
		ServerExtensions = origExts
	})
	Initialize(nil)

	ctx, cancel := context.WithCancel(context.Background())
	ctx = util.WithEnableDraftSpecs(ctx, true)
	defer cancel()
	ctxVersion := util.WithToolboxVersionKey(ctx, fakeVersionString)
	tests := []struct {
		name        string
		body        DiscoverRequest
		rawBody     []byte
		header      http.Header
		context     context.Context
		wantErr     bool
		errContains string
	}{
		{
			name: "missing version in context",
			body: DiscoverRequest{
				Request: jsonrpc.Request{
					Method: "server/discover",
				},
				Params: RequestParams{
					Meta: &RequestMetaObject{
						ProtocolVersion: PROTOCOL_VERSION,
						ClientInfo: Implementation{
							BaseMetadata: BaseMetadata{Name: "TestClient"},
							Version:      "1.0",
						},
						MetaClientCapabilities: &ClientCapabilities{},
					},
				},
			},
			header:      nil,
			context:     ctx,
			wantErr:     true,
			errContains: "unable to retrieve toolbox version",
		},
		{
			name:        "invalid json body",
			rawBody:     []byte(`{invalid json}`),
			header:      nil,
			context:     ctxVersion,
			wantErr:     true,
			errContains: "invalid server discover request",
		},
		{
			name: "header validation failure",
			body: DiscoverRequest{
				Request: jsonrpc.Request{
					Method: "server/discover",
				},
				Params: RequestParams{
					Meta: &RequestMetaObject{
						ProtocolVersion: PROTOCOL_VERSION,
						ClientInfo: Implementation{
							BaseMetadata: BaseMetadata{Name: "TestClient"},
							Version:      "1.0",
						},
						MetaClientCapabilities: &ClientCapabilities{},
					},
				},
			},
			header:      http.Header{"Mcp-Method": []string{"WRONG_METHOD"}},
			context:     ctxVersion,
			wantErr:     true,
			errContains: "does not match body value",
		},
		{
			name: "success",
			body: DiscoverRequest{
				Request: jsonrpc.Request{
					Method: "server/discover",
				},
				Params: RequestParams{
					Meta: &RequestMetaObject{
						ProtocolVersion: PROTOCOL_VERSION,
						ClientInfo: Implementation{
							BaseMetadata: BaseMetadata{Name: "TestClient"},
							Version:      "1.0",
						},
						MetaClientCapabilities: &ClientCapabilities{},
					},
				},
			},
			header:  http.Header{"Mcp-Method": []string{SERVER_DISCOVER}},
			context: ctxVersion,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.rawBody
			var err error
			if body == nil {
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("unexpected error during marshaling")
				}
			}
			got, err := serverDiscoverHandler(tt.context, dummyID, body, tt.header)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil {
					t.Errorf("expected valid response, got nil")
				} else if res, ok := got.(jsonrpc.JSONRPCResponse); ok {
					if discoverRes, ok := res.Result.(DiscoverResult); ok {
						if _, ok := discoverRes.Capabilities.Extensions["com.google.cloud/toolbox.v1"]; !ok {
							t.Errorf("expected com.google.cloud/toolbox.v1 in discover capabilities extensions, got %v", discoverRes.Capabilities.Extensions)
						}
						if _, ok := discoverRes.Capabilities.Extensions["io.modelcontextprotocol/ui"]; !ok {
							t.Errorf("expected io.modelcontextprotocol/ui in discover capabilities extensions, got %v", discoverRes.Capabilities.Extensions)
						}
					}
				}
				res, ok := got.(jsonrpc.JSONRPCResponse)
				if !ok {
					t.Fatalf("expected response of type jsonrpc.JSONRPCResponse, got %T", got)
				}
				discoverResult, ok := res.Result.(DiscoverResult)
				if !ok {
					t.Fatalf("expected result of type DiscoverResult, got %T", res.Result)
				}
				if discoverResult.Capabilities.Extensions == nil || discoverResult.Capabilities.Extensions["com.google.cloud/toolbox.v1"] == nil {
					t.Errorf("expected %s in Extensions capabilities, got %v", "com.google.cloud/toolbox.v1", discoverResult.Capabilities.Extensions)
				}
				if discoverResult.Capabilities.Extensions == nil || discoverResult.Capabilities.Extensions["io.modelcontextprotocol/ui"] == nil {
					t.Errorf("expected %s in Extensions capabilities, got %v", "io.modelcontextprotocol/ui", discoverResult.Capabilities.Extensions)
				}
			}
		})
	}
}

func TestToolsListHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = util.WithToolboxVersionKey(ctx, "v0.0.0")
	// Initialize tools using provided testutils mock instances
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2}
	toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups := testutils.SetUpPrimitives(t, mockTools, nil, nil, nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups)

	tests := []struct {
		name        string
		body        ListToolsRequest
		rawBody     []byte
		header      http.Header
		g           group.Group
		wantErr     bool
		errContains string
	}{
		{
			name:        "invalid json body",
			rawBody:     []byte(`{invalid json}`),
			header:      nil,
			g:           mustGroup(t, primitiveMgr),
			wantErr:     true,
			errContains: "invalid mcp tools list request",
		},
		{
			name: "header mismatch",
			body: ListToolsRequest{
				PaginatedRequest: PaginatedRequest{
					Request: jsonrpc.Request{
						Method: "tools/list",
					},
					Params: PaginatedRequestParams{
						RequestParams: RequestParams{
							Meta: &RequestMetaObject{
								ProtocolVersion: PROTOCOL_VERSION,
								ClientInfo: Implementation{
									BaseMetadata: BaseMetadata{Name: "TestClient"},
									Version:      "1.0",
								},
								MetaClientCapabilities: &ClientCapabilities{},
							},
						},
					},
				},
			},
			header:      http.Header{"Mcp-Method": []string{"WRONG_METHOD"}},
			g:           mustGroup(t, primitiveMgr),
			wantErr:     true,
			errContains: "does not match body value",
		},
		{
			name: "success - stdio (nil header)",
			body: ListToolsRequest{
				PaginatedRequest: PaginatedRequest{
					Request: jsonrpc.Request{
						Method: "tools/list",
					},
					Params: PaginatedRequestParams{
						RequestParams: RequestParams{
							Meta: &RequestMetaObject{
								ProtocolVersion: PROTOCOL_VERSION,
								ClientInfo: Implementation{
									BaseMetadata: BaseMetadata{Name: "TestClient"},
									Version:      "1.0",
								},
								MetaClientCapabilities: &ClientCapabilities{},
							},
						},
					},
				},
			},
			header:  nil,
			g:       mustGroup(t, primitiveMgr),
			wantErr: false,
		},
		{
			name: "success - http",
			body: ListToolsRequest{
				PaginatedRequest: PaginatedRequest{
					Request: jsonrpc.Request{
						Method: "tools/list",
					},
					Params: PaginatedRequestParams{
						RequestParams: RequestParams{
							Meta: &RequestMetaObject{
								ProtocolVersion: PROTOCOL_VERSION,
								ClientInfo: Implementation{
									BaseMetadata: BaseMetadata{Name: "TestClient"},
									Version:      "1.0",
								},
								MetaClientCapabilities: &ClientCapabilities{},
							},
						},
					},
				},
			},
			header:  http.Header{"Mcp-Method": []string{TOOLS_LIST}},
			g:       mustGroup(t, primitiveMgr),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.rawBody
			var err error
			if body == nil {
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("unexpected error during marshaling")
				}
			}
			got, err := toolsListHandler(ctx, dummyID, primitiveMgr, tt.g, body, tt.header)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want string containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil {
					t.Errorf("expected valid response, got nil")
				}
			}
		})
	}
}

func TestToolsCallHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = util.WithToolboxVersionKey(ctx, "v0.0.0")
	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}
	ctxLogger := util.WithLogger(ctx, testLogger)
	// Setup tools including the auth/unauth ones
	mockTools := []testutils.MockTool{
		testutils.MockTool1,
		testutils.MockTool2,
		testutils.MockTool4,
		testutils.MockTool5,
	}
	toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups := testutils.SetUpPrimitives(t, mockTools, nil, nil, nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups)

	tests := []struct {
		name            string
		body            CallToolRequest
		rawBody         []byte
		header          http.Header
		context         context.Context
		wantErr         bool
		errContains     string
		wantIsError     bool
		wantContentText string
	}{
		{
			name:        "invalid json body",
			rawBody:     []byte(`{invalid json}`),
			header:      nil,
			context:     ctxLogger,
			wantErr:     true,
			errContains: "invalid mcp tools call request",
		},
		{
			name: "missing logger in context",
			body: CallToolRequest{
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: CallToolRequestParams{
					Name: "no_params",
					RequestParams: RequestParams{
						Meta: &RequestMetaObject{
							ProtocolVersion: PROTOCOL_VERSION,
							ClientInfo: Implementation{
								BaseMetadata: BaseMetadata{Name: "TestClient"},
								Version:      "1.0",
							},
							MetaClientCapabilities: &ClientCapabilities{},
						},
					},
				},
			},
			header:      nil,
			context:     ctx,
			wantErr:     true,
			errContains: "unable to retrieve logger",
		},
		{
			name: "tool not in toolset",
			body: CallToolRequest{
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: CallToolRequestParams{
					Name: "unknown_tool",
					RequestParams: RequestParams{
						Meta: &RequestMetaObject{
							ProtocolVersion: PROTOCOL_VERSION,
							ClientInfo: Implementation{
								BaseMetadata: BaseMetadata{Name: "TestClient"},
								Version:      "1.0",
							},
							MetaClientCapabilities: &ClientCapabilities{},
						},
					},
				},
			},
			header:      nil,
			context:     ctxLogger,
			wantErr:     true,
			errContains: "tool with name \"unknown_tool\" does not exist",
		},
		{
			name: "missing client auth token",
			body: CallToolRequest{
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: CallToolRequestParams{
					Name: "require_client_auth_tool",
					RequestParams: RequestParams{
						Meta: &RequestMetaObject{
							ProtocolVersion: PROTOCOL_VERSION,
							ClientInfo: Implementation{
								BaseMetadata: BaseMetadata{Name: "TestClient"},
								Version:      "1.0",
							},
							MetaClientCapabilities: &ClientCapabilities{},
						},
					},
				},
			},
			header:      http.Header{"Mcp-Method": []string{TOOLS_CALL}, "Mcp-Name": []string{"require_client_auth_tool"}},
			context:     ctxLogger,
			wantErr:     true,
			errContains: "missing access token in the 'Authorization' header",
		},
		{
			name: "successful invocation - no params",
			body: CallToolRequest{
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: CallToolRequestParams{
					Name: "no_params",
					RequestParams: RequestParams{
						Meta: &RequestMetaObject{
							ProtocolVersion: PROTOCOL_VERSION,
							ClientInfo: Implementation{
								BaseMetadata: BaseMetadata{Name: "TestClient"},
								Version:      "1.0",
							},
							MetaClientCapabilities: &ClientCapabilities{},
						},
					},
				},
			},
			header:  http.Header{"Mcp-Method": []string{TOOLS_CALL}, "Mcp-Name": []string{"no_params"}},
			context: ctxLogger,
			wantErr: false,
		},
		{
			name: "successful invocation - URL bound parameters auto-populated",
			body: CallToolRequest{
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: CallToolRequestParams{
					Name: "some_params",
					Arguments: map[string]any{
						"param2": 20,
					},
					RequestParams: RequestParams{
						Meta: &RequestMetaObject{
							ProtocolVersion: PROTOCOL_VERSION,
							ClientInfo: Implementation{
								BaseMetadata: BaseMetadata{Name: "TestClient"},
								Version:      "1.0",
							},
							MetaClientCapabilities: &ClientCapabilities{},
						},
					},
				},
			},
			header:      http.Header{"Mcp-Method": []string{TOOLS_CALL}, "Mcp-Name": []string{"some_params"}},
			context:     util.WithUrlParams(ctxLogger, map[string]string{"param1": "10"}),
			wantErr:     false,
			wantIsError: false,
		},
		{
			name: "parameter validation error - missing required param",
			body: CallToolRequest{
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: CallToolRequestParams{
					Name:      "some_params",
					Arguments: map[string]any{},
					RequestParams: RequestParams{
						Meta: &RequestMetaObject{
							ProtocolVersion: PROTOCOL_VERSION,
							ClientInfo: Implementation{
								BaseMetadata: BaseMetadata{Name: "TestClient"},
								Version:      "1.0",
							},
							MetaClientCapabilities: &ClientCapabilities{},
						},
					},
				},
			},
			header:          http.Header{"Mcp-Method": []string{TOOLS_CALL}, "Mcp-Name": []string{"some_params"}},
			context:         ctxLogger,
			wantErr:         false,
			wantIsError:     true,
			wantContentText: `provided parameters were invalid: parameter "param1" is required`,
		},
		{
			name: "URL bound parameter override by client returns error",
			body: CallToolRequest{
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: CallToolRequestParams{
					Name: "some_params",
					Arguments: map[string]any{
						"param1": 10,
						"param2": 20,
					},
					RequestParams: RequestParams{
						Meta: &RequestMetaObject{
							ProtocolVersion: PROTOCOL_VERSION,
							ClientInfo: Implementation{
								BaseMetadata: BaseMetadata{Name: "TestClient"},
								Version:      "1.0",
							},
							MetaClientCapabilities: &ClientCapabilities{},
						},
					},
				},
			},
			header:          http.Header{"Mcp-Method": []string{TOOLS_CALL}, "Mcp-Name": []string{"some_params"}},
			context:         util.WithUrlParams(ctxLogger, map[string]string{"param1": "10"}),
			wantErr:         false,
			wantIsError:     true,
			wantContentText: `parameter "param1" is bound by URL and cannot be provided in client arguments`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.rawBody
			var err error
			if body == nil {
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("unexpected error during marshaling")
				}
			}
			got, err := toolsCallHandler(tt.context, dummyID, mustGroup(t, primitiveMgr), primitiveMgr, body, tt.header)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want string containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil {
					t.Errorf("expected valid response, got nil")
				}
				res, ok := got.(jsonrpc.JSONRPCResponse)
				if !ok {
					t.Fatalf("expected jsonrpc.JSONRPCResponse, got %T", got)
				}
				callResult, ok := res.Result.(CallToolResult)
				if !ok {
					t.Fatalf("expected CallToolResult, got %T", res.Result)
				}
				if callResult.IsError != tt.wantIsError {
					t.Errorf("callResult.IsError = %v, want %v", callResult.IsError, tt.wantIsError)
				}
				if tt.wantIsError {
					if callResult.ResultType != resultTypeComplete {
						t.Errorf("callResult.ResultType = %v, want %v", callResult.ResultType, resultTypeComplete)
					}
					if callResult.Meta == nil {
						t.Errorf("callResult.Meta is nil, expected populated meta")
					}
				}
				if tt.wantContentText != "" {
					if len(callResult.Content) == 0 {
						t.Fatalf("expected content in result, got empty")
					}
					if !strings.Contains(callResult.Content[0].Text, tt.wantContentText) {
						t.Errorf("callResult.Content[0].Text = %q, want string containing %q", callResult.Content[0].Text, tt.wantContentText)
					}
				}
			}
		})
	}
}

func TestPromptsListHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = util.WithToolboxVersionKey(ctx, "v0.0.0")
	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)
	// Initialize primitives
	mockPrompts := []testutils.MockPrompt{testutils.MockPrompt1, testutils.MockPrompt2}
	toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups := testutils.SetUpPrimitives(t, nil, mockPrompts, nil, nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups)
	tests := []struct {
		name        string
		body        ListPromptsRequest
		rawBody     []byte
		header      http.Header
		wantErr     bool
		errContains string
	}{
		{
			name:        "invalid json request",
			rawBody:     []byte(`{invalid json}`),
			header:      nil,
			wantErr:     true,
			errContains: "invalid mcp prompts list request",
		},
		{
			name: "success",
			body: ListPromptsRequest{
				PaginatedRequest: PaginatedRequest{
					Request: jsonrpc.Request{
						Method: "prompts/list",
					},
					Params: PaginatedRequestParams{
						RequestParams: RequestParams{
							Meta: &RequestMetaObject{
								ProtocolVersion: PROTOCOL_VERSION,
								ClientInfo: Implementation{
									BaseMetadata: BaseMetadata{Name: "TestClient"},
									Version:      "1.0",
								},
								MetaClientCapabilities: &ClientCapabilities{},
							},
						},
					},
				},
			},
			header:  http.Header{"Mcp-Method": []string{PROMPTS_LIST}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.rawBody
			var err error
			if body == nil {
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("unexpected error during marshaling")
				}
			}
			got, err := promptsListHandler(ctx, dummyID, primitiveMgr, mustGroup(t, primitiveMgr), body, tt.header)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want string containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil {
					t.Errorf("expected valid response, got nil")
				}
			}
		})
	}
}

func TestPromptsGetHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = util.WithToolboxVersionKey(ctx, "v0.0.0")
	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)
	// Initialize primitives
	mockPrompts := []testutils.MockPrompt{testutils.MockPrompt1, testutils.MockPrompt2}
	toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups := testutils.SetUpPrimitives(t, nil, mockPrompts, nil, nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups)
	tests := []struct {
		name        string
		body        GetPromptRequest
		rawBody     []byte
		header      http.Header
		wantErr     bool
		errContains string
	}{
		{
			name:        "invalid json request",
			rawBody:     []byte(`{invalid json}`),
			header:      nil,
			wantErr:     true,
			errContains: "invalid mcp prompts/get request",
		},
		{
			name: "prompt does not exist",
			body: GetPromptRequest{
				Request: jsonrpc.Request{
					Method: "prompts/get",
				},
				Params: GetPromptRequestParams{
					Name: "missing_prompt",
					RequestParams: RequestParams{
						Meta: &RequestMetaObject{
							ProtocolVersion: PROTOCOL_VERSION,
							ClientInfo: Implementation{
								BaseMetadata: BaseMetadata{Name: "TestClient"},
								Version:      "1.0",
							},
							MetaClientCapabilities: &ClientCapabilities{},
						},
					},
				},
			},
			header:      nil,
			wantErr:     true,
			errContains: "does not exist",
		},
		{
			name: "success with args",
			body: GetPromptRequest{
				Request: jsonrpc.Request{
					Method: "prompts/get",
				},
				Params: GetPromptRequestParams{
					Name: "prompt2",
					Arguments: map[string]any{
						"arg1": "value1",
					},
					RequestParams: RequestParams{
						Meta: &RequestMetaObject{
							ProtocolVersion: PROTOCOL_VERSION,
							ClientInfo: Implementation{
								BaseMetadata: BaseMetadata{Name: "TestClient"},
								Version:      "1.0",
							},
							MetaClientCapabilities: &ClientCapabilities{},
						},
					},
				},
			},
			header:  http.Header{"Mcp-Method": []string{PROMPTS_GET}, "Mcp-Name": []string{"prompt2"}},
			wantErr: false,
		},
		{
			name: "success without args",
			body: GetPromptRequest{
				Request: jsonrpc.Request{
					Method: "prompts/get",
				},
				Params: GetPromptRequestParams{
					Name: "prompt1",
					RequestParams: RequestParams{
						Meta: &RequestMetaObject{
							ProtocolVersion: PROTOCOL_VERSION,
							ClientInfo: Implementation{
								BaseMetadata: BaseMetadata{Name: "TestClient"},
								Version:      "1.0",
							},
							MetaClientCapabilities: &ClientCapabilities{},
						},
					},
				},
			},
			header:  http.Header{"Mcp-Method": []string{PROMPTS_GET}, "Mcp-Name": []string{"prompt1"}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.rawBody
			var err error
			if body == nil {
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("unexpected error during marshaling")
				}
			}
			got, err := promptsGetHandler(ctx, dummyID, mustGroup(t, primitiveMgr), primitiveMgr, body, tt.header)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want string containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil {
					t.Errorf("expected valid response, got nil")
				}
			}
		})
	}
}

func TestGroupsListHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2}
	toolsMap, promptsMap, _, _, groups := testutils.SetUpPrimitives(t, mockTools, nil, nil, nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, nil, nil, groups)

	validMeta := &RequestMetaObject{
		ProtocolVersion: PROTOCOL_VERSION,
		ClientInfo: Implementation{
			BaseMetadata: BaseMetadata{Name: "TestClient"},
			Version:      "1.0",
		},
		MetaClientCapabilities: &ClientCapabilities{},
	}

	tests := []struct {
		name        string
		rawBody     []byte
		body        ListGroupsRequest
		header      http.Header
		wantErr     bool
		errContains string
		wantNames   []string
	}{
		{
			name:        "invalid json body",
			rawBody:     []byte(`{invalid json}`),
			wantErr:     true,
			errContains: "invalid mcp groups list request",
		},
		{
			name: "success excludes default group and sorts",
			body: ListGroupsRequest{
				PaginatedRequest: PaginatedRequest{
					Request: jsonrpc.Request{Method: GROUPS_LIST},
					Params: PaginatedRequestParams{
						RequestParams: RequestParams{Meta: validMeta},
					},
				},
			},
			header:    http.Header{"Mcp-Method": []string{GROUPS_LIST}},
			wantErr:   false,
			wantNames: []string{"tool1_only", "tool2_only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.rawBody
			var err error
			if body == nil {
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("unexpected error during marshaling: %v", err)
				}
			}
			got, err := groupsListHandler(ctx, dummyID, primitiveMgr, body, tt.header)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want string containing %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			res, ok := got.(jsonrpc.JSONRPCResponse)
			if !ok {
				t.Fatalf("expected jsonrpc.JSONRPCResponse, got %T", got)
			}
			result, ok := res.Result.(ListGroupsResult)
			if !ok {
				t.Fatalf("expected ListGroupsResult, got %T", res.Result)
			}
			gotNames := make([]string, 0, len(result.Groups))
			for _, g := range result.Groups {
				gotNames = append(gotNames, g.Name)
			}
			if len(gotNames) != len(tt.wantNames) {
				t.Fatalf("got groups %v, want %v", gotNames, tt.wantNames)
			}
			for i, n := range tt.wantNames {
				if gotNames[i] != n {
					t.Errorf("group[%d] = %q, want %q", i, gotNames[i], n)
				}
			}
		})
	}
}

func TestGroupsGetHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2}
	toolsMap, promptsMap, _, _, groups := testutils.SetUpPrimitives(t, mockTools, nil, nil, nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, nil, nil, groups)

	validMeta := &RequestMetaObject{
		ProtocolVersion: PROTOCOL_VERSION,
		ClientInfo: Implementation{
			BaseMetadata: BaseMetadata{Name: "TestClient"},
			Version:      "1.0",
		},
		MetaClientCapabilities: &ClientCapabilities{},
	}

	tests := []struct {
		name        string
		rawBody     []byte
		body        GetGroupRequest
		header      http.Header
		wantErr     bool
		errContains string
		wantName    string
	}{
		{
			name:        "invalid json body",
			rawBody:     []byte(`{invalid json}`),
			wantErr:     true,
			errContains: "invalid mcp groups/get request",
		},
		{
			name: "group does not exist",
			body: GetGroupRequest{
				Request: jsonrpc.Request{Method: GROUPS_GET},
				Params: GetGroupRequestParams{
					RequestParams: RequestParams{Meta: validMeta},
					Name:          "missing_group",
				},
			},
			header:      http.Header{"Mcp-Method": []string{GROUPS_GET}, "Mcp-Name": []string{"missing_group"}},
			wantErr:     true,
			errContains: `group with name "missing_group" does not exist`,
		},
		{
			name: "success",
			body: GetGroupRequest{
				Request: jsonrpc.Request{Method: GROUPS_GET},
				Params: GetGroupRequestParams{
					RequestParams: RequestParams{Meta: validMeta},
					Name:          "tool1_only",
				},
			},
			header:   http.Header{"Mcp-Method": []string{GROUPS_GET}, "Mcp-Name": []string{"tool1_only"}},
			wantErr:  false,
			wantName: "tool1_only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.rawBody
			var err error
			if body == nil {
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("unexpected error during marshaling: %v", err)
				}
			}
			got, err := groupsGetHandler(ctx, dummyID, primitiveMgr, body, tt.header)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want string containing %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			res, ok := got.(jsonrpc.JSONRPCResponse)
			if !ok {
				t.Fatalf("expected jsonrpc.JSONRPCResponse, got %T", got)
			}
			result, ok := res.Result.(GetGroupResult)
			if !ok {
				t.Fatalf("expected GetGroupResult, got %T", res.Result)
			}
			if result.Name != tt.wantName {
				t.Errorf("result.Name = %q, want %q", result.Name, tt.wantName)
			}
		})
	}
}

func TestGetResultMetadata(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctxWithVersion := util.WithToolboxVersionKey(ctx, "v0.0.0")
	server_name := "Toolbox"
	// Define the table structure for our test cases
	tests := []struct {
		name       string
		ctx        context.Context
		curMeta    map[string]any
		want       map[string]any
		wantErr    bool
		errMessage string // Optional: check for specific error messages
	}{
		{
			name: "Success - Merge with existing metadata",
			ctx:  ctxWithVersion,
			curMeta: map[string]any{
				"existing_key": "existing_value",
				"another_key":  123,
			},
			want: map[string]any{
				"existing_key": "existing_value",
				"another_key":  123,
				"io.modelcontextprotocol/serverInfo": map[string]any{
					"name":    server_name,
					"version": "v0.0.0",
				},
			},
			wantErr: false,
		},
		{
			name:    "Success - Nil current metadata",
			ctx:     ctxWithVersion,
			curMeta: nil,
			want: map[string]any{
				"io.modelcontextprotocol/serverInfo": map[string]any{
					"name":    server_name,
					"version": "v0.0.0",
				},
			},
			wantErr: false,
		},
		{
			name: "Success - Overwrites duplicate keys in metadata",
			ctx:  ctxWithVersion,
			curMeta: map[string]any{
				"io.modelcontextprotocol/serverInfo": "old_data",
				"other_key":                          true,
			},
			want: map[string]any{
				"other_key": true,
				"io.modelcontextprotocol/serverInfo": map[string]any{
					"name":    server_name,
					"version": "v0.0.0",
				},
			},
			wantErr: false,
		},
		{
			name: "Failure - Context error (version retrieval fails)",
			ctx:  context.Background(),
			curMeta: map[string]any{
				"some_data": "value",
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.ctx
			got, err := getResultMetadata(ctx, tt.curMeta)
			if (err != nil) != tt.wantErr {
				t.Fatalf("getResultMetadata() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getResultMetadata() got =\n%v\nwant =\n%v", got, tt.want)
			}
		})
	}
}
func TestToolsCallHandlerWithSecureParams(t *testing.T) {
	origExts := ServerExtensions
	t.Cleanup(func() {
		ServerExtensions = origExts
	})
	Initialize(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = util.WithToolboxVersionKey(ctx, "v0.0.0")
	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}
	ctxLogger := util.WithLogger(ctx, testLogger)

	secureTool := testutils.NewMockTool(
		"secure_tool",
		"A tool with secure parameters",
		"",
		parameters.Parameters{
			&parameters.StringParameter{
				CommonParameter: parameters.CommonParameter{
					Name:     "api_key",
					Type:     parameters.TypeString,
					Desc:     "A secure api key",
					Required: &[]bool{true}[0],
					Secure:   true,
				},
			},
			parameters.NewStringParameter("query", "A standard search query"),
		},
		false,
		false,
	)

	toolsMap := map[string]tools.Tool{
		"secure_tool": secureTool,
	}

	g := group.NewGroup(group.GroupConfig{
		Name:      "test-toolset",
		ToolNames: []string{"secure_tool"},
	})
	groups := map[string]group.Group{
		"":             g,
		"test-toolset": g,
	}
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, nil, nil, nil, groups)

	tests := []struct {
		desc            string
		urlParams       map[string]string
		body            string // raw JSON-RPC body
		wantErr         bool
		errContains     string
		wantIsError     bool
		wantContentText string
	}{
		{
			desc: "Client does not support secure parameters",
			body: `{
				"jsonrpc": "2.0",
				"id": 1,
				"method": "tools/call",
				"params": {
					"name": "secure_tool",
					"arguments": {
						"query": "hello"
					},
					"secureArguments": {
						"api_key": "secret"
					},
					"_meta": {
						"io.modelcontextprotocol/protocolVersion": "2026-07-28",
						"io.modelcontextprotocol/clientInfo": {
							"name": "TestClient",
							"version": "1.0"
						},
						"io.modelcontextprotocol/clientCapabilities": {}
					}
				}
			}`,
			wantErr:     true,
			errContains: "missing required client capability: tool \"secure_tool\" requires com.google.cloud/toolbox.v1 extension which is not supported by the client",
		},
		{
			desc: "Secure parameter passed in standard arguments",
			body: `{
				"jsonrpc": "2.0",
				"id": 1,
				"method": "tools/call",
				"params": {
					"name": "secure_tool",
					"arguments": {
						"query": "hello",
						"api_key": "secret"
					},
					"_meta": {
						"io.modelcontextprotocol/protocolVersion": "2026-07-28",
						"io.modelcontextprotocol/clientInfo": {
							"name": "TestClient",
							"version": "1.0"
						},
						"io.modelcontextprotocol/clientCapabilities": {
							"extensions": {
								"com.google.cloud/toolbox.v1": {}
							}
						}
					}
				}
			}`,
			wantErr:         false,
			wantIsError:     true,
			wantContentText: `parameter "api_key" is secure and must not be passed in standard arguments`,
		},
		{
			desc: "Standard parameter passed in secureArguments",
			body: `{
				"jsonrpc": "2.0",
				"id": 1,
				"method": "tools/call",
				"params": {
					"name": "secure_tool",
					"arguments": {},
					"secureArguments": {
						"query": "hello",
						"api_key": "secret"
					},
					"_meta": {
						"io.modelcontextprotocol/protocolVersion": "2026-07-28",
						"io.modelcontextprotocol/clientInfo": {
							"name": "TestClient",
							"version": "1.0"
						},
						"io.modelcontextprotocol/clientCapabilities": {
							"extensions": {
								"com.google.cloud/toolbox.v1": {}
							}
						}
					}
				}
			}`,
			wantErr:     true,
			errContains: "parameter \"query\" is not secure and must not be passed in secureArguments",
		},
		{
			desc: "Missing required secure parameter",
			body: `{
				"jsonrpc": "2.0",
				"id": 1,
				"method": "tools/call",
				"params": {
					"name": "secure_tool",
					"arguments": {
						"query": "hello"
					},
					"_meta": {
						"io.modelcontextprotocol/protocolVersion": "2026-07-28",
						"io.modelcontextprotocol/clientInfo": {
							"name": "TestClient",
							"version": "1.0"
						},
						"io.modelcontextprotocol/clientCapabilities": {
							"extensions": {
								"com.google.cloud/toolbox.v1": {}
							}
						}
					}
				}
			}`,
			wantErr:     true,
			errContains: `missing required secure parameter "api_key" in secureArguments`,
		},
		{
			desc: "Successful invocation with correct routing (extensions)",
			body: `{
				"jsonrpc": "2.0",
				"id": 1,
				"method": "tools/call",
				"params": {
					"name": "secure_tool",
					"arguments": {
						"query": "hello"
					},
					"secureArguments": {
						"api_key": "secret"
					},
					"_meta": {
						"io.modelcontextprotocol/protocolVersion": "2026-07-28",
						"io.modelcontextprotocol/clientInfo": {
							"name": "TestClient",
							"version": "1.0"
						},
						"io.modelcontextprotocol/clientCapabilities": {
							"extensions": {
								"com.google.cloud/toolbox.v1": {}
							}
						}
					}
				}
			}`,
			wantErr: false,
		},
		{
			desc: "Successful invocation with secure parameter bound via URL params",
			urlParams: map[string]string{
				"api_key": "secret",
			},
			body: `{
				"jsonrpc": "2.0",
				"id": 1,
				"method": "tools/call",
				"params": {
					"name": "secure_tool",
					"arguments": {
						"query": "hello"
					},
					"_meta": {
						"io.modelcontextprotocol/protocolVersion": "2026-07-28",
						"io.modelcontextprotocol/clientInfo": {
							"name": "TestClient",
							"version": "1.0"
						},
						"io.modelcontextprotocol/clientCapabilities": {
							"extensions": {
								"com.google.cloud/toolbox.v1": {}
							}
						}
					}
				}
			}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := ctxLogger
			if tt.urlParams != nil {
				ctx = util.WithUrlParams(ctx, tt.urlParams)
			}
			got, err := toolsCallHandler(ctx, dummyID, g, primitiveMgr, []byte(tt.body), nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want string containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil {
					t.Errorf("expected valid response, got nil")
				}
				if tt.wantIsError {
					res, ok := got.(jsonrpc.JSONRPCResponse)
					if !ok {
						t.Fatalf("expected jsonrpc.JSONRPCResponse, got %T", got)
					}
					callResult, ok := res.Result.(CallToolResult)
					if !ok {
						t.Fatalf("expected CallToolResult, got %T", res.Result)
					}
					if !callResult.IsError {
						t.Errorf("callResult.IsError = false, want true")
					}
					if tt.wantContentText != "" {
						if len(callResult.Content) == 0 {
							t.Fatalf("expected content in result, got empty")
						}
						if !strings.Contains(callResult.Content[0].Text, tt.wantContentText) {
							t.Errorf("callResult.Content[0].Text = %q, want string containing %q", callResult.Content[0].Text, tt.wantContentText)
						}
					}
				}
			}
		})
	}
}

func TestResourcesListHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = util.WithToolboxVersionKey(ctx, "v0.0.0")
	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	sizeVal := int64(2048)
	mockResources := []testutils.MockResource{
		testutils.NewMockResource("res1", "file:///res1", "", "", "", nil, nil),
		testutils.NewMockResource("res2", "file:///res2", "Title 2", "", "application/json", &sizeVal, &resources.ResourceAnnotations{LastModified: "2024-01-01T00:00:00Z"}),
	}
	toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups := testutils.SetUpPrimitives(t, nil, nil, mockResources, nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups)

	tests := []struct {
		name        string
		body        ListResourcesRequest
		rawBody     []byte
		wantErr     bool
		errContains string
	}{
		{
			name:        "invalid json request",
			rawBody:     []byte(`{invalid json}`),
			wantErr:     true,
			errContains: "invalid mcp resources list request",
		},
		{
			name: "success",
			body: ListResourcesRequest{
				PaginatedRequest: PaginatedRequest{
					Request: jsonrpc.Request{Method: "resources/list"},
					Params: PaginatedRequestParams{
						RequestParams: RequestParams{
							Meta: &RequestMetaObject{
								ProtocolVersion: PROTOCOL_VERSION,
								ClientInfo: Implementation{
									BaseMetadata: BaseMetadata{Name: "TestClient"},
									Version:      "1.0",
								},
								MetaClientCapabilities: &ClientCapabilities{},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.rawBody
			if body == nil {
				var err error
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("failed to marshal request body: %s", err)
				}
			}

			got, err := resourcesListHandler(ctx, dummyID, primitiveMgr, mustGroup(t, primitiveMgr), body, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want string containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil {
					t.Errorf("expected valid response, got nil")
				} else {
					resp := got.(jsonrpc.JSONRPCResponse).Result.(ListResourcesResult)
					if len(resp.Resources) != 2 {
						t.Errorf("expected 2 resources, got %d", len(resp.Resources))
					} else {
						// res2 should have LastModified set
						for _, r := range resp.Resources {
							if r.Name == "res2" {
								if r.Annotations == nil || r.Annotations.LastModified != "2024-01-01T00:00:00Z" {
									t.Errorf("expected LastModified=2024-01-01T00:00:00Z, got %+v", r.Annotations)
								}
							}
						}
					}
				}
			}
		})
	}
}

func TestResourceTemplatesListHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = util.WithToolboxVersionKey(ctx, "v0.0.0")
	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	mockTemplates := []testutils.MockResourceTemplate{
		testutils.NewMockResourceTemplate("tmpl1", "file:///{tmpl}", "", "", "", nil),
		testutils.NewMockResourceTemplate("rt2", "file:///rt2/{path}", "Title RT", "", "text/plain", &resources.ResourceAnnotations{LastModified: "2024-01-01T00:00:00Z"}),
	}
	toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups := testutils.SetUpPrimitives(t, nil, nil, nil, mockTemplates)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups)

	tests := []struct {
		name        string
		body        ListResourceTemplatesRequest
		rawBody     []byte
		wantErr     bool
		errContains string
	}{
		{
			name:        "invalid json request",
			rawBody:     []byte(`{invalid json}`),
			wantErr:     true,
			errContains: "invalid mcp resource templates list request",
		},
		{
			name: "success",
			body: ListResourceTemplatesRequest{
				PaginatedRequest: PaginatedRequest{
					Request: jsonrpc.Request{Method: "resources/templates/list"},
					Params: PaginatedRequestParams{
						RequestParams: RequestParams{
							Meta: &RequestMetaObject{
								ProtocolVersion: PROTOCOL_VERSION,
								ClientInfo: Implementation{
									BaseMetadata: BaseMetadata{Name: "TestClient"},
									Version:      "1.0",
								},
								MetaClientCapabilities: &ClientCapabilities{},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.rawBody
			if body == nil {
				var err error
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("failed to marshal request body: %s", err)
				}
			}

			got, err := resourceTemplatesListHandler(ctx, dummyID, primitiveMgr, mustGroup(t, primitiveMgr), body, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want string containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil {
					t.Errorf("expected valid response, got nil")
				} else {
					resp := got.(jsonrpc.JSONRPCResponse).Result.(ListResourceTemplatesResult)
					if len(resp.ResourceTemplates) != 2 {
						t.Errorf("expected 2 templates, got %d", len(resp.ResourceTemplates))
					} else {
						// rt2 should have LastModified set
						for _, rt := range resp.ResourceTemplates {
							if rt.Name == "rt2" {
								if rt.Annotations == nil || rt.Annotations.LastModified != "2024-01-01T00:00:00Z" {
									t.Errorf("expected LastModified=2024-01-01T00:00:00Z, got %+v", rt.Annotations)
								}
							}
						}
					}
				}
			}
		})
	}
}

func TestResourcesReadHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = util.WithToolboxVersionKey(ctx, "v0.0.0")
	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	mockResources := []testutils.MockResource{
		testutils.NewMockResource("res1", "file:///res1", "", "", "", nil, nil),
	}
	toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups := testutils.SetUpPrimitives(t, nil, nil, mockResources, nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups)

	tests := []struct {
		name        string
		body        ReadResourceRequest
		rawBody     []byte
		wantErr     bool
		errContains string
	}{
		{
			name:        "invalid json request",
			rawBody:     []byte(`{invalid json}`),
			wantErr:     true,
			errContains: "invalid mcp resources read request",
		},
		{
			name: "success",
			body: ReadResourceRequest{
				Request: jsonrpc.Request{Method: "resources/read"},
				Params: ReadResourceRequestParams{
					RequestParams: RequestParams{
						Meta: &RequestMetaObject{
							ProtocolVersion: PROTOCOL_VERSION,
							ClientInfo: Implementation{
								BaseMetadata: BaseMetadata{Name: "TestClient"},
								Version:      "1.0",
							},
							MetaClientCapabilities: &ClientCapabilities{},
						},
					},
					Uri: "file:///res1",
				},
			},
			wantErr: false,
		},
		{
			name: "not found",
			body: ReadResourceRequest{
				Request: jsonrpc.Request{Method: "resources/read"},
				Params: ReadResourceRequestParams{
					RequestParams: RequestParams{
						Meta: &RequestMetaObject{
							ProtocolVersion: PROTOCOL_VERSION,
							ClientInfo: Implementation{
								BaseMetadata: BaseMetadata{Name: "TestClient"},
								Version:      "1.0",
							},
							MetaClientCapabilities: &ClientCapabilities{},
						},
					},
					Uri: "file:///notfound",
				},
			},
			wantErr:     true,
			errContains: "resource lookup failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.rawBody
			if body == nil {
				var err error
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("failed to marshal request body: %s", err)
				}
			}

			got, err := resourcesReadHandler(ctx, dummyID, primitiveMgr, mustGroup(t, primitiveMgr), body, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %v, want string containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil {
					t.Errorf("expected valid response, got nil")
				}
			}
		})
	}
}

func TestGetResourceOrTemplateByURI(t *testing.T) {
	resourcesMap := map[string]resources.Resource{
		"res1": testutils.NewMockResource("res1", "file:///res1", "", "", "", nil, nil),
		"res2": testutils.NewMockResource("res2", "file:///res2", "", "", "", nil, nil),
	}
	templatesMap := map[string]resources.ResourceTemplate{
		"tmpl1": testutils.NewMockResourceTemplate("tmpl1", "file:///tmpl/{path}", "", "", "", nil),
		"tmpl2": testutils.NewMockResourceTemplate("tmpl2", "file:///other/{path}", "", "", "", nil),
	}

	// Create a group that only contains res1 and tmpl1
	g, err := group.GroupConfig{
		Name:                  "test_group",
		ResourceNames:         []string{"res1"},
		ResourceTemplateNames: []string{"tmpl1"},
	}.Initialize(nil, nil, resourcesMap, templatesMap)
	if err != nil {
		t.Fatalf("failed to init group: %v", err)
	}

	primMgr := primitives.NewPrimitiveManager(nil, nil, nil, nil, nil, resourcesMap, templatesMap, map[string]group.Group{"test_group": g})

	t.Run("Exact Match Resource", func(t *testing.T) {
		res, tmpl, params, err := getResourceOrTemplateByURI("file:///res1", g, primMgr)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res == nil || res.GetName() != "res1" {
			t.Errorf("expected res1, got %v", res)
		}
		if tmpl != nil {
			t.Errorf("expected nil template, got %v", tmpl)
		}
		if params != nil {
			t.Errorf("expected nil params, got %v", params)
		}
	})

	t.Run("Excluded Resource (Not in Group)", func(t *testing.T) {
		_, _, _, err := getResourceOrTemplateByURI("file:///res2", g, primMgr)
		if err == nil {
			t.Fatal("expected error for resource not in group")
		}
	})

	t.Run("Template Match", func(t *testing.T) {
		res, tmpl, params, err := getResourceOrTemplateByURI("file:///tmpl/foo/bar.txt", g, primMgr)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res != nil {
			t.Errorf("expected nil resource, got %v", res)
		}
		if tmpl == nil || tmpl.GetName() != "tmpl1" {
			t.Errorf("expected tmpl1, got %v", tmpl)
		}
		if params["path"] != "foo/bar.txt" {
			t.Errorf("expected path param 'foo/bar.txt', got %v", params["path"])
		}
	})

	t.Run("Excluded Template (Not in Group)", func(t *testing.T) {
		_, _, _, err := getResourceOrTemplateByURI("file:///other/baz.txt", g, primMgr)
		if err == nil {
			t.Fatal("expected error for template not in group")
		}
	})

	t.Run("Not Found", func(t *testing.T) {
		_, _, _, err := getResourceOrTemplateByURI("file:///unknown", g, primMgr)
		if err == nil {
			t.Fatal("expected error for unknown URI")
		}
	})
}
