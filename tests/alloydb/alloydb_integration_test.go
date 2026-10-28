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

package alloydb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

func TestAlloyDBToolEndpoints(t *testing.T) {
	vars := getAlloyDBVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	args := []string{"--enable-api"}
	toolsFile := getAlloyDBToolsConfig()

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %v", err)
	}
	defer cleanup()

	waitCtx, cancelWait := context.WithTimeout(ctx, 20*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %v", err)
	}

	runAlloyDBToolGetTest(t)
	runAlloyDBMCPToolCallMethod(t, vars)

	// Run tool-specific invoke tests
	runAlloyDBListClustersTest(t, vars)
	runAlloyDBListInstancesTest(t, vars)
	runAlloyDBListUsersTest(t, vars)
	runAlloyDBGetClusterTest(t, vars)
	runAlloyDBGetInstanceTest(t, vars)
	runAlloyDBGetUserTest(t, vars)
}

func runAlloyDBToolGetTest(t *testing.T) {
	tcs := []struct {
		name string
		api  string
		want map[string]any
	}{
		{
			name: "get my-simple-tool",
			api:  "http://127.0.0.1:5000/api/tool/my-simple-tool/",
			want: map[string]any{
				"my-simple-tool": map[string]any{
					"description": "Simple tool to test end to end functionality.",
					"parameters": []any{
						map[string]any{"name": "project", "type": "string", "description": "The GCP project ID to list clusters for.", "required": true, "authServices": []any{}},
						map[string]any{"name": "location", "type": "string", "description": "Optional: The location to list clusters in (e.g., 'us-central1'). Use '-' to list clusters across all locations.(Default: '-')", "required": false, "default": "-", "authServices": []any{}},
					},
					"authRequired": []any{},
				},
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(tc.api)
			if err != nil {
				t.Fatalf("error when sending a request: %s", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("response status code is not 200")
			}

			var body map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}

			got, ok := body["tools"]
			if !ok {
				t.Fatalf("unable to find 'tools' in response body")
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("response mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func runAlloyDBMCPToolCallMethod(t *testing.T, vars map[string]string) {
	sessionId := tests.RunInitialize(t, "2024-11-05")
	header := map[string]string{}
	if sessionId != "" {
		header["Mcp-Session-Id"] = sessionId
	}

	invokeTcs := []struct {
		name         string
		requestBody  jsonrpc.JSONRPCRequest
		wantContains string
		isErr        bool
	}{
		{
			name: "MCP Invoke my-param-tool",
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "my-param-tool-mcp",
				Request: jsonrpc.Request{Method: "tools/call"},
				Params: map[string]any{
					"name": "my-param-tool",
					"arguments": map[string]any{
						"project":  vars["project"],
						"location": vars["location"],
					},
				},
			},
			wantContains: fmt.Sprintf(`"name\":\"projects/%s/locations/%s/clusters/%s\"`, vars["project"], vars["location"], vars["cluster"]),
			isErr:        false,
		},
		{
			name: "MCP Invoke my-fail-tool",
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke-fail-tool",
				Request: jsonrpc.Request{Method: "tools/call"},
				Params: map[string]any{
					"name": "my-fail-tool",
					"arguments": map[string]any{
						"location": vars["location"],
					},
				},
			},
			wantContains: `parameter \"project\" is required`,
			isErr:        true,
		},
		{
			name: "MCP Invoke invalid tool",
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invalid-tool-mcp",
				Request: jsonrpc.Request{Method: "tools/call"},
				Params: map[string]any{
					"name":      "non-existent-tool",
					"arguments": map[string]any{},
				},
			},
			wantContains: `tool with name \"non-existent-tool\" does not exist`,
			isErr:        true,
		},
		{
			name: "MCP Invoke tool without required parameters",
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke-without-params-mcp",
				Request: jsonrpc.Request{Method: "tools/call"},
				Params: map[string]any{
					"name":      "my-param-tool",
					"arguments": map[string]any{"location": vars["location"]},
				},
			},
			wantContains: `parameter \"project\" is required`,
			isErr:        true,
		},
		{
			name: "MCP Invoke my-auth-required-tool",
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke my-auth-required-tool",
				Request: jsonrpc.Request{Method: "tools/call"},
				Params: map[string]any{
					"name":      "my-auth-required-tool",
					"arguments": map[string]any{},
				},
			},
			wantContains: `tool with name \"my-auth-required-tool\" does not exist`,
			isErr:        true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			api := "http://127.0.0.1:5000/mcp"
			reqMarshal, err := json.Marshal(tc.requestBody)
			if err != nil {
				t.Fatalf("unexpected error during marshaling of request body: %v", err)
			}

			req, err := http.NewRequest(http.MethodPost, api, bytes.NewBuffer(reqMarshal))
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("unable to read request body: %s", err)
			}

			got := string(bytes.TrimSpace(respBody))
			if !strings.Contains(got, tc.wantContains) {
				t.Fatalf("Expected substring not found:\ngot:  %q\nwant: %q (to be contained within got)", got, tc.wantContains)
			}
		})
	}
}

