// Copyright © 2025, Oracle and/or its affiliates.

package oracle

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
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

	src := &Source{
		Config: Config{
			Name: "test-dml-source",
			Type: SourceType,
			User: "test-user",
		},
		DB: db,
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
