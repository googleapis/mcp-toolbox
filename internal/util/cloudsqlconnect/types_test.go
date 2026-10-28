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

package cloudsqlconnect

import (
	"strings"
	"testing"
)

func TestIsValidLanguage(t *testing.T) {
	tcs := []struct {
		desc  string
		input string
		want  bool
	}{
		{
			desc:  "valid python lowercase",
			input: "python",
			want:  true,
		},
		{
			desc:  "valid python uppercase",
			input: "PYTHON",
			want:  true,
		},
		{
			desc:  "valid python mixed case",
			input: "Python",
			want:  true,
		},
		{
			desc:  "valid nodejs",
			input: "nodejs",
			want:  true,
		},
		{
			desc:  "valid java",
			input: "java",
			want:  true,
		},
		{
			desc:  "valid go",
			input: "go",
			want:  true,
		},
		{
			desc:  "invalid language ruby",
			input: "ruby",
			want:  false,
		},
		{
			desc:  "invalid language rust",
			input: "rust",
			want:  false,
		},
		{
			desc:  "empty string",
			input: "",
			want:  false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got := IsValidLanguage(tc.input)
			if got != tc.want {
				t.Errorf("IsValidLanguage(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestValidateLanguage(t *testing.T) {
	tcs := []struct {
		desc    string
		input   string
		wantErr bool
	}{
		{
			desc:    "empty string is valid",
			input:   "",
			wantErr: false,
		},
		{
			desc:    "valid python",
			input:   "python",
			wantErr: false,
		},
		{
			desc:    "valid nodejs",
			input:   "nodejs",
			wantErr: false,
		},
		{
			desc:    "valid java",
			input:   "java",
			wantErr: false,
		},
		{
			desc:    "valid go",
			input:   "go",
			wantErr: false,
		},
		{
			desc:    "invalid language returns error",
			input:   "ruby",
			wantErr: true,
		},
		{
			desc:    "invalid language csharp",
			input:   "csharp",
			wantErr: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			err := ValidateLanguage(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ValidateLanguage(%q) expected error, got nil", tc.input)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateLanguage(%q) unexpected error: %v", tc.input, err)
				}
			}
		})
	}
}

func TestGetDatabasePort(t *testing.T) {
	tcs := []struct {
		desc   string
		dbType DatabaseType
		want   int
	}{
		{
			desc:   "postgresql port",
			dbType: PostgreSQL,
			want:   5432,
		},
		{
			desc:   "mysql port",
			dbType: MySQL,
			want:   3306,
		},
		{
			desc:   "sqlserver port",
			dbType: SQLServer,
			want:   1433,
		},
		{
			desc:   "unknown defaults to postgresql port",
			dbType: DatabaseType("unknown"),
			want:   5432,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got := GetDatabasePort(tc.dbType)
			if got != tc.want {
				t.Errorf("GetDatabasePort(%q) = %d, want %d", tc.dbType, got, tc.want)
			}
		})
	}
}

func TestDefaultDatabaseName(t *testing.T) {
	tcs := []struct {
		dbType DatabaseType
		want   string
	}{
		{PostgreSQL, "postgres"},
		{MySQL, "mysql"},
		{SQLServer, "master"},
		{DatabaseType("unknown"), ""},
	}
	for _, tc := range tcs {
		t.Run(string(tc.dbType), func(t *testing.T) {
			if got := DefaultDatabaseName(tc.dbType); got != tc.want {
				t.Errorf("DefaultDatabaseName(%q) = %q, want %q", tc.dbType, got, tc.want)
			}
		})
	}
}

func TestParseDatabaseType(t *testing.T) {
	tcs := []struct {
		desc    string
		version string
		want    DatabaseType
	}{
		{
			desc:    "postgres 14",
			version: "POSTGRES_14",
			want:    PostgreSQL,
		},
		{
			desc:    "postgres 15",
			version: "POSTGRES_15",
			want:    PostgreSQL,
		},
		{
			desc:    "mysql 8.0",
			version: "MYSQL_8_0",
			want:    MySQL,
		},
		{
			desc:    "mysql 5.7",
			version: "MYSQL_5_7",
			want:    MySQL,
		},
		{
			desc:    "sqlserver 2022 standard",
			version: "SQLSERVER_2022_STANDARD",
			want:    SQLServer,
		},
		{
			desc:    "sqlserver 2019 enterprise",
			version: "SQLSERVER_2019_ENTERPRISE",
			want:    SQLServer,
		},
		{
			desc:    "lowercased input is normalized",
			version: "postgres_15",
			want:    PostgreSQL,
		},
		{
			desc:    "unknown defaults to postgresql",
			version: "UNKNOWN_DB",
			want:    PostgreSQL,
		},
		{
			desc:    "empty string defaults to postgresql",
			version: "",
			want:    PostgreSQL,
		},
		{
			desc:    "short string defaults to postgresql",
			version: "PG",
			want:    PostgreSQL,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got := ParseDatabaseType(tc.version)
			if got != tc.want {
				t.Errorf("ParseDatabaseType(%q) = %q, want %q", tc.version, got, tc.want)
			}
		})
	}
}

