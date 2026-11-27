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

package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/telemetry"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newTelemetryTestServer builds a server whose instrumentation is bound to
// in-memory OTel test doubles, and returns the router plus the collectors.
// The doubles must be installed as the global providers BEFORE
// CreateTelemetryInstrumentation runs: otel.Tracer/otel.Meter bind to the
// provider that is current at creation time.
func newTelemetryTestServer(t *testing.T, routerKind string,
	toolsMap map[string]tools.Tool,
	promptsMap map[string]prompts.Prompt,
	resourcesMap map[string]resources.Resource,
	resourceTemplatesMap map[string]resources.ResourceTemplate,
	groups map[string]group.Group,
) (chi.Router, *tracetest.SpanRecorder, *sdkmetric.ManualReader) {
	t.Helper()
	ctx := context.Background()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)

	mr := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	prevMP := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)

	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetMeterProvider(prevMP)
		_ = tp.Shutdown(context.Background())
		_ = mp.Shutdown(context.Background())
	})

	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unable to initialize logger: %s", err)
	}
	instrumentation, err := telemetry.CreateTelemetryInstrumentation(testutils.MockVersionString)
	if err != nil {
		t.Fatalf("unable to create custom metrics: %s", err)
	}
	primitiveManager := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups)
	mcp.InitializeProtocols(mcp.ProtocolOptions{})
	server := Server{
		version:             testutils.MockVersionString,
		logger:              testLogger,
		instrumentation:     instrumentation,
		sseManager:          newSseManager(ctx),
		PrimitiveMgr:        primitiveManager,
		httpMaxRequestBytes: DefaultHTTPMaxRequestBytes,
	}

	var r chi.Router
	switch routerKind {
	case "api":
		r, err = apiRouter(&server)
	case "mcp":
		r, err = mcpRouter(&server)
	default:
		t.Fatalf("unknown router %q", routerKind)
	}
	if err != nil {
		t.Fatalf("unable to initialize %s router: %s", routerKind, err)
	}
	return r, sr, mr
}

// metricAttributeSets returns the attribute sets of every recorded data point
// across the aggregation types the server instruments use.
func metricAttributeSets(m metricdata.Metrics) []attribute.Set {
	var out []attribute.Set
	switch d := m.Data.(type) {
	case metricdata.Histogram[float64]:
		for _, dp := range d.DataPoints {
			out = append(out, dp.Attributes)
		}
	case metricdata.Sum[int64]:
		for _, dp := range d.DataPoints {
			out = append(out, dp.Attributes)
		}
	case metricdata.Sum[float64]:
		for _, dp := range d.DataPoints {
			out = append(out, dp.Attributes)
		}
	case metricdata.Gauge[float64]:
		for _, dp := range d.DataPoints {
			out = append(out, dp.Attributes)
		}
	}
	return out
}

// requireNoRawToolsetName fails if any recorded span or metric stream carries
// toolset.name == raw. Regression assertion for #4273: an unvalidated URL
// path segment must never mint new telemetry streams.
func requireNoRawToolsetName(t *testing.T, sr *tracetest.SpanRecorder, mr *sdkmetric.ManualReader, raw string) {
	t.Helper()
	for _, s := range sr.Ended() {
		for _, a := range s.Attributes() {
			if a.Key == "toolset.name" && a.Value.AsString() == raw {
				t.Errorf("span %q carries unvalidated toolset.name %q", s.Name(), raw)
			}
		}
	}
	var rm metricdata.ResourceMetrics
	if err := mr.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			for _, set := range metricAttributeSets(m) {
				if v, ok := set.Value(attribute.Key("toolset.name")); ok && v.AsString() == raw {
					t.Errorf("metric %q stream carries unvalidated toolset.name %q", m.Name, raw)
				}
			}
		}
	}
}

// requireToolsetNamePresent fails unless some recorded span or metric stream
// carries toolset.name == name. Positive control: valid toolset names must
// keep their telemetry attribution (the fix must not drop the attribute).
func requireToolsetNamePresent(t *testing.T, sr *tracetest.SpanRecorder, mr *sdkmetric.ManualReader, name string) {
	t.Helper()
	for _, s := range sr.Ended() {
		for _, a := range s.Attributes() {
			if a.Key == "toolset.name" && a.Value.AsString() == name {
				return
			}
		}
	}
	var rm metricdata.ResourceMetrics
	if err := mr.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collecting metrics: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			for _, set := range metricAttributeSets(m) {
				if v, ok := set.Value(attribute.Key("toolset.name")); ok && v.AsString() == name {
					return
				}
			}
		}
	}
	t.Errorf("no span or metric stream carries toolset.name %q; valid toolsets must keep attribution", name)
}

