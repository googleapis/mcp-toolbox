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
func initFirestoreConnection(project, database string) (*firestoreapi.Client, error) {
	ctx := context.Background()

	if database == "" {
		database = "(default)"
	}

	client, err := firestoreapi.NewClientWithDatabase(ctx, project, database, option.WithUserAgent("genai-toolbox-integration-test"))
	if err != nil {
		return nil, fmt.Errorf("failed to create Firestore client for project %q and database %q: %w", project, database, err)
	}
	return client, nil
}

func TestFirestoreToolEndpoints(t *testing.T) {
	sourceConfig := getFirestoreVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	client, err := initFirestoreConnection(FirestoreProject, FirestoreDatabase)
	if err != nil {
		t.Fatalf("unable to create Firestore connection: %s", err)
	}
	defer client.Close()

	// Create test collection and document names with UUID
	testCollectionName := fmt.Sprintf("test_collection_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	testSubCollectionName := fmt.Sprintf("test_subcollection_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	testDocID1 := fmt.Sprintf("doc_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	testDocID2 := fmt.Sprintf("doc_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	testDocID3 := fmt.Sprintf("doc_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))

	// Document paths for testing
	docPath1 := fmt.Sprintf("%s/%s", testCollectionName, testDocID1)
	docPath2 := fmt.Sprintf("%s/%s", testCollectionName, testDocID2)
	docPath3 := fmt.Sprintf("%s/%s", testCollectionName, testDocID3)

	// Set up test data
	teardown := setupFirestoreTestData(t, ctx, client, testCollectionName, testSubCollectionName,
		testDocID1, testDocID2, testDocID3)
	defer teardown(t)

	// Write config into a file and pass it to command
	toolsFile := getFirestoreToolsConfig(sourceConfig)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
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

	// Run Firestore-specific tool get test
	runFirestoreToolGetTest(t)

	// Run Firestore-specific MCP test
	runFirestoreMCPToolCallMethod(t, docPath1, docPath2)

	// Run specific Firestore tool tests
	runFirestoreGetDocumentsTest(t, docPath1, docPath2)
	runFirestoreQueryCollectionTest(t, testCollectionName)
	runFirestoreQueryTest(t, testCollectionName)
	runFirestoreQuerySelectArrayTest(t, testCollectionName)
	runFirestoreListCollectionsTest(t, testCollectionName, testSubCollectionName, docPath1)
	runFirestoreAddDocumentsTest(t, testCollectionName)
	runFirestoreUpdateDocumentTest(t, testCollectionName, testDocID1)
	runFirestoreDeleteDocumentsTest(t, docPath3)
	runFirestoreGetRulesTest(t)
	runFirestoreValidateRulesTest(t)
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

func runFirestoreValidateRulesTest(t *testing.T) {
	invokeTcs := []struct {
		name        string
		api         string
		requestBody io.Reader
		wantRegex   string
		isErr       bool
	}{
		{
			name: "validate valid rules",
			api:  "http://127.0.0.1:5000/api/tool/firestore-validate-rules/invoke",
			requestBody: bytes.NewBuffer([]byte(`{
				"source": "rules_version = '2';\nservice cloud.firestore {\n  match /databases/{database}/documents {\n    match /{document=**} {\n      allow read, write: if true;\n    }\n  }\n}"
			}`)),
			wantRegex: `"valid":true.*"issueCount":0`,
			isErr:     false,
		},
		{
			name: "validate rules with syntax error",
			api:  "http://127.0.0.1:5000/api/tool/firestore-validate-rules/invoke",
			requestBody: bytes.NewBuffer([]byte(`{
				"source": "rules_version = '2';\nservice cloud.firestore {\n  match /databases/{database}/documents {\n    match /{document=**} {\n      allow read, write: if true;;\n    }\n  }\n}"
			}`)),
			wantRegex: `"valid":false.*"issueCount":[1-9]`,
			isErr:     false,
		},
		{
			name: "validate rules with missing version",
			api:  "http://127.0.0.1:5000/api/tool/firestore-validate-rules/invoke",
			requestBody: bytes.NewBuffer([]byte(`{
				"source": "service cloud.firestore {\n  match /databases/{database}/documents {\n    match /{document=**} {\n      allow read, write: if true;\n    }\n  }\n}"
			}`)),
			wantRegex: `"valid":false.*"issueCount":[1-9]`,
			isErr:     false,
		},
		{
			name:        "validate empty rules",
			api:         "http://127.0.0.1:5000/api/tool/firestore-validate-rules/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"source": ""}`)),
			isErr:       true,
		},
		{
			name:        "missing source parameter",
			api:         "http://127.0.0.1:5000/api/tool/firestore-validate-rules/invoke",
			requestBody: bytes.NewBuffer([]byte(`{}`)),
			isErr:       true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if tc.wantRegex != "" {
				matched, err := regexp.MatchString(tc.wantRegex, got)
				if err != nil {
					t.Fatalf("invalid regex pattern: %v", err)
				}
				if !matched {
					t.Fatalf("result does not match expected pattern.\nGot: %s\nWant pattern: %s", got, tc.wantRegex)
				}
			}
		})
	}
}

func runFirestoreGetRulesTest(t *testing.T) {
	invokeTcs := []struct {
		name        string
		api         string
		requestBody io.Reader
		wantRegex   string
		isErr       bool
	}{
		{
			name:        "get firestore rules",
			api:         "http://127.0.0.1:5000/api/tool/firestore-get-rules/invoke",
			requestBody: bytes.NewBuffer([]byte(`{}`)),
			wantRegex:   `"content":"[^"]+"`, // Should contain at least one of these fields
			isErr:       false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				// The test might fail if there are no active rules in the project, which is acceptable
				if strings.Contains(string(bodyBytes), "no active Firestore rules") {
					t.Skipf("No active Firestore rules found in the project")
					return
				}
				if tc.isErr {
					return
				}
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if tc.wantRegex != "" {
				matched, err := regexp.MatchString(tc.wantRegex, got)
				if err != nil {
					t.Fatalf("invalid regex pattern: %v", err)
				}
				if !matched {
					t.Fatalf("result does not match expected pattern.\nGot: %s\nWant pattern: %s", got, tc.wantRegex)
				}
			}
		})
	}
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

func runFirestoreUpdateDocumentTest(t *testing.T, collectionName string, docID string) {
	docPath := fmt.Sprintf("%s/%s", collectionName, docID)

	invokeTcs := []struct {
		name            string
		api             string
		requestBody     io.Reader
		wantKeys        []string
		validateContent bool
		expectedContent map[string]interface{}
		isErr           bool
	}{
		{
			name: "update document with simple fields",
			api:  "http://127.0.0.1:5000/api/tool/firestore-update-doc/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"documentPath": "%s",
				"documentData": {
					"name": {"stringValue": "Alice Updated"},
					"status": {"stringValue": "active"}
				}
			}`, docPath))),
			wantKeys: []string{"documentPath", "updateTime"},
			isErr:    false,
		},
		{
			name: "update document with selective fields using updateMask",
			api:  "http://127.0.0.1:5000/api/tool/firestore-update-doc/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"documentPath": "%s",
				"documentData": {
					"age": {"integerValue": "31"},
					"email": {"stringValue": "alice@example.com"}
				},
				"updateMask": ["age"]
			}`, docPath))),
			wantKeys: []string{"documentPath", "updateTime"},
			isErr:    false,
		},
		{
			name: "update document with field deletion",
			api:  "http://127.0.0.1:5000/api/tool/firestore-update-doc/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"documentPath": "%s",
				"documentData": {
					"name": {"stringValue": "Alice Final"}
				},
				"updateMask": ["name", "status"]
			}`, docPath))),
			wantKeys: []string{"documentPath", "updateTime"},
			isErr:    false,
		},
		{
			name: "update document with complex types",
			api:  "http://127.0.0.1:5000/api/tool/firestore-update-doc/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"documentPath": "%s",
				"documentData": {
					"location": {
						"geoPointValue": {
							"latitude": 40.7128,
							"longitude": -74.0060
						}
					},
					"tags": {
						"arrayValue": {
							"values": [
								{"stringValue": "updated"},
								{"stringValue": "test"}
							]
						}
					},
					"metadata": {
						"mapValue": {
							"fields": {
								"lastModified": {"timestampValue": "2025-01-15T10:00:00Z"},
								"version": {"integerValue": "2"}
							}
						}
					}
				}
			}`, docPath))),
			wantKeys: []string{"documentPath", "updateTime"},
			isErr:    false,
		},
		{
			name: "update document with returnData",
			api:  "http://127.0.0.1:5000/api/tool/firestore-update-doc/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"documentPath": "%s",
				"documentData": {
					"testField": {"stringValue": "test value"},
					"testNumber": {"integerValue": "42"}
				},
				"returnData": true
			}`, docPath))),
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
			api:  "http://127.0.0.1:5000/api/tool/firestore-update-doc/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"documentPath": "%s",
				"documentData": {
					"profile": {
						"mapValue": {
							"fields": {
								"bio": {"stringValue": "Updated bio"},
								"avatar": {"stringValue": "avatar.jpg"}
							}
						}
					}
				},
				"updateMask": ["profile.bio", "profile.avatar"]
			}`, docPath))),
			wantKeys: []string{"documentPath", "updateTime"},
			isErr:    false,
		},
		{
			name:        "missing documentPath parameter",
			api:         "http://127.0.0.1:5000/api/tool/firestore-update-doc/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"documentData": {"test": {"stringValue": "value"}}}`)),
			isErr:       true,
		},
		{
			name:        "missing documentData parameter",
			api:         "http://127.0.0.1:5000/api/tool/firestore-update-doc/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{"documentPath": "%s"}`, docPath))),
			isErr:       true,
		},
		{
			name: "update non-existent document",
			api:  "http://127.0.0.1:5000/api/tool/firestore-update-doc/invoke",
			requestBody: bytes.NewBuffer([]byte(`{
				"documentPath": "non-existent-collection/non-existent-doc",
				"documentData": {
					"field": {"stringValue": "value"}
				}
			}`)),
			wantKeys: []string{"documentPath", "updateTime"}, // Set with MergeAll creates if doesn't exist
			isErr:    false,
		},
		{
			name: "invalid field in updateMask",
			api:  "http://127.0.0.1:5000/api/tool/firestore-update-doc/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"documentPath": "%s",
				"documentData": {
					"field1": {"stringValue": "value1"}
				},
				"updateMask": ["field1", "nonExistentField"]
			}`, docPath))),
			isErr: true, // Should fail because nonExistentField is not in documentData
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			// Parse the result string as JSON
			var resultJSON map[string]interface{}
			err = json.Unmarshal([]byte(got), &resultJSON)
			if err != nil {
				t.Fatalf("error parsing result as JSON: %v", err)
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

func runFirestoreAddDocumentsTest(t *testing.T, collectionName string) {
	invokeTcs := []struct {
		name            string
		api             string
		requestBody     io.Reader
		wantKeys        []string
		validateDocData bool
		expectedDocData map[string]interface{}
		isErr           bool
	}{
		{
			name: "add document with simple types",
			api:  "http://127.0.0.1:5000/api/tool/firestore-add-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collectionPath": "%s",
				"documentData": {
					"name": {"stringValue": "Test User"},
					"age": {"integerValue": "42"},
					"score": {"doubleValue": 99.5},
					"active": {"booleanValue": true},
					"notes": {"nullValue": null}
				}
			}`, collectionName))),
			wantKeys: []string{"documentPath", "createTime"},
			isErr:    false,
		},
		{
			name: "add document with complex types",
			api:  "http://127.0.0.1:5000/api/tool/firestore-add-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collectionPath": "%s",
				"documentData": {
					"location": {
						"geoPointValue": {
							"latitude": 37.7749,
							"longitude": -122.4194
						}
					},
					"timestamp": {
						"timestampValue": "2025-01-07T10:00:00Z"
					},
					"tags": {
						"arrayValue": {
							"values": [
								{"stringValue": "tag1"},
								{"stringValue": "tag2"}
							]
						}
					},
					"metadata": {
						"mapValue": {
							"fields": {
								"version": {"integerValue": "1"},
								"type": {"stringValue": "test"}
							}
						}
					}
				}
			}`, collectionName))),
			wantKeys: []string{"documentPath", "createTime"},
			isErr:    false,
		},
		{
			name: "add document with returnData",
			api:  "http://127.0.0.1:5000/api/tool/firestore-add-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collectionPath": "%s",
				"documentData": {
					"name": {"stringValue": "Return Test"},
					"value": {"integerValue": "123"}
				},
				"returnData": true
			}`, collectionName))),
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
			api:  "http://127.0.0.1:5000/api/tool/firestore-add-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collectionPath": "%s",
				"documentData": {
					"company": {
						"mapValue": {
							"fields": {
								"name": {"stringValue": "Tech Corp"},
								"employees": {
									"arrayValue": {
										"values": [
											{
												"mapValue": {
													"fields": {
														"name": {"stringValue": "John"},
														"role": {"stringValue": "Developer"}
													}
												}
											},
											{
												"mapValue": {
													"fields": {
														"name": {"stringValue": "Jane"},
														"role": {"stringValue": "Manager"}
													}
												}
											}
										]
									}
								}
							}
						}
					}
				}
			}`, collectionName))),
			wantKeys: []string{"documentPath", "createTime"},
			isErr:    false,
		},
		{
			name:        "missing collectionPath parameter",
			api:         "http://127.0.0.1:5000/api/tool/firestore-add-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"documentData": {"test": {"stringValue": "value"}}}`)),
			isErr:       true,
		},
		{
			name:        "missing documentData parameter",
			api:         "http://127.0.0.1:5000/api/tool/firestore-add-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{"collectionPath": "%s"}`, collectionName))),
			isErr:       true,
		},
		{
			name:        "invalid documentData format",
			api:         "http://127.0.0.1:5000/api/tool/firestore-add-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{"collectionPath": "%s", "documentData": "not an object"}`, collectionName))),
			isErr:       true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			// Parse the result string as JSON
			var resultJSON map[string]interface{}
			err = json.Unmarshal([]byte(got), &resultJSON)
			if err != nil {
				t.Fatalf("error parsing result as JSON: %v", err)
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

func setupFirestoreTestData(t *testing.T, ctx context.Context, client *firestoreapi.Client,
	collectionName, subCollectionName, docID1, docID2, docID3 string) func(*testing.T) {
	// Create test documents
	testData1 := map[string]interface{}{
		"name": "Alice",
		"age":  30,
	}
	testData2 := map[string]interface{}{
		"name": "Bob",
		"age":  25,
	}
	testData3 := map[string]interface{}{
		"name": "Charlie",
		"age":  35,
	}

	// Create documents
	_, err := client.Collection(collectionName).Doc(docID1).Set(ctx, testData1)
	if err != nil {
		t.Fatalf("Failed to create test document 1: %v", err)
	}

	_, err = client.Collection(collectionName).Doc(docID2).Set(ctx, testData2)
	if err != nil {
		t.Fatalf("Failed to create test document 2: %v", err)
	}

	_, err = client.Collection(collectionName).Doc(docID3).Set(ctx, testData3)
	if err != nil {
		t.Fatalf("Failed to create test document 3: %v", err)
	}

	// Create a subcollection document
	subDocData := map[string]interface{}{
		"type":  "subcollection_doc",
		"value": "test",
	}
	_, err = client.Collection(collectionName).Doc(docID1).Collection(subCollectionName).Doc("subdoc1").Set(ctx, subDocData)
	if err != nil {
		t.Fatalf("Failed to create subcollection document: %v", err)
	}

	// Return cleanup function that deletes ALL collections and documents in the database
	return func(t *testing.T) {
		// Helper function to recursively delete all documents in a collection
		var deleteCollection func(*firestoreapi.CollectionRef) error
		deleteCollection = func(collection *firestoreapi.CollectionRef) error {
			// Get all documents in the collection
			docs, err := collection.Documents(ctx).GetAll()
			if err != nil {
				return fmt.Errorf("failed to list documents in collection %s: %w", collection.Path, err)
			}

			// Delete each document and its subcollections
			for _, doc := range docs {
				// First, get all subcollections of this document
				subcollections, err := doc.Ref.Collections(ctx).GetAll()
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
				if _, err := doc.Ref.Delete(ctx); err != nil {
					return fmt.Errorf("failed to delete document %s: %w", doc.Ref.Path, err)
				}
			}

			return nil
		}

		// Get all root collections in the database
		rootCollections, err := client.Collections(ctx).GetAll()
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
	}
}

func runFirestoreGetDocumentsTest(t *testing.T, docPath1, docPath2 string) {
	invokeTcs := []struct {
		name        string
		api         string
		requestBody io.Reader
		wantRegex   string
		isErr       bool
	}{
		{
			name:        "get single document",
			api:         "http://127.0.0.1:5000/api/tool/firestore-get-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{"documentPaths": ["%s"]}`, docPath1))),
			wantRegex:   `"name":"Alice"`,
			isErr:       false,
		},
		{
			name:        "get multiple documents",
			api:         "http://127.0.0.1:5000/api/tool/firestore-get-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{"documentPaths": ["%s", "%s"]}`, docPath1, docPath2))),
			wantRegex:   `"name":"Alice".*"name":"Bob"`,
			isErr:       false,
		},
		{
			name:        "get non-existent document",
			api:         "http://127.0.0.1:5000/api/tool/firestore-get-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"documentPaths": ["non-existent-collection/non-existent-doc"]}`)),
			wantRegex:   `"exists":false`,
			isErr:       false,
		},
		{
			name:        "missing documentPaths parameter",
			api:         "http://127.0.0.1:5000/api/tool/firestore-get-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(`{}`)),
			isErr:       true,
		},
		{
			name:        "empty documentPaths array",
			api:         "http://127.0.0.1:5000/api/tool/firestore-get-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"documentPaths": []}`)),
			isErr:       true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if tc.wantRegex != "" {
				matched, err := regexp.MatchString(tc.wantRegex, got)
				if err != nil {
					t.Fatalf("invalid regex pattern: %v", err)
				}
				if !matched {
					t.Fatalf("result does not match expected pattern.\nGot: %s\nWant pattern: %s", got, tc.wantRegex)
				}
			}
		})
	}
}

func runFirestoreListCollectionsTest(t *testing.T, collectionName, subCollectionName, parentDocPath string) {
	invokeTcs := []struct {
		name        string
		api         string
		requestBody io.Reader
		want        string
		isErr       bool
	}{
		{
			name:        "list root collections",
			api:         "http://127.0.0.1:5000/api/tool/firestore-list-colls/invoke",
			requestBody: bytes.NewBuffer([]byte(`{}`)),
			want:        collectionName,
			isErr:       false,
		},
		{
			name:        "list subcollections",
			api:         "http://127.0.0.1:5000/api/tool/firestore-list-colls/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{"parentPath": "%s"}`, parentDocPath))),
			want:        subCollectionName,
			isErr:       false,
		},
		{
			name:        "list collections for non-existent parent",
			api:         "http://127.0.0.1:5000/api/tool/firestore-list-colls/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"parentPath": "non-existent-collection/non-existent-doc"}`)),
			want:        `[]`, // Empty array for no collections
			isErr:       false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected %q to contain %q, but it did not", got, tc.want)
			}
		})
	}
}

