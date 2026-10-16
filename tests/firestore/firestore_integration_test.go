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

package firestore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	firestoreapi "cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"google.golang.org/api/option"
)

var (
	FirestoreSourceType = "firestore"
	FirestoreProject    = os.Getenv("FIRESTORE_PROJECT")
	FirestoreDatabase   = os.Getenv("FIRESTORE_DATABASE") // Optional, defaults to "(default)"
)

// firestoreCleanupTimeout bounds the teardown, which runs on a context that is
// detached from the (possibly cancelled) test context.
const firestoreCleanupTimeout = 2 * time.Minute

func getFirestoreVars(t *testing.T) map[string]any {
	if FirestoreProject == "" {
		t.Fatal("'FIRESTORE_PROJECT' not set")
	}

	vars := map[string]any{
		"type":    FirestoreSourceType,
		"project": FirestoreProject,
	}

	// Only add database if it's explicitly set
	if FirestoreDatabase != "" {
		vars["database"] = FirestoreDatabase
	}

	return vars
}

// initFirestoreConnection creates a Firestore client for testing
func initFirestoreConnection(ctx context.Context, project, database string) (*firestoreapi.Client, error) {
	if database == "" {
		database = "(default)"
	}

	client, err := firestoreapi.NewClientWithDatabase(ctx, project, database, option.WithUserAgent("genai-toolbox-integration-test"))
	if err != nil {
		return nil, fmt.Errorf("failed to create Firestore client for project %q and database %q: %w", project, database, err)
	}
	return client, nil
}

// firestoreTestData holds the names of the collections and documents seeded
// for a test run.
type firestoreTestData struct {
	collection    string
	subCollection string
	docID1        string
	docID2        string
	docID3        string
	docPath1      string
	docPath2      string
	docPath3      string
}

// setupFirestoreTest seeds the test data and returns the tools file along with
// the seeded names. The client is closed and the data deleted on cleanup.
func setupFirestoreTest(t *testing.T, ctx context.Context) (map[string]any, firestoreTestData) {
	t.Helper()
	sourceConfig := getFirestoreVars(t)

	client, err := initFirestoreConnection(ctx, FirestoreProject, FirestoreDatabase)
	if err != nil {
		t.Fatalf("unable to create Firestore connection: %s", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("failed to close Firestore client: %v", err)
		}
	})

	// Create test collection and document names with UUID
	newName := func(prefix string) string {
		return prefix + strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	d := firestoreTestData{
		collection:    newName("test_collection_"),
		subCollection: newName("test_subcollection_"),
		docID1:        newName("doc_"),
		docID2:        newName("doc_"),
		docID3:        newName("doc_"),
	}
	d.docPath1 = fmt.Sprintf("%s/%s", d.collection, d.docID1)
	d.docPath2 = fmt.Sprintf("%s/%s", d.collection, d.docID2)
	d.docPath3 = fmt.Sprintf("%s/%s", d.collection, d.docID3)

	setupFirestoreTestData(t, ctx, client, d)

	return getFirestoreToolsConfig(sourceConfig), d
}

type firestoreTransport struct {
	isMCP bool
}

// firestoreResult is the outcome of a tool call. For REST, a non-200 response
// carries the response body as result. toolErr is set when the call returned
// an error result: an `{"error": ...}` result over REST, or a JSON-RPC error
// or isError result over MCP.
type firestoreResult struct {
	status  int
	result  string
	toolErr bool
}

