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

package spanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	SpannerSourceType = "spanner"
	SpannerToolType   = "spanner-sql"
)

// getSpannerToolInfo returns statements and param for my-tool for spanner-sql type
func getSpannerParamToolInfo(tableName string) (string, string, string, string, string, string, map[string]any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX)) PRIMARY KEY (id)", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (id, name) VALUES (1, @name1), (2, @name2), (3, @name3), (4, @name4)", tableName)
	toolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = @id OR name = @name", tableName)
	idToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id = @id", tableName)
	nameToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE name = @name", tableName)
	arrayToolStatement := fmt.Sprintf("SELECT * FROM %s WHERE id IN UNNEST(@idArray) AND name IN UNNEST(@nameArray)", tableName)
	params := map[string]any{"name1": "Alice", "name2": "Jane", "name3": "Sid", "name4": nil}
	return createStatement, insertStatement, toolStatement, idToolStatement, nameToolStatement, arrayToolStatement, params
}

// getSpannerAuthToolInfo returns statements and param of my-auth-tool for spanner-sql type
func getSpannerAuthToolInfo(tableName string) (string, string, string, map[string]any) {
	createStatement := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING(MAX), email STRING(MAX)) PRIMARY KEY (id)", tableName)
	insertStatement := fmt.Sprintf("INSERT INTO %s (id, name, email) VALUES (1, @name1, @email1), (2, @name2, @email2)", tableName)
	toolStatement := fmt.Sprintf("SELECT name FROM %s WHERE email = @email", tableName)
	params := map[string]any{
		"name1":  "Alice",
		"email1": tests.ServiceAccountEmail,
		"name2":  "Jane",
		"email2": "janedoe@gmail.com",
	}
	return createStatement, insertStatement, toolStatement, params
}

// setupSpannerTable creates and inserts data into a table of tool
// compatible with spanner-sql tool
func setupSpannerTable(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, dataClient *spanner.Client, createStatement, insertStatement, tableName, dbString string, params map[string]any) func(*testing.T) {

	// Create table
	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbString,
		Statements: []string{createStatement},
	})
	if err != nil {
		t.Fatalf("unable to start create table operation %s: %s", tableName, err)
	}
	err = op.Wait(ctx)
	if err != nil {
		t.Fatalf("unable to create test table %s: %s", tableName, err)
	}

	// Insert test data
	if insertStatement != "" {
		_, err = dataClient.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			stmt := spanner.Statement{
				SQL:    insertStatement,
				Params: params,
			}
			_, err := txn.Update(ctx, stmt)
			return err
		})
		if err != nil {
			t.Fatalf("unable to insert test data: %s", err)
		}
	}

	return func(t *testing.T) {
		// tear down test
		op, err = adminClient.UpdateDatabaseDdl(context.WithoutCancel(ctx), &databasepb.UpdateDatabaseDdlRequest{
			Database:   dbString,
			Statements: []string{fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)},
		})
		if err != nil {
			t.Errorf("unable to start drop %s operation: %s", tableName, err)
			return
		}

		opErr := op.Wait(context.WithoutCancel(ctx))
		if opErr != nil {
			t.Errorf("Teardown failed: %s", opErr)
		}
	}
}

// setupSpannerGraph creates a graph and inserts data into it.
func setupSpannerGraph(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, createStatement, graphName, dbString string) func(*testing.T) {
	// Create graph
	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbString,
		Statements: []string{createStatement},
	})
	if err != nil {
		t.Fatalf("unable to start create graph operation %s: %s", graphName, err)
	}
	err = op.Wait(ctx)
	if err != nil {
		t.Fatalf("unable to create test graph %s: %s", graphName, err)
	}

	return func(t *testing.T) {
		// tear down test
		op, err = adminClient.UpdateDatabaseDdl(context.WithoutCancel(ctx), &databasepb.UpdateDatabaseDdlRequest{
			Database:   dbString,
			Statements: []string{fmt.Sprintf("DROP PROPERTY GRAPH IF EXISTS %s", graphName)},
		})
		if err != nil {
			t.Errorf("unable to start drop %s operation: %s", graphName, err)
			return
		}

		opErr := op.Wait(context.WithoutCancel(ctx))
		if opErr != nil {
			t.Errorf("Teardown failed: %s", opErr)
		}
	}
}