func TestParseDatabaseTypeStrict(t *testing.T) {
	tcs := []struct {
		desc    string
		version string
		want    DatabaseType
		wantErr bool
	}{
		{desc: "postgres 15", version: "POSTGRES_15", want: PostgreSQL},
		{desc: "mysql 8.0", version: "MYSQL_8_0", want: MySQL},
		{desc: "sqlserver 2022 standard", version: "SQLSERVER_2022_STANDARD", want: SQLServer},
		{desc: "lowercased input is normalized", version: "postgres_15", want: PostgreSQL},
		{desc: "unknown returns error", version: "UNKNOWN_DB", wantErr: true},
		{desc: "empty returns error", version: "", wantErr: true},
		{desc: "short unknown returns error", version: "PG", wantErr: true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := ParseDatabaseTypeStrict(tc.version)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseDatabaseTypeStrict(%q) got no error, want one", tc.version)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDatabaseTypeStrict(%q) unexpected error: %v", tc.version, err)
			}
			if got != tc.want {
				t.Errorf("ParseDatabaseTypeStrict(%q) = %q, want %q", tc.version, got, tc.want)
			}
		})
	}
}

func TestDatabaseTypeConstants(t *testing.T) {
	// Verify constant values are correct
	if PostgreSQL != "postgres" {
		t.Errorf("PostgreSQL constant = %q, want %q", PostgreSQL, "postgres")
	}
	if MySQL != "mysql" {
		t.Errorf("MySQL constant = %q, want %q", MySQL, "mysql")
	}
	if SQLServer != "sqlserver" {
		t.Errorf("SQLServer constant = %q, want %q", SQLServer, "sqlserver")
	}
}

func TestAllDatabaseTypesCoverage(t *testing.T) {
	if len(AllDatabaseTypes) != 3 {
		t.Fatalf("AllDatabaseTypes has %d entries, want 3", len(AllDatabaseTypes))
	}
	want := map[DatabaseType]bool{PostgreSQL: false, MySQL: false, SQLServer: false}
	for _, dt := range AllDatabaseTypes {
		if _, ok := want[dt]; !ok {
			t.Errorf("unexpected DatabaseType in AllDatabaseTypes: %q", dt)
			continue
		}
		want[dt] = true
	}
	for dt, seen := range want {
		if !seen {
			t.Errorf("missing DatabaseType in AllDatabaseTypes: %q", dt)
		}
	}
}

