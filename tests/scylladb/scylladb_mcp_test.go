// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scylladb

import (
	"testing"

	"github.com/googleapis/mcp-toolbox/tests"
)

func TestScyllaDBMCPListTools(t *testing.T) {
	setupScyllaDBTest(t)
	expected := tests.GetBaseMCPExpectedTools()
	expected = append(expected, tests.GetTemplateParamMCPExpectedTools()...)
	tests.RunMCPToolsListMethod(t, expected)
}

func TestScyllaDBMCPCallTools(t *testing.T) {
	tableName := setupScyllaDBTest(t)
	// MCP wraps the null row result in a text content item.
	runScyllaDBCallTests(t, tableName, []tests.InvokeTestOption{tests.WithMCP()},
		[]tests.TemplateParamOption{tests.WithMCPTemplate(), tests.WithSelectEmptyWant(`[null]`)})
}
