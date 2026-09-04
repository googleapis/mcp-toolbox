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

package v20251125

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
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

func TestInitializeHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctxVersion := util.WithToolboxVersionKey(ctx, fakeVersionString)

	tests := []struct {
		name        string
		body        InitializeRequest
		rawBody     []byte
		context     context.Context
		wantErr     bool
		errContains string
	}{
		{
			name: "missing version in context",
			body: InitializeRequest{
				Request: jsonrpc.Request{
					Method: "initialize",
				},
				Params: InitializeParams{
					ProtocolVersion: PROTOCOL_VERSION,
				},
			},
			context:     ctx,
			wantErr:     true,
			errContains: "unable to retrieve toolbox version", // Adjust to match your util.ToolboxVersionFromContext error
		},
		{
			name:        "invalid json body",
			rawBody:     []byte(`{invalid json}`),
			context:     ctxVersion,
			wantErr:     true,
			errContains: "invalid mcp initialize request",
		},
		{
			name: "success",
			body: InitializeRequest{
				Request: jsonrpc.Request{
					Method: "initialize",
				},
				Params: InitializeParams{
					ProtocolVersion: PROTOCOL_VERSION,
				},
			},
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
					t.Fatalf("unexpected error during marshaling: %v", err)
				}
			}

			got, err := initializeHandler(tt.context, dummyID, body)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %v, want error containing %q", err, tt.errContains)
				}
				// Optional: You can also assert that 'got' is a jsonrpc.Error response here if you'd like
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got == nil {
					t.Fatalf("expected valid response, got nil")
				}

				// Verify the response structure for success
				res, ok := got.(jsonrpc.JSONRPCResponse)
				if !ok {
					t.Fatalf("expected response of type jsonrpc.JSONRPCResponse, got %T", got)
				}

				if res.Id != dummyID {
					t.Errorf("expected ID %v, got %v", dummyID, res.Id)
				}

				initResult, ok := res.Result.(InitializeResult)
				if !ok {
					t.Fatalf("expected result of type InitializeResult, got %T", res.Result)
				}
				if initResult.ServerInfo.Version != fakeVersionString {
					t.Errorf("expected version %q, got %q", fakeVersionString, initResult.ServerInfo.Version)
				}
			}
		})
	}
}

func TestPingHandler(t *testing.T) {
	got, err := pingHandler(dummyID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatalf("expected valid response, got nil")
	}

	res, ok := got.(jsonrpc.JSONRPCResponse)
	if !ok {
		t.Fatalf("expected response of type jsonrpc.JSONRPCResponse, got %T", got)
	}

	if res.Jsonrpc != jsonrpc.JSONRPC_VERSION {
		t.Errorf("expected JSONRPC version %q, got %q", jsonrpc.JSONRPC_VERSION, res.Jsonrpc)
	}

	if res.Id != dummyID {
		t.Errorf("expected ID %v, got %v", dummyID, res.Id)
	}

	// Verify Result is an empty struct
	if _, ok := res.Result.(struct{}); !ok {
		t.Errorf("expected result to be an empty struct, got %T", res.Result)
	}
}

