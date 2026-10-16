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

package arcadedb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	ArcadeDBSourceType = "arcadedb"
	ArcadeDBDatabase   = os.Getenv("ARCADEDB_DATABASE")
	ArcadeDBURI        = os.Getenv("ARCADEDB_URI")
	ArcadeDBUser       = os.Getenv("ARCADEDB_USER")
	ArcadeDBPass       = os.Getenv("ARCADEDB_PASS")
	// Optional override; when unset the test derives the HTTP base URL from
	// ArcadeDBURI by replacing the Bolt port with the default HTTP port 2480.
	ArcadeDBHTTPURL = os.Getenv("ARCADEDB_HTTP_URL")
)

// integrationTestVertexType is the throwaway vertex type seeded and torn down
// by the test. Kept unique enough to avoid colliding with anything a developer
// might have in a shared test database.
const integrationTestVertexType = "ArcadeDBIntegrationTestPerson"

func getArcadeDBVars(t *testing.T) map[string]any {
	switch "" {
	case ArcadeDBDatabase:
		t.Fatal("'ARCADEDB_DATABASE' not set")
	case ArcadeDBURI:
		t.Fatal("'ARCADEDB_URI' not set")
	case ArcadeDBUser:
		t.Fatal("'ARCADEDB_USER' not set")
	case ArcadeDBPass:
		t.Fatal("'ARCADEDB_PASS' not set")
	}

	vars := map[string]any{
		"type":     ArcadeDBSourceType,
		"uri":      ArcadeDBURI,
		"database": ArcadeDBDatabase,
		"user":     ArcadeDBUser,
		"password": ArcadeDBPass,
	}
	if ArcadeDBHTTPURL != "" {
		vars["httpUri"] = ArcadeDBHTTPURL
	}
	return vars
}

// arcadeHTTPBase returns the HTTP base URL for the configured ArcadeDB
// instance. Tests use this to seed and tear down data without going through
// the toolbox tools they are validating.
func arcadeHTTPBase(t *testing.T) string {
	if ArcadeDBHTTPURL != "" {
		return strings.TrimRight(ArcadeDBHTTPURL, "/")
	}
	parsed, err := url.Parse(ArcadeDBURI)
	if err != nil {
		t.Fatalf("failed to parse ARCADEDB_URI %q: %v", ArcadeDBURI, err)
	}
	host := parsed.Hostname()
	if host == "" {
		t.Fatalf("failed to extract host from ARCADEDB_URI %q", ArcadeDBURI)
	}
	return fmt.Sprintf("http://%s:2480", host)
}

// arcadeExecSQL POSTs a SQL command to ArcadeDB's HTTP API. Used for setup
// and teardown so the data path does not depend on the tools under test.
func arcadeExecSQL(t *testing.T, command string) {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"language": "sql",
		"command":  command,
	})
	if err != nil {
		t.Fatalf("failed to marshal SQL command: %v", err)
	}

	endpoint := fmt.Sprintf("%s/api/v1/command/%s", arcadeHTTPBase(t), url.PathEscape(ArcadeDBDatabase))
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to build SQL request: %v", err)
	}
	req.SetBasicAuth(ArcadeDBUser, ArcadeDBPass)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SQL request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("SQL request %q returned status %d: %s", command, resp.StatusCode, string(body))
	}
}

// seedFixtures creates a vertex type and a handful of rows the invoke tests
// read against. Idempotent so repeated local runs do not need a clean DB.
func seedFixtures(t *testing.T) {
	arcadeExecSQL(t, fmt.Sprintf("CREATE VERTEX TYPE %s IF NOT EXISTS", integrationTestVertexType))
	arcadeExecSQL(t, fmt.Sprintf("DELETE FROM %s", integrationTestVertexType))
	arcadeExecSQL(t, fmt.Sprintf("INSERT INTO %s SET name = 'Alice', age = 30", integrationTestVertexType))
	arcadeExecSQL(t, fmt.Sprintf("INSERT INTO %s SET name = 'Bob', age = 25", integrationTestVertexType))
}

