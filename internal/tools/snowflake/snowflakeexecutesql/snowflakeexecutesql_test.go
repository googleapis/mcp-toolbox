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

package snowflakeexecutesql_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/snowflake/snowflakeexecutesql"
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
			name: my-snowflake-tool
			type: snowflake-execute-sql
			source: my-snowflake-source
			description: Execute SQL on Snowflake
			`,
			want: server.ToolConfigs{
				"my-snowflake-tool": snowflakeexecutesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "my-snowflake-tool",
						Description:  "Execute SQL on Snowflake",
						AuthRequired: []string{},
					},
					Type:   "snowflake-execute-sql",
					Source: "my-snowflake-source",
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

func TestFailInitializeMissingDescription(t *testing.T) {
	cfg := snowflakeexecutesql.Config{
		ConfigBase: tools.ConfigBase{Name: "my-snowflake-tool"},
		Type:       "snowflake-execute-sql",
		Source:     "my-snowflake-source",
	}
	_, err := cfg.Initialize(context.Background())
	if err == nil {
		t.Fatalf("expect initialize to fail")
	}
	want := "description is required for tool \"my-snowflake-tool\""
	if err.Error() != want {
		t.Fatalf("unexpected error: got %q, want %q", err.Error(), want)
	}
}
