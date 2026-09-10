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

package bigtable

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	BigtableSourceType = "bigtable"
	BigtableToolType   = "bigtable-sql"
	BigtableProject    = os.Getenv("BIGTABLE_PROJECT")
	BigtableInstance   = os.Getenv("BIGTABLE_INSTANCE")
)

func getBigtableVars(t *testing.T) map[string]any {
	switch "" {
	case BigtableProject:
		t.Fatal("'BIGTABLE_PROJECT' not set")
	case BigtableInstance:
		t.Fatal("'BIGTABLE_INSTANCE' not set")
	}

	return map[string]any{
		"type":     BigtableSourceType,
		"project":  BigtableProject,
		"instance": BigtableInstance,
	}
}

type TestRow struct {
	RowKey     string
	ColumnName string
	Data       []byte
}

func TestBigtableToolEndpoints(t *testing.T) {
	sourceConfig := getBigtableVars(t)

	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	t.Logf("Starting Bigtable test with uniqueID: %s", uniqueID)

	args := []string{"--enable-api"}

	// Initialize AdminClient to create or delete tables
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

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	tableName := "param_table_" + uniqueID
	tableNameAuth := "auth_table_" + uniqueID
	tableNameTemplateParam := "tmpl_param_table_" + uniqueID

	columnFamilyName := "cf"
	muts, rowKeys := getTestData(columnFamilyName)

	// Do not change the shape of statement without checking tests/common_test.go.
	// The structure and value of seed data has to match https://github.com/googleapis/mcp-toolbox/blob/4dba0df12dc438eca3cb476ef52aa17cdf232c12/tests/common_test.go#L200-L251
	paramTestStatement := fmt.Sprintf("SELECT TO_INT64(cf['id']) as id, CAST(cf['name'] AS string) as name, FROM %s WHERE TO_INT64(cf['id']) = @id OR CAST(cf['name'] AS string) = @name;", tableName)
	idParamTestStatement := fmt.Sprintf("SELECT TO_INT64(cf['id']) as id, CAST(cf['name'] AS string) as name, FROM %s WHERE TO_INT64(cf['id']) = @id;", tableName)
	nameParamTestStatement := fmt.Sprintf("SELECT TO_INT64(cf['id']) as id, CAST(cf['name'] AS string) as name, FROM %s WHERE CAST(cf['name'] AS string) = @name;", tableName)
	arrayTestStatement := fmt.Sprintf(
		"SELECT TO_INT64(cf['id']) AS id, CAST(cf['name'] AS string) AS name FROM %s WHERE TO_INT64(cf['id']) IN UNNEST(@idArray) AND CAST(cf['name'] AS string) IN UNNEST(@nameArray);",
		tableName,
	)
	setupBtTable(t, adminClient, ctx, sourceConfig["project"].(string), sourceConfig["instance"].(string), tableName, columnFamilyName, muts, rowKeys)

	// Do not change the shape of statement without checking tests/common_test.go.
	// The structure and value of seed data has to match https://github.com/googleapis/mcp-toolbox/blob/4dba0df12dc438eca3cb476ef52aa17cdf232c12/tests/common_test.go#L200-L251
	authToolStatement := fmt.Sprintf("SELECT CAST(cf['name'] AS string) as name FROM %s WHERE CAST(cf['email'] AS string) = @email;", tableNameAuth)
	setupBtTable(t, adminClient, ctx, sourceConfig["project"].(string), sourceConfig["instance"].(string), tableNameAuth, columnFamilyName, muts, rowKeys)

	mutsTmpl, rowKeysTmpl := getTestDataTemplateParam(columnFamilyName)
	setupBtTable(t, adminClient, ctx, sourceConfig["project"].(string), sourceConfig["instance"].(string), tableNameTemplateParam, columnFamilyName, mutsTmpl, rowKeysTmpl)

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, BigtableToolType, paramTestStatement, idParamTestStatement, nameParamTestStatement, arrayTestStatement, authToolStatement)
	toolsFile = addTemplateParamConfig(t, toolsFile)
	toolsFile = addBigTableAdminToolsConfig(t, toolsFile)

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

	// Get configs for tests
	// Actual test parameters are set in https://github.com/googleapis/mcp-toolbox/blob/52b09a67cb40ac0c5f461598b4673136699a3089/tests/tool_test.go#L250
	select1Want := "[{\"$col1\":1}]"
	myToolById4Want := `[{"id":4,"name":""}]`
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing GCP request: unable to prepare statement: rpc error: code = InvalidArgument desc = Syntax error: Unexpected identifier \"SELEC\" [at 1:1]"}],"isError":true}}`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"$col1\":1}"}]}}`
	nameFieldArray := `["CAST(cf['name'] AS string) as name"]`
	nameColFilter := "CAST(cf['name'] AS string)"

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, select1Want,
		tests.WithMyToolById4Want(myToolById4Want),
	)
	tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	runBigTableAdminToolsGetTest(t)
	if os.Getenv("RUN_EXPENSIVE_TESTS") == "true" {
		runBigTableAdminToolsTest(t, sourceConfig["instance"].(string))
	} else {
		t.Log("Skipping expensive Bigtable admin tools tests (RUN_EXPENSIVE_TESTS is not true)")
	}
	opts := []tests.TemplateParamOption{
		tests.WithNameFieldArray(nameFieldArray),
		tests.WithNameColFilter(nameColFilter),
		tests.DisableDdlTest(),
		tests.DisableInsertTest(),
	}
	tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, opts...)
}

