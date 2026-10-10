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

package v20241105

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/auth"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	mcputil "github.com/googleapis/mcp-toolbox/internal/server/mcp/util"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// ProcessMethod returns a response for the request.
func ProcessMethod(ctx context.Context, id jsonrpc.RequestId, method string, g group.Group, primitiveMgr *primitives.PrimitiveManager, body []byte, header http.Header) (any, error) {
	switch method {
	case INITIALIZE:
		return initializeHandler(ctx, id, body)
	case PING:
		return pingHandler(id)
	case TOOLS_LIST:
		return toolsListHandler(ctx, id, primitiveMgr, g, body)
	case TOOLS_CALL:
		return toolsCallHandler(ctx, id, g, primitiveMgr, body, header)
	case RESOURCES_LIST:
		return resourcesListHandler(ctx, id, primitiveMgr, g, body)
	case RESOURCES_TEMPLATES_LIST:
		return resourceTemplatesListHandler(ctx, id, primitiveMgr, g, body)
	case RESOURCES_READ:
		return resourcesReadHandler(ctx, id, primitiveMgr, g, body)
	case PROMPTS_LIST:
		return promptsListHandler(ctx, id, primitiveMgr, g, body)
	case PROMPTS_GET:
		return promptsGetHandler(ctx, id, g, primitiveMgr, body)
	default:
		err := fmt.Errorf("invalid method %s", method)
		return jsonrpc.NewError(id, jsonrpc.METHOD_NOT_FOUND, err.Error(), nil), err
	}
}

// InitializeResponse runs capability negotiation and protocol version agreement.
// This is the Initialization phase of the lifecycle for MCP client-server connections.
// Always start with the latest protocol version supported.
func initializeHandler(ctx context.Context, id jsonrpc.RequestId, body []byte) (any, error) {
	v, err := util.ToolboxVersionFromContext(ctx)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}

	var req InitializeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid mcp initialize request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}

	toolsListChanged := false
	promptsListChanged := false
	result := InitializeResult{
		ProtocolVersion: PROTOCOL_VERSION,
		Capabilities: ServerCapabilities{
			Tools: &ListChanged{
				ListChanged: &toolsListChanged,
			},
			Prompts: &ListChanged{
				ListChanged: &promptsListChanged,
			},
			Resources: &ResourceCapabilities{},
		},
		ServerInfo: Implementation{
			BaseMetadata: BaseMetadata{
				Name: SERVER_NAME,
			},
			Version: v,
		},
	}
	res := jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  result,
	}

	return res, nil
}

// pingHandler handles the "ping" method by returning an empty response.
func pingHandler(id jsonrpc.RequestId) (any, error) {
	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  struct{}{},
	}, nil
}

func toolsListHandler(ctx context.Context, id jsonrpc.RequestId, primitiveMgr *primitives.PrimitiveManager, g group.Group, body []byte) (any, error) {
	var req ListToolsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid mcp tools list request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}

	urlParams, _ := util.UrlParamsFromContext(ctx)
	listToolsResult, err := GenerateListToolsResult(primitiveMgr, g, urlParams)
	if err != nil {
		err = fmt.Errorf("error generating manifest: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}

	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  listToolsResult,
	}, nil
}

