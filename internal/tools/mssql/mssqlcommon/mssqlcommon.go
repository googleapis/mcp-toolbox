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

// Package mssqlcommon holds what the mssql tools share: sending a statement to
// the source as the caller when the source authenticates that way.
package mssqlcommon

import (
	"context"
	"fmt"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

// ClientAuthorizedSource is implemented by sources that can authenticate to the
// database as the caller rather than as one configured identity. Sources that
// cannot are unaffected: the tools keep using the source's own connection.
type ClientAuthorizedSource interface {
	UseClientAuthorization() bool
	GetAuthTokenHeaderName() string
	RunSQLForClient(context.Context, tools.AccessToken, string, []any) (any, error)
}

type sqlRunner interface {
	RunSQL(context.Context, string, []any) (any, error)
}

// RunSQL sends the statement to the source, as the caller when the source is
// configured to authenticate that way.
func RunSQL(ctx context.Context, s sources.Source, accessToken tools.AccessToken, statement string, params []any) (any, error) {
	if ca, ok := s.(ClientAuthorizedSource); ok && ca.UseClientAuthorization() {
		return ca.RunSQLForClient(ctx, accessToken, statement, params)
	}
	r, ok := s.(sqlRunner)
	if !ok {
		return nil, fmt.Errorf("source is not compatible with the mssql tools")
	}
	return r.RunSQL(ctx, statement, params)
}

// RequiresClientAuthorization answers, for a tool, whether the server must hand it
// the caller's token. The BaseTool default is an unconditional false, which would
// leave a source that connects as the caller with nothing to connect with.
func RequiresClientAuthorization(s sources.Source) (bool, error) {
	ca, ok := s.(ClientAuthorizedSource)
	return ok && ca.UseClientAuthorization(), nil
}

// GetAuthTokenHeaderName answers, for a tool, which header the caller's token is
// read from. Sources with no notion of caller identity keep the default.
func GetAuthTokenHeaderName(s sources.Source) (string, error) {
	if ca, ok := s.(ClientAuthorizedSource); ok {
		return ca.GetAuthTokenHeaderName(), nil
	}
	return "Authorization", nil
}
