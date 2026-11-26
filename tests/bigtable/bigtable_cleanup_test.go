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

package bigtable

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestShouldCleanupBigtableResource(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	staleUnix := now.Add(-orphanedResourceTTL - time.Second).Unix()
	freshUnix := now.Add(-orphanedResourceTTL + time.Minute).Unix()
	boundaryUnix := now.Add(-orphanedResourceTTL).Unix()

	tcs := []struct {
		name     string
		resource string
		prefixes []string
		want     bool
	}{
		{
			name:     "stale timestamped table is swept",
			resource: fmt.Sprintf("param_table_%d_a1b2c3d4", staleUnix),
			prefixes: integrationTablePrefixes,
			want:     true,
		},
		{
			name:     "fresh timestamped table is kept",
			resource: fmt.Sprintf("param_table_%d_a1b2c3d4", freshUnix),
			prefixes: integrationTablePrefixes,
			want:     false,
		},
		{
			name:     "exact TTL boundary is kept",
			resource: fmt.Sprintf("admin_test_table_%d_a1b2c3d4", boundaryUnix),
			prefixes: integrationTablePrefixes,
			want:     false,
		},
		{
			name:     "stale test instance is swept",
			resource: fmt.Sprintf("testi-%d-a1b2c3d4", staleUnix),
			prefixes: []string{"testi-"},
			want:     true,
		},
		{
			name:     "legacy un-timestamped table is swept",
			resource: "param_table_legacyuuidonly",
			prefixes: integrationTablePrefixes,
			want:     true,
		},
		{
			name:     "legacy non-numeric prefix segment is swept",
			resource: "param_table_notatimestamp_abc",
			prefixes: integrationTablePrefixes,
			want:     true,
		},
		{
			name:     "non-test table is preserved",
			resource: "prod_customer_events",
			prefixes: integrationTablePrefixes,
			want:     false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldCleanupBigtableResource(tc.resource, now, tc.prefixes...); got != tc.want {
				t.Errorf("shouldCleanupBigtableResource(%q) = %v, want %v", tc.resource, got, tc.want)
			}
		})
	}
}

func TestIsBigtableNotFound(t *testing.T) {
	if isBigtableNotFound(nil) {
		t.Error("expected nil error not to match NotFound")
	}
	if !isBigtableNotFound(status.Error(codes.NotFound, "table not found")) {
		t.Error("expected gRPC NotFound to match")
	}
	if !isBigtableNotFound(fmt.Errorf("wrapped: %w", status.Error(codes.NotFound, "missing"))) {
		t.Error("expected wrapped gRPC NotFound to match")
	}
	if isBigtableNotFound(status.Error(codes.PermissionDenied, "denied")) {
		t.Error("expected PermissionDenied not to match NotFound")
	}
	if isBigtableNotFound(errors.New("plain error")) {
		t.Error("expected plain error not to match NotFound")
	}
}

func TestRegisterBigtableCleanupLIFOAndNotFound(t *testing.T) {
	var events []string

	t.Run("logical view deletes before table and NotFound is ignored", func(t *testing.T) {
		t.Cleanup(func() {
			events = append(events, "close-clients")
		})
		registerBigtableCleanup(t, "instance", "testi-already-gone", func(ctx context.Context) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Error("expected bounded cleanup context with deadline")
			}
			events = append(events, "delete-instance-notfound")
			return status.Error(codes.NotFound, "already deleted")
		})
		registerBigtableCleanup(t, "table", "admin_test_table", func(context.Context) error {
			events = append(events, "delete-table")
			return nil
		})
		registerBigtableCleanup(t, "logical view", "admin_test_view", func(context.Context) error {
			events = append(events, "delete-view")
			return nil
		})
	})

	want := []string{
		"delete-view",
		"delete-table",
		"delete-instance-notfound",
		"close-clients",
	}
	if !slices.Equal(events, want) {
		t.Fatalf("cleanup order = %v, want %v", events, want)
	}
}

func TestRunBoundedBigtableSweep(t *testing.T) {
	t.Run("empty slice is a no-op", func(t *testing.T) {
		called := false
		runBoundedBigtableSweep(nil, func(string) { called = true })
		if called {
			t.Fatal("expected callback not to run on empty slice")
		}
	})

	t.Run("single item runs inline", func(t *testing.T) {
		var got []string
		runBoundedBigtableSweep([]string{"only"}, func(s string) { got = append(got, s) })
		if !slices.Equal(got, []string{"only"}) {
			t.Fatalf("got = %v, want [only]", got)
		}
	})

	t.Run("multiple items process every element exactly once", func(t *testing.T) {
		items := make([]string, 15)
		for i := range items {
			items[i] = fmt.Sprintf("tbl-%02d", i)
		}
		var (
			mu   sync.Mutex
			seen = make(map[string]int)
		)
		runBoundedBigtableSweep(items, func(s string) {
			mu.Lock()
			seen[s]++
			mu.Unlock()
		})
		if len(seen) != len(items) {
			t.Fatalf("processed %d distinct items, want %d", len(seen), len(items))
		}
		for _, item := range items {
			if seen[item] != 1 {
				t.Errorf("item %q processed %d times, want 1", item, seen[item])
			}
		}
	})
}

func TestConvertToBytes(t *testing.T) {
	for _, v := range []int{0, 1, 4, 21, 100, -1} {
		got := convertToBytes(v)
		if len(got) != 8 {
			t.Fatalf("convertToBytes(%d) length = %d, want 8", v, len(got))
		}
		decoded := int(int64(binary.BigEndian.Uint64(got)))
		if decoded != v {
			t.Errorf("decoded convertToBytes(%d) = %d, want %d", v, decoded, v)
		}
	}
}
