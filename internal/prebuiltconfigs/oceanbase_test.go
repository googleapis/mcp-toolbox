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

package prebuiltconfigs

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// TestOceanBaseListTablesParameters checks that the oceanbase list_tables
// statement binds exactly one value per declared parameter, and that the
// optional table_names parameter has a default so it can be omitted.
func TestOceanBaseListTablesParameters(t *testing.T) {
	raw := getOrFatal(t, "oceanbase")
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	for {
		var doc struct {
			Kind       string `yaml:"kind"`
			Name       string `yaml:"name"`
			Statement  string `yaml:"statement"`
			Parameters []struct {
				Name    string `yaml:"name"`
				Default any    `yaml:"default"`
			} `yaml:"parameters"`
		}
		if err := decoder.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("unable to decode oceanbase prebuilt config: %s", err)
		}
		if doc.Kind != "tool" || doc.Name != "list_tables" {
			continue
		}

		if got, want := strings.Count(doc.Statement, "?"), len(doc.Parameters); got != want {
			t.Errorf("list_tables statement has %d placeholders, want %d (one per parameter)", got, want)
		}
		for _, p := range doc.Parameters {
			if p.Name == "table_names" && p.Default == nil {
				t.Errorf("list_tables parameter table_names has no default, so it cannot be omitted")
			}
		}
		return
	}
	t.Fatalf("list_tables tool not found in oceanbase prebuilt config")
}
