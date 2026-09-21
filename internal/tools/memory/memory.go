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

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// DefaultTableName is the default PostgreSQL table used to store agent memories.
	DefaultTableName = "toolbox_agent_memories"
)

// validTableIdentifierRegex matches a standard or schema-qualified PostgreSQL identifier.
var validTableIdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`)

// IsValidSQLIdentifier checks if an identifier is safe for SQL interpolation as a table name.
func IsValidSQLIdentifier(s string) bool {
	return validTableIdentifierRegex.MatchString(s)
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
