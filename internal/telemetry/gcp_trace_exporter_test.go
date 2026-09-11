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
	"errors"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeSpanExporter is a minimal sdktrace.SpanExporter whose ExportSpans
// result is scripted call-by-call, and which records how many times
// ExportSpans and Shutdown were actually invoked.
type fakeSpanExporter struct {
	results       []error
	exportCalls   int
	shutdownCalls int
}

func (f *fakeSpanExporter) ExportSpans(_ context.Context, _ []sdktrace.ReadOnlySpan) error {
	var err error
	if f.exportCalls < len(f.results) {
		err = f.results[f.exportCalls]
	}
	f.exportCalls++
	return err
}

func (f *fakeSpanExporter) Shutdown(_ context.Context) error {
	f.shutdownCalls++
	return nil
}

func permissionDeniedErr() error {
	return status.Error(codes.PermissionDenied, "denied")
}

func TestCircuitBreakingTraceExporter_SuccessPassesThrough(t *testing.T) {
	fake := &fakeSpanExporter{results: []error{nil}}
	exp := newCircuitBreakingTraceExporter(fake)

	if err := exp.ExportSpans(context.Background(), nil); err != nil {
		t.Fatalf("ExportSpans() = %v, want nil", err)
	}
	if fake.exportCalls != 1 {
		t.Fatalf("underlying ExportSpans called %d times, want 1", fake.exportCalls)
	}
}

func TestCircuitBreakingTraceExporter_NonPermissionDeniedAlwaysPassesThrough(t *testing.T) {
	unavailable := status.Error(codes.Unavailable, "try again")
	fake := &fakeSpanExporter{results: []error{unavailable, unavailable, unavailable, unavailable, unavailable}}
	exp := newCircuitBreakingTraceExporter(fake)

	for i := range 5 {
		if err := exp.ExportSpans(context.Background(), nil); !errors.Is(err, unavailable) {
			t.Fatalf("call %d: ExportSpans() = %v, want %v", i, err, unavailable)
		}
	}
	if fake.exportCalls != 5 {
		t.Fatalf("underlying ExportSpans called %d times, want 5 (never disabled)", fake.exportCalls)
	}
}

func TestCircuitBreakingTraceExporter_DisablesAfterThresholdConsecutivePermissionDenied(t *testing.T) {
	results := make([]error, 0, gcpTracePermissionDeniedThreshold+2)
	for range gcpTracePermissionDeniedThreshold {
		results = append(results, permissionDeniedErr())
	}
	fake := &fakeSpanExporter{results: results}
	exp := newCircuitBreakingTraceExporter(fake)

	for i := range gcpTracePermissionDeniedThreshold - 1 {
		if err := exp.ExportSpans(context.Background(), nil); err == nil {
			t.Fatalf("call %d: ExportSpans() = nil, want a PermissionDenied error (below threshold)", i)
		}
	}
	// The threshold-th consecutive failure trips the breaker: the caller
	// still sees a nil error (nothing left to usefully retry on).
	if err := exp.ExportSpans(context.Background(), nil); err != nil {
		t.Fatalf("threshold call: ExportSpans() = %v, want nil (circuit tripped)", err)
	}
	if fake.exportCalls != gcpTracePermissionDeniedThreshold {
		t.Fatalf("underlying ExportSpans called %d times, want %d", fake.exportCalls, gcpTracePermissionDeniedThreshold)
	}

	// Further calls must not reach the underlying exporter at all.
	for range 3 {
		if err := exp.ExportSpans(context.Background(), nil); err != nil {
			t.Fatalf("post-trip ExportSpans() = %v, want nil", err)
		}
	}
	if fake.exportCalls != gcpTracePermissionDeniedThreshold {
		t.Fatalf("underlying ExportSpans called %d times after trip, want still %d (breaker open)", fake.exportCalls, gcpTracePermissionDeniedThreshold)
	}
}

func TestCircuitBreakingTraceExporter_SuccessResetsConsecutiveCount(t *testing.T) {
	// One fewer than the threshold, then a success, then the threshold
	// again: the breaker must not trip, because the streak was reset.
	results := []error{}
	for range gcpTracePermissionDeniedThreshold - 1 {
		results = append(results, permissionDeniedErr())
	}
	results = append(results, nil)
	for range gcpTracePermissionDeniedThreshold - 1 {
		results = append(results, permissionDeniedErr())
	}
	fake := &fakeSpanExporter{results: results}
	exp := newCircuitBreakingTraceExporter(fake)

	for range results {
		exp.ExportSpans(context.Background(), nil)
	}
	if fake.exportCalls != len(results) {
		t.Fatalf("underlying ExportSpans called %d times, want %d (breaker never tripped)", fake.exportCalls, len(results))
	}
}

func TestCircuitBreakingTraceExporter_OtherErrorResetsConsecutiveCount(t *testing.T) {
	unavailable := status.Error(codes.Unavailable, "try again")
	results := []error{}
	for range gcpTracePermissionDeniedThreshold - 1 {
		results = append(results, permissionDeniedErr())
	}
	results = append(results, unavailable)
	for range gcpTracePermissionDeniedThreshold - 1 {
		results = append(results, permissionDeniedErr())
	}
	fake := &fakeSpanExporter{results: results}
	exp := newCircuitBreakingTraceExporter(fake)

	for range results {
		exp.ExportSpans(context.Background(), nil)
	}
	if fake.exportCalls != len(results) {
		t.Fatalf("underlying ExportSpans called %d times, want %d (breaker never tripped)", fake.exportCalls, len(results))
	}
}

func TestCircuitBreakingTraceExporter_ShutdownDelegates(t *testing.T) {
	fake := &fakeSpanExporter{}
	exp := newCircuitBreakingTraceExporter(fake)
	if err := exp.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() = %v, want nil", err)
	}
	if fake.shutdownCalls != 1 {
		t.Fatalf("underlying Shutdown called %d times, want 1", fake.shutdownCalls)
	}
}
