// Copyright 2024 Google LLC
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

package couchbase

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/couchbase/gocb/v2"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	tccouchbase "github.com/testcontainers/testcontainers-go/modules/couchbase"
)

const (
	couchbaseSourceType = "couchbase"
	couchbaseToolType   = "couchbase-sql"
	defaultBucketName   = "test-bucket"
	defaultUser         = "Administrator"
	defaultPass         = "password"
)

// getCouchbaseVars generates config using dynamic container info
func getCouchbaseVars(connectionString string) map[string]any {
	return map[string]any{
		"type":                 couchbaseSourceType,
		"connectionString":     connectionString,
		"bucket":               defaultBucketName,
		"scope":                "_default", // Testcontainers default
		"username":             defaultUser,
		"password":             defaultPass,
		"queryScanConsistency": 2,
	}
}

// initCouchbaseCluster initializes a connection to the Couchbase cluster
func initCouchbaseCluster(connectionString, username, password string) (*gocb.Cluster, error) {
	opts := gocb.ClusterOptions{
		Authenticator: gocb.PasswordAuthenticator{
			Username: username,
			Password: password,
		},
	}
	cluster, err := gocb.Connect(connectionString, opts)
	if err != nil {
		return nil, fmt.Errorf("gocb.Connect: %w", err)
	}
	return cluster, nil
}

func TestCouchbaseToolEndpoints(t *testing.T) {
	collectionName := setupCouchbaseTest(t, "--enable-api")
	t.Run("discovery", tests.RunToolGetTest)
	runCouchbaseCallTests(t, collectionName, nil, nil)
}

func setupCouchbaseTest(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	// Start Couchbase Container
	cbContainer, err := tccouchbase.Run(ctx, "couchbase/server:7.2.0",
		tccouchbase.WithAdminCredentials(defaultUser, defaultPass),
		tccouchbase.WithBuckets(tccouchbase.NewBucket(defaultBucketName)),
	)
	if err != nil {
		t.Fatalf("failed to start couchbase container: %s", err)
	}
	t.Cleanup(func() {
		if err := cbContainer.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate container: %s", err)
		}
	})

	connectionString, err := cbContainer.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %s", err)
	}

	// Set up Couchbase cluster
	cluster, err := initCouchbaseCluster(connectionString, defaultUser, defaultPass)
	if err != nil {
		t.Fatalf("unable to create Couchbase connection: %s", err)
	}
	t.Cleanup(func() {
		if err := cluster.Close(nil); err != nil {
			t.Errorf("failed to close Couchbase cluster: %v", err)
		}
	})

	sourceConfig := getCouchbaseVars(connectionString)
	scopeName := "_default"

	// Prepare Data
	collectionNameParam := "param_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	collectionNameAuth := "auth_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	collectionNameTemplateParam := "template_param_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, paramTestParams := getCouchbaseParamToolInfo(collectionNameParam)
	teardown1 := setupCouchbaseCollection(t, ctx, cluster, defaultBucketName, scopeName, collectionNameParam, paramTestParams)
	t.Cleanup(func() { teardown1(t) })

	authToolStmt, authTestParams := getCouchbaseAuthToolInfo(collectionNameAuth)
	teardown2 := setupCouchbaseCollection(t, ctx, cluster, defaultBucketName, scopeName, collectionNameAuth, authTestParams)
	t.Cleanup(func() { teardown2(t) })

	tmplSelectCombined, tmplSelectFilterCombined, tmplSelectAll, params3 := getCouchbaseTemplateParamToolInfo()
	teardown3 := setupCouchbaseCollection(t, ctx, cluster, defaultBucketName, scopeName, collectionNameTemplateParam, params3)
	t.Cleanup(func() { teardown3(t) })

	// Configure Toolbox
	toolsFile := tests.GetToolsConfig(sourceConfig, couchbaseToolType, paramToolStmt, idParamToolStmt, nameParamToolStmt, arrayToolStmt, authToolStmt)
	toolsFile = tests.AddTemplateParamConfig(t, toolsFile, couchbaseToolType, tmplSelectCombined, tmplSelectFilterCombined, tmplSelectAll)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization failed: %s", err)
	}
	t.Cleanup(func() {
		cmd.Stop()
		waitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := cmd.Wait(waitCtx); err != nil {
			t.Errorf("toolbox shutdown: %v", err)
		}
		cmd.Close()
		cleanup()
	})

	// Wait for server
	waitCtx, waitCancel := context.WithTimeout(ctx, 20*time.Second)
	defer waitCancel()
	if _, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out); err != nil {
		t.Fatalf("toolbox didn't start: %s", err)
	}

	return collectionNameTemplateParam
}

func runCouchbaseCallTests(t *testing.T, collectionName string, invokeOptions []tests.InvokeTestOption, templateOptions []tests.TemplateParamOption) {
	t.Helper()
	select1Want := "[{\"$1\":1}]"
	mcpMyFailToolWant := `{"jsonrpc":"2.0","id":"invoke-fail-tool","result":{"content":[{"type":"text","text":"error processing request: unable to execute query: parsing failure | {\"statement\":\"SELEC 1;\"`
	mcpSelect1Want := `{"jsonrpc":"2.0","id":"invoke my-auth-required-tool","result":{"content":[{"type":"text","text":"{\"$1\":1}"}]}}`
	tmplSelectId1Want := "[{\"age\":21,\"id\":1,\"name\":\"Alex\"}]"
	selectAllWant := "[{\"age\":21,\"id\":1,\"name\":\"Alex\"},{\"age\":100,\"id\":2,\"name\":\"Alice\"}]"

	t.Run("invoke", func(t *testing.T) {
		tests.RunToolInvokeTest(t, select1Want, invokeOptions...)
	})
	t.Run("mcp_call", func(t *testing.T) {
		tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want)
	})
	t.Run("template_parameters", func(t *testing.T) {
		opts := []tests.TemplateParamOption{
			tests.WithTmplSelectId1Want(tmplSelectId1Want),
			tests.WithSelectAllWant(selectAllWant),
			tests.DisableDdlTest(),
			tests.DisableInsertTest(),
		}
		tests.RunToolInvokeWithTemplateParameters(t, collectionName, append(opts, templateOptions...)...)
	})
}

