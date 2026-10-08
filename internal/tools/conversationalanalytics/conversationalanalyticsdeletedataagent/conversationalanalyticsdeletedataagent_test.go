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

package conversationalanalyticsdeletedataagent

import (
	"context"
	"encoding/json"
	"errors"
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

func TestParseFromYamlConversationalAnalyticsDeleteDataAgent(t *testing.T) {
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
            type: conversational-analytics-delete-data-agent
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
					Type:   "conversational-analytics-delete-data-agent",
					Source: "my-instance",
				},
			},
		},
		{
			desc: "advanced example with location",
			in: `
            kind: tool
            name: example_tool
            type: conversational-analytics-delete-data-agent
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
					Type:     "conversational-analytics-delete-data-agent",
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
            type: conversational-analytics-delete-data-agent
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
					Type:        "conversational-analytics-delete-data-agent",
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

func TestDeleteDataAgentAnnotations(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		ConfigBase: tools.ConfigBase{Name: "delete_agent", Description: "delete agent"},
		Type:       toolType,
		Source:     "my-instance",
	}

	t.Run("defaults to destructive annotations and is suppressed on a read-only source", func(t *testing.T) {
		tl, err := cfg.Initialize(ctx)
		if err != nil {
			t.Fatalf("unexpected Initialize error: %v", err)
		}
		if diff := cmp.Diff(tools.NewDestructiveAnnotations(), tl.GetAnnotations(nil)); diff != "" {
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

func TestInvokeDeleteDataAgent(t *testing.T) {
	ctx := context.Background()
	src := &fakeSource{projectID: "test-proj"}

	cfg := Config{
		ConfigBase: tools.ConfigBase{
			Name:        "delete_data_agent",
			Description: "delete tool",
		},
		Type:   toolType,
		Source: "my-instance",
	}
	tl, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	baseTool := tl.(Tool)

	t.Run("success with LRO polling and path verification", func(t *testing.T) {
		var gotDeletePath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/dataAgents/my-agent") {
				gotDeletePath = r.URL.Path
				_ = json.NewEncoder(w).Encode(map[string]any{
					"name": "projects/test-proj/locations/global/operations/op-789",
					"done": false,
				})
				return
			}
			if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/operations/op-789") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"name": "projects/test-proj/locations/global/operations/op-789",
					"done": true,
					"response": map[string]any{
						"@type": "type.googleapis.com/google.protobuf.Empty",
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
		}
		got, tbErr := tool.Invoke(ctx, src, params, "")
		if tbErr != nil {
			t.Fatalf("unexpected error: %v", tbErr)
		}
		if gotDeletePath != "/v1/projects/test-proj/locations/global/dataAgents/my-agent" {
			t.Fatalf("unexpected DELETE path: %q", gotDeletePath)
		}
		gotMap, ok := got.(map[string]any)
		if !ok || gotMap["@type"] != "type.googleapis.com/google.protobuf.Empty" {
			t.Fatalf("unexpected response: %v", got)
		}
	})

	t.Run("empty 204 response body and synchronous done without name", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
		}
		got, tbErr := tool.Invoke(ctx, src, params, "")
		if tbErr != nil {
			t.Fatalf("unexpected error on empty 204 body: %v", tbErr)
		}
		gotMap, ok := got.(map[string]any)
		if !ok || gotMap["done"] != true {
			t.Fatalf("expected done: true map, got: %v", got)
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
					"@type": "type.googleapis.com/google.protobuf.Empty",
				},
			})
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
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
					"@type": "type.googleapis.com/google.protobuf.Empty",
				},
			})
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
		}
		if _, tbErr := tool.Invoke(ctx, &fakeSource{projectID: "a/b"}, params, ""); tbErr != nil {
			t.Fatalf("unexpected error: %v", tbErr)
		}
		// PathEscape must encode "/" so the project ID stays a single path segment.
		if want := "/v1/projects/a%2Fb/locations/global/dataAgents/my-agent"; gotEscapedPath != want {
			t.Fatalf("expected escaped request path %q, got %q", want, gotEscapedPath)
		}
	})

	t.Run("LRO error payload and polling timeout", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "projects/test-proj/locations/global/operations/op-del-err",
				"done": true,
				"error": map[string]any{
					"code":    403,
					"message": "permission denied",
				},
			})
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		params := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
		}
		_, tbErr := tool.Invoke(ctx, src, params, "")
		var agentErr *util.AgentError
		if !errors.As(tbErr, &agentErr) || !strings.Contains(tbErr.Error(), "permission denied") {
			t.Fatalf("expected AgentError with 'permission denied', got %T: %v", tbErr, tbErr)
		}
	})

	t.Run("invalid data_agent_id, client OAuth missing token, and API 404", func(t *testing.T) {
		badParams := parameters.ParamValues{
			{Name: "data_agent_id", Value: "../bad-id"},
		}
		if _, tbErr := baseTool.Invoke(ctx, src, badParams, ""); tbErr == nil || !strings.Contains(tbErr.Error(), "disallowed characters") {
			t.Fatalf("expected validation error for bad data_agent_id, got: %v", tbErr)
		}

		clientAuthSrc := &fakeSource{projectID: "test-proj", clientAuth: true}
		validParams := parameters.ParamValues{
			{Name: "data_agent_id", Value: "my-agent"},
		}
		if _, tbErr := baseTool.Invoke(ctx, clientAuthSrc, validParams, ""); tbErr == nil || !strings.Contains(tbErr.Error(), "no token was provided") {
			t.Fatalf("expected client OAuth missing token error, got: %v", tbErr)
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": "not found"}`))
		}))
		defer srv.Close()

		tool := baseTool
		tool.endpoint = srv.URL
		tool.httpClient = srv.Client()

		if _, tbErr := tool.Invoke(ctx, src, validParams, ""); tbErr == nil || !strings.Contains(tbErr.Error(), "404") {
			t.Fatalf("expected 404 error, got: %v", tbErr)
		}
	})
}
