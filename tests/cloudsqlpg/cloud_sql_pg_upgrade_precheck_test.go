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

package cloudsqlpg

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
	sqladmin "google.golang.org/api/sqladmin/v1"
)

var (
	preCheckToolType = "postgres-upgrade-precheck"
)

type preCheckTransport struct {
	transport http.RoundTripper
	url       *url.URL
}

func (t *preCheckTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), "https://sqladmin.googleapis.com") {
		req.URL.Scheme = t.url.Scheme
		req.URL.Host = t.url.Host
	}
	return t.transport.RoundTrip(req)
}

type preCheckHandler struct {
	t            *testing.T
	opCount      int
	opResults    map[string][]*sqladmin.PreCheckResponse
	opPollCounts map[string]int
}

func (h *preCheckHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ua := r.Header.Get("User-Agent")
	if !strings.Contains(ua, "genai-toolbox/") {
		h.t.Errorf("User-Agent header not found in %q", ua)
	}

	if strings.Contains(r.URL.Path, "/operations/") {
		h.handleOperations(w, r)
		return
	}

	if strings.Contains(r.URL.Path, "/preCheckMajorVersionUpgrade") {
		h.handlePreCheckV1(w, r)
		return
	}

	http.Error(w, fmt.Sprintf("unhandled path: %s", r.URL.Path), http.StatusNotFound)
}

