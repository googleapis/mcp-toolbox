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

package bigquery_test

import (
	"context"
	"math/big"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestParseFromYamlBigQuery(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					Location:           "",
					WriteMode:          "allowed",
					MaxQueryResultRows: 50,
				},
			},
		},
		{
			desc: "all fields specified",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			location: asia
			writeMode: blocked
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					Location:           "asia",
					WriteMode:          "blocked",
					UseClientOAuth:     "",
					MaxQueryResultRows: 50,
				},
			},
		},
		{
			desc: "with readOnly true",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			readOnly: true
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					ReadOnly:           func() *bool { b := true; return &b }(),
					WriteMode:          "blocked",
					MaxQueryResultRows: 50,
				},
			},
		},
		{
			desc: "with readOnly false",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			readOnly: false
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					ReadOnly:           func() *bool { b := false; return &b }(),
					WriteMode:          "allowed",
					MaxQueryResultRows: 50,
				},
			},
		},
		{
			desc: "use client auth example",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			location: us
			useClientOAuth: "true"
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					Location:           "us",
					UseClientOAuth:     "true",
					WriteMode:          "allowed",
					MaxQueryResultRows: 50,
				},
			},
		},
		{
			desc: "with custom auth header name example",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			location: us
			useClientOAuth: X-Custom-Auth
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					Location:           "us",
					UseClientOAuth:     "X-Custom-Auth",
					WriteMode:          "allowed",
					MaxQueryResultRows: 50,
				},
			},
		},
		{
			desc: "use client auth with unquoted true",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			location: us
			useClientOAuth: true
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					Location:           "us",
					UseClientOAuth:     "true",
					WriteMode:          "allowed",
					MaxQueryResultRows: 50,
				},
			},
		},
		{
			desc: "use client auth with unquoted false",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			location: us
			useClientOAuth: false
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					Location:           "us",
					UseClientOAuth:     "false",
					WriteMode:          "allowed",
					MaxQueryResultRows: 50,
				},
			},
		},
		{
			desc: "quota project with client auth example",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			location: us
			useClientOAuth: true
			quotaProject: billing-project
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					Location:           "us",
					UseClientOAuth:     "true",
					QuotaProject:       "billing-project",
					WriteMode:          "allowed",
					MaxQueryResultRows: 50,
				},
			},
		},
		{
			desc: "with allowed datasets example",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			location: us
			allowedDatasets:
			- my_dataset
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					Location:           "us",
					AllowedDatasets:    []string{"my_dataset"},
					WriteMode:          "allowed",
					MaxQueryResultRows: 50,
				},
			},
		},
		{
			desc: "with service account impersonation example",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			location: us
			impersonateServiceAccount: service-account@my-project.iam.gserviceaccount.com
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:                      "my-instance",
					Type:                      bigquery.SourceType,
					Project:                   "my-project",
					Location:                  "us",
					ImpersonateServiceAccount: "service-account@my-project.iam.gserviceaccount.com",
					WriteMode:                 "allowed",
					MaxQueryResultRows:        50,
				},
			},
		},
		{
			desc: "with custom scopes example",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			location: us
			scopes:
			- https://www.googleapis.com/auth/bigquery
			- https://www.googleapis.com/auth/cloud-platform
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					Location:           "us",
					Scopes:             []string{"https://www.googleapis.com/auth/bigquery", "https://www.googleapis.com/auth/cloud-platform"},
					WriteMode:          "allowed",
					MaxQueryResultRows: 50,
				},
			},
		},
		{
			desc: "with max query result rows example",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			location: us
			maxQueryResultRows: 10
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					Location:           "us",
					MaxQueryResultRows: 10,
					WriteMode:          "allowed",
				},
			},
		},
		{
			desc: "with maximum bytes billed example",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			location: us
			maximumBytesBilled: 10737418240
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					Location:           "us",
					MaximumBytesBilled: 10737418240,
					WriteMode:          "allowed",
					MaxQueryResultRows: 50,
				},
			},
		},
		{
			desc: "with api endpoint",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			apiEndpoint: http://localhost:9050
			`,
			want: map[string]sources.SourceConfig{
				"my-instance": bigquery.Config{
					Name:               "my-instance",
					Type:               bigquery.SourceType,
					Project:            "my-project",
					APIEndpoint:        "http://localhost:9050",
					WriteMode:          "allowed",
					MaxQueryResultRows: 50,
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse (-want +got):\n%s", diff)
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
			name: my-instance
			type: bigquery
			project: my-project
			location: us
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-instance\" as \"bigquery\": [1:1] unknown field \"foo\"\n>  1 | foo: bar\n       ^\n   2 | location: us\n   3 | name: my-instance\n   4 | project: my-project\n   5 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			location: us
			`,
			err: "error unmarshaling source: unable to parse source \"my-instance\" as \"bigquery\": Key: 'Config.Project' Error:Field validation for 'Project' failed on the 'required' tag",
		},
		{
			desc: "negative maximum bytes billed",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			maximumBytesBilled: -1
			`,
			err: "error unmarshaling source: unable to parse source \"my-instance\" as \"bigquery\": [1:21] Key: 'Config.MaximumBytesBilled' Error:Field validation for 'MaximumBytesBilled' failed on the 'gte' tag\n>  1 | maximumBytesBilled: -1\n                           ^\n   2 | name: my-instance\n   3 | project: my-project\n   4 | type: bigquery",
		},
		{
			desc: "invalid value for write mode",
			in: `
			kind: source
			name: my-instance
			type: bigquery
			project: my-project
			writeMode: foo
			`,
			err: "error unmarshaling source: unable to parse source \"my-instance\" as \"bigquery\": [4:12] Key: 'Config.WriteMode' Error:Field validation for 'WriteMode' failed on the 'oneof' tag\n   1 | name: my-instance\n   2 | project: my-project\n   3 | type: bigquery\n>  4 | writeMode: foo\n                  ^\n",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
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

func TestInitialize_MaxQueryResultRows(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithUserAgent(ctx, "test-agent")
	tracer := noop.NewTracerProvider().Tracer("")

	tcs := []struct {
		desc string
		cfg  bigquery.Config
		want int
	}{
		{
			desc: "default value",
			cfg: bigquery.Config{
				Name:           "test-default",
				Type:           bigquery.SourceType,
				Project:        "test-project",
				UseClientOAuth: "true",
			},
			want: 50,
		},
		{
			desc: "configured value",
			cfg: bigquery.Config{
				Name:               "test-configured",
				Type:               bigquery.SourceType,
				Project:            "test-project",
				UseClientOAuth:     "true",
				MaxQueryResultRows: 100,
			},
			want: 100,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			src, err := tc.cfg.Initialize(ctx, tracer, false)
			if err != nil {
				t.Fatalf("Initialize failed: %v", err)
			}
			bqSrc, ok := src.(*bigquery.Source)
			if !ok {
				t.Fatalf("Expected *bigquery.Source, got %T", src)
			}
			if bqSrc.MaxQueryResultRows != tc.want {
				t.Errorf("MaxQueryResultRows = %d, want %d", bqSrc.MaxQueryResultRows, tc.want)
			}
		})
	}
}

func TestInitialize_MaximumBytesBilled(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithUserAgent(ctx, "test-agent")
	tracer := noop.NewTracerProvider().Tracer("")

	tcs := []struct {
		desc string
		cfg  bigquery.Config
		want int64
	}{
		{
			desc: "default value",
			cfg: bigquery.Config{
				Name:           "test-default",
				Type:           bigquery.SourceType,
				Project:        "test-project",
				UseClientOAuth: "true",
			},
			want: 0,
		},
		{
			desc: "configured value",
			cfg: bigquery.Config{
				Name:               "test-configured",
				Type:               bigquery.SourceType,
				Project:            "test-project",
				UseClientOAuth:     "true",
				MaximumBytesBilled: 10737418240,
			},
			want: 10737418240,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			src, err := tc.cfg.Initialize(ctx, tracer, false)
			if err != nil {
				t.Fatalf("Initialize failed: %v", err)
			}
			bqSrc, ok := src.(*bigquery.Source)
			if !ok {
				t.Fatalf("Expected *bigquery.Source, got %T", src)
			}
			if bqSrc.MaximumBytesBilled != tc.want {
				t.Errorf("MaximumBytesBilled = %d, want %d", bqSrc.MaximumBytesBilled, tc.want)
			}
		})
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want string
	}{
		{desc: "empty", in: "", want: ""},
		{desc: "whitespace only", in: "  ", want: ""},
		{desc: "https with host, no port", in: "https://proxy.example.com", want: "https://proxy.example.com:443"},
		{desc: "http with localhost and explicit port", in: "http://localhost:9050", want: "http://localhost:9050"},
		{desc: "bare host defaults to https and port 443", in: "proxy.example.com", want: "https://proxy.example.com:443"},
		{desc: "bare host with port keeps port and adds https", in: "host:8443", want: "https://host:8443"},
		{desc: "root trailing slash stripped", in: "https://proxy.example.com/", want: "https://proxy.example.com:443"},
		{desc: "custom path trailing slash preserved", in: "https://proxy.example.com/custom/path/", want: "https://proxy.example.com:443/custom/path/"},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got := bigquery.NormalizeEndpoint(tc.in)
			if got != tc.want {
				t.Errorf("NormalizeEndpoint(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestInitialize_APIEndpoint(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithUserAgent(ctx, "test-agent")
	tracer := noop.NewTracerProvider().Tracer("")

	tcs := []struct {
		desc         string
		cfg          bigquery.Config
		wantEndpoint string
	}{
		{
			desc: "no endpoint — option not added",
			cfg: bigquery.Config{
				Name: "test-no-ep", Type: bigquery.SourceType,
				Project: "proj", UseClientOAuth: "true",
			},
			wantEndpoint: "",
		},
		{
			desc: "http emulator endpoint wired through",
			cfg: bigquery.Config{
				Name: "test-emulator", Type: bigquery.SourceType,
				Project: "proj", UseClientOAuth: "true",
				APIEndpoint: "http://localhost:9050",
			},
			wantEndpoint: "http://localhost:9050",
		},
		{
			desc: "https proxy endpoint normalized and wired through",
			cfg: bigquery.Config{
				Name: "test-proxy", Type: bigquery.SourceType,
				Project: "proj", UseClientOAuth: "true",
				APIEndpoint: "https://proxy.example.com",
			},
			wantEndpoint: "https://proxy.example.com:443",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			src, err := tc.cfg.Initialize(ctx, tracer, false)
			if err != nil {
				t.Fatalf("Initialize failed: %v", err)
			}
			bqSrc, ok := src.(*bigquery.Source)
			if !ok {
				t.Fatalf("expected *bigquery.Source, got %T", src)
			}
			if bqSrc.APIEndpoint != tc.cfg.APIEndpoint {
				t.Errorf("Config.APIEndpoint = %q, want %q", bqSrc.APIEndpoint, tc.cfg.APIEndpoint)
			}
			// Exercise the ClientCreator; neither constructor makes a network call.
			_, _, err = bqSrc.BigQueryClientCreator()("fake-token", false)
			if err != nil {
				t.Errorf("ClientCreator unexpectedly failed with endpoint %q: %v", tc.wantEndpoint, err)
			}
		})
	}
}

func TestNormalizeValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{
			name:     "big.Rat 1/3 (NUMERIC scale 9)",
			input:    new(big.Rat).SetFrac64(1, 3),               // 0.33333333333...
			expected: "0.33333333333333333333333333333333333333", // FloatString(38)
		},
		{
			name:     "big.Rat 19/2 (9.5)",
			input:    new(big.Rat).SetFrac64(19, 2),
			expected: "9.5",
		},
		{
			name:     "big.Rat 12341/10 (1234.1)",
			input:    new(big.Rat).SetFrac64(12341, 10),
			expected: "1234.1",
		},
		{
			name:     "big.Rat 10/1 (10)",
			input:    new(big.Rat).SetFrac64(10, 1),
			expected: "10",
		},
		{
			name:     "string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "int",
			input:    123,
			expected: 123,
		},
		{
			name: "nested slice of big.Rat",
			input: []any{
				new(big.Rat).SetFrac64(19, 2),
				new(big.Rat).SetFrac64(1, 4),
			},
			expected: []any{"9.5", "0.25"},
		},
		{
			name: "nested map of big.Rat",
			input: map[string]any{
				"val1": new(big.Rat).SetFrac64(19, 2),
				"val2": new(big.Rat).SetFrac64(1, 2),
			},
			expected: map[string]any{
				"val1": "9.5",
				"val2": "0.5",
			},
		},
		{
			name: "complex nested structure",
			input: map[string]any{
				"list": []any{
					map[string]any{
						"rat": new(big.Rat).SetFrac64(3, 2),
					},
				},
			},
			expected: map[string]any{
				"list": []any{
					map[string]any{
						"rat": "1.5",
					},
				},
			},
		},
		{
			name: "slice of *big.Rat",
			input: []*big.Rat{
				new(big.Rat).SetFrac64(19, 2),
				new(big.Rat).SetFrac64(1, 4),
			},
			expected: []any{"9.5", "0.25"},
		},
		{
			name:     "slice of strings",
			input:    []string{"a", "b"},
			expected: []any{"a", "b"},
		},
		{
			name:     "byte slice (BYTES)",
			input:    []byte("hello"),
			expected: []byte("hello"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bigquery.NormalizeValue(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("NormalizeValue() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBigQuerySource_IsReadOnly(t *testing.T) {
	tests := []struct {
		name          string
		source        *bigquery.Source
		wantReadOnly  bool
		wantWriteMode string
	}{
		{
			name: "writeMode: blocked is read-only",
			source: &bigquery.Source{
				Config: bigquery.Config{
					Name:      "test-source",
					Type:      bigquery.SourceType,
					WriteMode: bigquery.WriteModeBlocked,
				},
			},
			wantReadOnly:  true,
			wantWriteMode: bigquery.WriteModeBlocked,
		},
		{
			name: "writeMode: protected is read-only",
			source: &bigquery.Source{
				Config: bigquery.Config{
					Name:      "test-source",
					Type:      bigquery.SourceType,
					WriteMode: bigquery.WriteModeProtected,
				},
			},
			wantReadOnly:  true,
			wantWriteMode: bigquery.WriteModeProtected,
		},
		{
			name: "writeMode: allowed is not read-only",
			source: &bigquery.Source{
				Config: bigquery.Config{
					Name:      "test-source",
					Type:      bigquery.SourceType,
					WriteMode: bigquery.WriteModeAllowed,
				},
			},
			wantReadOnly:  false,
			wantWriteMode: bigquery.WriteModeAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.source.IsReadOnly(); got != tt.wantReadOnly {
				t.Errorf("IsReadOnly() = %v, want %v", got, tt.wantReadOnly)
			}
			toCfg, ok := tt.source.ToConfig().(bigquery.Config)
			if !ok {
				t.Fatalf("ToConfig() did not return bigquery.Config, got %T", tt.source.ToConfig())
			}
			if toCfg.ReadOnly == nil {
				t.Errorf("ToConfig().ReadOnly is nil, want %v", tt.wantReadOnly)
			} else if *toCfg.ReadOnly != tt.wantReadOnly {
				t.Errorf("ToConfig().ReadOnly = %v, want %v", *toCfg.ReadOnly, tt.wantReadOnly)
			}
			if toCfg.WriteMode != tt.wantWriteMode {
				t.Errorf("ToConfig().WriteMode = %q, want %q", toCfg.WriteMode, tt.wantWriteMode)
			}
		})
	}
}

func TestInitialize_ReadOnlyAndWriteModeValidation(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithUserAgent(ctx, "test-agent")
	tracer := noop.NewTracerProvider().Tracer("")
	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name          string
		cfg           bigquery.Config
		wantWriteMode string
		wantReadOnly  bool
		wantErr       string
	}{
		// Valid defaulting and success cases
		{
			name: "readOnly: true with empty writeMode defaults to blocked",
			cfg: bigquery.Config{
				ReadOnly:       boolPtr(true),
				UseClientOAuth: "true",
			},
			wantWriteMode: bigquery.WriteModeBlocked,
			wantReadOnly:  true,
		},
		{
			name: "readOnly: false with empty writeMode defaults to allowed",
			cfg: bigquery.Config{
				ReadOnly:       boolPtr(false),
				UseClientOAuth: "true",
			},
			wantWriteMode: bigquery.WriteModeAllowed,
			wantReadOnly:  false,
		},
		{
			name: "nil readOnly with empty writeMode defaults to allowed",
			cfg: bigquery.Config{
				UseClientOAuth: "true",
			},
			wantWriteMode: bigquery.WriteModeAllowed,
			wantReadOnly:  false,
		},
		{
			name: "readOnly: true with explicit writeMode: blocked",
			cfg: bigquery.Config{
				ReadOnly:       boolPtr(true),
				WriteMode:      bigquery.WriteModeBlocked,
				UseClientOAuth: "true",
			},
			wantWriteMode: bigquery.WriteModeBlocked,
			wantReadOnly:  true,
		},
		// Conflict error cases
		{
			name: "readOnly: true + writeMode: allowed",
			cfg: bigquery.Config{
				ReadOnly:  boolPtr(true),
				WriteMode: bigquery.WriteModeAllowed,
			},
			wantErr: `conflicting source configuration: readOnly is true, but writeMode is "allowed"`,
		},
		{
			name: "readOnly: false + writeMode: blocked",
			cfg: bigquery.Config{
				ReadOnly:  boolPtr(false),
				WriteMode: bigquery.WriteModeBlocked,
			},
			wantErr: `conflicting source configuration: readOnly is false, but writeMode is "blocked"`,
		},
		{
			name: "readOnly: false + writeMode: protected",
			cfg: bigquery.Config{
				ReadOnly:  boolPtr(false),
				WriteMode: bigquery.WriteModeProtected,
			},
			wantErr: `conflicting source configuration: readOnly is false, but writeMode is "protected"`,
		},
		{
			name: "writeMode: protected + useClientOAuth: true",
			cfg: bigquery.Config{
				WriteMode:      bigquery.WriteModeProtected,
				UseClientOAuth: "true",
			},
			wantErr: `writeMode 'protected' cannot be used with useClientOAuth enabled`,
		},
		{
			name: "invalid writeMode",
			cfg: bigquery.Config{
				WriteMode: "invalid-mode",
			},
			wantErr: `invalid writeMode "invalid-mode": must be one of "allowed", "protected", or "blocked"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cfg.Name == "" {
				tt.cfg.Name = "test-source"
				tt.cfg.Type = bigquery.SourceType
				tt.cfg.Project = "test-project"
			}
			src, err := tt.cfg.Initialize(ctx, tracer, false)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Errorf("Initialize() error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			bqSrc, ok := src.(*bigquery.Source)
			if !ok {
				t.Fatalf("expected *bigquery.Source, got %T", src)
			}
			if bqSrc.WriteMode != tt.wantWriteMode {
				t.Errorf("WriteMode = %q, want %q", bqSrc.WriteMode, tt.wantWriteMode)
			}
			if bqSrc.IsReadOnly() != tt.wantReadOnly {
				t.Errorf("IsReadOnly() = %v, want %v", bqSrc.IsReadOnly(), tt.wantReadOnly)
			}
		})
	}
}
