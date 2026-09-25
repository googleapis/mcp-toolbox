// Copyright © 2025, Oracle and/or its affiliates.

package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestParseFromYamlOracle(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "connection string and useOCI=true",
			in: `
			kind: source
			name: my-oracle-cs
			type: oracle
			connectionString: "my-host:1521/XEPDB1"
			user: my_user
			password: my_pass
			useOCI: true
			`,
			want: map[string]sources.SourceConfig{
				"my-oracle-cs": Config{
					Name:             "my-oracle-cs",
					Type:             SourceType,
					ConnectionString: "my-host:1521/XEPDB1",
					User:             "my_user",
					Password:         "my_pass",
					UseOCI:           true,
				},
			},
		},
		{
			desc: "host/port/serviceName and default useOCI=false",
			in: `
			kind: source
			name: my-oracle-host
			type: oracle
			host: my-host
			port: 1521
			serviceName: ORCLPDB
			user: my_user
			password: my_pass
			`,
			want: map[string]sources.SourceConfig{
				"my-oracle-host": Config{
					Name:        "my-oracle-host",
					Type:        SourceType,
					Host:        "my-host",
					Port:        1521,
					ServiceName: "ORCLPDB",
					User:        "my_user",
					Password:    "my_pass",
					UseOCI:      false,
				},
			},
		},
		{
			desc: "tnsAlias and TnsAdmin specified with explicit useOCI=true",
			in: `
			kind: source
			name: my-oracle-tns-oci
			type: oracle
			tnsAlias: FINANCE_DB
			tnsAdmin: /opt/oracle/network/admin
			user: my_user
			password: my_pass
			useOCI: true 
			`,
			want: map[string]sources.SourceConfig{
				"my-oracle-tns-oci": Config{
					Name:     "my-oracle-tns-oci",
					Type:     SourceType,
					TnsAlias: "FINANCE_DB",
					TnsAdmin: "/opt/oracle/network/admin",
					User:     "my_user",
					Password: "my_pass",
					UseOCI:   true,
				},
			},
		},
		{
			desc: "dedicated connection isolation and VPD fields",
			in: `
			kind: source
			name: my-oracle-vpd
			type: oracle
			connectionString: "my-host:1521/XEPDB1"
			user: my_user
			password: my_pass
			disablePooling: true
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;"
			sessionContextClaim: "username"
			sessionResetBlock: "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;"
			`,
			want: map[string]sources.SourceConfig{
				"my-oracle-vpd": Config{
					Name:                "my-oracle-vpd",
					Type:                SourceType,
					ConnectionString:    "my-host:1521/XEPDB1",
					User:                "my_user",
					Password:            "my_pass",
					DisablePooling:      true,
					SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
					SessionContextClaim: "username",
					SessionResetBlock:   "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;",
				},
			},
		},
		{
			desc: "dedicated connection isolation and VPD fields with omitted sessionContextClaim",
			in: `
			kind: source
			name: my-oracle-vpd-default-claim
			type: oracle
			connectionString: "my-host:1521/XEPDB1"
			user: my_user
			password: my_pass
			disablePooling: true
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;"
			sessionResetBlock: "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;"
			`,
			want: map[string]sources.SourceConfig{
				"my-oracle-vpd-default-claim": Config{
					Name:                "my-oracle-vpd-default-claim",
					Type:                SourceType,
					ConnectionString:    "my-host:1521/XEPDB1",
					User:                "my_user",
					Password:            "my_pass",
					DisablePooling:      true,
					SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
					SessionResetBlock:   "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse:\nwant: %v\ngot:  %v\ndiff: %s", tc.want, got, cmp.Diff(tc.want, got))
			}
		})
	}
}

func TestBuildGoOraConnString(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name           string
		user           string
		password       string
		connectBase    string
		walletLocation string
		want           string
	}{
		{
			name:           "encodes_credentials_and_wallet",
			user:           "user[client]",
			password:       "pa:ss@word",
			connectBase:    "dbhost:1521/XEPDB1",
			walletLocation: "/tmp/my wallet",
			want:           "oracle://user%5Bclient%5D:pa%3Ass%40word@dbhost:1521/XEPDB1?ssl=true&wallet=%2Ftmp%2Fmy+wallet",
		},
		{
			name:        "no_wallet",
			user:        "scott",
			password:    "tiger",
			connectBase: "dbhost:1521/ORCL",
			want:        "oracle://scott:tiger@dbhost:1521/ORCL",
		},
		{
			name:        "does_not_double_encode_percent_encoded_user",
			user:        "app_user%5BCLIENT_A%5D",
			password:    "secret",
			connectBase: "dbhost:1521/ORCL",
			want:        "oracle://app_user%5BCLIENT_A%5D:secret@dbhost:1521/ORCL",
		},
		{
			name:           "uses_trimmed_wallet_location",
			user:           "scott",
			password:       "tiger",
			connectBase:    "dbhost:1521/ORCL",
			walletLocation: "  /tmp/wallet  ",
			want:           "oracle://scott:tiger@dbhost:1521/ORCL?ssl=true&wallet=%2Ftmp%2Fwallet",
		},
		{
			name:           "appends_wallet_query_to_existing_query",
			user:           "scott",
			password:       "tiger",
			connectBase:    "dbhost:1521/ORCL?custom_opt=true",
			walletLocation: " /tmp/wallet ",
			want:           "oracle://scott:tiger@dbhost:1521/ORCL?custom_opt=true&ssl=true&wallet=%2Ftmp%2Fwallet",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildGoOraConnString(tc.user, tc.password, tc.connectBase, tc.walletLocation)
			if got != tc.want {
				t.Fatalf("buildGoOraConnString() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFailParseFromYaml(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: `
			kind: source
			name: my-oracle-instance
			type: oracle
			host: my-host
			serviceName: ORCL
			user: my_user
			password: my_pass
			extraField: value
			`,
			err: "error unmarshaling source: unable to parse source \"my-oracle-instance\" as \"oracle\": [1:1] unknown field \"extraField\"\n>  1 | extraField: value\n       ^\n   2 | host: my-host\n   3 | name: my-oracle-instance\n   4 | password: my_pass\n   5 | ",
		},
		{
			desc: "missing required password field",
			in: `
			kind: source
			name: my-oracle-instance
			type: oracle
			host: my-host
			serviceName: ORCL
			user: my_user
			`,
			err: "error unmarshaling source: unable to parse source \"my-oracle-instance\" as \"oracle\": Key: 'Config.Password' Error:Field validation for 'Password' failed on the 'required' tag",
		},
		{
			desc: "missing connection method fields (validate fails)",
			in: `
			kind: source
			name: my-oracle-instance
			type: oracle
			user: my_user
			password: my_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-oracle-instance\" as \"oracle\": invalid Oracle configuration: must provide one of: 'tns_alias', 'connection_string', or both 'host' and 'service_name'",
		},
		{
			desc: "multiple connection methods provided (validate fails)",
			in: `
			kind: source
			name: my-oracle-instance
			type: oracle
			host: my-host
			serviceName: ORCL
			connectionString: "my-host:1521/XEPDB1"
			user: my_user
			password: my_pass
			`,
			err: "error unmarshaling source: unable to parse source \"my-oracle-instance\" as \"oracle\": invalid Oracle configuration: provide only one connection method: 'tns_alias', 'connection_string', or 'host'+'service_name'",
		},
		{
			desc: "fail on tnsAdmin with useOCI=false",
			in: `
			kind: source
			name: my-oracle-fail
			type: oracle
			tnsAlias: FINANCE_DB
			tnsAdmin: /opt/oracle/network/admin
			user: my_user
			password: my_pass
			useOCI: false
			`,
			err: "error unmarshaling source: unable to parse source \"my-oracle-fail\" as \"oracle\": invalid Oracle configuration: `tnsAdmin` can only be used when `UseOCI` is true, or use `walletLocation` instead",
		},
		{
			desc: "fail on sessionResetBlock without sessionContextBlock",
			in: `
			kind: source
			name: my-oracle-fail
			type: oracle
			connectionString: "my-host:1521/XEPDB1"
			user: my_user
			password: my_pass
			sessionResetBlock: "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;"
			`,
			err: "error unmarshaling source: unable to parse source \"my-oracle-fail\" as \"oracle\": invalid Oracle configuration: `sessionResetBlock` requires `sessionContextBlock` to be configured",
		},
		{
			desc: "fail on sessionContextClaim without sessionContextBlock",
			in: `
			kind: source
			name: my-oracle-fail
			type: oracle
			connectionString: "my-host:1521/XEPDB1"
			user: my_user
			password: my_pass
			sessionContextClaim: "custom_claim"
			`,
			err: "error unmarshaling source: unable to parse source \"my-oracle-fail\" as \"oracle\": invalid Oracle configuration: `sessionContextClaim` requires `sessionContextBlock` to be configured",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := strings.ReplaceAll(err.Error(), "\r", "")

			if errStr != tc.err {
				t.Fatalf("unexpected error:\ngot:\n%q\nwant:\n%q\n", errStr, tc.err)
			}
		})
	}
}

