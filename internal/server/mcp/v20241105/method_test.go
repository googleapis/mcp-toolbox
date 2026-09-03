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

package v20241105

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/log"
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
