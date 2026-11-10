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

package getindexrecommendations_test

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
	getrecs "github.com/googleapis/mcp-toolbox/internal/tools/databaseinsights/getindexrecommendations"
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
            name: get_index_recs
            type: databaseinsights-get-index-recommendations
            source: my-db-insights-source
            description: fetches index recommendations
            `,
			want: server.ToolConfigs{
				"get_index_recs": getrecs.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "get_index_recs",
						Description:  "fetches index recommendations",
						AuthRequired: []string{},
					},
					Type:   "databaseinsights-get-index-recommendations",
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
	expectedReq *databaseinsights.BatchQueryIndexRecommendationsRequest
	resp        *databaseinsights.BatchQueryIndexRecommendationsResponse
	err         error
}

func (m *mockSource) BatchQueryIndexRecommendations(ctx context.Context, req *databaseinsights.BatchQueryIndexRecommendationsRequest) (*databaseinsights.BatchQueryIndexRecommendationsResponse, error) {
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

	cfg := getrecs.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "get_index_recs",
			Description: "fetches index recommendations",
		},
		Type:   "databaseinsights-get-index-recommendations",
		Source: "my-db-insights-source",
	}

	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	mockSrc := &mockSource{
		t: t,
		expectedReq: &databaseinsights.BatchQueryIndexRecommendationsRequest{
			Parent:           "projects/mock-project/locations/us-central1",
			FullResourceName: "//alloydb.googleapis.com/clusters/my-cluster/instances/my-instance",
			DatabaseQueryIds: []databaseinsights.DatabaseQueryIds{
				{
					Database: "my-db",
					QueryIDs: []string{"12345"},
				},
			},
		},
		resp: &databaseinsights.BatchQueryIndexRecommendationsResponse{
			DatabaseIndexRecommendations: []databaseinsights.DatabaseIndexRecommendation{
				{
					Database: "my-db",
					IndexRecommendations: []databaseinsights.IndexRecommendation{
						{
							SQLCommand: "CREATE INDEX ON t(c)",
						},
					},
				},
			},
		},
	}

	params := parameters.ParamValues{
		{Name: "parent", Value: "projects/mock-project/locations/us-central1"},
		{Name: "full_resource_name", Value: "//alloydb.googleapis.com/clusters/my-cluster/instances/my-instance"},
		{
			Name: "database_query_ids",
			Value: []any{
				map[string]any{
					"database":  "my-db",
					"query_ids": []any{"12345"},
				},
			},
		},
	}

	got, tErr := tool.Invoke(ctx, mockSrc, params, "")
	if tErr != nil {
		t.Fatalf("Invoke failed: %v", tErr)
	}

	if diff := cmp.Diff(mockSrc.resp, got, cmpopts.IgnoreUnexported(databaseinsights.BatchQueryIndexRecommendationsResponse{})); diff != "" {
		t.Fatalf("Unexpected response from Invoke: diff %v", diff)
	}
}

func TestInvoke_OmittedDatabaseQueryIDs(t *testing.T) {
	ctx := context.Background()

	cfg := getrecs.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "get_index_recs",
			Description: "fetches index recommendations",
		},
		Type:   "databaseinsights-get-index-recommendations",
		Source: "my-db-insights-source",
	}

	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	mockSrc := &mockSource{
		t: t,
		expectedReq: &databaseinsights.BatchQueryIndexRecommendationsRequest{
			Parent:           "projects/mock-project/locations/us-central1",
			FullResourceName: "//alloydb.googleapis.com/clusters/my-cluster/instances/my-instance",
			DatabaseQueryIds: nil,
		},
		resp: &databaseinsights.BatchQueryIndexRecommendationsResponse{},
	}

	params := parameters.ParamValues{
		{Name: "parent", Value: "projects/mock-project/locations/us-central1"},
		{Name: "full_resource_name", Value: "//alloydb.googleapis.com/clusters/my-cluster/instances/my-instance"},
	}

	got, tErr := tool.Invoke(ctx, mockSrc, params, "")
	if tErr != nil {
		t.Fatalf("Invoke failed: %v", tErr)
	}

	if diff := cmp.Diff(mockSrc.resp, got, cmpopts.IgnoreUnexported(databaseinsights.BatchQueryIndexRecommendationsResponse{})); diff != "" {
		t.Fatalf("Unexpected response from Invoke: diff %v", diff)
	}
}

func TestInvoke_EmptyQueryIDs(t *testing.T) {
	ctx := context.Background()

	cfg := getrecs.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "get_index_recs",
			Description: "fetches index recommendations",
		},
		Type:   "databaseinsights-get-index-recommendations",
		Source: "my-db-insights-source",
	}

	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	mockSrc := &mockSource{
		t: t,
		expectedReq: &databaseinsights.BatchQueryIndexRecommendationsRequest{
			Parent:           "projects/mock-project/locations/us-central1",
			FullResourceName: "//alloydb.googleapis.com/clusters/my-cluster/instances/my-instance",
			DatabaseQueryIds: []databaseinsights.DatabaseQueryIds{
				{
					Database: "my-db",
					QueryIDs: nil,
				},
			},
		},
		resp: &databaseinsights.BatchQueryIndexRecommendationsResponse{},
	}

	params := parameters.ParamValues{
		{Name: "parent", Value: "projects/mock-project/locations/us-central1"},
		{Name: "full_resource_name", Value: "//alloydb.googleapis.com/clusters/my-cluster/instances/my-instance"},
		{
			Name: "database_query_ids",
			Value: []any{
				map[string]any{
					"database": "my-db",
				},
			},
		},
	}

	got, tErr := tool.Invoke(ctx, mockSrc, params, "")
	if tErr != nil {
		t.Fatalf("Invoke failed: %v", tErr)
	}

	if diff := cmp.Diff(mockSrc.resp, got, cmpopts.IgnoreUnexported(databaseinsights.BatchQueryIndexRecommendationsResponse{})); diff != "" {
		t.Fatalf("Unexpected response from Invoke: diff %v", diff)
	}
}
