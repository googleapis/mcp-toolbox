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
	"context"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/memory"
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

type mockEmbeddingModel struct {
	dim int
}

func (m mockEmbeddingModel) EmbeddingModelType() string                    { return "mock" }
func (m mockEmbeddingModel) ToConfig() embeddingmodels.EmbeddingModelConfig { return nil }
func (m mockEmbeddingModel) EmbedParameters(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, m.dim)
		if m.dim > 0 {
			vec[0] = 0.5
		}
		out[i] = vec
	}
	return out, nil
}

type mockPrimitiveManager struct {
	models map[string]embeddingmodels.EmbeddingModel
}

func (m mockPrimitiveManager) GetSource(string) (sources.Source, bool) { return nil, false }
func (m mockPrimitiveManager) GetEmbeddingModel(name string) (embeddingmodels.EmbeddingModel, bool) {
	em, ok := m.models[name]
	return em, ok
}

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
	if !strings.Contains(ddl, "embedding_model  TEXT") {
		t.Errorf("ddl missing embedding_model column:\n%s", ddl)
	}
	if !strings.Contains(ddl, "is_global        BOOLEAN NOT NULL DEFAULT FALSE") {
		t.Errorf("ddl missing is_global column:\n%s", ddl)
	}
	if !strings.Contains(ddl, "CREATE INDEX IF NOT EXISTS toolbox_agent_memories_tsv_idx ON toolbox_agent_memories USING gin (tsv)") {
		t.Errorf("ddl missing GIN index:\n%s", ddl)
	}
	if !strings.Contains(ddl, "CREATE INDEX IF NOT EXISTS toolbox_agent_memories_category_user_idx ON toolbox_agent_memories (category, user_id, is_global)") {
		t.Errorf("ddl missing category_user index:\n%s", ddl)
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
	if !strings.Contains(ddl, "CREATE INDEX IF NOT EXISTS custom_schema_my_memories_category_user_idx ON custom_schema.my_memories (category, user_id, is_global)") {
		t.Errorf("ddl missing custom category_user index prefix:\n%s", ddl)
	}
}

func TestVectorSchema(t *testing.T) {
	ddl, err := memory.VectorSchema("custom_schema.my_memories")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(ddl, "CREATE EXTENSION IF NOT EXISTS vector;") {
		t.Errorf("vector ddl missing extension:\n%s", ddl)
	}
	if !strings.Contains(ddl, "ALTER TABLE custom_schema.my_memories ADD COLUMN IF NOT EXISTS embedding VECTOR(768);") {
		t.Errorf("vector ddl missing embedding column:\n%s", ddl)
	}
	if !strings.Contains(ddl, "ALTER TABLE custom_schema.my_memories ADD COLUMN IF NOT EXISTS embedding_model TEXT;") {
		t.Errorf("vector ddl missing embedding_model column:\n%s", ddl)
	}
	if !strings.Contains(ddl, "CREATE INDEX IF NOT EXISTS custom_schema_my_memories_embedding_idx ON custom_schema.my_memories USING hnsw (embedding vector_cosine_ops);") {
		t.Errorf("vector ddl missing HNSW index:\n%s", ddl)
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
			if _, err := memory.Schema(name); err == nil {
				t.Fatalf("expected error for invalid table name %q in Schema, got nil", name)
			}
			if _, err := memory.VectorSchema(name); err == nil {
				t.Fatalf("expected error for invalid table name %q in VectorSchema, got nil", name)
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
			EmbeddingModel: "  my_embedder  ",
			DefaultUserID:  "  alice  ",
			TableName:      "  my_memories  ",
			AuthService:    "  my_auth  ",
		}
		resolved, err := cfg.Resolve()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.EmbeddingModel != "my_embedder" {
			t.Errorf("expected EmbeddingModel 'my_embedder', got %q", resolved.EmbeddingModel)
		}
		if resolved.DefaultUserID != "alice" {
			t.Errorf("expected DefaultUserID 'alice', got %q", resolved.DefaultUserID)
		}
		if resolved.TableName != "my_memories" {
			t.Errorf("expected TableName 'my_memories', got %q", resolved.TableName)
		}
		if resolved.AuthService != "my_auth" {
			t.Errorf("expected AuthService 'my_auth', got %q", resolved.AuthService)
		}
		if len(resolved.AuthRequired) != 1 || resolved.AuthRequired[0] != "my_auth" {
			t.Errorf("expected AuthRequired ['my_auth'], got %v", resolved.AuthRequired)
		}
	})

	t.Run("empty fields fall back to package defaults", func(t *testing.T) {
		cfg := memory.Config{}
		resolved, err := cfg.Resolve()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.DefaultUserID != memory.DefaultUserID {
			t.Errorf("expected DefaultUserID %q, got %q", memory.DefaultUserID, resolved.DefaultUserID)
		}
		if resolved.TableName != memory.DefaultTableName {
			t.Errorf("expected TableName %q, got %q", memory.DefaultTableName, resolved.TableName)
		}
		if resolved.UserIDField != memory.DefaultUserIDField {
			t.Errorf("expected UserIDField %q, got %q", memory.DefaultUserIDField, resolved.UserIDField)
		}
	})

	t.Run("invalid table name returns error", func(t *testing.T) {
		cfg := memory.Config{
			TableName: "drop table users; --",
		}
		if _, err := cfg.Resolve(); err == nil {
			t.Errorf("expected error for invalid table name in Resolve, got nil")
		}
	})
}

