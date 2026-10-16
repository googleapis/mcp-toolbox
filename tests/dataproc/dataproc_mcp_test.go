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
	"context"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/tests"
)

// dataprocMCP is the transport used by the MCP tests.
var dataprocMCP = dataprocTransport{isMCP: true}

func TestDataprocMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dataprocMCP.startServer(t, ctx, getDataprocToolsConfig(getDataprocVars(t)))

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getDataprocMCPExpectedTools())
	})
}

func TestDataprocClustersMCPToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute) // Clusters take time
	defer cancel()

	toolsFile, clusterClient, jobClient := setupDataprocTest(t, ctx)
	dataprocMCP.startServer(t, ctx, toolsFile)

	runDataprocTests(t, ctx, dataprocMCP, clusterClient, jobClient)
}

// getDataprocMCPExpectedTools returns the MCP manifests for the tools loaded by
// getDataprocToolsConfig. The config sets no descriptions.
func getDataprocMCPExpectedTools() []tests.MCPToolManifest {
	return []tests.MCPToolManifest{
		{
			Name: "get-cluster",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"clusterName": map[string]any{"description": "The short name of the cluster, e.g. for \"projects/my-project/regions/us-central1/clusters/my-cluster\", pass \"my-cluster\" (the project and region are inherited from the source)", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name: "get-cluster-with-auth",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"clusterName": map[string]any{"description": "The short name of the cluster, e.g. for \"projects/my-project/regions/us-central1/clusters/my-cluster\", pass \"my-cluster\" (the project and region are inherited from the source)", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name: "get-job",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"jobId": map[string]any{"description": "The job ID, e.g. for \"projects/my-project/regions/us-central1/jobs/my-job\", pass \"my-job\" (the project and region are inherited from the source)", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name: "get-job-with-auth",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"jobId": map[string]any{"description": "The job ID, e.g. for \"projects/my-project/regions/us-central1/jobs/my-job\", pass \"my-job\" (the project and region are inherited from the source)", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name: "list-clusters",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filter":    map[string]any{"description": "A filter constraining the clusters to list. Filters are case-sensitive and have the following syntax: field = value [AND [field = value]] ...  where field is one of status.state, clusterName, or labels.[KEY], and [KEY] is a label key. value can be * to match all values. status.state can be one of the following: ACTIVE, INACTIVE, CREATING, RUNNING, ERROR, DELETING, UPDATING, STOPPING, or STOPPED. ACTIVE contains the CREATING, UPDATING, and RUNNING states. INACTIVE contains the DELETING, ERROR, STOPPING, and STOPPED states. clusterName is the name of the cluster provided at creation time. Only the logical AND operator is supported; space-separated items are treated as having an implicit AND operator.", "type": "string"},
					"pageSize":  map[string]any{"default": float64(20), "description": "The maximum number of clusters to return in a single page (default 20)", "type": "integer"},
					"pageToken": map[string]any{"description": "A page token, received from a previous `ListClusters` call", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name: "list-clusters-with-auth",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filter":    map[string]any{"description": "A filter constraining the clusters to list. Filters are case-sensitive and have the following syntax: field = value [AND [field = value]] ...  where field is one of status.state, clusterName, or labels.[KEY], and [KEY] is a label key. value can be * to match all values. status.state can be one of the following: ACTIVE, INACTIVE, CREATING, RUNNING, ERROR, DELETING, UPDATING, STOPPING, or STOPPED. ACTIVE contains the CREATING, UPDATING, and RUNNING states. INACTIVE contains the DELETING, ERROR, STOPPING, and STOPPED states. clusterName is the name of the cluster provided at creation time. Only the logical AND operator is supported; space-separated items are treated as having an implicit AND operator.", "type": "string"},
					"pageSize":  map[string]any{"default": float64(20), "description": "The maximum number of clusters to return in a single page (default 20)", "type": "integer"},
					"pageToken": map[string]any{"description": "A page token, received from a previous `ListClusters` call", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name: "list-jobs",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filter":          map[string]any{"description": "A filter constraining the jobs to list. Filters are case-sensitive and have the following syntax: field = value [AND [field = value]] ... where field is clusterName, status.state, or labels.[KEY], and [KEY] is a label key. value can be * to match all values. status.state can be one of the following: PENDING, RUNNING, CANCEL_PENDING, JOB_STATE_CANCELLED, DONE, ERROR, or ATTEMPT_FAILURE. Only the logical AND operator is supported; space-separated items are treated as having an implicit AND operator. Filtering by clusterName is recommended to improve query performance.", "type": "string"},
					"jobStateMatcher": map[string]any{"description": "Specifies if the job state matcher should match ALL jobs, only ACTIVE jobs, or only NON_ACTIVE jobs. Defaults to ALL. Supported values: ALL, ACTIVE, NON_ACTIVE.", "type": "string"},
					"pageSize":        map[string]any{"default": float64(20), "description": "The maximum number of jobs to return in a single page (default 20)", "type": "integer"},
					"pageToken":       map[string]any{"description": "A page token, received from a previous `ListJobs` call", "type": "string"},
				},
				"required": []any{},
			},
		},
		{
			Name: "list-jobs-with-auth",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filter":          map[string]any{"description": "A filter constraining the jobs to list. Filters are case-sensitive and have the following syntax: field = value [AND [field = value]] ... where field is clusterName, status.state, or labels.[KEY], and [KEY] is a label key. value can be * to match all values. status.state can be one of the following: PENDING, RUNNING, CANCEL_PENDING, JOB_STATE_CANCELLED, DONE, ERROR, or ATTEMPT_FAILURE. Only the logical AND operator is supported; space-separated items are treated as having an implicit AND operator. Filtering by clusterName is recommended to improve query performance.", "type": "string"},
					"jobStateMatcher": map[string]any{"description": "Specifies if the job state matcher should match ALL jobs, only ACTIVE jobs, or only NON_ACTIVE jobs. Defaults to ALL. Supported values: ALL, ACTIVE, NON_ACTIVE.", "type": "string"},
					"pageSize":        map[string]any{"default": float64(20), "description": "The maximum number of jobs to return in a single page (default 20)", "type": "integer"},
					"pageToken":       map[string]any{"description": "A page token, received from a previous `ListJobs` call", "type": "string"},
				},
				"required": []any{},
			},
		},
	}
}
