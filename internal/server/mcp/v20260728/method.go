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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/auth"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
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
	case SERVER_DISCOVER:
		return serverDiscoverHandler(ctx, id, body, header)
	case TOOLS_LIST:
		return toolsListHandler(ctx, id, primitiveMgr, g, body, header)
	case TOOLS_CALL:
		return toolsCallHandler(ctx, id, g, primitiveMgr, body, header)
	case PROMPTS_LIST:
		return promptsListHandler(ctx, id, primitiveMgr, g, body, header)
	case PROMPTS_GET:
		return promptsGetHandler(ctx, id, g, primitiveMgr, body, header)
	case GROUPS_LIST:
		return groupsListHandler(ctx, id, primitiveMgr, body, header)
	case GROUPS_GET:
		return groupsGetHandler(ctx, id, primitiveMgr, body, header)
	default:
		err := fmt.Errorf("invalid method %s", method)
		return jsonrpc.NewError(id, jsonrpc.METHOD_NOT_FOUND, err.Error(), nil), err
	}
}

// validateMetadata checks the metadata of every requests
func validateMetadata(id jsonrpc.RequestId, params RequestParams, stdio bool) (any, error) {
	// Check _meta field for protocol version
	if params.Meta != nil {
		// check for protocol version metadata
		v := params.Meta.ProtocolVersion
		if v == "" {
			metaErr := fmt.Errorf("_meta error: missing io.modelcontextprotocol/protocolVersion")
			return jsonrpc.NewError(id, jsonrpc.INVALID_PARAMS, metaErr.Error(), nil), metaErr
		}
		// do not have to verify header for stdio; already verified during stdio
		// message processing
		if !stdio {
			if PROTOCOL_VERSION != v {
				metaErr := fmt.Errorf("_meta error: header mismatch: MCP-Protocol-Version header value '%s' does not match body value '%s'", PROTOCOL_VERSION, v)
				return jsonrpc.NewHeaderMismatchedError(id, metaErr), metaErr
			}
		}

		// check for clientInfo
		clientInfo := params.Meta.ClientInfo
		if clientInfo.Version == "" || clientInfo.Name == "" {
			metaErr := fmt.Errorf("_meta error: missing field from io.modelcontextprotocol/clientInfo")
			return jsonrpc.NewError(id, jsonrpc.INVALID_PARAMS, metaErr.Error(), nil), metaErr
		}
		// check for clientCapabilities
		clientCapabilities := params.Meta.MetaClientCapabilities
		if clientCapabilities == nil {
			metaErr := fmt.Errorf("_meta error: missing field from io.modelcontextprotocol/clientCapabilities")
			return jsonrpc.NewError(id, jsonrpc.INVALID_PARAMS, metaErr.Error(), nil), metaErr
		}
		// skip checking clientCapabilities since Toolbox do not utilize any of those
	} else {
		metaErr := fmt.Errorf("_meta error: missing required fields in request metadata")
		return jsonrpc.NewError(id, jsonrpc.INVALID_PARAMS, metaErr.Error(), nil), metaErr
	}
	return nil, nil
}

// validateHeader checks the header of every requests
// Toolbox do not check for `Mcp-Param-{Name}` header since we are not
// implementing custom headers from parameters
// Do not need to check for Base64-encoding or invalid characters since we are
// only checking `mcp-method` and `mcp-name`
func validateHeader(id jsonrpc.RequestId, header http.Header, method, name string) (any, error) {
	// stdio transport will not have header
	if header == nil {
		return nil, nil
	}
	headerMethod := header.Get("mcp-method")
	if headerMethod != method {
		err := fmt.Errorf("Mcp-Method header value '%s' does not match body value '%s'", headerMethod, method)
		return jsonrpc.NewHeaderMismatchedError(id, err), err
	}
	headerName := header.Get("mcp-name")
	if headerName != name {
		err := fmt.Errorf("Mcp-Name header value '%s' does not match body value '%s'", headerName, name)
		return jsonrpc.NewHeaderMismatchedError(id, err), err
	}
	return nil, nil
}

