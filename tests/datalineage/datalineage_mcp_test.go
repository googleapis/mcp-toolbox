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

package datalineage_test

import (
	"context"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/tests"
)

// datalineageMCP is the transport used by the MCP tests.
var datalineageMCP = datalineageTransport{isMCP: true}

func TestDatalineageMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	datalineageMCP.startServer(t, ctx, getDatalineageToolsConfig(getDatalineageVars(t)))

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getDatalineageMCPExpectedTools())
	})
}

func TestDatalineageMCPToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	toolsFile, sourceFQN, targetFQN, processName := setupDatalineageTest(t, ctx)
	datalineageMCP.startServer(t, ctx, toolsFile)

	runDatalineageSearchTests(t, ctx, datalineageMCP, sourceFQN, targetFQN, processName)
}

// getDatalineageMCPExpectedTools returns the MCP manifest for the tool loaded
// by getDatalineageToolsConfig.
func getDatalineageMCPExpectedTools() []tests.MCPToolManifest {
	return []tests.MCPToolManifest{
		{
			Name:        "my-datalineage-search-tool",
			Description: "Data Lineage search tool to test end to end functionality.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"direction":               map[string]any{"description": "Required. Direction of the search.", "type": "string"},
					"locations":               map[string]any{"description": "Required. The locations to search in. Must contain at least 1 location. The first location will be used to initiate the search.", "items": map[string]any{"description": "A location to search in (e.g., 'us', 'eu', 'global').", "type": "string"}, "type": "array"},
					"max_depth":               map[string]any{"description": "Optional. The maximum depth of the search. Default is 5, max is 100.", "type": "integer"},
					"max_process_per_link":    map[string]any{"description": "Optional. The maximum number of processes to return per link. Default is 0, max is 100. Must be greater than 0 if request_process_details is true.", "type": "integer"},
					"max_results":             map[string]any{"description": "Optional. The maximum number of links to return in the response. Default is 1000, max is 10000.", "type": "integer"},
					"request_process_details": map[string]any{"description": "Optional. If true, retrieves full process details (displayName, attributes, origin) for the links. Requires max_process_per_link to be greater than 0.", "type": "boolean"},
					"root_entities":           map[string]any{"description": "Required. The starting entities for the search. Each object must have 'fully_qualified_name' (string) and optionally 'fields' (array of strings).", "items": map[string]any{"additionalProperties": true, "description": "Entity reference containing fully_qualified_name and optional fields.", "type": "object"}, "type": "array"},
				},
				"required": []any{"locations", "root_entities", "direction"},
			},
		},
	}
}
