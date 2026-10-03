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

package dgraph

import (
	"context"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	DgraphSourceType = "dgraph"
	DgraphApiKey     = "api-key"
	DgraphUrl        = os.Getenv("DGRAPH_URL")
)

func getDgraphVars(t *testing.T) map[string]any {
	if DgraphUrl == "" {
		t.Fatal("'DGRAPH_URL' not set")
	}
	return map[string]any{
		"type":      DgraphSourceType,
		"dgraphUrl": DgraphUrl,
		"apiKey":    DgraphApiKey,
	}
}

func setupDgraphTest(t *testing.T) {
	t.Helper()
	sourceConfig := getDgraphVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)

	// Write config into a file and pass it to command
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-dgraph-instance": sourceConfig,
		},
		"tools": map[string]any{
			"my-simple-dql-tool": map[string]any{
				"type":        "dgraph-dql",
				"source":      "my-dgraph-instance",
				"description": "Simple tool to test end to end functionality.",
				"statement":   "{result(func: uid(0x0)) {constant: math(1)}}",
				"isQuery":     true,
				"timeout":     "20s",
				"parameters":  []any{},
			},
		},
	}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
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

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}
