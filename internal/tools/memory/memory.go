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

// Package memory provides the PostgreSQL schema definitions, common configuration,
// and table bootstrap utilities for agent memory tools.
package memory

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// DefaultTableName is the default PostgreSQL table used to store agent memories.
	DefaultTableName = "toolbox_agent_memories"

	// DefaultUserID is the fallback user identity in unauthenticated mode.
	DefaultUserID = "default"

	// DefaultUserIDField is the JWT claim field extracted for authenticated users.
	DefaultUserIDField = "sub"
)

// DefaultCategories defines the allowed category values shared across memory tools.
var DefaultCategories = []any{
	"user_preference",
	"coding_convention",
	"tool_guidance",
	"general_fact",
}

// Memory represents a stored memory entity in PostgreSQL.
type Memory struct {
	MemoryID       string    `json:"memory_id"`
	UserID         string    `json:"user_id"`
	IsGlobal       bool      `json:"is_global"`
	Category       string    `json:"category"`
	Content        string    `json:"content"`
	IsPinned       bool      `json:"is_pinned"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	LastAccessedAt time.Time `json:"last_accessed_at"`
	AccessCount    int32     `json:"access_count"`
	Score          *float64  `json:"score,omitempty"`
}

// Columns defines the standard projection matching ScanMemory, ensuring consistent column order for queries and scans.
const Columns = `memory_id::text, user_id, is_global, category, content, is_pinned, created_at, updated_at, last_accessed_at, access_count`

// validTableIdentifierRegex matches a standard or schema-qualified PostgreSQL identifier.
var validTableIdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`)

// IsValidSQLIdentifier checks if an identifier is safe for SQL interpolation as a table name.
func IsValidSQLIdentifier(s string) bool {
	return validTableIdentifierRegex.MatchString(s)
}

// CompatibleSource is satisfied by sources exposing a PostgreSQL connection pool.
type CompatibleSource interface {
	sources.Source
	PostgresPool() *pgxpool.Pool
}

// Config holds common configuration fields for all memory tools.
type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	TableName        string                 `yaml:"tableName"`
	AuthService      string                 `yaml:"authService"`
	UserIDField      string                 `yaml:"userIdField"`
	DefaultUserID    string                 `yaml:"defaultUserId"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

// Resolve validates and populates default settings for the shared configuration.
func (c Config) Resolve() (Config, error) {
	c.DefaultUserID = strings.TrimSpace(c.DefaultUserID)
	c.UserIDField = strings.TrimSpace(c.UserIDField)
	c.AuthService = strings.TrimSpace(c.AuthService)
	c.TableName = strings.TrimSpace(c.TableName)

	if c.DefaultUserID == "" {
		c.DefaultUserID = DefaultUserID
	}
	if c.UserIDField == "" {
		c.UserIDField = DefaultUserIDField
	}
	if c.TableName == "" {
		c.TableName = DefaultTableName
	}
	if !IsValidSQLIdentifier(c.TableName) {
		return c, fmt.Errorf("invalid tableName %q: must be a valid SQL identifier", c.TableName)
	}
	if c.AuthService != "" && len(c.AuthRequired) == 0 {
		c.AuthRequired = []string{c.AuthService}
	}
	return c, nil
}

// Validate checks that the tool's source satisfies CompatibleSource.
func (c Config) Validate(source sources.Source) error {
	if _, ok := source.(CompatibleSource); !ok {
		return fmt.Errorf("tool %q: source %q is not compatible with postgres memory tools (requires PostgresPool)", c.Name, c.Source)
	}
	return nil
}

// UserIDParameter builds the user_id parameter definition when an AuthService is configured.
// When unauthenticated, user_id is set server-side to DefaultUserID and is not exposed as a tool parameter.
func (c Config) UserIDParameter() parameters.Parameter {
	if c.AuthService == "" {
		return nil
	}
	return parameters.NewStringParameter(
		"user_id",
		"User ID bound from the caller ID token.",
		parameters.WithStringAuth([]parameters.ParamAuthService{{Name: c.AuthService, Field: c.UserIDField}}),
	)
}

// ResolveUserID resolves the user ID from the parameters or returns the default.
// In auth mode, it extracts this claim from params. In unauthenticated mode, it returns DefaultUserID, set via env var.
func (c Config) ResolveUserID(params parameters.ParamValues) (string, error) {
	if c.AuthService != "" {
		userID, _ := params.AsMap()["user_id"].(string)
		if userID == "" {
			return "", fmt.Errorf("user_id could not be resolved from auth token")
		}
		return userID, nil
	}
	return c.DefaultUserID, nil
}

// Schema returns the DDL statements that bootstrap the memory table and full-text search index.
func Schema(table string) (string, error) {
	if table == "" {
		table = DefaultTableName
	}
	if !IsValidSQLIdentifier(table) {
		return "", fmt.Errorf("invalid tableName %q: must be a valid SQL identifier", table)
	}

	idxPrefix := strings.ReplaceAll(table, ".", "_")
	ddl := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
    memory_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          VARCHAR(255) NOT NULL,
    is_global        BOOLEAN NOT NULL DEFAULT FALSE,
    category         VARCHAR(64) NOT NULL,
    content          TEXT NOT NULL,
    tsv              tsvector GENERATED ALWAYS AS (
                         setweight(to_tsvector('english', coalesce(category, '')), 'B') ||
                         setweight(to_tsvector('english', content), 'A')
                     ) STORED,
    is_pinned        BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    access_count     INT NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS %s_tsv_idx ON %s USING gin (tsv);
CREATE INDEX IF NOT EXISTS %s_category_user_idx ON %s (category, user_id, is_global);
`, table, idxPrefix, table, idxPrefix, table)

	return ddl, nil
}