func teardownFixtures(t *testing.T) {
	arcadeExecSQL(t, fmt.Sprintf("DROP TYPE %s IF EXISTS UNSAFE", integrationTestVertexType))
}

// setupArcadeDBInstance points the suite at an ArcadeDB instance. It defaults
// to an ephemeral container; the ARCADEDB_* environment variables take
// precedence when pointing the suite at an external instance. The
// package-level connection variables are restored on cleanup so each test gets
// its own container.
func setupArcadeDBInstance(ctx context.Context, t *testing.T) {
	t.Helper()
	origURI, origHTTPURL, origUser, origPass, origDatabase := ArcadeDBURI, ArcadeDBHTTPURL, ArcadeDBUser, ArcadeDBPass, ArcadeDBDatabase
	t.Cleanup(func() {
		ArcadeDBURI, ArcadeDBHTTPURL, ArcadeDBUser, ArcadeDBPass, ArcadeDBDatabase = origURI, origHTTPURL, origUser, origPass, origDatabase
	})

	if ArcadeDBURI != "" {
		return
	}
	boltURI, httpURL, cleanupFn := setupArcadeDBContainer(ctx, t)
	t.Cleanup(cleanupFn)
	ArcadeDBURI = boltURI
	ArcadeDBHTTPURL = httpURL
	ArcadeDBUser = "root"
	ArcadeDBPass = "playwithdata"
	ArcadeDBDatabase = "test_database"

	createDatabase(t, httpURL, ArcadeDBDatabase)
}

// getArcadeDBToolsConfig returns the tools file shared by the ArcadeDB
// integration tests.
func getArcadeDBToolsConfig(t *testing.T) map[string]any {
	t.Helper()
	sourceConfig := getArcadeDBVars(t)

	return map[string]any{
		"sources": map[string]any{
			"my-arcadedb": sourceConfig,
		},
		"tools": map[string]any{
			"my-execute-cypher": map[string]any{
				"type":        "arcadedb-execute-cypher",
				"source":      "my-arcadedb",
				"description": "Execute Cypher against ArcadeDB.",
			},
			"my-readonly-cypher": map[string]any{
				"type":        "arcadedb-execute-cypher",
				"source":      "my-arcadedb",
				"description": "Read-only Cypher against ArcadeDB.",
				"readOnly":    true,
			},
			"my-execute-sql": map[string]any{
				"type":        "arcadedb-execute-sql",
				"source":      "my-arcadedb",
				"description": "Execute SQL against ArcadeDB.",
			},
			"my-readonly-sql": map[string]any{
				"type":        "arcadedb-execute-sql",
				"source":      "my-arcadedb",
				"description": "Read-only SQL against ArcadeDB.",
				"readOnly":    true,
			},
		},
	}
}

