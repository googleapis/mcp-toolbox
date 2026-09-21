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

package mssqlgetdatabaseobjectsourcecode_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/mssql/mssqlgetdatabaseobjectsourcecode"
)

func TestParseFromYamlMssqlGetDatabaseObjectSourceCode(t *testing.T) {
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
            type: mssql-get-database-object-source-code
            source: my-mssql-instance
            description: some description
            authRequired:
                - my-google-auth-service
                - other-auth-service
			`,
			want: server.ToolConfigs{
				"example_tool": mssqlgetdatabaseobjectsourcecode.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:   "mssql-get-database-object-source-code",
					Source: "my-mssql-instance",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			_, _, _, got, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}

func TestGetStatement(t *testing.T) {
	tcs := []struct {
		desc       string
		objectType string
		namesOnly  bool
		wantVal    string
	}{
		{
			desc:       "namesOnly true",
			objectType: "tables",
			namesOnly:  true,
			wantVal:    "DECLARE @NamesOnly BIT = 1;",
		},
		{
			desc:       "namesOnly false",
			objectType: "tables",
			namesOnly:  false,
			wantVal:    "DECLARE @NamesOnly BIT = 0;",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			stmt, err := mssqlgetdatabaseobjectsourcecode.GetStatement(tc.objectType, tc.namesOnly)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(stmt, tc.wantVal) {
				t.Errorf("expected statement to contain %q, got: %s", tc.wantVal, stmt)
			}
			if !strings.Contains(stmt, "IF @NamesOnly = 1") {
				t.Errorf("expected statement to contain IF @NamesOnly = 1 check")
			}
			if !strings.Contains(stmt, "#SqlResourceList") {
				t.Errorf("expected statement to contain #SqlResourceList in ELSE block")
			}
		})
	}
}