func runAlloyDBListClustersTest(t *testing.T, vars map[string]string) {

	type ListClustersResponse struct {
		Clusters []struct {
			Name string `json:"name"`
		} `json:"clusters"`
	}

	type ToolResponse struct {
		Result string `json:"result"`
	}

	// NOTE: If clusters are added, removed or changed in the test project,
	// this list must be updated for the "list clusters specific locations" test to pass
	wantForSpecificLocation := []string{
		fmt.Sprintf("projects/%s/locations/us-central1/clusters/alloydb-ai-nl-testing", vars["project"]),
		fmt.Sprintf("projects/%s/locations/us-central1/clusters/alloydb-pg-testing", vars["project"]),
	}

	// NOTE: If clusters are added, removed, or changed in the test project,
	// this list must be updated for the "list clusters all locations" test to pass
	wantForAllLocations := []string{
		fmt.Sprintf("projects/%s/locations/us-central1/clusters/alloydb-ai-nl-testing", vars["project"]),
		fmt.Sprintf("projects/%s/locations/us-central1/clusters/alloydb-pg-testing", vars["project"]),
		fmt.Sprintf("projects/%s/locations/us-east4/clusters/alloydb-private-pg-testing", vars["project"]),
		fmt.Sprintf("projects/%s/locations/us-east4/clusters/colab-testing", vars["project"]),
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		want           []string
		wantStatusCode int
	}{
		{
			name:           "list clusters for all locations",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "-"}`, vars["project"])),
			want:           wantForAllLocations,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list clusters specific location",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "us-central1"}`, vars["project"])),
			want:           wantForSpecificLocation,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list clusters missing project",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"location": "%s"}`, vars["location"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list clusters non-existent location",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "abcd"}`, vars["project"])),
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name:           "list clusters non-existent project",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "non-existent-project", "location": "%s"}`, vars["location"])),
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name:           "list clusters empty project",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "", "location": "%s"}`, vars["location"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list clusters empty location",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": ""}`, vars["project"])),
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			api := "http://127.0.0.1:5000/api/tool/alloydb-list-clusters/invoke"
			req, err := http.NewRequest(http.MethodPost, api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatusCode {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not %d, got %d: %s", tc.wantStatusCode, resp.StatusCode, string(bodyBytes))
			}

			if tc.wantStatusCode == http.StatusOK {
				var body ToolResponse
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("error parsing outer response body: %v", err)
				}

				var clustersData ListClustersResponse
				if err := json.Unmarshal([]byte(body.Result), &clustersData); err != nil {
					t.Fatalf("error parsing nested result JSON: %v", err)
				}

				var got []string
				for _, cluster := range clustersData.Clusters {
					if strings.HasSuffix(cluster.Name, "/clusters/eval-test-cluster") {
						continue
					}
					got = append(got, cluster.Name)
				}

				sort.Strings(got)
				sort.Strings(tc.want)

				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("cluster list mismatch:\n got: %v\nwant: %v", got, tc.want)
				}
			}
		})
	}
}

