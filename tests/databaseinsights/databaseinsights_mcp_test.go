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
	"context"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// startDatabaseInsightsMCPServer starts the toolbox server with the Database
// Insights tools config and waits until it is ready to serve.
func startDatabaseInsightsMCPServer(t *testing.T, ctx context.Context) {
	t.Helper()
	cmd, cleanup, err := tests.StartCmd(ctx, getDatabaseInsightsToolsConfig())
	if err != nil {
		t.Fatalf("command initialization returned an error: %v", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	waitCtx, cancelWait := context.WithTimeout(ctx, 20*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %v", err)
	}
}

func TestDatabaseInsightsMCPListTools(t *testing.T) {
	if Project == "" || Region == "" || Cluster == "" || Instance == "" {
		t.Skip("Skipping Database Insights integration test: DATABASE_INSIGHTS_* environment variables not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	startDatabaseInsightsMCPServer(t, ctx)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getDatabaseInsightsMCPExpectedTools())
	})
}

func TestDatabaseInsightsMCPCallTool(t *testing.T) {
	if Project == "" || Region == "" || Cluster == "" || Instance == "" {
		t.Skip("Skipping Database Insights integration test: DATABASE_INSIGHTS_* environment variables not set")
	}
	vars := getDatabaseInsightsVars()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	startDatabaseInsightsMCPServer(t, ctx)

	invokeTcs := []struct {
		name           string
		toolName       string
		args           map[string]any
		wantContentErr string
	}{
		{
			name:     "invoke get_advanced_aggregated_query_stats",
			toolName: "get_advanced_aggregated_query_stats",
			args:     map[string]any{"parent": vars["parent"], "full_resource_name": vars["full_resource_name"], "page_size": 2},
		},
		{
			name:     "invoke get_advanced_aggregated_wait_event_stats",
			toolName: "get_advanced_aggregated_wait_event_stats",
			args:     map[string]any{"parent": vars["parent"], "full_resource_name": vars["full_resource_name"], "page_size": 2},
		},
		{
			name:     "invoke get_advanced_time_series_query_stats",
			toolName: "get_advanced_time_series_query_stats",
			args:     map[string]any{"parent": vars["parent"], "full_resource_name": vars["full_resource_name"]},
		},
		{
			name:     "invoke get_advanced_time_series_wait_event_stats",
			toolName: "get_advanced_time_series_wait_event_stats",
			args:     map[string]any{"parent": vars["parent"], "full_resource_name": vars["full_resource_name"]},
		},
		{
			name:     "invoke get_index_recommendations",
			toolName: "get_index_recommendations",
			args: map[string]any{
				"parent":             vars["parent"],
				"full_resource_name": vars["full_resource_name"],
				"database_query_ids": []any{
					map[string]any{"database": "postgres", "query_ids": []any{"2230678628280650964"}},
				},
			},
		},
		{
			name:           "invoke get_advanced_aggregated_query_stats without required parent",
			toolName:       "get_advanced_aggregated_query_stats",
			args:           map[string]any{"full_resource_name": vars["full_resource_name"]},
			wantContentErr: `parameter "parent" is required`,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, tc.toolName, tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing %s: %s", tc.toolName, err)
			}
			if statusCode != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, statusCode)
			}

			if tc.wantContentErr != "" {
				tests.AssertMCPError(t, mcpResp, tc.wantContentErr)
				return
			}

			if mcpResp.Error != nil {
				t.Fatalf("%s returned JSON-RPC error: %v", tc.toolName, mcpResp.Error)
			}
			if mcpResp.Result.IsError {
				t.Fatalf("%s returned error result: %v", tc.toolName, mcpResp.Result)
			}
			// Results come from live Database Insights data, so only verify that content was returned.
			if len(mcpResp.Result.Content) == 0 {
				t.Fatalf("%s returned empty content", tc.toolName)
			}
		})
	}
}

