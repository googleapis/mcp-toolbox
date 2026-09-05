// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package http

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// fakeSource is a minimal compatibleSource used to observe the request Invoke
// builds without making a real network call.
type fakeSource struct {
	baseURL     string
	lastRequest *http.Request
}

func (f *fakeSource) SourceType() string             { return "http" }
func (f *fakeSource) ToConfig() sources.SourceConfig { return nil }
func (f *fakeSource) IsReadOnly() bool               { return false }

func (f *fakeSource) HttpDefaultHeaders() map[string]string { return nil }
func (f *fakeSource) HttpBaseURL() string                   { return f.baseURL }
func (f *fakeSource) HttpQueryParams() map[string]string    { return nil }
func (f *fakeSource) RunRequest(_ context.Context, req *http.Request) (any, error) {
	f.lastRequest = req
	return map[string]any{"ok": true}, nil
}

// fakeTokenSource returns a fixed token, or an error when err is non-nil.
type fakeTokenSource struct {
	token *oauth2.Token
	err   error
}

func (f *fakeTokenSource) Token() (*oauth2.Token, error) {
	return f.token, f.err
}

func newConfigTool(t *testing.T, cfg Config) Tool {
	t.Helper()
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("unexpected error initializing tool: %s", err)
	}
	httpTool, ok := tool.(Tool)
	if !ok {
		t.Fatalf("Initialize did not return an http.Tool")
	}
	return httpTool
}

func TestInitializeFetchesCredentialsWhenSendGoogleAccessTokenIsSet(t *testing.T) {
	origFindDefaultCredentials := findDefaultCredentials
	t.Cleanup(func() { findDefaultCredentials = origFindDefaultCredentials })

	var gotScopes []string
	wantTokenSource := &fakeTokenSource{token: &oauth2.Token{AccessToken: "abc"}}
	findDefaultCredentials = func(_ context.Context, scopes ...string) (*google.Credentials, error) {
		gotScopes = scopes
		return &google.Credentials{TokenSource: wantTokenSource}, nil
	}

	cfg := Config{
		ConfigBase:            tools.ConfigBase{Name: "t", Description: "d"},
		Type:                  resourceType,
		Source:                "s",
		Path:                  "/p",
		Method:                "GET",
		SendGoogleAccessToken: true,
	}
	httpTool := newConfigTool(t, cfg)

	if httpTool.TokenSource != wantTokenSource {
		t.Fatalf("Initialize did not wire the credentials' TokenSource onto the tool")
	}
	if len(gotScopes) != 1 || gotScopes[0] != sources.CloudPlatformScope {
		t.Fatalf("unexpected scopes requested: %v", gotScopes)
	}
}

func TestInitializeDecouplesCredentialLookupFromStartupContext(t *testing.T) {
	origFindDefaultCredentials := findDefaultCredentials
	t.Cleanup(func() { findDefaultCredentials = origFindDefaultCredentials })

	var gotCtx context.Context
	findDefaultCredentials = func(ctx context.Context, _ ...string) (*google.Credentials, error) {
		gotCtx = ctx
		return &google.Credentials{TokenSource: &fakeTokenSource{token: &oauth2.Token{AccessToken: "abc"}}}, nil
	}

	cfg := Config{
		ConfigBase:            tools.ConfigBase{Name: "t", Description: "d"},
		Type:                  resourceType,
		Source:                "s",
		Path:                  "/p",
		Method:                "GET",
		SendGoogleAccessToken: true,
	}

	// A startup context that is cancelled right after Initialize returns,
	// modeling the real lifecycle: Initialize only runs during server
	// startup, and that context is not guaranteed to outlive it.
	startupCtx, cancel := context.WithCancel(context.Background())
	if _, err := cfg.Initialize(startupCtx); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	cancel()

	if gotCtx == nil {
		t.Fatalf("findDefaultCredentials was never called")
	}
	if err := gotCtx.Err(); err != nil {
		t.Fatalf("context passed to findDefaultCredentials was cancelled along with the startup context: %s", err)
	}
}

func TestInitializeFailsWhenCredentialsUnavailable(t *testing.T) {
	origFindDefaultCredentials := findDefaultCredentials
	t.Cleanup(func() { findDefaultCredentials = origFindDefaultCredentials })

	findDefaultCredentials = func(context.Context, ...string) (*google.Credentials, error) {
		return nil, errors.New("no credentials found")
	}

	cfg := Config{
		ConfigBase:            tools.ConfigBase{Name: "t", Description: "d"},
		Type:                  resourceType,
		Source:                "s",
		Path:                  "/p",
		Method:                "GET",
		SendGoogleAccessToken: true,
	}
	if _, err := cfg.Initialize(context.Background()); err == nil {
		t.Fatalf("expected Initialize to fail when no default credentials are available")
	}
}