// TestRunSQLExecutesDML verifies that RunSQL correctly routes operations to
// ExecContext instead of QueryContext when the readOnly flag is set to false.
func TestRunSQLExecutesDML(t *testing.T) {
	// Initialize a mock database connection.
	// This connection is not established with a real backend but
	// satisfies the interface requirements for the test.
	db, err := sql.Open("oracle", "oracle://user:pass@localhost:1521/service")
	if err != nil {
		t.Fatalf("failed to open mock db: %v", err)
	}
	defer db.Close()

	cfg := Config{
		Name: "test-dml-source",
		Type: SourceType,
		User: "test-user",
	}
	src := &Source{
		Config: cfg,
		conn:   sources.NewConnectOnce[*sql.DB](context.Background(), cfg.Name, SourceType, noop.NewTracerProvider().Tracer("test")),
	}

	// Seed the lazy connection with the mock handle so RunSQL does not dial Oracle.
	if _, err := src.conn.Do(context.Background(), func(context.Context) (*sql.DB, error) {
		return db, nil
	}); err != nil {
		t.Fatalf("failed to seed connection: %v", err)
	}

	// Invoke RunSQL with readOnly=false to force the DML execution path.
	_, err = src.RunSQL(context.Background(),
		"UPDATE users SET email='x' WHERE id=1", nil, false)

	// We expect an error because the mock database cannot execute the query.
	// If err is nil, it implies the logic skipped the execution block.
	if err == nil {
		t.Fatal("expected error from fake DB execution, but got nil; " +
			"DML path may not have been executed")
	}
}

func TestInitializeOracle(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("failed to create context with logger: %v", err)
	}
	tracer := noop.NewTracerProvider().Tracer("oracle-test")

	origPingDB := pingDB
	pingDB = func(ctx context.Context, db *sql.DB) error {
		return nil
	}
	defer func() { pingDB = origPingDB }()

	t.Run("populates fields on source and defaults sessionContextClaim to email when omitted", func(t *testing.T) {
		cfg := Config{
			Name:                "vpd-source",
			Type:                SourceType,
			ConnectionString:    "localhost:1521/XEPDB1",
			User:                "test_user",
			Password:            "test_pass",
			DisablePooling:      true,
			SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			SessionResetBlock:   "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;",
		}

		rawSrc, err := cfg.Initialize(ctx, tracer)
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		src, ok := rawSrc.(*Source)
		if !ok {
			t.Fatalf("expected *Source, got %T", rawSrc)
		}
		defer src.OracleDB().Close()

		if !src.DisablePooling {
			t.Errorf("expected src.DisablePooling to be true")
		}
		if src.SessionContextBlock != "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;" {
			t.Errorf("unexpected SessionContextBlock: %q", src.SessionContextBlock)
		}
		if src.SessionContextClaim != "email" {
			t.Errorf("expected SessionContextClaim to default to 'email', got %q", src.SessionContextClaim)
		}
		if src.SessionResetBlock != "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;" {
			t.Errorf("unexpected SessionResetBlock: %q", src.SessionResetBlock)
		}
		if src.OracleDB() == nil {
			t.Errorf("expected OracleDB to be non-nil")
		}

		cfgFromSrc, ok := src.ToConfig().(Config)
		if !ok {
			t.Fatalf("expected Config from ToConfig(), got %T", src.ToConfig())
		}
		if cfgFromSrc.SessionContextClaim != "email" {
			t.Errorf("expected ToConfig().SessionContextClaim to default to 'email', got %q", cfgFromSrc.SessionContextClaim)
		}
		if !cfgFromSrc.DisablePooling {
			t.Errorf("expected ToConfig().DisablePooling to be true")
		}
	})

	t.Run("preserves explicit sessionContextClaim", func(t *testing.T) {
		cfg := Config{
			Name:                "vpd-source-explicit-claim",
			Type:                SourceType,
			ConnectionString:    "localhost:1521/XEPDB1",
			User:                "test_user",
			Password:            "test_pass",
			DisablePooling:      true,
			SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			SessionContextClaim: "custom_claim",
			SessionResetBlock:   "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;",
		}

		rawSrc, err := cfg.Initialize(ctx, tracer)
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		src, ok := rawSrc.(*Source)
		if !ok {
			t.Fatalf("expected *Source, got %T", rawSrc)
		}
		defer src.OracleDB().Close()

		if src.SessionContextClaim != "custom_claim" {
			t.Errorf("expected SessionContextClaim to be 'custom_claim', got %q", src.SessionContextClaim)
		}
	})

	t.Run("trims whitespace on sessionContextBlock, sessionContextClaim, and sessionResetBlock", func(t *testing.T) {
		cfg := Config{
			Name:                "whitespace-claim-source",
			Type:                SourceType,
			ConnectionString:    "localhost:1521/XEPDB1",
			User:                "test_user",
			Password:            "test_pass",
			SessionContextBlock: "  BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;  ",
			SessionContextClaim: "   ",
			SessionResetBlock:   "  BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;  ",
		}

		rawSrc, err := cfg.Initialize(ctx, tracer)
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		src, ok := rawSrc.(*Source)
		if !ok {
			t.Fatalf("expected *Source, got %T", rawSrc)
		}
		defer src.OracleDB().Close()

		if src.SessionContextBlock != "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;" {
			t.Errorf("expected trimmed SessionContextBlock, got %q", src.SessionContextBlock)
		}
		if src.SessionContextClaim != "email" {
			t.Errorf("expected SessionContextClaim to default to 'email' when whitespace, got %q", src.SessionContextClaim)
		}
		if src.SessionResetBlock != "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;" {
			t.Errorf("expected trimmed SessionResetBlock, got %q", src.SessionResetBlock)
		}
	})

	t.Run("backwards compatibility when pooling and context blocks omitted", func(t *testing.T) {
		cfg := Config{
			Name:             "standard-source",
			Type:             SourceType,
			ConnectionString: "localhost:1521/XEPDB1",
			User:             "test_user",
			Password:         "test_pass",
		}

		rawSrc, err := cfg.Initialize(ctx, tracer)
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		src, ok := rawSrc.(*Source)
		if !ok {
			t.Fatalf("expected *Source, got %T", rawSrc)
		}
		defer src.OracleDB().Close()

		if src.DisablePooling {
			t.Errorf("expected DisablePooling to be false")
		}
		if src.SessionContextBlock != "" {
			t.Errorf("expected empty SessionContextBlock, got %q", src.SessionContextBlock)
		}
		if src.SessionContextClaim != "" {
			t.Errorf("expected empty SessionContextClaim, got %q", src.SessionContextClaim)
		}
		if src.SessionResetBlock != "" {
			t.Errorf("expected empty SessionResetBlock, got %q", src.SessionResetBlock)
		}
	})

	t.Run("returns error when pingDB fails", func(t *testing.T) {
		prevPingDB := pingDB
		pingDB = func(ctx context.Context, db *sql.DB) error {
			return fmt.Errorf("connection refused")
		}
		defer func() { pingDB = prevPingDB }()

		cfg := Config{
			Name:             "ping-failure-source",
			Type:             SourceType,
			ConnectionString: "localhost:1521/XEPDB1",
			User:             "test_user",
			Password:         "test_pass",
		}

		_, err := cfg.Initialize(ctx, tracer)
		if err == nil {
			t.Fatalf("expected error from failed pingDB, got nil")
		}
		if !strings.Contains(err.Error(), "unable to connect to Oracle successfully") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestInitOracleConnectionPooling(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("failed to create context with logger: %v", err)
	}
	tracer := noop.NewTracerProvider().Tracer("oracle-test")

	t.Run("sets pool limits when disablePooling is true", func(t *testing.T) {
		origSetPoolLimits := setPoolLimits
		var called bool
		var gotMaxIdle int
		var gotLifetime time.Duration

		setPoolLimits = func(db *sql.DB, maxIdleConns int, connMaxLifetime time.Duration) {
			called = true
			gotMaxIdle = maxIdleConns
			gotLifetime = connMaxLifetime
			origSetPoolLimits(db, maxIdleConns, connMaxLifetime)
		}
		defer func() { setPoolLimits = origSetPoolLimits }()

		cfg := Config{
			Name:             "unpooled-oracle",
			Type:             SourceType,
			ConnectionString: "localhost:1521/XEPDB1",
			User:             "test_user",
			Password:         "test_pass",
			DisablePooling:   true,
		}

		db, err := initOracleConnection(ctx, tracer, cfg)
		if err != nil {
			t.Fatalf("initOracleConnection failed: %v", err)
		}
		defer db.Close()

		if !called {
			t.Fatal("expected setPoolLimits to be called when disablePooling is true")
		}
		if gotMaxIdle != 0 {
			t.Errorf("expected maxIdleConns=0, got %d", gotMaxIdle)
		}
		if gotLifetime != 0 {
			t.Errorf("expected connMaxLifetime=0, got %v", gotLifetime)
		}
	})

	t.Run("does not set pool limits when disablePooling is false or omitted", func(t *testing.T) {
		for _, tc := range []struct {
			desc           string
			disablePooling bool
		}{
			{desc: "explicit false", disablePooling: false},
			{desc: "omitted default", disablePooling: false},
		} {
			t.Run(tc.desc, func(t *testing.T) {
				origSetPoolLimits := setPoolLimits
				var called bool

				setPoolLimits = func(db *sql.DB, maxIdleConns int, connMaxLifetime time.Duration) {
					called = true
					origSetPoolLimits(db, maxIdleConns, connMaxLifetime)
				}
				defer func() { setPoolLimits = origSetPoolLimits }()

				cfg := Config{
					Name:             "pooled-oracle-" + tc.desc,
					Type:             SourceType,
					ConnectionString: "localhost:1521/XEPDB1",
					User:             "test_user",
					Password:         "test_pass",
					DisablePooling:   tc.disablePooling,
				}

				db, err := initOracleConnection(ctx, tracer, cfg)
				if err != nil {
					t.Fatalf("initOracleConnection failed: %v", err)
				}
				defer db.Close()

				if called {
					t.Errorf("expected setPoolLimits not to be called for %s", tc.desc)
				}
			})
		}
	})
}

func TestExtractUserIdentity(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name                string
		sessionContextBlock string
		sessionContextClaim string
		claims              map[string]any
		hasClaims           bool
		wantIdentity        string
		wantErr             bool
		wantErrCode         int
		wantErrSubstr       string
	}{
		{
			name:                "AC-1.2.1: valid configured claim extracts identity",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "username",
			claims:              map[string]any{"username": "alice"},
			hasClaims:           true,
			wantIdentity:        "alice",
		},
		{
			name:                "AC-1.2.1: valid default email claim extracts identity",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "email",
			claims:              map[string]any{"email": "alice@company.com"},
			hasClaims:           true,
			wantIdentity:        "alice@company.com",
		},
		{
			name:                "valid default claim when sessionContextClaim is omitted",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "",
			claims:              map[string]any{"email": "alice@corp.com"},
			hasClaims:           true,
			wantIdentity:        "alice@corp.com",
		},
		{
			name:                "AC-1.2.2: fallback to sub claim when email claim is absent",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "email",
			claims:              map[string]any{"sub": "auth0|12345"},
			hasClaims:           true,
			wantIdentity:        "auth0|12345",
		},
		{
			name:                "strict custom claim matching: extracts custom claim when present",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "custom_user",
			claims:              map[string]any{"custom_user": "carol", "email": "other@corp.com"},
			hasClaims:           true,
			wantIdentity:        "carol",
		},
		{
			name:                "strict custom claim matching: does NOT fall back to email",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "custom_user",
			claims:              map[string]any{"email": "bob@corp.com"},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "strict custom claim matching: does NOT fall back to sub",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "custom_user",
			claims:              map[string]any{"sub": "sub|67890"},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "strict custom claim matching: whitespace custom claim does NOT fall back to email or sub",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "custom_user",
			claims:              map[string]any{"custom_user": "   ", "email": "carol@corp.com", "sub": "sub|999"},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "fallback when email claim is whitespace -> sub",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "email",
			claims:              map[string]any{"email": "   ", "sub": "sub|999"},
			hasClaims:           true,
			wantIdentity:        "sub|999",
		},
		{
			name:                "scalar numeric int claim value converted to string",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "uid",
			claims:              map[string]any{"uid": 123456},
			hasClaims:           true,
			wantIdentity:        "123456",
		},
		{
			name:                "scalar numeric int64 and uint64 claim value converted to string",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "uid",
			claims:              map[string]any{"uid": int64(9876543210)},
			hasClaims:           true,
			wantIdentity:        "9876543210",
		},
		{
			name:                "scalar numeric float64 claim value converted to string",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "uid",
			claims:              map[string]any{"uid": float64(42)},
			hasClaims:           true,
			wantIdentity:        "42",
		},
		{
			name:                "scalar numeric float64 claim >= 1e7 formats without exponential notation",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "uid",
			claims:              map[string]any{"uid": float64(12345678)},
			hasClaims:           true,
			wantIdentity:        "12345678",
		},
		{
			name:                "scalar numeric float32 claim converted to string",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "uid",
			claims:              map[string]any{"uid": float32(98765)},
			hasClaims:           true,
			wantIdentity:        "98765",
		},
		{
			name:                "scalar numeric float64 NaN rejected",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "uid",
			claims:              map[string]any{"uid": math.NaN()},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "scalar numeric float64 Inf rejected",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "uid",
			claims:              map[string]any{"uid": math.Inf(1)},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "scalar json.Number claim value converted to string",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "uid",
			claims:              map[string]any{"uid": json.Number("777")},
			hasClaims:           true,
			wantIdentity:        "777",
		},
		{
			name:                "non-scalar slice claim value rejected",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "email",
			claims:              map[string]any{"email": []string{"alice@corp.com"}},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "non-scalar empty slice claim value rejected",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "email",
			claims:              map[string]any{"email": []any{}},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "non-scalar map claim value rejected",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "email",
			claims:              map[string]any{"email": map[string]any{"nested": "val"}},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "non-scalar empty map claim value rejected",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "email",
			claims:              map[string]any{"email": map[string]any{}},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "non-scalar bool claim value rejected",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "email",
			claims:              map[string]any{"email": true},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "AC-1.2.3: missing all claims in context (nil claims)",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "email",
			hasClaims:           false,
			wantIdentity:        "",
		},
		{
			name:                "AC-1.2.3: empty claims map",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "email",
			claims:              map[string]any{},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "AC-1.2.3: claim key present but empty/nil",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "email",
			claims:              map[string]any{"email": "   ", "sub": nil},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "AC-1.2.3: target claim absent and no sub",
			sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
			sessionContextClaim: "email",
			claims:              map[string]any{"other": "val"},
			hasClaims:           true,
			wantIdentity:        "",
		},
		{
			name:                "AC-1.2.4: unconfigured session context block bypasses extraction",
			sessionContextBlock: "",
			sessionContextClaim: "",
			hasClaims:           false,
			wantIdentity:        "",
			wantErr:             false,
		},
		{
			name:                "AC-1.2.4: whitespace-only session context block bypasses extraction",
			sessionContextBlock: "   ",
			sessionContextClaim: "email",
			hasClaims:           false,
			wantIdentity:        "",
			wantErr:             false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := &Source{
				Config: Config{
					Name:                "test-oracle",
					Type:                SourceType,
					SessionContextBlock: tc.sessionContextBlock,
					SessionContextClaim: tc.sessionContextClaim,
				},
			}

			ctx := context.Background()
			if tc.hasClaims {
				ctx = util.WithAuthTokenClaims(ctx, tc.claims)
			}

			gotIdentity, err := src.extractUserIdentity(ctx)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("extractUserIdentity() expected error, got nil (identity: %q)", gotIdentity)
				}
				var clientErr *util.ClientServerError
				if !errors.As(err, &clientErr) {
					t.Fatalf("expected error of type *util.ClientServerError, got %T: %v", err, err)
				}
				if clientErr.Code != tc.wantErrCode {
					t.Errorf("expected error code %d, got %d", tc.wantErrCode, clientErr.Code)
				}
				if clientErr.Category() != util.CategoryServer {
					t.Errorf("expected CategoryServer, got %v", clientErr.Category())
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstr) {
					t.Errorf("error %q does not contain expected substring %q", err.Error(), tc.wantErrSubstr)
				}
				if strings.Contains(err.Error(), "alice") || strings.Contains(err.Error(), "auth0") {
					t.Errorf("error leaks caller identity: %q", err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("extractUserIdentity() unexpected error: %v", err)
				}
				if gotIdentity != tc.wantIdentity {
					t.Errorf("extractUserIdentity() = %q, want %q", gotIdentity, tc.wantIdentity)
				}
			}
		})
	}
}

