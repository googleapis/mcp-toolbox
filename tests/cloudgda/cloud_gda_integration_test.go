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

// cloudGDACleanupTimeout bounds each cleanup operation. Cleanups detach from
// the test context's cancellation (it is already cancelled when they run), so
// they need their own deadline to fail fast if an API stops responding.
const cloudGDACleanupTimeout = 2 * time.Minute

// startCloudGdaMockServer starts an in-process Data Chat gRPC server and points
// the source's client at it. The server is stopped and the client override
// restored on cleanup.
func startCloudGdaMockServer(t *testing.T) {
	t.Helper()
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
	t.Cleanup(s.Stop)

	// Configure toolbox to use the gRPC server
	endpoint := lis.Addr().String()

	// Override client creation
	origFunc := source.NewDataChatClient
	t.Cleanup(func() {
		source.NewDataChatClient = origFunc
	})

	source.NewDataChatClient = func(ctx context.Context, opts ...option.ClientOption) (*geminidataanalytics.DataChatClient, error) {
		opts = append(opts,
			option.WithEndpoint(endpoint),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
		return origFunc(ctx, opts...)
	}
}

// cloudGDATransport selects how the tests talk to the toolbox server: the
// legacy REST API, or the MCP endpoint when isMCP is set.
type cloudGDATransport struct {
	isMCP bool
}

// cloudGDAResult is the outcome of a tool invocation. result holds the tool
// result as the REST API returns it, or the error text when the call failed.
// toolErr is set when MCP reported an error.
type cloudGDAResult struct {
	status  int
	result  string
	toolErr bool
}

// startServer starts the toolbox server with toolsFile and waits until it is
// ready to serve. The REST API is only enabled for the non-MCP transport.
func (tr cloudGDATransport) startServer(t *testing.T, ctx context.Context, toolsFile map[string]any) {
	t.Helper()
	var args []string
	if !tr.isMCP {
		args = append(args, "--enable-api")
	}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

// invoke calls toolName with args and request headers through the selected
// transport, honoring ctx for the request. It returns an error only when the
// request itself fails, so callers can retry.
func (tr cloudGDATransport) invoke(t *testing.T, ctx context.Context, toolName string, args map[string]any, headers map[string]string) (cloudGDAResult, error) {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", toolName)
	var payload any = args
	reqHeaders := map[string]string{"Content-Type": "application/json"}
	for k, v := range headers {
		reqHeaders[k] = v
	}
	if tr.isMCP {
		url = "http://127.0.0.1:5000/mcp"
		payload = tests.NewMCPCallToolRequest(uuid.New().String(), toolName, args)
		reqHeaders = tests.NewMCPRequestHeader(t, headers)
	}

	reqBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("unable to marshal request body: %s", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(reqBytes))
	if err != nil {
		t.Fatalf("unable to create request: %s", err)
	}
	for k, v := range reqHeaders {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return cloudGDAResult{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return cloudGDAResult{}, err
	}

	if !tr.isMCP {
		if resp.StatusCode != http.StatusOK {
			return cloudGDAResult{status: resp.StatusCode, result: string(respBody)}, nil
		}
		var body map[string]any
		if err := json.Unmarshal(respBody, &body); err != nil {
			t.Fatalf("error parsing response body %q: %s", string(respBody), err)
		}
		result, ok := body["result"].(string)
		if !ok {
			t.Fatalf("unable to find result in response body: %s", string(respBody))
		}
		return cloudGDAResult{status: resp.StatusCode, result: result}, nil
	}

	var mcpResp tests.MCPCallToolResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		if resp.StatusCode != http.StatusOK {
			return cloudGDAResult{status: resp.StatusCode, result: string(respBody), toolErr: true}, nil
		}
		t.Fatalf("error parsing MCP response body %q: %s", string(respBody), err)
	}
	if mcpResp.Error != nil {
		return cloudGDAResult{status: resp.StatusCode, result: mcpResp.Error.Message, toolErr: true}, nil
	}
	var text strings.Builder
	for _, content := range mcpResp.Result.Content {
		text.WriteString(content.Text)
	}
	if mcpResp.Result.IsError || len(mcpResp.Result.Content) <= 1 {
		return cloudGDAResult{status: resp.StatusCode, result: text.String(), toolErr: mcpResp.Result.IsError}, nil
	}
	// Multiple content blocks are rows of a []any result; flatten them into the
	// JSON array the REST API returns.
	rows := []any{}
	for _, content := range mcpResp.Result.Content {
		var item any
		if err := json.Unmarshal([]byte(content.Text), &item); err != nil {
			rows = append(rows, content.Text)
			continue
		}
		if slice, ok := item.([]any); ok {
			rows = append(rows, slice...)
		} else {
			rows = append(rows, item)
		}
	}
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("error marshaling MCP result: %s", err)
	}
	return cloudGDAResult{status: resp.StatusCode, result: string(b)}, nil
}

// mustInvoke invokes toolName and fails the test if the request can't be sent.
func (tr cloudGDATransport) mustInvoke(t *testing.T, ctx context.Context, toolName string, args map[string]any, headers map[string]string) cloudGDAResult {
	t.Helper()
	res, err := tr.invoke(t, ctx, toolName, args, headers)
	if err != nil {
		t.Fatalf("unable to send request: %s", err)
	}
	return res
}

func TestCloudGdaToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	startCloudGdaMockServer(t)
	cloudGDATransport{}.startServer(t, ctx, getCloudGdaToolsConfig())

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

// setupBigQueryTable creates the dataset (if needed) and table used by the data
// agents. The table is dropped, and the dataset deleted if empty, on cleanup;
// the cleanup is registered first so a partially failed setup is still cleaned
// up.
func setupBigQueryTable(t *testing.T, ctx context.Context, client *bigqueryapi.Client, createStatement, insertStatement, datasetName string, tableName string) {
	t.Helper()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cloudGDACleanupTimeout)
		defer cancel()

		// tear down table
		dropSQL := fmt.Sprintf("drop table if exists %s", tableName)
		dropJob, err := client.Query(dropSQL).Run(cleanupCtx)
		if err != nil {
			t.Errorf("Failed to start drop table job for %s: %v", tableName, err)
			return
		}
		dropStatus, err := dropJob.Wait(cleanupCtx)
		if err != nil {
			t.Errorf("Failed to wait for drop table job for %s: %v", tableName, err)
			return
		}
		if err := dropStatus.Err(); err != nil {
			t.Errorf("Error dropping table %s: %v", tableName, err)
		}

		// tear down dataset
		datasetToTeardown := client.Dataset(datasetName)
		tablesIterator := datasetToTeardown.Tables(cleanupCtx)
		_, err = tablesIterator.Next()

		if err == iterator.Done {
			if err := datasetToTeardown.Delete(cleanupCtx); err != nil {
				t.Errorf("Failed to delete dataset %s: %v", datasetName, err)
			}
		} else if err != nil {
			if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code == http.StatusNotFound {
				return
			}
			t.Errorf("Failed to list tables in dataset %s to check emptiness: %v.", datasetName, err)
		}
	})

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
}