func TestConfigValidate(t *testing.T) {
	cfg := memory.Config{
		ConfigBase: tools.ConfigBase{Name: "test_tool"},
		Source:     "test_source",
	}

	if err := cfg.Validate(mockCompatibleSource{}); err != nil {
		t.Errorf("expected mockCompatibleSource to pass validation, got %v", err)
	}
	if err := cfg.Validate(mockIncompatibleSource{}); err == nil {
		t.Errorf("expected mockIncompatibleSource to fail validation, got nil")
	}
}

func TestUserIDParameter(t *testing.T) {
	t.Run("unauthenticated mode returns nil (server-bound)", func(t *testing.T) {
		cfg := memory.Config{}
		resolved, err := cfg.Resolve()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if param := resolved.UserIDParameter(); param != nil {
			t.Errorf("expected UserIDParameter() to be nil in unauthenticated mode, got %v", param)
		}

		userID, err := resolved.ResolveUserID(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if userID != memory.DefaultUserID {
			t.Errorf("expected userID %q, got %q", memory.DefaultUserID, userID)
		}
	})

	t.Run("authenticated mode configures auth service", func(t *testing.T) {
		cfg := memory.Config{
			AuthService: "google-auth",
			UserIDField: "email",
		}
		resolved, err := cfg.Resolve()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		param := resolved.UserIDParameter()
		if param == nil {
			t.Fatalf("expected non-nil UserIDParameter() in auth mode")
		}
		sp, ok := param.(*parameters.StringParameter)
		if !ok {
			t.Fatalf("expected *parameters.StringParameter, got %T", param)
		}
		authServices := sp.GetAuthServices()
		if len(authServices) != 1 || authServices[0].Name != "google-auth" || authServices[0].Field != "email" {
			t.Errorf("expected authServices [{google-auth email}], got %v", authServices)
		}
	})
}

func TestEmbeddingParameterAndEmbedParams(t *testing.T) {
	t.Run("returns nil when EmbeddingModel is empty", func(t *testing.T) {
		cfg := memory.Config{}
		if p := cfg.EmbeddingParameter("content_embedding", "content"); p != nil {
			t.Fatalf("expected nil EmbeddingParameter when EmbeddingModel is empty, got %v", p)
		}
	})

	t.Run("embeds non-empty value and tolerates nil optional value", func(t *testing.T) {
		cfg := memory.Config{EmbeddingModel: "my-model"}
		embParam := cfg.EmbeddingParameter("content_embedding", "content")
		if embParam == nil {
			t.Fatalf("expected non-nil EmbeddingParameter")
		}
		ps := parameters.Parameters{
			parameters.NewStringParameter("content", "content", parameters.WithStringRequired(false)),
			embParam,
		}
		// Hidden from Manifest because ValueFromParam is set.
		if len(ps.Manifest()) != 1 {
			t.Fatalf("expected 1 manifest parameter, got %d", len(ps.Manifest()))
		}

		pMgr := mockPrimitiveManager{
			models: map[string]embeddingmodels.EmbeddingModel{
				"my-model": mockEmbeddingModel{dim: memory.EmbeddingDimensions},
			},
		}

		// Case 1: nil optional content leaves content_embedding nil.
		nilParams := parameters.ParamValues{
			{Name: "content", Value: nil},
			{Name: "content_embedding", Value: nil},
		}
		gotNil, err := parameters.EmbedParams(context.Background(), ps, nilParams, pMgr, embeddingmodels.FormatVectorForPgvector)
		if err != nil {
			t.Fatalf("unexpected error on nil content: %v", err)
		}
		if gotNil.AsMap()["content_embedding"] != nil {
			t.Fatalf("expected nil content_embedding, got %v", gotNil.AsMap()["content_embedding"])
		}

		// Case 2: non-empty content formats pgvector string.
		valParams := parameters.ParamValues{
			{Name: "content", Value: "User prefers Go"},
			{Name: "content_embedding", Value: "User prefers Go"},
		}
		gotVal, err := parameters.EmbedParams(context.Background(), ps, valParams, pMgr, embeddingmodels.FormatVectorForPgvector)
		if err != nil {
			t.Fatalf("unexpected error on non-empty content: %v", err)
		}
		vecStr, ok := gotVal.AsMap()["content_embedding"].(string)
		if !ok || !strings.HasPrefix(vecStr, "[0.5, 0") {
			t.Fatalf("expected formatted pgvector string, got %v", gotVal.AsMap()["content_embedding"])
		}
	})
}

func TestScopeClause(t *testing.T) {
	clause := memory.ScopeClause("$1")
	want := "(user_id = $1 OR is_global = TRUE)"
	if clause != want {
		t.Errorf("ScopeClause($1) = %q, want %q", clause, want)
	}
}
