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

package datalineagesearchlineage_test

import (
	"context"
	"strings"
	"testing"

	lineagepb "cloud.google.com/go/datacatalog/lineage/apiv1/lineagepb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/datalineage/datalineagesearchlineage"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlDatalineageSearchLineage(t *testing.T) {
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
			name: search_tool
			type: datalineage-search-lineage
			source: my-lineage
			description: Search lineage links
			`,
			want: server.ToolConfigs{
				"search_tool": datalineagesearchlineage.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "search_tool",
						Description:  "Search lineage links",
						AuthRequired: []string{},
					},
					Type:   "datalineage-search-lineage",
					Source: "my-lineage",
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
	called                bool
	parentLocation        string
	locations             []string
	rootEntities          []*lineagepb.EntityReference
	direction             lineagepb.SearchLineageStreamingRequest_SearchDirection
	maxDepth              int32
	maxResults            int32
	maxProcessPerLink     int32
	requestProcessDetails bool

	// Configurable returns
	retLinks       []*lineagepb.LineageLink
	retUnreachable []string
	retErr         error
}

func (m *mockSource) SearchLineageStreaming(
	ctx context.Context,
	parentLocation string,
	locations []string,
	rootEntities []*lineagepb.EntityReference,
	direction lineagepb.SearchLineageStreamingRequest_SearchDirection,
	maxDepth int32,
	maxResults int32,
	maxProcessPerLink int32,
	requestProcessDetails bool,
) ([]*lineagepb.LineageLink, []string, error) {
	m.called = true
	m.parentLocation = parentLocation
	m.locations = locations
	m.rootEntities = rootEntities
	m.direction = direction
	m.maxDepth = maxDepth
	m.maxResults = maxResults
	m.maxProcessPerLink = maxProcessPerLink
	m.requestProcessDetails = requestProcessDetails
	return m.retLinks, m.retUnreachable, m.retErr
}

func TestInvoke(t *testing.T) {
	cfg := datalineagesearchlineage.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "search_tool",
			Description: "Search",
		},
		Type:   "datalineage-search-lineage",
		Source: "my-lineage",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	tcs := []struct {
		desc                  string
		locations             any
		rootEntities          any
		direction             any
		maxDepth              any
		maxResults            any
		maxProcessPerLink     any
		requestProcessDetails any
		wantErr               bool
		wantSubstr            string
		wantCalled            bool
		wantParentLoc         string
		wantLocations         []string
		wantEntities          []*lineagepb.EntityReference
		wantDirection         lineagepb.SearchLineageStreamingRequest_SearchDirection
		wantMaxDepth          int32
		wantMaxResults        int32
		wantMaxProcess        int32
		wantRequestDetails    bool
		retLinks              []*lineagepb.LineageLink
		retUnreachable        []string
		retErr                error
		wantLinks             []*lineagepb.LineageLink
		wantUnreachable       []string
	}{
		{
			desc:         "missing locations",
			locations:    []any{},
			rootEntities: []any{map[string]any{"fully_qualified_name": "e"}},
			direction:    "UPSTREAM",
			wantErr:      true,
			wantSubstr:   "at least one location",
		},
		{
			desc:         "missing root_entities",
			locations:    []any{"us"},
			rootEntities: []any{},
			direction:    "UPSTREAM",
			wantErr:      true,
			wantSubstr:   "at least one root entity",
		},
		{
			desc:         "invalid direction",
			locations:    []any{"us"},
			rootEntities: []any{map[string]any{"fully_qualified_name": "e"}},
			direction:    "INVALID",
			wantErr:      true,
			wantSubstr:   "invalid direction",
		},
		{
			desc:      "happy path upstream with omitted limits (API defaults)",
			locations: []any{"us", "eu"},
			rootEntities: []any{
				map[string]any{
					"fully_qualified_name": "entity1",
					"fields":               []any{"f1", "f2"},
				},
			},
			direction:     "UPSTREAM",
			wantCalled:    true,
			wantParentLoc: "us",
			wantLocations: []string{"us", "eu"},
			wantEntities: []*lineagepb.EntityReference{
				{
					FullyQualifiedName: "entity1",
					Field:              []string{"f1", "f2"},
				},
			},
			wantDirection:      lineagepb.SearchLineageStreamingRequest_UPSTREAM,
			wantMaxDepth:       0,
			wantMaxResults:     0,
			wantMaxProcess:     0,
			wantRequestDetails: false,
		},
		{
			desc:      "happy path downstream with custom limits",
			locations: []any{"eu"},
			rootEntities: []any{
				map[string]any{
					"fully_qualified_name": "entity2",
				},
			},
			direction:         "DOWNSTREAM",
			maxDepth:          10,
			maxResults:        500,
			maxProcessPerLink: 5,
			wantCalled:        true,
			wantParentLoc:     "eu",
			wantLocations:     []string{"eu"},
			wantEntities: []*lineagepb.EntityReference{
				{
					FullyQualifiedName: "entity2",
				},
			},
			wantDirection:      lineagepb.SearchLineageStreamingRequest_DOWNSTREAM,
			wantMaxDepth:       10,
			wantMaxResults:     500,
			wantMaxProcess:     5,
			wantRequestDetails: false,
		},
		{
			desc:      "error when process details requested but max_process is 0 (omitted)",
			locations: []any{"us"},
			rootEntities: []any{
				map[string]any{
					"fully_qualified_name": "entity3",
				},
			},
			direction:             "UPSTREAM",
			requestProcessDetails: true,
			wantErr:               true,
			wantSubstr:            "max_process_per_link must be greater than 0",
		},
		{
			desc:      "happy path with process details and custom max_process",
			locations: []any{"us"},
			rootEntities: []any{
				map[string]any{
					"fully_qualified_name": "entity4",
				},
			},
			direction:             "UPSTREAM",
			maxProcessPerLink:     25,
			requestProcessDetails: true,
			wantCalled:            true,
			wantParentLoc:         "us",
			wantLocations:         []string{"us"},
			wantEntities: []*lineagepb.EntityReference{
				{
					FullyQualifiedName: "entity4",
				},
			},
			wantDirection:      lineagepb.SearchLineageStreamingRequest_UPSTREAM,
			wantMaxDepth:       0,
			wantMaxResults:     0,
			wantMaxProcess:     25,
			wantRequestDetails: true,
		},
		{
			desc:      "happy path with unreachable locations",
			locations: []any{"us", "eu"},
			rootEntities: []any{
				map[string]any{
					"fully_qualified_name": "entity1",
				},
			},
			direction: "UPSTREAM",
			retLinks: []*lineagepb.LineageLink{
				{
					Source: &lineagepb.EntityReference{FullyQualifiedName: "source1"},
					Target: &lineagepb.EntityReference{FullyQualifiedName: "target1"},
				},
			},
			retUnreachable: []string{"eu"},
			wantCalled:     true,
			wantParentLoc:  "us",
			wantLocations:  []string{"us", "eu"},
			wantEntities: []*lineagepb.EntityReference{
				{
					FullyQualifiedName: "entity1",
				},
			},
			wantDirection: lineagepb.SearchLineageStreamingRequest_UPSTREAM,
			wantLinks: []*lineagepb.LineageLink{
				{
					Source: &lineagepb.EntityReference{FullyQualifiedName: "source1"},
					Target: &lineagepb.EntityReference{FullyQualifiedName: "target1"},
				},
			},
			wantUnreachable: []string{"eu"},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			src := &mockSource{
				retLinks:       tc.retLinks,
				retUnreachable: tc.retUnreachable,
				retErr:         tc.retErr,
			}

			params := parameters.ParamValues{
				{Name: "locations", Value: tc.locations},
				{Name: "root_entities", Value: tc.rootEntities},
				{Name: "direction", Value: tc.direction},
			}
			if tc.maxDepth != nil {
				params = append(params, parameters.ParamValue{Name: "max_depth", Value: tc.maxDepth})
			}
			if tc.maxResults != nil {
				params = append(params, parameters.ParamValue{Name: "max_results", Value: tc.maxResults})
			}
			if tc.maxProcessPerLink != nil {
				params = append(params, parameters.ParamValue{Name: "max_process_per_link", Value: tc.maxProcessPerLink})
			}
			if tc.requestProcessDetails != nil {
				params = append(params, parameters.ParamValue{Name: "request_process_details", Value: tc.requestProcessDetails})
			}

			gotResp, toolErr := tool.Invoke(context.Background(), src, params, "")
			if tc.wantErr {
				if toolErr == nil {
					t.Fatalf("expected error, got nil")
				}
				if _, ok := toolErr.(*util.AgentError); !ok {
					t.Fatalf("expected *AgentError, got %T: %v", toolErr, toolErr)
				}
				if !strings.Contains(toolErr.Error(), tc.wantSubstr) {
					t.Errorf("error %q does not contain %q", toolErr, tc.wantSubstr)
				}
				if src.called {
					t.Errorf("expected source not to be called on validation failure")
				}
				return
			}
			if toolErr != nil {
				t.Fatalf("unexpected error: %v", toolErr)
			}

			resp, ok := gotResp.(datalineagesearchlineage.SearchLineageResponse)
			if !ok {
				t.Fatalf("expected SearchLineageResponse, got %T", gotResp)
			}
			if diff := cmp.Diff(tc.wantLinks, resp.Links, cmpopts.IgnoreUnexported(lineagepb.LineageLink{}, lineagepb.EntityReference{}, lineagepb.ProcessLinkInfo{})); diff != "" {
				t.Errorf("links diff: %s", diff)
			}
			if diff := cmp.Diff(tc.wantUnreachable, resp.Unreachable); diff != "" {
				t.Errorf("unreachable diff: %s", diff)
			}

			if src.called != tc.wantCalled {
				t.Errorf("called = %v, want %v", src.called, tc.wantCalled)
			}
			if src.parentLocation != tc.wantParentLoc {
				t.Errorf("parentLocation = %q, want %q", src.parentLocation, tc.wantParentLoc)
			}
			if diff := cmp.Diff(tc.wantLocations, src.locations); diff != "" {
				t.Errorf("locations diff: %s", diff)
			}
			if diff := cmp.Diff(tc.wantEntities, src.rootEntities, cmpopts.IgnoreUnexported(lineagepb.EntityReference{})); diff != "" {
				t.Errorf("rootEntities diff: %s", diff)
			}
			if src.direction != tc.wantDirection {
				t.Errorf("direction = %v, want %v", src.direction, tc.wantDirection)
			}
			if src.maxDepth != tc.wantMaxDepth {
				t.Errorf("maxDepth = %d, want %d", src.maxDepth, tc.wantMaxDepth)
			}
			if src.maxResults != tc.wantMaxResults {
				t.Errorf("maxResults = %d, want %d", src.maxResults, tc.wantMaxResults)
			}
			if src.maxProcessPerLink != tc.wantMaxProcess {
				t.Errorf("maxProcessPerLink = %d, want %d", src.maxProcessPerLink, tc.wantMaxProcess)
			}
			if src.requestProcessDetails != tc.wantRequestDetails {
				t.Errorf("requestProcessDetails = %v, want %v", src.requestProcessDetails, tc.wantRequestDetails)
			}
		})
	}
}