func TestToolsListHandler(t *testing.T) {
	// Initialize primitives
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2}
	toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups := testutils.SetUpPrimitives(t, mockTools, nil, nil, nil)
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups)

	tests := []struct {
		name        string
		body        ListToolsRequest
		rawBody     []byte
		g           group.Group
		wantErr     bool
		errContains string
	}{
		{
			name:        "invalid json body",
			rawBody:     []byte(`{invalid json}`),
			g:           mustGroup(t, primitiveMgr),
			wantErr:     true,
			errContains: "invalid mcp tools list request",
		},
		{
			name: "success - stdio (nil header)",
			body: ListToolsRequest{
				PaginatedRequest: PaginatedRequest{
					Request: jsonrpc.Request{
						Method: "tools/list",
					},
				},
			},
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
				},
			},
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
			got, err := toolsListHandler(context.Background(), dummyID, primitiveMgr, tt.g, body)

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
		context         context.Context
		wantErr         bool
		errContains     string
		wantIsError     bool
		wantContentText string
	}{
		{
			name:        "invalid json body",
			rawBody:     []byte(`{invalid json}`),
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
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
				}{
					Name: "no_params",
				},
			},
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
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
				}{
					Name: "unknown_tool",
				},
			},
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
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
				}{
					Name: "require_client_auth_tool",
				},
			},
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
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
				}{
					Name: "no_params",
				},
			},
			context: ctxLogger,
			wantErr: false,
		},
		{
			name: "successful invocation - URL bound parameters auto-populated",
			body: CallToolRequest{
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
				}{
					Name: "some_params",
					Arguments: map[string]any{
						"param2": 20,
					},
				},
			},
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
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
				}{
					Name:      "some_params",
					Arguments: map[string]any{},
				},
			},
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
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
				}{
					Name: "some_params",
					Arguments: map[string]any{
						"param1": 10,
						"param2": 20,
					},
				},
			},
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
			got, err := toolsCallHandler(tt.context, dummyID, mustGroup(t, primitiveMgr), primitiveMgr, body, nil)

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
		wantErr     bool
		errContains string
	}{
		{
			name:        "invalid json request",
			rawBody:     []byte(`{invalid json}`),
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
				},
			},
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
			got, err := promptsListHandler(ctx, dummyID, primitiveMgr, mustGroup(t, primitiveMgr), body)

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
		wantErr     bool
		errContains string
	}{
		{
			name:        "invalid json request",
			rawBody:     []byte(`{invalid json}`),
			wantErr:     true,
			errContains: "invalid mcp prompts/get request",
		},
		{
			name: "prompt does not exist",
			body: GetPromptRequest{
				Request: jsonrpc.Request{
					Method: "prompts/get",
				},
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
				}{
					Name: "missing_prompt",
				},
			},
			wantErr:     true,
			errContains: "does not exist",
		},
		{
			name: "success with args",
			body: GetPromptRequest{
				Request: jsonrpc.Request{
					Method: "prompts/get",
				},
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
				}{
					Name: "prompt2",
					Arguments: map[string]any{
						"arg1": "value1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "success without args",
			body: GetPromptRequest{
				Request: jsonrpc.Request{
					Method: "prompts/get",
				},
				Params: struct {
					Name      string         `json:"name"`
					Arguments map[string]any `json:"arguments,omitempty"`
				}{
					Name: "prompt1",
				},
			},
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
			got, err := promptsGetHandler(ctx, dummyID, mustGroup(t, primitiveMgr), primitiveMgr, body)

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

func TestResourcesListHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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

			got, err := resourcesListHandler(ctx, dummyID, primitiveMgr, mustGroup(t, primitiveMgr), body)
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
						for _, r := range resp.Resources {
							if r.Name == "res2" {
								if r.Title != "Title 2" {
									t.Errorf("expected Title=Title 2, got %q", r.Title)
								}
								if r.MimeType != "application/json" {
									t.Errorf("expected MimeType=application/json, got %q", r.MimeType)
								}
								if r.Size == nil || *r.Size != 2048 {
									t.Errorf("expected Size=2048, got %v", r.Size)
								}
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

			got, err := resourceTemplatesListHandler(ctx, dummyID, primitiveMgr, mustGroup(t, primitiveMgr), body)
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
						for _, rt := range resp.ResourceTemplates {
							if rt.Name == "rt2" {
								if rt.Title != "Title RT" {
									t.Errorf("expected Title=Title RT, got %q", rt.Title)
								}
								if rt.MimeType != "text/plain" {
									t.Errorf("expected MimeType=text/plain, got %q", rt.MimeType)
								}
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

			got, err := resourcesReadHandler(ctx, dummyID, primitiveMgr, mustGroup(t, primitiveMgr), body)
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
