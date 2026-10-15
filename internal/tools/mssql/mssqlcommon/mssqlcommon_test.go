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

package mssqlcommon_test

import (
	"context"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/mssql/mssqlcommon"
	"github.com/googleapis/mcp-toolbox/internal/tools/mssql/mssqlexecutesql"
	"github.com/googleapis/mcp-toolbox/internal/tools/mssql/mssqllisttables"
	"github.com/googleapis/mcp-toolbox/internal/tools/mssql/mssqlsql"
)

// callerSource stands in for a source that can authenticate as the caller, and
// records which path a statement took.
type callerSource struct {
	sources.Source
	on     bool
	header string
	ran    string
}

func (s *callerSource) UseClientAuthorization() bool   { return s.on }
func (s *callerSource) GetAuthTokenHeaderName() string { return s.header }
func (s *callerSource) RunSQLForClient(_ context.Context, _ tools.AccessToken, statement string, _ []any) (any, error) {
	s.ran = "caller:" + statement
	return nil, nil
}
func (s *callerSource) RunSQL(_ context.Context, statement string, _ []any) (any, error) {
	s.ran = "shared:" + statement
	return nil, nil
}

func TestRunSQLFollowsTheSource(t *testing.T) {
	ctx := context.Background()
	on := &callerSource{on: true}
	if _, err := mssqlcommon.RunSQL(ctx, on, "Bearer t", "SELECT 1", nil); err != nil || on.ran != "caller:SELECT 1" {
		t.Errorf("client-authorized source: ran %q, err %v; want the caller path", on.ran, err)
	}
	off := &callerSource{on: false}
	if _, err := mssqlcommon.RunSQL(ctx, off, "", "SELECT 1", nil); err != nil || off.ran != "shared:SELECT 1" {
		t.Errorf("shared-identity source: ran %q, err %v; want the source's own connection", off.ran, err)
	}
}

// Every mssql tool must answer these from the source, not from the BaseTool
// defaults: otherwise the server never demands a token for a source that needs
// one, and a custom header name is never read.
func TestToolsAnswerClientAuthorizationFromTheSource(t *testing.T) {
	type answers interface {
		RequiresClientAuthorization(sources.Source) (bool, error)
		GetAuthTokenHeaderName(sources.Source) (string, error)
	}
	on := &callerSource{on: true, header: "X-Forwarded-Token"}
	off := &callerSource{on: false, header: "Authorization"}
	var plain sources.Source // no notion of caller identity

	for name, tool := range map[string]answers{
		"mssql-sql":         mssqlsql.Tool{},
		"mssql-execute-sql": mssqlexecutesql.Tool{},
		"mssql-list-tables": mssqllisttables.Tool{},
	} {
		t.Run(name, func(t *testing.T) {
			if got, _ := tool.RequiresClientAuthorization(on); !got {
				t.Errorf("RequiresClientAuthorization: got false for a source that authenticates as the caller")
			}
			if got, _ := tool.GetAuthTokenHeaderName(on); got != "X-Forwarded-Token" {
				t.Errorf("GetAuthTokenHeaderName: got %q, want the source's header", got)
			}
			if got, _ := tool.RequiresClientAuthorization(off); got {
				t.Errorf("RequiresClientAuthorization: got true for a source using its own identity")
			}
			if got, _ := tool.RequiresClientAuthorization(plain); got {
				t.Errorf("RequiresClientAuthorization: got true for a source without client authorization")
			}
			if got, _ := tool.GetAuthTokenHeaderName(plain); got != "Authorization" {
				t.Errorf("GetAuthTokenHeaderName: got %q, want the default", got)
			}
		})
	}
}
