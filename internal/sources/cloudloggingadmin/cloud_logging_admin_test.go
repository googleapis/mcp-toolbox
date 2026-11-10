// Copyright 2026 Google LLC
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
package cloudloggingadmin_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources/cloudloggingadmin"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
)

func TestParseFromYamlCloudLoggingAdmin(t *testing.T) {
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
			type: cloud-logging-admin
			project: my-project
			`,
			want: server.SourceConfigs{
				"my-instance": cloudloggingadmin.Config{
					Name:    "my-instance",
					Type:    cloudloggingadmin.SourceType,
					Project: "my-project",
				},
			},
		},
		{
			desc: "with client oauth",
			in: `
			kind: source
			name: my-instance
			type: cloud-logging-admin
			project: my-project
			useClientOAuth: true
			`,
			want: server.SourceConfigs{
				"my-instance": cloudloggingadmin.Config{
					Name:           "my-instance",
					Type:           cloudloggingadmin.SourceType,
					Project:        "my-project",
					UseClientOAuth: true,
				},
			},
		},
		{
			desc: "with service account impersonation",
			in: `
			kind: source
			name: my-instance
			type: cloud-logging-admin
			project: my-project
			impersonateServiceAccount: service-account@my-project.iam.gserviceaccount.com
			`,
			want: server.SourceConfigs{
				"my-instance": cloudloggingadmin.Config{
					Name:                      "my-instance",
					Type:                      cloudloggingadmin.SourceType,
					Project:                   "my-project",
					ImpersonateServiceAccount: "service-account@my-project.iam.gserviceaccount.com",
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
			name: my-instance
			type: cloud-logging-admin
			project: my-project
			foo: bar
			`,
			err: "error unmarshaling source: unable to parse source \"my-instance\" as \"cloud-logging-admin\": [1:1] unknown field \"foo\"\n>  1 | foo: bar\n       ^\n   2 | name: my-instance\n   3 | project: my-project\n   4 | type: cloud-logging-admin",
		},
		{
			desc: "missing required field",
			in: `
			kind: source
			name: my-instance
			type: cloud-logging-admin
			`,
			err: "error unmarshaling source: unable to parse source \"my-instance\" as \"cloud-logging-admin\": Key: 'Config.Project' Error:Field validation for 'Project' failed on the 'required' tag",
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