func TestTelemetryToolsetName(t *testing.T) {
	_, _, _, _, groups := testutils.SetUpPrimitives(t,
		[]testutils.MockTool{testutils.MockTool1, testutils.MockTool2}, nil, nil, nil)
	s := &Server{PrimitiveMgr: primitives.NewPrimitiveManager(nil, nil, nil, nil, nil, nil, nil, groups)}

	if got := s.telemetryToolsetName("tool1_only"); got != "tool1_only" {
		t.Errorf("telemetryToolsetName(tool1_only) = %q, want passthrough", got)
	}
	if got := s.telemetryToolsetName("no-such-toolset"); got != "" {
		t.Errorf("telemetryToolsetName(no-such-toolset) = %q, want empty", got)
	}
	if got := s.telemetryToolsetName(""); got != "" {
		t.Errorf("telemetryToolsetName(\"\") = %q, want empty", got)
	}
}

// waitForSpan polls until a span with the given name is ended or the timeout
// elapses. Needed for handlers that record telemetry from another goroutine.
func waitForSpan(t *testing.T, sr *tracetest.SpanRecorder, name string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, s := range sr.Ended() {
			if s.Name() == name {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for span %q", name)
}

func postJSONRPC(t *testing.T, url, method string) (int, string) {
	t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":"test-1","method":%q}`, method)
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// TestUnvalidatedToolsetNameTelemetry is the end-to-end regression test for
// #4273: requests to /{bogus}/ and /{bogus}/sse on the MCP router must not
// attach the bogus path segment to any span or metric stream, while
// configured toolsets keep their attribution. (The router is served bare here,
// so production's /mcp prefix is absent.)
func TestUnvalidatedToolsetNameTelemetry(t *testing.T) {
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2}
	toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups := testutils.SetUpPrimitives(t, mockTools, nil, nil, nil)
	r, sr, mr := newTelemetryTestServer(t, "mcp", toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groups)
	ts := runServer(r, false)
	defer ts.Close()

	bogus1 := "no-such-toolset-aaa"
	bogus2 := "no-such-toolset-bbb"

	// Bogus toolsets must still fail closed at the routing layer.
	for _, bogus := range []string{bogus1, bogus2} {
		_, body := postJSONRPC(t, ts.URL+"/"+bogus+"/", "tools/list")
		if !strings.Contains(body, "toolset does not exist") {
			t.Errorf("POST /%s/: expected 'toolset does not exist', got %s", bogus, body)
		}
	}

	// A configured toolset keeps working.
	if _, body := postJSONRPC(t, ts.URL+"/tool1_only/", "tools/list"); strings.Contains(body, "toolset does not exist") {
		t.Fatalf("POST /tool1_only/ unexpectedly failed: %s", body)
	}

	// SSE handshake against a bogus toolset: the handler blocks until the
	// client goes away, then records session telemetry exactly once.
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/"+bogus1+"/sse", nil)
	if err != nil {
		t.Fatalf("building SSE request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /%s/sse: %v", bogus1, err)
	}
	resp.Body.Close()
	cancel()
	waitForSpan(t, sr, "toolbox/server/mcp/sse")

	// Neither bogus name may appear on any span or metric stream, even though
	// two distinct bogus names were used (each would previously mint its own
	// cumulative stream).
	requireNoRawToolsetName(t, sr, mr, bogus1)
	requireNoRawToolsetName(t, sr, mr, bogus2)
	// Positive control: the valid toolset is still attributed.
	requireToolsetNamePresent(t, sr, mr, "tool1_only")
}

// TestToolsetHandlerTelemetryValidation covers the API router's
// /toolset/{toolsetName} endpoint for #4273.
func TestToolsetHandlerTelemetryValidation(t *testing.T) {
	mockTools := []testutils.MockTool{testutils.MockTool1, testutils.MockTool2}
	toolsMap, _, _, _, groups := testutils.SetUpPrimitives(t, mockTools, nil, nil, nil)
	r, sr, mr := newTelemetryTestServer(t, "api", toolsMap, nil, nil, nil, groups)
	ts := runServer(r, false)
	defer ts.Close()

	bogus := "no-such-toolset-ccc"
	resp, _, err := runRequest(ts, http.MethodGet, "/toolset/"+bogus, nil, nil)
	if err != nil {
		t.Fatalf("GET /toolset/%s: %v", bogus, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /toolset/%s: status = %d, want 404", bogus, resp.StatusCode)
	}
	waitForSpan(t, sr, "toolbox/server/toolset/get")
	requireNoRawToolsetName(t, sr, mr, bogus)
}
