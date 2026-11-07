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

package mssqlcommon

import (
	"errors"
	"fmt"
	"strings"

	mssql "github.com/microsoft/go-mssqldb"
)

// WithServerErrorDetails adds the errors SQL Server reported to an
// mssql.ServerError, and returns every other error unchanged.
//
// go-mssqldb returns mssql.ServerError when the server ends a request with a
// severe error (severity 20 and above). Its Error() is the fixed text
// "SQL Server had internal error"; the errors the server sent are kept in the
// mssql.Error it wraps. That error's own fields describe the last error, which
// is usually a consequence ("the session is in the kill state"); its All field
// lists every error from first to last, and the first is usually the cause.
// So all of them are appended, in order, after the existing text, and the
// original error stays wrapped.
func WithServerErrorDetails(err error) error {
	var serverErr mssql.ServerError
	if !errors.As(err, &serverErr) {
		return err
	}
	// mssql.ServerError always unwraps to an mssql.Error. If it ever did not,
	// sqlErr would stay empty, formatErrorChain would return "", and err would
	// come back unchanged.
	var sqlErr mssql.Error
	errors.As(err, &sqlErr)
	details := formatErrorChain(sqlErr)
	if details == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, details)
}

// formatErrorChain formats every error in e.All, or e itself when All is
// empty, as "<message> (error <number>, severity <class>, state <state>)",
// joined by "; ". Entries without a message are skipped.
//
// It reads the fields rather than calling Error(), whose format differs
// between go-mssqldb versions.
func formatErrorChain(e mssql.Error) string {
	chain := e.All
	if len(chain) == 0 {
		chain = []mssql.Error{e}
	}
	parts := make([]string, 0, len(chain))
	for _, c := range chain {
		if c.Message == "" {
			continue
		}
		// "error" stays lower case: util.ProcessGeneralError looks for
		// "Error 401", "Error 403" and "Error 429" in the text, and SQL Server
		// has errors with those numbers.
		parts = append(parts, fmt.Sprintf("%s (error %d, severity %d, state %d)", c.Message, c.Number, c.Class, c.State))
	}
	return strings.Join(parts, "; ")
}
