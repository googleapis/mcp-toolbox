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

// Package memory provides the PostgreSQL schema definitions and table bootstrap
// utilities for agent memory tools.
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

	// DefaultUserIDField is the ID-token claim used as user_id when an authService is configured.
	DefaultUserIDField = "sub"

	// DefaultUserID is used as user_id when no authService is configured.
	DefaultUserID = "default"

	VisibilityPrivate = "PRIVATE"
	VisibilityGlobal  = "GLOBAL"
)

// validTableIdentifierRegex matches a standard or schema-qualified PostgreSQL identifier.
var validTableIdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`)

// IsValidSQLIdentifier checks if an identifier is safe for SQL interpolation as a table name.
func IsValidSQLIdentifier(s string) bool {
	return validTableIdentifierRegex.MatchString(s)
}

// CompatibleSource is satisfied by sources that connect to PostgreSQL,
// including generic Postgres, Cloud SQL Postgres, and AlloyDB Postgres.
type CompatibleSource interface {
	sources.Source
	PostgresPool() *pgxpool.Pool
}

// Config holds the common configuration fields for memory tools.
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

// Resolve fills defaults and validates the shared config.
func (c Config) Resolve() (Config, error) {
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
	return c, nil
}

// Validate checks that the tool's source satisfies CompatibleSource.
func (c Config) Validate(source sources.Source) error {
	if _, ok := source.(CompatibleSource); !ok {
		return fmt.Errorf("tool %q: source %q is not compatible with postgres memory tools (requires PostgresPool)", c.Name, c.Source)
	}
	return nil
}

// UserIDParameter builds the user_id tool parameter.
func (c Config) UserIDParameter() parameters.Parameter {
	if c.AuthService != "" {
		return parameters.NewStringParameter(
			"user_id",
			"User ID bound from the caller ID token.",
			parameters.WithStringAuth([]parameters.ParamAuthService{{Name: c.AuthService, Field: c.UserIDField}}),
		)
	}
	return parameters.NewStringParameter(
		"user_id",
		"User ID scoping memories.",
		parameters.WithStringDefault(c.DefaultUserID),
	)
}

// Memory represents a stored memory object in PostgreSQL.
type Memory struct {
	MemoryID       string    `json:"memory_id"`
	UserID         string    `json:"user_id"`
	Visibility     string    `json:"visibility"`
	Category       string    `json:"category"`
	Content        string    `json:"content"`
	IsPinned       bool      `json:"is_pinned"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	LastAccessedAt time.Time `json:"last_accessed_at"`
	AccessCount    int32     `json:"access_count"`
	Score          *float64  `json:"score,omitempty"`
}

// Columns is the SELECT list matching ScanMemory (excluding score).
const Columns = `memory_id::text, user_id, visibility, category, content, is_pinned, created_at, updated_at, last_accessed_at, access_count`

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
    is_global       BOOLEAN DEFAULT FALSE,
    category         VARCHAR(64) NOT NULL,
    content          TEXT NOT NULL,
    tsv              tsvector GENERATED ALWAYS AS (
                         setweight(to_tsvector('english', coalesce(category, '')), 'B') ||
                         setweight(to_tsvector('english', content), 'A')
                     ) STORED,
    is_pinned        BOOLEAN DEFAULT FALSE,
    created_at       TIMESTAMPTZ DEFAULT NOW(),
    updated_at       TIMESTAMPTZ DEFAULT NOW(),
    last_accessed_at TIMESTAMPTZ DEFAULT NOW(),
    access_count     INT DEFAULT 1
);
CREATE INDEX IF NOT EXISTS %s_tsv_idx ON %s USING gin (tsv);
CREATE INDEX IF NOT EXISTS %s_user_category_idx ON %s (user_id, category);
`, table, idxPrefix, table, idxPrefix, table)

	return ddl, nil
}

var (
	tableMu   sync.Mutex
	tableDone bool
)

// EnsureMemoryTable creates the agent memories table and indexes. Assumes this will be the only memory table per server.
func EnsureMemoryTable(ctx context.Context, pool *pgxpool.Pool, table string) error {
	tableMu.Lock()
	defer tableMu.Unlock()
	if tableDone {
		return nil
	}

	ddl, err := Schema(table)
	if err != nil {
		return err
	}

	if _, err := pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("unable to bootstrap memory table %q: %w", table, err)
	}

	tableDone = true
	return nil
}

// ScanMemory scans a single row produced with Columns.
func ScanMemory(rows pgx.Rows) (Memory, error) {
	var m Memory
	var isPinned *bool
	var accessCount *int32
	dest := []any{&m.MemoryID, &m.UserID, &m.Visibility, &m.Category, &m.Content, &isPinned, &m.CreatedAt, &m.UpdatedAt, &m.LastAccessedAt, &accessCount}
	if err := rows.Scan(dest...); err != nil {
		return m, err
	}
	if isPinned != nil {
		m.IsPinned = *isPinned
	}
	if accessCount != nil {
		m.AccessCount = *accessCount
	}
	return m, nil
}

// CollectMemories drains rows into a slice of Memory.
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

// Touch bumps the salience counters (access_count, last_accessed_at) of an
// existing memory and returns the refreshed row.
func Touch(ctx context.Context, pool *pgxpool.Pool, table, memoryID string) (Memory, error) {
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

// ScopeClause returns the SQL predicate restricting rows to those visible to
// the given user, where userParam is the positional placeholder (e.g. "$1").
func ScopeClause(userParam string) string {
	return fmt.Sprintf("(user_id = %s OR visibility = '%s')", userParam, VisibilityGlobal)
}
