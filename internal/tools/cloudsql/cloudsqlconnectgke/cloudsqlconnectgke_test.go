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

package cloudsqlconnectgke_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudsql/cloudsqlconnectgke"
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
			desc: "basic example",
			in: `
			kind: tool
			name: connect_to_gke
			type: cloud-sql-connect-gke
			description: Connect a Cloud SQL instance to a GKE cluster
			source: my-cloudsql-source
			`,
			want: server.ToolConfigs{
				"connect_to_gke": cloudsqlconnectgke.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "connect_to_gke",
						Description:  "Connect a Cloud SQL instance to a GKE cluster",
						AuthRequired: []string{},
					},
					Type:   "cloud-sql-connect-gke",
					Source: "my-cloudsql-source",
				},
			},
		},
		{
			desc: "with authRequired",
			in: `
			kind: tool
			name: connect_to_gke
			type: cloud-sql-connect-gke
			description: Connect a Cloud SQL instance to a GKE cluster
			source: my-cloudsql-source
			authRequired:
				- https://www.googleapis.com/auth/cloud-platform
			`,
			want: server.ToolConfigs{
				"connect_to_gke": cloudsqlconnectgke.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "connect_to_gke",
						Description:  "Connect a Cloud SQL instance to a GKE cluster",
						AuthRequired: []string{"https://www.googleapis.com/auth/cloud-platform"},
					},
					Type:   "cloud-sql-connect-gke",
					Source: "my-cloudsql-source",
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
