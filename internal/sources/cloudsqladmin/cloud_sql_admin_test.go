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

package cloudsqladmin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/cloudsqladmin"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"google.golang.org/api/option"
	sqladmin "google.golang.org/api/sqladmin/v1"
)

func TestParseFromYamlCloudSQLAdmin(t *testing.T) {
	t.Parallel()
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: source
			name: my-cloud-sql-admin-instance
			type: cloud-sql-admin
			`,
			want: map[string]sources.SourceConfig{
				"my-cloud-sql-admin-instance": cloudsqladmin.Config{
					Name:           "my-cloud-sql-admin-instance",
					Type:           cloudsqladmin.SourceType,
					UseClientOAuth: false,
				},
			},
		},
		{
			desc: "use client auth example",
			in: `
			kind: source
			name: my-cloud-sql-admin-instance
			type: cloud-sql-admin
			useClientOAuth: true
			`,
			want: map[string]sources.SourceConfig{
				"my-cloud-sql-admin-instance": cloudsqladmin.Config{
					Name:           "my-cloud-sql-admin-instance",
					Type:           cloudsqladmin.SourceType,
					UseClientOAuth: true,
				},
			},
		},
		{
			desc: "readOnly set to true",
			in: `
			kind: source
			name: my-cloud-sql-admin-instance
			type: cloud-sql-admin
			readOnly: true
			`,
			want: map[string]sources.SourceConfig{
				"my-cloud-sql-admin-instance": cloudsqladmin.Config{
					Name:           "my-cloud-sql-admin-instance",
					Type:           cloudsqladmin.SourceType,
					UseClientOAuth: false,
					ReadOnly:       true,
				},
			},
		},
	}
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			got, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse: want %v, got %v", tc.want, got)
			}
			for _, sc := range got {
				if cfg, ok := sc.(cloudsqladmin.Config); ok {
					src := &cloudsqladmin.Source{Config: cfg}
					if src.IsReadOnly() != cfg.ReadOnly {
						t.Errorf("IsReadOnly() = %v, want %v", src.IsReadOnly(), cfg.ReadOnly)
					}
				}
			}
		})
	}
}

func TestFailParseFromYaml(t *testing.T) {
	t.Parallel()
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-cloud-sql-admin-instance
			type: cloud-sql-admin
			project: test-project
			`,
			err: "error unmarshaling source: unable to parse source \"my-cloud-sql-admin-instance\" as \"cloud-sql-admin\": [2:1] unknown field \"project\"\n   1 | name: my-cloud-sql-admin-instance\n>  2 | project: test-project\n       ^\n   3 | type: cloud-sql-admin",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-cloud-sql-admin-instance
			useClientOAuth: true
			`,
			err: "error unmarshaling source: missing 'type' field or it is not a string",
		},
	}
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
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

// Regression: empty list responses must serialize as an empty JSON array,
// not null.
func TestEmptyListSerializesAsEmptySlice(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items": []}`)
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	svc, err := sqladmin.NewService(ctx, option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("unable to create stub sqladmin service: %s", err)
	}
	s := &cloudsqladmin.Source{Service: svc}

	tcs := []struct {
		desc string
		call func() (any, error)
	}{
		{
			desc: "ListDatabase",
			call: func() (any, error) { return s.ListDatabase(ctx, "p", "i", "") },
		},
		{
			desc: "ListInstance",
			call: func() (any, error) { return s.ListInstance(ctx, "p", "") },
		},
	}
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			got, err := tc.call()
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			out, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("unable to marshal result: %s", err)
			}
			if string(out) != "[]" {
				t.Fatalf("empty list should serialize as [], got %s", string(out))
			}
		})
	}
}