// toolsCallHandler generate a response for tools call.
func toolsCallHandler(ctx context.Context, id jsonrpc.RequestId, g group.Group, primitiveMgr *primitives.PrimitiveManager, body []byte, header http.Header) (any, error) {
	if header != nil {
		if clientIP := util.ExtractClientIP(header); clientIP != "" {
			ctx = util.WithClientIP(ctx, clientIP)
		}
	}

	authServices := primitiveMgr.AuthServices()

	// retrieve logger from context
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}

	var req CallToolRequest
	if err = json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid mcp tools call request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}

	toolName := req.Params.Name
	toolArgument := req.Params.Arguments
	logger.DebugContext(ctx, fmt.Sprintf("tool name: %s", toolName))

	// Update span name and set gen_ai attributes
	span := trace.SpanFromContext(ctx)
	span.SetName(fmt.Sprintf("%s %s", TOOLS_CALL, toolName))
	span.SetAttributes(
		attribute.String("gen_ai.tool.name", toolName),
		attribute.String("gen_ai.operation.name", "execute_tool"),
	)

	// Verify tool belongs to the current group before resolving globally.
	if !g.ContainsTool(toolName) {
		err = fmt.Errorf("invalid tool name: tool with name %q does not exist", toolName)
		return jsonrpc.NewError(id, jsonrpc.INVALID_PARAMS, err.Error(), nil), err
	}

	tool, ok := primitiveMgr.GetTool(toolName)
	if !ok {
		err = fmt.Errorf("invalid tool name: tool with name %q does not exist", toolName)
		return jsonrpc.NewError(id, jsonrpc.INVALID_PARAMS, err.Error(), nil), err
	}

	if tool.HasSecureParams() {
		err = fmt.Errorf("invalid tool name: tool with name %q does not exist", toolName)
		return jsonrpc.NewError(id, jsonrpc.INVALID_PARAMS, err.Error(), nil), err
	}

	srcName := tool.GetSourceName()
	var src sources.Source
	if srcName != "" {
		src, ok = primitiveMgr.GetSource(srcName)
		if !ok {
			err = fmt.Errorf("unable to retrieve source for tool %s", toolName)
			return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
		}
	}

	err = tool.ValidateSource(src)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	// Populate gen_ai attributes for operation duration metric
	if genAIAttrs := util.GenAIMetricAttrsFromContext(ctx); genAIAttrs != nil {
		genAIAttrs.OperationName = "execute_tool"
		genAIAttrs.ToolName = toolName
	}

	// Get access token
	authTokenHeadername, err := tool.GetAuthTokenHeaderName(src)
	if err != nil {
		errMsg := fmt.Errorf("error during invocation: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, errMsg.Error(), nil), errMsg
	}
	var accessToken tools.AccessToken
	if header != nil {
		accessToken = tools.AccessToken(header.Get(authTokenHeadername))
	}

	// Check if this specific tool requires the standard authorization header
	clientAuth, err := tool.RequiresClientAuthorization(src)
	if err != nil {
		errMsg := fmt.Errorf("error during invocation: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, errMsg.Error(), nil), errMsg
	}
	if clientAuth {
		if accessToken == "" {
			err := util.NewClientServerError(
				"missing access token in the 'Authorization' header",
				http.StatusUnauthorized,
				nil,
			)
			return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
		}
	}

	// marshal arguments and decode it using decodeJSON instead to prevent loss between floats/int.
	var data map[string]any
	if toolArgument != nil {
		aMarshal, err := json.Marshal(toolArgument)
		if err != nil {
			err = fmt.Errorf("unable to marshal tools argument: %w", err)
			return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
		}

		if err = util.DecodeJSON(bytes.NewBuffer(aMarshal), &data); err != nil {
			err = fmt.Errorf("unable to decode tools argument: %w", err)
			return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
		}
	}
	if data == nil {
		data = make(map[string]any)
	}

	// Tool authentication
	// claimsFromAuth maps the name of the authservice to the claims retrieved from it.
	claimsFromAuth := make(map[string]map[string]any)

	// if using stdio, header will be nil and auth will not be supported
	if header != nil {
		for _, aS := range authServices {
			var claims map[string]any
			var err error

			if mSvc, ok := aS.(auth.MCPAuthService); ok && mSvc.IsMCPEnabled() {
				claims = util.AuthTokenClaimsFromContext(ctx)
			} else {
				claims, err = aS.GetClaimsFromHeader(ctx, header)
				if err != nil {
					logger.DebugContext(ctx, err.Error())
					continue
				}
			}

			if claims == nil {
				// authService not present in header
				continue
			}
			claimsFromAuth[aS.GetName()] = claims
		}
	}

	// Tool authorization check
	verifiedAuthServices := make([]string, len(claimsFromAuth))
	i := 0
	for k := range claimsFromAuth {
		verifiedAuthServices[i] = k
		i++
	}

	// Check if any of the specified auth services is verified
	isAuthorized := tool.Authorized(verifiedAuthServices)
	if !isAuthorized {
		err = util.NewClientServerError(
			"unauthorized Tool call: Please make sure you specify correct auth headers",
			http.StatusUnauthorized,
			nil,
		)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}
	logger.DebugContext(ctx, "tool invocation authorized")

	if err := mcputil.ValidateScopes(ctx, tool.GetScopesRequired(), authServices); err != nil {
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}

	toolParams, err := tool.GetParameters(src)
	if err != nil {
		err = fmt.Errorf("error getting parameters for tool: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}

	// Auto-populate arguments from URL parameters
	data, err = mcputil.PopulateUrlParams(ctx, data, toolParams)
	if err != nil {
		text := TextContent{
			Type: "text",
			Text: err.Error(),
		}
		return jsonrpc.JSONRPCResponse{
			Jsonrpc: jsonrpc.JSONRPC_VERSION,
			Id:      id,
			Result:  CallToolResult{Content: []TextContent{text}, IsError: true},
		}, nil
	}

	params, err := parameters.ParseParams(toolParams, data, claimsFromAuth)
	if err != nil {
		text := TextContent{
			Type: "text",
			Text: fmt.Sprintf("provided parameters were invalid: %s", err),
		}
		return jsonrpc.JSONRPCResponse{
			Jsonrpc: jsonrpc.JSONRPC_VERSION,
			Id:      id,
			Result:  CallToolResult{Content: []TextContent{text}, IsError: true},
		}, nil
	}
	logger.DebugContext(ctx, fmt.Sprintf("invocation params: %s", params))

	params, err = tool.EmbedParams(ctx, params, primitiveMgr)
	if err != nil {
		text := TextContent{
			Type: "text",
			Text: fmt.Sprintf("error embedding parameters: %s", err),
		}
		return jsonrpc.JSONRPCResponse{
			Jsonrpc: jsonrpc.JSONRPC_VERSION,
			Id:      id,
			Result:  CallToolResult{Content: []TextContent{text}, IsError: true},
		}, nil
	}

	// Get instrumentation for recording tool execution duration
	instrumentation, instrumentationErr := util.InstrumentationFromContext(ctx)

	// run tool invocation and generate response.
	executionStart := time.Now()
	results, err := tool.Invoke(ctx, src, params, accessToken)
	executionDuration := time.Since(executionStart).Seconds()

	// Record tool execution duration metric
	if instrumentationErr == nil {
		execAttrs := []attribute.KeyValue{
			attribute.String("gen_ai.tool.name", toolName),
		}
		// Add network protocol attributes from context
		if genAIAttrs := util.GenAIMetricAttrsFromContext(ctx); genAIAttrs != nil {
			if genAIAttrs.NetworkProtocolName != "" {
				execAttrs = append(execAttrs, attribute.String("network.protocol.name", genAIAttrs.NetworkProtocolName))
			}
			if genAIAttrs.NetworkProtocolVersion != "" {
				execAttrs = append(execAttrs, attribute.String("network.protocol.version", genAIAttrs.NetworkProtocolVersion))
			}
		}
		if err != nil {
			execAttrs = append(execAttrs, attribute.String("error.type", err.Error()))
		}
		instrumentation.ToolExecutionDuration.Record(ctx, executionDuration, metric.WithAttributes(execAttrs...))
	}

	if err != nil {
		var tbErr util.ToolboxError

		if errors.As(err, &tbErr) {
			switch tbErr.Category() {
			case util.CategoryAgent:
				// MCP - Tool execution error
				// Return SUCCESS but with IsError: true
				text := TextContent{
					Type: "text",
					Text: err.Error(),
				}
				return jsonrpc.JSONRPCResponse{
					Jsonrpc: jsonrpc.JSONRPC_VERSION,
					Id:      id,
					Result:  CallToolResult{Content: []TextContent{text}, IsError: true},
				}, nil

			case util.CategoryServer:
				// MCP Spec - Protocol error
				// Return JSON-RPC ERROR
				var clientServerErr *util.ClientServerError
				rpcCode := jsonrpc.INTERNAL_ERROR // Default to Internal Error (-32603)

				if errors.As(err, &clientServerErr) {
					if clientServerErr.Code == http.StatusUnauthorized || clientServerErr.Code == http.StatusForbidden {
						if clientAuth {
							rpcCode = jsonrpc.INVALID_REQUEST
						} else {
							rpcCode = jsonrpc.INTERNAL_ERROR
						}
					}
				}
				return jsonrpc.NewError(id, rpcCode, err.Error(), nil), err
			}
		} else {
			// Unknown error -> 500
			return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
		}
	}

	content := make([]TextContent, 0)

	sliceRes, ok := results.([]any)
	if !ok {
		sliceRes = []any{results}
	}

	for _, d := range sliceRes {
		text := TextContent{Type: "text"}
		dM, err := json.Marshal(d)
		if err != nil {
			text.Text = fmt.Sprintf("fail to marshal: %s, result: %s", err, d)
		} else {
			text.Text = string(dM)
		}
		content = append(content, text)
	}

	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  CallToolResult{Content: content},
	}, nil
}

// promptsListHandler handles the "prompts/list" method.
func promptsListHandler(ctx context.Context, id jsonrpc.RequestId, primitiveMgr *primitives.PrimitiveManager, g group.Group, body []byte) (any, error) {
	// retrieve logger from context
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	logger.DebugContext(ctx, "handling prompts/list request")

	var req ListPromptsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid mcp prompts list request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}

	listPromptsResult, err := GenerateListPromptsResult(primitiveMgr, g)
	if err != nil {
		err = fmt.Errorf("error generating manifest: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	logger.DebugContext(ctx, fmt.Sprintf("returning %d prompts", len(listPromptsResult.Prompts)))
	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  listPromptsResult,
	}, nil
}

// promptsGetHandler handles the "prompts/get" method.
func promptsGetHandler(ctx context.Context, id jsonrpc.RequestId, g group.Group, primitiveMgr *primitives.PrimitiveManager, body []byte) (any, error) {
	// retrieve logger from context
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	logger.DebugContext(ctx, "handling prompts/get request")

	var req GetPromptRequest
	if err := json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid mcp prompts/get request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}

	promptName := req.Params.Name
	logger.DebugContext(ctx, fmt.Sprintf("prompt name: %s", promptName))

	// Update span name and set gen_ai attributes
	span := trace.SpanFromContext(ctx)
	span.SetName(fmt.Sprintf("%s %s", PROMPTS_GET, promptName))
	span.SetAttributes(attribute.String("gen_ai.prompt.name", promptName))

	// Populate gen_ai attributes for operation duration metric
	if genAIAttrs := util.GenAIMetricAttrsFromContext(ctx); genAIAttrs != nil {
		genAIAttrs.OperationName = "get_prompt"
		genAIAttrs.PromptName = promptName
	}

	// Verify prompt belongs to the current group before resolving globally.
	if !g.ContainsPrompt(promptName) {
		err := fmt.Errorf("prompt with name %q does not exist", promptName)
		return jsonrpc.NewError(id, jsonrpc.INVALID_PARAMS, err.Error(), nil), err
	}

	prompt, ok := primitiveMgr.GetPrompt(promptName)
	if !ok {
		err := fmt.Errorf("prompt with name %q does not exist", promptName)
		return jsonrpc.NewError(id, jsonrpc.INVALID_PARAMS, err.Error(), nil), err
	}

	// Parse the arguments provided in the request.
	argValues, err := prompt.ParseArgs(req.Params.Arguments, nil)
	if err != nil {
		err = fmt.Errorf("invalid arguments for prompt %q: %w", promptName, err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_PARAMS, err.Error(), nil), err
	}
	logger.DebugContext(ctx, fmt.Sprintf("parsed args: %v", argValues))

	// Substitute the argument values into the prompt's messages.
	substituted, err := prompt.SubstituteParams(argValues)
	if err != nil {
		err = fmt.Errorf("error substituting params for prompt %q: %w", promptName, err)
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	logger.DebugContext(ctx, "substituted params successfully")

	// Cast the result to the expected []prompts.Message type.
	substitutedMessages, ok := substituted.([]prompts.Message)
	if !ok {
		err = fmt.Errorf("internal error: SubstituteParams returned unexpected type")
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}

	// Format the response messages into the required structure.
	promptMessages := make([]PromptMessage, len(substitutedMessages))
	for i, msg := range substitutedMessages {
		promptMessages[i] = PromptMessage{
			Role: msg.Role,
			Content: TextContent{
				Type: "text",
				Text: msg.Content,
			},
		}
	}

	result := GetPromptResult{
		Description: prompt.Manifest().Description,
		Messages:    promptMessages,
	}

	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  result,
	}, nil
}

// resourcesListHandler generates a response for resources/list.
func resourcesListHandler(ctx context.Context, id jsonrpc.RequestId, primitiveMgr *primitives.PrimitiveManager, g group.Group, body []byte) (any, error) {
	// retrieve logger from context
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	logger.DebugContext(ctx, "handling resources/list request")

	var req ListResourcesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid mcp resources list request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}

	result, err := GenerateListResourcesResult(primitiveMgr, g)
	if err != nil {
		err = fmt.Errorf("error generating manifest: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}

	logger.DebugContext(ctx, fmt.Sprintf("returning %d resources", len(result.Resources)))
	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  result,
	}, nil
}

// resourceTemplatesListHandler generates a response for resources/templates/list.
func resourceTemplatesListHandler(ctx context.Context, id jsonrpc.RequestId, primitiveMgr *primitives.PrimitiveManager, g group.Group, body []byte) (any, error) {
	// retrieve logger from context
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	logger.DebugContext(ctx, "handling resources/templates/list request")

	var req ListResourceTemplatesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid mcp resource templates list request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}

	result, err := GenerateListResourceTemplatesResult(primitiveMgr, g)
	if err != nil {
		err = fmt.Errorf("error generating manifest: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}

	logger.DebugContext(ctx, fmt.Sprintf("returning %d resource templates", len(result.ResourceTemplates)))
	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  result,
	}, nil
}

// getResourceOrTemplateByURI looks up a resource by exact URI match within a group.
// If not found, it attempts to match against resource templates (e.g. file://{path}).
// Returns the matched resource OR template, plus extracted params if a template was matched.
func getResourceOrTemplateByURI(uri string, g group.Group, primitiveMgr *primitives.PrimitiveManager) (resources.Resource, resources.ResourceTemplate, map[string]any, error) {
	for _, name := range g.ResourceNames {
		if res, ok := primitiveMgr.GetResource(name); ok {
			if res.GetURI() == uri {
				return res, nil, nil, nil
			}
		}
	}

	for _, name := range g.ResourceTemplateNames {
		if rt, ok := primitiveMgr.GetResourceTemplate(name); ok {
			tmpl := rt.GetURITemplate()
			if strings.Contains(tmpl, "{path}") {
				regexPattern := regexp.QuoteMeta(tmpl)
				regexPattern = strings.ReplaceAll(regexPattern, "\\{path\\}", "(.*)")
				re, err := regexp.Compile("^" + regexPattern + "$")
				if err != nil {
					continue
				}
				matches := re.FindStringSubmatch(uri)
				if len(matches) == 2 {
					return nil, rt, map[string]any{"path": matches[1]}, nil
				}
			}
		}
	}
	return nil, nil, nil, fmt.Errorf("no resource or template found for URI: %s", uri)
}

// resourcesReadHandler generates a response for resources/read.
func resourcesReadHandler(ctx context.Context, id jsonrpc.RequestId, primitiveMgr *primitives.PrimitiveManager, g group.Group, body []byte) (any, error) {
	// retrieve logger from context
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	logger.DebugContext(ctx, "handling resources/read request")

	var req ReadResourceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid mcp resources read request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}

	uri := req.Params.Uri
	logger.DebugContext(ctx, fmt.Sprintf("resource uri: %s", uri))

	// Update span name and set gen_ai attributes
	span := trace.SpanFromContext(ctx)
	span.SetName(fmt.Sprintf("%s %s", RESOURCES_READ, uri))
	span.SetAttributes(attribute.String("gen_ai.resource.name", uri))

	// Populate gen_ai attributes for operation duration metric
	if genAIAttrs := util.GenAIMetricAttrsFromContext(ctx); genAIAttrs != nil {
		genAIAttrs.OperationName = "read_resource"
	}

	// Verify resource belongs to the current group before resolving globally.
	res, resTmpl, params, err := getResourceOrTemplateByURI(uri, g, primitiveMgr)
	if err != nil {
		err = fmt.Errorf("resource lookup failed: %w", err)
		return jsonrpc.NewError(id, jsonrpc.RESOURCE_NOT_FOUND, err.Error(), nil), err
	}

	var content any
	var mimeType string

	if res != nil {
		content, err = res.Read(ctx, nil)
		mimeType = res.GetMimeType()
	} else {
		content, err = resTmpl.Read(ctx, params)
		mimeType = resTmpl.GetMimeType()
	}

	if err != nil {
		err = fmt.Errorf("failed to read resource: %w", err)
		if errors.Is(err, fs.ErrNotExist) {
			return jsonrpc.NewError(id, jsonrpc.RESOURCE_NOT_FOUND, err.Error(), nil), err
		}
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}

	// Only text content resource is supported
	textContent, ok := content.(string)
	if !ok {
		err = fmt.Errorf("unsupported resource content type, expected string")
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	logger.DebugContext(ctx, "read resource successfully")

	result := &ReadResourceResult{
		Contents: []TextResourceContents{
			{
				ResourceContents: ResourceContents{
					Uri:      uri,
					MimeType: mimeType,
				},
				Text: textContent,
			},
		},
	}

	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  result,
	}, nil
}