func runFirestoreDeleteDocumentsTest(t *testing.T, docPath string) {
	invokeTcs := []struct {
		name        string
		api         string
		requestBody io.Reader
		want        string
		isErr       bool
	}{
		{
			name:        "delete single document",
			api:         "http://127.0.0.1:5000/api/tool/firestore-delete-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{"documentPaths": ["%s"]}`, docPath))),
			want:        `"success":true`,
			isErr:       false,
		},
		{
			name:        "delete non-existent document",
			api:         "http://127.0.0.1:5000/api/tool/firestore-delete-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"documentPaths": ["non-existent-collection/non-existent-doc"]}`)),
			want:        `"success":true`, // Firestore delete succeeds even if doc doesn't exist
			isErr:       false,
		},
		{
			name:        "missing documentPaths parameter",
			api:         "http://127.0.0.1:5000/api/tool/firestore-delete-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(`{}`)),
			isErr:       true,
		},
		{
			name:        "empty documentPaths array",
			api:         "http://127.0.0.1:5000/api/tool/firestore-delete-docs/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"documentPaths": []}`)),
			isErr:       true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected %q to contain %q, but it did not", got, tc.want)
			}
		})
	}
}

func runFirestoreQueryTest(t *testing.T, collectionName string) {
	invokeTcs := []struct {
		name        string
		api         string
		requestBody io.Reader
		wantRegex   string
		isErr       bool
	}{
		{
			name: "query with parameterized filters - age greater than",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-param/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collection": "%s",
				"operator": ">",
				"ageValue": "25"
			}`, collectionName))),
			wantRegex: `"name":"Alice"`,
			isErr:     false,
		},
		{
			name: "query with parameterized filters - exact name match",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-param/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collection": "%s",
				"operator": "==",
				"ageValue": "25"
			}`, collectionName))),
			wantRegex: `"name":"Bob"`,
			isErr:     false,
		},
		{
			name: "query with parameterized filters - age less than or equal",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-param/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collection": "%s",
				"operator": "<=",
				"ageValue": "29"
			}`, collectionName))),
			wantRegex: `"name":"Bob"`,
			isErr:     false,
		},
		{
			name:        "missing required parameter",
			api:         "http://127.0.0.1:5000/api/tool/firestore-query-param/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"collection": "test", "operator": ">"}`)),
			isErr:       true,
		},
		{
			name: "query non-existent collection with parameters",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-param/invoke",
			requestBody: bytes.NewBuffer([]byte(`{
				"collection": "non-existent-collection",
				"operator": "==",
				"ageValue": "30"
			}`)),
			wantRegex: `^\[\]$`, // Empty array
			isErr:     false,
		},
		{
			// Invalid collection paths are agent errors, so they surface as a
			// 200 response whose result carries the validation message.
			name: "absolute collection path is rejected",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-param/invoke",
			requestBody: bytes.NewBuffer([]byte(`{
				"collection": "projects/my-project/databases/(default)/documents/users",
				"operator": "==",
				"ageValue": "30"
			}`)),
			wantRegex: `"error":"invalid collection path.*path must be relative`,
			isErr:     false,
		},
		{
			name: "document path is rejected as collection path",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-param/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collection": "%s/doc_1",
				"operator": "==",
				"ageValue": "30"
			}`, collectionName))),
			wantRegex: `"error":"invalid collection path.*must have an odd number of segments`,
			isErr:     false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if tc.wantRegex != "" {
				matched, err := regexp.MatchString(tc.wantRegex, got)
				if err != nil {
					t.Fatalf("invalid regex pattern: %v", err)
				}
				if !matched {
					t.Fatalf("result does not match expected pattern.\nGot: %s\nWant pattern: %s", got, tc.wantRegex)
				}
			}
		})
	}
}

