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

func TestLogAdminToolEndpoints(t *testing.T) {
	sourceConfig := getLogAdminVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	adminClient, err := initLogAdminConnection(LogAdminProject)
	if err != nil {
		t.Fatalf("unable to connect to logs: %s", err)
	}
	defer adminClient.Close()

	loggingClient, err := initLogConnection(LogAdminProject)
	if err != nil {
		t.Fatalf("unable to connect to logging: %s", err)
	}
	defer loggingClient.Close()

	testUUID := strings.ReplaceAll(uuid.New().String(), "-", "")
	logName := fmt.Sprintf("toolbox-integration-test-%s", testUUID)

	// set up test logs and wait for logs to be ingested.
	defer teardownTestLogs(t, ctx, adminClient, logName)
	setupTestLogs(t, loggingClient, logName)

	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer waitCancel()
	if err := waitForCloudLoggingEntries(waitCtx, adminClient, LogAdminProject, logName, 3); err != nil {
		t.Fatalf("test log %s was not visible before timeout: %v", logName, err)
	}

	toolsFile := getCloudLoggingAdminToolsConfig(sourceConfig)
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	serverWaitCtx, serverWaitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer serverWaitCancel()
	out, err := testutils.WaitForString(serverWaitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs:\n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	runListLogNamesTest(t)
	runAuthListLogNamesTest(t)
	runListResourceTypesTest(t)
	runQueryLogsTest(t, logName)
	runQueryLogsErrorTest(t)
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

func runListLogNamesTest(t *testing.T) {
	t.Run("list-log-names", func(t *testing.T) {
		resp, respBody := tests.RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/api/tool/list-log-names/invoke", bytes.NewBuffer([]byte(`{}`)), nil)
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}

		var body map[string]interface{}
		if err := json.Unmarshal(respBody, &body); err != nil {
			t.Fatalf("error parsing response body: %v", err)
		}

		result, ok := body["result"].(string)
		if !ok {
			t.Fatalf("expected result to be string")
		}

		var logs []string
		if err := json.Unmarshal([]byte(result), &logs); err != nil {
			t.Fatalf("expected result to be a JSON array of strings: %v", err)
		}
		if len(logs) == 0 {
			t.Errorf("expected result to contain at least one log")
		}
	})
}

func runAuthListLogNamesTest(t *testing.T) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}
	requestHeader := map[string]string{"my-google-auth_token": idToken}
	t.Run("auth-list-log-names", func(t *testing.T) {
		resp, respBody := tests.RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/api/tool/auth-list-log-names/invoke", bytes.NewBuffer([]byte(`{}`)), requestHeader)
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}

		var body map[string]interface{}
		if err := json.Unmarshal(respBody, &body); err != nil {
			t.Fatalf("error parsing response body: %v", err)
		}

		result, ok := body["result"].(string)
		if !ok {
			t.Fatalf("expected result to be string")
		}

		var logs []string
		if err := json.Unmarshal([]byte(result), &logs); err != nil {
			t.Fatalf("expected result to be a JSON array of strings: %v", err)
		}
		if len(logs) == 0 {
			t.Errorf("expected result to contain at least one log")
		}
	})
	t.Run("auth-list-log-names-missing-header", func(t *testing.T) {
		resp, _ := tests.RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/api/tool/auth-list-log-names/invoke", bytes.NewBuffer([]byte(`{}`)), nil)
		if resp.StatusCode != 401 {
			t.Fatalf("expected status 401 (Unauthorized), got %d", resp.StatusCode)
		}
	})
}

func runListResourceTypesTest(t *testing.T) {
	t.Run("list-resource-types", func(t *testing.T) {
		resp, respBody := tests.RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/api/tool/list-resource-types/invoke", bytes.NewBuffer([]byte(`{}`)), nil)

		if resp.StatusCode != 200 {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}

		var body map[string]interface{}
		if err := json.Unmarshal(respBody, &body); err != nil {
			t.Fatalf("error parsing response body")
		}

		result, ok := body["result"].(string)
		if !ok {
			t.Fatalf("expected result to be string")
		}

		expectedTypes := []string{"global", "gce_instance", "gcs_bucket", "project"}
		for _, resourceType := range expectedTypes {
			if !strings.Contains(result, resourceType) {
				t.Errorf("expected '%s' resource type in result, but it was missing", resourceType)
			}
		}
	})
}

func runQueryLogsTest(t *testing.T, logName string) {
	baseFilter := fmt.Sprintf(`logName="projects/%s/logs/%s"`, LogAdminProject, logName)

	t.Run("query-logs-simple", func(t *testing.T) {
		requestBody := fmt.Sprintf(`{"filter": %q, "limit": 10}`, baseFilter)
		result := invokeQueryTool(t, requestBody)

		if !strings.Contains(result, "test entry") {
			t.Errorf("expected test entries in result: %s", result)
		}
	})

	t.Run("query-logs-newest-first", func(t *testing.T) {
		requestBody := fmt.Sprintf(`{"filter": %q, "limit": 10, "newestFirst": true}`, baseFilter)
		result := invokeQueryTool(t, requestBody)

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
		requestBody := fmt.Sprintf(`{"filter": %q, "limit": 10, "verbose": true}`, baseFilter)
		result := invokeQueryTool(t, requestBody)

		if !strings.Contains(result, `"labels":`) {
			t.Errorf("expected 'labels' field in verbose output, got: %s", result)
		}
		if !strings.Contains(result, `"env":"test"`) && !strings.Contains(result, `"env": "test"`) {
			t.Errorf("expected label 'env: test' in verbose output, got: %s", result)
		}
	})

	// Regression: a filter that matches nothing must come back as an empty
	// JSON array, not null.
	t.Run("query-logs-no-match-returns-empty-slice", func(t *testing.T) {
		requestBody := fmt.Sprintf(`{"filter": %q, "limit": 10}`, baseFilter+` AND textPayload:"no_such_entry_9f3b7c"`)
		result := invokeQueryTool(t, requestBody)

		if result != "[]" {
			t.Fatalf("a no-match query should serialize as an empty slice, got: %s", result)
		}
	})
}

func invokeQueryTool(t *testing.T, requestBody string) string {
	t.Helper()
	resp, respBody := tests.RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/api/tool/query-logs/invoke", bytes.NewBuffer([]byte(requestBody)), nil)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(respBody, &body); err != nil {
		t.Fatalf("error parsing response body")
	}

	result, ok := body["result"].(string)
	if !ok {
		t.Fatalf("expected result to be string")
	}
	return result
}

func runQueryLogsErrorTest(t *testing.T) {
	t.Run("query-logs-error", func(t *testing.T) {
		requestBody := `{"filter": "INVALID_FILTER_SYNTAX :::", "limit": 10}`
		resp, _ := tests.RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/api/tool/query-logs/invoke", bytes.NewBuffer([]byte(requestBody)), nil)
		if resp.StatusCode != 200 {
			t.Errorf("expected 200 OK")
		}
	})
}
