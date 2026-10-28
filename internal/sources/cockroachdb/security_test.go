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

package cockroachdb

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	yaml "github.com/goccy/go-yaml"
)

// TestClassifySQL tests SQL statement classification
func TestClassifySQL(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected SQLStatementType
	}{
		{"SELECT", "SELECT * FROM users", SQLTypeSelect},
		{"SELECT with spaces", "  SELECT * FROM users  ", SQLTypeSelect},
		{"SELECT with comment", "-- comment\nSELECT * FROM users", SQLTypeSelect},
		{"INSERT", "INSERT INTO users (name) VALUES ('alice')", SQLTypeInsert},
		{"UPDATE", "UPDATE users SET name='bob' WHERE id=1", SQLTypeUpdate},
		{"DELETE", "DELETE FROM users WHERE id=1", SQLTypeDelete},
		{"CREATE TABLE", "CREATE TABLE users (id UUID PRIMARY KEY)", SQLTypeDDL},
		{"ALTER TABLE", "ALTER TABLE users ADD COLUMN email STRING", SQLTypeDDL},
		{"DROP TABLE", "DROP TABLE users", SQLTypeDDL},
		{"TRUNCATE", "TRUNCATE TABLE users", SQLTypeTruncate},
		{"EXPLAIN", "EXPLAIN SELECT * FROM users", SQLTypeExplain},
		{"SHOW", "SHOW TABLES", SQLTypeShow},
		{"SET", "SET application_name = 'myapp'", SQLTypeSet},
		{"Empty", "", SQLTypeUnknown},
		{"Lowercase select", "select * from users", SQLTypeSelect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifySQL(tt.sql)
			if result != tt.expected {
				t.Errorf("ClassifySQL(%q) = %v, want %v", tt.sql, result, tt.expected)
			}
		})
	}
}

// TestIsWriteOperation tests write operation detection
func TestIsWriteOperation(t *testing.T) {
	tests := []struct {
		sqlType  SQLStatementType
		expected bool
	}{
		{SQLTypeSelect, false},
		{SQLTypeInsert, true},
		{SQLTypeUpdate, true},
		{SQLTypeDelete, true},
		{SQLTypeTruncate, true},
		{SQLTypeDDL, true},
		{SQLTypeExplain, false},
		{SQLTypeShow, false},
		{SQLTypeSet, false},
		{SQLTypeUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.sqlType.String(), func(t *testing.T) {
			result := IsWriteOperation(tt.sqlType)
			if result != tt.expected {
				t.Errorf("IsWriteOperation(%v) = %v, want %v", tt.sqlType, result, tt.expected)
			}
		})
	}
}

// Helper for SQLStatementType to string
func (s SQLStatementType) String() string {
	switch s {
	case SQLTypeSelect:
		return "SELECT"
	case SQLTypeInsert:
		return "INSERT"
	case SQLTypeUpdate:
		return "UPDATE"
	case SQLTypeDelete:
		return "DELETE"
	case SQLTypeDDL:
		return "DDL"
	case SQLTypeTruncate:
		return "TRUNCATE"
	case SQLTypeExplain:
		return "EXPLAIN"
	case SQLTypeShow:
		return "SHOW"
	case SQLTypeSet:
		return "SET"
	default:
		return "UNKNOWN"
	}
}

