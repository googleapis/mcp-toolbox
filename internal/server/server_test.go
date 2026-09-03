// Copyright 2024 Google LLC
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

package server_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/auth"
	"github.com/googleapis/mcp-toolbox/internal/auth/generic"
	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels"
	_ "github.com/googleapis/mcp-toolbox/internal/embeddingmodels/gemini"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	_ "github.com/googleapis/mcp-toolbox/internal/prompts/custom"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	_ "github.com/googleapis/mcp-toolbox/internal/resources/file"
	_ "github.com/googleapis/mcp-toolbox/internal/resources/text"
	"github.com/googleapis/mcp-toolbox/internal/server"
	v20260728 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20260728"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/alloydbpg"
	_ "github.com/googleapis/mcp-toolbox/internal/sources/postgres"
	_ "github.com/googleapis/mcp-toolbox/internal/sources/sqlite"
	"github.com/googleapis/mcp-toolbox/internal/telemetry"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/http"
	"github.com/googleapis/mcp-toolbox/internal/tools/mysql/mysqlexecutesql"
	"github.com/googleapis/mcp-toolbox/internal/tools/postgres/postgresexecutesql"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

// Helper function to create temporary self-signed certs for the test
func generateTestCerts(t *testing.T) (string, string, func()) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"Test Co"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	// Create temp files
	certFile, err := os.CreateTemp("", "cert.*.pem")
	if err != nil {
		t.Fatalf("failed to create temp cert file: %v", err)
	}

	keyFile, err := os.CreateTemp("", "key.*.pem")
	if err != nil {
		t.Fatalf("failed to create temp key file: %v", err)
	}

	// Check the error return values for pem.Encode
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		t.Fatalf("failed to encode certificate: %v", err)
	}

	if err := pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		t.Fatalf("failed to encode key: %v", err)
	}

	certFile.Close()
	keyFile.Close()

	cleanup := func() {
		os.Remove(certFile.Name())
		os.Remove(keyFile.Name())
	}

	return certFile.Name(), keyFile.Name(), cleanup
}

func TestServe(t *testing.T) {
	certFile, keyFile, cleanupCerts := generateTestCerts(t)
	defer cleanupCerts()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	otelShutdown, err := telemetry.SetupOTel(ctx, "0.0.0", "", false, "", "toolbox")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	defer func() {
		err := otelShutdown(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
	}()

	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	tests := []struct {
		name string
		cert string
		key  string
		addr string
		port int
	}{
		{
			name: "HTTP mode",
			addr: "127.0.0.1",
			port: 5001,
		},
		{
			name: "HTTPS mode",
			cert: certFile,
			key:  keyFile,
			addr: "127.0.0.1",
			port: 5002,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := server.ServerConfig{
				Version:      "0.0.0",
				Address:      tt.addr,
				Port:         tt.port,
				AllowedHosts: []string{"*"},
			}

			instrumentation, err := telemetry.CreateTelemetryInstrumentation(cfg.Version)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			ctx = util.WithInstrumentation(ctx, instrumentation)

			s, err := server.NewServer(ctx, cfg)
			if err != nil {
				t.Fatalf("unable to initialize server: %v", err)
			}

			err = s.Listen(ctx, tt.cert, tt.key)
			if err != nil {
				t.Fatalf("unable to start server: %v", err)
			}

			// start server in background
			go func() {
				if err := s.Serve(ctx); err != nil && err != http.ErrServerClosed {
					t.Errorf("server serve error: %v", err)
				}
			}()

			// Setup Client to handle self-signed certs
			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
			}

			useTLS := tt.cert != "" || tt.key != ""
			protocol := "http"
			if useTLS {
				protocol = "https"
			}

			url := fmt.Sprintf("%s://%s:%d/", protocol, tt.addr, tt.port)
			resp, err := client.Get(url)
			if err != nil {
				t.Fatalf("error when sending a request: %s", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("response status code is not 200")
			}
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("error reading from request body: %s", err)
			}
			if got := string(raw); strings.Contains(got, "0.0.0") {
				t.Fatalf("version missing from output: %q", got)
			}
		})
	}

}

func TestHealthz(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, port := "127.0.0.1", 0
	cfg := server.ServerConfig{
		Version:      "0.0.0",
		Address:      addr,
		Port:         port,
		AllowedHosts: []string{"*"},
	}

	otelShutdown, err := telemetry.SetupOTel(ctx, "0.0.0", "", false, "", "toolbox")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	defer func() {
		err := otelShutdown(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
	}()

	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	instrumentation, err := telemetry.CreateTelemetryInstrumentation(cfg.Version)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	s, err := server.NewServer(ctx, cfg)
	if err != nil {
		t.Fatalf("unable to initialize server: %v", err)
	}

	err = s.Listen(ctx, "", "")
	if err != nil {
		t.Fatalf("unable to start server: %v", err)
	}

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		if serveErr := s.Serve(ctx); serveErr != nil {
			errCh <- serveErr
		}
	}()

	url := fmt.Sprintf("http://%s/healthz", s.Addr())
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("error when sending a request: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading from request body: %s", err)
	}

	var body map[string]string
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("expected JSON body, got %q: %s", string(raw), err)
	}
	if body["status"] != "ok" {
		t.Fatalf(`expected {"status":"ok"}, got %q`, string(raw))
	}
}

// TestHealthzBypassesHostCheck verifies that /healthz is reachable even when
// AllowedHosts does not include the request host. Container probes (Kubernetes,
// Docker, Cloud Run) commonly hit the endpoint via the pod IP or localhost,
// so the strict host validation must not block them.
func TestHealthzBypassesHostCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr, port := "127.0.0.1", 0
	cfg := server.ServerConfig{
		Version:      "0.0.0",
		Address:      addr,
		Port:         port,
		AllowedHosts: []string{"toolbox.example.com"},
	}

	otelShutdown, err := telemetry.SetupOTel(ctx, "0.0.0", "", false, "", "toolbox")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	defer func() {
		err := otelShutdown(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
	}()

	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	instrumentation, err := telemetry.CreateTelemetryInstrumentation(cfg.Version)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	s, err := server.NewServer(ctx, cfg)
	if err != nil {
		t.Fatalf("unable to initialize server: %v", err)
	}

	err = s.Listen(ctx, "", "")
	if err != nil {
		t.Fatalf("unable to start server: %v", err)
	}

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		if serveErr := s.Serve(ctx); serveErr != nil {
			errCh <- serveErr
		}
	}()

	// Hit /healthz via the pod IP (127.0.0.1), which is not in AllowedHosts.
	url := fmt.Sprintf("http://%s/healthz", s.Addr())
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("error when sending a request: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected /healthz to bypass host check and return 200, got %d", resp.StatusCode)
	}

	// Sanity check: confirm the host check is still active for other paths.
	rootURL := fmt.Sprintf("http://%s/", s.Addr())
	rootResp, err := http.Get(rootURL)
	if err != nil {
		t.Fatalf("error when sending root request: %s", err)
	}
	defer rootResp.Body.Close()
	if rootResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected / to be blocked by host check (403), got %d", rootResp.StatusCode)
	}
}

