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

package conversationalanalyticslistaccessibledataagents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func intPtr(v int) *int { return &v }

type fakeDataAgentsAPI struct {
	total       int
	alwaysToken bool

	mu        sync.Mutex
	queries   []url.Values
	apiClient []string
}

func (f *fakeDataAgentsAPI) serve(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		f.mu.Lock()
		f.queries = append(f.queries, query)
		f.apiClient = append(f.apiClient, r.Header.Get("X-Goog-API-Client"))
		callNumber := len(f.queries)
		f.mu.Unlock()

		size := 100
		if v := query.Get("pageSize"); v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil {
				http.Error(w, "invalid pageSize", http.StatusBadRequest)
				return
			}
			size = parsed
		}
		start := 0
		if v := query.Get("pageToken"); v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil {
				http.Error(w, "invalid pageToken", http.StatusBadRequest)
				return
			}
			start = parsed
		}

		end := min(start+size, f.total)
		agents := []map[string]any{}
		for i := start; i < end; i++ {
			agents = append(agents, map[string]any{
				"name":         fmt.Sprintf("projects/my-project/locations/global/dataAgents/agent-%d", i),
				"displayName":  fmt.Sprintf("agent-%d", i),
				"someNewField": "kept",
			})
		}

		body := map[string]any{"dataAgents": agents}
		if f.alwaysToken {
			body["nextPageToken"] = strconv.Itoa(callNumber)
		} else if end < f.total {
			body["nextPageToken"] = strconv.Itoa(end)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("unable to write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func (f *fakeDataAgentsAPI) recordedQueries() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.queries...)
}

type decodedResult struct {
	DataAgents []struct {
		DisplayName  string `json:"displayName"`
		SomeNewField string `json:"someNewField"`
	} `json:"dataAgents"`
	NextPageToken string `json:"nextPageToken"`
}

func decodeResult(t *testing.T, result any) decodedResult {
	t.Helper()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("unable to marshal result: %v", err)
	}
	var decoded decodedResult
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unable to unmarshal result %s: %v", encoded, err)
	}
	return decoded
}

func displayNames(t *testing.T, result any) ([]string, string) {
	t.Helper()
	decoded := decodeResult(t, result)
	names := make([]string, 0, len(decoded.DataAgents))
	for _, agent := range decoded.DataAgents {
		names = append(names, agent.DisplayName)
	}
	return names, decoded.NextPageToken
}

func TestListAccessibleDataAgentsDrainsEveryPageByDefault(t *testing.T) {
	api := &fakeDataAgentsAPI{total: 250}
	server := api.serve(t)

	result, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded := decodeResult(t, result)
	if len(decoded.DataAgents) != 250 {
		t.Fatalf("got %d data agents, want every one of the 250 available", len(decoded.DataAgents))
	}
	for _, agent := range decoded.DataAgents {
		if agent.SomeNewField != "kept" {
			t.Errorf("data agent %q lost its unknown fields", agent.DisplayName)
		}
	}
	if decoded.DataAgents[0].DisplayName != "agent-0" || decoded.DataAgents[249].DisplayName != "agent-249" {
		t.Errorf("got data agents from %q to %q, want agent-0 to agent-249", decoded.DataAgents[0].DisplayName, decoded.DataAgents[249].DisplayName)
	}
	if decoded.NextPageToken != "" {
		t.Errorf("got nextPageToken %q, want none once every page has been read", decoded.NextPageToken)
	}

	// The default drain sends no pageSize, leaving the page size to the API.
	wantQueries := []url.Values{
		{},
		{"pageToken": {"100"}},
		{"pageToken": {"200"}},
	}
	if diff := cmp.Diff(wantQueries, api.recordedQueries()); diff != "" {
		t.Errorf("unexpected requests: diff %v", diff)
	}
}

