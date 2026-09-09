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

package server

import "testing"

func TestResponseEncodingFlag(t *testing.T) {
	var e responseEncoding

	// Default before Set is "json".
	if got := e.String(); got != "json" {
		t.Errorf("default String() = %q, want %q", got, "json")
	}
	if got := e.Type(); got != "responseEncoding" {
		t.Errorf("Type() = %q, want %q", got, "responseEncoding")
	}

	// Accepted values, case-insensitively.
	for _, v := range []string{"json", "gcf", "JSON", "GCF"} {
		if err := e.Set(v); err != nil {
			t.Errorf("Set(%q) unexpected error: %v", v, err)
		}
	}
	if got := e.String(); got != "gcf" {
		t.Errorf("after Set(\"GCF\"), String() = %q, want %q", got, "gcf")
	}
	// Set normalizes to lowercase, so the stored value matches downstream ("gcf")
	// comparisons even when the flag was given as "GCF".
	if got := string(e); got != "gcf" {
		t.Errorf("Set(\"GCF\") stored %q, want lowercased %q", got, "gcf")
	}

	// Rejected values leave the previous value intact.
	for _, v := range []string{"yaml", "toon", ""} {
		if err := e.Set(v); err == nil {
			t.Errorf("Set(%q) should have returned an error", v)
		}
	}
}