func (tr firestoreTransport) startServer(t *testing.T, ctx context.Context, toolsFile map[string]any) {
	t.Helper()
	var args []string
	if !tr.isMCP {
		args = append(args, "--enable-api")
	}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

// invoke calls a tool over the transport. Over MCP, results split into one
// content block per element are joined back into a JSON array so that they
// match the REST result.
func (tr firestoreTransport) invoke(t *testing.T, ctx context.Context, toolName string, args map[string]any) (firestoreResult, error) {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", toolName)
	var reqBody any = args
	headers := map[string]string{"Content-Type": "application/json"}
	if tr.isMCP {
		url = "http://127.0.0.1:5000/mcp"
		reqBody = tests.NewMCPCallToolRequest(uuid.New().String(), toolName, args)
		headers = tests.NewMCPRequestHeader(t, nil)
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("unable to marshal request body: %s", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		t.Fatalf("unable to create request: %s", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return firestoreResult{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return firestoreResult{}, err
	}

	if !tr.isMCP {
		if resp.StatusCode != http.StatusOK {
			return firestoreResult{status: resp.StatusCode, result: string(respBody)}, nil
		}
		var body struct {
			Result string `json:"result"`
		}
		if err := json.Unmarshal(respBody, &body); err != nil {
			t.Fatalf("error parsing response body %q: %s", respBody, err)
		}
		var errBody map[string]any
		toolErr := json.Unmarshal([]byte(body.Result), &errBody) == nil && errBody["error"] != nil
		return firestoreResult{status: resp.StatusCode, result: body.Result, toolErr: toolErr}, nil
	}

	var mcpResp tests.MCPCallToolResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		t.Fatalf("error parsing MCP response %q: %s", respBody, err)
	}
	if mcpResp.Error != nil {
		return firestoreResult{status: resp.StatusCode, result: mcpResp.Error.Message, toolErr: true}, nil
	}
	texts := make([]string, 0, len(mcpResp.Result.Content))
	for _, content := range mcpResp.Result.Content {
		texts = append(texts, content.Text)
	}
	var result string
	switch {
	case mcpResp.Result.IsError || len(texts) == 1:
		result = strings.Join(texts, "")
	default:
		result = "[" + strings.Join(texts, ",") + "]"
	}
	return firestoreResult{status: resp.StatusCode, result: result, toolErr: mcpResp.Result.IsError}, nil
}

func (tr firestoreTransport) mustInvoke(t *testing.T, ctx context.Context, toolName string, args map[string]any) firestoreResult {
	t.Helper()
	res, err := tr.invoke(t, ctx, toolName, args)
	if err != nil {
		t.Fatalf("unable to send request: %s", err)
	}
	return res
}

// invokeCase invokes a tool for a test case. It returns the result and
// whether the case should go on to check it. A failed call ends the case when
// isErr is set, and fails the test otherwise.
func (tr firestoreTransport) invokeCase(t *testing.T, ctx context.Context, toolName string, args map[string]any, isErr bool) (string, bool) {
	t.Helper()
	res := tr.mustInvoke(t, ctx, toolName, args)
	if res.status != http.StatusOK || res.toolErr {
		if isErr {
			return "", false
		}
		t.Fatalf("invoking %s failed (status %d): %s", toolName, res.status, res.result)
	}
	return res.result, true
}

func checkRegex(t *testing.T, got, wantRegex string) {
	t.Helper()
	if wantRegex == "" {
		return
	}
	matched, err := regexp.MatchString(wantRegex, got)
	if err != nil {
		t.Fatalf("invalid regex pattern: %v", err)
	}
	if !matched {
		t.Fatalf("result does not match expected pattern.\nGot: %s\nWant pattern: %s", got, wantRegex)
	}
}

func TestFirestoreToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	toolsFile, d := setupFirestoreTest(t, ctx)

	tr := firestoreTransport{}
	tr.startServer(t, ctx, toolsFile)

	// Run Firestore-specific tool get test
	runFirestoreToolGetTest(t)

	runFirestoreTests(t, ctx, tr, d)
}

// runFirestoreTests runs the Firestore tool invocation checks.
func runFirestoreTests(t *testing.T, ctx context.Context, tr firestoreTransport, d firestoreTestData) {
	runFirestoreGetDocumentsTest(t, ctx, tr, d.docPath1, d.docPath2)
	runFirestoreQueryCollectionTest(t, ctx, tr, d.collection)
	runFirestoreQueryTest(t, ctx, tr, d.collection)
	runFirestoreQuerySelectArrayTest(t, ctx, tr, d.collection)
	runFirestoreListCollectionsTest(t, ctx, tr, d.collection, d.subCollection, d.docPath1)
	runFirestoreAddDocumentsTest(t, ctx, tr, d.collection)
	runFirestoreUpdateDocumentTest(t, ctx, tr, d.collection, d.docID1)
	runFirestoreDeleteDocumentsTest(t, ctx, tr, d.docPath3)
	runFirestoreGetRulesTest(t, ctx, tr)
	runFirestoreValidateRulesTest(t, ctx, tr)
}

func runFirestoreToolGetTest(t *testing.T) {
	// Test tool get endpoint for Firestore tools
	tcs := []struct {
		name string
		api  string
		want map[string]any
	}{
		{
			name: "get my-simple-tool",
			api:  "http://127.0.0.1:5000/api/tool/my-simple-tool/",
			want: map[string]any{
				"my-simple-tool": map[string]any{
					"description": "Simple tool to test end to end functionality.",
					"parameters": []any{
						map[string]any{
							"name":        "documentPaths",
							"type":        "array",
							"required":    true,
							"description": "Array of document paths to retrieve from Firestore.",
							"items": map[string]any{
								"name":         "item",
								"type":         "string",
								"required":     true,
								"description":  "Document path",
								"authServices": []any{},
							},
							"authServices": []any{},
						},
					},
					"authRequired": []any{},
				},
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(tc.api)
			if err != nil {
				t.Fatalf("error when sending a request: %s", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("response status code is not 200")
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["tools"]
			if !ok {
				t.Fatalf("unable to find tools in response body")
			}

			// Compare as JSON strings to handle any ordering differences
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tc.want)
			if string(gotJSON) != string(wantJSON) {
				t.Logf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func runFirestoreValidateRulesTest(t *testing.T, ctx context.Context, tr firestoreTransport) {
	toolName := "firestore-validate-rules"
	invokeTcs := []struct {
		name      string
		args      map[string]any
		wantRegex string
		isErr     bool
	}{
		{
			name: "validate valid rules",
			args: map[string]any{
				"source": "rules_version = '2';\nservice cloud.firestore {\n  match /databases/{database}/documents {\n    match /{document=**} {\n      allow read, write: if true;\n    }\n  }\n}",
			},
			wantRegex: `"valid":true.*"issueCount":0`,
			isErr:     false,
		},
		{
			name: "validate rules with syntax error",
			args: map[string]any{
				"source": "rules_version = '2';\nservice cloud.firestore {\n  match /databases/{database}/documents {\n    match /{document=**} {\n      allow read, write: if true;;\n    }\n  }\n}",
			},
			wantRegex: `"valid":false.*"issueCount":[1-9]`,
			isErr:     false,
		},
		{
			name: "validate rules with missing version",
			args: map[string]any{
				"source": "service cloud.firestore {\n  match /databases/{database}/documents {\n    match /{document=**} {\n      allow read, write: if true;\n    }\n  }\n}",
			},
			wantRegex: `"valid":false.*"issueCount":[1-9]`,
			isErr:     false,
		},
		{
			name:  "validate empty rules",
			args:  map[string]any{"source": ""},
			isErr: true,
		},
		{
			name:  "missing source parameter",
			args:  map[string]any{},
			isErr: true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tr.invokeCase(t, ctx, toolName, tc.args, tc.isErr)
			if !ok {
				return
			}
			checkRegex(t, got, tc.wantRegex)
		})
	}
}

func runFirestoreGetRulesTest(t *testing.T, ctx context.Context, tr firestoreTransport) {
	t.Run("get firestore rules", func(t *testing.T) {
		res := tr.mustInvoke(t, ctx, "firestore-get-rules", map[string]any{})
		if res.status != http.StatusOK || res.toolErr {
			// The test might fail if there are no active rules in the project, which is acceptable
			if strings.Contains(res.result, "no active Firestore rules") {
				t.Skipf("No active Firestore rules found in the project")
			}
			t.Fatalf("invoking firestore-get-rules failed (status %d): %s", res.status, res.result)
		}
		checkRegex(t, res.result, `"content":"[^"]+"`)
	})
}

func runFirestoreMCPToolCallMethod(t *testing.T, docPath1, docPath2 string) {
	sessionId := tests.RunInitialize(t, "2024-11-05")
	header := map[string]string{}
	if sessionId != "" {
		header["Mcp-Session-Id"] = sessionId
	}

	// Test tool invoke endpoint
	invokeTcs := []struct {
		name          string
		api           string
		requestBody   jsonrpc.JSONRPCRequest
		requestHeader map[string]string
		wantContains  string
		wantError     bool
	}{
		{
			name:          "MCP Invoke my-param-tool",
			api:           "http://127.0.0.1:5000/mcp",
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "my-param-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name": "my-param-tool",
					"arguments": map[string]any{
						"documentPaths": []string{docPath1},
					},
				},
			},
			wantContains: `\"name\":\"Alice\"`,
			wantError:    false,
		},
		{
			name:          "MCP Invoke invalid tool",
			api:           "http://127.0.0.1:5000/mcp",
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invalid-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "foo",
					"arguments": map[string]any{},
				},
			},
			wantContains: `tool with name \"foo\" does not exist`,
			wantError:    true,
		},
		{
			name:          "MCP Invoke my-param-tool without parameters",
			api:           "http://127.0.0.1:5000/mcp",
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke-without-parameter",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-param-tool",
					"arguments": map[string]any{},
				},
			},
			wantContains: `parameter \"documentPaths\" is required`,
			wantError:    true,
		},
		{
			name:          "MCP Invoke my-auth-required-tool",
			api:           "http://127.0.0.1:5000/mcp",
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke my-auth-required-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-auth-required-tool",
					"arguments": map[string]any{},
				},
			},
			wantContains: `tool with name \"my-auth-required-tool\" does not exist`,
			wantError:    true,
		},
		{
			name:          "MCP Invoke my-fail-tool",
			api:           "http://127.0.0.1:5000/mcp",
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke-fail-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name": "my-fail-tool",
					"arguments": map[string]any{
						"documentPaths": []string{"non-existent/path"},
					},
				},
			},
			wantContains: `\"exists\":false`,
			wantError:    false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			reqMarshal, err := json.Marshal(tc.requestBody)
			if err != nil {
				t.Fatalf("unexpected error during marshaling of request body")
			}

			req, err := http.NewRequest(http.MethodPost, tc.api, bytes.NewBuffer(reqMarshal))
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")
			for k, v := range header {
				req.Header.Add(k, v)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("unable to read request body: %s", err)
			}

			got := string(bytes.TrimSpace(respBody))

			if !strings.Contains(got, tc.wantContains) {
				t.Fatalf("Expected substring not found:\ngot:  %q\nwant: %q (to be contained within got)", got, tc.wantContains)
			}
		})
	}
}

