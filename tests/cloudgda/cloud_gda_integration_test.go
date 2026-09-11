// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloudgda_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	bigqueryapi "cloud.google.com/go/bigquery"
	geminidataanalytics "cloud.google.com/go/geminidataanalytics/apiv1beta"
	"cloud.google.com/go/geminidataanalytics/apiv1beta/geminidataanalyticspb"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	source "github.com/googleapis/mcp-toolbox/internal/sources/cloudgda"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudgda"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/tests"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	cloudGdaToolType   = "cloud-gemini-data-analytics-query"
	CloudGDASourceType = "cloud-gemini-data-analytics"
	CloudGdaProject    = os.Getenv("CLOUD_GDA_PROJECT")
)

func getCloudGDAProject(t *testing.T) string {
	if CloudGdaProject == "" {
		t.Fatal("'CLOUD_GDA_PROJECT' not set")
	}
	return CloudGdaProject
}

type mockDataChatServer struct {
	geminidataanalyticspb.UnimplementedDataChatServiceServer
	t *testing.T
}

func (s *mockDataChatServer) QueryData(ctx context.Context, req *geminidataanalyticspb.QueryDataRequest) (*geminidataanalyticspb.QueryDataResponse, error) {
	if req.Prompt == "" {
		s.t.Errorf("missing prompt")
		return nil, fmt.Errorf("missing prompt")
	}

	return &geminidataanalyticspb.QueryDataResponse{
		GeneratedQuery:        "SELECT * FROM table;",
		NaturalLanguageAnswer: "Here is the answer.",
	}, nil
}

func getCloudGdaToolsConfig() map[string]any {
	return map[string]any{
		"sources": map[string]any{
			"my-gda-source": map[string]any{
				"type":      "cloud-gemini-data-analytics",
				"projectId": "test-project",
			},
		},
		"tools": map[string]any{
			"cloud-gda-query": map[string]any{
				"type":        cloudGdaToolType,
				"source":      "my-gda-source",
				"description": "Test GDA Tool",
				"location":    "us-central1",
				"context": map[string]any{
					"datasourceReferences": map[string]any{
						"spannerReference": map[string]any{
							"databaseReference": map[string]any{
								"projectId":  "test-project",
								"instanceId": "test-instance",
								"databaseId": "test-db",
								"engine":     "GOOGLE_SQL",
							},
						},
					},
				},
			},
		},
	}
}

func TestCloudGdaToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Start a gRPC server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	geminidataanalyticspb.RegisterDataChatServiceServer(s, &mockDataChatServer{t: t})
	go func() {
		if err := s.Serve(lis); err != nil {
			// This might happen on strict shutdown, log if unexpected
			t.Logf("server executed: %v", err)
		}
	}()
	defer s.Stop()

	// Configure toolbox to use the gRPC server
	endpoint := lis.Addr().String()

	// Override client creation
	origFunc := source.NewDataChatClient
	defer func() {
		source.NewDataChatClient = origFunc
	}()

	source.NewDataChatClient = func(ctx context.Context, opts ...option.ClientOption) (*geminidataanalytics.DataChatClient, error) {
		opts = append(opts,
			option.WithEndpoint(endpoint),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
		return origFunc(ctx, opts...)
	}

	args := []string{"--enable-api"}
	toolsFile := getCloudGdaToolsConfig()
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

	toolName := "cloud-gda-query"

	// 1. RunToolGetTestByName
	expectedManifest := map[string]any{
		toolName: map[string]any{
			"description": "Test GDA Tool\n\n" + cloudgda.Guidance,
			"parameters": []any{
				map[string]any{
					"name":         "query",
					"type":         "string",
					"description":  "A natural language formulation of a database query.",
					"required":     true,
					"authServices": []any{},
				},
			},
			"authRequired": []any{},
		},
	}
	tests.RunToolGetTestByName(t, toolName, expectedManifest)

	// 2. RunToolInvokeParametersTest
	params := []byte(`{"query": "test question"}`)
	tests.RunToolInvokeParametersTest(t, toolName, params, "\"generated_query\":\"SELECT * FROM table;\"")

	// 3. Manual MCP Tool Call Test
	// Initialize MCP session
	sessionId := tests.RunInitialize(t, "2024-11-05")

	// Construct MCP Request
	mcpReq := jsonrpc.JSONRPCRequest{
		Jsonrpc: "2.0",
		Id:      "test-mcp-call",
		Request: jsonrpc.Request{
			Method: "tools/call",
		},
		Params: map[string]any{
			"name": toolName,
			"arguments": map[string]any{
				"query": "test question",
			},
		},
	}
	reqBytes, _ := json.Marshal(mcpReq)

	headers := map[string]string{}
	if sessionId != "" {
		headers["Mcp-Session-Id"] = sessionId
	}

	// Send Request
	resp, respBody := tests.RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/mcp", bytes.NewBuffer(reqBytes), headers)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("MCP request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Check Response
	respStr := string(respBody)
	if !strings.Contains(respStr, "SELECT * FROM table;") {
		t.Errorf("MCP response does not contain expected query result: %s", respStr)
	}
}