var (
	tableMu    sync.Mutex
	tablesDone = make(map[string]bool)
)

// EnsureMemoryTable creates the agent memories table and indexes once per table identifier.
func EnsureMemoryTable(ctx context.Context, pool *pgxpool.Pool, table string) error {
	if table == "" {
		table = DefaultTableName
	}
	if !IsValidSQLIdentifier(table) {
		return fmt.Errorf("invalid tableName %q: must be a valid SQL identifier", table)
	}

	tableMu.Lock()
	defer tableMu.Unlock()
	if tablesDone[table] {
		return nil
	}

	ddl, err := Schema(table)
	if err != nil {
		return err
	}

	if _, err := pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("unable to bootstrap memory table %q: %w", table, err)
	}

	tablesDone[table] = true
	return nil
}

// ScanMemory scans a single row produced with Columns, so changes to column type or table layout only require updating this function.
func ScanMemory(rows pgx.Rows) (Memory, error) {
	var m Memory
	dest := []any{&m.MemoryID, &m.UserID, &m.IsGlobal, &m.Category, &m.Content, &m.IsPinned, &m.CreatedAt, &m.UpdatedAt, &m.LastAccessedAt, &m.AccessCount}
	if err := rows.Scan(dest...); err != nil {
		return m, err
	}
	return m, nil
}

// CollectMemories drains rows into a slice of Memory, to be used for formatting query results that may return multiple rows.
func CollectMemories(rows pgx.Rows) ([]Memory, error) {
	defer rows.Close()
	out := []Memory{}
	for rows.Next() {
		m, err := ScanMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("unable to parse memory row: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("unable to read memories: %w", err)
	}
	return out, nil
}

// Touch bumps the salience counters (access_count, last_accessed_at) of an existing memory.
func Touch(ctx context.Context, pool *pgxpool.Pool, table, memoryID string) (Memory, error) {
	if table == "" {
		table = DefaultTableName
	}
	if !IsValidSQLIdentifier(table) {
		return Memory{}, fmt.Errorf("invalid table name %q", table)
	}
	stmt := fmt.Sprintf(`UPDATE %s SET access_count = access_count + 1, last_accessed_at = NOW() WHERE memory_id = $1::uuid RETURNING %s`, table, Columns)
	rows, err := pool.Query(ctx, stmt, memoryID)
	if err != nil {
		return Memory{}, fmt.Errorf("unable to refresh existing memory: %w", err)
	}
	ms, err := CollectMemories(rows)
	if err != nil {
		return Memory{}, err
	}
	if len(ms) != 1 {
		return Memory{}, fmt.Errorf("memory %q was not found", memoryID)
	}
	return ms[0], nil
}

// ScopeClause returns the SQL predicate restricting rows to those visible to userParam.
func ScopeClause(userParam string) string {
	return fmt.Sprintf("(user_id = %s OR is_global = TRUE)", userParam)
}
