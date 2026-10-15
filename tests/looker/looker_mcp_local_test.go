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
		for _, name := range []string{"get_looks", "get_dashboards"} {
			got := invokeLookerTool(t, "http://127.0.0.1:5000/api/tool/"+name+"/invoke", []byte(`{"title":"FOO","desc":"BAR"}`), withLookerMCP())
			if string(got) != "[]" {
				t.Fatalf("%s empty result=%s, want []", name, got)
			}
		}
	})
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
