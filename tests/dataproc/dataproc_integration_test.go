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

package dataproc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	dataproc "cloud.google.com/go/dataproc/v2/apiv1"
	"cloud.google.com/go/dataproc/v2/apiv1/dataprocpb"
	"github.com/google/go-cmp/cmp"
	dataprocsrc "github.com/googleapis/mcp-toolbox/internal/sources/dataproc"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	dataprocRegion  = os.Getenv("DATAPROC_REGION")
	dataprocProject = os.Getenv("DATAPROC_PROJECT")

	// dataprocListJobsCluster is the name of a cluster in the project that has jobs.
	//
	// This is necessary to work around a performance issue in the Dataproc API where listing all
	// jobs in a project is very slow.
	dataprocListJobsCluster = os.Getenv("DATAPROC_LIST_JOBS_CLUSTER")
)

const (
	clusterURLPrefix = "https://console.cloud.google.com/dataproc/clusters/"
	jobURLPrefix     = "https://console.cloud.google.com/dataproc/jobs/"
	logsURLPrefix    = "https://console.cloud.google.com/logs/viewer?"
)

func getDataprocVars(t *testing.T) map[string]any {
	switch "" {
	case dataprocRegion:
		t.Fatal("'DATAPROC_REGION' not set")
	case dataprocProject:
		t.Fatal("'DATAPROC_PROJECT' not set")
	case dataprocListJobsCluster:
		t.Fatal("'DATAPROC_LIST_JOBS_CLUSTER' not set")
	}

	return map[string]any{
		"type":    "dataproc",
		"project": dataprocProject,
		"region":  dataprocRegion,
	}
}

// getDataprocToolsConfig returns the tools file shared by the Dataproc
// integration tests.
func getDataprocToolsConfig(sourceConfig map[string]any) map[string]any {
	return map[string]any{
		"sources": map[string]any{
			"my-dataproc": sourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"type":     "google",
				"clientId": tests.ClientId,
			},
		},
		"tools": map[string]any{
			"get-cluster": map[string]any{
				"type":   "dataproc-get-cluster",
				"source": "my-dataproc",
			},
			"get-cluster-with-auth": map[string]any{
				"type":         "dataproc-get-cluster",
				"source":       "my-dataproc",
				"authRequired": []string{"my-google-auth"},
			},
			"get-job": map[string]any{
				"type":   "dataproc-get-job",
				"source": "my-dataproc",
			},
			"get-job-with-auth": map[string]any{
				"type":         "dataproc-get-job",
				"source":       "my-dataproc",
				"authRequired": []string{"my-google-auth"},
			},
			"list-clusters": map[string]any{
				"type":   "dataproc-list-clusters",
				"source": "my-dataproc",
			},
			"list-clusters-with-auth": map[string]any{
				"type":         "dataproc-list-clusters",
				"source":       "my-dataproc",
				"authRequired": []string{"my-google-auth"},
			},
			"list-jobs": map[string]any{
				"type":   "dataproc-list-jobs",
				"source": "my-dataproc",
			},
			"list-jobs-with-auth": map[string]any{
				"type":         "dataproc-list-jobs",
				"source":       "my-dataproc",
				"authRequired": []string{"my-google-auth"},
			},
		},
	}
}

// setupDataprocTest returns the tools file along with Dataproc cluster and job
// clients used to compute expected results. The clients are closed on cleanup.
func setupDataprocTest(t *testing.T, ctx context.Context) (map[string]any, *dataproc.ClusterControllerClient, *dataproc.JobControllerClient) {
	t.Helper()
	sourceConfig := getDataprocVars(t)

	endpoint := fmt.Sprintf("%s-dataproc.googleapis.com:443", dataprocRegion)
	clusterClient, err := dataproc.NewClusterControllerClient(ctx, option.WithEndpoint(endpoint))
	if err != nil {
		t.Fatalf("failed to create dataproc client: %v", err)
	}
	t.Cleanup(func() {
		if err := clusterClient.Close(); err != nil {
			t.Errorf("failed to close dataproc cluster client: %v", err)
		}
	})

	jobClient, err := dataproc.NewJobControllerClient(ctx, option.WithEndpoint(endpoint))
	if err != nil {
		t.Fatalf("failed to create dataproc client: %v", err)
	}
	t.Cleanup(func() {
		if err := jobClient.Close(); err != nil {
			t.Errorf("failed to close dataproc job client: %v", err)
		}
	})

	return getDataprocToolsConfig(sourceConfig), clusterClient, jobClient
}

func TestDataprocClustersToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute) // Clusters take time
	defer cancel()

	toolsFile, clusterClient, jobClient := setupDataprocTest(t, ctx)

	tr := dataprocTransport{}
	tr.startServer(t, ctx, toolsFile)

	runDataprocTests(t, ctx, tr, clusterClient, jobClient)
}

// runDataprocTests runs the cluster and job tool checks.
func runDataprocTests(t *testing.T, ctx context.Context, tr dataprocTransport, clusterClient *dataproc.ClusterControllerClient, jobClient *dataproc.JobControllerClient) {
	t.Run("get-cluster", func(t *testing.T) {
		clusterName := listClustersRpc(t, clusterClient, ctx, "", 1)[0].Name
		t.Run("success", func(t *testing.T) {
			t.Parallel()
			runGetClusterTest(t, ctx, tr, clusterClient, clusterName)
		})
		t.Run("errors", func(t *testing.T) {
			t.Parallel()
			missingClusterFullName := fmt.Sprintf("projects/%s/regions/%s/clusters/INVALID_CLUSTER", dataprocProject, dataprocRegion)
			tcs := []struct {
				name     string
				toolName string
				request  map[string]any
				wantCode int
				wantMsg  string
			}{
				{
					name:     "missing cluster",
					toolName: "get-cluster",
					request:  map[string]any{"clusterName": "INVALID_CLUSTER"},
					wantCode: http.StatusOK,
					wantMsg:  fmt.Sprintf("Not found: Cluster projects/%s/regions/%s/clusters/INVALID_CLUSTER", dataprocProject, dataprocRegion),
				},
				{
					name:     "full cluster name",
					toolName: "get-cluster",
					request:  map[string]any{"clusterName": missingClusterFullName},
					wantCode: http.StatusOK,
					wantMsg:  fmt.Sprintf("clusterName must be a short name without '/': %s", missingClusterFullName),
				},
			}
			for _, tc := range tcs {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					testError(t, ctx, tr, tc.toolName, tc.request, tc.wantCode, tc.wantMsg)
				})
			}
		})
		t.Run("auth", func(t *testing.T) {
			t.Parallel()
			runAuthTest(t, ctx, tr, "get-cluster-with-auth", map[string]any{"clusterName": shortName(clusterName)}, http.StatusOK)
		})
	})

	t.Run("get-job", func(t *testing.T) {
		jobId := listJobsRpc(t, jobClient, ctx, "", 1)[0].ID
		t.Run("success", func(t *testing.T) {
			t.Parallel()
			runGetJobTest(t, ctx, tr, jobClient, jobId)
		})
		t.Run("errors", func(t *testing.T) {
			t.Parallel()
			missingJobFullName := fmt.Sprintf("projects/%s/regions/%s/jobs/INVALID_JOB", dataprocProject, dataprocRegion)
			tcs := []struct {
				name     string
				toolName string
				request  map[string]any
				wantCode int
				wantMsg  string
			}{
				{
					name:     "missing job",
					toolName: "get-job",
					request:  map[string]any{"jobId": "INVALID_JOB"},
					wantCode: http.StatusOK,
					wantMsg:  fmt.Sprintf("Not found: Job projects/%s/regions/%s/jobs/INVALID_JOB", dataprocProject, dataprocRegion),
				},
				{
					name:     "full job name",
					toolName: "get-job",
					request:  map[string]any{"jobId": missingJobFullName},
					wantCode: http.StatusOK,
					wantMsg:  fmt.Sprintf("jobId must be a short name without '/': %s", missingJobFullName),
				},
			}
			for _, tc := range tcs {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					testError(t, ctx, tr, tc.toolName, tc.request, tc.wantCode, tc.wantMsg)
				})
			}
		})
		t.Run("auth", func(t *testing.T) {
			t.Parallel()
			runAuthTest(t, ctx, tr, "get-job-with-auth", map[string]any{"jobId": jobId}, http.StatusOK)
		})
	})
	t.Run("list-clusters", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			runListClustersTest(t, ctx, tr, clusterClient)
		})
		t.Run("errors", func(t *testing.T) {
			t.Parallel()
			tcs := []struct {
				name     string
				toolName string
				request  map[string]any
				wantCode int
				wantMsg  string
			}{
				{
					name:     "zero page size",
					toolName: "list-clusters",
					request:  map[string]any{"pageSize": 0},
					wantCode: http.StatusOK,
					wantMsg:  "pageSize must be positive: 0",
				},
				{
					name:     "negative page size",
					toolName: "list-clusters",
					request:  map[string]any{"pageSize": -1},
					wantCode: http.StatusOK,
					wantMsg:  "pageSize must be positive: -1",
				},
			}
			for _, tc := range tcs {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					testError(t, ctx, tr, tc.toolName, tc.request, tc.wantCode, tc.wantMsg)
				})
			}
		})
		t.Run("auth", func(t *testing.T) {
			t.Parallel()
			runAuthTest(t, ctx, tr, "list-clusters-with-auth", map[string]any{"pageSize": 1}, http.StatusOK)
		})
	})

	t.Run("list-jobs", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			runListJobsTest(t, ctx, tr, jobClient)
		})
		t.Run("errors", func(t *testing.T) {
			t.Parallel()
			tcs := []struct {
				name     string
				toolName string
				request  map[string]any
				wantCode int
				wantMsg  string
			}{
				{
					name:     "zero page size",
					toolName: "list-jobs",
					request:  map[string]any{"pageSize": 0},
					wantCode: http.StatusOK,
					wantMsg:  "pageSize must be positive: 0",
				},
				{
					name:     "negative page size",
					toolName: "list-jobs",
					request:  map[string]any{"pageSize": -1},
					wantCode: http.StatusOK,
					wantMsg:  "pageSize must be positive: -1",
				},
			}
			for _, tc := range tcs {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					testError(t, ctx, tr, tc.toolName, tc.request, tc.wantCode, tc.wantMsg)
				})
			}
		})
		t.Run("auth", func(t *testing.T) {
			t.Parallel()
			runAuthTest(t, ctx, tr, "list-jobs-with-auth", map[string]any{
				"pageSize": 1,
				"filter":   "clusterName = " + dataprocListJobsCluster,
			}, http.StatusOK)
		})
	})
}

