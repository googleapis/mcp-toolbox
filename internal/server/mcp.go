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
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/auth"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	mcputil "github.com/googleapis/mcp-toolbox/internal/server/mcp/util"
	v20241105 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20241105"
	v20250326 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20250326"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type sseSession struct {
	writer     http.ResponseWriter
	flusher    http.Flusher
	done       chan struct{}
	eventQueue chan string
	lastActive time.Time
}

// sseManager manages and control access to sse sessions
type sseManager struct {
	mu          sync.Mutex
	sseSessions map[string]*sseSession
}

func (m *sseManager) get(id string) (*sseSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sseSessions[id]
	if !ok || session == nil {
		// Be defensive: a nil session entry should be treated as unavailable.
		if ok && session == nil {
			delete(m.sseSessions, id)
		}
		return nil, false
	}
	session.lastActive = time.Now()
	return session, true
}

func newSseManager(ctx context.Context) *sseManager {
	sseM := &sseManager{
		mu:          sync.Mutex{},
		sseSessions: make(map[string]*sseSession),
	}
	go sseM.cleanupRoutine(ctx)
	return sseM
}

func (m *sseManager) add(id string, session *sseSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sseSessions[id] = session
	session.lastActive = time.Now()
}

func (m *sseManager) remove(id string) {
	m.mu.Lock()
	delete(m.sseSessions, id)
	m.mu.Unlock()
}

func (m *sseManager) cleanupRoutine(ctx context.Context) {
	timeout := 10 * time.Minute
	ticker := time.NewTicker(timeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				m.mu.Lock()
				defer m.mu.Unlock()
				now := time.Now()
				for id, sess := range m.sseSessions {
					if now.Sub(sess.lastActive) > timeout {
						delete(m.sseSessions, id)
					}
				}
			}()
		}
	}
}

type stdioSession struct {
	protocol string
	server   *Server
	reader   *bufio.Reader
	writer   io.Writer
}

// traceContextCarrier implements propagation.TextMapCarrier for extracting trace context from _meta
type traceContextCarrier map[string]string

func (c traceContextCarrier) Get(key string) string {
	return c[key]
}

func (c traceContextCarrier) Set(key, value string) {
	c[key] = value
}