func TestUpdateServer(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}

	addr, port := "127.0.0.1", 5000
	cfg := server.ServerConfig{
		Version: "0.0.0",
		Address: addr,
		Port:    port,
	}

	instrumentation, err := telemetry.CreateTelemetryInstrumentation(cfg.Version)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	ctx = util.WithInstrumentation(ctx, instrumentation)

	s, err := server.NewServer(ctx, cfg)
	if err != nil {
		t.Fatalf("error setting up server: %s", err)
	}

	newSources := map[string]sources.Source{
		"example-source": &alloydbpg.Source{
			Config: alloydbpg.Config{
				Name: "example-alloydb-source",
				Type: "alloydb-postgres",
			},
		},
	}
	newAuth := map[string]auth.AuthService{"example-auth": nil}
	newEmbeddingModels := map[string]embeddingmodels.EmbeddingModel{"example-model": nil}
	newTools := map[string]tools.Tool{"example-tool": nil}
	newPrompts := map[string]prompts.Prompt{"example-prompt": testutils.NewMockPrompt("example-prompt", "", prompts.Arguments{})}
	newGroups := map[string]group.Group{
		"example-toolset": group.NewGroup(group.GroupConfig{Name: "example-toolset", ToolNames: []string{"example-tool"}}),
	}
	newResources := map[string]resources.Resource{"example-resource": nil}
	newResourceTemplates := map[string]resources.ResourceTemplate{"example-template": nil}
	s.PrimitiveMgr.SetPrimitives(newSources, newAuth, newEmbeddingModels, newTools, newPrompts, newResources, newResourceTemplates, newGroups)
	if err != nil {
		t.Errorf("error updating server: %s", err)
	}

	gotSource, _ := s.PrimitiveMgr.GetSource("example-source")
	if diff := cmp.Diff(gotSource, newSources["example-source"]); diff != "" {
		t.Errorf("error updating server, sources (-want +got):\n%s", diff)
	}

	gotAuthService, _ := s.PrimitiveMgr.GetAuthService("example-auth")
	if diff := cmp.Diff(gotAuthService, newAuth["example-auth"]); diff != "" {
		t.Errorf("error updating server, authServices (-want +got):\n%s", diff)
	}

	gotTool, _ := s.PrimitiveMgr.GetTool("example-tool")
	if diff := cmp.Diff(gotTool, newTools["example-tool"]); diff != "" {
		t.Errorf("error updating server, tools (-want +got):\n%s", diff)
	}

	wantGroup := newGroups["example-toolset"]
	gotGroup, ok := s.PrimitiveMgr.GetGroup("example-toolset")
	if !ok {
		t.Fatal("expected group \"example-toolset\" to exist")
	}
	if diff := cmp.Diff(wantGroup, gotGroup, cmp.AllowUnexported(group.Group{})); diff != "" {
		t.Errorf("error updating server, group (-want +got):\n%s", diff)
	}

	gotPrompt, _ := s.PrimitiveMgr.GetPrompt("example-prompt")
	if diff := cmp.Diff(gotPrompt, newPrompts["example-prompt"], cmp.AllowUnexported(testutils.MockPrompt{})); diff != "" {
		t.Errorf("error updating server, prompts (-want +got):\n%s", diff)
	}

	gotResource, _ := s.PrimitiveMgr.GetResource("example-resource")
	if diff := cmp.Diff(gotResource, newResources["example-resource"]); diff != "" {
		t.Errorf("error updating server, resources (-want +got):\n%s", diff)
	}

	gotTemplate, _ := s.PrimitiveMgr.GetResourceTemplate("example-template")
	if diff := cmp.Diff(gotTemplate, newResourceTemplates["example-template"]); diff != "" {
		t.Errorf("error updating server, resource templates (-want +got):\n%s", diff)
	}
}

func TestEndpointSecurityAllowedOrigin(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}

	testCases := []struct {
		desc           string
		allowedOrigins []string
		origin         string
		corsBlocked    bool
	}{
		{
			desc:           "allowed origin all",
			allowedOrigins: []string{"*"},
			origin:         "https://evil.com",
		},
		{
			desc:           "allowed origin trusted with trusted origin",
			allowedOrigins: []string{"https://trusted.com"},
			origin:         "https://trusted.com",
		},
		{
			desc:           "allowed origin trusted with evil origin",
			allowedOrigins: []string{"https://trusted.com"},
			origin:         "https://evil.com",
			corsBlocked:    true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			addr, port := "127.0.0.1", 0
			cfg := server.ServerConfig{
				Version:        "0.0.0",
				Address:        addr,
				Port:           port,
				EnableAPI:      true,
				AllowedOrigins: tc.allowedOrigins,
				AllowedHosts:   []string{"*"},
			}

			instrumentation, err := telemetry.CreateTelemetryInstrumentation(cfg.Version)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			ctx = util.WithInstrumentation(ctx, instrumentation)

			s, err := server.NewServer(ctx, cfg)
			if err != nil {
				t.Fatalf("error setting up server: %s", err)
			}

			err = s.Listen(ctx, "", "")
			if err != nil {
				t.Fatalf("unable to start server: %v", err)
			}

			urlAddr := s.Addr()

			// start server in background
			go func() {
				if err := s.Serve(ctx); err != nil && err != http.ErrServerClosed {
					t.Errorf("server serve error: %v", err)
				}
			}()

			// test every endpoints that we support in Toolbox
			endpoints := []struct {
				desc        string
				requestType string
				url         string
			}{
				{
					desc:        "GET api toolset",
					requestType: "GET",
					url:         "/api/toolset",
				},
				{
					desc:        "GET api tool",
					requestType: "GET",
					url:         "/api/tool/tool_one",
				},
				{
					desc:        "POST api tool",
					requestType: "POST",
					url:         "/api/tool/tool_one/invoke",
				},
				{
					desc:        "GET mcp sse",
					requestType: "GET",
					url:         "/mcp/sse",
				},
				{
					desc:        "GET mcp",
					requestType: "GET",
					url:         "/mcp",
				},
				{
					desc:        "POST mcp",
					requestType: "POST",
					url:         "/mcp",
				},
				{
					desc:        "DELETE mcp",
					requestType: "DELETE",
					url:         "/mcp",
				},
			}
			for _, e := range endpoints {
				t.Run(e.desc, func(t *testing.T) {
					url := fmt.Sprintf("http://%s%s", urlAddr, e.url)
					client := &http.Client{}
					req, err := http.NewRequest(e.requestType, url, nil)
					if err != nil {
						t.Fatalf("Failed to create request: %v", err)
					}
					req.Header.Set("Origin", tc.origin)
					resp, err := client.Do(req)
					if err != nil {
						t.Fatalf("Failed to send request: %v", err)
					}
					defer resp.Body.Close()

					gotOrigin := resp.Header.Get("Access-Control-Allow-Origin")
					if !tc.corsBlocked {
						// if cors is not blocked, the origin header should be
						// within allowedOrigins
						if !slices.Contains(tc.allowedOrigins, gotOrigin) {
							t.Errorf(`origin "%s" is not part of allowed origins %s`, gotOrigin, tc.allowedOrigins)
						}
					} else if tc.corsBlocked {
						// if cors is blocked, the origin header should not
						// contain origin
						if gotOrigin == "*" {
							t.Errorf("REGRESSION: Server is forcing a wildcard '*' header!")
						}
						if gotOrigin == tc.origin {
							t.Errorf("server allowed an origin not in the whitelist: %s", gotOrigin)
						}
					}
				})
			}
		})
	}
}

func TestEndpointSecurityAllowedHost(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}

	testCases := []struct {
		desc         string
		allowedHosts []string
		host         string
		wantStatus   int
	}{
		{
			desc:         "allowed hosts all",
			allowedHosts: []string{"*"},
			host:         "evil.com",
			wantStatus:   http.StatusOK,
		},
		{
			desc:         "allowed hosts trusted with trusted host",
			allowedHosts: []string{"trusted.com"},
			host:         "trusted.com",
			wantStatus:   http.StatusOK,
		},
		{
			desc:         "allowed hosts trusted with evil host",
			allowedHosts: []string{"trusted.com"},
			host:         "evil.com",
			wantStatus:   http.StatusForbidden,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			addr, port := "127.0.0.1", 0
			cfg := server.ServerConfig{
				Version:      "0.0.0",
				Address:      addr,
				Port:         port,
				EnableAPI:    true,
				AllowedHosts: tc.allowedHosts,
			}

			instrumentation, err := telemetry.CreateTelemetryInstrumentation(cfg.Version)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			ctx = util.WithInstrumentation(ctx, instrumentation)

			s, err := server.NewServer(ctx, cfg)
			if err != nil {
				t.Fatalf("error setting up server: %s", err)
			}

			err = s.Listen(ctx, "", "")
			if err != nil {
				t.Fatalf("unable to start server: %v", err)
			}

			urlAddr := s.Addr()
			_, actualPort, err := net.SplitHostPort(urlAddr)
			if err != nil {
				t.Fatalf("failed to parse server address: %v", err)
			}
			hostWithPort := net.JoinHostPort(tc.host, actualPort)

			// start server in background
			go func() {
				if err := s.Serve(ctx); err != nil && err != http.ErrServerClosed {
					t.Errorf("server serve error: %v", err)
				}
			}()

			// test every endpoints that we support in Toolbox
			endpoints := []struct {
				desc        string
				requestType string
				url         string
				requestErr  int
				errStr      string
			}{
				{
					desc:        "GET api toolset",
					requestType: "GET",
					url:         "/api/toolset",
				},
				{
					desc:        "GET api tool",
					requestType: "GET",
					url:         "/api/tool/tool_one",
					requestErr:  http.StatusNotFound,
					errStr:      "invalid tool name",
				},
				{
					desc:        "POST api tool",
					requestType: "POST",
					url:         "/api/tool/tool_one/invoke",
					requestErr:  http.StatusNotFound,
					errStr:      "invalid tool name",
				},
				{
					desc:        "GET mcp sse",
					requestType: "GET",
					url:         "/mcp/sse",
				},
				{
					desc:        "GET mcp",
					requestType: "GET",
					url:         "/mcp",
					requestErr:  http.StatusMethodNotAllowed,
					errStr:      "toolbox does not support streaming in streamable HTTP transport",
				},
				{
					desc:        "POST mcp",
					requestType: "POST",
					url:         "/mcp",
				},
				{
					desc:        "DELETE mcp",
					requestType: "DELETE",
					url:         "/mcp",
				},
			}
			for _, e := range endpoints {
				t.Run(e.desc, func(t *testing.T) {
					url := fmt.Sprintf("http://%s%s", urlAddr, e.url)
					client := &http.Client{}
					req, err := http.NewRequest(e.requestType, url, nil)
					if err != nil {
						t.Fatalf("Failed to create request: %v", err)
					}
					req.Host = hostWithPort
					resp, err := client.Do(req)
					if err != nil {
						t.Fatalf("Failed to send request: %v", err)
					}
					defer resp.Body.Close()

					if resp.StatusCode != tc.wantStatus {
						bodyBytes, _ := io.ReadAll(resp.Body)
						if resp.StatusCode == e.requestErr {
							if !strings.Contains(string(bodyBytes), e.errStr) {
								t.Fatalf("got err %s, expected error %s", string(bodyBytes), e.errStr)
							}
							return
						}
						t.Fatalf("expected status %d, got %d: %s", tc.wantStatus, resp.StatusCode, string(bodyBytes))
					}
				})
			}
		})
	}
}

