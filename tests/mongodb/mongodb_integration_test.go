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

package mongodb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	tcmongodb "github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	MongoDbSourceType = "mongodb"
	MongoDbToolType   = "mongodb-find"
	MongoDbDatabase   = "testdb"
)

func setupMongoDBContainer(ctx context.Context, t *testing.T) (string, func()) {
	t.Helper()

	mongodbContainer, err := tcmongodb.Run(ctx, "mongo:6")
	if err != nil {
		t.Fatalf("failed to start mongodb container: %s", err)
	}

	cleanup := func() {
		if err := mongodbContainer.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate mongodb container: %s", err)
		}
	}

	endpoint, err := mongodbContainer.ConnectionString(ctx)
	if err != nil {
		cleanup()
		t.Fatalf("failed to get mongodb connection string: %s", err)
	}

	return endpoint, cleanup
}

func getMongoDBVars(uri string) map[string]any {
	return map[string]any{
		"type": MongoDbSourceType,
		"uri":  uri,
	}
}

// Expected results shared by the REST and MCP MongoDB integration tests.
const (
	mongoDBSelect1Want               = `[{"_id":3,"id":3,"name":"Sid"}]`
	mongoDBMyToolId3NameAliceWant    = `[{"_id":5,"id":3,"name":"Alice"}]`
	mongoDBMyToolById4Want           = `[]`
	mongoDBMcpMyFailToolWant         = `invalid JSON input: missing colon after key `
	mongoDBMcpMyToolId3NameAliceWant = `{"jsonrpc":"2.0","id":"my-tool","result":{"content":[{"type":"text","text":"{\"_id\":5,\"id\":3,\"name\":\"Alice\"}"}]}}`
	mongoDBMcpAuthRequiredWant       = `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"_id\":3,\"id\":3,\"name\":\"Sid\"}"}]}}`
)

// setupMongoDBInstance starts an ephemeral MongoDB container, registers its
// termination on cleanup and returns its connection URI.
func setupMongoDBInstance(ctx context.Context, t *testing.T) string {
	t.Helper()
	uri, cleanupContainer := setupMongoDBContainer(ctx, t)
	t.Cleanup(cleanupContainer)
	return uri
}

// seedMongoDB connects to the MongoDB instance and seeds the test collection.
// The collection is dropped and the client disconnected on cleanup.
func seedMongoDB(ctx context.Context, t *testing.T, uri string) {
	t.Helper()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("unable to connect to mongodb: %s", err)
	}
	t.Cleanup(func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Errorf("unable to disconnect from mongodb: %s", err)
		}
	})
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("unable to connect to mongodb: %s", err)
	}

	collection := client.Database(MongoDbDatabase).Collection("test_collection")

	// Registered before seeding so a partially failed setup is still cleaned
	// up. Runs before the client is disconnected.
	t.Cleanup(func() {
		if err := collection.Drop(context.Background()); err != nil {
			t.Errorf("Teardown failed: %s", err)
		}
	})

	if err := collection.Drop(ctx); err != nil {
		t.Logf("Warning: failed to drop collection before setup: %v", err)
	}

	documents := []map[string]any{
		{"_id": 1, "id": 1, "name": "Alice", "email": tests.ServiceAccountEmail},
		{"_id": 14, "id": 2, "name": "FakeAlice", "email": "fakeAlice@gmail.com"},
		{"_id": 2, "id": 2, "name": "Jane"},
		{"_id": 3, "id": 3, "name": "Sid"},
		{"_id": 5, "id": 3, "name": "Alice", "email": "alice@gmail.com"},
		{"_id": 6, "id": 100, "name": "ToBeDeleted", "email": "bob@gmail.com"},
		{"_id": 7, "id": 101, "name": "ToBeDeleted", "email": "bob1@gmail.com"},
		{"_id": 8, "id": 101, "name": "ToBeDeleted", "email": "bob2@gmail.com"},
		{"_id": 9, "id": 300, "name": "ToBeUpdatedToBob", "email": "bob@gmail.com"},
		{"_id": 10, "id": 400, "name": "ToBeUpdatedToAlice", "email": "alice@gmail.com"},
		{"_id": 11, "id": 400, "name": "ToBeUpdatedToAlice", "email": "alice@gmail.com"},
		{"_id": 12, "id": 500, "name": "ToBeAggregated", "email": "agatha@gmail.com"},
		{"_id": 13, "id": 501, "name": "ToBeAggregated", "email": "agatha@gmail.com"},
	}
	for _, doc := range documents {
		if _, err := collection.InsertOne(ctx, doc); err != nil {
			t.Fatalf("unable to insert test data: %s", err)
		}
	}
}

func TestMongoDBToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	uri := setupMongoDBInstance(ctx, t)
	sourceConfig := getMongoDBVars(uri)

	args := []string{"--enable-api"}

	// set up data for param tool
	seedMongoDB(ctx, t, uri)

	// Write config into a file and pass it to command
	toolsFile := getMongoDBToolsConfig(sourceConfig, MongoDbToolType)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, mongoDBSelect1Want,
		tests.WithMyToolId3NameAliceWant(mongoDBMyToolId3NameAliceWant),
		tests.WithMyArrayToolWant(mongoDBMyToolId3NameAliceWant),
		tests.WithMyToolById4Want(mongoDBMyToolById4Want),
	)
	tests.RunMCPToolCallMethod(t, mongoDBMcpMyFailToolWant, mongoDBSelect1Want,
		tests.WithMcpMyToolId3NameAliceWant(mongoDBMcpMyToolId3NameAliceWant),
		tests.WithMcpSelect1Want(mongoDBMcpAuthRequiredWant),
	)

	runMongoDBRESTInvokeTests(t, ctx)
}

// mongoDBInvokeTestCase describes one tool invocation shared by the REST and
// MCP MongoDB integration tests. want is the REST result string. wantMCPErr is
// set for invocations the tool rejects; over MCP these return an error result
// containing this text.
type mongoDBInvokeTestCase struct {
	name       string
	toolName   string
	args       map[string]any
	want       string
	wantMCPErr string
}

// getMongoDBInvokeTestCases returns the delete, insert, update, aggregate and
// runtime collection invocations exercised by both the REST and MCP MongoDB
// integration tests. The cases run in order against the seeded collection.
func getMongoDBInvokeTestCases() []mongoDBInvokeTestCase {
	return []mongoDBInvokeTestCase{
		{
			name:     "invoke my-delete-one-tool",
			toolName: "my-delete-one-tool",
			args:     map[string]any{"id": 100},
			want:     "1",
		},
		{
			name:     "invoke my-delete-many-tool",
			toolName: "my-delete-many-tool",
			args:     map[string]any{"id": 101},
			want:     "2",
		},
		{
			name:     "invoke my-insert-one-tool",
			toolName: "my-insert-one-tool",
			args:     map[string]any{"data": `{ "_id": { "$oid": "68666e1035bb36bf1b4d47fb" },  "id" : 200 }`},
			want:     `"68666e1035bb36bf1b4d47fb"`,
		},
		{
			name:     "invoke my-insert-many-tool",
			toolName: "my-insert-many-tool",
			args:     map[string]any{"data": `[{ "_id": { "$oid": "68667a6436ec7d0363668db7"} , "id" : 201 }, { "_id" : { "$oid": "68667a6436ec7d0363668db8"}, "id" : 202 }, { "_id": { "$oid": "68667a6436ec7d0363668db9"}, "id": 203 }]`},
			want:     `["68667a6436ec7d0363668db7","68667a6436ec7d0363668db8","68667a6436ec7d0363668db9"]`,
		},
		{
			name:     "invoke my-update-one-tool",
			toolName: "my-update-one-tool",
			args:     map[string]any{"id": 300, "name": "Bob"},
			want:     "1",
		},
		{
			name:     "invoke my-update-many-tool",
			toolName: "my-update-many-tool",
			args:     map[string]any{"id": 400, "name": "Alice"},
			want:     "[2,0,2]",
		},
		{
			name:     "invoke my-aggregate-tool",
			toolName: "my-aggregate-tool",
			args:     map[string]any{"name": "Jane"},
			want:     `[{"id":2}]`,
		},
		{
			name:     "invoke my-aggregate-tool",
			toolName: "my-aggregate-tool",
			args:     map[string]any{"name": "ToBeAggregated"},
			want:     `[{"id":500},{"id":501}]`,
		},
		{
			name:       "invoke my-read-only-aggregate-tool",
			toolName:   "my-read-only-aggregate-tool",
			args:       map[string]any{"name": "ToBeAggregated"},
			want:       `{"error":"error processing request: this is not a read-only pipeline: {\"$out\":\"target_collection\"}"}`,
			wantMCPErr: "this is not a read-only pipeline",
		},
		{
			name:     "invoke my-read-write-aggregate-tool",
			toolName: "my-read-write-aggregate-tool",
			args:     map[string]any{"name": "ToBeAggregated"},
			want:     "[]",
		},
		{
			// The tool has no collection in its config, so it is supplied at runtime.
			name:     "invoke with runtime collection",
			toolName: "my-runtime-collection-tool",
			args:     map[string]any{"id": 3, "collection": "test_collection"},
			want:     mongoDBSelect1Want,
		},
		{
			name:       "invoke without collection returns an error",
			toolName:   "my-runtime-collection-tool",
			args:       map[string]any{"id": 3},
			want:       `{"error":"parameter \"collection\" is required"}`,
			wantMCPErr: `parameter "collection" is required`,
		},
	}
}

