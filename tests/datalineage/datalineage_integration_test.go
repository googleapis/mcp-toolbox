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

package datalineage_test

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

	lineage "cloud.google.com/go/datacatalog/lineage/apiv1"
	lineagepb "cloud.google.com/go/datacatalog/lineage/apiv1/lineagepb"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// datalineageCleanupTimeout bounds the deletion of the lineage resources.
// Cleanups detach from the test context's cancellation (it is already cancelled
// when they run), so they need their own deadline to fail fast if the API stops
// responding.
const datalineageCleanupTimeout = 2 * time.Minute

var (
	DatalineageSourceType     = "datalineage"
	DatalineageSearchToolType = "datalineage-search-lineage"
	DatalineageProject        = os.Getenv("DATALINEAGE_PROJECT")
)

func getDatalineageVars(t *testing.T) map[string]any {
	if DatalineageProject == "" {
		t.Fatal("'DATALINEAGE_PROJECT' environment variable not set")
	}
	return map[string]any{
		"type":    DatalineageSourceType,
		"project": DatalineageProject,
	}
}

func initLineageConnection(ctx context.Context) (*lineage.Client, error) {
	cred, err := google.FindDefaultCredentials(ctx, sources.CloudPlatformScope)
	if err != nil {
		return nil, fmt.Errorf("failed to find default Google Cloud credentials: %w", err)
	}

	client, err := lineage.NewClient(ctx, option.WithCredentials(cred))
	if err != nil {
		return nil, fmt.Errorf("failed to create Lineage client %w", err)
	}
	return client, nil
}

// setupDatalineageResources creates a lineage process, run and event linking a
// source entity to a target entity. The process (and everything under it) is
// deleted on cleanup; the cleanup is registered before the process is created
// so a partially failed setup is still cleaned up.
func setupDatalineageResources(t *testing.T, ctx context.Context, client *lineage.Client, project string, uuidStr string) (string, string, string) {
	t.Helper()
	parent := fmt.Sprintf("projects/%s/locations/us", project)
	processID := fmt.Sprintf("mcp-process-%s", uuidStr)
	runID := fmt.Sprintf("mcp-run-%s", uuidStr)
	eventID := fmt.Sprintf("mcp-event-%s", uuidStr)
	processName := fmt.Sprintf("%s/processes/%s", parent, processID)

	sourceFQN := fmt.Sprintf("custom:%s_source", uuidStr)
	targetFQN := fmt.Sprintf("custom:%s_target", uuidStr)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), datalineageCleanupTimeout)
		defer cancel()
		op, err := client.DeleteProcess(cleanupCtx, &lineagepb.DeleteProcessRequest{Name: processName})
		if err != nil {
			if status.Code(err) != codes.NotFound {
				t.Errorf("Failed to delete process %s: %v", processName, err)
			}
			return
		}
		if err := op.Wait(cleanupCtx); err != nil {
			t.Logf("Warning: Failed to wait for delete process %s: %v", processName, err)
		}
	})

	// 1. Create Process
	createProcessReq := &lineagepb.CreateProcessRequest{
		Parent: parent,
		Process: &lineagepb.Process{
			Name:        processName,
			DisplayName: fmt.Sprintf("MCP Test Process %s", uuidStr),
		},
	}
	process, err := client.CreateProcess(ctx, createProcessReq)
	if err != nil {
		t.Fatalf("Failed to create process %s: %v", processID, err)
	}

	// 2. Create Run
	tTime, err := time.Parse(time.RFC3339Nano, "2026-01-01T01:01:01.010Z")
	if err != nil {
		t.Fatalf("failed to parse start time: %v", err)
	}
	startTime := timestamppb.New(tTime)

	createRunReq := &lineagepb.CreateRunRequest{
		Parent: process.GetName(),
		Run: &lineagepb.Run{
			Name:      fmt.Sprintf("%s/runs/%s", process.GetName(), runID),
			StartTime: startTime,
			State:     lineagepb.Run_COMPLETED,
		},
	}
	run, err := client.CreateRun(ctx, createRunReq)
	if err != nil {
		t.Fatalf("Failed to create run %s: %v", runID, err)
	}

	// 3. Create Lineage Event
	createEventReq := &lineagepb.CreateLineageEventRequest{
		Parent: run.GetName(),
		LineageEvent: &lineagepb.LineageEvent{
			Name:      fmt.Sprintf("%s/lineageEvents/%s", run.GetName(), eventID),
			StartTime: startTime,
			Links: []*lineagepb.EventLink{
				{
					Source: &lineagepb.EntityReference{FullyQualifiedName: sourceFQN},
					Target: &lineagepb.EntityReference{FullyQualifiedName: targetFQN},
				},
			},
		},
	}
	_, err = client.CreateLineageEvent(ctx, createEventReq)
	if err != nil {
		t.Fatalf("Failed to create lineage event %s: %v", eventID, err)
	}

	return sourceFQN, targetFQN, process.GetName()
}

