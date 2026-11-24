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
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	falkordb "github.com/FalkorDB/falkordb-go/v2"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// falkorDBImage is the image used for the ephemeral test container.
const falkorDBImage = "falkordb/falkordb:v4.18.0"

// testGraphPrefix starts every temporary FalkorDB graph name created by these
// tests, followed by its Unix creation timestamp so the TestMain sweeper can
// reclaim leaked graphs on shared external instances.
const testGraphPrefix = "toolbox_it_"

// orphanedGraphTTL is the minimum age before TestMain sweeps a leaked test
// graph on an external FalkorDB instance. Test timeouts in this package are 5m,
// so any test graph older than 3h cannot belong to an active run.
const orphanedGraphTTL = 3 * time.Hour

// falkorCleanupTimeout bounds each per-test cleanup RPC so teardown cannot hang
// indefinitely if the server becomes unresponsive.
const falkorCleanupTimeout = 30 * time.Second

var validCypherIdentifierRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var (
	FalkorDBSourceType = "falkordb"
	FalkorDBHost       = os.Getenv("FALKORDB_HOST")
	FalkorDBPort       = os.Getenv("FALKORDB_PORT")
	FalkorDBUsername   = os.Getenv("FALKORDB_USERNAME")
	FalkorDBPassword   = os.Getenv("FALKORDB_PASSWORD")
	FalkorDBGraph      = os.Getenv("FALKORDB_GRAPH")
)

func TestMain(m *testing.M) {
	if FalkorDBHost != "" && FalkorDBPort != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		sweepOrphanedFalkorDBGraphs(ctx)
		cancel()
	}
	os.Exit(m.Run())
}

func newFalkorDBUniqueID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
}

func newTestGraphName(kind, uniqueID string, now time.Time) string {
	return fmt.Sprintf("%s%d_%s_%s", testGraphPrefix, now.UTC().Unix(), kind, uniqueID)
}

func parseTestGraphCreatedAt(graphName string) (time.Time, bool) {
	rest, ok := strings.CutPrefix(graphName, testGraphPrefix)
	if !ok {
		return time.Time{}, false
	}
	tsStr, _, ok := strings.Cut(rest, "_")
	if !ok || tsStr == "" {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}, false
	}
	return time.Unix(sec, 0).UTC(), true
}

func shouldSweepFalkorDBGraph(graphName string, now time.Time) bool {
	createdAt, ok := parseTestGraphCreatedAt(graphName)
	if !ok {
		return false
	}
	return now.Sub(createdAt) > orphanedGraphTTL
}

func isFalkorDBNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "empty key") ||
		strings.Contains(msg, "no such index") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "doesn't exist") ||
		strings.Contains(msg, "not found")
}

func mustValidCypherIdentifier(t *testing.T, id string) string {
	t.Helper()
	if !validCypherIdentifierRe.MatchString(id) {
		t.Fatalf("invalid Cypher identifier %q", id)
	}
	return id
}

// execFalkorDBCleanupBatch executes one or more Redis/FalkorDB cleanup commands
// under ctx, pipelining multi-command batches into a single round-trip while
// ignoring NotFound/empty-key responses and aggregating real errors.
func execFalkorDBCleanupBatch(ctx context.Context, client *falkordb.FalkorDB, rawCmds ...[]any) error {
	if len(rawCmds) == 0 {
		return nil
	}
	if len(rawCmds) == 1 {
		err := client.Conn.Do(ctx, rawCmds[0]...).Err()
		if err != nil && !isFalkorDBNotFound(err) {
			return fmt.Errorf("falkordb cleanup %v failed: %w", rawCmds[0], err)
		}
		return nil
	}

	pipe := client.Conn.Pipeline()
	for _, args := range rawCmds {
		pipe.Do(ctx, args...)
	}
	cmds, execErr := pipe.Exec(ctx)
	var errs []error
	for i, cmd := range cmds {
		if err := cmd.Err(); err != nil && !isFalkorDBNotFound(err) {
			errs = append(errs, fmt.Errorf("falkordb cleanup %v failed: %w", rawCmds[i], err))
		}
	}
	if len(errs) == 0 && execErr != nil && !isFalkorDBNotFound(execErr) {
		return fmt.Errorf("falkordb cleanup pipeline failed: %w", execErr)
	}
	return errors.Join(errs...)
}

