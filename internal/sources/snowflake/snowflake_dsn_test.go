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

package snowflake

import "testing"

func TestSnowflakeDSNOmitsEmptyRole(t *testing.T) {
	got := snowflakeDSN("user", "pw", "acct", "db", "PUBLIC", "WH", "")
	want := "user:pw@acct/db/PUBLIC?warehouse=WH"
	if got != want {
		t.Fatalf("empty role: got %q want %q", got, want)
	}

	got = snowflakeDSN("user", "pw", "acct", "db", "PUBLIC", "", "ANALYST")
	want = "user:pw@acct/db/PUBLIC?warehouse=COMPUTE_WH&role=ANALYST"
	if got != want {
		t.Fatalf("default warehouse: got %q want %q", got, want)
	}
}
