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

package bigtablegetinstance_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigtable/bigtablegetinstance"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestParseFromYamlBigtable(t *testing.T) {
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
			type: bigtable-get-instance
			source: my-bq-instance
			description: some description
			`,
			want: server.ToolConfigs{
				"example_tool": bigtablegetinstance.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "bigtable-get-instance",
					Source: "my-bq-instance",
				},
			},
		},
		{
			desc: "with auth required",
			in: `
			kind: tool
			name: example_tool
			type: bigtable-get-instance
			source: my-bq-instance
			description: some description
			authRequired: 
			- my-google-auth-service
			- other-auth-service
			`,
			want: server.ToolConfigs{
				"example_tool": bigtablegetinstance.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:   "bigtable-get-instance",
					Source: "my-bq-instance",
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
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}

}

type mockSource struct {
	sources.Source
	err error
}

func (m *mockSource) GetInstance(ctx context.Context, instanceID string) (any, error) {
	if m.err != nil {
		return nil, m.err
	}
	return map[string]string{"instance": "inst-1"}, nil
}

func TestInvoke(t *testing.T) {
	cfg := bigtablegetinstance.Config{
		ConfigBase: tools.ConfigBase{Name: "test_tool"},
		Type:       "bigtable-get-instance",
		Source:     "test-source",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		src := &mockSource{}
		params := parameters.ParamValues{
			{Name: "instance_id", Value: "inst-1"},
		}
		got, err := tool.Invoke(context.Background(), src, params, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := any(map[string]string{"instance": "inst-1"})
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("unexpected output (-want +got):\n%s", diff)
		}
	})

	t.Run("error", func(t *testing.T) {
		src := &mockSource{err: errors.New("gcp error")}
		params := parameters.ParamValues{
			{Name: "instance_id", Value: "inst-1"},
		}
		_, err := tool.Invoke(context.Background(), src, params, "")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})
}
