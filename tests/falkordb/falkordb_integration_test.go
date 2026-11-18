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

package falkordb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	falkordb "github.com/FalkorDB/falkordb-go/v2"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// falkorDBImage is the image used for the ephemeral test container.
const falkorDBImage = "falkordb/falkordb:v4.18.0"

var (
	FalkorDBSourceType = "falkordb"
	FalkorDBHost       = os.Getenv("FALKORDB_HOST")
	FalkorDBPort       = os.Getenv("FALKORDB_PORT")
	FalkorDBUsername   = os.Getenv("FALKORDB_USERNAME")
	FalkorDBPassword   = os.Getenv("FALKORDB_PASSWORD")
	FalkorDBGraph      = os.Getenv("FALKORDB_GRAPH")
)

// setupFalkorDBContainer starts an ephemeral FalkorDB container and returns
// its host and mapped port, along with a cleanup function that terminates it.
func setupFalkorDBContainer(ctx context.Context, t *testing.T) (string, string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        falkorDBImage,
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(120 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start FalkorDB container: %s", err)
	}

	cleanup := func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	}

	host, err := container.Host(ctx)
	if err != nil {
		cleanup()
		t.Fatalf("failed to get container host: %s", err)
	}

	port, err := container.MappedPort(ctx, "6379")
	if err != nil {
		cleanup()
		t.Fatalf("failed to get container mapped port 6379: %s", err)
	}

	return host, port.Port(), cleanup
}

// getFalkorDBVars retrieves the FalkorDB connection details from environment
// variables. Username and password are optional; the rest are required.
func getFalkorDBVars(t *testing.T) map[string]any {
	switch "" {
	case FalkorDBHost:
		t.Fatal("'FALKORDB_HOST' not set")
	case FalkorDBPort:
		t.Fatal("'FALKORDB_PORT' not set")
	case FalkorDBGraph:
		t.Fatal("'FALKORDB_GRAPH' not set")
	}

	return map[string]any{
		"type":     FalkorDBSourceType,
		"host":     FalkorDBHost,
		"port":     FalkorDBPort,
		"username": FalkorDBUsername,
		"password": FalkorDBPassword,
		"graph":    FalkorDBGraph,
	}
}

// newFalkorDBClient connects directly to the instance for test setup and
// teardown.
func newFalkorDBClient(t *testing.T) *falkordb.FalkorDB {
	client, err := falkordb.FalkorDBNew(&falkordb.ConnectionOption{
		Addr:     net.JoinHostPort(FalkorDBHost, FalkorDBPort),
		Username: FalkorDBUsername,
		Password: FalkorDBPassword,
	})
	if err != nil {
		t.Fatalf("failed to create falkordb client: %v", err)
	}
	return client
}

// setupFalkorDBInstance points the suite at a FalkorDB instance. It defaults to
// an ephemeral container; the FALKORDB_* environment variables take precedence
// when pointing the suite at an external instance. The package-level
// connection variables are restored on cleanup so each test gets its own
// container.
func setupFalkorDBInstance(ctx context.Context, t *testing.T) {
	t.Helper()
	origHost, origPort, origGraph := FalkorDBHost, FalkorDBPort, FalkorDBGraph
	t.Cleanup(func() {
		FalkorDBHost, FalkorDBPort, FalkorDBGraph = origHost, origPort, origGraph
	})

	if FalkorDBHost == "" {
		host, port, cleanup := setupFalkorDBContainer(ctx, t)
		t.Cleanup(cleanup)
		FalkorDBHost, FalkorDBPort = host, port
		FalkorDBGraph = "toolbox_test_graph"
	}
}