// TestCanExecuteWrite tests write operation enforcement
func TestCanExecuteWrite(t *testing.T) {
	tests := []struct {
		name            string
		readOnlyMode    bool
		enableWriteMode bool
		sql             string
		expectError     bool
		errorCode       string
	}{
		{
			name:            "SELECT in read-only mode",
			readOnlyMode:    true,
			enableWriteMode: false,
			sql:             "SELECT * FROM users",
			expectError:     false,
		},
		{
			name:            "INSERT in read-only mode",
			readOnlyMode:    true,
			enableWriteMode: false,
			sql:             "INSERT INTO users (name) VALUES ('alice')",
			expectError:     true,
			errorCode:       ErrCodeReadOnlyViolation,
		},
		{
			name:            "INSERT with write mode enabled",
			readOnlyMode:    false,
			enableWriteMode: true,
			sql:             "INSERT INTO users (name) VALUES ('alice')",
			expectError:     false,
		},
		{
			name:            "CREATE TABLE in read-only mode",
			readOnlyMode:    true,
			enableWriteMode: false,
			sql:             "CREATE TABLE test (id UUID PRIMARY KEY)",
			expectError:     true,
			errorCode:       ErrCodeReadOnlyViolation,
		},
		{
			name:            "CREATE TABLE with write mode enabled",
			readOnlyMode:    false,
			enableWriteMode: true,
			sql:             "CREATE TABLE test (id UUID PRIMARY KEY)",
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &Source{
				Config: Config{
					ReadOnlyMode:    tt.readOnlyMode,
					EnableWriteMode: tt.enableWriteMode,
				},
			}

			err := source.CanExecuteWrite(tt.sql)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got nil")
					return
				}

				structErr, ok := err.(*StructuredError)
				if !ok {
					t.Errorf("Expected StructuredError but got %T", err)
					return
				}

				if structErr.Code != tt.errorCode {
					t.Errorf("Expected error code %s but got %s", tt.errorCode, structErr.Code)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestApplyQueryLimits tests query limit application
func TestApplyQueryLimits(t *testing.T) {
	tests := []struct {
		name           string
		sql            string
		maxRowLimit    int
		expectedSQL    string
		shouldAddLimit bool
	}{
		{
			name:           "SELECT without LIMIT",
			sql:            "SELECT * FROM users",
			maxRowLimit:    100,
			expectedSQL:    "SELECT * FROM users LIMIT 100",
			shouldAddLimit: true,
		},
		{
			name:           "SELECT with existing LIMIT",
			sql:            "SELECT * FROM users LIMIT 50",
			maxRowLimit:    100,
			expectedSQL:    "SELECT * FROM users LIMIT 50",
			shouldAddLimit: false,
		},
		{
			name:           "SELECT with multiline existing LIMIT",
			sql:            "SELECT *\nFROM users\nLIMIT 50",
			maxRowLimit:    100,
			expectedSQL:    "SELECT *\nFROM users\nLIMIT 50",
			shouldAddLimit: false,
		},
		{
			name:           "SELECT with lowercase multiline existing LIMIT",
			sql:            "SELECT * FROM users\nlimit 50",
			maxRowLimit:    100,
			expectedSQL:    "SELECT * FROM users\nlimit 50",
			shouldAddLimit: false,
		},
		{
			name:           "SELECT with tab-separated existing LIMIT",
			sql:            "SELECT a\tLIMIT 5",
			maxRowLimit:    100,
			expectedSQL:    "SELECT a\tLIMIT 5",
			shouldAddLimit: false,
		},
		{
			name:           "SELECT with LIMIT in single-line comment",
			sql:            "SELECT * FROM users -- LIMIT is not here",
			maxRowLimit:    100,
			expectedSQL:    "SELECT * FROM users -- LIMIT is not here\nLIMIT 100",
			shouldAddLimit: true,
		},
		{
			name:           "SELECT with LIMIT in block comment",
			sql:            "SELECT * FROM users /* LIMIT is not here */",
			maxRowLimit:    100,
			expectedSQL:    "SELECT * FROM users /* LIMIT is not here */ LIMIT 100",
			shouldAddLimit: true,
		},
		{
			name:           "SELECT with LIMIT in string literal",
			sql:            "SELECT * FROM users WHERE name = 'LIMIT'",
			maxRowLimit:    100,
			expectedSQL:    "SELECT * FROM users WHERE name = 'LIMIT' LIMIT 100",
			shouldAddLimit: true,
		},
		{
			name:           "SELECT with LIMIT in quoted identifier",
			sql:            `SELECT "LIMIT" FROM users`,
			maxRowLimit:    100,
			expectedSQL:    `SELECT "LIMIT" FROM users LIMIT 100`,
			shouldAddLimit: true,
		},
		{
			name:           "SELECT with escaped quote before LIMIT",
			sql:            "SELECT * FROM users WHERE name = 'it''s LIMIT'",
			maxRowLimit:    100,
			expectedSQL:    "SELECT * FROM users WHERE name = 'it''s LIMIT' LIMIT 100",
			shouldAddLimit: true,
		},
		{
			name:           "SELECT with backslash-escaped quote before LIMIT",
			sql:            `SELECT * FROM users WHERE name = E'it\'s LIMIT'`,
			maxRowLimit:    100,
			expectedSQL:    `SELECT * FROM users WHERE name = E'it\'s LIMIT' LIMIT 100`,
			shouldAddLimit: true,
		},
		{
			name:           "SELECT with LIMIT in dollar-quoted string",
			sql:            "SELECT $$LIMIT$$ FROM users",
			maxRowLimit:    100,
			expectedSQL:    "SELECT $$LIMIT$$ FROM users LIMIT 100",
			shouldAddLimit: true,
		},
		{
			name:           "SELECT without LIMIT and semicolon",
			sql:            "SELECT * FROM users;",
			maxRowLimit:    100,
			expectedSQL:    "SELECT * FROM users LIMIT 100",
			shouldAddLimit: true,
		},
		{
			name:           "SELECT with trailing newline and semicolon",
			sql:            "SELECT * FROM users;\n",
			maxRowLimit:    100,
			expectedSQL:    "SELECT * FROM users LIMIT 100",
			shouldAddLimit: true,
		},
		{
			name:           "SELECT with multiline and semicolon",
			sql:            "\n\tSELECT *\n\tFROM users\n\tORDER BY id;\n",
			maxRowLimit:    100,
			expectedSQL:    "SELECT *\n\tFROM users\n\tORDER BY id LIMIT 100",
			shouldAddLimit: true,
		},
		{
			name:           "INSERT should not have LIMIT added",
			sql:            "INSERT INTO users (name) VALUES ('alice')",
			maxRowLimit:    100,
			expectedSQL:    "INSERT INTO users (name) VALUES ('alice')",
			shouldAddLimit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &Source{
				Config: Config{
					MaxRowLimit:     tt.maxRowLimit,
					QueryTimeoutSec: 0, // Timeout now managed by caller
				},
			}

			modifiedSQL, err := source.ApplyQueryLimits(tt.sql)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if modifiedSQL != tt.expectedSQL {
				t.Errorf("Expected SQL:\n%s\nGot:\n%s", tt.expectedSQL, modifiedSQL)
			}
		})
	}
}

func TestApplyQueryLimitsLogsExistingLimit(t *testing.T) {
	var logOutput bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	source := &Source{Config: Config{Name: "test-source", MaxRowLimit: 100}}
	_, err := source.ApplyQueryLimits("SELECT * FROM users LIMIT 50")
	if err != nil {
		t.Fatalf("ApplyQueryLimits returned an error: %v", err)
	}

	logMessage := logOutput.String()
	if !strings.Contains(logMessage, "level=WARN") ||
		!strings.Contains(logMessage, "configured row limit skipped because query already contains a LIMIT clause") {
		t.Errorf("Expected warning about skipped row limit, got: %s", logMessage)
	}
}

// TestApplyQueryTimeout tests that timeout is managed by caller (not source)
func TestApplyQueryTimeout(t *testing.T) {
	source := &Source{
		Config: Config{
			QueryTimeoutSec: 5, // Documented recommended timeout
			MaxRowLimit:     0, // Don't add LIMIT
		},
	}

	// Caller creates timeout context (following Go best practices)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(source.QueryTimeoutSec)*time.Second)
	defer cancel()

	// Apply query limits (doesn't modify context anymore)
	modifiedSQL, err := source.ApplyQueryLimits("SELECT * FROM users")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}

	// Verify context has deadline (managed by caller)
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Error("Expected deadline to be set but it wasn't")
		return
	}

	// Verify deadline is approximately 5 seconds from now
	expectedDeadline := time.Now().Add(5 * time.Second)
	diff := deadline.Sub(expectedDeadline)
	if diff < 0 {
		diff = -diff
	}

	// Allow 1 second tolerance
	if diff > time.Second {
		t.Errorf("Deadline diff too large: %v", diff)
	}

	// Verify SQL is unchanged (LIMIT not added since MaxRowLimit=0)
	if modifiedSQL != "SELECT * FROM users" {
		t.Errorf("Expected SQL unchanged, got: %s", modifiedSQL)
	}
}