func TestNameValidation(t *testing.T) {
	testCases := []struct {
		desc         string
		resourceName string
		errStr       string
	}{
		{
			desc:         "names with 0 length",
			resourceName: "",
			errStr:       "resource name SHOULD be between 1 and 128 characters in length (inclusive)",
		},
		{
			desc:         "names with allowed length",
			resourceName: "foo",
		},
		{
			desc:         "names with 128 length",
			resourceName: strings.Repeat("a", 128),
		},
		{
			desc:         "names with more than 128 length",
			resourceName: strings.Repeat("a", 129),
			errStr:       "resource name SHOULD be between 1 and 128 characters in length (inclusive)",
		},
		{
			desc:         "names with space",
			resourceName: "foo bar",
			errStr:       "invalid character for resource name; only uppercase and lowercase ASCII letters (A-Z, a-z), digits (0-9), underscore (_), hyphen (-), and dot (.) is allowed",
		},
		{
			desc:         "names with commas",
			resourceName: "foo,bar",
			errStr:       "invalid character for resource name; only uppercase and lowercase ASCII letters (A-Z, a-z), digits (0-9), underscore (_), hyphen (-), and dot (.) is allowed",
		},
		{
			desc:         "names with other special character",
			resourceName: "foo!",
			errStr:       "invalid character for resource name; only uppercase and lowercase ASCII letters (A-Z, a-z), digits (0-9), underscore (_), hyphen (-), and dot (.) is allowed",
		},
		{
			desc:         "names with allowed special character",
			resourceName: "foo_.-bar6",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := server.NameValidation(tc.resourceName)
			if err != nil {
				if tc.errStr != err.Error() {
					t.Fatalf("unexpected error: %s", err)
				}
			}
			if err == nil && tc.errStr != "" {
				t.Fatalf("expect error: %s", tc.errStr)
			}
		})
	}
}