// runMongoDBRESTInvokeTests runs the shared invocation cases against the REST
// API.
func runMongoDBRESTInvokeTests(t *testing.T, ctx context.Context) {
	for _, tc := range getMongoDBInvokeTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
			reqBytes, err := json.Marshal(tc.args)
			if err != nil {
				t.Fatalf("unable to marshal request body: %s", err)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, api, bytes.NewBuffer(reqBytes))
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
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			if err = json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("error parsing response body")
			}
			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}
			if got != tc.want {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}

func getMongoDBToolsConfig(sourceConfig map[string]any, toolType string) map[string]any {
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"type":     "google",
				"clientId": tests.ClientId,
			},
		},
		"tools": map[string]any{
			"my-simple-tool": map[string]any{
				"type":           "mongodb-find-one",
				"source":         "my-instance",
				"description":    "Simple tool to test end to end functionality.",
				"collection":     "test_collection",
				"filterPayload":  `{ "_id" : 3 }`,
				"filterParams":   []any{},
				"projectPayload": `{ "_id": 1, "id": 1, "name" : 1 }`,
				"database":       MongoDbDatabase,
			},
			"my-tool": map[string]any{
				"type":          toolType,
				"source":        "my-instance",
				"description":   "Tool to test invocation with params.",
				"authRequired":  []string{},
				"collection":    "test_collection",
				"filterPayload": `{ "id" : {{ .id }}, "name" : {{json .name }} }`,
				"filterParams": []map[string]any{
					{
						"name":        "id",
						"type":        "integer",
						"description": "user id",
					},
					{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
				"projectPayload": `{ "_id": 1, "id": 1, "name" : 1 }`,
				"database":       MongoDbDatabase,
				"limit":          10,
			},
			"my-secure-tool": map[string]any{
				"type":          toolType,
				"source":        "my-instance",
				"description":   "Tool to test secure parameters.",
				"authRequired":  []string{},
				"collection":    "test_collection",
				"filterPayload": `{ "$or": [ { "id": 1, "name": {{json .name }} }, { "id": {{ .id }}, "name": "Sid" } ] }`,
				"filterParams": []map[string]any{
					{
						"name":        "id",
						"type":        "integer",
						"description": "user id",
					},
					{
						"name":        "name",
						"type":        "string",
						"description": "user name",
						"secure":      true,
					},
				},
				"projectPayload": `{ "_id": 0, "id": 1, "name" : 1 }`,
				"sortPayload":    `{ "id": 1 }`,
				"database":       MongoDbDatabase,
				"limit":          10,
			},
			"my-tool-by-id": map[string]any{
				"type":          toolType,
				"source":        "my-instance",
				"description":   "Tool to test invocation with params.",
				"authRequired":  []string{},
				"collection":    "test_collection",
				"filterPayload": `{ "id" : {{ .id }} }`,
				"filterParams": []map[string]any{
					{
						"name":        "id",
						"type":        "integer",
						"description": "user id",
					},
				},
				"projectPayload": `{ "_id": 1, "id": 1, "name" : 1 }`,
				"database":       MongoDbDatabase,
				"limit":          10,
			},
			"my-runtime-collection-tool": map[string]any{
				"type":          toolType,
				"source":        "my-instance",
				"description":   "Tool to test runtime collection selection.",
				"authRequired":  []string{},
				"filterPayload": `{ "_id" : {{ .id }} }`,
				"filterParams": []map[string]any{
					{
						"name":        "id",
						"type":        "integer",
						"description": "user id",
					},
				},
				"projectPayload": `{ "_id": 1, "id": 1, "name" : 1 }`,
				"database":       MongoDbDatabase,
				"limit":          10,
			},
			"my-tool-by-name": map[string]any{
				"type":          toolType,
				"source":        "my-instance",
				"description":   "Tool to test invocation with params.",
				"authRequired":  []string{},
				"collection":    "test_collection",
				"filterPayload": `{ "name" : {{json .name }} }`,
				"filterParams": []map[string]any{
					{
						"name":        "name",
						"type":        "string",
						"description": "user name",
						"required":    false,
					},
				},
				"projectPayload": `{ "_id": 1, "id": 1, "name" : 1 }`,
				"database":       MongoDbDatabase,
				"limit":          10,
			},
			"my-array-tool": map[string]any{
				"type":          toolType,
				"source":        "my-instance",
				"description":   "Tool to test invocation with array.",
				"authRequired":  []string{},
				"collection":    "test_collection",
				"filterPayload": `{ "name": { "$in": {{json .nameArray}} }, "_id": 5 }`,
				"filterParams": []map[string]any{
					{
						"name":        "nameArray",
						"type":        "array",
						"description": "user names",
						"items": map[string]any{
							"name":        "username",
							"type":        "string",
							"description": "string item"},
					},
				},
				"projectPayload": `{ "_id": 1, "id": 1, "name" : 1 }`,
				"database":       MongoDbDatabase,
				"limit":          10,
			},
			"my-auth-tool": map[string]any{
				"type":          toolType,
				"source":        "my-instance",
				"description":   "Tool to test authenticated parameters.",
				"authRequired":  []string{},
				"collection":    "test_collection",
				"filterPayload": `{ "email" : {{json .email }} }`,
				"filterParams": []map[string]any{
					{
						"name":        "email",
						"type":        "string",
						"description": "user email",
						"authServices": []map[string]string{
							{
								"name":  "my-google-auth",
								"field": "email",
							},
						},
					},
				},
				"projectPayload": `{ "_id": 0, "name" : 1 }`,
				"database":       MongoDbDatabase,
				"limit":          10,
			},
			"my-auth-required-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test auth required invocation.",
				"authRequired": []string{
					"my-google-auth",
				},
				"collection":    "test_collection",
				"filterPayload": `{ "_id": 3, "id": 3 }`,
				"filterParams":  []any{},
				"database":      MongoDbDatabase,
				"limit":         10,
			},
			"my-fail-tool": map[string]any{
				"type":          toolType,
				"source":        "my-instance",
				"description":   "Tool to test statement with incorrect syntax.",
				"authRequired":  []string{},
				"collection":    "test_collection",
				"filterPayload": `{ "id" ; 1 }"}`,
				"filterParams":  []any{},
				"database":      MongoDbDatabase,
				"limit":         10,
			},
			"my-delete-one-tool": map[string]any{
				"type":          "mongodb-delete-one",
				"source":        "my-instance",
				"description":   "Tool to test deleting an entry.",
				"authRequired":  []string{},
				"collection":    "test_collection",
				"filterPayload": `{ "id" : 100 }"}`,
				"filterParams":  []any{},
				"database":      MongoDbDatabase,
			},
			"my-delete-many-tool": map[string]any{
				"type":          "mongodb-delete-many",
				"source":        "my-instance",
				"description":   "Tool to test deleting multiple entries.",
				"authRequired":  []string{},
				"collection":    "test_collection",
				"filterPayload": `{ "id" : 101 }"}`,
				"filterParams":  []any{},
				"database":      MongoDbDatabase,
			},
			"my-insert-one-tool": map[string]any{
				"type":         "mongodb-insert-one",
				"source":       "my-instance",
				"description":  "Tool to test inserting an entry.",
				"authRequired": []string{},
				"collection":   "test_collection",
				"canonical":    true,
				"database":     MongoDbDatabase,
			},
			"my-insert-many-tool": map[string]any{
				"type":         "mongodb-insert-many",
				"source":       "my-instance",
				"description":  "Tool to test inserting multiple entries.",
				"authRequired": []string{},
				"collection":   "test_collection",
				"canonical":    true,
				"database":     MongoDbDatabase,
			},
			"my-update-one-tool": map[string]any{
				"type":          "mongodb-update-one",
				"source":        "my-instance",
				"description":   "Tool to test updating an entry.",
				"authRequired":  []string{},
				"collection":    "test_collection",
				"canonical":     true,
				"filterPayload": `{ "id" : {{ .id }} }`,
				"filterParams": []map[string]any{
					{
						"name":        "id",
						"type":        "integer",
						"description": "id",
					},
				},
				"updatePayload": `{ "$set" : { "name": {{json .name}} } }`,
				"updateParams": []map[string]any{
					{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
				"database": MongoDbDatabase,
			},
			"my-update-many-tool": map[string]any{
				"type":          "mongodb-update-many",
				"source":        "my-instance",
				"description":   "Tool to test updating multiple entries.",
				"authRequired":  []string{},
				"collection":    "test_collection",
				"canonical":     true,
				"filterPayload": `{ "id" : {{ .id }} }`,
				"filterParams": []map[string]any{
					{
						"name":        "id",
						"type":        "integer",
						"description": "id",
					},
				},
				"updatePayload": `{ "$set" : { "name": {{json .name}} } }`,
				"updateParams": []map[string]any{
					{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
				"database": MongoDbDatabase,
			},
			"my-aggregate-tool": map[string]any{
				"type":            "mongodb-aggregate",
				"source":          "my-instance",
				"description":     "Tool to test an aggregation.",
				"authRequired":    []string{},
				"collection":      "test_collection",
				"canonical":       true,
				"pipelinePayload": `[{ "$match" : { "name": {{json .name}} } }, { "$project" : { "id" : 1, "_id" : 0 }}]`,
				"pipelineParams": []map[string]any{
					{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
				"database": MongoDbDatabase,
			},
			"my-read-only-aggregate-tool": map[string]any{
				"type":            "mongodb-aggregate",
				"source":          "my-instance",
				"description":     "Tool to test an aggregation.",
				"authRequired":    []string{},
				"collection":      "test_collection",
				"canonical":       true,
				"readOnly":        true,
				"pipelinePayload": `[{ "$match" : { "name": {{json .name}} } }, { "$out" : "target_collection" }]`,
				"pipelineParams": []map[string]any{
					{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
				"database": MongoDbDatabase,
			},
			"my-read-write-aggregate-tool": map[string]any{
				"type":            "mongodb-aggregate",
				"source":          "my-instance",
				"description":     "Tool to test an aggregation.",
				"authRequired":    []string{},
				"collection":      "test_collection",
				"canonical":       true,
				"readOnly":        false,
				"pipelinePayload": `[{ "$match" : { "name": {{json .name}} } }, { "$out" : "target_collection" }]`,
				"pipelineParams": []map[string]any{
					{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
				"database": MongoDbDatabase,
			},
		},
	}

	return toolsFile

}
