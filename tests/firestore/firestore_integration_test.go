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
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	firestoreapi "cloud.google.com/go/firestore"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	FirestoreSourceType = "firestore"
	FirestoreProject    = os.Getenv("FIRESTORE_PROJECT")
	FirestoreDatabase   = os.Getenv("FIRESTORE_DATABASE") // Optional, defaults to "(default)"
)

const (
	testCollectionPrefix     = "test_collection_"
	orphanedCollectionTTL    = 3 * time.Hour
	firestoreCleanupTimeout  = 2 * time.Minute
	firestoreEmulatorHostEnv = "FIRESTORE_EMULATOR_HOST"
)

func TestMain(m *testing.M) {
	if FirestoreProject != "" && os.Getenv(firestoreEmulatorHostEnv) == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		sweepOrphanedFirestoreCollections(ctx, FirestoreProject, FirestoreDatabase)
		cancel()
	}
	os.Exit(m.Run())
}

func shouldSweepFirestoreCollection(name string, now time.Time) bool {
	if !strings.HasPrefix(name, testCollectionPrefix) {
		return false
	}
	suffix := strings.TrimPrefix(name, testCollectionPrefix)
	sep := strings.IndexByte(suffix, '_')
	if sep <= 0 {
		return false
	}
	createdAt, err := strconv.ParseInt(suffix[:sep], 10, 64)
	if err != nil || createdAt <= 0 {
		return false
	}
	return now.Sub(time.Unix(createdAt, 0)) > orphanedCollectionTTL
}

func isFirestoreNotFound(err error) bool {
	return status.Code(err) == codes.NotFound
}

func sweepOrphanedFirestoreCollections(ctx context.Context, project, database string) {
	client, err := initFirestoreConnection(project, database)
	if err != nil {
		log.Printf("INTEGRATION CLEANUP: failed to create Firestore client: %v", err)
		return
	}
	defer func() { _ = client.Close() }()

	colls, err := client.Collections(ctx).GetAll()
	if err != nil {
		log.Printf("INTEGRATION CLEANUP: failed to list root collections: %v", err)
		return
	}
	now := time.Now()
	for _, coll := range colls {
		if !shouldSweepFirestoreCollection(coll.ID, now) {
			continue
		}
		log.Printf("INTEGRATION CLEANUP: deleting orphaned Firestore collection %q", coll.ID)
		if err := deleteFirestoreCollection(ctx, client, coll); err != nil {
			log.Printf("INTEGRATION CLEANUP: failed to delete collection %q: %v", coll.ID, err)
		}
	}
}

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

	client := newFirestoreTestClient(t, FirestoreProject, FirestoreDatabase)

	// Embed Unix timestamp + UUID so TestMain can sweep collections leaked by
	// interrupted runs while keeping concurrent runs isolated.
	testCollectionName := fmt.Sprintf("%s%d_%s", testCollectionPrefix, time.Now().Unix(), strings.ReplaceAll(uuid.New().String(), "-", "")[:16])
	testSubCollectionName := fmt.Sprintf("test_subcollection_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	testDocID1 := fmt.Sprintf("doc_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	testDocID2 := fmt.Sprintf("doc_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	testDocID3 := fmt.Sprintf("doc_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))

	// Document paths for testing
	docPath1 := fmt.Sprintf("%s/%s", testCollectionName, testDocID1)
	docPath2 := fmt.Sprintf("%s/%s", testCollectionName, testDocID2)
	docPath3 := fmt.Sprintf("%s/%s", testCollectionName, testDocID3)

	// Register collection cleanup via t.Cleanup before writing seed data.
	setupFirestoreTestData(t, ctx, client, testCollectionName, testSubCollectionName,
		testDocID1, testDocID2, testDocID3)

	// Write config into a file and pass it to command
	toolsFile := getFirestoreToolsConfig(sourceConfig)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)

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
	runFirestoreQueryCollectionTest(t, testCollectionName, true)
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
			// Keep inside collectionName because Set with MergeAll creates the
			// document, and teardown deletes only collectionName.
			requestBody: bytes.NewBuffer([]byte(fmt.Sprintf(`{
				"documentPath": "%s/non-existent-doc",
				"documentData": {
					"field": {"stringValue": "value"}
				}
			}`, collectionName))),
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

// newFirestoreTestClient builds a Firestore client from context.Background()
// and registers client.Close() via t.Cleanup first so LIFO closes the client
// after collection teardowns finish.
func newFirestoreTestClient(t *testing.T, project, database string) *firestoreapi.Client {
	t.Helper()
	client, err := initFirestoreConnection(project, database)
	if err != nil {
		t.Fatalf("unable to create Firestore connection: %s", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("failed to close Firestore client: %v", err)
		}
	})
	return client
}

