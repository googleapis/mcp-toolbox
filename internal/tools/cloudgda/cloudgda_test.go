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

package cloudgda_test

import (
	"context"
	"fmt"
	"testing"

	"cloud.google.com/go/geminidataanalytics/apiv1beta/geminidataanalyticspb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	cloudgdatool "github.com/googleapis/mcp-toolbox/internal/tools/cloudgda"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYaml(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	t.Parallel()
	tcs := []struct {
		desc string
		in   string
		want server.ToolConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: tool
			name: my-gda-query-tool
			type: cloud-gemini-data-analytics-query
			source: gda-api-source
			description: Test Description
			location: us-central1
			context:
				datasourceReferences:
					spannerReference:
						databaseReference:
							projectId:  "cloud-db-nl2sql"
							region:     "us-central1"
							instanceId: "evalbench"
							databaseId: "financial"
							engine:     "GOOGLE_SQL"
						agentContextReference:
							contextSetId: "projects/cloud-db-nl2sql/locations/us-east1/contextSets/bdf_gsql_gemini_all_templates"
			generationOptions:
				generateQueryResult: true
			`,
			want: map[string]tools.ToolConfig{
				"my-gda-query-tool": cloudgdatool.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "my-gda-query-tool",
						Description:  "Test Description",
						AuthRequired: []string{},
					},
					Type:     "cloud-gemini-data-analytics-query",
					Source:   "gda-api-source",
					Location: "us-central1",
					Context: &cloudgdatool.QueryDataContext{
						QueryDataContext: &geminidataanalyticspb.QueryDataContext{
							DatasourceReferences: &geminidataanalyticspb.DatasourceReferences{
								References: &geminidataanalyticspb.DatasourceReferences_SpannerReference{
									SpannerReference: &geminidataanalyticspb.SpannerReference{
										DatabaseReference: &geminidataanalyticspb.SpannerDatabaseReference{
											ProjectId:  "cloud-db-nl2sql",
											Region:     "us-central1",
											InstanceId: "evalbench",
											DatabaseId: "financial",
											Engine:     geminidataanalyticspb.SpannerDatabaseReference_GOOGLE_SQL,
										},
										AgentContextReference: &geminidataanalyticspb.AgentContextReference{
											ContextSetId: "projects/cloud-db-nl2sql/locations/us-east1/contextSets/bdf_gsql_gemini_all_templates",
										},
									},
								},
							},
						},
					},
					GenerationOptions: &cloudgdatool.GenerationOptions{
						GenerationOptions: &geminidataanalyticspb.GenerationOptions{
							GenerateQueryResult: true,
						},
					},
				},
			},
		},
	}
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got, cmpopts.IgnoreUnexported(geminidataanalyticspb.QueryDataContext{}, geminidataanalyticspb.DatasourceReferences{}, geminidataanalyticspb.SpannerReference{}, geminidataanalyticspb.SpannerDatabaseReference{}, geminidataanalyticspb.AgentContextReference{}, geminidataanalyticspb.GenerationOptions{}, geminidataanalyticspb.DatasourceReferences_SpannerReference{})) {
				t.Fatalf("incorrect parse: want %v, got %v", tc.want, got)
			}
		})
	}
}

// fakeSource implements the compatibleSource interface for testing.
type fakeSource struct {
	projectID      string
	useClientOAuth bool
	expectedQuery  string
	expectedParent string
	response       *geminidataanalyticspb.QueryDataResponse
}

func (f *fakeSource) GetProjectID() string {
	return f.projectID
}

func (f *fakeSource) UseClientAuthorization() bool {
	return f.useClientOAuth
}

func (f *fakeSource) IsReadOnly() bool {
	return false
}

func (f *fakeSource) SourceType() string {
	return "cloud-gemini-data-analytics"
}

func (f *fakeSource) ToConfig() sources.SourceConfig {
	return nil
}

func (f *fakeSource) Initialize(ctx context.Context, tracer interface{}) (sources.Source, error) {
	return f, nil
}

func (f *fakeSource) RunQuery(ctx context.Context, token string, req *geminidataanalyticspb.QueryDataRequest) (*geminidataanalyticspb.QueryDataResponse, error) {
	if req.Prompt != f.expectedQuery {
		return nil, fmt.Errorf("unexpected query: got %q, want %q", req.Prompt, f.expectedQuery)
	}
	if req.Parent != f.expectedParent {
		return nil, fmt.Errorf("unexpected parent: got %q, want %q", req.Parent, f.expectedParent)
	}
	// Basic validation of context/options could be added here if needed,
	// but the test case mainly checks if they are passed correctly via successful invocation.

	return f.response, nil
}

func TestInitialize(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		desc string
		cfg  cloudgdatool.Config
	}{
		{
			desc: "successful initialization",
			cfg: cloudgdatool.Config{
				ConfigBase: tools.ConfigBase{
					Name:        "my-gda-query-tool",
					Description: "Test Description",
				},
				Type:     "cloud-gemini-data-analytics-query",
				Source:   "gda-api-source",
				Location: "us-central1",
			},
		},
	}

	// No incompatible source for testing needed with fakeSource
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			tool, err := tc.cfg.Initialize(context.Background())
			if err != nil {
				t.Fatalf("did not expect an error but got: %v", err)
			}
			// Basic sanity check on the returned tool
			_ = tool // Avoid unused variable error
		})
	}
}

func TestInvoke(t *testing.T) {
	t.Parallel()

	projectID := "test-project"
	location := "us-central1"
	query := "How many accounts who have region in Prague are eligible for loans?"
	expectedParent := fmt.Sprintf("projects/%s/locations/%s", projectID, location)

	// Prepare expected response
	expectedResp := &geminidataanalyticspb.QueryDataResponse{
		GeneratedQuery:        "SELECT count(*) FROM accounts WHERE region = 'Prague' AND eligible_for_loans = true;",
		NaturalLanguageAnswer: "There are 5 accounts in Prague eligible for loans.",
	}

	fake := &fakeSource{
		projectID:      projectID,
		expectedQuery:  query,
		expectedParent: expectedParent,
		response:       expectedResp,
	}

	// Initialize the tool config with context
	toolCfg := cloudgdatool.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "query-data-tool",
			Description: "Query Gemini Data Analytics",
		},
		Type:     "cloud-gemini-data-analytics-query",
		Source:   "mock-gda-source",
		Location: location,
		Context: &cloudgdatool.QueryDataContext{
			QueryDataContext: &geminidataanalyticspb.QueryDataContext{
				DatasourceReferences: &geminidataanalyticspb.DatasourceReferences{
					References: &geminidataanalyticspb.DatasourceReferences_SpannerReference{
						SpannerReference: &geminidataanalyticspb.SpannerReference{
							DatabaseReference: &geminidataanalyticspb.SpannerDatabaseReference{
								ProjectId:  "cloud-db-nl2sql",
								Region:     "us-central1",
								InstanceId: "evalbench",
								DatabaseId: "financial",
								Engine:     geminidataanalyticspb.SpannerDatabaseReference_GOOGLE_SQL,
							},
							AgentContextReference: &geminidataanalyticspb.AgentContextReference{
								ContextSetId: "projects/cloud-db-nl2sql/locations/us-east1/contextSets/bdf_gsql_gemini_all_templates",
							},
						},
					},
				},
			},
		},
		GenerationOptions: &cloudgdatool.GenerationOptions{
			GenerationOptions: &geminidataanalyticspb.GenerationOptions{
				GenerateQueryResult: true,
			},
		},
	}

	tool, err := toolCfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	// Prepare parameters for invocation - ONLY query
	params := parameters.ParamValues{
		{Name: "query", Value: query},
	}

	ctx := testutils.ContextWithUserAgent(context.Background(), "test-user-agent")

	// Invoke the tool
	result, err := tool.Invoke(ctx, fake, params, "")
	if err != nil {
		t.Fatalf("tool invocation failed: %v", err)
	}

	gotResp, ok := result.(*geminidataanalyticspb.QueryDataResponse)
	if !ok {
		t.Fatalf("expected result type *geminidataanalyticspb.QueryDataResponse, got %T", result)
	}

	if diff := cmp.Diff(expectedResp, gotResp, cmpopts.IgnoreUnexported(geminidataanalyticspb.QueryDataResponse{})); diff != "" {
		t.Errorf("unexpected result mismatch (-want +got):\n%s", diff)
	}
}
