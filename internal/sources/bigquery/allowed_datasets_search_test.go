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

package bigquery

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/sources/dataplex/searchcatalog"
)

func TestBigQueryDatasetFromResource(t *testing.T) {
	tests := []struct {
		name        string
		resource    string
		wantProject string
		wantDataset string
		wantOK      bool
	}{
		{"table", "//bigquery.googleapis.com/projects/p/datasets/d/tables/t", "p", "d", true},
		{"view", "//bigquery.googleapis.com/projects/p/datasets/d/tables/v", "p", "d", true},
		{"dataset only", "//bigquery.googleapis.com/projects/p/datasets/d", "p", "d", true},
		{"connection is not dataset-scoped", "//bigquery.googleapis.com/projects/p/connections/c", "", "", false},
		{"project-level policy is not dataset-scoped", "//bigquery.googleapis.com/projects/p/policies/pol", "", "", false},
		{"project only", "//bigquery.googleapis.com/projects/p", "", "", false},
		{"non-bigquery resource", "//storage.googleapis.com/projects/p/buckets/b", "", "", false},
		{"empty project segment", "//bigquery.googleapis.com/projects//datasets/d", "", "", false},
		{"empty dataset segment", "//bigquery.googleapis.com/projects/p/datasets/", "", "", false},
		{"empty", "", "", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotProject, gotDataset, gotOK := bigQueryDatasetFromResource(tc.resource)
			if gotProject != tc.wantProject || gotDataset != tc.wantDataset || gotOK != tc.wantOK {
				t.Errorf("bigQueryDatasetFromResource(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.resource, gotProject, gotDataset, gotOK, tc.wantProject, tc.wantDataset, tc.wantOK)
			}
		})
	}
}

func bqSearchResult(project, dataset, table string) searchcatalog.DataplexSearchResponse {
	return searchcatalog.DataplexSearchResponse{
		DisplayName: table,
		Resource:    "//bigquery.googleapis.com/projects/" + project + "/datasets/" + dataset + "/tables/" + table,
	}
}

func TestFilterSearchCatalogByAllowedDatasets(t *testing.T) {
	allowed := bqSearchResult("proj", "allowed_ds", "customers")
	forbidden := bqSearchResult("proj", "secret_ds", "customers")
	connection := searchcatalog.DataplexSearchResponse{
		DisplayName: "conn",
		Resource:    "//bigquery.googleapis.com/projects/proj/connections/c",
	}
	all := []searchcatalog.DataplexSearchResponse{allowed, forbidden, connection}

	tests := []struct {
		name            string
		allowedDatasets map[string]struct{}
		results         []searchcatalog.DataplexSearchResponse
		want            []searchcatalog.DataplexSearchResponse
	}{
		{
			name:            "no allowlist returns everything",
			allowedDatasets: nil,
			results:         all,
			want:            all,
		},
		{
			name:            "allowlist keeps only the allowed dataset and drops non-dataset entries",
			allowedDatasets: map[string]struct{}{"proj.allowed_ds": {}},
			results:         all,
			want:            []searchcatalog.DataplexSearchResponse{allowed},
		},
		{
			name:            "allowlist drops everything when nothing matches",
			allowedDatasets: map[string]struct{}{"proj.other_ds": {}},
			results:         all,
			want:            []searchcatalog.DataplexSearchResponse{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &Source{AllowedDatasets: tc.allowedDatasets}
			got := s.filterSearchCatalogByAllowedDatasets(tc.results)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("filterSearchCatalogByAllowedDatasets() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