func TestInitializeSkipsCredentialLookupByDefault(t *testing.T) {
	origFindDefaultCredentials := findDefaultCredentials
	t.Cleanup(func() { findDefaultCredentials = origFindDefaultCredentials })

	findDefaultCredentials = func(context.Context, ...string) (*google.Credentials, error) {
		t.Fatalf("findDefaultCredentials must not be called when sendGoogleAccessToken is unset")
		return nil, nil
	}

	cfg := Config{
		ConfigBase: tools.ConfigBase{Name: "t", Description: "d"},
		Type:       resourceType,
		Source:     "s",
		Path:       "/p",
		Method:     "GET",
	}
	httpTool := newConfigTool(t, cfg)

	if httpTool.TokenSource != nil {
		t.Fatalf("TokenSource must be nil when sendGoogleAccessToken is unset")
	}
}

func TestInvokeSendsGoogleAccessTokenAsAuthorizationHeader(t *testing.T) {
	source := &fakeSource{baseURL: "https://api.example.com"}
	httpTool := Tool{
		BaseTool: tools.NewBaseTool(
			Config{
				ConfigBase:            tools.ConfigBase{Name: "t", Description: "d"},
				Type:                  resourceType,
				Source:                "s",
				Path:                  "/p",
				Method:                "GET",
				SendGoogleAccessToken: true,
				// A static Authorization header set alongside sendGoogleAccessToken
				// must not win: the fetched token always overrides it, since a
				// static value would otherwise go stale.
				Headers: map[string]string{"Authorization": "static-should-be-overridden"},
			},
			tools.NewDestructiveAnnotations(),
			tools.Manifest{},
			nil,
		),
		TokenSource: &fakeTokenSource{token: &oauth2.Token{AccessToken: "the-token"}},
	}

	_, toolErr := httpTool.Invoke(context.Background(), source, parameters.ParamValues{}, "")
	if toolErr != nil {
		t.Fatalf("unexpected error: %s", toolErr)
	}

	if source.lastRequest == nil {
		t.Fatalf("request was never sent to the source")
	}
	got := source.lastRequest.Header.Get("Authorization")
	want := "Bearer the-token"
	if got != want {
		t.Fatalf("Authorization header = %q, want %q", got, want)
	}
}

func TestInvokeSurfacesTokenFetchError(t *testing.T) {
	source := &fakeSource{baseURL: "https://api.example.com"}
	httpTool := Tool{
		BaseTool: tools.NewBaseTool(
			Config{
				ConfigBase:            tools.ConfigBase{Name: "t", Description: "d"},
				Type:                  resourceType,
				Source:                "s",
				Path:                  "/p",
				Method:                "GET",
				SendGoogleAccessToken: true,
			},
			tools.NewDestructiveAnnotations(),
			tools.Manifest{},
			nil,
		),
		TokenSource: &fakeTokenSource{err: errors.New("token endpoint unreachable")},
	}

	_, toolErr := httpTool.Invoke(context.Background(), source, parameters.ParamValues{}, "")
	if toolErr == nil {
		t.Fatalf("expected an error when the token source fails")
	}
	if source.lastRequest != nil {
		t.Fatalf("request must not be sent when the token could not be fetched")
	}
}

func TestInvokeDoesNotSetAuthorizationWhenTokenNotRequested(t *testing.T) {
	source := &fakeSource{baseURL: "https://api.example.com"}
	httpTool := Tool{
		BaseTool: tools.NewBaseTool(
			Config{
				ConfigBase: tools.ConfigBase{Name: "t", Description: "d"},
				Type:       resourceType,
				Source:     "s",
				Path:       "/p",
				Method:     "GET",
			},
			tools.NewDestructiveAnnotations(),
			tools.Manifest{},
			nil,
		),
	}

	_, toolErr := httpTool.Invoke(context.Background(), source, parameters.ParamValues{}, "")
	if toolErr != nil {
		t.Fatalf("unexpected error: %s", toolErr)
	}
	if got := source.lastRequest.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization header = %q, want empty", got)
	}
}
