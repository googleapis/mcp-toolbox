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
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"cloud.google.com/go/geminidataanalytics/apiv1beta/geminidataanalyticspb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	cloudgdatool "github.com/googleapis/mcp-toolbox/internal/tools/cloudgda"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/protobuf/testing/protocmp"
)

// wantToolConfigs returns the parsed form of the test tool, which differs
// between cases only in its datasource references.
func wantToolConfigs(refs *geminidataanalyticspb.DatasourceReferences) server.ToolConfigs {
	return server.ToolConfigs{
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
					DatasourceReferences: refs,
				},
			},
			GenerationOptions: &cloudgdatool.GenerationOptions{
				GenerationOptions: &geminidataanalyticspb.GenerationOptions{
					GenerateQueryResult: true,
				},
			},
		},
	}
}

func TestParseFromYaml(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	t.Parallel()
	spannerRefs := &geminidataanalyticspb.DatasourceReferences{
		References: &geminidataanalyticspb.DatasourceReferences_SpannerReference{
			SpannerReference: &geminidataanalyticspb.SpannerReference{
				DatabaseReference: &geminidataanalyticspb.SpannerDatabaseReference{
					ProjectId:  "cloud-db-nl2sql",
					InstanceId: "evalbench",
					DatabaseId: "financial",
					Engine:     geminidataanalyticspb.SpannerDatabaseReference_GOOGLE_SQL,
				},
				AgentContextReference: &geminidataanalyticspb.AgentContextReference{
					ContextSetId: "projects/cloud-db-nl2sql/locations/us-east1/contextSets/bdf_gsql_gemini_all_templates",
				},
			},
		},
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
							instanceId: "evalbench"
							databaseId: "financial"
							engine:     "GOOGLE_SQL"
						agentContextReference:
							contextSetId: "projects/cloud-db-nl2sql/locations/us-east1/contextSets/bdf_gsql_gemini_all_templates"
			generationOptions:
				generateQueryResult: true
			`,
			want: wantToolConfigs(spannerRefs),
		},
		{
			desc: "spanner region is ignored",
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
			want: wantToolConfigs(spannerRefs),
		},
		{
			desc: "spanner region is ignored with proto field names",
			in: `
			kind: tool
			name: my-gda-query-tool
			type: cloud-gemini-data-analytics-query
			source: gda-api-source
			description: Test Description
			location: us-central1
			context:
				datasource_references:
					spanner_reference:
						database_reference:
							project_id:  "cloud-db-nl2sql"
							region:      "us-central1"
							instance_id: "evalbench"
							database_id: "financial"
							engine:      "GOOGLE_SQL"
						agent_context_reference:
							context_set_id: "projects/cloud-db-nl2sql/locations/us-east1/contextSets/bdf_gsql_gemini_all_templates"
			generationOptions:
				generateQueryResult: true
			`,
			want: wantToolConfigs(spannerRefs),
		},
		{
			desc: "bigtable reference",
			in: `
			kind: tool
			name: my-gda-query-tool
			type: cloud-gemini-data-analytics-query
			source: gda-api-source
			description: Test Description
			location: us-central1
			context:
				datasourceReferences:
					bigtableReference:
						databaseReference:
							projectId:  "my-project"
							instanceId: "my-instance"
							tableIds:
								- "flights"
								- "airports"
						agentContextReference:
							contextSetId: "projects/my-project/locations/us-east1/contextSets/flights"
			generationOptions:
				generateQueryResult: true
			`,
			want: wantToolConfigs(&geminidataanalyticspb.DatasourceReferences{
				References: &geminidataanalyticspb.DatasourceReferences_BigtableReference{
					BigtableReference: &geminidataanalyticspb.BigtableReference{
						DatabaseReference: &geminidataanalyticspb.BigtableDatabaseReference{
							ProjectId:  "my-project",
							InstanceId: "my-instance",
							TableIds:   []string{"flights", "airports"},
						},
						AgentContextReference: &geminidataanalyticspb.AgentContextReference{
							ContextSetId: "projects/my-project/locations/us-east1/contextSets/flights",
						},
					},
				},
			}),
		},
		{
			desc: "firestore reference",
			in: `
			kind: tool
			name: my-gda-query-tool
			type: cloud-gemini-data-analytics-query
			source: gda-api-source
			description: Test Description
			location: us-central1
			context:
				datasourceReferences:
					firestoreReference:
						databaseReference:
							projectId:  "my-project"
							databaseId: "my-database"
							collectionIds:
								- "orders"
			generationOptions:
				generateQueryResult: true
			`,
			want: wantToolConfigs(&geminidataanalyticspb.DatasourceReferences{
				References: &geminidataanalyticspb.DatasourceReferences_FirestoreReference{
					FirestoreReference: &geminidataanalyticspb.FirestoreReference{
						DatabaseReference: &geminidataanalyticspb.FirestoreDatabaseReference{
							ProjectId:     "my-project",
							DatabaseId:    "my-database",
							CollectionIds: []string{"orders"},
						},
					},
				},
			}),
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
			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Fatalf("incorrect parse (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFailParseFromYaml(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "region is rejected outside spannerReference",
			in: `
			kind: tool
			name: my-gda-query-tool
			type: cloud-gemini-data-analytics-query
			source: gda-api-source
			description: Test Description
			location: us-central1
			context:
				datasourceReferences:
					bigtableReference:
						databaseReference:
							projectId:  "my-project"
							region:     "us-central1"
							instanceId: "my-instance"
			`,
			err: `unknown field "region"`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			if errStr := err.Error(); !strings.Contains(errStr, tc.err) {
				t.Fatalf("unexpected error string: got %q, want substring %q", errStr, tc.err)
			}
		})
	}
}

func TestParseFromYamlSpannerRegionWarning(t *testing.T) {
	const warning = "spannerReference.databaseReference.region"
	tcs := []struct {
		desc        string
		region      string
		wantWarning bool
	}{
		{desc: "warns when region is set", region: `region: "us-central1"`, wantWarning: true},
		{desc: "silent when region is unset", region: "", wantWarning: false},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			var logs bytes.Buffer
			logger, err := log.NewStdLogger(&logs, &logs, "info")
			if err != nil {
				t.Fatalf("unable to create logger: %s", err)
			}
			ctx := util.WithLogger(context.Background(), logger)
			in := fmt.Sprintf(`
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
							instanceId: "evalbench"
							databaseId: "financial"
							engine:     "GOOGLE_SQL"
							%s
			`, tc.region)
			if _, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(in)); err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if got := strings.Contains(logs.String(), warning); got != tc.wantWarning {
				t.Errorf("warning logged = %t, want %t; logs: %q", got, tc.wantWarning, logs.String())
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
