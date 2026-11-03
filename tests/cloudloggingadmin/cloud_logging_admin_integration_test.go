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

// To run these tests, set the following environment variables:
// LOGADMIN_PROJECT: Google Cloud project ID.
package cloudloggingadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/logadmin"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

var (
	LogAdminSourceType = "cloud-logging-admin"
	LogAdminProject    = os.Getenv("LOGADMIN_PROJECT")
)

func getLogAdminVars(t *testing.T) map[string]any {
	switch "" {
	case LogAdminProject:
		t.Fatal("'LOGADMIN_PROJECT' not set")
	}

	return map[string]any{
		"type":    LogAdminSourceType,
		"project": LogAdminProject,
	}
}

// Copied over from cloud_logging_admin.go
func initLogAdminConnection(project string) (*logadmin.Client, error) {
	ctx := context.Background()
	cred, err := google.FindDefaultCredentials(ctx, logging.AdminScope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials with scope %q: %w", logging.AdminScope, err)
	}
	client, err := logadmin.NewClient(ctx, project, option.WithCredentials(cred))
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Logging Admin client for project %q: %w", project, err)
	}
	return client, nil
}

// This client will be used to add logs to the project
func initLogConnection(project string) (*logging.Client, error) {
	ctx := context.Background()
	cred, err := google.FindDefaultCredentials(ctx, logging.WriteScope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials with scope %q: %w", logging.WriteScope, err)
	}
	client, err := logging.NewClient(ctx, project, option.WithCredentials(cred))
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Logging client for project %q: %w", project, err)
	}
	return client, nil
}

// setupLogAdminTest writes test log entries and waits until they are visible,
// returning the tools file and the test log name. The clients are closed and
// the test log is deleted on cleanup.
func setupLogAdminTest(t *testing.T, ctx context.Context) (map[string]any, string) {
	t.Helper()
	sourceConfig := getLogAdminVars(t)

	adminClient, err := initLogAdminConnection(LogAdminProject)
	if err != nil {
		t.Fatalf("unable to connect to logs: %s", err)
	}
	t.Cleanup(func() {
		if err := adminClient.Close(); err != nil {
			t.Errorf("unable to close Cloud Logging Admin client: %s", err)
		}
	})

	loggingClient, err := initLogConnection(LogAdminProject)
	if err != nil {
		t.Fatalf("unable to connect to logging: %s", err)
	}
	t.Cleanup(func() {
		if err := loggingClient.Close(); err != nil {
			t.Errorf("unable to close Cloud Logging client: %s", err)
		}
	})

	testUUID := strings.ReplaceAll(uuid.New().String(), "-", "")
	logName := fmt.Sprintf("toolbox-integration-test-%s", testUUID)

	// set up test logs and wait for logs to be ingested. The teardown is
	// registered first so a partially failed setup is still cleaned up.
	t.Cleanup(func() { teardownTestLogs(t, ctx, adminClient, logName) })
	setupTestLogs(t, loggingClient, logName)

	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer waitCancel()
	if err := waitForCloudLoggingEntries(waitCtx, adminClient, LogAdminProject, logName, 3); err != nil {
		t.Fatalf("test log %s was not visible before timeout: %v", logName, err)
	}

	return getCloudLoggingAdminToolsConfig(sourceConfig), logName
}

// logAdminTransport selects how the tests talk to the toolbox server: the
// legacy REST API, or the MCP endpoint when isMCP is set.
type logAdminTransport struct {
	isMCP bool
}

// logAdminResult is the outcome of a tool invocation. result holds the tool
// result as the REST API returns it. toolErr is set when the tool call was
// rejected: an MCP error, or an error result over REST.
type logAdminResult struct {
	status  int
	result  string
	toolErr bool
}

