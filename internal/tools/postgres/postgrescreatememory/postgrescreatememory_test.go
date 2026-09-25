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

package postgrescreatememory_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/memory"
	"github.com/googleapis/mcp-toolbox/internal/tools/postgres/postgrescreatememory"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mockCompatibleSource struct {
	sources.Source
	pool *pgxpool.Pool
}

func (m mockCompatibleSource) PostgresPool() *pgxpool.Pool {
	return m.pool
}

type mockIncompatibleSource struct {
	sources.Source
}

func TestParseFromYaml(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	in := `
            kind: tool
            name: create_memory
            type: postgres-create-memory
            source: my-pg
            authService: my-auth
            userIdField: email
	`
	want := server.ToolConfigs{
		"create_memory": postgrescreatememory.Config{
			Config: memory.Config{
				ConfigBase:  tools.ConfigBase{Name: "create_memory", AuthRequired: []string{}},
				Type:        "postgres-create-memory",
				Source:      "my-pg",
				AuthService: "my-auth",
				UserIDField: "email",
			},
		},
	}
	_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(in))
	if err != nil {
		t.Fatalf("unable to unmarshal: %s", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("incorrect parse: diff %v", diff)
	}
}

func TestInitializeParameters(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	t.Run("default parameters and categories", func(t *testing.T) {
		cfg := postgrescreatememory.Config{
			Config: memory.Config{
				ConfigBase: tools.ConfigBase{Name: "create_memory"},
				Type:       "postgres-create-memory",
				Source:     "pg",
			},
		}
		tool, err := cfg.Initialize(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		manifest, err := tool.Manifest(nil)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}

		var gotParamNames []string
		for _, p := range manifest.Parameters {
			gotParamNames = append(gotParamNames, p.Name)
		}

		wantParamNames := []string{"content", "category", "is_global", "is_pinned"}
		if diff := cmp.Diff(wantParamNames, gotParamNames); diff != "" {
			t.Errorf("parameters diff: %s", diff)
		}

		params, err := tool.GetParameters(nil)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		for _, p := range params {
			if sp, ok := p.(*parameters.StringParameter); ok && sp.GetName() == "category" {
				if diff := cmp.Diff(postgrescreatememory.DefaultCategories, sp.AllowedValues); diff != "" {
					t.Errorf("category allowed values diff: %s", diff)
				}
				if _, err := sp.Parse("invalid_category"); err == nil {
					t.Errorf("expected error parsing invalid category, got nil")
				}
				if _, err := sp.Parse("user_preference"); err != nil {
					t.Errorf("unexpected error parsing valid category: %v", err)
				}
			}
			if bp, ok := p.(*parameters.BooleanParameter); ok && bp.GetName() == "is_global" {
				if bp.GetDefault() != false {
					t.Errorf("expected is_global default to be false, got %v", bp.GetDefault())
				}
			}
		}
	})

	t.Run("invalid table name errors", func(t *testing.T) {
		cfg := postgrescreatememory.Config{
			Config: memory.Config{
				ConfigBase: tools.ConfigBase{Name: "create_memory"},
				Type:       "postgres-create-memory",
				Source:     "pg",
				TableName:  "drop table users; --",
			},
		}
		_, err := cfg.Initialize(ctx)
		if err == nil {
			t.Fatalf("expected error for invalid tableName, got nil")
		}
	})
}

func TestValidateSource(t *testing.T) {
	cfg := postgrescreatememory.Config{
		Config: memory.Config{
			ConfigBase: tools.ConfigBase{Name: "create_memory"},
			Type:       "postgres-create-memory",
			Source:     "pg",
		},
	}
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unexpected error initializing: %s", err)
	}

	if err := tool.ValidateSource(mockCompatibleSource{}); err != nil {
		t.Errorf("expected mockCompatibleSource to pass validation, got %v", err)
	}
	if err := tool.ValidateSource(mockIncompatibleSource{}); err == nil {
		t.Errorf("expected mockIncompatibleSource to fail validation, got nil")
	}
}