// dataprocTransport selects how the tests talk to the toolbox server: the
// legacy REST API, or the MCP endpoint when isMCP is set.
type dataprocTransport struct {
	isMCP bool
}

// dataprocResult is the outcome of a tool invocation. result holds the tool
// result as the REST API returns it, and body the full response text used for
// error message checks. toolErr is set when MCP reported an error result.
type dataprocResult struct {
	status  int
	result  string
	body    string
	toolErr bool
}

// startServer starts the toolbox server with toolsFile and waits until it is
// ready to serve. The REST API is only enabled for the non-MCP transport.
func (tr dataprocTransport) startServer(t *testing.T, ctx context.Context, toolsFile map[string]any) {
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

// invoke calls toolName with request and headers through the selected
// transport.
func (tr dataprocTransport) invoke(t *testing.T, ctx context.Context, toolName string, request map[string]any, headers map[string]string) dataprocResult {
	t.Helper()
	if tr.isMCP {
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, toolName, request, headers)
		if err != nil {
			return dataprocResult{status: statusCode, body: err.Error(), toolErr: true}
		}
		if mcpResp.Error != nil {
			return dataprocResult{status: statusCode, body: mcpResp.Error.Message, toolErr: true}
		}
		var text strings.Builder
		for _, content := range mcpResp.Result.Content {
			text.WriteString(content.Text)
		}
		if mcpResp.Result.IsError {
			return dataprocResult{status: statusCode, body: text.String(), toolErr: true}
		}
		// Dataproc results are a single JSON document, which the MCP server sends
		// as one text content block.
		if len(mcpResp.Result.Content) != 1 {
			t.Fatalf("%s returned %d content blocks, want 1: %v", toolName, len(mcpResp.Result.Content), mcpResp.Result.Content)
		}
		return dataprocResult{status: statusCode, result: text.String(), body: text.String()}
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}
	url := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", toolName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(requestBytes))
	if err != nil {
		t.Fatalf("unable to create request: %v", err)
	}
	req.Header.Add("Content-type", "application/json")
	for k, v := range headers {
		req.Header.Add(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("invokeTool failed: %v", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	res := dataprocResult{status: resp.StatusCode, body: string(bodyBytes)}
	if resp.StatusCode != http.StatusOK {
		return res
	}
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("error parsing response body %q: %v", string(bodyBytes), err)
	}
	res.result, _ = body["result"].(string)
	return res
}

// invokeForResult invokes toolName, requires a successful result and returns it.
func (tr dataprocTransport) invokeForResult(t *testing.T, ctx context.Context, toolName string, request map[string]any) string {
	t.Helper()
	res := tr.invoke(t, ctx, toolName, request, nil)
	if res.status != http.StatusOK || res.toolErr {
		t.Fatalf("response status code is not 200 (tool error: %v), got %d: %s", res.toolErr, res.status, res.body)
	}
	if res.result == "" {
		t.Fatalf("unable to find result in response body: %s", res.body)
	}
	return res.result
}

func runListClustersTest(t *testing.T, ctx context.Context, tr dataprocTransport, client *dataproc.ClusterControllerClient) {
	tcs := []struct {
		name     string
		filter   string
		pageSize int
		numPages int
		wantN    int
	}{
		{name: "one page", pageSize: 2, numPages: 1, wantN: 2},
		{name: "two pages", pageSize: 1, numPages: 2, wantN: 2},
		{name: "5 clusters", pageSize: 5, numPages: 1, wantN: 5},
		{name: "omit page size", numPages: 1, wantN: 20},
		{
			name:     "filtered",
			filter:   "status.state = STOPPED",
			pageSize: 2,
			numPages: 1,
			wantN:    2,
		},
		{
			name:     "empty",
			filter:   "status.state = STOPPED AND status.state = RUNNING",
			pageSize: 1,
			numPages: 1,
			wantN:    0,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want := []dataprocsrc.Cluster{}
			if tc.wantN > 0 {
				want = listClustersRpc(t, client, ctx, tc.filter, tc.wantN)
			}

			actual := []dataprocsrc.Cluster{}
			var pageToken string

			for i := 0; i < tc.numPages; i++ {
				request := map[string]any{
					"filter":    tc.filter,
					"pageToken": pageToken,
				}
				if tc.pageSize > 0 {
					request["pageSize"] = tc.pageSize
				}

				result := tr.invokeForResult(t, ctx, "list-clusters", request)

				var listResponse dataprocsrc.ListClustersResponse
				if err := json.Unmarshal([]byte(result), &listResponse); err != nil {
					t.Fatalf("error unmarshalling result %q: %s", result, err)
				}
				actual = append(actual, listResponse.Clusters...)
				pageToken = listResponse.NextPageToken
			}

			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("unexpected clusters: got %+v, want %+v", actual, want)
			}

			// want has URLs because it's created from Batch instances by the same utility function
			// used by the tool internals. Double-check that the URLs are reasonable.
			for _, cluster := range want {
				if !strings.HasPrefix(cluster.ConsoleURL, clusterURLPrefix) {
					t.Errorf("unexpected consoleUrl in cluster: %#v", cluster)
				}
				if !strings.HasPrefix(cluster.LogsURL, logsURLPrefix) {
					t.Errorf("unexpected logsUrl in cluster: %#v", cluster)
				}
			}
		})
	}
}

