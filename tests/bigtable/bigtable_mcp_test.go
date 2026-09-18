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
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// setupBigtableMCPServer seeds the test tables and starts a Toolbox server serving
// the Bigtable tools over the MCP endpoint.
func setupBigtableMCPServer(t *testing.T, ctx context.Context) (string, func()) {
	sourceConfig := getBigtableVars(t)

	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Logf("Starting Bigtable MCP test with uniqueID: %s", uniqueID)

	adminClient, err := bigtable.NewAdminClient(context.Background(), sourceConfig["project"].(string), sourceConfig["instance"].(string))
	if err != nil {
		t.Fatalf("Failed to create AdminClient: %v", err)
	}

	t.Cleanup(func() {
		adminClient.Close()
	})

	t.Cleanup(func() {
		t.Logf("Running global cleanup for uniqueID: %s", uniqueID)
		tests.CleanupBigtableTables(t, context.Background(), adminClient, uniqueID)
	})

	tableName := "param_table_" + uniqueID
	tableNameAuth := "auth_table_" + uniqueID
	tableNameTemplateParam := "tmpl_param_table_" + uniqueID

	columnFamilyName := "cf"
	muts, rowKeys := getTestData(columnFamilyName)

	// Do not change the shape of statement without checking tests/common_test.go.
	paramTestStatement := fmt.Sprintf("SELECT TO_INT64(cf['id']) as id, CAST(cf['name'] AS string) as name, FROM %s WHERE TO_INT64(cf['id']) = @id OR CAST(cf['name'] AS string) = @name;", tableName)
	idParamTestStatement := fmt.Sprintf("SELECT TO_INT64(cf['id']) as id, CAST(cf['name'] AS string) as name, FROM %s WHERE TO_INT64(cf['id']) = @id;", tableName)
	nameParamTestStatement := fmt.Sprintf("SELECT TO_INT64(cf['id']) as id, CAST(cf['name'] AS string) as name, FROM %s WHERE CAST(cf['name'] AS string) = @name;", tableName)
	arrayTestStatement := fmt.Sprintf(
		"SELECT TO_INT64(cf['id']) AS id, CAST(cf['name'] AS string) AS name FROM %s WHERE TO_INT64(cf['id']) IN UNNEST(@idArray) AND CAST(cf['name'] AS string) IN UNNEST(@nameArray);",
		tableName,
	)
	setupBtTable(t, adminClient, ctx, sourceConfig["project"].(string), sourceConfig["instance"].(string), tableName, columnFamilyName, muts, rowKeys)

	authToolStatement := fmt.Sprintf("SELECT CAST(cf['name'] AS string) as name FROM %s WHERE CAST(cf['email'] AS string) = @email;", tableNameAuth)
	setupBtTable(t, adminClient, ctx, sourceConfig["project"].(string), sourceConfig["instance"].(string), tableNameAuth, columnFamilyName, muts, rowKeys)

	mutsTmpl, rowKeysTmpl := getTestDataTemplateParam(columnFamilyName)
	setupBtTable(t, adminClient, ctx, sourceConfig["project"].(string), sourceConfig["instance"].(string), tableNameTemplateParam, columnFamilyName, mutsTmpl, rowKeysTmpl)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, BigtableToolType, paramTestStatement, idParamTestStatement, nameParamTestStatement, arrayTestStatement, authToolStatement)
	toolsFile = addTemplateParamConfig(t, toolsFile)
	toolsFile = addBigTableAdminToolsConfig(t, toolsFile)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	return tableNameTemplateParam, cleanup
}

func TestBigtableMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	_, cleanup := setupBigtableMCPServer(t, ctx)
	defer cleanup()

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getBigtableMCPExpectedTools())
	})
}

func TestBigtableMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	tableNameTemplateParam, cleanup := setupBigtableMCPServer(t, ctx)
	defer cleanup()

	// Actual test parameters are set in tests/tool.go.
	select1Want := "[{\"$col1\":1}]"
	myToolById4Want := `[{"id":4,"name":""}]`
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing GCP request: unable to prepare statement: rpc error: code = InvalidArgument desc = Syntax error: Unexpected identifier \"SELEC\" [at 1:1]"}],"isError":true}}`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"$col1\":1}"}]}}`
	nameFieldArray := `["CAST(cf['name'] AS string) as name"]`
	nameColFilter := "CAST(cf['name'] AS string)"

	tests.RunToolInvokeTest(t, select1Want,
		tests.WithMyToolById4Want(myToolById4Want),
		tests.WithMCP(),
	)
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam,
		tests.WithNameFieldArray(nameFieldArray),
		tests.WithNameColFilter(nameColFilter),
		tests.DisableDdlTest(),
		tests.DisableInsertTest(),
		tests.WithMCPTemplate(),
	)
}