// registerFirestoreCollectionCleanup registers a t.Cleanup that deletes only
// collectionName (and its subcollections) under a bounded fresh context.
func registerFirestoreCollectionCleanup(t *testing.T, client *firestoreapi.Client, collectionName string) {
	t.Helper()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), firestoreCleanupTimeout)
		defer cancel()
		if err := deleteFirestoreCollection(cleanupCtx, client, client.Collection(collectionName)); err != nil {
			t.Errorf("failed to delete test collection %s: %v", collectionName, err)
		}
	})
}

func setupFirestoreTestData(t *testing.T, ctx context.Context, client *firestoreapi.Client,
	collectionName, subCollectionName, docID1, docID2, docID3 string) {
	t.Helper()
	registerFirestoreCollectionCleanup(t, client, collectionName)
	seedFirestoreTestCollection(t, ctx, client, collectionName, subCollectionName, docID1, docID2, docID3)
}

// seedFirestoreTestCollection writes all seed documents in a single batched
// BulkWriter flush. Emulator tests call this directly so container termination
// does not spend extra RPC round-trips deleting in-memory documents first.
func seedFirestoreTestCollection(t *testing.T, ctx context.Context, client *firestoreapi.Client,
	collectionName, subCollectionName, docID1, docID2, docID3 string) {
	t.Helper()

	coll := client.Collection(collectionName)
	bw := client.BulkWriter(ctx)
	type seedWrite struct {
		name string
		ref  *firestoreapi.DocumentRef
		data map[string]any
	}
	writes := []seedWrite{
		{name: docID1, ref: coll.Doc(docID1), data: map[string]any{"name": "Alice", "age": 30}},
		{name: docID2, ref: coll.Doc(docID2), data: map[string]any{"name": "Bob", "age": 25}},
		{name: docID3, ref: coll.Doc(docID3), data: map[string]any{"name": "Charlie", "age": 35}},
		{name: "subdoc1", ref: coll.Doc(docID1).Collection(subCollectionName).Doc("subdoc1"), data: map[string]any{"type": "subcollection_doc", "value": "test"}},
	}

	jobs := make([]*firestoreapi.BulkWriterJob, 0, len(writes))
	for _, w := range writes {
		job, err := bw.Set(w.ref, w.data)
		if err != nil {
			bw.End()
			t.Fatalf("Failed to enqueue seed document %s: %v", w.name, err)
		}
		jobs = append(jobs, job)
	}
	bw.End()

	var errs []error
	for i, job := range jobs {
		if _, err := job.Results(); err != nil {
			errs = append(errs, fmt.Errorf("seed document %s: %w", writes[i].name, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		t.Fatalf("Failed to seed Firestore test collection %s: %v", collectionName, err)
	}
}

func collectFirestoreDocumentRefs(ctx context.Context, collection *firestoreapi.CollectionRef, dst *[]*firestoreapi.DocumentRef) error {
	refs, err := collection.DocumentRefs(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("failed to list documents in collection %s: %w", collection.Path, err)
	}
	for _, ref := range refs {
		subcollections, err := ref.Collections(ctx).GetAll()
		if err != nil {
			return fmt.Errorf("failed to list subcollections of document %s: %w", ref.Path, err)
		}
		for _, subcoll := range subcollections {
			if err := collectFirestoreDocumentRefs(ctx, subcoll, dst); err != nil {
				return err
			}
		}
		*dst = append(*dst, ref)
	}
	return nil
}

// deleteFirestoreCollection recursively collects every document in collection
// (including missing parent documents via DocumentRefs) and deletes them in
// batched BulkWriter requests without touching any other collection.
func deleteFirestoreCollection(ctx context.Context, client *firestoreapi.Client, collection *firestoreapi.CollectionRef) error {
	var refs []*firestoreapi.DocumentRef
	if err := collectFirestoreDocumentRefs(ctx, collection, &refs); err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}
	if len(refs) == 1 {
		if _, err := refs[0].Delete(ctx); err != nil && !isFirestoreNotFound(err) {
			return fmt.Errorf("failed to delete document %s: %w", refs[0].Path, err)
		}
		return nil
	}

	bw := client.BulkWriter(ctx)
	jobs := make([]*firestoreapi.BulkWriterJob, 0, len(refs))
	var errs []error
	for _, ref := range refs {
		job, err := bw.Delete(ref)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to enqueue delete for %s: %w", ref.Path, err))
			continue
		}
		jobs = append(jobs, job)
	}
	bw.End()

	for i, job := range jobs {
		if _, err := job.Results(); err != nil && !isFirestoreNotFound(err) {
			errs = append(errs, fmt.Errorf("failed to delete document %s: %w", refs[i].Path, err))
		}
	}
	return errors.Join(errs...)
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

func runFirestoreQueryCollectionTest(t *testing.T, collectionName string, supportsAnalyzeQuery bool) {
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
			if tc.name == "query with analyzeQuery" && !supportsAnalyzeQuery {
				t.Skip("analyzeQuery (Query Explain) is not supported by the Firestore emulator")
			}
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
