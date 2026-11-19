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

package testutils

import (
	"fmt"
	"strings"
	"testing"

	dataproc "cloud.google.com/go/dataproc/v2/apiv1/dataprocpb"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/serverlessspark/createbatch"
	"google.golang.org/protobuf/testing/protocmp"
)

// RunParseFromYAMLTests runs a suite of tests for parsing tool configurations from YAML.
func RunParseFromYAMLTests(t *testing.T, resourceType string, newConfig func(c createbatch.Config) tools.ToolConfig) {
	t.Helper()
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	tcs := []struct {
		desc    string
		in      string
		want    server.ToolConfigs
		wantErr string
	}{
		{
			desc: "basic example",
			in: fmt.Sprintf(`
			kind: tool
			name: example_tool
			type: %s
			source: my-instance
			description: some description
			`, resourceType),
			want: server.ToolConfigs{
				"example_tool": newConfig(createbatch.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   resourceType,
					Source: "my-instance",
				}),
			},
		},
		{
			desc: "detailed config",
			in: fmt.Sprintf(`
			kind: tool
			name: example_tool
			type: %s
			source: my-instance
			description: some description
			runtimeConfig:
			  properties:
				  "spark.driver.memory": "1g"
			environmentConfig:
			  executionConfig:
				  networkUri: "my-network"
			`, resourceType),
			want: server.ToolConfigs{
				"example_tool": newConfig(createbatch.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   resourceType,
					Source: "my-instance",
					RuntimeConfig: &dataproc.RuntimeConfig{
						Properties: map[string]string{"spark.driver.memory": "1g"},
					},
					EnvironmentConfig: &dataproc.EnvironmentConfig{
						ExecutionConfig: &dataproc.ExecutionConfig{
							Network: &dataproc.ExecutionConfig_NetworkUri{NetworkUri: "my-network"},
						},
					},
				}),
			},
		},
		{
			desc: "invalid runtime config",
			in: fmt.Sprintf(`
			kind: tool
			name: example_tool
			type: %s
			source: my-instance
			description: some description
			runtimeConfig:
			  invalidField: true
			`, resourceType),
			wantErr: "unmarshal runtimeConfig",
		},
		{
			desc: "invalid environment config",
			in: fmt.Sprintf(`
			kind: tool
			name: example_tool
			type: %s
			source: my-instance
			description: some description
			environmentConfig:
			  invalidField: true
			`, resourceType),
			wantErr: "unmarshal environmentConfig",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error to contain %q, got %q", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}

			if diff := cmp.Diff(tc.want, got, protocmp.Transform()); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}