// getDatabaseInsightsMCPExpectedTools returns the MCP manifests for the tools loaded by getDatabaseInsightsToolsConfig.
func getDatabaseInsightsMCPExpectedTools() []tests.MCPToolManifest {
	return []tests.MCPToolManifest{
		{
			Name:        "get_advanced_aggregated_query_stats",
			Description: "Aggregated query stats",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database":           map[string]any{"description": "Optional. Filter results to a specific database name.", "type": "string"},
					"end_time":           map[string]any{"description": "Optional. End of the interval for fetching stats in RFC3339 format (Defaults to 'now').", "type": "string"},
					"full_resource_name": map[string]any{"description": "Required. The full identifier for the AlloyDB instance. Provide the full resource name ONLY in the following format: //alloydb.googleapis.com/projects/{project_id}/locations/{location}/clusters/{cluster_id}/instances/{instance_id}", "type": "string"},
					"page_size":          map[string]any{"description": "Optional. Maximum number of query stats to return (Default: 20).", "type": "integer"},
					"page_token":         map[string]any{"description": "Optional. Token for fetching the next set of results.", "type": "string"},
					"parent":             map[string]any{"description": "Required. Project and location. Format: projects/{project_id}/locations/{location}", "type": "string"},
					"query_id":           map[string]any{"description": "Optional. Fetch aggregated statistics for a single, specific query hash.", "type": "string"},
					"start_time":         map[string]any{"description": "Optional. Beginning of the interval for fetching stats in RFC3339 format (Defaults to 1 hour ago).", "type": "string"},
					"username":           map[string]any{"description": "Optional. Filter results to a specific database user.", "type": "string"},
				},
				"required": []any{"parent", "full_resource_name"},
			},
		},
		{
			Name:        "get_advanced_aggregated_wait_event_stats",
			Description: "Aggregated wait stats",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database":           map[string]any{"description": "Optional. Filter stats to a specific database.", "type": "string"},
					"end_time":           map[string]any{"description": "Optional. End of the interval for fetching stats in RFC3339 format (Defaults to 'now').", "type": "string"},
					"full_resource_name": map[string]any{"description": "Required. The full identifier for the AlloyDB instance. Provide the full resource name ONLY in the following format: //alloydb.googleapis.com/projects/{project_id}/locations/{location}/clusters/{cluster_id}/instances/{instance_id}", "type": "string"},
					"page_size":          map[string]any{"description": "Optional. Maximum number of results to return (Default: 20).", "type": "integer"},
					"page_token":         map[string]any{"description": "Optional. Token for fetching the next set of results.", "type": "string"},
					"parent":             map[string]any{"description": "Required. Project and location. Format: projects/{project_id}/locations/{location}", "type": "string"},
					"query_id":           map[string]any{"description": "Optional. Breakdown wait events for a specific query hash.", "type": "string"},
					"start_time":         map[string]any{"description": "Optional. Beginning of the interval for fetching stats in RFC3339 format (Defaults to 1 hour ago).", "type": "string"},
					"username":           map[string]any{"description": "Optional. Filter stats to a specific database user.", "type": "string"},
					"view":               map[string]any{"description": "Optional. Aggregation level. Use 'WAIT_CLASS' for high-level categories (e.g., Lock, IO) or 'WAIT_EVENT' for granular details (e.g., ClientWrite, DataFileRead). Defaults to 'WAIT_CLASS'.", "type": "string"},
				},
				"required": []any{"parent", "full_resource_name"},
			},
		},
		{
			Name:        "get_advanced_time_series_query_stats",
			Description: "Query time series stats",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database":           map[string]any{"description": "Optional. Filter results to a specific database name.", "type": "string"},
					"end_time":           map[string]any{"description": "Optional. End of the interval for fetching history in RFC3339 format (Defaults to 'now').", "type": "string"},
					"full_resource_name": map[string]any{"description": "Required. The full identifier for the AlloyDB instance. Format: //alloydb.googleapis.com/projects/{project_id}/locations/{location}/clusters/{cluster_id}/instances/{instance_id}", "type": "string"},
					"parent":             map[string]any{"description": "Required. Project and location. Format: projects/{project_id}/locations/{location}", "type": "string"},
					"query_id":           map[string]any{"description": "Optional. Fetch history for a single, specific query hash.", "type": "string"},
					"start_time":         map[string]any{"description": "Optional. Beginning of the interval for fetching history in RFC3339 format (Defaults to 1 hour ago).", "type": "string"},
					"username":           map[string]any{"description": "Optional. Filter results to a specific database user.", "type": "string"},
				},
				"required": []any{"parent", "full_resource_name"},
			},
		},
		{
			Name:        "get_advanced_time_series_wait_event_stats",
			Description: "Wait time series stats",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database":           map[string]any{"description": "Optional. Filter history to a specific database.", "type": "string"},
					"end_time":           map[string]any{"description": "Optional. End of the interval for fetching history in RFC3339 format (Defaults to 'now').", "type": "string"},
					"full_resource_name": map[string]any{"description": "Required. The full identifier for the AlloyDB instance. Format: //alloydb.googleapis.com/projects/{project_id}/locations/{location}/clusters/{cluster_id}/instances/{instance_id}", "type": "string"},
					"parent":             map[string]any{"description": "Required. Project and location. Format: projects/{project_id}/locations/{location}", "type": "string"},
					"query_id":           map[string]any{"description": "Optional. Breakdown wait events over time for a specific query hash.", "type": "string"},
					"start_time":         map[string]any{"description": "Optional. Beginning of the interval for fetching history in RFC3339 format (Defaults to 1 hour ago).", "type": "string"},
					"username":           map[string]any{"description": "Optional. Filter history to a specific database user.", "type": "string"},
					"view":               map[string]any{"description": "Optional. Aggregation level. Use 'WAIT_CLASS' for high-level categories (e.g., Lock, IO) or 'WAIT_EVENT' for granular details (e.g., ClientWrite, DataFileRead). Defaults to 'WAIT_CLASS'.", "type": "string"},
				},
				"required": []any{"parent", "full_resource_name"},
			},
		},
		{
			Name:        "get_index_recommendations",
			Description: "Index recommendations",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database_query_ids": map[string]any{"description": "Optional. A list of objects used to target specific queries. Example schema: [{'database': 'dbname', 'query_ids': [12345]}]", "items": map[string]any{"additionalProperties": true, "description": "", "type": "object"}, "type": "array"},
					"full_resource_name": map[string]any{"description": "Required. The full identifier for the AlloyDB instance. Provide the full resource name ONLY in the following format: //alloydb.googleapis.com/projects/{project_id}/locations/{location}/clusters/{cluster_id}/instances/{instance_id}", "type": "string"},
					"parent":             map[string]any{"description": "Required. Project and location. Format: projects/{project_id}/locations/{location}", "type": "string"},
				},
				"required": []any{"parent", "full_resource_name"},
			},
		},
	}
}