func TestPRMEndpoint(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup telemetry and logging
	otelShutdown, err := telemetry.SetupOTel(ctx, "0.0.0", "", false, "", "toolbox")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	defer func() {
		if err := otelShutdown(ctx); err != nil {
			t.Fatalf("unexpected error shutting down otel: %s", err)
		}
	}()

	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	// Create a mock OIDC server to bypass JWKS discovery during init
	mockOIDC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"issuer": "http://%s", "jwks_uri": "http://%s/jwks"}`, r.Host, r.Host)
			return
		}
		if r.URL.Path == "/jwks" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"keys": []}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockOIDC.Close()

	tests := []struct {
		name         string
		toolboxURL   string
		wantEndpoint string
		wantResource string
		wantErr      string
	}{
		{
			name:         "valid absolute url without path",
			toolboxURL:   "https://my-toolbox.example.com",
			wantEndpoint: "/.well-known/oauth-protected-resource",
			wantResource: "https://my-toolbox.example.com",
		},
		{
			name:         "valid absolute url with path",
			toolboxURL:   "https://my-toolbox.example.com/mcp",
			wantEndpoint: "/.well-known/oauth-protected-resource/mcp",
			wantResource: "https://my-toolbox.example.com/mcp",
		},
		{
			name:         "valid relative path without leading slash",
			toolboxURL:   "mcp",
			wantEndpoint: "/.well-known/oauth-protected-resource/mcp",
			wantResource: "mcp",
		},
		{
			name:         "valid relative path with leading slash",
			toolboxURL:   "/mcp",
			wantEndpoint: "/.well-known/oauth-protected-resource/mcp",
			wantResource: "/mcp",
		},
		{
			name:       "invalid absolute url missing host",
			toolboxURL: "http://",
			wantErr:    "must be a valid absolute URL with scheme and host",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := server.ServerConfig{
				Version:      "0.0.0",
				Address:      "127.0.0.1",
				Port:         0,
				ToolboxUrl:   tc.toolboxURL,
				AllowedHosts: []string{"*"},
				AuthServiceConfigs: map[string]auth.AuthServiceConfig{
					"generic1": generic.Config{
						Name:                "generic1",
						Type:                generic.AuthServiceType,
						McpEnabled:          true,
						AuthorizationServer: mockOIDC.URL,
						ScopesRequired:      []string{"read", "write"},
					},
				},
			}

			s, err := server.NewServer(ctx, cfg)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unable to initialize server: %v", err)
			}

			if err := s.Listen(ctx, "", ""); err != nil {
				t.Fatalf("unable to start server: %v", err)
			}

			go func() {
				if err := s.Serve(ctx); err != nil && err != http.ErrServerClosed {
					t.Errorf("server serve error: %v", err)
				}
			}()
			defer func() {
				if err := s.Shutdown(ctx); err != nil {
					t.Errorf("failed to cleanly shutdown server: %v", err)
				}
			}()

			reqURL := fmt.Sprintf("http://%s%s", s.Addr(), tc.wantEndpoint)
			resp, err := http.Get(reqURL)
			if err != nil {
				t.Fatalf("error when sending a request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("unexpected error reading body: %s", err)
			}

			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("unexpected error unmarshalling body: %s", err)
			}

			want := map[string]any{
				"resource": tc.wantResource,
				"authorization_servers": []any{
					mockOIDC.URL,
				},
				"scopes_supported":         []any{"read", "write"},
				"bearer_methods_supported": []any{"header"},
			}

			if !reflect.DeepEqual(got, want) {
				t.Errorf("unexpected PRM response:\ngot  %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestPRMOverride(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup a temporary PRM file
	prmContent := `{
		"resource": "https://override.example.com",
		"authorization_servers": ["https://auth.example.com"],
		"scopes_supported": ["read", "write"],
		"bearer_methods_supported": ["header"]
	}`
	tmpFile, err := os.CreateTemp("", "prm-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	if err := os.WriteFile(tmpFile.Name(), []byte(prmContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Setup Logging and Instrumentation (Using Discard to act as Noop)
	testLogger, err := log.NewStdLogger(io.Discard, io.Discard, "info")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	// Configure the server with the Override Flag
	addr, port := "127.0.0.1", 5004
	cfg := server.ServerConfig{
		Version:      "0.0.0",
		Address:      addr,
		Port:         port,
		McpPrmFile:   tmpFile.Name(),
		AllowedHosts: []string{"*"},
	}

	// Initialize and Start the Server
	s, err := server.NewServer(ctx, cfg)
	if err != nil {
		t.Fatalf("unable to initialize server: %v", err)
	}

	if err := s.Listen(ctx, "", ""); err != nil {
		t.Fatalf("unable to start listener: %v", err)
	}

	go func() {
		if err := s.Serve(ctx); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server serve error: %v\n", err)
		}
	}()
	defer func() {
		if err := s.Shutdown(ctx); err != nil {
			t.Errorf("failed to cleanly shutdown server: %v", err)
		}
	}()

	// Perform the request to the well-known endpoint
	url := fmt.Sprintf("http://%s:%d/.well-known/oauth-protected-resource", addr, port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("error when sending request: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading body: %s", err)
	}

	// Verification
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("invalid json response: %s", err)
	}

	if got["resource"] != "https://override.example.com" {
		t.Errorf("expected resource 'https://override.example.com', got '%v'", got["resource"])
	}
}

// TestLegacyAPIGone verifies that requests to legacy /api/* endpoints return 410 Gone.
func TestLegacyAPIGone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup Logging and Instrumentation (Using Discard to act as Noop)
	testLogger, err := log.NewStdLogger(io.Discard, io.Discard, "info")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	// Configure the server (EnableAPI defaults to false)
	addr, port := "127.0.0.1", 5005
	cfg := server.ServerConfig{
		Version:      "0.0.0",
		Address:      addr,
		Port:         port,
		AllowedHosts: []string{"*"},
	}

	// Initialize and Start the Server
	s, err := server.NewServer(ctx, cfg)
	if err != nil {
		t.Fatalf("unable to initialize server: %v", err)
	}

	if err := s.Listen(ctx, "", ""); err != nil {
		t.Fatalf("unable to start listener: %v", err)
	}

	go func() {
		if err := s.Serve(ctx); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server serve error: %v\n", err)
		}
	}()
	defer func() {
		if err := s.Shutdown(ctx); err != nil {
			t.Errorf("failed to cleanly shutdown server: %v", err)
		}
	}()

	// Perform the request to a legacy endpoint
	url := fmt.Sprintf("http://%s:%d/api/tool/list", addr, port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("error when sending request: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusGone {
		t.Fatalf("expected status 410 (Gone), got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading body: %s", err)
	}

	want := "/api native endpoints are disabled by default. Please use the standard /mcp JSON-RPC endpoint"
	if !strings.Contains(string(body), want) {
		t.Errorf("expected response body to contain %q, got %q", want, string(body))
	}
}

func TestMCPAuthMiddleware(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup telemetry and logging
	otelShutdown, err := telemetry.SetupOTel(ctx, "0.0.0", "", false, "", "toolbox")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	defer func() {
		if err := otelShutdown(ctx); err != nil {
			t.Fatalf("unexpected error shutting down otel: %s", err)
		}
	}()

	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	// Setup mock introspection server
	var mockResponse map[string]any
	var mockStatus int
	var mockRawResponse string

	mockOIDC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"issuer": "http://%s", "jwks_uri": "http://%s/jwks", "introspection_endpoint": "http://%s/introspect"}`, r.Host, r.Host, r.Host)
			return
		}
		if r.URL.Path == "/jwks" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"keys": []}`)
			return
		}
		if r.URL.Path == "/introspect" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(mockStatus)
			if mockRawResponse != "" {
				_, _ = w.Write([]byte(mockRawResponse))
			} else {
				respCopy := make(map[string]any)
				for k, v := range mockResponse {
					respCopy[k] = v
				}
				if _, hasIss := respCopy["iss"]; !hasIss {
					respCopy["iss"] = "http://" + r.Host
				}
				_ = json.NewEncoder(w).Encode(respCopy)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockOIDC.Close()

	// Configure the server
	addr, port := "127.0.0.1", 5004
	cfg := server.ServerConfig{
		Version:      "0.0.0",
		Address:      addr,
		Port:         port,
		ToolboxUrl:   "https://my-toolbox.example.com",
		AllowedHosts: []string{"*"},
		AuthServiceConfigs: map[string]auth.AuthServiceConfig{
			"generic1": generic.Config{
				Name:                "generic1",
				Type:                generic.AuthServiceType,
				McpEnabled:          true,
				AuthorizationServer: mockOIDC.URL,
				ScopesRequired:      []string{"mcp"},
			},
		},
	}

	// Initialize and start the server
	s, err := server.NewServer(ctx, cfg)
	if err != nil {
		t.Fatalf("unable to initialize server: %v", err)
	}

	if err := s.Listen(ctx, "", ""); err != nil {
		t.Fatalf("unable to start server: %v", err)
	}

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		if err := s.Serve(ctx); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	defer func() {
		if err := s.Shutdown(ctx); err != nil {
			t.Errorf("failed to cleanly shutdown server: %v", err)
		}
	}()

	tests := []struct {
		name           string
		token          string
		setupMock      func()
		wantStatusCode int
	}{
		{
			name:  "valid opaque token",
			token: "valid-token",
			setupMock: func() {
				mockStatus = http.StatusOK
				mockResponse = map[string]any{
					"active": true,
					"scope":  "mcp",
					"aud":    "test-audience",
					"exp":    time.Now().Add(time.Hour).Unix(),
				}
				mockRawResponse = ""
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:  "insufficient scope",
			token: "bad-scope-token",
			setupMock: func() {
				mockStatus = http.StatusOK
				mockResponse = map[string]any{
					"active": true,
					"scope":  "wrong-scope",
					"aud":    "test-audience",
					"exp":    time.Now().Add(time.Hour).Unix(),
				}
				mockRawResponse = ""
			},
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:  "malformed introspection",
			token: "any-token",
			setupMock: func() {
				mockStatus = http.StatusOK
				mockRawResponse = "{invalid json}"
			},
			wantStatusCode: http.StatusInternalServerError,
		},
		{
			name:  "unreachable introspection",
			token: "any-token",
			setupMock: func() {
				mockOIDC.Close()
			},
			wantStatusCode: http.StatusInternalServerError,
		},
	}

	url := fmt.Sprintf("http://%s:%d/mcp", addr, port)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMock()

			reqBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
			req, _ := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
			req.Header.Set("Authorization", "Bearer "+tc.token)
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.wantStatusCode {
				t.Errorf("expected status %d, got %d", tc.wantStatusCode, resp.StatusCode)
			}

			contentType := resp.Header.Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				t.Errorf("expected Content-Type to contain application/json, got %q", contentType)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}

			var jsonResp map[string]any
			if err := json.Unmarshal(body, &jsonResp); err != nil {
				t.Errorf("response body is not valid JSON: %v\nBody: %s", err, string(body))
			}

			if tc.wantStatusCode != http.StatusOK {
				if _, ok := jsonResp["error"]; !ok {
					t.Errorf("expected error field in response, got: %s", string(body))
				}
				if jsonResp["jsonrpc"] != "2.0" {
					t.Errorf("expected jsonrpc 2.0, got: %v", jsonResp["jsonrpc"])
				}
			} else {
				if _, ok := jsonResp["result"]; !ok {
					t.Errorf("expected result field in response, got: %s", string(body))
				}
			}
		})
	}
}

func TestGoogleAuthConfigValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		yaml        string
		wantError   bool
		errContains string
	}{
		{
			name: "only clientId, mcpEnabled false",
			yaml: `
kind: authService
name: my-google-auth
type: google
clientId: my-client-id
`,
			wantError: false,
		},
		{
			name: "only audience, mcpEnabled false",
			yaml: `
kind: authService
name: my-google-auth
type: google
audience: my-audience
`,
			wantError: true,
		},
		{
			name: "only audience, mcpEnabled true",
			yaml: `
kind: authService
name: my-google-auth
type: google
audience: my-audience
mcpEnabled: true
`,
			wantError: false,
		},
		{
			name: "scopesRequired, mcpEnabled false",
			yaml: `
kind: authService
name: my-google-auth
type: google
scopesRequired:
  - email
`,
			wantError: true,
		},
		{
			name: "scopesRequired, mcpEnabled true",
			yaml: `
kind: authService
name: my-google-auth
type: google
scopesRequired:
  - email
mcpEnabled: true
`,
			wantError: false,
		},
		{
			name: "both clientId and audience, mcpEnabled true",
			yaml: `
kind: authService
name: my-google-auth
type: google
clientId: my-client-id
audience: my-audience
mcpEnabled: true
`,
			wantError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, []byte(tc.yaml))
			if (err != nil) != tc.wantError {
				t.Fatalf("UnmarshalPrimitiveConfig() returned error: %v, wantError: %v", err, tc.wantError)
			}
			if err != nil && tc.errContains != "" {
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("expected error to contain %q, got %q", tc.errContains, err.Error())
				}
			}
		})
	}
}

func TestGenericAuthConfigValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		yaml        string
		wantError   bool
		errContains string
	}{
		{
			name: "valid mcpEnabled false",
			yaml: `
kind: authService
name: my-generic-auth
type: generic
audience: my-audience
authorizationServer: https://example.com
`,
			wantError: false,
		},
		{
			name: "valid mcpEnabled true",
			yaml: `
