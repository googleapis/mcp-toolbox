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

package redis

import (
	"testing"

	"github.com/googleapis/mcp-toolbox/tests"
)

func TestRedisMCPListTools(t *testing.T) {
	setupRedisTest(t)
	expectedTools := tests.GetBaseMCPExpectedTools()
	for i := range expectedTools {
		if expectedTools[i].Name == "my-array-tool" {
			// Redis accepts a command array rather than SQL ID/name arrays.
			expectedTools[i].InputSchema = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cmdArray": map[string]any{"type": "array", "description": "cmd array", "items": map[string]any{"type": "string", "description": "field"}},
				},
				"required": []any{"cmdArray"},
			}
		}
	}
	tests.RunMCPToolsListMethod(t, expectedTools)
}

func TestRedisMCPCallTools(t *testing.T) {
	setupRedisTest(t)
	runRedisCallTests(t, tests.WithMCP())
}
