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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/telemetry"
	"github.com/googleapis/mcp-toolbox/internal/tools"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

var (
	_ tools.Tool     = testutils.MockTool{}
	_ prompts.Prompt = testutils.MockPrompt{}
)

// setUpServer create a new server with tools, prompts, resources and groups.
func setUpServer(t *testing.T, router string, tools map[string]tools.Tool, prompts map[string]prompts.Prompt, resourcesMap map[string]resources.Resource, resourceTemplatesMap map[string]resources.ResourceTemplate, groups map[string]group.Group, opts ...func(*Server)) (chi.Router, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}

	otelShutdown, err := telemetry.SetupOTel(ctx, testutils.MockVersionString, "", false, "", "toolbox")
	if err != nil {
		t.Fatalf("unable to setup otel: %s", err)
	}

	instrumentation, err := telemetry.CreateTelemetryInstrumentation(testutils.MockVersionString)
	if err != nil {
		t.Fatalf("unable to create custom metrics: %s", err)
	}

	sseManager := newSseManager(ctx)

	primitiveManager := primitives.NewPrimitiveManager(nil, nil, nil, tools, prompts, resourcesMap, resourceTemplatesMap, groups)

	mcp.InitializeProtocols(mcp.ProtocolOptions{})

	server := Server{
		version:         testutils.MockVersionString,
		logger:          testLogger,
		instrumentation: instrumentation,
		sseManager:      sseManager,
		PrimitiveMgr:    primitiveManager,
	}
	for _, opt := range opts {
		opt(&server)
	}
	if server.httpMaxRequestBytes == 0 {
		server.httpMaxRequestBytes = DefaultHTTPMaxRequestBytes
	}

	var r chi.Router
	switch router {
	case "api":
		r, err = apiRouter(&server)
		if err != nil {
			t.Fatalf("unable to initialize api router: %s", err)
		}
	case "mcp":
		r, err = mcpRouter(&server)
		if err != nil {
			t.Fatalf("unable to initialize mcp router: %s", err)
		}
	default:
		t.Fatalf("unknown router")
	}
	shutdown := func() {
		// cancel context
		cancel()
		// shutdown otel
		err := otelShutdown(ctx)
		if err != nil {
			t.Fatalf("error shutting down OpenTelemetry: %s", err)
		}
	}

	return r, shutdown
}

func runServer(r chi.Router, tls bool) *httptest.Server {
	var ts *httptest.Server
	if tls {
		ts = httptest.NewTLSServer(r)
	} else {
		ts = httptest.NewServer(r)
	}
	return ts
}

func runRequest(ts *httptest.Server, method, path string, body io.Reader, header map[string]string) (*http.Response, []byte, error) {
	req, err := http.NewRequest(method, ts.URL+path, body)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to send request: %w", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to read request body: %w", err)
	}
	defer resp.Body.Close()

	return resp, respBody, nil
}

func withHTTPMaxRequestBytes(limit int64) func(*Server) {
	return func(s *Server) {
		s.httpMaxRequestBytes = limit
	}
}

func withEnableDraftSpecs() func(*Server) {
	return func(s *Server) {
		s.enableDraftSpecs = true
	}
}
