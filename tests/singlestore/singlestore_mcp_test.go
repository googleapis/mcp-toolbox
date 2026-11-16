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

package singlestore

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// startSingleStoreMCPServer starts the toolbox server with toolsFile and waits
// until it is ready to serve.
func startSingleStoreMCPServer(t *testing.T, ctx context.Context, toolsFile map[string]any) {
	t.Helper()
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

// getSingleStoreMCPExpectedTools returns the MCP manifests for the tools loaded
// by setupSingleStoreTest. SingleStore doesn't support array parameters, so
// getSingleStoreToolsConfig removes my-array-tool.
func getSingleStoreMCPExpectedTools() []tests.MCPToolManifest {
	expectedTools := []tests.MCPToolManifest{}
	for _, manifest := range tests.GetBaseMCPExpectedTools() {
		if manifest.Name == "my-array-tool" {
			continue
		}
		expectedTools = append(expectedTools, manifest)
	}
	expectedTools = append(expectedTools, tests.GetExecuteSQLMCPExpectedTools()...)
	return append(expectedTools, tests.GetTemplateParamMCPExpectedTools()...)
}

func TestSingleStoreMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	_, _, toolsFile := setupSingleStoreTest(t, ctx)
	startSingleStoreMCPServer(t, ctx, toolsFile)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getSingleStoreMCPExpectedTools())
	})
}

func TestSingleStoreMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tableNameTemplateParam, _, toolsFile := setupSingleStoreTest(t, ctx)
	startSingleStoreMCPServer(t, ctx, toolsFile)

	select1Want, mcpMyFailToolWant, createTableStatement, mcpSelect1Want := getSingleStoreWants()

	tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest(), tests.WithMCP())
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, tests.WithMCPSql())
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithMCPTemplate())
}