func convertToBytes(v int) []byte {
	binary1 := new(bytes.Buffer)
	if err := binary.Write(binary1, binary.BigEndian, int64(v)); err != nil {
		log.Fatalf("Unable to encode id: %v", err)
	}
	return binary1.Bytes()
}

func getTestData(columnFamilyName string) ([]*bigtable.Mutation, []string) {
	muts := []*bigtable.Mutation{}
	rowKeys := []string{}

	var ids [4][]byte
	for i := range ids {
		ids[i] = convertToBytes(i + 1)
	}

	now := bigtable.Time(time.Now())
	for rowKey, mutData := range map[string]map[string][]byte{
		// Do not change the test data without checking tests/common_test.go.
		// The structure and value of seed data has to match https://github.com/googleapis/mcp-toolbox/blob/4dba0df12dc438eca3cb476ef52aa17cdf232c12/tests/common_test.go#L200-L251
		// Expected values are defined in https://github.com/googleapis/mcp-toolbox/blob/52b09a67cb40ac0c5f461598b4673136699a3089/tests/tool_test.go#L229-L310
		"row-01": {
			"name":  []byte("Alice"),
			"email": []byte(tests.ServiceAccountEmail),
			"id":    ids[0],
		},
		"row-02": {
			"name":  []byte("Jane"),
			"email": []byte("janedoe@gmail.com"),
			"id":    ids[1],
		},
		"row-03": {
			"name": []byte("Sid"),
			"id":   ids[2],
		},
		"row-04": {
			"name": nil,
			"id":   ids[3],
		},
	} {
		mut := bigtable.NewMutation()
		for col, v := range mutData {
			mut.Set(columnFamilyName, col, now, v)
		}
		muts = append(muts, mut)
		rowKeys = append(rowKeys, rowKey)
	}
	return muts, rowKeys
}

func getTestDataTemplateParam(columnFamilyName string) ([]*bigtable.Mutation, []string) {
	muts := []*bigtable.Mutation{}
	rowKeys := []string{}

	var ids [2][]byte
	for i := range ids {
		ids[i] = convertToBytes(i + 1)
	}

	now := bigtable.Time(time.Now())
	for rowKey, mutData := range map[string]map[string][]byte{
		// Do not change the test data without checking tests/common_test.go.
		// The structure and value of seed data has to match https://github.com/googleapis/mcp-toolbox/blob/4dba0df12dc438eca3cb476ef52aa17cdf232c12/tests/common_test.go#L200-L251
		// Expected values are defined in https://github.com/googleapis/mcp-toolbox/blob/52b09a67cb40ac0c5f461598b4673136699a3089/tests/tool_test.go#L229-L310
		"row-01": {
			"name": []byte("Alex"),
			"age":  convertToBytes(21),
			"id":   ids[0],
		},
		"row-02": {
			"name": []byte("Alice"),
			"age":  convertToBytes(100),
			"id":   ids[1],
		},
	} {
		mut := bigtable.NewMutation()
		for col, v := range mutData {
			mut.Set(columnFamilyName, col, now, v)
		}
		muts = append(muts, mut)
		rowKeys = append(rowKeys, rowKey)
	}
	return muts, rowKeys
}

