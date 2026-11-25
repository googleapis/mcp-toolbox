// Copyright 2025 Google LLC
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

package bigquerysearchcatalog_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	bigqueryds "github.com/googleapis/mcp-toolbox/internal/sources/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerysearchcatalog"
)

func TestAllowedDatasetsManifest(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	cfg := bigquerysearchcatalog.Config{
		ConfigBase: tools.ConfigBase{Name: "search", Description: "Search catalog"},
		Type:       "bigquery-search-catalog",
		Source:     "my-bq-source",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	src := &bigqueryds.Source{
		AllowedDatasets: map[string]struct{}{
			"p2.shared":  {},
			"p1.allowed": {},
		},
	}

	params, err := tool.GetParameters(src)
	if err != nil {
		t.Fatalf("GetParameters() unexpected error: %v", err)
	}
	descriptions := map[string]string{}
	for _, p := range params {
		descriptions[p.GetName()] = p.GetDesc()
	}
	for _, name := range []string{"projectIds", "datasetIds"} {
		if !strings.Contains(descriptions[name], "`p1.allowed`, `p2.shared`") {
			t.Errorf("%s description = %q, want the sorted allowlist", name, descriptions[name])
		}
	}
	if !strings.Contains(descriptions["types"], "DATASET, MODEL, ROUTINE, TABLE, VIEW") ||
		!strings.Contains(descriptions["types"], "CONNECTION and POLICY are excluded") {
		t.Errorf("types description = %q, want dataset-scoped types and the exclusion", descriptions["types"])
	}

	manifest, err := tool.Manifest(src)
	if err != nil {
		t.Fatalf("Manifest() unexpected error: %v", err)
	}
	foundDatasetDescription := false
	for _, p := range manifest.Parameters {
		if p.Name == "datasetIds" {
			foundDatasetDescription = strings.Contains(p.Description, "`p1.allowed`, `p2.shared`")
		}
	}
	if !foundDatasetDescription {
		t.Error("Manifest() datasetIds description does not contain the sorted allowlist")
	}
}

func TestParseFromYamlBigQuerySearch(t *testing.T) {
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
            type: bigquery-search-catalog
            source: my-instance
            description: some description
            `,
			want: server.ToolConfigs{
				"example_tool": bigquerysearchcatalog.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "bigquery-search-catalog",
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