func TestListAccessibleDataAgentsDrainReturnsEmptyList(t *testing.T) {
	api := &fakeDataAgentsAPI{total: 0}
	server := api.serve(t)

	result, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatalf("unable to marshal result: %v", marshalErr)
	}
	if got, want := string(encoded), `{"dataAgents":[]}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestListAccessibleDataAgentsDrainStopsAtDataAgentLimit(t *testing.T) {
	api := &fakeDataAgentsAPI{total: maxAutoDataAgents * 3}
	server := api.serve(t)

	result, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names, nextPageToken := displayNames(t, result)
	if len(names) != maxAutoDataAgents {
		t.Errorf("got %d data agents, want the %d the drain is capped at", len(names), maxAutoDataAgents)
	}
	if nextPageToken == "" {
		t.Error("got no nextPageToken, want the token the drain stopped at")
	}
	if got, want := len(api.recordedQueries()), 10; got != want {
		t.Errorf("got %d requests, want %d", got, want)
	}
}

func TestListAccessibleDataAgentsDrainAtExactLimitHasNoToken(t *testing.T) {
	api := &fakeDataAgentsAPI{total: maxAutoDataAgents}
	server := api.serve(t)

	result, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names, nextPageToken := displayNames(t, result)
	if len(names) != maxAutoDataAgents {
		t.Errorf("got %d data agents, want %d", len(names), maxAutoDataAgents)
	}
	if nextPageToken != "" {
		t.Errorf("got nextPageToken %q, want none when the last page lands exactly on the cap", nextPageToken)
	}
}

func TestListAccessibleDataAgentsDrainStopsAtPageLimit(t *testing.T) {
	api := &fakeDataAgentsAPI{total: 0, alwaysToken: true}
	server := api.serve(t)

	result, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names, nextPageToken := displayNames(t, result)
	if len(names) != 0 {
		t.Errorf("got %d data agents, want none", len(names))
	}
	if nextPageToken == "" {
		t.Error("got no nextPageToken, want the token the drain stopped at")
	}
	if got := len(api.recordedQueries()); got != maxAutoPages {
		t.Errorf("got %d requests, want the %d page guard to stop the drain", got, maxAutoPages)
	}
}

func TestListAccessibleDataAgentsDrainStopsOnRepeatedToken(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"dataAgents": [{"displayName": "agent-0"}], "nextPageToken": "stuck"}`)
	}))
	defer server.Close()

	result, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Errorf("got %d requests, want the drain to stop as soon as the token repeats", got)
	}
	names, nextPageToken := displayNames(t, result)
	if len(names) > 2 {
		t.Errorf("got %d data agents, want the drain to stop before it piles up duplicates", len(names))
	}
	if nextPageToken != "" {
		t.Errorf("got nextPageToken %q, want none: handing back a token the API ignores would loop the caller", nextPageToken)
	}
}