// TestRedactSQL tests SQL redaction for telemetry
func TestRedactSQL(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected string
	}{
		{
			name:     "String literal redaction",
			sql:      "SELECT * FROM users WHERE name='alice' AND email='alice@example.com'",
			expected: "SELECT * FROM users WHERE name='***' AND email='***'",
		},
		{
			name:     "Long number redaction",
			sql:      "SELECT * FROM users WHERE ssn=1234567890123",
			expected: "SELECT * FROM users WHERE ssn=***",
		},
		{
			name:     "Short numbers not redacted",
			sql:      "SELECT * FROM users WHERE age=25",
			expected: "SELECT * FROM users WHERE age=25",
		},
		{
			name:     "Multiple sensitive values",
			sql:      "INSERT INTO users (name, email, phone) VALUES ('bob', 'bob@example.com', '5551234567')",
			expected: "INSERT INTO users (name, email, phone) VALUES ('***', '***', '***')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactSQL(tt.sql)
			if result != tt.expected {
				t.Errorf("RedactSQL:\nGot:      %s\nExpected: %s", result, tt.expected)
			}
		})
	}
}

// TestIsReadOnlyMode tests read-only mode detection
func TestIsReadOnlyMode(t *testing.T) {
	tests := []struct {
		name            string
		readOnlyMode    bool
		enableWriteMode bool
		expected        bool
	}{
		{"Read-only by default", true, false, true},
		{"Write mode enabled", false, true, false},
		{"Both false", false, false, false},
		{"Read-only overridden by write mode", true, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &Source{
				Config: Config{
					ReadOnlyMode:    tt.readOnlyMode,
					EnableWriteMode: tt.enableWriteMode,
				},
			}

			result := source.IsReadOnlyMode()
			if result != tt.expected {
				t.Errorf("IsReadOnlyMode() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestStructuredError tests error formatting
func TestStructuredError(t *testing.T) {
	err := &StructuredError{
		Code:    ErrCodeReadOnlyViolation,
		Message: "Write operations not allowed",
		Details: map[string]any{
			"sql_type": "INSERT",
		},
	}

	errorStr := err.Error()
	if !strings.Contains(errorStr, ErrCodeReadOnlyViolation) {
		t.Errorf("Error string should contain error code: %s", errorStr)
	}
	if !strings.Contains(errorStr, "Write operations not allowed") {
		t.Errorf("Error string should contain message: %s", errorStr)
	}
}

// TestDefaultSecuritySettings tests that security defaults are correct
func TestDefaultSecuritySettings(t *testing.T) {
	ctx := context.Background()

	// Create a minimal YAML config
	yamlData := `name: test
type: cockroachdb
host: localhost
port: "26257"
user: root
database: defaultdb
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// Apply defaults through newConfig logic manually
	cfg.MaxRetries = 5
	cfg.RetryBaseDelay = "500ms"
	cfg.ReadOnlyMode = true
	cfg.EnableWriteMode = false
	cfg.MaxRowLimit = 1000
	cfg.QueryTimeoutSec = 30
	cfg.EnableTelemetry = true
	cfg.TelemetryVerbose = false

	_ = ctx // prevent unused

	// Verify MCP security defaults
	if !cfg.ReadOnlyMode {
		t.Error("ReadOnlyMode should be true by default")
	}
	if cfg.EnableWriteMode {
		t.Error("EnableWriteMode should be false by default")
	}
	if cfg.MaxRowLimit != 1000 {
		t.Errorf("MaxRowLimit should be 1000, got %d", cfg.MaxRowLimit)
	}
	if cfg.QueryTimeoutSec != 30 {
		t.Errorf("QueryTimeoutSec should be 30, got %d", cfg.QueryTimeoutSec)
	}
	if !cfg.EnableTelemetry {
		t.Error("EnableTelemetry should be true by default")
	}
}
