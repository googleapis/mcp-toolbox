// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package http

import (
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestGetHeadersNonStringValueNamesType(t *testing.T) {
	headerParams := parameters.Parameters{parameters.NewStringParameter("X-Token", "token header")}
	paramsMap := map[string]any{"X-Token": 42}

	_, err := getHeaders(headerParams, map[string]string{}, paramsMap)
	if err == nil {
		t.Fatal("expected an error for a non-string header value, got nil")
	}

	msg := err.Error()
	// The message should name the real type (e.g. "int"); a wrong format verb
	// would instead emit a malformed token like "%!t(int=42)".
	if strings.Contains(msg, "%!") {
		t.Errorf("error message has a malformed format verb: %q", msg)
	}
	if !strings.Contains(msg, "type int,") {
		t.Errorf("error message should name the type as int, got: %q", msg)
	}
}
