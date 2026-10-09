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

package firestoremongodb

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

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	FirestoreSourceType      = "firestore"
	FirestoreMongodbProject  = os.Getenv("FIRESTORE_MONGODB_PROJECT")
	FirestoreMongodbDatabase = os.Getenv("FIRESTORE_MONGODB_DATABASE")
)

const precreatedCollection = "testcollection"

func getFirestoreMongodbVars(t *testing.T) map[string]any {
	project := FirestoreMongodbProject
	if project == "" {
		project = os.Getenv("FIRESTORE_PROJECT")
	}
	if project == "" {
		t.Fatal("'FIRESTORE_MONGODB_PROJECT' or 'FIRESTORE_PROJECT' not set")
	}

	database := FirestoreMongodbDatabase
	if database == "" {
		database = "mcp-toolbox-db-native-schema"
	}

	vars := map[string]any{
		"type":     FirestoreSourceType,
		"project":  project,
		"database": database,
	}

	return vars
}

func TestFirestoreMongodbToolEndpoints(t *testing.T) {
	sourceConfig := getFirestoreMongodbVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	// Write config into a file and pass it to command
	toolsFile := getFirestoreMongodbToolsConfig(sourceConfig)

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

	// Run tool get tests
	runFirestoreMongodbToolGetTest(t)

	// Run tool execution tests against pre-created collection
	runFirestoreMongodbGetSchemaTest(t, precreatedCollection)
	runFirestoreMongodbExecuteMQLTest(t, precreatedCollection)
}

