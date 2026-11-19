// Copyright © 2025, Oracle and/or its affiliates.
package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
	_ "github.com/godror/godror"   // OCI driver
	_ "github.com/sijms/go-ora/v3" // Pure Go driver

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
	Name             string `yaml:"name" validate:"required"`
	Type             string `yaml:"type" validate:"required"`
	ConnectionString string `yaml:"connectionString,omitempty"`
	TnsAlias         string `yaml:"tnsAlias,omitempty"`
	TnsAdmin         string `yaml:"tnsAdmin,omitempty"`
	Host             string `yaml:"host,omitempty"`
	Port             int    `yaml:"port,omitempty"`
	ServiceName      string `yaml:"serviceName,omitempty"`
	User             string `yaml:"user" validate:"required"`
	Password         string `yaml:"password" validate:"required"`
	UseOCI           bool   `yaml:"useOCI,omitempty"`
	WalletLocation   string `yaml:"walletLocation,omitempty"`
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

	return nil
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	db, err := initOracleConnection(ctx, tracer, r)
	if err != nil {
		return nil, fmt.Errorf("unable to create Oracle connection: %w", err)
	}

	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("unable to connect to Oracle successfully: %w", err)
	}

	s := &Source{
		Config: r,
		DB:     db,
	}
	return s, nil
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	DB *sql.DB
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

func (s *Source) RunSQL(ctx context.Context, statement string, params []any, readOnly bool) (any, error) {
	if !readOnly && validateReadOnlySQL(statement) != nil {
		result, err := s.OracleDB().ExecContext(ctx, statement, params...)
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

	var (
		rows *sql.Rows
		tx   *sql.Tx
		err  error
	)
	if readOnly {
		if err = validateReadOnlySQL(statement); err != nil {
			return nil, fmt.Errorf("read-only mode violation: %w", err)
		}
		tx, err = s.beginReadOnlyTx(ctx)
		if err != nil {
			return nil, fmt.Errorf("unable to start read-only transaction: %w", err)
		}
		defer func() { _ = tx.Rollback() }()
		rows, err = tx.QueryContext(ctx, statement, params...)
	} else {
		rows, err = s.OracleDB().QueryContext(ctx, statement, params...)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve columns: %w", err)
	}

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

func (s *Source) beginReadOnlyTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.OracleDB().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "readonly transaction is not supported") {
		return tx, err
	}
	tx, err = s.OracleDB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "SET TRANSACTION READ ONLY"); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '$' || b == '#' || b >= 0x80
}

func qQuoteEnd(open byte) byte {
	switch open {
	case '[':
		return ']'
	case '{':
		return '}'
	case '<':
		return '>'
	case '(':
		return ')'
	default:
		return open
	}
}

// validateReadOnlySQL ensures the query starts with SELECT or WITH, has no
// FOR UPDATE locking clause, and contains no non-trailing semicolons (blocking
// DDL, PL/SQL blocks, WITH FUNCTION/PROCEDURE declarations, and stacked SQL).
func validateReadOnlySQL(s string) error {
	firstWord, prevWord, prevDot, sawSemi := "", "", false, false
	for i, n := 0, len(s); i < n; {
		ch := s[i]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' || ch == '\v' {
			i++
			continue
		}
		if ch == '-' && i+1 < n && s[i+1] == '-' {
			for i += 2; i < n && s[i] != '\n'; i++ {
			}
			continue
		}
		if ch == '/' && i+1 < n && s[i+1] == '*' {
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return fmt.Errorf("unterminated block comment")
			}
			i += end + 4
			continue
		}
		if ch == ';' {
			if firstWord == "" {
				return fmt.Errorf("empty SQL statement")
			}
			sawSemi = true
			i++
			continue
		}
		if sawSemi {
			return fmt.Errorf("multiple SQL statements are not allowed in read-only mode")
		}
		if firstWord == "" {
			if ch == '(' {
				i++
				continue
			}
			start := i
			for i < n && isIdentByte(s[i]) {
				i++
			}
			firstWord = strings.ToUpper(s[start:i])
			if firstWord != "SELECT" && firstWord != "WITH" {
				return fmt.Errorf("only SELECT or WITH queries are allowed in read-only mode")
			}
			prevWord = firstWord
			continue
		}
		qOff := 0
		if (ch == 'q' || ch == 'Q') && i+2 < n && s[i+1] == '\'' {
			qOff = 2
		} else if (ch == 'n' || ch == 'N') && i+3 < n && (s[i+1] == 'q' || s[i+1] == 'Q') && s[i+2] == '\'' {
			qOff = 3
		}
		if qOff > 0 {
			end := strings.Index(s[i+qOff+1:], string([]byte{qQuoteEnd(s[i+qOff]), '\''}))
			if end < 0 {
				return fmt.Errorf("unterminated quoted literal")
			}
			i += qOff + 1 + end + 2
			prevWord, prevDot = "", false
			continue
		}
		if (ch == 'n' || ch == 'N') && i+1 < n && s[i+1] == '\'' {
			i++
			ch = '\''
		}
		if ch == '\'' || ch == '"' {
			quote, closed := ch, false
			for i++; i < n; {
				if s[i] == quote {
					i++
					if i < n && s[i] == quote {
						i++
						continue
					}
					closed = true
					break
				}
				i++
			}
			if !closed {
				return fmt.Errorf("unterminated quoted string")
			}
			prevWord, prevDot = "", false
			continue
		}
		if ch == ':' {
			for i++; i < n && isIdentByte(s[i]); i++ {
			}
			prevWord, prevDot = "", false
			continue
		}
		if isIdentByte(ch) {
			start := i
			for i++; i < n && isIdentByte(s[i]); i++ {
			}
			if !prevDot {
				word := strings.ToUpper(s[start:i])
				if prevWord == "FOR" && word == "UPDATE" {
					return fmt.Errorf("FOR UPDATE locking clauses are not allowed in read-only mode")
				}
				prevWord = word
			} else {
				prevWord = ""
			}
			prevDot = false
			continue
		}
		prevWord, prevDot = "", ch == '.'
		i++
	}
	if firstWord == "" {
		return fmt.Errorf("empty SQL statement")
	}
	return nil
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

func initOracleConnection(ctx context.Context, tracer trace.Tracer, config Config) (*sql.DB, error) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, config.Name)
	defer span.End()

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

	return db, nil
}
