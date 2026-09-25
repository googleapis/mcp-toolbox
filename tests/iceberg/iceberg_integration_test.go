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

package iceberg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/catalog/rest"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	IcebergSourceType = "iceberg"
	// The REST catalog fixture image the apache/iceberg-go project tests
	// against. With no warehouse configured it defaults to a temp directory
	// inside the container, which is all the metadata-only tools need.
	IcebergRestFixtureImage = "apache/iceberg-rest-fixture:1.10.1"
)

const (
	testNamespace = "toolbox_test_ns"
	testTable     = "test_table"
)

// setupIcebergRestCatalogContainer starts the Iceberg REST catalog fixture and
// returns its base URI.
func setupIcebergRestCatalogContainer(ctx context.Context, t *testing.T) (string, func()) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        IcebergRestFixtureImage,
		ExposedPorts: []string{"8181/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForHTTP("/v1/config"),
			wait.ForExposedPort(),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start iceberg rest catalog container: %s", err)
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

	mappedPort, err := container.MappedPort(ctx, "8181")
	if err != nil {
		cleanup()
		t.Fatalf("failed to get container mapped port: %s", err)
	}

	return fmt.Sprintf("http://%s:%s", host, mappedPort.Port()), cleanup
}

// seedCatalog creates a namespace and a table for the tools to read, going
// through the iceberg-go client directly so the data path does not depend on
// the tools under test.
func seedCatalog(ctx context.Context, t *testing.T, uri string) {
	t.Helper()

	cat, err := rest.NewCatalog(ctx, "seed", uri)
	if err != nil {
		t.Fatalf("failed to create seed catalog client: %s", err)
	}

	if err := cat.CreateNamespace(ctx, []string{testNamespace}, nil); err != nil {
		t.Fatalf("failed to create namespace: %s", err)
	}

	schema := iceberg.NewSchema(0,
		iceberg.NestedField{ID: 1, Name: "id", Type: iceberg.PrimitiveTypes.Int64, Required: true},
		iceberg.NestedField{ID: 2, Name: "name", Type: iceberg.PrimitiveTypes.String, Required: false},
	)
	if _, err := cat.CreateTable(ctx, []string{testNamespace, testTable}, schema); err != nil {
		t.Fatalf("failed to create table: %s", err)
	}
}

func getIcebergToolsConfig(uri string) map[string]any {
	return map[string]any{
		"sources": map[string]any{
			"my-iceberg-instance": map[string]any{
				"type": IcebergSourceType,
				"uri":  uri,
			},
		},
		"tools": map[string]any{
			"my-list-namespaces": map[string]any{
				"type":        "iceberg-list-namespaces",
				"source":      "my-iceberg-instance",
				"description": "Lists namespaces in the Iceberg catalog.",
			},
			"my-list-tables": map[string]any{
				"type":        "iceberg-list-tables",
				"source":      "my-iceberg-instance",
				"description": "Lists tables in a namespace.",
			},
			"my-get-table-info": map[string]any{
				"type":        "iceberg-get-table-info",
				"source":      "my-iceberg-instance",
				"description": "Gets a table's metadata.",
			},
		},
	}
}

// invokeTool POSTs to a tool's invoke endpoint and returns the response status
// code and the raw response body.
func invokeTool(t *testing.T, toolName string, params map[string]any) (int, []byte) {
	t.Helper()

	api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", toolName)
	bodyBytes, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal request body: %s", err)
	}
	resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer(bodyBytes), nil)
	return resp.StatusCode, respBody
}

// requireAgentError asserts an invoke response carries an in-band agent error:
// HTTP 200 with an error payload in the result instead of tool output.
func requireAgentError(t *testing.T, status int, respBody []byte) {
	t.Helper()

	if status != http.StatusOK {
		t.Fatalf("expected an agent error with status 200, got %d: %s", status, string(respBody))
	}
	var body map[string]any
	if err := json.Unmarshal(respBody, &body); err != nil {
		t.Fatalf("error parsing response body: %s", err)
	}
	result, ok := body["result"].(string)
	if !ok {
		t.Fatalf("unable to find result in response body: %s", string(respBody))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("error parsing result payload: %s", err)
	}
	if _, ok := payload["error"]; !ok {
		t.Fatalf("expected an error in the result payload, got: %s", string(respBody))
	}
}

// invokeToolResult invokes a tool, requires a 200, and returns the decoded
// "result" payload, which the server encodes as a JSON string.
func invokeToolResult(t *testing.T, toolName string, params map[string]any) string {
	t.Helper()

	status, respBody := invokeTool(t, toolName, params)
	if status != http.StatusOK {
		t.Fatalf("invoke of %q returned status %d: %s", toolName, status, string(respBody))
	}
	var body map[string]any
	if err := json.Unmarshal(respBody, &body); err != nil {
		t.Fatalf("error parsing response body: %s", err)
	}
	result, ok := body["result"].(string)
	if !ok {
		t.Fatalf("unable to find result in response body: %s", string(respBody))
	}
	return result
}

