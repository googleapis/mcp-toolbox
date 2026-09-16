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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	crdbpgx "github.com/cockroachdb/cockroach-go/v2/crdb/crdbpgxv5"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "cockroachdb"

var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	// MCP compliance: Read-only by default, require explicit opt-in for writes
	actual := Config{
		Name:             name,
		MaxRetries:       5,
		RetryBaseDelay:   "500ms",
		ReadOnlyMode:     true,  // MCP requirement: read-only by default
		EnableWriteMode:  false, // Must be explicitly enabled
		MaxRowLimit:      1000,  // MCP requirement: limit query results
		QueryTimeoutSec:  30,    // MCP requirement: prevent long-running queries
		EnableTelemetry:  true,  // MCP requirement: observability
		TelemetryVerbose: false,
	}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}

	// Security validation: If EnableWriteMode is true, ReadOnlyMode should be false
	if actual.EnableWriteMode {
		actual.ReadOnlyMode = false
	}

	return actual, nil
}

type Config struct {
	Name           string            `yaml:"name" validate:"required"`
	Type           string            `yaml:"type" validate:"required"`
	Host           string            `yaml:"host" validate:"required"`
	Port           string            `yaml:"port" validate:"required"`
	User           string            `yaml:"user" validate:"required"`
	Password       string            `yaml:"password"`
	Database       string            `yaml:"database" validate:"required"`
	QueryParams    map[string]string `yaml:"queryParams"`
	MaxRetries     int               `yaml:"maxRetries"`
	RetryBaseDelay string            `yaml:"retryBaseDelay"`

	// MCP Security Features
	ReadOnlyMode    bool `yaml:"readOnlyMode"`    // Default: true (enforced in Initialize)
	EnableWriteMode bool `yaml:"enableWriteMode"` // Explicit opt-in for write operations
	MaxRowLimit     int  `yaml:"maxRowLimit"`     // Default: 1000
	QueryTimeoutSec int  `yaml:"queryTimeoutSec"` // Default: 30

	// Observability
	EnableTelemetry  bool   `yaml:"enableTelemetry"`  // Default: true
	TelemetryVerbose bool   `yaml:"telemetryVerbose"` // Default: false
	ClusterID        string `yaml:"clusterID"`        // Optional cluster identifier for telemetry
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	retryBaseDelay, err := time.ParseDuration(r.RetryBaseDelay)
	if err != nil {
		return nil, fmt.Errorf("invalid retryBaseDelay: %w", err)
	}

	pool, err := initCockroachDBConnectionPoolWithRetry(ctx, tracer, r.Name, r.Host, r.Port, r.User, r.Password, r.Database, r.QueryParams, r.MaxRetries, retryBaseDelay)
	if err != nil {
		return nil, fmt.Errorf("unable to create pool: %w", err)
	}

	s := &Source{
		Config: r,
		Pool:   pool,
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	Pool *pgxpool.Pool
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) CockroachDBPool() *pgxpool.Pool {
	return s.Pool
}

func (s *Source) PostgresPool() *pgxpool.Pool {
	return s.Pool
}

// ExecuteTxWithRetry executes a function within a transaction with automatic retry logic
// using the official CockroachDB retry mechanism from cockroach-go/v2
func (s *Source) ExecuteTxWithRetry(ctx context.Context, fn func(pgx.Tx) error) error {
	return crdbpgx.ExecuteTx(ctx, s.Pool, pgx.TxOptions{}, fn)
}

// Query executes a query using the connection pool with MCP security enforcement.
// For read-only queries, connection-level retry is sufficient.
// For write operations requiring transaction retry, use ExecuteTxWithRetry directly.
// Note: Callers should manage context timeouts as needed.
func (s *Source) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	// MCP Security Check 1: Enforce write operation restrictions
	if err := s.CanExecuteWrite(sql); err != nil {
		return nil, err
	}

	// MCP Security Check 2: Apply query limits (row limit)
	modifiedSQL, err := s.ApplyQueryLimits(sql)
	if err != nil {
		return nil, err
	}

	return s.Pool.Query(ctx, modifiedSQL, args...)
}

