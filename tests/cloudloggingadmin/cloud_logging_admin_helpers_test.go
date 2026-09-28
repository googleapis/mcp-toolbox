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

package cloudloggingadmin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/logging/logadmin"
	"google.golang.org/api/iterator"
)

func waitForLogEntries(ctx context.Context, interval time.Duration, hasEntries func(context.Context) (bool, error)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		found, err := hasEntries(ctx)
		if err != nil {
			return err
		}
		if found {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForCloudLoggingEntries(ctx context.Context, adminClient *logadmin.Client, projectID, logName string, minEntries int) error {
	return waitForLogEntries(ctx, 5*time.Second, func(ctx context.Context) (bool, error) {
		it := adminClient.Entries(ctx, logadmin.Filter(cloudLoggingLogFilter(projectID, logName)))
		count := 0
		for {
			_, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return false, err
			}
			count++
			if count >= minEntries {
				return true, nil
			}
		}
		return false, nil
	})
}

func cloudLoggingLogFilter(projectID, logID string) string {
	return fmt.Sprintf(`logName="projects/%s/logs/%s"`, projectID, strings.ReplaceAll(logID, "/", "%2F"))
}

func TestWaitForLogEntriesPollsUntilVisible(t *testing.T) {
	attempts := 0
	err := waitForLogEntries(context.Background(), time.Nanosecond, func(context.Context) (bool, error) {
		attempts++
		return attempts == 3, nil
	})
	if err != nil {
		t.Fatalf("waitForLogEntries returned error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("waitForLogEntries called checker %d times, want 3", attempts)
	}
}

func TestWaitForLogEntriesReturnsQueryError(t *testing.T) {
	queryErr := errors.New("query failed")
	err := waitForLogEntries(context.Background(), time.Nanosecond, func(context.Context) (bool, error) {
		return false, queryErr
	})
	if !errors.Is(err, queryErr) {
		t.Fatalf("waitForLogEntries returned %v, want %v", err, queryErr)
	}
}

func TestWaitForLogEntriesReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForLogEntries(ctx, time.Nanosecond, func(context.Context) (bool, error) {
		return false, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForLogEntries returned %v, want %v", err, context.Canceled)
	}
}

func TestCloudLoggingLogHelpers(t *testing.T) {
	const projectID = "test-project"
	const logID = "toolbox-integration-test-abc"

	if got, want := cloudLoggingLogFilter(projectID, logID), `logName="projects/test-project/logs/toolbox-integration-test-abc"`; got != want {
		t.Fatalf("cloudLoggingLogFilter() = %q, want %q", got, want)
	}
}

func TestTeardownTestLogsContextIgnoresParentCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	cleanupCtx, cleanupCancel := teardownTestLogsContext(parent)
	defer cleanupCancel()

	if err := cleanupCtx.Err(); err != nil {
		t.Fatalf("teardownTestLogsContext returned canceled context: %v", err)
	}
	if _, ok := cleanupCtx.Deadline(); !ok {
		t.Fatal("teardownTestLogsContext did not set a cleanup deadline")
	}
}
