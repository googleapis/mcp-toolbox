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

package cloudstorage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
)

func TestShouldSweepGCSBucket(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	staleTime := now.Add(-orphanedBucketTTL - time.Second)
	freshTime := now.Add(-orphanedBucketTTL + time.Minute)
	exactBoundary := now.Add(-orphanedBucketTTL)

	tcs := []struct {
		name       string
		bucketName string
		created    time.Time
		want       bool
	}{
		{
			name:       "stale test bucket is swept",
			bucketName: "toolbox-it-stale-bucket",
			created:    staleTime,
			want:       true,
		},
		{
			name:       "fresh test bucket is kept",
			bucketName: "toolbox-it-fresh-bucket",
			created:    freshTime,
			want:       false,
		},
		{
			name:       "exact TTL boundary is kept",
			bucketName: "toolbox-it-boundary-bucket",
			created:    exactBoundary,
			want:       false,
		},
		{
			name:       "zero creation timestamp is kept",
			bucketName: "toolbox-it-zero-created",
			created:    time.Time{},
			want:       false,
		},
		{
			name:       "non-test bucket is never swept",
			bucketName: "prod-customer-assets",
			created:    staleTime,
			want:       false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSweepGCSBucket(tc.bucketName, tc.created, now); got != tc.want {
				t.Errorf("shouldSweepGCSBucket(%q, %v, %v) = %v, want %v", tc.bucketName, tc.created, now, got, tc.want)
			}
		})
	}
}

func TestIsGCSNotFound(t *testing.T) {
	tcs := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "ErrBucketNotExist", err: storage.ErrBucketNotExist, want: true},
		{name: "wrapped ErrObjectNotExist", err: fmt.Errorf("wrap: %w", storage.ErrObjectNotExist), want: true},
		{name: "googleapi 404", err: &googleapi.Error{Code: http.StatusNotFound, Message: "Not Found"}, want: true},
		{name: "wrapped googleapi 404", err: fmt.Errorf("wrap: %w", &googleapi.Error{Code: http.StatusNotFound}), want: true},
		{name: "googleapi 403", err: &googleapi.Error{Code: http.StatusForbidden, Message: "Forbidden"}, want: false},
		{name: "generic error", err: errors.New("boom"), want: false},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if got := isGCSNotFound(tc.err); got != tc.want {
				t.Errorf("isGCSNotFound(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

type fakeGCSAdminServer struct {
	mu                  sync.Mutex
	buckets             map[string]*fakeGCSBucket
	failObj             map[string]int
	events              []string
	lastObjectListQuery url.Values
}

type fakeGCSBucket struct {
	created time.Time
	objects map[string]bool
}

func newFakeGCSAdminServer(t *testing.T) *fakeGCSAdminServer {
	t.Helper()
	srv := &fakeGCSAdminServer{
		buckets: make(map[string]*fakeGCSBucket),
		failObj: make(map[string]int),
	}
	httpSrv := httptest.NewServer(http.HandlerFunc(srv.handle))
	t.Cleanup(httpSrv.Close)
	t.Setenv("STORAGE_EMULATOR_HOST", httpSrv.URL)
	return srv
}

func (f *fakeGCSAdminServer) writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": msg,
		},
	})
}