// addSpannerExecuteSqlConfig gets the tools config for `spanner-execute-sql`
func addSpannerExecuteSqlConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["my-exec-sql-tool-read-only"] = map[string]any{
		"type":        "spanner-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"readOnly":    true,
	}
	tools["my-exec-sql-tool"] = map[string]any{
		"type":        "spanner-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
	}
	tools["my-auth-exec-sql-tool"] = map[string]any{
		"type":        "spanner-execute-sql",
		"source":      "my-instance",
		"description": "Tool to execute sql",
		"authRequired": []string{
			"my-google-auth",
		},
	}
	config["tools"] = tools
	return config
}

func addSpannerReadOnlyConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	tools["access-schema-read-only"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Tool to access information schema in read-only mode.",
		"statement":   "SELECT schema_name FROM `INFORMATION_SCHEMA`.SCHEMATA WHERE schema_name='INFORMATION_SCHEMA';",
		"readOnly":    true,
	}
	tools["access-schema"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Tool to access information schema.",
		"statement":   "SELECT schema_name FROM `INFORMATION_SCHEMA`.SCHEMATA WHERE schema_name='INFORMATION_SCHEMA';",
	}
	config["tools"] = tools
	return config
}

// addSpannerListTablesConfig adds the spanner-list-tables tool configuration
func addSpannerListTablesConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}

	// Add spanner-list-tables tool
	tools["list-tables-tool"] = map[string]any{
		"type":        "spanner-list-tables",
		"source":      "my-instance",
		"description": "Lists tables with their schema information",
	}

	config["tools"] = tools
	return config
}

// addSpannerListGraphsConfig adds the spanner-list-graphs tool configuration
func addSpannerListGraphsConfig(t *testing.T, config map[string]any) map[string]any {
	tools, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}

	// Add spanner-list-graphs tool
	tools["list-graphs-tool"] = map[string]any{
		"type":        "spanner-list-graphs",
		"source":      "my-instance",
		"description": "Lists graphs with their schema information",
	}

	config["tools"] = tools
	return config
}

func addTemplateParamConfig(t *testing.T, config map[string]any) map[string]any {
	toolsMap, ok := config["tools"].(map[string]any)
	if !ok {
		t.Fatalf("unable to get tools from config")
	}
	toolsMap["insert-table-templateParams-tool"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Insert tool with template parameters",
		"statement":   "INSERT INTO {{.tableName}} ({{array .columns}}) VALUES ({{.values}})",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewArrayParameter("columns", "The columns to insert into", parameters.NewStringParameter("column", "A column name that will be returned from the query.")),
			parameters.NewStringParameter("values", "The values to insert as a comma separated string"),
		},
	}
	toolsMap["select-templateParams-tool"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT * FROM {{.tableName}}",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-templateParams-combined-tool"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT * FROM {{.tableName}} WHERE id = @id",
		"parameters":  []parameters.Parameter{parameters.NewIntParameter("id", "the id of the user")},
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
		},
	}
	toolsMap["select-fields-templateParams-tool"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT {{array .fields}} FROM {{.tableName}}",
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewArrayParameter("fields", "The fields to select from", parameters.NewStringParameter("field", "A field that will be returned from the query.")),
		},
	}
	toolsMap["select-filter-templateParams-combined-tool"] = map[string]any{
		"type":        "spanner-sql",
		"source":      "my-instance",
		"description": "Create table tool with template parameters",
		"statement":   "SELECT * FROM {{.tableName}} WHERE {{.columnFilter}} = @name",
		"parameters":  []parameters.Parameter{parameters.NewStringParameter("name", "the name of the user")},
		"templateParameters": []parameters.Parameter{
			parameters.NewStringParameter("tableName", "some description"),
			parameters.NewStringParameter("columnFilter", "some description"),
		},
	}
	config["tools"] = toolsMap
	return config
}

