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

package mssql_test

import (
	"context"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/sources/mssql"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"go.opentelemetry.io/otel/trace/noop"
)

// initClientAuthSource builds a source that authenticates as the caller. No
// connection is opened here: with no identity of its own there is nothing to
// verify at startup, which is what lets this run without a server.
func initClientAuthSource(t *testing.T, cfg mssql.Config) *mssql.Source {
	t.Helper()
	cfg.Name = "my-mssql-instance"
	cfg.Type = mssql.SourceType
	cfg.Host = "0.0.0.0"
	cfg.Port = "1433"
	cfg.Database = "my_db"

	s, err := cfg.Initialize(context.Background(), noop.NewTracerProvider().Tracer("test"), false)
	if err != nil {
		t.Fatalf("unable to initialize source: %v", err)
	}
	source, ok := s.(*mssql.Source)
	if !ok {
		t.Fatalf("expected *mssql.Source, got %T", s)
	}
	return source
}

func TestClientAuthorizationConfig(t *testing.T) {
	tcs := []struct {
		desc       string
		cfg        mssql.Config
		wantOn     bool
		wantHeader string
	}{
		{
			desc:   "off by default",
			cfg:    mssql.Config{User: "my_user", Password: "my_pass"},
			wantOn: false,
		},
		{
			desc:   "false is off, and keeps the shared login",
			cfg:    mssql.Config{User: "my_user", Password: "my_pass", UseClientOAuth: "false"},
			wantOn: false,
		},
		{
			desc:       "true reads the standard header",
			cfg:        mssql.Config{UseClientOAuth: "true"},
			wantOn:     true,
			wantHeader: "Authorization",
		},
		{
			desc:       "any other value names the header",
			cfg:        mssql.Config{UseClientOAuth: "My-Auth-Header"},
			wantOn:     true,
			wantHeader: "My-Auth-Header",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// The off cases would need a reachable server to initialize, so they
			// are read straight off the configuration.
			if !tc.wantOn {
				source := &mssql.Source{Config: tc.cfg}
				if source.UseClientAuthorization() {
					t.Errorf("UseClientAuthorization: got true, want false")
				}
				return
			}
			source := initClientAuthSource(t, tc.cfg)
			if !source.UseClientAuthorization() {
				t.Errorf("UseClientAuthorization: got false, want true")
			}
			if got := source.GetAuthTokenHeaderName(); got != tc.wantHeader {
				t.Errorf("GetAuthTokenHeaderName: got %q, want %q", got, tc.wantHeader)
			}
		})
	}
}

func TestInitializeRejectsAmbiguousIdentity(t *testing.T) {
	tcs := []struct {
		desc    string
		cfg     mssql.Config
		wantErr string
	}{
		{
			desc:    "a login alongside the caller's identity",
			cfg:     mssql.Config{UseClientOAuth: "true", User: "my_user", Password: "my_pass"},
			wantErr: "must not be set",
		},
		{
			desc:    "useClientOAuth switched off, and no login left",
			cfg:     mssql.Config{UseClientOAuth: "false"},
			wantErr: "are required unless",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := tc.cfg
			cfg.Name, cfg.Type, cfg.Host, cfg.Port, cfg.Database = "n", mssql.SourceType, "0.0.0.0", "1433", "my_db"
			_, err := cfg.Initialize(context.Background(), noop.NewTracerProvider().Tracer("test"), true)
			if err == nil {
				t.Fatalf("expected initialization to fail")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("unexpected error: got %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestDBForClientRequiresABearerToken(t *testing.T) {
	source := initClientAuthSource(t, mssql.Config{UseClientOAuth: "true"})

	for _, header := range []tools.AccessToken{"", "not-a-bearer-token", "Basic dXNlcjpwYXNz"} {
		if _, err := source.DBForClient(context.Background(), header); err == nil {
			t.Errorf("expected %q to be refused, but a connection pool was returned", header)
		}
	}
}

func TestDBForClientPoolsPerCaller(t *testing.T) {
	source := initClientAuthSource(t, mssql.Config{UseClientOAuth: "true"})
	ctx := context.Background()

	first, err := source.DBForClient(ctx, "Bearer token-for-alice")
	if err != nil {
		t.Fatalf("unable to get a pool for the first caller: %v", err)
	}
	again, err := source.DBForClient(ctx, "Bearer token-for-alice")
	if err != nil {
		t.Fatalf("unable to get a pool for the first caller a second time: %v", err)
	}
	if first != again {
		t.Errorf("the same caller got two connection pools; the cache is not being used")
	}

	other, err := source.DBForClient(ctx, "Bearer token-for-bob")
	if err != nil {
		t.Fatalf("unable to get a pool for the second caller: %v", err)
	}
	if first == other {
		t.Errorf("two callers share one connection pool, so the database would see one identity")
	}
}

func TestDBForClientRefusesASharedIdentitySource(t *testing.T) {
	source := &mssql.Source{Config: mssql.Config{Name: "shared", User: "my_user", Password: "my_pass"}}
	if _, err := source.DBForClient(context.Background(), "Bearer token-for-alice"); err == nil {
		t.Fatal("expected a source with its own identity to refuse a per-caller connection")
	}
}