func setupBtTable(t *testing.T, adminClient *bigtable.AdminClient, ctx context.Context, projectId string, instance string, tableName string, columnFamilyName string, muts []*bigtable.Mutation, rowKeys []string) {

	client, err := bigtable.NewClient(ctx, projectId, instance)
	if err != nil {
		t.Fatalf("Could not create data operations client: %v", err)
	}
	defer client.Close()

	// Creating tables
	tables, err := adminClient.Tables(ctx)
	if err != nil {
		t.Fatalf("Could not fetch table list: %v", err)
	}

	if !slices.Contains(tables, tableName) {
		t.Logf("Creating table %s", tableName)
		if err := adminClient.CreateTable(ctx, tableName); err != nil {
			t.Fatalf("Could not create table %s: %v", tableName, err)
		}
	}

	tblInfo, err := adminClient.TableInfo(ctx, tableName)
	if err != nil {
		t.Fatalf("Could not read info for table %s: %v", tableName, err)
	}

	// Creating column family
	if !slices.Contains(tblInfo.Families, columnFamilyName) {
		if err := adminClient.CreateColumnFamily(ctx, tableName, columnFamilyName); err != nil {
			t.Fatalf("Could not create column family %s: %v", columnFamilyName, err)
		}
	}

	tbl := client.Open(tableName)
	rowErrs, err := tbl.ApplyBulk(ctx, rowKeys, muts)
	if err != nil {
		t.Fatalf("Could not apply bulk row mutation: %v", err)
	}
	if rowErrs != nil {
		for _, rowErr := range rowErrs {
			t.Logf("Error writing row: %v", rowErr)
		}
		t.Fatalf("Could not write some rows")
	}
}

func addTemplateParamConfig(t *testing.T, config map[string]any) map[string]any {
	toolsMap, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	toolsMap["select-templateParams-tool"] = map[string]any{
		"type":        "bigtable-sql",
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT TO_INT64(cf['age']) as age, TO_INT64(cf['id']) as id, CAST(cf['name'] AS string) as name, FROM {{.tableName}};",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-templateParams-combined-tool"] = map[string]any{
		"type":        "bigtable-sql",
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT TO_INT64(cf['age']) as age, TO_INT64(cf['id']) as id, CAST(cf['name'] AS string) as name, FROM {{.tableName}} WHERE TO_INT64(cf['id']) = @id;",
		"parameters":  []parameters.Parameter{parameters.NewIntParameter("id", "the id of the user")},
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-fields-templateParams-tool"] = map[string]any{
		"type":        "bigtable-sql",
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT {{array .fields}}, FROM {{.tableName}};",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewArrayParameter("fields", "The fields to select from", parameters.NewStringParameter("field", "A field that will be returned from the query.")),
		},
	}
	toolsMap["select-filter-templateParams-combined-tool"] = map[string]any{
		"type":        "bigtable-sql",
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT TO_INT64(cf['age']) as age, TO_INT64(cf['id']) as id, CAST(cf['name'] AS string) as name, FROM {{.tableName}} WHERE {{.columnFilter}} = @name;",
		"parameters":  []parameters.Parameter{parameters.NewStringParameter("name", "the name of the user")},
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewStringParameter("columnFilter", "some description"),
		},
	}

	config["tools"] = toolsMap
	return config
}

// assertMCPSuccess invokes an MCP tool and strictly asserts that the HTTP status is 200,
// no JSON-RPC error is returned, and Result.IsError is false.
func assertMCPSuccess(t *testing.T, toolName string, args map[string]any) *tests.MCPCallToolResponse {
	t.Helper()
	statusCode, mcpResp, err := tests.InvokeMCPTool(t, toolName, args, map[string]string{})
	if err != nil {
		t.Fatalf("native error executing %s: %s", toolName, err)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("%s: expected HTTP status 200, got %d", toolName, statusCode)
	}
	if mcpResp.Error != nil {
		t.Fatalf("%s: unexpected JSON-RPC error: %v", toolName, mcpResp.Error)
	}
	if mcpResp.Result.IsError {
		t.Fatalf("%s: expected success, got error result: %v", toolName, mcpResp.Result.Content)
	}
	return mcpResp
}

