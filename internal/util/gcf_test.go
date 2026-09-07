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
	"strings"
	"testing"

	gcf "github.com/blackwell-systems/gcf-go"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
)

func gcfRows(n int) []any {
	out := make([]any, n)
	for i := 0; i < n; i++ {
		r := orderedmap.Row{}
		r.Add("id", int64(60+i)) // column order is deliberately non-alphabetical
		r.Add("name", "svc")
		r.Add("region", "us-central1")
		out[i] = r
	}
	return out
}

func TestEncodeGCFToolResult_RoundTripsAndFactorsHeader(t *testing.T) {
	wire, ok := EncodeGCFToolResult(gcfRows(20))
	if !ok {
		t.Fatal("expected GCF for a 20-row uniform result")
	}
	if !strings.HasPrefix(wire, "GCF profile=generic") {
		t.Fatalf("not a generic wire: %q", wire)
	}
	if got := strings.Count(wire, "{id,name,region}"); got != 1 {
		t.Errorf("expected the field names factored into one header, got %d in:\n%s", got, wire)
	}

	dec, err := gcf.DecodeGeneric(wire)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, _ := dec.([]any)
	if len(got) != 20 {
		t.Fatalf("decoded row count = %d, want 20", len(got))
	}
	// Column order is preserved (not alphabetized) end to end.
	first, _ := got[0].(*gcf.OrderedMap)
	if keys := first.Keys(); len(keys) != 3 || keys[0] != "id" || keys[1] != "name" || keys[2] != "region" {
		t.Errorf("column order not preserved: %v", first.Keys())
	}
}

func TestEncodeGCFToolResult_NeverGrowTinyResult(t *testing.T) {
	r := orderedmap.Row{}
	r.Add("n", int64(1))
	if _, ok := EncodeGCFToolResult([]any{r}); ok {
		t.Error("a tiny single-cell result should fall back to JSON (never-grow), got GCF")
	}
}

func TestEncodeGCFToolResult_PreservesInt64AbovePow53(t *testing.T) {
	out := make([]any, 20)
	for i := range out {
		r := orderedmap.Row{}
		r.Add("id", int64(9007199254740993)) // 2^53 + 1, lost by a float64 round-trip
		r.Add("name", "x")
		out[i] = r
	}
	wire, ok := EncodeGCFToolResult(out)
	if !ok {
		t.Fatal("expected GCF")
	}
	if !strings.Contains(wire, "9007199254740993") || strings.Contains(wire, "9.007") {
		t.Errorf("int64 not preserved exactly: %q", wire)
	}
}

func TestScalarEqualNoInt64FloatPrecisionMasking(t *testing.T) {
	// 2^53 + 1 is not exactly representable as a float64; it must not be reported
	// equal to the float64 it would round to, or a lossy encoding could pass the
	// round-trip guard.
	big := int64(1<<53 + 1)
	rounded := float64(1 << 53)
	if scalarEqual(big, rounded) || scalarEqual(rounded, big) {
		t.Errorf("a large int64 (%d) must not equal a rounded float64 (%v)", big, rounded)
	}
	// In-range integers still equal their exact float; non-integer floats don't.
	if !scalarEqual(int64(5), 5.0) || !scalarEqual(5.0, int64(5)) {
		t.Error("an in-range integer must equal its exact float")
	}
	if scalarEqual(int64(3), 3.5) || scalarEqual(3.5, int64(3)) {
		t.Error("an integer must not equal a non-integer float")
	}
}

func TestEncodeGCFToolResult_ByteColumnIsSafe(t *testing.T) {
	// A []byte column (bytea/BLOB) is a type gcf-go may not round-trip. Whatever it
	// does, the outcome must be safe: either declined, or a genuine lossless
	// round-trip. This exercises the fail-safe verify guard.
	out := make([]any, 20)
	for i := range out {
		r := orderedmap.Row{}
		r.Add("id", int64(i))
		r.Add("blob", []byte{0x01, 0x02, 0x03})
		out[i] = r
	}
	wire, ok := EncodeGCFToolResult(out)
	if !ok {
		return // conservative fallback to JSON: acceptable
	}
	converted := make([]any, len(out))
	for i, r := range out {
		converted[i] = convertRow(r)
	}
	dec, err := gcf.DecodeGeneric(wire)
	if err != nil || !gcfValuesEqual(dec, converted) {
		t.Errorf("EncodeGCFToolResult returned ok for a wire that is not a lossless round-trip")
	}
}