func TestRunSQLIdentityExtractionAndFailClosedGuard(t *testing.T) {
	t.Parallel()

	t.Run("unauthenticated read query proceeds with empty identity", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		ctx := context.Background()
		_, err := src.RunSQL(ctx, "SELECT 1 FROM DUAL", nil, true)
		if err != nil {
			t.Fatalf("expected RunSQL to succeed without 401 error, got %v", err)
		}
		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		calls := conns[0].getCalls()
		if len(calls) < 2 {
			t.Fatalf("expected at least 2 calls, got %d", len(calls))
		}
		if calls[0].Query != src.SessionContextBlock {
			t.Errorf("call[0] query = %q, want %q", calls[0].Query, src.SessionContextBlock)
		}
		if len(calls[0].Args) != 1 {
			t.Fatalf("expected 1 bind argument, got %d", len(calls[0].Args))
		}
		if calls[0].Args[0].Name != "user_identity" || calls[0].Args[0].Value != "" {
			t.Errorf("expected NamedArg(user_identity, \"\"), got %#v", calls[0].Args[0])
		}
	})

	t.Run("empty claims proceed with empty identity", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"email": "   ",
			"sub":   nil,
		})
		_, err := src.RunSQL(ctx, "SELECT 1 FROM DUAL", nil, true)
		if err != nil {
			t.Fatalf("expected RunSQL to succeed without 401 error, got %v", err)
		}
		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		calls := conns[0].getCalls()
		if len(calls[0].Args) != 1 || calls[0].Args[0].Name != "user_identity" || calls[0].Args[0].Value != "" {
			t.Errorf("expected NamedArg(user_identity, \"\"), got %#v", calls[0].Args)
		}
	})

	t.Run("write path (readOnly: false) succeeds when unauthenticated", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		ctx := context.Background()
		_, err := src.RunSQL(ctx, "UPDATE employees SET salary = salary * 1.1", nil, false)
		if err != nil {
			t.Fatalf("expected RunSQL write path to succeed without 401 error, got %v", err)
		}
		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		calls := conns[0].getCalls()
		if len(calls[0].Args) != 1 || calls[0].Args[0].Name != "user_identity" || calls[0].Args[0].Value != "" {
			t.Errorf("expected NamedArg(user_identity, \"\"), got %#v", calls[0].Args)
		}
	})

	t.Run("write path (readOnly: false) succeeds when claims contain non-scalar identity", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"email": []string{"not-a-scalar"},
		})
		_, err := src.RunSQL(ctx, "DELETE FROM audit_logs", nil, false)
		if err != nil {
			t.Fatalf("expected RunSQL write path to succeed, got %v", err)
		}
		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		calls := conns[0].getCalls()
		if len(calls[0].Args) != 1 || calls[0].Args[0].Name != "user_identity" || calls[0].Args[0].Value != "" {
			t.Errorf("expected NamedArg(user_identity, \"\"), got %#v", calls[0].Args)
		}
	})

	t.Run("AC-1.2.4: unconfigured session context block bypasses extraction", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "standard-source",
				Type:                SourceType,
				SessionContextBlock: "",
			},
			DB: db,
		}

		ctx := context.Background()
		_, err := src.RunSQL(ctx, "SELECT 1 FROM DUAL", nil, true)
		if err != nil {
			t.Fatalf("expected unconfigured session context block query to succeed, got: %v", err)
		}
		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		calls := conns[0].getCalls()
		// Only Query call and Close call, no session setup
		if len(calls) != 2 || calls[0].Type != "Query" || calls[1].Type != "Close" {
			t.Errorf("expected Query and Close calls, got %v", calls)
		}
	})

	t.Run("AC-1.2.1: valid configured email claim binds caller identity", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"email": "alice@company.com",
		})
		_, err := src.RunSQL(ctx, "SELECT 1 FROM DUAL", nil, true)
		if err != nil {
			t.Fatalf("expected query with valid claims to succeed, got %v", err)
		}
		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		calls := conns[0].getCalls()
		if len(calls[0].Args) != 1 || calls[0].Args[0].Name != "user_identity" || calls[0].Args[0].Value != "alice@company.com" {
			t.Errorf("expected NamedArg(user_identity, alice@company.com), got %#v", calls[0].Args)
		}
	})

	t.Run("AC-1.2.2: fallback to sub claim binds caller identity", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"sub": "auth0|12345",
		})
		_, err := src.RunSQL(ctx, "SELECT 1 FROM DUAL", nil, true)
		if err != nil {
			t.Fatalf("expected query with sub fallback to succeed, got %v", err)
		}
		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		calls := conns[0].getCalls()
		if len(calls[0].Args) != 1 || calls[0].Args[0].Name != "user_identity" || calls[0].Args[0].Value != "auth0|12345" {
			t.Errorf("expected NamedArg(user_identity, auth0|12345), got %#v", calls[0].Args)
		}
	})
}

