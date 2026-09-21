// Copyright 2024 Google LLC
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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/googleapis/mcp-toolbox/internal/auth/generic"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// apiRouter creates a router that represents the routes under /api
func apiRouter(s *Server) (chi.Router, error) {
	r := chi.NewRouter()

	r.Use(middleware.AllowContentType("application/json"))
	r.Use(middleware.StripSlashes)
	r.Use(render.SetContentType(render.ContentTypeJSON))

	r.Get("/toolset", func(w http.ResponseWriter, r *http.Request) { toolsetHandler(s, w, r) })
	r.Get("/toolset/{toolsetName}", func(w http.ResponseWriter, r *http.Request) { toolsetHandler(s, w, r) })

	r.Route("/tool/{toolName}", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) { toolGetHandler(s, w, r) })
		r.Post("/invoke", func(w http.ResponseWriter, r *http.Request) { toolInvokeHandler(s, w, r) })
	})

	return r, nil
}

// toolsetHandler handles the request for information about a Toolset.
func toolsetHandler(s *Server, w http.ResponseWriter, r *http.Request) {
	ctx, span := s.instrumentation.Tracer.Start(r.Context(), "toolbox/server/toolset/get")
	r = r.WithContext(ctx)

	toolsetName := chi.URLParam(r, "toolsetName")
	s.logger.DebugContext(ctx, fmt.Sprintf("toolset name: %s", toolsetName))
	span.SetAttributes(attribute.String("toolset.name", toolsetName))
	var err error
	defer func() {
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	g, ok := s.PrimitiveMgr.GetGroup(toolsetName)
	if !ok {
		err = fmt.Errorf("toolset %q does not exist", toolsetName)
		s.logger.DebugContext(ctx, err.Error())
		_ = render.Render(w, r, newErrResponse(err, http.StatusNotFound))
		return
	}

	manifest, err := g.ToolsetManifest(s.version, s.PrimitiveMgr)
	if err != nil {
		s.logger.DebugContext(ctx, err.Error())
		_ = render.Render(w, r, newErrResponse(err, http.StatusInternalServerError))
		return
	}

	render.JSON(w, r, manifest)
}

// toolGetHandler handles requests for a single Tool.
func toolGetHandler(s *Server, w http.ResponseWriter, r *http.Request) {
	ctx, span := s.instrumentation.Tracer.Start(r.Context(), "toolbox/server/tool/get")
	r = r.WithContext(ctx)

	toolName := chi.URLParam(r, "toolName")
	s.logger.DebugContext(ctx, fmt.Sprintf("tool name: %s", toolName))
	span.SetAttributes(attribute.String("tool_name", toolName))
	var err error
	defer func() {
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	tool, ok := s.PrimitiveMgr.GetTool(toolName)
	if !ok {
		err = fmt.Errorf("invalid tool name: tool with name %q does not exist", toolName)
		s.logger.DebugContext(ctx, err.Error())
		_ = render.Render(w, r, newErrResponse(err, http.StatusNotFound))
		return
	}
	srcName := tool.GetSourceName()
	var src sources.Source
	if srcName != "" {
		src, ok = s.PrimitiveMgr.GetSource(srcName)
		if !ok {
			err = fmt.Errorf("unable to retrieve source for tool %s", toolName)
			s.logger.DebugContext(ctx, err.Error())
			_ = render.Render(w, r, newErrResponse(err, http.StatusNotFound))
			return
		}
	}
	toolManifest, err := tool.Manifest(src)
	if err != nil {
		err = fmt.Errorf("error generating manifest for tool %q: %w", toolName, err)
		s.logger.DebugContext(ctx, err.Error())
		_ = render.Render(w, r, newErrResponse(err, http.StatusInternalServerError))
		return
	}
	// TODO: this can be optimized later with some caching
	m := tools.ToolsetManifest{
		ServerVersion: s.version,
		ToolsManifest: map[string]tools.Manifest{
			toolName: toolManifest,
		},
	}

	render.JSON(w, r, m)
}

// toolInvokeHandler handles the API request to invoke a specific Tool.
func toolInvokeHandler(s *Server, w http.ResponseWriter, r *http.Request) {
	ctx, span := s.instrumentation.Tracer.Start(r.Context(), "toolbox/server/tool/invoke")
	r = r.WithContext(ctx)
	ctx = util.WithLogger(r.Context(), s.logger)

	toolName := chi.URLParam(r, "toolName")
	s.logger.DebugContext(ctx, fmt.Sprintf("tool name: %s", toolName))
	span.SetAttributes(attribute.String("tool_name", toolName))
	var err error
	defer func() {
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	tool, ok := s.PrimitiveMgr.GetTool(toolName)
	if !ok {
		err = fmt.Errorf("invalid tool name: tool with name %q does not exist", toolName)
		s.logger.DebugContext(ctx, err.Error())
		_ = render.Render(w, r, newErrResponse(err, http.StatusNotFound))
		return
	}

	srcName := tool.GetSourceName()
	var src sources.Source
	if srcName != "" {
		src, ok = s.PrimitiveMgr.GetSource(srcName)
		if !ok {
			err = fmt.Errorf("unable to retrieve source for tool %s", toolName)
			s.logger.DebugContext(ctx, err.Error())
			_ = render.Render(w, r, newErrResponse(err, http.StatusNotFound))
			return
		}
	}

	err = tool.ValidateSource(src)
	if err != nil {
		s.logger.DebugContext(ctx, err.Error())
		_ = render.Render(w, r, newErrResponse(err, http.StatusNotFound))
		return
	}

	// Extract OAuth access token from the "Authorization" header (currently for
	// BigQuery end-user credentials usage only)
	accessToken := tools.AccessToken(r.Header.Get("Authorization"))

	// Check if this specific tool requires the standard authorization header
	clientAuth, err := tool.RequiresClientAuthorization(src)
	if err != nil {
		errMsg := fmt.Errorf("error during invocation: %w", err)
		s.logger.DebugContext(ctx, errMsg.Error())
		_ = render.Render(w, r, newErrResponse(errMsg, http.StatusNotFound))
		return
	}
	if clientAuth {
		if accessToken == "" {
			err = fmt.Errorf("tool requires client authorization but access token is missing from the request header")
			s.logger.DebugContext(ctx, err.Error())
			_ = render.Render(w, r, newErrResponse(err, http.StatusUnauthorized))
			return
		}
	}

	// Tool authentication
	// claimsFromAuth maps the name of the authservice to the claims retrieved from it.
	claimsFromAuth := make(map[string]map[string]any)
	for _, aS := range s.PrimitiveMgr.AuthServices() {
		var claims map[string]any
		var err error

		cfg := aS.ToConfig()
		if genCfg, ok := cfg.(generic.Config); ok && genCfg.McpEnabled {
			claims = util.AuthTokenClaimsFromContext(ctx)
		} else {
			claims, err = aS.GetClaimsFromHeader(ctx, r.Header)
			if err != nil {
				s.logger.DebugContext(ctx, err.Error())
				continue
			}
		}

		if claims == nil {
			// authService not present in header
			continue
		}
		claimsFromAuth[aS.GetName()] = claims
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
		err = fmt.Errorf("tool invocation not authorized. Please make sure you specify correct auth headers")
		s.logger.DebugContext(ctx, err.Error())
		_ = render.Render(w, r, newErrResponse(err, http.StatusUnauthorized))
		return
	}
	s.logger.DebugContext(ctx, "tool invocation authorized")

	limit := s.httpMaxRequestBytes
	r.Body = http.MaxBytesReader(w, r.Body, limit)

	var data map[string]any
	if err = util.DecodeJSON(r.Body, &data); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			err = fmt.Errorf("request body exceeds %d bytes", limit)
			s.logger.DebugContext(ctx, err.Error())
			_ = render.Render(w, r, newErrResponse(err, http.StatusRequestEntityTooLarge))
			return
		}
		render.Status(r, http.StatusBadRequest)
		err = fmt.Errorf("request body was invalid JSON: %w", err)
		s.logger.DebugContext(ctx, err.Error())
		_ = render.Render(w, r, newErrResponse(err, http.StatusBadRequest))
		return
	}

	toolParams, err := tool.GetParameters(src)
	if err != nil {
		err = fmt.Errorf("error getting parameters for tool: %w", err)
		s.logger.DebugContext(ctx, err.Error())
		_ = render.Render(w, r, newErrResponse(err, http.StatusInternalServerError))
		return
	}
	params, err := parameters.ParseParams(toolParams, data, claimsFromAuth)
	if err != nil {
		var clientServerErr *util.ClientServerError

		// Return 401 Authentication errors
		if errors.As(err, &clientServerErr) && clientServerErr.Code == http.StatusUnauthorized {
			s.logger.DebugContext(ctx, fmt.Sprintf("auth error: %v", err))
			_ = render.Render(w, r, newErrResponse(err, http.StatusUnauthorized))
			return
		}

		var agentErr *util.AgentError
		if errors.As(err, &agentErr) {
			s.logger.DebugContext(ctx, fmt.Sprintf("agent validation error: %v", err))
			errMap := map[string]string{"error": err.Error()}
			errMarshal, _ := json.Marshal(errMap)

			_ = render.Render(w, r, &resultResponse{Result: string(errMarshal)})
			return
		}

		// Return 500 if it's a specific ClientServerError that isn't a 401, or any other unexpected error
		s.logger.ErrorContext(ctx, fmt.Sprintf("internal server error: %v", err))
		_ = render.Render(w, r, newErrResponse(err, http.StatusInternalServerError))
		return
	}
	s.logger.DebugContext(ctx, fmt.Sprintf("invocation params: %s", params))

	params, err = tool.EmbedParams(ctx, params, s.PrimitiveMgr)
	if err != nil {
		err = fmt.Errorf("error embedding parameters: %w", err)
		s.logger.DebugContext(ctx, err.Error())
		_ = render.Render(w, r, newErrResponse(err, http.StatusBadRequest))
		return
	}

	res, err := tool.Invoke(ctx, src, params, accessToken)

	// Determine what error to return to the users.
	if err != nil {
		var tbErr util.ToolboxError

		if errors.As(err, &tbErr) {
			switch tbErr.Category() {
			case util.CategoryAgent:
				// Agent Errors -> 200 OK
				s.logger.DebugContext(ctx, fmt.Sprintf("Tool invocation agent error: %v", err))
				res = map[string]string{
					"error": err.Error(),
				}

			case util.CategoryServer:
				// Server Errors -> Check the specific code inside
				var clientServerErr *util.ClientServerError
				statusCode := http.StatusInternalServerError // Default to 500

				if errors.As(err, &clientServerErr) {
					if clientServerErr.Code != 0 {
						statusCode = clientServerErr.Code
					}
				}

				// Process auth error
				if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
					if clientAuth {
						// Token error, pass through 401/403
						s.logger.DebugContext(ctx, fmt.Sprintf("Client credentials lack authorization: %v", err))
						_ = render.Render(w, r, newErrResponse(err, statusCode))
						return
					}
					// ADC/Config error, return 500
					statusCode = http.StatusInternalServerError
				}

				s.logger.ErrorContext(ctx, fmt.Sprintf("Tool invocation server error: %v", err))
				_ = render.Render(w, r, newErrResponse(err, statusCode))
				return
			}
		} else {
			// Unknown error -> 500
			s.logger.ErrorContext(ctx, fmt.Sprintf("Tool invocation unknown error: %v", err))
			_ = render.Render(w, r, newErrResponse(err, http.StatusInternalServerError))
			return
		}
	}

	res, truncation := tools.CapResult(
		res,
		tools.ResolveCap(tool.GetMaxRows(), s.maxRows),
		tools.ResolveCap(tool.GetMaxResponseBytes(), s.maxResponseBytes),
	)

	resMarshal, err := json.Marshal(res)
	if err != nil {
		err = fmt.Errorf("unable to marshal result: %w", err)
		s.logger.DebugContext(ctx, err.Error())
		_ = render.Render(w, r, newErrResponse(err, http.StatusInternalServerError))
		return
	}

	_ = render.Render(w, r, &resultResponse{Result: string(resMarshal), Truncation: truncation})
}

