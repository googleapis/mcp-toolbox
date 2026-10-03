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

// Internal-package tests for HTTP server timeout configuration.
// The internal package is used so tests can inspect the unexported srv field.

import (
	"context"
	"os"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/telemetry"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

// TestHTTPServerReadHeaderTimeoutIsSet verifies that NewServer sets a non-zero
// ReadHeaderTimeout on the underlying http.Server to prevent Slowloris attacks.
// Without ReadHeaderTimeout an attacker can trickle request headers
// indefinitely, consuming a goroutine per connection until the server is
// exhausted.
//
// ReadTimeout is intentionally omitted: in Go, http.Server.ReadTimeout sets an
// absolute read deadline that is not cleared when the handler starts. The
// server's background connection-monitoring goroutine then hits that deadline
// and cancels r.Context(), prematurely terminating SSE streams after the
// timeout period regardless of active subscribers.
func TestHTTPServerReadHeaderTimeoutIsSet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unexpected error creating logger: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error creating instrumentation: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	cfg := ServerConfig{
		Version:      "0.0.0",
		Address:      "127.0.0.1",
		Port:         0,
		AllowedHosts: []string{"*"},
	}

	s, err := NewServer(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error from NewServer: %s", err)
	}

	if s.srv.ReadHeaderTimeout == 0 {
		t.Error("srv.ReadHeaderTimeout is 0: server is vulnerable to Slowloris attacks")
	}
	if s.srv.ReadHeaderTimeout != DefaultReadHeaderTimeout {
		t.Errorf("srv.ReadHeaderTimeout = %v, want %v", s.srv.ReadHeaderTimeout, DefaultReadHeaderTimeout)
	}

	if s.srv.ReadTimeout != 0 {
		t.Errorf("srv.ReadTimeout = %v, want 0: a non-zero ReadTimeout breaks SSE streams", s.srv.ReadTimeout)
	}
}
