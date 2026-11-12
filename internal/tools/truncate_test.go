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

package tools

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func listOf(n int) []any {
	rows := make([]any, n)
	for i := range rows {
		rows[i] = map[string]any{"id": i}
	}
	return rows
}

func TestCapResultUncapped(t *testing.T) {
	rows := listOf(5)
	got, trunc := CapResult(rows, 0, 0)
	if trunc != nil {
		t.Fatalf("expected nil truncation, got %+v", trunc)
	}
	if !reflect.DeepEqual(got, rows) {
		t.Errorf("result changed without caps: %v", got)
	}
}

func TestCapResultRowCap(t *testing.T) {
	got, trunc := CapResult(listOf(10), 3, 0)
	rows := got.([]any)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	want := &Truncation{Truncated: true, AppliedLimit: "maxRows", ReturnedRows: 3, TotalRows: 10}
	if !reflect.DeepEqual(trunc, want) {
		t.Errorf("truncation: got %+v, want %+v", trunc, want)
	}
}

func TestCapResultRowCapNotExceeded(t *testing.T) {
	got, trunc := CapResult(listOf(3), 3, 0)
	if trunc != nil {
		t.Fatalf("cap equal to size must not truncate, got %+v", trunc)
	}
	if len(got.([]any)) != 3 {
		t.Errorf("rows dropped without truncation")
	}
}

func TestCapResultByteCapOnList(t *testing.T) {
	// Each row marshals to {"id":N} = 8 bytes for single-digit ids.
	got, trunc := CapResult(listOf(9), 0, 20)
	rows := got.([]any)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows within 20 bytes, got %d", len(rows))
	}
	if trunc == nil || trunc.AppliedLimit != "maxResponseBytes" {
		t.Fatalf("unexpected truncation: %+v", trunc)
	}
	if trunc.ReturnedRows != 2 || trunc.TotalRows != 9 || trunc.ReturnedBytes != 16 || trunc.LimitBytes != 20 {
		t.Errorf("unexpected truncation detail: %+v", trunc)
	}
}

func TestCapResultBothCaps(t *testing.T) {
	got, trunc := CapResult(listOf(9), 5, 20)
	rows := got.([]any)
	if len(rows) != 2 {
		t.Fatalf("expected byte cap to fire after row cap, got %d rows", len(rows))
	}
	if trunc == nil || trunc.AppliedLimit != "maxRows,maxResponseBytes" {
		t.Fatalf("unexpected truncation: %+v", trunc)
	}
	if trunc.TotalRows != 9 || trunc.ReturnedRows != 2 {
		t.Errorf("unexpected truncation detail: %+v", trunc)
	}
}

func TestCapResultFirstRowOverBudget(t *testing.T) {
	rows := []any{map[string]any{"blob": strings.Repeat("x", 100)}}
	got, trunc := CapResult(rows, 0, 10)
	if len(got.([]any)) != 0 {
		t.Fatalf("expected zero rows when the first row exceeds the budget")
	}
	if trunc == nil || !trunc.Truncated {
		t.Fatalf("expected truncation notice, got %+v", trunc)
	}
}

func TestCapResultNonListByteCap(t *testing.T) {
	result := map[string]any{"blob": strings.Repeat("x", 100)}
	got, trunc := CapResult(result, 0, 24)
	wrapped, ok := got.(map[string]string)
	if !ok {
		t.Fatalf("expected wrapped truncated result, got %T", got)
	}
	fragment, ok := wrapped["truncatedResult"]
	if !ok {
		t.Fatalf("expected truncatedResult key, got %v", wrapped)
	}
	if len(fragment) > 24 {
		t.Errorf("truncated result exceeds cap: %d bytes", len(fragment))
	}
	want := &Truncation{Truncated: true, AppliedLimit: "maxResponseBytes", ReturnedBytes: len(fragment), LimitBytes: 24}
	if !reflect.DeepEqual(trunc, want) {
		t.Errorf("truncation: got %+v, want %+v", trunc, want)
	}
}

