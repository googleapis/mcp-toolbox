// Copyright 2026 Google LLC
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

package cloudsql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"google.golang.org/api/sqladmin/v1"
)

var (
	restoreBackupToolKind = "cloud-sql-restore-backup"
)

type restoreBackupTransport struct {
	transport http.RoundTripper
	url       *url.URL
}

func (t *restoreBackupTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), "https://sqladmin.googleapis.com") {
		req.URL.Scheme = t.url.Scheme
		req.URL.Host = t.url.Host
	}
	return t.transport.RoundTrip(req)
}

type masterRestoreBackupHandler struct {
	t *testing.T
}

func (h *masterRestoreBackupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.UserAgent(), "genai-toolbox/") {
		h.t.Errorf("User-Agent header not found")
	}
	var body sqladmin.InstancesRestoreBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.t.Errorf("failed to decode request body: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.t.Logf("Received request body: %+v", body)

	var expectedBody sqladmin.InstancesRestoreBackupRequest
	var response any
	var statusCode int

	switch {
	case body.Backup != "":
		expectedBody = sqladmin.InstancesRestoreBackupRequest{
			Backup: "projects/p1/backups/test-uid",
		}
		response = map[string]any{"name": "op1", "status": "PENDING"}
		statusCode = http.StatusOK
	case body.BackupdrBackup != "":
		expectedBody = sqladmin.InstancesRestoreBackupRequest{
			BackupdrBackup: "projects/p1/locations/us-central1/backupVaults/test-vault/dataSources/test-ds/backups/test-uid",
		}
		response = map[string]any{"name": "op1", "status": "PENDING"}
		statusCode = http.StatusOK
	case body.RestoreBackupContext != nil:
		expectedBody = sqladmin.InstancesRestoreBackupRequest{
			RestoreBackupContext: &sqladmin.RestoreBackupContext{
				Project:     "p1",
				InstanceId:  "source",
				BackupRunId: 12345,
			},
		}
		response = map[string]any{"name": "op1", "status": "PENDING"}
		statusCode = http.StatusOK
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": `oaraneter "backup_id" is required`,
		})
		return
	}

	if diff := cmp.Diff(expectedBody, body); diff != "" {
		h.t.Errorf("unexpected request body (-want +got):\n%s", diff)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func TestRestoreBackupToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	handler := &masterRestoreBackupHandler{t: t}
	server := httptest.NewServer(handler)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}

	originalTransport := http.DefaultClient.Transport
	if originalTransport == nil {
		originalTransport = http.DefaultTransport
	}
	http.DefaultClient.Transport = &restoreBackupTransport{
		transport: originalTransport,
		url:       serverURL,
	}
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
	})

	args := []string{"--enable-api"}
	toolsFile := getRestoreBackupToolsConfig()
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	tcs := []struct {
		name        string
		toolName    string
		body        string
		want        string
		expectError bool
		errorStatus int
	}{
		{
			name:     "successful restore with standard backup",
			toolName: "restore-backup",
			body:     `{"target_project": "p1", "target_instance": "instance-standard", "backup_id": "12345", "source_project": "p1", "source_instance": "source"}`,
			want:     `{"name":"op1","status":"PENDING"}`,
		},
		{
			name:     "successful restore with project level backup",
			toolName: "restore-backup",
			body:     `{"target_project": "p1", "target_instance": "instance-project-level", "backup_id": "projects/p1/backups/test-uid"}`,
			want:     `{"name":"op1","status":"PENDING"}`,
		},
		{
			name:     "successful restore with BackupDR backup",
			toolName: "restore-backup",
			body:     `{"target_project": "p1", "target_instance": "instance-project-level", "backup_id": "projects/p1/locations/us-central1/backupVaults/test-vault/dataSources/test-ds/backups/test-uid"}`,
			want:     `{"name":"op1","status":"PENDING"}`,
		},
		{
			name:     "missing source instance info for standard backup",
			toolName: "restore-backup",
			body:     `{"target_project": "p1", "target_instance": "instance-project-level", "backup_id": "12345"}`,
			want:     `{"error":"error processing GCP request: source project and instance are required when restoring via backup ID"}`,
		},
		{
			name:     "missing source instance with source project for standard backup",
			toolName: "restore-backup",
			body:     `{"target_project": "p1", "target_instance": "instance-project-level", "backup_id": "12345", "source_project": "p1"}`,
			want:     `{"error":"error processing GCP request: source project and instance are required when restoring via backup ID"}`,
		},
		{
			name:     "missing backup identifier",
			toolName: "restore-backup",
			body:     `{"target_project": "p1", "target_instance": "instance-project-level"}`,
			want:     `{"error":"parameter \"backup_id\" is required"}`,
		},
		{
			name:     "missing target instance info",
			toolName: "restore-backup",
			body:     `{"backup_id": "12345"}`,
			want:     `{"error":"parameter \"target_project\" is required"}`,
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
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

			if tc.expectError {
				if resp.StatusCode != tc.errorStatus {
					bodyBytes, _ := io.ReadAll(resp.Body)
					t.Fatalf("expected status %d but got %d: %s", tc.errorStatus, resp.StatusCode, string(bodyBytes))
				}
				return
			}

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var result struct {
				Result string `json:"result"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("failed to decode response envelope: %v", err)
			}

			got := strings.TrimSpace(result.Result)
			want := strings.TrimSpace(tc.want)

			if got != want {
				t.Fatalf("unexpected result string:\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func getRestoreBackupToolsConfig() map[string]any {
	return map[string]any{
		"sources": map[string]any{
			"my-cloud-sql-source": map[string]any{
				"kind": "cloud-sql-admin",
			},
		},
		"tools": map[string]any{
			"restore-backup": map[string]any{
				"kind":   restoreBackupToolKind,
				"source": "my-cloud-sql-source",
			},
		},
	}
}
