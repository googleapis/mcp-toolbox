// Copyright 2026 Google LLC
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
package util

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected string
	}{
		{
			name:     "No headers",
			headers:  map[string]string{},
			expected: "",
		},
		{
			name: "Only X-Real-IP",
			headers: map[string]string{
				"X-Real-IP": "1.2.3.4",
			},
			expected: "1.2.3.4",
		},
		{
			name: "X-Real-IP with whitespace",
			headers: map[string]string{
				"X-Real-IP": "  1.2.3.4  ",
			},
			expected: "1.2.3.4",
		},
		{
			name: "Only X-Forwarded-For single IP",
			headers: map[string]string{
				"X-Forwarded-For": "1.2.3.4",
			},
			expected: "1.2.3.4",
		},
		{
			name: "Only X-Forwarded-For multiple IPs",
			headers: map[string]string{
				"X-Forwarded-For": "1.2.3.4, 5.6.7.8, 9.10.11.12",
			},
			expected: "1.2.3.4",
		},
		{
			name: "X-Forwarded-For starts with empty and whitespace",
			headers: map[string]string{
				"X-Forwarded-For": ", 1.2.3.4",
			},
			expected: "1.2.3.4",
		},
		{
			name: "X-Forwarded-For starts with multiple spaces",
			headers: map[string]string{
				"X-Forwarded-For": "   ,  , 1.2.3.4",
			},
			expected: "1.2.3.4",
		},
		{
			name: "X-Forwarded-For and X-Real-IP preferred X-Forwarded-For",
			headers: map[string]string{
				"X-Forwarded-For": "1.2.3.4",
				"X-Real-IP":       "5.6.7.8",
			},
			expected: "1.2.3.4",
		},
		{
			name: "X-Forwarded-For empty and X-Real-IP fallback",
			headers: map[string]string{
				"X-Forwarded-For": ", ",
				"X-Real-IP":       "5.6.7.8",
			},
			expected: "5.6.7.8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			actual := ExtractClientIP(req.Header)
			if actual != tt.expected {
				t.Errorf("ExtractClientIP() = %q, expected %q", actual, tt.expected)
			}
		})
	}
}

func TestSnakeFromCamelCase(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "displayName", want: "display_name"},
		{in: "ownerEmails", want: "owner_emails"},
		{in: "accessGroups", want: "access_groups"},
		{in: "description", want: "description"},
		{in: "display_name", want: "display_name"},
		{in: "a", want: "a"},
		{in: "A", want: "a"},
		{in: "locationId", want: "location_id"},
		{in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := SnakeFromCamelCase(tt.in)
			if got != tt.want {
				t.Errorf("SnakeFromCamelCase(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestConvertNumbers(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want any
	}{
		{"integer", json.Number("123"), int64(123)},
		{"negative integer", json.Number("-7"), int64(-7)},
		{"decimal", json.Number("1.5"), float64(1.5)},
		// Exponent-form numbers contain no ".", so deciding the type on the
		// presence of a "." alone sent them to Int64, which rejects exponent
		// syntax and returned an error. json.Marshal emits this form for
		// large or small floats (e.g. 1e-07, 1e+21), so it is reachable.
		{"positive exponent", json.Number("1e5"), float64(100000)},
		{"uppercase exponent", json.Number("1E5"), float64(100000)},
		{"negative exponent", json.Number("1e-07"), float64(1e-07)},
		{"large exponent beyond int64", json.Number("1e+21"), float64(1e21)},
		// An integer literal larger than int64 also falls back to float
		// instead of erroring.
		{"integer overflowing int64", json.Number("99999999999999999999"), float64(99999999999999999999)},
		{"string passthrough", "hello", "hello"},
		{"bool passthrough", true, true},
		{
			"nested map and slice",
			map[string]any{"a": json.Number("2e3"), "b": []any{json.Number("4")}},
			map[string]any{"a": float64(2000), "b": []any{int64(4)}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertNumbers(tc.in)
			if err != nil {
				t.Fatalf("ConvertNumbers(%v) returned error: %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ConvertNumbers(%v) = %#v (%T), want %#v (%T)", tc.in, got, got, tc.want, tc.want)
			}
		})
	}
}