const mockPipelineDriverName = "mock-oracle-pipeline"

func init() {
	sql.Register(mockPipelineDriverName, &mockPipelineDriver{})
}

type mockCall struct {
	Type  string // "Exec", "Query", "Begin", "BeginTx", "Close"
	Query string
	Args  []driver.NamedValue
}

type mockConn struct {
	id     int
	driver *mockPipelineDriverState
	closed bool
	mu     sync.Mutex
	calls  []mockCall
}

func (c *mockConn) Prepare(query string) (driver.Stmt, error) {
	return nil, errors.New("prepare not implemented in mock")
}

func (c *mockConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.calls = append(c.calls, mockCall{Type: "Close"})
	c.mu.Unlock()

	c.driver.mu.Lock()
	onClose := c.driver.onConnClose
	c.driver.mu.Unlock()
	if onClose != nil {
		onClose(c.id)
	}
	return nil
}

func (c *mockConn) Begin() (driver.Tx, error) {
	c.mu.Lock()
	c.calls = append(c.calls, mockCall{Type: "Begin"})
	c.mu.Unlock()
	return nil, errors.New("transactions not supported")
}

func (c *mockConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.mu.Lock()
	c.calls = append(c.calls, mockCall{Type: "BeginTx"})
	c.mu.Unlock()
	return nil, errors.New("transactions not supported")
}

func (c *mockConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.mu.Lock()
	c.calls = append(c.calls, mockCall{Type: "Exec", Query: query, Args: args})
	c.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.driver.mu.Lock()
	handler := c.driver.onExec
	c.driver.mu.Unlock()

	if handler != nil {
		return handler(ctx, c.id, query, args)
	}
	return mockResult{rowsAffected: 1}, nil
}

func (c *mockConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.mu.Lock()
	c.calls = append(c.calls, mockCall{Type: "Query", Query: query, Args: args})
	c.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.driver.mu.Lock()
	handler := c.driver.onQuery
	c.driver.mu.Unlock()

	if handler != nil {
		return handler(ctx, c.id, query, args)
	}
	return &mockRows{
		cols:     []string{"ID"},
		colTypes: []string{"NUMBER"},
		rows:     [][]driver.Value{{int64(1)}},
	}, nil
}

func (c *mockConn) getCalls() []mockCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	res := make([]mockCall, len(c.calls))
	copy(res, c.calls)
	return res
}

func (c *mockConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

type mockResult struct {
	rowsAffected int64
}

func (r mockResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (r mockResult) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}

type mockRows struct {
	cols     []string
	colTypes []string
	rows     [][]driver.Value
	idx      int
	closed   bool
}

func (r *mockRows) Columns() []string {
	return r.cols
}

func (r *mockRows) Close() error {
	r.closed = true
	return nil
}

func (r *mockRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

func (r *mockRows) ColumnTypeDatabaseTypeName(index int) string {
	if index < len(r.colTypes) {
		return r.colTypes[index]
	}
	return "VARCHAR2"
}

func (r *mockRows) ColumnTypePrecisionScale(index int) (precision, scale int64, ok bool) {
	if index < len(r.colTypes) && r.colTypes[index] == "NUMBER" {
		return 38, 0, true
	}
	return 0, 0, false
}

type mockPanicRows struct{}

func (r *mockPanicRows) Columns() []string {
	return []string{"ID"}
}

func (r *mockPanicRows) Close() error {
	return nil
}

func (r *mockPanicRows) Next(dest []driver.Value) error {
	panic("simulated fatal driver panic during row scanning")
}

func (r *mockPanicRows) ColumnTypeDatabaseTypeName(index int) string {
	return "NUMBER"
}

func (r *mockPanicRows) ColumnTypePrecisionScale(index int) (precision, scale int64, ok bool) {
	return 38, 0, true
}

type mockPipelineDriverState struct {
	mu          sync.Mutex
	conns       []*mockConn
	nextConnID  int
	onOpen      func() error
	onExec      func(ctx context.Context, connID int, query string, args []driver.NamedValue) (driver.Result, error)
	onQuery     func(ctx context.Context, connID int, query string, args []driver.NamedValue) (driver.Rows, error)
	onConnClose func(connID int)
}

func (s *mockPipelineDriverState) getConns() []*mockConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]*mockConn, len(s.conns))
	copy(res, s.conns)
	return res
}

type mockPipelineDriver struct{}

var (
	mockPipelineRegistry   = make(map[string]*mockPipelineDriverState)
	mockPipelineRegistryMu sync.Mutex
	mockPipelineCounter    atomic.Int64
)

func (d *mockPipelineDriver) Open(name string) (driver.Conn, error) {
	mockPipelineRegistryMu.Lock()
	state, ok := mockPipelineRegistry[name]
	mockPipelineRegistryMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("mock driver state not found for %s", name)
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.onOpen != nil {
		if err := state.onOpen(); err != nil {
			return nil, err
		}
	}

	state.nextConnID++
	conn := &mockConn{
		id:     state.nextConnID,
		driver: state,
	}
	state.conns = append(state.conns, conn)
	return conn, nil
}

func newMockDB(t *testing.T) (*sql.DB, *mockPipelineDriverState, func()) {
	t.Helper()
	id := fmt.Sprintf("test-%d", mockPipelineCounter.Add(1))
	state := &mockPipelineDriverState{}

	mockPipelineRegistryMu.Lock()
	mockPipelineRegistry[id] = state
	mockPipelineRegistryMu.Unlock()

	db, err := sql.Open(mockPipelineDriverName, id)
	if err != nil {
		t.Fatalf("failed to open mock db: %v", err)
	}
	db.SetMaxIdleConns(0)

	cleanup := func() {
		db.Close()
		mockPipelineRegistryMu.Lock()
		delete(mockPipelineRegistry, id)
		mockPipelineRegistryMu.Unlock()
	}
	return db, state, cleanup
}