func runFirestoreQuerySelectArrayTest(t *testing.T, collectionName string) {
	invokeTcs := []struct {
		name           string
		api            string
		requestBody    io.Reader
		wantRegex      string
		validateFields bool
		isErr          bool
	}{
		{
			name: "query with array select fields - single field",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-select-array/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collection": "%s",
				"fields": ["name"]
			}`, collectionName))),
			wantRegex:      `"name":"`,
			validateFields: true,
			isErr:          false,
		},
		{
			name: "query with array select fields - multiple fields",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-select-array/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collection": "%s",
				"fields": ["name", "age"]
			}`, collectionName))),
			wantRegex:      `"name":".*"age":`,
			validateFields: true,
			isErr:          false,
		},
		{
			name: "query with empty array select fields",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-select-array/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collection": "%s",
				"fields": []
			}`, collectionName))),
			wantRegex: `\[.*\]`, // Should return documents with all fields
			isErr:     false,
		},
		{
			name:        "missing fields parameter",
			api:         "http://127.0.0.1:5000/api/tool/firestore-query-select-array/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{"collection": "%s"}`, collectionName))),
			isErr:       true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if tc.wantRegex != "" {
				matched, err := regexp.MatchString(tc.wantRegex, got)
				if err != nil {
					t.Fatalf("invalid regex pattern: %v", err)
				}
				if !matched {
					t.Fatalf("result does not match expected pattern.\nGot: %s\nWant pattern: %s", got, tc.wantRegex)
				}
			}

			// Additional validation for field selection
			if tc.validateFields {
				// Parse the result to check if only selected fields are present
				var results []map[string]interface{}
				err = json.Unmarshal([]byte(got), &results)
				if err != nil {
					t.Fatalf("error parsing result as JSON array: %v", err)
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

func runFirestoreQueryCollectionTest(t *testing.T, collectionName string) {
	invokeTcs := []struct {
		name        string
		api         string
		requestBody io.Reader
		wantRegex   string
		isErr       bool
	}{
		{
			name: "query collection with filter",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-coll/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collectionPath": "%s",
				"filters": ["{\"field\": \"age\", \"op\": \">\", \"value\": 25}"],
				"orderBy": "",
				"limit": 10
			}`, collectionName))),
			wantRegex: `"name":"Alice"`,
			isErr:     false,
		},
		{
			name: "query collection with orderBy",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-coll/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collectionPath": "%s",
				"filters": [],
				"orderBy": "{\"field\": \"age\", \"direction\": \"DESCENDING\"}",
				"limit": 2
			}`, collectionName))),
			wantRegex: `"age":35.*"age":30`, // Should be ordered by age descending (Charlie=35, Alice=30)
			isErr:     false,
		},
		{
			name: "query collection with multiple filters",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-coll/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collectionPath": "%s",
				"filters": [
					"{\"field\": \"age\", \"op\": \">=\", \"value\": 25}",
					"{\"field\": \"age\", \"op\": \"<=\", \"value\": 30}"
				],
				"orderBy": "",
				"limit": 10
			}`, collectionName))),
			wantRegex: `"name":"Bob".*"name":"Alice"`, // Results may be ordered by document ID
			isErr:     false,
		},
		{
			name: "query with limit",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-coll/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collectionPath": "%s",
				"filters": [],
				"orderBy": "",
				"limit": 1
			}`, collectionName))),
			wantRegex: `^\[{.*}\]$`, // Should return exactly one document
			isErr:     false,
		},
		{
			name: "query non-existent collection",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-coll/invoke",
			requestBody: bytes.NewBuffer([]byte(`{
				"collectionPath": "non-existent-collection",
				"filters": [],
				"orderBy": "",
				"limit": 10
			}`)),
			wantRegex: `^\[\]$`, // Empty array
			isErr:     false,
		},
		{
			name:        "missing collectionPath parameter",
			api:         "http://127.0.0.1:5000/api/tool/firestore-query-coll/invoke",
			requestBody: bytes.NewBuffer([]byte(`{}`)),
			isErr:       true,
		},
		{
			name: "invalid filter operator",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-coll/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collectionPath": "%s",
				"filters": ["{\"field\": \"age\", \"op\": \"INVALID\", \"value\": 25}"],
				"orderBy": ""
			}`, collectionName))),
			isErr: true,
		},
		{
			name: "query with analyzeQuery",
			api:  "http://127.0.0.1:5000/api/tool/firestore-query-coll/invoke",
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"collectionPath": "%s",
				"filters": [],
				"orderBy": "",
				"analyzeQuery": true,
				"limit": 1
			}`, collectionName))),
			wantRegex: `"documents":\[.*\]`,
			isErr:     false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if tc.wantRegex != "" {
				matched, err := regexp.MatchString(tc.wantRegex, got)
				if err != nil {
					t.Fatalf("invalid regex pattern: %v", err)
				}
				if !matched {
					t.Fatalf("result does not match expected pattern.\nGot: %s\nWant pattern: %s", got, tc.wantRegex)
				}
			}
		})
	}
}