// getFalkorDBToolsConfig returns the tools file shared by the FalkorDB
// integration tests.
func getFalkorDBToolsConfig(t *testing.T) map[string]any {
	t.Helper()
	sourceConfig := getFalkorDBVars(t)

	emptyGraphSourceConfig := getFalkorDBVars(t)
	emptyGraphSourceConfig["graph"] = "toolbox_nonexistent_graph"

	return map[string]any{
		"sources": map[string]any{
			"my-instance":       sourceConfig,
			"my-empty-instance": emptyGraphSourceConfig,
		},
		"tools": map[string]any{
			"my-simple-cypher-tool": map[string]any{
				"type":        "falkordb-cypher",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
				"statement":   "RETURN 1 as a",
			},
			"my-param-cypher-tool": map[string]any{
				"type":        "falkordb-cypher",
				"source":      "my-instance",
				"description": "A tool with a parameterized statement.",
				"statement":   "RETURN $value as echoed",
				"parameters": []map[string]any{
					{
						"name":        "value",
						"type":        "string",
						"description": "The value to echo back.",
					},
				},
			},
			"my-simple-execute-cypher-tool": map[string]any{
				"type":        "falkordb-execute-cypher",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"my-readonly-execute-cypher-tool": map[string]any{
				"type":        "falkordb-execute-cypher",
				"source":      "my-instance",
				"description": "A readonly cypher execution tool.",
				"readOnly":    true,
			},
			"my-graph-override-execute-cypher-tool": map[string]any{
				"type":               "falkordb-execute-cypher",
				"source":             "my-instance",
				"description":        "A cypher execution tool allowing graph override.",
				"allowGraphOverride": true,
			},
			"my-schema-tool": map[string]any{
				"type":        "falkordb-schema",
				"source":      "my-instance",
				"description": "A tool to get the FalkorDB graph schema.",
			},
			"my-empty-graph-schema-tool": map[string]any{
				"type":        "falkordb-schema",
				"source":      "my-empty-instance",
				"description": "A schema tool pointed at a graph that does not exist yet.",
			},
			"my-list-graphs-tool": map[string]any{
				"type":        "falkordb-list-graphs",
				"source":      "my-instance",
				"description": "A tool to list the graphs on the instance.",
			},
		},
	}
}

// TestFalkorDBToolEndpoints sets up an integration test server and tests the
// API endpoints of the FalkorDB tools: parameterized cypher, ad-hoc cypher
// execution (including read-only enforcement, dry runs, and graph override),
// schema extraction, and graph listing.
func TestFalkorDBToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	setupFalkorDBInstance(ctx, t)

	args := []string{"--enable-api"}
	toolsFile := getFalkorDBToolsConfig(t)

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

	// Test tool `GET` endpoints to verify their manifests are correct.
	tcs := []struct {
		name string
		api  string
		want map[string]any
	}{
		{
			name: "get my-simple-cypher-tool",
			api:  "http://127.0.0.1:5000/api/tool/my-simple-cypher-tool/",
			want: map[string]any{
				"my-simple-cypher-tool": map[string]any{
					"description":  "Simple tool to test end to end functionality.",
					"parameters":   []any{},
					"authRequired": []any{},
				},
			},
		},
		{
			name: "get my-simple-execute-cypher-tool",
			api:  "http://127.0.0.1:5000/api/tool/my-simple-execute-cypher-tool/",
			want: map[string]any{
				"my-simple-execute-cypher-tool": map[string]any{
					"description": "Simple tool to test end to end functionality.",
					"parameters": []any{
						map[string]any{
							"name":         "cypher",
							"type":         "string",
							"required":     true,
							"description":  "The cypher to execute.",
							"authServices": []any{},
						},
						map[string]any{
							"name":         "dry_run",
							"type":         "boolean",
							"required":     false,
							"description":  "If set to true, the query will be validated and its execution plan returned without running the query. Defaults to false.",
							"default":      false,
							"authServices": []any{},
						},
					},
					"authRequired": []any{},
				},
			},
		},
		{
			name: "get my-schema-tool",
			api:  "http://127.0.0.1:5000/api/tool/my-schema-tool/",
			want: map[string]any{
				"my-schema-tool": map[string]any{
					"description":  "A tool to get the FalkorDB graph schema.",
					"parameters":   []any{},
					"authRequired": []any{},
				},
			},
		},
		{
			name: "get my-list-graphs-tool",
			api:  "http://127.0.0.1:5000/api/tool/my-list-graphs-tool/",
			want: map[string]any{
				"my-list-graphs-tool": map[string]any{
					"description":  "A tool to list the graphs on the instance.",
					"parameters":   []any{},
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

			var body map[string]any
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["tools"]
			if !ok {
				t.Fatalf("unable to find tools in response body")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}

	// Test tool `invoke` endpoints to verify their functionality.
	for _, tc := range getFalkorDBInvokeTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			if tc.prepareData != nil {
				tc.prepareData(t)
			}

			api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
			reqBytes, err := json.Marshal(tc.args)
			if err != nil {
				t.Fatalf("error marshaling request body: %s", err)
			}
			resp, err := http.Post(api, "application/json", bytes.NewBuffer(reqBytes))
			if err != nil {
				t.Fatalf("error when sending a request: %s", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code: got %d, want %d: %s", resp.StatusCode, http.StatusOK, string(bodyBytes))
			}

			var body map[string]any
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}
			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			switch {
			case tc.wantContentErr != "":
				if !strings.Contains(got, tc.wantContentErr) {
					t.Errorf("expected %q in body: %s", tc.wantContentErr, got)
				}
			case tc.validateFunc != nil:
				tc.validateFunc(t, got)
			case got != tc.want:
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}

// falkorDBInvokeTestCase describes one tool invocation shared by the REST and
// MCP FalkorDB integration tests.
type falkorDBInvokeTestCase struct {
	name           string
	toolName       string
	args           map[string]any
	want           string
	wantContentErr string
	prepareData    func(t *testing.T)
	validateFunc   func(t *testing.T, body string)
}

// getFalkorDBInvokeTestCases returns the tool invocations exercised by both the
// REST and MCP FalkorDB integration tests.
func getFalkorDBInvokeTestCases() []falkorDBInvokeTestCase {
	return []falkorDBInvokeTestCase{
		{
			name:     "invoke my-simple-cypher-tool",
			toolName: "my-simple-cypher-tool",
			args:     map[string]any{},
			want:     "[{\"a\":1}]",
		},
		{
			name:     "invoke my-param-cypher-tool",
			toolName: "my-param-cypher-tool",
			args:     map[string]any{"value": "hello"},
			want:     "[{\"echoed\":\"hello\"}]",
		},
		{
			name:     "invoke my-simple-execute-cypher-tool",
			toolName: "my-simple-execute-cypher-tool",
			args:     map[string]any{"cypher": "RETURN 1 as a"},
			want:     "[{\"a\":1}]",
		},
		{
			name:     "invoke my-simple-execute-cypher-tool with write query",
			toolName: "my-simple-execute-cypher-tool",
			args:     map[string]any{"cypher": "CREATE (:WriteTest {x: 1})"},
			prepareData: func(t *testing.T) {
				client := newFalkorDBClient(t)
				t.Cleanup(func() {
					_, _ = client.SelectGraph(FalkorDBGraph).Query("MATCH (n:WriteTest) DELETE n", nil, nil)
				})
			},
			validateFunc: func(t *testing.T, body string) {
				var result map[string]any
				if err := json.Unmarshal([]byte(body), &result); err != nil {
					t.Fatalf("failed to unmarshal write result: %v", err)
				}
				stats, ok := result["stats"].(map[string]any)
				if !ok {
					t.Fatalf("expected 'stats' in write response, got: %s", body)
				}
				if nodesCreated, ok := stats["nodesCreated"].(float64); !ok || nodesCreated != 1 {
					t.Errorf("expected stats.nodesCreated == 1, got: %v", stats["nodesCreated"])
				}
			},
		},
		{
			name:     "invoke my-simple-execute-cypher-tool with dry_run",
			toolName: "my-simple-execute-cypher-tool",
			args:     map[string]any{"cypher": "MATCH (n) RETURN n", "dry_run": true},
			validateFunc: func(t *testing.T, body string) {
				var result map[string]any
				if err := json.Unmarshal([]byte(body), &result); err != nil {
					t.Fatalf("failed to unmarshal dry_run result: %v", err)
				}
				plan, ok := result["plan"].([]any)
				if !ok || len(plan) == 0 {
					t.Fatalf("expected a non-empty query plan, got: %s", body)
				}
				if first, ok := plan[0].(string); !ok || !strings.Contains(first, "Results") {
					t.Errorf("expected plan to start with a Results operation, got: %v", plan[0])
				}
			},
		},
		{
			name:           "invoke readonly tool with write query",
			toolName:       "my-readonly-execute-cypher-tool",
			args:           map[string]any{"cypher": "CREATE (:ReadOnlyViolation)"},
			wantContentErr: "read-only",
		},
		{
			name:     "invoke graph override tool against another graph",
			toolName: "my-graph-override-execute-cypher-tool",
			args:     map[string]any{"cypher": "CREATE (:OverrideNode {x: 1}) RETURN 1 as ok", "graph": "toolbox_override_test"},
			prepareData: func(t *testing.T) {
				client := newFalkorDBClient(t)
				t.Cleanup(func() {
					_ = client.SelectGraph("toolbox_override_test").Delete()
				})
			},
			validateFunc: func(t *testing.T, body string) {
				if !strings.Contains(body, "\"ok\":1") {
					t.Errorf("expected create to succeed on override graph, got: %s", body)
				}
				client := newFalkorDBClient(t)
				result, err := client.SelectGraph("toolbox_override_test").ROQuery("MATCH (n:OverrideNode) RETURN count(n) as c", nil, nil)
				if err != nil {
					t.Fatalf("failed to verify override graph: %v", err)
				}
				if !result.Next() {
					t.Fatalf("no rows verifying override graph")
				}
				if c, _ := result.Record().Get("c"); fmt.Sprintf("%v", c) != "1" {
					t.Errorf("expected 1 node in override graph, got %v", c)
				}
			},
		},
		{
			name:     "invoke my-list-graphs-tool",
			toolName: "my-list-graphs-tool",
			args:     map[string]any{},
			prepareData: func(t *testing.T) {
				client := newFalkorDBClient(t)
				if _, err := client.SelectGraph(FalkorDBGraph).Query("RETURN 1", nil, nil); err != nil {
					t.Fatalf("failed to touch test graph: %v", err)
				}
			},
			validateFunc: func(t *testing.T, body string) {
				var result map[string]any
				if err := json.Unmarshal([]byte(body), &result); err != nil {
					t.Fatalf("failed to unmarshal list-graphs result: %v", err)
				}
				graphs, ok := result["graphs"].([]any)
				if !ok {
					t.Fatalf("expected 'graphs' in response, got: %s", body)
				}
				found := false
				for _, graph := range graphs {
					if graph == FalkorDBGraph {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected graph %q in list, got: %v", FalkorDBGraph, graphs)
				}
			},
		},
		{
			name:     "invoke schema tool on nonexistent graph",
			toolName: "my-empty-graph-schema-tool",
			args:     map[string]any{},
			validateFunc: func(t *testing.T, body string) {
				var schema map[string]any
				if err := json.Unmarshal([]byte(body), &schema); err != nil {
					t.Fatalf("failed to unmarshal schema json: %v\nResponse body: %s", err, body)
				}
				graphInfo, ok := schema["graphInfo"].(map[string]any)
				if !ok {
					t.Fatalf("expected graphInfo in empty schema response, got: %s", body)
				}
				if graphInfo["name"] != "toolbox_nonexistent_graph" {
					t.Errorf("expected graphInfo.name 'toolbox_nonexistent_graph', got %v", graphInfo["name"])
				}
				if nodeCount, ok := graphInfo["nodeCount"].(float64); !ok || nodeCount != 0 {
					t.Errorf("expected nodeCount 0 for empty graph, got %v", graphInfo["nodeCount"])
				}
			},
		},
		{
			name:     "invoke my-schema-tool with populated data",
			toolName: "my-schema-tool",
			args:     map[string]any{},
			prepareData: func(t *testing.T) {
				client := newFalkorDBClient(t)
				graph := client.SelectGraph(FalkorDBGraph)

				t.Cleanup(func() {
					_, _ = graph.Query("MATCH (n) DETACH DELETE n", nil, nil)
					_, _ = graph.Query("DROP INDEX ON :Movie(title)", nil, nil)
				})

				if _, err := graph.Query("MERGE (p:Person {name: 'Alice'}) MERGE (m:Movie {title: 'The Matrix'}) MERGE (p)-[:ACTED_IN {role: 'Neo'}]->(m)", nil, nil); err != nil {
					t.Fatalf("failed to seed data: %v", err)
				}
				if _, err := graph.Query("CREATE INDEX FOR (m:Movie) ON (m.title)", nil, nil); err != nil && !strings.Contains(err.Error(), "already indexed") {
					t.Fatalf("failed to create index: %v", err)
				}
			},
			validateFunc: func(t *testing.T, body string) {
				type Property struct {
					Name  string   `json:"name"`
					Types []string `json:"types"`
				}
				type NodeLabel struct {
					Name       string     `json:"name"`
					Count      int64      `json:"count"`
					Properties []Property `json:"properties"`
				}
				type Relationship struct {
					Type      string `json:"type"`
					StartNode string `json:"startNode"`
					EndNode   string `json:"endNode"`
				}
				type Schema struct {
					GraphInfo     map[string]any   `json:"graphInfo"`
					NodeLabels    []NodeLabel      `json:"nodeLabels"`
					Relationships []Relationship   `json:"relationships"`
					Indexes       []map[string]any `json:"indexes"`
					Statistics    map[string]any   `json:"statistics"`
				}

				var schema Schema
				if err := json.Unmarshal([]byte(body), &schema); err != nil {
					t.Fatalf("failed to unmarshal schema json: %v\nResponse body: %s", err, body)
				}

				if schema.GraphInfo["name"] != FalkorDBGraph {
					t.Errorf("expected graphInfo.name %q, got %v", FalkorDBGraph, schema.GraphInfo["name"])
				}

				var personFound, movieFound bool
				for _, label := range schema.NodeLabels {
					if label.Name == "Person" {
						personFound = true
						propFound := false
						for _, property := range label.Properties {
							if property.Name == "name" {
								propFound = true
								break
							}
						}
						if !propFound {
							t.Errorf("expected Person label to have 'name' property")
						}
					}
					if label.Name == "Movie" {
						movieFound = true
					}
				}
				if !personFound {
					t.Error("expected to find 'Person' in nodeLabels")
				}
				if !movieFound {
					t.Error("expected to find 'Movie' in nodeLabels")
				}

				relFound := false
				for _, relationship := range schema.Relationships {
					if relationship.Type == "ACTED_IN" && relationship.StartNode == "Person" && relationship.EndNode == "Movie" {
						relFound = true
						break
					}
				}
				if !relFound {
					t.Errorf("expected to find relationship '(:Person)-[:ACTED_IN]->(:Movie)', got: %v", schema.Relationships)
				}

				if len(schema.Indexes) == 0 {
					t.Error("expected at least one index in schema response")
				}
			},
		},
	}
}