// runBigTableAdminToolsTest verifies all 20 Bigtable admin lifecycle tools for instances, clusters, tables, and logical views.
func runBigTableAdminToolsTest(t *testing.T, instanceId string) {
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	tableName := "admin_test_table_" + uniqueID
	viewName := "admin_test_view_" + uniqueID

	// List instances
	listInstResp := assertMCPSuccess(t, "bigtable-list-instances", map[string]any{})
	if len(listInstResp.Result.Content) == 0 || !strings.Contains(listInstResp.Result.Content[0].Text, instanceId) {
		t.Fatalf("bigtable-list-instances output does not contain expected instance %q: %v", instanceId, listInstResp.Result.Content)
	}

	// Get existing instance
	getInstResp := assertMCPSuccess(t, "bigtable-get-instance", map[string]any{
		"instance_id": instanceId,
	})
	if len(getInstResp.Result.Content) == 0 || !strings.Contains(getInstResp.Result.Content[0].Text, instanceId) {
		t.Fatalf("bigtable-get-instance unexpected output: %v", getInstResp.Result.Content)
	}

	// List clusters
	listClustersResp := assertMCPSuccess(t, "bigtable-list-clusters", map[string]any{
		"instance_id": instanceId,
	})
	var clusters []map[string]any
	if err := json.Unmarshal([]byte(listClustersResp.Result.Content[0].Text), &clusters); err != nil || len(clusters) == 0 {
		t.Fatalf("failed to parse bigtable-list-clusters output: %v, body: %s", err, listClustersResp.Result.Content[0].Text)
	}
	primaryClusterName, _ := clusters[0]["Name"].(string)
	primaryClusterId := primaryClusterName
	if idx := strings.LastIndex(primaryClusterName, "/clusters/"); idx >= 0 {
		primaryClusterId = primaryClusterName[idx+len("/clusters/"):]
	}
	if primaryClusterId == "" {
		t.Fatalf("unexpected empty cluster ID from Name: %q", primaryClusterName)
	}

	// Get existing cluster
	getClusterResp := assertMCPSuccess(t, "bigtable-get-cluster", map[string]any{
		"instance_id": instanceId,
		"cluster_id":  primaryClusterId,
	})
	if len(getClusterResp.Result.Content) == 0 || !strings.Contains(getClusterResp.Result.Content[0].Text, primaryClusterId) {
		t.Fatalf("bigtable-get-cluster unexpected output: %v", getClusterResp.Result.Content)
	}

	// Create test instance for lifecycle tools (createinstance, updateinstance, updatecluster, createcluster, deletecluster, deleteinstance)
	testInstId := "testi-" + uniqueID[:8]
	testClusterId1 := "testc1-" + uniqueID[:8]
	createInstResp := assertMCPSuccess(t, "bigtable-create-instance", map[string]any{
		"instance_id":  testInstId,
		"display_name": "Test Instance " + uniqueID[:8],
		"cluster_id":   testClusterId1,
		"zone":         "us-east1-b",
		"num_nodes":    1,
	})
	if len(createInstResp.Result.Content) == 0 || !strings.Contains(createInstResp.Result.Content[0].Text, "instance created successfully") {
		t.Fatalf("bigtable-create-instance unexpected output: %v", createInstResp.Result.Content)
	}
	defer func() {
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, "bigtable-delete-instance", map[string]any{
			"instance_id": testInstId,
		}, map[string]string{})
		if err != nil || statusCode != http.StatusOK || (mcpResp != nil && (mcpResp.Error != nil || mcpResp.Result.IsError)) {
			t.Logf("cleanup: bigtable-delete-instance failed for %q (status %d): err=%v, mcpResp=%v", testInstId, statusCode, err, mcpResp)
		}
	}()

	// Update instance
	updateInstResp := assertMCPSuccess(t, "bigtable-update-instance", map[string]any{
		"instance_id":  testInstId,
		"display_name": "Updated Display Name",
	})
	if len(updateInstResp.Result.Content) == 0 || !strings.Contains(updateInstResp.Result.Content[0].Text, "instance updated successfully") {
		t.Fatalf("bigtable-update-instance unexpected output: %v", updateInstResp.Result.Content)
	}

	// Update cluster
	updateClusterResp := assertMCPSuccess(t, "bigtable-update-cluster", map[string]any{
		"instance_id": testInstId,
		"cluster_id":  testClusterId1,
		"serve_nodes": 1,
	})
	if len(updateClusterResp.Result.Content) == 0 || !strings.Contains(updateClusterResp.Result.Content[0].Text, "cluster updated successfully") {
		t.Fatalf("bigtable-update-cluster unexpected output: %v", updateClusterResp.Result.Content)
	}

	// Create secondary cluster
	testClusterId2 := "testc2-" + uniqueID[:8]
	createClusterResp := assertMCPSuccess(t, "bigtable-create-cluster", map[string]any{
		"instance_id": testInstId,
		"cluster_id":  testClusterId2,
		"zone":        "us-west1-b",
		"num_nodes":   1,
	})
	if len(createClusterResp.Result.Content) == 0 || !strings.Contains(createClusterResp.Result.Content[0].Text, "cluster created successfully") {
		t.Fatalf("bigtable-create-cluster unexpected output: %v", createClusterResp.Result.Content)
	}

	// Delete secondary cluster
	deleteClusterResp := assertMCPSuccess(t, "bigtable-delete-cluster", map[string]any{
		"instance_id": testInstId,
		"cluster_id":  testClusterId2,
	})
	if len(deleteClusterResp.Result.Content) == 0 || !strings.Contains(deleteClusterResp.Result.Content[0].Text, "cluster deleted successfully") {
		t.Fatalf("bigtable-delete-cluster unexpected output: %v", deleteClusterResp.Result.Content)
	}

	// Create table
	createTableResp := assertMCPSuccess(t, "bigtable-create-table", map[string]any{
		"table_id":      tableName,
		"column_family": "cf1",
	})
	if len(createTableResp.Result.Content) == 0 || !strings.Contains(createTableResp.Result.Content[0].Text, "table created successfully") {
		t.Fatalf("bigtable-create-table unexpected output: %v", createTableResp.Result.Content)
	}

	// Make sure we clean up the table!
	defer func() {
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, "bigtable-delete-table", map[string]any{
			"table_id": tableName,
		}, map[string]string{})
		if err != nil || statusCode != http.StatusOK || (mcpResp != nil && (mcpResp.Error != nil || mcpResp.Result.IsError)) {
			t.Logf("cleanup: bigtable-delete-table failed for %q (status %d): err=%v, mcpResp=%v", tableName, statusCode, err, mcpResp)
		}
	}()

	// Get table
	getTableResp := assertMCPSuccess(t, "bigtable-get-table", map[string]any{
		"table_id": tableName,
	})
	if len(getTableResp.Result.Content) == 0 || !strings.Contains(getTableResp.Result.Content[0].Text, "DeletionProtection") {
		t.Fatalf("bigtable-get-table unexpected output: %v", getTableResp.Result.Content)
	}

	// Update table (disable_change_stream)
	updateTableResp := assertMCPSuccess(t, "bigtable-update-table", map[string]any{
		"table_id":              tableName,
		"disable_change_stream": true,
	})
	if len(updateTableResp.Result.Content) == 0 || !strings.Contains(updateTableResp.Result.Content[0].Text, "table updated successfully") {
		t.Fatalf("bigtable-update-table unexpected output: %v", updateTableResp.Result.Content)
	}

	// List tables
	listTablesResp := assertMCPSuccess(t, "bigtable-list-tables", map[string]any{})
	if len(listTablesResp.Result.Content) == 0 || !strings.Contains(listTablesResp.Result.Content[0].Text, tableName) {
		t.Fatalf("bigtable-list-tables output does not contain expected table %q: %v", tableName, listTablesResp.Result.Content)
	}

	// Create logical view
	createViewResp := assertMCPSuccess(t, "bigtable-create-logical-view", map[string]any{
		"instance_id":     instanceId,
		"logical_view_id": viewName,
		"query":           "SELECT _key FROM " + tableName,
	})
	if len(createViewResp.Result.Content) == 0 || !strings.Contains(createViewResp.Result.Content[0].Text, "logical view created successfully") {
		t.Fatalf("bigtable-create-logical-view unexpected output: %v", createViewResp.Result.Content)
	}

	// Make sure we clean up the view!
	defer func() {
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, "bigtable-delete-logical-view", map[string]any{
			"instance_id":     instanceId,
			"logical_view_id": viewName,
		}, map[string]string{})
		if err != nil || statusCode != http.StatusOK || (mcpResp != nil && (mcpResp.Error != nil || mcpResp.Result.IsError)) {
			t.Logf("cleanup: bigtable-delete-logical-view failed for %q (status %d): err=%v, mcpResp=%v", viewName, statusCode, err, mcpResp)
		}
	}()

	// Get logical view
	getViewResp := assertMCPSuccess(t, "bigtable-get-logical-view", map[string]any{
		"instance_id":     instanceId,
		"logical_view_id": viewName,
	})
	if len(getViewResp.Result.Content) == 0 || (!strings.Contains(getViewResp.Result.Content[0].Text, viewName) && !strings.Contains(getViewResp.Result.Content[0].Text, "SELECT")) {
		t.Fatalf("bigtable-get-logical-view unexpected output: %v", getViewResp.Result.Content)
	}

	// Update logical view
	updateViewResp := assertMCPSuccess(t, "bigtable-update-logical-view", map[string]any{
		"instance_id":     instanceId,
		"logical_view_id": viewName,
		"query":           "SELECT _key FROM " + tableName,
	})
	if len(updateViewResp.Result.Content) == 0 || !strings.Contains(updateViewResp.Result.Content[0].Text, "logical view updated successfully") {
		t.Fatalf("bigtable-update-logical-view unexpected output: %v", updateViewResp.Result.Content)
	}

	// List logical views
	listViewsResp := assertMCPSuccess(t, "bigtable-list-logical-views", map[string]any{
		"instance_id": instanceId,
	})
	if len(listViewsResp.Result.Content) == 0 || !strings.Contains(listViewsResp.Result.Content[0].Text, viewName) {
		t.Fatalf("bigtable-list-logical-views output does not contain expected view %q: %v", viewName, listViewsResp.Result.Content)
	}

	// List schemas
	listSchemasResp := assertMCPSuccess(t, "bigtable-list-schemas", map[string]any{})
	if len(listSchemasResp.Result.Content) == 0 {
		t.Fatalf("bigtable-list-schemas returned empty content")
	}
	var schemas struct {
		Tables []struct {
			TableName string `json:"table_name"`
			Info      struct {
				Families    []string `json:"Families"`
				FamilyInfos []struct {
					Name string `json:"Name"`
				} `json:"FamilyInfos"`
			} `json:"info"`
		} `json:"tables"`
		LogicalViews []struct {
			LogicalViewID string `json:"LogicalViewID"`
			Name          string `json:"Name"`
			Columns       []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"columns"`
		} `json:"logical_views"`
		MaterializedViews []struct {
			MaterializedViewID string `json:"MaterializedViewID"`
			Name               string `json:"Name"`
			Columns            []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"columns"`
		} `json:"materialized_views"`
	}
	if err := json.Unmarshal([]byte(listSchemasResp.Result.Content[0].Text), &schemas); err != nil {
		t.Fatalf("failed to unmarshal bigtable-list-schemas JSON output: %v\nRaw output: %s", err, listSchemasResp.Result.Content[0].Text)
	}

	foundTable := false
	for _, tbl := range schemas.Tables {
		if tbl.TableName == tableName {
			foundTable = true
			foundCF := false
			for _, f := range tbl.Info.Families {
				if f == "cf1" {
					foundCF = true
					break
				}
			}
			for _, fi := range tbl.Info.FamilyInfos {
				if fi.Name == "cf1" {
					foundCF = true
					break
				}
			}
			if !foundCF {
				t.Fatalf("bigtable-list-schemas table %q missing expected column family 'cf1': %+v", tableName, tbl.Info)
			}
			break
		}
	}
	if !foundTable {
		t.Fatalf("bigtable-list-schemas did not return table %q in tables list", tableName)
	}

	foundView := false
	for _, v := range schemas.LogicalViews {
		if v.LogicalViewID == viewName || strings.HasSuffix(v.Name, "/logicalViews/"+viewName) {
			foundView = true
			foundKeyCol := false
			for _, col := range v.Columns {
				if col.Name == "_key" && col.Type == "BYTES" {
					foundKeyCol = true
					break
				}
			}
			if !foundKeyCol {
				t.Fatalf("bigtable-list-schemas logical view %q missing expected extracted column (_key: BYTES): %+v", viewName, v.Columns)
			}
			break
		}
	}
	if !foundView {
		t.Fatalf("bigtable-list-schemas did not return logical view %q in logical_views list", viewName)
	}

	if schemas.MaterializedViews == nil {
		t.Fatalf("bigtable-list-schemas returned nil materialized_views slice")
	}

	// List materialized views
	listMViewsResp := assertMCPSuccess(t, "bigtable-list-materialized-views", map[string]any{
		"instance_id": instanceId,
	})
	if listMViewsResp.Result.Content == nil {
		t.Fatalf("bigtable-list-materialized-views returned nil content")
	}
}

