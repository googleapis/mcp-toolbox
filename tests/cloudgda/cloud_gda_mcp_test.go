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

func getCloudGdaMCPManifests() []tests.MCPToolManifest {
	return []tests.MCPToolManifest{
		{
			Name:        "cloud-gda-query",
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
	}
}

func TestCloudGdaMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	startCloudGdaMockServer(t)
	cloudGDATransport{isMCP: true}.startServer(t, ctx, getCloudGdaToolsConfig())

	tests.RunMCPToolsListMethod(t, getCloudGdaMCPManifests())
}

func TestCloudGdaMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	startCloudGdaMockServer(t)
	tr := cloudGDATransport{isMCP: true}
	tr.startServer(t, ctx, getCloudGdaToolsConfig())

	toolName := "cloud-gda-query"
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
	createSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data_agent_id": map[string]any{
				"type":        "string",
				"description": `The ID to use for the new data agent. Must start with a lowercase letter, contain only lowercase letters, digits, and hyphens, end with a lowercase letter or digit, and be at most 63 characters long (e.g. "sales-agent").`,
			},
			"agent_config": map[string]any{
				"type": "object",
				"description": `The JSON representation of the DataAgent resource to create. Example:
{
  "displayName": "My Support Agent",
  "description": "An agent that analyzes support tickets.",
  "dataAnalyticsAgent": {
    "publishedContext": {
      "datasourceReferences": {
        "bq": {
          "tableReferences": [
            {
              "projectId": "my-project",
              "datasetId": "support_data",
              "tableId": "tickets"
            }
          ]
        }
      },
      "systemInstruction": "You are a helpful assistant."
    }
  }
}`,
				"additionalProperties": true,
			},
		},
		"required": []any{"data_agent_id", "agent_config"},
	}
	updateSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data_agent_id": map[string]any{
				"type":        "string",
				"description": "The ID of the data agent to update.",
			},
			"agent_config": map[string]any{
				"type": "object",
				"description": `The JSON representation of the DataAgent resource fields to update. Example:
{
  "displayName": "Updated Support Agent",
  "description": "Updated description",
  "dataAnalyticsAgent": {
    "publishedContext": {
      "systemInstruction": "You are a helpful support data analyst."
    }
  }
}`,
				"additionalProperties": true,
			},
			"update_mask": map[string]any{
				"type":        "string",
				"description": `Comma-separated list of fields to update, using the API's camelCase JSON field names (e.g. "displayName,description" or "dataAnalyticsAgent.publishedContext.systemInstruction"). Every field listed here must also be present in agent_config.`,
			},
		},
		"required": []any{"data_agent_id", "agent_config", "update_mask"},
	}
	deleteSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data_agent_id": map[string]any{
				"type":        "string",
				"description": "The ID of the data agent to delete.",
			},
		},
		"required": []any{"data_agent_id"},
	}
	schemas := map[string]map[string]any{
		"list-accessible-data-agents": listSchema,
		"get-data-agent-info":         getInfoSchema,
		"ask-data-agent":              askSchema,
		"create-data-agent":           createSchema,
		"update-data-agent":           updateSchema,
		"delete-data-agent":           deleteSchema,
	}
	var manifests []tests.MCPToolManifest
	for _, tool := range conversationalAnalyticsTestTools() {
		manifests = append(manifests, tests.MCPToolManifest{Name: tool.name, Description: tool.description, InputSchema: schemas[tool.action]})
	}
	return manifests
}

func TestCloudGDAConversationalAnalyticsMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	cloudGDATransport{isMCP: true}.startServer(t, ctx, getConversationalAnalyticsToolsConfig(getCloudGDAProject(t)))

	tests.RunMCPToolsListMethod(t, getConversationalAnalyticsMCPManifests())
}

func TestCloudGDAConversationalAnalyticsMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	toolsFile, projectID, datasetName, tableName, secondDataAgentDisplayName := setupConversationalAnalyticsTest(t, ctx)

	tr := cloudGDATransport{isMCP: true}
	tr.startServer(t, ctx, toolsFile)

	runConversationalAnalyticsTests(t, ctx, tr, projectID, datasetName, tableName, secondDataAgentDisplayName)
}
