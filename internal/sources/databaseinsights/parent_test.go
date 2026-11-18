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

package databaseinsights

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetEndpointForParent(t *testing.T) {
	s := &Source{}
	valid := []struct {
		parent string
		want   string
	}{
		{"projects/my-project/locations/global", "https://databaseinsights.googleapis.com"},
		{"projects/my-project/locations/us-central1", "https://us-central1-databaseinsights.googleapis.com"},
		{"projects/example.com:my-project/locations/europe-west1", "https://europe-west1-databaseinsights.googleapis.com"},
	}
	for _, tc := range valid {
		got, err := s.getEndpointForParent(tc.parent)
		if err != nil || got != tc.want {
			t.Fatalf("getEndpointForParent(%q) = (%q, %v), want (%q, nil)", tc.parent, got, err, tc.want)
		}
	}

	custom := &Source{endpoint: "https://staging-databaseinsights.googleapis.com"}
	if got, err := custom.getEndpointForParent("projects/my-project/locations/us-central1"); err != nil || got != custom.endpoint {
		t.Fatalf("custom endpoint got (%q, %v), want (%q, nil)", got, err, custom.endpoint)
	}

	invalid := []string{
		"",
		"/projects/my-project/locations/us-central1",
		"projects/my-project/locations/example.com?",
		"projects/my-project/locations/us-central1@example.com",
		"projects/my-project/locations/us-central1#frag",
		"projects/my-project/locations/../global",
		"projects/a..b/locations/us-central1",
		"projects/my-project/locations/US-CENTRAL1",
		"projects/my-project/locations/us-central1/extra",
		"projects/my-project/locations/us-central1\nextra",
	}
	for _, bad := range invalid {
		if got, err := s.getEndpointForParent(bad); err == nil {
			t.Fatalf("getEndpointForParent(%q) = %q, expected error", bad, got)
		}
	}
}

func TestMethodsRejectInvalidParentWithoutOutboundRequest(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	s := &Source{httpClient: ts.Client(), endpoint: ts.URL}
	ctx := context.Background()
	badParent := "projects/my-project/locations/example.com?"

	if _, err := s.FetchQueryStats(ctx, &FetchQueryStatsRequest{Parent: badParent}); err == nil {
		t.Fatal("FetchQueryStats expected error on invalid parent")
	}
	if _, err := s.FetchWaitEventStats(ctx, &FetchWaitEventStatsRequest{Parent: badParent}); err == nil {
		t.Fatal("FetchWaitEventStats expected error on invalid parent")
	}
	if _, err := s.FetchQueryTimeSeries(ctx, &FetchQueryTimeSeriesRequest{Parent: badParent}); err == nil {
		t.Fatal("FetchQueryTimeSeries expected error on invalid parent")
	}
	if _, err := s.FetchWaitEventTimeSeries(ctx, &FetchWaitEventTimeSeriesRequest{Parent: badParent}); err == nil {
		t.Fatal("FetchWaitEventTimeSeries expected error on invalid parent")
	}
	if _, err := s.BatchQueryIndexRecommendations(ctx, &BatchQueryIndexRecommendationsRequest{Parent: badParent}); err == nil {
		t.Fatal("BatchQueryIndexRecommendations expected error on invalid parent")
	}
	if called {
		t.Fatal("expected zero HTTP requests for invalid parent")
	}

	if _, err := s.FetchQueryStats(ctx, &FetchQueryStatsRequest{Parent: "projects/my-project/locations/us-central1"}); err != nil || !called {
		t.Fatalf("expected valid FetchQueryStats to succeed and call server, err=%v called=%v", err, called)
	}
}
