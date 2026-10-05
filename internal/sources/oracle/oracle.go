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
	if !readOnly {
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
	rows, err := s.OracleDB().QueryContext(ctx, statement, params...)
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