func (h *preCheckHandler) handlePreCheckV1(w http.ResponseWriter, r *http.Request) {
	var body sqladmin.InstancesPreCheckMajorVersionUpgradeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.t.Fatalf("failed to decode request body: %v", err)
	}

	if body.PreCheckMajorVersionUpgradeContext == nil || body.PreCheckMajorVersionUpgradeContext.TargetDatabaseVersion == "" {
		http.Error(w, "missing targetDatabaseVersion", http.StatusBadRequest)
		return
	}

	parts := strings.Split(r.URL.Path, "/")

	if len(parts) < 7 {
		msg := fmt.Sprintf("handlePreCheckV1: Expected 7 path parts, got %d for path %s", len(parts), r.URL.Path)
		h.t.Errorf("%s", msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	project := parts[3]
	instanceName := parts[5]

	h.opCount++
	opName := fmt.Sprintf("op-%s-%s-%d", project, instanceName, h.opCount)

	var preCheckResult []*sqladmin.PreCheckResponse
	statusCode := http.StatusOK

	switch instanceName {
	case "instance-ok":
		h.opResults[opName] = nil // This will make PreCheckResponse nil inside the context
	case "instance-empty":
		preCheckResult = []*sqladmin.PreCheckResponse{} // No issues
		h.opResults[opName] = preCheckResult
	case "instance-warnings":
		preCheckResult = []*sqladmin.PreCheckResponse{
			{
				Message:         "This is a warning.",
				MessageType:     "WARNING",
				ActionsRequired: []string{"Check documentation."},
			},
		}
		h.opResults[opName] = preCheckResult
	case "instance-errors":
		preCheckResult = []*sqladmin.PreCheckResponse{
			{
				Message:         "This is a critical error.",
				MessageType:     "ERROR",
				ActionsRequired: []string{"Fix this now."},
			},
		}
		h.opResults[opName] = preCheckResult
	case "instance-notfound":
		http.Error(w, "Not authorized to access instance", http.StatusForbidden)
		return
	default:
		msg := fmt.Sprintf("unhandled instance name in mock: %s", instanceName)
		h.t.Errorf("handlePreCheckV1 default case: %s", msg)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	response := map[string]any{"name": opName, "status": "PENDING"}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *preCheckHandler) handleOperations(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	opName := parts[len(parts)-1]

	h.opPollCounts[opName]++

	result, ok := h.opResults[opName]
	if !ok {
		http.Error(w, fmt.Sprintf("operation not found: %s", opName), http.StatusNotFound)
		return
	}

	status := "PENDING"
	if h.opPollCounts[opName] > 1 {
		status = "DONE"
	}

	opResponse := sqladmin.Operation{
		Name:   opName,
		Status: status,
		Kind:   "sql#operation",
	}

	if status == "DONE" {
		opResponse.PreCheckMajorVersionUpgradeContext = &sqladmin.PreCheckMajorVersionUpgradeContext{
			PreCheckResponse: result, // This can be nil or empty
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(opResponse); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// PreCheckResultItem holds the details of a single check result.
type PreCheckResultItem struct {
	Message         string   `json:"message"`
	MessageType     string   `json:"messageType"`
	ActionsRequired []string `json:"actionsRequired"`
}

// PreCheckAPIResponse holds the array of pre-check results.
type PreCheckAPIResponse struct {
	Items []PreCheckResultItem `json:"preCheckResponse"`
}

func TestPreCheckToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	handler := &preCheckHandler{
		t:            t,
		opResults:    make(map[string][]*sqladmin.PreCheckResponse),
		opPollCounts: make(map[string]int),
	}
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
	http.DefaultClient.Transport = &preCheckTransport{
		transport: originalTransport,
		url:       serverURL,
	}
	t.Cleanup(func() {
		http.DefaultClient.Transport = originalTransport
	})

	args := []string{"--enable-api"}
	toolsFile := getPreCheckToolsConfig()
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, err = testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	tcs := []struct {
		name        string
		toolName    string
		body        string
		want        string
		expectError bool
		errorStatus int
		errorMsg    string
	}{
		{
			name:     "successful precheck - nil response in context",
			toolName: "precheck-tool",
			body:     `{"project": "p1", "instance": "instance-ok", "targetDatabaseVersion": "POSTGRES_18"}`,
			want:     `{"preCheckResponse":[]}`, // Expect empty items list
		},
		{
			name:     "successful precheck - empty issues",
			toolName: "precheck-tool",
			body:     `{"project": "p1", "instance": "instance-empty", "targetDatabaseVersion": "POSTGRES_18"}`,
			want:     `{"preCheckResponse":[]}`,
		},
		{
			name:     "successful precheck - with warnings",
			toolName: "precheck-tool",
			body:     `{"project": "p1", "instance": "instance-warnings", "targetDatabaseVersion": "POSTGRES_18"}`,
			want:     `{"preCheckResponse":[{"actionsRequired":["Check documentation."],"type":"","message":"This is a warning.","messageType":"WARNING"}]}`,
		},
		{
			name:     "successful precheck - with errors",
			toolName: "precheck-tool",
			body:     `{"project": "p1", "instance": "instance-errors", "targetDatabaseVersion": "POSTGRES_18"}`,
			want:     `{"preCheckResponse":[{"actionsRequired":["Fix this now."],"type":"","message":"This is a critical error.","messageType":"ERROR"}]}`,
		},
		{
			name:        "instance not found",
			toolName:    "precheck-tool",
			body:        `{"project": "p1", "instance": "instance-notfound", "targetDatabaseVersion": "POSTGRES_18"}`,
			want:        `{"error":"failed to access GCP resource: googleapi: got HTTP response code 403 with body: Not authorized to access instance\n"}`,
			expectError: true,
			errorStatus: http.StatusInternalServerError,
			errorMsg:    "failed to access GCP resource: googleapi: got HTTP response code 403",
		},
		{
			name:     "missing required parameter - project",
			toolName: "precheck-tool",
			body:     `{"instance": "instance-ok", "targetDatabaseVersion": "POSTGRES_18"}`,
			want:     `{"error":"parameter \"project\" is required"}`,
		},
		{
			name:     "missing required parameter - instance",
			toolName: "precheck-tool",
			body:     `{"project": "p1", "targetDatabaseVersion": "POSTGRES_18"}`, // Missing instance
			want:     `{"error":"parameter \"instance\" is required"}`,
		},
		{
			name:     "missing parameter - targetDatabaseVersion",
			toolName: "precheck-tool",
			body:     `{"project": "p1", "instance": "instance-empty"}`, // Uses default POSTGRES_18
			want:     `{"preCheckResponse":[]}`,
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, api, bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")
			req.Header.Add("Authorization", "Bearer FAKE_TOKEN")

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
				bodyBytes, _ := io.ReadAll(resp.Body)
				if !strings.Contains(string(bodyBytes), tc.errorMsg) {
					t.Errorf("expected error message to contain %q, got %s", tc.errorMsg, string(bodyBytes))
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
				t.Fatalf("failed to decode response: %v", err)
			}

			var got PreCheckAPIResponse
			if err := json.Unmarshal([]byte(result.Result), &got); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}

			var want PreCheckAPIResponse
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatalf("failed to unmarshal want: %v", err)
			}

			if diff := cmp.Diff(want.Items, got.Items, cmp.Comparer(func(a, b PreCheckResultItem) bool {
				return a.Message == b.Message && a.MessageType == b.MessageType && cmp.Equal(a.ActionsRequired, b.ActionsRequired)
			})); diff != "" {
				t.Errorf("unexpected result: diff (-want +got):\n%s", diff)
			}
		})
	}
}

func getPreCheckToolsConfig() map[string]any {
	return map[string]any{
		"sources": map[string]any{
			"my-cloud-sql-source": map[string]any{
				"type": "cloud-sql-admin",
			},
		},
		"tools": map[string]any{
			"precheck-tool": map[string]any{
				"type":   preCheckToolType,
				"source": "my-cloud-sql-source",
			},
		},
	}
}