func getFirestoreToolsConfig(sourceConfig map[string]any) map[string]any {
	sources := map[string]any{
		"my-instance": sourceConfig,
	}

	tools := map[string]any{
		// Tool for RunToolGetTest
		"my-simple-tool": map[string]any{
			"type":        "firestore-get-documents",
			"source":      "my-instance",
			"description": "Simple tool to test end to end functionality.",
		},
		// Tool for MCP test - this will get documents
		"my-param-tool": map[string]any{
			"type":        "firestore-get-documents",
			"source":      "my-instance",
			"description": "Tool to get documents by paths",
		},
		// Tool for MCP test that fails
		"my-fail-tool": map[string]any{
			"type":        "firestore-get-documents",
			"source":      "my-instance",
			"description": "Tool that will fail",
		},
		// Firestore specific tools
		"firestore-get-docs": map[string]any{
			"type":        "firestore-get-documents",
			"source":      "my-instance",
			"description": "Get multiple documents from Firestore",
		},
		"firestore-list-colls": map[string]any{
			"type":        "firestore-list-collections",
			"source":      "my-instance",
			"description": "List Firestore collections",
		},
		"firestore-delete-docs": map[string]any{
			"type":        "firestore-delete-documents",
			"source":      "my-instance",
			"description": "Delete documents from Firestore",
		},
		"firestore-query-coll": map[string]any{
			"type":        "firestore-query-collection",
			"source":      "my-instance",
			"description": "Query a Firestore collection",
		},
		"firestore-query-param": map[string]any{
			"type":           "firestore-query",
			"source":         "my-instance",
			"description":    "Query a Firestore collection with parameterizable filters",
			"collectionPath": "{{.collection}}",
			"filters": `{
					"field": "age", "op": "{{.operator}}", "value": {"integerValue": "{{.ageValue}}"}
			}`,
			"limit": 10,
			"parameters": []map[string]any{
				{
					"name":        "collection",
					"type":        "string",
					"description": "Collection to query",
					"required":    true,
				},
				{
					"name":        "operator",
					"type":        "string",
					"description": "Comparison operator",
					"required":    true,
				},
				{
					"name":        "ageValue",
					"type":        "string",
					"description": "Age value to compare",
					"required":    true,
				},
			},
		},
		"firestore-query-select-array": map[string]any{
			"type":           "firestore-query",
			"source":         "my-instance",
			"description":    "Query with array-based select fields",
			"collectionPath": "{{.collection}}",
			"select":         []string{"{{.fields}}"},
			"limit":          10,
			"parameters": []map[string]any{
				{
					"name":        "collection",
					"type":        "string",
					"description": "Collection to query",
					"required":    true,
				},
				{
					"name":        "fields",
					"type":        "array",
					"description": "Fields to select",
					"required":    true,
					"items": map[string]any{
						"name":        "field",
						"type":        "string",
						"description": "field",
					},
				},
			},
		},
		"firestore-get-rules": map[string]any{
			"type":        "firestore-get-rules",
			"source":      "my-instance",
			"description": "Get Firestore security rules",
		},
		"firestore-validate-rules": map[string]any{
			"type":        "firestore-validate-rules",
			"source":      "my-instance",
			"description": "Validate Firestore security rules",
		},
		"firestore-add-docs": map[string]any{
			"type":        "firestore-add-documents",
			"source":      "my-instance",
			"description": "Add documents to Firestore",
		},
		"firestore-update-doc": map[string]any{
			"type":        "firestore-update-document",
			"source":      "my-instance",
			"description": "Update a document in Firestore",
		},
	}

	return map[string]any{
		"sources": sources,
		"tools":   tools,
	}
}

