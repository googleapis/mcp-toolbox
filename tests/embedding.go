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

// Package tests contains end to end tests meant to verify the Toolbox Server
// works as expected when executed as a binary.

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/jackc/pgx/v5/pgxpool"
)

var apiKey = os.Getenv("API_KEY")

// AddSemanticSearchConfig adds embedding models and semantic search tools to the config
// with configurable tool kind and SQL statements.
func AddSemanticSearchConfig(t *testing.T, config map[string]any, toolKind, insertStmt, searchStmt string) map[string]any {
	config["embeddingModels"] = map[string]any{
		"gemini_model": map[string]any{
			"kind":      "gemini",
			"model":     "gemini-embedding-001",
			"apiKey":    apiKey,
			"dimension": 768,
		},
	}

	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}

	queryKey := "statement"
	if toolKind == "elasticsearch-esql" {
		queryKey = "query"
	}

	tools["insert_docs"] = map[string]any{
		"kind":        toolKind,
		"source":      "my-instance",
		"description": "Stores content and its vector embedding into the documents table.",
		queryKey:      insertStmt,
		"parameters": []any{
			map[string]any{
				"name":        "content",
				"type":        "string",
				"description": "The text content associated with the vector.",
			},
			map[string]any{
				"name":           "text_to_embed",
				"type":           "string",
				"description":    "The text content used to generate the vector.",
				"embeddedBy":     "gemini_model",
				"valueFromParam": "content",
			},
		},
	}

	tools["search_docs"] = map[string]any{
		"kind":        toolKind,
		"source":      "my-instance",
		"description": "Finds the most semantically similar document to the query vector.",
		queryKey:      searchStmt,
		"parameters": []any{
			map[string]any{
				"name":        "query",
				"type":        "string",
				"description": "The text content to search for.",
				"embeddedBy":  "gemini_model",
			},
		},
	}

	config["tools"] = tools
	return config
}

// RunSemanticSearchToolInvokeTest runs the insert_docs and search_docs tools
// via both HTTP and MCP endpoints and verifies the output.
// Pass WithMCPExec() to run only the MCP test cases.
func RunSemanticSearchToolInvokeTest(t *testing.T, insertWant, mcpInsertWant, searchWant string, opts ...ToolExecOption) {
	config := &ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	// Initialize MCP session once for the MCP test cases
	sessionId := RunInitialize(t, "2024-11-05")

	tcs := []struct {
		name        string
		api         string
		isMcp       bool
		requestBody interface{}
		want        string
	}{
		{
			name:        "HTTP invoke insert_docs",
			api:         "http://127.0.0.1:5000/api/tool/insert_docs/invoke",
			isMcp:       false,
			requestBody: `{"content": "The quick brown fox jumps over the lazy dog"}`,
			want:        insertWant,
		},
		{
			name:        "HTTP invoke search_docs",
			api:         "http://127.0.0.1:5000/api/tool/search_docs/invoke",
			isMcp:       false,
			requestBody: `{"query": "fast fox jumping"}`,
			want:        searchWant,
		},
		{
			name:  "MCP invoke insert_docs",
			api:   "http://127.0.0.1:5000/mcp",
			isMcp: true,
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "mcp-insert-docs",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name": "insert_docs",
					"arguments": map[string]any{
						"content": "The quick brown fox jumps over the lazy dog",
					},
				},
			},
			want: mcpInsertWant,
		},
		{
			name:  "MCP invoke search_docs",
			api:   "http://127.0.0.1:5000/mcp",
			isMcp: true,
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "mcp-search-docs",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name": "search_docs",
					"arguments": map[string]any{
						"query": "fast fox jumping",
					},
				},
			},
			want: searchWant,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if config.isMCP && !tc.isMcp {
				t.Skip("legacy /api test case is skipped when running over MCP")
			}
			var bodyReader io.Reader
			headers := map[string]string{}

			// Prepare Request Body and Headers
			if tc.isMcp {
				reqBytes, err := json.Marshal(tc.requestBody)
				if err != nil {
					t.Fatalf("failed to marshal mcp request: %v", err)
				}
				bodyReader = bytes.NewBuffer(reqBytes)
				if sessionId != "" {
					headers["Mcp-Session-Id"] = sessionId
				}
			} else {
				bodyReader = bytes.NewBufferString(tc.requestBody.(string))
			}

			// Send Request
			resp, respBody := RunRequest(t, http.MethodPost, tc.api, bodyReader, headers)

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
			}

			// Normalize Response to get the actual tool result string
			var got string
			if tc.isMcp {
				var mcpResp struct {
					Result struct {
						Content []struct {
							Text string `json:"text"`
						} `json:"content"`
					} `json:"result"`
				}
				if err := json.Unmarshal(respBody, &mcpResp); err != nil {
					t.Fatalf("error parsing mcp response: %s", err)
				}
				if len(mcpResp.Result.Content) > 0 {
					got = mcpResp.Result.Content[0].Text
				}
			} else {
				var httpResp map[string]interface{}
				if err := json.Unmarshal(respBody, &httpResp); err != nil {
					t.Fatalf("error parsing http response: %s", err)
				}
				if res, ok := httpResp["result"].(string); ok {
					got = res
				}
			}

			if !strings.Contains(got, tc.want) {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}

// SetupPostgresVectorTable sets up the vector extension and a vector table
func SetupPostgresVectorTable(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (string, func(*testing.T)) {
	t.Helper()
	if _, err := pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector"); err != nil {
		t.Fatalf("failed to create vector extension: %v", err)
	}

	tableName := "vector_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	createTableStmt := fmt.Sprintf(`CREATE TABLE %s (
		id SERIAL PRIMARY KEY, 
		content TEXT, 
		embedding vector(768)
	)`, tableName)

	if _, err := pool.Exec(ctx, createTableStmt); err != nil {
		t.Fatalf("failed to create table %s: %v", tableName, err)
	}

	return tableName, func(t *testing.T) {
		if _, err := pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)); err != nil {
			t.Errorf("failed to drop table %s: %v", tableName, err)
		}
	}
}

func GetPostgresVectorSearchStmts(vectorTableName string) (string, string) {
	insertStmt := fmt.Sprintf("INSERT INTO %s (content, embedding) VALUES ($1, $2)", vectorTableName)
	searchStmt := fmt.Sprintf("SELECT id, content, embedding <-> $1 AS distance FROM %s ORDER BY distance LIMIT 1", vectorTableName)
	return insertStmt, searchStmt
}
