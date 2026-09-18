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

package mssql_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/mssql"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestParseFromYamlMssql(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-mssql-instance
			type: mssql
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-mssql-instance": mssql.Config{
					Name:     "my-mssql-instance",
					Type:     mssql.SourceType,
					Host:     "0.0.0.0",
					Port:     "my-port",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
				},
			},
		},
		{
			desc: "with encrypt field",
			in: `
			kind: source
			name: my-mssql-instance
			type: mssql
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			encrypt: strict
			`,
			want: map[string]sources.SourceConfig{
				"my-mssql-instance": mssql.Config{
					Name:     "my-mssql-instance",
					Type:     mssql.SourceType,
					Host:     "0.0.0.0",
					Port:     "my-port",
					Database: "my_db",
					User:     "my_user",
					Password: "my_pass",
					Encrypt:  "strict",
				},
			},
		},
		{
			desc: "with entra id auth and no sql login",
			in: `
			kind: source
			name: my-mssql-instance
			type: mssql
			host: my-server.database.windows.net
			port: "1433"
			database: my_db
			encrypt: strict
			azureAuth:
			  mode: workload-identity
			  clientId: 11111111-1111-1111-1111-111111111111
			  tenantId: 22222222-2222-2222-2222-222222222222
			  disableInstanceDiscovery: true
			  additionallyAllowedTenants:
			    - 33333333-3333-3333-3333-333333333333
			`,
			want: map[string]sources.SourceConfig{
				"my-mssql-instance": mssql.Config{
					Name:     "my-mssql-instance",
					Type:     mssql.SourceType,
					Host:     "my-server.database.windows.net",
					Port:     "1433",
					Database: "my_db",
					Encrypt:  "strict",
					AzureAuth: &mssql.AzureAuthConfig{
						Mode:                       "workload-identity",
						ClientID:                   "11111111-1111-1111-1111-111111111111",
						TenantID:                   "22222222-2222-2222-2222-222222222222",
						DisableInstanceDiscovery:   true,
						AdditionallyAllowedTenants: []string{"33333333-3333-3333-3333-333333333333"},
					},
				},
			},
		},
		{
			desc: "with entra id default mode only",
			in: `
			kind: source
			name: my-mssql-instance
			type: mssql
			host: my-server.database.windows.net
			port: "1433"
			database: my_db
			azureAuth:
			  mode: default
			`,
			want: map[string]sources.SourceConfig{
				"my-mssql-instance": mssql.Config{
					Name:      "my-mssql-instance",
					Type:      mssql.SourceType,
					Host:      "my-server.database.windows.net",
					Port:      "1433",
					Database:  "my_db",
					AzureAuth: &mssql.AzureAuthConfig{Mode: "default"},
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
				t.Fatalf("incorrect psarse: want %v, got %v", tc.want, got)
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
			name: my-mssql-instance
			type: mssql
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			password: my_pass
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-mssql-instance\" as \"mssql\": [2:1] unknown field \"foo\"\n   1 | database: my_db\n>  2 | foo: bar\n       ^\n   3 | host: 0.0.0.0\n   4 | name: my-mssql-instance\n   5 | password: my_pass\n   6 | ",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-mssql-instance
			type: mssql
			host: 0.0.0.0
			port: my-port
			database: my_db
			user: my_user
			`,
			err: "error unmarshaling source: unable to parse source \"my-mssql-instance\" as \"mssql\": Key: 'Config.Password' Error:Field validation for 'Password' failed on the 'required_without' tag",
		},
		{
			desc: "entra id tenant without a client id",
			in: `
			kind: source
			name: my-mssql-instance
			type: mssql
			host: my-server.database.windows.net
			port: "1433"
			database: my_db
			azureAuth:
			  mode: default
			  tenantId: 22222222-2222-2222-2222-222222222222
			`,
			err: "error unmarshaling source: unable to parse source \"my-mssql-instance\" as \"mssql\": [1:10] Key: 'AzureAuthConfig.ClientID' Error:Field validation for 'ClientID' failed on the 'required_with' tag\n>  1 | azureAuth:\n                ^\n   2 |   mode: default\n   3 |   tenantId: 22222222-2222-2222-2222-222222222222\n   4 | database: my_db\n   5 | ",
		},
		{
			desc: "entra id service principal without a client id",
			in: `
			kind: source
			name: my-mssql-instance
			type: mssql
			host: my-server.database.windows.net
			port: "1433"
			database: my_db
			password: my_secret
			azureAuth:
			  mode: service-principal
			`,
			err: "error unmarshaling source: unable to parse source \"my-mssql-instance\" as \"mssql\": [1:10] Key: 'AzureAuthConfig.ClientID' Error:Field validation for 'ClientID' failed on the 'required_if' tag\n>  1 | azureAuth:\n                ^\n   2 |   mode: service-principal\n   3 | database: my_db\n   4 | host: my-server.database.windows.net\n   5 | ",
		},
		{
			desc: "unsupported entra id mode",
			in: `
			kind: source
			name: my-mssql-instance
			type: mssql
			host: my-server.database.windows.net
			port: "1433"
			database: my_db
			azureAuth:
			  mode: certificate
			`,
			err: "error unmarshaling source: unable to parse source \"my-mssql-instance\" as \"mssql\": [2:9] Key: 'AzureAuthConfig.Mode' Error:Field validation for 'Mode' failed on the 'oneof' tag\n   1 | azureAuth:\n>  2 |   mode: certificate\n               ^\n   3 | database: my_db\n   4 | host: my-server.database.windows.net\n   5 | name: my-mssql-instance\n   6 | ",
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

func TestInitializeRejectsMixedIdentity(t *testing.T) {
	tcs := []struct {
		desc    string
		cfg     mssql.Config
		wantErr string
	}{
		{
			desc:    "a sql user alongside an entra identity",
			cfg:     mssql.Config{User: "my_user", AzureAuth: &mssql.AzureAuthConfig{Mode: "default"}},
			wantErr: "'user' must not be set",
		},
		{
			desc:    "a service principal without its client secret",
			cfg:     mssql.Config{AzureAuth: &mssql.AzureAuthConfig{Mode: "service-principal", ClientID: "11111111-1111-1111-1111-111111111111"}},
			wantErr: "needs the client secret in 'password'",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := tc.cfg
			cfg.Name, cfg.Type, cfg.Host, cfg.Port, cfg.Database = "n", mssql.SourceType, "0.0.0.0", "1433", "my_db"
			_, err := cfg.Initialize(context.Background(), noop.NewTracerProvider().Tracer("test"))
			if err == nil {
				t.Fatalf("expected initialization to fail")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("unexpected error: got %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}
