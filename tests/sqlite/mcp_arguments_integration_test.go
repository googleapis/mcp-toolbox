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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

func TestMCPToolCallPreservesIntegerArguments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	config := map[string]any{
		"sources": map[string]any{
			"argument-db": map[string]any{"type": "sqlite", "database": ":memory:"},
		},
		"tools": map[string]any{
			"echo-integer": map[string]any{
				"type":        "sqlite-sql",
				"source":      "argument-db",
				"description": "Return the integer bound to the query.",
				"statement":   "SELECT ? AS value",
				"parameters": []any{
					map[string]any{"name": "value", "type": "integer", "description": "The integer to return."},
				},
			},
		},
	}
	cmd, cleanup, err := tests.StartCmd(ctx, config, "--address", "127.0.0.1", "--port", "23530", "--disable-reload")
	if err != nil {
		t.Fatalf("command initialization returned an error: %v", err)
	}
	defer cleanup()
	defer func() {
		cmd.Stop()
		cmd.Close()
		stopCtx, cancelStop := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelStop()
		if err := cmd.Wait(stopCtx); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("toolbox shutdown failed: %v", err)
		}
	}()
	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs:\n%s", out)
		t.Fatalf("toolbox did not start: %v", err)
	}

	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "large integer", value: "9007199254740993", want: `{"value":9007199254740993}`},
		{name: "exponent integer control", value: "1e3", want: `{"value":1000}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Keep the numeric token intact on the wire, including its exponent spelling.
			body := fmt.Sprintf(`{"jsonrpc":"2.0","id":"integer-argument","method":"tools/call","params":{"name":"echo-integer","arguments":{"value":%s}}}`, tc.value)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:23530/mcp", strings.NewReader(body))
			if err != nil {
				t.Fatalf("unable to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("MCP-Protocol-Version", "2025-11-25")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %v", err)
			}
			defer resp.Body.Close()
			responseBody, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("unable to read response: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected status OK, got %s: %s", resp.Status, responseBody)
			}
			var result tests.MCPCallToolResponse
			if err := json.Unmarshal(responseBody, &result); err != nil {
				t.Fatalf("unable to decode response: %v", err)
			}
			if result.Error != nil || result.Result.IsError {
				t.Fatalf("tools/call failed: %s", responseBody)
			}
			if len(result.Result.Content) != 1 {
				t.Fatalf("expected one result row, got %s", responseBody)
			}
			// Compare the result text directly, so the assertion cannot round the integer itself.
			if got := result.Result.Content[0].Text; got != tc.want {
				t.Errorf("unexpected bound integer: got %s, want %s", got, tc.want)
			}
		})
	}
}
