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
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/util"
	mssql "github.com/microsoft/go-mssqldb"
)

// severeChain is what go-mssqldb reports for
// RAISERROR('severe error text', 20, 1) WITH LOG on SQL Server 2017 to 2025.
var severeChain = []mssql.Error{
	{Number: 50000, Class: 20, State: 1, Message: "severe error text"},
	{Number: 2745, Class: 16, State: 2, Message: "Process ID 51 has raised user error 50000, severity 20. SQL Server is terminating this process."},
	{Number: 596, Class: 21, State: 1, Message: "Cannot continue the execution because the session is in the kill state."},
}

func lastWithAll(chain []mssql.Error) mssql.Error {
	e := chain[len(chain)-1]
	e.All = chain
	return e
}

// serverError stands in for the error go-mssqldb returns for a severe error:
// an mssql.ServerError that wraps an mssql.Error. mssql.ServerError keeps the
// wrapped error in an unexported field, so it can only be built empty outside
// the driver; errors.Join puts the two side by side in one chain instead.
func serverError(wrapped mssql.Error) error {
	return errors.Join(wrapped, mssql.ServerError{})
}

func TestFormatErrorChain(t *testing.T) {
	tcs := []struct {
		name string
		in   mssql.Error
		want string
	}{
		{
			name: "every error in All, first to last",
			in:   lastWithAll(severeChain),
			want: "severe error text (error 50000, severity 20, state 1); " +
				"Process ID 51 has raised user error 50000, severity 20. SQL Server is terminating this process. (error 2745, severity 16, state 2); " +
				"Cannot continue the execution because the session is in the kill state. (error 596, severity 21, state 1)",
		},
		{
			name: "the error itself when All is empty",
			in:   mssql.Error{Number: 211, Class: 23, State: 8, Message: "Possible schema corruption. Run DBCC CHECKCATALOG."},
			want: "Possible schema corruption. Run DBCC CHECKCATALOG. (error 211, severity 23, state 8)",
		},
		{
			name: "entries without a message are skipped",
			in:   lastWithAll([]mssql.Error{{Number: 1, Class: 20}, {Number: 2, Class: 21, State: 3, Message: "kept"}}),
			want: "kept (error 2, severity 21, state 3)",
		},
		{
			name: "nothing to report",
			in:   mssql.Error{},
			want: "",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatErrorChain(tc.in); got != tc.want {
				t.Errorf("formatErrorChain() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWithServerErrorDetailsAddsTheChain(t *testing.T) {
	in := serverError(lastWithAll(severeChain))
	got := WithServerErrorDetails(in)

	want := in.Error() + ": " + formatErrorChain(lastWithAll(severeChain))
	if got.Error() != want {
		t.Errorf("WithServerErrorDetails().Error() = %q, want %q", got.Error(), want)
	}
	if !strings.HasSuffix(in.Error(), "SQL Server had internal error") {
		t.Fatalf("test setup: %q does not end with the driver's fixed text", in.Error())
	}
	if !errors.Is(got, in) {
		t.Errorf("WithServerErrorDetails() does not wrap the original error")
	}
	var serverErr mssql.ServerError
	if !errors.As(got, &serverErr) {
		t.Errorf("errors.As(mssql.ServerError) = false after WithServerErrorDetails()")
	}
}

func TestWithServerErrorDetailsLeavesOtherErrorsUnchanged(t *testing.T) {
	plain := errors.New("connection reset")
	sqlErr := mssql.Error{Number: 50000, Class: 16, State: 1, Message: "normal error text"}
	tcs := []struct {
		name string
		in   error
	}{
		{name: "nil", in: nil},
		{name: "plain error", in: plain},
		{name: "mssql.Error below severity 20", in: sqlErr},
		{name: "wrapped mssql.Error", in: fmt.Errorf("unable to execute query: %w", sqlErr)},
		{name: "mssql.ServerError without details", in: mssql.ServerError{}},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got := WithServerErrorDetails(tc.in)
			// mssql.Error and mssql.ServerError hold a slice, so they can't be
			// compared with ==; the same type and text means nothing was added.
			if tc.in == nil {
				if got != nil {
					t.Errorf("WithServerErrorDetails(nil) = %v, want nil", got)
				}
				return
			}
			if got == nil || fmt.Sprintf("%T", got) != fmt.Sprintf("%T", tc.in) || got.Error() != tc.in.Error() {
				t.Errorf("WithServerErrorDetails() = %T %v, want the input unchanged (%T %v)", got, got, tc.in, tc.in)
			}
		})
	}
}

// SQL Server has errors 401, 403 and 429; their numbers must not make
// util.ProcessGeneralError report an HTTP auth or rate-limit failure.
func TestWithServerErrorDetailsKeepsTheErrorCategory(t *testing.T) {
	for _, number := range []int32{401, 403, 429} {
		t.Run(fmt.Sprint(number), func(t *testing.T) {
			in := serverError(lastWithAll([]mssql.Error{
				{Number: number, Class: 20, State: 1, Message: "severe error text"},
			}))
			err := fmt.Errorf("unable to execute query: %w", WithServerErrorDetails(in))
			got := util.ProcessGeneralError(err)
			if got.Category() != util.CategoryAgent {
				t.Errorf("ProcessGeneralError(%q).Category() = %s, want %s", err, got.Category(), util.CategoryAgent)
			}
		})
	}
}