// TestIcebergToolEndpoints spins up a toolbox server backed by a live Iceberg
// REST catalog fixture and exercises the three catalog exploration tools.
func TestIcebergToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	uri, containerCleanup := setupIcebergRestCatalogContainer(ctx, t)
	t.Cleanup(containerCleanup)

	seedCatalog(ctx, t, uri)

	toolsFile := getIcebergToolsConfig(uri)
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, "--enable-api")
	if err != nil {
		t.Fatalf("failed to start cmd: %v", err)
	}
	t.Cleanup(cleanup)

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	t.Run("get my-list-tables manifest", func(t *testing.T) {
		resp, respBody := tests.RunRequest(t, http.MethodGet, "http://127.0.0.1:5000/api/tool/my-list-tables/", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("manifest request returned status %d: %s", resp.StatusCode, string(respBody))
		}
		var body map[string]any
		if err := json.Unmarshal(respBody, &body); err != nil {
			t.Fatalf("error parsing response body: %s", err)
		}
		toolsManifest, ok := body["tools"].(map[string]any)
		if !ok {
			t.Fatalf("unable to find tools in response body: %s", string(respBody))
		}
		manifest, ok := toolsManifest["my-list-tables"].(map[string]any)
		if !ok {
			t.Fatalf("unable to find my-list-tables in manifest: %s", string(respBody))
		}
		params, ok := manifest["parameters"].([]any)
		if !ok || len(params) != 1 {
			t.Fatalf("expected 1 parameter in manifest, got %v", manifest["parameters"])
		}
		param, ok := params[0].(map[string]any)
		if !ok || param["name"] != "namespace" {
			t.Fatalf("expected a namespace parameter, got %v", params[0])
		}
	})

	t.Run("invoke my-list-namespaces", func(t *testing.T) {
		got := invokeToolResult(t, "my-list-namespaces", map[string]any{})
		want := fmt.Sprintf(`["%s"]`, testNamespace)
		if got != want {
			t.Fatalf("unexpected value: got %q, want %q", got, want)
		}
	})

	t.Run("invoke my-list-tables", func(t *testing.T) {
		got := invokeToolResult(t, "my-list-tables", map[string]any{"namespace": testNamespace})
		want := fmt.Sprintf(`["%s"]`, testTable)
		if got != want {
			t.Fatalf("unexpected value: got %q, want %q", got, want)
		}
	})

	t.Run("invoke my-list-tables with missing namespace", func(t *testing.T) {
		status, respBody := invokeTool(t, "my-list-tables", map[string]any{"namespace": "no_such_namespace"})
		requireAgentError(t, status, respBody)
	})

	t.Run("invoke my-get-table-info", func(t *testing.T) {
		got := invokeToolResult(t, "my-get-table-info", map[string]any{
			"namespace": testNamespace,
			"table":     testTable,
		})

		var info map[string]any
		if err := json.Unmarshal([]byte(got), &info); err != nil {
			t.Fatalf("error parsing table info: %s", err)
		}

		wantTable := fmt.Sprintf("%s.%s", testNamespace, testTable)
		if info["table"] != wantTable {
			t.Fatalf("unexpected table: got %v, want %q", info["table"], wantTable)
		}
		if loc, _ := info["location"].(string); loc == "" {
			t.Fatalf("expected a non-empty location, got %v", info["location"])
		}
		if loc, _ := info["metadata-location"].(string); loc == "" {
			t.Fatalf("expected a non-empty metadata-location, got %v", info["metadata-location"])
		}

		// Assert the stable structural parts of the schema: the two seeded
		// field names in order.
		schema, ok := info["schema"].(map[string]any)
		if !ok {
			t.Fatalf("expected schema to be an object, got %v", info["schema"])
		}
		fields, ok := schema["fields"].([]any)
		if !ok || len(fields) != 2 {
			t.Fatalf("expected 2 schema fields, got %v", schema["fields"])
		}
		wantFieldNames := []string{"id", "name"}
		for i, field := range fields {
			fieldMap, ok := field.(map[string]any)
			if !ok {
				t.Fatalf("expected schema field to be an object, got %v", field)
			}
			if fieldMap["name"] != wantFieldNames[i] {
				t.Fatalf("unexpected field name at %d: got %v, want %q", i, fieldMap["name"], wantFieldNames[i])
			}
		}

		// A freshly created table has no snapshot.
		if _, ok := info["current-snapshot"]; ok {
			t.Fatalf("expected no current-snapshot on a fresh table, got %v", info["current-snapshot"])
		}
	})

	t.Run("invoke my-get-table-info with missing table", func(t *testing.T) {
		status, respBody := invokeTool(t, "my-get-table-info", map[string]any{
			"namespace": testNamespace,
			"table":     "no_such_table",
		})
		requireAgentError(t, status, respBody)
	})
}
