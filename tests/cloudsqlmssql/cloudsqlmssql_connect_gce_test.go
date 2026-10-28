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

package cloudsqlmssql

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
	envMssqlConnectGCEConnectionName = "CLOUDSQL_MSSQL_CONNECTION_NAME" // project:region:instance
	envMssqlConnectGCEDatabase       = "CLOUDSQL_MSSQL_DATABASE"
	envMssqlConnectGCEVMName         = "CLOUDSQL_TEST_GCE_VM_NAME"
	envMssqlConnectGCEVMZone         = "CLOUDSQL_TEST_GCE_VM_ZONE"
)

func getMssqlConnectGCEParams(t *testing.T) map[string]any {
	t.Helper()
	conn := os.Getenv(envMssqlConnectGCEConnectionName)
	if conn == "" {
		t.Skipf("skipping: %s not set", envMssqlConnectGCEConnectionName)
	}
	vm := os.Getenv(envMssqlConnectGCEVMName)
	if vm == "" {
		t.Skipf("skipping: %s not set", envMssqlConnectGCEVMName)
	}
	params := map[string]any{
		"instance_connection_name": conn,
		"vm_name":                  vm,
	}
	if zone := os.Getenv(envMssqlConnectGCEVMZone); zone != "" {
		params["vm_zone"] = zone
	}
	if db := os.Getenv(envMssqlConnectGCEDatabase); db != "" {
		params["database_name"] = db
	}
	return params
}

func getMssqlConnectGCEToolsConfig() map[string]any {
	return map[string]any{
		"sources": map[string]any{
			"my-cloud-sql-source": map[string]any{
				"type": "cloud-sql-admin",
			},
		},
		"tools": map[string]any{
			"connect_to_gce": map[string]any{
				"type":        "cloud-sql-connect-gce",
				"source":      "my-cloud-sql-source",
				"description": "Integration test: Connect SQL Server to GCE VM",
			},
			"connect_to_gce_with_language": map[string]any{
				"type":        "cloud-sql-connect-gce",
				"source":      "my-cloud-sql-source",
				"description": "Integration test: SQL Server → GCE with python snippet",
			},
		},
	}
}

func TestCloudSQLMSSQLConnectGCE(t *testing.T) {
	baseParams := getMssqlConnectGCEParams(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd, cleanup, err := tests.StartCmd(ctx, getMssqlConnectGCEToolsConfig(), "--enable-api")
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
			toolName: "connect_to_gce",
			params:   baseParams,
		},
		{
			name:     "invoke with python snippet",
			toolName: "connect_to_gce_with_language",
			params:   mergeMssqlConnectGCEParams(baseParams, map[string]any{"language": "python"}),
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
		})
	}
}

func mergeMssqlConnectGCEParams(maps ...map[string]any) map[string]any {
	out := make(map[string]any)
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