// The truncated fragment is not valid JSON on its own, so the capped result
// must still marshal cleanly and exactly once — a raw string or a
// json.RawMessage of the fragment would double-escape or fail to marshal.
func TestCapResultNonListRemainsMarshalable(t *testing.T) {
	result := map[string]any{"blob": strings.Repeat("x", 100)}
	got, trunc := CapResult(result, 0, 24)
	if trunc == nil {
		t.Fatal("expected truncation")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("capped result must marshal without error, got %v", err)
	}
	var roundTripped map[string]string
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("capped result must be valid JSON, got %v (%s)", err, encoded)
	}
	if !strings.HasPrefix(roundTripped["truncatedResult"], `{"blob":"xxx`) {
		t.Errorf("expected the leading fragment to survive, got %q", roundTripped["truncatedResult"])
	}
}

func TestCapResultNonListWithinCap(t *testing.T) {
	result := map[string]any{"ok": true}
	got, trunc := CapResult(result, 5, 1000)
	if trunc != nil {
		t.Fatalf("expected nil truncation, got %+v", trunc)
	}
	if !reflect.DeepEqual(got, result) {
		t.Errorf("result changed: %v", got)
	}
}

func TestTruncateUTF8Boundary(t *testing.T) {
	b := []byte("aé") // 'é' is 2 bytes starting at index 1
	if got := truncateUTF8(b, 2); string(got) != "a" {
		t.Errorf("expected rune-safe cut to %q, got %q", "a", got)
	}
	if got := truncateUTF8(b, 3); string(got) != "aé" {
		t.Errorf("expected full string, got %q", got)
	}
}

func TestResolveCap(t *testing.T) {
	intPtr := func(i int) *int { return &i }

	tcs := []struct {
		name          string
		declared      *int
		serverDefault int
		want          int
	}{
		{name: "undeclared inherits the server default", declared: nil, serverDefault: 100, want: 100},
		{name: "undeclared with no default is uncapped", declared: nil, serverDefault: 0, want: 0},
		{name: "declared overrides the server default", declared: intPtr(10), serverDefault: 100, want: 10},
		{name: "declared 0 opts out of the server default", declared: intPtr(0), serverDefault: 100, want: 0},
		{name: "declared applies with no server default", declared: intPtr(10), serverDefault: 0, want: 10},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveCap(tc.declared, tc.serverDefault); got != tc.want {
				t.Errorf("ResolveCap(%v, %d) = %d, want %d", tc.declared, tc.serverDefault, got, tc.want)
			}
		})
	}
}

// TestCapResultForToolAppliesServerDefaults checks the precedence the MCP
// handlers rely on: a tool that declares nothing is capped by the server-wide
// defaults it is handed, and one that declares its own is not.
func TestCapResultForToolAppliesServerDefaults(t *testing.T) {
	intPtr := func(i int) *int { return &i }
	rows := []any{"a", "b", "c"}

	got, trunc := CapResultForTool(capStub{}, rows, 2, 0)
	if trunc == nil || trunc.ReturnedRows != 2 || trunc.TotalRows != 3 {
		t.Errorf("expected the server default to cap at 2 of 3 rows, got %v (%+v)", got, trunc)
	}

	got, trunc = CapResultForTool(capStub{maxRows: intPtr(0)}, rows, 2, 0)
	if trunc != nil {
		t.Errorf("expected an explicit 0 to opt out of the server default, got %+v", trunc)
	}
	if !reflect.DeepEqual(got, rows) {
		t.Errorf("expected the full result, got %v", got)
	}
}

// capStub stands in for a tool's declared caps.
type capStub struct {
	maxRows  *int
	maxBytes *int
}

func (c capStub) GetMaxRows() *int          { return c.maxRows }
func (c capStub) GetMaxResponseBytes() *int { return c.maxBytes }