// ============================================================================
// MCP Security & Observability Features
// ============================================================================

// TelemetryEvent represents a structured telemetry event for MCP tool calls
type TelemetryEvent struct {
	Timestamp    time.Time         `json:"timestamp"`
	ToolName     string            `json:"tool_name"`
	ClusterID    string            `json:"cluster_id"`
	Database     string            `json:"database"`
	User         string            `json:"user"`
	SQLRedacted  string            `json:"sql_redacted"` // Query with values redacted
	Status       string            `json:"status"`       // "success" | "failure"
	ErrorCode    string            `json:"error_code,omitempty"`
	ErrorMsg     string            `json:"error_msg,omitempty"`
	LatencyMs    int64             `json:"latency_ms"`
	RowsAffected int64             `json:"rows_affected,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// StructuredError represents an MCP-compliant error with error codes
type StructuredError struct {
	Code    string         `json:"error_code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *StructuredError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// MCP Error Codes
const (
	ErrCodeUnauthorized         = "CRDB_UNAUTHORIZED"
	ErrCodeReadOnlyViolation    = "CRDB_READONLY_VIOLATION"
	ErrCodeQueryTimeout         = "CRDB_QUERY_TIMEOUT"
	ErrCodeRowLimitExceeded     = "CRDB_ROW_LIMIT_EXCEEDED"
	ErrCodeInvalidSQL           = "CRDB_INVALID_SQL"
	ErrCodeConnectionFailed     = "CRDB_CONNECTION_FAILED"
	ErrCodeWriteModeRequired    = "CRDB_WRITE_MODE_REQUIRED"
	ErrCodeQueryExecutionFailed = "CRDB_QUERY_EXECUTION_FAILED"
)

// SQLStatementType represents the type of SQL statement
type SQLStatementType int

const (
	SQLTypeUnknown SQLStatementType = iota
	SQLTypeSelect
	SQLTypeInsert
	SQLTypeUpdate
	SQLTypeDelete
	SQLTypeDDL // CREATE, ALTER, DROP
	SQLTypeTruncate
	SQLTypeExplain
	SQLTypeShow
	SQLTypeSet
)

// ClassifySQL analyzes a SQL statement and returns its type
func ClassifySQL(sql string) SQLStatementType {
	// Normalize: trim and convert to uppercase for analysis
	normalized := strings.TrimSpace(strings.ToUpper(sql))

	if normalized == "" {
		return SQLTypeUnknown
	}

	// Remove comments
	normalized = regexp.MustCompile(`--.*`).ReplaceAllString(normalized, "")
	normalized = regexp.MustCompile(`/\*.*?\*/`).ReplaceAllString(normalized, "")
	normalized = strings.TrimSpace(normalized)

	// Check statement type
	switch {
	case strings.HasPrefix(normalized, "SELECT"):
		return SQLTypeSelect
	case strings.HasPrefix(normalized, "INSERT"):
		return SQLTypeInsert
	case strings.HasPrefix(normalized, "UPDATE"):
		return SQLTypeUpdate
	case strings.HasPrefix(normalized, "DELETE"):
		return SQLTypeDelete
	case strings.HasPrefix(normalized, "TRUNCATE"):
		return SQLTypeTruncate
	case strings.HasPrefix(normalized, "CREATE"):
		return SQLTypeDDL
	case strings.HasPrefix(normalized, "ALTER"):
		return SQLTypeDDL
	case strings.HasPrefix(normalized, "DROP"):
		return SQLTypeDDL
	case strings.HasPrefix(normalized, "EXPLAIN"):
		return SQLTypeExplain
	case strings.HasPrefix(normalized, "SHOW"):
		return SQLTypeShow
	case strings.HasPrefix(normalized, "SET"):
		return SQLTypeSet
	default:
		return SQLTypeUnknown
	}
}

// IsWriteOperation returns true if the SQL statement modifies data
func IsWriteOperation(sqlType SQLStatementType) bool {
	switch sqlType {
	case SQLTypeInsert, SQLTypeUpdate, SQLTypeDelete, SQLTypeTruncate, SQLTypeDDL:
		return true
	default:
		return false
	}
}

// IsReadOnlyMode returns whether the source is in read-only mode
func (s *Source) IsReadOnlyMode() bool {
	return s.ReadOnlyMode && !s.EnableWriteMode
}

// CanExecuteWrite checks if a write operation is allowed
func (s *Source) CanExecuteWrite(sql string) error {
	sqlType := ClassifySQL(sql)

	if IsWriteOperation(sqlType) && s.IsReadOnlyMode() {
		return &StructuredError{
			Code:    ErrCodeReadOnlyViolation,
			Message: "Write operations are not allowed in read-only mode. Set enableWriteMode: true to allow writes.",
			Details: map[string]any{
				"sql_type":          sqlType,
				"read_only_mode":    s.ReadOnlyMode,
				"enable_write_mode": s.EnableWriteMode,
			},
		}
	}

	return nil
}

// limitClauseRegexp matches a LIMIT keyword regardless of surrounding
// whitespace, so multiline or tab-formatted queries are detected too.
var limitClauseRegexp = regexp.MustCompile(`(?i)\bLIMIT\b`)

func stripSQLCommentsAndQuotedText(sql string) (string, bool) {
	var result strings.Builder
	result.Grow(len(sql))
	trailingLineComment := false

	for i := 0; i < len(sql); {
		switch {
		case sql[i] == '-' && i+1 < len(sql) && sql[i+1] == '-':
			trailingLineComment = true
			for i < len(sql) && sql[i] != '\n' {
				result.WriteByte(' ')
				i++
			}
		case sql[i] == '/' && i+1 < len(sql) && sql[i+1] == '*':
			depth := 1
			result.WriteString("  ")
			i += 2
			for i < len(sql) && depth > 0 {
				switch {
				case sql[i] == '/' && i+1 < len(sql) && sql[i+1] == '*':
					depth++
					result.WriteString("  ")
					i += 2
				case sql[i] == '*' && i+1 < len(sql) && sql[i+1] == '/':
					depth--
					result.WriteString("  ")
					i += 2
				default:
					result.WriteByte(' ')
					i++
				}
			}
		case sql[i] == '\'' || sql[i] == '"':
			quote := sql[i]
			result.WriteByte(' ')
			i++
			for i < len(sql) {
				result.WriteByte(' ')
				if sql[i] == '\\' && i+1 < len(sql) {
					result.WriteByte(' ')
					i += 2
					continue
				}
				if sql[i] == quote {
					if i+1 < len(sql) && sql[i+1] == quote {
						result.WriteByte(' ')
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case sql[i] == '$':
			end := i + 1
			for end < len(sql) && (sql[end] == '_' || sql[end] >= 'a' && sql[end] <= 'z' || sql[end] >= 'A' && sql[end] <= 'Z' || end > i+1 && sql[end] >= '0' && sql[end] <= '9') {
				end++
			}
			if end >= len(sql) || sql[end] != '$' {
				result.WriteByte(sql[i])
				i++
				continue
			}
			delimiter := sql[i : end+1]
			closing := strings.Index(sql[end+1:], delimiter)
			if closing < 0 {
				result.WriteByte(sql[i])
				i++
				continue
			}
			quotedLength := end + 1 + closing + len(delimiter) - i
			result.WriteString(strings.Repeat(" ", quotedLength))
			i += quotedLength
		default:
			result.WriteByte(sql[i])
			if sql[i] == '\n' {
				trailingLineComment = false
			}
			i++
		}
	}

	return result.String(), trailingLineComment
}

// ApplyQueryLimits applies row limits to a SQL query for MCP security compliance.
// Context timeout management is the responsibility of the caller (following Go best practices).
// Returns potentially modified SQL with LIMIT clause for SELECT queries.
func (s *Source) ApplyQueryLimits(sql string) (string, error) {
	sqlType := ClassifySQL(sql)

	// Apply row limit only to SELECT queries
	if sqlType == SQLTypeSelect && s.MaxRowLimit > 0 {
		// Check if query already has LIMIT clause
		searchableSQL, trailingLineComment := stripSQLCommentsAndQuotedText(sql)
		if limitClauseRegexp.MatchString(searchableSQL) {
			slog.Warn("configured row limit skipped because query already contains a LIMIT clause", "source", s.Name, "maxRowLimit", s.MaxRowLimit)
		} else {
			// Add LIMIT clause - trim trailing whitespace and semicolon
			trimmedSearchable := strings.TrimRight(searchableSQL, " \t\r\n")
			if strings.HasSuffix(trimmedSearchable, ";") {
				semiColonIdx := len(trimmedSearchable) - 1
				sql = sql[:semiColonIdx] + sql[semiColonIdx+1:]
			}
			sql = strings.TrimSpace(sql)
			separator := " "
			if trailingLineComment {
				separator = "\n"
			}
			sql = fmt.Sprintf("%s%sLIMIT %d", sql, separator, s.MaxRowLimit)
		}
	}

	return sql, nil
}

// RedactSQL redacts sensitive values from SQL for telemetry
func RedactSQL(sql string) string {
	// Redact string literals
	sql = regexp.MustCompile(`'[^']*'`).ReplaceAllString(sql, "'***'")

	// Redact numbers that might be sensitive
	sql = regexp.MustCompile(`\b\d{10,}\b`).ReplaceAllString(sql, "***")

	return sql
}

// EmitTelemetry logs a telemetry event in structured JSON format
func (s *Source) EmitTelemetry(ctx context.Context, event TelemetryEvent) {
	if !s.EnableTelemetry {
		return
	}

	// Set cluster ID if not already set
	if event.ClusterID == "" {
		event.ClusterID = s.ClusterID
		if event.ClusterID == "" {
			event.ClusterID = s.Database // Fallback to database name
		}
	}

	// Set database and user
	if event.Database == "" {
		event.Database = s.Database
	}
	if event.User == "" {
		event.User = s.User
	}

	// Log as structured JSON
	if s.TelemetryVerbose {
		jsonBytes, _ := json.Marshal(event)
		slog.Info("CockroachDB MCP Telemetry", "event", string(jsonBytes))
	} else {
		// Minimal logging
		slog.Info("CockroachDB MCP",
			"tool", event.ToolName,
			"status", event.Status,
			"latency_ms", event.LatencyMs,
			"error_code", event.ErrorCode,
		)
	}
}

func initCockroachDBConnectionPoolWithRetry(ctx context.Context, tracer trace.Tracer, name, host, port, user, pass, dbname string, queryParams map[string]string, maxRetries int, baseDelay time.Duration) (*pgxpool.Pool, error) {
	//nolint:all
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()

	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		userAgent = "genai-toolbox"
	}
	if queryParams == nil {
		queryParams = make(map[string]string)
	}
	if _, ok := queryParams["application_name"]; !ok {
		queryParams["application_name"] = userAgent
	}

	connURL := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, pass),
		Host:     fmt.Sprintf("%s:%s", host, port),
		Path:     dbname,
		RawQuery: ConvertParamMapToRawQuery(queryParams),
	}

	var pool *pgxpool.Pool
	for attempt := 0; attempt <= maxRetries; attempt++ {
		pool, err = pgxpool.New(ctx, connURL.String())
		if err == nil {
			err = pool.Ping(ctx)
		}

		if err == nil {
			return pool, nil
		}

		if attempt < maxRetries {
			backoff := baseDelay * time.Duration(math.Pow(2, float64(attempt)))
			time.Sleep(backoff)
		}
	}

	return nil, fmt.Errorf("failed to connect to CockroachDB after %d retries: %w", maxRetries, err)
}

func ConvertParamMapToRawQuery(queryParams map[string]string) string {
	values := url.Values{}
	for k, v := range queryParams {
		values.Add(k, v)
	}
	return values.Encode()
}