var _ render.Renderer = &resultResponse{} // Renderer interface for managing response payloads.

// resultResponse is the response sent back when the tool was invocated successfully.
type resultResponse struct {
	Result string `json:"result"` // result of tool invocation
	// Truncation is set only when the tool's declared result caps
	// (maxRows / maxResponseBytes) dropped part of the result.
	Truncation *tools.Truncation `json:"truncation,omitempty"`
}

// Render renders a single payload and respond to the client request.
func (rr resultResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, http.StatusOK)
	return nil
}

var _ render.Renderer = &errResponse{} // Renderer interface for managing response payloads.

// newErrResponse is a helper function initializing an ErrResponse
func newErrResponse(err error, code int) *errResponse {
	return &errResponse{
		Err:            err,
		HTTPStatusCode: code,

		StatusText: http.StatusText(code),
		ErrorText:  err.Error(),
	}
}

// errResponse is the response sent back when an error has been encountered.
type errResponse struct {
	Err            error `json:"-"` // low-level runtime error
	HTTPStatusCode int   `json:"-"` // http response status code

	StatusText string `json:"status"`          // user-level status message
	ErrorText  string `json:"error,omitempty"` // application-level error message, for debugging
}

func (e *errResponse) Render(w http.ResponseWriter, r *http.Request) error {
	render.Status(r, e.HTTPStatusCode)
	return nil
}
