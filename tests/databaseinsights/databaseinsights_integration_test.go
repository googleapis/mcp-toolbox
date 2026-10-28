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

package databaseinsights

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	Project  = os.Getenv("DATABASE_INSIGHTS_PROJECT")
	Region   = os.Getenv("DATABASE_INSIGHTS_REGION")
	Cluster  = os.Getenv("DATABASE_INSIGHTS_CLUSTER")
	Instance = os.Getenv("DATABASE_INSIGHTS_INSTANCE")
)

func getDatabaseInsightsVars() map[string]string {
	return map[string]string{
		"parent":             fmt.Sprintf("projects/%s/locations/%s", Project, Region),
		"full_resource_name": fmt.Sprintf("//alloydb.googleapis.com/projects/%s/locations/%s/clusters/%s/instances/%s", Project, Region, Cluster, Instance),
	}
}

func getDatabaseInsightsToolsConfig() map[string]any {
	return map[string]any{
		"sources": map[string]any{
			"dbinsights-source": map[string]any{
				"type": "databaseinsights",
			},
		},
		"tools": map[string]any{
			"get_advanced_aggregated_query_stats": map[string]any{
				"type":        "databaseinsights-get-advanced-aggregated-query-stats",
				"source":      "dbinsights-source",
				"description": "Aggregated query stats",
			},
			"get_advanced_aggregated_wait_event_stats": map[string]any{
				"type":        "databaseinsights-get-advanced-aggregated-wait-event-stats",
				"source":      "dbinsights-source",
				"description": "Aggregated wait stats",
			},
			"get_advanced_time_series_query_stats": map[string]any{
				"type":        "databaseinsights-get-advanced-time-series-query-stats",
				"source":      "dbinsights-source",
				"description": "Query time series stats",
			},
			"get_advanced_time_series_wait_event_stats": map[string]any{
				"type":        "databaseinsights-get-advanced-time-series-wait-event-stats",
				"source":      "dbinsights-source",
				"description": "Wait time series stats",
			},
			"get_index_recommendations": map[string]any{
				"type":        "databaseinsights-get-index-recommendations",
				"source":      "dbinsights-source",
				"description": "Index recommendations",
			},
		},
	}
}

func TestDatabaseInsightsToolEndpoints(t *testing.T) {
	if Project == "" || Region == "" || Cluster == "" || Instance == "" {
		t.Skip("Skipping Database Insights integration test: DATABASE_INSIGHTS_* environment variables not set")
	}
	vars := getDatabaseInsightsVars()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}
	toolsFile := getDatabaseInsightsToolsConfig()

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %v", err)
	}
	defer cleanup()

	waitCtx, cancelWait := context.WithTimeout(ctx, 20*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %v", err)
	}

	runToolGetTest(t)
	runGetAdvancedAggregatedQueryStatsTest(t, vars)
	runGetAdvancedAggregatedWaitEventStatsTest(t, vars)
	runGetAdvancedTimeSeriesQueryStatsTest(t, vars)
	runGetAdvancedTimeSeriesWaitEventStatsTest(t, vars)
	runGetIndexRecommendationsTest(t, vars)
	runMCPCallTest(t, vars)
}

type ToolResponse struct {
	Result string `json:"result"`
}

func runToolGetTest(t *testing.T) {
	t.Run("get get_advanced_aggregated_query_stats manifest", func(t *testing.T) {
		resp, err := http.Get("http://127.0.0.1:5000/api/tool/get_advanced_aggregated_query_stats/")
		if err != nil {
			t.Fatalf("error sending GET request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode manifest response: %v", err)
		}
		toolsMap, ok := body["tools"].(map[string]any)
		if !ok {
			t.Fatalf("expected 'tools' map in response body")
		}
		if _, ok := toolsMap["get_advanced_aggregated_query_stats"]; !ok {
			t.Fatalf("expected 'get_advanced_aggregated_query_stats' tool in manifest response")
		}
	})
}