func TestListAccessibleDataAgentsDrainMidwayFailureReturnsError(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"dataAgents": [{"displayName": "agent-0"}], "nextPageToken": "page-2"}`)
			return
		}
		http.Error(w, `{"error": {"message": "upstream failure"}}`, http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", nil, "")
	if err == nil {
		t.Fatal("expected an error when a later page fails, got nil")
	}
	if !strings.Contains(err.Error(), "upstream failure") {
		t.Errorf("error %q does not mention the upstream failure", err.Error())
	}
}

// Top-level fields the tool does not model, such as the partial-failure
// `unreachable` list, must survive the drain whether they appear on the first
// page or the last page.
func TestListAccessibleDataAgentsDrainKeepsUnknownTopLevelFields(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			fmt.Fprint(w, `{"dataAgents": [{"displayName": "agent-0"}], "nextPageToken": "page-2", "firstPageExtra": "kept"}`)
			return
		}
		fmt.Fprint(w, `{"dataAgents": [{"displayName": "agent-1"}], "unreachable": ["locations/us-east1"]}`)
	}))
	defer server.Close()

	result, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatalf("unable to marshal result: %v", marshalErr)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unable to unmarshal result %s: %v", encoded, err)
	}

	if got, want := decoded["firstPageExtra"], "kept"; got != want {
		t.Errorf("firstPageExtra = %v, want %v (extras from earlier pages must be preserved)", got, want)
	}
	want := []any{"locations/us-east1"}
	if diff := cmp.Diff(want, decoded["unreachable"]); diff != "" {
		t.Errorf("the drain dropped the unreachable field: diff %v", diff)
	}
	if _, ok := decoded["nextPageToken"]; ok {
		t.Errorf("got a nextPageToken in %s, want none once every page has been read", encoded)
	}
}

// A partial-failure list reported on more than one page must be unioned, not
// overwritten: dropping an earlier page's entries would overstate the coverage.
func TestListAccessibleDataAgentsDrainMergesRepeatedUnknownFields(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			fmt.Fprint(w, `{"dataAgents": [], "nextPageToken": "page-2", "unreachable": ["locations/us-east1"], "scalarExtra": 1}`)
			return
		}
		fmt.Fprint(w, `{"dataAgents": [], "unreachable": ["locations/eu-west1"], "scalarExtra": 2}`)
	}))
	defer server.Close()

	result, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatalf("unable to marshal result: %v", marshalErr)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unable to unmarshal result %s: %v", encoded, err)
	}

	want := []any{"locations/us-east1", "locations/eu-west1"}
	if diff := cmp.Diff(want, decoded["unreachable"]); diff != "" {
		t.Errorf("the drain lost an unreachable location: diff %v", diff)
	}
	// Non-array fields cannot be merged, so the last page still wins.
	if got, want := decoded["scalarExtra"], float64(2); got != want {
		t.Errorf("scalarExtra = %v, want %v", got, want)
	}
}

// concatJSONArrays must never trade an aggregated list for a value it cannot
// merge: an earlier page's entries are worth more than one unmergeable page.
func TestConcatJSONArrays(t *testing.T) {
	for _, tc := range []struct {
		desc string
		prev json.RawMessage
		next json.RawMessage
		want json.RawMessage
	}{
		{desc: "no prev", prev: nil, next: json.RawMessage(`["a"]`), want: json.RawMessage(`["a"]`)},
		{desc: "both arrays", prev: json.RawMessage(`["a"]`), next: json.RawMessage(`["b"]`), want: json.RawMessage(`["a","b"]`)},
		{desc: "null next keeps prev", prev: json.RawMessage(`["a"]`), next: json.RawMessage(`null`), want: json.RawMessage(`["a"]`)},
		{desc: "null prev loses to scalar next", prev: json.RawMessage(`null`), next: json.RawMessage(`2`), want: json.RawMessage(`2`)},
		{desc: "null prev loses to empty array next", prev: json.RawMessage(`null`), next: json.RawMessage(`[]`), want: json.RawMessage(`[]`)},
		{desc: "scalar prev loses to next", prev: json.RawMessage(`1`), next: json.RawMessage(`2`), want: json.RawMessage(`2`)},
		{desc: "scalar prev kept when next is empty", prev: json.RawMessage(`1`), next: nil, want: json.RawMessage(`1`)},
		{desc: "scalar next keeps aggregated prev", prev: json.RawMessage(`["a","b"]`), next: json.RawMessage(`1`), want: json.RawMessage(`["a","b"]`)},
		{desc: "object next keeps aggregated prev", prev: json.RawMessage(`["a"]`), next: json.RawMessage(`{"x":1}`), want: json.RawMessage(`["a"]`)},
		{desc: "empty next keeps prev", prev: json.RawMessage(`["a"]`), next: nil, want: json.RawMessage(`["a"]`)},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			got := concatJSONArrays(tc.prev, tc.next)
			if diff := cmp.Diff(string(tc.want), string(got)); diff != "" {
				t.Errorf("concatJSONArrays(%s, %s) diff %v", tc.prev, tc.next, diff)
			}
		})
	}
}

func TestParseDataAgentsPageRejectsWrongFieldTypes(t *testing.T) {
	for _, tc := range []struct {
		desc string
		body string
	}{
		{desc: "null body", body: `null`},
		{desc: "array body", body: `[]`},
		{desc: "string body", body: `"nope"`},
		{desc: "empty body", body: ``},
		{desc: "dataAgents is not an array", body: `{"dataAgents": "nope"}`},
		{desc: "nextPageToken is not a string", body: `{"nextPageToken": 7}`},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if _, _, _, err := parseDataAgentsPage([]byte(tc.body)); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestListAccessibleDataAgentsPassesCallerPaginationThrough(t *testing.T) {
	tcs := []struct {
		desc      string
		pageSize  *int
		pageToken string
		wantQuery url.Values
		wantNames []string
		wantToken bool
	}{
		{
			desc:      "page size only",
			pageSize:  intPtr(2),
			wantQuery: url.Values{"pageSize": {"2"}},
			wantNames: []string{"agent-0", "agent-1"},
			wantToken: true,
		},
		{
			desc:      "page token only",
			pageToken: "3",
			wantQuery: url.Values{"pageToken": {"3"}},
			wantNames: []string{"agent-3", "agent-4", "agent-5", "agent-6", "agent-7", "agent-8", "agent-9"},
		},
		{
			desc:      "page size and page token",
			pageSize:  intPtr(2),
			pageToken: "4",
			wantQuery: url.Values{"pageSize": {"2"}, "pageToken": {"4"}},
			wantNames: []string{"agent-4", "agent-5"},
			wantToken: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			api := &fakeDataAgentsAPI{total: 10}
			server := api.serve(t)

			result, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", tc.pageSize, tc.pageToken)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			queries := api.recordedQueries()
			if len(queries) != 1 {
				t.Fatalf("got %d requests, want 1", len(queries))
			}
			if diff := cmp.Diff(tc.wantQuery, queries[0]); diff != "" {
				t.Errorf("unexpected query parameters: diff %v", diff)
			}

			names, nextPageToken := displayNames(t, result)
			if diff := cmp.Diff(tc.wantNames, names); diff != "" {
				t.Errorf("unexpected data agents: diff %v", diff)
			}
			if tc.wantToken && nextPageToken == "" {
				t.Error("got no nextPageToken, want the one the API returned to be passed through")
			}
			if !tc.wantToken && nextPageToken != "" {
				t.Errorf("got nextPageToken %q, want none: the API returned the last page", nextPageToken)
			}
		})
	}
}

func TestListAccessibleDataAgentsPassThroughKeepsResponseUnchanged(t *testing.T) {
	body := `{"dataAgents": [{"displayName": "agent-1", "someNewField": "kept"}], "nextPageToken": "next-token", "unknownTopLevelField": 7}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	got, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", intPtr(1), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Spelled out rather than decoded from `body`, so that a field the tool
	// drops or renames actually fails the comparison.
	want := map[string]any{
		"dataAgents": []any{
			map[string]any{"displayName": "agent-1", "someNewField": "kept"},
		},
		"nextPageToken":        "next-token",
		"unknownTopLevelField": float64(7),
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected result: diff %v", diff)
	}
}

