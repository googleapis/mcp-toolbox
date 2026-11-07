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

package tests_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

func reserveTestPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve test port: %v", err)
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	if err := ln.Close(); err != nil {
		t.Fatalf("failed to release reserved port: %v", err)
	}
	return port
}

func TestStartCmdSequentialKeepAliveCleanup(t *testing.T) {
	port := reserveTestPort(t)
	toolsFile := map[string]any{
		"tools": map[string]any{
			"quick-wait": map[string]any{
				"type":        "wait",
				"description": "Short wait tool for server lifecycle testing.",
				"timeout":     "5s",
			},
		},
	}
	invokeURL := fmt.Sprintf("http://127.0.0.1:%s/api/tool/quick-wait/invoke", port)

	for i := range 10 {
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, "--port", port, "--enable-api")
			if err != nil {
				t.Fatalf("iteration %d: StartCmd failed: %v", i, err)
			}
			defer cleanup()

			waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
			defer waitCancel()
			out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
			if err != nil {
				t.Logf("iteration %d toolbox logs:\n%s", i, out)
				t.Fatalf("iteration %d: server did not become ready: %v", i, err)
			}

			req, err := http.NewRequest(http.MethodPost, invokeURL, bytes.NewBufferString(`{"duration": "1ms"}`))
			if err != nil {
				t.Fatalf("iteration %d: NewRequest failed: %v", i, err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("iteration %d: POST failed (stale keep-alive or port race): %v", i, err)
			}
			_, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("iteration %d: unexpected status %d", i, resp.StatusCode)
			}

			// Verify Close and cleanup are idempotent when called multiple times.
			if i%2 == 1 {
				cmd.Close()
				cleanup()
			}
		}()
	}
}
