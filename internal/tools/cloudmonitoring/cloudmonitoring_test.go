// Copyright 2025 Google LLC
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

package cloudmonitoring_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudmonitoring"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestInitialize(t *testing.T) {
	t.Parallel()

	wantParams := parameters.Parameters{
		parameters.NewStringParameter("projectId", "The Id of the Google Cloud project.", parameters.WithStringRequired(true)),
		parameters.NewStringParameter("query", "The promql query to execute.", parameters.WithStringRequired(true)),
	}

	testCases := []struct {
		desc    string
		cfg     cloudmonitoring.Config
		want    *tools.Manifest
		wantErr string
	}{
		{
			desc: "Success case with nil authRequired",
			cfg: cloudmonitoring.Config{
				ConfigBase: tools.ConfigBase{
					Name:         "test-tool",
					Description:  "A test description.",
					AuthRequired: nil,
				},
				Type:   "cloud-monitoring-query-prometheus",
				Source: "my-monitoring-source",
			},
			want: &tools.Manifest{
				Description:  "A test description.",
				Parameters:   wantParams.Manifest(),
				AuthRequired: nil,
			},
		},
		{
			desc: "Success case with specified authRequired",
			cfg: cloudmonitoring.Config{
				ConfigBase: tools.ConfigBase{
					Name:         "test-tool-with-auth",
					Description:  "Another test description.",
					AuthRequired: []string{"google-auth-service"},
				},
				Type:   "cloud-monitoring-query-prometheus",
				Source: "my-monitoring-source",
			},
			want: &tools.Manifest{
				Description:  "Another test description.",
				Parameters:   wantParams.Manifest(),
				AuthRequired: []string{"google-auth-service"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			tool, err := tc.cfg.Initialize(context.Background())

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("Initialize() succeeded, want error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Initialize() error = %q, want error containing %q", err, tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Initialize() failed: %v", err)
			}

			got, err := tool.Manifest(nil)
			if err != nil {
				t.Fatalf("Manifest() failed: %v", err)
			}
			if diff := cmp.Diff(tc.want, &got); diff != "" {
				t.Errorf("Initialize() manifest mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

type mockSource struct {
	sources.Source
	client   *http.Client
	runQuery func(projectID, query string) (any, error)
}

func (m *mockSource) SourceType() string {
	return "cloud-monitoring"
}

func (m *mockSource) IsReadOnly() bool {
	return false
}

func (m *mockSource) Client() *http.Client {
	return m.client
}

func (m *mockSource) RunQuery(projectID, query string) (any, error) {
	if m.runQuery != nil {
		return m.runQuery(projectID, query)
	}
	return nil, nil
}

func TestTool_Invoke(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer server.Close()

	mock := &mockSource{
		client: server.Client(),
		runQuery: func(projectID, query string) (any, error) {
			resp, err := server.Client().Get(server.URL + "?query=" + url.QueryEscape(query))
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return nil, fmt.Errorf("query failed with status %d: %s", resp.StatusCode, string(body))
			}
			var res any
			if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
				return nil, err
			}
			return res, nil
		},
	}

	cfg := cloudmonitoring.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test-monitoring-tool",
			Description: "Query Prometheus metrics in Cloud Monitoring",
		},
		Type:   "cloud-monitoring-query-prometheus",
		Source: "my-monitoring-source",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	params := parameters.ParamValues{
		{Name: "projectId", Value: "test-project"},
		{Name: "query", Value: "up"},
	}

	resp, toolErr := tool.Invoke(context.Background(), mock, params, "")
	if toolErr != nil {
		t.Fatalf("tool.Invoke() failed: %v", toolErr)
	}
	if resp == nil {
		t.Fatalf("expected non-nil response from tool.Invoke()")
	}
}

func TestTool_Invoke_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":"error","error":"bad promql syntax"}`))
	}))
	defer server.Close()

	mock := &mockSource{
		client: server.Client(),
		runQuery: func(projectID, query string) (any, error) {
			resp, err := server.Client().Get(server.URL + "?query=" + url.QueryEscape(query))
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return nil, fmt.Errorf("query failed with status %d: %s", resp.StatusCode, string(body))
			}
			return nil, nil
		},
	}

	cfg := cloudmonitoring.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test-monitoring-tool-err",
			Description: "Query Prometheus metrics with error",
		},
		Type:   "cloud-monitoring-query-prometheus",
		Source: "my-monitoring-source",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	params := parameters.ParamValues{
		{Name: "projectId", Value: "test-project"},
		{Name: "query", Value: "invalid{{{"},
	}

	_, toolErr := tool.Invoke(context.Background(), mock, params, "")
	if toolErr == nil {
		t.Fatalf("expected tool.Invoke() to fail, got nil")
	}
	if strings.Contains(toolErr.Error(), "source used is not compatible") {
		t.Fatalf("tool failed prematurely due to source incompatibility rather than query error: %v", toolErr)
	}
}

func TestTool_Invoke_IncompatibleSource(t *testing.T) {
	cfg := cloudmonitoring.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "test-incompatible-tool",
			Description: "Test incompatible source",
		},
		Type:   "cloud-monitoring-query-prometheus",
		Source: "my-monitoring-source",
	}

	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	params := parameters.ParamValues{
		{Name: "projectId", Value: "test-project"},
		{Name: "query", Value: "up"},
	}

	_, toolErr := tool.Invoke(context.Background(), nil, params, "")
	if toolErr == nil {
		t.Fatalf("expected error with nil source, got nil")
	}
	if !strings.Contains(toolErr.Error(), "source used is not compatible") {
		t.Errorf("expected incompatible source error, got: %v", toolErr)
	}
}

func TestParseFromYamlCloudMonitoring(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		want server.ToolConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: tool
			name: example_tool
			type: cloud-monitoring-query-prometheus
			source: my-instance
			description: some description
			`,
			want: server.ToolConfigs{
				"example_tool": cloudmonitoring.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "cloud-monitoring-query-prometheus",
					Source: "my-instance",
				},
			},
		},
		{
			desc: "advanced example",
			in: `
			kind: tool
			name: example_tool
			type: cloud-monitoring-query-prometheus
			source: my-instance
			description: some description
			authRequired:
				- my-google-auth-service
				- other-auth-service
			`,
			want: server.ToolConfigs{
				"example_tool": cloudmonitoring.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:   "cloud-monitoring-query-prometheus",
					Source: "my-instance",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}

func TestFailParseFromYamlCloudMonitoring(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "Invalid type",
			in: `
			kind: tool
			name: example_tool
			type: invalid-type
			source: my-instance
			description: some description
			`,
			err: `unknown tool type: "invalid-type"`,
		},
		{
			desc: "missing source",
			in: `
			kind: tool
			name: example_tool
			type: cloud-monitoring-query-prometheus
			description: some description
			`,
			err: `Key: 'Config.Source' Error:Field validation for 'Source' failed on the 'required' tag`,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if !strings.Contains(errStr, tc.err) {
				t.Fatalf("unexpected error string: got %q, want substring %q", errStr, tc.err)
			}
		})
	}
}
