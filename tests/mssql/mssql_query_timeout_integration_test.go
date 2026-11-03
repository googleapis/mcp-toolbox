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

package mssql

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// TestMSSQLQueryTimeout needs only a database (no auth service), so it also
// runs against a local SQL Server.
func TestMSSQLQueryTimeout(t *testing.T) {
	sourceConfig := getMsSQLVars(t)
	sourceConfig["queryTimeout"] = "1s"
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{
			"my-exec-sql-tool": map[string]any{
				"type":        "mssql-execute-sql",
				"source":      "my-instance",
				"description": "Tool to execute sql",
			},
		},
	}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	t.Run("query longer than queryTimeout is cancelled", func(t *testing.T) {
		started := time.Now()
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, "my-exec-sql-tool", map[string]any{"sql": "WAITFOR DELAY '00:00:10'; SELECT 1 AS a"}, nil)
		elapsed := time.Since(started)
		if err != nil {
			t.Fatalf("native error executing my-exec-sql-tool: %s", err)
		}
		if statusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", statusCode)
		}
		if !mcpResp.Result.IsError {
			t.Fatalf("expected an error result, got %v", mcpResp.Result)
		}
		if len(mcpResp.Result.Content) == 0 || !strings.Contains(mcpResp.Result.Content[0].Text, "queryTimeout of 1s") {
			t.Errorf("expected the error to name the queryTimeout, got %v", mcpResp.Result.Content)
		}
		if elapsed > 5*time.Second {
			t.Errorf("expected the query to be cancelled after about 1s, it took %s", elapsed)
		}
	})

	t.Run("source is usable after a cancelled query", func(t *testing.T) {
		tests.RunMCPCustomToolCallMethod(t, "my-exec-sql-tool", map[string]any{"sql": "SELECT 1 AS a"}, `[{"a":1}]`)
	})
}
