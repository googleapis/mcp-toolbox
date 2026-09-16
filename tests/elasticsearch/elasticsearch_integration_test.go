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

package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/esapi"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	ElasticsearchSourceType = "elasticsearch"
	ElasticsearchToolType   = "elasticsearch-esql"
	EsAddress               = ""
	EsUser                  = "elastic"
	EsPass                  = "test-password"
)

func setupElasticsearchContainer(ctx context.Context, t *testing.T) (string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "docker.elastic.co/elasticsearch/elasticsearch:9.3.2",
		ExposedPorts: []string{"9200/tcp"},
		Env: map[string]string{
			"discovery.type":         "single-node",
			"xpack.security.enabled": "false",
		},
		WaitingFor: wait.ForAll(
			wait.ForHTTP("/"),
			wait.ForExposedPort(),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start elasticsearch container: %s", err)
	}

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	}

	host, err := container.Host(ctx)
	if err != nil {
		cleanup()
		t.Fatalf("failed to get container host: %s", err)
	}

	mappedPort, err := container.MappedPort(ctx, "9200")
	if err != nil {
		cleanup()
		t.Fatalf("failed to get container mapped port: %s", err)
	}

	return fmt.Sprintf("http://%s:%s", host, mappedPort.Port()), cleanup
}

func getElasticsearchVars(t *testing.T) map[string]any {
	return map[string]any{
		"type":      ElasticsearchSourceType,
		"addresses": []string{EsAddress},
		"username":  EsUser,
		"password":  EsPass,
	}
}

type ElasticsearchWants struct {
	Select1               string
	MyToolId3NameAlice    string
	MyToolById4           string
	Null                  string
	McpMyFailTool         string
	McpMyToolId3NameAlice string
	McpSelect1            string
}

func TestElasticsearchToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var containerCleanup func()
	EsAddress, containerCleanup = setupElasticsearchContainer(ctx, t)
	defer containerCleanup()

	args := []string{"--enable-api"}

	sourceConfig := getElasticsearchVars(t)

	index := "test-index"

	paramToolStatement, idParamToolStatement, nameParamToolStatement, arrayParamToolStatement, authToolStatement := getElasticsearchQueries(index)

	toolsConfig := getElasticsearchToolsConfig(sourceConfig, ElasticsearchToolType, paramToolStatement, idParamToolStatement, nameParamToolStatement, arrayParamToolStatement, authToolStatement)

	searchStmt := fmt.Sprintf(`FROM %s | WHERE KNN(embedding, ?query) | LIMIT 1 | KEEP id, name`, index)
	insertStmt := fmt.Sprintf("FROM %s | WHERE name == ?content | EVAL dummy = ?text_to_embed | LIMIT 0", index)
	toolsConfig = tests.AddSemanticSearchConfig(t, toolsConfig, ElasticsearchToolType, insertStmt, searchStmt)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsConfig, args...)
	if err != nil {
		t.Fatalf("failed to start cmd: %v", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	esClient, err := elasticsearch.NewBaseClient(elasticsearch.Config{
		Addresses: []string{EsAddress},
		Username:  EsUser,
		Password:  EsPass,
	})
	if err != nil {
		t.Fatalf("error creating the Elasticsearch client: %s", err)
	}

	// Delete indices if already exists
	defer func() {
		_, err = esapi.IndicesDeleteRequest{
			Index: []string{index},
		}.Do(ctx, esClient)
		if err != nil {
			t.Errorf("error deleting indices: %s", err)
		}
	}()

	alice := fmt.Sprintf(`{
                  "id": 1,
                  "name": "Alice",
                  "email": "%s"
                }`, tests.ServiceAccountEmail)

	// Create index with mapping for vector search
	mapping := `{
    "mappings": {
      "properties": {
        "embedding": {
          "type": "dense_vector",
          "dims": 768,
          "index": true,
          "similarity": "cosine"
        }
      }
    }
  }`
	res, err := esapi.IndicesCreateRequest{
		Index: index,
		Body:  strings.NewReader(mapping),
	}.Do(ctx, esClient)
	if err != nil {
		t.Fatalf("error creating index: %s", err)
	}
	if res.IsError() {
		t.Logf("Create index response error (might be ignored): %s", res.String())
	}

	vectorSize := 768
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < vectorSize; i++ {
		sb.WriteString("0.1")
		if i < vectorSize-1 {
			sb.WriteString(", ")
		}
	}
	sb.WriteString("]")
	semanticDoc := fmt.Sprintf(`{"id": 5, "name": "Semantic", "embedding": %s}`, sb.String())

	// Index sample documents
	sampleDocs := []string{
		alice,
		`{"id": 2, "name": "Jane", "email": "janedoe@gmail.com"}`,
		`{"id": 3, "name": "Sid"}`,
		`{"id": 4, "name": "null"}`,
		semanticDoc,
	}
	for _, doc := range sampleDocs {
		res, err := esapi.IndexRequest{
			Index:   "test-index",
			Body:    strings.NewReader(doc),
			Refresh: "true",
		}.Do(ctx, esClient)
		if res.IsError() {
			t.Fatalf("error indexing document: %s", res.String())
		}
		if err != nil {
			t.Fatalf("error indexing document: %s", err)
		}
	}

	// Get configs for tests
	wants := getElasticsearchWants()

	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, wants.Select1,
		tests.DisableArrayTest(),

		tests.WithMyToolId3NameAliceWant(wants.MyToolId3NameAlice),
		tests.WithMyToolById4Want(wants.MyToolById4),
		tests.WithNullWant(wants.Null),
	)
	tests.RunMCPToolCallMethod(t, wants.McpMyFailTool, wants.McpSelect1, tests.WithMcpMyToolId3NameAliceWant(wants.McpMyToolId3NameAlice))

	// Semantic search tests
	semanticSearchWant := `[{"id":5,"name":"Semantic"}]`
	tests.RunSemanticSearchToolInvokeTest(t, "[]", "[]", semanticSearchWant)
	runExecuteEsqlTest(t, index)
}

