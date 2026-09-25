// Copyright © 2025, Oracle and/or its affiliates.
package oracle

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	_ "github.com/godror/godror"   // OCI driver
	_ "github.com/sijms/go-ora/v2" // Pure Go driver

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "oracle"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}

	// Validate that we have one of: tnsAlias, connectionString, or host+service_name
	if err := actual.validate(); err != nil {
		return nil, fmt.Errorf("invalid Oracle configuration: %w", err)
	}

	return actual, nil
}

type Config struct {
	Name                string `yaml:"name" validate:"required"`
	Type                string `yaml:"type" validate:"required"`
	ConnectionString    string `yaml:"connectionString,omitempty"`
	TnsAlias            string `yaml:"tnsAlias,omitempty"`
	TnsAdmin            string `yaml:"tnsAdmin,omitempty"`
	Host                string `yaml:"host,omitempty"`
	Port                int    `yaml:"port,omitempty"`
	ServiceName         string `yaml:"serviceName,omitempty"`
	User                string `yaml:"user" validate:"required"`
	Password            string `yaml:"password" validate:"required"`
	UseOCI              bool   `yaml:"useOCI,omitempty"`
	WalletLocation      string `yaml:"walletLocation,omitempty"`
	DisablePooling      bool   `yaml:"disablePooling,omitempty"`
	SessionContextBlock string `yaml:"sessionContextBlock,omitempty"`
	SessionContextClaim string `yaml:"sessionContextClaim,omitempty"`
	SessionResetBlock   string `yaml:"sessionResetBlock,omitempty"`
}

func (c Config) validate() error {
	hasTnsAdmin := strings.TrimSpace(c.TnsAdmin) != ""
	hasTnsAlias := strings.TrimSpace(c.TnsAlias) != ""
	hasConnStr := strings.TrimSpace(c.ConnectionString) != ""
	hasHostService := strings.TrimSpace(c.Host) != "" && strings.TrimSpace(c.ServiceName) != ""
	hasWallet := strings.TrimSpace(c.WalletLocation) != ""

	connectionMethods := 0
	if hasTnsAlias {
		connectionMethods++
	}
	if hasConnStr {
		connectionMethods++
	}
	if hasHostService {
		connectionMethods++
	}

	if connectionMethods == 0 {
		return fmt.Errorf("must provide one of: 'tns_alias', 'connection_string', or both 'host' and 'service_name'")
	}

	if connectionMethods > 1 {
		return fmt.Errorf("provide only one connection method: 'tns_alias', 'connection_string', or 'host'+'service_name'")
	}

	if hasTnsAdmin && !c.UseOCI {
		return fmt.Errorf("`tnsAdmin` can only be used when `UseOCI` is true, or use `walletLocation` instead")
	}

	if hasWallet && c.UseOCI {
		return fmt.Errorf("when using an OCI driver, use `tnsAdmin` to specify credentials file location instead")
	}

	if strings.TrimSpace(c.SessionResetBlock) != "" && strings.TrimSpace(c.SessionContextBlock) == "" {
		return fmt.Errorf("`sessionResetBlock` requires `sessionContextBlock` to be configured")
	}

	if strings.TrimSpace(c.SessionContextClaim) != "" && strings.TrimSpace(c.SessionContextBlock) == "" {
		return fmt.Errorf("`sessionContextClaim` requires `sessionContextBlock` to be configured")
	}

	return nil
}

func (r Config) SourceConfigType() string {
	return SourceType
}

var pingDB = func(ctx context.Context, db *sql.DB) error {
	return db.PingContext(ctx)
}