func runSpannerExecuteSqlToolInvokeTest(t *testing.T, select1Want, invokeParamWant, tableNameParam string, opts ...tests.ToolExecOption) {
	config := &tests.ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	// Get ID token
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	sqlArgs := func(sql string) map[string]any { return map[string]any{"sql": sql} }

	// Test tool invoke endpoint
	invokeTcs := []struct {
		name          string
		toolName      string
		requestHeader map[string]string
		args          map[string]any
		want          string
		isErr         bool
	}{
		{
			name:          "invoke my-exec-sql-tool-read-only",
			toolName:      "my-exec-sql-tool-read-only",
			requestHeader: map[string]string{},
			args:          sqlArgs("SELECT 1"),
			want:          select1Want,
			isErr:         false,
		},
		{
			name:          "invoke my-exec-sql-tool-read-only with data present in table",
			toolName:      "my-exec-sql-tool-read-only",
			requestHeader: map[string]string{},
			args:          sqlArgs(fmt.Sprintf("SELECT * FROM %s WHERE id = 3 OR name = 'Alice'", tableNameParam)),
			want:          invokeParamWant,
			isErr:         false,
		},
		{
			name:          "invoke my-exec-sql-tool-read-only create table",
			toolName:      "my-exec-sql-tool-read-only",
			requestHeader: map[string]string{},
			args:          sqlArgs("CREATE TABLE t (id SERIAL PRIMARY KEY, name TEXT)"),
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool-read-only drop table",
			toolName:      "my-exec-sql-tool-read-only",
			requestHeader: map[string]string{},
			args:          sqlArgs("DROP TABLE t"),
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool-read-only insert entry",
			toolName:      "my-exec-sql-tool-read-only",
			requestHeader: map[string]string{},
			args:          sqlArgs(fmt.Sprintf("INSERT INTO %s (id, name) VALUES (4, 'test_name')", tableNameParam)),
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool without body",
			toolName:      "my-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{},
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool",
			toolName:      "my-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          sqlArgs("SELECT 1"),
			want:          select1Want,
			isErr:         false,
		},
		{
			name:          "invoke my-exec-sql-tool create table",
			toolName:      "my-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          sqlArgs("CREATE TABLE t (id SERIAL PRIMARY KEY, name TEXT)"),
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool drop table",
			toolName:      "my-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          sqlArgs("DROP TABLE t"),
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool insert entry",
			toolName:      "my-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          sqlArgs(fmt.Sprintf("INSERT INTO %s (id, name) VALUES (5, 'test_name')", tableNameParam)),
			want:          "[]",
			isErr:         false,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool with auth token",
			toolName:      "my-auth-exec-sql-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          sqlArgs("SELECT 1"),
			isErr:         false,
			want:          select1Want,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool with invalid auth token",
			toolName:      "my-auth-exec-sql-tool",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			args:          sqlArgs("SELECT 1"),
			isErr:         true,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool without auth token",
			toolName:      "my-auth-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          sqlArgs("SELECT 1"),
			isErr:         true,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var got string

			if config.IsMCP() {
				statusCode, mcpResp, err := tests.InvokeMCPTool(t, tc.toolName, tc.args, tc.requestHeader)
				if statusCode != http.StatusOK || err != nil || mcpResp == nil {
					if tc.isErr {
						return
					}
					t.Fatalf("response status code is not 200, got %d: %v", statusCode, err)
				}
				// A tool or auth failure is a 200 with isError set, so it has
				// to be inspected rather than inferred from the status code.
				if mcpResp.Error != nil || mcpResp.Result.IsError {
					if tc.isErr {
						return
					}
					t.Fatalf("%s returned an error result: %+v", tc.toolName, mcpResp.Result.Content)
				}
				got = tests.GetMCPResultJSON(t, mcpResp)
			} else {
				api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
				reqBytes, err := json.Marshal(tc.args)
				if err != nil {
					t.Fatalf("unable to marshal args: %s", err)
				}
				// Send Tool invocation request
				req, err := http.NewRequest(http.MethodPost, api, bytes.NewBuffer(reqBytes))
				if err != nil {
					t.Fatalf("unable to create request: %s", err)
				}
				req.Header.Add("Content-type", "application/json")
				for k, v := range tc.requestHeader {
					req.Header.Add(k, v)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("unable to send request: %s", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					if tc.isErr {
						return
					}
					bodyBytes, _ := io.ReadAll(resp.Body)
					t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
				}

				// Check response body
				var body map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("error parsing response body")
				}

				result, ok := body["result"].(string)
				if !ok {
					t.Fatalf("unable to find result in response body")
				}
				got = result
			}

			// Database errors are returned with a 200 status and an error in
			// the result.
			if tc.isErr {
				if !strings.Contains(got, `"error"`) {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}

			if got != tc.want {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}

// Helper function to verify table list results
func verifyTableListResult(t *testing.T, body map[string]interface{}, expectedTables []string, expectedSimpleFormat bool) {
	// Parse the result
	result, ok := body["result"].(string)
	if !ok {
		t.Fatalf("unable to find result in response body")
	}

	var tables []interface{}
	err := json.Unmarshal([]byte(result), &tables)
	if err != nil {
		t.Fatalf("unable to parse result as JSON array: %s", err)
	}

	// If we expect specific tables, verify they exist
	if len(expectedTables) > 0 {
		tableNames := make(map[string]bool)
		requiredKeys := []string{"schema_name", "object_name", "object_type", "columns", "constraints", "indexes"}
		if expectedSimpleFormat {
			requiredKeys = []string{"name"}
		}

		for _, table := range tables {
			tableMap, ok := table.(map[string]interface{})
			if !ok {
				continue
			}

			objectDetails, ok := tableMap["object_details"].(map[string]interface{})
			if !ok {
				t.Fatalf("object_details is not of type map[string]interface{}, got: %T", tableMap["object_details"])
			}

			for _, reqKey := range requiredKeys {
				if _, hasKey := objectDetails[reqKey]; !hasKey {
					t.Errorf("missing required key '%s', for object_details: %v", reqKey, objectDetails)
				}
			}

			if name, ok := tableMap["object_name"].(string); ok {
				tableNames[name] = true
			}
		}

		for _, expected := range expectedTables {
			if !tableNames[expected] {
				t.Errorf("expected table %s not found in results", expected)
			}
		}
	}
}

// runSpannerListTablesTest tests the spanner-list-tables tool
func runSpannerListTablesTest(t *testing.T, tableNameParam, tableNameAuth, tableNameTemplateParam string, opts ...tests.ToolExecOption) {
	config := &tests.ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	invokeTcs := []struct {
		name            string
		args            map[string]any
		expectedTables  []string // empty means don't check specific tables
		useSimpleFormat bool
	}{
		{
			name:           "list all tables with detailed format",
			args:           map[string]any{},
			expectedTables: []string{tableNameParam, tableNameAuth, tableNameTemplateParam},
		},
		{
			name:            "list tables with simple format",
			args:            map[string]any{"output_format": "simple"},
			expectedTables:  []string{tableNameParam, tableNameAuth, tableNameTemplateParam},
			useSimpleFormat: true,
		},
		{
			name:           "list specific tables",
			args:           map[string]any{"table_names": fmt.Sprintf("%s,%s", tableNameParam, tableNameAuth)},
			expectedTables: []string{tableNameParam, tableNameAuth},
		},
		{
			name:           "list non-existent table",
			args:           map[string]any{"table_names": "non_existent_table_xyz"},
			expectedTables: []string{},
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]interface{}

			if config.IsMCP() {
				statusCode, mcpResp, err := tests.InvokeMCPTool(t, "list-tables-tool", tc.args, nil)
				if statusCode != http.StatusOK || err != nil || mcpResp == nil {
					t.Fatalf("response status code is not 200, got %d: %v", statusCode, err)
				}
				if mcpResp.Error != nil || mcpResp.Result.IsError {
					t.Fatalf("list-tables-tool returned an error result: %+v", mcpResp.Result.Content)
				}
				// The verifier reads body["result"] as the JSON array string
				// that /api returns, so hand it the same shape.
				body = map[string]interface{}{"result": tests.GetMCPResultJSON(t, mcpResp)}
			} else {
				url := "http://127.0.0.1:5000/api/tool/list-tables-tool/invoke"
				reqBytes, err := json.Marshal(tc.args)
				if err != nil {
					t.Fatalf("unable to marshal args: %s", err)
				}
				resp, respBody := tests.RunRequest(t, http.MethodPost, url, bytes.NewBuffer(reqBytes), map[string]string{})

				if resp.StatusCode != http.StatusOK {
					t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
				}

				// Check response body
				if err := json.Unmarshal(respBody, &body); err != nil {
					t.Fatalf("error parsing response body: %s", err)
				}
			}

			verifyTableListResult(t, body, tc.expectedTables, tc.useSimpleFormat)
		})
	}
}

// Helper function to verify graph list results
func verifyGraphListResult(t *testing.T, body map[string]interface{}, expectedGraphs []string, expectedSimpleFormat bool) {
	// Parse the result
	result, ok := body["result"].(string)
	if !ok {
		t.Fatalf("unable to find result in response body")
	}

	var graphs []interface{}
	err := json.Unmarshal([]byte(result), &graphs)
	if err != nil {
		t.Fatalf("unable to parse result as JSON array: %s", err)
	}

	// If we expect specific graphs, verify they exist
	if len(expectedGraphs) > 0 {
		graphNames := make(map[string]bool)
		requiredKeys := []string{"schema_name", "object_name", "catalog", "node_tables", "edge_tables", "labels", "property_declarations"}
		if expectedSimpleFormat {
			requiredKeys = []string{"name"}
		}

		for _, graph := range graphs {
			graphMap, ok := graph.(map[string]interface{})
			if !ok {
				continue
			}

			objectDetails, ok := graphMap["object_details"].(map[string]interface{})
			if !ok {
				t.Fatalf("object_details is not of type map[string]interface{}, got: %T", graphMap["object_details"])
			}
			for _, reqKey := range requiredKeys {
				if _, hasKey := objectDetails[reqKey]; !hasKey {
					t.Errorf("missing required key '%s', for object_details: %v", reqKey, objectDetails)
				}
			}

			if name, ok := graphMap["object_name"].(string); ok {
				graphNames[name] = true
			}
		}

		for _, expected := range expectedGraphs {
			if !graphNames[expected] {
				t.Errorf("expected graph %s not found in results", expected)
			}
		}
	}
}

// runSpannerListGraphsTest tests the spanner-list-graphs tool
func runSpannerListGraphsTest(t *testing.T, graphName string, opts ...tests.ToolExecOption) {
	config := &tests.ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	invokeTcs := []struct {
		name            string
		args            map[string]any
		expectedGraphs  []string // empty means don't check specific graphs
		useSimpleFormat bool
	}{
		{
			name:           "list all graphs with detailed format",
			args:           map[string]any{},
			expectedGraphs: []string{graphName},
		},
		{
			name:            "list graphs with simple format",
			args:            map[string]any{"output_format": "simple"},
			expectedGraphs:  []string{graphName},
			useSimpleFormat: true,
		},
		{
			name:           "list specific graphs",
			args:           map[string]any{"graph_names": graphName},
			expectedGraphs: []string{graphName},
		},
		{
			name:           "list non-existent graph",
			args:           map[string]any{"graph_names": "non_existent_graph_xyz"},
			expectedGraphs: []string{},
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]interface{}

			if config.IsMCP() {
				statusCode, mcpResp, err := tests.InvokeMCPTool(t, "list-graphs-tool", tc.args, nil)
				if statusCode != http.StatusOK || err != nil || mcpResp == nil {
					t.Fatalf("response status code is not 200, got %d: %v", statusCode, err)
				}
				if mcpResp.Error != nil || mcpResp.Result.IsError {
					t.Fatalf("list-graphs-tool returned an error result: %+v", mcpResp.Result.Content)
				}
				// The verifier reads body["result"] as the JSON array string
				// that /api returns, so hand it the same shape.
				body = map[string]interface{}{"result": tests.GetMCPResultJSON(t, mcpResp)}
			} else {
				url := "http://127.0.0.1:5000/api/tool/list-graphs-tool/invoke"
				reqBytes, err := json.Marshal(tc.args)
				if err != nil {
					t.Fatalf("unable to marshal args: %s", err)
				}
				resp, respBody := tests.RunRequest(t, http.MethodPost, url, bytes.NewBuffer(reqBytes), map[string]string{})

				if resp.StatusCode != http.StatusOK {
					t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
				}

				// Check response body
				if err := json.Unmarshal(respBody, &body); err != nil {
					t.Fatalf("error parsing response body: %s", err)
				}
			}

			verifyGraphListResult(t, body, tc.expectedGraphs, tc.useSimpleFormat)
		})
	}
}

