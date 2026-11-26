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
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/googleapis/mcp-toolbox/tests"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	BigtableSourceType = "bigtable"
	BigtableToolType   = "bigtable-sql"
	BigtableProject    = os.Getenv("BIGTABLE_PROJECT")
	BigtableInstance   = os.Getenv("BIGTABLE_INSTANCE")
)

const (
	orphanedResourceTTL         = 2 * time.Hour
	bigtableCleanupTimeout      = 5 * time.Minute
	maxConcurrentBigtableSweeps = 4
)

var integrationTablePrefixes = []string{
	"admin_test_table_",
	"auth_table_",
	"param_table_",
	"tmpl_param_table_",
}

func TestMain(m *testing.M) {
	if BigtableProject != "" && BigtableInstance != "" && os.Getenv("BIGTABLE_EMULATOR_HOST") == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		cleanupOrphanedBigtableResources(ctx)
		cancel()
	}
	os.Exit(m.Run())
}

func isBigtableNotFound(err error) bool {
	return status.Code(err) == codes.NotFound
}

func runBoundedBigtableSweep(items []string, fn func(string)) {
	switch len(items) {
	case 0:
		return
	case 1:
		fn(items[0])
		return
	}
	workers := min(len(items), maxConcurrentBigtableSweeps)
	var (
		nextIdx atomic.Int64
		wg      sync.WaitGroup
	)
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for {
				i := int(nextIdx.Add(1) - 1)
				if i >= len(items) {
					return
				}
				fn(items[i])
			}
		}()
	}
	wg.Wait()
}

func cleanupOrphanedBigtableResources(ctx context.Context) {
	adminClient, err := bigtable.NewAdminClient(ctx, BigtableProject, BigtableInstance)
	if err != nil {
		log.Printf("INTEGRATION CLEANUP: Failed to create Bigtable admin client: %v", err)
		return
	}
	defer func() { _ = adminClient.Close() }()

	instanceAdminClient, err := bigtable.NewInstanceAdminClient(ctx, BigtableProject)
	if err != nil {
		log.Printf("INTEGRATION CLEANUP: Failed to create Bigtable instance admin client: %v", err)
		return
	}
	defer func() { _ = instanceAdminClient.Close() }()

	now := time.Now()

	// Logical views must be deleted before the tables they reference.
	views, err := instanceAdminClient.LogicalViews(ctx, BigtableInstance)
	if err != nil {
		log.Printf("INTEGRATION CLEANUP: Failed to list Bigtable logical views: %v", err)
	} else {
		var staleViews []string
		for _, view := range views {
			if shouldCleanupBigtableResource(view.LogicalViewID, now, "admin_test_view_") {
				staleViews = append(staleViews, view.LogicalViewID)
			}
		}
		runBoundedBigtableSweep(staleViews, func(viewID string) {
			log.Printf("INTEGRATION CLEANUP: Deleting orphaned logical view %s", viewID)
			if err := instanceAdminClient.DeleteLogicalView(ctx, BigtableInstance, viewID); err != nil && !isBigtableNotFound(err) {
				log.Printf("INTEGRATION CLEANUP: Failed to delete logical view %s: %v", viewID, err)
			}
		})
	}

	tables, err := adminClient.Tables(ctx)
	if err != nil {
		log.Printf("INTEGRATION CLEANUP: Failed to list Bigtable tables: %v", err)
	} else {
		var staleTables []string
		for _, table := range tables {
			if shouldCleanupBigtableResource(table, now, integrationTablePrefixes...) {
				staleTables = append(staleTables, table)
			}
		}
		runBoundedBigtableSweep(staleTables, func(table string) {
			log.Printf("INTEGRATION CLEANUP: Deleting orphaned table %s", table)
			if err := adminClient.DeleteTable(ctx, table); err != nil && !isBigtableNotFound(err) {
				log.Printf("INTEGRATION CLEANUP: Failed to delete table %s: %v", table, err)
			}
		})
	}

	instances, err := instanceAdminClient.Instances(ctx)
	if err != nil {
		log.Printf("INTEGRATION CLEANUP: Failed to list Bigtable instances: %v", err)
		return
	}
	var staleInstances []string
	for _, instance := range instances {
		instanceID := instance.Name
		if idx := strings.LastIndex(instanceID, "/"); idx >= 0 {
			instanceID = instanceID[idx+1:]
		}
		if shouldCleanupBigtableResource(instanceID, now, "testi-") {
			staleInstances = append(staleInstances, instanceID)
		}
	}
	runBoundedBigtableSweep(staleInstances, func(instanceID string) {
		log.Printf("INTEGRATION CLEANUP: Deleting orphaned instance %s", instanceID)
		if err := instanceAdminClient.DeleteInstance(ctx, instanceID); err != nil && !isBigtableNotFound(err) {
			log.Printf("INTEGRATION CLEANUP: Failed to delete instance %s: %v", instanceID, err)
		}
	})
}

