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

package firestoremongodb

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// setupFirestoreMongodbMCPServer starts a Toolbox server serving the Firestore
// MongoDB-compatible tools over the MCP endpoint. Teardown is registered with
// t.Cleanup, so nothing leaks if setup fails partway through.
func setupFirestoreMongodbMCPServer(t *testing.T, ctx context.Context) {
	sourceConfig := getFirestoreMongodbVars(t)

	// Write config into a file and pass it to command
	toolsFile := getFirestoreMongodbToolsConfig(sourceConfig)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	// StartCmd's cleanup only removes the temporary config file, so Close is
	// what stops the server and closes its pipes.
	t.Cleanup(func() {
		cmd.Close()
		cleanup()
	})

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

func TestFirestoreMongodbMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	setupFirestoreMongodbMCPServer(t, ctx)

	// This source uses its own tools config rather than GetToolsConfig, so none
	// of the shared expected-tool helpers apply and the manifest is the two
	// tools it registers.
	expectedTools := []tests.MCPToolManifest{
		{
			Name:        "firestore-mongodb-get-schema",
			Description: "Get schema for Firestore collections",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"collection": map[string]any{
						"type":        "string",
						"default":     "",
						"description": "Optional name or path of a specific collection to get schema for. If omitted, schemas for all root collections are returned.",
					},
				},
				"required": []any{},
			},
		},
		{
			Name:        "firestore-mongodb-execute-mql",
			Description: "Execute MQL query or aggregation pipeline against Firestore",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The MQL query or aggregation pipeline to execute against Firestore.",
					},
				},
				"required": []any{"query"},
			},
		},
	}

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, expectedTools)
	})
}

func TestFirestoreMongodbMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	setupFirestoreMongodbMCPServer(t, ctx)

	// Run tool execution tests against the pre-created collection, over MCP.
	runFirestoreMongodbGetSchemaTest(t, precreatedCollection, tests.WithMCPExec())
	runFirestoreMongodbExecuteMQLTest(t, precreatedCollection, tests.WithMCPExec())
}
