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

package dataplex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"cloud.google.com/go/dataplex/apiv1/dataplexpb"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	dataplexsource "github.com/googleapis/mcp-toolbox/internal/sources/dataplex"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"go.opentelemetry.io/otel/trace"
)

// The local source exercises Toolbox's real MCP path without cloud resources.
// The authenticated resource lifecycles remain in TestDataplexMCPCallTools.
type localDataplexConfig struct{}

func (localDataplexConfig) SourceConfigType() string { return "dataplex-mcp-test" }
func (localDataplexConfig) Initialize(context.Context, trace.Tracer) (sources.Source, error) {
	return &localDataplexSource{Source: &dataplexsource.Source{Config: dataplexsource.Config{Project: "test-project"}}}, nil
}

type localDataplexSource struct{ *dataplexsource.Source }

func (*localDataplexSource) SearchEntries(_ context.Context, query string, pageSize int, orderBy, scope string) ([]*dataplexpb.SearchEntriesResult, error) {
	if pageSize != 5 || orderBy != "relevance" || scope != "" {
		return nil, fmt.Errorf("unexpected search arguments")
	}
	if query == "empty" {
		return nil, nil
	}
	return []*dataplexpb.SearchEntriesResult{{DataplexEntry: &dataplexpb.Entry{Name: query}}}, nil
}
func init() {
	sources.Register("dataplex-mcp-test", func(context.Context, string, *yaml.Decoder) (sources.SourceConfig, error) {
		return localDataplexConfig{}, nil
	})
}
func TestDataplexMCPLocal(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	config := getDataplexToolsConfig(map[string]any{"type": "dataplex-mcp-test"})
	// A local audience is sufficient for missing/invalid-token rejection. No valid token is fabricated.
	config["authServices"].(map[string]any)["my-google-auth"].(map[string]any)["clientId"] = "dataplex-local-test"
	cmd, cleanup, err := tests.StartCmd(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	defer func() {
		cmd.Stop()
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := cmd.Wait(shutdown); err != nil {
			t.Error(err)
		}
		cmd.Close()
	}()
	if out, err := testutils.WaitForString(ctx, regexp.MustCompile(`Server ready to serve`), cmd.Out); err != nil {
		t.Fatalf("start: %v\n%s", err, out)
	}
	t.Run("ListTools", func(t *testing.T) { tests.RunMCPToolsListMethod(t, dataplexExpectedTools(t)) })
	for _, tc := range []struct {
		name, tool, body, want, wantError string
		status                            int
	}{
		{name: "search entry", tool: "my-dataplex-search-entries-tool", body: `{"query":"entry-name"}`, want: `[{"dataplex_entry":{"name":"entry-name"}}]`, status: 200},
		{name: "empty search", tool: "my-dataplex-search-entries-tool", body: `{"query":"empty"}`, want: `null`, status: 200},
		{name: "invalid entry", tool: "my-dataplex-lookup-entry-tool", body: `{"entry":"bad"}`, wantError: "invalid entry format", status: 200},
		{name: "invalid context", tool: "my-dataplex-lookup-context-tool", body: `{"resources":["bad"]}`, wantError: "invalid resource format", status: 200},
		{name: "missing auth", tool: "my-auth-dataplex-search-entries-tool", body: `{"query":"entry-name"}`, status: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := invokeDataplexTool(t, "http://127.0.0.1:5000/api/tool/"+tc.tool+"/invoke", bytes.NewBufferString(tc.body), nil, tc.status, tc.wantError, withDataplexMCP())
			if got != tc.want {
				t.Fatalf("result=%s, want %s", got, tc.want)
			}
		})
	}
}

func TestDataplexRejectsInvalidMCPResults(t *testing.T) {
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
			if _, err := dataplexMCPResult(&response, ""); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}

func TestDataplexLegacyErrorResults(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"result":"{\"error\":\"NotFound: entry does not exist\"}"}`)
	}))
	defer backend.Close()
	invokeDataplexTool(t, backend.URL, bytes.NewBufferString(`{}`), nil, http.StatusOK, "not.?found|permission.?denied")
}
