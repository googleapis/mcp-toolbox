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

package sqlite

import (
	"testing"

	"github.com/googleapis/mcp-toolbox/tests"
)

func TestSQLiteMCPListTools(t *testing.T) {
	t.Run("tools", func(t *testing.T) {
		setupSQLiteTest(t)
		expected := tests.GetBaseMCPExpectedTools()
		expected = append(expected, tests.GetTemplateParamMCPExpectedTools()...)
		tests.RunMCPToolsListMethod(t, expected)
	})
	t.Run("execute_sql", func(t *testing.T) {
		setupSQLiteExecuteSQLTest(t)
		// This fixture only configures the unauthenticated execute-SQL tool.
		for _, tool := range tests.GetExecuteSQLMCPExpectedTools() {
			if tool.Name == "my-exec-sql-tool" {
				tool.Description = "Tool to execute SQL statements"
				tests.RunMCPToolsListMethod(t, []tests.MCPToolManifest{tool})
				return
			}
		}
		t.Fatal("execute-SQL manifest is missing")
	})
}

func TestSQLiteMCPCallTools(t *testing.T) {
	t.Run("tools", func(t *testing.T) {
		tableName := setupSQLiteTest(t)
		runSQLiteCallTests(t, tableName, []tests.InvokeTestOption{tests.WithMCP()}, []tests.TemplateParamOption{tests.WithMCPTemplate()})
	})
	t.Run("execute_sql", func(t *testing.T) {
		tableName := setupSQLiteExecuteSQLTest(t)
		runSQLiteExecuteSQLTests(t, tableName, tests.WithMCPExec())
	})
}