func runFirestoreMongodbToolGetTest(t *testing.T) {
	tcs := []struct {
		name string
		api  string
		want map[string]any
	}{
		{
			name: "get firestore-mongodb-get-schema",
			api:  "http://127.0.0.1:5000/api/tool/firestore-mongodb-get-schema/",
			want: map[string]any{
				"firestore-mongodb-get-schema": map[string]any{
					"description": "Get schema for Firestore collections",
					"parameters": []any{
						map[string]any{
							"name":         "collection",
							"type":         "string",
							"required":     false,
							"default":      "",
							"description":  "Optional name or path of a specific collection to get schema for. If omitted, schemas for all root collections are returned.",
							"authServices": []any{},
						},
					},
					"authRequired": []any{},
				},
			},
		},
		{
			name: "get firestore-mongodb-execute-mql",
			api:  "http://127.0.0.1:5000/api/tool/firestore-mongodb-execute-mql/",
			want: map[string]any{
				"firestore-mongodb-execute-mql": map[string]any{
					"description": "Execute MQL query or aggregation pipeline against Firestore",
					"parameters": []any{
						map[string]any{
							"name":         "query",
							"type":         "string",
							"required":     true,
							"description":  "The MQL query or aggregation pipeline to execute against Firestore.",
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

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tc.want)
			if string(gotJSON) != string(wantJSON) {
				t.Logf("got %v, want %v", got, tc.want)
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func getFirestoreMongodbToolsConfig(sourceConfig map[string]any) map[string]any {
	sources := map[string]any{
		"my-instance": sourceConfig,
	}

	tools := map[string]any{
		"firestore-mongodb-get-schema": map[string]any{
			"type":        "firestore-mongodb-get-schema",
			"source":      "my-instance",
			"description": "Get schema for Firestore collections",
		},
		"firestore-mongodb-execute-mql": map[string]any{
			"type":        "firestore-mongodb-execute-mql",
			"source":      "my-instance",
			"description": "Execute MQL query or aggregation pipeline against Firestore",
		},
	}

	return map[string]any{
		"sources": sources,
		"tools":   tools,
	}
}

func runFirestoreMongodbGetSchemaTest(t *testing.T, collectionName string, opts ...tests.ToolExecOption) {
	config := &tests.ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	invokeTcs := []struct {
		name      string
		toolName  string
		args      map[string]any
		wantRegex string
		isErr     bool
	}{
		{
			name:      "get schema for specific collection",
			toolName:  "firestore-mongodb-get-schema",
			args:      map[string]any{"collection": collectionName},
			wantRegex: fmt.Sprintf(`"collection":"%s"`, collectionName),
			isErr:     false,
		},
		{
			name:      "get schema for all root collections",
			toolName:  "firestore-mongodb-get-schema",
			args:      map[string]any{},
			wantRegex: `.*`,
			isErr:     false,
		},
		{
			name:      "get schema for non-existent collection",
			toolName:  "firestore-mongodb-get-schema",
			args:      map[string]any{"collection": "non_existent_collection_xyz"},
			wantRegex: `.*`,
			isErr:     false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var got string

			if config.IsMCP() {
				statusCode, mcpResp, err := tests.InvokeMCPTool(t, tc.toolName, tc.args, nil)
				if err != nil {
					// InvokeMCPTool only returns an error when a non-200
					// response body could not be parsed as JSON-RPC, which is
					// one of the shapes an expected failure can take.
					if tc.isErr {
						return
					}
					t.Fatalf("unable to send request: %s", err)
				}
				if statusCode != http.StatusOK {
					if tc.isErr {
						return
					}
					t.Fatalf("response status code is not 200, got %d", statusCode)
				}
				if mcpResp.Result.IsError || mcpResp.Error != nil {
					if tc.isErr {
						return
					}
					t.Fatalf("%s returned an error result: %v", tc.toolName, mcpResp.Result)
				}
				if tc.isErr {
					t.Fatalf("expected %s to fail, but it succeeded", tc.toolName)
				}
				var sb strings.Builder
				for _, content := range mcpResp.Result.Content {
					sb.WriteString(content.Text)
				}
				got = sb.String()
			} else {
				api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
				reqBytes, err := json.Marshal(tc.args)
				if err != nil {
					t.Fatalf("unable to marshal args: %s", err)
				}
				req, err := http.NewRequest(http.MethodPost, api, bytes.NewBuffer(reqBytes))
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
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("error parsing response body: %v", err)
				}

				result, ok := body["result"].(string)
				if !ok {
					t.Fatalf("unable to find result in response body")
				}
				got = result
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

func runFirestoreMongodbExecuteMQLTest(t *testing.T, collectionName string, opts ...tests.ToolExecOption) {
	config := &tests.ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	invokeTcs := []struct {
		name      string
		toolName  string
		args      map[string]any
		wantRegex string
		isErr     bool
	}{
		{
			name:      "execute MQL structured pipeline get_schema stage",
			toolName:  "firestore-mongodb-execute-mql",
			args:      map[string]any{"query": fmt.Sprintf(`{"structuredPipeline": {"pipeline": {"stages": [{"name": "get_schema", "args": [{"stringValue": "{\"collection\": \"%s\", \"semantics\": \"mongodb\"}"}]}]}}}`, collectionName)},
			wantRegex: `.*`,
			isErr:     false,
		},
		{
			name:      "execute MQL find query",
			toolName:  "firestore-mongodb-execute-mql",
			args:      map[string]any{"query": fmt.Sprintf("%s.find({})", collectionName)},
			wantRegex: `.*`,
			isErr:     false,
		},
		{
			name:     "execute MQL with empty query",
			toolName: "firestore-mongodb-execute-mql",
			args:     map[string]any{"query": ""},
			isErr:    true,
		},
		{
			name:     "missing query parameter",
			toolName: "firestore-mongodb-execute-mql",
			args:     map[string]any{},
			isErr:    true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var got string

			if config.IsMCP() {
				statusCode, mcpResp, err := tests.InvokeMCPTool(t, tc.toolName, tc.args, nil)
				if err != nil {
					// InvokeMCPTool only returns an error when a non-200
					// response body could not be parsed as JSON-RPC, which is
					// one of the shapes an expected failure can take.
					if tc.isErr {
						return
					}
					t.Fatalf("unable to send request: %s", err)
				}
				if statusCode != http.StatusOK {
					if tc.isErr {
						return
					}
					t.Fatalf("response status code is not 200, got %d", statusCode)
				}
				if mcpResp.Result.IsError || mcpResp.Error != nil {
					if tc.isErr {
						return
					}
					t.Fatalf("%s returned an error result: %v", tc.toolName, mcpResp.Result)
				}
				if tc.isErr {
					t.Fatalf("expected %s to fail, but it succeeded", tc.toolName)
				}
				var sb strings.Builder
				for _, content := range mcpResp.Result.Content {
					sb.WriteString(content.Text)
				}
				got = sb.String()
			} else {
				api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
				reqBytes, err := json.Marshal(tc.args)
				if err != nil {
					t.Fatalf("unable to marshal args: %s", err)
				}
				req, err := http.NewRequest(http.MethodPost, api, bytes.NewBuffer(reqBytes))
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
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("error parsing response body: %v", err)
				}

				result, ok := body["result"].(string)
				if !ok {
					t.Fatalf("unable to find result in response body")
				}
				got = result
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