// Copied over from bigquery_integration_test.go
func initBigQueryConnection(project string) (*bigqueryapi.Client, error) {
	ctx := context.Background()
	cred, err := google.FindDefaultCredentials(ctx, bigqueryapi.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials with scope %q: %w", bigqueryapi.Scope, err)
	}

	client, err := bigqueryapi.NewClient(ctx, project, option.WithCredentials(cred))
	if err != nil {
		return nil, fmt.Errorf("failed to create BigQuery client for project %q: %w", project, err)
	}
	return client, nil
}

func setupBigQueryTable(t *testing.T, ctx context.Context, client *bigqueryapi.Client, createStatement, insertStatement, datasetName string, tableName string) func(*testing.T) {
	// Create dataset
	dataset := client.Dataset(datasetName)
	_, err := dataset.Metadata(ctx)

	if err != nil {
		apiErr, ok := err.(*googleapi.Error)
		if !ok || apiErr.Code != 404 {
			t.Fatalf("Failed to check dataset %q existence: %v", datasetName, err)
		}
		metadataToCreate := &bigqueryapi.DatasetMetadata{Name: datasetName}
		if err := dataset.Create(ctx, metadataToCreate); err != nil {
			t.Fatalf("Failed to create dataset %q: %v", datasetName, err)
		}
	}

	// Create table
	createJob, err := client.Query(createStatement).Run(ctx)
	if err != nil {
		t.Fatalf("Failed to start create table job for %s: %v", tableName, err)
	}
	createStatus, err := createJob.Wait(ctx)
	if err != nil {
		t.Fatalf("Failed to wait for create table job for %s: %v", tableName, err)
	}
	if err := createStatus.Err(); err != nil {
		t.Fatalf("Create table job for %s failed: %v", tableName, err)
	}

	if insertStatement != "" {
		// Insert test data
		insertQuery := client.Query(insertStatement)
		insertJob, err := insertQuery.Run(ctx)
		if err != nil {
			t.Fatalf("Failed to start insert job for %s: %v", tableName, err)
		}
		insertStatus, err := insertJob.Wait(ctx)
		if err != nil {
			t.Fatalf("Failed to wait for insert job for %s: %v", tableName, err)
		}
		if err := insertStatus.Err(); err != nil {
			t.Fatalf("Insert job for %s failed: %v", tableName, err)
		}
	}

	return func(t *testing.T) {
		// tear down table
		dropSQL := fmt.Sprintf("drop table %s", tableName)
		dropJob, err := client.Query(dropSQL).Run(ctx)
		if err != nil {
			t.Errorf("Failed to start drop table job for %s: %v", tableName, err)
			return
		}
		dropStatus, err := dropJob.Wait(ctx)
		if err != nil {
			t.Errorf("Failed to wait for drop table job for %s: %v", tableName, err)
			return
		}
		if err := dropStatus.Err(); err != nil {
			t.Errorf("Error dropping table %s: %v", tableName, err)
		}

		// tear down dataset
		datasetToTeardown := client.Dataset(datasetName)
		tablesIterator := datasetToTeardown.Tables(ctx)
		_, err = tablesIterator.Next()

		if err == iterator.Done {
			if err := datasetToTeardown.Delete(ctx); err != nil {
				t.Errorf("Failed to delete dataset %s: %v", datasetName, err)
			}
		} else if err != nil {
			t.Errorf("Failed to list tables in dataset %s to check emptiness: %v.", datasetName, err)
		}
	}
}

func TestCloudGDAConservationalAnalyticsTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	projectID := getCloudGDAProject(t)
	client, err := initBigQueryConnection(projectID)
	if err != nil {
		t.Fatalf("unable to create BigQuery client: %s", err)
	}

	// Setup dataset and table for Data Agent
	datasetName := fmt.Sprintf("data_agent_test_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	tableName := "test_table"
	tableNameParam := fmt.Sprintf("`%s.%s.%s`", projectID, datasetName, tableName)

	createTableStmt := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING)", tableNameParam)
	teardownTable := setupBigQueryTable(t, ctx, client, createTableStmt, "", datasetName, tableNameParam)
	defer teardownTable(t)

	// The tool server needs to be started before we can use the create tool
	// Configure tools with cloud-gemini-data-analytics source
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": map[string]any{
				"type":      "cloud-gemini-data-analytics",
				"projectId": projectID,
			},
			"my-client-auth-source": map[string]any{
				"type":           "cloud-gemini-data-analytics",
				"projectId":      projectID,
				"useClientOAuth": true,
			},
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"kind":     "google",
				"clientId": tests.ClientId,
			},
		},
		"tools": map[string]any{
			"my-list-accessible-data-agents-tool": map[string]any{
				"type":        "conversational-analytics-list-accessible-data-agents",
				"source":      "my-instance",
				"description": "Tool to list data agents.",
			},
			"my-auth-list-accessible-data-agents-tool": map[string]any{
				"type":         "conversational-analytics-list-accessible-data-agents",
				"source":       "my-instance",
				"description":  "Tool to list data agents with auth.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-client-auth-list-accessible-data-agents-tool": map[string]any{
				"type":        "conversational-analytics-list-accessible-data-agents",
				"source":      "my-client-auth-source",
				"description": "Tool to list data agents with client auth.",
			},
			"my-get-data-agent-info-tool": map[string]any{
				"type":        "conversational-analytics-get-data-agent-info",
				"source":      "my-instance",
				"description": "Tool to get data agent info.",
			},
			"my-auth-get-data-agent-info-tool": map[string]any{
				"type":         "conversational-analytics-get-data-agent-info",
				"source":       "my-instance",
				"description":  "Tool to get data agent info with auth.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-client-auth-get-data-agent-info-tool": map[string]any{
				"type":        "conversational-analytics-get-data-agent-info",
				"source":      "my-client-auth-source",
				"description": "Tool to get data agent info with client auth.",
			},
			"my-ask-data-agent-tool": map[string]any{
				"type":        "conversational-analytics-ask-data-agent",
				"source":      "my-instance",
				"description": "Tool to ask data agent.",
			},
			"my-auth-ask-data-agent-tool": map[string]any{
				"type":         "conversational-analytics-ask-data-agent",
				"source":       "my-instance",
				"description":  "Tool to ask data agent with auth.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-client-auth-ask-data-agent-tool": map[string]any{
				"type":        "conversational-analytics-ask-data-agent",
				"source":      "my-client-auth-source",
				"description": "Tool to ask data agent with client auth.",
			},
			"my-create-data-agent-tool": map[string]any{
				"type":        "conversational-analytics-create-data-agent",
				"source":      "my-instance",
				"description": "Tool to create data agent.",
			},
			"my-auth-create-data-agent-tool": map[string]any{
				"type":         "conversational-analytics-create-data-agent",
				"source":       "my-instance",
				"description":  "Tool to create data agent with auth.",
				"authRequired": []string{"my-google-auth"},
			},
			"my-client-auth-create-data-agent-tool": map[string]any{
				"type":        "conversational-analytics-create-data-agent",
				"source":      "my-client-auth-source",
				"description": "Tool to create data agent with client auth.",
			},
		},
	}

	args := []string{"--enable-api"}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancelCmd := context.WithTimeout(ctx, 10*time.Second)
	defer cancelCmd()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Create Data Agent using the tool
	dataAgentID, dataAgentDisplayName, teardownDataAgent := runCreateDataAgentInvokeTest(t, projectID, datasetName, tableName)
	defer teardownDataAgent(t)

	runListAccessibleDataAgentsInvokeTest(t, dataAgentDisplayName)
	runGetDataAgentInfoInvokeTest(t, dataAgentID, dataAgentDisplayName)
	runAskDataAgentInvokeTest(t, dataAgentID)
}

func runListAccessibleDataAgentsInvokeTest(t *testing.T, dataAgentDisplayName string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	invokeTcs := []struct {
		name          string
		api           string
		requestHeader map[string]string
		requestBody   io.Reader
		want          string
		isErr         bool
	}{
		{
			name:          "invoke my-list-accessible-data-agents-tool",
			api:           "http://127.0.0.1:5000/api/tool/my-list-accessible-data-agents-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{}`)),
			want:          dataAgentDisplayName,
			isErr:         false,
		},
		{
			name:          "invoke my-auth-list-accessible-data-agents-tool with auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-list-accessible-data-agents-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody:   bytes.NewBuffer([]byte(`{}`)),
			want:          dataAgentDisplayName,
			isErr:         false,
		},
		{
			name:          "invoke my-auth-list-accessible-data-agents-tool without auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-list-accessible-data-agents-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{}`)),
			isErr:         true,
		},
		{
			name:          "invoke my-client-auth-list-accessible-data-agents-tool with auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-client-auth-list-accessible-data-agents-tool/invoke",
			requestHeader: map[string]string{"Authorization": accessToken},
			requestBody:   bytes.NewBuffer([]byte(`{}`)),
			want:          dataAgentDisplayName,
			isErr:         false,
		},
		{
			name:          "invoke my-client-auth-list-accessible-data-agents-tool without auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-client-auth-list-accessible-data-agents-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{}`)),
			isErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
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

			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("error reading response body: %v", err)
			}

			var body map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &body); err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected %q to contain %q, but it did not", got, tc.want)
			}
		})
	}
}

