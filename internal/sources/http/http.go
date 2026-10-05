// Copyright 2025 Google LLC
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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "http"
const maxErrorBodyLogBytes = 1024

// cgnatRange is the RFC 6598 shared address space (100.64.0.0/10). It is not
// globally routable, so net.IP.IsPrivate reports false for it, but cloud
// providers and Kubernetes CNIs use it for internal node and Pod networking.
// The default SSRF guard treats it as private and blocks it.
var cgnatRange = mustParseCIDR("100.64.0.0/10")

// ietfProtocolAssignmentRange is the RFC 6890 "IETF Protocol Assignments"
// space (192.0.0.0/24). It is not globally routable, so net.IP.IsPrivate
// reports false for it, but it hosts special-purpose protocol machinery such
// as the NAT64/DNS64 discovery anycast addresses (RFC 8880) rather than
// ordinary endpoints. The default SSRF guard treats it as private and blocks it.
var ietfProtocolAssignmentRange = mustParseCIDR("192.0.0.0/24")

func mustParseCIDR(cidr string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("invalid CIDR %q: %v", cidr, err))
	}
	return ipNet
}

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name, Timeout: "30s"} // Default timeout
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type Config struct {
	Name                   string            `yaml:"name" validate:"required"`
	Type                   string            `yaml:"type" validate:"required"`
	BaseURL                string            `yaml:"baseUrl"`
	Timeout                string            `yaml:"timeout"`
	DefaultHeaders         map[string]string `yaml:"headers"`
	QueryParams            map[string]string `yaml:"queryParams"`
	ReturnFullError        bool              `yaml:"returnFullError"`
	DisableSslVerification bool              `yaml:"disableSslVerification"`
	AllowedIPRanges        []string          `yaml:"allowedIpRanges"`
	CustomBlockedIPRanges  []string          `yaml:"customBlockedIpRanges"`
	AllowPrivateNetworks   bool              `yaml:"allowPrivateNetworks"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

// Initialize initializes an HTTP Source instance.
func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	duration, err := time.ParseDuration(r.Timeout)
	if err != nil {
		return nil, fmt.Errorf("unable to parse Timeout string as time.Duration: %s", err)
	}

	var tr *http.Transport
	if defaultTr, ok := http.DefaultTransport.(*http.Transport); ok {
		tr = defaultTr.Clone()
	} else {
		tr = &http.Transport{}
	}

	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get logger from ctx: %s", err)
	}

	if r.DisableSslVerification {
		tr.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}

		logger.WarnContext(ctx, "WARNING: TLS certificate verification is skipped (InsecureSkipVerify: true) for HTTP source %s. This exposes all traffic for this source to Man-in-the-Middle (MITM) attacks. Do not use in production.", r.Name)
	}

	// Validate BaseURL
	parsedURL, err := url.ParseRequestURI(r.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse BaseUrl %v", err)
	}

	allowedRanges, err := parseCIDRs(r.AllowedIPRanges)
	if err != nil {
		return nil, fmt.Errorf("invalid allowedIpRanges: %w", err)
	}

	customBlocked, err := parseCIDRs(r.CustomBlockedIPRanges)
	if err != nil {
		return nil, fmt.Errorf("invalid customBlockedIpRanges: %w", err)
	}

	guard := &SSRFGuard{
		AllowPrivateNetworks: r.AllowPrivateNetworks,
		AllowedRanges:        allowedRanges,
		CustomBlocked:        customBlocked,
	}

	// Quick fast-fail check for direct IP configurations in the YAML
	if ip := net.ParseIP(parsedURL.Hostname()); ip != nil {
		if guard.IsIPBlocked(ip) {
			return nil, fmt.Errorf("invalid BaseURL %s: points to a blocked internal IP address", r.BaseURL)
		}
	}

	client, err := createHTTPClient(duration, tr, guard, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create secure HTTP client: %w", err)
	}

	ua, err := util.UserAgentFromContext(ctx)
	if err != nil {
		warnMsg := fmt.Sprintf("Error in User Agent retrieval: %s", err)
		logger.WarnContext(ctx, warnMsg)
	}
	if r.DefaultHeaders == nil {
		r.DefaultHeaders = make(map[string]string)
	}
	if existingUA, ok := r.DefaultHeaders["User-Agent"]; ok {
		ua = ua + " " + existingUA
	}
	r.DefaultHeaders["User-Agent"] = ua

	s := &Source{
		Config: r,
		client: client,
	}
	return s, nil

}

var _ sources.Source = &Source{}

type Source struct {
	Config
	client *http.Client
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) HttpDefaultHeaders() map[string]string {
	return s.DefaultHeaders
}

func (s *Source) HttpBaseURL() string {
	return s.BaseURL
}

func (s *Source) HttpQueryParams() map[string]string {
	return s.QueryParams
}

func (s *Source) Client() *http.Client {
	return s.client
}

func (s *Source) RunRequest(ctx context.Context, req *http.Request) (any, error) {
	// Make request and fetch response
	resp, err := s.Client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making HTTP request: %s", err)
	}
	defer resp.Body.Close()

	var body []byte
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if s.ReturnFullError {
			return nil, fmt.Errorf("unexpected status code: %d, response body: %s", resp.StatusCode, string(body))
		}

		logger, err := util.LoggerFromContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to get logger from ctx: %s", err)
		}
		logger.DebugContext(ctx, "http source upstream error", "status", resp.StatusCode, "body", truncateForLog(body, maxErrorBodyLogBytes))

		statusText := http.StatusText(resp.StatusCode)
		if statusText != "" {
			return nil, fmt.Errorf("unexpected status code: %d (%s)", resp.StatusCode, statusText)
		}
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var data any
	if err = json.Unmarshal(body, &data); err != nil {
		// if unable to unmarshal data, return result as string.
		return string(body), nil
	}
	return data, nil
}

func truncateForLog(body []byte, limit int) string {
	if limit <= 0 || len(body) == 0 {
		return ""
	}
	if len(body) <= limit {
		return string(body)
	}
	return fmt.Sprintf("%s...(%d bytes truncated)", string(body[:limit]), len(body)-limit)
}

type dnsResolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// SSRFGuard manages network boundaries for the HTTP client
type SSRFGuard struct {
	AllowPrivateNetworks bool
	AllowedRanges        []*net.IPNet
	CustomBlocked        []*net.IPNet
	Resolver             dnsResolver
}

func (g *SSRFGuard) IsIPBlocked(ip net.IP) bool {
	// Check explicit whitelist overrides first
	for _, r := range g.AllowedRanges {
		if r.Contains(ip) {
			return false
		}
	}

	// Check explicit custom blacklists
	for _, r := range g.CustomBlocked {
		if r.Contains(ip) {
			return true
		}
	}

	// Default strict RFC 1918 / Link-Local / Loopback protection
	if !g.AllowPrivateNetworks {
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || cgnatRange.Contains(ip) || ietfProtocolAssignmentRange.Contains(ip) {
			return true
		}
	}

	return false
}

func parseCIDRs(list []string) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	for _, entry := range list {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		// If it is a single IP address (does not contain a slash), we can append /32 or /128
		if !strings.Contains(entry, "/") {
			ip := net.ParseIP(entry)
			if ip != nil {
				if ip.To4() != nil {
					entry = entry + "/32"
				} else {
					entry = entry + "/128"
				}
			}
		}
		_, ipNet, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR or IP address %q: %w", entry, err)
		}
		nets = append(nets, ipNet)
	}
	return nets, nil
}

func createHTTPClient(duration time.Duration, tr *http.Transport, guard *SSRFGuard, res dnsResolver) (*http.Client, error) {
	if res != nil {
		guard.Resolver = res
	}

	resolver := guard.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip != nil {
				if guard.IsIPBlocked(ip) {
					return fmt.Errorf("connection to blocked IP %s denied", ip)
				}
			}
			return nil
		},
	}

	if r, ok := resolver.(*net.Resolver); ok {
		dialer.Resolver = r
	}

	tr.DialContext = dialer.DialContext

	client := &http.Client{
		Timeout:   duration,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}

			hostname := req.URL.Hostname()
			if ip := net.ParseIP(hostname); ip != nil {
				if guard.IsIPBlocked(ip) {
					return fmt.Errorf("redirect to blocked IP %s denied", ip)
				}
				return nil
			}

			addrs, err := resolver.LookupHost(req.Context(), hostname)
			if err != nil {
				return fmt.Errorf("failed to resolve redirect host %s: %w", hostname, err)
			}

			for _, addr := range addrs {
				if ip := net.ParseIP(addr); ip != nil {
					if guard.IsIPBlocked(ip) {
						return fmt.Errorf("redirect host %s resolves to blocked IP %s", hostname, addr)
					}
				}
			}

			return nil
		},
	}
	return client, nil
}
