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

package conversationalanalyticscreatedataagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestParseFromYamlConversationalAnalyticsCreateDataAgent(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	readOnlyFalse, destructiveFalse := false, false
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
            type: conversational-analytics-create-data-agent
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
					Type:   "conversational-analytics-create-data-agent",
					Source: "my-instance",
				},
			},
		},
		{
			desc: "advanced example with location",
			in: `
            kind: tool
            name: example_tool
            type: conversational-analytics-create-data-agent
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
					Type:     "conversational-analytics-create-data-agent",
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
            type: conversational-analytics-create-data-agent
            source: my-instance
            description: some description
            annotations:
                readOnlyHint: false
                destructiveHint: false
            `,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:        "conversational-analytics-create-data-agent",
					Source:      "my-instance",
					Annotations: &tools.ToolAnnotations{ReadOnlyHint: &readOnlyFalse, DestructiveHint: &destructiveFalse},
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

	t.Run("initialize defaults location to global and rejects invalid location", func(t *testing.T) {
		cfg := Config{
			ConfigBase: tools.ConfigBase{Name: "create_agent", Description: "create agent"},
			Type:       toolType,
			Source:     "my-instance",
		}
		tl, err := cfg.Initialize(ctx)
		if err != nil {
			t.Fatalf("unexpected Initialize error: %v", err)
		}
		if gotCfg := tl.ToConfig().(Config); gotCfg.Location != "global" {
			t.Fatalf("expected default location 'global', got %q", gotCfg.Location)
		}

		badCfg := cfg
		badCfg.Location = "../bad-location"
		if _, err := badCfg.Initialize(ctx); err == nil {
			t.Fatalf("expected error for invalid location, got nil")
		}
	})
}

func TestCreateDataAgentAnnotations(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		ConfigBase: tools.ConfigBase{Name: "create_agent", Description: "create agent"},
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
		readOnly, destructive := false, false
		overrideCfg := cfg
		overrideCfg.Annotations = &tools.ToolAnnotations{ReadOnlyHint: &readOnly, DestructiveHint: &destructive}
		tl, err := overrideCfg.Initialize(ctx)
		if err != nil {
			t.Fatalf("unexpected Initialize error: %v", err)
		}
		if diff := cmp.Diff(overrideCfg.Annotations, tl.GetAnnotations(nil)); diff != "" {
			t.Fatalf("unexpected configured annotations (-want +got):\n%s", diff)
		}
	})
}