func runFirestoreUpdateDocumentTest(t *testing.T, ctx context.Context, tr firestoreTransport, collectionName string, docID string) {
	toolName := "firestore-update-doc"
	docPath := fmt.Sprintf("%s/%s", collectionName, docID)

	invokeTcs := []struct {
		name            string
		args            map[string]any
		wantKeys        []string
		validateContent bool
		expectedContent map[string]interface{}
		isErr           bool
	}{
		{
			name: "update document with simple fields",
			args: map[string]any{
				"documentPath": docPath,
				"documentData": map[string]any{
					"name":   map[string]any{"stringValue": "Alice Updated"},
					"status": map[string]any{"stringValue": "active"},
				},
			},
			wantKeys: []string{"documentPath", "updateTime"},
			isErr:    false,
		},
		{
			name: "update document with selective fields using updateMask",
			args: map[string]any{
				"documentPath": docPath,
				"documentData": map[string]any{
					"age":   map[string]any{"integerValue": "31"},
					"email": map[string]any{"stringValue": "alice@example.com"},
				},
				"updateMask": []any{"age"},
			},
			wantKeys: []string{"documentPath", "updateTime"},
			isErr:    false,
		},
		{
			name: "update document with field deletion",
			args: map[string]any{
				"documentPath": docPath,
				"documentData": map[string]any{
					"name": map[string]any{"stringValue": "Alice Final"},
				},
				"updateMask": []any{"name", "status"},
			},
			wantKeys: []string{"documentPath", "updateTime"},
			isErr:    false,
		},
		{
			name: "update document with complex types",
			args: map[string]any{
				"documentPath": docPath,
				"documentData": map[string]any{
					"location": map[string]any{
						"geoPointValue": map[string]any{
							"latitude":  40.7128,
							"longitude": -74.0060,
						},
					},
					"tags": map[string]any{
						"arrayValue": map[string]any{
							"values": []any{
								map[string]any{"stringValue": "updated"},
								map[string]any{"stringValue": "test"},
							},
						},
					},
					"metadata": map[string]any{
						"mapValue": map[string]any{
							"fields": map[string]any{
								"lastModified": map[string]any{"timestampValue": "2025-01-15T10:00:00Z"},
								"version":      map[string]any{"integerValue": "2"},
							},
						},
					},
				},
			},
			wantKeys: []string{"documentPath", "updateTime"},
			isErr:    false,
		},
		{
			name: "update document with returnData",
			args: map[string]any{
				"documentPath": docPath,
				"documentData": map[string]any{
					"testField":  map[string]any{"stringValue": "test value"},
					"testNumber": map[string]any{"integerValue": "42"},
				},
				"returnData": true,
			},
			wantKeys:        []string{"documentPath", "updateTime", "documentData"},
			validateContent: true,
			expectedContent: map[string]interface{}{
				"testField":  "test value",
				"testNumber": float64(42), // JSON numbers are decoded as float64
			},
			isErr: false,
		},
		{
			name: "update nested fields with updateMask",
			args: map[string]any{
				"documentPath": docPath,
				"documentData": map[string]any{
					"profile": map[string]any{
						"mapValue": map[string]any{
							"fields": map[string]any{
								"bio":    map[string]any{"stringValue": "Updated bio"},
								"avatar": map[string]any{"stringValue": "avatar.jpg"},
							},
						},
					},
				},
				"updateMask": []any{"profile.bio", "profile.avatar"},
			},
			wantKeys: []string{"documentPath", "updateTime"},
			isErr:    false,
		},
		{
			name: "missing documentPath parameter",
			args: map[string]any{
				"documentData": map[string]any{"test": map[string]any{"stringValue": "value"}},
			},
			isErr: true,
		},
		{
			name:  "missing documentData parameter",
			args:  map[string]any{"documentPath": docPath},
			isErr: true,
		},
		{
			name: "update non-existent document",
			args: map[string]any{
				"documentPath": "non-existent-collection/non-existent-doc",
				"documentData": map[string]any{
					"field": map[string]any{"stringValue": "value"},
				},
			},
			wantKeys: []string{"documentPath", "updateTime"}, // Set with MergeAll creates if doesn't exist
			isErr:    false,
		},
		{
			name: "invalid field in updateMask",
			args: map[string]any{
				"documentPath": docPath,
				"documentData": map[string]any{
					"field1": map[string]any{"stringValue": "value1"},
				},
				"updateMask": []any{"field1", "nonExistentField"},
			},
			isErr: true, // Should fail because nonExistentField is not in documentData
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tr.invokeCase(t, ctx, toolName, tc.args, tc.isErr)
			if !ok {
				return
			}

			// Parse the result string as JSON
			var resultJSON map[string]interface{}
			if err := json.Unmarshal([]byte(got), &resultJSON); err != nil {
				t.Fatalf("error parsing result %q as JSON: %v", got, err)
			}

			// Check if all wanted keys exist
			for _, key := range tc.wantKeys {
				if _, exists := resultJSON[key]; !exists {
					t.Fatalf("expected key %q not found in result: %s", key, got)
				}
			}

			// Validate document data if required
			if tc.validateContent {
				docData, ok := resultJSON["documentData"].(map[string]interface{})
				if !ok {
					t.Fatalf("documentData is not a map: %v", resultJSON["documentData"])
				}

				// Check that expected fields are present with correct values
				for key, expectedValue := range tc.expectedContent {
					actualValue, exists := docData[key]
					if !exists {
						t.Fatalf("expected field %q not found in documentData", key)
					}
					if actualValue != expectedValue {
						t.Fatalf("field %q mismatch: expected %v, got %v", key, expectedValue, actualValue)
					}
				}
			}
		})
	}
}