// sweepOrphanedFalkorDBGraphs deletes temporary test graphs older than
// orphanedGraphTTL on shared external FalkorDB instances. Failures are logged
// without blocking the test run.
func sweepOrphanedFalkorDBGraphs(ctx context.Context) {
	client, err := initFalkorDBConnection()
	if err != nil {
		log.Printf("INTEGRATION CLEANUP: failed to create FalkorDB client: %v", err)
		return
	}
	defer func() { _ = client.Conn.Close() }()

	sweepOrphanedFalkorDBGraphsWithClient(ctx, client, time.Now())
}

func sweepOrphanedFalkorDBGraphsWithClient(ctx context.Context, client *falkordb.FalkorDB, now time.Time) {
	graphs, err := client.Conn.Do(ctx, "GRAPH.LIST").StringSlice()
	if err != nil {
		log.Printf("INTEGRATION CLEANUP: failed to list FalkorDB graphs: %v", err)
		return
	}
	var deleteCmds [][]any
	for _, graphName := range graphs {
		createdAt, ok := parseTestGraphCreatedAt(graphName)
		if !ok || !shouldSweepFalkorDBGraph(graphName, now) {
			continue
		}
		log.Printf("INTEGRATION CLEANUP: deleting orphaned FalkorDB graph %q created %s", graphName, createdAt.Format(time.RFC3339))
		deleteCmds = append(deleteCmds, []any{"GRAPH.DELETE", graphName})
	}
	if err := execFalkorDBCleanupBatch(ctx, client, deleteCmds...); err != nil {
		log.Printf("INTEGRATION CLEANUP: %v", err)
	}
}

func registerFalkorDBCleanup(t *testing.T, client *falkordb.FalkorDB, rawCmds ...[]any) {
	t.Helper()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), falkorCleanupTimeout)
		defer cancel()
		if err := execFalkorDBCleanupBatch(cleanupCtx, client, rawCmds...); err != nil {
			t.Errorf("%v", err)
		}
	})
}

func registerFalkorDBGraphCleanup(t *testing.T, client *falkordb.FalkorDB, graphName string) {
	t.Helper()
	registerFalkorDBCleanup(t, client, []any{"GRAPH.DELETE", graphName})
}

func registerFalkorDBLabelCleanup(t *testing.T, client *falkordb.FalkorDB, graphName string, labels ...string) {
	t.Helper()
	cmds := make([][]any, 0, len(labels))
	for _, label := range labels {
		mustValidCypherIdentifier(t, label)
		cypher := fmt.Sprintf("MATCH (n:%s) DETACH DELETE n", label)
		cmds = append(cmds, []any{"GRAPH.QUERY", graphName, cypher, "--compact"})
	}
	registerFalkorDBCleanup(t, client, cmds...)
}

func registerFalkorDBSchemaCleanup(t *testing.T, client *falkordb.FalkorDB, graphName, indexLabel, indexProperty string, nodeLabels ...string) {
	t.Helper()
	mustValidCypherIdentifier(t, indexLabel)
	mustValidCypherIdentifier(t, indexProperty)
	cmds := make([][]any, 0, 1+len(nodeLabels))
	dropIndexCypher := fmt.Sprintf("DROP INDEX FOR (n:%s) ON (n.%s)", indexLabel, indexProperty)
	cmds = append(cmds, []any{"GRAPH.QUERY", graphName, dropIndexCypher, "--compact"})
	for _, label := range nodeLabels {
		mustValidCypherIdentifier(t, label)
		deleteCypher := fmt.Sprintf("MATCH (n:%s) DETACH DELETE n", label)
		cmds = append(cmds, []any{"GRAPH.QUERY", graphName, deleteCypher, "--compact"})
	}
	registerFalkorDBCleanup(t, client, cmds...)
}

