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

package conversationalanalyticsupdatedataagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	cloudgdads "github.com/googleapis/mcp-toolbox/internal/sources/cloudgda"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"golang.org/x/oauth2"
)

type fakeSource struct {
	sources.Source
	projectID  string
	clientAuth bool
}

func (f *fakeSource) GoogleCloudTokenSourceWithScope(ctx context.Context, scope string) (oauth2.TokenSource, error) {
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fake-token"}), nil
}
func (f *fakeSource) GetProjectID() string         { return f.projectID }
func (f *fakeSource) UseClientAuthorization() bool { return f.clientAuth }

func TestParseFromYamlConversationalAnalyticsUpdateDataAgent(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	readOnlyFalse, idempotentTrue := false, true
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
            type: conversational-analytics-update-data-agent
            source: my-instance
            description: some description
            `,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "conversational-analytics-update-data-agent",
					Source: "my-instance",
				},
			},
		},
		{
			desc: "advanced example with location",
			in: `
            kind: tool
            name: example_tool
            type: conversational-analytics-update-data-agent
            source: my-instance
            description: some description
            location: us-central1
            authRequired:
                - my-google-auth-service
            `,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service"},
					},
					Type:     "conversational-analytics-update-data-agent",
					Source:   "my-instance",
					Location: "us-central1",
				},
			},
		},
		{
			desc: "annotations override",
			in: `
            kind: tool
            name: example_tool
            type: conversational-analytics-update-data-agent
            source: my-instance
            description: some description
            annotations:
                readOnlyHint: false
                idempotentHint: true
            `,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:        "conversational-analytics-update-data-agent",
					Source:      "my-instance",
					Annotations: &tools.ToolAnnotations{ReadOnlyHint: &readOnlyFalse, IdempotentHint: &idempotentTrue},
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
				t.Fatalf("incorrect parse: %v", diff)
			}
		})
	}
}

func TestUpdateDataAgentAnnotations(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		ConfigBase: tools.ConfigBase{Name: "update_agent", Description: "update agent"},
		Type:       toolType,
		Source:     "my-instance",
	}

	t.Run("defaults to write annotations and is suppressed on a read-only source", func(t *testing.T) {
		tl, err := cfg.Initialize(ctx)
		if err != nil {
			t.Fatalf("unexpected Initialize error: %v", err)
		}
		if diff := cmp.Diff(tools.NewWriteAnnotations(), tl.GetAnnotations(nil)); diff != "" {
			t.Fatalf("unexpected default annotations (-want +got):\n%s", diff)
		}
		if !tools.ShouldSuppress(ctx, tl, &cloudgdads.Source{Config: cloudgdads.Config{ReadOnly: true}}) {
			t.Fatalf("expected tool to be suppressed on a read-only source")
		}
		if tools.ShouldSuppress(ctx, tl, &cloudgdads.Source{Config: cloudgdads.Config{ReadOnly: false}}) {
			t.Fatalf("expected tool not to be suppressed on a writable source")
		}
	})

	t.Run("configured annotations override the default", func(t *testing.T) {
		readOnly, idempotent := false, true
		overrideCfg := cfg
		overrideCfg.Annotations = &tools.ToolAnnotations{ReadOnlyHint: &readOnly, IdempotentHint: &idempotent}
		tl, err := overrideCfg.Initialize(ctx)
		if err != nil {
			t.Fatalf("unexpected Initialize error: %v", err)
		}
		if diff := cmp.Diff(overrideCfg.Annotations, tl.GetAnnotations(nil)); diff != "" {
			t.Fatalf("unexpected configured annotations (-want +got):\n%s", diff)
		}
	})
}

func TestInvokeUpdateDataAgent(t *testing.T) {
	ctx := context.Background()
	src := &fakeSource{projectID: "test-proj"}

	cfg := Config{
		ConfigBase: tools.ConfigBase{
			Name:        "update_data_agent",
			Description: "update tool",
		},
		Type:   toolType,
		Source: "my-instance",
	}
	tl, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	baseTool := tl.(Tool)

	t.Run("success with flat and nested update_mask", func(t *testing.T) {
		var gotUpdateMask string
		var gotBody map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/dataAgents/my-agent") {
				gotUpdateMask = r.URL.Query().Get("updateMask")
				raw, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(raw, &gotBody)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"name": "projects/test-proj/locations/global/operations/op-456",
					"done": false,
				})
				return
			}
			if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/operations/op-456") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"name": "projects/test-proj/locations/global/operations/op-456",
					"done": true,
					"response": map[string]any{
						"name":        "projects/test-proj/locations/global/dataAgents/my-agent",
						"displayName": "Updated Agent",
					},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()
		tool.pollInterval = 5 * time.Millisecond
		tool.pollTimeout = 500 * time.Millisecond

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{
				"displayName": "Updated Agent",
				"dataAnalyticsAgent": map[string]any{
					"publishedContext": map[string]any{
						"systemInstruction": "Be concise.",
					},
				},
			}},
			{Name: "update_mask", Value: "displayName, dataAnalyticsAgent.publishedContext.systemInstruction"},
		}
		got, tbErr := tool.Invoke(ctx, src, params, "")
		if tbErr != nil {
			t.Fatalf("unexpected error: %v", tbErr)
		}
		if gotUpdateMask != "displayName,dataAnalyticsAgent.publishedContext.systemInstruction" {
			t.Fatalf("unexpected updateMask query param: %q", gotUpdateMask)
		}
		if gotBody["displayName"] != "Updated Agent" {
			t.Fatalf("unexpected request body: %v", gotBody)
		}
		gotMap, ok := got.(map[string]any)
		if !ok || gotMap["displayName"] != "Updated Agent" {
			t.Fatalf("unexpected response: %v", got)
		}
	})

	t.Run("domain-scoped project ID is accepted and used in the request path", func(t *testing.T) {
		var gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"done": true,
				"response": map[string]any{
					"name":        "projects/google.com:test-proj/locations/global/dataAgents/my-agent",
					"displayName": "Updated Agent",
				},
			})
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "Updated Agent"}},
			{Name: "update_mask", Value: "displayName"},
		}
		if _, tbErr := tool.Invoke(ctx, &fakeSource{projectID: "google.com:test-proj"}, params, ""); tbErr != nil {
			t.Fatalf("unexpected error for domain-scoped project: %v", tbErr)
		}
		if want := "/v1/projects/google.com:test-proj/locations/global/dataAgents/my-agent"; gotPath != want {
			t.Fatalf("expected request path %q, got %q", want, gotPath)
		}
	})

	t.Run("project ID is path-escaped in the request URL", func(t *testing.T) {
		var gotEscapedPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotEscapedPath = r.URL.EscapedPath()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"done": true,
				"response": map[string]any{
					"displayName": "Updated Agent",
				},
			})
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "Updated Agent"}},
			{Name: "update_mask", Value: "displayName"},
		}
		if _, tbErr := tool.Invoke(ctx, &fakeSource{projectID: "a/b"}, params, ""); tbErr != nil {
			t.Fatalf("unexpected error: %v", tbErr)
		}
		// PathEscape must encode "/" so the project ID stays a single path segment.
		if want := "/v1/projects/a%2Fb/locations/global/dataAgents/my-agent"; gotEscapedPath != want {
			t.Fatalf("expected escaped request path %q, got %q", want, gotEscapedPath)
		}
	})

	t.Run("synchronous done on initial response and LRO error payload", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"done": true,
				"error": map[string]any{
					"code":    400,
					"message": "invalid field mask",
				},
			})
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "Updated Agent"}},
			{Name: "update_mask", Value: "displayName"},
		}
		_, tbErr := tool.Invoke(ctx, src, params, "")
		var agentErr *util.AgentError
		if !errors.As(tbErr, &agentErr) || !strings.Contains(tbErr.Error(), "invalid field mask") {
			t.Fatalf("expected AgentError with 'invalid field mask', got %T: %v", tbErr, tbErr)
		}
	})

	t.Run("non-retryable poll HTTP 404 and polling timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodPatch {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"name": "projects/test-proj/locations/global/operations/op-404",
					"done": false,
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": "operation not found"}`))
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()
		tool.pollInterval = 5 * time.Millisecond
		tool.pollTimeout = 500 * time.Millisecond

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "Updated Agent"}},
			{Name: "update_mask", Value: "displayName"},
		}
		if _, tbErr := tool.Invoke(ctx, src, params, ""); tbErr == nil || !strings.Contains(tbErr.Error(), "polling failed with 404") {
			t.Fatalf("expected immediate 404 poll error, got: %v", tbErr)
		}

		timeoutSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "projects/test-proj/locations/global/operations/op-slow",
				"done": false,
			})
		}))
		defer timeoutSrv.Close()

		tool.endpoint = timeoutSrv.URL
		tool.httpClient = timeoutSrv.Client()
		tool.pollInterval = 10 * time.Millisecond
		tool.pollTimeout = 25 * time.Millisecond
		if _, tbErr := tool.Invoke(ctx, src, params, ""); tbErr == nil || !strings.Contains(tbErr.Error(), "Do not retry the operation.") {
			t.Fatalf("expected timeout error with 'Do not retry' guidance, got: %v", tbErr)
		}
	})

	t.Run("missing field in agent_config rejected before API call", func(t *testing.T) {
		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "Updated Agent"}},
			{Name: "update_mask", Value: "displayName,description"},
		}
		_, tbErr := baseTool.Invoke(ctx, src, params, "")
		if tbErr == nil || !strings.Contains(tbErr.Error(), "not present in agent_config") {
			t.Fatalf("expected missing update_mask field error, got: %v", tbErr)
		}
	})

	t.Run("empty update_mask, empty agent_config, invalid data_agent_id, and client OAuth missing token", func(t *testing.T) {
		emptyMaskParams := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "Updated Agent"}},
			{Name: "update_mask", Value: " , "},
		}
		if _, tbErr := baseTool.Invoke(ctx, src, emptyMaskParams, ""); tbErr == nil || !strings.Contains(tbErr.Error(), "update_mask is required") {
			t.Fatalf("expected empty update_mask error, got: %v", tbErr)
		}

		emptyConfigParams := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{}},
			{Name: "update_mask", Value: "displayName"},
		}
		if _, tbErr := baseTool.Invoke(ctx, src, emptyConfigParams, ""); tbErr == nil || !strings.Contains(tbErr.Error(), "non-empty JSON object") {
			t.Fatalf("expected empty agent_config error, got: %v", tbErr)
		}

		badIDParams := parameters.ParamValues{
			{Name: "data_agent_id", Value: "../bad-id"},
			{Name: "agent_config", Value: map[string]any{"displayName": "Updated Agent"}},
			{Name: "update_mask", Value: "displayName"},
		}
		if _, tbErr := baseTool.Invoke(ctx, src, badIDParams, ""); tbErr == nil || !strings.Contains(tbErr.Error(), "disallowed characters") {
			t.Fatalf("expected validation error for bad data_agent_id, got: %v", tbErr)
		}

		clientAuthSrc := &fakeSource{projectID: "test-proj", clientAuth: true}
		validParams := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "Updated Agent"}},
			{Name: "update_mask", Value: "displayName"},
		}
		if _, tbErr := baseTool.Invoke(ctx, clientAuthSrc, validParams, ""); tbErr == nil || !strings.Contains(tbErr.Error(), "no token was provided") {
			t.Fatalf("expected client OAuth missing token error, got: %v", tbErr)
		}
	})
}
