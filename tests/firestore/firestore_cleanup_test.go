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

package firestore

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestShouldSweepFirestoreCollection(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	staleUnix := now.Add(-orphanedCollectionTTL - time.Second).Unix()
	freshUnix := now.Add(-orphanedCollectionTTL + time.Minute).Unix()
	boundaryUnix := now.Add(-orphanedCollectionTTL).Unix()

	tcs := []struct {
		name           string
		collectionName string
		want           bool
	}{
		{
			name:           "stale timestamped test collection is swept",
			collectionName: fmt.Sprintf("test_collection_%d_abcdef0123456789", staleUnix),
			want:           true,
		},
		{
			name:           "fresh timestamped test collection is kept",
			collectionName: fmt.Sprintf("test_collection_%d_abcdef0123456789", freshUnix),
			want:           false,
		},
		{
			name:           "exact TTL boundary is kept",
			collectionName: fmt.Sprintf("test_collection_%d_abcdef0123456789", boundaryUnix),
			want:           false,
		},
		{
			name:           "un-timestamped collection is kept to protect concurrent runs",
			collectionName: "test_collection_abcdef0123456789abcdef0123456789",
			want:           false,
		},
		{
			name:           "emulator collection is kept",
			collectionName: "test_collection_emu_abcdef0123456789",
			want:           false,
		},
		{
			name:           "non-test collection is never swept",
			collectionName: "users",
			want:           false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSweepFirestoreCollection(tc.collectionName, now); got != tc.want {
				t.Errorf("shouldSweepFirestoreCollection(%q) = %v, want %v", tc.collectionName, got, tc.want)
			}
		})
	}
}

func TestIsFirestoreNotFound(t *testing.T) {
	if isFirestoreNotFound(nil) {
		t.Error("expected nil error not to match NotFound")
	}
	if !isFirestoreNotFound(status.Error(codes.NotFound, "doc missing")) {
		t.Error("expected gRPC NotFound to match")
	}
	if !isFirestoreNotFound(fmt.Errorf("wrap: %w", status.Error(codes.NotFound, "missing"))) {
		t.Error("expected wrapped gRPC NotFound to match")
	}
	if isFirestoreNotFound(status.Error(codes.PermissionDenied, "denied")) {
		t.Error("expected PermissionDenied not to match")
	}
	if isFirestoreNotFound(errors.New("generic")) {
		t.Error("expected generic error not to match")
	}
}

func TestFirestoreClientCleanupLIFO(t *testing.T) {
	t.Setenv(firestoreEmulatorHostEnv, "127.0.0.1:1")
	var events []string

	t.Run("collection cleanup runs before client close", func(t *testing.T) {
		_ = newFirestoreTestClient(t, "test-project", "")
		t.Cleanup(func() {
			events = append(events, "after-client-close-registered")
		})
		t.Cleanup(func() {
			events = append(events, "collection-cleanup")
		})
	})

	want := []string{"collection-cleanup", "after-client-close-registered"}
	if !slices.Equal(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}