func runGetAdvancedAggregatedQueryStatsTest(t *testing.T, vars map[string]string) {
	t.Run("get_advanced_aggregated_query_stats success", func(t *testing.T) {
		api := "http://127.0.0.1:5000/api/tool/get_advanced_aggregated_query_stats/invoke"
		body := fmt.Sprintf(`{"parent": "%s", "full_resource_name": "%s", "page_size": 2}`, vars["parent"], vars["full_resource_name"])
		resp, err := http.Post(api, "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		assertSuccessfulResponse(t, resp)
	})
}

func runGetAdvancedAggregatedWaitEventStatsTest(t *testing.T, vars map[string]string) {
	t.Run("get_advanced_aggregated_wait_event_stats success", func(t *testing.T) {
		api := "http://127.0.0.1:5000/api/tool/get_advanced_aggregated_wait_event_stats/invoke"
		body := fmt.Sprintf(`{"parent": "%s", "full_resource_name": "%s", "page_size": 2}`, vars["parent"], vars["full_resource_name"])
		resp, err := http.Post(api, "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		assertSuccessfulResponse(t, resp)
	})
}

func runGetAdvancedTimeSeriesQueryStatsTest(t *testing.T, vars map[string]string) {
	t.Run("get_advanced_time_series_query_stats success", func(t *testing.T) {
		api := "http://127.0.0.1:5000/api/tool/get_advanced_time_series_query_stats/invoke"
		body := fmt.Sprintf(`{"parent": "%s", "full_resource_name": "%s"}`, vars["parent"], vars["full_resource_name"])
		resp, err := http.Post(api, "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		assertSuccessfulResponse(t, resp)
	})
}

func runGetAdvancedTimeSeriesWaitEventStatsTest(t *testing.T, vars map[string]string) {
	t.Run("get_advanced_time_series_wait_event_stats success", func(t *testing.T) {
		api := "http://127.0.0.1:5000/api/tool/get_advanced_time_series_wait_event_stats/invoke"
		body := fmt.Sprintf(`{"parent": "%s", "full_resource_name": "%s"}`, vars["parent"], vars["full_resource_name"])
		resp, err := http.Post(api, "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		assertSuccessfulResponse(t, resp)
	})
}

func runGetIndexRecommendationsTest(t *testing.T, vars map[string]string) {
	t.Run("get_index_recommendations success", func(t *testing.T) {
		api := "http://127.0.0.1:5000/api/tool/get_index_recommendations/invoke"
		body := fmt.Sprintf(`{
			"parent": "%s",
			"full_resource_name": "%s",
			"database_query_ids": [
				{"database": "postgres", "query_ids": ["2230678628280650964"]}
			]
		}`, vars["parent"], vars["full_resource_name"])
		resp, err := http.Post(api, "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		assertSuccessfulResponse(t, resp)
	})
}

func assertSuccessfulResponse(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var toolResp ToolResponse
	if err := json.Unmarshal(bodyBytes, &toolResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v\nBody: %s", err, string(bodyBytes))
	}

	if toolResp.Result == "" {
		t.Errorf("expected non-empty result")
	}

	var resultErr map[string]any
	if err := json.Unmarshal([]byte(toolResp.Result), &resultErr); err == nil {
		if errMsg, exists := resultErr["error"]; exists {
			t.Fatalf("tool invocation returned error inside result: %v", errMsg)
		}
	}
}

func runMCPCallTest(t *testing.T, vars map[string]string) {
	t.Run("mcp_tools_call_get_advanced_aggregated_query_stats", func(t *testing.T) {
		api := "http://127.0.0.1:5000/mcp"
		body := fmt.Sprintf(`{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "tools/call",
			"params": {
				"name": "get_advanced_aggregated_query_stats",
				"arguments": {
					"parent": "%s",
					"full_resource_name": "%s",
					"page_size": 2
				}
			}
		}`, vars["parent"], vars["full_resource_name"])
		resp, err := http.Post(api, "application/json", bytes.NewBufferString(body))
		if err != nil {
			t.Fatalf("failed to send MCP request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}

		var mcpResp struct {
			Jsonrpc string `json:"jsonrpc"`
			Id      int    `json:"id"`
			Result  struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
			Error any `json:"error"`
		}
		if err := json.Unmarshal(bodyBytes, &mcpResp); err != nil {
			t.Fatalf("failed to unmarshal MCP response: %v\nBody: %s", err, string(bodyBytes))
		}
		if mcpResp.Error != nil {
			t.Fatalf("MCP response returned error: %v", mcpResp.Error)
		}
		if len(mcpResp.Result.Content) == 0 {
			t.Fatalf("expected non-empty content in MCP result")
		}
	})
}
