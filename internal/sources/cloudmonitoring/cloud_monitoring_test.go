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

package cloudmonitoring_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/cloudmonitoring"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlCloudMonitoring(t *testing.T) {
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
			name: my-cloud-monitoring-instance
			type: cloud-monitoring
			`,
			want: map[string]sources.SourceConfig{
				"my-cloud-monitoring-instance": cloudmonitoring.Config{
					Name:           "my-cloud-monitoring-instance",
					Type:           cloudmonitoring.SourceType,
					UseClientOAuth: false,
				},
			},
		},
		{
			desc: "use client auth example",
			in: `
			kind: source
			name: my-cloud-monitoring-instance
			type: cloud-monitoring
			useClientOAuth: true
			`,
			want: map[string]sources.SourceConfig{
				"my-cloud-monitoring-instance": cloudmonitoring.Config{
					Name:           "my-cloud-monitoring-instance",
					Type:           cloudmonitoring.SourceType,
					UseClientOAuth: true,
				},
			},
		},
	}
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
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
			name: my-cloud-monitoring-instance
			type: cloud-monitoring
			project: test-project
			`,
			err: "error unmarshaling source: unable to parse source \"my-cloud-monitoring-instance\" as \"cloud-monitoring\": [2:1] unknown field \"project\"\n   1 | name: my-cloud-monitoring-instance\n>  2 | project: test-project\n       ^\n   3 | type: cloud-monitoring",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-cloud-monitoring-instance
			useClientOAuth: true
			`,
			err: "error unmarshaling source: missing 'type' field or it is not a string",
		},
	}
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
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
