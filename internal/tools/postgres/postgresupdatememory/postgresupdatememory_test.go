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

package postgresupdatememory_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/memory"
	"github.com/googleapis/mcp-toolbox/internal/tools/postgres/postgresupdatememory"
	"github.com/googleapis/mcp-toolbox/internal/util"
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
            name: update_memory
            type: postgres-update-memory
            source: my-pg
            authService: my-auth
            userIdField: email
	`
	want := server.ToolConfigs{
		"update_memory": postgresupdatememory.Config{
			Config: memory.Config{
				ConfigBase:  tools.ConfigBase{Name: "update_memory", AuthRequired: []string{}},
				Type:        "postgres-update-memory",
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

	t.Run("default parameters, categories and annotations", func(t *testing.T) {
		cfg := postgresupdatememory.Config{
			Config: memory.Config{
				ConfigBase: tools.ConfigBase{Name: "update_memory"},
				Type:       "postgres-update-memory",
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
		gotRequired := map[string]bool{}
		for _, p := range manifest.Parameters {
			gotParamNames = append(gotParamNames, p.Name)
			gotRequired[p.Name] = p.Required
		}

		wantParamNames := []string{"memory_id", "content", "category", "is_pinned"}
		if diff := cmp.Diff(wantParamNames, gotParamNames); diff != "" {
			t.Errorf("parameters diff: %s", diff)
		}
		wantRequired := map[string]bool{"memory_id": true, "content": false, "category": false, "is_pinned": false}
		if diff := cmp.Diff(wantRequired, gotRequired); diff != "" {
			t.Errorf("required parameters diff: %s", diff)
		}

		params, err := tool.GetParameters(nil)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		for _, p := range params {
			if sp, ok := p.(*parameters.StringParameter); ok && sp.GetName() == "category" {
				if diff := cmp.Diff(memory.DefaultCategories, sp.AllowedValues); diff != "" {
					t.Errorf("category allowed values diff: %s", diff)
				}
				if _, err := sp.Parse("invalid_category"); err == nil {
					t.Errorf("expected error parsing invalid category, got nil")
				}
				if _, err := sp.Parse("user_preference"); err != nil {
					t.Errorf("unexpected error parsing valid category: %v", err)
				}
			}
			// is_pinned must not default to false, otherwise omitting it would unpin the memory.
			if bp, ok := p.(*parameters.BooleanParameter); ok && bp.GetName() == "is_pinned" {
				if bp.GetDefault() != nil {
					t.Errorf("expected is_pinned to have no default, got %v", bp.GetDefault())
				}
			}
			if p.GetName() == "user_id" {
				t.Fatalf("user_id should not be exposed in unauthenticated mode")
			}
		}

		if diff := cmp.Diff(tools.NewDestructiveAnnotations(), tool.GetAnnotations(nil)); diff != "" {
			t.Errorf("annotations diff: %s", diff)
		}
	})

	t.Run("authenticated mode includes user_id parameter", func(t *testing.T) {
		cfg := postgresupdatememory.Config{
			Config: memory.Config{
				ConfigBase:  tools.ConfigBase{Name: "update_memory"},
				Type:        "postgres-update-memory",
				Source:      "pg",
				AuthService: "my-auth",
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

		wantParamNames := []string{"memory_id", "content", "category", "is_pinned", "user_id"}
		if diff := cmp.Diff(wantParamNames, gotParamNames); diff != "" {
			t.Errorf("parameters diff: %s", diff)
		}
	})

	t.Run("invalid table name errors", func(t *testing.T) {
		cfg := postgresupdatememory.Config{
			Config: memory.Config{
				ConfigBase: tools.ConfigBase{Name: "update_memory"},
				Type:       "postgres-update-memory",
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

func TestInvokeValidation(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	cfg := postgresupdatememory.Config{
		Config: memory.Config{
			ConfigBase: tools.ConfigBase{Name: "update_memory"},
			Type:       "postgres-update-memory",
			Source:     "pg",
		},
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unexpected error initializing: %s", err)
	}

	// These inputs are rejected before the database is touched, so a source without a pool is enough.
	const validID = "b1945ee6-a407-48a9-a9e9-b00f710cae1c"
	tcs := []struct {
		desc    string
		params  parameters.ParamValues
		wantErr string
	}{
		{
			desc:    "invalid memory_id",
			params:  parameters.ParamValues{{Name: "memory_id", Value: "not-a-uuid"}, {Name: "is_pinned", Value: true}},
			wantErr: `memory_id "not-a-uuid" is not a valid UUID`,
		},
		{
			desc: "no fields to update",
			params: parameters.ParamValues{
				{Name: "memory_id", Value: validID},
				{Name: "content", Value: nil},
				{Name: "category", Value: nil},
				{Name: "is_pinned", Value: nil},
			},
			wantErr: "at least one of content, category or is_pinned must be provided",
		},
		{
			desc:    "blank content",
			params:  parameters.ParamValues{{Name: "memory_id", Value: validID}, {Name: "content", Value: "   "}},
			wantErr: "content must not be empty",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, err := tool.Invoke(ctx, mockCompatibleSource{}, tc.params, "")
			if err == nil {
				t.Fatalf("expected error %q, got nil", tc.wantErr)
			}
			if err.Category() != util.CategoryAgent {
				t.Errorf("expected an agent error, got category %q", err.Category())
			}
			if err.Error() != tc.wantErr {
				t.Errorf("unexpected error: got %q, want %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidateSource(t *testing.T) {
	cfg := postgresupdatememory.Config{
		Config: memory.Config{
			ConfigBase: tools.ConfigBase{Name: "update_memory"},
			Type:       "postgres-update-memory",
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
