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

package dataplex

import (
	"context"
	_ "embed"
	"encoding/json"
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

type dataplexTestOption func(*bool)

func withDataplexMCP() dataplexTestOption { return func(mcp *bool) { *mcp = true } }
func dataplexMCPEnabled(options []dataplexTestOption) bool {
	mcp := false
	for _, option := range options {
		option(&mcp)
	}
	return mcp
}

func TestDataplexMCPCallTools(t *testing.T) { runDataplexCallTools(t, withDataplexMCP()) }

func TestDataplexMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	cmd, cleanup, err := tests.StartCmd(ctx, getDataplexToolsConfig(getDataplexVars(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	t.Cleanup(func() {
		cmd.Stop()
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := cmd.Wait(shutdown); err != nil {
			t.Errorf("stop toolbox: %v", err)
		}
		cmd.Close()
	})
	if out, err := testutils.WaitForString(ctx, regexp.MustCompile(`Server ready to serve`), cmd.Out); err != nil {
		t.Fatalf("start toolbox: %v\n%s", err, out)
	}
	runDataplexToolGetTest(t, withDataplexMCP())
}

// invokeDataplexTool decodes the selected protocol before the shared domain assertions.
func invokeDataplexTool(t *testing.T, api string, body io.Reader, headers map[string]string, wantStatus int, wantError string, options ...dataplexTestOption) string {
	t.Helper()
	if !dataplexMCPEnabled(options) {
		resp, data := tests.RunRequest(t, http.MethodPost, api, body, headers)
		if resp.StatusCode != wantStatus {
			t.Fatalf("status = %d, want %d: %s", resp.StatusCode, wantStatus, data)
		}
		if wantStatus != http.StatusOK {
			return ""
		}
		var result struct {
			Result string `json:"result"`
			Error  any    `json:"error"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		if wantError != "" {
			// Agent errors are JSON inside the legacy API's result string.
			var nested struct {
				Error string `json:"error"`
			}
			_ = json.Unmarshal([]byte(result.Result), &nested)
			message := nested.Error
			if result.Error != nil {
				message = fmt.Sprint(result.Error)
			}
			if !regexp.MustCompile("(?i)" + wantError).MatchString(message) {
				t.Fatalf("expected error %q, got %s", wantError, data)
			}
		}
		return result.Result
	}
	var args map[string]any
	if err := json.NewDecoder(body).Decode(&args); err != nil {
		t.Fatal(err)
	}
	name := strings.TrimSuffix(strings.TrimPrefix(api, "http://127.0.0.1:5000/api/tool/"), "/invoke")
	status, response, err := tests.InvokeMCPTool(t, name, args, headers)
	if status != wantStatus {
		t.Fatalf("%s status = %d, want %d: %v", name, status, wantStatus, err)
	}
	if wantStatus == http.StatusUnauthorized {
		// Check the intended authentication failure, not an arbitrary HTTP error.
		if response == nil || response.Error == nil || !strings.Contains(strings.ToLower(response.Error.Message), "unauthorized") {
			t.Fatalf("expected unauthorized Tool call: response=%+v, error=%v", response, err)
		}
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	text, err := dataplexMCPResult(response, wantError)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func dataplexMCPResult(response *tests.MCPCallToolResponse, wantError string) (string, error) {
	if response == nil {
		return "", fmt.Errorf("missing response")
	}
	if response.Error != nil {
		return "", fmt.Errorf("unexpected JSON-RPC error: %+v", response.Error)
	}
	if response.Result.IsError != (wantError != "") {
		return "", fmt.Errorf("unexpected isError: %+v", response.Result)
	}
	blocks := response.Result.Content
	if blocks == nil {
		return "", fmt.Errorf("missing content array")
	}
	if wantError != "" {
		if len(blocks) != 1 || blocks[0].Type != "text" || !regexp.MustCompile("(?i)"+wantError).MatchString(blocks[0].Text) {
			return "", fmt.Errorf("expected error %q: %+v", wantError, blocks)
		}
		return "", nil
	}
	// Dataplex returns typed slices, maps and proto pointers. Each is serialized
	// as one JSON text block by the MCP server, including [] and null results.
	if len(blocks) != 1 || blocks[0].Type != "text" || !json.Valid([]byte(blocks[0].Text)) {
		return "", fmt.Errorf("expected one JSON text result: %+v", blocks)
	}
	return blocks[0].Text, nil
}

//go:embed testdata/mcp_tools.json
var dataplexToolsJSON []byte

func dataplexExpectedTools(t *testing.T) []tests.MCPToolManifest {
	t.Helper()
	var tools []tests.MCPToolManifest
	if err := json.Unmarshal(dataplexToolsJSON, &tools); err != nil {
		t.Fatal(err)
	}
	return tools
}