func getDatalineageToolsConfig(sourceConfig map[string]any) map[string]any {
	return map[string]any{
		"sources": map[string]any{
			"my-datalineage-source": sourceConfig,
		},
		"tools": map[string]any{
			"my-datalineage-search-tool": map[string]any{
				"type":        DatalineageSearchToolType,
				"source":      "my-datalineage-source",
				"description": "Data Lineage search tool to test end to end functionality.",
			},
		},
	}
}

// setupDatalineageTest creates the lineage resources used by the search tests
// and returns the tools file along with the source and target FQNs and the
// process name. The Lineage client is closed on cleanup.
func setupDatalineageTest(t *testing.T, ctx context.Context) (map[string]any, string, string, string) {
	t.Helper()
	sourceConfig := getDatalineageVars(t)
	project := sourceConfig["project"].(string)

	lineageClient, err := initLineageConnection(ctx)
	if err != nil {
		t.Fatalf("unable to create Lineage connection: %s", err)
	}
	t.Cleanup(func() {
		if err := lineageClient.Close(); err != nil {
			t.Errorf("unable to close Lineage client: %s", err)
		}
	})

	uuidStr := strings.ReplaceAll(uuid.New().String(), "-", "")
	sourceFQN, targetFQN, processName := setupDatalineageResources(t, ctx, lineageClient, project, uuidStr)

	return getDatalineageToolsConfig(sourceConfig), sourceFQN, targetFQN, processName
}

// datalineageTransport selects how the tests talk to the toolbox server: the
// legacy REST API, or the MCP endpoint when isMCP is set.
type datalineageTransport struct {
	isMCP bool
}

// datalineageResult is the outcome of a tool invocation. result holds the tool
// result as the REST API returns it. toolErr is set when the tool reported an
// error: an MCP error result, or an error result over REST.
type datalineageResult struct {
	status  int
	result  string
	toolErr bool
}

// startServer starts the toolbox server with toolsFile and waits until it is
// ready to serve. The REST API is only enabled for the non-MCP transport.
func (tr datalineageTransport) startServer(t *testing.T, ctx context.Context, toolsFile map[string]any) {
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

	waitCtx, waitCancel := context.WithTimeout(ctx, 3*time.Minute)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

// invoke calls toolName with args through the selected transport. It returns
// an error only when the request itself fails, so callers that poll can retry.
func (tr datalineageTransport) invoke(t *testing.T, ctx context.Context, toolName string, args map[string]any) (datalineageResult, error) {
	t.Helper()
	if tr.isMCP {
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, toolName, args, nil)
		if err != nil {
			return datalineageResult{}, err
		}
		if mcpResp.Error != nil {
			return datalineageResult{status: statusCode, result: mcpResp.Error.Message, toolErr: true}, nil
		}
		var text strings.Builder
		for _, content := range mcpResp.Result.Content {
			text.WriteString(content.Text)
		}
		if mcpResp.Result.IsError {
			return datalineageResult{status: statusCode, result: text.String(), toolErr: true}, nil
		}
		// The search tool returns a single JSON document, which the MCP server
		// sends as one text content block.
		if len(mcpResp.Result.Content) != 1 {
			t.Fatalf("%s returned %d content blocks, want 1: %v", toolName, len(mcpResp.Result.Content), mcpResp.Result.Content)
		}
		return datalineageResult{status: statusCode, result: text.String()}, nil
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
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return datalineageResult{}, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return datalineageResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return datalineageResult{status: resp.StatusCode, result: string(bodyBytes)}, nil
	}

	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return datalineageResult{}, fmt.Errorf("error parsing response body %q: %w", string(bodyBytes), err)
	}
	if errVal, ok := body["error"]; ok && errVal != nil {
		return datalineageResult{status: resp.StatusCode, result: fmt.Sprint(errVal), toolErr: true}, nil
	}
	resultStr, _ := body["result"].(string)
	// Tool errors are returned over REST as a successful response whose result
	// is an {"error": ...} object.
	var errResult map[string]any
	toolErr := json.Unmarshal([]byte(resultStr), &errResult) == nil && errResult["error"] != nil
	return datalineageResult{status: resp.StatusCode, result: resultStr, toolErr: toolErr}, nil
}

func TestDatalineageToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	toolsFile, sourceFQN, targetFQN, processName := setupDatalineageTest(t, ctx)

	tr := datalineageTransport{}
	tr.startServer(t, ctx, toolsFile)

	runDatalineageToolGetTest(t)
	runDatalineageSearchTests(t, ctx, tr, sourceFQN, targetFQN, processName)
}

// runDatalineageSearchTests polls until the new lineage link is indexed, then
// checks upstream search, process details and parameter validation.
func runDatalineageSearchTests(t *testing.T, ctx context.Context, tr datalineageTransport, sourceFQN, targetFQN, processName string) {
	reqBody := map[string]any{
		"locations": []string{"us"},
		"root_entities": []any{
			map[string]any{
				"fully_qualified_name": targetFQN,
			},
		},
		"direction": "UPSTREAM",
	}
	t.Log("Polling search lineage index for the new link with exponential backoff...")
	// Poll up to 3 minutes for eventual consistency
	pollTimeout := 3 * time.Minute
	links, err := pollSearchLineage(t, ctx, tr, "my-datalineage-search-tool", reqBody, sourceFQN, targetFQN, pollTimeout)
	if err != nil {
		t.Fatalf("failed to find the link in search index within %s timeout: %v", pollTimeout, err)
	}

	runDatalineageSearchUpstreamTest(t, links, sourceFQN, targetFQN)
	runDatalineageSearchWithProcessDetailsTest(t, ctx, tr, sourceFQN, targetFQN, processName)
	runDatalineageSearchValidationErrorTest(t, ctx, tr, targetFQN)
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

func pollSearchLineage(t *testing.T, ctx context.Context, tr datalineageTransport, toolName string, reqBody map[string]any, wantSourceFQN, wantTargetFQN string, timeout time.Duration) ([]map[string]any, error) {
	startTime := time.Now()
	delay := 2 * time.Second
	maxDelay := 30 * time.Second

	for time.Since(startTime) < timeout {
		t.Logf("Querying search lineage index (elapsed: %s, next poll in %s)...", time.Since(startTime).Round(time.Second), delay)
		res, err := tr.invoke(t, ctx, toolName, reqBody)
		switch {
		case err != nil:
			t.Logf("  request error: %v", err)
		case res.status != http.StatusOK:
			t.Logf("  Server returned HTTP %d: %s", res.status, res.result)
		case res.toolErr:
			t.Logf("  Tool returned error: %s", res.result)
		case res.result != "" && res.result != "null":
			var searchResp struct {
				Links       []map[string]any `json:"links"`
				Unreachable []string         `json:"unreachable"`
			}
			if err := json.Unmarshal([]byte(res.result), &searchResp); err != nil {
				t.Logf("  failed to unmarshal result %q: %v", res.result, err)
				break
			}
			if len(searchResp.Unreachable) > 0 {
				t.Logf("  Unreachable locations detected: %v", searchResp.Unreachable)
			}
			// Check if our link is in the list
			for _, link := range searchResp.Links {
				source, _ := link["source"].(map[string]any)
				target, _ := link["target"].(map[string]any)
				// Assert using snake_case as protobuf generates standard JSON tags in snake_case
				if source["fully_qualified_name"] == wantSourceFQN && target["fully_qualified_name"] == wantTargetFQN {
					t.Logf("Link successfully indexed after %s", time.Since(startTime).String())
					return searchResp.Links, nil // Found!
				}
			}
		}

		if err := sleepCtx(ctx, delay); err != nil {
			return nil, err
		}
		// Exponential backoff
		delay = delay * 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
	return nil, fmt.Errorf("timeout waiting for lineage link %q -> %q", wantSourceFQN, wantTargetFQN)
}

func runDatalineageToolGetTest(t *testing.T) {
	t.Run("get my-datalineage-search-tool manifest", func(t *testing.T) {
		resp, err := http.Get("http://127.0.0.1:5000/api/tool/my-datalineage-search-tool/")
		if err != nil {
			t.Fatalf("error when sending a request: %s", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("response status code is not 200: %d", resp.StatusCode)
		}
		var body map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&body)
		if err != nil {
			t.Fatalf("error parsing response body: %v", err)
		}
		got, ok := body["tools"]
		if !ok {
			t.Fatalf("unable to find tools in response body")
		}

		toolsMap, ok := got.(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'tools' to be a map, got %T", got)
		}
		tool, ok := toolsMap["my-datalineage-search-tool"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected tool 'my-datalineage-search-tool' to be a map, got %T", toolsMap["my-datalineage-search-tool"])
		}
		params, ok := tool["parameters"].([]interface{})
		if !ok {
			t.Fatalf("expected 'parameters' to be a slice, got %T", tool["parameters"])
		}
		paramSet := make(map[string]struct{})
		for _, param := range params {
			paramMap, ok := param.(map[string]interface{})
			if ok {
				if name, ok := paramMap["name"].(string); ok {
					paramSet[name] = struct{}{}
				}
			}
		}
		expectedParams := []string{"locations", "root_entities", "direction", "max_depth", "max_results", "max_process_per_link", "request_process_details"}
		var missing []string
		for _, want := range expectedParams {
			if _, found := paramSet[want]; !found {
				missing = append(missing, want)
			}
		}
		if len(missing) > 0 {
			t.Fatalf("missing parameters for tool my-datalineage-search-tool: %v", missing)
		}
	})
}

func runDatalineageSearchUpstreamTest(t *testing.T, links []map[string]any, sourceFQN, targetFQN string) {
	t.Run("Search Upstream Lineage (API Defaults)", func(t *testing.T) {
		found := false
		for _, link := range links {
			source, _ := link["source"].(map[string]any)
			target, _ := link["target"].(map[string]any)
			depth, _ := link["depth"].(float64)

			if source["fully_qualified_name"] == sourceFQN && target["fully_qualified_name"] == targetFQN {
				found = true
				if depth != 1 {
					t.Errorf("expected depth to be 1, got %f", depth)
				}
				break
			}
		}

		if !found {
			t.Fatalf("failed to find expected link connecting %q -> %q in results", sourceFQN, targetFQN)
		}
	})
}

func runDatalineageSearchWithProcessDetailsTest(t *testing.T, ctx context.Context, tr datalineageTransport, sourceFQN, targetFQN, processName string) {
	t.Run("Search Lineage with Full Process Details (FieldMask)", func(t *testing.T) {
		reqBody := map[string]any{
			"locations": []string{"us"},
			"root_entities": []any{
				map[string]any{
					"fully_qualified_name": targetFQN,
				},
			},
			"direction":               "UPSTREAM",
			"max_process_per_link":    1,
			"request_process_details": true,
		}

		// Poll for process details to appear (eventual consistency of joins in GCP backend)
		startTime := time.Now()
		timeout := 90 * time.Second
		delay := 3 * time.Second
		found := false

		for time.Since(startTime) < timeout {
			if err := ctx.Err(); err != nil {
				t.Fatalf("context done while waiting for process details: %v", err)
			}
			res, err := tr.invoke(t, ctx, "my-datalineage-search-tool", reqBody)
			if err != nil {
				t.Logf("  request error: %v", err)
				if err := sleepCtx(ctx, delay); err != nil {
					t.Fatalf("context done while waiting for process details: %v", err)
				}
				continue
			}
			if res.status != http.StatusOK || res.toolErr {
				t.Logf("  Server returned HTTP %d (tool error: %v): %s", res.status, res.toolErr, res.result)
				if err := sleepCtx(ctx, delay); err != nil {
					t.Fatalf("context done while waiting for process details: %v", err)
				}
				continue
			}
			if res.result == "" || res.result == "null" {
				t.Log("  Empty result in process details query, retrying...")
				if err := sleepCtx(ctx, delay); err != nil {
					t.Fatalf("context done while waiting for process details: %v", err)
				}
				continue
			}

			var searchResp struct {
				Links       []map[string]any `json:"links"`
				Unreachable []string         `json:"unreachable"`
			}
			if err := json.Unmarshal([]byte(res.result), &searchResp); err != nil {
				t.Logf("  failed to unmarshal search response %q: %v", res.result, err)
				if err := sleepCtx(ctx, delay); err != nil {
					t.Fatalf("context done while waiting for process details: %v", err)
				}
				continue
			}
			links := searchResp.Links

			for _, link := range links {
				source, _ := link["source"].(map[string]any)
				target, _ := link["target"].(map[string]any)

				if source["fully_qualified_name"] == sourceFQN && target["fully_qualified_name"] == targetFQN {
					processesRaw, ok := link["processes"].([]any)
					if ok && len(processesRaw) > 0 {
						processLinkInfo, ok := processesRaw[0].(map[string]any)
						if ok {
							process, ok := processLinkInfo["process"].(map[string]any)
							if ok {
								displayName, _ := process["display_name"].(string)
								if displayName != "" {
									if !strings.HasPrefix(displayName, "MCP Test Process") {
										t.Errorf("expected display_name to start with 'MCP Test Process', got %q", displayName)
									}
									if process["name"] != processName {
										t.Errorf("expected process name %q, got %q", processName, process["name"])
									}
									t.Logf("Process details successfully retrieved after %s", time.Since(startTime).String())
									found = true
									break
								}
							}
						}
					}
				}
			}

			if found {
				break
			}
			t.Log("  Process details display_name not populated yet, retrying...")
			if err := sleepCtx(ctx, delay); err != nil {
				t.Fatalf("context done while waiting for process details: %v", err)
			}
		}

		if !found {
			t.Fatalf("timeout waiting for full process details to be materialized")
		}
	})
}

func runDatalineageSearchValidationErrorTest(t *testing.T, ctx context.Context, tr datalineageTransport, targetFQN string) {
	t.Run("Search Lineage Validation Error (Missing max_process)", func(t *testing.T) {
		reqBody := map[string]any{
			"locations": []string{"us"},
			"root_entities": []any{
				map[string]any{
					"fully_qualified_name": targetFQN,
				},
			},
			"direction":               "UPSTREAM",
			"request_process_details": true,
			// max_process_per_link is omitted (defaults to 0)
		}

		res, err := tr.invoke(t, ctx, "my-datalineage-search-tool", reqBody)
		if err != nil {
			t.Fatalf("unable to send request: %s", err)
		}

		// Validation errors (Agent errors) should return 200 OK with the error
		// in the body; over MCP they are reported as an error result.
		if res.status != http.StatusOK {
			t.Fatalf("response status code is not 200. It is %d. Body: %s", res.status, res.result)
		}
		if tr.isMCP && !res.toolErr {
			t.Fatalf("expected an error result, got: %s", res.result)
		}

		if !strings.Contains(res.result, "max_process_per_link must be greater than 0 when request_process_details is true") {
			t.Fatalf("expected validation error message, got: %s", res.result)
		}
	})
}