func shouldCleanupBigtableResource(name string, now time.Time, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if !strings.HasPrefix(name, prefix) {
			continue
		}

		suffix := strings.TrimPrefix(name, prefix)
		separator := strings.IndexAny(suffix, "_-")
		if separator < 0 {
			// Resources created before timestamps were added are already orphaned.
			return true
		}
		createdAt, err := strconv.ParseInt(suffix[:separator], 10, 64)
		if err != nil {
			// Clean up legacy integration-test resources whose UUID immediately
			// followed the prefix and therefore cannot be age checked.
			return true
		}
		return now.Sub(time.Unix(createdAt, 0)) > orphanedResourceTTL
	}
	return false
}

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

	resourceID := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	createdAt := strconv.FormatInt(time.Now().Unix(), 10)
	t.Logf("Starting Bigtable test with resourceID: %s", resourceID)

	args := []string{"--enable-api"}

	// Initialize AdminClient to create or delete tables
	adminClient, err := bigtable.NewAdminClient(context.Background(), sourceConfig["project"].(string), sourceConfig["instance"].(string))
	if err != nil {
		t.Fatalf("Failed to create AdminClient: %v", err)
	}

	t.Cleanup(func() {
		adminClient.Close()
	})

	instanceAdminClient, err := bigtable.NewInstanceAdminClient(context.Background(), sourceConfig["project"].(string))
	if err != nil {
		t.Fatalf("Failed to create InstanceAdminClient: %v", err)
	}
	t.Cleanup(func() {
		instanceAdminClient.Close()
	})

	t.Cleanup(func() {
		t.Logf("Running global cleanup for resourceID: %s", resourceID)
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), bigtableCleanupTimeout)
		defer cleanupCancel()
		tests.CleanupBigtableTables(t, cleanupCtx, adminClient, resourceID)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	tableName := "param_table_" + createdAt + "_" + resourceID
	tableNameAuth := "auth_table_" + createdAt + "_" + resourceID
	tableNameTemplateParam := "tmpl_param_table_" + createdAt + "_" + resourceID

	columnFamilyName := "cf"
	muts, rowKeys := getTestData(columnFamilyName)
	mutsTmpl, rowKeysTmpl := getTestDataTemplateParam(columnFamilyName)

	// Do not change the shape of statement without checking tests/common_test.go.
	// The structure and value of seed data has to match https://github.com/googleapis/mcp-toolbox/blob/4dba0df12dc438eca3cb476ef52aa17cdf232c12/tests/common_test.go#L200-L251
	paramTestStatement := fmt.Sprintf("SELECT TO_INT64(cf['id']) as id, CAST(cf['name'] AS string) as name, FROM %s WHERE TO_INT64(cf['id']) = @id OR CAST(cf['name'] AS string) = @name;", tableName)
	idParamTestStatement := fmt.Sprintf("SELECT TO_INT64(cf['id']) as id, CAST(cf['name'] AS string) as name, FROM %s WHERE TO_INT64(cf['id']) = @id;", tableName)
	nameParamTestStatement := fmt.Sprintf("SELECT TO_INT64(cf['id']) as id, CAST(cf['name'] AS string) as name, FROM %s WHERE CAST(cf['name'] AS string) = @name;", tableName)
	arrayTestStatement := fmt.Sprintf(
		"SELECT TO_INT64(cf['id']) AS id, CAST(cf['name'] AS string) AS name FROM %s WHERE TO_INT64(cf['id']) IN UNNEST(@idArray) AND CAST(cf['name'] AS string) IN UNNEST(@nameArray);",
		tableName,
	)
	authToolStatement := fmt.Sprintf("SELECT CAST(cf['name'] AS string) as name FROM %s WHERE CAST(cf['email'] AS string) = @email;", tableNameAuth)

	seedBigtableTablesConcurrently(t, ctx, adminClient, sourceConfig["project"].(string), sourceConfig["instance"].(string), columnFamilyName, []btTableSeed{
		{tableName: tableName, muts: muts, rowKeys: rowKeys},
		{tableName: tableNameAuth, muts: muts, rowKeys: rowKeys},
		{tableName: tableNameTemplateParam, muts: mutsTmpl, rowKeys: rowKeysTmpl},
	})

	// Write config into a file and pass it to command
	toolsFile := tests.GetToolsConfig(sourceConfig, BigtableToolType, paramTestStatement, idParamTestStatement, nameParamTestStatement, arrayTestStatement, authToolStatement)
	toolsFile = addTemplateParamConfig(t, toolsFile)
	toolsFile = addBigTableAdminToolsConfig(t, toolsFile)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)

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
		runBigTableAdminToolsTest(t, sourceConfig["instance"].(string), adminClient, instanceAdminClient)
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
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(int64(v)))
	return buf[:]
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

type btTableSeed struct {
	tableName string
	muts      []*bigtable.Mutation
	rowKeys   []string
}