func runAlloyDBListUsersTest(t *testing.T, vars map[string]string) {
	type UsersResponse struct {
		Users []struct {
			Name string `json:"name"`
		} `json:"users"`
	}

	type ToolResponse struct {
		Result string `json:"result"`
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantContains   string
		wantStatusCode int
		expectAgentErr bool
	}{
		{
			name:           "list users success",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "%s"}`, vars["project"], vars["location"], vars["cluster"])),
			wantContains:   fmt.Sprintf("projects/%s/locations/%s/clusters/%s/users/%s", vars["project"], vars["location"], vars["cluster"], AlloyDBUser),
			wantStatusCode: http.StatusOK,
			expectAgentErr: false,
		},
		{
			name:           "list users missing project",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"location": "%s", "cluster": "%s"}`, vars["location"], vars["cluster"])),
			wantStatusCode: http.StatusOK,
			wantContains:   `parameter \"project\" is required`,
			expectAgentErr: true,
		},
		{
			name:           "list users missing location",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "cluster": "%s"}`, vars["project"], vars["cluster"])),
			wantStatusCode: http.StatusOK,
			wantContains:   `parameter \"location\" is required`,
			expectAgentErr: true,
		},
		{
			name:           "list users missing cluster",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s"}`, vars["project"], vars["cluster"])),
			wantStatusCode: http.StatusOK,
			wantContains:   `parameter \"cluster\" is required`,
			expectAgentErr: true,
		},
		{
			name:           "list users non-existent cluster",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "non-existent-cluster"}`, vars["project"], vars["location"])),
			wantStatusCode: http.StatusOK,
			wantContains:   `was not found`,
			expectAgentErr: true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			api := "http://127.0.0.1:5000/api/tool/alloydb-list-users/invoke"
			req, err := http.NewRequest(http.MethodPost, api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatusCode {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code: got %d, want %d: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}

			if tc.wantStatusCode == http.StatusOK {
				var body ToolResponse
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("error parsing outer response body: %v", err)
				}

				if tc.expectAgentErr {
					// Logic for checking wrapped error messages
					if !strings.Contains(body.Result, tc.wantContains) {
						t.Errorf("expected agent error message not found:\n got: %s\nwant: %s", body.Result, tc.wantContains)
					}
				} else {
					// Logic for checking successful resource lists
					var usersData UsersResponse
					if err := json.Unmarshal([]byte(body.Result), &usersData); err != nil {
						t.Fatalf("error parsing nested result JSON: %v. Result was: %s", err, body.Result)
					}

					found := false
					for _, user := range usersData.Users {
						if user.Name == tc.wantContains {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected user name %q not found in response", tc.wantContains)
					}
				}
			}
		})
	}
}

func runAlloyDBListInstancesTest(t *testing.T, vars map[string]string) {
	type ListInstancesResponse struct {
		Instances []struct {
			Name string `json:"name"`
		} `json:"instances"`
	}

	type ToolResponse struct {
		Result string `json:"result"`
	}

	wantForSpecificClusterAndLocation := []string{
		fmt.Sprintf("projects/%s/locations/%s/clusters/%s/instances/%s", vars["project"], vars["location"], vars["cluster"], vars["instance"]),
	}

	// NOTE: If clusters or instances are added, removed or changed in the test project,
	// the below lists must be updated for the tests to pass.
	wantForAllClustersSpecificLocation := []string{
		fmt.Sprintf("projects/%s/locations/%s/clusters/alloydb-ai-nl-testing/instances/alloydb-ai-nl-testing-instance", vars["project"], vars["location"]),
		fmt.Sprintf("projects/%s/locations/%s/clusters/alloydb-pg-testing/instances/alloydb-pg-testing-instance", vars["project"], vars["location"]),
	}

	wantForAllClustersAllLocations := []string{
		fmt.Sprintf("projects/%s/locations/us-central1/clusters/alloydb-ai-nl-testing/instances/alloydb-ai-nl-testing-instance", vars["project"]),
		fmt.Sprintf("projects/%s/locations/us-central1/clusters/alloydb-pg-testing/instances/alloydb-pg-testing-instance", vars["project"]),
		fmt.Sprintf("projects/%s/locations/us-east4/clusters/alloydb-private-pg-testing/instances/alloydb-private-pg-testing-instance", vars["project"]),
		fmt.Sprintf("projects/%s/locations/us-east4/clusters/colab-testing/instances/colab-testing-primary", vars["project"]),
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		want           []string
		wantStatusCode int
	}{
		{
			name:           "list instances for a specific cluster and location",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "%s"}`, vars["project"], vars["location"], vars["cluster"])),
			want:           wantForSpecificClusterAndLocation,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list instances for all clusters and specific location",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "-"}`, vars["project"], vars["location"])),
			want:           wantForAllClustersSpecificLocation,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list instances for all clusters and all locations",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "-", "cluster": "-"}`, vars["project"])),
			want:           wantForAllClustersAllLocations,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list instances missing project",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"location": "%s", "cluster": "%s"}`, vars["location"], vars["cluster"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list instances non-existent project",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "non-existent-project", "location": "%s", "cluster": "%s"}`, vars["location"], vars["cluster"])),
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name:           "list instances non-existent location",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "non-existent-location", "cluster": "%s"}`, vars["project"], vars["cluster"])),
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name:           "list instances non-existent cluster",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "non-existent-cluster"}`, vars["project"], vars["location"])),
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			api := "http://127.0.0.1:5000/api/tool/alloydb-list-instances/invoke"
			req, err := http.NewRequest(http.MethodPost, api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatusCode {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not %d, got %d: %s", tc.wantStatusCode, resp.StatusCode, string(bodyBytes))
			}

			if tc.wantStatusCode == http.StatusOK {
				var body ToolResponse
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("error parsing outer response body: %v", err)
				}

				var instancesData ListInstancesResponse
				if err := json.Unmarshal([]byte(body.Result), &instancesData); err != nil {
					t.Fatalf("error parsing nested result JSON: %v", err)
				}

				var got []string
				for _, instance := range instancesData.Instances {
					if strings.Contains(instance.Name, "/clusters/eval-test-cluster/") || strings.HasSuffix(instance.Name, "/instances/eval-test-instance") {
						continue
					}
					got = append(got, instance.Name)
				}

				sort.Strings(got)
				sort.Strings(tc.want)

				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("instance list mismatch:\n got: %v\nwant: %v", got, tc.want)
				}
			}
		})
	}
}

func runAlloyDBGetClusterTest(t *testing.T, vars map[string]string) {
	type ToolResponse struct {
		Result string `json:"result"`
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		want           map[string]any
		wantStatusCode int
	}{
		{
			name:        "get cluster success",
			requestBody: bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "%s"}`, vars["project"], vars["location"], vars["cluster"])),
			want: map[string]any{
				"clusterType": "PRIMARY",
				"name":        fmt.Sprintf("projects/%s/locations/%s/clusters/%s", vars["project"], vars["location"], vars["cluster"]),
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get cluster missing project",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"location": "%s", "cluster": "%s"}`, vars["location"], vars["cluster"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get cluster missing location",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "cluster": "%s"}`, vars["project"], vars["cluster"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get cluster missing cluster",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s"}`, vars["project"], vars["location"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get cluster non-existent cluster",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "non-existent-cluster"}`, vars["project"], vars["location"])),
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			api := "http://127.0.0.1:5000/api/tool/alloydb-get-cluster/invoke"
			req, err := http.NewRequest(http.MethodPost, api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatusCode {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not %d, got %d: %s", tc.wantStatusCode, resp.StatusCode, string(bodyBytes))
			}

			if tc.wantStatusCode == http.StatusOK {
				var body ToolResponse
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("error parsing response body: %v", err)
				}

				if tc.want != nil {
					var gotMap map[string]any
					if err := json.Unmarshal([]byte(body.Result), &gotMap); err != nil {
						t.Fatalf("failed to unmarshal JSON result into map: %v", err)
					}

					got := make(map[string]any)
					for key := range tc.want {
						if value, ok := gotMap[key]; ok {
							got[key] = value
						}
					}

					if diff := cmp.Diff(tc.want, got); diff != "" {
						t.Errorf("Unexpected result: got %#v, want: %#v", got, tc.want)
					}
				}
			}
		})
	}
}