var setPoolLimits = func(db *sql.DB, maxIdleConns int, connMaxLifetime time.Duration) {
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	r.SessionContextBlock = strings.TrimSpace(r.SessionContextBlock)
	r.SessionContextClaim = strings.TrimSpace(r.SessionContextClaim)
	r.SessionResetBlock = strings.TrimSpace(r.SessionResetBlock)
	if r.SessionContextBlock != "" && r.SessionContextClaim == "" {
		r.SessionContextClaim = "email"
	}

	db, err := initOracleConnection(ctx, tracer, r)
	if err != nil {
		return nil, fmt.Errorf("unable to create Oracle connection: %w", err)
	}

	err = pingDB(ctx, db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("unable to connect to Oracle successfully: %w", err)
	}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s := &Source{
		Config: r,
		conn:   sources.NewConnectOnce[*sql.DB](ctx, r.Name, SourceType, tracer),
	}
	if deferConnect {
		return s, nil
	}
	if _, err := s.pool(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[*sql.DB]
}

func (s *Source) pool(ctx context.Context) (*sql.DB, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*sql.DB, error) {
		r := s.Config
		db, err := initOracleConnection(ctx, r)
		if err != nil {
			return nil, fmt.Errorf("unable to create Oracle connection: %w", err)
		}

		if err := db.PingContext(ctx); err != nil {
			db.Close()
			return nil, fmt.Errorf("unable to connect to Oracle successfully: %w", err)
		}
		return db, nil
	})
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

func (s *Source) OracleDB() *sql.DB {
	return s.DB
}

type contextKey string

const (
	toolParamsKey contextKey = "oracleToolParams"
	authClaimsKey contextKey = "oracleAuthClaims"
)

// WithToolParams adds tool invocation parameters into the context as a value.
func WithToolParams(ctx context.Context, params map[string]any) context.Context {
	return context.WithValue(ctx, toolParamsKey, params)
}

// ToolParamsFromContext retrieves tool invocation parameters from context.
func ToolParamsFromContext(ctx context.Context) map[string]any {
	if params, ok := ctx.Value(toolParamsKey).(map[string]any); ok {
		return params
	}
	return nil
}

// WithAuthClaims adds auth claims into the context as a value.
func WithAuthClaims(ctx context.Context, claims map[string]any) context.Context {
	return context.WithValue(ctx, authClaimsKey, claims)
}

// AuthClaimsFromContext retrieves auth claims from context.
func AuthClaimsFromContext(ctx context.Context) map[string]any {
	if claims, ok := ctx.Value(authClaimsKey).(map[string]any); ok {
		return claims
	}
	return nil
}

// ParseJWTClaims decodes the payload portion of a JWT token into a map of claims.
// It is used to read claims from tokens that were already verified by the auth service.
func ParseJWTClaims(token string) map[string]any {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payloadBytes, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil
	}
	return claims
}

func (s *Source) extractUserIdentity(ctx context.Context) (string, error) {
	if strings.TrimSpace(s.SessionContextBlock) == "" {
		return "", nil
	}

	claims := util.AuthTokenClaimsFromContext(ctx)
	if len(claims) == 0 {
		claims = AuthClaimsFromContext(ctx)
	}
	if len(claims) == 0 {
		return "", nil
	}

	claimKey := strings.TrimSpace(s.SessionContextClaim)
	if claimKey == "" {
		claimKey = "email"
	}

	candidateKeys := []string{claimKey}
	if claimKey == "email" {
		candidateKeys = append(candidateKeys, "sub")
	}

	for _, key := range candidateKeys {
		val, ok := claims[key]
		if !ok || val == nil {
			continue
		}
		var str string
		switch v := val.(type) {
		case string:
			str = v
		case int:
			str = fmt.Sprintf("%d", v)
		case int8:
			str = fmt.Sprintf("%d", v)
		case int16:
			str = fmt.Sprintf("%d", v)
		case int32:
			str = fmt.Sprintf("%d", v)
		case int64:
			str = fmt.Sprintf("%d", v)
		case uint:
			str = fmt.Sprintf("%d", v)
		case uint8:
			str = fmt.Sprintf("%d", v)
		case uint16:
			str = fmt.Sprintf("%d", v)
		case uint32:
			str = fmt.Sprintf("%d", v)
		case uint64:
			str = fmt.Sprintf("%d", v)
		case float32:
			f := float64(v)
			if math.IsNaN(f) || math.IsInf(f, 0) {
				continue
			}
			str = strconv.FormatFloat(f, 'f', -1, 32)
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			str = strconv.FormatFloat(v, 'f', -1, 64)
		case json.Number:
			str = v.String()
		default:
			continue
		}
		trimmed := strings.TrimSpace(str)
		if trimmed != "" {
			return trimmed, nil
		}
	}

	return "", nil
}