func getElasticsearchQueries(index string) (string, string, string, string, string) {
	paramToolStatement := fmt.Sprintf(`FROM %s | WHERE id == ?id OR name == ?name | SORT id ASC | KEEP id, name, name.keyword, email, email.keyword`, index)
	idParamToolStatement := fmt.Sprintf(`FROM %s | WHERE id == ?id | KEEP id, name, name.keyword, email, email.keyword`, index)
	nameParamToolStatement := fmt.Sprintf(`FROM %s | WHERE name == ?name | KEEP id, name, name.keyword, email, email.keyword`, index)
	authToolStatement := fmt.Sprintf(`FROM %s | WHERE email == ?email | KEEP name`, index)
	return paramToolStatement, idParamToolStatement, nameParamToolStatement, "", authToolStatement
}

func getElasticsearchWants() ElasticsearchWants {
	select1Want := fmt.Sprintf(`[{"email":"%[1]s","email.keyword":"%[1]s","id":1,"name":"Alice","name.keyword":"Alice"},{"email":"janedoe@gmail.com","email.keyword":"janedoe@gmail.com","id":2,"name":"Jane","name.keyword":"Jane"},{"email":null,"email.keyword":null,"id":3,"name":"Sid","name.keyword":"Sid"},{"email":null,"email.keyword":null,"id":4,"name":"null","name.keyword":"null"},{"email":null,"email.keyword":null,"id":5,"name":"Semantic","name.keyword":"Semantic"}]`, tests.ServiceAccountEmail)
	myToolId3NameAliceWant := fmt.Sprintf(`[{"email":"%[1]s","email.keyword":"%[1]s","id":1,"name":"Alice","name.keyword":"Alice"},{"email":null,"email.keyword":null,"id":3,"name":"Sid","name.keyword":"Sid"}]`, tests.ServiceAccountEmail)
	myToolById4Want := `[{"email":null,"email.keyword":null,"id":4,"name":"null","name.keyword":"null"}]`
	nullWant := `{"error":{"root_cause":[{"type":"verification_exception","reason":"Found 1 problem\nline 1:25: first argument of [name == ?name] is [text] so second argument must also be [text] but was [null]"}],"type":"verification_exception","reason":"Found 1 problem\nline 1:25: first argument of [name == ?name] is [text] so second argument must also be [text] but was [null]"},"status":400}`
	mcpMyFailToolWant := `{"content":[{"type":"text","text":"{\"error\":{\"root_cause\":[{\"type\":\"parsing_exception\",\"reason\":\"line 1:1: mismatched input 'SELEC' expecting {, 'row', 'from', 'ts', 'set', 'show'}\"}],\"type\":\"parsing_exception\",\"reason\":\"line 1:1: mismatched input 'SELEC' expecting {, 'row', 'from', 'ts', 'set', 'show'}\",\"caused_by\":{\"type\":\"input_mismatch_exception\",\"reason\":null}},\"status\":400}"}]}`
	mcpMyToolId3NameAliceWant := fmt.Sprintf(`{"jsonrpc":"2.0","id":"my-tool","result":{"content":[{"type":"text","text":"[{\"email\":\"%[1]s\",\"email.keyword\":\"%[1]s\",\"id\":1,\"name\":\"Alice\",\"name.keyword\":\"Alice\"},{\"email\":null,\"email.keyword\":null,\"id\":3,\"name\":\"Sid\",\"name.keyword\":\"Sid\"}]"}]}}`, tests.ServiceAccountEmail)
	mcpSelect1Want := fmt.Sprintf(`{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"[{\"email\":\"%[1]s\",\"email.keyword\":\"%[1]s\",\"id\":1,\"name\":\"Alice\",\"name.keyword\":\"Alice\"},{\"email\":\"janedoe@gmail.com\",\"email.keyword\":\"janedoe@gmail.com\",\"id\":2,\"name\":\"Jane\",\"name.keyword\":\"Jane\"},{\"email\":null,\"email.keyword\":null,\"id\":3,\"name\":\"Sid\",\"name.keyword\":\"Sid\"},{\"email\":null,\"email.keyword\":null,\"id\":4,\"name\":\"null\",\"name.keyword\":\"null\"},{\"email\":null,\"email.keyword\":null,\"id\":5,\"name\":\"Semantic\",\"name.keyword\":\"Semantic\"}]"}]}}`, tests.ServiceAccountEmail)

	return ElasticsearchWants{
		Select1:               select1Want,
		MyToolId3NameAlice:    myToolId3NameAliceWant,
		MyToolById4:           myToolById4Want,
		Null:                  nullWant,
		McpMyFailTool:         mcpMyFailToolWant,
		McpMyToolId3NameAlice: mcpMyToolId3NameAliceWant,
		McpSelect1:            mcpSelect1Want,
	}
}