kind: authService
name: my-generic-auth
type: generic
audience: my-audience
authorizationServer: https://example.com
mcpEnabled: true
`,
			wantError: false,
		},
		{
			name: "introspectionEndpoint, mcpEnabled false",
			yaml: `
kind: authService
name: my-generic-auth
type: generic
audience: my-audience
authorizationServer: https://example.com
introspectionEndpoint: http://example.com/introspect
`,
			wantError: true,
		},
		{
			name: "introspectionMethod, mcpEnabled false",
			yaml: `
kind: authService
name: my-generic-auth
type: generic
audience: my-audience
authorizationServer: https://example.com
introspectionMethod: POST
`,
			wantError: true,
		},
		{
			name: "introspectionParamName, mcpEnabled false",
			yaml: `
kind: authService
name: my-generic-auth
type: generic
audience: my-audience
authorizationServer: https://example.com
introspectionParamName: token
`,
			wantError: true,
		},
		{
			name: "scopesRequired, mcpEnabled false",
			yaml: `
kind: authService
name: my-generic-auth
type: generic
audience: my-audience
authorizationServer: https://example.com
scopesRequired:
  - email
`,
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, []byte(tc.yaml))
			if (err != nil) != tc.wantError {
				t.Fatalf("UnmarshalPrimitiveConfig() returned error: %v, wantError: %v", err, tc.wantError)
			}
			if err != nil && tc.errContains != "" {
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("expected error to contain %q, got %q", tc.errContains, err.Error())
				}
			}
		})
	}
}

func TestDuplicateResourceConfig(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "duplicate source",
			yaml: `
kind: source
name: my_source
type: alloydb-postgres
project: my-project
region: us-central1
cluster: my-cluster
instance: my-instance
database: my-db
---
kind: source
name: my_source
type: alloydb-postgres
project: my-project
region: us-central1
cluster: my-cluster
instance: my-instance
database: my-db
`,
		},
		{
			name: "duplicate authService",
			yaml: `
kind: authService
name: my_auth
type: generic
audience: my-audience
authorizationServer: https://example.com
---
kind: authService
name: my_auth
type: generic
audience: my-audience
authorizationServer: https://example.com
`,
		},
		{
			name: "duplicate tool",
			yaml: `
kind: tool
name: my_tool
type: http
source: my_source
path: /a
method: GET
---
kind: tool
name: my_tool
type: http
source: my_source
path: /b
method: GET
`,
		},
		{
			name: "duplicate toolset",
			yaml: `
kind: toolset
name: my_toolset
tools:
  - tool_a
---
kind: toolset
name: my_toolset
tools:
  - tool_b
`,
		},
		{
			name: "duplicate embeddingModel",
			yaml: `
kind: embeddingModel
name: my_model
type: gemini
model: text-embedding-005
---
kind: embeddingModel
name: my_model
type: gemini
model: text-embedding-005
`,
		},
		{
			name: "duplicate prompt",
			yaml: `
kind: prompt
name: my_prompt
messages:
  - role: user
    content: hello
---
kind: prompt
name: my_prompt
messages:
  - role: user
    content: world
`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, []byte(tc.yaml))
			if err == nil {
				t.Fatalf("UnmarshalPrimitiveConfig() expected a duplicate error, got nil")
			}
			if !strings.Contains(err.Error(), "declared more than once") {
				t.Fatalf("UnmarshalPrimitiveConfig() error = %v, want it to mention 'declared more than once'", err)
			}
		})
	}
}

func TestGroupConfigParsing(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		yaml        string
		want        group.GroupConfig
		wantError   bool
		errContains string
	}{
		{
			name: "valid named group",
			yaml: `
kind: group
name: my_group
description: a group of tools and prompts
tools:
  - tool_a
  - tool_b
prompts:
  - prompt_a
`,
			want: group.GroupConfig{
				Name:        "my_group",
				Description: "a group of tools and prompts",
				ToolNames:   []string{"tool_a", "tool_b"},
				PromptNames: []string{"prompt_a"},
			},
		},
		{
			name: "named group with only description",
			yaml: `
kind: group
name: my_group
description: just a description
`,
			want: group.GroupConfig{
				Name:        "my_group",
				Description: "just a description",
			},
		},
		{
			name: "default group with only description",
			yaml: `
kind: group
name:
description: default server instruction
`,
			want: group.GroupConfig{
				Description: "default server instruction",
			},
		},
		{
			name: "default group omitting name field",
			yaml: `
kind: group
description: default server instruction
`,
			want: group.GroupConfig{
				Description: "default server instruction",
			},
		},
		{
			name: "kind toolset folds into a tools-only group",
			yaml: `
kind: toolset
name: my_toolset
tools:
  - tool_a
  - tool_b
`,
			want: group.GroupConfig{
				Name:      "my_toolset",
				ToolNames: []string{"tool_a", "tool_b"},
			},
		},
		{
			name: "valid named group with resources and resource templates",
			yaml: `
kind: group
name: my_group_with_resources
tools:
  - tool_a
prompts:
  - prompt_a
resources:
  - res_a
resourceTemplates:
  - tmpl_a
`,
			want: group.GroupConfig{
				Name:                  "my_group_with_resources",
				ToolNames:             []string{"tool_a"},
				PromptNames:           []string{"prompt_a"},
				ResourceNames:         []string{"res_a"},
				ResourceTemplateNames: []string{"tmpl_a"},
			},
		},
		{
			name: "default group declaring tools is an error",
			yaml: `
kind: group
name:
tools:
  - tool_a
`,
			wantError: true,
		},
		{
			name: "default group declaring prompts is an error",
			yaml: `
kind: group
name:
prompts:
  - prompt_a
`,
			wantError: true,
		},
		{
			name: "default group declaring resources is an error",
			yaml: `
kind: group
name:
resources:
  - res_a
`,
			wantError: true,
		},
		{
			name: "default group declaring resourceTemplates is an error",
			yaml: `
kind: group
name:
resourceTemplates:
  - tmpl_a
`,
			wantError: true,
		},
		{
			name: "unknown field is an error",
			yaml: `
kind: group
name: my_group
unknown_field:
  - res_a
`,
			wantError: true,
		},
		{
			name: "duplicate default group is an error",
			yaml: `
kind: group
name:
description: first
---
kind: group
name:
description: second
`,
			wantError: true,
		},
		{
			name: "duplicate named group is an error",
			yaml: `
kind: group
name: my_group
tools:
  - tool_a
---
kind: group
name: my_group
tools:
  - tool_b
`,
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, _, _, _, groups, err := server.UnmarshalPrimitiveConfig(ctx, []byte(tc.yaml))
			if (err != nil) != tc.wantError {
				t.Fatalf("UnmarshalPrimitiveConfig() returned error: %v, wantError: %v", err, tc.wantError)
			}
			if tc.wantError {
				return
			}
			gc, ok := groups[tc.want.Name]
			if !ok {
				t.Fatalf("expected group %q to be parsed, got: %v", tc.want.Name, groups)
			}
			if diff := cmp.Diff(tc.want, gc); diff != "" {
				t.Errorf("group mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGroupConfigValues(t *testing.T) {
	ctx := context.Background()
	yaml := `
kind: group
name: my_group
description: a group
tools:
  - tool_a
  - tool_b
prompts:
  - prompt_a
