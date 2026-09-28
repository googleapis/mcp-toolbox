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

package postgresdeletememory_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/memory"
	"github.com/googleapis/mcp-toolbox/internal/tools/postgres/postgresdeletememory"
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
            name: delete_memory
            type: postgres-delete-memory
            source: my-pg
            authService: my-auth
            userIdField: email
	`
	want := server.ToolConfigs{
		"delete_memory": postgresdeletememory.Config{
			Config: memory.Config{
				ConfigBase:  tools.ConfigBase{Name: "delete_memory", AuthRequired: []string{}},
				Type:        "postgres-delete-memory",
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

	t.Run("default parameters and annotations", func(t *testing.T) {
		cfg := postgresdeletememory.Config{
			Config: memory.Config{
				ConfigBase: tools.ConfigBase{Name: "delete_memory"},
				Type:       "postgres-delete-memory",
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

		wantParamNames := []string{"memory_id"}
		if diff := cmp.Diff(wantParamNames, gotParamNames); diff != "" {
			t.Errorf("parameters diff: %s", diff)
		}
		wantRequired := map[string]bool{"memory_id": true}
		if diff := cmp.Diff(wantRequired, gotRequired); diff != "" {
			t.Errorf("required parameters diff: %s", diff)
		}

		params, err := tool.GetParameters(nil)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		for _, p := range params {
			if p.GetName() == "user_id" {
				t.Fatalf("user_id should not be exposed in unauthenticated mode")
			}
		}

		if diff := cmp.Diff(tools.NewDestructiveAnnotations(), tool.GetAnnotations(nil)); diff != "" {
			t.Errorf("annotations diff: %s", diff)
		}
	})

	t.Run("authenticated mode includes user_id parameter", func(t *testing.T) {
		cfg := postgresdeletememory.Config{
			Config: memory.Config{
				ConfigBase:  tools.ConfigBase{Name: "delete_memory"},
				Type:        "postgres-delete-memory",
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

		wantParamNames := []string{"memory_id", "user_id"}
		if diff := cmp.Diff(wantParamNames, gotParamNames); diff != "" {
			t.Errorf("parameters diff: %s", diff)
		}
	})

	t.Run("invalid table name errors", func(t *testing.T) {
		cfg := postgresdeletememory.Config{
			Config: memory.Config{
				ConfigBase: tools.ConfigBase{Name: "delete_memory"},
				Type:       "postgres-delete-memory",
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
	cfg := postgresdeletememory.Config{
		Config: memory.Config{
			ConfigBase: tools.ConfigBase{Name: "delete_memory"},
			Type:       "postgres-delete-memory",
			Source:     "pg",
		},
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("unexpected error initializing: %s", err)
	}

	// Invalid UUIDs are rejected before the database is touched, so a source without a pool is enough.
	tcs := []struct {
		desc    string
		params  parameters.ParamValues
		wantErr string
	}{
		{
			desc:    "invalid memory_id",
			params:  parameters.ParamValues{{Name: "memory_id", Value: "not-a-uuid"}},
			wantErr: `memory_id "not-a-uuid" is not a valid UUID`,
		},
		{
			desc:    "empty memory_id",
			params:  parameters.ParamValues{{Name: "memory_id", Value: "   "}},
			wantErr: `memory_id "" is not a valid UUID`,
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
	cfg := postgresdeletememory.Config{
		Config: memory.Config{
			ConfigBase: tools.ConfigBase{Name: "delete_memory"},
			Type:       "postgres-delete-memory",
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