func getElasticsearchToolsConfig(sourceConfig map[string]any, toolType, paramToolStatement, idParamToolStmt, nameParamToolStmt, arrayToolStatement, authToolStatement string) map[string]any {
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
				"type":        toolType,
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
				"query":       "FROM test-index | SORT id ASC | KEEP id, name, name.keyword, email, email.keyword",
			},
			"my-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"query":       paramToolStatement,
				"parameters": []any{
					map[string]any{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
					map[string]any{
						"name":        "name",
						"type":        "string",
						"description": "user name",
					},
				},
			},
			"my-secure-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test secure parameters.",
				"query":       "FROM test-index | WHERE id == ?id OR name == ?name | SORT id ASC | KEEP id, name",
				"parameters": []any{
					map[string]any{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
					map[string]any{
						"name":        "name",
						"type":        "string",
						"description": "user name",
						"secure":      true,
					},
				},
			},
			"my-tool-by-id": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"query":       idParamToolStmt,
				"parameters": []any{
					map[string]any{
						"name":        "id",
						"type":        "integer",
						"description": "user ID",
					},
				},
			},
			"my-tool-by-name": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test invocation with params.",
				"query":       nameParamToolStmt,
				"parameters": []any{
					map[string]any{
						"name":        "name",
						"type":        "string",
						"description": "user name",
						"required":    false,
					},
				},
			},
			"my-auth-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test authenticated parameters.",
				// statement to auto-fill authenticated parameter
				"query": authToolStatement,
				"parameters": []map[string]any{
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
			},
			"my-auth-required-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test auth required invocation.",
				"query":       "FROM test-index | SORT id ASC | KEEP id, name, name.keyword, email, email.keyword",
				"authRequired": []string{
					"my-google-auth",
				},
			},
			"my-fail-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"description": "Tool to test statement with incorrect syntax.",
				"query":       "SELEC 1;",
			},
			"my-execute-tool": map[string]any{
				"type":        "elasticsearch-execute-esql",
				"source":      "my-instance",
				"description": "Tool to test arbitrary ES|QL execution.",
			},
		},
	}
	return toolsFile
}

func runExecuteEsqlTest(t *testing.T, index string) {
	t.Run("invoke my-execute-tool", func(t *testing.T) {
		api := "http://127.0.0.1:5000/api/tool/my-execute-tool/invoke"
		reqBody := map[string]any{
			"query": fmt.Sprintf("FROM %s | KEEP id | SORT id ASC", index),
		}
		bodyBytes, _ := json.Marshal(reqBody)
		resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer(bodyBytes), nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
		}
		var body map[string]interface{}
		err := json.Unmarshal(respBody, &body)
		if err != nil {
			t.Fatalf("error parsing response body")
		}
		got, ok := body["result"].(string)
		if !ok {
			t.Fatalf("unable to find result in response body")
		}
		want := `[{"id":1},{"id":2},{"id":3},{"id":4},{"id":5}]`
		if got != want {
			t.Fatalf("unexpected value: got %q, want %q", got, want)
		}
	})
}
