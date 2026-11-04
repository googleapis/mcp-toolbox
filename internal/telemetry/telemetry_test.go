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
	"net"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"cloud.google.com/go/trace/apiv2/tracepb"
	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/api/option"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeTraceServer struct {
	tracepb.UnimplementedTraceServiceServer
	mu    sync.Mutex
	spans []*tracepb.Span
}

func (s *fakeTraceServer) BatchWriteSpans(ctx context.Context, req *tracepb.BatchWriteSpansRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spans = append(s.spans, req.GetSpans()...)
	return &emptypb.Empty{}, nil
}

func (s *fakeTraceServer) spanNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.spans))
	for _, sp := range s.spans {
		if sp.GetDisplayName() != nil {
			names = append(names, sp.GetDisplayName().GetValue())
		}
	}
	return names
}

type fakeMetricServer struct {
	monitoringpb.UnimplementedMetricServiceServer
	mu         sync.Mutex
	timeSeries []*monitoringpb.TimeSeries
}

func (s *fakeMetricServer) CreateMetricDescriptor(ctx context.Context, req *monitoringpb.CreateMetricDescriptorRequest) (*metricpb.MetricDescriptor, error) {
	return req.GetMetricDescriptor(), nil
}

func (s *fakeMetricServer) CreateTimeSeries(ctx context.Context, req *monitoringpb.CreateTimeSeriesRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeSeries = append(s.timeSeries, req.GetTimeSeries()...)
	return &emptypb.Empty{}, nil
}

func (s *fakeMetricServer) timeSeriesCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.timeSeries)
}

func TestGCPExportersDoNotSelfTrace(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	traceSrv := &fakeTraceServer{}
	metricSrv := &fakeMetricServer{}
	grpcSrv := grpc.NewServer()
	tracepb.RegisterTraceServiceServer(grpcSrv, traceSrv)
	monitoringpb.RegisterMetricServiceServer(grpcSrv, metricSrv)
	go func() {
		_ = grpcSrv.Serve(lis)
	}()
	defer grpcSrv.Stop()

	testClientOpts := []option.ClientOption{
		option.WithEndpoint(lis.Addr().String()),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}

	traceExporter, err := texporter.New(gcpTraceExporterOpts("test-project", testClientOpts...)...)
	if err != nil {
		t.Fatalf("failed to create trace exporter: %v", err)
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(traceExporter, tracesdk.WithBatchTimeout(20*time.Millisecond)),
	)
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer func() {
		otel.SetTracerProvider(prevTP)
		_ = tp.Shutdown(context.Background())
	}()

	metricExporter, err := mexporter.New(gcpMetricExporterOpts("test-project", testClientOpts...)...)
	if err != nil {
		t.Fatalf("failed to create metric exporter: %v", err)
	}
	reader := metric.NewPeriodicReader(metricExporter)
	mp := metric.NewMeterProvider(metric.WithReader(reader))
	defer func() {
		_ = mp.Shutdown(context.Background())
		_ = metricExporter.Shutdown(context.Background())
	}()

	// Emit a single application span and metric.
	_, span := tp.Tracer("test").Start(context.Background(), "app-span")
	span.End()

	counter, err := mp.Meter("test").Int64Counter("test_counter")
	if err != nil {
		t.Fatalf("failed to create counter: %v", err)
	}
	counter.Add(context.Background(), 1)

	if err := mp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("failed to flush meter provider: %v", err)
	}
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("failed to flush trace provider: %v", err)
	}

	// Wait across multiple batch timeouts to ensure no self-tracing loop occurs.
	time.Sleep(100 * time.Millisecond)

	if got := metricSrv.timeSeriesCount(); got != 1 {
		t.Fatalf("expected 1 exported time series, got %d", got)
	}

	names := traceSrv.spanNames()
	if len(names) != 1 || names[0] != "app-span" {
		t.Fatalf("expected only [app-span], got %v", names)
	}
}
