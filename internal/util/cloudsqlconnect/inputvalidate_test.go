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

package cloudsqlconnect

import "testing"

func TestValidateInstanceConnectionName(t *testing.T) {
	tcs := []struct {
		desc    string
		input   string
		wantErr bool
	}{
		{desc: "valid", input: "my-project:us-central1:my-instance"},
		{desc: "europe", input: "project-123:europe-west1:db-prod"},
		{desc: "uppercase project rejected", input: "MyProject:us-central1:my-instance", wantErr: true},
		{desc: "shell metachar in instance rejected", input: "my-project:us-central1:i;rm-rf", wantErr: true},
		{desc: "quote in instance rejected", input: "my-project:us-central1:i'inst", wantErr: true},
		{desc: "spaces rejected", input: "my-project:us-central1:my inst", wantErr: true},
		{desc: "trailing hyphen on instance rejected", input: "my-project:us-central1:inst-", wantErr: true},
		{desc: "too short project rejected", input: "p:us-central1:i", wantErr: true},
		{desc: "newline in instance rejected", input: "my-project:us-central1:i\ninst", wantErr: true},
		{desc: "backtick in project rejected", input: "my`project:us-central1:my-instance", wantErr: true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, err := ValidateInstanceConnectionName(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got none", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
		})
	}
}

func TestValidateGCEResourceName(t *testing.T) {
	tcs := []struct {
		desc    string
		input   string
		wantErr bool
	}{
		{desc: "valid", input: "my-vm-1"},
		{desc: "single letter", input: "v"},
		{desc: "uppercase rejected", input: "MyVM", wantErr: true},
		{desc: "shell metachar rejected", input: "vm; rm -rf /", wantErr: true},
		{desc: "starts with digit rejected", input: "1vm", wantErr: true},
		{desc: "starts with hyphen rejected", input: "-vm", wantErr: true},
		{desc: "ends with hyphen rejected", input: "vm-", wantErr: true},
		{desc: "too long rejected", input: "vm-" + repeat("a", 70), wantErr: true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			err := ValidateGCEResourceName(tc.input, "vm_name")
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got none", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
		})
	}
}

func TestValidateDatabaseName(t *testing.T) {
	tcs := []struct {
		desc    string
		input   string
		wantErr bool
	}{
		{desc: "valid lower", input: "appdb"},
		{desc: "valid mixed case", input: "MyDB"},
		{desc: "valid with underscore", input: "my_db_1"},
		{desc: "valid leading underscore", input: "_appdb"},
		{desc: "starts with digit rejected", input: "1db", wantErr: true},
		{desc: "quote rejected", input: `app"db`, wantErr: true},
		{desc: "semicolon rejected", input: "app;drop", wantErr: true},
		{desc: "space rejected", input: "app db", wantErr: true},
		{desc: "newline rejected", input: "app\ndb", wantErr: true},
		{desc: "shell substitution rejected", input: "app$(id)", wantErr: true},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			err := ValidateDatabaseName(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got none", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
		})
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