func runAlloyDBGetInstanceTest(t *testing.T, vars map[string]string) {
	type ToolResponse struct {
		Result string `json:"result"`
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		want           map[string]any
		wantStatusCode int
	}{
		{
			name:        "get instance success",
			requestBody: bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "%s", "instance": "%s"}`, vars["project"], vars["location"], vars["cluster"], vars["instance"])),
			want: map[string]any{
				"instanceType": "PRIMARY",
				"name":         fmt.Sprintf("projects/%s/locations/%s/clusters/%s/instances/%s", vars["project"], vars["location"], vars["cluster"], vars["instance"]),
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get instance missing project",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"location": "%s", "cluster": "%s", "instance": "%s"}`, vars["location"], vars["cluster"], vars["instance"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get instance missing location",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "cluster": "%s", "instance": "%s"}`, vars["project"], vars["cluster"], vars["instance"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get instance missing cluster",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "instance": "%s"}`, vars["project"], vars["location"], vars["instance"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get instance missing instance",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "%s"}`, vars["project"], vars["location"], vars["cluster"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get instance non-existent instance",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "%s", "instance": "non-existent-instance"}`, vars["project"], vars["location"], vars["cluster"])),
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			api := "http://127.0.0.1:5000/api/tool/alloydb-get-instance/invoke"
			req, err := http.NewRequest(http.MethodPost, api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatusCode {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not %d, got %d: %s", tc.wantStatusCode, resp.StatusCode, string(bodyBytes))
			}

			if tc.wantStatusCode == http.StatusOK {
				var body ToolResponse
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("error parsing response body: %v", err)
				}

				if tc.want != nil {
					var gotMap map[string]any
					if err := json.Unmarshal([]byte(body.Result), &gotMap); err != nil {
						t.Fatalf("failed to unmarshal JSON result into map: %v", err)
					}

					got := make(map[string]any)
					for key := range tc.want {
						if value, ok := gotMap[key]; ok {
							got[key] = value
						}
					}

					if diff := cmp.Diff(tc.want, got); diff != "" {
						t.Errorf("Unexpected result: got %#v, want: %#v", got, tc.want)
					}
				}
			}
		})
	}
}

