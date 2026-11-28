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

package falkordb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	falkordb "github.com/FalkorDB/falkordb-go/v2"
	"github.com/redis/go-redis/v9"
)

type mockRedisHook struct {
	handler         func(ctx context.Context, cmd redis.Cmder) error
	onPipelineBatch func(size int)
}

func (h *mockRedisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, errors.New("unexpected network dial in unit test")
	}
}

func (h *mockRedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		return h.handler(ctx, cmd)
	}
}

func (h *mockRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		if h.onPipelineBatch != nil {
			h.onPipelineBatch(len(cmds))
		}
		var firstErr error
		for _, cmd := range cmds {
			if err := h.handler(ctx, cmd); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
}

func newMockFalkorDBClient(t *testing.T, hook *mockRedisHook) (*falkordb.FalkorDB, *redis.Client) {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	rdb.AddHook(hook)
	return &falkordb.FalkorDB{Conn: rdb}, rdb
}

func TestShouldSweepFalkorDBGraph(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

	tcs := []struct {
		name      string
		graphName string
		want      bool
	}{
		{
			name:      "stale override test graph is swept",
			graphName: newTestGraphName("override", "abc123def456", now.Add(-(orphanedGraphTTL + time.Minute))),
			want:      true,
		},
		{
			name:      "stale default test graph is swept",
			graphName: newTestGraphName("default", "abc123def456", now.Add(-(orphanedGraphTTL + 2*time.Hour))),
			want:      true,
		},
		{
			name:      "fresh test graph is kept",
			graphName: newTestGraphName("override", "abc123def456", now.Add(-(orphanedGraphTTL - time.Minute))),
			want:      false,
		},
		{
			name:      "exact TTL boundary is kept",
			graphName: newTestGraphName("override", "abc123def456", now.Add(-orphanedGraphTTL)),
			want:      false,
		},
		{
			name:      "non-test graph is never swept",
			graphName: "production_knowledge_graph",
			want:      false,
		},
		{
			name:      "legacy unprefixed test graph without timestamp is kept",
			graphName: "toolbox_test_graph",
			want:      false,
		},
		{
			name:      "malformed timestamp in test prefix is kept",
			graphName: "toolbox_it_notanumber_override_abc123",
			want:      false,
		},
		{
			name:      "zero timestamp in test prefix is kept",
			graphName: "toolbox_it_0_override_abc123",
			want:      false,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSweepFalkorDBGraph(tc.graphName, now); got != tc.want {
				t.Errorf("shouldSweepFalkorDBGraph(%q, %v) = %v, want %v", tc.graphName, now, got, tc.want)
			}
		})
	}
}

func TestSweepOrphanedFalkorDBGraphs(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	staleOverride := newTestGraphName("override", "stale0000001", now.Add(-4*time.Hour))
	staleVanished := newTestGraphName("default", "stale0000002", now.Add(-5*time.Hour))
	freshOverride := newTestGraphName("override", "fresh0000001", now.Add(-15*time.Minute))

	var (
		mu              sync.Mutex
		deleted         []string
		pipelineBatches atomic.Int32
	)

	client, rdb := newMockFalkorDBClient(t, &mockRedisHook{
		onPipelineBatch: func(size int) {
			pipelineBatches.Add(1)
			if size != 2 {
				t.Errorf("expected 2 stale deletes batched into 1 pipeline, got batch size %d", size)
			}
		},
		handler: func(ctx context.Context, cmd redis.Cmder) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Errorf("expected bounded context deadline on %v", cmd.Args())
			}
			args := cmd.Args()
			rawCmd, ok := cmd.(*redis.Cmd)
			if !ok {
				return fmt.Errorf("unexpected cmd type %T", cmd)
			}
			switch fmt.Sprint(args[0]) {
			case "GRAPH.LIST":
				rawCmd.SetVal([]any{
					staleOverride,
					staleVanished,
					freshOverride,
					"toolbox_it_unlabeled",
					"production_graph",
				})
				return nil
			case "GRAPH.DELETE":
				graphName := fmt.Sprint(args[1])
				mu.Lock()
				deleted = append(deleted, graphName)
				mu.Unlock()
				if graphName == staleVanished {
					err := errors.New("ERR Invalid graph operation on empty key")
					rawCmd.SetErr(err)
					return err
				}
				rawCmd.SetVal("Graph removed")
				return nil
			default:
				err := fmt.Errorf("unexpected command %v", args)
				rawCmd.SetErr(err)
				return err
			}
		},
	})
	t.Cleanup(func() { _ = rdb.Close() })

	sweepCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	sweepOrphanedFalkorDBGraphsWithClient(sweepCtx, client, now)

	wantDeleted := []string{staleOverride, staleVanished}
	if !slices.Equal(deleted, wantDeleted) {
		t.Errorf("deleted graphs = %v, want %v", deleted, wantDeleted)
	}
	if got := pipelineBatches.Load(); got != 1 {
		t.Errorf("expected 1 pipelined delete batch, got %d", got)
	}
}