func TestDedicatedConnectionPipeline(t *testing.T) {
	t.Parallel()

	t.Run("AC-1.3.1, AC-1.3.2, AC-1.3.3: acquires dedicated connection, binds :user_identity, executes autocommit query without *sql.Tx", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-pipeline-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"email": "alice@company.com",
		})

		res, err := src.RunSQL(ctx, "SELECT ID FROM USERS WHERE STATUS = :status", []any{"active"}, true)
		if err != nil {
			t.Fatalf("unexpected RunSQL error: %v", err)
		}

		// Verify query results
		rows, ok := res.([]any)
		if !ok || len(rows) != 1 {
			t.Fatalf("expected 1 row result, got %v", res)
		}
		rowMap, ok := rows[0].(map[string]any)
		if !ok || rowMap["ID"] != int64(1) {
			t.Fatalf("unexpected row data: %v", rows[0])
		}

		// AC-1.3.1: verify a single dedicated connection was acquired and held for both setup and query
		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected exactly 1 dedicated connection acquired, got %d", len(conns))
		}
		conn := conns[0]
		calls := conn.getCalls()

		// Expected call sequence on conn: Exec(setup), Query(statement), Close
		if len(calls) < 2 {
			t.Fatalf("expected at least 2 calls on conn, got %d: %v", len(calls), calls)
		}

		// Verify setup call (AC-1.3.1, AC-1.3.2)
		setupCall := calls[0]
		if setupCall.Type != "Exec" {
			t.Errorf("expected first call to be Exec, got %q", setupCall.Type)
		}
		if setupCall.Query != src.SessionContextBlock {
			t.Errorf("expected setup query %q, got %q", src.SessionContextBlock, setupCall.Query)
		}
		if len(setupCall.Args) != 1 {
			t.Fatalf("expected 1 arg in setup call, got %d", len(setupCall.Args))
		}
		// AC-1.3.2: bound using sql.Named("user_identity", userIdentity)
		if setupCall.Args[0].Name != "user_identity" {
			t.Errorf("expected named parameter 'user_identity', got %q", setupCall.Args[0].Name)
		}
		if setupCall.Args[0].Value != "alice@company.com" {
			t.Errorf("expected user identity value 'alice@company.com', got %v", setupCall.Args[0].Value)
		}

		// Verify query call on the same conn (AC-1.3.1)
		queryCall := calls[1]
		if queryCall.Type != "Query" {
			t.Errorf("expected second call to be Query, got %q", queryCall.Type)
		}
		if queryCall.Query != "SELECT ID FROM USERS WHERE STATUS = :status" {
			t.Errorf("unexpected query statement: %q", queryCall.Query)
		}
		if len(queryCall.Args) != 1 {
			t.Fatalf("expected 1 arg in query call, got %d", len(queryCall.Args))
		}
		if queryCall.Args[0].Value != "active" {
			t.Errorf("expected query param value 'active', got %v", queryCall.Args[0].Value)
		}

		// AC-1.3.3: statements execute directly on conn without enclosing in *sql.Tx
		for _, c := range calls {
			if c.Type == "Begin" || c.Type == "BeginTx" {
				t.Errorf("unexpected transaction call %q: statements must execute in autocommit mode without *sql.Tx", c.Type)
			}
		}

		// AC-1.3.4: conn.Close() guaranteed to execute in defer
		if !conn.isClosed() {
			t.Errorf("expected dedicated connection to be closed via defer")
		}
	})

	t.Run("AC-1.3.1, AC-1.3.3: dedicated connection executes DML in autocommit mode", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-dml-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"email": "bob@company.com",
		})

		res, err := src.RunSQL(ctx, "UPDATE USERS SET STATUS = 'inactive' WHERE ID = :id", []any{100}, false)
		if err != nil {
			t.Fatalf("unexpected RunSQL DML error: %v", err)
		}

		resMap, ok := res.(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any, got %T", res)
		}
		if resMap["status"] != "success" || resMap["rows_affected"] != int64(1) {
			t.Fatalf("unexpected DML result map: %v", resMap)
		}

		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected exactly 1 connection, got %d", len(conns))
		}
		conn := conns[0]
		calls := conn.getCalls()

		if len(calls) < 2 {
			t.Fatalf("expected at least 2 calls on conn, got %d", len(calls))
		}
		// Verify setup call
		if calls[0].Type != "Exec" || calls[0].Query != src.SessionContextBlock {
			t.Errorf("unexpected setup call: %v", calls[0])
		}
		// Verify DML call on the same connection
		dmlCall := calls[1]
		if dmlCall.Type != "Exec" || dmlCall.Query != "UPDATE USERS SET STATUS = 'inactive' WHERE ID = :id" {
			t.Errorf("unexpected DML call on conn: %v", dmlCall)
		}
		if len(dmlCall.Args) != 1 {
			t.Fatalf("expected 1 arg in DML call, got %d", len(dmlCall.Args))
		}
		if fmt.Sprintf("%v", dmlCall.Args[0].Value) != "100" {
			t.Errorf("expected DML param value '100', got %v", dmlCall.Args[0].Value)
		}

		// AC-1.3.3: no transaction
		for _, c := range calls {
			if c.Type == "Begin" || c.Type == "BeginTx" {
				t.Errorf("unexpected transaction call %q", c.Type)
			}
		}

		// AC-1.3.4: closed
		if !conn.isClosed() {
			t.Errorf("expected conn to be closed via defer")
		}
	})

	t.Run("AC-1.3.6: session reset block executes on conn immediately prior to conn.Close()", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-reset-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionResetBlock:   "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"email": "charlie@company.com",
		})

		_, err := src.RunSQL(ctx, "SELECT ID FROM USERS", nil, true)
		if err != nil {
			t.Fatalf("unexpected RunSQL error: %v", err)
		}

		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		conn := conns[0]
		calls := conn.getCalls()

		// Expected call sequence on conn:
		// [0] Exec(SessionContextBlock)
		// [1] Query(statement)
		// [2] Exec(SessionResetBlock)
		// [3] Close
		if len(calls) != 4 {
			t.Fatalf("expected exactly 4 calls on conn (setup, query, reset, close), got %d: %v", len(calls), calls)
		}

		if calls[0].Type != "Exec" || calls[0].Query != src.SessionContextBlock {
			t.Errorf("call 0: expected setup Exec, got %v", calls[0])
		}
		if calls[1].Type != "Query" || calls[1].Query != "SELECT ID FROM USERS" {
			t.Errorf("call 1: expected query, got %v", calls[1])
		}
		// AC-1.3.6: s.SessionResetBlock executes on conn immediately prior to conn.Close()
		if calls[2].Type != "Exec" || calls[2].Query != src.SessionResetBlock {
			t.Errorf("call 2: expected reset Exec %q, got %v", src.SessionResetBlock, calls[2])
		}
		if calls[3].Type != "Close" {
			t.Errorf("call 3: expected Close, got %v", calls[3])
		}
		if !conn.isClosed() {
			t.Errorf("expected conn to be closed")
		}
	})

	t.Run("AC-1.3.6: session reset block error is ignored in defer and query result is preserved", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-reset-err-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionResetBlock:   "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		// Inject failure only into the reset block execution
		state.onExec = func(ctx context.Context, connID int, query string, args []driver.NamedValue) (driver.Result, error) {
			if query == src.SessionResetBlock {
				return nil, errors.New("ORA-00001: simulated reset block failure")
			}
			return mockResult{rowsAffected: 1}, nil
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"email": "david@company.com",
		})

		res, err := src.RunSQL(ctx, "SELECT ID FROM USERS", nil, true)
		// Reset errors must be ignored in defer; query result preserved
		if err != nil {
			t.Fatalf("expected query result to be preserved despite reset block failure, got err: %v", err)
		}
		rows, ok := res.([]any)
		if !ok || len(rows) != 1 {
			t.Fatalf("expected query output rows, got %v", res)
		}

		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		conn := conns[0]
		if !conn.isClosed() {
			t.Errorf("expected conn to be closed despite reset block error")
		}
	})

	t.Run("AC-1.3.4: conn.Close() guaranteed in defer when session setup block fails", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-setup-err-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN INVALID_SETUP; END;",
				SessionResetBlock:   "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		state.onExec = func(ctx context.Context, connID int, query string, args []driver.NamedValue) (driver.Result, error) {
			if query == src.SessionContextBlock {
				return nil, errors.New("ORA-00900: invalid SQL statement")
			}
			return mockResult{rowsAffected: 1}, nil
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"email": "eve@company.com",
		})

		_, err := src.RunSQL(ctx, "SELECT 1 FROM DUAL", nil, true)
		if err == nil {
			t.Fatal("expected error on setup failure, got nil")
		}
		expectedErrSubstr := "failed to execute session context setup: ORA-00900: invalid SQL statement"
		if !strings.Contains(err.Error(), expectedErrSubstr) {
			t.Errorf("expected error %q to contain %q", err.Error(), expectedErrSubstr)
		}

		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		conn := conns[0]
		// AC-1.3.4: conn.Close() guaranteed to execute in defer
		if !conn.isClosed() {
			t.Errorf("expected connection to be closed via defer when setup fails")
		}
	})

	t.Run("AC-1.3.4: conn.Close() guaranteed in defer when query execution fails", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-query-err-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionResetBlock:   "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		state.onQuery = func(ctx context.Context, connID int, query string, args []driver.NamedValue) (driver.Rows, error) {
			return nil, errors.New("ORA-00942: table or view does not exist")
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"email": "frank@company.com",
		})

		_, err := src.RunSQL(ctx, "SELECT * FROM NONEXISTENT_TABLE", nil, true)
		if err == nil {
			t.Fatal("expected error on query failure, got nil")
		}
		if !strings.Contains(err.Error(), "unable to execute query: ORA-00942: table or view does not exist") {
			t.Errorf("unexpected error: %v", err)
		}

		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		conn := conns[0]
		calls := conn.getCalls()
		// Expected call sequence on conn: setup, failed query, reset, close
		if len(calls) != 4 {
			t.Fatalf("expected 4 calls on conn (setup, failed query, reset, close), got %d: %v", len(calls), calls)
		}
		if calls[0].Type != "Exec" || calls[0].Query != src.SessionContextBlock {
			t.Errorf("expected setup Exec, got %v", calls[0])
		}
		if calls[1].Type != "Query" || calls[1].Query != "SELECT * FROM NONEXISTENT_TABLE" {
			t.Errorf("expected query call, got %v", calls[1])
		}
		if calls[2].Type != "Exec" || calls[2].Query != src.SessionResetBlock {
			t.Errorf("expected session reset block Exec before Close, got %v", calls[2])
		}
		if calls[3].Type != "Close" {
			t.Errorf("expected Close call, got %v", calls[3])
		}
		// AC-1.3.4: conn.Close() guaranteed
		if !conn.isClosed() {
			t.Errorf("expected connection to be closed via defer when query fails")
		}
	})

	t.Run("AC-1.3.4: conn.Close() guaranteed in defer when panic occurs during execution", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-panic-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionResetBlock:   "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		state.onQuery = func(ctx context.Context, connID int, query string, args []driver.NamedValue) (driver.Rows, error) {
			return &mockPanicRows{}, nil
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"email": "grace@company.com",
		})

		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected panic from query execution")
				}
			}()
			_, _ = src.RunSQL(ctx, "SELECT 1 FROM DUAL", nil, true)
		}()

		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		conn := conns[0]
		// AC-1.3.4: conn.Close() guaranteed even across panic unwind
		if !conn.isClosed() {
			t.Errorf("expected connection to be closed via defer across panic unwind")
		}
	})

	t.Run("dedicated connection acquisition failure returns wrapped error", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-acquire-fail-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		state.onOpen = func() error {
			return errors.New("connection pool exhausted")
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"email": "heidi@company.com",
		})

		_, err := src.RunSQL(ctx, "SELECT 1 FROM DUAL", nil, true)
		if err == nil {
			t.Fatal("expected acquisition failure error, got nil")
		}
		expectedErrSubstr := "unable to acquire dedicated Oracle connection: connection pool exhausted"
		if !strings.Contains(err.Error(), expectedErrSubstr) {
			t.Errorf("expected error %q to contain %q", err.Error(), expectedErrSubstr)
		}
	})

	t.Run("AC-1.3.5: client context cancellation propagates immediately to conn.QueryContext", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-cancel-query-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionResetBlock:   "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		ctx, cancel := context.WithCancel(context.Background())
		ctx = util.WithAuthTokenClaims(ctx, map[string]any{
			"email": "ian@company.com",
		})

		var resetExecuted bool
		var resetContextErr error
		state.onExec = func(ctx context.Context, connID int, query string, args []driver.NamedValue) (driver.Result, error) {
			if query == src.SessionResetBlock {
				resetExecuted = true
				resetContextErr = ctx.Err()
			}
			return mockResult{rowsAffected: 1}, nil
		}

		state.onQuery = func(ctx context.Context, connID int, query string, args []driver.NamedValue) (driver.Rows, error) {
			cancel() // cancel client request during query execution
			return nil, ctx.Err()
		}

		_, err := src.RunSQL(ctx, "SELECT * FROM LARGE_TABLE", nil, true)
		if err == nil {
			t.Fatal("expected cancellation error, got nil")
		}
		// AC-1.3.5: context cancellation propagates immediately
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected errors.Is(err, context.Canceled), got: %v", err)
		}

		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		conn := conns[0]
		calls := conn.getCalls()
		// Expected call sequence on conn: setup, cancelled query, reset (with detached context), close
		if len(calls) != 4 {
			t.Fatalf("expected 4 calls on conn (setup, query, reset, close), got %d: %v", len(calls), calls)
		}
		if calls[0].Type != "Exec" || calls[0].Query != src.SessionContextBlock {
			t.Errorf("expected setup call, got %v", calls[0])
		}
		if calls[1].Type != "Query" || calls[1].Query != "SELECT * FROM LARGE_TABLE" {
			t.Errorf("expected query call, got %v", calls[1])
		}
		if calls[2].Type != "Exec" || calls[2].Query != src.SessionResetBlock {
			t.Errorf("expected reset block call before close, got %v", calls[2])
		}
		if calls[3].Type != "Close" {
			t.Errorf("expected Close call, got %v", calls[3])
		}
		if !resetExecuted {
			t.Errorf("expected session reset block to execute despite client context cancellation")
		}
		if resetContextErr != nil {
			t.Errorf("expected detached context for reset block to have nil Err, got %v", resetContextErr)
		}
		if !conn.isClosed() {
			t.Errorf("expected conn to be closed after cancellation")
		}
	})

	t.Run("AC-1.3.5: client context cancellation propagates immediately to conn.ExecContext during setup", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "vpd-cancel-setup-source",
				Type:                SourceType,
				SessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;",
				SessionContextClaim: "email",
			},
			DB: db,
		}

		ctx, cancel := context.WithCancel(context.Background())
		ctx = util.WithAuthTokenClaims(ctx, map[string]any{
			"email": "judy@company.com",
		})

		state.onExec = func(ctx context.Context, connID int, query string, args []driver.NamedValue) (driver.Result, error) {
			if query == src.SessionContextBlock {
				cancel() // cancel during setup block execution
				return nil, ctx.Err()
			}
			return mockResult{rowsAffected: 1}, nil
		}

		_, err := src.RunSQL(ctx, "SELECT 1 FROM DUAL", nil, true)
		if err == nil {
			t.Fatal("expected cancellation error during setup, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected errors.Is(err, context.Canceled), got: %v", err)
		}

		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		conn := conns[0]
		if !conn.isClosed() {
			t.Errorf("expected conn to be closed after setup cancellation")
		}
	})

	t.Run("unconfigured session context block executes directly on pool fallback", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "unconfigured-source",
				Type:                SourceType,
				SessionContextBlock: "",
			},
			DB: db,
		}

		ctx := context.Background()
		res, err := src.RunSQL(ctx, "SELECT ID FROM USERS", nil, true)
		if err != nil {
			t.Fatalf("unexpected RunSQL error: %v", err)
		}

		rows, ok := res.([]any)
		if !ok || len(rows) != 1 {
			t.Fatalf("expected 1 row result, got %v", res)
		}

		// Ensure no setup block was executed
		conns := state.getConns()
		for _, c := range conns {
			for _, call := range c.getCalls() {
				if strings.Contains(call.Query, "SET_IDENTIFIER") {
					t.Errorf("setup block should not be executed when SessionContextBlock is empty: %v", call)
				}
			}
		}
	})
}