func addBigTableAdminToolsConfig(t *testing.T, config map[string]any) map[string]any {
	toolsMap, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}

	adminTools := []string{
		"bigtable-create-cluster", "bigtable-update-cluster", "bigtable-delete-cluster", "bigtable-get-cluster", "bigtable-list-clusters",
		"bigtable-create-instance", "bigtable-update-instance", "bigtable-delete-instance", "bigtable-get-instance", "bigtable-list-instances",
		"bigtable-create-table", "bigtable-update-table", "bigtable-delete-table", "bigtable-get-table", "bigtable-list-tables", "bigtable-list-schemas",
		"bigtable-create-logical-view", "bigtable-update-logical-view", "bigtable-delete-logical-view", "bigtable-get-logical-view", "bigtable-list-logical-views",
		"bigtable-create-materialized-view", "bigtable-update-materialized-view", "bigtable-delete-materialized-view", "bigtable-get-materialized-view", "bigtable-list-materialized-views",
	}

	for _, toolType := range adminTools {
		toolsMap[toolType] = map[string]any{
			"type":   toolType,
			"source": "my-instance",
		}
	}

	config["tools"] = toolsMap
	return config
}
func runBigTableAdminToolsGetTest(t *testing.T) {
	tests.RunToolGetTestByName(t, "bigtable-create-cluster",
		map[string]any{
			"bigtable-create-cluster": map[string]any{
				"description":  "Create a new Bigtable cluster in an instance.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the cluster",
						"name":         "cluster_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The zone for the cluster (e.g. us-central1-b)",
						"name":         "zone",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The number of nodes to allocate",
						"name":         "num_nodes",
						"required":     true,
						"type":         "integer",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-create-instance",
		map[string]any{
			"bigtable-create-instance": map[string]any{
				"description":  "Create a new Bigtable instance.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance to create",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Display name for the instance",
						"name":         "display_name",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the primary cluster",
						"name":         "cluster_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The zone for the cluster (e.g. us-central1-b)",
						"name":         "zone",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The number of nodes for the cluster",
						"name":         "num_nodes",
						"required":     true,
						"type":         "integer",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-create-logical-view",
		map[string]any{
			"bigtable-create-logical-view": map[string]any{
				"description":  "Create a new Bigtable logical view.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the logical view",
						"name":         "logical_view_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The logical view query",
						"name":         "query",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-create-table",
		map[string]any{
			"bigtable-create-table": map[string]any{
				"description":  "Create a new Bigtable table.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the table to create",
						"name":         "table_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Optional column family name to create with the table",
						"name":         "column_family",
						"required":     false,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-delete-cluster",
		map[string]any{
			"bigtable-delete-cluster": map[string]any{
				"description":  "Delete a Bigtable cluster.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the cluster",
						"name":         "cluster_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-delete-instance",
		map[string]any{
			"bigtable-delete-instance": map[string]any{
				"description":  "Delete a Bigtable instance.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance to delete",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-delete-logical-view",
		map[string]any{
			"bigtable-delete-logical-view": map[string]any{
				"description":  "Delete a Bigtable logical view.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the logical view",
						"name":         "logical_view_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-delete-table",
		map[string]any{
			"bigtable-delete-table": map[string]any{
				"description":  "Delete a Bigtable table.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the table to delete",
						"name":         "table_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-get-cluster",
		map[string]any{
			"bigtable-get-cluster": map[string]any{
				"description":  "Get details of a Bigtable cluster.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the cluster",
						"name":         "cluster_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-get-instance",
		map[string]any{
			"bigtable-get-instance": map[string]any{
				"description":  "Get details of a Bigtable instance.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance to get",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-get-logical-view",
		map[string]any{
			"bigtable-get-logical-view": map[string]any{
				"description":  "Get details of a Bigtable logical view.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the logical view",
						"name":         "logical_view_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-get-table",
		map[string]any{
			"bigtable-get-table": map[string]any{
				"description":  "Get details of a Bigtable table.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the table to get",
						"name":         "table_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-list-clusters",
		map[string]any{
			"bigtable-list-clusters": map[string]any{
				"description":  "List all Bigtable clusters in the instance.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-list-instances",
		map[string]any{
			"bigtable-list-instances": map[string]any{
				"description":  "List all Bigtable instances in the project.",
				"authRequired": []any{},
				"parameters":   []any{},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-list-logical-views",
		map[string]any{
			"bigtable-list-logical-views": map[string]any{
				"description":  "List all Bigtable logical views in the instance.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-list-tables",
		map[string]any{
			"bigtable-list-tables": map[string]any{
				"description":  "List all Bigtable tables in the instance.",
				"authRequired": []any{},
				"parameters":   []any{},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-update-cluster",
		map[string]any{
			"bigtable-update-cluster": map[string]any{
				"description":  "Update the number of nodes in a Bigtable cluster.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the cluster",
						"name":         "cluster_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The new number of nodes to allocate",
						"name":         "serve_nodes",
						"required":     true,
						"type":         "integer",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-update-instance",
		map[string]any{
			"bigtable-update-instance": map[string]any{
				"description":  "Update an existing Bigtable instance.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance to update",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The new display name",
						"name":         "display_name",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-update-logical-view",
		map[string]any{
			"bigtable-update-logical-view": map[string]any{
				"description":  "Update an existing Bigtable logical view.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the logical view",
						"name":         "logical_view_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The new logical view query",
						"name":         "query",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-create-materialized-view",
		map[string]any{
			"bigtable-create-materialized-view": map[string]any{
				"description":  "Create a new Bigtable materialized view.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the materialized view",
						"name":         "materialized_view_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The materialized view query",
						"name":         "query",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-delete-materialized-view",
		map[string]any{
			"bigtable-delete-materialized-view": map[string]any{
				"description":  "Delete an existing Bigtable materialized view.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the materialized view",
						"name":         "materialized_view_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-get-materialized-view",
		map[string]any{
			"bigtable-get-materialized-view": map[string]any{
				"description":  "Get information about an existing Bigtable materialized view.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the materialized view",
						"name":         "materialized_view_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-list-materialized-views",
		map[string]any{
			"bigtable-list-materialized-views": map[string]any{
				"description":  "List all existing Bigtable materialized views.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-update-materialized-view",
		map[string]any{
			"bigtable-update-materialized-view": map[string]any{
				"description":  "Update an existing Bigtable materialized view.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the instance",
						"name":         "instance_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the materialized view",
						"name":         "materialized_view_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The updated materialized view query",
						"name":         "query",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "bigtable-list-schemas",
		map[string]any{
			"bigtable-list-schemas": map[string]any{
				"description":  "List all Bigtable schemas, including tables with column family definitions, logical views, and materialized views.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"default":      float64(20),
						"description":  "Optional: The maximum number of tables to return. Default is 20",
						"name":         "limit",
						"required":     false,
						"type":         "integer",
					},
				},
			},
		})

	tests.RunToolGetTestByName(t, "bigtable-update-table",
		map[string]any{
			"bigtable-update-table": map[string]any{
				"description":  "Update an existing Bigtable table's configuration.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the table to update",
						"name":         "table_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"default":      true,
						"description":  "Disable change stream",
						"name":         "disable_change_stream",
						"required":     false,
						"type":         "boolean",
					},
				},
			},
		},
	)
}