// getBigtableMCPExpectedTools returns the MCP manifests for every tool loaded by the Bigtable tests.
func getBigtableMCPExpectedTools() []tests.MCPToolManifest {
	expectedTools := tests.GetBaseMCPExpectedTools()

	// Bigtable only registers the select template tools; DDL and insert are not supported.
	bigtableTemplateTools := map[string]bool{
		"select-templateParams-tool":                 true,
		"select-templateParams-combined-tool":        true,
		"select-fields-templateParams-tool":          true,
		"select-filter-templateParams-combined-tool": true,
	}
	for _, tool := range tests.GetTemplateParamMCPExpectedTools() {
		if bigtableTemplateTools[tool.Name] {
			expectedTools = append(expectedTools, tool)
		}
	}

	instanceID := map[string]any{"type": "string", "description": "The ID of the instance"}
	clusterID := map[string]any{"type": "string", "description": "The ID of the cluster"}
	logicalViewID := map[string]any{"type": "string", "description": "The ID of the logical view"}
	materializedViewID := map[string]any{"type": "string", "description": "The ID of the materialized view"}

	return append(expectedTools, []tests.MCPToolManifest{
		{
			Name:        "bigtable-create-cluster",
			Description: "Create a new Bigtable cluster in an instance.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instance_id": instanceID,
					"cluster_id":  clusterID,
					"zone":        map[string]any{"type": "string", "description": "The zone for the cluster (e.g. us-central1-b)"},
					"num_nodes":   map[string]any{"type": "integer", "description": "The number of nodes to allocate"},
				},
				"required": []any{"instance_id", "cluster_id", "zone", "num_nodes"},
			},
		},
		{
			Name:        "bigtable-create-instance",
			Description: "Create a new Bigtable instance.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instance_id":  map[string]any{"type": "string", "description": "The ID of the instance to create"},
					"display_name": map[string]any{"type": "string", "description": "Display name for the instance"},
					"cluster_id":   map[string]any{"type": "string", "description": "The ID of the primary cluster"},
					"zone":         map[string]any{"type": "string", "description": "The zone for the cluster (e.g. us-central1-b)"},
					"num_nodes":    map[string]any{"type": "integer", "description": "The number of nodes for the cluster"},
				},
				"required": []any{"instance_id", "display_name", "cluster_id", "zone", "num_nodes"},
			},
		},
		{
			Name:        "bigtable-create-logical-view",
			Description: "Create a new Bigtable logical view.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instance_id":     instanceID,
					"logical_view_id": logicalViewID,
					"query":           map[string]any{"type": "string", "description": "The logical view query"},
				},
				"required": []any{"instance_id", "logical_view_id", "query"},
			},
		},
		{
			Name:        "bigtable-create-table",
			Description: "Create a new Bigtable table.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"table_id":      map[string]any{"type": "string", "description": "The ID of the table to create"},
					"column_family": map[string]any{"type": "string", "description": "Optional column family name to create with the table"},
				},
				"required": []any{"table_id"},
			},
		},
		{
			Name:        "bigtable-delete-cluster",
			Description: "Delete a Bigtable cluster.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"instance_id": instanceID, "cluster_id": clusterID},
				"required":   []any{"instance_id", "cluster_id"},
			},
		},
		{
			Name:        "bigtable-delete-instance",
			Description: "Delete a Bigtable instance.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"instance_id": map[string]any{"type": "string", "description": "The ID of the instance to delete"}},
				"required":   []any{"instance_id"},
			},
		},
		{
			Name:        "bigtable-delete-logical-view",
			Description: "Delete a Bigtable logical view.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"instance_id": instanceID, "logical_view_id": logicalViewID},
				"required":   []any{"instance_id", "logical_view_id"},
			},
		},
		{
			Name:        "bigtable-delete-table",
			Description: "Delete a Bigtable table.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"table_id": map[string]any{"type": "string", "description": "The ID of the table to delete"}},
				"required":   []any{"table_id"},
			},
		},
		{
			Name:        "bigtable-get-cluster",
			Description: "Get details of a Bigtable cluster.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"instance_id": instanceID, "cluster_id": clusterID},
				"required":   []any{"instance_id", "cluster_id"},
			},
		},
		{
			Name:        "bigtable-get-instance",
			Description: "Get details of a Bigtable instance.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"instance_id": map[string]any{"type": "string", "description": "The ID of the instance to get"}},
				"required":   []any{"instance_id"},
			},
		},
		{
			Name:        "bigtable-get-logical-view",
			Description: "Get details of a Bigtable logical view.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"instance_id": instanceID, "logical_view_id": logicalViewID},
				"required":   []any{"instance_id", "logical_view_id"},
			},
		},
		{
			Name:        "bigtable-get-table",
			Description: "Get details of a Bigtable table.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"table_id": map[string]any{"type": "string", "description": "The ID of the table to get"}},
				"required":   []any{"table_id"},
			},
		},
		{
			Name:        "bigtable-list-clusters",
			Description: "List all Bigtable clusters in the instance.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"instance_id": instanceID},
				"required":   []any{"instance_id"},
			},
		},
		{
			Name:        "bigtable-list-instances",
			Description: "List all Bigtable instances in the project.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "bigtable-list-logical-views",
			Description: "List all Bigtable logical views in the instance.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"instance_id": instanceID},
				"required":   []any{"instance_id"},
			},
		},
		{
			Name:        "bigtable-list-tables",
			Description: "List all Bigtable tables in the instance.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "bigtable-update-cluster",
			Description: "Update the number of nodes in a Bigtable cluster.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instance_id": instanceID,
					"cluster_id":  clusterID,
					"serve_nodes": map[string]any{"type": "integer", "description": "The new number of nodes to allocate"},
				},
				"required": []any{"instance_id", "cluster_id", "serve_nodes"},
			},
		},
		{
			Name:        "bigtable-update-instance",
			Description: "Update an existing Bigtable instance.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instance_id":  map[string]any{"type": "string", "description": "The ID of the instance to update"},
					"display_name": map[string]any{"type": "string", "description": "The new display name"},
				},
				"required": []any{"instance_id", "display_name"},
			},
		},
		{
			Name:        "bigtable-update-logical-view",
			Description: "Update an existing Bigtable logical view.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instance_id":     instanceID,
					"logical_view_id": logicalViewID,
					"query":           map[string]any{"type": "string", "description": "The new logical view query"},
				},
				"required": []any{"instance_id", "logical_view_id", "query"},
			},
		},
		{
			Name:        "bigtable-create-materialized-view",
			Description: "Create a new Bigtable materialized view.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instance_id":          instanceID,
					"materialized_view_id": materializedViewID,
					"query":                map[string]any{"type": "string", "description": "The materialized view query"},
				},
				"required": []any{"instance_id", "materialized_view_id", "query"},
			},
		},
		{
			Name:        "bigtable-delete-materialized-view",
			Description: "Delete an existing Bigtable materialized view.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"instance_id": instanceID, "materialized_view_id": materializedViewID},
				"required":   []any{"instance_id", "materialized_view_id"},
			},
		},
		{
			Name:        "bigtable-get-materialized-view",
			Description: "Get information about an existing Bigtable materialized view.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"instance_id": instanceID, "materialized_view_id": materializedViewID},
				"required":   []any{"instance_id", "materialized_view_id"},
			},
		},
		{
			Name:        "bigtable-list-materialized-views",
			Description: "List all existing Bigtable materialized views.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"instance_id": instanceID},
				"required":   []any{"instance_id"},
			},
		},
		{
			Name:        "bigtable-update-materialized-view",
			Description: "Update an existing Bigtable materialized view.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instance_id":          instanceID,
					"materialized_view_id": materializedViewID,
					"query":                map[string]any{"type": "string", "description": "The updated materialized view query"},
				},
				"required": []any{"instance_id", "materialized_view_id", "query"},
			},
		},
		{
			Name:        "bigtable-list-schemas",
			Description: "List all Bigtable schemas, including tables with column family definitions, logical views, and materialized views.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{"type": "integer", "description": "Optional: The maximum number of tables to return. Default is 20", "default": float64(20)},
				},
				"required": []any{},
			},
		},
		{
			Name:        "bigtable-update-table",
			Description: "Update an existing Bigtable table's configuration.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"table_id":              map[string]any{"type": "string", "description": "The ID of the table to update"},
					"disable_change_stream": map[string]any{"type": "boolean", "description": "Disable change stream", "default": true},
				},
				"required": []any{"table_id"},
			},
		},
	}...)
}