func runAlloyDBGetUserTest(t *testing.T, vars map[string]string) {
	type ToolResponse struct {
		Result string `json:"result"`
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		want           map[string]any
		wantStatusCode int
	}{
		{
			name:        "get user success",
			requestBody: bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "%s", "user": "%s"}`, vars["project"], vars["location"], vars["cluster"], vars["user"])),
			want: map[string]any{
				"name":     fmt.Sprintf("projects/%s/locations/%s/clusters/%s/users/%s", vars["project"], vars["location"], vars["cluster"], vars["user"]),
				"userType": "ALLOYDB_BUILT_IN",
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get user missing project",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"location": "%s", "cluster": "%s", "user": "%s"}`, vars["location"], vars["cluster"], vars["user"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get user missing location",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "cluster": "%s", "user": "%s"}`, vars["project"], vars["cluster"], vars["user"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get user missing cluster",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "user": "%s"}`, vars["project"], vars["location"], vars["user"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get user missing user",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "%s"}`, vars["project"], vars["location"], vars["cluster"])),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get non-existent user",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"project": "%s", "location": "%s", "cluster": "%s", "user": "non-existent-user"}`, vars["project"], vars["location"], vars["cluster"])),
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			api := "http://127.0.0.1:5000/api/tool/alloydb-get-user/invoke"
			req, err := http.NewRequest(http.MethodPost, api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatusCode {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not %d, got %d: %s", tc.wantStatusCode, resp.StatusCode, string(bodyBytes))
			}

			if tc.wantStatusCode == http.StatusOK {
				var body ToolResponse
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("error parsing response body: %v", err)
				}

				if tc.want != nil {
					var gotMap map[string]any
					if err := json.Unmarshal([]byte(body.Result), &gotMap); err != nil {
						t.Fatalf("failed to unmarshal JSON result into map: %v", err)
					}

					got := make(map[string]any)
					for key := range tc.want {
						if value, ok := gotMap[key]; ok {
							got[key] = value
						}
					}

					if diff := cmp.Diff(tc.want, got); diff != "" {
						t.Errorf("Unexpected result: got %#v, want: %#v", got, tc.want)
					}
				}
			}
		})
	}
}

