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

package mysqlexecutesql_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/mysql/mysqlexecutesql"
)

func TestParseFromYamlExecuteSql(t *testing.T) {
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
            type: mysql-execute-sql
            source: my-instance
            description: some description
            authRequired:
                - my-google-auth-service
                - other-auth-service
			`,
			want: server.ToolConfigs{
				"example_tool": mysqlexecutesql.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:   "mysql-execute-sql",
					Source: "my-instance",
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

func TestGetAnnotations(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	boolPtr := func(b bool) *bool { return &b }
	readOnlySrc := &testutils.MockSource{MockSourceConfig: testutils.MockSourceConfig{ReadOnly: true}}
	readWriteSrc := &testutils.MockSource{MockSourceConfig: testutils.MockSourceConfig{ReadOnly: false}}

	tests := []struct {
		desc        string
		src         sources.Source
		annotations *tools.ToolAnnotations
		want        *tools.ToolAnnotations
	}{
		{
			desc: "nil source returns default destructive annotations unmodified",
			src:  nil,
			want: tools.NewDestructiveAnnotations(),
		},
		{
			desc: "read-write source returns default destructive annotations unmodified",
			src:  readWriteSrc,
			want: tools.NewDestructiveAnnotations(),
		},
		{
			desc: "read-only source dynamically flips default destructive annotations to read-only",
			src:  readOnlySrc,
			want: tools.NewReadOnlyAnnotations(),
		},
		{
			desc:        "read-only source with explicit read-only base remains read-only",
			src:         readOnlySrc,
			annotations: tools.NewReadOnlyAnnotations(),
			want:        tools.NewReadOnlyAnnotations(),
		},
		{
			desc: "read-only source preserves custom hints (idempotent, openWorld) when flipped",
			src:  readOnlySrc,
			annotations: &tools.ToolAnnotations{
				DestructiveHint: boolPtr(true),
				ReadOnlyHint:    boolPtr(false),
				IdempotentHint:  boolPtr(true),
				OpenWorldHint:   boolPtr(true),
			},
			want: &tools.ToolAnnotations{
				ReadOnlyHint:   boolPtr(true),
				IdempotentHint: boolPtr(true),
				OpenWorldHint:  boolPtr(true),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := mysqlexecutesql.Config{
				ConfigBase:  tools.ConfigBase{Name: "mysql-execute-sql", Description: "execute sql query"},
				Type:        "mysql-execute-sql",
				Source:      "my-instance",
				Annotations: tc.annotations,
			}
			tool, err := cfg.Initialize(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			got := tool.GetAnnotations(tc.src)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("GetAnnotations() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