// validateToolboxExtension checks that the client negotiated the Toolbox
// extension. Methods that are only part of the extension must not be served to
// clients that did not declare it.
func validateToolboxExtension(id jsonrpc.RequestId, params RequestParams, method string) (any, error) {
	supportedExts := ParseSupportedExtensions(params.Meta.MetaClientCapabilities.Extensions)
	if _, ok := supportedExts[ToolboxExtensionURI]; !ok {
		err := fmt.Errorf("missing required client capability: method %q requires %s extension which is not supported by the client", method, ToolboxExtensionURI)
		return jsonrpc.NewError(id, jsonrpc.MISSING_REQUIRED_CLIENT_CAPABILITY, err.Error(), nil), err
	}
	return nil, nil
}

// getResultMetadata append the resultMetaObject on existing metadata
func getResultMetadata(ctx context.Context, curMeta map[string]any) (map[string]any, error) {
	v, err := util.ToolboxVersionFromContext(ctx)
	if err != nil {
		return nil, err
	}

	resMetaObj := ResultMetaObject{
		ServerInfo: Implementation{
			BaseMetadata: BaseMetadata{
				Name: SERVER_NAME,
			},
			Version: v,
		},
	}
	jsonData, err := json.Marshal(resMetaObj)
	if err != nil {
		return nil, err
	}
	resMeta := make(map[string]any)
	if err := json.Unmarshal(jsonData, &resMeta); err != nil {
		return nil, err
	}
	// if there are no existing metadata field, we can just return the
	// ResultMetaObject
	if curMeta == nil {
		return resMeta, nil
	}

	newMeta := make(map[string]any)
	// copy current Metadata into the new meta obj
	for k, v := range curMeta {
		newMeta[k] = v
	}
	// copy the ResultMetaObject items over
	for k, v := range resMeta {
		newMeta[k] = v
	}
	return newMeta, nil
}

func serverDiscoverHandler(ctx context.Context, id jsonrpc.RequestId, body []byte, header http.Header) (any, error) {
	enableDraft, ok := util.EnableDraftSpecsFromContext(ctx)
	if !ok {
		err := fmt.Errorf("unable to retrieve enableDraftSpecs from context")
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}

	var req DiscoverRequest
	if err := json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid server discover request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}
	validateHeaderErr, err := validateHeader(id, header, SERVER_DISCOVER, "")
	if err != nil {
		return validateHeaderErr, err
	}
	validateErr, err := validateMetadata(id, req.Params, header == nil)
	if err != nil {
		return validateErr, err
	}

	toolsListChanged := false
	promptsListChanged := false
	meta, err := getResultMetadata(ctx, nil)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	result := DiscoverResult{
		Result: Result{
			ResultType: resultTypeComplete,
			Result: jsonrpc.Result{
				Meta: meta,
			},
		},
		SupportedVersions: mcputil.GetSupportedVersions(enableDraft),
		Capabilities: ServerCapabilities{
			Extensions: ServerExtensions,
			Tools: &ListChanged{
				ListChanged: &toolsListChanged,
			},
			Prompts: &ListChanged{
				ListChanged: &promptsListChanged,
			},
		},
	}
	res := jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  result,
	}
	return res, nil
}

func toolsListHandler(ctx context.Context, id jsonrpc.RequestId, primitiveMgr *primitives.PrimitiveManager, g group.Group, body []byte, header http.Header) (any, error) {
	var req ListToolsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid mcp tools list request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}
	validateHeaderErr, err := validateHeader(id, header, TOOLS_LIST, "")
	if err != nil {
		return validateHeaderErr, err
	}
	validateErr, err := validateMetadata(id, req.Params.RequestParams, header == nil)
	if err != nil {
		return validateErr, err
	}

	urlParams, _ := util.UrlParamsFromContext(ctx)
	supportedExts := ParseSupportedExtensions(req.Params.Meta.MetaClientCapabilities.Extensions)
	_, hasSecureParamsSupport := supportedExts[ToolboxExtensionURI]
	listToolsResult, err := GenerateListToolsResult(primitiveMgr, g, urlParams, hasSecureParamsSupport)
	if err != nil {
		err = fmt.Errorf("error generating manifest: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	meta, err := getResultMetadata(ctx, listToolsResult.Meta)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	listToolsResult.Meta = meta
	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  listToolsResult,
	}, nil
}

