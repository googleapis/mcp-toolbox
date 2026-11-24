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

package spanner

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNewSpannerTestObjectName(t *testing.T) {
	now := time.Date(2026, 11, 24, 17, 0, 0, 0, time.UTC)
	got := newSpannerTestObjectName("param_table_", now)
	if got != strings.ToLower(got) {
		t.Fatalf("newSpannerTestObjectName(%q) = %q, want lowercase identifier", "param_table_", got)
	}
	wantPrefix := fmt.Sprintf("param_table_%d_", now.Unix())
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("newSpannerTestObjectName(%q) = %q, want prefix %q", "param_table_", got, wantPrefix)
	}
	if suffix := strings.TrimPrefix(got, wantPrefix); len(suffix) != 12 {
		t.Fatalf("newSpannerTestObjectName(%q) hex suffix = %q (len %d), want 12 hex chars", "param_table_", suffix, len(suffix))
	}
	if muts := getSpannerParamSeedMutations(got); len(muts) != 4 {
		t.Fatalf("len(getSpannerParamSeedMutations) = %d, want 4", len(muts))
	}
	if muts := getSpannerAuthSeedMutations(got); len(muts) != 2 {
		t.Fatalf("len(getSpannerAuthSeedMutations) = %d, want 2", len(muts))
	}
	if shouldSweepSpannerObject(got, now, spannerTestTablePrefixes...) {
		t.Errorf("freshly generated object %q should not be swept at creation time", got)
	}
	if !shouldSweepSpannerObject(got, now.Add(orphanedSpannerObjectTTL+time.Second), spannerTestTablePrefixes...) {
		t.Errorf("object %q should be swept once older than %v", got, orphanedSpannerObjectTTL)
	}
}

func TestShouldSweepSpannerObject(t *testing.T) {
	now := time.Date(2026, 11, 24, 17, 0, 0, 0, time.UTC)
	staleUnix := now.Add(-orphanedSpannerObjectTTL - time.Second).Unix()
	freshUnix := now.Add(-orphanedSpannerObjectTTL + time.Minute).Unix()
	boundaryUnix := now.Add(-orphanedSpannerObjectTTL).Unix()

	tcs := []struct {
		name     string
		object   string
		prefixes []string
		want     bool
	}{
		{
			name:     "stale timestamped table is swept",
			object:   fmt.Sprintf("param_table_%d_a1b2c3d4e5f6", staleUnix),
			prefixes: spannerTestTablePrefixes,
			want:     true,
		},
		{
			name:     "stale timestamped graph is swept",
			object:   fmt.Sprintf("graph_%d_a1b2c3d4e5f6", staleUnix),
			prefixes: spannerTestGraphPrefixes,
			want:     true,
		},
		{
			name:     "fresh timestamped table is preserved",
			object:   fmt.Sprintf("vector_table_%d_a1b2c3d4e5f6", freshUnix),
			prefixes: spannerTestTablePrefixes,
			want:     false,
		},
		{
			name:     "exact TTL boundary is preserved",
			object:   fmt.Sprintf("edge_table_%d_a1b2c3d4e5f6", boundaryUnix),
			prefixes: spannerTestTablePrefixes,
			want:     false,
		},
		{
			name:     "un-timestamped concurrent run table is preserved",
			object:   "param_table_9f8e7d6c5b4a3210fedcba9876543210",
			prefixes: spannerTestTablePrefixes,
			want:     false,
		},
		{
			name:     "non-numeric timestamp segment is preserved",
			object:   "node_table_notatimestamp_abc123",
			prefixes: spannerTestTablePrefixes,
			want:     false,
		},
		{
			name:     "non-positive timestamp is preserved",
			object:   "auth_table_0_abc123",
			prefixes: spannerTestTablePrefixes,
			want:     false,
		},
		{
			name:     "unrelated user table is preserved",
			object:   "customer_accounts",
			prefixes: spannerTestTablePrefixes,
			want:     false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSweepSpannerObject(tc.object, now, tc.prefixes...); got != tc.want {
				t.Errorf("shouldSweepSpannerObject(%q) = %v, want %v", tc.object, got, tc.want)
			}
		})
	}
}

func TestOrderSpannerDropStatements(t *testing.T) {
	graphs := []string{"graph_100_b", "graph_100_a"}
	tables := []string{
		"node_table_100_a",
		"param_table_100_a",
		"edge_table_100_b",
		"edge_table_100_a",
		"auth_table_100_a",
	}

	gotGoogleSQL := orderSpannerDropStatements(graphs, tables, quoteSpannerGoogleSQLIdent)
	wantGoogleSQL := []string{
		"DROP PROPERTY GRAPH IF EXISTS `graph_100_a`",
		"DROP PROPERTY GRAPH IF EXISTS `graph_100_b`",
		"DROP TABLE IF EXISTS `edge_table_100_a`",
		"DROP TABLE IF EXISTS `edge_table_100_b`",
		"DROP TABLE IF EXISTS `auth_table_100_a`",
		"DROP TABLE IF EXISTS `node_table_100_a`",
		"DROP TABLE IF EXISTS `param_table_100_a`",
	}
	if !slices.Equal(gotGoogleSQL, wantGoogleSQL) {
		t.Fatalf("orderSpannerDropStatements (GoogleSQL) =\n%v\nwant:\n%v", gotGoogleSQL, wantGoogleSQL)
	}

	gotPG := orderSpannerDropStatements(nil, []string{"vector_table_100_a", "param_table_100_a"}, quoteSpannerPostgresIdent)
	wantPG := []string{
		`DROP TABLE IF EXISTS "param_table_100_a"`,
		`DROP TABLE IF EXISTS "vector_table_100_a"`,
	}
	if !slices.Equal(gotPG, wantPG) {
		t.Fatalf("orderSpannerDropStatements (PostgreSQL) =\n%v\nwant:\n%v", gotPG, wantPG)
	}
}

func TestIsSpannerNotFound(t *testing.T) {
	if isSpannerNotFound(nil) {
		t.Error("expected nil error not to match NotFound")
	}
	if !isSpannerNotFound(status.Error(codes.NotFound, "database not found")) {
		t.Error("expected gRPC NotFound to match")
	}
	if isSpannerNotFound(status.Error(codes.PermissionDenied, "denied")) {
		t.Error("expected PermissionDenied not to match NotFound")
	}
	if isSpannerNotFound(errors.New("plain error")) {
		t.Error("expected plain error not to match NotFound")
	}
}

func TestSpannerCleanupLIFO(t *testing.T) {
	var events []string
	var ddlRuns int

	t.Run("teardown executes in strict LIFO order before clients and container close", func(t *testing.T) {
		t.Cleanup(func() {
			events = append(events, "terminate-container")
		})
		t.Cleanup(func() {
			events = append(events, "close-admin-client")
		})
		t.Cleanup(func() {
			events = append(events, "close-data-client")
		})

		var once sync.Once
		teardownDdl := func() {
			once.Do(func() {
				ddlRuns++
				events = append(events, "drop-schema-ddl")
			})
		}
		t.Cleanup(teardownDdl)

		t.Cleanup(func() {
			events = append(events, "stop-server")
		})
	})

	if ddlRuns != 1 {
		t.Fatalf("ddlRuns = %d, want 1", ddlRuns)
	}
	want := []string{
		"stop-server",
		"drop-schema-ddl",
		"close-data-client",
		"close-admin-client",
		"terminate-container",
	}
	if !slices.Equal(events, want) {
		t.Fatalf("cleanup order = %v, want %v", events, want)
	}
}