// setupSpannerVectorTable creates a vector table in Spanner for semantic search testing
func setupSpannerVectorTable(t *testing.T, ctx context.Context, adminClient *database.DatabaseAdminClient, dbString string) (string, func(*testing.T)) {
	tableName := "vector_table_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createStatement := fmt.Sprintf(`CREATE TABLE %s (
		id INT64,
		content STRING(MAX),
		embedding ARRAY<FLOAT32>
	) PRIMARY KEY (id)`, tableName)

	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbString,
		Statements: []string{createStatement},
	})
	if err != nil {
		t.Fatalf("unable to start create vector table operation %s: %s", tableName, err)
	}
	err = op.Wait(ctx)
	if err != nil {
		t.Fatalf("unable to create test vector table %s: %s", tableName, err)
	}

	return tableName, func(t *testing.T) {
		op, err = adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
			Database:   dbString,
			Statements: []string{fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)},
		})
		if err != nil {
			t.Errorf("unable to start drop %s operation: %s", tableName, err)
			return
		}
		opErr := op.Wait(ctx)
		if opErr != nil {
			t.Errorf("Teardown failed: %s", opErr)
		}
	}
}

// getSpannerVectorSearchStmts returns statements for spanner semantic search
func getSpannerVectorSearchStmts(vectorTableName string) (string, string) {
	insertStmt := fmt.Sprintf("INSERT INTO %s (id, content, embedding) VALUES (1, @content, @text_to_embed)", vectorTableName)
	searchStmt := fmt.Sprintf("SELECT id, content, COSINE_DISTANCE(embedding, @query) AS distance FROM %s ORDER BY distance LIMIT 1", vectorTableName)
	return insertStmt, searchStmt
}

func runSpannerSchemaToolInvokeTest(t *testing.T, accessSchemaWant string, opts ...tests.ToolExecOption) {
	config := &tests.ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	invokeTcs := []struct {
		name          string
		toolName      string
		requestHeader map[string]string
		args          map[string]any
		want          string
		isErr         bool
	}{
		{
			name:          "invoke list-tables-read-only",
			toolName:      "access-schema-read-only",
			requestHeader: map[string]string{},
			args:          map[string]any{},
			want:          accessSchemaWant,
			isErr:         false,
		},
		{
			name:          "invoke list-tables",
			toolName:      "access-schema",
			requestHeader: map[string]string{},
			args:          map[string]any{},
			isErr:         true,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var got string

			if config.IsMCP() {
				statusCode, mcpResp, err := tests.InvokeMCPTool(t, tc.toolName, tc.args, tc.requestHeader)
				if statusCode != http.StatusOK || err != nil || mcpResp == nil {
					if tc.isErr {
						return
					}
					t.Fatalf("response status code is not 200, got %d: %v", statusCode, err)
				}
				if mcpResp.Error != nil || mcpResp.Result.IsError {
					if tc.isErr {
						return
					}
					t.Fatalf("%s returned an error result: %+v", tc.toolName, mcpResp.Result.Content)
				}
				got = tests.GetMCPResultJSON(t, mcpResp)
			} else {
				api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
				reqBytes, err := json.Marshal(tc.args)
				if err != nil {
					t.Fatalf("unable to marshal args: %s", err)
				}
				// Send Tool invocation request
				req, err := http.NewRequest(http.MethodPost, api, bytes.NewBuffer(reqBytes))
				if err != nil {
					t.Fatalf("unable to create request: %s", err)
				}
				req.Header.Add("Content-type", "application/json")
				for k, v := range tc.requestHeader {
					req.Header.Add(k, v)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("unable to send request: %s", err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					if tc.isErr {
						return
					}
					bodyBytes, _ := io.ReadAll(resp.Body)
					t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
				}

				// Check response body
				var body map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("error parsing response body")
				}

				result, ok := body["result"].(string)
				if !ok {
					t.Fatalf("unable to find result in response body")
				}
				got = result
			}

			// Database errors are returned with a 200 status and an error in
			// the result.
			if tc.isErr {
				if !strings.Contains(got, `"error"`) {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}

			if got != tc.want {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}
