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

package spanneromni

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/api/option"
)

const (
	SpannerOmniImage = "us-docker.pkg.dev/spanner-omni/images/spanner-omni:2026.r4-lts"
	SpannerOmniPort  = "15000"
)

// By default the test starts a plaintext single-server Spanner Omni container.
// Set SPANNER_OMNI_ENDPOINT to test against an existing deployment instead,
// with SPANNER_OMNI_CA_CERT (and optionally SPANNER_OMNI_CLIENT_CERT and
// SPANNER_OMNI_CLIENT_KEY) for TLS/mTLS, SPANNER_OMNI_USERNAME and
// SPANNER_OMNI_PASSWORD for password authentication, or
// SPANNER_OMNI_USE_PLAINTEXT=true.
var (
	SpannerOmniEndpoint     = os.Getenv("SPANNER_OMNI_ENDPOINT")
	SpannerOmniUsePlainText = os.Getenv("SPANNER_OMNI_USE_PLAINTEXT") == "true"
	SpannerOmniCaCert       = os.Getenv("SPANNER_OMNI_CA_CERT")
	SpannerOmniClientCert   = os.Getenv("SPANNER_OMNI_CLIENT_CERT")
	SpannerOmniClientKey    = os.Getenv("SPANNER_OMNI_CLIENT_KEY")
	SpannerOmniUsername     = os.Getenv("SPANNER_OMNI_USERNAME")
	SpannerOmniPassword     = os.Getenv("SPANNER_OMNI_PASSWORD")
)

// setupSpannerOmniContainer starts Spanner Omni and returns its host:port.
func setupSpannerOmniContainer(ctx context.Context, t *testing.T) (string, func()) {
	t.Helper()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: SpannerOmniImage,
			// Omni listens only on localhost by default, which the mapped port can't reach.
			Cmd:          []string{"start-single-server", "--listen-addresses=0.0.0.0"},
			ExposedPorts: []string{SpannerOmniPort + "/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForLog("Spanner is ready"),
				// The Omni image has no shell tools for the in-container port check.
				wait.ForListeningPort(SpannerOmniPort+"/tcp").SkipInternalCheck(),
			).WithDeadline(5 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		if container != nil {
			_ = container.Terminate(context.Background())
		}
		t.Fatalf("failed to start Spanner Omni container: %s", err)
	}

	cleanup := func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cleanupCancel()
		if err := container.Terminate(cleanupCtx); err != nil {
			t.Errorf("failed to terminate container: %s", err)
		}
	}

	endpoint, err := container.PortEndpoint(ctx, SpannerOmniPort+"/tcp", "")
	if err != nil {
		cleanup()
		t.Fatalf("failed to get Spanner Omni endpoint: %s", err)
	}
	return endpoint, cleanup
}

func omniClientConfig() spanner.ClientConfig {
	return spanner.ClientConfig{
		Type:                  spanner.OMNI,
		UsePlainText:          SpannerOmniUsePlainText,
		CaCertificateFile:     SpannerOmniCaCert,
		ClientCertificateFile: SpannerOmniClientCert,
		ClientKeyFile:         SpannerOmniClientKey,
		Username:              SpannerOmniUsername,
		Password:              []byte(SpannerOmniPassword),
	}
}

