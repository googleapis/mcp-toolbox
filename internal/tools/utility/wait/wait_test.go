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

package wait_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"

	wait "github.com/googleapis/mcp-toolbox/internal/tools/utility/wait"
)

func TestParseFromYamlWait(t *testing.T) {
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
			type: wait
			description: some description
			timeout: 10s
			authRequired:
				- my-google-auth-service
			`,
			want: server.ToolConfigs{
				"example_tool": wait.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service"},
					},
					Type:    "wait",
					Timeout: "10s",
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

func TestWaitInvokeRespectsTimeout(t *testing.T) {
	cfg := wait.Config{
		ConfigBase: tools.ConfigBase{Name: "wait", Description: "wait tool"},
		Type:       "wait",
		Timeout:    "100ms",
	}
	tool, err := cfg.Initialize(context.Background())
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	t.Run("duration exceeding timeout is rejected", func(t *testing.T) {
		_, boxErr := tool.Invoke(context.Background(), nil, parameters.ParamValues{
			{Name: "duration", Value: "200ms"},
		}, tools.AccessToken(""))
		if boxErr == nil {
			t.Fatal("expected an error for a duration exceeding the configured timeout")
		}
	})

	t.Run("duration within timeout succeeds", func(t *testing.T) {
		if _, boxErr := tool.Invoke(context.Background(), nil, parameters.ParamValues{
			{Name: "duration", Value: "50ms"},
		}, tools.AccessToken("")); boxErr != nil {
			t.Fatalf("expected success: %v", boxErr)
		}
	})

	t.Run("non-positive duration is rejected", func(t *testing.T) {
		_, boxErr := tool.Invoke(context.Background(), nil, parameters.ParamValues{
			{Name: "duration", Value: "0s"},
		}, tools.AccessToken(""))
		if boxErr == nil {
			t.Fatal("expected an error for a non-positive duration")
		}
	})

	t.Run("cancelled context aborts the sleep", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, boxErr := tool.Invoke(ctx, nil, parameters.ParamValues{
			{Name: "duration", Value: "100ms"},
		}, tools.AccessToken(""))
		if boxErr == nil {
			t.Fatal("expected an error when the request is cancelled")
		}
	})
}