func setupDataAgent(t *testing.T, ctx context.Context, projectID, datasetID, tableID, dataAgentDisplayName string) string {
	t.Helper()
	t.Logf("Setting up data agent with ProjectID: %q, DatasetID: %q, TableID: %q, DisplayName: %q", projectID, datasetID, tableID, dataAgentDisplayName)

	dataAgentId := "test" + strings.ReplaceAll(uuid.New().String(), "-", "")
	parent := fmt.Sprintf("projects/%s/locations/global", projectID)
	url := fmt.Sprintf("%s/v1/%s/dataAgents?dataAgentId=%s", util.GetGDAEndpoint(), parent, dataAgentId)

	requestBody := map[string]any{
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

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("failed to marshal create data agent request: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Only credential refreshes use this context, and they must keep working
	// during cleanup.
	client, err := util.NewGDAClient(context.WithoutCancel(ctx))
	if err != nil {
		t.Fatalf("failed to create GDA client: %v", err)
	}

	// Registered before the request is sent: it may create the agent even when
	// the client sees a transport error. A 404 on delete is tolerated.
	agentName := fmt.Sprintf("%s/dataAgents/%s", parent, dataAgentId)
	t.Cleanup(func() { deleteDataAgent(t, ctx, client, agentName) })

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to create data agent: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to create data agent, status: %d, body: %s", resp.StatusCode, string(respBody))
	}

	var op map[string]any
	if err := json.Unmarshal(respBody, &op); err != nil {
		t.Fatalf("failed to unmarshal operation: %v", err)
	}

	opName, ok := op["name"].(string)
	if !ok {
		t.Fatalf("operation response missing name: %s", string(respBody))
	}

	// Poll for operation completion
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	timeout := time.After(60 * time.Second)

	done := false
	for !done {
		select {
		case <-ctx.Done():
			t.Fatalf("context cancelled while waiting for data agent creation")
		case <-timeout:
			t.Fatalf("timed out waiting for data agent creation")
		case <-ticker.C:
			opUrl := fmt.Sprintf("%s/v1/%s", util.GetGDAEndpoint(), opName)
			opReq, err := http.NewRequestWithContext(ctx, http.MethodGet, opUrl, nil)
			if err != nil {
				t.Fatalf("failed to build operation poll request: %v", err)
			}
			opResp, err := client.Do(opReq)
			if err != nil {
				t.Logf("failed to poll operation: %v", err)
				continue
			}
			opRespBody, _ := io.ReadAll(opResp.Body)
			opResp.Body.Close()

			if opResp.StatusCode != http.StatusOK {
				t.Logf("operation poll returned status %d: %s", opResp.StatusCode, string(opRespBody))
				continue
			}

			var pollOp map[string]any
			if err := json.Unmarshal(opRespBody, &pollOp); err != nil {
				t.Logf("failed to unmarshal polling response: %v", err)
				continue
			}

			if d, ok := pollOp["done"].(bool); ok && d {
				if errVal, ok := pollOp["error"]; ok && errVal != nil {
					t.Fatalf("data agent creation failed: %v", errVal)
				}
				done = true
			}
		}
	}

	return dataAgentId
}

func deleteDataAgent(t *testing.T, ctx context.Context, client *http.Client, agentName string) {
	t.Helper()

	// The test context may already be done, which would abort the delete and
	// leak the agent, so give the cleanup its own deadline.
	deleteCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	deleteUrl := fmt.Sprintf("%s/v1/%s", util.GetGDAEndpoint(), agentName)
	delReq, err := http.NewRequestWithContext(deleteCtx, http.MethodDelete, deleteUrl, nil)
	if err != nil {
		t.Errorf("failed to build delete request for data agent %s: %v", agentName, err)
		return
	}

	delResp, err := client.Do(delReq)
	if err != nil {
		t.Errorf("failed to delete data agent %s: %v", agentName, err)
		return
	}
	defer delResp.Body.Close()

	if delResp.StatusCode == http.StatusNotFound {
		return
	}
	// Delete returns a long-running operation, so any 2xx is a success.
	if delResp.StatusCode < 200 || delResp.StatusCode >= 300 {
		body, _ := io.ReadAll(delResp.Body)
		t.Errorf("failed to delete data agent %s, status: %d, body: %s", agentName, delResp.StatusCode, string(body))
	}
}

// getConversationalAnalyticsToolsConfig returns the tools file shared by the
// conversational analytics integration tests.
func getConversationalAnalyticsToolsConfig(projectID string) map[string]any {
	return map[string]any{
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
		},
	}
}

