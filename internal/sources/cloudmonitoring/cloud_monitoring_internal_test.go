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

package cloudmonitoring

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestRunQueryProjectID(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()
	s := &Source{baseURL: srv.URL, client: srv.Client()}
	// takePaths returns the request paths received since the previous call.
	takePaths := func() []string {
		mu.Lock()
		defer mu.Unlock()
		got := paths
		paths = nil
		return got
	}

	for _, projectID := range []string{"my-project", "example.com:my-project", "123456789012"} {
		t.Run("valid "+projectID, func(t *testing.T) {
			_, err := s.RunQuery(projectID, "up")
			got := takePaths()
			if err != nil {
				t.Fatalf("RunQuery(%q) unexpected error: %v", projectID, err)
			}
			want := "/v1/projects/" + projectID + "/location/global/prometheus/api/v1/query"
			if len(got) != 1 || got[0] != want {
				t.Fatalf("RunQuery(%q) requested %q, want [%q]", projectID, got, want)
			}
		})
	}

	for _, projectID := range []string{
		"",
		"..",
		"../../v3/projects/victim/alertPolicies?",
		"my-project/location/global/prometheus/api/v1/labels#",
		"my-project?x=1",
		"my project",
		"my-project%2F..",
	} {
		t.Run("invalid "+projectID, func(t *testing.T) {
			_, err := s.RunQuery(projectID, "up")
			got := takePaths()
			if err == nil || !strings.Contains(err.Error(), "invalid projectId") {
				t.Fatalf("RunQuery(%q) error = %v, want invalid projectId error", projectID, err)
			}
			if len(got) != 0 {
				t.Fatalf("RunQuery(%q) sent requests to %q, want none", projectID, got)
			}
		})
	}
}
