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

package memory_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/tools/memory"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestSchemaDefaultTable(t *testing.T) {
	ddl, err := memory.Schema("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(ddl, "CREATE TABLE IF NOT EXISTS toolbox_agent_memories") {
		t.Errorf("ddl missing table creation:\n%s", ddl)
	}
	if !strings.Contains(ddl, "tsvector GENERATED ALWAYS AS") {
		t.Errorf("ddl missing tsvector column:\n%s", ddl)
	}
	if !strings.Contains(ddl, "is_global       BOOLEAN DEFAULT FALSE") {
		t.Errorf("ddl missing is_global column:\n%s", ddl)
	}

	if !strings.Contains(ddl, "CREATE INDEX IF NOT EXISTS toolbox_agent_memories_tsv_idx ON toolbox_agent_memories USING gin (tsv)") {
		t.Errorf("ddl missing GIN index:\n%s", ddl)
	}

	if !strings.Contains(ddl, "CREATE INDEX IF NOT EXISTS toolbox_agent_memories_user_category_idx ON toolbox_agent_memories (user_id, category)") {
		t.Errorf("ddl missing user_category index:\n%s", ddl)
	}
}

func TestSchemaCustomTable(t *testing.T) {
	ddl, err := memory.Schema("custom_schema.my_memories")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(ddl, "CREATE TABLE IF NOT EXISTS custom_schema.my_memories") {
		t.Errorf("ddl missing custom table creation:\n%s", ddl)
	}
	if !strings.Contains(ddl, "CREATE INDEX IF NOT EXISTS custom_schema_my_memories_tsv_idx ON custom_schema.my_memories USING gin (tsv)") {
		t.Errorf("ddl missing custom GIN index prefix:\n%s", ddl)
	}
	if !strings.Contains(ddl, "CREATE INDEX IF NOT EXISTS custom_schema_my_memories_user_category_idx ON custom_schema.my_memories (user_id, category)") {
		t.Errorf("ddl missing custom user_category index prefix:\n%s", ddl)
	}
}

func TestSchemaInvalidTableNames(t *testing.T) {
	invalidNames := []string{
		"toolbox_agent_memories; DROP TABLE users; --",
		"invalid name with spaces",
		"123startsWithNumber",
		"schema..double_dot",
		"a.b.c",
		"table-with-dashes",
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			_, err := memory.Schema(name)
			if err == nil {
				t.Fatalf("expected error for invalid table name %q, got nil", name)
			}
		})
	}
}

func TestIsValidSQLIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"toolbox_agent_memories", true},
		{"memories", true},
		{"_leading_underscore", true},
		{"custom_schema.my_table", true},
		{"public.toolbox_agent_memories", true},
		{"table123", true},
		{"", false},
		{"123table", false},
		{"table with spaces", false},
		{"table;drop", false},
		{"a.b.c", false},
		{"table-dash", false},
		{"table'quote", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := memory.IsValidSQLIdentifier(tc.input)
			if got != tc.want {
				t.Errorf("IsValidSQLIdentifier(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestConfigResolve(t *testing.T) {
	t.Run("applies defaults and trims whitespace", func(t *testing.T) {
		cfg := memory.Config{
			AuthService:   "  my-auth  ",
			UserIDField:   "  email  ",
			DefaultUserID: "  alice  ",
			TableName:     "  my_memories  ",
		}
		resolved, err := cfg.Resolve()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.AuthService != "my-auth" {
			t.Errorf("expected AuthService 'my-auth', got %q", resolved.AuthService)
		}
		if resolved.UserIDField != "email" {
			t.Errorf("expected UserIDField 'email', got %q", resolved.UserIDField)
		}
		if resolved.DefaultUserID != "alice" {
			t.Errorf("expected DefaultUserID 'alice', got %q", resolved.DefaultUserID)
		}
		if resolved.TableName != "my_memories" {
			t.Errorf("expected TableName 'my_memories', got %q", resolved.TableName)
		}
	})

	t.Run("empty fields fall back to package defaults", func(t *testing.T) {
		cfg := memory.Config{}
		resolved, err := cfg.Resolve()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.AuthService != "" {
			t.Errorf("expected empty AuthService, got %q", resolved.AuthService)
		}
		if resolved.UserIDField != memory.DefaultUserIDField {
			t.Errorf("expected UserIDField %q, got %q", memory.DefaultUserIDField, resolved.UserIDField)
		}
		if resolved.DefaultUserID != memory.DefaultUserID {
			t.Errorf("expected DefaultUserID %q, got %q", memory.DefaultUserID, resolved.DefaultUserID)
		}
		if resolved.TableName != memory.DefaultTableName {
			t.Errorf("expected TableName %q, got %q", memory.DefaultTableName, resolved.TableName)
		}
	})
}

func TestUserIDParameter(t *testing.T) {
	t.Run("unauthenticated mode without authService", func(t *testing.T) {
		cfg := memory.Config{
			DefaultUserID: "default_user",
		}
		resolved, err := cfg.Resolve()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		param := resolved.UserIDParameter()
		sp, ok := param.(*parameters.StringParameter)
		if !ok {
			t.Fatalf("expected *parameters.StringParameter, got %T", param)
		}
		if sp.GetName() != "user_id" {
			t.Errorf("expected name 'user_id', got %q", sp.GetName())
		}
		if sp.GetDefault() != "default_user" {
			t.Errorf("expected default 'default_user', got %v", sp.GetDefault())
		}
		if len(sp.GetAuthServices()) != 0 {
			t.Errorf("expected no auth services, got %v", sp.GetAuthServices())
		}
	})

	t.Run("authenticated mode with authService and default userIdField", func(t *testing.T) {
		cfg := memory.Config{
			AuthService: "google-auth",
		}
		resolved, err := cfg.Resolve()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		param := resolved.UserIDParameter()
		sp, ok := param.(*parameters.StringParameter)
		if !ok {
			t.Fatalf("expected *parameters.StringParameter, got %T", param)
		}
		wantAuth := []parameters.ParamAuthService{
			{Name: "google-auth", Field: "sub"},
		}
		if diff := cmp.Diff(wantAuth, sp.GetAuthServices()); diff != "" {
			t.Errorf("auth services diff: %s", diff)
		}
	})

	t.Run("authenticated mode with custom userIdField", func(t *testing.T) {
		cfg := memory.Config{
			AuthService: "generic-auth",
			UserIDField: "email",
		}
		resolved, err := cfg.Resolve()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		param := resolved.UserIDParameter()
		sp, ok := param.(*parameters.StringParameter)
		if !ok {
			t.Fatalf("expected *parameters.StringParameter, got %T", param)
		}
		wantAuth := []parameters.ParamAuthService{
			{Name: "generic-auth", Field: "email"},
		}
		if diff := cmp.Diff(wantAuth, sp.GetAuthServices()); diff != "" {
			t.Errorf("auth services diff: %s", diff)
		}
	})
}
