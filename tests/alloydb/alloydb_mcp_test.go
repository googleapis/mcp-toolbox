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

package alloydb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	AlloyDBProject  = os.Getenv("ALLOYDB_PROJECT")
	AlloyDBLocation = os.Getenv("ALLOYDB_REGION")
	AlloyDBCluster  = os.Getenv("ALLOYDB_CLUSTER")
	AlloyDBInstance = os.Getenv("ALLOYDB_INSTANCE")
	AlloyDBUser     = os.Getenv("ALLOYDB_POSTGRES_USER")
)

func getAlloyDBVars(t *testing.T) map[string]string {
	if AlloyDBProject == "" {
		t.Fatal("'ALLOYDB_PROJECT' not set")
	}
	if AlloyDBLocation == "" {
		t.Fatal("'ALLOYDB_REGION' not set")
	}
	if AlloyDBCluster == "" {
		t.Fatal("'ALLOYDB_CLUSTER' not set")
	}
	if AlloyDBInstance == "" {
		t.Fatal("'ALLOYDB_INSTANCE' not set")
	}
	if AlloyDBUser == "" {
		t.Fatal("'ALLOYDB_USER' not set")
	}
	return map[string]string{
		"project":  AlloyDBProject,
		"location": AlloyDBLocation,
		"cluster":  AlloyDBCluster,
		"instance": AlloyDBInstance,
		"user":     AlloyDBUser,
	}
}

func getAlloyDBToolsConfig() map[string]any {
	return map[string]any{
		"sources": map[string]any{
			"alloydb-admin-source": map[string]any{
				"type": "alloydb-admin",
			},
		},
		"tools": map[string]any{
			// Tool for RunAlloyDBToolGetTest
			"my-simple-tool": map[string]any{
				"type":        "alloydb-list-clusters",
				"source":      "alloydb-admin-source",
				"description": "Simple tool to test end to end functionality.",
			},
			// Tool for MCP test
			"my-param-tool": map[string]any{
				"type":        "alloydb-list-clusters",
				"source":      "alloydb-admin-source",
				"description": "Tool to list clusters",
			},
			// Tool for MCP test that fails
			"my-fail-tool": map[string]any{
				"type":        "alloydb-list-clusters",
				"source":      "alloydb-admin-source",
				"description": "Tool that will fail",
			},
			// AlloyDB specific tools
			"alloydb-list-clusters": map[string]any{
				"type":        "alloydb-list-clusters",
				"source":      "alloydb-admin-source",
				"description": "Lists all AlloyDB clusters in a given project and location.",
			},
			"alloydb-list-users": map[string]any{
				"type":        "alloydb-list-users",
				"source":      "alloydb-admin-source",
				"description": "Lists all AlloyDB users within a specific cluster.",
			},
			"alloydb-list-instances": map[string]any{
				"type":        "alloydb-list-instances",
				"source":      "alloydb-admin-source",
				"description": "Lists all AlloyDB instances within a specific cluster.",
			},
			"alloydb-get-cluster": map[string]any{
				"type":        "alloydb-get-cluster",
				"source":      "alloydb-admin-source",
				"description": "Retrieves details of a specific AlloyDB cluster.",
			},
			"alloydb-get-instance": map[string]any{
				"type":        "alloydb-get-instance",
				"source":      "alloydb-admin-source",
				"description": "Retrieves details of a specific AlloyDB instance.",
			},
			"alloydb-get-user": map[string]any{
				"type":        "alloydb-get-user",
				"source":      "alloydb-admin-source",
				"description": "Retrieves details of a specific AlloyDB user.",
			},
			"alloydb-create-cluster": map[string]any{
				"type":        "alloydb-create-cluster",
				"description": "create cluster",
				"source":      "alloydb-admin-source",
			},
			"alloydb-create-instance": map[string]any{
				"type":        "alloydb-create-instance",
				"description": "create instance",
				"source":      "alloydb-admin-source",
			},
			"alloydb-create-user": map[string]any{
				"type":        "alloydb-create-user",
				"description": "create user",
				"source":      "alloydb-admin-source",
			},
		},
	}
}

func TestAlloyDBListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	toolsFile := map[string]any{
		"sources": map[string]any{
			"alloydb-admin-source": map[string]any{
				"type": "alloydb-admin",
			},
		},
		"tools": map[string]any{
			"alloydb-list-clusters": map[string]any{
				"type":        "alloydb-list-clusters",
				"source":      "alloydb-admin-source",
				"description": "Lists all AlloyDB clusters in a given project and location.",
			},
			"alloydb-list-users": map[string]any{
				"type":        "alloydb-list-users",
				"source":      "alloydb-admin-source",
				"description": "Lists all AlloyDB users within a specific cluster.",
			},
		},
	}

	// Start the toolbox server
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
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

	// Verify list of tools
	expectedTools := []tests.MCPToolManifest{
		{
			Name:        "alloydb-list-clusters",
			Description: "Lists all AlloyDB clusters in a given project and location.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project": map[string]any{
						"description": "The GCP project ID to list clusters for.",
						"type":        "string",
					},
					"location": map[string]any{
						"default":     "-",
						"description": "Optional: The location to list clusters in (e.g., 'us-central1'). Use '-' to list clusters across all locations.(Default: '-')",
						"type":        "string",
					},
				},
				"required": []any{"project"},
			},
		},
		{
			Name:        "alloydb-list-users",
			Description: "Lists all AlloyDB users within a specific cluster.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cluster": map[string]any{
						"description": "The ID of the cluster to list users from.",
						"type":        "string",
					},
					"location": map[string]any{
						"description": "The location of the cluster (e.g., 'us-central1').",
						"type":        "string",
					},
					"project": map[string]any{
						"description": "The GCP project ID.",
						"type":        "string",
					},
				},
				"required": []any{"project", "location", "cluster"},
			},
		},
	}

	tests.RunMCPToolsListMethod(t, expectedTools)
}

func TestAlloyDBCallTool(t *testing.T) {
	vars := getAlloyDBVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	toolsFile := getAlloyDBToolsConfig()

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
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

	// Run tool-specific invoke tests
	runAlloyDBListClustersMCPTest(t, vars)
	runAlloyDBListInstancesMCPTest(t, vars)
	runAlloyDBListUsersMCPTest(t, vars)
	runAlloyDBGetClusterMCPTest(t, vars)
	runAlloyDBGetInstanceMCPTest(t, vars)
	runAlloyDBGetUserMCPTest(t, vars)

	t.Run("MCP Invoke invalid tool", func(t *testing.T) {
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, "non-existent-tool", map[string]any{}, nil)
		if err != nil {
			t.Fatalf("native error executing %s: %s", "non-existent-tool", err)
		}
		if statusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", statusCode)
		}
		tests.AssertMCPError(t, mcpResp, `tool with name "non-existent-tool" does not exist`)
	})
}

