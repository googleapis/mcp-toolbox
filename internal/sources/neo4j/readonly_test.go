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

package neo4j

import (
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
)

func TestExecuteQueryOptions(t *testing.T) {
	tcs := []struct {
		name        string
		readOnly    bool
		wantRouting neo4j.RoutingControl
	}{
		{
			name:        "readOnly query runs in read access mode",
			readOnly:    true,
			wantRouting: neo4j.Read,
		},
		{
			name:        "other query keeps write access mode",
			readOnly:    false,
			wantRouting: neo4j.Write,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			var got neo4j.ExecuteQueryConfiguration
			for _, opt := range executeQueryOptions("my-db", tc.readOnly) {
				opt(&got)
			}
			if got.Routing != tc.wantRouting {
				t.Errorf("Routing = %v, want %v", got.Routing, tc.wantRouting)
			}
			if got.Database != "my-db" {
				t.Errorf("Database = %q, want %q", got.Database, "my-db")
			}
		})
	}
}
