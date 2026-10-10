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

package invoke

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/googleapis/mcp-toolbox/cmd/internal"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/spf13/cobra"
)

func NewCommand(opts *internal.ToolboxOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "invoke <tool-name> [params]",
		Short: "Execute a tool directly",
		Long: `Execute a tool directly with parameters.
Params must be a JSON string.
Example:
  toolbox invoke my-tool '{"param1": "value1"}'`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runInvoke(c, args, opts)
		},
	}
	flags := cmd.Flags()
	internal.ConfigFileFlags(cmd, flags, opts)
	return cmd
}

func runInvoke(cmd *cobra.Command, args []string, opts *internal.ToolboxOptions) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	ctx, shutdown, err := opts.Setup(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	_, err = opts.LoadConfig(ctx, &internal.ConfigParser{})
	if err != nil {
		return err
	}

	// Initialize Resources
	sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groupsMap, err := server.InitializeConfigs(ctx, opts.Cfg)
	if err != nil {
		errMsg := fmt.Errorf("failed to initialize resources: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	primitiveMgr := primitives.NewPrimitiveManager(sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groupsMap)

	// Execute Tool
	toolName := args[0]
	tool, ok := primitiveMgr.GetTool(toolName)
	if !ok {
		errMsg := fmt.Errorf("tool %q not found", toolName)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	srcName := tool.GetSourceName()
	var src sources.Source
	if srcName != "" {
		src, ok = primitiveMgr.GetSource(srcName)
		if !ok {
			errMsg := fmt.Errorf("unable to retrieve source for tool %s", toolName)
			opts.Logger.ErrorContext(ctx, errMsg.Error())
			return errMsg
		}
	}

	err = tool.ValidateSource(src)
	if err != nil {
		opts.Logger.ErrorContext(ctx, err.Error())
		return err
	}

	var paramsInput string
	if len(args) > 1 {
		paramsInput = args[1]
	}

	params := make(map[string]any)
	if paramsInput != "" {
		if err := util.DecodeJSON(strings.NewReader(paramsInput), &params); err != nil {
			errMsg := fmt.Errorf("params must be a valid JSON string: %w", err)
			opts.Logger.ErrorContext(ctx, errMsg.Error())
			return errMsg
		}
	}

	toolParams, err := tool.GetParameters(src)
	if err != nil {
		errMsg := fmt.Errorf("error getting parameters for tool: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	parsedParams, err := parameters.ParseParams(toolParams, params, nil)
	if err != nil {
		errMsg := fmt.Errorf("invalid parameters: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	parsedParams, err = tool.EmbedParams(ctx, parsedParams, primitiveMgr)
	if err != nil {
		errMsg := fmt.Errorf("error embedding parameters: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	// Client Auth not supported for ephemeral CLI call
	requiresAuth, err := tool.RequiresClientAuthorization(src)
	if err != nil {
		errMsg := fmt.Errorf("failed to check auth requirements: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}
	if requiresAuth {
		errMsg := fmt.Errorf("client authorization is not supported")
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	result, err := tool.Invoke(ctx, src, parsedParams, "")
	if err != nil {
		errMsg := fmt.Errorf("tool execution failed: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	// Print Result
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		errMsg := fmt.Errorf("failed to marshal result: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}
	fmt.Fprintln(opts.IOStreams.Out, string(output))

	return nil
}