var oracleNamedBindRegex = regexp.MustCompile(`:([a-zA-Z0-9_]+)`)

// extractBindParameterNames extracts distinct bind variable names from SQL/PL-SQL text,
// stripping comments, string literals, and excluding PL/SQL assignment operator :=.
func extractBindParameterNames(text string) []string {
	var buf strings.Builder
	length := len(text)
	inSingleQuote := false
	inDoubleQuote := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < length; i++ {
		ch := text[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				buf.WriteByte('\n')
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && i+1 < length && text[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inSingleQuote {
			if ch == '\'' {
				if i+1 < length && text[i+1] == '\'' {
					i++
				} else {
					inSingleQuote = false
				}
			}
			continue
		}
		if inDoubleQuote {
			if ch == '"' {
				inDoubleQuote = false
			}
			continue
		}

		if ch == '-' && i+1 < length && text[i+1] == '-' {
			inLineComment = true
			i++
			continue
		}
		if ch == '/' && i+1 < length && text[i+1] == '*' {
			inBlockComment = true
			i++
			continue
		}
		if ch == '\'' {
			inSingleQuote = true
			continue
		}
		if ch == '"' {
			inDoubleQuote = true
			continue
		}

		buf.WriteByte(ch)
	}

	matches := oracleNamedBindRegex.FindAllStringSubmatch(buf.String(), -1)
	var names []string
	seen := make(map[string]bool)
	for _, m := range matches {
		if len(m) > 1 {
			lower := strings.ToLower(m[1])
			if !seen[lower] {
				seen[lower] = true
				names = append(names, m[1])
			}
		}
	}
	return names
}

// buildSessionBlockBinds constructs named bind arguments for a session initialization or reset block.
// It maps parameters from tool invocation context, caller identity claims, and safe fallbacks.
func (s *Source) buildSessionBlockBinds(ctx context.Context, block string, userIdentity string) []any {
	names := extractBindParameterNames(block)
	if len(names) == 0 {
		return nil
	}

	toolParams := ToolParamsFromContext(ctx)
	toolParamsLower := make(map[string]any, len(toolParams))
	for k, v := range toolParams {
		toolParamsLower[strings.ToLower(k)] = v
	}

	// If JWT claims did not yield an identity, check tool parameters for identity fallbacks
	resolvedIdentity := userIdentity
	if resolvedIdentity == "" {
		candidates := []string{}
		if s.SessionContextClaim != "" {
			candidates = append(candidates, strings.ToLower(strings.TrimSpace(s.SessionContextClaim)))
		}
		candidates = append(candidates, "user_identity", "user_id", "username")
		for _, c := range candidates {
			if val, ok := toolParamsLower[c]; ok && val != nil {
				strVal := strings.TrimSpace(fmt.Sprintf("%v", val))
				if strVal != "" {
					resolvedIdentity = strVal
					break
				}
			}
		}
	}

	binds := make([]any, 0, len(names))
	for _, name := range names {
		lower := strings.ToLower(name)
		if val, ok := toolParamsLower[lower]; ok {
			binds = append(binds, sql.Named(name, val))
		} else if lower == "user_identity" || lower == "user_id" || (s.SessionContextClaim != "" && lower == strings.ToLower(strings.TrimSpace(s.SessionContextClaim))) {
			binds = append(binds, sql.Named(name, resolvedIdentity))
		} else {
			binds = append(binds, sql.Named(name, ""))
		}
	}
	return binds
}

type sqlRunner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any, readOnly bool) (any, error) {
	userIdentity, err := s.extractUserIdentity(ctx)
	if err != nil {
		return nil, err
	}

	sessionContextBlock := strings.TrimSpace(s.SessionContextBlock)
	if sessionContextBlock == "" {
		return executeSQL(ctx, s.OracleDB(), statement, params, readOnly)
	}

	conn, err := s.OracleDB().Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to acquire dedicated Oracle connection: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	sessionResetBlock := strings.TrimSpace(s.SessionResetBlock)
	if sessionResetBlock != "" {
		resetBinds := s.buildSessionBlockBinds(ctx, sessionResetBlock, userIdentity)
		defer func() {
			resetCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, _ = conn.ExecContext(resetCtx, sessionResetBlock, resetBinds...)
		}()
	}

	contextBinds := s.buildSessionBlockBinds(ctx, sessionContextBlock, userIdentity)
	if _, err := conn.ExecContext(ctx, sessionContextBlock, contextBinds...); err != nil {
		return nil, fmt.Errorf("failed to execute session context setup: %w", err)
	}

	return executeSQL(ctx, conn, statement, params, readOnly)
}

func executeSQL(ctx context.Context, runner sqlRunner, statement string, params []any, readOnly bool) (any, error) {
	if !readOnly {
		result, err := runner.ExecContext(ctx, statement, params...)
func (s *Source) RunSQL(ctx context.Context, statement string, params []any, readOnly bool) (any, error) {
	db, err := s.pool(ctx)
	if err != nil {
		return nil, err
	}
	if !readOnly {
		result, err := db.ExecContext(ctx, statement, params...)
		if err != nil {
			return nil, fmt.Errorf("unable to execute DML statement: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("unable to get rows affected: %w", err)
		}

		return map[string]any{
			"status":        "success",
			"rows_affected": rowsAffected,
		}, nil
	}
	rows, err := runner.QueryContext(ctx, statement, params...)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	defer rows.Close()

	// If Columns() errors, it might be a DDL/DML without an OUTPUT clause.
	// We proceed, and results.Err() will catch actual query execution errors.
	// 'out' will remain an empty slice if cols is empty or err is not nil here.
	cols, _ := rows.Columns()

	// Get Column types
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("query execution error: %w", err)
		}
		return []any{}, nil
	}

	out := []any{}
	for rows.Next() {
		values := make([]any, len(cols))
		for i, colType := range colTypes {
			switch strings.ToUpper(colType.DatabaseTypeName()) {
			case "NUMBER", "FLOAT", "BINARY_FLOAT", "BINARY_DOUBLE":
				if _, scale, ok := colType.DecimalSize(); ok && scale == 0 {
					// Scale is 0, treat it as an integer.
					values[i] = new(sql.NullInt64)
				} else {
					// Scale is non-zero or unknown, treat
					// it as a float.
					values[i] = new(sql.NullFloat64)
				}
			case "DATE", "TIMESTAMP", "TIMESTAMP WITH TIME ZONE", "TIMESTAMP WITH LOCAL TIME ZONE":
				values[i] = new(sql.NullTime)
			case "JSON":
				values[i] = new(sql.RawBytes)
			default:
				values[i] = new(sql.NullString)
			}
		}

		if err := rows.Scan(values...); err != nil {
			return nil, fmt.Errorf("unable to scan row: %w", err)
		}

		vMap := make(map[string]any)
		for i, col := range cols {
			receiver := values[i]

			switch v := receiver.(type) {
			case *sql.NullInt64:
				if v.Valid {
					vMap[col] = v.Int64
				} else {
					vMap[col] = nil
				}
			case *sql.NullFloat64:
				if v.Valid {
					vMap[col] = v.Float64
				} else {
					vMap[col] = nil
				}
			case *sql.NullString:
				if v.Valid {
					vMap[col] = v.String
				} else {
					vMap[col] = nil
				}
			case *sql.NullTime:
				if v.Valid {
					vMap[col] = v.Time
				} else {
					vMap[col] = nil
				}
			case *sql.RawBytes:
				if *v != nil {
					var unmarshaledData any
					if err := json.Unmarshal(*v, &unmarshaledData); err != nil {
						return nil, fmt.Errorf("unable to unmarshal json data for column %s", col)
					}
					vMap[col] = unmarshaledData
				} else {
					vMap[col] = nil
				}
			default:
				return nil, fmt.Errorf("unexpected receiver type: %T", v)
			}
		}
		out = append(out, vMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("errors encountered during query execution or row processing: %w", err)
	}

	return out, nil
}

func buildGoOraConnString(user, password, connectStringBase, walletLocation string) string {
	userInfo := url.UserPassword(
		decodePercentEncodedUserInfo(user),
		decodePercentEncodedUserInfo(password),
	).String()

	base := fmt.Sprintf("oracle://%s@%s", userInfo, connectStringBase)
	trimmedWalletLocation := strings.TrimSpace(walletLocation)
	if trimmedWalletLocation == "" {
		return base
	}

	q := url.Values{}
	q.Set("ssl", "true")
	q.Set("wallet", trimmedWalletLocation)

	separator := "?"
	if strings.Contains(connectStringBase, "?") {
		separator = "&"
		if strings.HasSuffix(base, "?") || strings.HasSuffix(base, "&") {
			separator = ""
		}
	}

	return fmt.Sprintf("%s%s%s", base, separator, q.Encode())
}

func decodePercentEncodedUserInfo(value string) string {
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

func initOracleConnection(ctx context.Context, config Config) (*sql.DB, error) {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		panic(err)
	}

	hasWallet := strings.TrimSpace(config.WalletLocation) != ""

	if config.TnsAdmin != "" {
		originalTnsAdmin := os.Getenv("TNS_ADMIN")
		os.Setenv("TNS_ADMIN", config.TnsAdmin)
		logger.DebugContext(ctx, fmt.Sprintf("Setting TNS_ADMIN to: %s\n", config.TnsAdmin))
		// Restore original TNS_ADMIN after connection
		defer func() {
			if originalTnsAdmin != "" {
				os.Setenv("TNS_ADMIN", originalTnsAdmin)
			} else {
				os.Unsetenv("TNS_ADMIN")
			}
		}()
	}

	var connectStringBase string
	if config.TnsAlias != "" {
		connectStringBase = strings.TrimSpace(config.TnsAlias)
	} else if config.ConnectionString != "" {
		connectStringBase = strings.TrimSpace(config.ConnectionString)
	} else {
		if config.Port > 0 {
			connectStringBase = fmt.Sprintf("%s:%d/%s", config.Host, config.Port, config.ServiceName)
		} else {
			connectStringBase = fmt.Sprintf("%s/%s", config.Host, config.ServiceName)
		}
	}

	var driverName string
	var finalConnStr string

	if config.UseOCI {
		// Use godror driver (requires OCI)
		driverName = "godror"
		finalConnStr = fmt.Sprintf(`user="%s" password="%s" connectString="%s"`,
			config.User, config.Password, connectStringBase)
		logger.DebugContext(ctx, fmt.Sprintf("Using godror driver (OCI-based) with connectString: %s\n", connectStringBase))
	} else {
		// Use go-ora driver (pure Go)
		driverName = "oracle"

		finalConnStr = buildGoOraConnString(config.User, config.Password, connectStringBase, config.WalletLocation)

		if hasWallet {
			logger.DebugContext(ctx, fmt.Sprintf("Using go-ora driver (pure-Go) with wallet and serverString: %s\n", connectStringBase))
		} else {
			logger.DebugContext(ctx, fmt.Sprintf("Using go-ora driver (pure-Go) with serverString: %s\n", connectStringBase))
		}
	}

	db, err := sql.Open(driverName, finalConnStr)
	if err != nil {
		return nil, fmt.Errorf("unable to open Oracle connection with driver %s: %w", driverName, err)
	}

	if config.DisablePooling {
		setPoolLimits(db, 0, 0)
	}

	return db, nil
}