// toolsCallHandler generate a response for tools call.
func toolsCallHandler(ctx context.Context, id jsonrpc.RequestId, g group.Group, primitiveMgr *primitives.PrimitiveManager, body []byte, header http.Header) (any, error) {
	authServices := primitiveMgr.AuthServices()

	// retrieve logger from context
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	meta, err := getResultMetadata(ctx, nil)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}

	var req CallToolRequest
	if err = json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid mcp tools call request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}
	validateHeaderErr, err := validateHeader(id, header, TOOLS_CALL, req.Params.Name)
	if err != nil {
		return validateHeaderErr, err
	}
	validateErr, err := validateMetadata(id, req.Params.RequestParams, header == nil)
	if err != nil {
		return validateErr, err
	}

	toolName := req.Params.Name
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

	supportedExts := ParseSupportedExtensions(req.Params.Meta.MetaClientCapabilities.Extensions)
	_, hasSecureParamsSupport := supportedExts[ToolboxExtensionURI]
	if tool.HasSecureParams() && !hasSecureParamsSupport {
		err = fmt.Errorf("missing required client capability: tool %q requires %s extension which is not supported by the client", toolName, ToolboxExtensionURI)
		return jsonrpc.NewError(id, jsonrpc.MISSING_REQUIRED_CLIENT_CAPABILITY, err.Error(), nil), err
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

	toolParams, err := tool.GetParameters(src)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}

	toolArguments, agentErr, protocolErr := validateAndMergeSecureParams(ctx, &req, toolParams)
	if protocolErr != nil {
		return jsonrpc.NewError(id, jsonrpc.INVALID_PARAMS, protocolErr.Error(), nil), protocolErr
	}
	if agentErr != nil {
		text := TextContent{
			Type: "text",
			Text: agentErr.Error(),
		}
		return jsonrpc.JSONRPCResponse{
			Jsonrpc: jsonrpc.JSONRPC_VERSION,
			Id:      id,
			Result: CallToolResult{
				Result: Result{
					ResultType: resultTypeComplete,
					Result: jsonrpc.Result{
						Meta: meta,
					},
				},
				Content: []TextContent{text},
				IsError: true,
			},
		}, nil
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

	var data map[string]any
	if toolArguments != nil {
		aMarshal, err := json.Marshal(toolArguments)
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
			Result: CallToolResult{
				Result: Result{
					ResultType: resultTypeComplete,
					Result: jsonrpc.Result{
						Meta: meta,
					},
				},
				Content: []TextContent{text},
				IsError: true,
			},
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
			Result: CallToolResult{
				Result: Result{
					ResultType: resultTypeComplete,
					Result: jsonrpc.Result{
						Meta: meta,
					},
				},
				Content: []TextContent{text},
				IsError: true,
			},
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
			Result: CallToolResult{
				Result: Result{
					ResultType: resultTypeComplete,
					Result: jsonrpc.Result{
						Meta: meta,
					},
				},
				Content: []TextContent{text},
				IsError: true,
			},
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
					Result: CallToolResult{
						Result: Result{
							ResultType: resultTypeComplete,
							Result: jsonrpc.Result{
								Meta: meta,
							},
						},
						Content: []TextContent{text},
						IsError: true,
					},
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
		Result: CallToolResult{
			Result: Result{
				ResultType: resultTypeComplete,
				Result: jsonrpc.Result{
					Meta: meta,
				},
			},
			Content: content,
		},
	}, nil
}

