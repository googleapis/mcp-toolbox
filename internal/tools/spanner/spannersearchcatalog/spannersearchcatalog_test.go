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

package spannersearchcatalog_test

import (
	"context"
	"testing"

	dataplexapi "cloud.google.com/go/dataplex/apiv1"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/dataplex/searchcatalog"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/spanner/spannersearchcatalog"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlSpannerSearch(t *testing.T) {
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
            name: example_tool
            type: spanner-search-catalog
            source: my-instance
            description: some description
            `,
			want: server.ToolConfigs{
				"example_tool": spannersearchcatalog.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "spanner-search-catalog",
					Source: "my-instance",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
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

type mockSpannerSource struct {
	projectID              string
	useClientAuthorization bool
	searchResponse         []searchcatalog.DataplexSearchResponse
	err                    error
}

func (m mockSpannerSource) ProjectID() string            { return m.projectID }
func (m mockSpannerSource) UseClientAuthorization() bool { return m.useClientAuthorization }
func (m mockSpannerSource) GetCatalogClient(ctx context.Context, tokenString string) (*dataplexapi.CatalogClient, error) {
	return nil, nil
}
func (m mockSpannerSource) InvokeSearchCatalog(ctx context.Context, params map[string]any, tokenStr string) ([]searchcatalog.DataplexSearchResponse, error) {
	return m.searchResponse, m.err
}
func (m mockSpannerSource) IsReadOnly() bool               { return false }
func (m mockSpannerSource) SourceType() string             { return "spanner" }
func (m mockSpannerSource) ToConfig() sources.SourceConfig { return nil }

func TestConfig_Initialize(t *testing.T) {
	cfg := spannersearchcatalog.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test-tool",
			Description: "Test description",
		},
		Type:   "spanner-search-catalog",
		Source: "test-source",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if tool.GetName() != "test-tool" {
		t.Errorf("GetName() = %v, want %v", tool.GetName(), "test-tool")
	}

	if tool.GetDescription() != "Test description" {
		t.Errorf("GetDescription() = %v, want %v", tool.GetDescription(), "Test description")
	}
}

func TestTool_Invoke(t *testing.T) {
	ctx := context.Background()
	mockSource := mockSpannerSource{
		searchResponse: []searchcatalog.DataplexSearchResponse{
			{
				DataplexEntry: "test-entry",
			},
		},
	}

	cfg := spannersearchcatalog.Config{
		ConfigBase: tools.ConfigBase{
			Name: "test-tool",
		},
		Type:   "spanner-search-catalog",
		Source: "test-source",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	params := parameters.ParamValues{
		{
			Name:  "prompt",
			Value: "test prompt",
		},
	}

	resp, err := tool.Invoke(ctx, mockSource, params, tools.AccessToken(""))
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}

	results, ok := resp.([]searchcatalog.DataplexSearchResponse)
	if !ok {
		t.Fatalf("expected []searchcatalog.DataplexSearchResponse, got %T", resp)
	}

	if len(results) != 1 || results[0].DataplexEntry != "test-entry" {
		t.Errorf("unexpected results: %v", results)
	}
}
