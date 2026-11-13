// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloudmonitoring

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

type monitoringTransport struct {
	backend *url.URL
	next    http.RoundTripper
}

func (m monitoringTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Hostname() {
	case "monitoring.googleapis.com":
		clone := req.Clone(req.Context())
		clone.URL.Scheme, clone.URL.Host = m.backend.Scheme, m.backend.Host
		return m.next.RoundTrip(clone)
	case "127.0.0.1", "localhost":
		return m.next.RoundTrip(req)
	default:
		return nil, fmt.Errorf("unexpected external request to %s", req.URL.Host)
	}
}

// The fixture exercises the real source and MCP server with a local Monitoring
// backend. It does not validate Google credentials or service availability.
func setupCloudMonitoringTest(t *testing.T) *atomic.Int32 {
	t.Helper()
	calls := new(atomic.Int32)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/projects/test-project/location/global/prometheus/api/v1/query" {
			t.Errorf("unexpected backend request: %s %s", r.Method, r.URL)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		switch r.URL.Query().Get("query") {
		case "up":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
		case "backend-error":
			http.Error(w, "monitoring backend unavailable", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
			http.Error(w, "unexpected query", http.StatusBadRequest)
		}
	}))
	t.Cleanup(backend.Close)
	endpoint, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	original := http.DefaultTransport
	http.DefaultTransport = monitoringTransport{backend: endpoint, next: original}
	t.Cleanup(func() { http.DefaultTransport = original })
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	config := map[string]any{
		"sources": map[string]any{"monitoring": map[string]any{"type": "cloud-monitoring", "useClientOAuth": true}},
		"tools":   map[string]any{"query-prometheus": map[string]any{"type": "cloud-monitoring-query-prometheus", "source": "monitoring", "description": "Query Prometheus metrics."}},
	}
	cmd, cleanup, err := tests.StartCmd(ctx, config)
	if err != nil {
		t.Fatalf("start toolbox: %v", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		cmd.Stop()
		waitCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := cmd.Wait(waitCtx); err != nil {
			t.Errorf("stop toolbox: %v", err)
		}
		cmd.Close()
	})
	waitCtx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	if out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out); err != nil {
		t.Fatalf("toolbox did not start: %v\n%s", err, out)
	}
	return calls
}
