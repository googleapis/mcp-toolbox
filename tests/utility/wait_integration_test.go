// Copyright 2025 Google LLC
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

package utility

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

func setupWaitTest(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	config := map[string]any{
		"tools": map[string]any{
			"my-wait-for-tool": map[string]any{
				"type": "wait", "description": "Wait for a specified duration.", "timeout": "30s",
			},
		},
	}
	cmd, cleanup, err := tests.StartCmd(ctx, config)
	if err != nil {
		t.Fatalf("start toolbox: %v", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		cmd.Stop()
		waitCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := cmd.Wait(waitCtx); err != nil {
			t.Errorf("stop toolbox: %v", err)
		}
		cmd.Close()
	})
	waitCtx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	if out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out); err != nil {
		t.Fatalf("toolbox did not start: %v\n%s", err, out)
	}
}
