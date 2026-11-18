// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigqueryconversationalanalytics_test

import (
	"context"
	"strings"
	"testing"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigqueryconversationalanalytics"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"golang.org/x/oauth2"
)

type stubConversationalSource struct {
	allowedDatasets []string
}

func (s stubConversationalSource) SourceType() string                  { return "bigquery" }
func (s stubConversationalSource) ToConfig() sources.SourceConfig      { return nil }
func (s stubConversationalSource) IsReadOnly() bool                    { return true }
func (s stubConversationalSource) BigQueryClient() *bigqueryapi.Client { return nil }
func (s stubConversationalSource) BigQueryTokenSourceWithScope(context.Context, []string) (oauth2.TokenSource, error) {
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"}), nil
}
func (s stubConversationalSource) BigQueryProject() string           { return "proj" }
func (s stubConversationalSource) BigQueryLocation() string          { return "us" }
func (s stubConversationalSource) BigQueryQuotaProject() string      { return "" }
func (s stubConversationalSource) GetMaxQueryResultRows() int        { return 50 }
func (s stubConversationalSource) UseClientAuthorization() bool      { return false }
func (s stubConversationalSource) GetAuthTokenHeaderName() string    { return "" }
func (s stubConversationalSource) BigQueryAllowedDatasets() []string { return s.allowedDatasets }
func (s stubConversationalSource) IsDatasetAllowed(projectID, datasetID string) bool {
	target := projectID + "." + datasetID
	for _, ds := range s.allowedDatasets {
		if ds == target {
			return true
		}
	}
	return false
}

func TestParseFromYamlBigQueryConversationalAnalytics(t *testing.T) {
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
            type: bigquery-conversational-analytics
            source: my-instance
            description: some description
            `,
			want: server.ToolConfigs{
				"example_tool": bigqueryconversationalanalytics.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "bigquery-conversational-analytics",
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

func TestInvokeAllowedDatasetsRejectsEmptyTableReferences(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	cfg := bigqueryconversationalanalytics.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "ca_restricted",
			Description: "Restricted conversational analytics",
		},
		Type:   "bigquery-conversational-analytics",
		Source: "my-bq",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	src := stubConversationalSource{allowedDatasets: []string{"proj.allowed_ds"}}

	for _, tc := range []struct {
		name      string
		tableRefs string
		wantErr   string
	}{
		{
			name:      "empty string table_references rejected",
			tableRefs: "",
			wantErr:   "at least one table reference in 'table_references' is required",
		},
		{
			name:      "empty JSON array table_references rejected",
			tableRefs: "[]",
			wantErr:   "at least one table reference in 'table_references' is required",
		},
		{
			name:      "disallowed dataset rejected",
			tableRefs: `[{"projectId":"proj","datasetId":"secret_ds","tableId":"users"}]`,
			wantErr:   "access to dataset 'proj.secret_ds' (from table 'users') is not allowed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := parameters.ParamValues{
				{Name: "user_query_with_context", Value: "Show me all rows"},
				{Name: "table_references", Value: tc.tableRefs},
			}
			_, tbErr := tool.Invoke(ctx, src, params, "")
			if tbErr == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(tbErr.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", tbErr.Error(), tc.wantErr)
			}
		})
	}
}