// TestArcadeDBToolEndpoints spins up a toolbox server backed by a live
// ArcadeDB instance and exercises the manifest and invoke surfaces of the
// Cypher and SQL execution tools, including readOnly enforcement, dry_run
// plan output, and parameter binding.
func TestArcadeDBToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	setupArcadeDBInstance(ctx, t)

	args := []string{"--enable-api"}
	toolsFile := getArcadeDBToolsConfig(t)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)

	waitCtx, cancelWait := context.WithTimeout(ctx, 30*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	seedFixtures(t)
	t.Cleanup(func() { teardownFixtures(t) })

	// Manifest tests: assert each tool exposes the expected parameter surface.
	manifestTcs := []struct {
		name string
		api  string
		want map[string]any
	}{
		{
			name: "get my-execute-cypher",
			api:  "http://127.0.0.1:5000/api/tool/my-execute-cypher/",
			want: map[string]any{
				"my-execute-cypher": map[string]any{
					"description": "Execute Cypher against ArcadeDB.",
					"parameters": []any{
						map[string]any{
							"name":         "cypher",
							"type":         "string",
							"required":     true,
							"description":  "The cypher to execute.",
							"authServices": []any{},
						},
						map[string]any{
							"name":                 "params",
							"type":                 "object",
							"required":             false,
							"description":          "Optional query parameters to use with the cypher statement.",
							"default":              map[string]any{},
							"authServices":         []any{},
							"additionalProperties": true,
						},
						map[string]any{
							"name":         "dry_run",
							"type":         "boolean",
							"required":     false,
							"description":  "If set to true, the query will be validated and information about the execution will be returned without running the query. Defaults to false.",
							"default":      false,
							"authServices": []any{},
						},
					},
					"authRequired": []any{},
				},
			},
		},
		{
			name: "get my-execute-sql",
			api:  "http://127.0.0.1:5000/api/tool/my-execute-sql/",
			want: map[string]any{
				"my-execute-sql": map[string]any{
					"description": "Execute SQL against ArcadeDB.",
					"parameters": []any{
						map[string]any{
							"name":         "sql",
							"type":         "string",
							"required":     true,
							"description":  "The SQL statement to execute.",
							"authServices": []any{},
						},
						map[string]any{
							"name":                 "params",
							"type":                 "object",
							"required":             false,
							"description":          "Optional query parameters to use with the SQL statement.",
							"default":              map[string]any{},
							"authServices":         []any{},
							"additionalProperties": true,
						},
						map[string]any{
							"name":         "dry_run",
							"type":         "boolean",
							"required":     false,
							"description":  "If set to true, the SQL will be validated and execution plan metadata will be returned without running it. Defaults to false.",
							"default":      false,
							"authServices": []any{},
						},
					},
					"authRequired": []any{},
				},
			},
		},
	}
	for _, tc := range manifestTcs {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(tc.api)
			if err != nil {
				t.Fatalf("error when sending a request: %s", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("response status code is not 200: got %d", resp.StatusCode)
			}

			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("error parsing response body: %v", err)
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

	// Invoke tests: end-to-end through the toolbox against the live ArcadeDB.
	for _, tc := range getArcadeDBInvokeTestCases() {
		t.Run(tc.name, func(t *testing.T) {
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
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}
			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body: %v", body)
			}
			if tc.validateFunc != nil {
				tc.validateFunc(t, got)
			}
		})
	}
}

// arcadeDBInvokeTestCase describes one tool invocation shared by the REST and
// MCP ArcadeDB integration tests. wantErr marks invocations the tool is
// expected to reject; the MCP test asserts these return an error result.
type arcadeDBInvokeTestCase struct {
	name         string
	toolName     string
	args         map[string]any
	wantErr      bool
	validateFunc func(t *testing.T, body string)
}

