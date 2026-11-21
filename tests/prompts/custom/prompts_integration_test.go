// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package custom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"testing"
	"time"

	_ "github.com/googleapis/mcp-toolbox/internal/prompts/custom"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

func getPromptsConfig() map[string]any {
	return map[string]any{
		"prompts": map[string]any{
			"prompt1": map[string]any{
				"description": "description1.",
				"arguments": []any{
					map[string]any{
						"name":        "arg1",
						"description": "Description of arg1 for prompt1",
					},
				},
				"messages": []any{
					map[string]any{
						"content": "content arg1: {{.arg1}}",
					},
				},
			},
			"prompt2": map[string]any{
				"description": "description2",
				"arguments": []any{
					map[string]any{
						"name":        "arg1",
						"description": "Description for arg1 for prompt2",
					},
					map[string]any{
						"name":        "arg2",
						"description": "Description for arg2 for prompt2",
					},
				},
				"messages": []any{
					map[string]any{
						"role":    "user",
						"content": "message1 args: {{.arg1}}, {{.arg2}}",
					},
					map[string]any{
						"role":    "assistant",
						"content": "{{.arg1}}",
					},
				},
			},
			"typed_prompt": map[string]any{
				"arguments": []any{
					map[string]any{"name": "count", "type": "integer", "description": "A count"},
					map[string]any{"name": "ratio", "type": "float", "description": "A ratio"},
					map[string]any{"name": "enabled", "type": "boolean", "description": "Whether enabled"},
					map[string]any{
						"name": "tags", "type": "array", "description": "Numeric tags",
						"items": map[string]any{"name": "item", "type": "integer", "description": "A tag"},
					},
					map[string]any{"name": "limits", "type": "map", "valueType": "integer", "description": "Numeric limits"},
					map[string]any{"name": "literal", "description": "Literal text"},
				},
				"messages": []any{
					map[string]any{
						"content": `count={{.count}} ratio={{.ratio}} enabled={{.enabled}} tags={{range .tags}}[{{.}}]{{end}} limit={{index .limits "max"}} literal={{.literal}}`,
					},
				},
			},
		},
	}
}

// TestMCPPromptsIntegration is the main entrypoint for the prompts integration tests.
func TestMCPPromptsIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}
	cmd, cleanup, err := tests.StartCmd(ctx, getPromptsConfig(), args...)
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

	waitCtx, cancelWait := context.WithTimeout(ctx, 20*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %v", err)
	}

	runMCPListPromptsTest(t)
	runMCPGetPromptTest(t)
	runMCPGetTypedPromptTest(t)
}

func runMCPListPromptsTest(t *testing.T) {
	t.Run("MCP_prompts/list", func(t *testing.T) {
		api := "http://127.0.0.1:5000/mcp"
		reqBody := jsonrpc.JSONRPCRequest{
			Jsonrpc: "2.0",
			Id:      "list-prompts-1",
			Request: jsonrpc.Request{Method: "prompts/list"},
		}
		jsonBody, _ := json.Marshal(reqBody)

		resp, err := http.Post(api, "application/json", bytes.NewBuffer(jsonBody))
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status OK; got %s", resp.Status)
		}

		var result struct {
			Result struct {
				Prompts []map[string]any `json:"prompts"`
			} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(result.Result.Prompts) != 3 {
			t.Errorf("expected 3 prompts, got %d", len(result.Result.Prompts))
		}
	})
}

func runMCPGetPromptTest(t *testing.T) {
	t.Run("MCP_prompts/get", func(t *testing.T) {
		api := "http://127.0.0.1:5000/mcp"
		reqBody := jsonrpc.JSONRPCRequest{
			Jsonrpc: "2.0",
			Id:      "get-prompt-2",
			Request: jsonrpc.Request{Method: "prompts/get"},
			Params: map[string]any{
				"name": "prompt2",
				"arguments": map[string]any{
					"arg1": "value1",
					"arg2": "value2",
				},
			},
		}
		jsonBody, _ := json.Marshal(reqBody)

		resp, err := http.Post(api, "application/json", bytes.NewBuffer(jsonBody))
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status OK; got %s, body: %s", resp.Status, string(body))
		}

		var result map[string]any
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		messages := result["result"].(map[string]any)["messages"].([]any)
		if len(messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(messages))
		}

		// Check first message
		msg1 := messages[0].(map[string]any)
		content1 := msg1["content"].(map[string]any)["text"].(string)
		expectedContent1 := "message1 args: value1, value2"
		if content1 != expectedContent1 {
			t.Errorf("unexpected content in message 1.\nGot: %q\nWant: %q", content1, expectedContent1)
		}

		// Check second message
		msg2 := messages[1].(map[string]any)
		content2 := msg2["content"].(map[string]any)["text"].(string)
		expectedContent2 := "value1"
		if content2 != expectedContent2 {
			t.Errorf("unexpected content in message 2.\nGot: %q\nWant: %q", content2, expectedContent2)
		}
	})
}

func runMCPGetTypedPromptTest(t *testing.T) {
	t.Run("MCP_prompts/get_typed_arguments", func(t *testing.T) {
		reqBody := jsonrpc.JSONRPCRequest{
			Jsonrpc: "2.0",
			Id:      "get-typed-prompt",
			Request: jsonrpc.Request{Method: "prompts/get"},
			Params: map[string]any{
				"name": "typed_prompt",
				"arguments": map[string]string{
					"count": "0", "ratio": "1.5", "enabled": "false",
					"tags": "[0,3]", "limits": `{"max":5}`, "literal": `{"x":true}`,
				},
			},
		}
		jsonBody, _ := json.Marshal(reqBody)
		resp, err := http.Post("http://127.0.0.1:5000/mcp", "application/json", bytes.NewBuffer(jsonBody))
		if err != nil {
			t.Fatalf("failed to send request: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read response: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status OK; got %s, body: %s", resp.Status, body)
		}
		var result struct {
			Result struct {
				Messages []struct {
					Content struct{ Text string }
				}
			}
			Error *struct {
				Code    int
				Message string
			}
		}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if result.Error != nil {
			t.Fatalf("prompts/get returned an error: %+v", result.Error)
		}
		if len(result.Result.Messages) != 1 {
			t.Fatalf("expected one prompt message, got %s", body)
		}
		want := `count=0 ratio=1.5 enabled=false tags=[0][3] limit=5 literal={"x":true}`
		if got := result.Result.Messages[0].Content.Text; got != want {
			t.Errorf("unexpected typed prompt content:\nGot: %q\nWant: %q", got, want)
		}
	})
}
