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

package looker_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	toolboxlog "github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/looker"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
)

func TestParseFromYamlLooker(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-looker-instance
			type: looker
			base_url: http://example.looker.com/
			client_id: jasdl;k;tjl
			client_secret: sdakl;jgflkasdfkfg
			`,
			want: map[string]sources.SourceConfig{
				"my-looker-instance": looker.Config{
					Name:               "my-looker-instance",
					Type:               looker.SourceType,
					BaseURL:            "http://example.looker.com/",
					ClientId:           "jasdl;k;tjl",
					ClientSecret:       "sdakl;jgflkasdfkfg",
					Timeout:            "600s",
					SslVerification:    true,
					UseClientOAuth:     "false",
					ShowHiddenModels:   true,
					ShowHiddenExplores: true,
					ShowHiddenFields:   true,
					Location:           "us",
					SessionLength:      1200,
				},
			},
		},
		{
			desc: "with quota project",
			in: `
			kind: source
			name: my-looker-instance
			type: looker
			base_url: http://example.looker.com/
			client_id: jasdl;k;tjl
			client_secret: sdakl;jgflkasdfkfg
			quotaProject: billing-project
			`,
			want: map[string]sources.SourceConfig{
				"my-looker-instance": looker.Config{
					Name:               "my-looker-instance",
					Type:               looker.SourceType,
					BaseURL:            "http://example.looker.com/",
					ClientId:           "jasdl;k;tjl",
					ClientSecret:       "sdakl;jgflkasdfkfg",
					Timeout:            "600s",
					SslVerification:    true,
					UseClientOAuth:     "false",
					ShowHiddenModels:   true,
					ShowHiddenExplores: true,
					ShowHiddenFields:   true,
					Location:           "us",
					QuotaProject:       "billing-project",
					SessionLength:      1200,
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse: want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestFailParseFromYaml(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-looker-instance
			type: looker
			base_url: http://example.looker.com/
			client_id: jasdl;k;tjl
			client_secret: sdakl;jgflkasdfkfg
			schema: test-schema
			`,
			err: "error unmarshaling source: unable to parse source \"my-looker-instance\" as \"looker\": [5:1] unknown field \"schema\"\n   2 | client_id: jasdl;k;tjl\n   3 | client_secret: sdakl;jgflkasdfkfg\n   4 | name: my-looker-instance\n>  5 | schema: test-schema\n       ^\n   6 | type: looker",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-looker-instance
			type: looker
			client_id: jasdl;k;tjl
			`,
			err: "error unmarshaling source: unable to parse source \"my-looker-instance\" as \"looker\": Key: 'Config.BaseURL' Error:Field validation for 'BaseURL' failed on the 'required' tag",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if errStr != tc.err {
				t.Fatalf("unexpected error: got %q, want %q", errStr, tc.err)
			}
		})
	}
}

func TestGetLookerSDK_ClientIPPropagation(t *testing.T) {
	// 1. Start a local test server
	serverReceivedHeaders := make(http.Header)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range r.Header {
			serverReceivedHeaders[k] = v
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"id": 123}`)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer ts.Close()

	// 2. Construct looker Config with UseClientOAuth = "true" pointing to the local test server
	cfg := looker.Config{
		Name:            "test-looker",
		Type:            "looker",
		BaseURL:         ts.URL,
		UseClientOAuth:  "true",
		Timeout:         "5s",
		SslVerification: false,
	}

	// 3. Initialize the source
	ctx := context.Background()
	// Inject a logger so Initialize doesn't fail
	logger, err := toolboxlog.NewStdLogger(io.Discard, io.Discard, "DEBUG")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	ctx = util.WithLogger(ctx, logger)
	ctx = util.WithUserAgent(ctx, "test-agent")

	src, err := cfg.Initialize(ctx, nil)
	if err != nil {
		t.Fatalf("failed to initialize source: %v", err)
	}

	lookerSrc, ok := src.(*looker.Source)
	if !ok {
		t.Fatalf("source is not of type *looker.Source")
	}

	// 4. Inject Client IP into the context
	testIP := "203.0.113.195"
	ctxWithIP := util.WithClientIP(ctx, testIP)

	// 5. Retrieve the Looker SDK using GetLookerSDK
	sdk, err := lookerSrc.GetLookerSDK(ctxWithIP, "mock-token-123")
	if err != nil {
		t.Fatalf("GetLookerSDK failed: %v", err)
	}

	// 6. Retrieve session and request a call using the session client
	authSession, ok := sdk.AuthSession.(*rtl.AuthSession)
	if !ok {
		t.Fatalf("SDK session is not *rtl.AuthSession")
	}

	client := authSession.Client
	req, err := http.NewRequestWithContext(ctxWithIP, "GET", ts.URL+"/api/4.0/user", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// 7. Assert headers are correctly propagated to the test server
	if gotIP := serverReceivedHeaders.Get("X-Forwarded-For"); gotIP != testIP {
		t.Errorf("expected X-Forwarded-For to be %q, got %q", testIP, gotIP)
	}
	if gotIP := serverReceivedHeaders.Get("X-Real-IP"); gotIP != testIP {
		t.Errorf("expected X-Real-IP to be %q, got %q", testIP, gotIP)
	}
	if gotAuth := serverReceivedHeaders.Get("Authorization"); gotAuth != "mock-token-123" {
		t.Errorf("expected Authorization header to be %q, got %q", "mock-token-123", gotAuth)
	}
}

