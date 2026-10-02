// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloudgda_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/tools/cloudgda"
	"github.com/googleapis/mcp-toolbox/tests"
)

func TestCloudGdaMCPToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	startCloudGdaMockServer(t)
	tr := cloudGDATransport{isMCP: true}
	tr.startServer(t, ctx, getCloudGdaToolsConfig())

	toolName := "cloud-gda-query"

	tests.RunMCPToolsListMethod(t, []tests.MCPToolManifest{
		{
			Name:        toolName,
			Description: "Test GDA Tool\n\n" + cloudgda.Guidance,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "A natural language formulation of a database query.",
					},
				},
				"required": []any{"query"},
			},
		},
	})

	res := tr.mustInvoke(t, ctx, toolName, map[string]any{"query": "test question"}, nil)
	if res.status != http.StatusOK || res.toolErr {
		t.Fatalf("unexpected result (status %d, toolErr %t): %s", res.status, res.toolErr, res.result)
	}
	want := `"generated_query":"SELECT * FROM table;"`
	if !strings.Contains(res.result, want) {
		t.Errorf("result %q does not contain %q", res.result, want)
	}
}

func getConversationalAnalyticsMCPManifests() []tests.MCPToolManifest {
	listSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"page_size": map[string]any{
				"type":        "integer",
				"description": "Optional. The maximum number of data agents to return in this call. Must be a positive integer. Only set this to page through the results manually: when both `page_size` and `page_token` are omitted, every page is fetched automatically and all accessible data agents are returned.",
			},
			"page_token": map[string]any{
				"type":        "string",
				"description": "Optional. A page token returned as `nextPageToken` by a previous call, used to fetch the next page. Only set this to page through the results manually; omit it to fetch all accessible data agents at once.",
			},
		},
		"required": []any{},
	}
	getInfoSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data_agent_id": map[string]any{
				"type":        "string",
				"description": "The ID of the data agent to retrieve info for.",
			},
		},
		"required": []any{"data_agent_id"},
	}
	askSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data_agent_id": map[string]any{
				"type":        "string",
				"description": "The ID of the data agent to ask.",
			},
			"user_query_with_context": map[string]any{
				"type":        "string",
				"description": "The question to ask the agent, potentially including conversation history for context.",
			},
		},
		"required": []any{"data_agent_id", "user_query_with_context"},
	}
	return []tests.MCPToolManifest{
		{Name: "my-list-accessible-data-agents-tool", Description: "Tool to list data agents.", InputSchema: listSchema},
		{Name: "my-auth-list-accessible-data-agents-tool", Description: "Tool to list data agents with auth.", InputSchema: listSchema},
		{Name: "my-client-auth-list-accessible-data-agents-tool", Description: "Tool to list data agents with client auth.", InputSchema: listSchema},
		{Name: "my-get-data-agent-info-tool", Description: "Tool to get data agent info.", InputSchema: getInfoSchema},
		{Name: "my-auth-get-data-agent-info-tool", Description: "Tool to get data agent info with auth.", InputSchema: getInfoSchema},
		{Name: "my-client-auth-get-data-agent-info-tool", Description: "Tool to get data agent info with client auth.", InputSchema: getInfoSchema},
		{Name: "my-ask-data-agent-tool", Description: "Tool to ask data agent.", InputSchema: askSchema},
		{Name: "my-auth-ask-data-agent-tool", Description: "Tool to ask data agent with auth.", InputSchema: askSchema},
		{Name: "my-client-auth-ask-data-agent-tool", Description: "Tool to ask data agent with client auth.", InputSchema: askSchema},
	}
}

func TestCloudGDAConversationalAnalyticsMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cloudGDATransport{isMCP: true}.startServer(t, ctx, getConversationalAnalyticsToolsConfig(getCloudGDAProject(t)))

	tests.RunMCPToolsListMethod(t, getConversationalAnalyticsMCPManifests())
}

func TestCloudGDAConversationalAnalyticsMCPTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	toolsFile, dataAgentID, dataAgentDisplayName, secondDataAgentDisplayName := setupConversationalAnalyticsTest(t, ctx)

	tr := cloudGDATransport{isMCP: true}
	tr.startServer(t, ctx, toolsFile)

	runConversationalAnalyticsTests(t, ctx, tr, dataAgentID, dataAgentDisplayName, secondDataAgentDisplayName)
}