type mockAlloyDBTransport struct {
	transport http.RoundTripper
	url       *url.URL
}

func (t *mockAlloyDBTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), "https://alloydb.googleapis.com") {
		req.URL.Scheme = t.url.Scheme
		req.URL.Host = t.url.Host
	}
	return t.transport.RoundTrip(req)
}

type mockAlloyDBHandler struct {
	t       *testing.T
	idParam string
}

func (h *mockAlloyDBHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.UserAgent(), "genai-toolbox/") {
		h.t.Errorf("User-Agent header not found")
	}

	id := r.URL.Query().Get(h.idParam)

	var response string
	var statusCode int

	switch id {
	case "c1-success":
		response = `{
			"name": "projects/p1/locations/l1/operations/mock-operation-success",
			"metadata": {
				"verb": "create",
				"target": "projects/p1/locations/l1/clusters/c1-success"
			}
		}`
		statusCode = http.StatusOK
	case "c2-api-failure":
		response = `{"error":{"message":"internal api error"}}`
		statusCode = http.StatusInternalServerError
	case "i1-success":
		response = `{
			"metadata": {
				"@type": "type.googleapis.com/google.cloud.alloydb.v1.OperationMetadata",
				"target": "projects/p1/locations/l1/clusters/c1/instances/i1-success",
				"verb": "create",
				"requestedCancellation": false,
				"apiVersion": "v1"
			},
			"name": "projects/p1/locations/l1/operations/mock-operation-success"
		}`
		statusCode = http.StatusOK
	case "i2-api-failure":
		response = `{"error":{"message":"internal api error"}}`
		statusCode = http.StatusInternalServerError
	case "u1-iam-success":
		response = `{
			"databaseRoles": ["alloydbiamuser"],
			"name": "projects/p1/locations/l1/clusters/c1/users/u1-iam-success",
			"userType": "ALLOYDB_IAM_USER"
		}`
		statusCode = http.StatusOK
	case "u2-builtin-success":
		response = `{
			"databaseRoles": ["alloydbsuperuser"],
			"name": "projects/p1/locations/l1/clusters/c1/users/u2-builtin-success",
			"userType": "ALLOYDB_BUILT_IN"
		}`
		statusCode = http.StatusOK
	case "u3-api-failure":
		response = `{"error":{"message":"user internal api error"}}`
		statusCode = http.StatusInternalServerError
	default:
		http.Error(w, fmt.Sprintf("unhandled %s in mock server: %s", h.idParam, id), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if _, err := w.Write([]byte(response)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func setupTestServer(t *testing.T, idParam string) func() {
	handler := &mockAlloyDBHandler{t: t, idParam: idParam}
	server := httptest.NewServer(handler)

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}

	originalTransport := http.DefaultClient.Transport
	if originalTransport == nil {
		originalTransport = http.DefaultTransport
	}
	http.DefaultClient.Transport = &mockAlloyDBTransport{
		transport: originalTransport,
		url:       serverURL,
	}

	return func() {
		server.Close()
		http.DefaultClient.Transport = originalTransport
	}
}

func TestAlloyDBCreateCluster(t *testing.T) {
	cleanup := setupTestServer(t, "clusterId")
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}
	toolsFile := getAlloyDBToolsConfig()
	cmd, cleanupCmd, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %v", err)
	}
	defer cleanupCmd()

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	tcs := []struct {
		name           string
		body           string
		want           string
		wantStatusCode int
	}{
		{
			name:           "successful creation",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1-success", "password": "p1"}`,
			want:           `{"name":"projects/p1/locations/l1/operations/mock-operation-success", "metadata": {"verb": "create", "target": "projects/p1/locations/l1/clusters/c1-success"}}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "api failure",
			body:           `{"project": "p1", "location": "l1", "cluster": "c2-api-failure", "password": "p1"}`,
			want:           `{"error":"error processing GCP request: error creating AlloyDB cluster: googleapi: Error 500: internal api error"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing project",
			body:           `{"location": "l1", "cluster": "c1", "password": "p1"}`,
			want:           `{"error":"parameter \"project\" is required"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing cluster",
			body:           `{"project": "p1", "location": "l1", "password": "p1"}`,
			want:           `{"error":"parameter \"cluster\" is required"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing password",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1"}`,
			want:           `{"error":"parameter \"password\" is required"}`,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			api := "http://127.0.0.1:5000/api/tool/alloydb-create-cluster/invoke"
			req, err := http.NewRequest(http.MethodPost, api, bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			bodyBytes, _ := io.ReadAll(resp.Body)

			if tc.wantStatusCode != http.StatusOK {
				if tc.want != "" && !bytes.Contains(bodyBytes, []byte(tc.want)) {
					t.Fatalf("expected error response to contain %q, but got: %s", tc.want, string(bodyBytes))
				}
				return
			}

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var result struct {
				Result string `json:"result"`
			}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			var got, want map[string]any
			if err := json.Unmarshal([]byte(result.Result), &got); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatalf("failed to unmarshal want: %v", err)
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAlloyDBCreateInstance(t *testing.T) {
	cleanup := setupTestServer(t, "instanceId")
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}
	toolsFile := getAlloyDBToolsConfig()
	cmd, cleanupCmd, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %v", err)
	}
	defer cleanupCmd()

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	tcs := []struct {
		name           string
		body           string
		want           string
		wantStatusCode int
	}{
		{
			name:           "successful creation",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1", "instance": "i1-success", "instanceType": "PRIMARY", "displayName": "i1-success"}`,
			want:           `{"metadata":{"@type":"type.googleapis.com/google.cloud.alloydb.v1.OperationMetadata","target":"projects/p1/locations/l1/clusters/c1/instances/i1-success","verb":"create","requestedCancellation":false,"apiVersion":"v1"},"name":"projects/p1/locations/l1/operations/mock-operation-success"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "api failure",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1", "instance": "i2-api-failure", "instanceType": "PRIMARY", "displayName": "i1-success"}`,
			want:           `{"error":"error processing GCP request: error creating AlloyDB instance: googleapi: Error 500: internal api error"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing project",
			body:           `{"location": "l1", "cluster": "c1", "instance": "i1", "instanceType": "PRIMARY"}`,
			want:           `{"error":"parameter \"project\" is required"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing cluster",
			body:           `{"project": "p1", "location": "l1", "instance": "i1", "instanceType": "PRIMARY"}`,
			want:           `{"error":"parameter \"cluster\" is required"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing location",
			body:           `{"project": "p1", "cluster": "c1", "instance": "i1", "instanceType": "PRIMARY"}`,
			want:           `{"error":"parameter \"location\" is required"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing instance",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1", "instanceType": "PRIMARY"}`,
			want:           `{"error":"parameter \"instance\" is required"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "invalid instanceType",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1", "instance": "i1", "instanceType": "INVALID", "displayName": "invalid"}`,
			want:           `{"error":"invalid 'instanceType' parameter; expected 'PRIMARY' or 'READ_POOL'"}`,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			api := "http://127.0.0.1:5000/api/tool/alloydb-create-instance/invoke"
			req, err := http.NewRequest(http.MethodPost, api, bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			bodyBytes, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("expected status %d but got %d: %s", tc.wantStatusCode, resp.StatusCode, string(bodyBytes))
			}

			if tc.wantStatusCode != http.StatusOK {
				if tc.want != "" && !bytes.Contains(bodyBytes, []byte(tc.want)) {
					t.Fatalf("expected error response to contain %q, but got: %s", tc.want, string(bodyBytes))
				}
				return
			}

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var result struct {
				Result string `json:"result"`
			}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			var got, want map[string]any
			if err := json.Unmarshal([]byte(result.Result), &got); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatalf("failed to unmarshal want: %v", err)
			}

			if !reflect.DeepEqual(want, got) {
				t.Errorf("unexpected result:\n- want: %+v\n-  got: %+v", want, got)
			}
		})
	}
}

func TestAlloyDBCreateUser(t *testing.T) {
	cleanup := setupTestServer(t, "userId")
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := []string{"--enable-api"}
	toolsFile := getAlloyDBToolsConfig()
	cmd, cleanupCmd, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %v", err)
	}
	defer cleanupCmd()

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	tcs := []struct {
		name           string
		body           string
		want           string
		wantStatusCode int
	}{
		{
			name:           "successful creation IAM user",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1", "user": "u1-iam-success", "userType": "ALLOYDB_IAM_USER"}`,
			want:           `{"databaseRoles": ["alloydbiamuser"], "name": "projects/p1/locations/l1/clusters/c1/users/u1-iam-success", "userType": "ALLOYDB_IAM_USER"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "successful creation builtin user",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1", "user": "u2-builtin-success", "userType": "ALLOYDB_BUILT_IN", "password": "pass123", "databaseRoles": ["alloydbsuperuser"]}`,
			want:           `{"databaseRoles": ["alloydbsuperuser"], "name": "projects/p1/locations/l1/clusters/c1/users/u2-builtin-success", "userType": "ALLOYDB_BUILT_IN"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "api failure",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1", "user": "u3-api-failure", "userType": "ALLOYDB_IAM_USER"}`,
			want:           `{"error":"error processing GCP request: error creating AlloyDB user: googleapi: Error 500: user internal api error"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing project",
			body:           `{"location": "l1", "cluster": "c1", "user": "u-fail", "userType": "ALLOYDB_IAM_USER"}`,
			want:           `{"error":"parameter \"project\" is required"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing cluster",
			body:           `{"project": "p1", "location": "l1", "user": "u-fail", "userType": "ALLOYDB_IAM_USER"}`,
			want:           `{"error":"parameter \"cluster\" is required"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing location",
			body:           `{"project": "p1", "cluster": "c1", "user": "u-fail", "userType": "ALLOYDB_IAM_USER"}`,
			want:           `{"error":"parameter \"location\" is required"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing user",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1", "userType": "ALLOYDB_IAM_USER"}`,
			want:           `{"error":"parameter \"user\" is required"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing userType",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1", "user": "u-fail"}`,
			want:           `{"error":"parameter \"userType\" is required"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "missing password for builtin user",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1", "user": "u-fail", "userType": "ALLOYDB_BUILT_IN"}`,
			want:           `{"error":"password is required when userType is ALLOYDB_BUILT_IN"}`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "invalid userType",
			body:           `{"project": "p1", "location": "l1", "cluster": "c1", "user": "u-fail", "userType": "invalid"}`,
			want:           `{"error":"invalid or missing 'userType' parameter; expected 'ALLOYDB_BUILT_IN' or 'ALLOYDB_IAM_USER'"}`,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			api := "http://127.0.0.1:5000/api/tool/alloydb-create-user/invoke"
			req, err := http.NewRequest(http.MethodPost, api, bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			bodyBytes, _ := io.ReadAll(resp.Body)

			if tc.wantStatusCode != http.StatusOK {
				if tc.want != "" && !bytes.Contains(bodyBytes, []byte(tc.want)) {
					t.Fatalf("expected error response to contain %q, but got: %s", tc.want, string(bodyBytes))
				}
				return
			}

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var result struct {
				Result string `json:"result"`
			}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			var got, want map[string]any
			if err := json.Unmarshal([]byte(result.Result), &got); err != nil {
				t.Fatalf("failed to unmarshal result string: %v. Result: %s", err, result.Result)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatalf("failed to unmarshal want string: %v. Want: %s", err, tc.want)
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("unexpected result map (-want +got):\n%s", diff)
			}
		})
	}
}