func runGetClusterTest(t *testing.T, ctx context.Context, tr dataprocTransport, client *dataproc.ClusterControllerClient, fullName string) {
	// First get the cluster details directly from the Go proto API.
	req := &dataprocpb.GetClusterRequest{
		ProjectId:   dataprocProject,
		Region:      dataprocRegion,
		ClusterName: fullName[strings.LastIndex(fullName, "/")+1:],
	}
	rawWantClusterPb, err := client.GetCluster(ctx, req)
	if err != nil {
		t.Fatalf("failed to get cluster: %s", err)
	}

	// Trim unknown fields from the proto by marshalling and unmarshalling.
	jsonBytes, err := protojson.Marshal(rawWantClusterPb)
	if err != nil {
		t.Fatalf("failed to marshal cluster to JSON: %s", err)
	}
	var wantClusterPb dataprocpb.Cluster
	if err := protojson.Unmarshal(jsonBytes, &wantClusterPb); err != nil {
		t.Fatalf("error unmarshalling result: %s", err)
	}

	shortName := fullName[strings.LastIndex(fullName, "/")+1:]

	tcs := []struct {
		name        string
		clusterName string
		want        *dataprocpb.Cluster
	}{
		{
			name:        "found cluster",
			clusterName: shortName,
			want:        &wantClusterPb,
		},
	}

	t.Run("success", func(t *testing.T) {
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				request := map[string]any{"clusterName": tc.clusterName}
				resultStr := tr.invokeForResult(t, ctx, "get-cluster", request)
				var wrappedResult map[string]any
				if err := json.Unmarshal([]byte(resultStr), &wrappedResult); err != nil {
					t.Fatalf("error unmarshalling result %q: %s", resultStr, err)
				}

				consoleURL, ok := wrappedResult["consoleUrl"].(string)
				if !ok || !strings.HasPrefix(consoleURL, clusterURLPrefix) {
					t.Errorf("unexpected consoleUrl: %v", consoleURL)
				}
				logsURL, ok := wrappedResult["logsUrl"].(string)
				if !ok || !strings.HasPrefix(logsURL, logsURLPrefix) {
					t.Errorf("unexpected logsUrl: %v", logsURL)
				}

				clusterJSON, err := json.Marshal(wrappedResult["cluster"])
				if err != nil {
					t.Fatalf("failed to marshal cluster: %v", err)
				}

				// Unmarshal JSON to proto for proto-aware deep comparison.
				var cluster dataprocpb.Cluster
				if err := protojson.Unmarshal(clusterJSON, &cluster); err != nil {
					t.Fatalf("error unmarshalling cluster from wrapped result: %s", err)
				}

				if !cmp.Equal(&cluster, tc.want, protocmp.Transform()) {
					diff := cmp.Diff(&cluster, tc.want, protocmp.Transform())
					t.Errorf("GetCluster() returned diff (-got +want):\n%s", diff)
				}
			})
		}
	})

	t.Run("errors", func(t *testing.T) {
		tcs := []struct {
			name     string
			request  map[string]any
			wantCode int
			wantMsg  string
		}{
			{
				name:     "missing clusterName",
				request:  map[string]any{},
				wantCode: http.StatusOK,
				wantMsg:  "missing required parameter: clusterName",
			},
			{
				name:     "invalid name with slash",
				request:  map[string]any{"clusterName": "projects/foo/regions/bar/clusters/baz"}, // Full name requires matching project/region
				wantCode: http.StatusOK,
				wantMsg:  "clusterName must be a short name without '/'",
			},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				testError(t, ctx, tr, "get-cluster", tc.request, tc.wantCode, tc.wantMsg)
			})
		}
	})
}