// setupSpannerOmniDatabase creates a database with the test schema and data.
func setupSpannerOmniDatabase(ctx context.Context, t *testing.T, endpoint, dbName, tableName, graphName string) func() {
	t.Helper()
	config := omniClientConfig()
	opt := option.WithEndpoint(endpoint)

	adminClient, err := database.NewDatabaseAdminClientWithConfig(ctx, config, opt)
	if err != nil {
		t.Fatalf("unable to create Spanner Omni admin client: %s", err)
	}

	nodeTable := tableName + "_node"
	edgeTable := tableName + "_edge"
	op, err := adminClient.CreateDatabase(ctx, &databasepb.CreateDatabaseRequest{
		Parent:          "projects/default/instances/default",
		CreateStatement: fmt.Sprintf("CREATE DATABASE `%s`", dbName),
		ExtraStatements: []string{
			fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX)) PRIMARY KEY (id)", tableName),
			fmt.Sprintf("CREATE TABLE %s (id INT64 NOT NULL) PRIMARY KEY (id)", nodeTable),
			fmt.Sprintf(`CREATE TABLE %[1]s (
				id INT64 NOT NULL,
				target_id INT64 NOT NULL,
				FOREIGN KEY (target_id) REFERENCES %[2]s (id)
			) PRIMARY KEY (id, target_id), INTERLEAVE IN PARENT %[2]s ON DELETE CASCADE`, edgeTable, nodeTable),
			fmt.Sprintf(`CREATE PROPERTY GRAPH %[3]s
				NODE TABLES (%[1]s)
				EDGE TABLES (%[2]s
					SOURCE KEY (id) REFERENCES %[1]s
					DESTINATION KEY (target_id) REFERENCES %[1]s
					LABEL EDGE)`, nodeTable, edgeTable, graphName),
		},
	})
	if err != nil {
		adminClient.Close()
		t.Fatalf("unable to start create database operation: %s", err)
	}
	if _, err := op.Wait(ctx); err != nil {
		adminClient.Close()
		t.Fatalf("unable to create database %s: %s", dbName, err)
	}
	dbString := fmt.Sprintf("projects/default/instances/default/databases/%s", dbName)

	dataClient, err := spanner.NewClientWithConfig(ctx, dbString, config, opt)
	if err != nil {
		adminClient.Close()
		t.Fatalf("unable to create Spanner Omni client: %s", err)
	}
	_, err = dataClient.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		_, err := txn.Update(ctx, spanner.Statement{
			SQL: fmt.Sprintf("INSERT INTO %s (id, name) VALUES (1, 'Alice'), (2, 'Jane'), (3, 'Sid')", tableName),
		})
		return err
	})
	dataClient.Close()
	if err != nil {
		adminClient.Close()
		t.Fatalf("unable to insert test data: %s", err)
	}

	return func() {
		defer adminClient.Close()
		if err := adminClient.DropDatabase(context.WithoutCancel(ctx), &databasepb.DropDatabaseRequest{Database: dbString}); err != nil {
			t.Errorf("unable to drop database %s: %s", dbName, err)
		}
	}
}

func getSpannerOmniToolsConfig(endpoint, dbName string) map[string]any {
	// project and instance are omitted on purpose: they default for Omni.
	source := map[string]any{
		"type":         "spanner",
		"database":     dbName,
		"instanceType": "omni",
		"omniEndpoint": endpoint,
	}
	if SpannerOmniUsePlainText {
		source["omniUsePlainText"] = true
	}
	if SpannerOmniCaCert != "" {
		source["omniCaCertificateFile"] = SpannerOmniCaCert
	}
	if SpannerOmniClientCert != "" {
		source["omniClientCertificateFile"] = SpannerOmniClientCert
		source["omniClientKeyFile"] = SpannerOmniClientKey
	}
	if SpannerOmniUsername != "" {
		source["omniUsername"] = SpannerOmniUsername
		source["omniPassword"] = SpannerOmniPassword
	}

	tool := func(toolType string, extra map[string]any) map[string]any {
		t := map[string]any{"type": toolType, "source": "my-spanner-omni", "description": toolType}
		for k, v := range extra {
			t[k] = v
		}
		return t
	}
	return map[string]any{
		"sources": map[string]any{"my-spanner-omni": source},
		"tools": map[string]any{
			"my-exec-sql-tool-read-only": tool("spanner-execute-sql", map[string]any{"readOnly": true}),
			"my-exec-sql-tool":           tool("spanner-execute-sql", nil),
			"my-list-tables-tool":        tool("spanner-list-tables", nil),
			"my-list-graphs-tool":        tool("spanner-list-graphs", nil),
			"my-search-catalog-tool":     tool("spanner-search-catalog", nil),
		},
	}
}

func TestSpannerOmniToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	endpoint := SpannerOmniEndpoint
	if endpoint == "" {
		var cleanupContainer func()
		endpoint, cleanupContainer = setupSpannerOmniContainer(ctx, t)
		defer cleanupContainer()
		SpannerOmniUsePlainText = true
	}

	suffix := strings.ReplaceAll(uuid.New().String(), "-", "")
	dbName := "omni_" + suffix[:16]
	tableName := "omni_table_" + suffix
	graphName := "omni_graph_" + suffix
	teardownDatabase := setupSpannerOmniDatabase(ctx, t, endpoint, dbName, tableName, graphName)
	defer teardownDatabase()

	cmd, cleanup, err := tests.StartCmd(ctx, getSpannerOmniToolsConfig(endpoint, dbName), "--enable-api")
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	runSpannerOmniToolInvokeTest(t, tableName, graphName)
}

func runSpannerOmniToolInvokeTest(t *testing.T, tableName, graphName string) {
	tcs := []struct {
		name     string
		tool     string
		body     map[string]any
		want     string // exact result when wantSub is false
		wantSub  bool   // want is a substring of the result
		wantFail string // the response must contain this error
	}{
		{
			name: "read-only select 1",
			tool: "my-exec-sql-tool-read-only",
			body: map[string]any{"sql": "SELECT 1"},
			want: `[{"":"1"}]`,
		},
		{
			name: "read-only select from table",
			tool: "my-exec-sql-tool-read-only",
			body: map[string]any{"sql": fmt.Sprintf("SELECT * FROM %s WHERE id = 3 OR name = 'Alice' ORDER BY id", tableName)},
			want: `[{"id":"1","name":"Alice"},{"id":"3","name":"Sid"}]`,
		},
		{
			name:     "read-only rejects DML",
			tool:     "my-exec-sql-tool-read-only",
			body:     map[string]any{"sql": fmt.Sprintf("INSERT INTO %s (id, name) VALUES (4, 'Omni')", tableName)},
			wantFail: "DML statements may not be performed in single-use transactions",
		},
		{
			name: "read-write insert",
			tool: "my-exec-sql-tool",
			body: map[string]any{"sql": fmt.Sprintf("INSERT INTO %s (id, name) VALUES (4, 'Omni')", tableName)},
			want: `[]`,
		},
		{
			name: "read-only sees insert",
			tool: "my-exec-sql-tool-read-only",
			body: map[string]any{"sql": fmt.Sprintf("SELECT name FROM %s WHERE id = 4", tableName)},
			want: `[{"name":"Omni"}]`,
		},
		{
			name:    "list tables",
			tool:    "my-list-tables-tool",
			body:    map[string]any{"table_names": tableName, "output_format": "simple"},
			want:    fmt.Sprintf(`"object_name":"%s"`, tableName),
			wantSub: true,
		},
		{
			name:    "list graphs",
			tool:    "my-list-graphs-tool",
			body:    map[string]any{"graph_names": graphName, "output_format": "simple"},
			want:    fmt.Sprintf(`"object_name":"%s"`, graphName),
			wantSub: true,
		},
		{
			name:     "search catalog is not supported",
			tool:     "my-search-catalog-tool",
			body:     map[string]any{"prompt": "table"},
			wantFail: "search catalog is not supported for Spanner Omni sources",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("unable to marshal request: %s", err)
			}
			api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.tool)
			resp, respBody := tests.RunRequest(t, http.MethodPost, api, bytes.NewBuffer(reqBody), nil)
			if tc.wantFail != "" {
				if !strings.Contains(string(respBody), tc.wantFail) {
					t.Fatalf("expected error containing %q, got %d: %s", tc.wantFail, resp.StatusCode, respBody)
				}
				return
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, respBody)
			}
			var body map[string]any
			if err := json.Unmarshal(respBody, &body); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body: %s", respBody)
			}
			if tc.wantSub && !strings.Contains(got, tc.want) {
				t.Fatalf("result %q does not contain %q", got, tc.want)
			}
			if !tc.wantSub && got != tc.want {
				t.Fatalf("unexpected result: got %q, want %q", got, tc.want)
			}
		})
	}
}
