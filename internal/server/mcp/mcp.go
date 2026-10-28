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

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	mcputil "github.com/googleapis/mcp-toolbox/internal/server/mcp/util"
	v20241105 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20241105"
	v20250326 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20250326"
	v20250618 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20250618"
	v20251125 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20251125"
	v20260728 "github.com/googleapis/mcp-toolbox/internal/server/mcp/v20260728"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

// ProtocolOptions contains configuration passed during server initialization to protocol handlers.
type ProtocolOptions struct {
	DisableExt []string
}

// InitializeProtocols performs version-specific protocol setup across all supported MCP versions.
func InitializeProtocols(opts ProtocolOptions) {
	v20260728.Initialize(opts.DisableExt)
}

// NotificationHandler process notifications request. It MUST NOT send a response.
// Currently Toolbox does not process any notifications.
func NotificationHandler(ctx context.Context, body []byte) error {
	var notification jsonrpc.JSONRPCNotification
	if err := json.Unmarshal(body, &notification); err != nil {
		return fmt.Errorf("invalid notification request: %w", err)
	}
	// Since we do not enforce notifications, we do not need to check the
	// `Mcp-Method` header here
	return nil
}

// ProcessMethod returns a response for the request.
// This is the Operation phase of the lifecycle for MCP client-server connections.
func ProcessMethod(ctx context.Context, mcpVersion string, id jsonrpc.RequestId, method string, g group.Group, primitiveMgr *primitives.PrimitiveManager, body []byte, header http.Header) (any, error) {
	enableDraft, ok := util.EnableDraftSpecsFromContext(ctx)
	if !ok {
		err := fmt.Errorf("unable to retrieve enableDraftSpecs from context")
		return jsonrpc.NewError(id, jsonrpc.INTERNAL_ERROR, err.Error(), nil), err
	}
	switch mcpVersion {
	case mcputil.VERSION_20260728:
		return v20260728.ProcessMethod(ctx, id, method, g, primitiveMgr, body, header)
	case mcputil.VERSION_20251125:
		return v20251125.ProcessMethod(ctx, id, method, g, primitiveMgr, body, header)
	case mcputil.VERSION_20250618:
		return v20250618.ProcessMethod(ctx, id, method, g, primitiveMgr, body, header)
	case mcputil.VERSION_20250326:
		return v20250326.ProcessMethod(ctx, id, method, g, primitiveMgr, body, header)
	case "", mcputil.VERSION_20241105:
		return v20241105.ProcessMethod(ctx, id, method, g, primitiveMgr, body, header)
	default:
		return jsonrpc.NewUnsupportedProtocolVersionError(id, mcpVersion, enableDraft)
	}
}