func listClustersRpc(t *testing.T, client *dataproc.ClusterControllerClient, ctx context.Context, filter string, n int) []dataprocsrc.Cluster {
	req := &dataprocpb.ListClustersRequest{
		ProjectId: dataprocProject,
		Region:    dataprocRegion,
		PageSize:  int32(n),
	}
	if filter != "" {
		req.Filter = filter
	}

	it := client.ListClusters(ctx, req)
	pager := iterator.NewPager(it, n, "")
	var clusterPbs []*dataprocpb.Cluster
	_, err := pager.NextPage(&clusterPbs)
	if err != nil {
		t.Fatalf("failed to list clusters: %s", err)
	}

	clusters, err := dataprocsrc.ToClusters(clusterPbs, dataprocRegion)
	if err != nil {
		t.Fatalf("failed to convert clusters to JSON: %v", err)
	}

	return clusters
}

func runAuthTest(t *testing.T, ctx context.Context, tr dataprocTransport, toolName string, request map[string]any, wantStatus int) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	tcs := []struct {
		name     string
		headers  map[string]string
		wantCode int
	}{
		{
			name:     "valid token",
			headers:  map[string]string{"my-google-auth_token": idToken},
			wantCode: wantStatus,
		},
		{
			name:     "invalid token",
			headers:  map[string]string{"my-google-auth_token": "INVALID"},
			wantCode: http.StatusUnauthorized,
		},
		{
			name:     "missing header",
			headers:  nil,
			wantCode: http.StatusUnauthorized,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			res := tr.invoke(t, ctx, toolName, request, tc.headers)
			if res.status != tc.wantCode {
				t.Errorf("status code got %d, want %d. Body: %s", res.status, tc.wantCode, res.body)
			}
		})
	}
}

