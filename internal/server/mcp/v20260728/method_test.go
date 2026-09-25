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
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/resources/file"
	"github.com/googleapis/mcp-toolbox/internal/resources/skills"
	"github.com/googleapis/mcp-toolbox/internal/resources/text"
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
	ctx = util.WithToolboxVersionKey(ctx, fakeVersionString)
	Initialize(nil)
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2}
	toolsMap, promptsMap, _, _, groups := testutils.SetUpPrimitives(t, mockTools, nil, nil, nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, nil, nil, groups)

	validMeta := &RequestMetaObject{
		ProtocolVersion: PROTOCOL_VERSION,
		ClientInfo: Implementation{
			BaseMetadata: BaseMetadata{Name: "TestClient"},
			Version:      "1.0",
		},
		MetaClientCapabilities: &ClientCapabilities{
			Extensions: map[string]any{"com.google.cloud/toolbox.v1": map[string]any{}},
		},
	}
	noExtensionMeta := &RequestMetaObject{
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
			name: "client did not declare the toolbox extension",
			body: ListGroupsRequest{
				Request: jsonrpc.Request{Method: GROUPS_LIST},
				Params:  RequestParams{Meta: noExtensionMeta},
			},
			header:      http.Header{"Mcp-Method": []string{GROUPS_LIST}},
			wantErr:     true,
			errContains: `missing required client capability: method "groups/list" requires com.google.cloud/toolbox.v1 extension which is not supported by the client`,
		},
		{
			name: "success excludes default group and sorts",
			body: ListGroupsRequest{
				Request: jsonrpc.Request{Method: GROUPS_LIST},
				Params:  RequestParams{Meta: validMeta},
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
			if result.ResultType != resultTypeComplete {
				t.Errorf("result.ResultType = %q, want %q", result.ResultType, resultTypeComplete)
			}
			if result.Meta == nil {
				t.Error("result.Meta = nil, want server info metadata")
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
	ctx = util.WithToolboxVersionKey(ctx, fakeVersionString)
	Initialize(nil)
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2}
	toolsMap, promptsMap, _, _, groups := testutils.SetUpPrimitives(t, mockTools, nil, nil, nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, nil, nil, groups)

	validMeta := &RequestMetaObject{
		ProtocolVersion: PROTOCOL_VERSION,
		ClientInfo: Implementation{
			BaseMetadata: BaseMetadata{Name: "TestClient"},
			Version:      "1.0",
		},
		MetaClientCapabilities: &ClientCapabilities{
			Extensions: map[string]any{"com.google.cloud/toolbox.v1": map[string]any{}},
		},
	}
	noExtensionMeta := &RequestMetaObject{
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
		wantTools   []string
	}{
		{
			name:        "invalid json body",
			rawBody:     []byte(`{invalid json}`),
			wantErr:     true,
			errContains: "invalid mcp groups/get request",
		},
		{
			name: "client did not declare the toolbox extension",
			body: GetGroupRequest{
				Request: jsonrpc.Request{Method: GROUPS_GET},
				Params: GetGroupRequestParams{
					RequestParams: RequestParams{Meta: noExtensionMeta},
					Name:          "tool1_only",
				},
			},
			header:      http.Header{"Mcp-Method": []string{GROUPS_GET}, "Mcp-Name": []string{"tool1_only"}},
			wantErr:     true,
			errContains: `missing required client capability: method "groups/get" requires com.google.cloud/toolbox.v1 extension which is not supported by the client`,
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
			header:    http.Header{"Mcp-Method": []string{GROUPS_GET}, "Mcp-Name": []string{"tool1_only"}},
			wantErr:   false,
			wantName:  "tool1_only",
			wantTools: []string{"no_params"},
		},
		{
			// An omitted name resolves to the default group, matching
			// GET /api/toolset. groups/list hides the default group, so this is
			// the only way to reach it.
			name: "omitted name returns the default group",
			body: GetGroupRequest{
				Request: jsonrpc.Request{Method: GROUPS_GET},
				Params: GetGroupRequestParams{
					RequestParams: RequestParams{Meta: validMeta},
				},
			},
			header:    http.Header{"Mcp-Method": []string{GROUPS_GET}},
			wantErr:   false,
			wantName:  "",
			wantTools: []string{"no_params", "some_params"},
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
			gotTools := make([]string, 0, len(result.Tools))
			for _, tool := range result.Tools {
				gotTools = append(gotTools, tool.Name)
			}
			slices.Sort(gotTools)
			if !slices.Equal(gotTools, tt.wantTools) {
				t.Errorf("result tools = %v, want %v", gotTools, tt.wantTools)
			}
			if result.ResultType != resultTypeComplete {
				t.Errorf("result.ResultType = %q, want %q", result.ResultType, resultTypeComplete)
			}
			if result.Meta == nil {
				t.Error("result.Meta = nil, want server info metadata")
			}
			if result.TtlMs != group.DefaultTTLMs {
				t.Errorf("result.TtlMs = %d, want %d", result.TtlMs, group.DefaultTTLMs)
			}
			if string(result.CacheScope) != group.DefaultCacheScope {
				t.Errorf("result.CacheScope = %q, want %q", result.CacheScope, group.DefaultCacheScope)
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
		testutils.NewMockUIResource("uiRes", "ui://test", "UI Title", "", "text/html", nil, nil, nil, nil, "", nil),
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
						// uiRes should be omitted from resources/list
						for _, r := range resp.Resources {
							if r.Name == "uiRes" {
								t.Errorf("expected uiRes to be omitted from resources/list")
							}
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
		testutils.NewMockUIResourceTemplate("uiTmpl", "ui://test/{path}", "UI Template Title", "", "text/html", nil, nil, nil, "", nil),
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
						// uiTmpl should be omitted from resources/templates/list
						for _, rt := range resp.ResourceTemplates {
							if rt.Name == "uiTmpl" {
								t.Errorf("expected uiTmpl to be omitted from resources/templates/list")
							}
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

	prefersBorder := true
	mockResources := []testutils.MockResource{
		testutils.NewMockResource("res1", "file:///res1", "", "", "", nil, nil),
		testutils.NewMockUIResource("ui_res", "ui://test/dashboard", "Dashboard", "UI Dashboard", "text/html;profile=mcp-app", nil, nil, &resources.CSPConfig{
			ConnectDomains: []string{"https://api.example.com"},
		}, nil, "custom-domain", &prefersBorder),
	}
	toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups := testutils.SetUpPrimitives(t, nil, nil, mockResources, nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups)

	tests := []struct {
		name        string
		header      http.Header
		body        ReadResourceRequest
		rawBody     []byte
		wantErr     bool
		errContains string
		verifyFunc  func(t *testing.T, resp any)
	}{
		{
			name:        "invalid json request",
			rawBody:     []byte(`{invalid json}`),
			wantErr:     true,
			errContains: "invalid mcp resources read request",
		},
		{
			name: "success without headers (stdio transport)",
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
			name: "success with valid Mcp-Name and Mcp-Method headers",
			header: http.Header{
				"Mcp-Method": []string{RESOURCES_READ},
				"Mcp-Name":   []string{"file:///res1"},
			},
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
			name: "success UI resource includes _meta.ui",
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
					Uri: "ui://test/dashboard",
				},
			},
			wantErr: false,
			verifyFunc: func(t *testing.T, resp any) {
				jsonResp, ok := resp.(jsonrpc.JSONRPCResponse)
				if !ok {
					t.Fatalf("expected JSONRPCResponse, got %T", resp)
				}
				readRes, ok := jsonResp.Result.(*ReadResourceResult)
				if !ok {
					t.Fatalf("expected *ReadResourceResult, got %T", jsonResp.Result)
				}
				if len(readRes.Contents) != 1 {
					t.Fatalf("expected 1 content item, got %d", len(readRes.Contents))
				}
				metaUI, ok := readRes.Contents[0].Metadata["ui"]
				if !ok || metaUI == nil {
					t.Fatalf("expected Contents[0].Metadata to have 'ui', got %v", readRes.Contents[0].Metadata)
				}
			},
		},
		{
			name: "mismatched Mcp-Name header",
			header: http.Header{
				"Mcp-Method": []string{RESOURCES_READ},
				"Mcp-Name":   []string{"file:///wrong"},
			},
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
			wantErr:     true,
			errContains: "Mcp-Name header value 'file:///wrong' does not match body value 'file:///res1'",
		},
		{
			name: "mismatched Mcp-Method header",
			header: http.Header{
				"Mcp-Method": []string{"wrong-method"},
				"Mcp-Name":   []string{"file:///res1"},
			},
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
			wantErr:     true,
			errContains: "Mcp-Method header value 'wrong-method' does not match body value 'resources/read'",
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

			got, err := resourcesReadHandler(ctx, dummyID, primitiveMgr, mustGroup(t, primitiveMgr), body, tt.header)
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
				if tt.verifyFunc != nil {
					tt.verifyFunc(t, got)
				}
			}
		})
	}
}

func TestGetResourceOrTemplateByURI(t *testing.T) {
	resourcesMap := map[string]resources.Resource{
		"res1":  testutils.NewMockResource("res1", "file:///res1", "", "", "", nil, nil),
		"res2":  testutils.NewMockResource("res2", "file:///res2", "", "", "", nil, nil),
		"uiRes": testutils.NewMockUIResource("uiRes", "ui://test-ui", "", "", "", nil, nil, nil, nil, "", nil),
	}
	templatesMap := map[string]resources.ResourceTemplate{
		"tmpl1":  testutils.NewMockResourceTemplate("tmpl1", "file:///tmpl/{path}", "", "", "", nil),
		"tmpl2":  testutils.NewMockResourceTemplate("tmpl2", "file:///other/{path}", "", "", "", nil),
		"uiTmpl": testutils.NewMockUIResourceTemplate("uiTmpl", "ui://tmpl/{path}", "", "", "", nil, nil, nil, "", nil),
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

	t.Run("UI Resource (Not in Group, Globally Accessible)", func(t *testing.T) {
		res, tmpl, params, err := getResourceOrTemplateByURI("ui://test-ui", g, primMgr)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res == nil || res.GetName() != "uiRes" {
			t.Errorf("expected uiRes, got %v", res)
		}
		if tmpl != nil {
			t.Errorf("expected nil template, got %v", tmpl)
		}
		if params != nil {
			t.Errorf("expected nil params, got %v", params)
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

	t.Run("UI Template (Not in Group, Globally Accessible)", func(t *testing.T) {
		res, tmpl, params, err := getResourceOrTemplateByURI("ui://tmpl/dashboard.html", g, primMgr)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res != nil {
			t.Errorf("expected nil resource, got %v", res)
		}
		if tmpl == nil || tmpl.GetName() != "uiTmpl" {
			t.Errorf("expected uiTmpl, got %v", tmpl)
		}
		if params["path"] != "dashboard.html" {
			t.Errorf("expected path param 'dashboard.html', got %v", params["path"])
		}
	})

	t.Run("Not Found", func(t *testing.T) {
		_, _, _, err := getResourceOrTemplateByURI("file:///unknown", g, primMgr)
		if err == nil {
			t.Fatal("expected error for unknown URI")
		}
	})
}

// skillTextResource builds a resource whose Read returns real content, which a
// SKILL.md needs. testutils.MockResource returns a fixed string, so it cannot
// carry frontmatter.
func skillTextResource(t *testing.T, ctx context.Context, name, uri, content string) resources.Resource {
	t.Helper()
	cfg := &text.Config{
		ResourceConfigBase: resources.ResourceConfigBase{
			ConfigBase: resources.ConfigBase{Name: name, Type: "text", MimeType: "text/markdown"},
			URI:        uri,
		},
		Text: content,
	}
	res, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unable to initialize %q: %s", uri, err)
	}
	return res
}

// skillsTestPrimitives builds a manager holding one skill with one supporting
// file, plus a resource outside any skill.
func skillsTestPrimitives(t *testing.T, ctx context.Context) *primitives.PrimitiveManager {
	t.Helper()
	resourcesMap := map[string]resources.Resource{
		"guide": skillTextResource(t, ctx, "guide", "skill://analytics-guide/SKILL.md",
			"---\nname: analytics-guide\ndescription: Query the warehouse\n---\n\n# analytics-guide\n"),
		"queries": skillTextResource(t, ctx, "queries", "skill://analytics-guide/references/queries.md",
			"# Common queries\n"),
		"plain": skillTextResource(t, ctx, "plain", "text:///not-a-skill", "unrelated"),
	}
	return primitives.NewPrimitiveManager(nil, nil, nil, nil, nil, resourcesMap, nil, nil)
}

func skillsTestContext(t *testing.T) context.Context {
	t.Helper()
	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}
	ctx := util.WithLogger(context.Background(), testLogger)
	return util.WithToolboxVersionKey(ctx, fakeVersionString)
}

func skillsValidMeta() *RequestMetaObject {
	return &RequestMetaObject{
		ProtocolVersion: PROTOCOL_VERSION,
		ClientInfo: Implementation{
			BaseMetadata: BaseMetadata{Name: "TestClient"},
			Version:      "1.0",
		},
		MetaClientCapabilities: &ClientCapabilities{},
	}
}

func TestSkillsListHandler(t *testing.T) {
	ctx := skillsTestContext(t)
	Initialize(nil)
	primitiveMgr := skillsTestPrimitives(t, ctx)

	tests := []struct {
		name        string
		rawBody     []byte
		body        ListSkillsRequest
		header      http.Header
		wantErr     bool
		errContains string
		wantURIs    []string
	}{
		{
			name:        "invalid json body",
			rawBody:     []byte(`{invalid json}`),
			wantErr:     true,
			errContains: "invalid mcp skills/list request",
		},
		{
			name: "missing metadata",
			body: ListSkillsRequest{
				Request: jsonrpc.Request{Method: SKILLS_LIST},
				Params:  RequestParams{},
			},
			header:      http.Header{"Mcp-Method": []string{SKILLS_LIST}},
			wantErr:     true,
			errContains: "_meta error: missing required fields in request metadata",
		},
		{
			name: "header method mismatch",
			body: ListSkillsRequest{
				Request: jsonrpc.Request{Method: SKILLS_LIST},
				Params:  RequestParams{Meta: skillsValidMeta()},
			},
			header:      http.Header{"Mcp-Method": []string{"skills/get"}},
			wantErr:     true,
			errContains: "does not match body value",
		},
		{
			name: "lists every skill, ignoring non-skill resources",
			body: ListSkillsRequest{
				Request: jsonrpc.Request{Method: SKILLS_LIST},
				Params:  RequestParams{Meta: skillsValidMeta()},
			},
			header:   http.Header{"Mcp-Method": []string{SKILLS_LIST}},
			wantURIs: []string{"skill://analytics-guide/SKILL.md"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.rawBody
			if body == nil {
				var err error
				body, err = json.Marshal(tc.body)
				if err != nil {
					t.Fatalf("unable to marshal body: %s", err)
				}
			}
			res, err := skillsListHandler(ctx, "id", primitiveMgr, body, tc.header)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("skillsListHandler() = nil error, want one containing %q", tc.errContains)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("error = %q, want it to contain %q", err, tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("skillsListHandler() = %v, want nil", err)
			}
			response, ok := res.(jsonrpc.JSONRPCResponse)
			if !ok {
				t.Fatalf("response is %T, want jsonrpc.JSONRPCResponse", res)
			}
			result, ok := response.Result.(ListSkillsResult)
			if !ok {
				t.Fatalf("result is %T, want ListSkillsResult", response.Result)
			}
			if result.ResultType != resultTypeComplete {
				t.Errorf("resultType = %q, want %q", result.ResultType, resultTypeComplete)
			}
			if result.Meta == nil {
				t.Error("result _meta is nil, want serverInfo")
			}
			if result.TtlMs != skillsTTLMs || result.CacheScope != skillsCacheScope {
				t.Errorf("ttlMs/cacheScope = %d/%q, want %d/%q",
					result.TtlMs, result.CacheScope, skillsTTLMs, skillsCacheScope)
			}
			var gotURIs []string
			for _, e := range result.Skills {
				gotURIs = append(gotURIs, e.URI)
			}
			if !slices.Equal(gotURIs, tc.wantURIs) {
				t.Errorf("skills = %v, want %v", gotURIs, tc.wantURIs)
			}
			// The URI check above reports an unexpected empty result.
			if len(result.Skills) == 0 {
				return
			}
			// The manifest must carry a fresh digest for every member.
			for _, ref := range result.Skills[0].Resources.Refs {
				if !strings.HasPrefix(ref.Digest, "sha256:") {
					t.Errorf("ref %q digest = %q, want a sha256: prefix", ref.URI, ref.Digest)
				}
			}
			if got := len(result.Skills[0].Resources.Refs); got != 2 {
				t.Errorf("got %d refs, want 2 (SKILL.md and its supporting file)", got)
			}
		})
	}
}

func TestSkillsGetHandler(t *testing.T) {
	ctx := skillsTestContext(t)
	Initialize(nil)
	primitiveMgr := skillsTestPrimitives(t, ctx)

	tests := []struct {
		name        string
		rawBody     []byte
		body        GetSkillRequest
		header      http.Header
		wantErr     bool
		errContains string
	}{
		{
			name:        "invalid json body",
			rawBody:     []byte(`{invalid json}`),
			wantErr:     true,
			errContains: "invalid mcp skills/get request",
		},
		{
			name: "unknown uri",
			body: GetSkillRequest{
				Request: jsonrpc.Request{Method: SKILLS_GET},
				Params: GetSkillRequestParams{
					RequestParams: RequestParams{Meta: skillsValidMeta()},
					URI:           "skill://nope/SKILL.md",
				},
			},
			header: http.Header{
				"Mcp-Method": []string{SKILLS_GET},
				"Mcp-Name":   []string{"skill://nope/SKILL.md"},
			},
			wantErr:     true,
			errContains: "unknown skill: skill://nope/SKILL.md",
		},
		{
			name: "a supporting file is not a skill",
			body: GetSkillRequest{
				Request: jsonrpc.Request{Method: SKILLS_GET},
				Params: GetSkillRequestParams{
					RequestParams: RequestParams{Meta: skillsValidMeta()},
					URI:           "skill://analytics-guide/references/queries.md",
				},
			},
			header: http.Header{
				"Mcp-Method": []string{SKILLS_GET},
				"Mcp-Name":   []string{"skill://analytics-guide/references/queries.md"},
			},
			wantErr:     true,
			errContains: "unknown skill",
		},
		{
			name: "returns the skill",
			body: GetSkillRequest{
				Request: jsonrpc.Request{Method: SKILLS_GET},
				Params: GetSkillRequestParams{
					RequestParams: RequestParams{Meta: skillsValidMeta()},
					URI:           "skill://analytics-guide/SKILL.md",
				},
			},
			header: http.Header{
				"Mcp-Method": []string{SKILLS_GET},
				"Mcp-Name":   []string{"skill://analytics-guide/SKILL.md"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.rawBody
			if body == nil {
				var err error
				body, err = json.Marshal(tc.body)
				if err != nil {
					t.Fatalf("unable to marshal body: %s", err)
				}
			}
			res, err := skillsGetHandler(ctx, "id", primitiveMgr, body, tc.header)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("skillsGetHandler() = nil error, want one containing %q", tc.errContains)
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("error = %q, want it to contain %q", err, tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("skillsGetHandler() = %v, want nil", err)
			}
			response, ok := res.(jsonrpc.JSONRPCResponse)
			if !ok {
				t.Fatalf("response is %T, want jsonrpc.JSONRPCResponse", res)
			}
			result, ok := response.Result.(GetSkillResult)
			if !ok {
				t.Fatalf("result is %T, want GetSkillResult", response.Result)
			}
			if result.Skill.URI != "skill://analytics-guide/SKILL.md" {
				t.Errorf("skill uri = %q, want the requested one", result.Skill.URI)
			}
			if result.ResultType != resultTypeComplete {
				t.Errorf("resultType = %q, want %q", result.ResultType, resultTypeComplete)
			}
			if result.Meta == nil {
				t.Error("result _meta is nil, want serverInfo")
			}
			if result.TtlMs != skillsTTLMs || result.CacheScope != skillsCacheScope {
				t.Errorf("ttlMs/cacheScope = %d/%q, want %d/%q",
					result.TtlMs, result.CacheScope, skillsTTLMs, skillsCacheScope)
			}
		})
	}
}

// dynamicSkillTextResource builds a SKILL.md resource whose skill publishes the
// "dynamic" marker in place of a file list.
func dynamicSkillTextResource(t *testing.T, ctx context.Context, name, uri, content string) resources.Resource {
	t.Helper()
	cfg := &text.Config{
		ResourceConfigBase: resources.ResourceConfigBase{
			ConfigBase: resources.ConfigBase{Name: name, Type: "text", MimeType: "text/markdown"},
			URI:        uri,
			Dynamic:    true,
		},
		Text: content,
	}
	res, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unable to initialize %q: %s", uri, err)
	}
	return res
}

// TestSkillsMethodsDynamicSkill pins the wire shape of a dynamic skill. The
// marker is a JSON string where a static skill carries an array. A host
// distinguishes the two by that type alone.
func TestSkillsMethodsDynamicSkill(t *testing.T) {
	ctx := skillsTestContext(t)
	Initialize(nil)

	const (
		staticURI  = "skill://analytics-guide/SKILL.md"
		dynamicURI = "skill://live-report/SKILL.md"
	)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, nil, nil,
		map[string]resources.Resource{
			"guide": skillTextResource(t, ctx, "guide", staticURI,
				"---\nname: analytics-guide\ndescription: Query the warehouse\n---\n\n# analytics-guide\n"),
			"queries": skillTextResource(t, ctx, "queries", "skill://analytics-guide/references/queries.md",
				"# Common queries\n"),
			"report": dynamicSkillTextResource(t, ctx, "report", dynamicURI,
				"---\nname: live-report\ndescription: Summarize the current run\n---\n\n# live-report\n"),
			"rows": skillTextResource(t, ctx, "rows", "skill://live-report/rows.csv", "a,b\n1,2\n"),
		}, nil, nil)

	// resourcesOf marshals one entry the way the server does and returns the
	// resources field. The assertion then sees the JSON type a client sees.
	resourcesOf := func(t *testing.T, e skills.Entry) any {
		t.Helper()
		raw, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("unable to marshal entry %q: %s", e.URI, err)
		}
		var decoded struct {
			Resources any `json:"resources"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("unable to unmarshal entry %q: %s", e.URI, err)
		}
		return decoded.Resources
	}

	t.Run("skills/list", func(t *testing.T) {
		body, err := json.Marshal(ListSkillsRequest{
			Request: jsonrpc.Request{Method: SKILLS_LIST},
			Params:  RequestParams{Meta: skillsValidMeta()},
		})
		if err != nil {
			t.Fatalf("unable to marshal body: %s", err)
		}
		res, err := skillsListHandler(ctx, "id", primitiveMgr, body,
			http.Header{"Mcp-Method": []string{SKILLS_LIST}})
		if err != nil {
			t.Fatalf("skillsListHandler() = %v, want nil", err)
		}
		result := res.(jsonrpc.JSONRPCResponse).Result.(ListSkillsResult)
		if len(result.Skills) != 2 {
			t.Fatalf("got %d skills, want 2", len(result.Skills))
		}

		byURI := map[string]skills.Entry{}
		for _, e := range result.Skills {
			byURI[e.URI] = e
		}

		if got := resourcesOf(t, byURI[dynamicURI]); got != "dynamic" {
			t.Errorf("dynamic skill resources = %#v, want the string \"dynamic\"", got)
		}
		// Discover never hashes the dynamic skill's supporting file, so the
		// static skill beside it must still publish its own array.
		got, ok := resourcesOf(t, byURI[staticURI]).([]any)
		if !ok {
			t.Fatalf("static skill resources = %#v, want an array", resourcesOf(t, byURI[staticURI]))
		}
		if len(got) != 2 {
			t.Errorf("static skill has %d refs, want 2", len(got))
		}
	})

	t.Run("skills/get", func(t *testing.T) {
		body, err := json.Marshal(GetSkillRequest{
			Request: jsonrpc.Request{Method: SKILLS_GET},
			Params: GetSkillRequestParams{
				RequestParams: RequestParams{Meta: skillsValidMeta()},
				URI:           dynamicURI,
			},
		})
		if err != nil {
			t.Fatalf("unable to marshal body: %s", err)
		}
		res, err := skillsGetHandler(ctx, "id", primitiveMgr, body, http.Header{
			"Mcp-Method": []string{SKILLS_GET},
			"Mcp-Name":   []string{dynamicURI},
		})
		if err != nil {
			t.Fatalf("skillsGetHandler() = %v, want nil", err)
		}
		result := res.(jsonrpc.JSONRPCResponse).Result.(GetSkillResult)
		if result.Skill.URI != dynamicURI {
			t.Fatalf("skill uri = %q, want %q", result.Skill.URI, dynamicURI)
		}
		if got := resourcesOf(t, result.Skill); got != "dynamic" {
			t.Errorf("resources = %#v, want the string \"dynamic\"", got)
		}
		if name := result.Skill.Frontmatter["name"]; name != "live-report" {
			t.Errorf("frontmatter name = %v, want live-report", name)
		}
	})
}

// TestSkillsHandlersHideServerPaths checks that an unreadable skill file tells
// the client nothing about the server's filesystem. The detail belongs in the
// logs, which the caller of these handlers cannot read.
func TestSkillsHandlersHideServerPaths(t *testing.T) {
	ctx := skillsTestContext(t)
	Initialize(nil)

	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: vanishing\ndescription: Goes away\n---\n"), 0600); err != nil {
		t.Fatalf("unable to write the skill file: %s", err)
	}
	cfg := &file.Config{
		ResourceConfigBase: resources.ResourceConfigBase{
			ConfigBase: resources.ConfigBase{Name: "vanishing", Type: "file", MimeType: "text/markdown"},
			URI:        "skill://vanishing/SKILL.md",
		},
		Path: filepath.ToSlash(path),
	}
	res, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unable to initialize the skill resource: %s", err)
	}
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, nil, nil,
		map[string]resources.Resource{"vanishing": res}, nil, nil)

	// The resource sizes its file at startup, so remove it only now.
	if err := os.Remove(path); err != nil {
		t.Fatalf("unable to remove the skill file: %s", err)
	}

	listBody, err := json.Marshal(ListSkillsRequest{
		Request: jsonrpc.Request{Method: SKILLS_LIST},
		Params:  RequestParams{Meta: skillsValidMeta()},
	})
	if err != nil {
		t.Fatalf("unable to marshal the skills/list body: %s", err)
	}
	getBody, err := json.Marshal(GetSkillRequest{
		Request: jsonrpc.Request{Method: SKILLS_GET},
		Params: GetSkillRequestParams{
			RequestParams: RequestParams{Meta: skillsValidMeta()},
			URI:           "skill://vanishing/SKILL.md",
		},
	})
	if err != nil {
		t.Fatalf("unable to marshal the skills/get body: %s", err)
	}

	tests := []struct {
		name string
		call func() (any, error)
	}{
		{
			name: SKILLS_LIST,
			call: func() (any, error) {
				return skillsListHandler(ctx, "id", primitiveMgr, listBody,
					http.Header{"Mcp-Method": []string{SKILLS_LIST}})
			},
		},
		{
			name: SKILLS_GET,
			call: func() (any, error) {
				return skillsGetHandler(ctx, "id", primitiveMgr, getBody, http.Header{
					"Mcp-Method": []string{SKILLS_GET},
					"Mcp-Name":   []string{"skill://vanishing/SKILL.md"},
				})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.call()
			if err == nil {
				t.Fatal("handler returned a nil error, want the unreadable file to fail the request")
			}
			// The error the server keeps still names the file, for the logs.
			if !strings.Contains(err.Error(), dir) {
				t.Errorf("internal error = %q, want it to name %q", err, dir)
			}
			rpcErr, ok := res.(jsonrpc.JSONRPCError)
			if !ok {
				t.Fatalf("response is %T, want jsonrpc.JSONRPCError", res)
			}
			want := `unable to read "skill://vanishing/SKILL.md"`
			if got := rpcErr.Error.Message; got != want {
				t.Errorf("client message = %q, want %q", got, want)
			}
			if rpcErr.Error.Code != jsonrpc.INTERNAL_ERROR {
				t.Errorf("client error code = %d, want %d", rpcErr.Error.Code, jsonrpc.INTERNAL_ERROR)
			}
		})
	}
}

// TestSkillsListSendsConfigErrors is the other half of the test above: a
// misconfigured skill names no server path, so its message reaches the client
// whole and the operator can fix the config without reading the logs.
func TestSkillsListSendsConfigErrors(t *testing.T) {
	ctx := skillsTestContext(t)
	Initialize(nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, nil, nil,
		map[string]resources.Resource{
			"broken": skillTextResource(t, ctx, "broken", "skill://broken/SKILL.md", "no frontmatter here\n"),
		}, nil, nil)

	body, err := json.Marshal(ListSkillsRequest{
		Request: jsonrpc.Request{Method: SKILLS_LIST},
		Params:  RequestParams{Meta: skillsValidMeta()},
	})
	if err != nil {
		t.Fatalf("unable to marshal body: %s", err)
	}
	res, err := skillsListHandler(ctx, "id", primitiveMgr, body,
		http.Header{"Mcp-Method": []string{SKILLS_LIST}})
	if err == nil {
		t.Fatal("skillsListHandler() = nil error, want the bad frontmatter to fail the request")
	}
	rpcErr, ok := res.(jsonrpc.JSONRPCError)
	if !ok {
		t.Fatalf("response is %T, want jsonrpc.JSONRPCError", res)
	}
	want := `skill "skill://broken/SKILL.md": SKILL.md must open with YAML frontmatter delimited by ---`
	if got := rpcErr.Error.Message; got != want {
		t.Errorf("client message = %q, want %q", got, want)
	}
}

// TestSkillsListEmptyCatalogue pins the wire shape when the config declares no
// skill. A nil slice marshals to null, not to a list, so
// GenerateListSkillsResult substitutes an empty slice.
func TestSkillsListEmptyCatalogue(t *testing.T) {
	ctx := skillsTestContext(t)
	Initialize(nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, nil, nil,
		map[string]resources.Resource{
			"plain": skillTextResource(t, ctx, "plain", "text:///not-a-skill", "unrelated"),
		}, nil, nil)

	body, err := json.Marshal(ListSkillsRequest{
		Request: jsonrpc.Request{Method: SKILLS_LIST},
		Params:  RequestParams{Meta: skillsValidMeta()},
	})
	if err != nil {
		t.Fatalf("unable to marshal body: %s", err)
	}
	res, err := skillsListHandler(ctx, "id", primitiveMgr, body, http.Header{"Mcp-Method": []string{SKILLS_LIST}})
	if err != nil {
		t.Fatalf("skillsListHandler() = %v, want nil", err)
	}
	result, ok := res.(jsonrpc.JSONRPCResponse).Result.(ListSkillsResult)
	if !ok {
		t.Fatalf("result is %T, want ListSkillsResult", res.(jsonrpc.JSONRPCResponse).Result)
	}
	if len(result.Skills) != 0 {
		t.Errorf("skills = %v, want none", result.Skills)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("unable to marshal result: %s", err)
	}
	if !strings.Contains(string(encoded), `"skills":[]`) {
		t.Errorf("result marshalled to %s, want it to carry \"skills\":[]", encoded)
	}
}

// TestSkillsMethodsDisabled pins --disable-ext: a switched-off extension has no
// methods, so the dispatcher must not reach a handler.
func TestSkillsMethodsDisabled(t *testing.T) {
	ctx := skillsTestContext(t)
	Initialize([]string{SkillsExtensionURI})
	t.Cleanup(func() { Initialize(nil) })
	primitiveMgr := skillsTestPrimitives(t, ctx)

	for _, method := range []string{SKILLS_LIST, SKILLS_GET} {
		t.Run(method, func(t *testing.T) {
			_, err := ProcessMethod(ctx, "id", method, group.Group{}, primitiveMgr, []byte(`{}`), nil)
			if err == nil {
				t.Fatalf("ProcessMethod(%q) = nil error, want method not found", method)
			}
			if want := fmt.Sprintf("invalid method %s", method); err.Error() != want {
				t.Errorf("error = %q, want %q", err, want)
			}
		})
	}
}
