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
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGCPSourceTypes(t *testing.T) {
	want := []string{"alloydb-postgres", "cloud-sql-mssql", "cloud-sql-mysql", "cloud-sql-postgres", "spanner"}
	if diff := cmp.Diff(want, gcpSourceTypes); diff != "" {
		t.Errorf("gcpSourceTypes mismatch (-want +got):\n%s", diff)
	}
}

func TestExpandSourcePresets(t *testing.T) {
	tcs := []struct {
		desc string
		in   []string
		want []string
	}{
		{desc: "nil", in: nil, want: nil},
		{desc: "no preset", in: []string{"bigquery"}, want: []string{"bigquery"}},
		{desc: "preset only", in: []string{"gcp-sources"}, want: gcpSourceTypes},
		{desc: "preset with whitespace", in: []string{" gcp-sources "}, want: gcpSourceTypes},
		{desc: "preset with others", in: []string{"bigquery", "gcp-sources"}, want: append([]string{"bigquery"}, gcpSourceTypes...)},
		{desc: "preset is case insensitive", in: []string{"GCP-Sources"}, want: gcpSourceTypes},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, expandSourcePresets(tc.in)); diff != "" {
				t.Errorf("expandSourcePresets(%q) mismatch (-want +got):\n%s", tc.in, diff)
			}
		})
	}
}

func TestLoadRestrictionsFromEnv(t *testing.T) {
	tcs := []struct {
		desc                string
		envAllowedSources   string
		envDisallowedTools  string
		wantAllowedSources  []string
		wantDisallowedTools []string
	}{
		{
			desc: "unset means no restriction",
		},
		{
			desc:                "both set",
			envAllowedSources:   "cloud-sql-postgres, spanner",
			envDisallowedTools:  "wait,postgres-execute-sql",
			wantAllowedSources:  []string{"cloud-sql-postgres", " spanner"},
			wantDisallowedTools: []string{"wait", "postgres-execute-sql"},
		},
		{
			desc:               "gcp-sources preset is expanded",
			envAllowedSources:  "gcp-sources,bigquery",
			wantAllowedSources: append(append([]string{}, gcpSourceTypes...), "bigquery"),
		},
		{
			desc:               "mixed-case gcp-sources preset is expanded",
			envAllowedSources:  "GCP-Sources",
			wantAllowedSources: gcpSourceTypes,
		},
		{
			desc:               "whitespace only means no restriction",
			envAllowedSources:  "   ",
			envDisallowedTools: "\t",
		},
		{
			desc:               "separators only is kept",
			envAllowedSources:  ",",
			wantAllowedSources: []string{"", ""},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			t.Setenv(allowedSourcesEnvVar, tc.envAllowedSources)
			t.Setenv(disallowedToolsEnvVar, tc.envDisallowedTools)
			opts := NewToolboxOptions()

			opts.loadRestrictionsFromEnv()

			if diff := cmp.Diff(tc.wantAllowedSources, opts.Cfg.AllowedSources); diff != "" {
				t.Errorf("AllowedSources mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantDisallowedTools, opts.Cfg.DisallowedTools); diff != "" {
				t.Errorf("DisallowedTools mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
