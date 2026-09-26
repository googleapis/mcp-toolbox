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

package bigquerygettableinfo

import (
	"encoding/json"
	"strings"
	"testing"

	bigqueryapi "cloud.google.com/go/bigquery"
)

func TestCompactTableMetadataSchema(t *testing.T) {
	metadata := &bigqueryapi.TableMetadata{
		NumBytes:    9007199254740993,
		Description: "table description",
		Schema: bigqueryapi.Schema{
			{Name: "id", Type: bigqueryapi.IntegerFieldType},
			{Name: "nested", Type: bigqueryapi.RecordFieldType, Repeated: true, Schema: bigqueryapi.Schema{
				{Name: "label", Type: bigqueryapi.StringFieldType, Description: "kept"},
			}},
		},
	}

	compacted, err := compactTableMetadata(metadata)
	if err != nil {
		t.Fatalf("compactTableMetadata() error = %v", err)
	}

	encoded, err := json.Marshal(compacted)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	got := string(encoded)
	for _, unwanted := range []string{`"Description":""`, `"Repeated":false`, `"Required":false`, `"Precision":0`, `"Schema":null`} {
		if strings.Contains(got, unwanted) {
			t.Errorf("compacted metadata contains zero-valued field %s: %s", unwanted, got)
		}
	}
	for _, wanted := range []string{`"NumBytes":9007199254740993`, `"Name":"id"`, `"Type":"INTEGER"`, `"Name":"nested"`, `"Repeated":true`, `"Description":"kept"`} {
		if !strings.Contains(got, wanted) {
			t.Errorf("compacted metadata is missing %s: %s", wanted, got)
		}
	}
	if !strings.Contains(got, `"Description":"table description"`) {
		t.Errorf("compacted metadata dropped table-level metadata: %s", got)
	}
}