func TestOracleVPDLifecycleEndToEnd(t *testing.T) {
	// AC-1.4.1, AC-1.4.2, AC-1.4.4: full VPD lifecycle from YAML parsing to teardown with race detection

	// 1. Configuration parsing from YAML (AC-1.4.1)
	yamlConfig := `
kind: source
name: e2e-vpd-oracle
type: oracle
connectionString: "dbhost:1521/XEPDB1"
user: test_db_user
password: test_db_password
disablePooling: true
sessionContextBlock: "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;"
sessionContextClaim: "caller_identity"
sessionResetBlock: "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;"
`
	parsedConfigs, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(
		context.Background(),
		testutils.FormatYaml(yamlConfig),
	)
	if err != nil {
		t.Fatalf("failed to unmarshal YAML configuration: %v", err)
	}

	rawCfg, ok := parsedConfigs["e2e-vpd-oracle"]
	if !ok {
		t.Fatalf("expected config for 'e2e-vpd-oracle' in parsed configs")
	}
	cfg, ok := rawCfg.(Config)
	if !ok {
		t.Fatalf("expected Config type, got %T", rawCfg)
	}

	// Verify parsed fields (AC-1.4.1)
	if !cfg.DisablePooling {
		t.Errorf("expected DisablePooling to be true")
	}
	if cfg.SessionContextBlock != "BEGIN DBMS_SESSION.SET_IDENTIFIER(:user_identity); END;" {
		t.Errorf("unexpected SessionContextBlock: %q", cfg.SessionContextBlock)
	}
	if cfg.SessionContextClaim != "caller_identity" {
		t.Errorf("unexpected SessionContextClaim: %q", cfg.SessionContextClaim)
	}
	if cfg.SessionResetBlock != "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;" {
		t.Errorf("unexpected SessionResetBlock: %q", cfg.SessionResetBlock)
	}

	// 2. Unpooled options initialization sets zero idle connections (AC-1.4.1)
	var poolLimitsCalled bool
	var maxIdleSet int
	var lifetimeSet time.Duration

	origSetPoolLimits := setPoolLimits
	setPoolLimits = func(db *sql.DB, maxIdleConns int, connMaxLifetime time.Duration) {
		poolLimitsCalled = true
		maxIdleSet = maxIdleConns
		lifetimeSet = connMaxLifetime
		origSetPoolLimits(db, maxIdleConns, connMaxLifetime)
	}
	defer func() { setPoolLimits = origSetPoolLimits }()

	origPingDB := pingDB
	pingDB = func(ctx context.Context, db *sql.DB) error {
		return nil
	}
	defer func() { pingDB = origPingDB }()

	ctxWithLogger, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("failed to create context with logger: %v", err)
	}
	tracer := noop.NewTracerProvider().Tracer("oracle-e2e-test")

	initSourceRaw, err := cfg.Initialize(ctxWithLogger, tracer)
	// Restore package globals immediately after Initialize completes
	setPoolLimits = origSetPoolLimits
	pingDB = origPingDB

	if err != nil {
		t.Fatalf("cfg.Initialize failed: %v", err)
	}
	initSource, ok := initSourceRaw.(*Source)
	if !ok {
		t.Fatalf("expected *Source from Initialize, got %T", initSourceRaw)
	}
	_ = initSource.OracleDB().Close()

	if !poolLimitsCalled {
		t.Errorf("expected setPoolLimits to be called during unpooled initialization")
	}
	if maxIdleSet != 0 {
		t.Errorf("expected maxIdleConns = 0, got %d", maxIdleSet)
	}
	if lifetimeSet != 0 {
		t.Errorf("expected connMaxLifetime = 0, got %v", lifetimeSet)
	}

	// 3. Mock database driver setup for execution pipeline tests
	db, state, cleanup := newMockDB(t)
	defer cleanup()

	src := &Source{
		Config: cfg,
		DB:     db,
	}

	// 4. Optional authorization verification: missing or invalid claims do not return HTTP 401
	optionalAuthCases := []struct {
		desc   string
		claims map[string]any
	}{
		{desc: "nil context claims", claims: nil},
		{desc: "empty claims map", claims: map[string]any{}},
		{desc: "missing target claim", claims: map[string]any{"other_claim": "user@example.com"}},
		{desc: "whitespace target claim", claims: map[string]any{"caller_identity": "   "}},
		{desc: "non-scalar target claim", claims: map[string]any{"caller_identity": []string{"invalid"}}},
	}

	for _, tc := range optionalAuthCases {
		reqCtx := context.Background()
		if tc.claims != nil {
			reqCtx = util.WithAuthTokenClaims(reqCtx, tc.claims)
		}

		// Read query succeeds
		_, err := src.RunSQL(reqCtx, "SELECT 1 FROM DUAL", nil, true)
		if err != nil {
			t.Fatalf("[%s] expected RunSQL read to succeed with optional auth, got %v", tc.desc, err)
		}

		// Write DML statement also succeeds
		_, err = src.RunSQL(reqCtx, "UPDATE USERS SET ACTIVE = 1", nil, false)
		if err != nil {
			t.Fatalf("[%s] expected RunSQL write to succeed with optional auth, got %v", tc.desc, err)
		}
	}

	connsBefore := len(state.getConns())

	// 5. Valid claim executes sessionContextBlock with caller identity prior to query
	validUser := "enterprise-analyst-007"
	validCtx := util.WithAuthTokenClaims(context.Background(), map[string]any{
		"caller_identity": validUser,
	})

	queryResult, err := src.RunSQL(validCtx, "SELECT ID FROM SENSITIVE_TABLE WHERE ROWNUM = 1", nil, true)
	if err != nil {
		t.Fatalf("RunSQL with valid claims failed: %v", err)
	}
	rows, ok := queryResult.([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("expected 1 row result, got %v", queryResult)
	}

	conns := state.getConns()
	if len(conns) != connsBefore+1 {
		t.Fatalf("expected 1 dedicated connection for query, got %d", len(conns)-connsBefore)
	}
	conn := conns[connsBefore]
	calls := conn.getCalls()

	// Full lifecycle call sequence:
	// [0] Exec(sessionContextBlock) with sql.Named("user_identity", validUser)
	// [1] Query(statement)
	// [2] Exec(sessionResetBlock)
	// [3] Close()
	if len(calls) != 4 {
		t.Fatalf("expected 4 calls on dedicated connection (setup, query, reset, close), got %d: %v", len(calls), calls)
	}

	// Verify setup call (AC-1.4.2)
	if calls[0].Type != "Exec" || calls[0].Query != cfg.SessionContextBlock {
		t.Errorf("call 0: expected Exec of session context block %q, got: %v", cfg.SessionContextBlock, calls[0])
	}
	if len(calls[0].Args) != 1 || calls[0].Args[0].Name != "user_identity" || calls[0].Args[0].Value != validUser {
		t.Errorf("call 0: expected :user_identity = %q, got: %v", validUser, calls[0].Args)
	}

	// Verify query call
	if calls[1].Type != "Query" || calls[1].Query != "SELECT ID FROM SENSITIVE_TABLE WHERE ROWNUM = 1" {
		t.Errorf("call 1: expected query statement, got: %v", calls[1])
	}

	// Verify reset call
	if calls[2].Type != "Exec" || calls[2].Query != cfg.SessionResetBlock {
		t.Errorf("call 2: expected Exec of session reset block %q, got: %v", cfg.SessionResetBlock, calls[2])
	}

	// Verify close call and socket termination
	if calls[3].Type != "Close" {
		t.Errorf("call 3: expected Close, got: %v", calls[3])
	}
	if !conn.isClosed() {
		t.Errorf("expected connection to be marked closed")
	}

	// 6. DML write path verification
	dmlResult, err := src.RunSQL(validCtx, "UPDATE SENSITIVE_TABLE SET ACCESSED = 1", nil, false)
	if err != nil {
		t.Fatalf("RunSQL DML write failed: %v", err)
	}
	dmlMap, ok := dmlResult.(map[string]any)
	if !ok || dmlMap["status"] != "success" || dmlMap["rows_affected"] != int64(1) {
		t.Fatalf("unexpected DML result map: %v", dmlResult)
	}

	conns = state.getConns()
	if len(conns) != connsBefore+2 {
		t.Fatalf("expected 2 total connections (1 per statement), got %d", len(conns)-connsBefore)
	}
	dmlConn := conns[connsBefore+1]
	dmlCalls := dmlConn.getCalls()
	if len(dmlCalls) != 4 {
		t.Fatalf("expected 4 calls on DML connection, got %d: %v", len(dmlCalls), dmlCalls)
	}
	if dmlCalls[0].Type != "Exec" || dmlCalls[0].Query != cfg.SessionContextBlock {
		t.Errorf("DML call 0: expected setup Exec, got: %v", dmlCalls[0])
	}
	if len(dmlCalls[0].Args) != 1 || dmlCalls[0].Args[0].Name != "user_identity" || dmlCalls[0].Args[0].Value != validUser {
		t.Errorf("DML call 0: expected :user_identity = %q, got: %v", validUser, dmlCalls[0].Args)
	}
	if dmlCalls[1].Type != "Exec" || dmlCalls[1].Query != "UPDATE SENSITIVE_TABLE SET ACCESSED = 1" {
		t.Errorf("DML call 1: expected DML Exec, got: %v", dmlCalls[1])
	}
	if dmlCalls[2].Type != "Exec" || dmlCalls[2].Query != cfg.SessionResetBlock {
		t.Errorf("DML call 2: expected reset Exec, got: %v", dmlCalls[2])
	}
	if dmlCalls[3].Type != "Close" {
		t.Errorf("DML call 3: expected Close, got: %v", dmlCalls[3])
	}
	if !dmlConn.isClosed() {
		t.Errorf("expected DML connection to be marked closed")
	}

	// 7. Concurrent multi-user execution with race detection (AC-1.4.4)
	const numConcurrent = 20
	var wg sync.WaitGroup
	errs := make(chan error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(userNum int) {
			defer wg.Done()
			userClaim := fmt.Sprintf("tenant-user-%03d", userNum)
			userCtx := util.WithAuthTokenClaims(context.Background(), map[string]any{
				"caller_identity": userClaim,
			})

			res, err := src.RunSQL(userCtx, "SELECT ID FROM USERS WHERE USERNAME = :1", []any{userClaim}, true)
			if err != nil {
				errs <- fmt.Errorf("concurrent query failed for %s: %w", userClaim, err)
				return
			}
			rows, ok := res.([]any)
			if !ok || len(rows) != 1 {
				errs <- fmt.Errorf("unexpected rows for %s: %v", userClaim, res)
				return
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrency error: %v", err)
	}

	// Verify all concurrent connections executed full lifecycle and closed
	conns = state.getConns()
	if len(conns) != connsBefore+2+numConcurrent {
		t.Fatalf("expected %d total connections, got %d", connsBefore+2+numConcurrent, len(conns))
	}
	for idx, c := range conns[connsBefore+2:] {
		if !c.isClosed() {
			t.Errorf("concurrent connection %d was not closed", idx)
		}
		cCalls := c.getCalls()
		if len(cCalls) != 4 {
			t.Errorf("concurrent connection %d has %d calls, want 4", idx, len(cCalls))
			continue
		}
		if cCalls[0].Type != "Exec" || cCalls[0].Query != cfg.SessionContextBlock {
			t.Errorf("concurrent connection %d setup call mismatch: %v", idx, cCalls[0])
		}
		if len(cCalls[0].Args) == 0 || cCalls[0].Args[0].Name != "user_identity" {
			t.Errorf("concurrent connection %d missing or invalid user_identity argument: %v", idx, cCalls[0].Args)
			continue
		}
		if cCalls[1].Type != "Query" || cCalls[1].Query != "SELECT ID FROM USERS WHERE USERNAME = :1" {
			t.Errorf("concurrent connection %d query call mismatch: %v", idx, cCalls[1])
		}
		if len(cCalls[1].Args) == 0 {
			t.Errorf("concurrent connection %d missing query arguments: %v", idx, cCalls[1].Args)
			continue
		}
		// Confirm identity bound matches the query argument executed on this connection
		setupIdentity := cCalls[0].Args[0].Value
		queryArg := cCalls[1].Args[0].Value
		if setupIdentity != queryArg {
			t.Errorf("connection affinity violation: setup identity %v != query arg %v", setupIdentity, queryArg)
		}
		if cCalls[2].Type != "Exec" || cCalls[2].Query != cfg.SessionResetBlock {
			t.Errorf("concurrent connection %d reset call mismatch: %v", idx, cCalls[2])
		}
		if cCalls[3].Type != "Close" {
			t.Errorf("concurrent connection %d close call mismatch: %v", idx, cCalls[3])
		}
	}
}

func TestExtractBindParameterNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name: "PeopleSoft PL/SQL block with := assignment and :user_id",
			input: `declare 
            l_ret varchar2(100) ;
            BEGIN
            l_ret := sysadm.ps_security_pkg1.initialize_session(:user_id);
            end;`,
			expected: []string{"user_id"},
		},
		{
			name:     "single-line and multi-line comments and string literals with colons",
			input:    "/* :ignore_block */ -- :ignore_line\nSELECT :real_param, ':literal_param' FROM DUAL",
			expected: []string{"real_param"},
		},
		{
			name:     "multiple distinct parameters with deduplication",
			input:    "BEGIN pkg.setup(:user_id, :tenant_id, :user_id); END;",
			expected: []string{"user_id", "tenant_id"},
		},
		{
			name:     "no bind variables (static DDL/DCL/ALTER)",
			input:    "ALTER SESSION SET CURRENT_SCHEMA = HR",
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractBindParameterNames(tc.input)
			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("extractBindParameterNames mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRunSQLToolParameterBindings(t *testing.T) {
	t.Parallel()

	t.Run("binds tool parameter user_id into PeopleSoft session block", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		sessionBlock := `declare 
            l_ret varchar2(100) ;
            BEGIN
            l_ret := sysadm.ps_security_pkg1.initialize_session(:user_id);
            end;`

		src := &Source{
			Config: Config{
				Name:                "peoplesoft-source",
				Type:                SourceType,
				SessionContextBlock: sessionBlock,
				SessionResetBlock:   "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;",
			},
			DB: db,
		}

		ctx := WithToolParams(context.Background(), map[string]any{
			"user_id": "SYSADM_AI",
		})

		_, err := src.RunSQL(ctx, "SELECT count(*) FROM PSOPRDEFN", nil, true)
		if err != nil {
			t.Fatalf("RunSQL failed: %v", err)
		}

		conns := state.getConns()
		if len(conns) != 1 {
			t.Fatalf("expected 1 connection, got %d", len(conns))
		}
		calls := conns[0].getCalls()
		if len(calls) < 4 {
			t.Fatalf("expected at least 4 calls, got %d", len(calls))
		}

		// Verify session block was called with :user_id = SYSADM_AI
		if calls[0].Type != "Exec" || calls[0].Query != sessionBlock {
			t.Errorf("expected Exec of session block, got %v", calls[0])
		}
		if len(calls[0].Args) != 1 || calls[0].Args[0].Name != "user_id" || calls[0].Args[0].Value != "SYSADM_AI" {
			t.Errorf("expected setup Arg Named(user_id, SYSADM_AI), got %#v", calls[0].Args)
		}

		// Verify reset block executed with 0 arguments (since it has no bind parameters)
		if calls[2].Type != "Exec" || calls[2].Query != src.SessionResetBlock {
			t.Errorf("expected Exec of reset block, got %v", calls[2])
		}
		if len(calls[2].Args) != 0 {
			t.Errorf("expected 0 args for reset block, got %v", calls[2].Args)
		}
	})

	t.Run("static session initialization block without bind variables executes with zero args", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		src := &Source{
			Config: Config{
				Name:                "static-session-source",
				Type:                SourceType,
				SessionContextBlock: "ALTER SESSION SET CURRENT_SCHEMA = HR",
			},
			DB: db,
		}

		ctx := context.Background()
		_, err := src.RunSQL(ctx, "SELECT count(*) FROM EMPLOYEES", nil, true)
		if err != nil {
			t.Fatalf("RunSQL failed: %v", err)
		}

		conns := state.getConns()
		calls := conns[0].getCalls()
		if calls[0].Type != "Exec" || calls[0].Query != "ALTER SESSION SET CURRENT_SCHEMA = HR" {
			t.Errorf("expected Exec of ALTER SESSION, got %v", calls[0])
		}
		if len(calls[0].Args) != 0 {
			t.Errorf("expected 0 args for ALTER SESSION, got %v", calls[0].Args)
		}
	})

	t.Run("session block with both user_id and user_identity bound from claims when tool param absent", func(t *testing.T) {
		t.Parallel()
		db, state, cleanup := newMockDB(t)
		defer cleanup()

		sessionBlock := "BEGIN pkg.set_caller(:user_id, :user_identity); END;"

		src := &Source{
			Config: Config{
				Name:                "vpd-source",
				Type:                SourceType,
				SessionContextBlock: sessionBlock,
				SessionContextClaim: "email",
			},
			DB: db,
		}

		ctx := util.WithAuthTokenClaims(context.Background(), map[string]any{
			"email": "user@example.com",
		})

		_, err := src.RunSQL(ctx, "SELECT 1 FROM DUAL", nil, true)
		if err != nil {
			t.Fatalf("RunSQL failed: %v", err)
		}

		conns := state.getConns()
		calls := conns[0].getCalls()
		if len(calls[0].Args) != 2 {
			t.Fatalf("expected 2 args, got %d: %v", len(calls[0].Args), calls[0].Args)
		}
		for _, arg := range calls[0].Args {
			if arg.Value != "user@example.com" {
				t.Errorf("expected arg %s value to be user@example.com, got %v", arg.Name, arg.Value)
			}
		}
	})
}