`
	_, _, _, _, _, _, _, groups, err := server.UnmarshalPrimitiveConfig(ctx, []byte(yaml))
	if err != nil {
		t.Fatalf("UnmarshalPrimitiveConfig() returned unexpected error: %v", err)
	}
	gc, ok := groups["my_group"]
	if !ok {
		t.Fatalf("expected group %q to be parsed, got: %v", "my_group", groups)
	}
	if gc.Name != "my_group" {
		t.Errorf("group name: got %q, want %q", gc.Name, "my_group")
	}
	if gc.Description != "a group" {
		t.Errorf("group description: got %q, want %q", gc.Description, "a group")
	}
	if diff := cmp.Diff([]string{"tool_a", "tool_b"}, gc.ToolNames); diff != "" {
		t.Errorf("group tools mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"prompt_a"}, gc.PromptNames); diff != "" {
		t.Errorf("group prompts mismatch (-want +got):\n%s", diff)
	}
}

func TestInitializeConfigs(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}
	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)
	t.Run("valid initialization", func(t *testing.T) {
		sourceConfig1 := testutils.MockSourceConfig{Name: "my-source", Type: "mock-source"}
		source1, _ := sourceConfig1.Initialize(ctx, nil)
		tools1 := testutils.NewMockTool("my-tool", "mock tool for offline config", "my-source", nil, false, false)
		validCfg := server.ServerConfig{
			Version: "0.0.0",
			SourceConfigs: server.SourceConfigs{
				"my-source": testutils.MockSourceConfig{Name: "my-source", Type: "mock-source"},
			},
			ToolConfigs: server.ToolConfigs{
				"my-tool": tools1.ToConfig(),
			},
		}
		sourcesMap, _, _, toolsMap, _, _, _, _, err := server.InitializeConfigs(ctx, validCfg)
		if err != nil {
			t.Fatalf("unexpected error during config initialization: %s", err)
		}
		wantSourcesMap := map[string]sources.Source{"my-source": source1}
		wantToolsMap := map[string]tools.Tool{"my-tool": tools1}
		if !reflect.DeepEqual(sourcesMap, wantSourcesMap) {
			t.Fatalf("sources map mismatch: want %s, got %s", wantSourcesMap, sourcesMap)
		}
		if !reflect.DeepEqual(toolsMap, wantToolsMap) {
			t.Fatalf("tools map mismatch: want %s, got %s", wantToolsMap, toolsMap)
		}
	})
	t.Run("invalid initialization", func(t *testing.T) {
		invalidCfg := server.ServerConfig{
			Version: "0.0.0",
			SourceConfigs: server.SourceConfigs{
				"my-source": testutils.MockSourceConfig{Name: "my-source", Type: "invalid-type"},
			},
			ToolConfigs: server.ToolConfigs{
				"my-invalid-tool": testutils.NewMockTool("my-tool", "mock tool for offline config", "my-source", nil, false, false).ToConfig(),
			},
		}
		_, _, _, _, _, _, _, _, err := server.InitializeConfigs(ctx, invalidCfg)
		if err == nil {
			t.Fatalf("expected error but got nil")
		}
		wantErr := `invalid source for "mock-tool" tool: source "my-source" is not a compatible type`
		if err.Error() != wantErr {
			t.Fatalf("unexpected error: want %s, got %s", wantErr, err.Error())
		}
	})
}

func TestInitializeOfflineConfigs(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}
	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	cfg := server.ServerConfig{
		Version: "0.0.0",
		SourceConfigs: server.SourceConfigs{
			"my-source": testutils.MockSourceConfig{Name: "my-source"},
		},
		ToolConfigs: server.ToolConfigs{
			"my-tool": testutils.NewMockTool("my-tool", "mock tool for offline config", "non-existance-source", nil, false, false).ToConfig(),
		},
	}

	toolsMap, groupsMap, err := server.InitializeOfflineConfigs(ctx, cfg)
	if err != nil {
		t.Fatalf("InitializeOfflineConfigs returned error: %s", err)
	}
	if _, ok := toolsMap["my-tool"]; !ok {
		t.Errorf("expected tool %q in toolsMap, got %v", "my-tool", toolsMap)
	}
	// The implicit default ("") group should always be present.
	if _, ok := groupsMap[""]; !ok {
		t.Error("expected default group to be present")
	}
}

type mockClashAuthConfig struct{}

var _ auth.AuthServiceConfig = mockClashAuthConfig{}
var _ auth.MCPAuthService = mockClashAuthService{}

func (c mockClashAuthConfig) AuthServiceConfigType() string { return "mock-clash" }
func (c mockClashAuthConfig) IsMCPEnabled() bool            { return true }
func (c mockClashAuthConfig) Initialize() (auth.AuthService, error) {
	return mockClashAuthService{}, nil
}

type mockClashAuthService struct{}

func (s mockClashAuthService) AuthServiceType() string { return "mock-clash" }
func (s mockClashAuthService) GetName() string         { return "mock-clash" }
func (s mockClashAuthService) GetClaimsFromHeader(ctx context.Context, h http.Header) (map[string]any, error) {
	return nil, nil
}
func (s mockClashAuthService) ToConfig() auth.AuthServiceConfig { return mockClashAuthConfig{} }
func (s mockClashAuthService) IsMCPEnabled() bool               { return true }
func (s mockClashAuthService) GetScopesRequired() []string      { return nil }
func (s mockClashAuthService) GetAuthorizationServer() string   { return "mock-auth-server" }
func (s mockClashAuthService) ValidateMCPAuth(ctx context.Context, h http.Header) (map[string]any, error) {
	return nil, nil
}

func TestMCPAuthEnableAPIClash(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}
	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	cfg := server.ServerConfig{
		Version:   "0.0.0",
		EnableAPI: true,
		AuthServiceConfigs: map[string]auth.AuthServiceConfig{
			"mock-clash": mockClashAuthConfig{},
		},
	}

	_, err = server.NewServer(ctx, cfg)
	if err == nil {
		t.Fatal("expected error when starting server with MCP Auth and EnableAPI both enabled, got nil")
	}
	if !strings.Contains(err.Error(), "MCP Auth cannot be enabled together with the legacy HTTP API") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestResourceConfigValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		yaml        string
		wantError   bool
		errContains string
	}{
		{
			name: "missing required text field for text resource triggers validation error",
			yaml: `
kind: resource
name: test-resource
type: text
`,
			wantError: true,
		},
		{
			name: "unknown field triggers strict decoder error",
			yaml: `
kind: resource
name: test-resource
type: mock
uri: mock://test
invalidRandomField: true
`,
			wantError: true,
		},
		{
			name: "size field is rejected by strict decoder",
			yaml: `
kind: resource
name: test-resource
type: mock
uri: mock://test
size: 100
`,
			wantError:   true,
			errContains: "unknown field \"size\"",
		},
		{
			name: "valid mock resource parses successfully",
			yaml: `
kind: resource
name: test-resource
type: mock
uri: mock://test
`,
			wantError: false,
		},
		{
			name: "missing type field triggers error",
			yaml: `
kind: resource
name: test-resource
uri: mock://test
`,
			wantError: true,
		},
		{
			name: "invalid type field (not string) triggers error",
			yaml: `
kind: resource
name: test-resource
type: 123
uri: mock://test
`,
			wantError: true,
		},
		{
			name: "missing uri field triggers error",
			yaml: `
kind: resource
name: test-resource
type: mock
`,
			wantError: true,
		},
		{
			name: "invalid uri field (not string) triggers error",
			yaml: `
kind: resource
name: test-resource
type: mock
uri: 123
`,
			wantError: true,
		},
		{
			name: "invalid RFC URI triggers error",
			yaml: `
kind: resource
name: test-resource
type: mock
uri: ://missing.scheme
`,
			wantError: true,
		},
		{
			name: "duplicate resource names triggers error",
			yaml: `
kind: resource
name: duplicate-resource
type: mock
uri: mock://test1
---
kind: resource
name: duplicate-resource
type: mock
uri: mock://test2
`,
			wantError: true,
		},
		{
			name: "duplicate resource URIs triggers error",
			yaml: `
kind: resource
name: resource1
type: mock
uri: mock://duplicate
---
kind: resource
name: resource2
type: mock
uri: mock://duplicate
`,
			wantError: true,
		},
		{
			name: "malformed file type with missing path triggers validation error",
			yaml: `
kind: resource
name: bad-file
type: file
uri: file://bad-file
`,
			wantError: true,
		},
		{
			name: "missing type",
			yaml: `
kind: resource
name: test
uri: mock://test
`,
			wantError: true,
		},
		{
			name: "valid resource template",
			yaml: `
kind: resourceTemplate
type: file
name: project_files
uriTemplate: file://{path}
description: Access files in the project directory.
`,
			wantError: false,
		},
		{
			name: "missing name",
			yaml: `
kind: resourceTemplate
type: file
uriTemplate: file://{path}
`,
			wantError:   true,
			errContains: "missing 'name' field",
		},
		{
			name: "missing type throws error",
			yaml: `
kind: resourceTemplate
name: test
uriTemplate: file://{path}
`,
			wantError:   true,
			errContains: "missing 'type' field or it is not a string",
		},
		{
			name: "invalid scheme for file template",
			yaml: `
kind: resourceTemplate
type: file
name: test
uriTemplate: http://example.com/{path}
`,
			wantError:   true,
			errContains: "invalid scheme for file resource template",
		},
		{
			name: "duplicate uri template",
			yaml: `
kind: resourceTemplate
type: file
name: t1
uriTemplate: file://{path}
---
kind: resourceTemplate
type: file
name: t2
uriTemplate: file://{path}
`,
			wantError:   true,
			errContains: "duplicate resource URI",
		},
		{
			name: "malformed template without URITemplate",
			yaml: `
kind: resourceTemplate
name: test
type: file
`,
			wantError:   true,
			errContains: "validation for 'URITemplate' failed on the 'required' tag",
		},
		{
			name: "duplicate name",
			yaml: `