// getArcadeDBInvokeTestCases returns the tool invocations exercised by both the
// REST and MCP ArcadeDB integration tests.
func getArcadeDBInvokeTestCases() []arcadeDBInvokeTestCase {
	return []arcadeDBInvokeTestCase{
		{
			name:     "cypher: simple read",
			toolName: "my-execute-cypher",
			args:     map[string]any{"cypher": "RETURN 1 AS a"},
			validateFunc: func(t *testing.T, body string) {
				if !strings.Contains(body, `"a":1`) {
					t.Errorf("expected result to contain \"a\":1, got %s", body)
				}
			},
		},
		{
			name:     "cypher: read from seeded vertex type",
			toolName: "my-execute-cypher",
			args:     map[string]any{"cypher": fmt.Sprintf("MATCH (n:%s) RETURN count(n) AS c", integrationTestVertexType)},
			validateFunc: func(t *testing.T, body string) {
				if !strings.Contains(body, `"c":2`) {
					t.Errorf("expected count 2 for seeded fixture, got %s", body)
				}
			},
		},
		{
			name:     "cypher: parameterized read",
			toolName: "my-execute-cypher",
			args:     map[string]any{"cypher": fmt.Sprintf("MATCH (n:%s {name: $name}) RETURN n.age AS age", integrationTestVertexType), "params": map[string]any{"name": "Alice"}},
			validateFunc: func(t *testing.T, body string) {
				if !strings.Contains(body, `"age":30`) {
					t.Errorf("expected age 30 for Alice, got %s", body)
				}
			},
		},
		{
			name:     "cypher: dry_run returns a plan",
			toolName: "my-execute-cypher",
			args:     map[string]any{"cypher": fmt.Sprintf("MATCH (n:%s) RETURN n", integrationTestVertexType), "dry_run": true},
			validateFunc: func(t *testing.T, body string) {
				var result []map[string]any
				if err := json.Unmarshal([]byte(body), &result); err != nil {
					t.Fatalf("failed to unmarshal dry_run result: %v\nbody: %s", err, body)
				}
				if len(result) == 0 {
					t.Fatalf("expected a query plan, got empty result: %s", body)
				}
				if _, ok := result[0]["operator"]; !ok {
					t.Errorf("expected key 'operator' in dry_run response, got: %s", body)
				}
			},
		},
		{
			name:     "cypher: readOnly rejects write",
			toolName: "my-readonly-cypher",
			args:     map[string]any{"cypher": fmt.Sprintf("CREATE (n:%s {name: 'Mallory'})", integrationTestVertexType)},
			wantErr:  true,
			validateFunc: func(t *testing.T, body string) {
				if !strings.Contains(strings.ToLower(body), "read-only") {
					t.Errorf("expected read-only rejection, got %s", body)
				}
			},
		},
		{
			name:     "sql: simple read",
			toolName: "my-execute-sql",
			args:     map[string]any{"sql": "SELECT 1 AS a"},
			validateFunc: func(t *testing.T, body string) {
				if !strings.Contains(body, `"a":1`) {
					t.Errorf("expected result to contain \"a\":1, got %s", body)
				}
			},
		},
		{
			name:     "sql: read from seeded vertex type",
			toolName: "my-execute-sql",
			args:     map[string]any{"sql": fmt.Sprintf("SELECT count(*) AS c FROM %s", integrationTestVertexType)},
			validateFunc: func(t *testing.T, body string) {
				if !strings.Contains(body, `"c":2`) {
					t.Errorf("expected count 2 for seeded fixture, got %s", body)
				}
			},
		},
		{
			name:     "sql: parameterized read",
			toolName: "my-execute-sql",
			args:     map[string]any{"sql": fmt.Sprintf("SELECT age FROM %s WHERE name = :name", integrationTestVertexType), "params": map[string]any{"name": "Bob"}},
			validateFunc: func(t *testing.T, body string) {
				if !strings.Contains(body, `"age":25`) {
					t.Errorf("expected age 25 for Bob, got %s", body)
				}
			},
		},
		{
			name:     "sql: dry_run returns a plan",
			toolName: "my-execute-sql",
			args:     map[string]any{"sql": fmt.Sprintf("SELECT * FROM %s", integrationTestVertexType), "dry_run": true},
			validateFunc: func(t *testing.T, body string) {
				if !strings.Contains(body, "executionPlan") && !strings.Contains(body, "executionPlanAsString") {
					t.Errorf("expected an execution plan in dry_run response, got %s", body)
				}
			},
		},
		{
			name:     "sql: readOnly rejects write",
			toolName: "my-readonly-sql",
			args:     map[string]any{"sql": fmt.Sprintf("INSERT INTO %s SET name = 'Mallory'", integrationTestVertexType)},
			wantErr:  true,
			validateFunc: func(t *testing.T, body string) {
				low := strings.ToLower(body)
				if !strings.Contains(low, "read-only") && !strings.Contains(low, "readonly") && !strings.Contains(low, "idempotent") {
					t.Errorf("expected read-only rejection on write, got %s", body)
				}
			},
		},
		{
			name:     "sql: readOnly rejects stacked write after read",
			toolName: "my-readonly-sql",
			args:     map[string]any{"sql": fmt.Sprintf("SELECT * FROM %s; DELETE FROM %s", integrationTestVertexType, integrationTestVertexType)},
			wantErr:  true,
			validateFunc: func(t *testing.T, body string) {
				low := strings.ToLower(body)
				if !strings.Contains(low, "read-only") && !strings.Contains(low, "readonly") && !strings.Contains(low, "idempotent") && !strings.Contains(low, "syntax error") && !strings.Contains(low, "mismatched input") {
					t.Errorf("expected stacked write to be rejected under readOnly, got %s", body)
				}
				// Sanity-check that the DELETE did not actually run.
				count := arcadeCountVertices(t, integrationTestVertexType)
				if count != 2 {
					t.Errorf("expected fixture count to remain 2 after rejected stacked statement, got %d", count)
				}
			},
		},
		{
			name:     "sql: write then read round-trip",
			toolName: "my-execute-sql",
			args:     map[string]any{"sql": fmt.Sprintf("INSERT INTO %s SET name = 'Carol', age = 40", integrationTestVertexType)},
			validateFunc: func(t *testing.T, body string) {
				if strings.Contains(strings.ToLower(body), "error") {
					t.Errorf("unexpected error on INSERT: %s", body)
				}
				count := arcadeCountVertices(t, integrationTestVertexType)
				if count != 3 {
					t.Errorf("expected fixture count to be 3 after INSERT, got %d", count)
				}
			},
		},
	}
}

