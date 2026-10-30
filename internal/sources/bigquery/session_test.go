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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	bigqueryrestapi "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

type jobInsertPayload struct {
	JobReference struct {
		ProjectId string `json:"projectId"`
		Location  string `json:"location"`
	} `json:"jobReference"`
	Configuration struct {
		DryRun bool `json:"dryRun"`
		Query  struct {
			Query                string `json:"query"`
			UseLegacySql         *bool  `json:"useLegacySql"`
			CreateSession        bool   `json:"createSession"`
			ConnectionProperties []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"connectionProperties"`
		} `json:"query"`
	} `json:"configuration"`
}

func writeLegacySQLError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`{"error":{"code":400,"message":"Legacy SQL query is not supported for this project. Consider using GoogleSQL."}}`))
}

func TestNewBigQuerySessionProvider_LifecycleAndGoogleSQL(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("ContextWithNewLogger() unexpected error: %v", err)
	}

	var createCalls atomic.Int32
	var validateCalls atomic.Int32
	var failValidation atomic.Bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/projects/test-project/jobs") {
			http.Error(w, fmt.Sprintf("unexpected request: %s %s", r.Method, r.URL.Path), http.StatusNotFound)
			return
		}

		var payload jobInsertPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, fmt.Sprintf("decode body: %v", err), http.StatusBadRequest)
			return
		}

		if !payload.Configuration.DryRun {
			http.Error(w, "expected dryRun=true", http.StatusBadRequest)
			return
		}
		if payload.Configuration.Query.Query != "SELECT 1" {
			http.Error(w, fmt.Sprintf("unexpected query %q", payload.Configuration.Query.Query), http.StatusBadRequest)
			return
		}
		// Reject omitted (nil) or true useLegacySql, matching projects with Legacy SQL disabled.
		if payload.Configuration.Query.UseLegacySql == nil || *payload.Configuration.Query.UseLegacySql {
			writeLegacySQLError(w)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if payload.Configuration.Query.CreateSession {
			if payload.JobReference.ProjectId != "test-project" || payload.JobReference.Location != "US" {
				http.Error(w, fmt.Sprintf("unexpected jobReference: %+v", payload.JobReference), http.StatusBadRequest)
				return
			}
			n := createCalls.Add(1)
			resp := &bigqueryrestapi.Job{
				Status: &bigqueryrestapi.JobStatus{State: "DONE"},
				Statistics: &bigqueryrestapi.JobStatistics{
					SessionInfo: &bigqueryrestapi.SessionInfo{
						SessionId: fmt.Sprintf("session-%d", n),
					},
				},
				Configuration: &bigqueryrestapi.JobConfiguration{
					Query: &bigqueryrestapi.JobConfigurationQuery{
						DestinationTable: &bigqueryrestapi.TableReference{
							ProjectId: "test-project",
							DatasetId: fmt.Sprintf("_session_ds_%d", n),
							TableId:   "anon_table",
						},
					},
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Errorf("encode create session response: %v", err)
			}
			return
		}

		validateCalls.Add(1)
		var gotSessionID string
		for _, prop := range payload.Configuration.Query.ConnectionProperties {
			if prop.Key == "session_id" {
				gotSessionID = prop.Value
			}
		}
		if gotSessionID == "" {
			http.Error(w, "missing session_id connection property on validation", http.StatusBadRequest)
			return
		}
		if failValidation.Load() {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":400,"message":"Session expired"}}`))
			return
		}
		if err := json.NewEncoder(w).Encode(&bigqueryrestapi.Job{Status: &bigqueryrestapi.JobStatus{State: "DONE"}}); err != nil {
			t.Errorf("encode validate session response: %v", err)
		}
	}))
	defer ts.Close()

	restService, err := bigqueryrestapi.NewService(ctx, option.WithoutAuthentication(), option.WithEndpoint(ts.URL))
	if err != nil {
		t.Fatalf("bigqueryrestapi.NewService() unexpected error: %v", err)
	}

	src := &Source{
		Config: Config{
			Name:      "test-bq",
			Type:      SourceType,
			Project:   "test-project",
			Location:  "US",
			WriteMode: WriteModeProtected,
		},
		RestService: restService,
	}
	provider := src.newBigQuerySessionProvider()

	// 1. Initial session creation uses GoogleSQL (useLegacySql=false).
	sess1, err := provider(ctx)
	if err != nil {
		t.Fatalf("initial provider() unexpected error: %v", err)
	}
	if sess1 == nil {
		t.Fatal("initial provider() returned nil session")
	}
	if sess1.ID != "session-1" || sess1.ProjectID != "test-project" || sess1.DatasetID != "_session_ds_1" {
		t.Fatalf("unexpected initial session: %+v", sess1)
	}
	if createCalls.Load() != 1 || validateCalls.Load() != 0 {
		t.Fatalf("counts after initial create: create=%d validate=%d, want 1 and 0", createCalls.Load(), validateCalls.Load())
	}
	firstLastUsed := sess1.LastUsed

	// 2. Cached session validation uses GoogleSQL and reuses existing session.
	sess2, err := provider(ctx)
	if err != nil {
		t.Fatalf("cached provider() unexpected error: %v", err)
	}
	if sess2.ID != "session-1" {
		t.Fatalf("cached session ID = %q, want %q", sess2.ID, "session-1")
	}
	if sess2.LastUsed.Before(firstLastUsed) {
		t.Errorf("LastUsed did not advance: got %v, before %v", sess2.LastUsed, firstLastUsed)
	}
	if createCalls.Load() != 1 || validateCalls.Load() != 1 {
		t.Fatalf("counts after cached reuse: create=%d validate=%d, want 1 and 1", createCalls.Load(), validateCalls.Load())
	}

	// 3. Expired session validation failure falls back to creating a new GoogleSQL session.
	failValidation.Store(true)
	sess3, err := provider(ctx)
	if err != nil {
		t.Fatalf("fallback provider() unexpected error: %v", err)
	}
	if sess3.ID != "session-2" || sess3.DatasetID != "_session_ds_2" {
		t.Fatalf("unexpected fallback session: %+v", sess3)
	}
	if createCalls.Load() != 2 || validateCalls.Load() != 2 {
		t.Fatalf("counts after fallback re-create: create=%d validate=%d, want 2 and 2", createCalls.Load(), validateCalls.Load())
	}

	// 4. Approaching 7-day lifetime skips validation and creates a fresh GoogleSQL session.
	failValidation.Store(false)
	src.Session.CreationTime = time.Now().Add(-(7*24*time.Hour - 15*time.Minute))
	sess4, err := provider(ctx)
	if err != nil {
		t.Fatalf("lifetime refresh provider() unexpected error: %v", err)
	}
	if sess4.ID != "session-3" || sess4.DatasetID != "_session_ds_3" {
		t.Fatalf("unexpected refreshed session: %+v", sess4)
	}
	if createCalls.Load() != 3 || validateCalls.Load() != 2 {
		t.Fatalf("counts after lifetime refresh: create=%d validate=%d, want 3 and 2", createCalls.Load(), validateCalls.Load())
	}
}