kind: resourceTemplate
type: file
name: t1
uriTemplate: file://{path}
---
kind: resourceTemplate
type: file
name: t1
uriTemplate: file:///{path}
`,
			wantError:   true,
			errContains: "duplicate resourceTemplate name",
		},
	}

	// Register a mock factory for this test package to use
	mockFactory := func(ctx context.Context, name string, decoder *yaml.Decoder) (resources.ResourceConfig, error) {
		var cfg testutils.MockResourceConfig
		cfg.Name = name
		if err := decoder.DecodeContext(ctx, &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	resources.Register("mock", mockFactory)

	mockTemplateFactory := func(ctx context.Context, name string, decoder *yaml.Decoder) (resources.ResourceTemplateConfig, error) {
		var cfg testutils.MockResourceTemplateConfig
		cfg.Name = name
		if err := decoder.DecodeContext(ctx, &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	resources.RegisterTemplate("file", mockTemplateFactory)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, []byte(tc.yaml))
			if (err != nil) != tc.wantError {
				t.Fatalf("UnmarshalPrimitiveConfig() returned error: %v, wantError: %v", err, tc.wantError)
			}
			if err != nil && tc.errContains != "" {
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("expected error to contain %q, got %q", tc.errContains, err.Error())
				}
			}
		})
	}
}

func TestResourceAnnotationsParsing(t *testing.T) {
	ctx := context.Background()

	mockFactory := func(ctx context.Context, name string, decoder *yaml.Decoder) (resources.ResourceConfig, error) {
		var cfg testutils.MockResourceConfig
		cfg.Name = name
		if err := decoder.DecodeContext(ctx, &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	resources.Register("mock", mockFactory)

	// Scenario 1: Valid parsed config (checks case-insensitivity on 'Priority' and 'LastModified')
	yamlBytes := []byte(`
kind: resource
name: test-annotations
type: mock
uri: mock://test
annotations:
  priority: 0.8
  audience:
    - user
    - assistant
  lastModified: 2024-01-01T00:00:00Z
`)
	_, _, _, _, _, resConfigs, _, _, err := server.UnmarshalPrimitiveConfig(ctx, yamlBytes)
	if err != nil {
		t.Fatalf("unexpected error parsing valid config: %v", err)
	}

	cfg, ok := resConfigs["test-annotations"]
	if !ok {
		t.Fatalf("missing parsed config")
	}

	mockCfg, ok := cfg.(*testutils.MockResourceConfig)
	if !ok {
		t.Fatalf("config is not a MockResourceConfig")
	}
	if mockCfg.Annotations == nil {
		t.Fatalf("annotations map is nil")
	}

	if mockCfg.Annotations.Priority == nil || *mockCfg.Annotations.Priority != 0.8 {
		t.Errorf("priority = %v, want 0.8", mockCfg.Annotations.Priority)
	}

	if len(mockCfg.Annotations.Audience) != 2 || mockCfg.Annotations.Audience[0] != resources.RoleUser {
		t.Errorf("audience = %v, want [user, assistant]", mockCfg.Annotations.Audience)
	}

	// Verify the unquoted timestamp parsed correctly into the string field
	if mockCfg.Annotations.LastModified != "2024-01-01T00:00:00Z" {
		t.Errorf("lastModified = %v, want 2024-01-01T00:00:00Z", mockCfg.Annotations.LastModified)
	}

	// Edge Cases & Validation Testing
	testCases := []struct {
		name        string
		yaml        string
		wantError   bool
		errContains string
	}{
		{
			name: "unknown field strict error",
			yaml: `
kind: resource
name: test-invalid
type: mock
uri: mock://test
annotations:
  unknownField: "should error"`,
			wantError:   true,
			errContains: "unknownField",
		},
		{
			name: "invalid priority type",
			yaml: `
kind: resource
name: test-invalid
type: mock
uri: mock://test
annotations:
  priority: "high"`,
			wantError:   true,
			errContains: "cannot unmarshal",
		},
		{
			name: "invalid audience scalar",
			yaml: `
kind: resource
name: test-invalid
type: mock
uri: mock://test
annotations:
  audience: user`,
			wantError:   true,
			errContains: "string was used where sequence is expected",
		},
		{
			name: "invalid audience value",
			yaml: `
kind: resource
name: test-invalid
type: mock
uri: mock://test
annotations:
  audience:
    - admin`,
			wantError:   true,
			errContains: "invalid audience \"admin\"",
		},
		{
			name: "duplicate audience value",
			yaml: `
kind: resource
name: test-duplicate
type: mock
uri: mock://test
annotations:
  audience:
    - user
    - user`,
			wantError:   true,
			errContains: "duplicate audience \"user\"",
		},
		{
			name: "empty block safety (no error expected)",
			yaml: `
