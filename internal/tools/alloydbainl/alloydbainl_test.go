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

package alloydbainl_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/alloydbainl"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlAlloyDBNLA(t *testing.T) {
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
            name: example_tool
            type: alloydb-ai-nl
            source: my-alloydb-instance
            description: AlloyDB natural language query tool
            nlConfig: 'my_nl_config'
            authRequired:
            - my-google-auth-service
            nlConfigParameters:
            - name: user_id
              type: string
              description: user_id to use
              authServices:
                - name: my-google-auth-service
                  field: sub
            `,
			want: server.ToolConfigs{
				"example_tool": alloydbainl.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "AlloyDB natural language query tool",
						AuthRequired: []string{"my-google-auth-service"},
					},
					Type:     "alloydb-ai-nl",
					Source:   "my-alloydb-instance",
					NLConfig: "my_nl_config",
					NLConfigParameters: []parameters.Parameter{
						parameters.NewStringParameter("user_id", "user_id to use", parameters.WithStringAuth(
							[]parameters.ParamAuthService{{Name: "my-google-auth-service", Field: "sub"}})),
					},
				},
			},
		},
		{
			desc: "with multiple parameters",
			in: `
            kind: tool
            name: complex_tool
            type: alloydb-ai-nl
            source: my-alloydb-instance
            description: AlloyDB natural language query tool with multiple parameters
            nlConfig: 'complex_nl_config'
            authRequired:
            - my-google-auth-service
            - other-auth-service
            nlConfigParameters:
            - name: user_id
              type: string
              description: user_id to use
              authServices:
                - name: my-google-auth-service
                  field: sub
            - name: user_email
              type: string
              description: user_email to use
              authServices:
                - name: my-google-auth-service
                  field: user_email
            `,
			want: server.ToolConfigs{
				"complex_tool": alloydbainl.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "complex_tool",
						Description:  "AlloyDB natural language query tool with multiple parameters",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:     "alloydb-ai-nl",
					Source:   "my-alloydb-instance",
					NLConfig: "complex_nl_config",
					NLConfigParameters: []parameters.Parameter{
						parameters.NewStringParameter("user_id", "user_id to use", parameters.WithStringAuth(
							[]parameters.ParamAuthService{{Name: "my-google-auth-service", Field: "sub"}})),

						parameters.NewStringParameter("user_email", "user_email to use", parameters.WithStringAuth(
							[]parameters.ParamAuthService{{Name: "my-google-auth-service", Field: "user_email"}})),
					},
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}