func TestNewBigQuerySessionProvider_NonProtectedModes(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("ContextWithNewLogger() unexpected error: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP call in non-protected mode: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	restService, err := bigqueryrestapi.NewService(ctx, option.WithoutAuthentication(), option.WithEndpoint(ts.URL))
	if err != nil {
		t.Fatalf("bigqueryrestapi.NewService() unexpected error: %v", err)
	}

	for _, mode := range []string{WriteModeAllowed, WriteModeBlocked} {
		t.Run(mode, func(t *testing.T) {
			src := &Source{
				Config:      Config{Project: "test-project", WriteMode: mode},
				RestService: restService,
			}
			sess, err := src.newBigQuerySessionProvider()(ctx)
			if err != nil {
				t.Fatalf("provider() unexpected error: %v", err)
			}
			if sess != nil {
				t.Fatalf("provider() = %+v, want nil", sess)
			}
		})
	}
}

func TestNewBigQuerySessionProvider_CreateSessionError(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("ContextWithNewLogger() unexpected error: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeLegacySQLError(w)
	}))
	defer ts.Close()

	restService, err := bigqueryrestapi.NewService(ctx, option.WithoutAuthentication(), option.WithEndpoint(ts.URL))
	if err != nil {
		t.Fatalf("bigqueryrestapi.NewService() unexpected error: %v", err)
	}

	src := &Source{
		Config: Config{
			Project:   "test-project",
			Location:  "US",
			WriteMode: WriteModeProtected,
		},
		RestService: restService,
	}

	sess, err := src.newBigQuerySessionProvider()(ctx)
	if err == nil {
		t.Fatal("expected error from failed session creation, got nil")
	}
	if !strings.Contains(err.Error(), "failed to create new session") {
		t.Errorf("error = %q, want substring %q", err.Error(), "failed to create new session")
	}
	if sess != nil || src.Session != nil {
		t.Errorf("expected nil session on error, got sess=%+v src.Session=%+v", sess, src.Session)
	}
}