// startServer starts the toolbox server with toolsFile and waits until it is
// ready to serve. The REST API is only enabled for the non-MCP transport.
func (tr logAdminTransport) startServer(t *testing.T, ctx context.Context, toolsFile map[string]any) {
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

	serverWaitCtx, serverWaitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer serverWaitCancel()
	out, err := testutils.WaitForString(serverWaitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs:\n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

// invoke calls toolName with args and request headers through the selected
// transport.
func (tr logAdminTransport) invoke(t *testing.T, ctx context.Context, toolName string, args map[string]any, headers map[string]string) logAdminResult {
	t.Helper()
	if tr.isMCP {
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, toolName, args, headers)
		if err != nil {
			return logAdminResult{status: statusCode, result: err.Error(), toolErr: true}
		}
		if mcpResp.Error != nil {
			return logAdminResult{status: statusCode, result: mcpResp.Error.Message, toolErr: true}
		}
		var text strings.Builder
		for _, content := range mcpResp.Result.Content {
			text.WriteString(content.Text)
		}
		if mcpResp.Result.IsError {
			return logAdminResult{status: statusCode, result: text.String(), toolErr: true}
		}
		// Cloud Logging Admin results are a single JSON document, which the MCP
		// server sends as one text content block.
		if len(mcpResp.Result.Content) != 1 {
			t.Fatalf("%s returned %d content blocks, want 1: %v", toolName, len(mcpResp.Result.Content), mcpResp.Result.Content)
		}
		return logAdminResult{status: statusCode, result: text.String()}
	}

	reqBytes, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("error marshaling request body: %s", err)
	}
	api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", toolName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api, bytes.NewBuffer(reqBytes))
	if err != nil {
		t.Fatalf("error creating request: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("error when sending a request: %s", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading response body: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return logAdminResult{status: resp.StatusCode, result: string(respBody)}
	}

	var body map[string]any
	if err := json.Unmarshal(respBody, &body); err != nil {
		t.Fatalf("error parsing response body %q: %s", string(respBody), err)
	}
	if errVal, ok := body["error"]; ok && errVal != nil {
		return logAdminResult{status: resp.StatusCode, result: fmt.Sprint(errVal), toolErr: true}
	}
	result, ok := body["result"].(string)
	if !ok {
		t.Fatalf("expected result to be string, got body: %s", string(respBody))
	}
	// Tool errors are returned over REST as a successful response whose result
	// is an {"error": ...} object.
	var errResult map[string]any
	toolErr := json.Unmarshal([]byte(result), &errResult) == nil && errResult["error"] != nil
	return logAdminResult{status: resp.StatusCode, result: result, toolErr: toolErr}
}

func TestLogAdminToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	toolsFile, logName := setupLogAdminTest(t, ctx)

	tr := logAdminTransport{}
	tr.startServer(t, ctx, toolsFile)

	runLogAdminTests(t, ctx, tr, logName)
}

// runLogAdminTests runs the Cloud Logging Admin tool checks.
func runLogAdminTests(t *testing.T, ctx context.Context, tr logAdminTransport, logName string) {
	runListLogNamesTest(t, ctx, tr)
	runAuthListLogNamesTest(t, ctx, tr)
	runListResourceTypesTest(t, ctx, tr)
	runQueryLogsTest(t, ctx, tr, logName)
	runQueryLogsErrorTest(t, ctx, tr)
}

func setupTestLogs(t *testing.T, client *logging.Client, logName string) {
	now := time.Now().Truncate(time.Second)
	logger := client.Logger(logName)
	logger.Log(logging.Entry{
		Timestamp: now,
		Payload:   map[string]string{"test_id": logName, "message": "test entry 1"},
		Severity:  logging.Info,
		Labels:    map[string]string{"env": "test", "run_id": "1"},
	})

	logger.Log(logging.Entry{
		Timestamp: now.Add(1 * time.Second),
		Payload:   map[string]string{"test_id": logName, "message": "test entry 2"},
		Severity:  logging.Warning,
	})

	logger.Log(logging.Entry{
		Timestamp: now.Add(2 * time.Second),
		Payload:   map[string]string{"test_id": logName, "message": "test entry 3"},
		Severity:  logging.Error,
	})
	if err := logger.Flush(); err != nil {
		t.Fatalf("failed to flush logs: %v", err)
	}
}

func teardownTestLogs(t *testing.T, ctx context.Context, adminClient *logadmin.Client, logName string) {
	cleanupCtx, cancel := teardownTestLogsContext(ctx)
	defer cancel()

	if err := adminClient.DeleteLog(cleanupCtx, logName); err != nil {
		t.Logf("failed to delete test log %s: %v", logName, err)
	}
}

func teardownTestLogsContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
}

func getCloudLoggingAdminToolsConfig(sourceConfig map[string]any) map[string]any {
	return map[string]any{
		"sources": map[string]any{
			"my-logging-instance": sourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"type":     "google",
				"clientId": tests.ClientId,
			},
		},
		"tools": map[string]any{
			"list-log-names": map[string]any{
				"type":        "cloud-logging-admin-list-log-names",
				"source":      "my-logging-instance",
				"description": "Lists log names in the project",
			},
			"list-resource-types": map[string]any{
				"type":        "cloud-logging-admin-list-resource-types",
				"source":      "my-logging-instance",
				"description": "Lists monitored resource types",
			},
			"query-logs": map[string]any{
				"type":        "cloud-logging-admin-query-logs",
				"source":      "my-logging-instance",
				"description": "Queries log entries",
			},
			"auth-list-log-names": map[string]any{
				"type":         "cloud-logging-admin-list-log-names",
				"source":       "my-logging-instance",
				"authRequired": []string{"my-google-auth"},
				"description":  "Lists log names with authentication",
			},
		},
	}
}