func TestSessionContextBlockExecutedForEveryTool(t *testing.T) {
	t.Parallel()
	db, state, cleanup := newMockDB(t)
	defer cleanup()

	sessionBlock := `
declare 
  l_ret varchar2(100);
begin
  l_ret := sysadm.ps_security_pkg1.initialize_session(:user_id);
end;`
	resetBlock := "BEGIN DBMS_SESSION.CLEAR_IDENTIFIER; END;"

	src := &Source{
		Config: Config{
			Name:                "ps-oracle-source",
			Type:                SourceType,
			SessionContextBlock: sessionBlock,
			SessionResetBlock:   resetBlock,
		},
		DB: db,
	}

	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("failed to create context with logger: %v", err)
	}

	type testLifecycleTool struct {
		name         string
		query        string
		params       map[string]any
		readOnly     bool
		wantUserID   any
		wantQuerySub string
	}
	testCases := []testLifecycleTool{
		{
			name:         "list_roles tool",
			query:        "SELECT distinct RU.ROLENAME FROM PSOPRDEFN O, PSROLEUSER RU WHERE O.OPRID = :1 AND O.OPRID = RU.ROLEUSER",
			params:       map[string]any{"user_id": "SYSADM_AI"},
			readOnly:     true,
			wantUserID:   "SYSADM_AI",
			wantQuerySub: "SELECT distinct RU.ROLENAME",
		},
		{
			name:         "list_tables tool without user_id param",
			query:        "SELECT table_name FROM user_tables",
			params:       map[string]any{},
			readOnly:     true,
			wantUserID:   "",
			wantQuerySub: "SELECT table_name FROM user_tables",
		},
		{
			name:         "execute_sql tool with DML and user_id",
			query:        "UPDATE PS_JOB SET STATUS = 'A' WHERE EMPLID = '123'",
			params:       map[string]any{"user_id": "SYSADM_HR"},
			readOnly:     false,
			wantUserID:   "SYSADM_HR",
			wantQuerySub: "UPDATE PS_JOB SET STATUS = 'A'",
		},
		{
			name:         "get_employee tool with multiple params",
			query:        "SELECT * FROM PS_PERSONAL_DATA WHERE EMPLID = :1",
			params:       map[string]any{"emplid": "E100", "user_id": "SYSADM_SECURITY"},
			readOnly:     true,
			wantUserID:   "SYSADM_SECURITY",
			wantQuerySub: "SELECT * FROM PS_PERSONAL_DATA",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			connsBefore := len(state.getConns())

			runCtx := WithToolParams(ctx, tc.params)
			_, runErr := src.RunSQL(runCtx, tc.query, nil, tc.readOnly)
			if runErr != nil {
				t.Fatalf("tool %s RunSQL failed: %v", tc.name, runErr)
			}

			conns := state.getConns()
			if len(conns) != connsBefore+1 {
				t.Fatalf("expected 1 new dedicated connection for tool %s, got %d (total %d)",
					tc.name, len(conns)-connsBefore, len(conns))
			}

			conn := conns[i]
			calls := conn.getCalls()
			if len(calls) < 3 {
				t.Fatalf("expected at least 3 calls on connection for tool %s, got %d", tc.name, len(calls))
			}

			// Step 1: Session context block must ALWAYS be executed FIRST on this connection
			if calls[0].Type != "Exec" || calls[0].Query != strings.TrimSpace(sessionBlock) {
				t.Errorf("tool %s: expected call 0 to be Exec of sessionContextBlock, got %v", tc.name, calls[0])
			}
			if len(calls[0].Args) != 1 || calls[0].Args[0].Name != "user_id" || calls[0].Args[0].Value != tc.wantUserID {
				t.Errorf("tool %s: expected sessionContextBlock arg Named(user_id, %v), got %#v",
					tc.name, tc.wantUserID, calls[0].Args)
			}

			// Step 2: The tool's query must be executed SECOND on this connection
			if !strings.Contains(calls[1].Query, tc.wantQuerySub) {
				t.Errorf("tool %s: expected call 1 query to contain %q, got %q",
					tc.name, tc.wantQuerySub, calls[1].Query)
			}

			// Step 3: Session reset block must ALWAYS be executed on teardown
			var resetCallFound bool
			for _, call := range calls[2:] {
				if call.Type == "Exec" && call.Query == resetBlock {
					resetCallFound = true
					break
				}
			}
			if !resetCallFound {
				t.Errorf("tool %s: expected Exec of sessionResetBlock in teardown calls, got %v", tc.name, calls)
			}

			// Step 4: The dedicated connection must be closed
			if !conn.isClosed() {
				t.Errorf("tool %s: dedicated connection was not closed after execution", tc.name)
			}
		})
	}
}

