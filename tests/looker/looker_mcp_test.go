// Copyright 2025 Google LLC
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

package looker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/tests"
)

type lookerTestOption func(*bool)

func withLookerMCP() lookerTestOption { return func(mcp *bool) { *mcp = true } }

func lookerMCPEnabled(options []lookerTestOption) bool {
	mcp := false
	for _, option := range options {
		option(&mcp)
	}
	return mcp
}

func TestLookerMCPListTools(t *testing.T) {
	setupLooker(t, getLookerVars(t))
	tests.RunMCPToolsListMethod(t, lookerExpectedTools(t))
}
func TestLookerMCPCallTools(t *testing.T) {
	setupLooker(t, getLookerVars(t))
	runLookerCallTools(t, withLookerMCP())
}
func runLookerToolContains(t *testing.T, name string, params []byte, want string, options ...lookerTestOption) {
	t.Helper()
	t.Run("invoke "+name, func(t *testing.T) {
		got := invokeLookerTool(t, "http://127.0.0.1:5000/api/tool/"+name+"/invoke", params, options...)
		if !strings.Contains(string(got), want) {
			t.Fatalf("result = %s, want to contain %q", got, want)
		}
	})
}
func invokeLookerTool(t *testing.T, api string, params []byte, options ...lookerTestOption) []byte {
	t.Helper()
	if !lookerMCPEnabled(options) {
		resp, data := tests.RunRequest(t, http.MethodPost, api, bytes.NewReader(params), nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status=%d: %s", resp.StatusCode, data)
		}
		var result struct {
			Result string `json:"result"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return []byte(result.Result)
	}
	var args map[string]any
	if err := json.Unmarshal(params, &args); err != nil {
		t.Fatal(err)
	}
	name := strings.TrimSuffix(strings.TrimPrefix(api, "http://127.0.0.1:5000/api/tool/"), "/invoke")
	status, resp, err := tests.InvokeMCPTool(t, name, args, nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("%s status=%d: %v", name, status, err)
	}
	result, err := lookerMCPResult(resp)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func lookerMCPResult(resp *tests.MCPCallToolResponse) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("missing MCP response")
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("JSON-RPC error: %+v", resp.Error)
	}
	if resp.Result.IsError {
		return nil, fmt.Errorf("tool error: %+v", resp.Result)
	}
	if resp.Result.Content == nil {
		return nil, fmt.Errorf("missing content array")
	}
	values := make([]json.RawMessage, 0, len(resp.Result.Content))
	for _, block := range resp.Result.Content {
		if block.Type != "text" || !json.Valid([]byte(block.Text)) {
			return nil, fmt.Errorf("invalid JSON text content: %+v", block)
		}
		values = append(values, json.RawMessage(block.Text))
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return json.Marshal(values)
}