// checkLogNamesResult asserts the result is a non-empty JSON array of log names.
func checkLogNamesResult(t *testing.T, res logAdminResult) {
	t.Helper()
	if res.status != http.StatusOK || res.toolErr {
		t.Fatalf("expected status 200, got %d: %s", res.status, res.result)
	}

	var logs []string
	if err := json.Unmarshal([]byte(res.result), &logs); err != nil {
		t.Fatalf("expected result to be a JSON array of strings, got %q: %v", res.result, err)
	}
	if len(logs) == 0 {
		t.Errorf("expected result to contain at least one log")
	}
}

func runListLogNamesTest(t *testing.T, ctx context.Context, tr logAdminTransport) {
	t.Run("list-log-names", func(t *testing.T) {
		checkLogNamesResult(t, tr.invoke(t, ctx, "list-log-names", map[string]any{}, nil))
	})
}

func runAuthListLogNamesTest(t *testing.T, ctx context.Context, tr logAdminTransport) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}
	requestHeader := map[string]string{"my-google-auth_token": idToken}
	t.Run("auth-list-log-names", func(t *testing.T) {
		checkLogNamesResult(t, tr.invoke(t, ctx, "auth-list-log-names", map[string]any{}, requestHeader))
	})
	t.Run("auth-list-log-names-missing-header", func(t *testing.T) {
		res := tr.invoke(t, ctx, "auth-list-log-names", map[string]any{}, nil)
		if res.status != http.StatusUnauthorized {
			t.Fatalf("expected status 401 (Unauthorized), got %d", res.status)
		}
		if tr.isMCP && !strings.Contains(res.result, "unauthorized Tool call") {
			t.Fatalf("expected an unauthorized tool call error, got: %s", res.result)
		}
	})
}

func runListResourceTypesTest(t *testing.T, ctx context.Context, tr logAdminTransport) {
	t.Run("list-resource-types", func(t *testing.T) {
		res := tr.invoke(t, ctx, "list-resource-types", map[string]any{}, nil)
		if res.status != http.StatusOK || res.toolErr {
			t.Fatalf("expected status 200, got %d: %s", res.status, res.result)
		}

		expectedTypes := []string{"global", "gce_instance", "gcs_bucket", "project"}
		for _, resourceType := range expectedTypes {
			if !strings.Contains(res.result, resourceType) {
				t.Errorf("expected '%s' resource type in result, but it was missing", resourceType)
			}
		}
	})
}

func runQueryLogsTest(t *testing.T, ctx context.Context, tr logAdminTransport, logName string) {
	baseFilter := fmt.Sprintf(`logName="projects/%s/logs/%s"`, LogAdminProject, logName)

	t.Run("query-logs-simple", func(t *testing.T) {
		result := invokeQueryTool(t, ctx, tr, map[string]any{"filter": baseFilter, "limit": 10})

		if !strings.Contains(result, "test entry") {
			t.Errorf("expected test entries in result: %s", result)
		}
	})

	t.Run("query-logs-newest-first", func(t *testing.T) {
		result := invokeQueryTool(t, ctx, tr, map[string]any{"filter": baseFilter, "limit": 10, "newestFirst": true})

		idx3 := strings.Index(result, "test entry 3")
		idx1 := strings.Index(result, "test entry 1")

		if idx3 == -1 || idx1 == -1 {
			t.Fatalf("missing expected entries in result: %s", result)
		}

		if idx3 > idx1 {
			t.Errorf("expected entry 3 to appear before entry 1 with newestFirst=true, but got: ...%s... then ...%s...", "test entry 3", "test entry 1")
		}
	})

	t.Run("query-logs-verbose", func(t *testing.T) {
		result := invokeQueryTool(t, ctx, tr, map[string]any{"filter": baseFilter, "limit": 10, "verbose": true})

		if !strings.Contains(result, `"labels":`) {
			t.Errorf("expected 'labels' field in verbose output, got: %s", result)
		}
		if !strings.Contains(result, `"env":"test"`) && !strings.Contains(result, `"env": "test"`) {
			t.Errorf("expected label 'env: test' in verbose output, got: %s", result)
		}
	})
}

func invokeQueryTool(t *testing.T, ctx context.Context, tr logAdminTransport, args map[string]any) string {
	t.Helper()
	res := tr.invoke(t, ctx, "query-logs", args, nil)
	if res.status != http.StatusOK || res.toolErr {
		t.Fatalf("expected status 200, got %d: %s", res.status, res.result)
	}
	return res.result
}

func runQueryLogsErrorTest(t *testing.T, ctx context.Context, tr logAdminTransport) {
	t.Run("query-logs-error", func(t *testing.T) {
		res := tr.invoke(t, ctx, "query-logs", map[string]any{"filter": "INVALID_FILTER_SYNTAX :::", "limit": 10}, nil)
		if res.status != http.StatusOK {
			t.Errorf("expected 200 OK, got %d: %s", res.status, res.result)
		}
		// Over MCP an invalid filter must be reported as an error result.
		if tr.isMCP && !res.toolErr {
			t.Errorf("expected an error result, got: %s", res.result)
		}
	})
}