// promptsListHandler handles the "prompts/list" method.
func promptsListHandler(ctx context.Context, id jsonrpc.RequestId, primitiveMgr *primitives.PrimitiveManager, g group.Group, body []byte, header http.Header) (any, error) {
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
	validateHeaderErr, err := validateHeader(id, header, PROMPTS_LIST, "")
	if err != nil {
		return validateHeaderErr, err
	}
	validateErr, err := validateMetadata(id, req.Params.RequestParams, header == nil)
	if err != nil {
		return validateErr, err
	}

	listPromptsResult, err := GenerateListPromptsResult(primitiveMgr, g)
	if err != nil {
		err = fmt.Errorf("error generating manifest: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	logger.DebugContext(ctx, fmt.Sprintf("returning %d prompts", len(listPromptsResult.Prompts)))
	meta, err := getResultMetadata(ctx, listPromptsResult.Meta)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	listPromptsResult.Meta = meta
	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  listPromptsResult,
	}, nil
}

// promptsGetHandler handles the "prompts/get" method.
func promptsGetHandler(ctx context.Context, id jsonrpc.RequestId, g group.Group, primitiveMgr *primitives.PrimitiveManager, body []byte, header http.Header) (any, error) {
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
	validateHeaderErr, err := validateHeader(id, header, PROMPTS_GET, req.Params.Name)
	if err != nil {
		return validateHeaderErr, err
	}
	validateErr, err := validateMetadata(id, req.Params.RequestParams, header == nil)
	if err != nil {
		return validateErr, err
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

	// Cast the result to the expected []prompts.Message type.
	substitutedMessages, ok := substituted.([]prompts.Message)
	if !ok {
		err = fmt.Errorf("internal error: SubstituteParams returned unexpected type")
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	logger.DebugContext(ctx, "substituted params successfully")

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
	meta, err := getResultMetadata(ctx, nil)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	result := GetPromptResult{
		Result: Result{
			ResultType: resultTypeComplete,
			Result: jsonrpc.Result{
				Meta: meta,
			},
		},
		Description: prompt.Manifest().Description,
		Messages:    promptMessages,
	}
	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  result,
	}, nil
}

// groupsListHandler handles the "groups/list" method. It returns every named
// group's name and description. The default nameless group is omitted.
func groupsListHandler(ctx context.Context, id jsonrpc.RequestId, primitiveMgr *primitives.PrimitiveManager, body []byte, header http.Header) (any, error) {
	// retrieve logger from context
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	logger.DebugContext(ctx, "handling groups/list request")

	var req ListGroupsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid mcp groups list request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}
	validateHeaderErr, err := validateHeader(id, header, GROUPS_LIST, "")
	if err != nil {
		return validateHeaderErr, err
	}
	validateErr, err := validateMetadata(id, req.Params, header == nil)
	if err != nil {
		return validateErr, err
	}
	extErr, err := validateToolboxExtension(id, req.Params, GROUPS_LIST)
	if err != nil {
		return extErr, err
	}

	groupsList := primitiveMgr.GroupsList()
	groups := []Group{}
	for _, v := range groupsList {
		groups = append(groups, Group{Name: v.Name, Description: v.Description})
	}
	logger.DebugContext(ctx, fmt.Sprintf("returning %d groups", len(groups)))
	meta, err := getResultMetadata(ctx, nil)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result: ListGroupsResult{
			Result: Result{
				ResultType: resultTypeComplete,
				Result: jsonrpc.Result{
					Meta: meta,
				},
			},
			Groups: groups,
		},
	}, nil
}