// setupFalkorDBContainer starts an ephemeral FalkorDB container, schedules its
// termination via t.Cleanup, and returns its host and mapped port.
func setupFalkorDBContainer(ctx context.Context, t *testing.T) (string, string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        falkorDBImage,
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("6379/tcp").WithStartupTimeout(2*time.Minute),
			wait.ForLog("Ready to accept connections").WithStartupTimeout(2*time.Minute),
		).WithDeadline(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Logger:           tclog.TestLogger(t),
	})
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("failed to start FalkorDB container: %s", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %s", err)
	}

	port, err := container.MappedPort(ctx, "6379/tcp")
	if err != nil {
		t.Fatalf("failed to get container mapped port 6379/tcp: %s", err)
	}

	return host, port.Port()
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

func initFalkorDBConnection() (*falkordb.FalkorDB, error) {
	return falkordb.FalkorDBNew(&falkordb.ConnectionOption{
		Addr:     net.JoinHostPort(FalkorDBHost, FalkorDBPort),
		Username: FalkorDBUsername,
		Password: FalkorDBPassword,
	})
}

// newFalkorDBClient connects directly to the instance for test setup and
// teardown, registering connection closure via t.Cleanup so it closes after any
// subsequently registered resource cleanups (t.Cleanup is LIFO).
func newFalkorDBClient(t *testing.T) *falkordb.FalkorDB {
	t.Helper()
	client, err := initFalkorDBConnection()
	if err != nil {
		t.Fatalf("failed to create falkordb client: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Conn.Close(); err != nil {
			t.Errorf("failed to close falkordb client: %v", err)
		}
	})
	return client
}

// setupFalkorDBInstance points the suite at a FalkorDB instance. It defaults to
// an ephemeral container; the FALKORDB_* environment variables take precedence
// when pointing the suite at an external instance. The package-level
// connection variables are restored on cleanup so each test gets its own
// container.
func setupFalkorDBInstance(ctx context.Context, t *testing.T, uniqueID string) {
	t.Helper()
	origHost, origPort, origGraph := FalkorDBHost, FalkorDBPort, FalkorDBGraph
	t.Cleanup(func() {
		FalkorDBHost, FalkorDBPort, FalkorDBGraph = origHost, origPort, origGraph
	})

	if FalkorDBHost == "" {
		host, port := setupFalkorDBContainer(ctx, t)
		FalkorDBHost, FalkorDBPort = host, port
		FalkorDBGraph = newTestGraphName("default", uniqueID, time.Now())
	}
}