// testError invokes toolName and checks that it is rejected with wantCode and
// an error message containing wantMsg. Over MCP the rejection must also be an
// error result.
func testError(t *testing.T, ctx context.Context, tr dataprocTransport, toolName string, request map[string]any, wantCode int, wantMsg string) {
	res := tr.invoke(t, ctx, toolName, request, nil)

	if res.status != wantCode {
		t.Fatalf("response status code is not %d, got %d: %s", wantCode, res.status, res.body)
	}
	if tr.isMCP && !res.toolErr {
		t.Fatalf("expected an error result, got: %s", res.body)
	}
	if !strings.Contains(res.body, wantMsg) {
		t.Fatalf("response body does not contain %q: %s", wantMsg, res.body)
	}
}

func runListJobsTest(t *testing.T, ctx context.Context, tr dataprocTransport, client *dataproc.JobControllerClient) {
	tcs := []struct {
		name     string
		filter   string
		pageSize int
		numPages int
		wantN    int
	}{
		{name: "one page", pageSize: 2, numPages: 1, wantN: 2},
		{name: "two pages", pageSize: 1, numPages: 2, wantN: 2},
		{name: "10 batches", pageSize: 10, numPages: 1, wantN: 10},
		{name: "omit page size", numPages: 1, wantN: 20},
		{
			name:     "filtered",
			filter:   "status.state = NON_ACTIVE",
			pageSize: 20,
			numPages: 1,
			wantN:    20,
		},
		{
			name:     "empty",
			filter:   "status.state = NON_ACTIVE AND status.state = ACTIVE",
			pageSize: 1,
			numPages: 1,
			wantN:    0,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want := []dataprocsrc.Job{}
			if tc.wantN > 0 {
				want = listJobsRpc(t, client, ctx, tc.filter, tc.wantN)
			}

			actual := []dataprocsrc.Job{}
			var pageToken string
			for i := 0; i < tc.numPages; i++ {
				filter := tc.filter
				if filter != "" {
					filter += " AND "
				}
				filter += "clusterName = " + dataprocListJobsCluster
				request := map[string]any{
					"filter":    filter,
					"pageToken": pageToken,
				}
				if tc.pageSize > 0 {
					request["pageSize"] = tc.pageSize
				}

				result := tr.invokeForResult(t, ctx, "list-jobs", request)

				var listResponse dataprocsrc.ListJobsResponse
				if err := json.Unmarshal([]byte(result), &listResponse); err != nil {
					t.Fatalf("error unmarshalling result %q: %s", result, err)
				}
				actual = append(actual, listResponse.Jobs...)
				pageToken = listResponse.NextPageToken
			}

			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("unexpected jobs: got %+v, want %+v", actual, want)
			}

			// want has URLs because it's created from Job instances by the same utility function
			// used by the tool internals. Double-check that the URLs are reasonable.
			for _, job := range want {
				if !strings.HasPrefix(job.ConsoleURL, jobURLPrefix) {
					t.Errorf("unexpected consoleUrl in job: %#v", job)
				}
				if !strings.HasPrefix(job.LogsURL, logsURLPrefix) {
					t.Errorf("unexpected logsUrl in job: %#v", job)
				}
			}
		})
	}
}

