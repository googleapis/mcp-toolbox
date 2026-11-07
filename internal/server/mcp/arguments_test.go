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

package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	v20241105 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20241105"
	v20250326 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20250326"
	v20250618 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20250618"
	v20251125 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20251125"
	v20260728 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20260728"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestToolCallIntegerArguments(t *testing.T) {
	logger, err := log.NewStdLogger(os.Stderr, os.Stderr, "warn")
	if err != nil {
		t.Fatal(err)
	}
	ctx := util.WithLogger(context.Background(), logger)
	ctx = util.WithToolboxVersionKey(ctx, "0.0.0")
	tool := testutils.NewMockTool("echo", "Echo an integer", "", []parameters.Parameter{
		parameters.NewIntParameter("value", "Value to echo"),
	}, false, false)
	tool.ReturnParamsInInvoke = true
	tools, prompts, resources, templates, groups := testutils.SetUpPrimitives(t, []testutils.MockTool{tool, testutils.MockTool1}, nil, nil, nil)
	mgr := primitives.NewPrimitiveManager(nil, nil, nil, tools, prompts, resources, templates, groups)
	versions := []struct {
		name    string
		process func(context.Context, jsonrpc.RequestId, string, group.Group, *primitives.PrimitiveManager, []byte, http.Header) (any, error)
	}{
		{"2024-11-05", v20241105.ProcessMethod},
		{"2025-03-26", v20250326.ProcessMethod},
		{"2025-06-18", v20250618.ProcessMethod},
		{"2025-11-25", v20251125.ProcessMethod},
		{"2026-07-28", v20260728.ProcessMethod},
	}
	for _, version := range versions {
		t.Run(version.name, func(t *testing.T) {
			for _, tc := range []struct{ input, want string }{
				{"9007199254740993", "9007199254740993"},
				{"1e3", "1000"},
				{"1.0", "1"},
			} {
				t.Run(tc.input, func(t *testing.T) {
					body := []byte(fmt.Sprintf(`{"method":"tools/call","params":{"name":"echo","arguments":{"value":%s},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"},"io.modelcontextprotocol/clientCapabilities":{}}}}`, tc.input))
					got, err := version.process(ctx, 1, "tools/call", groups[""], mgr, body, nil)
					if err != nil {
						t.Fatal(err)
					}
					encoded, err := json.Marshal(got)
					if err != nil {
						t.Fatal(err)
					}
					var response struct {
						Result struct {
							Content []struct{ Text string }
							IsError bool
						}
					}
					if err := json.Unmarshal(encoded, &response); err != nil {
						t.Fatal(err)
					}
					if response.Result.IsError || len(response.Result.Content) != 2 || response.Result.Content[1].Text != tc.want {
						t.Fatalf("got %s, want echoed integer %s", encoded, tc.want)
					}
				})
			}
		})
	}
}