func TestListAccessibleDataAgentsPageBuildsRequest(t *testing.T) {
	tcs := []struct {
		desc      string
		pageSize  *int
		pageToken string
		wantQuery url.Values
	}{
		{
			desc:      "no pagination parameters leaves the request untouched",
			wantQuery: url.Values{},
		},
		{
			desc:      "page size only",
			pageSize:  intPtr(25),
			wantQuery: url.Values{"pageSize": {"25"}},
		},
		{
			desc:      "page token only",
			pageToken: "token-abc",
			wantQuery: url.Values{"pageToken": {"token-abc"}},
		},
		{
			desc:      "page size and page token",
			pageSize:  intPtr(10),
			pageToken: "token/with+special=chars",
			wantQuery: url.Values{"pageSize": {"10"}, "pageToken": {"token/with+special=chars"}},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			var (
				mu         sync.Mutex
				gotMethod  string
				gotPath    string
				gotURI     string
				gotQuery   url.Values
				gotClient  string
				requestGot bool
			)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				gotMethod, gotPath, gotURI = r.Method, r.URL.Path, r.RequestURI
				gotQuery, gotClient, requestGot = r.URL.Query(), r.Header.Get("X-Goog-API-Client"), true
				mu.Unlock()

				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"dataAgents": []}`)
			}))
			defer server.Close()

			if _, err := listAccessibleDataAgentsPage(context.Background(), server.Client(), server.URL, "my-project", "global", tc.pageSize, tc.pageToken); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			mu.Lock()
			defer mu.Unlock()
			if !requestGot {
				t.Fatal("the server received no request")
			}
			if gotMethod != http.MethodGet {
				t.Errorf("got method %q, want %q", gotMethod, http.MethodGet)
			}
			wantPath := "/v1/projects/my-project/locations/global/dataAgents:listAccessible"
			if gotPath != wantPath {
				t.Errorf("got path %q, want %q", gotPath, wantPath)
			}
			if gotClient != util.GDAClientID {
				t.Errorf("got X-Goog-API-Client %q, want %q", gotClient, util.GDAClientID)
			}
			if len(tc.wantQuery) == 0 && strings.Contains(gotURI, "?") {
				t.Errorf("got request URI %q, want no query string", gotURI)
			}
			if diff := cmp.Diff(tc.wantQuery, gotQuery); diff != "" {
				t.Errorf("unexpected query parameters: diff %v", diff)
			}
		})
	}
}

func TestListAccessibleDataAgentsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error": {"message": "permission denied"}}`)
	}))
	defer server.Close()

	for _, tc := range []struct {
		desc      string
		pageSize  *int
		pageToken string
	}{
		{desc: "while draining"},
		{desc: "while passing through", pageSize: intPtr(5)},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", tc.pageSize, tc.pageToken)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), "permission denied") {
				t.Errorf("error %q does not mention the API failure", err.Error())
			}
		})
	}
}

func TestListAccessibleDataAgentsMalformedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"dataAgents": [`)
	}))
	defer server.Close()

	for _, tc := range []struct {
		desc     string
		pageSize *int
	}{
		{desc: "while draining"},
		{desc: "while passing through", pageSize: intPtr(5)},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", tc.pageSize, "")
			if err == nil {
				t.Fatal("expected an error for a malformed body, got nil")
			}
			if !strings.Contains(err.Error(), "failed to decode response") {
				t.Errorf("error %q does not mention the decode failure", err.Error())
			}
		})
	}
}

func TestListAccessibleDataAgentsHonorsContext(t *testing.T) {
	t.Run("cancelled before first request", func(t *testing.T) {
		api := &fakeDataAgentsAPI{total: 10}
		server := api.serve(t)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := listAccessibleDataAgents(ctx, server.Client(), server.URL, "my-project", "global", nil, "")
		if err == nil {
			t.Fatal("expected an error for a cancelled context, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error %q does not wrap context.Canceled", err.Error())
		}
		if got := len(api.recordedQueries()); got != 0 {
			t.Errorf("the server received %d requests, want none", got)
		}
	})

	t.Run("cancelled between pages returns error instead of partial result", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var calls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"dataAgents": [{"displayName": "agent-0"}], "nextPageToken": "page-2"}`)
				cancel()
				return
			}
			<-r.Context().Done()
		}))
		defer server.Close()

		_, err := listAccessibleDataAgents(ctx, server.Client(), server.URL, "my-project", "global", nil, "")
		if err == nil {
			t.Fatal("expected context cancellation to return an error, got a partial result")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error %q does not wrap context.Canceled", err.Error())
		}
	})
}