kind: resource
name: test-empty
type: mock
uri: mock://test
annotations: {}`,
			wantError: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err = server.UnmarshalPrimitiveConfig(ctx, []byte(tt.yaml))
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error for %q, got nil", tt.name)
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain %q, got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for %q: %v", tt.name, err)
				}
			}
		})
	}
}

func TestDefaultToolsetIsAlphabeticallySorted(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}
	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	cfg := server.ServerConfig{
		Version: "0.0.0",
		ToolConfigs: server.ToolConfigs{
			"zoo":    testutils.NewMockTool("zoo", "", "", nil, false, false).ToConfig(),
			"apple":  testutils.NewMockTool("apple", "", "", nil, false, false).ToConfig(),
			"banana": testutils.NewMockTool("banana", "", "", nil, false, false).ToConfig(),
		},
	}

	_, toolsetsMap, err := server.InitializeOfflineConfigs(ctx, cfg)
	if err != nil {
		t.Fatalf("InitializeOfflineConfigs returned error: %s", err)
	}

	defaultToolset, ok := toolsetsMap[""]
	if !ok {
		t.Fatal("expected default toolset to be present")
	}

	expectedOrder := []string{"apple", "banana", "zoo"}
	if diff := cmp.Diff(expectedOrder, defaultToolset.ToolNames); diff != "" {
		t.Errorf("default toolset ToolNames mismatch (-want +got):\n%s", diff)
	}
}

func TestNewServer_Extensions(t *testing.T) {
	orig := v20260728.SupportedExtensions
	t.Cleanup(func() {
		v20260728.SupportedExtensions = orig
	})
	v20260728.SupportedExtensions = map[string]any{"com.google.cloud/toolbox.v1": map[string]any{}, "io.modelcontextprotocol/tasks": map[string]any{}}

	ctx := context.Background()
	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	tests := []struct {
		name       string
		disableExt []string
		want       []string
	}{
		{
			name:       "default enables all supported extensions",
			disableExt: nil,
			want:       []string{"com.google.cloud/toolbox.v1", "io.modelcontextprotocol/tasks"},
		},
		{
			name:       "disable one extension",
			disableExt: []string{"io.modelcontextprotocol/tasks"},
			want:       []string{"com.google.cloud/toolbox.v1"},
		},
		{
			name:       "disable all supported extensions",
			disableExt: []string{"com.google.cloud/toolbox.v1", "io.modelcontextprotocol/tasks"},
			want:       nil,
		},
		{
			name:       "empty strings or unknown extensions in disableExt ignored",
			disableExt: []string{"", "com.example/unknown"},
			want:       []string{"com.google.cloud/toolbox.v1", "io.modelcontextprotocol/tasks"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := server.ServerConfig{
				DisableExt: tt.disableExt,
			}
			_, err := server.NewServer(ctx, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got []string
			for k := range v20260728.ServerExtensions {
				got = append(got, k)
			}
			slices.Sort(got)
			slices.Sort(tt.want)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("v20260728.ServerExtensions keys = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldSuppressTool(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}

	readOnlySource := &testutils.MockSource{MockSourceConfig: testutils.MockSourceConfig{Name: "readonly-db", ReadOnly: true}}
	readWriteSource := &testutils.MockSource{MockSourceConfig: testutils.MockSourceConfig{Name: "readwrite-db", ReadOnly: false}}

	initTool := func(cfg testutils.MockToolConfig) tools.Tool {
		t, err := cfg.Initialize(ctx)
		if err != nil {
			panic(err)
		}
		return t
	}

	testCases := []struct {
		desc   string
		source sources.Source
		tool   tools.Tool
		want   bool
	}{
		{
			desc:   "nil source -> not suppressed",
			source: nil,
			tool:   initTool(testutils.MockToolConfig{ConfigBase: tools.ConfigBase{Name: "some-tool"}, Source: "readonly-db"}),
			want:   false,
		},
		{
			desc:   "nil tool -> not suppressed",
			source: readOnlySource,
			tool:   nil,
			want:   false,
		},
		{
			desc:   "write tool on read-write source (readOnlyHint: false) -> not suppressed",
			source: readWriteSource,
			tool:   initTool(testutils.MockToolConfig{ConfigBase: tools.ConfigBase{Name: "write-tool"}, Source: "readwrite-db", Annotations: tools.NewWriteAnnotations()}),
			want:   false,
		},
		{
			desc:   "write tool on read-only source (readOnlyHint: false) -> suppressed",
			source: readOnlySource,
			tool:   initTool(testutils.MockToolConfig{ConfigBase: tools.ConfigBase{Name: "write-tool"}, Source: "readonly-db", Annotations: tools.NewWriteAnnotations()}),
			want:   true,
		},
		{
			desc:   "read-only tool on read-only source (readOnlyHint: true) -> not suppressed",
			source: readOnlySource,
			tool:   initTool(testutils.MockToolConfig{ConfigBase: tools.ConfigBase{Name: "readonly-tool"}, Source: "readonly-db", Annotations: tools.NewReadOnlyAnnotations()}),
			want:   false,
		},
		{
			desc:   "unannotated tool on read-only source -> not suppressed",
			source: readOnlySource,
			tool:   initTool(testutils.MockToolConfig{ConfigBase: tools.ConfigBase{Name: "unannotated-tool"}, Source: "readonly-db", Annotations: nil}),
			want:   false,
		},
		{
			desc:   "tool with non-nil annotations but nil readOnlyHint on read-only source -> not suppressed",
			source: readOnlySource,
			tool:   initTool(testutils.MockToolConfig{ConfigBase: tools.ConfigBase{Name: "nil-hint-tool"}, Source: "readonly-db", Annotations: &tools.ToolAnnotations{ReadOnlyHint: nil}}),
			want:   false,
		},
		{
			desc:   "mysql-execute-sql on read-only source -> not suppressed (dynamically reports readOnlyHint: true)",
			source: readOnlySource,
			tool: func() tools.Tool {
				t, err := mysqlexecutesql.Config{
					ConfigBase: tools.ConfigBase{Name: "mysql-execute-sql", Description: "execute sql query"},
					Type:       "mysql-execute-sql",
					Source:     "readonly-db",
				}.Initialize(ctx)
				if err != nil {
					panic(err)
				}
				return t
			}(),
			want: false,
		},
		{
			desc:   "postgres-execute-sql on read-only source -> not suppressed (dynamically reports readOnlyHint: true)",
			source: readOnlySource,
			tool: func() tools.Tool {
				t, err := postgresexecutesql.Config{
					ConfigBase: tools.ConfigBase{Name: "postgres-execute-sql", Description: "execute sql query"},
					Type:       "postgres-execute-sql",
					Source:     "readonly-db",
				}.Initialize(ctx)
				if err != nil {
					panic(err)
				}
				return t
			}(),
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := tools.ShouldSuppress(ctx, tc.tool, tc.source)
			if got != tc.want {
				t.Errorf("ShouldSuppress(ctx) = %v, want %v", got, tc.want)
			}
			gotNilLogger := tools.ShouldSuppress(context.Background(), tc.tool, tc.source)
			if gotNilLogger != tc.want {
				t.Errorf("ShouldSuppress(nil logger) = %v, want %v", gotNilLogger, tc.want)
			}
		})
	}
}

func TestInitializeGroups(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("error setting up logger: %s", err)
	}
	instrumentation, err := telemetry.CreateTelemetryInstrumentation("0.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	t.Run("suppressed tool is pruned from group", func(t *testing.T) {
		cfg := server.ServerConfig{
			SourceConfigs: server.SourceConfigs{
				"readonly-db": &testutils.MockSourceConfig{Name: "readonly-db", ReadOnly: true},
			},
			ToolConfigs: server.ToolConfigs{
				"allowed_read_tool": &testutils.MockToolConfig{
					ConfigBase:  tools.ConfigBase{Name: "allowed_read_tool"},
					Source:      "readonly-db",
					Annotations: tools.NewReadOnlyAnnotations(),
				},
				"suppressed_write_tool": &testutils.MockToolConfig{
					ConfigBase:  tools.ConfigBase{Name: "suppressed_write_tool"},
					Source:      "readonly-db",
					Annotations: tools.NewWriteAnnotations(),
				},
			},
			GroupConfigs: server.GroupConfigs{
				"my-custom-group": group.GroupConfig{
					Name:      "my-custom-group",
					ToolNames: []string{"allowed_read_tool", "suppressed_write_tool"},
				},
			},
		}

		s, err := server.NewServer(ctx, cfg)
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		grp, ok := s.PrimitiveMgr.GetGroup("my-custom-group")
		if !ok {
			t.Fatalf("expected group 'my-custom-group' to be registered, but not found")
		}

		// Verify allowed tool is present in group
		if !grp.ContainsTool("allowed_read_tool") {
			t.Errorf("expected group to contain 'allowed_read_tool'")
		}

		// Verify suppressed tool was pruned from group
		if grp.ContainsTool("suppressed_write_tool") {
			t.Errorf("expected group to NOT contain suppressed tool 'suppressed_write_tool'")
		}

		// Verify group ToolNames slice only contains allowed_read_tool
		wantTools := []string{"allowed_read_tool"}
		if diff := cmp.Diff(wantTools, grp.ToolNames); diff != "" {
			t.Errorf("group ToolNames mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("group with all tools suppressed remains registered with empty tools", func(t *testing.T) {
		cfg := server.ServerConfig{
			SourceConfigs: server.SourceConfigs{
				"readonly-db": &testutils.MockSourceConfig{Name: "readonly-db", ReadOnly: true},
			},
			ToolConfigs: server.ToolConfigs{
				"write_tool_1": &testutils.MockToolConfig{
					ConfigBase:  tools.ConfigBase{Name: "write_tool_1"},
					Source:      "readonly-db",
					Annotations: tools.NewWriteAnnotations(),
				},
				"write_tool_2": &testutils.MockToolConfig{
					ConfigBase:  tools.ConfigBase{Name: "write_tool_2"},
					Source:      "readonly-db",
					Annotations: tools.NewWriteAnnotations(),
				},
			},
			GroupConfigs: server.GroupConfigs{
				"admin-group": group.GroupConfig{
					Name:      "admin-group",
					ToolNames: []string{"write_tool_1", "write_tool_2"},
				},
			},
		}

		s, err := server.NewServer(ctx, cfg)
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		grp, ok := s.PrimitiveMgr.GetGroup("admin-group")
		if !ok {
			t.Fatalf("expected group 'admin-group' to be registered even when empty, but not found")
		}

		if len(grp.ToolNames) != 0 {
			t.Errorf("expected group ToolNames to be empty, got: %v", grp.ToolNames)
		}
	})

	t.Run("group with no tools suppressed initializes normally", func(t *testing.T) {
		cfg := server.ServerConfig{
			SourceConfigs: server.SourceConfigs{
				"readwrite-db": &testutils.MockSourceConfig{Name: "readwrite-db", ReadOnly: false},
			},
			ToolConfigs: server.ToolConfigs{
				"tool_1": &testutils.MockToolConfig{
					ConfigBase:  tools.ConfigBase{Name: "tool_1"},
					Source:      "readwrite-db",
					Annotations: tools.NewWriteAnnotations(),
				},
				"tool_2": &testutils.MockToolConfig{
					ConfigBase:  tools.ConfigBase{Name: "tool_2"},
					Source:      "readwrite-db",
					Annotations: tools.NewReadOnlyAnnotations(),
				},
			},
			GroupConfigs: server.GroupConfigs{
				"all-tools-group": group.GroupConfig{
					Name:      "all-tools-group",
					ToolNames: []string{"tool_1", "tool_2"},
				},
			},
		}

		s, err := server.NewServer(ctx, cfg)
		if err != nil {
			t.Fatalf("failed to create server: %v", err)
		}

		grp, ok := s.PrimitiveMgr.GetGroup("all-tools-group")
		if !ok {
			t.Fatalf("expected group 'all-tools-group' to be registered, but not found")
		}

		wantTools := []string{"tool_1", "tool_2"}
		if diff := cmp.Diff(wantTools, grp.ToolNames); diff != "" {
			t.Errorf("group ToolNames mismatch (-want +got):\n%s", diff)
		}
	})
}
