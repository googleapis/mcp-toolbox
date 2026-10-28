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

// TestGenerateCodeSnippetSQLServer verifies that the dispatcher routes
// SQLServer requests to the SQL-Server-specific generators (which don't share
// driver libraries with Postgres/MySQL) for every supported language and the
// two methods that make sense for SQL Server (Auth Proxy, Direct Private IP).
func TestGenerateCodeSnippetSQLServer(t *testing.T) {
	const (
		connName  = "my-project:us-central1:my-mssql"
		dbName    = "appdb"
		port      = 1433
		privateIP = "10.0.0.5"
	)

	tcs := []struct {
		desc            string
		lang            Language
		method          ConnectionMethod
		wantCodeSubstrs []string
		wantDeps        []string
	}{
		{
			desc:            "python auth proxy uses pyodbc",
			lang:            LangPython,
			method:          MethodAuthProxy,
			wantCodeSubstrs: []string{"mssql+pyodbc", "ODBC Driver 18 for SQL Server", "127.0.0.1"},
			wantDeps:        []string{"pyodbc", "sqlalchemy"},
		},
		{
			desc:            "python direct private ip uses pyodbc with private IP",
			lang:            LangPython,
			method:          MethodDirectPrivateIP,
			wantCodeSubstrs: []string{"mssql+pyodbc", privateIP},
		},
		{
			desc:            "nodejs auth proxy uses mssql package",
			lang:            LangNodeJS,
			method:          MethodAuthProxy,
			wantCodeSubstrs: []string{"require('mssql')", "127.0.0.1", "encrypt: true"},
			wantDeps:        []string{"mssql"},
		},
		{
			desc:            "java auth proxy uses jdbc:sqlserver",
			lang:            LangJava,
			method:          MethodAuthProxy,
			wantCodeSubstrs: []string{"jdbc:sqlserver://", "encrypt=true", "trustServerCertificate=true"},
		},
		{
			desc:            "go auth proxy uses go-mssqldb driver",
			lang:            LangGo,
			method:          MethodAuthProxy,
			wantCodeSubstrs: []string{"github.com/microsoft/go-mssqldb", `sql.Open("sqlserver"`},
			wantDeps:        []string{"github.com/microsoft/go-mssqldb"},
		},
		{
			desc:            "go direct private ip references private IP",
			lang:            LangGo,
			method:          MethodDirectPrivateIP,
			wantCodeSubstrs: []string{`sql.Open("sqlserver"`},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			snippet := GenerateCodeSnippet(tc.lang, tc.method, SQLServer, connName, dbName, port, privateIP)
			if snippet == nil {
				t.Fatalf("snippet is nil")
			}
			if snippet.Language != tc.lang {
				t.Errorf("Language = %q, want %q", snippet.Language, tc.lang)
			}
			for _, want := range tc.wantCodeSubstrs {
				if !strings.Contains(snippet.Code, want) {
					t.Errorf("Code missing substring %q\nGot:\n%s", want, snippet.Code)
				}
			}
			for _, dep := range tc.wantDeps {
				if !sliceContainsPackage(snippet.Dependencies, dep) {
					t.Errorf("Dependencies missing package %q, got %v", dep, snippet.Dependencies)
				}
			}
		})
	}
}

func TestGenerateCodeSnippetSQLServerConnectorFallback(t *testing.T) {
	// SQL Server doesn't have first-class Connector library snippets, so
	// the Connector method should produce a "use Auth Proxy" fallback note
	// instead of an empty snippet.
	snippet := GenerateCodeSnippet(LangPython, MethodConnector, SQLServer, "p:r:i", "master", 1433, "")
	if snippet == nil {
		t.Fatalf("nil snippet")
	}
	if !strings.Contains(strings.ToLower(snippet.Code), "auth proxy") &&
		(len(snippet.Notes) == 0 || !strings.Contains(strings.ToLower(snippet.Notes[0]), "auth proxy")) {
		t.Errorf("expected fallback to mention Auth Proxy, got code=%q notes=%v", snippet.Code, snippet.Notes)
	}
}

func TestGenerateCodeSnippetUnsupportedLanguageForSQLServer(t *testing.T) {
	snippet := GenerateCodeSnippet(Language("ruby"), MethodAuthProxy, SQLServer, "p:r:i", "master", 1433, "")
	if snippet == nil {
		t.Fatalf("nil snippet")
	}
	if !strings.Contains(strings.ToLower(snippet.Code), "unsupported") {
		t.Errorf("expected unsupported-language message, got %q", snippet.Code)
	}
}

// sliceContainsPackage matches when any dep string equals needle or begins
// with needle followed by one of the version-suffix delimiters used across
// package managers (pip `>=`, npm/Go `@`, Maven `:`).
func sliceContainsPackage(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle ||
			strings.HasPrefix(h, needle+">=") ||
			strings.HasPrefix(h, needle+"@") ||
			strings.HasPrefix(h, needle+":") {
			return true
		}
	}
	return false
}