func runGetJobTest(t *testing.T, ctx context.Context, tr dataprocTransport, client *dataproc.JobControllerClient, jobId string) {
	// First get the job details directly from the Go proto API.
	req := &dataprocpb.GetJobRequest{
		ProjectId: dataprocProject,
		Region:    dataprocRegion,
		JobId:     jobId,
	}
	rawWantJobPb, err := client.GetJob(ctx, req)
	if err != nil {
		t.Fatalf("failed to get job: %s", err)
	}

	// Trim unknown fields from the proto by marshalling and unmarshalling.
	jsonBytes, err := protojson.Marshal(rawWantJobPb)
	if err != nil {
		t.Fatalf("failed to marshal job to JSON: %s", err)
	}
	var wantJobPb dataprocpb.Job
	if err := protojson.Unmarshal(jsonBytes, &wantJobPb); err != nil {
		t.Fatalf("error unmarshalling result: %s", err)
	}

	tcs := []struct {
		name  string
		jobId string
		want  *dataprocpb.Job
	}{
		{
			name:  "found job",
			jobId: jobId,
			want:  &wantJobPb,
		},
	}

	t.Run("success", func(t *testing.T) {
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				request := map[string]any{"jobId": tc.jobId}
				resultStr := tr.invokeForResult(t, ctx, "get-job", request)
				var wrappedResult map[string]any
				if err := json.Unmarshal([]byte(resultStr), &wrappedResult); err != nil {
					t.Fatalf("error unmarshalling result %q: %s", resultStr, err)
				}

				consoleURL, ok := wrappedResult["consoleUrl"].(string)
				if !ok || !strings.HasPrefix(consoleURL, jobURLPrefix) {
					t.Errorf("unexpected consoleUrl: %v", consoleURL)
				}
				logsURL, ok := wrappedResult["logsUrl"].(string)
				if !ok || !strings.HasPrefix(logsURL, logsURLPrefix) {
					t.Errorf("unexpected logsUrl: %v", logsURL)
				}

				jobJSON, err := json.Marshal(wrappedResult["job"])
				if err != nil {
					t.Fatalf("failed to marshal job: %v", err)
				}

				// Unmarshal JSON to proto for proto-aware deep comparison.
				var job dataprocpb.Job
				if err := protojson.Unmarshal(jobJSON, &job); err != nil {
					t.Fatalf("error unmarshalling job from wrapped result: %s", err)
				}

				if !cmp.Equal(&job, tc.want, protocmp.Transform()) {
					diff := cmp.Diff(&job, tc.want, protocmp.Transform())
					t.Errorf("GetJob() returned diff (-got +want):\n%s", diff)
				}
			})
		}
	})

	t.Run("errors", func(t *testing.T) {
		tcs := []struct {
			name     string
			request  map[string]any
			wantCode int
			wantMsg  string
		}{
			{
				name:     "missing jobId",
				request:  map[string]any{},
				wantCode: http.StatusOK,
				wantMsg:  "missing required parameter: jobId",
			},
			{
				name:     "invalid name with slash",
				request:  map[string]any{"jobId": "projects/foo/regions/bar/jobs/baz"},
				wantCode: http.StatusOK,
				wantMsg:  "jobId must be a short name without '/'",
			},
		}
		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				testError(t, ctx, tr, "get-job", tc.request, tc.wantCode, tc.wantMsg)
			})
		}
	})
}

func listJobsRpc(t *testing.T, client *dataproc.JobControllerClient, ctx context.Context, filter string, n int) []dataprocsrc.Job {
	req := &dataprocpb.ListJobsRequest{
		ProjectId:   dataprocProject,
		Region:      dataprocRegion,
		ClusterName: dataprocListJobsCluster,
		PageSize:    int32(n),
	}
	if filter != "" {
		req.Filter = filter
	}

	it := client.ListJobs(ctx, req)
	pager := iterator.NewPager(it, n, "")
	var jobPbs []*dataprocpb.Job
	_, err := pager.NextPage(&jobPbs)
	if err != nil {
		t.Fatalf("failed to list jobs: %s", err)
	}

	jobs, err := dataprocsrc.ToJobs(jobPbs, dataprocRegion)
	if err != nil {
		t.Fatalf("failed to convert jobs: %v", err)
	}
	return jobs
}

func shortName(fullName string) string {
	parts := strings.Split(fullName, "/")
	return parts[len(parts)-1]
}