// setupConversationalAnalyticsTest creates the BigQuery table and two data
// agents used by the conversational analytics tests. It returns the tools file,
// the first agent's ID, and both agents' display names. All resources are
// deleted and the BigQuery client closed on cleanup.
func setupConversationalAnalyticsTest(t *testing.T, ctx context.Context) (map[string]any, string, string, string) {
	t.Helper()
	projectID := getCloudGDAProject(t)
	client, err := initBigQueryConnection(projectID)
	if err != nil {
		t.Fatalf("unable to create BigQuery client: %s", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("unable to close BigQuery client: %s", err)
		}
	})

	// Setup dataset and table for Data Agent
	datasetName := fmt.Sprintf("data_agent_test_%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	tableName := "test_table"
	tableNameParam := fmt.Sprintf("`%s.%s.%s`", projectID, datasetName, tableName)

	createTableStmt := fmt.Sprintf("CREATE TABLE %s (id INT64, name STRING)", tableNameParam)
	// The data agents are deleted before the table and dataset, since their
	// cleanups are registered later. Deleting an agent does not touch its
	// datasource, so either order is safe.
	setupBigQueryTable(t, ctx, client, createTableStmt, "", datasetName, tableNameParam)

	// Create Data Agent
	dataAgentDisplayName := fmt.Sprintf("test-agent-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	dataAgentID := setupDataAgent(t, ctx, projectID, datasetName, tableName, dataAgentDisplayName)

	// A second agent guarantees the project holds more than one accessible data
	// agent, which is what makes the page size assertions below deterministic
	// instead of dependent on whatever the project already contains. Its
	// deletion is registered by setupDataAgent.
	secondDataAgentDisplayName := fmt.Sprintf("test-agent-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	setupDataAgent(t, ctx, projectID, datasetName, tableName, secondDataAgentDisplayName)

	return getConversationalAnalyticsToolsConfig(projectID), dataAgentID, dataAgentDisplayName, secondDataAgentDisplayName
}

func TestCloudGDAConservationalAnalyticsTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	toolsFile, dataAgentID, dataAgentDisplayName, secondDataAgentDisplayName := setupConversationalAnalyticsTest(t, ctx)

	tr := cloudGDATransport{}
	tr.startServer(t, ctx, toolsFile)

	runConversationalAnalyticsTests(t, ctx, tr, dataAgentID, dataAgentDisplayName, secondDataAgentDisplayName)
}

// runConversationalAnalyticsTests runs the list, get and ask data agent checks.
func runConversationalAnalyticsTests(t *testing.T, ctx context.Context, tr cloudGDATransport, dataAgentID, dataAgentDisplayName, secondDataAgentDisplayName string) {
	// Both agents created above must come back from a single default call,
	// which is what fetching every page automatically is supposed to give.
	runListAccessibleDataAgentsInvokeTest(t, ctx, tr, dataAgentDisplayName, secondDataAgentDisplayName)
	runListAccessibleDataAgentsPageSizeTest(t, ctx, tr, 1)
	runGetDataAgentInfoInvokeTest(t, ctx, tr, dataAgentID, dataAgentDisplayName)
	runAskDataAgentInvokeTest(t, ctx, tr, dataAgentID)
}

type listDataAgentsResult struct {
	DataAgents []struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"dataAgents"`
	NextPageToken string `json:"nextPageToken"`
}

func (r listDataAgentsResult) contains(displayName string) bool {
	for _, agent := range r.DataAgents {
		if agent.DisplayName == displayName {
			return true
		}
	}
	return false
}

// runListAccessibleDataAgentsInvokeTest checks the default invocation, which
// takes no pagination parameters and therefore has to fetch every page on its
// own and return all accessible data agents in one response.
func runListAccessibleDataAgentsInvokeTest(t *testing.T, ctx context.Context, tr cloudGDATransport, dataAgentDisplayNames ...string) {
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
		toolName      string
		requestHeader map[string]string
		want          []string
		isErr         bool
	}{
		{
			name:          "invoke my-list-accessible-data-agents-tool",
			toolName:      "my-list-accessible-data-agents-tool",
			requestHeader: map[string]string{},
			want:          dataAgentDisplayNames,
			isErr:         false,
		},
		{
			name:          "invoke my-auth-list-accessible-data-agents-tool with auth token",
			toolName:      "my-auth-list-accessible-data-agents-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			want:          dataAgentDisplayNames,
			isErr:         false,
		},
		{
			name:          "invoke my-auth-list-accessible-data-agents-tool without auth token",
			toolName:      "my-auth-list-accessible-data-agents-tool",
			requestHeader: map[string]string{},
			isErr:         true,
		},
		{
			name:          "invoke my-client-auth-list-accessible-data-agents-tool with auth token",
			toolName:      "my-client-auth-list-accessible-data-agents-tool",
			requestHeader: map[string]string{"Authorization": accessToken},
			want:          dataAgentDisplayNames,
			isErr:         false,
		},
		{
			name:          "invoke my-client-auth-list-accessible-data-agents-tool without auth token",
			toolName:      "my-client-auth-list-accessible-data-agents-tool",
			requestHeader: map[string]string{},
			isErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			if tc.isErr {
				res := invokeListAccessibleDataAgents(t, ctx, tr, tc.toolName, tc.requestHeader, 0, "")
				if res.status == http.StatusOK {
					t.Fatalf("expected an error response, got status %d: %s", res.status, res.result)
				}
				return
			}

			result := listAccessibleDataAgents(t, ctx, tr, tc.toolName, tc.requestHeader, 0, "")
			for _, want := range tc.want {
				if !result.contains(want) {
					t.Errorf("data agent %q is missing from the %d data agents returned by a single default call", want, len(result.DataAgents))
				}
			}
			if len(result.DataAgents) < 1000 && result.NextPageToken != "" {
				t.Errorf("got nextPageToken %q, want none: a default call must return every data agent", result.NextPageToken)
			}
		})
	}
}

// runListAccessibleDataAgentsPageSizeTest checks caller-driven pagination:
// setting pageSize must hand control back to the caller, one page at a time.
// It relies on the test having created at least pageSize+1 data agents, so the
// assertions never depend on what the project already contained.
func runListAccessibleDataAgentsPageSizeTest(t *testing.T, ctx context.Context, tr cloudGDATransport, pageSize int) {
	toolName := "my-list-accessible-data-agents-tool"

	t.Run("invoke my-list-accessible-data-agents-tool with page size", func(t *testing.T) {
		firstPage := listAccessibleDataAgents(t, ctx, tr, toolName, map[string]string{}, pageSize, "")
		if len(firstPage.DataAgents) != pageSize {
			t.Fatalf("got %d data agents, want exactly the %d that were asked for", len(firstPage.DataAgents), pageSize)
		}
		if firstPage.NextPageToken == "" {
			t.Fatalf("no nextPageToken for a page of %d data agents, but the test created more than that", pageSize)
		}

		secondPage := listAccessibleDataAgents(t, ctx, tr, toolName, map[string]string{}, pageSize, firstPage.NextPageToken)
		if len(secondPage.DataAgents) == 0 || len(secondPage.DataAgents) > pageSize {
			t.Fatalf("got %d data agents on the second page, want between 1 and %d", len(secondPage.DataAgents), pageSize)
		}

		seen := make(map[string]bool, len(firstPage.DataAgents))
		for _, agent := range firstPage.DataAgents {
			// Empty names would make the overlap check below match everything.
			if agent.Name == "" {
				t.Fatalf("data agent entry has no name: %+v", agent)
			}
			seen[agent.Name] = true
		}
		for _, agent := range secondPage.DataAgents {
			if seen[agent.Name] {
				t.Errorf("data agent %q was returned on both pages, the page token did not advance", agent.Name)
			}
		}
	})
}

func listAccessibleDataAgents(t *testing.T, ctx context.Context, tr cloudGDATransport, toolName string, requestHeader map[string]string, pageSize int, pageToken string) listDataAgentsResult {
	t.Helper()

	res := invokeListAccessibleDataAgents(t, ctx, tr, toolName, requestHeader, pageSize, pageToken)
	if res.status != http.StatusOK || res.toolErr {
		t.Fatalf("response status code is not 200 (tool error: %v), got %d: %s", res.toolErr, res.status, res.result)
	}

	var parsed listDataAgentsResult
	if err := json.Unmarshal([]byte(res.result), &parsed); err != nil {
		t.Fatalf("error parsing tool result %q as JSON: %v", res.result, err)
	}
	return parsed
}

func invokeListAccessibleDataAgents(t *testing.T, ctx context.Context, tr cloudGDATransport, toolName string, requestHeader map[string]string, pageSize int, pageToken string) cloudGDAResult {
	t.Helper()

	requestBody := map[string]any{}
	if pageSize > 0 {
		requestBody["page_size"] = pageSize
	}
	if pageToken != "" {
		requestBody["page_token"] = pageToken
	}
	return tr.mustInvoke(t, ctx, toolName, requestBody, requestHeader)
}

func runGetDataAgentInfoInvokeTest(t *testing.T, ctx context.Context, tr cloudGDATransport, dataAgentName, dataAgentDisplayName string) {
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
		toolName      string
		requestHeader map[string]string
		args          map[string]any
		want          string
		isErr         bool
	}{
		{
			name:          "invoke my-get-data-agent-info-tool",
			toolName:      "my-get-data-agent-info-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"data_agent_id": dataAgentName},
			want:          dataAgentDisplayName,
			isErr:         false,
		},
		{
			name:          "invoke my-auth-get-data-agent-info-tool with auth token",
			toolName:      "my-auth-get-data-agent-info-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"data_agent_id": dataAgentName},
			want:          dataAgentDisplayName,
			isErr:         false,
		},
		{
			name:          "invoke my-auth-get-data-agent-info-tool without auth token",
			toolName:      "my-auth-get-data-agent-info-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"data_agent_id": dataAgentName},
			isErr:         true,
		},
		{
			name:          "invoke my-client-auth-get-data-agent-info-tool with auth token",
			toolName:      "my-client-auth-get-data-agent-info-tool",
			requestHeader: map[string]string{"Authorization": accessToken},
			args:          map[string]any{"data_agent_id": dataAgentName},
			want:          dataAgentDisplayName,
			isErr:         false,
		},
		{
			name:          "invoke my-client-auth-get-data-agent-info-tool without auth token",
			toolName:      "my-client-auth-get-data-agent-info-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"data_agent_id": dataAgentName},
			isErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			res := tr.mustInvoke(t, ctx, tc.toolName, tc.args, tc.requestHeader)
			if res.status != http.StatusOK || res.toolErr {
				if tc.isErr && res.status != http.StatusOK {
					return
				}
				t.Fatalf("response status code is not 200 (tool error: %v), got %d: %s", res.toolErr, res.status, res.result)
			}
			if tc.isErr {
				t.Fatalf("expected an error response, got status %d", res.status)
			}

			got := res.result
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected %q to contain %q, but it did not", got, tc.want)
			}
		})
	}
}

func runAskDataAgentInvokeTest(t *testing.T, ctx context.Context, tr cloudGDATransport, dataAgentID string) {
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
		toolName      string
		requestHeader map[string]string
		args          map[string]any
		want          string
		isErr         bool
	}{
		{
			name:          "invoke my-ask-data-agent-tool",
			toolName:      "my-ask-data-agent-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"user_query_with_context": "What are the names in the table?", "data_agent_id": dataAgentID},
			want:          dataAgentWant,
			isErr:         false,
		},
		{
			name:          "invoke my-auth-ask-data-agent-tool with auth token",
			toolName:      "my-auth-ask-data-agent-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"user_query_with_context": "What are the names in the table?", "data_agent_id": dataAgentID},
			want:          dataAgentWant,
			isErr:         false,
		},
		{
			name:          "invoke my-auth-ask-data-agent-tool without auth token",
			toolName:      "my-auth-ask-data-agent-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"user_query_with_context": "What are the names in the table?", "data_agent_id": dataAgentID},
			isErr:         true,
		},
		{
			name:          "invoke my-client-auth-ask-data-agent-tool with auth token",
			toolName:      "my-client-auth-ask-data-agent-tool",
			requestHeader: map[string]string{"Authorization": accessToken},
			args:          map[string]any{"user_query_with_context": "What are the names in the table?", "data_agent_id": dataAgentID},
			want:          dataAgentWant,
			isErr:         false,
		},
		{
			name:          "invoke my-client-auth-ask-data-agent-tool without auth token",
			toolName:      "my-client-auth-ask-data-agent-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"user_query_with_context": "What are the names in the table?", "data_agent_id": dataAgentID},
			isErr:         true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var res cloudGDAResult
			var err error

			for i := 0; i < maxRetries; i++ {
				reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
				res, err = tr.invoke(t, reqCtx, tc.toolName, tc.args, tc.requestHeader)
				cancel()
				if err != nil {
					// Retry on time out.
					if os.IsTimeout(err) {
						t.Logf("Request timed out (attempt %d/%d), retrying...", i+1, maxRetries)
						if err := sleepCtx(ctx, 5*time.Second); err != nil {
							t.Fatalf("context done while retrying: %v", err)
						}
						continue
					}
					t.Fatalf("unable to send request: %s", err)
				}
				if res.status == http.StatusServiceUnavailable {
					t.Logf("Received 503 Service Unavailable (attempt %d/%d), retrying...", i+1, maxRetries)
					if err := sleepCtx(ctx, 15*time.Second); err != nil {
						t.Fatalf("context done while retrying: %v", err)
					}
					continue
				}
				break
			}

			if err != nil {
				t.Fatalf("Request failed after %d retries: %v", maxRetries, err)
			}

			if res.status != http.StatusOK || res.toolErr {
				if tc.isErr && res.status != http.StatusOK {
					return
				}
				t.Fatalf("response status code is not 200 (tool error: %v), got %d: %s", res.toolErr, res.status, res.result)
			}
			if tc.isErr {
				t.Fatalf("expected an error response, got status %d", res.status)
			}

			got := res.result

			wantPattern := regexp.MustCompile(tc.want)
			if !wantPattern.MatchString(got) {
				t.Fatalf("response did not match the expected pattern.\nFull response:\n%s", got)
			}
		})
	}
}

// sleepCtx waits for d, returning early with the context's error if ctx is
// done first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
