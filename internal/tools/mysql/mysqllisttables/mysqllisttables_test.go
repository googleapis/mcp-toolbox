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

package mysqllisttables_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	mysqllisttables "github.com/googleapis/mcp-toolbox/internal/tools/mysql/mysqllisttables"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

type mockSourceProvider struct {
	source sources.Source
}

func (m mockSourceProvider) GetSource(name string) (sources.Source, bool) {
	return m.source, true
}

type mockMySQLSource struct {
	database string
	params   []any
}

func (m *mockMySQLSource) SourceType() string {
	return "mysql"
}

func (m *mockMySQLSource) ToConfig() sources.SourceConfig {
	return nil
}

func (m *mockMySQLSource) MySQLPool() *sql.DB {
	return nil
}

func (m *mockMySQLSource) MySQLDatabase() string {
	return m.database
}

func (m *mockMySQLSource) RunSQL(_ context.Context, _ string, params []any) (any, error) {
	m.params = params
	return []any{}, nil
}

func TestParseFromYamlMySQLListTables(t *testing.T) {
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
            type: mysql-list-tables
            source: my-mysql-instance
            description: some description
            authRequired:
                - my-google-auth-service
                - other-auth-service
			`,
			want: server.ToolConfigs{
				"example_tool": mysqllisttables.Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{"my-google-auth-service", "other-auth-service"},
					},
					Type:   "mysql-list-tables",
					Source: "my-mysql-instance",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			_, _, _, got, _, _, err := server.UnmarshalResourceConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}

func TestInvokeScopesListTablesToConfiguredDatabase(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	cfg := mysqllisttables.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "example_tool",
			Description: "some description",
		},
		Type:   "mysql-list-tables",
		Source: "my-mysql-instance",
	}
	tool, err := cfg.Initialize()
	if err != nil {
		t.Fatalf("unable to initialize tool: %s", err)
	}

	source := &mockMySQLSource{database: "app_db"}
	got, invokeErr := tool.Invoke(ctx, mockSourceProvider{source: source}, parameters.ParamValues{
		{Name: "table_names", Value: ""},
		{Name: "output_format", Value: "detailed"},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected invoke error: %s", invokeErr)
	}
	if diff := cmp.Diff([]any{}, got); diff != "" {
		t.Fatalf("unexpected invoke response diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]any{"", "detailed", "app_db"}, source.params); diff != "" {
		t.Fatalf("incorrect SQL params diff (-want +got):\n%s", diff)
	}
}

func TestInvokePreservesUnscopedBehaviorWithoutConfiguredDatabase(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	cfg := mysqllisttables.Config{
		ConfigBase: tools.ConfigBase{
			Name:        "example_tool",
			Description: "some description",
		},
		Type:   "mysql-list-tables",
		Source: "my-mysql-instance",
	}
	tool, err := cfg.Initialize()
	if err != nil {
		t.Fatalf("unable to initialize tool: %s", err)
	}

	source := &mockMySQLSource{}
	_, invokeErr := tool.Invoke(ctx, mockSourceProvider{source: source}, parameters.ParamValues{
		{Name: "table_names", Value: "orders"},
		{Name: "output_format", Value: "simple"},
	}, "")
	if invokeErr != nil {
		t.Fatalf("unexpected invoke error: %s", invokeErr)
	}
	if diff := cmp.Diff([]any{"orders", "simple", ""}, source.params); diff != "" {
		t.Fatalf("incorrect SQL params diff (-want +got):\n%s", diff)
	}
}