func (f *fakeGCSAdminServer) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	escapedPath := r.URL.EscapedPath()
	if escapedPath == "" {
		escapedPath = r.URL.Path
	}
	const base = "/storage/v1/b"

	// List buckets: GET /storage/v1/b?project=...&prefix=...
	if r.Method == http.MethodGet && (escapedPath == base || escapedPath == base+"/") {
		prefix := r.URL.Query().Get("prefix")
		var items []map[string]any
		for name, b := range f.buckets {
			if prefix != "" && !strings.HasPrefix(name, prefix) {
				continue
			}
			item := map[string]any{"name": name}
			if !b.created.IsZero() {
				item["timeCreated"] = b.created.UTC().Format(time.RFC3339Nano)
			}
			items = append(items, item)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
		return
	}

	rest, ok := strings.CutPrefix(escapedPath, base+"/")
	if !ok {
		f.writeJSONError(w, http.StatusNotFound, "unknown path")
		return
	}

	// Bucket-level DELETE: DELETE /storage/v1/b/{bucket}
	if !strings.Contains(rest, "/") && r.Method == http.MethodDelete {
		bucketName, _ := url.PathUnescape(rest)
		b, exists := f.buckets[bucketName]
		if !exists {
			f.writeJSONError(w, http.StatusNotFound, "bucket not found")
			return
		}
		if len(b.objects) > 0 {
			f.writeJSONError(w, http.StatusConflict, "The bucket you tried to delete is not empty")
			return
		}
		delete(f.buckets, bucketName)
		f.events = append(f.events, "delete-bucket:"+bucketName)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	bucketPart, subPath, ok := strings.Cut(rest, "/")
	if !ok {
		f.writeJSONError(w, http.StatusNotFound, "unknown bucket subresource")
		return
	}
	bucketName, _ := url.PathUnescape(bucketPart)
	b, exists := f.buckets[bucketName]
	if !exists {
		f.writeJSONError(w, http.StatusNotFound, "bucket not found")
		return
	}

	// List objects: GET /storage/v1/b/{bucket}/o
	if r.Method == http.MethodGet && subPath == "o" {
		f.lastObjectListQuery = r.URL.Query()
		var items []map[string]any
		for objName := range b.objects {
			items = append(items, map[string]any{"name": objName})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
		return
	}

	// Delete object: DELETE /storage/v1/b/{bucket}/o/{object}
	if objEscaped, ok := strings.CutPrefix(subPath, "o/"); ok && r.Method == http.MethodDelete {
		objName, _ := url.PathUnescape(objEscaped)
		if code, fail := f.failObj[bucketName+"/"+objName]; fail {
			f.writeJSONError(w, code, "injected object delete error")
			return
		}
		if !b.objects[objName] {
			f.writeJSONError(w, http.StatusNotFound, "object not found")
			return
		}
		delete(b.objects, objName)
		f.events = append(f.events, "delete-object:"+bucketName+"/"+objName)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	f.writeJSONError(w, http.StatusNotFound, "unhandled endpoint")
}

func TestDeleteGCSBucket(t *testing.T) {
	t.Run("missing bucket is treated as success", func(t *testing.T) {
		_ = newFakeGCSAdminServer(t)
		client := newGCSTestClient(t)
		if err := deleteGCSBucket(context.Background(), client, "toolbox-it-never-created"); err != nil {
			t.Fatalf("deleteGCSBucket on missing bucket returned error: %v", err)
		}
	})

	t.Run("empty bucket deletes directly with minimal list projection", func(t *testing.T) {
		srv := newFakeGCSAdminServer(t)
		srv.buckets["toolbox-it-empty"] = &fakeGCSBucket{
			created: time.Now().Add(-time.Hour),
			objects: map[string]bool{},
		}

		client := newGCSTestClient(t)
		if err := deleteGCSBucket(context.Background(), client, "toolbox-it-empty"); err != nil {
			t.Fatalf("deleteGCSBucket on empty bucket returned error: %v", err)
		}

		srv.mu.Lock()
		defer srv.mu.Unlock()
		if _, exists := srv.buckets["toolbox-it-empty"]; exists {
			t.Fatalf("expected empty bucket to be deleted")
		}
		if got := srv.lastObjectListQuery.Get("projection"); got != "noAcl" {
			t.Errorf("list projection = %q, want noAcl", got)
		}
		if got := srv.lastObjectListQuery.Get("fields"); !strings.Contains(got, "name") {
			t.Errorf("list fields = %q, want name selection", got)
		}
	})

	t.Run("single object deletes inline before deleting bucket", func(t *testing.T) {
		srv := newFakeGCSAdminServer(t)
		srv.buckets["toolbox-it-single"] = &fakeGCSBucket{
			created: time.Now().Add(-time.Hour),
			objects: map[string]bool{"seed/only.txt": true},
		}

		client := newGCSTestClient(t)
		if err := deleteGCSBucket(context.Background(), client, "toolbox-it-single"); err != nil {
			t.Fatalf("deleteGCSBucket on single-object bucket returned error: %v", err)
		}

		srv.mu.Lock()
		defer srv.mu.Unlock()
		want := []string{
			"delete-object:toolbox-it-single/seed/only.txt",
			"delete-bucket:toolbox-it-single",
		}
		if len(srv.events) != len(want) || srv.events[0] != want[0] || srv.events[1] != want[1] {
			t.Fatalf("events = %v, want %v", srv.events, want)
		}
	})

	t.Run("deletes all objects concurrently before deleting bucket", func(t *testing.T) {
		srv := newFakeGCSAdminServer(t)
		objs := make(map[string]bool)
		for i := range 20 {
			objs[fmt.Sprintf("seed/nested/file-%02d.txt", i)] = true
		}
		srv.buckets["toolbox-it-populated"] = &fakeGCSBucket{
			created: time.Now().Add(-time.Hour),
			objects: objs,
		}

		client := newGCSTestClient(t)
		if err := deleteGCSBucket(context.Background(), client, "toolbox-it-populated"); err != nil {
			t.Fatalf("deleteGCSBucket returned unexpected error: %v", err)
		}

		srv.mu.Lock()
		defer srv.mu.Unlock()
		if _, exists := srv.buckets["toolbox-it-populated"]; exists {
			t.Fatalf("expected bucket to be deleted")
		}
		if len(srv.events) != 21 {
			t.Fatalf("expected 20 object deletes + 1 bucket delete (21 events), got %d", len(srv.events))
		}
		if got := srv.events[len(srv.events)-1]; got != "delete-bucket:toolbox-it-populated" {
			t.Fatalf("expected bucket delete to occur last after all object deletes, got %q", got)
		}
	})

	t.Run("aggregates non-404 errors", func(t *testing.T) {
		srv := newFakeGCSAdminServer(t)
		srv.buckets["toolbox-it-broken"] = &fakeGCSBucket{
			created: time.Now().Add(-time.Hour),
			objects: map[string]bool{"bad.txt": true},
		}
		srv.failObj["toolbox-it-broken/bad.txt"] = http.StatusInternalServerError

		client := newGCSTestClient(t)
		err := deleteGCSBucket(context.Background(), client, "toolbox-it-broken")
		if err == nil {
			t.Fatalf("expected deleteGCSBucket to fail when object delete returns 500")
		}
		if !strings.Contains(err.Error(), "bad.txt") {
			t.Errorf("expected error to mention failed object bad.txt, got: %v", err)
		}
	})
}

func TestSweepOrphanedGCSBucketsWithClient(t *testing.T) {
	srv := newFakeGCSAdminServer(t)
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

	srv.buckets["toolbox-it-stale-1"] = &fakeGCSBucket{
		created: now.Add(-4 * time.Hour),
		objects: map[string]bool{"a.txt": true, "b.txt": true},
	}
	srv.buckets["toolbox-it-stale-2"] = &fakeGCSBucket{
		created: now.Add(-5 * time.Hour),
		objects: map[string]bool{},
	}
	srv.buckets["toolbox-it-fresh"] = &fakeGCSBucket{
		created: now.Add(-30 * time.Minute),
		objects: map[string]bool{"keep.txt": true},
	}
	srv.buckets["toolbox-it-zero-created"] = &fakeGCSBucket{
		created: time.Time{},
		objects: map[string]bool{},
	}
	srv.buckets["unrelated-prod-bucket"] = &fakeGCSBucket{
		created: now.Add(-48 * time.Hour),
		objects: map[string]bool{"important.txt": true},
	}

	client := newGCSTestClient(t)
	sweepOrphanedGCSBucketsWithClient(context.Background(), client, "test-project", now)

	srv.mu.Lock()
	defer srv.mu.Unlock()
	for _, swept := range []string{"toolbox-it-stale-1", "toolbox-it-stale-2"} {
		if _, exists := srv.buckets[swept]; exists {
			t.Errorf("expected stale bucket %q to be swept", swept)
		}
	}
	for _, kept := range []string{"toolbox-it-fresh", "toolbox-it-zero-created", "unrelated-prod-bucket"} {
		if _, exists := srv.buckets[kept]; !exists {
			t.Errorf("expected bucket %q to be preserved", kept)
		}
	}
}

func TestGCSCleanupLIFOOrdering(t *testing.T) {
	srv := newFakeGCSAdminServer(t)
	srv.buckets["toolbox-it-lifo"] = &fakeGCSBucket{
		created: time.Now(),
		objects: map[string]bool{"seed/hello.txt": true},
	}

	t.Run("subtest deletes bucket before closing client", func(t *testing.T) {
		client := newGCSTestClient(t)
		t.Cleanup(func() {
			srv.mu.Lock()
			srv.events = append(srv.events, "after-client-registered")
			srv.mu.Unlock()
		})
		registerGCSBucketCleanup(t, client, "toolbox-it-lifo")
	})

	srv.mu.Lock()
	defer srv.mu.Unlock()
	wantOrder := []string{
		"delete-object:toolbox-it-lifo/seed/hello.txt",
		"delete-bucket:toolbox-it-lifo",
		"after-client-registered",
	}
	if len(srv.events) != len(wantOrder) {
		t.Fatalf("events = %v, want %v", srv.events, wantOrder)
	}
	for i, want := range wantOrder {
		if srv.events[i] != want {
			t.Errorf("event[%d] = %q, want %q (all: %v)", i, srv.events[i], want, srv.events)
		}
	}
}

func TestOptimalGCSChunkSize(t *testing.T) {
	tcs := []struct {
		name string
		size int
		want int
	}{
		{name: "empty object uses minimum chunk size", size: 0, want: googleapi.MinUploadChunkSize},
		{name: "tiny object uses minimum chunk size", size: len(helloBody), want: googleapi.MinUploadChunkSize},
		{name: "exact 256 KiB boundary rounds above object size", size: googleapi.MinUploadChunkSize, want: 2 * googleapi.MinUploadChunkSize},
		{name: "large seed object stays below 16 MiB default", size: largeObjectSize, want: 33 * googleapi.MinUploadChunkSize},
		{name: "huge object caps at default 16 MiB chunk size", size: 32 << 20, want: googleapi.DefaultUploadChunkSize},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got := optimalGCSChunkSize(tc.size)
			if got != tc.want {
				t.Errorf("optimalGCSChunkSize(%d) = %d, want %d", tc.size, got, tc.want)
			}
			if got%googleapi.MinUploadChunkSize != 0 {
				t.Errorf("optimalGCSChunkSize(%d) = %d is not aligned to %d", tc.size, got, googleapi.MinUploadChunkSize)
			}
		})
	}
}
