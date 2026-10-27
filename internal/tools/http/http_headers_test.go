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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestGetHeadersOptionalParams(t *testing.T) {
	headerParams := parameters.Parameters{
		parameters.NewStringParameter("X-Trace-Id", "optional trace id", parameters.WithStringRequired(false)),
	}
	defaultHeaders := map[string]string{"Content-Type": "application/json"}

	testCases := []struct {
		name  string
		input map[string]any
		want  map[string]string
	}{
		{
			name:  "optional header omitted",
			input: map[string]any{},
			want:  map[string]string{"Content-Type": "application/json"},
		},
		{
			name:  "optional header provided",
			input: map[string]any{"X-Trace-Id": "abc123"},
			want:  map[string]string{"Content-Type": "application/json", "X-Trace-Id": "abc123"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build paramsMap through ParseParams, as Invoke does.
			paramValues, err := parameters.ParseParams(headerParams, tc.input, nil)
			if err != nil {
				t.Fatalf("unexpected error parsing params: %v", err)
			}
			got, err := getHeaders(headerParams, defaultHeaders, paramValues.AsMap())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("incorrect headers (-want +got):\n%s", diff)
			}
		})
	}
}
