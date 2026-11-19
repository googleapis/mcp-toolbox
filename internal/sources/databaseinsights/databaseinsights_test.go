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

package databaseinsights_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/databaseinsights"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestParseFromYamlDatabaseInsights(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-insights
			type: databaseinsights
			project: my-project
			`,
			want: map[string]sources.SourceConfig{
				"my-insights": databaseinsights.Config{
					Name:    "my-insights",
					Type:    databaseinsights.SourceKind,
					Project: "my-project",
				},
			},
		},
		{
			desc: "with endpoint override",
			in: `
			kind: source
			name: my-insights
			type: databaseinsights
			project: my-project
			endpoint: staging-databaseinsights.sandbox.googleapis.com
			`,
			want: map[string]sources.SourceConfig{
				"my-insights": databaseinsights.Config{
					Name:     "my-insights",
					Type:     databaseinsights.SourceKind,
					Project:  "my-project",
					Endpoint: "staging-databaseinsights.sandbox.googleapis.com",
				},
			},
		},
		{
			desc: "without project",
			in: `
			kind: source
			name: my-insights
			type: databaseinsights
			`,
			want: map[string]sources.SourceConfig{
				"my-insights": databaseinsights.Config{
					Name: "my-insights",
					Type: databaseinsights.SourceKind,
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse: want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestFailParseFromYaml(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-insights
			type: databaseinsights
			project: my-project
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-insights\" as \"databaseinsights\": [1:1] unknown field \"foo\"\n>  1 | foo: bar\n       ^\n   2 | name: my-insights\n   3 | project: my-project\n   4 | type: databaseinsights",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if errStr != tc.err {
				t.Fatalf("unexpected error: got %q, want %q", errStr, tc.err)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func newTestSource(t *testing.T, endpoint string, rt http.RoundTripper) *databaseinsights.Source {
	t.Helper()
	credsFile := filepath.Join(t.TempDir(), "creds.json")
	credsJSON := `{"type":"authorized_user","client_id":"id","client_secret":"secret","refresh_token":"token"}`
	if err := os.WriteFile(credsFile, []byte(credsJSON), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credsFile)

	ctx := util.WithUserAgent(context.Background(), "mcp-toolbox-test")
	cfg := databaseinsights.Config{Name: "test", Type: databaseinsights.SourceKind, Endpoint: endpoint}
	src, err := cfg.Initialize(ctx, noop.NewTracerProvider().Tracer("test"))
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	diSrc := src.(*databaseinsights.Source)
	diSrc.HTTPClient().Transport = rt
	return diSrc
}

func TestParentValidationAndEndpointRouting(t *testing.T) {
	var capturedURL string
	called := false
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		capturedURL = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
		}, nil
	})

	s := newTestSource(t, "", rt)
	ctx := context.Background()

	valid := []struct {
		parent  string
		wantURL string
	}{
		{"projects/my-project/locations/global", "https://databaseinsights.googleapis.com/v1beta/projects/my-project/locations/global/queryStats:fetch"},
		{"projects/my-project/locations/us-central1", "https://us-central1-databaseinsights.googleapis.com/v1beta/projects/my-project/locations/us-central1/queryStats:fetch"},
		{"projects/example.com:my-project/locations/europe-west1", "https://europe-west1-databaseinsights.googleapis.com/v1beta/projects/example.com:my-project/locations/europe-west1/queryStats:fetch"},
	}
	for _, tc := range valid {
		called = false
		if _, err := s.FetchQueryStats(ctx, &databaseinsights.FetchQueryStatsRequest{Parent: tc.parent}); err != nil {
			t.Fatalf("FetchQueryStats(%q) unexpected error: %v", tc.parent, err)
		}
		if !called || capturedURL != tc.wantURL {
			t.Fatalf("FetchQueryStats(%q) URL = %q (called=%v), want %q", tc.parent, capturedURL, called, tc.wantURL)
		}
	}

	custom := newTestSource(t, "https://staging-databaseinsights.googleapis.com", rt)
	if _, err := custom.FetchQueryStats(ctx, &databaseinsights.FetchQueryStatsRequest{Parent: "projects/my-project/locations/us-central1"}); err != nil {
		t.Fatalf("custom endpoint unexpected error: %v", err)
	}
	if want := "https://staging-databaseinsights.googleapis.com/v1beta/projects/my-project/locations/us-central1/queryStats:fetch"; capturedURL != want {
		t.Fatalf("custom endpoint URL = %q, want %q", capturedURL, want)
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
	called = false
	for _, bad := range invalid {
		if _, err := s.FetchQueryStats(ctx, &databaseinsights.FetchQueryStatsRequest{Parent: bad}); err == nil {
			t.Fatalf("FetchQueryStats(%q) expected error", bad)
		}
		if _, err := s.FetchWaitEventStats(ctx, &databaseinsights.FetchWaitEventStatsRequest{Parent: bad}); err == nil {
			t.Fatalf("FetchWaitEventStats(%q) expected error", bad)
		}
		if _, err := s.FetchQueryTimeSeries(ctx, &databaseinsights.FetchQueryTimeSeriesRequest{Parent: bad}); err == nil {
			t.Fatalf("FetchQueryTimeSeries(%q) expected error", bad)
		}
		if _, err := s.FetchWaitEventTimeSeries(ctx, &databaseinsights.FetchWaitEventTimeSeriesRequest{Parent: bad}); err == nil {
			t.Fatalf("FetchWaitEventTimeSeries(%q) expected error", bad)
		}
		if _, err := s.BatchQueryIndexRecommendations(ctx, &databaseinsights.BatchQueryIndexRecommendationsRequest{Parent: bad}); err == nil {
			t.Fatalf("BatchQueryIndexRecommendations(%q) expected error", bad)
		}
	}
	if called {
		t.Fatal("expected zero outbound HTTP requests for invalid parent inputs")
	}
}
