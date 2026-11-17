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

package internal

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/safetext/yamltemplate"
	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/mcp-toolbox/internal/prompts/custom"
	httpsrc "github.com/googleapis/mcp-toolbox/internal/sources/http"
	pgsrc "github.com/googleapis/mcp-toolbox/internal/sources/postgres"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	httptool "github.com/googleapis/mcp-toolbox/internal/tools/http"
	"google.golang.org/api/option"
)

type fakeSecretManagerClient struct {
	secrets  map[string]*secretmanagerpb.SecretPayload
	accessed []string
	closed   bool
}

func (f *fakeSecretManagerClient) AccessSecretVersion(_ context.Context, req *secretmanagerpb.AccessSecretVersionRequest, _ ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error) {
	f.accessed = append(f.accessed, req.GetName())
	payload, ok := f.secrets[req.GetName()]
	if !ok {
		return nil, fmt.Errorf("secret version %q not found", req.GetName())
	}
	return &secretmanagerpb.AccessSecretVersionResponse{
		Name:    req.GetName(),
		Payload: payload,
	}, nil
}

func (f *fakeSecretManagerClient) Close() error {
	f.closed = true
	return nil
}

func makeSecretPayload(val string) *secretmanagerpb.SecretPayload {
	return makeSecretPayloadBytes([]byte(val))
}

func makeSecretPayloadBytes(data []byte) *secretmanagerpb.SecretPayload {
	crc := int64(crc32.Checksum(data, crc32.MakeTable(crc32.Castagnoli)))
	return &secretmanagerpb.SecretPayload{
		Data:       data,
		DataCrc32C: &crc,
	}
}

