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

//go:build integration

package cloudsqlmysql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

const (
	envMysqlConnectGKEConnectionName  = "CLOUDSQL_MYSQL_CONNECTION_NAME" // project:region:instance
	envMysqlConnectGKEDatabase        = "CLOUDSQL_MYSQL_DATABASE"
	envMysqlConnectGKEClusterName     = "CLOUDSQL_TEST_GKE_CLUSTER_NAME"
	envMysqlConnectGKEClusterLocation = "CLOUDSQL_TEST_GKE_CLUSTER_LOCATION"
	envMysqlConnectGKEClusterProject  = "CLOUDSQL_TEST_GKE_CLUSTER_PROJECT"
)

func getMysqlConnectGKEParams(t *testing.T) map[string]any {
	t.Helper()
	conn := os.Getenv(envMysqlConnectGKEConnectionName)
	if conn == "" {
		t.Skipf("skipping: %s not set", envMysqlConnectGKEConnectionName)
	}
	cluster := os.Getenv(envMysqlConnectGKEClusterName)
	if cluster == "" {
		t.Skipf("skipping: %s not set", envMysqlConnectGKEClusterName)
	}
	params := map[string]any{
		"instance_connection_name": conn,
		"cluster_name":             cluster,
	}
	if loc := os.Getenv(envMysqlConnectGKEClusterLocation); loc != "" {
		params["cluster_location"] = loc
	}
	if proj := os.Getenv(envMysqlConnectGKEClusterProject); proj != "" {
		params["cluster_project"] = proj
	}
	if db := os.Getenv(envMysqlConnectGKEDatabase); db != "" {
		params["database_name"] = db
	}
	return params
}

func getMysqlConnectGKEToolsConfig() map[string]any {
	return map[string]any{
		"sources": map[string]any{
			"my-cloud-sql-source": map[string]any{
				"type": "cloud-sql-admin",
			},
		},
		"tools": map[string]any{
			"connect_to_gke": map[string]any{
				"type":        "cloud-sql-connect-gke",
				"source":      "my-cloud-sql-source",
				"description": "Integration test: Connect MySQL to GKE cluster",
			},
			"connect_to_gke_with_language": map[string]any{
				"type":        "cloud-sql-connect-gke",
				"source":      "my-cloud-sql-source",
				"description": "Integration test: MySQL → GKE with python snippet",
			},
		},
	}
}

func TestCloudSQLMySQLConnectGKE(t *testing.T) {
	baseParams := getMysqlConnectGKEParams(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd, cleanup, err := tests.StartCmd(ctx, getMysqlConnectGKEToolsConfig(), "--enable-api")
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, wcancel := context.WithTimeout(ctx, 30*time.Second)
	defer wcancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs:\n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	cases := []struct {
		name     string
		toolName string
		params   map[string]any
	}{
		{
			name:     "invoke without language",
			toolName: "connect_to_gke",
			params:   baseParams,
		},
		{
			name:     "invoke with python snippet",
			toolName: "connect_to_gke_with_language",
			params:   mergeMysqlConnectGCEParams(baseParams, map[string]any{"language": "python"}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.params)
			if err != nil {
				t.Fatalf("marshal params: %v", err)
			}
			api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, api, bytes.NewReader(body))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			defer resp.Body.Close()

			raw, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, body = %s", resp.StatusCode, string(raw))
			}
			var envelope struct {
				Result string `json:"result"`
			}
			if err := json.Unmarshal(raw, &envelope); err != nil {
				t.Fatalf("decode envelope: %v (body=%s)", err, string(raw))
			}
			if envelope.Result == "" {
				t.Fatalf("empty result; body=%s", string(raw))
			}
			var result struct {
				ComputeType string `json:"computeType"`
				Summary     string `json:"summary"`
				SetupSteps  []any  `json:"setupSteps"`
			}
			if err := json.Unmarshal([]byte(envelope.Result), &result); err != nil {
				t.Fatalf("decode result: %v (result=%s)", err, envelope.Result)
			}
			if result.ComputeType != "gke" || result.Summary == "" || len(result.SetupSteps) == 0 {
				t.Fatalf("unexpected GKE result: %s", envelope.Result)
			}
		})
	}
}