// groupsGetHandler handles the "groups/get" method. It returns the named group's
// tools and prompts.
func groupsGetHandler(ctx context.Context, id jsonrpc.RequestId, primitiveMgr *primitives.PrimitiveManager, body []byte, header http.Header) (any, error) {
	// retrieve logger from context
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	logger.DebugContext(ctx, "handling groups/get request")

	var req GetGroupRequest
	if err := json.Unmarshal(body, &req); err != nil {
		err = fmt.Errorf("invalid mcp groups/get request: %w", err)
		return jsonrpc.NewError(id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}
	validateHeaderErr, err := validateHeader(id, header, GROUPS_GET, req.Params.Name)
	if err != nil {
		return validateHeaderErr, err
	}
	validateErr, err := validateMetadata(id, req.Params.RequestParams, header == nil)
	if err != nil {
		return validateErr, err
	}
	extErr, err := validateToolboxExtension(id, req.Params.RequestParams, GROUPS_GET)
	if err != nil {
		return extErr, err
	}

	groupName := req.Params.Name
	logger.DebugContext(ctx, fmt.Sprintf("group name: %s", groupName))

	// Update span name and set gen_ai attributes
	span := trace.SpanFromContext(ctx)
	span.SetName(fmt.Sprintf("%s %s", GROUPS_GET, groupName))
	span.SetAttributes(attribute.String("gen_ai.group.name", groupName))

	// Populate gen_ai attributes for operation duration metric
	if genAIAttrs := util.GenAIMetricAttrsFromContext(ctx); genAIAttrs != nil {
		genAIAttrs.OperationName = "get_group"
		genAIAttrs.GroupName = groupName
	}

	g, ok := primitiveMgr.GetGroup(groupName)
	if !ok {
		err := fmt.Errorf("invalid group name: group with name %q does not exist", groupName)
		return jsonrpc.NewError(id, jsonrpc.INVALID_PARAMS, err.Error(), nil), err
	}

	urlParams, _ := util.UrlParamsFromContext(ctx)
	// validateToolboxExtension above already established that the client
	// declared the extension, so secure params are always supported here.
	result, err := GenerateGetGroupResult(primitiveMgr, g, urlParams, true)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	meta, err := getResultMetadata(ctx, result.Meta)
	if err != nil {
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	result.Meta = meta

	return jsonrpc.JSONRPCResponse{
		Jsonrpc: jsonrpc.JSONRPC_VERSION,
		Id:      id,
		Result:  result,
	}, nil
}

// validateAndMergeSecureParams validates and merges standard and secure arguments.
func validateAndMergeSecureParams(ctx context.Context, req *CallToolRequest, paramDefs parameters.Parameters) (map[string]any, error, error) {
	secureParamMap := make(map[string]bool)
	urlParams, _ := util.UrlParamsFromContext(ctx)

	for _, p := range paramDefs {
		if p != nil && p.GetSecure() {
			secureParamMap[p.GetName()] = true
		}
	}

	// Validate that secure parameters are not passed in standard arguments (Agent error)
	for argName := range req.Params.Arguments {
		if secureParamMap[argName] {
			return nil, fmt.Errorf("parameter %q is secure and must not be passed in standard arguments", argName), nil
		}
	}

	// Validate that non-secure parameters are not passed in secureArguments (Protocol error)
	for argName := range req.Params.SecureArguments {
		if !secureParamMap[argName] {
			return nil, nil, fmt.Errorf("parameter %q is not secure and must not be passed in secureArguments", argName)
		}
	}

	// Validate that required secure parameters are present in secureArguments (Protocol error)
	for _, p := range paramDefs {
		if p != nil && p.GetSecure() {
			name := p.GetName()
			if p.GetValueFromParam() == "" {
				if _, bound := urlParams[name]; !bound {
					if parameters.CheckParamRequired(p.GetRequired(), p.GetDefault()) {
						if req.Params.SecureArguments == nil {
							return nil, nil, fmt.Errorf("missing required secure parameter %q in secureArguments", name)
						}
						if _, ok := req.Params.SecureArguments[name]; !ok {
							return nil, nil, fmt.Errorf("missing required secure parameter %q in secureArguments", name)
						}
					}
				}
			}
		}
	}

	// Merge standard arguments and secure arguments.
	toolArgument := make(map[string]any)
	maps.Copy(toolArgument, req.Params.Arguments)
	maps.Copy(toolArgument, req.Params.SecureArguments)

	return toolArgument, nil, nil
}
