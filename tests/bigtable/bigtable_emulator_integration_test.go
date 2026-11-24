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

package bigtable

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
)

// The Cloud SDK Bigtable emulator implements the Bigtable data plane and table
// admin API in memory. Real-GCP coverage in TestBigtableToolEndpoints exercises
// features the emulator does not implement: GoogleSQL (PrepareQuery/ExecuteQuery),
// InstanceAdmin (instances, clusters, logical views, materialized views, and
// bigtable-list-schemas), and IAM/authentication.
const (
	bigtableEmulatorImage    = "gcr.io/google.com/cloudsdktool/google-cloud-cli:592.0.0-emulators"
	bigtableEmulatorPort     = "8086"
	bigtableEmulatorProject  = "emulator-project"
	bigtableEmulatorInstance = "emulator-instance"
)

// setupBigtableEmulatorContainer starts the Bigtable emulator in memory,
// schedules container termination via t.Cleanup, and returns its host:port.
func setupBigtableEmulatorContainer(ctx context.Context, t *testing.T) string {
	t.Helper()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        bigtableEmulatorImage,
			Cmd:          []string{"gcloud", "beta", "emulators", "bigtable", "start", "--host-port=0.0.0.0:" + bigtableEmulatorPort},
			ExposedPorts: []string{bigtableEmulatorPort + "/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForLog("Cloud Bigtable emulator running").WithStartupTimeout(3*time.Minute),
				wait.ForListeningPort(bigtableEmulatorPort+"/tcp").WithStartupTimeout(3*time.Minute),
			).WithDeadline(3 * time.Minute),
		},
		Started: true,
		Logger:  tclog.TestLogger(t),
	})
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("failed to start Bigtable emulator container: %s", err)
	}

	endpoint, err := container.PortEndpoint(ctx, bigtableEmulatorPort+"/tcp", "")
	if err != nil {
		t.Fatalf("failed to get Bigtable emulator endpoint: %s", err)
	}
	return endpoint
}

func TestBigtableEmulatorToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	endpoint := setupBigtableEmulatorContainer(ctx, t)
	t.Setenv("BIGTABLE_EMULATOR_HOST", endpoint)

	adminClient, err := bigtable.NewAdminClient(context.Background(), bigtableEmulatorProject, bigtableEmulatorInstance)
	if err != nil {
		t.Fatalf("failed to create emulator AdminClient: %v", err)
	}
	t.Cleanup(func() {
		if err := adminClient.Close(); err != nil {
			t.Errorf("failed to close emulator AdminClient: %v", err)
		}
	})

	// Seed an in-memory table; container termination discards emulator state.
	columnFamilyName := "cf"
	seededTable := "emulator_seeded_table"
	muts, rowKeys := getTestData(columnFamilyName)
	setupBtTable(t, adminClient, ctx, bigtableEmulatorProject, bigtableEmulatorInstance, seededTable, columnFamilyName, muts, rowKeys)
	verifyEmulatorSeedData(t, ctx, seededTable, columnFamilyName, rowKeys)

	toolsMap := map[string]any{}
	for _, toolType := range []string{
		"bigtable-create-table",
		"bigtable-get-table",
		"bigtable-list-tables",
		"bigtable-update-table",
		"bigtable-delete-table",
	} {
		toolsMap[toolType] = map[string]any{
			"type":   toolType,
			"source": "my-instance",
		}
	}
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": map[string]any{
				"type":     BigtableSourceType,
				"project":  bigtableEmulatorProject,
				"instance": bigtableEmulatorInstance,
			},
		},
		"tools": toolsMap,
	}

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, "--enable-api")
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	runBigtableEmulatorTableToolsTest(t, adminClient, seededTable, columnFamilyName)
}

func verifyEmulatorSeedData(t *testing.T, ctx context.Context, tableName, columnFamilyName string, wantRowKeys []string) {
	t.Helper()
	client, err := bigtable.NewClient(ctx, bigtableEmulatorProject, bigtableEmulatorInstance)
	if err != nil {
		t.Fatalf("failed to create emulator data client: %v", err)
	}
	defer func() { _ = client.Close() }()

	var gotRowKeys []string
	err = client.Open(tableName).ReadRows(ctx, bigtable.InfiniteRange(""), func(row bigtable.Row) bool {
		if len(row[columnFamilyName]) == 0 {
			t.Errorf("row %q has no cells in column family %q", row.Key(), columnFamilyName)
		}
		gotRowKeys = append(gotRowKeys, row.Key())
		return true
	})
	if err != nil {
		t.Fatalf("failed to read seeded rows from %s: %v", tableName, err)
	}
	want := slices.Clone(wantRowKeys)
	slices.Sort(want)
	if !slices.Equal(gotRowKeys, want) {
		t.Fatalf("seeded row keys = %v, want %v", gotRowKeys, want)
	}
}