func seedOneBigtableTable(ctx context.Context, adminClient *bigtable.AdminClient, dataClient *bigtable.Client, tableName, columnFamilyName string, muts []*bigtable.Mutation, rowKeys []string) error {
	conf := &bigtable.TableConf{
		TableID: tableName,
		Families: map[string]bigtable.GCPolicy{
			columnFamilyName: bigtable.NoGcPolicy(),
		},
	}
	if err := adminClient.CreateTableFromConf(ctx, conf); err != nil && status.Code(err) != codes.AlreadyExists {
		return fmt.Errorf("create table %s: %w", tableName, err)
	}

	rowErrs, err := dataClient.Open(tableName).ApplyBulk(ctx, rowKeys, muts)
	if err != nil {
		return fmt.Errorf("apply bulk mutations to %s: %w", tableName, err)
	}
	if len(rowErrs) > 0 {
		return fmt.Errorf("apply bulk row errors on %s: %w", tableName, errors.Join(rowErrs...))
	}
	return nil
}

// seedBigtableTablesConcurrently shares a single data client and creates/seeds
// independent tables concurrently using single-RPC CreateTableFromConf calls.
func seedBigtableTablesConcurrently(t *testing.T, ctx context.Context, adminClient *bigtable.AdminClient, projectID, instance, columnFamilyName string, seeds []btTableSeed) {
	t.Helper()
	dataClient, err := bigtable.NewClient(ctx, projectID, instance)
	if err != nil {
		t.Fatalf("Could not create data operations client: %v", err)
	}
	defer func() { _ = dataClient.Close() }()

	var (
		errsMu sync.Mutex
		errs   []error
		wg     sync.WaitGroup
	)
	wg.Add(len(seeds))
	for _, s := range seeds {
		go func(seed btTableSeed) {
			defer wg.Done()
			if err := seedOneBigtableTable(ctx, adminClient, dataClient, seed.tableName, columnFamilyName, seed.muts, seed.rowKeys); err != nil {
				errsMu.Lock()
				errs = append(errs, err)
				errsMu.Unlock()
			}
		}(s)
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatalf("Failed seeding Bigtable tables: %v", err)
	}
}

func setupBtTable(t *testing.T, adminClient *bigtable.AdminClient, ctx context.Context, projectId string, instance string, tableName string, columnFamilyName string, muts []*bigtable.Mutation, rowKeys []string) {
	t.Helper()
	seedBigtableTablesConcurrently(t, ctx, adminClient, projectId, instance, columnFamilyName, []btTableSeed{
		{tableName: tableName, muts: muts, rowKeys: rowKeys},
	})
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

// registerBigtableCleanup registers a t.Cleanup that deletes a Bigtable
// resource under a fresh bounded context, ignoring NotFound so it can be
// registered before issuing the create call.
func registerBigtableCleanup(t *testing.T, kind, name string, deleteFn func(context.Context) error) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), bigtableCleanupTimeout)
		defer cancel()
		if err := deleteFn(ctx); err != nil && !isBigtableNotFound(err) {
			t.Errorf("cleanup: failed to delete Bigtable %s %q: %v", kind, name, err)
		}
	})
}

// runBigTableAdminToolsTest verifies all 20 Bigtable admin lifecycle tools for instances, clusters, tables, and logical views.
func runBigTableAdminToolsTest(t *testing.T, instanceId string, adminClient *bigtable.AdminClient, instanceAdminClient *bigtable.InstanceAdminClient) {
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	createdAt := strconv.FormatInt(time.Now().Unix(), 10)
	tableName := "admin_test_table_" + createdAt + "_" + uniqueID[:8]
	viewName := "admin_test_view_" + createdAt + "_" + uniqueID[:8]

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
	testInstId := "testi-" + createdAt + "-" + uniqueID[:8]
	testClusterId1 := "testc1-" + uniqueID[:8]
	// Register cleanup before creating so response-assertion or timeout failures
	// never leak the instance or its clusters.
	registerBigtableCleanup(t, "instance", testInstId, func(ctx context.Context) error {
		return instanceAdminClient.DeleteInstance(ctx, testInstId)
	})
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

	// Create table (register cleanup before issuing create).
	registerBigtableCleanup(t, "table", tableName, func(ctx context.Context) error {
		return adminClient.DeleteTable(ctx, tableName)
	})
	createTableResp := assertMCPSuccess(t, "bigtable-create-table", map[string]any{
		"table_id":      tableName,
		"column_family": "cf1",
	})
	if len(createTableResp.Result.Content) == 0 || !strings.Contains(createTableResp.Result.Content[0].Text, "table created successfully") {
		t.Fatalf("bigtable-create-table unexpected output: %v", createTableResp.Result.Content)
	}

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

	// Create logical view. Registered after tableName so LIFO deletes the view
	// before deleting its underlying table.
	registerBigtableCleanup(t, "logical view", viewName, func(ctx context.Context) error {
		return instanceAdminClient.DeleteLogicalView(ctx, instanceId, viewName)
	})
	createViewResp := assertMCPSuccess(t, "bigtable-create-logical-view", map[string]any{
		"instance_id":     instanceId,
		"logical_view_id": viewName,
		"query":           "SELECT _key FROM " + tableName,
	})
	if len(createViewResp.Result.Content) == 0 || !strings.Contains(createViewResp.Result.Content[0].Text, "logical view created successfully") {
		t.Fatalf("bigtable-create-logical-view unexpected output: %v", createViewResp.Result.Content)
	}

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
