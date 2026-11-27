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

package bigquery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	bigqueryapi "cloud.google.com/go/bigquery"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

func TestShouldSweepBigQueryDataset(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

	tcs := []struct {
		name      string
		datasetID string
		createdAt time.Time
		want      bool
	}{
		{
			name:      "stale toolbox_it_ dataset is swept",
			datasetID: "toolbox_it_abc123",
			createdAt: now.Add(-(orphanedDatasetTTL + time.Minute)),
			want:      true,
		},
		{
			name:      "stale legacy temp_toolbox_test_ dataset is swept",
			datasetID: "temp_toolbox_test_abc123",
			createdAt: now.Add(-(orphanedDatasetTTL + time.Hour)),
			want:      true,
		},
		{
			name:      "fresh toolbox_it_ dataset is kept",
			datasetID: "toolbox_it_fresh",
			createdAt: now.Add(-(orphanedDatasetTTL - time.Minute)),
			want:      false,
		},
		{
			name:      "exact TTL boundary is kept",
			datasetID: "toolbox_it_boundary",
			createdAt: now.Add(-orphanedDatasetTTL),
			want:      false,
		},
		{
			name:      "zero creation time is kept",
			datasetID: "toolbox_it_zero",
			createdAt: time.Time{},
			want:      false,
		},
		{
			name:      "non-test dataset is never swept regardless of age",
			datasetID: "analytics_production",
			createdAt: now.Add(-365 * 24 * time.Hour),
			want:      false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSweepBigQueryDataset(tc.datasetID, tc.createdAt, now); got != tc.want {
				t.Errorf("shouldSweepBigQueryDataset(%q, %v, %v) = %v, want %v", tc.datasetID, tc.createdAt, now, got, tc.want)
			}
		})
	}
}

