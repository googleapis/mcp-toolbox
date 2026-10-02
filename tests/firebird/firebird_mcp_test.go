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

package firebird

import (
	"testing"

	"github.com/googleapis/mcp-toolbox/tests"
)

func TestFirebirdMCPListTools(t *testing.T) {
	setupFirebirdTest(t)
	tests.RunMCPToolsListMethod(t, getFirebirdMCPExpectedTools())
}

func TestFirebirdMCPCallTools(t *testing.T) {
	tableName := setupFirebirdTest(t)
	runFirebirdCallTests(t, tableName, []tests.InvokeTestOption{tests.WithMCP()},
		[]tests.ExecuteSqlOption{tests.WithMCPSql()}, []tests.TemplateParamOption{tests.WithMCPTemplate()})
}

// Firebird customizes the array parameter and template manifests in its fixtures.
func getFirebirdMCPExpectedTools() []tests.MCPToolManifest {
	expected := tests.GetBaseMCPExpectedTools()
	for i := range expected {
		if expected[i].Name == "my-array-tool" {
			expected[i].InputSchema = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"idArray": map[string]any{
						"type": "array", "description": "ID array (Firebird will use first element only)",
						"items": map[string]any{"type": "integer", "description": "ID"},
					},
				},
				"required": []any{"idArray"},
			}
		}
	}
	expected = append(expected, tests.GetExecuteSQLMCPExpectedTools()...)
	templates := tests.GetTemplateParamMCPExpectedTools()
	for i := range templates {
		switch templates[i].Name {
		case "insert-table-templateParams-tool":
			templates[i].Description = "Insert table tool with template parameters"
		case "select-templateParams-tool":
			templates[i].Description = "Select table tool with template parameters"
		case "select-templateParams-combined-tool":
			templates[i].Description = "Select table tool with combined template parameters"
		case "select-fields-templateParams-tool":
			templates[i].Description = "Select specific fields tool with template parameters"
			templates[i].InputSchema = map[string]any{
				"type":       "object",
				"properties": map[string]any{"tableName": map[string]any{"type": "string", "description": "some description"}},
				"required":   []any{"tableName"},
			}
		case "select-filter-templateParams-combined-tool":
			templates[i].Description = "Select table tool with filter template parameters"
			properties := templates[i].InputSchema["properties"].(map[string]any)
			properties["name"] = map[string]any{"type": "string", "description": "the name to filter by"}
		}
	}
	return append(expected, templates...)
}
