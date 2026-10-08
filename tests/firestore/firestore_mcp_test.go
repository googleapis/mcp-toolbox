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

package firestore

import (
	"context"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/tests"
)

// getFirestoreMCPManifests returns the expected tools/list entries for the
// tools in getFirestoreToolsConfig.
func getFirestoreMCPManifests() []tests.MCPToolManifest {
	addDocsSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"collectionPath": map[string]any{
				"type":        "string",
				"description": "The relative path of the collection where the document will be added to (e.g., 'users' or 'users/userId/posts'). Note: This is a relative path, NOT an absolute path like 'projects/{project_id}/databases/{database_id}/documents/...'",
			},
			"documentData": map[string]any{
				"type":                 "object",
				"description":          "The document data in Firestore's native JSON format. Each field must be wrapped with a type indicator:\n- Strings: {\"stringValue\": \"text\"}\n- Integers: {\"integerValue\": \"123\"} or {\"integerValue\": 123}\n- Doubles: {\"doubleValue\": 123.45}\n- Booleans: {\"booleanValue\": true}\n- Timestamps: {\"timestampValue\": \"2025-01-07T10:00:00Z\"}\n- GeoPoints: {\"geoPointValue\": {\"latitude\": 34.05, \"longitude\": -118.24}}\n- Arrays: {\"arrayValue\": {\"values\": [{\"stringValue\": \"item1\"}, {\"integerValue\": \"2\"}]}}\n- Maps: {\"mapValue\": {\"fields\": {\"key1\": {\"stringValue\": \"value1\"}, \"key2\": {\"booleanValue\": true}}}}\n- Null: {\"nullValue\": null}\n- Bytes: {\"bytesValue\": \"base64EncodedString\"}\n- References: {\"referenceValue\": \"collection/document\"}",
				"additionalProperties": true,
			},
			"returnData": map[string]any{
				"type":        "boolean",
				"description": "If set to true the output will have the data of the created document. This flag if set to false will help avoid overloading the context of the agent.",
				"default":     false,
			},
		},
		"required": []any{"collectionPath", "documentData"},
	}
	deleteDocsSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"documentPaths": map[string]any{
				"type":        "array",
				"description": "Array of relative document paths to delete from Firestore (e.g., 'users/userId' or 'users/userId/posts/postId'). Note: These are relative paths, NOT absolute paths like 'projects/{project_id}/databases/{database_id}/documents/...'",
				"items": map[string]any{
					"type":        "string",
					"description": "Relative document path",
				},
			},
		},
		"required": []any{"documentPaths"},
	}
	getDocsSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"documentPaths": map[string]any{
				"type":        "array",
				"description": "Array of relative document paths to retrieve from Firestore (e.g., 'users/userId' or 'users/userId/posts/postId'). Note: These are relative paths, NOT absolute paths like 'projects/{project_id}/databases/{database_id}/documents/...'",
				"items": map[string]any{
					"type":        "string",
					"description": "Relative document path",
				},
			},
		},
		"required": []any{"documentPaths"},
	}
	getRulesSchema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []any{},
	}
	listCollectionsSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"parentPath": map[string]any{
				"type":        "string",
				"description": "Relative parent document path to list subcollections from (e.g., 'users/userId'). If not provided, lists root collections. Note: This is a relative path, NOT an absolute path like 'projects/{project_id}/databases/{database_id}/documents/...'",
				"default":     "",
			},
		},
		"required": []any{},
	}
	queryCollectionSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"analyzeQuery": map[string]any{
				"type":        "boolean",
				"description": "If true, returns query explain metrics including execution statistics",
				"default":     false,
			},
			"collectionPath": map[string]any{
				"type":        "string",
				"description": "The relative path to the Firestore collection to query (e.g., 'users' or 'users/userId/posts'). Note: This is a relative path, NOT an absolute path like 'projects/{project_id}/databases/{database_id}/documents/...'",
			},
			"filters": map[string]any{
				"type":        "array",
				"description": "Array of filter objects to apply to the query. Each filter is a JSON string with:\n- field: The field name to filter on\n- op: The operator to use (\"<\", \"<=\", \">\", \">=\", \"==\", \"!=\", \"array-contains\", \"array-contains-any\", \"in\", \"not-in\")\n- value: The value to compare against (can be string, number, boolean, or array)\nExample: {\"field\": \"age\", \"op\": \">\", \"value\": 18}",
				"items": map[string]any{
					"type":        "string",
					"description": "JSON string representation of a filter object",
				},
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "The maximum number of documents to return",
				"default":     float64(100), // JSON numbers decode as float64
			},
			"orderBy": map[string]any{
				"type":        "string",
				"description": "JSON string specifying the field and direction to order by (e.g., {\"field\": \"name\", \"direction\": \"ASCENDING\"}). Leave empty if not specified",
			},
		},
		"required": []any{"collectionPath", "filters", "orderBy"},
	}
	queryParamSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ageValue": map[string]any{
				"type":        "string",
				"description": "Age value to compare",
			},
			"collection": map[string]any{
				"type":        "string",
				"description": "Collection to query",
			},
			"operator": map[string]any{
				"type":        "string",
				"description": "Comparison operator",
			},
		},
		"required": []any{"collection", "operator", "ageValue"},
	}
	querySelectArraySchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"collection": map[string]any{
				"type":        "string",
				"description": "Collection to query",
			},
			"fields": map[string]any{
				"type":        "array",
				"description": "Fields to select",
				"items": map[string]any{
					"type":        "string",
					"description": "field",
				},
			},
		},
		"required": []any{"collection", "fields"},
	}
	updateDocSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"documentData": map[string]any{
				"type":                 "object",
				"description":          "The document data in Firestore's native JSON format. Each field must be wrapped with a type indicator:\n- Strings: {\"stringValue\": \"text\"}\n- Integers: {\"integerValue\": \"123\"} or {\"integerValue\": 123}\n- Doubles: {\"doubleValue\": 123.45}\n- Booleans: {\"booleanValue\": true}\n- Timestamps: {\"timestampValue\": \"2025-01-07T10:00:00Z\"}\n- GeoPoints: {\"geoPointValue\": {\"latitude\": 34.05, \"longitude\": -118.24}}\n- Arrays: {\"arrayValue\": {\"values\": [{\"stringValue\": \"item1\"}, {\"integerValue\": \"2\"}]}}\n- Maps: {\"mapValue\": {\"fields\": {\"key1\": {\"stringValue\": \"value1\"}, \"key2\": {\"booleanValue\": true}}}}\n- Null: {\"nullValue\": null}\n- Bytes: {\"bytesValue\": \"base64EncodedString\"}\n- References: {\"referenceValue\": \"collection/document\"}",
				"additionalProperties": true,
			},
			"documentPath": map[string]any{
				"type":        "string",
				"description": "The relative path of the document which needs to be updated (e.g., 'users/userId' or 'users/userId/posts/postId'). Note: This is a relative path, NOT an absolute path like 'projects/{project_id}/databases/{database_id}/documents/...'",
			},
			"returnData": map[string]any{
				"type":        "boolean",
				"description": "If set to true the output will have the data of the updated document. This flag if set to false will help avoid overloading the context of the agent.",
				"default":     false,
			},
			"updateMask": map[string]any{
				"type":        "array",
				"description": "The selective fields to update. If not provided, all fields in documentData will be updated. When provided, only the specified fields will be updated. Fields referenced in the mask but not present in documentData will be deleted from the document",
				"items": map[string]any{
					"type":        "string",
					"description": "Field path to update or delete. Use dot notation to access nested fields within maps (e.g., 'address.city' to update the city field within an address map, or 'user.profile.name' for deeply nested fields). To delete a field, include it in the mask but omit it from documentData. Note: You cannot update individual array elements; you must update the entire array field",
				},
			},
		},
		"required": []any{"documentPath", "documentData"},
	}
	validateRulesSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"source": map[string]any{
				"type":        "string",
				"description": "The Firestore Rules source code to validate",
			},
		},
		"required": []any{"source"},
	}

	return []tests.MCPToolManifest{
		{Name: "my-simple-tool", Description: "Simple tool to test end to end functionality.", InputSchema: getDocsSchema},
		{Name: "my-param-tool", Description: "Tool to get documents by paths", InputSchema: getDocsSchema},
		{Name: "my-fail-tool", Description: "Tool that will fail", InputSchema: getDocsSchema},
		{Name: "firestore-add-docs", Description: "Add documents to Firestore", InputSchema: addDocsSchema},
		{Name: "firestore-delete-docs", Description: "Delete documents from Firestore", InputSchema: deleteDocsSchema},
		{Name: "firestore-get-docs", Description: "Get multiple documents from Firestore", InputSchema: getDocsSchema},
		{Name: "firestore-get-rules", Description: "Get Firestore security rules", InputSchema: getRulesSchema},
		{Name: "firestore-list-colls", Description: "List Firestore collections", InputSchema: listCollectionsSchema},
		{Name: "firestore-query-coll", Description: "Query a Firestore collection", InputSchema: queryCollectionSchema},
		{Name: "firestore-query-param", Description: "Query a Firestore collection with parameterizable filters", InputSchema: queryParamSchema},
		{Name: "firestore-query-select-array", Description: "Query with array-based select fields", InputSchema: querySelectArraySchema},
		{Name: "firestore-update-doc", Description: "Update a document in Firestore", InputSchema: updateDocSchema},
		{Name: "firestore-validate-rules", Description: "Validate Firestore security rules", InputSchema: validateRulesSchema},
	}
}

func TestFirestoreMCPToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	toolsFile, d := setupFirestoreTest(t, ctx)

	tr := firestoreTransport{isMCP: true}
	tr.startServer(t, ctx, toolsFile)

	tests.RunMCPToolsListMethod(t, getFirestoreMCPManifests())

	// Run Firestore-specific MCP test
	runFirestoreMCPToolCallMethod(t, d.docPath1, d.docPath2)

	runFirestoreTests(t, ctx, tr, d)
}
