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

package util

import (
	"encoding/json"
	"testing"
)

func TestUnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		wantErr     bool
	}{
		{"number and trailing whitespace", "{\"value\":9007199254740993} \n", false},
		{"incomplete value", `{`, true},
		{"second value", `{"value":1} {}`, true},
		{"trailing garbage", `{"value":1} x`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]any
			err := UnmarshalJSON([]byte(tc.input), &got)
			if (err != nil) != tc.wantErr {
				t.Fatalf("UnmarshalJSON() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && got["value"] != json.Number("9007199254740993") {
				t.Fatalf("decoded value = %v (%T), want exact json.Number", got["value"], got["value"])
			}
			if !tc.wantErr {
				encoded, err := MarshalArguments(got)
				if err != nil || string(encoded) != `{"value":9007199254740993}` {
					t.Fatalf("MarshalArguments() = %s, %v; want exact integer", encoded, err)
				}
			}
		})
	}
}

func TestMarshalArgumentsInvalidNumber(t *testing.T) {
	if _, err := MarshalArguments(map[string]any{"value": json.Number("1e400")}); err == nil {
		t.Fatal("expected an error for an unrepresentable number")
	}
}