func TestParseInlineSecretRef(t *testing.T) {
	tcs := []struct {
		desc    string
		input   string
		want    SecretRef
		wantErr string
	}{
		{
			desc:  "double-quoted latest alias",
			input: `"//secretmanager.googleapis.com/projects/project1/secrets/pass/versions/latest"`,
			want: SecretRef{
				Project:  "project1",
				SecretID: "pass",
				Version:  "latest",
			},
		},
		{
			desc:  "escaped double-quoted reference from double-quoted YAML scalar",
			input: `\"//secretmanager.googleapis.com/projects/project1/secrets/pass/versions/3\"`,
			want: SecretRef{
				Project:  "project1",
				SecretID: "pass",
				Version:  "3",
			},
		},
		{
			desc:  "single-quoted cross-project reference with surrounding spaces",
			input: ` '//secretmanager.googleapis.com/projects/project3/secrets/api-key/versions/prod-alias' `,
			want: SecretRef{
				Project:  "project3",
				SecretID: "api-key",
				Version:  "prod-alias",
			},
		},
		{
			desc:  "unquoted reference",
			input: `//secretmanager.googleapis.com/projects/project1/secrets/pass/versions/1`,
			want: SecretRef{
				Project:  "project1",
				SecretID: "pass",
				Version:  "1",
			},
		},
		{
			desc:  "regional secret reference",
			input: `"//secretmanager.googleapis.com/projects/project1/locations/us-central1/secrets/pass/versions/latest"`,
			want: SecretRef{
				Project:  "project1",
				Location: "us-central1",
				SecretID: "pass",
				Version:  "latest",
			},
		},
		{
			desc:  "domain-scoped project regional reference",
			input: `'//secretmanager.googleapis.com/projects/example.com:proj-1/locations/europe-west1/secrets/db_pass/versions/2'`,
			want: SecretRef{
				Project:  "example.com:proj-1",
				Location: "europe-west1",
				SecretID: "db_pass",
				Version:  "2",
			},
		},
		{
			desc:    "locations/global is rejected",
			input:   `"//secretmanager.googleapis.com/projects/project1/locations/global/secrets/pass/versions/latest"`,
			wantErr: `global secrets must omit '/locations/global'`,
		},
		{
			desc:    "whitespace inside resource segment is rejected",
			input:   `"//secretmanager.googleapis.com/projects/my project/secrets/pass/versions/latest"`,
			wantErr: `invalid secret reference`,
		},
		{
			desc:    "query string in version segment is rejected",
			input:   `"//secretmanager.googleapis.com/projects/project1/secrets/pass/versions/latest?foo=bar"`,
			wantErr: `invalid secret reference`,
		},
		{
			desc:    "empty reference",
			input:   `   `,
			wantErr: `empty secret reference in __REF__()`,
		},
		{
			desc:    "empty quoted reference",
			input:   `""`,
			wantErr: `empty secret reference in __REF__("")`,
		},
		{
			desc:    "mismatched quotes",
			input:   `"//secretmanager.googleapis.com/projects/project1/secrets/pass/versions/latest'`,
			wantErr: `mismatched quotes in secret reference`,
		},
		{
			desc:    "missing closing quote",
			input:   `"//secretmanager.googleapis.com/projects/project1/secrets/pass/versions/latest`,
			wantErr: `mismatched quotes in secret reference`,
		},
		{
			desc:    "missing versions segment",
			input:   `"//secretmanager.googleapis.com/projects/project1/secrets/pass"`,
			wantErr: `invalid secret reference "//secretmanager.googleapis.com/projects/project1/secrets/pass"`,
		},
		{
			desc:    "missing secretmanager.googleapis.com prefix",
			input:   `"projects/project1/secrets/pass/versions/latest"`,
			wantErr: `invalid secret reference "projects/project1/secrets/pass/versions/latest"`,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := ParseInlineSecretRef(tc.input)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ParseInlineSecretRef mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSecretRefResourceNameAndRegionalEndpoint(t *testing.T) {
	globalRef := SecretRef{Project: "p1", SecretID: "s1", Version: "latest"}
	if got, want := globalRef.ResourceName(), "projects/p1/secrets/s1/versions/latest"; got != want {
		t.Errorf("global ResourceName() = %q, want %q", got, want)
	}

	regionalRef := SecretRef{Project: "p1", Location: "us-central1", SecretID: "s1", Version: "3"}
	if got, want := regionalRef.ResourceName(), "projects/p1/locations/us-central1/secrets/s1/versions/3"; got != want {
		t.Errorf("regional ResourceName() = %q, want %q", got, want)
	}

	if got, want := regionalSecretManagerEndpoint("us-central1"), "secretmanager.us-central1.rep.googleapis.com:443"; got != want {
		t.Errorf("regionalSecretManagerEndpoint() = %q, want %q", got, want)
	}
}

func TestParseConfig_InlineSecretResolutionWithSafetext(t *testing.T) {
	clientsByLocation := map[string]*fakeSecretManagerClient{
		"": {
			secrets: map[string]*secretmanagerpb.SecretPayload{
				"projects/project1/secrets/pass/versions/latest": makeSecretPayload("latest-pass"),
				"projects/project1/secrets/pass/versions/3":      makeSecretPayload("v3-pass"),
				"projects/project3/secrets/pass/versions/latest": makeSecretPayload(`p#ss: "with" {braces}`),
				"projects/project1/secrets/num/versions/1":       makeSecretPayload("123456"),
			},
		},
		"us-central1": {
			secrets: map[string]*secretmanagerpb.SecretPayload{
				"projects/project1/locations/us-central1/secrets/pass/versions/latest": makeSecretPayload("regional-usc1-pass"),
			},
		},
		"europe-west1": {
			secrets: map[string]*secretmanagerpb.SecretPayload{
				"projects/project2/locations/europe-west1/secrets/api-key/versions/2": makeSecretPayload("regional-euw1-key"),
			},
		},
	}
	var initLocations []string
	restore := SetSecretManagerClientFactoryForTest(func(_ context.Context, location string, _ ...option.ClientOption) (SecretManagerClient, error) {
		initLocations = append(initLocations, location)
		c, ok := clientsByLocation[location]
		if !ok {
			return nil, fmt.Errorf("unexpected client location %q", location)
		}
		return c, nil
	})
	t.Cleanup(restore)

	t.Setenv("PROJECT_ID", "project1")
	t.Setenv("EMBEDDED_REF", `__REF__("//secretmanager.googleapis.com/projects/project1/secrets/pass/versions/3")`)

	yamlInput := `
# Commented out ref should be ignored: __REF__("//secretmanager.googleapis.com/projects/nonexistent/secrets/nope/versions/1")
sources:
  pg-default:
    kind: postgres
    host: 127.0.0.1
    port: 5432
    database: flights
    user: app_user # inline comment __REF__("//secretmanager.googleapis.com/projects/nonexistent/secrets/nope/versions/2")
    password: __REF__("//secretmanager.googleapis.com/projects/${PROJECT_ID}/secrets/pass/versions/latest")
  pg-dedup:
    kind: postgres
    host: 127.0.0.2
    port: 5432
    database: flights
    user: app_user
    password: __REF__ ( '//secretmanager.googleapis.com/projects/project1/secrets/pass/versions/latest' )
  pg-from-env:
    kind: postgres
    host: 127.0.0.3
    port: 5432
    database: flights
    user: app_user
    password: ${EMBEDDED_REF}
  pg-quoted-special-chars:
    kind: postgres
    host: 127.0.0.4
    port: 5432
    database: flights
    user: "__REF__(\"//secretmanager.googleapis.com/projects/project1/secrets/num/versions/1\")"
    password: '__REF__("//secretmanager.googleapis.com/projects/project3/secrets/pass/versions/latest")'
  pg-regional:
    kind: postgres
    host: 127.0.0.5
    port: 5432
    database: flights
    user: app_user
    password: __REF__("//secretmanager.googleapis.com/projects/project1/locations/us-central1/secrets/pass/versions/latest")
  pg-regional-dedup:
    kind: postgres
    host: 127.0.0.6
    port: 5432
    database: flights
    user: app_user
    password: '__REF__("//secretmanager.googleapis.com/projects/project1/locations/us-central1/secrets/pass/versions/latest")'
  api-source:
    kind: http
    baseUrl: https://api.example.com/
    headers:
      Authorization: Bearer __REF__("//secretmanager.googleapis.com/projects/project1/secrets/pass/versions/3")
      X-Regional-Key: __REF__("//secretmanager.googleapis.com/projects/project2/locations/europe-west1/secrets/api-key/versions/2")
tools:
  get_item:
    kind: http
    source: api-source
    method: GET
    path: items/{{.item_id}}
    description: Fetch item by ID
prompts:
  code_review:
    description: Review code
    messages:
      - content: "Review this snippet: {{.code}}"
    arguments:
      - name: code
        description: Code to review
`

	parser := &ConfigParser{}
	cfg, err := parser.ParseConfig(context.Background(), []byte(yamlInput))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	pgDefault, ok := cfg.Sources["pg-default"].(pgsrc.Config)
	if !ok {
		t.Fatalf("expected pgsrc.Config for pg-default, got %T", cfg.Sources["pg-default"])
	}
	if pgDefault.Password != "latest-pass" {
		t.Errorf("pg-default password = %q, want %q", pgDefault.Password, "latest-pass")
	}

	pgDedup, ok := cfg.Sources["pg-dedup"].(pgsrc.Config)
	if !ok {
		t.Fatalf("expected pgsrc.Config for pg-dedup, got %T", cfg.Sources["pg-dedup"])
	}
	if pgDedup.Password != "latest-pass" {
		t.Errorf("pg-dedup password = %q, want %q", pgDedup.Password, "latest-pass")
	}

	pgFromEnv, ok := cfg.Sources["pg-from-env"].(pgsrc.Config)
	if !ok {
		t.Fatalf("expected pgsrc.Config for pg-from-env, got %T", cfg.Sources["pg-from-env"])
	}
	if pgFromEnv.Password != "v3-pass" {
		t.Errorf("pg-from-env password = %q, want %q", pgFromEnv.Password, "v3-pass")
	}

	pgSpecial, ok := cfg.Sources["pg-quoted-special-chars"].(pgsrc.Config)
	if !ok {
		t.Fatalf("expected pgsrc.Config for pg-quoted-special-chars, got %T", cfg.Sources["pg-quoted-special-chars"])
	}
	if pgSpecial.User != "123456" {
		t.Errorf("pg-quoted-special-chars user = %q, want %q", pgSpecial.User, "123456")
	}
	wantSpecial := `p#ss: "with" {braces}`
	if pgSpecial.Password != wantSpecial {
		t.Errorf("pg-quoted-special-chars password = %q, want %q", pgSpecial.Password, wantSpecial)
	}

	pgRegional, ok := cfg.Sources["pg-regional"].(pgsrc.Config)
	if !ok {
		t.Fatalf("expected pgsrc.Config for pg-regional, got %T", cfg.Sources["pg-regional"])
	}
	if pgRegional.Password != "regional-usc1-pass" {
		t.Errorf("pg-regional password = %q, want %q", pgRegional.Password, "regional-usc1-pass")
	}

	pgRegionalDedup, ok := cfg.Sources["pg-regional-dedup"].(pgsrc.Config)
	if !ok {
		t.Fatalf("expected pgsrc.Config for pg-regional-dedup, got %T", cfg.Sources["pg-regional-dedup"])
	}
	if pgRegionalDedup.Password != "regional-usc1-pass" {
		t.Errorf("pg-regional-dedup password = %q, want %q", pgRegionalDedup.Password, "regional-usc1-pass")
	}

	httpCfg, ok := cfg.Sources["api-source"].(httpsrc.Config)
	if !ok {
		t.Fatalf("expected httpsrc.Config for api-source, got %T", cfg.Sources["api-source"])
	}
	if got := httpCfg.DefaultHeaders["Authorization"]; got != "Bearer v3-pass" {
		t.Errorf("Authorization header = %q, want %q", got, "Bearer v3-pass")
	}
	if got := httpCfg.DefaultHeaders["X-Regional-Key"]; got != "regional-euw1-key" {
		t.Errorf("X-Regional-Key header = %q, want %q", got, "regional-euw1-key")
	}

	// Verify existing {{.param}} Go templates in HTTP tools and prompts are preserved untouched.
	toolCfg, ok := cfg.Tools["get_item"].(httptool.Config)
	if !ok {
		t.Fatalf("expected httptool.Config for get_item, got %T", cfg.Tools["get_item"])
	}
	if toolCfg.Path != "items/{{.item_id}}" {
		t.Errorf("get_item path = %q, want %q", toolCfg.Path, "items/{{.item_id}}")
	}

	promptCfg, ok := cfg.Prompts["code_review"].(*custom.Config)
	if !ok {
		t.Fatalf("expected *custom.Config for code_review, got %T", cfg.Prompts["code_review"])
	}
	if len(promptCfg.Messages) != 1 || promptCfg.Messages[0].Content != "Review this snippet: {{.code}}" {
		t.Errorf("code_review prompt messages = %+v, want {{.code}} preserved", promptCfg.Messages)
	}

	// Verify one client initialized per unique location ("", "us-central1", "europe-west1").
	wantLocations := []string{"", "us-central1", "europe-west1"}
	if diff := cmp.Diff(wantLocations, initLocations); diff != "" {
		t.Errorf("initialized client locations mismatch (-want +got):\n%s", diff)
	}

	// Verify deduplication across global and regional clients, and verify all clients are closed.
	wantGlobalAccessed := []string{
		"projects/project1/secrets/pass/versions/latest",
		"projects/project1/secrets/pass/versions/3",
		"projects/project1/secrets/num/versions/1",
		"projects/project3/secrets/pass/versions/latest",
	}
	if diff := cmp.Diff(wantGlobalAccessed, clientsByLocation[""].accessed); diff != "" {
		t.Errorf("global accessed secrets mismatch (-want +got):\n%s", diff)
	}
	wantUSC1Accessed := []string{
		"projects/project1/locations/us-central1/secrets/pass/versions/latest",
	}
	if diff := cmp.Diff(wantUSC1Accessed, clientsByLocation["us-central1"].accessed); diff != "" {
		t.Errorf("us-central1 accessed secrets mismatch (-want +got):\n%s", diff)
	}
	wantEUW1Accessed := []string{
		"projects/project2/locations/europe-west1/secrets/api-key/versions/2",
	}
	if diff := cmp.Diff(wantEUW1Accessed, clientsByLocation["europe-west1"].accessed); diff != "" {
		t.Errorf("europe-west1 accessed secrets mismatch (-want +got):\n%s", diff)
	}
	for loc, c := range clientsByLocation {
		if !c.closed {
			t.Errorf("expected Secret Manager client for location %q to be closed", loc)
		}
	}
}

func TestParseConfig_SafetextPreventsYAMLInjection(t *testing.T) {
	tcs := []struct {
		desc          string
		yamlTemplate  string
		secretPayload string
	}{
		{
			desc: "unquoted colon-space map injection",
			yamlTemplate: `
sources:
  pg1:
    kind: postgres
    host: 127.0.0.1
    port: 5432
    database: db
    user: u
    password: __REF__("//secretmanager.googleapis.com/projects/p1/secrets/s1/versions/1")
`,
			secretPayload: "yaml: injected",
		},
		{
			desc: "unquoted newline key injection",
			yamlTemplate: `
sources:
  pg1:
    kind: postgres
    host: 127.0.0.1
    port: 5432
    database: db
    user: u
    password: __REF__("//secretmanager.googleapis.com/projects/p1/secrets/s1/versions/1")
`,
			secretPayload: "secret\n    injectedKey: evil",
		},
		{
			desc: "unquoted flow map injection",
			yamlTemplate: `
sources:
  pg1:
    kind: postgres
    host: 127.0.0.1
    port: 5432
    database: db
    user: u
    password: __REF__("//secretmanager.googleapis.com/projects/p1/secrets/s1/versions/1")
`,
			secretPayload: "{injected: map}",
		},
		{
			desc: "unquoted flow sequence injection",
			yamlTemplate: `
sources:
  pg1:
    kind: postgres
    host: 127.0.0.1
    port: 5432
    database: db
    user: u
    password: __REF__("//secretmanager.googleapis.com/projects/p1/secrets/s1/versions/1")
`,
			secretPayload: "[injected1, injected2]",
		},
		{
			desc: "single-quote breakout key injection",
			yamlTemplate: `
sources:
  pg1:
    kind: postgres
    host: 127.0.0.1
    port: 5432
    database: db
    user: u
    password: '__REF__("//secretmanager.googleapis.com/projects/p1/secrets/s1/versions/1")'
`,
			secretPayload: "pass'\n    injectedKey: 'evil",
		},
		{
			desc: "double-quote breakout key injection",
			yamlTemplate: `
sources:
  pg1:
    kind: postgres
    host: 127.0.0.1
    port: 5432
    database: db
    user: u
    password: "__REF__(\"//secretmanager.googleapis.com/projects/p1/secrets/s1/versions/1\")"
`,
			secretPayload: "pass\"\n    injectedKey: \"evil",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			restore := SetSecretManagerClientFactoryForTest(func(context.Context, string, ...option.ClientOption) (SecretManagerClient, error) {
				return &fakeSecretManagerClient{
					secrets: map[string]*secretmanagerpb.SecretPayload{
						"projects/p1/secrets/s1/versions/1": makeSecretPayload(tc.secretPayload),
					},
				}, nil
			})
			t.Cleanup(restore)

			parser := &ConfigParser{}
			_, err := parser.ParseConfig(context.Background(), []byte(tc.yamlTemplate))
			if err == nil {
				t.Fatalf("expected ErrYAMLInjection error, got nil")
			}
			if !errors.Is(err, yamltemplate.ErrYAMLInjection) {
				t.Fatalf("expected error wrapping yamltemplate.ErrYAMLInjection, got: %v", err)
			}
			if strings.Contains(err.Error(), tc.secretPayload) {
				t.Fatalf("error message leaked secret payload: %v", err)
			}
		})
	}
}

func TestParseConfig_SecretResolutionFailFastErrors(t *testing.T) {
	t.Run("missing secret fails fast", func(t *testing.T) {
		restore := SetSecretManagerClientFactoryForTest(func(context.Context, string, ...option.ClientOption) (SecretManagerClient, error) {
			return &fakeSecretManagerClient{secrets: map[string]*secretmanagerpb.SecretPayload{}}, nil
		})
		t.Cleanup(restore)

		yamlInput := `
kind: source
name: pg1
type: postgres
host: 127.0.0.1
port: 5432
database: db
user: u
password: __REF__("//secretmanager.googleapis.com/projects/p1/secrets/missing/versions/latest")
`
		parser := &ConfigParser{}
		_, err := parser.ParseConfig(context.Background(), []byte(yamlInput))
		if err == nil || !strings.Contains(err.Error(), `failed to access secret "projects/p1/secrets/missing/versions/latest"`) {
			t.Fatalf("expected missing secret error, got: %v", err)
		}
	})

	t.Run("corrupted CRC32C checksum fails fast", func(t *testing.T) {
		badCRC := int64(99999)
		restore := SetSecretManagerClientFactoryForTest(func(context.Context, string, ...option.ClientOption) (SecretManagerClient, error) {
			return &fakeSecretManagerClient{
				secrets: map[string]*secretmanagerpb.SecretPayload{
					"projects/p1/secrets/corrupt/versions/1": {
						Data:       []byte("secret-data"),
						DataCrc32C: &badCRC,
					},
				},
			}, nil
		})
		t.Cleanup(restore)

		yamlInput := `
kind: source
name: pg1
type: postgres
host: 127.0.0.1
port: 5432
database: db
user: u
password: __REF__("//secretmanager.googleapis.com/projects/p1/secrets/corrupt/versions/1")
`
		parser := &ConfigParser{}
		_, err := parser.ParseConfig(context.Background(), []byte(yamlInput))
		if err == nil || !strings.Contains(err.Error(), "failed CRC32C integrity check") {
			t.Fatalf("expected CRC32C integrity error, got: %v", err)
		}
		if strings.Contains(err.Error(), "secret-data") {
			t.Fatalf("error message leaked secret payload: %v", err)
		}
	})

	t.Run("invalid UTF-8 secret payload fails fast", func(t *testing.T) {
		restore := SetSecretManagerClientFactoryForTest(func(context.Context, string, ...option.ClientOption) (SecretManagerClient, error) {
			return &fakeSecretManagerClient{
				secrets: map[string]*secretmanagerpb.SecretPayload{
					"projects/p1/secrets/invalid-utf8/versions/1": makeSecretPayloadBytes([]byte{0xe2, 0x8c}),
				},
			}, nil
		})
		t.Cleanup(restore)

		yamlInput := `
kind: source
name: pg1
type: postgres
host: 127.0.0.1
port: 5432
database: db
user: u
password: __REF__("//secretmanager.googleapis.com/projects/p1/secrets/invalid-utf8/versions/1")
`
		parser := &ConfigParser{}
		_, err := parser.ParseConfig(context.Background(), []byte(yamlInput))
		if err == nil || !strings.Contains(err.Error(), "not valid UTF-8") {
			t.Fatalf("expected invalid UTF-8 error, got: %v", err)
		}
	})

	t.Run("nil secret payload fails fast", func(t *testing.T) {
		restore := SetSecretManagerClientFactoryForTest(func(context.Context, string, ...option.ClientOption) (SecretManagerClient, error) {
			return &fakeSecretManagerClient{
				secrets: map[string]*secretmanagerpb.SecretPayload{
					"projects/p1/secrets/empty/versions/1": nil,
				},
			}, nil
		})
		t.Cleanup(restore)

		yamlInput := `
kind: source
name: pg1
type: postgres
host: 127.0.0.1
port: 5432
database: db
user: u
password: __REF__("//secretmanager.googleapis.com/projects/p1/secrets/empty/versions/1")
`
		parser := &ConfigParser{}
		_, err := parser.ParseConfig(context.Background(), []byte(yamlInput))
		if err == nil || !strings.Contains(err.Error(), "returned an empty payload") {
			t.Fatalf("expected empty payload error, got: %v", err)
		}
	})

	t.Run("client initialization error fails fast", func(t *testing.T) {
		restore := SetSecretManagerClientFactoryForTest(func(context.Context, string, ...option.ClientOption) (SecretManagerClient, error) {
			return nil, fmt.Errorf("ADC credentials unavailable")
		})
		t.Cleanup(restore)

		yamlInput := `
kind: source
name: pg1
type: postgres
host: 127.0.0.1
port: 5432
database: db
user: u
password: __REF__("//secretmanager.googleapis.com/projects/p1/secrets/pass/versions/1")
`
		parser := &ConfigParser{}
		_, err := parser.ParseConfig(context.Background(), []byte(yamlInput))
		if err == nil || !strings.Contains(err.Error(), "failed to initialize Secret Manager client") {
			t.Fatalf("expected client initialization error, got: %v", err)
		}
	})

	t.Run("unclosed __REF__ fails fast", func(t *testing.T) {
		yamlInput := `
kind: source
name: pg1
type: postgres
host: 127.0.0.1
port: 5432
database: db
user: u
password: '__REF__("//secretmanager.googleapis.com/projects/p1/secrets/pass/versions/1"'
`
		parser := &ConfigParser{}
		_, err := parser.ParseConfig(context.Background(), []byte(yamlInput))
		if err == nil || !strings.Contains(err.Error(), "unclosed __REF__(...) secret reference") {
			t.Fatalf("expected unclosed __REF__ error, got: %v", err)
		}
	})

	t.Run("malformed __REF__ URI fails fast", func(t *testing.T) {
		yamlInput := `
kind: source
name: pg1
type: postgres
host: 127.0.0.1
port: 5432
database: db
user: u
password: __REF__("invalid-uri")
`
		parser := &ConfigParser{}
		_, err := parser.ParseConfig(context.Background(), []byte(yamlInput))
		if err == nil || !strings.Contains(err.Error(), `invalid secret reference "invalid-uri"`) {
			t.Fatalf("expected invalid secret reference error, got: %v", err)
		}
	})
}

func TestOfflineAndMigrateDoNotFetchSecrets(t *testing.T) {
	clientCalled := false
	restore := SetSecretManagerClientFactoryForTest(func(context.Context, string, ...option.ClientOption) (SecretManagerClient, error) {
		clientCalled = true
		return nil, fmt.Errorf("Secret Manager client should not be called")
	})
	t.Cleanup(restore)

	nestedYAML := `sources:
  pg1:
    kind: postgres
    host: 127.0.0.1
    port: 5432
    database: db
    user: u
    password: __REF__("//secretmanager.googleapis.com/projects/p1/secrets/pass/versions/latest")
`

	// 1. ConvertConfig (used by 'toolbox migrate') preserves __REF__(...) without fetching secrets.
	converted, err := ConvertConfig(context.Background(), []byte(nestedYAML))
	if err != nil {
		t.Fatalf("ConvertConfig failed: %v", err)
	}
	if clientCalled {
		t.Fatalf("ConvertConfig unexpectedly called Secret Manager client")
	}
	if !strings.Contains(string(converted), `//secretmanager.googleapis.com/projects/p1/secrets/pass/versions/latest`) {
		t.Errorf("expected ConvertConfig output to retain secret reference, got:\n%s", string(converted))
	}

	// 2. AllowMissingEnvVars (used by offline 'toolbox skills-generate') skips Secret Manager RPCs.
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("ContextWithNewLogger failed: %v", err)
	}
	cfgPath := filepath.Join(t.TempDir(), "tools.yaml")
	if err := os.WriteFile(cfgPath, []byte(nestedYAML), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	opts := &ToolboxOptions{Config: cfgPath}
	offlineParser := &ConfigParser{AllowMissingEnvVars: true}
	if _, err := opts.LoadConfig(ctx, offlineParser); err != nil {
		t.Fatalf("LoadConfig in offline mode failed: %v", err)
	}
	if clientCalled {
		t.Fatalf("offline LoadConfig unexpectedly called Secret Manager client")
	}
}
