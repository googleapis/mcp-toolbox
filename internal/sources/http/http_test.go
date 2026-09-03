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

package http

import (
	"bytes"
	"context"
	"net"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

func TestParseFromYamlHttp(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-http-instance
			type: http
			baseUrl: http://test_server/
			`,
			want: map[string]sources.SourceConfig{
				"my-http-instance": Config{
					Name:                   "my-http-instance",
					Type:                   SourceType,
					BaseURL:                "http://test_server/",
					Timeout:                "30s",
					DisableSslVerification: false,
				},
			},
		},
		{
			desc: "advanced example",
			in: `
			kind: source
			name: my-http-instance
			type: http
			baseUrl: http://test_server/
			timeout: 10s
			headers:
				Authorization: test_header
				Custom-Header: custom
			queryParams:
				api-key: test_api_key
				param: param-value
			returnFullError: true
			disableSslVerification: true
			`,
			want: map[string]sources.SourceConfig{
				"my-http-instance": Config{
					Name:                   "my-http-instance",
					Type:                   SourceType,
					BaseURL:                "http://test_server/",
					Timeout:                "10s",
					DefaultHeaders:         map[string]string{"Authorization": "test_header", "Custom-Header": "custom"},
					QueryParams:            map[string]string{"api-key": "test_api_key", "param": "param-value"},
					ReturnFullError:        true,
					DisableSslVerification: true,
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
			name: my-http-instance
			type: http
			baseUrl: http://test_server/
			timeout: 10s
			headers:
				Authorization: test_header
			queryParams:
				api-key: test_api_key
			project: test-project
			`,
			err: "error unmarshaling source: unable to parse source \"my-http-instance\" as \"http\": [5:1] unknown field \"project\"\n   2 | headers:\n   3 |   Authorization: test_header\n   4 | name: my-http-instance\n>  5 | project: test-project\n       ^\n   6 | queryParams:\n   7 |   api-key: test_api_key\n   8 | timeout: 10s\n   9 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-http-instance
			baseUrl: http://test_server/
			`,
			err: "error unmarshaling source: missing 'type' field or it is not a string",
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

func TestRunRequestSanitizesErrorBodyByDefault(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(nethttp.StatusBadRequest)
		_, _ = w.Write([]byte("sensitive details"))
	}))
	defer server.Close()

	logger, err := log.NewLogger("standard", log.Debug, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	sourceConfig := Config{
		Name:                 "test-http",
		Type:                 SourceType,
		BaseURL:              server.URL,
		Timeout:              "30s",
		AllowPrivateNetworks: true,
	}
	initialized, err := sourceConfig.Initialize(ctx, nil)
	if err != nil {
		t.Fatalf("failed to initialize source: %v", err)
	}
	source := initialized.(*Source)

	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	_, err = source.RunRequest(ctx, req)
	if err == nil {
		t.Fatalf("expected error for non-2xx response")
	}
	if strings.Contains(err.Error(), "sensitive details") {
		t.Fatalf("expected sanitized error message, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "unexpected status code: 400") {
		t.Fatalf("expected status code in error message, got %q", err.Error())
	}
}

func TestRunRequestIncludesErrorBodyWhenEnabled(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(nethttp.StatusInternalServerError)
		_, _ = w.Write([]byte("sensitive details"))
	}))
	defer server.Close()

	logger, err := log.NewLogger("standard", log.Debug, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	sourceConfig := Config{
		Name:                 "test-http",
		Type:                 SourceType,
		BaseURL:              server.URL,
		Timeout:              "30s",
		ReturnFullError:      true,
		AllowPrivateNetworks: true,
	}
	initialized, err := sourceConfig.Initialize(ctx, nil)
	if err != nil {
		t.Fatalf("failed to initialize source: %v", err)
	}
	source := initialized.(*Source)

	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	_, err = source.RunRequest(ctx, req)
	if err == nil {
		t.Fatalf("expected error for non-2xx response")
	}
	if !strings.Contains(err.Error(), "response body: sensitive details") {
		t.Fatalf("expected response body in error message, got %q", err.Error())
	}
}

type mockResolver struct {
	lookupFunc func(ctx context.Context, host string) ([]string, error)
}

func (m *mockResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	return m.lookupFunc(ctx, host)
}

func mustParseCIDRs(t *testing.T, list []string) []*net.IPNet {
	t.Helper()
	nets, err := parseCIDRs(list)
	if err != nil {
		t.Fatalf("failed to parse CIDRs: %v", err)
	}
	return nets
}

func TestParseCIDRs(t *testing.T) {
	input := []string{
		"127.0.0.1",
		"10.0.0.0/8",
		"  ",
		"192.168.1.1",
	}
	nets, err := parseCIDRs(input)
	if err != nil {
		t.Fatalf("unexpected error parsing valid CIDRs: %v", err)
	}
	// Test invalid CIDR error reporting
	_, err = parseCIDRs([]string{"invalid-ip"})
	if err == nil {
		t.Error("expected error for invalid CIDR entry, got nil")
	}

	if len(nets) != 3 {
		t.Fatalf("expected 3 parsed networks, got %d", len(nets))
	}
	// Verify parsing contains correct ranges
	if !nets[0].Contains(net.ParseIP("127.0.0.1")) {
		t.Error("expected nets[0] to contain 127.0.0.1")
	}
	if !nets[1].Contains(net.ParseIP("10.1.2.3")) {
		t.Error("expected nets[1] to contain 10.1.2.3")
	}
	if !nets[2].Contains(net.ParseIP("192.168.1.1")) {
		t.Error("expected nets[2] to contain 192.168.1.1")
	}
}

func TestSSRFGuard(t *testing.T) {
	guard := &SSRFGuard{
		AllowedRanges:        mustParseCIDRs(t, []string{"10.0.0.1"}),
		CustomBlocked:        mustParseCIDRs(t, []string{"192.168.1.1"}),
		AllowPrivateNetworks: false,
	}

	tcs := []struct {
		desc string
		ip   net.IP
		want bool // true if blocked
	}{
		{
			desc: "Public IP allowed",
			ip:   net.ParseIP("8.8.8.8"),
			want: false,
		},
		{
			desc: "Loopback IP blocked",
			ip:   net.ParseIP("127.0.0.1"),
			want: true,
		},
		{
			desc: "Private IP blocked",
			ip:   net.ParseIP("10.0.0.2"),
			want: true,
		},
		{
			desc: "Custom blocked IP blocked",
			ip:   net.ParseIP("192.168.1.1"),
			want: true,
		},
		{
			desc: "Custom allowed IP override allowed",
			ip:   net.ParseIP("10.0.0.1"),
			want: false,
		},
		{
			desc: "CGNAT shared address space blocked",
			ip:   net.ParseIP("100.64.0.1"),
			want: true,
		},
		{
			desc: "CGNAT shared address space upper bound blocked",
			ip:   net.ParseIP("100.127.255.255"),
			want: true,
		},
		{
			desc: "IP just below CGNAT range allowed",
			ip:   net.ParseIP("100.63.255.255"),
			want: false,
		},
		{
			desc: "IP just above CGNAT range allowed",
			ip:   net.ParseIP("100.128.0.0"),
			want: false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got := guard.IsIPBlocked(tc.ip)
			if got != tc.want {
				t.Errorf("IsIPBlocked(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}

	// Test with AllowPrivateNetworks = true
	guardPrivate := &SSRFGuard{
		AllowedRanges:        mustParseCIDRs(t, []string{"10.0.0.1"}),
		CustomBlocked:        mustParseCIDRs(t, []string{"192.168.1.1"}),
		AllowPrivateNetworks: true,
	}
	if guardPrivate.IsIPBlocked(net.ParseIP("127.0.0.1")) {
		t.Error("expected loopback to be allowed when AllowPrivateNetworks is true")
	}
	if guardPrivate.IsIPBlocked(net.ParseIP("10.0.0.2")) {
		t.Error("expected private IP to be allowed when AllowPrivateNetworks is true")
	}
	if !guardPrivate.IsIPBlocked(net.ParseIP("192.168.1.1")) {
		t.Error("expected custom blocked IP to remain blocked when AllowPrivateNetworks is true")
	}
	if guardPrivate.IsIPBlocked(net.ParseIP("100.64.0.1")) {
		t.Error("expected CGNAT IP to be allowed when AllowPrivateNetworks is true")
	}

	// Test that an explicit allowedIpRanges override lets a CGNAT IP through,
	// preserving the whitelist precedence over the default CGNAT block.
	guardAllowCGNAT := &SSRFGuard{
		AllowedRanges:        mustParseCIDRs(t, []string{"100.64.0.0/10"}),
		AllowPrivateNetworks: false,
	}
	if guardAllowCGNAT.IsIPBlocked(net.ParseIP("100.64.0.1")) {
		t.Error("expected CGNAT IP to be allowed when explicitly whitelisted via AllowedRanges")
	}
}

func TestSecureDialContextAndCheckRedirect(t *testing.T) {
	// Setup mock resolver that points:
	// safe.example.com -> 8.8.8.8
	// unsafe.example.com -> 127.0.0.1
	// rebind.example.com -> 127.0.0.1, 8.8.8.8
	mockRes := &mockResolver{
		lookupFunc: func(ctx context.Context, host string) ([]string, error) {
			switch host {
			case "safe.example.com":
				return []string{"8.8.8.8"}, nil
			case "unsafe.example.com":
				return []string{"127.0.0.1"}, nil
			case "rebind.example.com":
				return []string{"127.0.0.1", "8.8.8.8"}, nil
			default:
				return []string{"127.0.0.1"}, nil
			}
		},
	}

	config := Config{
		CustomBlockedIPRanges: []string{"192.168.1.1"},
		AllowedIPRanges:       []string{"10.0.0.1"},
		AllowPrivateNetworks:  false,
	}

	guard := &SSRFGuard{
		AllowPrivateNetworks: config.AllowPrivateNetworks,
		AllowedRanges:        mustParseCIDRs(t, config.AllowedIPRanges),
		CustomBlocked:        mustParseCIDRs(t, config.CustomBlockedIPRanges),
		Resolver:             mockRes,
	}

	// 1. Unsafe host should fail early in CheckRedirect
	tr := &nethttp.Transport{}
	client, err := createHTTPClient(5*time.Second, tr, guard, mockRes)
	if err != nil {
		t.Fatalf("failed to create secure HTTP client: %v", err)
	}

	req, _ := nethttp.NewRequest("GET", "http://unsafe.example.com/some-path", nil)
	viaReq, _ := nethttp.NewRequest("GET", "http://safe.example.com/original-path", nil)
	err = client.CheckRedirect(req, []*nethttp.Request{viaReq})
	if err == nil {
		t.Error("expected CheckRedirect to fail for unsafe.example.com")
	} else if !strings.Contains(err.Error(), "redirect host unsafe.example.com resolves to blocked IP") {
		t.Errorf("unexpected error message: %v", err)
	}

	// 2. Safe host should pass CheckRedirect
	reqSafe, _ := nethttp.NewRequest("GET", "http://safe.example.com/some-path", nil)
	err = client.CheckRedirect(reqSafe, []*nethttp.Request{viaReq})
	if err != nil {
		t.Errorf("expected CheckRedirect to pass for safe.example.com, got: %v", err)
	}

	// 3. Rebinding host check (with multiple IPs)
	// It has one unsafe IP (127.0.0.1) and one safe IP (8.8.8.8).
	// Under secure design, if any IP is unsafe, it must be denied.
	reqRebind, _ := nethttp.NewRequest("GET", "http://rebind.example.com/some-path", nil)
	err = client.CheckRedirect(reqRebind, []*nethttp.Request{viaReq})
	if err == nil {
		t.Error("expected CheckRedirect to fail for rebind.example.com (since 127.0.0.1 is blocked)")
	}

	// 4. Test DialContext on dialer via raw IP connection
	dialFn := tr.DialContext
	if dialFn == nil {
		t.Fatal("expected DialContext to be configured on Transport")
	}

	// Dialing unsafe loopback IP directly should fail at TCP connection hook layer
	ctx := context.Background()
	_, err = dialFn(ctx, "tcp", "127.0.0.1:80")
	if err == nil {
		t.Error("expected dial to fail for loopback IP 127.0.0.1")
	} else if !strings.Contains(err.Error(), "denied") && !strings.Contains(err.Error(), "blocked") {
		t.Errorf("unexpected dial error: %v", err)
	}
}

func TestRedirectLoopbackIntegration(t *testing.T) {
	// Create a redirect server that redirects to a private IP (10.0.0.1)
	redirectServer := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		nethttp.Redirect(w, r, "http://10.0.0.1/", nethttp.StatusMovedPermanently)
	}))
	defer redirectServer.Close()

	// 1. With SSRF protection enabled (default), redirecting to private IP must fail.
	// We allow 127.0.0.1 and ::1 in AllowedIPRanges so the initial connection to the local redirect server succeeds.
	guardDefault := &SSRFGuard{
		AllowPrivateNetworks: false,
		AllowedRanges:        mustParseCIDRs(t, []string{"127.0.0.1", "::1"}),
	}
	trDefault := &nethttp.Transport{}
	clientDefault, err := createHTTPClient(5*time.Second, trDefault, guardDefault, nil)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = clientDefault.Get(redirectServer.URL)
	if err == nil {
		t.Error("expected request to fail due to redirect SSRF protection")
	} else if !strings.Contains(err.Error(), "redirect to blocked IP") {
		t.Errorf("expected SSRF blocked message, got error: %v", err)
	}

	// 2. With SSRF protection bypass (AllowPrivateNetworks = true), it should follow the redirect
	// (it will fail to connect to 10.0.0.1, but the redirect itself should be followed).
	guardBypass := &SSRFGuard{
		AllowPrivateNetworks: true,
		AllowedRanges:        mustParseCIDRs(t, []string{"127.0.0.1", "::1"}),
	}
	trBypass := &nethttp.Transport{}
	clientBypass, err := createHTTPClient(5*time.Second, trBypass, guardBypass, nil)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = clientBypass.Get(redirectServer.URL)
	if err == nil {
		t.Fatal("expected request to fail connecting to 10.0.0.1")
	}
	// The error should be a connection/dial error to 10.0.0.1, not a redirect blocked error!
	if strings.Contains(err.Error(), "SSRF mitigation: redirect") {
		t.Errorf("unexpected redirect block error: %v", err)
	}
}