func runAlloyDBListClustersMCPTest(t *testing.T, vars map[string]string) {
	type ListClustersResponse struct {
		Clusters []struct {
			Name string `json:"name"`
		} `json:"clusters"`
	}

	wantForSpecificLocation := []string{
		fmt.Sprintf("projects/%s/locations/us-central1/clusters/alloydb-ai-nl-testing", vars["project"]),
		fmt.Sprintf("projects/%s/locations/us-central1/clusters/alloydb-pg-testing", vars["project"]),
	}

	wantForAllLocations := []string{
		fmt.Sprintf("projects/%s/locations/us-central1/clusters/alloydb-ai-nl-testing", vars["project"]),
		fmt.Sprintf("projects/%s/locations/us-central1/clusters/alloydb-pg-testing", vars["project"]),
		fmt.Sprintf("projects/%s/locations/us-east4/clusters/alloydb-private-pg-testing", vars["project"]),
		fmt.Sprintf("projects/%s/locations/us-east4/clusters/colab-testing", vars["project"]),
	}

	invokeTcs := []struct {
		name           string
		args           map[string]any
		want           []string
		wantContentErr string
		expectError    bool
		wantStatusCode int
	}{
		{
			name:           "list clusters for all locations",
			args:           map[string]any{"project": vars["project"], "location": "-"},
			want:           wantForAllLocations,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list clusters specific location",
			args:           map[string]any{"project": vars["project"], "location": "us-central1"},
			want:           wantForSpecificLocation,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list clusters missing project",
			args:           map[string]any{"location": vars["location"]},
			expectError:    true,
			wantContentErr: `parameter "project" is required`,
			wantStatusCode: http.StatusOK, // Caught by schema validation, returns JSON-RPC error wrapped in 200
		},
		{
			name:           "list clusters non-existent location",
			args:           map[string]any{"project": vars["project"], "location": "abcd"},
			expectError:    true,
			wantStatusCode: http.StatusInternalServerError, // GCP error maps to 500
		},
		{
			name:           "list clusters non-existent project",
			args:           map[string]any{"project": "non-existent-project", "location": vars["location"]},
			expectError:    true,
			wantStatusCode: http.StatusInternalServerError, // GCP error maps to 500
		},
		{
			name:           "list clusters empty project",
			args:           map[string]any{"project": "", "location": vars["location"]},
			expectError:    true,
			wantStatusCode: http.StatusOK, // Caught by tool validation, returns tool error in 200
		},
		{
			name:           "list clusters empty location",
			args:           map[string]any{"project": vars["project"], "location": ""},
			expectError:    true,
			wantStatusCode: http.StatusOK, // Caught by tool validation (or GCP 400 mapped to Agent), returns tool error in 200
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, "alloydb-list-clusters", tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing: %s", err)
			}

			if statusCode != tc.wantStatusCode {
				t.Fatalf("expected status %d, got %d", tc.wantStatusCode, statusCode)
			}

			if tc.expectError {
				tests.AssertMCPError(t, mcpResp, tc.wantContentErr)
			} else {
				if mcpResp.Result.IsError {
					t.Fatalf("returned error result: %v", mcpResp.Result)
				}
				gotStr := mcpResp.Result.Content[0].Text
				var clustersData ListClustersResponse
				if err := json.Unmarshal([]byte(gotStr), &clustersData); err != nil {
					t.Fatalf("error parsing result JSON: %v", err)
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

func runAlloyDBListInstancesMCPTest(t *testing.T, vars map[string]string) {
	type ListInstancesResponse struct {
		Instances []struct {
			Name string `json:"name"`
		} `json:"instances"`
	}

	wantForSpecificClusterAndLocation := []string{
		fmt.Sprintf("projects/%s/locations/%s/clusters/%s/instances/%s", vars["project"], vars["location"], vars["cluster"], vars["instance"]),
	}

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
		args           map[string]any
		want           []string
		wantContentErr string
		expectError    bool
		wantStatusCode int
	}{
		{
			name:           "list instances for a specific cluster and location",
			args:           map[string]any{"project": vars["project"], "location": vars["location"], "cluster": vars["cluster"]},
			want:           wantForSpecificClusterAndLocation,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list instances for all clusters and specific location",
			args:           map[string]any{"project": vars["project"], "location": vars["location"], "cluster": "-"},
			want:           wantForAllClustersSpecificLocation,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list instances for all clusters and all locations",
			args:           map[string]any{"project": vars["project"], "location": "-", "cluster": "-"},
			want:           wantForAllClustersAllLocations,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list instances missing project",
			args:           map[string]any{"location": vars["location"], "cluster": vars["cluster"]},
			expectError:    true,
			wantContentErr: `parameter "project" is required`,
			wantStatusCode: http.StatusOK, // Caught by schema validation
		},
		{
			name:           "list instances non-existent project",
			args:           map[string]any{"project": "non-existent-project", "location": vars["location"], "cluster": vars["cluster"]},
			expectError:    true,
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name:           "list instances non-existent location",
			args:           map[string]any{"project": vars["project"], "location": "non-existent-location", "cluster": vars["cluster"]},
			expectError:    true,
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name:           "list instances non-existent cluster",
			args:           map[string]any{"project": vars["project"], "location": vars["location"], "cluster": "non-existent-cluster"},
			expectError:    true,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, "alloydb-list-instances", tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing: %s", err)
			}

			if statusCode != tc.wantStatusCode {
				t.Fatalf("expected status %d, got %d", tc.wantStatusCode, statusCode)
			}

			if tc.expectError {
				tests.AssertMCPError(t, mcpResp, tc.wantContentErr)
			} else {
				if mcpResp.Result.IsError {
					t.Fatalf("returned error result: %v", mcpResp.Result)
				}
				gotStr := mcpResp.Result.Content[0].Text
				var instancesData ListInstancesResponse
				if err := json.Unmarshal([]byte(gotStr), &instancesData); err != nil {
					t.Fatalf("error parsing result JSON: %v", err)
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

func runAlloyDBListUsersMCPTest(t *testing.T, vars map[string]string) {
	type UsersResponse struct {
		Users []struct {
			Name string `json:"name"`
		} `json:"users"`
	}

	invokeTcs := []struct {
		name           string
		args           map[string]any
		want           string
		expectError    bool
		wantStatusCode int
	}{
		{
			name:           "list users success",
			args:           map[string]any{"project": vars["project"], "location": vars["location"], "cluster": vars["cluster"]},
			want:           fmt.Sprintf("projects/%s/locations/%s/clusters/%s/users/%s", vars["project"], vars["location"], vars["cluster"], vars["user"]),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list users missing project",
			args:           map[string]any{"location": vars["location"], "cluster": vars["cluster"]},
			expectError:    true,
			want:           `parameter "project" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list users missing location",
			args:           map[string]any{"project": vars["project"], "cluster": vars["cluster"]},
			expectError:    true,
			want:           `parameter "location" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list users missing cluster",
			args:           map[string]any{"project": vars["project"], "location": vars["location"]},
			expectError:    true,
			want:           `parameter "cluster" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list users non-existent cluster",
			args:           map[string]any{"project": vars["project"], "location": vars["location"], "cluster": "non-existent-cluster"},
			expectError:    true,
			want:           `was not found`,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, "alloydb-list-users", tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing: %s", err)
			}

			if statusCode != tc.wantStatusCode {
				t.Fatalf("expected status %d, got %d", tc.wantStatusCode, statusCode)
			}

			if tc.expectError {
				tests.AssertMCPError(t, mcpResp, tc.want)
			} else {
				if mcpResp.Result.IsError {
					t.Fatalf("returned error result: %v", mcpResp.Result)
				}
				gotStr := mcpResp.Result.Content[0].Text
				var usersData UsersResponse
				if err := json.Unmarshal([]byte(gotStr), &usersData); err != nil {
					t.Fatalf("error parsing result JSON: %v. Result was: %s", err, gotStr)
				}

				found := false
				for _, user := range usersData.Users {
					if user.Name == tc.want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected user name %q not found in response", tc.want)
				}
			}
		})
	}
}

func runAlloyDBGetClusterMCPTest(t *testing.T, vars map[string]string) {
	invokeTcs := []struct {
		name           string
		args           map[string]any
		want           map[string]any
		wantContentErr string
		expectError    bool
		wantStatusCode int
	}{
		{
			name: "get cluster success",
			args: map[string]any{"project": vars["project"], "location": vars["location"], "cluster": vars["cluster"]},
			want: map[string]any{
				"clusterType": "PRIMARY",
				"name":        fmt.Sprintf("projects/%s/locations/%s/clusters/%s", vars["project"], vars["location"], vars["cluster"]),
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get cluster missing project",
			args:           map[string]any{"location": vars["location"], "cluster": vars["cluster"]},
			expectError:    true,
			wantContentErr: `parameter "project" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get cluster missing location",
			args:           map[string]any{"project": vars["project"], "cluster": vars["cluster"]},
			expectError:    true,
			wantContentErr: `parameter "location" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get cluster missing cluster",
			args:           map[string]any{"project": vars["project"], "location": vars["location"]},
			expectError:    true,
			wantContentErr: `parameter "cluster" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get cluster non-existent cluster",
			args:           map[string]any{"project": vars["project"], "location": vars["location"], "cluster": "non-existent-cluster"},
			expectError:    true,
			wantContentErr: `was not found`,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, "alloydb-get-cluster", tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing: %s", err)
			}

			if statusCode != tc.wantStatusCode {
				t.Fatalf("expected status %d, got %d", tc.wantStatusCode, statusCode)
			}

			if tc.expectError {
				tests.AssertMCPError(t, mcpResp, tc.wantContentErr)
			} else {
				if mcpResp.Result.IsError {
					t.Fatalf("returned error result: %v", mcpResp.Result)
				}
				gotStr := mcpResp.Result.Content[0].Text
				var gotMap map[string]any
				if err := json.Unmarshal([]byte(gotStr), &gotMap); err != nil {
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
		})
	}
}

func runAlloyDBGetInstanceMCPTest(t *testing.T, vars map[string]string) {
	invokeTcs := []struct {
		name           string
		args           map[string]any
		want           map[string]any
		wantContentErr string
		expectError    bool
		wantStatusCode int
	}{
		{
			name: "get instance success",
			args: map[string]any{"project": vars["project"], "location": vars["location"], "cluster": vars["cluster"], "instance": vars["instance"]},
			want: map[string]any{
				"instanceType": "PRIMARY",
				"name":         fmt.Sprintf("projects/%s/locations/%s/clusters/%s/instances/%s", vars["project"], vars["location"], vars["cluster"], vars["instance"]),
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get instance missing project",
			args:           map[string]any{"location": vars["location"], "cluster": vars["cluster"], "instance": vars["instance"]},
			expectError:    true,
			wantContentErr: `parameter "project" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get instance missing location",
			args:           map[string]any{"project": vars["project"], "cluster": vars["cluster"], "instance": vars["instance"]},
			expectError:    true,
			wantContentErr: `parameter "location" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get instance missing cluster",
			args:           map[string]any{"project": vars["project"], "location": vars["location"], "instance": vars["instance"]},
			expectError:    true,
			wantContentErr: `parameter "cluster" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get instance missing instance",
			args:           map[string]any{"project": vars["project"], "location": vars["location"], "cluster": vars["cluster"]},
			expectError:    true,
			wantContentErr: `parameter "instance" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get instance non-existent instance",
			args:           map[string]any{"project": vars["project"], "location": vars["location"], "cluster": vars["cluster"], "instance": "non-existent-instance"},
			expectError:    true,
			wantContentErr: `was not found`,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, "alloydb-get-instance", tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing: %s", err)
			}

			if statusCode != tc.wantStatusCode {
				t.Fatalf("expected status %d, got %d", tc.wantStatusCode, statusCode)
			}

			if tc.expectError {
				tests.AssertMCPError(t, mcpResp, tc.wantContentErr)
			} else {
				if mcpResp.Result.IsError {
					t.Fatalf("returned error result: %v", mcpResp.Result)
				}
				gotStr := mcpResp.Result.Content[0].Text
				var gotMap map[string]any
				if err := json.Unmarshal([]byte(gotStr), &gotMap); err != nil {
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
		})
	}
}

func runAlloyDBGetUserMCPTest(t *testing.T, vars map[string]string) {
	invokeTcs := []struct {
		name           string
		args           map[string]any
		want           map[string]any
		wantContentErr string
		expectError    bool
		wantStatusCode int
	}{
		{
			name: "get user success",
			args: map[string]any{"project": vars["project"], "location": vars["location"], "cluster": vars["cluster"], "user": vars["user"]},
			want: map[string]any{
				"name":     fmt.Sprintf("projects/%s/locations/%s/clusters/%s/users/%s", vars["project"], vars["location"], vars["cluster"], vars["user"]),
				"userType": "ALLOYDB_BUILT_IN",
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get user missing project",
			args:           map[string]any{"location": vars["location"], "cluster": vars["cluster"], "user": vars["user"]},
			expectError:    true,
			wantContentErr: `parameter "project" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get user missing location",
			args:           map[string]any{"project": vars["project"], "cluster": vars["cluster"], "user": vars["user"]},
			expectError:    true,
			wantContentErr: `parameter "location" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get user missing cluster",
			args:           map[string]any{"project": vars["project"], "location": vars["location"], "user": vars["user"]},
			expectError:    true,
			wantContentErr: `parameter "cluster" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get user missing user",
			args:           map[string]any{"project": vars["project"], "location": vars["location"], "cluster": vars["cluster"]},
			expectError:    true,
			wantContentErr: `parameter "user" is required`,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "get non-existent user",
			args:           map[string]any{"project": vars["project"], "location": vars["location"], "cluster": vars["cluster"], "user": "non-existent-user"},
			expectError:    true,
			wantContentErr: `does not exist`,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, "alloydb-get-user", tc.args, nil)
			if err != nil {
				t.Fatalf("native error executing: %s", err)
			}

			if statusCode != tc.wantStatusCode {
				t.Fatalf("expected status %d, got %d", tc.wantStatusCode, statusCode)
			}

			if tc.expectError {
				tests.AssertMCPError(t, mcpResp, tc.wantContentErr)
			} else {
				if mcpResp.Result.IsError {
					t.Fatalf("returned error result: %v", mcpResp.Result)
				}
				gotStr := mcpResp.Result.Content[0].Text
				var gotMap map[string]any
				if err := json.Unmarshal([]byte(gotStr), &gotMap); err != nil {
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
		})
	}
}

type mockAlloyDBTransportMCP struct {
	transport http.RoundTripper
	url       *url.URL
}

func (t *mockAlloyDBTransportMCP) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), "https://alloydb.googleapis.com") {
		req.URL.Scheme = t.url.Scheme
		req.URL.Host = t.url.Host
	}
	return t.transport.RoundTrip(req)
}

type mockAlloyDBHandlerMCP struct {
	t       *testing.T
	idParam string
}

func (h *mockAlloyDBHandlerMCP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

func setupTestServerMCP(t *testing.T, idParam string) func() {
	handler := &mockAlloyDBHandlerMCP{t: t, idParam: idParam}
	server := httptest.NewServer(handler)

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}

	originalTransport := http.DefaultClient.Transport
	if originalTransport == nil {
		originalTransport = http.DefaultTransport
	}
	http.DefaultClient.Transport = &mockAlloyDBTransportMCP{
		transport: originalTransport,
		url:       serverURL,
	}

	return func() {
		server.Close()
		http.DefaultClient.Transport = originalTransport
	}
}

func TestAlloyDBCreateClusterMCP(t *testing.T) {
	cleanup := setupTestServerMCP(t, "clusterId")
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	toolsFile := getAlloyDBToolsConfig()
	cmd, cleanupCmd, err := tests.StartCmd(ctx, toolsFile)
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
		name string
		body string
		want string
	}{
		{
			name: "successful creation",
			body: `{"project": "p1", "location": "l1", "cluster": "c1-success", "password": "p1"}`,
			want: `{"name":"projects/p1/locations/l1/operations/mock-operation-success", "metadata": {"verb": "create", "target": "projects/p1/locations/l1/clusters/c1-success"}}`,
		},
		{
			name: "api failure",
			body: `{"project": "p1", "location": "l1", "cluster": "c2-api-failure", "password": "p1"}`,
			want: `{"error":"error processing GCP request: error creating AlloyDB cluster: googleapi: Error 500: internal api error"}`,
		},
		{
			name: "missing project",
			body: `{"location": "l1", "cluster": "c1", "password": "p1"}`,
			want: `{"error":"parameter \"project\" is required"}`,
		},
		{
			name: "missing cluster",
			body: `{"project": "p1", "location": "l1", "password": "p1"}`,
			want: `{"error":"parameter \"cluster\" is required"}`,
		},
		{
			name: "missing password",
			body: `{"project": "p1", "location": "l1", "cluster": "c1"}`,
			want: `{"error":"parameter \"password\" is required"}`,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			var args map[string]any
			if err := json.Unmarshal([]byte(tc.body), &args); err != nil {
				t.Fatalf("failed to unmarshal body: %v", err)
			}

			statusCode, mcpResp, err := tests.InvokeMCPTool(t, "alloydb-create-cluster", args, nil)
			if err != nil {
				t.Fatalf("native error executing %s: %s", "alloydb-create-cluster", err)
			}

			if statusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", statusCode)
			}

			if tc.name == "successful creation" {
				if mcpResp.Result.IsError {
					t.Fatalf("expected success, got error result: %v", mcpResp.Result)
				}
				gotStr := mcpResp.Result.Content[0].Text
				var got, want map[string]any
				if err := json.Unmarshal([]byte(gotStr), &got); err != nil {
					t.Fatalf("failed to unmarshal result: %v", err)
				}
				if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
					t.Fatalf("failed to unmarshal want: %v", err)
				}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("unexpected result (-want +got):\n%s", diff)
				}
			} else {
				var wantMap map[string]string
				if err := json.Unmarshal([]byte(tc.want), &wantMap); err != nil {
					t.Fatalf("failed to unmarshal want: %v", err)
				}
				tests.AssertMCPError(t, mcpResp, wantMap["error"])
			}
		})
	}
}

func TestAlloyDBCreateInstanceMCP(t *testing.T) {
	cleanup := setupTestServerMCP(t, "instanceId")
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	toolsFile := getAlloyDBToolsConfig()
	cmd, cleanupCmd, err := tests.StartCmd(ctx, toolsFile)
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
		name string
		body string
		want string
	}{
		{
			name: "successful creation",
			body: `{"project": "p1", "location": "l1", "cluster": "c1", "instance": "i1-success", "instanceType": "PRIMARY", "displayName": "i1-success"}`,
			want: `{"metadata":{"@type":"type.googleapis.com/google.cloud.alloydb.v1.OperationMetadata","target":"projects/p1/locations/l1/clusters/c1/instances/i1-success","verb":"create","requestedCancellation":false,"apiVersion":"v1"},"name":"projects/p1/locations/l1/operations/mock-operation-success"}`,
		},
		{
			name: "api failure",
			body: `{"project": "p1", "location": "l1", "cluster": "c1", "instance": "i2-api-failure", "instanceType": "PRIMARY", "displayName": "i1-success"}`,
			want: `{"error":"error processing GCP request: error creating AlloyDB instance: googleapi: Error 500: internal api error"}`,
		},
		{
			name: "missing project",
			body: `{"location": "l1", "cluster": "c1", "instance": "i1", "instanceType": "PRIMARY"}`,
			want: `{"error":"parameter \"project\" is required"}`,
		},
		{
			name: "missing cluster",
			body: `{"project": "p1", "location": "l1", "instance": "i1", "instanceType": "PRIMARY"}`,
			want: `{"error":"parameter \"cluster\" is required"}`,
		},
		{
			name: "missing location",
			body: `{"project": "p1", "cluster": "c1", "instance": "i1", "instanceType": "PRIMARY"}`,
			want: `{"error":"parameter \"location\" is required"}`,
		},
		{
			name: "missing instance",
			body: `{"project": "p1", "location": "l1", "cluster": "c1", "instanceType": "PRIMARY"}`,
			want: `{"error":"parameter \"instance\" is required"}`,
		},
		{
			name: "invalid instanceType",
			body: `{"project": "p1", "location": "l1", "cluster": "c1", "instance": "i1", "instanceType": "INVALID", "displayName": "invalid"}`,
			want: `{"error":"invalid 'instanceType' parameter; expected 'PRIMARY' or 'READ_POOL'"}`,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			var args map[string]any
			if err := json.Unmarshal([]byte(tc.body), &args); err != nil {
				t.Fatalf("failed to unmarshal body: %v", err)
			}

			statusCode, mcpResp, err := tests.InvokeMCPTool(t, "alloydb-create-instance", args, nil)
			if err != nil {
				t.Fatalf("native error executing %s: %s", "alloydb-create-instance", err)
			}

			if statusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", statusCode)
			}

			if tc.name == "successful creation" {
				if mcpResp.Result.IsError {
					t.Fatalf("expected success, got error result: %v", mcpResp.Result)
				}
				gotStr := mcpResp.Result.Content[0].Text
				var got, want map[string]any
				if err := json.Unmarshal([]byte(gotStr), &got); err != nil {
					t.Fatalf("failed to unmarshal result: %v", err)
				}
				if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
					t.Fatalf("failed to unmarshal want: %v", err)
				}
				if !reflect.DeepEqual(want, got) {
					t.Errorf("unexpected result:\n- want: %+v\n-  got: %+v", want, got)
				}
			} else {
				var wantMap map[string]string
				if err := json.Unmarshal([]byte(tc.want), &wantMap); err != nil {
					t.Fatalf("failed to unmarshal want: %v", err)
				}
				tests.AssertMCPError(t, mcpResp, wantMap["error"])
			}
		})
	}
}

func TestAlloyDBCreateUserMCP(t *testing.T) {
	cleanup := setupTestServerMCP(t, "userId")
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	toolsFile := getAlloyDBToolsConfig()
	cmd, cleanupCmd, err := tests.StartCmd(ctx, toolsFile)
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
		name string
		body string
		want string
	}{
		{
			name: "successful creation IAM user",
			body: `{"project": "p1", "location": "l1", "cluster": "c1", "user": "u1-iam-success", "userType": "ALLOYDB_IAM_USER"}`,
			want: `{"databaseRoles": ["alloydbiamuser"], "name": "projects/p1/locations/l1/clusters/c1/users/u1-iam-success", "userType": "ALLOYDB_IAM_USER"}`,
		},
		{
			name: "successful creation builtin user",
			body: `{"project": "p1", "location": "l1", "cluster": "c1", "user": "u2-builtin-success", "userType": "ALLOYDB_BUILT_IN", "password": "pass123", "databaseRoles": ["alloydbsuperuser"]}`,
			want: `{"databaseRoles": ["alloydbsuperuser"], "name": "projects/p1/locations/l1/clusters/c1/users/u2-builtin-success", "userType": "ALLOYDB_BUILT_IN"}`,
		},
		{
			name: "api failure",
			body: `{"project": "p1", "location": "l1", "cluster": "c1", "user": "u3-api-failure", "userType": "ALLOYDB_IAM_USER"}`,
			want: `{"error":"error processing GCP request: error creating AlloyDB user: googleapi: Error 500: user internal api error"}`,
		},
		{
			name: "missing project",
			body: `{"location": "l1", "cluster": "c1", "user": "u-fail", "userType": "ALLOYDB_IAM_USER"}`,
			want: `{"error":"parameter \"project\" is required"}`,
		},
		{
			name: "missing cluster",
			body: `{"project": "p1", "location": "l1", "user": "u-fail", "userType": "ALLOYDB_IAM_USER"}`,
			want: `{"error":"parameter \"cluster\" is required"}`,
		},
		{
			name: "missing location",
			body: `{"project": "p1", "cluster": "c1", "user": "u-fail", "userType": "ALLOYDB_IAM_USER"}`,
			want: `{"error":"parameter \"location\" is required"}`,
		},
		{
			name: "missing user",
			body: `{"project": "p1", "location": "l1", "cluster": "c1", "userType": "ALLOYDB_IAM_USER"}`,
			want: `{"error":"parameter \"user\" is required"}`,
		},
		{
			name: "missing userType",
			body: `{"project": "p1", "location": "l1", "cluster": "c1", "user": "u-fail"}`,
			want: `{"error":"parameter \"userType\" is required"}`,
		},
		{
			name: "missing password for builtin user",
			body: `{"project": "p1", "location": "l1", "cluster": "c1", "user": "u-fail", "userType": "ALLOYDB_BUILT_IN"}`,
			want: `{"error":"password is required when userType is ALLOYDB_BUILT_IN"}`,
		},
		{
			name: "invalid userType",
			body: `{"project": "p1", "location": "l1", "cluster": "c1", "user": "u-fail", "userType": "invalid"}`,
			want: `{"error":"invalid or missing 'userType' parameter; expected 'ALLOYDB_BUILT_IN' or 'ALLOYDB_IAM_USER'"}`,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			var args map[string]any
			if err := json.Unmarshal([]byte(tc.body), &args); err != nil {
				t.Fatalf("failed to unmarshal body: %v", err)
			}

			statusCode, mcpResp, err := tests.InvokeMCPTool(t, "alloydb-create-user", args, nil)
			if err != nil {
				t.Fatalf("native error executing %s: %s", "alloydb-create-user", err)
			}

			if statusCode != http.StatusOK {
				t.Fatalf("expected status 200, got %d", statusCode)
			}

			if tc.name == "successful creation IAM user" || tc.name == "successful creation builtin user" {
				if mcpResp.Result.IsError {
					t.Fatalf("expected success, got error result: %v", mcpResp.Result)
				}
				gotStr := mcpResp.Result.Content[0].Text
				var got, want map[string]any
				if err := json.Unmarshal([]byte(gotStr), &got); err != nil {
					t.Fatalf("failed to unmarshal result string: %v. Result: %s", err, gotStr)
				}
				if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
					t.Fatalf("failed to unmarshal want string: %v. Want: %s", err, tc.want)
				}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("unexpected result map (-want +got):\n%s", diff)
				}
			} else {
				var wantMap map[string]string
				if err := json.Unmarshal([]byte(tc.want), &wantMap); err != nil {
					t.Fatalf("failed to unmarshal want: %v", err)
				}
				tests.AssertMCPError(t, mcpResp, wantMap["error"])
			}
		})
	}
}
