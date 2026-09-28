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

package sources

import (
	"testing"
)

func TestNormalizeValue(t *testing.T) {
	// Test case 1: UUID OID (2950) with [16]byte
	uuidBytes := [16]byte{1, 158, 103, 138, 45, 22, 120, 229, 146, 32, 191, 118, 245, 114, 247, 50}
	expectedUUIDStr := "019e678a-2d16-78e5-9220-bf76f572f732"

	result := NormalizeValue(uuidBytes, 2950)
	if str, ok := result.(string); !ok || str != expectedUUIDStr {
		t.Errorf("Expected UUID string %s, got %v", expectedUUIDStr, result)
	}

	// Test case 2: UUID OID (2950) with other type
	result = NormalizeValue("not-bytes", 2950)
	if str, ok := result.(string); !ok || str != "not-bytes" {
		t.Errorf("Expected original string 'not-bytes', got %v", result)
	}

	// Test case 3: Other OID
	result = NormalizeValue(uuidBytes, 1000)
	if bytes, ok := result.([16]byte); !ok || bytes != uuidBytes {
		t.Errorf("Expected original [16]byte array, got %v", result)
	}

	// Test case 4: UUID Array OID (2951) with [][16]byte
	uuidArray := [][16]byte{uuidBytes}
	result = NormalizeValue(uuidArray, 2951)
	if arr, ok := result.([]string); !ok || len(arr) != 1 || arr[0] != expectedUUIDStr {
		t.Errorf("Expected []string with %s, got %v", expectedUUIDStr, result)
	}

	// Test case 5: UUID Array OID (2951) with []any
	uuidAnyArray := []any{uuidBytes, "not-a-uuid"}
	result = NormalizeValue(uuidAnyArray, 2951)
	if arr, ok := result.([]any); !ok || len(arr) != 2 || arr[0] != expectedUUIDStr || arr[1] != "not-a-uuid" {
		t.Errorf("Expected []any with normalized UUID, got %v", result)
	}
}