func (c traceContextCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// extractMeta parses params._meta from the request body in a single pass,
// extracting both W3C Trace Context and client telemetry attributes.
func extractMeta(ctx context.Context, body []byte) (string, context.Context) {
	var req struct {
		Params struct {
			Meta struct {
				Traceparent     string            `json:"traceparent,omitempty"`
				Tracestate      string            `json:"tracestate,omitempty"`
				TelemetryAttrs  map[string]string `json:"dev.mcp-toolbox/telemetry,omitempty"`
				ProtocolVersion string            `json:"io.modelcontextprotocol/protocolVersion"`
			} `json:"_meta,omitempty"`
		} `json:"params,omitempty"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return "", ctx
	}

	// Extract W3C Trace Context
	if req.Params.Meta.Traceparent != "" {
		carrier := traceContextCarrier{
			"traceparent": req.Params.Meta.Traceparent,
		}
		if req.Params.Meta.Tracestate != "" {
			carrier["tracestate"] = req.Params.Meta.Tracestate
		}
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
	}

	// Extract client telemetry attributes
	if attrs := req.Params.Meta.TelemetryAttrs; len(attrs) > 0 {
		ta := &util.TelemetryAttributes{
			ClientName:    attrs["client.name"],
			ClientVersion: attrs["client.version"],
			ClientModel:   attrs["client.model"],
			ClientUserID:  attrs["client.user.id"],
			ClientAgentID: attrs["client.agent.id"],
		}
		ctx = util.WithTelemetryAttributes(ctx, ta)
	}

	return req.Params.Meta.ProtocolVersion, ctx
}

func NewStdioSession(s *Server, stdin io.Reader, stdout io.Writer) *stdioSession {
	stdioSession := &stdioSession{
		server: s,
		reader: bufio.NewReader(stdin),
		writer: stdout,
	}
	return stdioSession
}

func (s *stdioSession) Start(ctx context.Context) error {
	return s.readInputStream(ctx)
}

// readInputStream reads requests/notifications from MCP clients through stdin
func (s *stdioSession) readInputStream(ctx context.Context) error {
	sessionStart := time.Now()
	ctx = util.WithUserAgent(ctx, s.server.version)
	ctx = util.WithSQLCommenterEnabled(ctx, s.server.sqlCommenterEnabled)

	// Define attributes for session metrics
	// Note: mcp.protocol.version is added dynamically after protocol negotiation
	sessionAttrs := []attribute.KeyValue{
		attribute.String("network.transport", "pipe"),
		attribute.String("network.protocol.name", "stdio"),
	}

	s.server.instrumentation.McpActiveSessions.Add(ctx, 1, metric.WithAttributes(sessionAttrs...))

	var err error
	defer func() {
		// Build full attributes including mcp.protocol.version if negotiated
		fullAttrs := sessionAttrs
		if s.protocol != "" {
			fullAttrs = append(fullAttrs, attribute.String("mcp.protocol.version", s.protocol))
		}

		// Decrement active sessions counter
		s.server.instrumentation.McpActiveSessions.Add(ctx, -1, metric.WithAttributes(fullAttrs...))

		// Record session duration
		sessionDuration := time.Since(sessionStart).Seconds()
		durationAttrs := make([]attribute.KeyValue, len(fullAttrs))
		copy(durationAttrs, fullAttrs)
		if err != nil && err != io.EOF {
			durationAttrs = append(durationAttrs, attribute.String("error.type", err.Error()))
		}
		s.server.instrumentation.McpSessionDuration.Record(ctx, sessionDuration, metric.WithAttributes(durationAttrs...))
	}()

	for {
		if err = ctx.Err(); err != nil {
			return err
		}

		var line string
		line, err = s.readLine(ctx)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if err := func() error {
			// This ensures the transport span becomes a child of the client span
			metaProtocolVersion, msgCtx := extractMeta(ctx, []byte(line))

			// Create span for STDIO transport
			msgCtx, span := s.server.instrumentation.Tracer.Start(msgCtx, "toolbox/server/mcp/stdio",
				trace.WithSpanKind(trace.SpanKindServer),
			)
			defer span.End()

			protocol := s.protocol
			// if protocol version was found in meta, it takes precedence
			// the metaProtocolVersion does not replace existing protocol
			// version if initialize method took place
			if protocol == "" && metaProtocolVersion != "" {
				protocol = metaProtocolVersion
			}

			var v string
			var res any
			v, res, err = processMcpMessage(msgCtx, []byte(line), s.server, protocol, "", nil, "")
			if err != nil {
				// errors during the processing of message will generate a valid MCP Error response.
				// server can continue to run.
				s.server.logger.ErrorContext(msgCtx, err.Error())
				span.SetStatus(codes.Error, err.Error())
			}

			if v != "" {
				s.protocol = v
			}
			// no responses for notifications
			if res != nil {
				if err = s.write(msgCtx, res); err != nil {
					return err
				}
			}
			return nil
		}(); err != nil {
			return err
		}
	}
}

// readLine process each line within the input stream.
func (s *stdioSession) readLine(ctx context.Context) (string, error) {
	readChan := make(chan string, 1)
	errChan := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)
	defer close(readChan)
	defer close(errChan)

	go func() {
		select {
		case <-done:
			return
		default:
			line, err := s.reader.ReadString('\n')
			if err != nil {
				select {
				case errChan <- err:
				case <-done:
				}
				return
			}
			select {
			case readChan <- line:
			case <-done:
			}
			return
		}
	}()

	select {
	// if context is cancelled, return an empty string
	case <-ctx.Done():
		return "", ctx.Err()
	// return error if error is found
	case err := <-errChan:
		return "", err
	// return line if successful
	case line := <-readChan:
		return line, nil
	}
}

// write writes to stdout with response to client
func (s *stdioSession) write(_ context.Context, response any) error {
	res, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response to JSON: %w", err)
	}

	_, err = fmt.Fprintf(s.writer, "%s\n", res)
	return err
}

// mcpRouter creates a router that represents the routes under /mcp
func mcpRouter(s *Server) (chi.Router, error) {
	r := chi.NewRouter()

	r.Use(middleware.AllowContentType("application/json", "application/json-rpc", "application/jsonrequest"))
	r.Use(middleware.StripSlashes)
	r.Use(render.SetContentType(render.ContentTypeJSON))
	// Inject logger into ctx
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := util.WithLogger(r.Context(), s.logger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Use(mcpAuthMiddleware(s))

	r.Get("/sse", func(w http.ResponseWriter, r *http.Request) { sseHandler(s, w, r) })
	r.Get("/", func(w http.ResponseWriter, r *http.Request) { methodNotAllowed(s, w, r) })
	r.Post("/", func(w http.ResponseWriter, r *http.Request) { httpHandler(s, w, r) })
	r.Delete("/", func(w http.ResponseWriter, r *http.Request) {})

	r.Route("/{toolsetName}", func(r chi.Router) {
		r.Get("/sse", func(w http.ResponseWriter, r *http.Request) { sseHandler(s, w, r) })
		r.Get("/", func(w http.ResponseWriter, r *http.Request) { methodNotAllowed(s, w, r) })
		r.Post("/", func(w http.ResponseWriter, r *http.Request) { httpHandler(s, w, r) })
		r.Delete("/", func(w http.ResponseWriter, r *http.Request) {})
	})

	return r, nil
}

// sseHandler handles sse initialization and message.
func sseHandler(s *Server, w http.ResponseWriter, r *http.Request) {
	sessionStart := time.Now()
	ctx, span := s.instrumentation.Tracer.Start(r.Context(), "toolbox/server/mcp/sse",
		trace.WithSpanKind(trace.SpanKindServer),
	)
	r = r.WithContext(ctx)

	sessionId := uuid.New().String()
	groupName := chi.URLParam(r, "toolsetName")
	s.logger.DebugContext(ctx, fmt.Sprintf("toolset name: %s", groupName))
	span.SetAttributes(attribute.String("mcp.session.id", sessionId))
	span.SetAttributes(attribute.String("toolset.name", groupName))

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Define attributes for session metrics
	networkProtocolVersion := fmt.Sprintf("%d.%d", r.ProtoMajor, r.ProtoMinor)
	sessionAttrs := []attribute.KeyValue{
		attribute.String("network.transport", "tcp"),
		attribute.String("network.protocol.name", "http"),
		attribute.String("network.protocol.version", networkProtocolVersion),
		attribute.String("mcp.protocol.version", "2024-11-05"),
		attribute.String("toolset.name", groupName),
	}

	// Increment active sessions counter
	s.instrumentation.McpActiveSessions.Add(ctx, 1, metric.WithAttributes(sessionAttrs...))

	var err error
	defer func() {
		// Decrement active sessions counter
		s.instrumentation.McpActiveSessions.Add(ctx, -1, metric.WithAttributes(sessionAttrs...))

		// Record session duration
		sessionDuration := time.Since(sessionStart).Seconds()
		durationAttrs := make([]attribute.KeyValue, len(sessionAttrs))
		copy(durationAttrs, sessionAttrs)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			durationAttrs = append(durationAttrs, attribute.String("error.type", err.Error()))
		}
		s.instrumentation.McpSessionDuration.Record(ctx, sessionDuration, metric.WithAttributes(durationAttrs...))
		span.End()
	}()

	flusher, ok := w.(http.Flusher)
	if !ok {
		err = fmt.Errorf("unable to retrieve flusher for sse")
		s.logger.DebugContext(ctx, err.Error())
		_ = render.Render(w, r, newErrResponse(err, http.StatusInternalServerError))
		return
	}
	session := &sseSession{
		writer:     w,
		flusher:    flusher,
		done:       make(chan struct{}),
		eventQueue: make(chan string, 100),
	}
	s.sseManager.add(sessionId, session)
	defer s.sseManager.remove(sessionId)

	// https scheme formatting if (forwarded) request is a TLS request
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if r.TLS == nil {
			proto = "http"
		} else {
			proto = "https"
		}
	}

	// send initial endpoint event
	toolsetURL := ""
	if groupName != "" {
		toolsetURL = fmt.Sprintf("/%s", groupName)
	}
	// attach url query params to message endpoint
	q := r.URL.Query()
	q.Set("sessionId", sessionId)
	messageEndpoint := fmt.Sprintf("%s://%s/mcp%s?%s", proto, r.Host, toolsetURL, q.Encode())
	s.logger.DebugContext(ctx, fmt.Sprintf("sending endpoint event: %s", messageEndpoint))
	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", messageEndpoint)
	flusher.Flush()

	clientClose := r.Context().Done()
	for {
		select {
		// Ensure that only a single responses are written at once
		case event := <-session.eventQueue:
			fmt.Fprint(w, event)
			s.logger.DebugContext(ctx, fmt.Sprintf("sending event: %s", event))
			flusher.Flush()
			// channel for client disconnection
		case <-clientClose:
			close(session.done)
			s.logger.DebugContext(ctx, "client disconnected")
			return
		}
	}
}

// methodNotAllowed handles all mcp messages.
func methodNotAllowed(s *Server, w http.ResponseWriter, r *http.Request) {
	err := fmt.Errorf("toolbox does not support streaming in streamable HTTP transport")
	s.logger.DebugContext(r.Context(), err.Error())
	_ = render.Render(w, r, newErrResponse(err, http.StatusMethodNotAllowed))
}

// httpHandler handles all mcp messages.
func httpHandler(s *Server, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx := r.Context()
	ctx = util.WithLogger(ctx, s.logger)
	ctx = util.WithUserAgent(ctx, s.version)
	ctx = util.WithSQLCommenterEnabled(ctx, s.sqlCommenterEnabled)

	queryParams := r.URL.Query()
	urlParams := make(map[string]string)
	for k, v := range queryParams {
		if k == "sessionId" {
			continue
		}
		if len(v) > 0 {
			urlParams[k] = v[0]
		}
	}
	if len(urlParams) > 0 {
		ctx = util.WithUrlParams(ctx, urlParams)
	}

	limit := s.httpMaxRequestBytes
	r.Body = http.MaxBytesReader(w, r.Body, limit)

	// Read body first so we can extract trace context
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// The id cannot be determined from an unreadable body. Per JSON-RPC 2.0,
		// the response id MUST be null in that case.
		// See https://www.jsonrpc.org/specification#response_object
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			err = fmt.Errorf("request body exceeds %d bytes", limit)
		}
		s.logger.DebugContext(ctx, err.Error())
		render.JSON(w, r, jsonrpc.NewError(nil, jsonrpc.PARSE_ERROR, err.Error(), nil))
		return
	}

	// This ensures the transport span becomes a child of the client span
	// _meta.ProtocolVersion is not checked here for http transport.
	_, ctx = extractMeta(ctx, body)

	// Create span for HTTP transport
	ctx, span := s.instrumentation.Tracer.Start(ctx, "toolbox/server/mcp/http",
		trace.WithSpanKind(trace.SpanKindServer),
	)
	r = r.WithContext(ctx)

	var sessionId, protocolVersion string
	var session *sseSession

	// check if client connects via sse
	// v2024-11-05 supports http with sse
	paramSessionId := r.URL.Query().Get("sessionId")
	if paramSessionId != "" {
		sessionId = paramSessionId
		protocolVersion = v20241105.PROTOCOL_VERSION
		var ok bool
		session, ok = s.sseManager.get(sessionId)
		if !ok {
			err := fmt.Errorf("sse session not available")
			s.logger.DebugContext(ctx, err.Error())
			_ = render.Render(w, r, newErrResponse(err, http.StatusBadRequest))
			return
		}
	}

	// check if client have `Mcp-Session-Id` header
	// `Mcp-Session-Id` is only set for v2025-03-26 in Toolbox
	headerSessionId := r.Header.Get("Mcp-Session-Id")
	if headerSessionId != "" {
		protocolVersion = v20250326.PROTOCOL_VERSION
	}

	// check if client have `MCP-Protocol-Version` header
	// Only supported for v2025-06-18+.
	headerProtocolVersion := r.Header.Get("Mcp-Protocol-Version")
	if headerProtocolVersion != "" {
		protocolVersion = headerProtocolVersion
	}

	groupName := chi.URLParam(r, "toolsetName")
	s.logger.DebugContext(ctx, fmt.Sprintf("toolset name: %s", groupName))
	span.SetAttributes(attribute.String("toolset.name", groupName))

	defer func() {
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	networkProtocolVersion := fmt.Sprintf("%d.%d", r.ProtoMajor, r.ProtoMinor)

	v, res, err := processMcpMessage(ctx, body, s, protocolVersion, groupName, r.Header, networkProtocolVersion)
	if err != nil {
		s.logger.DebugContext(ctx, fmt.Errorf("error processing message: %w", err).Error())
	}

	// notifications will return empty string
	if res == nil {
		// Notifications do not expect a response
		// Toolbox doesn't do anything with notifications yet
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// for v20250326, add the `Mcp-Session-Id` header
	if v == v20250326.PROTOCOL_VERSION {
		sessionId = uuid.New().String()
		w.Header().Set("Mcp-Session-Id", sessionId)
	}

	if session != nil {
		// queue sse event
		eventData, _ := json.Marshal(res)
		select {
		case session.eventQueue <- fmt.Sprintf("event: message\ndata: %s\n\n", eventData):
			s.logger.DebugContext(ctx, "event queue successful")
		case <-session.done:
			s.logger.DebugContext(ctx, "session is close")
		default:
			s.logger.DebugContext(ctx, "unable to add to event queue")
		}
	}
	if rpcResponse, ok := res.(jsonrpc.JSONRPCError); ok {
		code := rpcResponse.Error.Code
		switch code {
		case jsonrpc.INTERNAL_ERROR:
			// Map Internal RPC Error (-32603) to HTTP 500
			render.Status(r, http.StatusInternalServerError)
		case jsonrpc.INVALID_REQUEST:
			var clientServerErr *util.ClientServerError
			if errors.As(err, &clientServerErr) {
				render.Status(r, clientServerErr.Code)
			}
			var mcpErr *auth.MCPAuthError
			if errors.As(err, &mcpErr) {
				switch mcpErr.Code {
				case http.StatusForbidden:
					w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="insufficient_scope", scope="%s", resource_metadata="%s", error_description="%s"`, strings.Join(mcpErr.ScopesRequired, " "), s.getPRMURL(), mcpErr.Message))
					render.Status(r, http.StatusForbidden)
				case http.StatusUnauthorized:
					scopesArg := ""
					if len(mcpErr.ScopesRequired) > 0 {
						scopesArg = fmt.Sprintf(`, scope="%s"`, strings.Join(mcpErr.ScopesRequired, " "))
					}
					w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s"%s`, s.getPRMURL(), scopesArg))
					render.Status(r, http.StatusUnauthorized)
				}
			}
		case jsonrpc.METHOD_NOT_FOUND:
			render.Status(r, http.StatusNotFound)
		case jsonrpc.HEADER_MISMATCH, jsonrpc.UNSUPPORTED_PROTOCOL_VERSION:
			render.Status(r, http.StatusBadRequest)
		case jsonrpc.INVALID_PARAMS:
			if strings.Contains(rpcResponse.Error.Message, "_meta error") {
				render.Status(r, http.StatusBadRequest)
			}
		}
	}

	// send HTTP response
	render.JSON(w, r, res)
}