// setupCouchbaseCollection creates a scope and collection and inserts test data
func setupCouchbaseCollection(t *testing.T, ctx context.Context, cluster *gocb.Cluster,
	bucketName, scopeName, collectionName string, params []map[string]any) func(t *testing.T) {

	// Get bucket reference
	bucket := cluster.Bucket(bucketName)

	// Wait for bucket to be ready
	err := bucket.WaitUntilReady(5*time.Second, nil)
	if err != nil {
		t.Fatalf("failed to connect to bucket: %v", err)
	}

	// Create scope if it doesn't exist
	bucketMgr := bucket.CollectionsV2()
	err = bucketMgr.CreateScope(scopeName, nil)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Logf("failed to create scope (might already exist): %v", err)
	}

	// Create a collection if it doesn't exist
	err = bucketMgr.CreateCollection(scopeName, collectionName, nil, nil)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("failed to create collection: %v", err)
	}

	// Get a reference to the collection
	collection := bucket.Scope(scopeName).Collection(collectionName)

	// Create primary index if it doesn't exist
	// Create primary index with retry logic
	maxRetries := 5
	retryDelay := 50 * time.Millisecond
	actualRetries := 0
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err = collection.QueryIndexes().CreatePrimaryIndex(
			&gocb.CreatePrimaryQueryIndexOptions{
				IgnoreIfExists: true,
			})
		if err == nil {
			lastErr = err // clear previous error
			break
		}

		lastErr = err
		t.Logf("Attempt %d: failed to create primary index: %v, retrying in %v", attempt+1, err, retryDelay)
		time.Sleep(retryDelay)
		// Exponential backoff
		retryDelay *= 2
		actualRetries += 1
	}

	if lastErr != nil {
		t.Fatalf("failed to create primary index collection after %d attempts: %v", actualRetries, lastErr)
	}

	// Insert test documents
	for i, param := range params {
		_, err = collection.Upsert(fmt.Sprintf("%d", i+1), param, &gocb.UpsertOptions{
			DurabilityLevel: gocb.DurabilityLevelMajority,
		})
		if err != nil {
			t.Fatalf("failed to insert test data: %v", err)
		}
	}

	// Return a cleanup function
	return func(t *testing.T) {
		// Drop the collection
		err := bucketMgr.DropCollection(scopeName, collectionName, nil)
		if err != nil {
			t.Logf("failed to drop collection: %v", err)
		}
	}
}

// getCouchbaseParamToolInfo returns statements and params for my-tool couchbase-sql type
func getCouchbaseParamToolInfo(collectionName string) (string, string, string, string, []map[string]any) {
	// N1QL uses positional or named parameters with $ prefix
	toolStatement := fmt.Sprintf("SELECT TONUMBER(meta().id) as id, "+
		"%s.* FROM %s WHERE meta().id = TOSTRING($id) OR name = $name order by meta().id",
		collectionName, collectionName)
	idToolStatement := fmt.Sprintf("SELECT TONUMBER(meta().id) as id, "+
		"%s.* FROM %s WHERE meta().id = TOSTRING($id) order by meta().id",
		collectionName, collectionName)
	nameToolStatement := fmt.Sprintf("SELECT TONUMBER(meta().id) as id, "+
		"%s.* FROM %s WHERE name = $name order by meta().id",
		collectionName, collectionName)
	arrayToolStatemnt := fmt.Sprintf("SELECT TONUMBER(meta().id) as id, "+
		"%s.* FROM %s WHERE TONUMBER(meta().id) IN $idArray AND name IN $nameArray order by meta().id", collectionName, collectionName)
	params := []map[string]any{
		{"name": "Alice"},
		{"name": "Jane"},
		{"name": "Sid"},
		{"name": nil},
	}
	return toolStatement, idToolStatement, nameToolStatement, arrayToolStatemnt, params
}

// getCouchbaseAuthToolInfo returns statements and param of my-auth-tool for couchbase-sql type
func getCouchbaseAuthToolInfo(collectionName string) (string, []map[string]any) {
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = $email", collectionName)

	params := []map[string]any{
		{"name": "Alice", "email": tests.ServiceAccountEmail},
		{"name": "Jane", "email": "janedoe@gmail.com"},
	}
	return toolStatement, params
}

func getCouchbaseTemplateParamToolInfo() (string, string, string, []map[string]any) {
	tmplSelectCombined := "SELECT {{.tableName}}.* FROM {{.tableName}} WHERE id = $id"
	tmplSelectFilterCombined := "SELECT {{.tableName}}.* FROM {{.tableName}} WHERE {{.columnFilter}} = $name"
	tmplSelectAll := "SELECT {{.tableName}}.* FROM {{.tableName}}"

	params := []map[string]any{
		{"name": "Alex", "id": 1, "age": 21},
		{"name": "Alice", "id": 2, "age": 100},
	}
	return tmplSelectCombined, tmplSelectFilterCombined, tmplSelectAll, params
}
