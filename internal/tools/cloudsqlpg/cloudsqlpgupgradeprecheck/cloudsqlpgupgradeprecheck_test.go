// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloudsqlpgupgradeprecheck_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudsqlpg/cloudsqlpgupgradeprecheck"
)

func TestParseFromYaml(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		want server.ToolConfigs
	}{
		{
			desc: "basic precheck example",
			in: `
			kind: tool
			name: precheck-upgrade-tool
			type: postgres-upgrade-precheck
			description: a precheck test description
			source: some-admin-source
			authRequired:
				- https://www.googleapis.com/auth/cloud-platform
			`,
			want: server.ToolConfigs{
				"precheck-upgrade-tool": cloudsqlpgupgradeprecheck.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "precheck-upgrade-tool",
						Description:  "a precheck test description",
						AuthRequired: []string{"https://www.googleapis.com/auth/cloud-platform"},
					},
					Type:   "postgres-upgrade-precheck",
					Source: "some-admin-source",
				},
			},
		},
		{
			desc: "precheck example with no auth",
			in: `
			kind: tool
			name: precheck-upgrade-tool-no-auth
			type: postgres-upgrade-precheck
			description: a precheck test description no auth
			source: other-admin-source
			`,
			want: server.ToolConfigs{
				"precheck-upgrade-tool-no-auth": cloudsqlpgupgradeprecheck.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "precheck-upgrade-tool-no-auth",
						Description:  "a precheck test description no auth",
						AuthRequired: []string{},
					},
					Type:   "postgres-upgrade-precheck",
					Source: "other-admin-source",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff (-want +got):\n%s", diff)
			}
		})
	}
}
