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

package clickhouse

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

// TestClickHouseExecuteSQLQueryParameters checks that clickhouse-execute-sql
// binds params on the server: {name:Type} placeholders get typed values, and a
// value is never read as SQL.
func TestClickHouseExecuteSQLQueryParameters(t *testing.T) {
	sourceConfig := getClickHouseVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{
			"execute-sql-tool": map[string]any{
				"type":        "clickhouse-execute-sql",
				"source":      "my-instance",
				"description": "Test query parameters",
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

	t.Run("typed values", func(t *testing.T) {
		tests.RunMCPCustomToolCallMethod(t, "execute-sql-tool", map[string]any{
			"sql":    "SELECT {n:UInt8} AS n, has({names:Array(String)}, 'b') AS found, {s:String} AS s, {d:Date} AS d",
			"params": map[string]any{"n": "42", "names": "['a','b']", "s": "x'; DROP TABLE t --", "d": "2024-01-31"},
		}, `{"d":"2024-01-31T00:00:00Z","found":1,"n":42,"s":"x'; DROP TABLE t --"}`)
	})
	t.Run("identifiers", func(t *testing.T) {
		tests.RunMCPCustomToolCallMethod(t, "execute-sql-tool", map[string]any{
			"sql":    "SELECT count() AS rows FROM {db:Identifier}.{t:Identifier}",
			"params": map[string]any{"db": "system", "t": "one"},
		}, `{"rows":1}`)
	})
	t.Run("without params", func(t *testing.T) {
		tests.RunMCPCustomToolCallMethod(t, "execute-sql-tool", map[string]any{"sql": "SELECT 1 AS one"}, `{"one":1}`)
	})

	errorCases := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{
			name:    "identifier value is not SQL",
			args:    map[string]any{"sql": "SELECT count() FROM system.{t:Identifier}", "params": map[string]any{"t": "one` UNION ALL SELECT 1 --"}},
			wantErr: "UNKNOWN_TABLE",
		},
		{
			name:    "placeholder without a value",
			args:    map[string]any{"sql": "SELECT {missing:String} AS m"},
			wantErr: "UNKNOWN_QUERY_PARAMETER",
		},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, "execute-sql-tool", tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing execute-sql-tool: %s", err)
			}
			if statusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", statusCode)
			}
			if !mcpResp.Result.IsError {
				t.Fatalf("expected an error result, got %v", mcpResp.Result)
			}
			if len(mcpResp.Result.Content) == 0 || !strings.Contains(mcpResp.Result.Content[0].Text, tc.wantErr) {
				t.Errorf("expected the error to contain %q, got %v", tc.wantErr, mcpResp.Result.Content)
			}
		})
	}
}