// arcadeCountVertices issues a direct HTTP query against ArcadeDB to count
// rows in a vertex type. Used by invoke tests to confirm side effects (or
// lack thereof) independently of the toolbox response payload.
func arcadeCountVertices(t *testing.T, vertexType string) int {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"language": "sql",
		"command":  fmt.Sprintf("SELECT count(*) AS c FROM %s", vertexType),
	})
	if err != nil {
		t.Fatalf("failed to marshal count command: %v", err)
	}

	endpoint := fmt.Sprintf("%s/api/v1/command/%s", arcadeHTTPBase(t), url.PathEscape(ArcadeDBDatabase))
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to build count request: %v", err)
	}
	req.SetBasicAuth(ArcadeDBUser, ArcadeDBPass)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("count request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("count request returned status %d: %s", resp.StatusCode, string(body))
	}

	var decoded struct {
		Result []map[string]any `json:"result"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("failed to parse count response: %v\nbody: %s", err, string(body))
	}
	if len(decoded.Result) == 0 {
		t.Fatalf("empty count response: %s", string(body))
	}
	switch v := decoded.Result[0]["c"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		t.Fatalf("unexpected count type %T in response: %s", v, string(body))
	}
	return 0
}

func setupArcadeDBContainer(ctx context.Context, t *testing.T) (boltURI string, httpURL string, cleanup func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "arcadedata/arcadedb:26.7.1",
		ExposedPorts: []string{"2480/tcp", "7687/tcp"},
		Env: map[string]string{
			"JAVA_OPTS":                        "-Darcadedb.server.rootPassword=playwithdata -Darcadedb.server.plugins=Bolt:com.arcadedb.bolt.BoltProtocolPlugin",
			"arcadedb.server.defaultDatabases": "",
		},
		WaitingFor: wait.ForHTTP("/api/v1/ready").
			WithPort("2480/tcp").
			WithStatusCodeMatcher(func(status int) bool {
				return status == http.StatusOK || status == http.StatusNoContent
			}).
			WithStartupTimeout(180 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start ArcadeDB container: %s", err)
	}

	cleanupFn := func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	}

	host, err := container.Host(ctx)
	if err != nil {
		cleanupFn()
		t.Fatalf("failed to get container host: %s", err)
	}

	port2480, err := container.MappedPort(ctx, "2480")
	if err != nil {
		cleanupFn()
		t.Fatalf("failed to get container mapped port 2480: %s", err)
	}

	port7687, err := container.MappedPort(ctx, "7687")
	if err != nil {
		cleanupFn()
		t.Fatalf("failed to get container mapped port 7687: %s", err)
	}

	boltURI = fmt.Sprintf("bolt://%s:%s", host, port7687.Port())
	httpURL = fmt.Sprintf("http://%s:%s", host, port2480.Port())

	return boltURI, httpURL, cleanupFn
}

func createDatabase(t *testing.T, httpURL, dbName string) {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"command": fmt.Sprintf("create database %s", dbName),
	})
	if err != nil {
		t.Fatalf("failed to marshal create database command: %v", err)
	}

	endpoint := fmt.Sprintf("%s/api/v1/server", httpURL)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to build create database request: %v", err)
	}
	req.SetBasicAuth("root", "playwithdata")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("create database request failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("create database request returned status %d: %s", resp.StatusCode, string(body))
	}
}