func TestListAccessibleDataAgentsDrainStopsAtTimeBudget(t *testing.T) {
	t.Run("first page stall reports budget error", func(t *testing.T) {
		released := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-released:
			}
		}))
		defer server.Close()
		defer close(released)

		original := maxAutoDuration
		maxAutoDuration = 500 * time.Millisecond
		defer func() { maxAutoDuration = original }()

		_, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", nil, "")
		if err == nil {
			t.Fatal("expected an error once the first page runs out of time, got nil")
		}
		if !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "page_size") {
			t.Errorf("error %q does not explain the timeout or how to work around it", err.Error())
		}
	})

	t.Run("later page stall reports budget error", func(t *testing.T) {
		released := make(chan struct{})
		var calls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"dataAgents": [{"displayName": "agent-0"}], "nextPageToken": "page-2"}`)
				return
			}
			select {
			case <-r.Context().Done():
			case <-released:
			}
		}))
		defer server.Close()
		defer close(released)

		original := maxAutoDuration
		maxAutoDuration = 500 * time.Millisecond
		defer func() { maxAutoDuration = original }()

		_, err := listAccessibleDataAgents(context.Background(), server.Client(), server.URL, "my-project", "global", nil, "")
		if err == nil {
			t.Fatal("expected an error when a later page times out, got nil")
		}
		if !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "page_size") {
			t.Errorf("error %q does not explain the timeout or how to work around it", err.Error())
		}
	})

	t.Run("per-request client timeout reports budget error", func(t *testing.T) {
		released := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-released:
			}
		}))
		defer server.Close()
		defer close(released)

		client := server.Client()
		client.Timeout = 50 * time.Millisecond

		_, err := listAccessibleDataAgents(context.Background(), client, server.URL, "my-project", "global", nil, "")
		if err == nil {
			t.Fatal("expected an error when http.Client.Timeout fires, got nil")
		}
		if !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "page_size") {
			t.Errorf("error %q does not explain the timeout or how to work around it", err.Error())
		}
	})

	t.Run("an unreachable endpoint is not mislabelled as the budget", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		endpoint := server.URL
		server.Close()

		_, err := listAccessibleDataAgents(context.Background(), server.Client(), endpoint, "my-project", "global", nil, "")
		if err == nil {
			t.Fatal("expected an error when the endpoint refuses connections, got nil")
		}
		if strings.Contains(err.Error(), "timed out") {
			t.Errorf("error %q blames a timeout for an unreachable endpoint, where paging would not help", err.Error())
		}
	})

	t.Run("caller deadline is not mislabelled as the budget", func(t *testing.T) {
		released := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-released:
			}
		}))
		defer server.Close()
		defer close(released)

		callerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()

		_, err := listAccessibleDataAgents(callerCtx, server.Client(), server.URL, "my-project", "global", nil, "")
		if err == nil {
			t.Fatal("expected an error once the caller's deadline expires, got nil")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("error %q does not wrap context.DeadlineExceeded", err.Error())
		}
		if strings.Contains(err.Error(), "timed out while fetching accessible data agents") {
			t.Errorf("error %q blames the tool's own budget for the caller's deadline", err.Error())
		}
	})
}

func TestPaginationParamsAreDeclaredAndConsumed(t *testing.T) {
	cfg := Config{
		ConfigBase: tools.ConfigBase{Name: "test-tool", Description: "A test description."},
		Type:       resourceType,
		Source:     "my-gda-source",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	manifest, err := tool.Manifest(nil)
	if err != nil {
		t.Fatalf("Manifest() failed: %v", err)
	}

	var gotNames []string
	for _, p := range manifest.Parameters {
		gotNames = append(gotNames, p.Name)
		if p.Required {
			t.Errorf("parameter %q is required, want optional so that omitting it fetches every data agent", p.Name)
		}
	}
	if diff := cmp.Diff([]string{"page_size", "page_token"}, gotNames); diff != "" {
		t.Errorf("unexpected declared parameters: diff %v", diff)
	}

	declared := parameters.ParamValues{}
	for _, p := range manifest.Parameters {
		switch p.Type {
		case "integer":
			declared = append(declared, parameters.ParamValue{Name: p.Name, Value: 7})
		case "string":
			declared = append(declared, parameters.ParamValue{Name: p.Name, Value: "token-from-manifest"})
		default:
			t.Fatalf("parameter %q has unexpected type %q", p.Name, p.Type)
		}
	}
	pageSize, pageToken, tErr := parsePaginationParams(declared)
	if tErr != nil {
		t.Fatalf("unexpected error: %v", tErr)
	}
	if pageSize == nil || *pageSize != 7 {
		t.Errorf("declared page_size parameter was not consumed, got %v", pageSize)
	}
	if pageToken != "token-from-manifest" {
		t.Errorf("declared page_token parameter was not consumed, got %q", pageToken)
	}
}

func TestPaginationParamsFromRequestValues(t *testing.T) {
	cfg := Config{
		ConfigBase: tools.ConfigBase{Name: "test-tool", Description: "A test description."},
		Type:       resourceType,
		Source:     "my-gda-source",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	params, err := tool.GetParameters(nil)
	if err != nil {
		t.Fatalf("GetParameters() failed: %v", err)
	}

	tcs := []struct {
		desc          string
		data          map[string]any
		wantErr       bool
		wantPageSize  *int
		wantPageToken string
	}{
		{
			desc: "empty request drains every page",
			data: map[string]any{},
		},
		{
			desc:          "json numbers are accepted",
			data:          map[string]any{"page_size": json.Number("25"), "page_token": "token-abc"},
			wantPageSize:  intPtr(25),
			wantPageToken: "token-abc",
		},
		{
			desc:    "a zero page size is rejected before the tool runs",
			data:    map[string]any{"page_size": json.Number("0")},
			wantErr: true,
		},
		{
			desc:    "a negative page size is rejected before the tool runs",
			data:    map[string]any{"page_size": json.Number("-1")},
			wantErr: true,
		},
		{
			desc:    "a non-numeric page size is rejected before the tool runs",
			data:    map[string]any{"page_size": "ten"},
			wantErr: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			values, err := parameters.ParseParams(params, tc.data, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected ParseParams to reject the request, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseParams() failed: %v", err)
			}

			pageSize, pageToken, tErr := parsePaginationParams(values)
			if tErr != nil {
				t.Fatalf("unexpected error: %v", tErr)
			}
			if diff := cmp.Diff(tc.wantPageSize, pageSize); diff != "" {
				t.Errorf("unexpected pageSize: diff %v", diff)
			}
			if pageToken != tc.wantPageToken {
				t.Errorf("got pageToken %q, want %q", pageToken, tc.wantPageToken)
			}
		})
	}
}

func TestInitializeRequiresDescription(t *testing.T) {
	cfg := Config{
		ConfigBase: tools.ConfigBase{Name: "test-tool"},
		Type:       resourceType,
		Source:     "my-gda-source",
	}
	if _, err := cfg.Initialize(context.Background()); err == nil {
		t.Fatal("expected an error for a config without a description, got nil")
	}
}

func TestParsePaginationParams(t *testing.T) {
	tcs := []struct {
		desc          string
		params        parameters.ParamValues
		wantPageSize  *int
		wantPageToken string
		wantErr       string
	}{
		{
			desc:   "no parameters leaves the page size unset",
			params: parameters.ParamValues{},
		},
		{
			desc: "omitted parameters arrive as explicit nils",
			params: parameters.ParamValues{
				{Name: "page_size", Value: nil},
				{Name: "page_token", Value: nil},
			},
		},
		{
			desc: "honors page_size and page_token",
			params: parameters.ParamValues{
				{Name: "page_size", Value: 25},
				{Name: "page_token", Value: "token-abc"},
			},
			wantPageSize:  intPtr(25),
			wantPageToken: "token-abc",
		},
		{
			desc:    "rejects a zero page_size",
			params:  parameters.ParamValues{{Name: "page_size", Value: 0}},
			wantErr: "must be positive",
		},
		{
			desc:    "rejects a negative page_size",
			params:  parameters.ParamValues{{Name: "page_size", Value: -1}},
			wantErr: "must be positive",
		},
		{
			desc:    "rejects a non-integer page_size",
			params:  parameters.ParamValues{{Name: "page_size", Value: "ten"}},
			wantErr: "error casting 'page_size' parameter",
		},
		{
			desc:    "rejects a non-string page_token",
			params:  parameters.ParamValues{{Name: "page_token", Value: 123}},
			wantErr: "error casting 'page_token' parameter",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			pageSize, pageToken, err := parsePaginationParams(tc.params)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tc.wantPageSize, pageSize); diff != "" {
				t.Errorf("unexpected pageSize: diff %v", diff)
			}
			if pageToken != tc.wantPageToken {
				t.Errorf("got pageToken %q, want %q", pageToken, tc.wantPageToken)
			}
		})
	}
}