func runBigtableEmulatorTableToolsTest(t *testing.T, adminClient *bigtable.AdminClient, seededTable, columnFamilyName string) {
	listTables := func(t *testing.T) []string {
		t.Helper()
		resp := assertMCPSuccess(t, "bigtable-list-tables", map[string]any{})
		if len(resp.Result.Content) == 0 {
			t.Fatalf("bigtable-list-tables returned empty content")
		}
		var tables []string
		if err := json.Unmarshal([]byte(resp.Result.Content[0].Text), &tables); err != nil {
			t.Fatalf("failed to parse bigtable-list-tables output: %v, body: %s", err, resp.Result.Content[0].Text)
		}
		return tables
	}

	t.Run("get seeded table", func(t *testing.T) {
		if tables := listTables(t); !slices.Contains(tables, seededTable) {
			t.Fatalf("bigtable-list-tables = %v, want it to contain %q", tables, seededTable)
		}
		resp := assertMCPSuccess(t, "bigtable-get-table", map[string]any{"table_id": seededTable})
		if len(resp.Result.Content) == 0 {
			t.Fatalf("bigtable-get-table returned empty content")
		}
		var info struct {
			Families []string `json:"Families"`
		}
		if err := json.Unmarshal([]byte(resp.Result.Content[0].Text), &info); err != nil {
			t.Fatalf("failed to parse bigtable-get-table output: %v, body: %s", err, resp.Result.Content[0].Text)
		}
		if !slices.Contains(info.Families, columnFamilyName) {
			t.Fatalf("bigtable-get-table families = %v, want it to contain %q", info.Families, columnFamilyName)
		}
	})

	t.Run("table lifecycle", func(t *testing.T) {
		tableName := "emulator_lifecycle_table"
		cf := "cf1"
		registerBigtableCleanup(t, "table", tableName, func(ctx context.Context) error {
			return adminClient.DeleteTable(ctx, tableName)
		})

		createResp := assertMCPSuccess(t, "bigtable-create-table", map[string]any{
			"table_id":      tableName,
			"column_family": cf,
		})
		if len(createResp.Result.Content) == 0 || !strings.Contains(createResp.Result.Content[0].Text, "table created successfully") {
			t.Fatalf("bigtable-create-table unexpected output: %v", createResp.Result.Content)
		}

		getResp := assertMCPSuccess(t, "bigtable-get-table", map[string]any{"table_id": tableName})
		if len(getResp.Result.Content) == 0 || !strings.Contains(getResp.Result.Content[0].Text, cf) {
			t.Fatalf("bigtable-get-table output does not contain column family %q: %v", cf, getResp.Result.Content)
		}

		if tables := listTables(t); !slices.Contains(tables, tableName) {
			t.Fatalf("bigtable-list-tables = %v, want it to contain %q", tables, tableName)
		}

		updateResp := assertMCPSuccess(t, "bigtable-update-table", map[string]any{
			"table_id":              tableName,
			"disable_change_stream": true,
		})
		if len(updateResp.Result.Content) == 0 || !strings.Contains(updateResp.Result.Content[0].Text, "table updated successfully") {
			t.Fatalf("bigtable-update-table unexpected output: %v", updateResp.Result.Content)
		}

		deleteResp := assertMCPSuccess(t, "bigtable-delete-table", map[string]any{"table_id": tableName})
		if len(deleteResp.Result.Content) == 0 || !strings.Contains(deleteResp.Result.Content[0].Text, "table deleted successfully") {
			t.Fatalf("bigtable-delete-table unexpected output: %v", deleteResp.Result.Content)
		}

		if tables := listTables(t); slices.Contains(tables, tableName) {
			t.Fatalf("bigtable-list-tables = %v, want %q to be deleted", tables, tableName)
		}
	})

	t.Run("get missing table", func(t *testing.T) {
		statusCode, resp, err := tests.InvokeMCPTool(t, "bigtable-get-table", map[string]any{"table_id": "does_not_exist"}, map[string]string{})
		if err != nil {
			t.Fatalf("native error executing bigtable-get-table: %s", err)
		}
		if statusCode != http.StatusOK {
			t.Fatalf("bigtable-get-table: expected HTTP status 200, got %d", statusCode)
		}
		if resp.Error != nil {
			t.Fatalf("bigtable-get-table: unexpected JSON-RPC error: %v", resp.Error)
		}
		if !resp.Result.IsError {
			t.Fatalf("bigtable-get-table on a missing table: expected an error result, got %v", resp.Result.Content)
		}
	})
}