func TestGetHostURL(t *testing.T) {
	// 1. Setup mock server that handles /api/4.0/versions
	var requestCount int
	var responseBody []byte
	var responseStatus int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/4.0/versions") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		requestCount++
		w.WriteHeader(responseStatus)
		if _, err := w.Write(responseBody); err != nil {
			t.Errorf("failed to write mock response: %v", err)
		}
	}))
	defer ts.Close()

	// 2. Configure Looker config pointing to mock server
	cfg := looker.Config{
		Name:            "test-looker",
		Type:            "looker",
		BaseURL:         ts.URL,
		Timeout:         "5s",
		SslVerification: false,
	}

	ctx := context.Background()
	logger, _ := toolboxlog.NewStdLogger(io.Discard, io.Discard, "DEBUG")
	ctx = util.WithLogger(ctx, logger)
	ctx = util.WithUserAgent(ctx, "test-agent")

	srcVal, err := cfg.Initialize(ctx, nil)
	if err != nil {
		t.Fatalf("failed to initialize source: %v", err)
	}
	src := srcVal.(*looker.Source)

	sdk, err := src.GetLookerSDK(ctx, "mock-token-123")
	if err != nil {
		t.Fatalf("failed to get sdk: %v", err)
	}

	// Scenario 1: Success Path & Success Cache TTL
	responseStatus = http.StatusOK
	responseBody = []byte(`{"web_server_url": "https://public.customdomain.com"}`)
	requestCount = 0

	// First call - should perform request
	url1, err := src.GetHostURL(ctx, sdk)
	if err != nil {
		t.Fatalf("GetHostURL failed: %v", err)
	}
	if url1 != "https://public.customdomain.com" {
		t.Errorf("expected URL to be %q, got %q", "https://public.customdomain.com", url1)
	}
	if requestCount != 1 {
		t.Errorf("expected exactly 1 request to mock server, got %d", requestCount)
	}

	// Second call - should hit success cache, no extra request
	url2, err := src.GetHostURL(ctx, sdk)
	if err != nil {
		t.Fatalf("GetHostURL failed: %v", err)
	}
	if url2 != "https://public.customdomain.com" {
		t.Errorf("expected URL to be %q, got %q", "https://public.customdomain.com", url2)
	}
	if requestCount != 1 {
		t.Errorf("expected request count to remain 1 (cached), got %d", requestCount)
	}

	// Scenario 2: Failure Path & Fallback TTL
	// Reinitialize Source to clear the success cache
	srcVal, _ = cfg.Initialize(ctx, nil)
	src = srcVal.(*looker.Source)
	sdk, _ = src.GetLookerSDK(ctx, "mock-token-123")

	responseStatus = http.StatusInternalServerError
	responseBody = []byte(`Internal Server Error`)
	requestCount = 0

	// First failure call - should hit server and fallback to BaseUrl, returning error
	expectedFallback := strings.TrimSuffix(ts.URL, "/")
	urlFail1, err := src.GetHostURL(ctx, sdk)
	if err == nil {
		t.Fatal("expected GetHostURL to return an error on failure")
	}
	if urlFail1 != expectedFallback {
		t.Errorf("expected fallback URL to be %q, got %q", expectedFallback, urlFail1)
	}
	if requestCount != 1 {
		t.Errorf("expected exactly 1 request for failed fetch, got %d", requestCount)
	}

	// Second call within 1 minute - should immediately return fallback without hitting server, returning cached error
	urlFail2, err := src.GetHostURL(ctx, sdk)
	if err == nil {
		t.Fatal("expected GetHostURL to return an error on cached failure")
	}
	if urlFail2 != expectedFallback {
		t.Errorf("expected fallback URL to be %q, got %q", expectedFallback, urlFail2)
	}
	if requestCount != 1 {
		t.Errorf("expected request count to remain 1 (cached failure), got %d", requestCount)
	}
}

func TestGetHostURL_Concurrent(t *testing.T) {
	// Setup mock server
	var requestCount int
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		// simulate slow API latency
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"web_server_url": "https://public-concurrent.looker.com"}`))
	}))
	defer ts.Close()

	cfg := looker.Config{
		Name:            "test-looker-concurrent",
		Type:            "looker",
		BaseURL:         ts.URL,
		Timeout:         "5s",
		SslVerification: false,
	}

	ctx := context.Background()
	logger, _ := toolboxlog.NewStdLogger(io.Discard, io.Discard, "DEBUG")
	ctx = util.WithLogger(ctx, logger)
	ctx = util.WithUserAgent(ctx, "test-agent")

	srcVal, _ := cfg.Initialize(ctx, nil)
	src := srcVal.(*looker.Source)
	sdk, _ := src.GetLookerSDK(ctx, "mock-token-123")

	// Spawn 50 goroutines to concurrently call GetHostURL
	concurrency := 50
	errChan := make(chan error, concurrency)
	urlChan := make(chan string, concurrency)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resolved, err := src.GetHostURL(ctx, sdk)
			if err != nil {
				errChan <- err
				return
			}
			urlChan <- resolved
		}()
	}

	wg.Wait()
	close(errChan)
	close(urlChan)

	for err := range errChan {
		t.Fatalf("concurrent GetHostURL failed: %v", err)
	}

	for resolved := range urlChan {
		if resolved != "https://public-concurrent.looker.com" {
			t.Errorf("expected resolved URL to be %q, got %q", "https://public-concurrent.looker.com", resolved)
		}
	}

	// Verify singleflight deduplicated the requests to exactly 1
	mu.Lock()
	finalCount := requestCount
	mu.Unlock()
	if finalCount != 1 {
		t.Errorf("expected exactly 1 network request due to singleflight deduplication, got %d", finalCount)
	}
}