func TestOracleContextHelpersAndParseJWTClaims(t *testing.T) {
	ctx := context.Background()

	// Tool params
	params := map[string]any{"user_id": "test_user"}
	ctxWithParams := WithToolParams(ctx, params)
	retrievedParams := ToolParamsFromContext(ctxWithParams)
	if retrievedParams == nil || retrievedParams["user_id"] != "test_user" {
		t.Errorf("expected toolParams with test_user, got: %v", retrievedParams)
	}

	// Auth claims
	claims := map[string]any{"email": "user@example.com"}
	ctxWithClaims := WithAuthClaims(ctx, claims)
	retrievedClaims := AuthClaimsFromContext(ctxWithClaims)
	if retrievedClaims == nil || retrievedClaims["email"] != "user@example.com" {
		t.Errorf("expected authClaims with user@example.com, got: %v", retrievedClaims)
	}

	// ParseJWTClaims
	// Payload: {"email":"jwt_user@example.com","sub":"12345"}
	// Base64URL without padding: eyJlbWFpbCI6Imp3dF91c2VyQGV4YW1wbGUuY29tIiwic3ViIjoiMTIzNDUifQ
	fakeJWT := "header.eyJlbWFpbCI6Imp3dF91c2VyQGV4YW1wbGUuY29tIiwic3ViIjoiMTIzNDUifQ.signature"
	parsed := ParseJWTClaims(fakeJWT)
	if parsed == nil || parsed["email"] != "jwt_user@example.com" || parsed["sub"] != "12345" {
		t.Errorf("expected parsed claims with jwt_user@example.com, got: %v", parsed)
	}

	// Invalid JWT returns nil
	if ParseJWTClaims("invalid-token") != nil {
		t.Errorf("expected nil for invalid token")
	}
}
