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

package mssqlsql_test

import (
	"context"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/mssql/mssqlsql"
)

// callerSource stands in for a source that can authenticate as the caller.
type callerSource struct {
	sources.Source
	on     bool
	header string
}

func (s *callerSource) UseClientAuthorization() bool   { return s.on }
func (s *callerSource) GetAuthTokenHeaderName() string { return s.header }
func (s *callerSource) RunSQLForClient(context.Context, tools.AccessToken, string, []any) (any, error) {
	return nil, nil
}

// The tool must answer these from the source, not from the BaseTool defaults:
// otherwise the server never demands a token for a source that needs one, and a
// custom header name is never read.
func TestClientAuthorizationFollowsTheSource(t *testing.T) {
	tool := mssqlsql.Tool{}

	on := &callerSource{on: true, header: "X-Forwarded-Token"}
	if got, _ := tool.RequiresClientAuthorization(on); !got {
		t.Errorf("RequiresClientAuthorization: got false for a source that authenticates as the caller")
	}
	if got, _ := tool.GetAuthTokenHeaderName(on); got != "X-Forwarded-Token" {
		t.Errorf("GetAuthTokenHeaderName: got %q, want the source's header", got)
	}

	off := &callerSource{on: false}
	if got, _ := tool.RequiresClientAuthorization(off); got {
		t.Errorf("RequiresClientAuthorization: got true for a source using its own identity")
	}
	if got, _ := tool.GetAuthTokenHeaderName(off); got != "Authorization" {
		t.Errorf("GetAuthTokenHeaderName: got %q, want the default", got)
	}

	// A source with no notion of caller identity leaves the defaults in place.
	var plain sources.Source
	if got, _ := tool.RequiresClientAuthorization(plain); got {
		t.Errorf("RequiresClientAuthorization: got true for a source without client authorization")
	}
	if got, _ := tool.GetAuthTokenHeaderName(plain); got != "Authorization" {
		t.Errorf("GetAuthTokenHeaderName: got %q, want the default", got)
	}
}