// processMcpMessage process the messages received from clients
func processMcpMessage(ctx context.Context, body []byte, s *Server, protocolVersion string, groupName string, header http.Header, networkProtocolVersion string) (string, any, error) {
	operationStart := time.Now()

	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return "", jsonrpc.NewError("", jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}

	// Generic baseMessage could either be a JSONRPCNotification or JSONRPCRequest
	var baseMessage jsonrpc.BaseMessage
	if err = util.DecodeJSON(bytes.NewBuffer(body), &baseMessage); err != nil {
		// The id cannot be determined from an undecodable body (batch or parse
		// error). Per JSON-RPC 2.0, the response id MUST be null in that case.
		// See https://www.jsonrpc.org/specification#response_object

		// check if user is sending a batch request
		var a []any
		unmarshalErr := json.Unmarshal(body, &a)
		if unmarshalErr == nil {
			err = fmt.Errorf("not supporting batch requests")
			return "", jsonrpc.NewError(nil, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
		}

		return "", jsonrpc.NewError(nil, jsonrpc.PARSE_ERROR, err.Error(), nil), err
	}

	// Check if method is present
	if baseMessage.Method == "" {
		err = fmt.Errorf("method not found")
		return "", jsonrpc.NewError(baseMessage.Id, jsonrpc.METHOD_NOT_FOUND, err.Error(), nil), err
	}
	logger.DebugContext(ctx, fmt.Sprintf("method is: %s", baseMessage.Method))

	// Check for JSON-RPC 2.0
	if baseMessage.Jsonrpc != jsonrpc.JSONRPC_VERSION {
		err = fmt.Errorf("invalid json-rpc version")
		return "", jsonrpc.NewError(baseMessage.Id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
	}

	// Create method-specific span with semantic conventions
	// Note: Trace context is already extracted and set in ctx by the caller
	ctx, span := s.instrumentation.Tracer.Start(ctx, baseMessage.Method,
		trace.WithSpanKind(trace.SpanKindServer),
	)
	defer span.End()

	// Determine network transport and protocol based on header presence
	networkTransport := "pipe" // default for stdio
	networkProtocolName := "stdio"
	if header != nil {
		networkTransport = "tcp" // HTTP/SSE transport
		networkProtocolName = "http"
	}

	var metricErrorType string
	genAIAttrs := &util.GenAIMetricAttrs{
		NetworkProtocolName:    networkProtocolName,
		NetworkProtocolVersion: networkProtocolVersion,
	}
	ctx = util.WithGenAIMetricAttrs(ctx, genAIAttrs)

	// Record operation duration metric on function exit
	defer func() {
		operationDuration := time.Since(operationStart).Seconds()
		durationAttrs := []attribute.KeyValue{
			attribute.String("mcp.method.name", baseMessage.Method),
			attribute.String("network.transport", networkTransport),
			attribute.String("network.protocol.name", networkProtocolName),
			attribute.String("toolset.name", groupName),
		}
		if protocolVersion != "" {
			durationAttrs = append(durationAttrs, attribute.String("mcp.protocol.version", protocolVersion))
		}
		if networkProtocolVersion != "" {
			durationAttrs = append(durationAttrs, attribute.String("network.protocol.version", networkProtocolVersion))
		}
		// Add gen_ai attributes populated by method handlers
		if genAIAttrs.OperationName != "" {
			durationAttrs = append(durationAttrs, attribute.String("gen_ai.operation.name", genAIAttrs.OperationName))
		}
		if genAIAttrs.ToolName != "" {
			durationAttrs = append(durationAttrs, attribute.String("gen_ai.tool.name", genAIAttrs.ToolName))
		}
		if genAIAttrs.PromptName != "" {
			durationAttrs = append(durationAttrs, attribute.String("gen_ai.prompt.name", genAIAttrs.PromptName))
		}
		if genAIAttrs.GroupName != "" {
			durationAttrs = append(durationAttrs, attribute.String("gen_ai.group.name", genAIAttrs.GroupName))
		}
		if metricErrorType != "" {
			durationAttrs = append(durationAttrs, attribute.String("error.type", metricErrorType))
		}
		s.instrumentation.McpOperationDuration.Record(ctx, operationDuration, metric.WithAttributes(durationAttrs...))
	}()

	// Set required semantic attributes for span according to OTEL MCP semcov
	// ref: https://opentelemetry.io/docs/specs/semconv/gen-ai/mcp/#server
	span.SetAttributes(
		attribute.String("mcp.method.name", baseMessage.Method),
		attribute.String("network.transport", networkTransport),
		attribute.String("network.protocol.name", networkProtocolName),
	)

	// Set client telemetry attributes from _meta["dev.mcp-toolbox/telemetry"]
	if ta := util.TelemetryAttributesFromContext(ctx); ta != nil {
		telemetryAttrs := make([]attribute.KeyValue, 0, 5)
		if ta.ClientName != "" {
			telemetryAttrs = append(telemetryAttrs, attribute.String("client.name", ta.ClientName))
		}
		if ta.ClientVersion != "" {
			telemetryAttrs = append(telemetryAttrs, attribute.String("client.version", ta.ClientVersion))
		}
		if ta.ClientModel != "" {
			telemetryAttrs = append(telemetryAttrs, attribute.String("client.model", ta.ClientModel))
		}
		if ta.ClientUserID != "" {
			telemetryAttrs = append(telemetryAttrs, attribute.String("client.user.id", ta.ClientUserID))
		}
		if ta.ClientAgentID != "" {
			telemetryAttrs = append(telemetryAttrs, attribute.String("client.agent.id", ta.ClientAgentID))
		}
		if len(telemetryAttrs) > 0 {
			span.SetAttributes(telemetryAttrs...)
		}
	}

	// Set network protocol version if available
	if networkProtocolVersion != "" {
		span.SetAttributes(attribute.String("network.protocol.version", networkProtocolVersion))
	}

	// Set MCP protocol version if available
	if protocolVersion != "" {
		span.SetAttributes(attribute.String("mcp.protocol.version", protocolVersion))
	}

	// Set request ID
	if baseMessage.Id != nil {
		span.SetAttributes(attribute.String("jsonrpc.request.id", fmt.Sprintf("%v", baseMessage.Id)))
	}

	// Set toolset name
	span.SetAttributes(attribute.String("toolset.name", groupName))

	// Check if message is a notification
	if baseMessage.Id == nil {
		err := mcp.NotificationHandler(ctx, body)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
		}
		return "", nil, err
	}

	// Add instrumentation and toolbox version to context for use in method handlers
	ctx = util.WithInstrumentation(ctx, s.instrumentation)
	ctx = util.WithToolboxVersionKey(ctx, s.version)
	ctx = util.WithEnableDraftSpecs(ctx, s.enableDraftSpecs)
	// Process the method
	switch baseMessage.Method {
	// This is only used for <v2026
	case "initialize":
		var initReq struct {
			Params struct {
				ProtocolVersion string `json:"protocolVersion"`
			} `json:"params,omitempty"`
		}
		if err := json.Unmarshal(body, &initReq); err != nil {
			err = fmt.Errorf("fail to parse protocolVersion from initialize request")
			return "", jsonrpc.NewError(baseMessage.Id, jsonrpc.INVALID_REQUEST, err.Error(), nil), err
		}

		var version string
		v := initReq.Params.ProtocolVersion
		if slices.Contains(mcputil.GetSupportedVersions(s.enableDraftSpecs), v) {
			version = v
		} else {
			version = mcputil.GetLatestSupportedVersion(s.enableDraftSpecs)
		}

		result, err := mcp.ProcessMethod(ctx, version, baseMessage.Id, baseMessage.Method, group.Group{}, nil, body, nil)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			if rpcErr, ok := result.(jsonrpc.JSONRPCError); ok {
				metricErrorType = rpcErr.Error.String()
				span.SetAttributes(attribute.String("error.type", metricErrorType))
			}
			return "", result, err
		}
		span.SetAttributes(attribute.String("mcp.protocol.version", version))
		return version, result, err
	default:
		// The URL segment names a group; its derived toolset and promptset views
		// share that name, so prompts scope to the connected group.
		g, ok := s.PrimitiveMgr.GetGroup(groupName)
		if !ok {
			err := fmt.Errorf("toolset does not exist")
			rpcErr := jsonrpc.NewError(baseMessage.Id, jsonrpc.INVALID_REQUEST, err.Error(), nil)
			metricErrorType = rpcErr.Error.String()
			span.SetStatus(codes.Error, err.Error())
			span.SetAttributes(attribute.String("error.type", metricErrorType))
			return "", rpcErr, err
		}
		result, err := mcp.ProcessMethod(ctx, protocolVersion, baseMessage.Id, baseMessage.Method, g, s.PrimitiveMgr, body, header)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			// Set error.type based on JSON-RPC error code
			if rpcErr, ok := result.(jsonrpc.JSONRPCError); ok {
				metricErrorType = rpcErr.Error.String()
				span.SetAttributes(attribute.Int("jsonrpc.error.code", rpcErr.Error.Code))
				span.SetAttributes(attribute.String("error.type", metricErrorType))
			}
		}
		return "", result, err
	}
}

type prmResponse struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
}

// prmHandler generates the Protected Resource Metadata (PRM) file for MCP Authorization.
func prmHandler(s *Server, w http.ResponseWriter, r *http.Request) {
	var server string
	scopes := []string{}
	for _, authSvc := range s.PrimitiveMgr.AuthServices() {
		if mSvc, ok := authSvc.(auth.MCPAuthService); ok && mSvc.IsMCPEnabled() {
			server = mSvc.GetAuthorizationServer()
			scopes = mSvc.GetScopesRequired()
			break
		}
	}

	res := prmResponse{
		Resource:               s.toolboxUrl,
		AuthorizationServers:   []string{server},
		ScopesSupported:        scopes,
		BearerMethodsSupported: []string{"header"},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		s.logger.ErrorContext(r.Context(), fmt.Sprintf("Failed to encode PRM response: %v", err))
		http.Error(w, "Failed to encode PRM response", http.StatusInternalServerError)
	}
}
