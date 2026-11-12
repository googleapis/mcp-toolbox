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

package looker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/googleapis/mcp-toolbox/tests"
)

func TestLookerMCPLocal(t *testing.T) {
	var modelCalls atomic.Int32
	var legacyFeatureCount atomic.Int32
	var savedQueryCalls, importCalls, textTileCalls atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/login"):
			fmt.Fprint(w, `{"access_token":"local-test-token","token_type":"Bearer","expires_in":3600}`)
		case strings.HasSuffix(r.URL.Path, "/user"):
			fmt.Fprint(w, `{"first_name":"Local","last_name":"Test"}`)
		case strings.HasSuffix(r.URL.Path, "/looks/search"), strings.HasSuffix(r.URL.Path, "/dashboards/search"), strings.HasSuffix(r.URL.Path, "/dashboards/lookml/search"):
			fmt.Fprint(w, `[]`)
		case strings.HasSuffix(r.URL.Path, "/lookml_models"):
			modelCalls.Add(1)
			fmt.Fprint(w, `[{"name":"local_model","label":"Local Model","project_name":"local_project","allowed_db_connection_names":[]}]`)
		case strings.HasSuffix(r.URL.Path, "/queries/saved-query/run/json"):
			savedQueryCalls.Add(1)
			if r.Method != http.MethodGet {
				t.Errorf("saved query method = %s, want GET", r.Method)
			}
			fmt.Fprint(w, `[{"look.count":7}]`)
		case strings.HasSuffix(r.URL.Path, "/legacy_features"):
			features := []map[string]any{}
			for i := int32(0); i < legacyFeatureCount.Load(); i++ {
				features = append(features, map[string]any{"name": fmt.Sprintf("legacy-%d", i), "enabled": true})
			}
			if err := json.NewEncoder(w).Encode(features); err != nil {
				t.Error(err)
			}
		case strings.HasSuffix(r.URL.Path, "/dashboards/lookml"):
			importCalls.Add(1)
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if r.Method != http.MethodPost || body["folder_id"] != "local-folder" || body["lookml"] != "- dashboard: local_import\n  title: Local Import\n" {
				t.Errorf("unexpected dashboard import: %s %v", r.Method, body)
			}
			fmt.Fprint(w, `{"id":"local-import","url":"/dashboards/local-import"}`)
		case strings.HasSuffix(r.URL.Path, "/versions"):
			fmt.Fprint(w, `{"web_server_url":"https://local.looker.test"}`)
		case strings.HasSuffix(r.URL.Path, "/dashboard_elements"), strings.HasSuffix(r.URL.Path, "/dashboard_elements/local-tile"):
			textTileCalls.Add(1)
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["type"] != "text" || body["body_text"] != "Local text tile" || body["dashboard_id"] != "local-dashboard" {
				t.Errorf("unexpected text tile body: %v", body)
			}
			if body["query_id"] != nil || body["query"] != nil {
				t.Errorf("text tile unexpectedly creates a query: %v", body)
			}
			wantMethod := http.MethodPost
			if strings.HasSuffix(r.URL.Path, "/local-tile") {
				wantMethod = http.MethodPatch
			}
			if r.Method != wantMethod {
				t.Errorf("text tile method = %s, want %s", r.Method, wantMethod)
			}
			fmt.Fprint(w, `{"id":"local-tile"}`)
		default:
			t.Errorf("unexpected backend request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(backend.Close)
	setupLooker(t, map[string]any{"type": "looker", "base_url": backend.URL, "verify_ssl": true, "client_id": "local-test", "client_secret": "local-test", "project": "local-project", "location": "us-central1"})
	t.Run("ListTools", func(t *testing.T) { tests.RunMCPToolsListMethod(t, lookerExpectedTools(t)) })
	t.Run("CallTools", func(t *testing.T) {
		runLookerToolContains(t, "get_models", []byte(`{}`), `"name":"local_model"`, withLookerMCP())
		runLookerToolContains(t, "query", []byte(`{"query_id":"saved-query"}`), `"look.count":7`, withLookerMCP())
		runLookerToolContains(t, "import_dashboard_from_lookml", []byte(`{"lookml":"- dashboard: local_import\n  title: Local Import\n","folder":"local-folder"}`), `"id":"local-import"`, withLookerMCP())
		runLookerToolContains(t, "add_dashboard_element", []byte(`{"dashboard_id":"local-dashboard","type":"text","body_text":"Local text tile"}`), "Dashboard element added to dashboard local-dashboard", withLookerMCP())
		runLookerToolContains(t, "update_dashboard_element", []byte(`{"dashboard_id":"local-dashboard","dashboard_element_id":"local-tile","type":"text","body_text":"Local text tile"}`), "Dashboard element local-tile updated", withLookerMCP())
		for count := int32(0); count <= 2; count++ {
			t.Run(fmt.Sprintf("legacy features/%d", count), func(t *testing.T) {
				legacyFeatureCount.Store(count)
				result := invokeLookerTool(t, "http://127.0.0.1:5000/api/tool/health_pulse/invoke", []byte(`{"action":"check_legacy_features"}`), withLookerMCP())
				if err := validateLookerLegacyFeatures(result); err != nil {
					t.Fatal(err)
				}
				var features []map[string]string
				if err := json.Unmarshal(result, &features); err != nil {
					t.Fatal(err)
				}
				if len(features) != int(count) {
					t.Fatalf("feature count = %d, want %d", len(features), count)
				}
			})
		}

		for _, name := range []string{"get_looks", "get_dashboards"} {
			got := invokeLookerTool(t, "http://127.0.0.1:5000/api/tool/"+name+"/invoke", []byte(`{"title":"FOO","desc":"BAR"}`), withLookerMCP())
			if string(got) != "[]" {
				t.Fatalf("%s empty result=%s, want []", name, got)
			}
		}
	})
	if savedQueryCalls.Load() != 1 || importCalls.Load() != 1 || textTileCalls.Load() != 2 {
		t.Errorf("backend calls: saved query=%d, import=%d, text tiles=%d; want 1, 1, 2", savedQueryCalls.Load(), importCalls.Load(), textTileCalls.Load())
	}
	if modelCalls.Load() != 1 {
		t.Fatalf("model requests=%d, want 1", modelCalls.Load())
	}
}

func TestLookerRejectsInvalidMCPResults(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"protocol error", `{"error":{"code":-32603,"message":"backend failed"}}`},
		{"tool error", `{"result":{"isError":true,"content":[{"type":"text","text":"bad arguments"}]}}`},
		{"missing content", `{"result":{}}`},
		{"null content", `{"result":{"content":null}}`},
		{"nontext content", `{"result":{"content":[{"type":"image","text":"{}"}]}}`},
		{"invalid JSON", `{"result":{"content":[{"type":"text","text":"not JSON"}]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response tests.MCPCallToolResponse
			if err := json.Unmarshal([]byte(tc.body), &response); err != nil {
				t.Fatal(err)
			}
			if _, err := lookerMCPResult(&response); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}

func TestLookerMCPStringResults(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"created", `{"result":{"content":[{"type":"text","text":"\"created\""}]}}`, `"created"`},
		{"empty string", `{"result":{"content":[{"type":"text","text":"\"\""}]}}`, `""`},
		{"conversation", `{"result":{"content":[{"type":"text","text":"\"Revenue grew by 10%.\""}]}}`, `"Revenue grew by 10%."`},
		{"multiple strings", `{"result":{"content":[{"type":"text","text":"\"created\""},{"type":"text","text":"\"\""}]}}`, `["created",""]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var response tests.MCPCallToolResponse
			if err := json.Unmarshal([]byte(tc.body), &response); err != nil {
				t.Fatal(err)
			}
			got, err := lookerMCPResult(&response)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("result = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestLookerRejectsInvalidLegacyFeatures(t *testing.T) {
	for _, tc := range []struct{ name, result string }{
		{"null", `null`},
		{"object", `{"Feature":"legacy"}`},
		{"missing feature", `[{}]`},
		{"empty feature", `[{"Feature":""}]`},
		{"SDK error", `[{"Feature":"Unable to pull legacy features due to SDK error"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateLookerLegacyFeatures([]byte(tc.result)); err == nil {
				t.Fatalf("invalid legacy feature result accepted: %s", tc.result)
			}
		})
	}
}
