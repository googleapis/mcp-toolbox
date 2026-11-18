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

package cloudsqlpg

import (
	"testing"

	"github.com/googleapis/mcp-toolbox/tests"
)

func TestPreCheckMCPListTools(t *testing.T) {
	setupPreCheckTest(t)
	var expected []tests.MCPToolManifest
	for _, name := range []string{"precheck-tool"} {
		expected = append(expected, tests.MCPToolManifest{Name: name, Description: "Check PostgreSQL upgrade readiness.", InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project":               map[string]any{"type": "string", "description": "The project ID"},
				"instance":              map[string]any{"type": "string", "description": "The name of the instance to check"},
				"targetDatabaseVersion": map[string]any{"type": "string", "description": "The target PostgreSQL version for the upgrade (e.g., POSTGRES_18). If not specified, defaults to the PostgreSQL 18.", "default": "POSTGRES_18"},
			},
			"required": []any{"project", "instance"},
		}})
	}
	tests.RunMCPToolsListMethod(t, expected)
}

func TestPreCheckMCPCallTools(t *testing.T) {
	ctx := setupPreCheckTest(t)
	runPreCheckToolCalls(t, ctx, withPreCheckMCP)
}