func TestSweepOrphanedBigQueryDatasets(t *testing.T) {
	const (
		project        = "test-project"
		datasetsPrefix = "/projects/" + project + "/datasets"
	)
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

	creationTimes := map[string]time.Time{
		"toolbox_it_stale":         now.Add(-4 * time.Hour),
		"temp_toolbox_test_legacy": now.Add(-5 * time.Hour),
		"toolbox_it_fresh":         now.Add(-15 * time.Minute),
		"production_dataset":       now.Add(-48 * time.Hour),
	}

	var (
		mu              sync.Mutex
		metadataFetched []string
		deleted         []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == datasetsPrefix:
			resp := &bq.DatasetList{
				Datasets: []*bq.DatasetListDatasets{
					{DatasetReference: &bq.DatasetReference{ProjectId: project, DatasetId: "toolbox_it_stale"}},
					{DatasetReference: &bq.DatasetReference{ProjectId: project, DatasetId: "temp_toolbox_test_legacy"}},
					{DatasetReference: &bq.DatasetReference{ProjectId: project, DatasetId: "toolbox_it_fresh"}},
					{DatasetReference: &bq.DatasetReference{ProjectId: project, DatasetId: "production_dataset"}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, datasetsPrefix+"/"):
			if gotView := r.URL.Query().Get("datasetView"); gotView != string(bigqueryapi.DatasetMetadataView) {
				t.Errorf("expected datasetView=%q on %s, got %q", bigqueryapi.DatasetMetadataView, r.URL.Path, gotView)
			}
			datasetID := strings.TrimPrefix(r.URL.Path, datasetsPrefix+"/")
			mu.Lock()
			metadataFetched = append(metadataFetched, datasetID)
			mu.Unlock()
			created, ok := creationTimes[datasetID]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Not found"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(&bq.Dataset{
				DatasetReference: &bq.DatasetReference{ProjectId: project, DatasetId: datasetID},
				CreationTime:     created.UnixMilli(),
			})

		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, datasetsPrefix+"/"):
			if r.URL.Query().Get("deleteContents") != "true" {
				t.Errorf("expected deleteContents=true on %s, got query %q", r.URL.Path, r.URL.RawQuery)
			}
			datasetID := strings.TrimPrefix(r.URL.Path, datasetsPrefix+"/")
			mu.Lock()
			deleted = append(deleted, datasetID)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	client, err := bigqueryapi.NewClient(ctx, project, option.WithoutAuthentication(), option.WithEndpoint(srv.URL))
	if err != nil {
		t.Fatalf("bigquery.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	sweepOrphanedBigQueryDatasetsWithClient(ctx, client, now)

	if slices.Contains(metadataFetched, "production_dataset") {
		t.Errorf("sweeper fetched metadata for non-test dataset: %v", metadataFetched)
	}
	wantDeleted := []string{"toolbox_it_stale", "temp_toolbox_test_legacy"}
	if !slices.Equal(deleted, wantDeleted) {
		t.Errorf("deleted datasets = %v, want %v", deleted, wantDeleted)
	}
}

func TestDeleteBigQueryDataset(t *testing.T) {
	const project = "test-project"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/projects/" + project + "/datasets/missing_ds":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Not found"}}`))
		case "/projects/" + project + "/datasets/failing_ds":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":403,"message":"Access Denied"}}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	client, err := bigqueryapi.NewClient(ctx, project, option.WithoutAuthentication(), option.WithEndpoint(srv.URL))
	if err != nil {
		t.Fatalf("bigquery.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if err := deleteBigQueryDataset(ctx, client, "existing_ds"); err != nil {
		t.Errorf("deleteBigQueryDataset(existing_ds) unexpected error: %v", err)
	}
	if err := deleteBigQueryDataset(ctx, client, "missing_ds"); err != nil {
		t.Errorf("deleteBigQueryDataset(missing_ds) should ignore 404 NotFound, got: %v", err)
	}
	if err := deleteBigQueryDataset(ctx, client, "failing_ds"); err == nil {
		t.Errorf("deleteBigQueryDataset(failing_ds) = nil, want non-nil error on 403")
	}
}

func TestBigQueryCleanupLIFOOrder(t *testing.T) {
	const project = "test-project"

	var (
		mu      sync.Mutex
		actions []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		actions = append(actions, r.Method+" "+r.URL.Path)
		mu.Unlock()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/projects/"+project+"/datasets/ds1/tables":
			_ = json.NewEncoder(w).Encode(&bq.Table{
				TableReference: &bq.TableReference{ProjectId: project, DatasetId: "ds1", TableId: "v1"},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/projects/"+project+"/datasets/uncreated_ds":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Not found"}}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	client, err := bigqueryapi.NewClient(ctx, project, option.WithoutAuthentication(), option.WithEndpoint(srv.URL))
	if err != nil {
		t.Fatalf("bigquery.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	t.Run("subtest runs view cleanup before dataset cleanup and ignores uncreated dataset", func(t *testing.T) {
		registerBigQueryDatasetCleanup(t, client, "ds1", "uncreated_ds")
		setupBigQueryView(t, ctx, client, "ds1", "v1", "SELECT 1")
	})

	if len(actions) != 4 {
		t.Fatalf("cleanup actions = %v, want 4 actions", actions)
	}
	if actions[0] != "POST /projects/test-project/datasets/ds1/tables" {
		t.Errorf("actions[0] = %q, want table creation POST", actions[0])
	}
	if actions[1] != "DELETE /projects/test-project/datasets/ds1/tables/v1" {
		t.Errorf("actions[1] = %q, want view deleted before datasets", actions[1])
	}
	gotDatasetDeletes := slices.Clone(actions[2:])
	slices.Sort(gotDatasetDeletes)
	wantDatasetDeletes := []string{
		"DELETE /projects/test-project/datasets/ds1",
		"DELETE /projects/test-project/datasets/uncreated_ds",
	}
	if !slices.Equal(gotDatasetDeletes, wantDatasetDeletes) {
		t.Errorf("dataset delete actions = %v, want %v", gotDatasetDeletes, wantDatasetDeletes)
	}
}
