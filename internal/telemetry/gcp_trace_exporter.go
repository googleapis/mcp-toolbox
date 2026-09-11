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

package telemetry

import (
	"context"
	"log/slog"
	"sync/atomic"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// gcpTracePermissionDeniedThreshold is the number of consecutive
// PermissionDenied export failures after which the exporter disables itself
// for the rest of the process lifetime. A PermissionDenied on trace export
// means the service account is missing an IAM grant, which does not appear
// spontaneously, unlike Unavailable/DeadlineExceeded which are genuinely
// worth retrying.
const gcpTracePermissionDeniedThreshold = 3

// gcpTracePermissionDeniedRole is named in the one-time warning so an
// operator does not have to read the raw error to find it.
const gcpTracePermissionDeniedRole = "roles/cloudtrace.agent"

// circuitBreakingTraceExporter wraps a Cloud Trace span exporter and stops
// calling it after gcpTracePermissionDeniedThreshold consecutive
// PermissionDenied failures, logging a single WARN instead of the
// underlying exporter's own per-export error log (the batch span processor
// otherwise retries on every export interval, ~5s by default, for the life
// of the process).
type circuitBreakingTraceExporter struct {
	sdktrace.SpanExporter
	consecutivePermissionDenied atomic.Int32
	disabled                    atomic.Bool
}

// newCircuitBreakingTraceExporter wraps exp with the PermissionDenied
// circuit breaker described above.
func newCircuitBreakingTraceExporter(exp sdktrace.SpanExporter) sdktrace.SpanExporter {
	return &circuitBreakingTraceExporter{SpanExporter: exp}
}

// ExportSpans implements sdktrace.SpanExporter.
func (e *circuitBreakingTraceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if e.disabled.Load() {
		return nil
	}
	err := e.SpanExporter.ExportSpans(ctx, spans)
	if err == nil {
		e.consecutivePermissionDenied.Store(0)
		return nil
	}
	if status.Code(err) != codes.PermissionDenied {
		e.consecutivePermissionDenied.Store(0)
		return err
	}
	if e.consecutivePermissionDenied.Add(1) < gcpTracePermissionDeniedThreshold {
		return err
	}
	if e.disabled.CompareAndSwap(false, true) {
		slog.WarnContext(ctx,
			"disabling Google Cloud Trace export after repeated PermissionDenied failures; grant the missing role to the service account to re-enable",
			"consecutiveFailures", gcpTracePermissionDeniedThreshold,
			"requiredRole", gcpTracePermissionDeniedRole,
			"error", err.Error(),
		)
	}
	return nil
}