func TestInvokeCreateDataAgent(t *testing.T) {
	ctx := context.Background()
	src := &fakeSource{projectID: "test-proj"}

	cfg := Config{
		ConfigBase: tools.ConfigBase{
			Name:        "create_data_agent",
			Description: "create tool",
		},
		Type:   toolType,
		Source: "my-instance",
	}
	tl, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	baseTool := tl.(Tool)

	t.Run("success with LRO polling and request verification", func(t *testing.T) {
		var pollCount int32
		var gotQueryID string
		var gotBody map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/dataAgents") {
				gotQueryID = r.URL.Query().Get("dataAgentId")
				raw, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(raw, &gotBody)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"name": "projects/test-proj/locations/global/operations/op-123",
					"done": false,
				})
				return
			}
			if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/operations/op-123") {
				count := atomic.AddInt32(&pollCount, 1)
				if count == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"error": "transient"}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"name": "projects/test-proj/locations/global/operations/op-123",
					"done": true,
					"response": map[string]any{
						"name":        "projects/test-proj/locations/global/dataAgents/my-agent",
						"displayName": "My Agent",
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
			{Name: "agent_config", Value: map[string]any{"displayName": "My Agent"}},
		}
		got, tbErr := tool.Invoke(ctx, src, params, "")
		if tbErr != nil {
			t.Fatalf("unexpected error: %v", tbErr)
		}
		if gotQueryID != "my-agent" {
			t.Fatalf("expected dataAgentId query param 'my-agent', got %q", gotQueryID)
		}
		if gotBody["displayName"] != "My Agent" {
			t.Fatalf("expected request body displayName 'My Agent', got %v", gotBody)
		}
		gotMap, ok := got.(map[string]any)
		if !ok || gotMap["displayName"] != "My Agent" {
			t.Fatalf("unexpected response: %v", got)
		}
	})

	t.Run("synchronous done on initial response without polling", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"done": true,
				"response": map[string]any{
					"name":        "projects/test-proj/locations/global/dataAgents/fast-agent",
					"displayName": "Fast Agent",
				},
			})
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "fast-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "Fast Agent"}},
		}
		got, tbErr := tool.Invoke(ctx, src, params, "")
		if tbErr != nil {
			t.Fatalf("unexpected error: %v", tbErr)
		}
		gotMap, ok := got.(map[string]any)
		if !ok || gotMap["displayName"] != "Fast Agent" {
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
					"displayName": "My Agent",
				},
			})
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "My Agent"}},
		}
		if _, tbErr := tool.Invoke(ctx, &fakeSource{projectID: "google.com:test-proj"}, params, ""); tbErr != nil {
			t.Fatalf("unexpected error for domain-scoped project: %v", tbErr)
		}
		if want := "/v1/projects/google.com:test-proj/locations/global/dataAgents"; gotPath != want {
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
					"displayName": "My Agent",
				},
			})
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "My Agent"}},
		}
		if _, tbErr := tool.Invoke(ctx, &fakeSource{projectID: "a/b"}, params, ""); tbErr != nil {
			t.Fatalf("unexpected error: %v", tbErr)
		}
		// PathEscape must encode "/" so the project ID stays a single path segment.
		if want := "/v1/projects/a%2Fb/locations/global/dataAgents"; gotEscapedPath != want {
			t.Fatalf("expected escaped request path %q, got %q", want, gotEscapedPath)
		}
	})

	t.Run("LRO done with error payload returns AgentError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "projects/test-proj/locations/global/operations/op-err",
				"done": true,
				"error": map[string]any{
					"code":    400,
					"message": "invalid table reference",
				},
			})
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "My Agent"}},
		}
		_, tbErr := tool.Invoke(ctx, src, params, "")
		var agentErr *util.AgentError
		if !errors.As(tbErr, &agentErr) || !strings.Contains(tbErr.Error(), "invalid table reference") {
			t.Fatalf("expected AgentError with 'invalid table reference', got %T: %v", tbErr, tbErr)
		}
	})

	t.Run("non-retryable poll HTTP error 404 aborts immediately", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodPost {
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
			{Name: "agent_config", Value: map[string]any{"displayName": "My Agent"}},
		}
		_, tbErr := tool.Invoke(ctx, src, params, "")
		if tbErr == nil || !strings.Contains(tbErr.Error(), "polling failed with 404") {
			t.Fatalf("expected immediate 404 poll error, got: %v", tbErr)
		}
	})

	t.Run("polling timeout returns AgentError with do-not-retry guidance", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "projects/test-proj/locations/global/operations/op-slow",
				"done": false,
			})
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()
		tool.pollInterval = 5 * time.Millisecond
		tool.pollTimeout = 20 * time.Millisecond

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "My Agent"}},
		}
		_, tbErr := tool.Invoke(ctx, src, params, "")
		if tbErr == nil || !strings.Contains(tbErr.Error(), "Do not retry the operation") {
			t.Fatalf("expected timeout error with do-not-retry guidance, got: %v", tbErr)
		}
	})

	t.Run("unfinished operation without /operations/ name returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "projects/test-proj/locations/global/dataAgents/my-agent",
				"done": false,
			})
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "My Agent"}},
		}
		_, tbErr := tool.Invoke(ctx, src, params, "")
		if tbErr == nil || !strings.Contains(tbErr.Error(), "no pollable '/operations/' name") {
			t.Fatalf("expected non-pollable operation error, got: %v", tbErr)
		}
	})

	t.Run("invalid data_agent_id and empty agent_config and client OAuth missing token", func(t *testing.T) {
		badParams := parameters.ParamValues{
			{Name: "data_agent_id", Value: "../bad-id"},
			{Name: "agent_config", Value: map[string]any{"displayName": "My Agent"}},
		}
		if _, tbErr := baseTool.Invoke(ctx, src, badParams, ""); tbErr == nil || !strings.Contains(tbErr.Error(), "disallowed characters") {
			t.Fatalf("expected validation error for bad data_agent_id, got: %v", tbErr)
		}

		emptyConfigParams := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{}},
		}
		if _, tbErr := baseTool.Invoke(ctx, src, emptyConfigParams, ""); tbErr == nil || !strings.Contains(tbErr.Error(), "non-empty JSON object") {
			t.Fatalf("expected validation error for empty agent_config, got: %v", tbErr)
		}

		clientAuthSrc := &fakeSource{projectID: "test-proj", clientAuth: true}
		validParams := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "My Agent"}},
		}
		if _, tbErr := baseTool.Invoke(ctx, clientAuthSrc, validParams, ""); tbErr == nil || !strings.Contains(tbErr.Error(), "no token was provided") {
			t.Fatalf("expected client OAuth missing token error, got: %v", tbErr)
		}
	})

	t.Run("API HTTP error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error": "already exists"}`))
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
			{Name: "agent_config", Value: map[string]any{"displayName": "My Agent"}},
		}
		_, tbErr := tool.Invoke(ctx, src, params, "")
		if tbErr == nil || !strings.Contains(tbErr.Error(), "409") {
			t.Fatalf("expected 409 error, got: %v", tbErr)
		}
	})
}