func TestBuildConnectionStrings(t *testing.T) {
	sqlInfo := &CloudSQLInstanceInfo{PrivateIPAddress: "10.0.0.5"}
	connName := "p:r:i"

	tcs := []struct {
		desc          string
		method        ConnectionMethod
		dbType        DatabaseType
		dbName        string
		wantHost      string
		wantPort      int
		wantDSNSubstr string
	}{
		{
			desc:          "postgres direct private ip",
			method:        MethodDirectPrivateIP,
			dbType:        PostgreSQL,
			dbName:        "appdb",
			wantHost:      "10.0.0.5",
			wantPort:      5432,
			wantDSNSubstr: "postgresql://USER:PASS@10.0.0.5:5432/appdb",
		},
		{
			desc:          "mysql auth proxy",
			method:        MethodAuthProxy,
			dbType:        MySQL,
			dbName:        "appdb",
			wantHost:      "127.0.0.1",
			wantPort:      3306,
			wantDSNSubstr: "mysql://USER:PASS@127.0.0.1:3306/appdb",
		},
		{
			desc:          "sqlserver auth proxy",
			method:        MethodAuthProxy,
			dbType:        SQLServer,
			dbName:        "master",
			wantHost:      "127.0.0.1",
			wantPort:      1433,
			wantDSNSubstr: "sqlserver://USER:PASS@127.0.0.1:1433?database=master",
		},
		{
			desc:          "sqlserver direct private ip",
			method:        MethodDirectPrivateIP,
			dbType:        SQLServer,
			dbName:        "master",
			wantHost:      "10.0.0.5",
			wantPort:      1433,
			wantDSNSubstr: "sqlserver://USER:PASS@10.0.0.5:1433?database=master",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cs := BuildConnectionStrings(tc.method, tc.dbType, sqlInfo, tc.dbName, connName)
			if cs.Host != tc.wantHost {
				t.Errorf("Host = %q, want %q", cs.Host, tc.wantHost)
			}
			if cs.Port != tc.wantPort {
				t.Errorf("Port = %d, want %d", cs.Port, tc.wantPort)
			}
			if cs.DSN == "" {
				t.Errorf("DSN should not be empty")
			} else if !strings.Contains(cs.DSN, tc.wantDSNSubstr) {
				t.Errorf("DSN = %q, want substring %q", cs.DSN, tc.wantDSNSubstr)
			}
			if cs.JDBC == "" {
				t.Errorf("JDBC should not be empty")
			}
		})
	}
}

func TestBuildConnectionStringsConnectorMethod(t *testing.T) {
	cs := BuildConnectionStrings(MethodConnector, PostgreSQL, &CloudSQLInstanceInfo{}, "appdb", "p:r:i")
	if cs.Host != "" || cs.JDBC != "" {
		t.Errorf("connector method should not set Host/JDBC, got %+v", cs)
	}
	if cs.DSN == "" {
		t.Errorf("connector method should set a descriptive DSN")
	}
}

func TestComputeTypeConstants(t *testing.T) {
	// Verify constant values are correct
	if ComputeGCE != "gce" {
		t.Errorf("ComputeGCE constant = %q, want %q", ComputeGCE, "gce")
	}
	if ComputeGKE != "gke" {
		t.Errorf("ComputeGKE constant = %q, want %q", ComputeGKE, "gke")
	}
	if ComputeCloudRun != "cloudrun" {
		t.Errorf("ComputeCloudRun constant = %q, want %q", ComputeCloudRun, "cloudrun")
	}
	if ComputeLocal != "local" {
		t.Errorf("ComputeLocal constant = %q, want %q", ComputeLocal, "local")
	}
}

func TestConnectionMethodConstants(t *testing.T) {
	// Verify constant values are correct
	if MethodAuthProxy != "auth_proxy" {
		t.Errorf("MethodAuthProxy constant = %q, want %q", MethodAuthProxy, "auth_proxy")
	}
	if MethodConnector != "connector" {
		t.Errorf("MethodConnector constant = %q, want %q", MethodConnector, "connector")
	}
	if MethodDirectPrivateIP != "direct_private_ip" {
		t.Errorf("MethodDirectPrivateIP constant = %q, want %q", MethodDirectPrivateIP, "direct_private_ip")
	}
	if MethodUnixSocket != "unix_socket" {
		t.Errorf("MethodUnixSocket constant = %q, want %q", MethodUnixSocket, "unix_socket")
	}
}

func TestMethodNameConstants(t *testing.T) {
	// Verify display name constants are set
	if MethodNameAuthProxy == "" {
		t.Error("MethodNameAuthProxy should not be empty")
	}
	if MethodNameConnector == "" {
		t.Error("MethodNameConnector should not be empty")
	}
	if MethodNameDirectPrivateIP == "" {
		t.Error("MethodNameDirectPrivateIP should not be empty")
	}
	if MethodNameUnixSocket == "" {
		t.Error("MethodNameUnixSocket should not be empty")
	}
}

func TestAvailableLanguages(t *testing.T) {
	// Verify AvailableLanguages contains expected languages
	expected := map[Language]bool{
		LangPython: true,
		LangNodeJS: true,
		LangJava:   true,
		LangGo:     true,
	}

	if len(AvailableLanguages) != len(expected) {
		t.Errorf("AvailableLanguages has %d items, expected %d", len(AvailableLanguages), len(expected))
	}

	for _, lang := range AvailableLanguages {
		if !expected[lang] {
			t.Errorf("unexpected language in AvailableLanguages: %q", lang)
		}
	}
}
