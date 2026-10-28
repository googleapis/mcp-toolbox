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

package getadvancedaggregatedwaiteventstats_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/databaseinsights"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	getstats "github.com/googleapis/mcp-toolbox/internal/tools/databaseinsights/getadvancedaggregatedwaiteventstats"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYaml(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		want server.ToolConfigs
	}{
		{
			desc: "basic example",
			in: `
            kind: tool
            name: get_wait_event_stats
            type: databaseinsights-get-advanced-aggregated-wait-event-stats
            source: my-db-insights-source
            description: fetches wait event statistics
            `,
			want: server.ToolConfigs{
				"get_wait_event_stats": getstats.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "get_wait_event_stats",
						Description:  "fetches wait event statistics",
						AuthRequired: []string{},
					},
					Type:   "databaseinsights-get-advanced-aggregated-wait-event-stats",
					Source: "my-db-insights-source",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}

type mockSource struct {
	sources.Source
	t           *testing.T
	expectedReq *databaseinsights.FetchWaitEventStatsRequest
	resp        *databaseinsights.FetchWaitEventStatsResponse
	err         error
}

func (m *mockSource) FetchWaitEventStats(ctx context.Context, req *databaseinsights.FetchWaitEventStatsRequest) (*databaseinsights.FetchWaitEventStatsResponse, error) {
	if diff := cmp.Diff(m.expectedReq, req); diff != "" {
		m.t.Errorf("Unexpected request payload: diff %v", diff)
	}
	return m.resp, m.err
}

func (m *mockSource) SourceType() string {
	return "databaseinsights"
}

func (m *mockSource) ToConfig() sources.SourceConfig {
	return nil
}

func TestInvoke(t *testing.T) {
	ctx := context.Background()

	cfg := getstats.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "get_wait_event_stats",
			Description: "fetches wait event statistics",
		},
		Type:   "databaseinsights-get-advanced-aggregated-wait-event-stats",
		Source: "my-db-insights-source",
	}

	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	mockSrc := &mockSource{
		t: t,
		expectedReq: &databaseinsights.FetchWaitEventStatsRequest{
			Parent:           "projects/mock-project/locations/us-central1",
			FullResourceName: "//alloydb.googleapis.com/clusters/my-cluster/instances/my-instance",
			StartTime:        "2026-06-15T10:00:00Z",
			Database:         "my-db",
			PageSize:         10,
			View:             "WAIT_CLASS",
		},
		resp: &databaseinsights.FetchWaitEventStatsResponse{
			Results: [][]any{
				{"Lock", 12.5},
			},
			Metadata: databaseinsights.ResultSetMetadata{
				Fields: []databaseinsights.Field{
					{Name: "wait_class", Type: "STRING"},
					{Name: "sum(time_spent)", Type: "DOUBLE"},
				},
			},
		},
	}

	params := parameters.ParamValues{
		{Name: "parent", Value: "projects/mock-project/locations/us-central1"},
		{Name: "full_resource_name", Value: "//alloydb.googleapis.com/clusters/my-cluster/instances/my-instance"},
		{Name: "start_time", Value: "2026-06-15T10:00:00Z"},
		{Name: "database", Value: "my-db"},
		{Name: "page_size", Value: 10},
		{Name: "view", Value: "WAIT_CLASS"},
	}

	got, tErr := tool.Invoke(ctx, mockSrc, params, "")
	if tErr != nil {
		t.Fatalf("Invoke failed: %v", tErr)
	}

	if diff := cmp.Diff(mockSrc.resp, got, cmpopts.IgnoreUnexported(databaseinsights.FetchWaitEventStatsResponse{})); diff != "" {
		t.Fatalf("Unexpected response from Invoke: diff %v", diff)
	}
}