func runGetDataAgentInfoInvokeTest(t *testing.T, dataAgentName, dataAgentDisplayName string) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	invokeTcs := []struct {
		name          string
		api           string
		requestHeader map[string]string
		requestBody   io.Reader
		want          string
		isErr         bool
	}{
		{
			name:          "invoke my-get-data-agent-info-tool",
			api:           "http://127.0.0.1:5000/api/tool/my-get-data-agent-info-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"data_agent_id": "%s"}`, dataAgentName))),
			want:          dataAgentDisplayName,
			isErr:         false,
		},
		{
			name:          "invoke my-auth-get-data-agent-info-tool with auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-get-data-agent-info-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"data_agent_id": "%s"}`, dataAgentName))),
			want:          dataAgentDisplayName,
			isErr:         false,
		},
		{
			name:          "invoke my-auth-get-data-agent-info-tool without auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-get-data-agent-info-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"data_agent_id": "%s"}`, dataAgentName))),
			isErr:         true,
		},
		{
			name:          "invoke my-client-auth-get-data-agent-info-tool with auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-client-auth-get-data-agent-info-tool/invoke",
			requestHeader: map[string]string{"Authorization": accessToken},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"data_agent_id": "%s"}`, dataAgentName))),
			want:          dataAgentDisplayName,
			isErr:         false,
		},
		{
			name:          "invoke my-client-auth-get-data-agent-info-tool without auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-client-auth-get-data-agent-info-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf(`{"data_agent_id": "%s"}`, dataAgentName))),
			isErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
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

			var body map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected %q to contain %q, but it did not", got, tc.want)
			}
		})
	}
}

func runAskDataAgentInvokeTest(t *testing.T, dataAgentID string) {
	const maxRetries = 3
	const requestTimeout = 340 * time.Second

	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	dataAgentWant := `FINAL_RESPONSE`

	invokeTcs := []struct {
		name          string
		api           string
		requestHeader map[string]string
		requestBody   string
		want          string
		isErr         bool
	}{
		{
			name:          "invoke my-ask-data-agent-tool",
			api:           "http://127.0.0.1:5000/api/tool/my-ask-data-agent-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   fmt.Sprintf(`{"user_query_with_context": "What are the names in the table?", "data_agent_id": "%s"}`, dataAgentID),
			want:          dataAgentWant,
			isErr:         false,
		},
		{
			name:          "invoke my-auth-ask-data-agent-tool with auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-ask-data-agent-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody:   fmt.Sprintf(`{"user_query_with_context": "What are the names in the table?", "data_agent_id": "%s"}`, dataAgentID),
			want:          dataAgentWant,
			isErr:         false,
		},
		{
			name:          "invoke my-auth-ask-data-agent-tool without auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-ask-data-agent-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   fmt.Sprintf(`{"user_query_with_context": "What are the names in the table?", "data_agent_id": "%s"}`, dataAgentID),
			isErr:         true,
		},
		{
			name:          "invoke my-client-auth-ask-data-agent-tool with auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-client-auth-ask-data-agent-tool/invoke",
			requestHeader: map[string]string{"Authorization": accessToken},
			requestBody:   fmt.Sprintf(`{"user_query_with_context": "What are the names in the table?", "data_agent_id": "%s"}`, dataAgentID),
			want:          dataAgentWant,
			isErr:         false,
		},
		{
			name:          "invoke my-client-auth-ask-data-agent-tool without auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-client-auth-ask-data-agent-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   fmt.Sprintf(`{"user_query_with_context": "What are the names in the table?", "data_agent_id": "%s"}`, dataAgentID),
			isErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var resp *http.Response
			var err error
			bodyBytes := []byte(tc.requestBody)

			req, err := http.NewRequest(http.MethodPost, tc.api, nil)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Set("Content-type", "application/json")
			for k, v := range tc.requestHeader {
				req.Header.Add(k, v)
			}

			for i := 0; i < maxRetries; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
				defer cancel()

				req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				req.GetBody = func() (io.ReadCloser, error) {
					return io.NopCloser(bytes.NewReader(bodyBytes)), nil
				}
				reqWithCtx := req.WithContext(ctx)

				resp, err = http.DefaultClient.Do(reqWithCtx)
				if err != nil {
					// Retry on time out.
					if os.IsTimeout(err) {
						t.Logf("Request timed out (attempt %d/%d), retrying...", i+1, maxRetries)
						time.Sleep(5 * time.Second)
						continue
					}
					t.Fatalf("unable to send request: %s", err)
				}
				if resp.StatusCode == http.StatusServiceUnavailable {
					t.Logf("Received 503 Service Unavailable (attempt %d/%d), retrying...", i+1, maxRetries)
					time.Sleep(15 * time.Second)
					continue
				}
				break
			}

			if err != nil {
				t.Fatalf("Request failed after %d retries: %v", maxRetries, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			if err = json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			wantPattern := regexp.MustCompile(tc.want)
			if !wantPattern.MatchString(got) {
				t.Fatalf("response did not match the expected pattern.\nFull response:\n%s", got)
			}
		})
	}
}

func runCreateDataAgentInvokeTest(t *testing.T, projectID, datasetID, tableID string) (string, string, func(*testing.T)) {
	t.Logf("Creating data agent with ProjectID: %q, DatasetID: %q, TableID: %q", projectID, datasetID, tableID)

	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}

	dataAgentId := "test" + strings.ReplaceAll(uuid.New().String(), "-", "")
	dataAgentDisplayName := "test-agent-" + strings.ReplaceAll(uuid.New().String(), "-", "")

	requestBodyMap := map[string]any{
		"displayName": dataAgentDisplayName,
		"dataAnalyticsAgent": map[string]any{
			"publishedContext": map[string]any{
				"datasourceReferences": map[string]any{
					"bq": map[string]any{
						"tableReferences": []map[string]string{
							{
								"projectId": projectID,
								"datasetId": datasetID,
								"tableId":   tableID,
							},
						},
					},
				},
			},
		},
	}
	agentConfigBytes, _ := json.Marshal(requestBodyMap)

	api := "http://127.0.0.1:5000/api/tool/my-client-auth-create-data-agent-tool/invoke"
	requestBody := bytes.NewBuffer([]byte(fmt.Sprintf(`{"data_agent_id": "%s", "agent_config": %s}`, dataAgentId, string(agentConfigBytes))))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, api, requestBody)
	if err != nil {
		t.Fatalf("unable to create request: %s", err)
	}
	req.Header.Add("Content-type", "application/json")
	req.Header.Add("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unable to send request: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("error parsing response body")
	}

	got, ok := body["result"].(string)
	if !ok {
		t.Fatalf("unable to find result in response body")
	}

	var createdAgent map[string]any
	if err := json.Unmarshal([]byte(got), &createdAgent); err != nil {
		t.Fatalf("failed to unmarshal created agent string: %v", err)
	}

	nameVal, ok := createdAgent["name"].(string)
	if !ok {
		t.Fatalf("created agent missing name: %s", got)
	}
	if !strings.HasSuffix(nameVal, dataAgentId) {
		t.Fatalf("expected created agent name to end with %s, got: %s", dataAgentId, nameVal)
	}

	if displayName, ok := createdAgent["displayName"].(string); !ok || displayName != requestBodyMap["displayName"].(string) {
		t.Fatalf("expected displayName %s, got: %s", requestBodyMap["displayName"], displayName)
	}

	// Setup client for teardown
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	client, err := util.NewGDAClient(t.Context(), option.WithTokenSource(tokenSource))
	if err != nil {
		t.Fatalf("failed to create GDA client: %v", err)
	}

	teardown := func(t *testing.T) {
		parent := fmt.Sprintf("projects/%s/locations/global", projectID)
		agentName := fmt.Sprintf("%s/dataAgents/%s", parent, dataAgentId)
		deleteUrl := fmt.Sprintf("%s/v1/%s", util.GetGDAEndpoint(), agentName)
		delReq, _ := http.NewRequestWithContext(t.Context(), http.MethodDelete, deleteUrl, nil)
		delResp, err := client.Do(delReq)
		if err != nil {
			t.Errorf("failed to delete data agent %s: %v", agentName, err)
			return
		}
		defer delResp.Body.Close()
		if delResp.StatusCode != http.StatusOK {
			t.Errorf("failed to delete data agent %s, status: %d", agentName, delResp.StatusCode)
		}
	}

	return dataAgentId, dataAgentDisplayName, teardown
}