func runFirestoreAddDocumentsTest(t *testing.T, ctx context.Context, tr firestoreTransport, collectionName string) {
	toolName := "firestore-add-docs"
	invokeTcs := []struct {
		name            string
		args            map[string]any
		wantKeys        []string
		validateDocData bool
		expectedDocData map[string]interface{}
		isErr           bool
	}{
		{
			name: "add document with simple types",
			args: map[string]any{
				"collectionPath": collectionName,
				"documentData": map[string]any{
					"name":   map[string]any{"stringValue": "Test User"},
					"age":    map[string]any{"integerValue": "42"},
					"score":  map[string]any{"doubleValue": 99.5},
					"active": map[string]any{"booleanValue": true},
					"notes":  map[string]any{"nullValue": nil},
				},
			},
			wantKeys: []string{"documentPath", "createTime"},
			isErr:    false,
		},
		{
			name: "add document with complex types",
			args: map[string]any{
				"collectionPath": collectionName,
				"documentData": map[string]any{
					"location": map[string]any{
						"geoPointValue": map[string]any{
							"latitude":  37.7749,
							"longitude": -122.4194,
						},
					},
					"timestamp": map[string]any{
						"timestampValue": "2025-01-07T10:00:00Z",
					},
					"tags": map[string]any{
						"arrayValue": map[string]any{
							"values": []any{
								map[string]any{"stringValue": "tag1"},
								map[string]any{"stringValue": "tag2"},
							},
						},
					},
					"metadata": map[string]any{
						"mapValue": map[string]any{
							"fields": map[string]any{
								"version": map[string]any{"integerValue": "1"},
								"type":    map[string]any{"stringValue": "test"},
							},
						},
					},
				},
			},
			wantKeys: []string{"documentPath", "createTime"},
			isErr:    false,
		},
		{
			name: "add document with returnData",
			args: map[string]any{
				"collectionPath": collectionName,
				"documentData": map[string]any{
					"name":  map[string]any{"stringValue": "Return Test"},
					"value": map[string]any{"integerValue": "123"},
				},
				"returnData": true,
			},
			wantKeys:        []string{"documentPath", "createTime", "documentData"},
			validateDocData: true,
			expectedDocData: map[string]interface{}{
				"name":  "Return Test",
				"value": float64(123), // JSON numbers are decoded as float64
			},
			isErr: false,
		},
		{
			name: "add document with nested maps and arrays",
			args: map[string]any{
				"collectionPath": collectionName,
				"documentData": map[string]any{
					"company": map[string]any{
						"mapValue": map[string]any{
							"fields": map[string]any{
								"name": map[string]any{"stringValue": "Tech Corp"},
								"employees": map[string]any{
									"arrayValue": map[string]any{
										"values": []any{
											map[string]any{
												"mapValue": map[string]any{
													"fields": map[string]any{
														"name": map[string]any{"stringValue": "John"},
														"role": map[string]any{"stringValue": "Developer"},
													},
												},
											},
											map[string]any{
												"mapValue": map[string]any{
													"fields": map[string]any{
														"name": map[string]any{"stringValue": "Jane"},
														"role": map[string]any{"stringValue": "Manager"},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantKeys: []string{"documentPath", "createTime"},
			isErr:    false,
		},
		{
			name: "missing collectionPath parameter",
			args: map[string]any{
				"documentData": map[string]any{"test": map[string]any{"stringValue": "value"}},
			},
			isErr: true,
		},
		{
			name:  "missing documentData parameter",
			args:  map[string]any{"collectionPath": collectionName},
			isErr: true,
		},
		{
			name:  "invalid documentData format",
			args:  map[string]any{"collectionPath": collectionName, "documentData": "not an object"},
			isErr: true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tr.invokeCase(t, ctx, toolName, tc.args, tc.isErr)
			if !ok {
				return
			}

			// Parse the result string as JSON
			var resultJSON map[string]interface{}
			if err := json.Unmarshal([]byte(got), &resultJSON); err != nil {
				t.Fatalf("error parsing result %q as JSON: %v", got, err)
			}

			// Check if all wanted keys exist
			for _, key := range tc.wantKeys {
				if _, exists := resultJSON[key]; !exists {
					t.Fatalf("expected key %q not found in result: %s", key, got)
				}
			}

			// Validate document data if required
			if tc.validateDocData {
				docData, ok := resultJSON["documentData"].(map[string]interface{})
				if !ok {
					t.Fatalf("documentData is not a map: %v", resultJSON["documentData"])
				}

				// Use reflect.DeepEqual to compare the document data
				if !reflect.DeepEqual(docData, tc.expectedDocData) {
					t.Fatalf("documentData mismatch:\nexpected: %v\nactual: %v", tc.expectedDocData, docData)
				}
			}
		})
	}
}

// setupFirestoreTestData seeds the test documents. The cleanup that deletes
// all collections and documents in the database is registered first, so a
// partially seeded database is still cleaned up.
func setupFirestoreTestData(t *testing.T, ctx context.Context, client *firestoreapi.Client, d firestoreTestData) {
	t.Helper()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), firestoreCleanupTimeout)
		defer cancel()

		// Helper function to recursively delete all documents in a collection
		var deleteCollection func(*firestoreapi.CollectionRef) error
		deleteCollection = func(collection *firestoreapi.CollectionRef) error {
			// Get all documents in the collection
			docs, err := collection.Documents(cleanupCtx).GetAll()
			if err != nil {
				return fmt.Errorf("failed to list documents in collection %s: %w", collection.Path, err)
			}

			// Delete each document and its subcollections
			for _, doc := range docs {
				// First, get all subcollections of this document
				subcollections, err := doc.Ref.Collections(cleanupCtx).GetAll()
				if err != nil {
					return fmt.Errorf("failed to list subcollections of document %s: %w", doc.Ref.Path, err)
				}

				// Recursively delete each subcollection
				for _, subcoll := range subcollections {
					if err := deleteCollection(subcoll); err != nil {
						return fmt.Errorf("failed to delete subcollection %s: %w", subcoll.Path, err)
					}
				}

				// Delete the document itself
				if _, err := doc.Ref.Delete(cleanupCtx); err != nil {
					return fmt.Errorf("failed to delete document %s: %w", doc.Ref.Path, err)
				}
			}

			return nil
		}

		// Get all root collections in the database
		rootCollections, err := client.Collections(cleanupCtx).GetAll()
		if err != nil {
			t.Errorf("Failed to list root collections: %v", err)
			return
		}

		// Delete each root collection and all its contents
		for _, collection := range rootCollections {
			if err := deleteCollection(collection); err != nil {
				t.Errorf("Failed to delete collection %s and its contents: %v", collection.ID, err)
			}
		}

		t.Logf("Successfully deleted all collections and documents in the database")
	})

	docs := []struct {
		ref  *firestoreapi.DocumentRef
		data map[string]interface{}
	}{
		{client.Collection(d.collection).Doc(d.docID1), map[string]interface{}{"name": "Alice", "age": 30}},
		{client.Collection(d.collection).Doc(d.docID2), map[string]interface{}{"name": "Bob", "age": 25}},
		{client.Collection(d.collection).Doc(d.docID3), map[string]interface{}{"name": "Charlie", "age": 35}},
		// A subcollection document
		{
			client.Collection(d.collection).Doc(d.docID1).Collection(d.subCollection).Doc("subdoc1"),
			map[string]interface{}{"type": "subcollection_doc", "value": "test"},
		},
	}

	// Write the documents in a single batch
	bw := client.BulkWriter(ctx)
	jobs := make([]*firestoreapi.BulkWriterJob, 0, len(docs))
	for _, doc := range docs {
		job, err := bw.Set(doc.ref, doc.data)
		if err != nil {
			bw.End()
			t.Fatalf("Failed to queue test document %s: %v", doc.ref.Path, err)
		}
		jobs = append(jobs, job)
	}
	bw.End()
	for i, job := range jobs {
		if _, err := job.Results(); err != nil {
			t.Fatalf("Failed to create test document %s: %v", docs[i].ref.Path, err)
		}
	}
}

func runFirestoreGetDocumentsTest(t *testing.T, ctx context.Context, tr firestoreTransport, docPath1, docPath2 string) {
	toolName := "firestore-get-docs"
	invokeTcs := []struct {
		name      string
		args      map[string]any
		wantRegex string
		isErr     bool
	}{
		{
			name:      "get single document",
			args:      map[string]any{"documentPaths": []any{docPath1}},
			wantRegex: `"name":"Alice"`,
			isErr:     false,
		},
		{
			name:      "get multiple documents",
			args:      map[string]any{"documentPaths": []any{docPath1, docPath2}},
			wantRegex: `"name":"Alice".*"name":"Bob"`,
			isErr:     false,
		},
		{
			name:      "get non-existent document",
			args:      map[string]any{"documentPaths": []any{"non-existent-collection/non-existent-doc"}},
			wantRegex: `"exists":false`,
			isErr:     false,
		},
		{
			name:  "missing documentPaths parameter",
			args:  map[string]any{},
			isErr: true,
		},
		{
			name:  "empty documentPaths array",
			args:  map[string]any{"documentPaths": []any{}},
			isErr: true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tr.invokeCase(t, ctx, toolName, tc.args, tc.isErr)
			if !ok {
				return
			}
			checkRegex(t, got, tc.wantRegex)
		})
	}
}

func runFirestoreListCollectionsTest(t *testing.T, ctx context.Context, tr firestoreTransport, collectionName, subCollectionName, parentDocPath string) {
	toolName := "firestore-list-colls"
	invokeTcs := []struct {
		name  string
		args  map[string]any
		want  string
		isErr bool
	}{
		{
			name:  "list root collections",
			args:  map[string]any{},
			want:  collectionName,
			isErr: false,
		},
		{
			name:  "list subcollections",
			args:  map[string]any{"parentPath": parentDocPath},
			want:  subCollectionName,
			isErr: false,
		},
		{
			name:  "list collections for non-existent parent",
			args:  map[string]any{"parentPath": "non-existent-collection/non-existent-doc"},
			want:  `[]`, // Empty array for no collections
			isErr: false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tr.invokeCase(t, ctx, toolName, tc.args, tc.isErr)
			if !ok {
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected %q to contain %q, but it did not", got, tc.want)
			}
		})
	}
}

func runFirestoreDeleteDocumentsTest(t *testing.T, ctx context.Context, tr firestoreTransport, docPath string) {
	toolName := "firestore-delete-docs"
	invokeTcs := []struct {
		name  string
		args  map[string]any
		want  string
		isErr bool
	}{
		{
			name:  "delete single document",
			args:  map[string]any{"documentPaths": []any{docPath}},
			want:  `"success":true`,
			isErr: false,
		},
		{
			name:  "delete non-existent document",
			args:  map[string]any{"documentPaths": []any{"non-existent-collection/non-existent-doc"}},
			want:  `"success":true`, // Firestore delete succeeds even if doc doesn't exist
			isErr: false,
		},
		{
			name:  "missing documentPaths parameter",
			args:  map[string]any{},
			isErr: true,
		},
		{
			name:  "empty documentPaths array",
			args:  map[string]any{"documentPaths": []any{}},
			isErr: true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tr.invokeCase(t, ctx, toolName, tc.args, tc.isErr)
			if !ok {
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected %q to contain %q, but it did not", got, tc.want)
			}
		})
	}
}

func runFirestoreQueryTest(t *testing.T, ctx context.Context, tr firestoreTransport, collectionName string) {
	toolName := "firestore-query-param"
	invokeTcs := []struct {
		name      string
		args      map[string]any
		wantRegex string
		isErr     bool
	}{
		{
			name: "query with parameterized filters - age greater than",
			args: map[string]any{
				"collection": collectionName,
				"operator":   ">",
				"ageValue":   "25",
			},
			wantRegex: `"name":"Alice"`,
			isErr:     false,
		},
		{
			name: "query with parameterized filters - exact name match",
			args: map[string]any{
				"collection": collectionName,
				"operator":   "==",
				"ageValue":   "25",
			},
			wantRegex: `"name":"Bob"`,
			isErr:     false,
		},
		{
			name: "query with parameterized filters - age less than or equal",
			args: map[string]any{
				"collection": collectionName,
				"operator":   "<=",
				"ageValue":   "29",
			},
			wantRegex: `"name":"Bob"`,
			isErr:     false,
		},
		{
			name:  "missing required parameter",
			args:  map[string]any{"collection": "test", "operator": ">"},
			isErr: true,
		},
		{
			name: "query non-existent collection with parameters",
			args: map[string]any{
				"collection": "non-existent-collection",
				"operator":   "==",
				"ageValue":   "30",
			},
			wantRegex: `^\[\]$`, // Empty array
			isErr:     false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tr.invokeCase(t, ctx, toolName, tc.args, tc.isErr)
			if !ok {
				return
			}
			checkRegex(t, got, tc.wantRegex)
		})
	}
}

func runFirestoreQuerySelectArrayTest(t *testing.T, ctx context.Context, tr firestoreTransport, collectionName string) {
	toolName := "firestore-query-select-array"
	invokeTcs := []struct {
		name           string
		args           map[string]any
		wantRegex      string
		validateFields bool
		isErr          bool
	}{
		{
			name: "query with array select fields - single field",
			args: map[string]any{
				"collection": collectionName,
				"fields":     []any{"name"},
			},
			wantRegex:      `"name":"`,
			validateFields: true,
			isErr:          false,
		},
		{
			name: "query with array select fields - multiple fields",
			args: map[string]any{
				"collection": collectionName,
				"fields":     []any{"name", "age"},
			},
			wantRegex:      `"name":".*"age":`,
			validateFields: true,
			isErr:          false,
		},
		{
			name: "query with empty array select fields",
			args: map[string]any{
				"collection": collectionName,
				"fields":     []any{},
			},
			wantRegex: `\[.*\]`, // Should return documents with all fields
			isErr:     false,
		},
		{
			name:  "missing fields parameter",
			args:  map[string]any{"collection": collectionName},
			isErr: true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tr.invokeCase(t, ctx, toolName, tc.args, tc.isErr)
			if !ok {
				return
			}
			checkRegex(t, got, tc.wantRegex)

			// Additional validation for field selection
			if tc.validateFields {
				// Parse the result to check if only selected fields are present
				var results []map[string]interface{}
				if err := json.Unmarshal([]byte(got), &results); err != nil {
					t.Fatalf("error parsing result %q as JSON array: %v", got, err)
				}

				// For single field test, ensure only 'name' field is present in data
				if tc.name == "query with array select fields - single field" && len(results) > 0 {
					for _, result := range results {
						if data, ok := result["data"].(map[string]interface{}); ok {
							if _, hasName := data["name"]; !hasName {
								t.Fatalf("expected 'name' field in data, but not found")
							}
							// The 'age' field should not be present when only 'name' is selected
							if _, hasAge := data["age"]; hasAge {
								t.Fatalf("unexpected 'age' field in data when only 'name' was selected")
							}
						}
					}
				}

				// For multiple fields test, ensure both fields are present
				if tc.name == "query with array select fields - multiple fields" && len(results) > 0 {
					for _, result := range results {
						if data, ok := result["data"].(map[string]interface{}); ok {
							if _, hasName := data["name"]; !hasName {
								t.Fatalf("expected 'name' field in data, but not found")
							}
							if _, hasAge := data["age"]; !hasAge {
								t.Fatalf("expected 'age' field in data, but not found")
							}
						}
					}
				}
			}
		})
	}
}

func runFirestoreQueryCollectionTest(t *testing.T, ctx context.Context, tr firestoreTransport, collectionName string) {
	toolName := "firestore-query-coll"
	invokeTcs := []struct {
		name      string
		args      map[string]any
		wantRegex string
		isErr     bool
	}{
		{
			name: "query collection with filter",
			args: map[string]any{
				"collectionPath": collectionName,
				"filters":        []any{`{"field": "age", "op": ">", "value": 25}`},
				"orderBy":        "",
				"limit":          10,
			},
			wantRegex: `"name":"Alice"`,
			isErr:     false,
		},
		{
			name: "query collection with orderBy",
			args: map[string]any{
				"collectionPath": collectionName,
				"filters":        []any{},
				"orderBy":        `{"field": "age", "direction": "DESCENDING"}`,
				"limit":          2,
			},
			wantRegex: `"age":35.*"age":30`, // Should be ordered by age descending (Charlie=35, Alice=30)
			isErr:     false,
		},
		{
			name: "query collection with multiple filters",
			args: map[string]any{
				"collectionPath": collectionName,
				"filters": []any{
					`{"field": "age", "op": ">=", "value": 25}`,
					`{"field": "age", "op": "<=", "value": 30}`,
				},
				"orderBy": "",
				"limit":   10,
			},
			wantRegex: `"name":"Bob".*"name":"Alice"`, // Results may be ordered by document ID
			isErr:     false,
		},
		{
			name: "query with limit",
			args: map[string]any{
				"collectionPath": collectionName,
				"filters":        []any{},
				"orderBy":        "",
				"limit":          1,
			},
			wantRegex: `^\[{.*}\]$`, // Should return exactly one document
			isErr:     false,
		},
		{
			name: "query non-existent collection",
			args: map[string]any{
				"collectionPath": "non-existent-collection",
				"filters":        []any{},
				"orderBy":        "",
				"limit":          10,
			},
			wantRegex: `^\[\]$`, // Empty array
			isErr:     false,
		},
		{
			name:  "missing collectionPath parameter",
			args:  map[string]any{},
			isErr: true,
		},
		{
			name: "invalid filter operator",
			args: map[string]any{
				"collectionPath": collectionName,
				"filters":        []any{`{"field": "age", "op": "INVALID", "value": 25}`},
				"orderBy":        "",
			},
			isErr: true,
		},
		{
			name: "query with analyzeQuery",
			args: map[string]any{
				"collectionPath": collectionName,
				"filters":        []any{},
				"orderBy":        "",
				"analyzeQuery":   true,
				"limit":          1,
			},
			wantRegex: `"documents":\[.*\]`,
			isErr:     false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tr.invokeCase(t, ctx, toolName, tc.args, tc.isErr)
			if !ok {
				return
			}
			checkRegex(t, got, tc.wantRegex)
		})
	}
}
