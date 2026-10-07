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
	"google.golang.org/protobuf/types/known/timestamppb"
)

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

func setupDatalineageResources(t *testing.T, ctx context.Context, client *lineage.Client, project string, uuidStr string) (string, string, string, func(*testing.T)) {
	parent := fmt.Sprintf("projects/%s/locations/us", project)
	processID := fmt.Sprintf("mcp-process-%s", uuidStr)
	runID := fmt.Sprintf("mcp-run-%s", uuidStr)
	eventID := fmt.Sprintf("mcp-event-%s", uuidStr)

	sourceFQN := fmt.Sprintf("custom:%s_source", uuidStr)
	targetFQN := fmt.Sprintf("custom:%s_target", uuidStr)

	// 1. Create Process
	createProcessReq := &lineagepb.CreateProcessRequest{
		Parent: parent,
		Process: &lineagepb.Process{
			Name:        fmt.Sprintf("%s/processes/%s", parent, processID),
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

	// Teardown function
	teardown := func(t *testing.T) {
		deleteProcessReq := &lineagepb.DeleteProcessRequest{
			Name: process.GetName(),
		}
		op, err := client.DeleteProcess(ctx, deleteProcessReq)
		if err != nil {
			t.Errorf("Failed to delete process %s: %v", process.GetName(), err)
			return
		}
		if err := op.Wait(ctx); err != nil {
			t.Logf("Warning: Failed to wait for delete process %s: %v", process.GetName(), err)
		}
	}

	return sourceFQN, targetFQN, process.GetName(), teardown
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

func TestDatalineageToolEndpoints(t *testing.T) {
	sourceConfig := getDatalineageVars(t)
	project := sourceConfig["project"].(string)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}

	lineageClient, err := initLineageConnection(ctx)
	if err != nil {
		t.Fatalf("unable to create Lineage connection: %s", err)
	}
	defer lineageClient.Close()

	uuidStr := strings.ReplaceAll(uuid.New().String(), "-", "")
	sourceFQN, targetFQN, processName, teardownResources := setupDatalineageResources(t, ctx, lineageClient, project, uuidStr)
	defer teardownResources(t)

	toolsFile := getDatalineageToolsConfig(sourceConfig)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	runDatalineageToolGetTest(t)

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
	// Poll up to 5 minutes for eventual consistency
	pollTimeout := 5 * time.Minute
	links, err := pollSearchLineage(t, "my-datalineage-search-tool", reqBody, sourceFQN, targetFQN, pollTimeout)
	if err != nil {
		t.Fatalf("failed to find the link in search index within %s timeout: %v", pollTimeout, err)
	}

	runDatalineageSearchUpstreamTest(t, links, sourceFQN, targetFQN)
	runDatalineageSearchWithProcessDetailsTest(t, sourceFQN, targetFQN, processName)
	runDatalineageSearchValidationErrorTest(t, targetFQN)
}

func pollSearchLineage(t *testing.T, toolName string, reqBody map[string]any, wantSourceFQN, wantTargetFQN string, timeout time.Duration) ([]map[string]any, error) {
	reqBytes, _ := json.Marshal(reqBody)
	startTime := time.Now()
	delay := 2 * time.Second
	maxDelay := 30 * time.Second

	for time.Since(startTime) < timeout {
		t.Logf("Querying search lineage index (elapsed: %s, next poll in %s)...", time.Since(startTime).Round(time.Second), delay)
		resp, err := http.Post(
			fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", toolName),
			"application/json",
			bytes.NewBuffer(reqBytes),
		)
		if err == nil {
			defer resp.Body.Close()
			bodyBytes, _ := io.ReadAll(resp.Body)
			if resp.StatusCode == 200 {
				var result map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &result); err == nil {
					if errVal, ok := result["error"]; ok && errVal != nil {
						t.Logf("  Tool returned error: %v", errVal)
					} else {
						resultStr, ok := result["result"].(string)
						if ok && resultStr != "" && resultStr != "null" {
							var searchResp struct {
								Links       []map[string]any `json:"links"`
								Unreachable []string         `json:"unreachable"`
							}
							if err := json.Unmarshal([]byte(resultStr), &searchResp); err == nil {
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
							} else {
								t.Logf("  failed to unmarshal resultStr: %v", err)
							}
						}
					}
				}
			} else {
				t.Logf("  Server returned HTTP %d: %s", resp.StatusCode, string(bodyBytes))
			}
		} else {
			t.Logf("  HTTP POST error: %v", err)
		}

		time.Sleep(delay)
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

func runDatalineageSearchWithProcessDetailsTest(t *testing.T, sourceFQN, targetFQN, processName string) {
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
		reqBytes, _ := json.Marshal(reqBody)

		// Poll for process details to appear (eventual consistency of joins in GCP backend)
		startTime := time.Now()
		timeout := 90 * time.Second
		delay := 3 * time.Second
		found := false

		for time.Since(startTime) < timeout {
			resp, err := http.Post(
				"http://127.0.0.1:5000/api/tool/my-datalineage-search-tool/invoke",
				"application/json",
				bytes.NewBuffer(reqBytes),
			)
			if err != nil {
				t.Logf("  HTTP POST error: %v", err)
				time.Sleep(delay)
				continue
			}
			defer resp.Body.Close()

			bodyBytes, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != 200 {
				t.Logf("  Server returned HTTP %d: %s", resp.StatusCode, string(bodyBytes))
				time.Sleep(delay)
				continue
			}

			var result map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Logf("  failed to decode response body: %v", err)
				time.Sleep(delay)
				continue
			}

			resultStr, ok := result["result"].(string)
			if !ok || resultStr == "" || resultStr == "null" {
				t.Log("  Empty result in process details query, retrying...")
				time.Sleep(delay)
				continue
			}

			var searchResp struct {
				Links       []map[string]any `json:"links"`
				Unreachable []string         `json:"unreachable"`
			}
			if err := json.Unmarshal([]byte(resultStr), &searchResp); err != nil {
				t.Logf("  failed to unmarshal search response: %v", err)
				time.Sleep(delay)
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
			time.Sleep(delay)
		}

		if !found {
			t.Fatalf("timeout waiting for full process details to be materialized")
		}
	})
}

func runDatalineageSearchValidationErrorTest(t *testing.T, targetFQN string) {
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
		reqBytes, _ := json.Marshal(reqBody)

		resp, err := http.Post(
			"http://127.0.0.1:5000/api/tool/my-datalineage-search-tool/invoke",
			"application/json",
			bytes.NewBuffer(reqBytes),
		)
		if err != nil {
			t.Fatalf("unable to send request: %s", err)
		}
		defer resp.Body.Close()

		// Validation errors (Agent errors) should return 200 OK with error in body
		if resp.StatusCode != 200 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("response status code is not 200. It is %d. Body: %s", resp.StatusCode, string(bodyBytes))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("error parsing response body: %s", err)
		}

		resultStr, ok := result["result"].(string)
		if !ok {
			t.Fatalf("expected 'result' field to be a string, got %T", result["result"])
		}

		if !strings.Contains(resultStr, "max_process_per_link must be greater than 0 when request_process_details is true") {
			t.Fatalf("expected validation error message, got: %s", resultStr)
		}
	})
}