func TestExecFalkorDBCleanupBatch(t *testing.T) {
	client, rdb := newMockFalkorDBClient(t, &mockRedisHook{
		handler: func(ctx context.Context, cmd redis.Cmder) error {
			args := cmd.Args()
			rawCmd := cmd.(*redis.Cmd)
			switch fmt.Sprint(args[0]) {
			case "GRAPH.DELETE":
				switch fmt.Sprint(args[1]) {
				case "missing_graph":
					err := errors.New("ERR Invalid graph operation on empty key")
					rawCmd.SetErr(err)
					return err
				case "denied_graph":
					err := errors.New("NOPERM user has no permissions to run 'graph.delete'")
					rawCmd.SetErr(err)
					return err
				default:
					rawCmd.SetVal("Graph removed")
					return nil
				}
			case "GRAPH.QUERY":
				cypher := fmt.Sprint(args[2])
				switch {
				case strings.Contains(cypher, "MissingIndex"):
					err := errors.New("ERR Unable to drop index on :MissingIndex(title): no such index.")
					rawCmd.SetErr(err)
					return err
				case strings.Contains(cypher, "DeniedLabel"):
					err := errors.New("NOPERM user has no permissions to run 'graph.query'")
					rawCmd.SetErr(err)
					return err
				default:
					rawCmd.SetVal([]any{})
					return nil
				}
			default:
				err := fmt.Errorf("unexpected command %v", args)
				rawCmd.SetErr(err)
				return err
			}
		},
	})
	t.Cleanup(func() { _ = rdb.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := execFalkorDBCleanupBatch(ctx, client, []any{"GRAPH.DELETE", "existing_graph"}); err != nil {
		t.Errorf("single existing graph delete unexpected error: %v", err)
	}
	if err := execFalkorDBCleanupBatch(ctx, client, []any{"GRAPH.DELETE", "missing_graph"}); err != nil {
		t.Errorf("missing graph delete should ignore empty-key error, got: %v", err)
	}
	if err := execFalkorDBCleanupBatch(ctx, client, []any{"GRAPH.DELETE", "denied_graph"}); err == nil {
		t.Errorf("denied graph delete = nil, want error on NOPERM")
	}

	// Pipeline where first command returns ignorable NotFound and second succeeds.
	if err := execFalkorDBCleanupBatch(ctx, client,
		[]any{"GRAPH.QUERY", "g1", "DROP INDEX FOR (n:MissingIndex) ON (n.title)", "--compact"},
		[]any{"GRAPH.QUERY", "g1", "MATCH (n:OkLabel) DETACH DELETE n", "--compact"},
	); err != nil {
		t.Errorf("pipeline with ignorable MissingIndex error failed: %v", err)
	}

	// Pipeline where first command returns ignorable NotFound and second fails with NOPERM.
	if err := execFalkorDBCleanupBatch(ctx, client,
		[]any{"GRAPH.QUERY", "g1", "DROP INDEX FOR (n:MissingIndex) ON (n.title)", "--compact"},
		[]any{"GRAPH.QUERY", "g1", "MATCH (n:DeniedLabel) DETACH DELETE n", "--compact"},
	); err == nil {
		t.Errorf("pipeline with DeniedLabel after MissingIndex = nil, want error on NOPERM")
	}
}

func TestFalkorDBCleanupLIFOAndPipelining(t *testing.T) {
	var (
		mu              sync.Mutex
		clientOpen      = true
		recorded        []string
		pipelineBatches []int
	)

	client, rdb := newMockFalkorDBClient(t, &mockRedisHook{
		onPipelineBatch: func(size int) {
			mu.Lock()
			pipelineBatches = append(pipelineBatches, size)
			mu.Unlock()
		},
		handler: func(ctx context.Context, cmd redis.Cmder) error {
			if err := ctx.Err(); err != nil {
				t.Errorf("cleanup command %v executed with canceled context: %v", cmd.Args(), err)
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Errorf("cleanup command %v executed without timeout deadline", cmd.Args())
			}

			mu.Lock()
			if !clientOpen {
				t.Errorf("cleanup command %v executed after client was closed", cmd.Args())
			}
			args := cmd.Args()
			switch fmt.Sprint(args[0]) {
			case "GRAPH.DELETE":
				recorded = append(recorded, "GRAPH.DELETE "+fmt.Sprint(args[1]))
			case "GRAPH.QUERY":
				recorded = append(recorded, "GRAPH.QUERY "+fmt.Sprint(args[1])+" | "+fmt.Sprint(args[2]))
			}
			mu.Unlock()

			rawCmd := cmd.(*redis.Cmd)
			if fmt.Sprint(args[0]) == "GRAPH.DELETE" && fmt.Sprint(args[1]) == "uncreated_graph" {
				err := errors.New("ERR Invalid graph operation on empty key")
				rawCmd.SetErr(err)
				return err
			}
			rawCmd.SetVal("OK")
			return nil
		},
	})

	t.Run("subtest pipelines multi-step schema cleanup and closes client last", func(t *testing.T) {
		t.Cleanup(func() {
			mu.Lock()
			clientOpen = false
			mu.Unlock()
			_ = rdb.Close()
		})

		registerFalkorDBGraphCleanup(t, client, "uncreated_graph")
		registerFalkorDBLabelCleanup(t, client, "shared_graph", "WriteTest_abc123")
		registerFalkorDBSchemaCleanup(t, client, "shared_graph", "Movie_abc123", "title", "Person_abc123", "Movie_abc123")
	})

	wantRecorded := []string{
		"GRAPH.QUERY shared_graph | DROP INDEX FOR (n:Movie_abc123) ON (n.title)",
		"GRAPH.QUERY shared_graph | MATCH (n:Person_abc123) DETACH DELETE n",
		"GRAPH.QUERY shared_graph | MATCH (n:Movie_abc123) DETACH DELETE n",
		"GRAPH.QUERY shared_graph | MATCH (n:WriteTest_abc123) DETACH DELETE n",
		"GRAPH.DELETE uncreated_graph",
	}
	if !slices.Equal(recorded, wantRecorded) {
		t.Errorf("recorded cleanup sequence = %v, want %v", recorded, wantRecorded)
	}
	if !slices.Equal(pipelineBatches, []int{3}) {
		t.Errorf("pipelineBatches = %v, want [3] (3-step schema cleanup pipelined in 1 round-trip)", pipelineBatches)
	}
	for _, entry := range recorded {
		if strings.Contains(entry, "MATCH (n) DETACH DELETE n") {
			t.Errorf("cleanup issued unscoped global wipe: %q", entry)
		}
	}
	if clientOpen {
		t.Errorf("expected client close cleanup to run last")
	}
}