// getFalkorDBToolsConfig returns the tools file shared by the FalkorDB
// integration tests.
func getFalkorDBToolsConfig(t *testing.T, emptyGraphName string) map[string]any {
	t.Helper()
	sourceConfig := getFalkorDBVars(t)

	emptyGraphSourceConfig := getFalkorDBVars(t)
	emptyGraphSourceConfig["graph"] = emptyGraphName

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
	t.Cleanup(cancel)

	uniqueID := newFalkorDBUniqueID()
	setupFalkorDBInstance(ctx, t, uniqueID)
	client := newFalkorDBClient(t)
	emptyGraphName := newTestGraphName("empty", uniqueID, time.Now())

	args := []string{"--enable-api"}
	toolsFile := getFalkorDBToolsConfig(t, emptyGraphName)

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
	for _, tc := range getFalkorDBInvokeTestCases(ctx, client, uniqueID, emptyGraphName) {
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
// REST and MCP FalkorDB integration tests, reusing a single shared client pool.
func getFalkorDBInvokeTestCases(ctx context.Context, client *falkordb.FalkorDB, uniqueID, emptyGraphName string) []falkorDBInvokeTestCase {
	writeLabel := "WriteTest_" + uniqueID
	readOnlyLabel := "ReadOnlyViolation_" + uniqueID
	overrideGraph := newTestGraphName("override", uniqueID, time.Now())
	personLabel := "Person_" + uniqueID
	movieLabel := "Movie_" + uniqueID
	actedInRel := "ACTED_IN_" + uniqueID

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
			args:     map[string]any{"cypher": fmt.Sprintf("CREATE (:%s {x: 1})", writeLabel)},
			prepareData: func(t *testing.T) {
				registerFalkorDBLabelCleanup(t, client, FalkorDBGraph, writeLabel)
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
			args:           map[string]any{"cypher": fmt.Sprintf("CREATE (:%s)", readOnlyLabel)},
			wantContentErr: "read-only",
			prepareData: func(t *testing.T) {
				registerFalkorDBLabelCleanup(t, client, FalkorDBGraph, readOnlyLabel)
			},
		},
		{
			name:     "invoke graph override tool against another graph",
			toolName: "my-graph-override-execute-cypher-tool",
			args:     map[string]any{"cypher": "CREATE (:OverrideNode {x: 1}) RETURN 1 as ok", "graph": overrideGraph},
			prepareData: func(t *testing.T) {
				registerFalkorDBGraphCleanup(t, client, overrideGraph)
			},
			validateFunc: func(t *testing.T, body string) {
				if !strings.Contains(body, "\"ok\":1") {
					t.Errorf("expected create to succeed on override graph, got: %s", body)
				}
				result, err := client.SelectGraph(overrideGraph).ROQuery("MATCH (n:OverrideNode) RETURN count(n) as c", nil, nil)
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
				if err := client.Conn.Do(ctx, "GRAPH.QUERY", FalkorDBGraph, "RETURN 1", "--compact").Err(); err != nil {
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
			prepareData: func(t *testing.T) {
				registerFalkorDBGraphCleanup(t, client, emptyGraphName)
			},
			validateFunc: func(t *testing.T, body string) {
				var schema map[string]any
				if err := json.Unmarshal([]byte(body), &schema); err != nil {
					t.Fatalf("failed to unmarshal schema json: %v\nResponse body: %s", err, body)
				}
				graphInfo, ok := schema["graphInfo"].(map[string]any)
				if !ok {
					t.Fatalf("expected graphInfo in empty schema response, got: %s", body)
				}
				if graphInfo["name"] != emptyGraphName {
					t.Errorf("expected graphInfo.name %q, got %v", emptyGraphName, graphInfo["name"])
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
				registerFalkorDBSchemaCleanup(t, client, FalkorDBGraph, movieLabel, "title", personLabel, movieLabel)

				seedCypher := fmt.Sprintf(
					"MERGE (p:%s {name: 'Alice'}) MERGE (m:%s {title: 'The Matrix'}) MERGE (p)-[:%s {role: 'Neo'}]->(m)",
					personLabel, movieLabel, actedInRel,
				)
				indexCypher := fmt.Sprintf("CREATE INDEX FOR (m:%s) ON (m.title)", movieLabel)

				pipe := client.Conn.Pipeline()
				seedCmd := pipe.Do(ctx, "GRAPH.QUERY", FalkorDBGraph, seedCypher, "--compact")
				indexCmd := pipe.Do(ctx, "GRAPH.QUERY", FalkorDBGraph, indexCypher, "--compact")
				_, _ = pipe.Exec(ctx)
				if err := seedCmd.Err(); err != nil {
					t.Fatalf("failed to seed data: %v", err)
				}
				if err := indexCmd.Err(); err != nil && !strings.Contains(err.Error(), "already indexed") {
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
					if label.Name == personLabel {
						personFound = true
						propFound := false
						for _, property := range label.Properties {
							if property.Name == "name" {
								propFound = true
								break
							}
						}
						if !propFound {
							t.Errorf("expected %s label to have 'name' property", personLabel)
						}
					}
					if label.Name == movieLabel {
						movieFound = true
					}
				}
				if !personFound {
					t.Errorf("expected to find %q in nodeLabels", personLabel)
				}
				if !movieFound {
					t.Errorf("expected to find %q in nodeLabels", movieLabel)
				}

				relFound := false
				for _, relationship := range schema.Relationships {
					if relationship.Type == actedInRel && relationship.StartNode == personLabel && relationship.EndNode == movieLabel {
						relFound = true
						break
					}
				}
				if !relFound {
					t.Errorf("expected to find relationship '(:%s)-[:%s]->(:%s)', got: %v", personLabel, actedInRel, movieLabel, schema.Relationships)
				}

				if len(schema.Indexes) == 0 {
					t.Error("expected at least one index in schema response")
				}
			},
		},
	}
}
