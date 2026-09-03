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

package internal

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/prebuiltconfigs"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/telemetry"
	"github.com/googleapis/mcp-toolbox/internal/util"
)

type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

// ToolboxOptions holds dependencies shared by all commands.
type ToolboxOptions struct {
	IOStreams       IOStreams
	Logger          log.Logger
	Cfg             server.ServerConfig
	Config          string
	Configs         []string
	ConfigFolder    string
	PrebuiltConfigs []string
	VersionNum      string
}

// Option defines a function that modifies the ToolboxOptions struct.
type Option func(*ToolboxOptions)

// NewToolboxOptions creates a new instance with defaults, then applies any
// provided options.
func NewToolboxOptions(opts ...Option) *ToolboxOptions {
	o := &ToolboxOptions{
		IOStreams: IOStreams{
			In:     os.Stdin,
			Out:    os.Stdout,
			ErrOut: os.Stderr,
		},
	}

	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Apply allows you to update an EXISTING ToolboxOptions instance.
// This is useful for "late binding".
func (o *ToolboxOptions) Apply(opts ...Option) {
	for _, opt := range opts {
		opt(o)
	}
}

// WithIOStreams updates the IO streams.
func WithIOStreams(out, err io.Writer) Option {
	return func(o *ToolboxOptions) {
		o.IOStreams.Out = out
		o.IOStreams.ErrOut = err
	}
}

// Setup create logger and telemetry instrumentations.
func (opts *ToolboxOptions) Setup(ctx context.Context) (context.Context, func(context.Context) error, error) {
	// If stdio, set logger's out stream (usually DEBUG and INFO logs) to
	// errStream
	loggerOut := opts.IOStreams.Out
	if opts.Cfg.Stdio {
		loggerOut = opts.IOStreams.ErrOut
	}

	// Handle logger separately from config
	logger, err := log.NewLogger(opts.Cfg.LoggingFormat.String(), opts.Cfg.LogLevel.String(), loggerOut, opts.IOStreams.ErrOut)
	if err != nil {
		return ctx, nil, fmt.Errorf("unable to initialize logger: %w", err)
	}

	ctx = util.WithLogger(ctx, logger)
	opts.Logger = logger

	ctx = util.WithIgnoreUnknownTools(ctx, opts.Cfg.IgnoreUnknownTools)

	logger.InfoContext(ctx, fmt.Sprintf("Starting MCP Toolbox for Databases version %s", opts.Cfg.Version))

	// Set up OpenTelemetry
	otelShutdown, err := telemetry.SetupOTel(ctx, opts.Cfg.Version, opts.Cfg.TelemetryOTLP, opts.Cfg.TelemetryGCP, opts.Cfg.TelemetryGCPProject, opts.Cfg.TelemetryServiceName)
	if err != nil {
		errMsg := fmt.Errorf("error setting up OpenTelemetry: %w", err)
		logger.ErrorContext(ctx, errMsg.Error())
		return ctx, nil, errMsg
	}

	shutdownFunc := func(ctx context.Context) error {
		err := otelShutdown(ctx)
		if err != nil {
			errMsg := fmt.Errorf("error shutting down OpenTelemetry: %w", err)
			logger.ErrorContext(ctx, errMsg.Error())
			return err
		}
		return nil
	}

	instrumentation, err := telemetry.CreateTelemetryInstrumentation(opts.Cfg.Version)
	if err != nil {
		errMsg := fmt.Errorf("unable to create telemetry instrumentation: %w", err)
		logger.ErrorContext(ctx, errMsg.Error())
		return ctx, shutdownFunc, errMsg
	}

	ctx = util.WithInstrumentation(ctx, instrumentation)

	return ctx, shutdownFunc, nil
}

// GetCustomConfigFiles retrieves the list of custom config file paths
func (opts *ToolboxOptions) GetCustomConfigFiles(ctx context.Context) ([]string, bool, error) {
	// Determine if Custom Files should be loaded
	// Check for explicit custom flags
	isCustomConfigured := opts.Config != "" || len(opts.Configs) > 0 || opts.ConfigFolder != ""

	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, isCustomConfigured, err
	}

	// Load Custom Configurations
	if isCustomConfigured {
		if len(opts.Configs) > 0 {
			// Use tools-files
			logger.InfoContext(ctx, fmt.Sprintf("retrieving %d tool configuration files", len(opts.Configs)))
			return opts.Configs, isCustomConfigured, nil
		} else if opts.ConfigFolder != "" {
			// Use tools-folder
			allFiles, err := GetPathsFromConfigFolder(ctx, opts.ConfigFolder)
			return allFiles, isCustomConfigured, err
		} else {
			// use tools-file
			return []string{opts.Config}, isCustomConfigured, nil
		}
	}

	// Determine if default 'tools.yaml' should be used (No prebuilt AND No custom flags)
	useDefaultConfig := len(opts.PrebuiltConfigs) == 0
	if useDefaultConfig {
		// else we will add the default path regardless
		return []string{"tools.yaml"}, true, nil
	}

	// no custom config files are found
	// server are likely using prebuilt configs
	return []string{}, false, nil
}

// LoadConfig checks and merge files that should be loaded into the server
func (opts *ToolboxOptions) LoadConfig(ctx context.Context, parser *ConfigParser) (bool, error) {
	// get all the file paths for custom config file
	filesPaths, isCustomConfigured, err := opts.GetCustomConfigFiles(ctx)
	if err != nil {
		return isCustomConfigured, err
	}

	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return isCustomConfigured, err
	}

	var allConfigs []Config

	// Load Prebuilt Configuration
	if len(opts.PrebuiltConfigs) > 0 {
		slices.Sort(opts.PrebuiltConfigs)
		sourcesList := strings.Join(opts.PrebuiltConfigs, ", ")
		logMsg := fmt.Sprintf("Using prebuilt tool configurations for: %s", sourcesList)
		logger.InfoContext(ctx, logMsg)
		logger.WarnContext(ctx, "These prebuilt configs are intended for 'build-time' use cases, where agents are helping trusted developers build things. They are not secure enough for 'run time' use cases, where the agent will be talking to potentially untrusted developers.")

		for _, configName := range opts.PrebuiltConfigs {
			if !strings.Contains(configName, "/") {
				for _, sep := range []string{".", ":", "@"} {
					if strings.Contains(configName, sep) {
						parts := strings.SplitN(configName, sep, 2)
						if slices.Contains(prebuiltconfigs.GetPrebuiltSources(), parts[0]) {
							errMsg := fmt.Errorf("invalid prebuilt config format '%s'. Did you mean '%s/%s'? Use '/' to specify a toolset", configName, parts[0], parts[1])
							logger.ErrorContext(ctx, errMsg.Error())
							return isCustomConfigured, errMsg
						}
					}
				}
			}

			sourceName, toolsetName, _ := strings.Cut(configName, "/")

			buf, err := prebuiltconfigs.Get(sourceName)
			if err != nil {
				logger.ErrorContext(ctx, err.Error())
				return isCustomConfigured, err
			}

			// Parse into Config struct
			parsed, err := parser.ParseConfig(ctx, buf)
			if err != nil {
				errMsg := fmt.Errorf("unable to parse prebuilt tool configuration for '%s': %w", configName, err)
				logger.ErrorContext(ctx, errMsg.Error())
				return isCustomConfigured, errMsg
			}

			if toolsetName != "" {
				// Legacy toolsets are folded into groups at unmarshal, so the named
				// toolset resolves as a group.
				targetGroup, exists := parsed.Groups[toolsetName]
				if !exists {
					var available []string
					for k := range parsed.Groups {
						if k == "" {
							continue
						}
						available = append(available, k)
					}
					slices.Sort(available)
					errMsg := fmt.Errorf("toolset '%s' not found in prebuilt configuration '%s'. Available toolsets: %s", toolsetName, sourceName, strings.Join(available, ", "))
					logger.ErrorContext(ctx, errMsg.Error())
					return isCustomConfigured, errMsg
				}

				// Filter tools to only include those in the target group
				filteredTools := make(server.ToolConfigs)
				for _, tName := range targetGroup.ToolNames {
					if tCfg, tExists := parsed.Tools[tName]; tExists {
						filteredTools[tName] = tCfg
					}
				}
				parsed.Tools = filteredTools

				// Filter groups to only include the target group
				filteredGroups := make(server.GroupConfigs)
				filteredGroups[toolsetName] = targetGroup
				parsed.Groups = filteredGroups
			}

			allConfigs = append(allConfigs, parsed)
		}
	}

	// Load Custom Configurations
	if isCustomConfigured {
		customTools, err := parser.LoadAndMergeConfigs(ctx, filesPaths)
		if err != nil {
			logger.ErrorContext(ctx, err.Error())
			return isCustomConfigured, err
		}
		allConfigs = append(allConfigs, customTools)
	}

	// Modify version string based on loaded configurations
	if len(opts.PrebuiltConfigs) > 0 {
		tag := "prebuilt"
		if isCustomConfigured {
			tag = "custom"
		}
		// prebuiltConfigs is already sorted above
		for _, configName := range opts.PrebuiltConfigs {
			sanitizedConfigName := strings.ReplaceAll(configName, "/", ".")
			opts.Cfg.Version += fmt.Sprintf("+%s.%s", tag, sanitizedConfigName)
		}
	}

	// Merge Everything
	// This will error if custom tools collide with prebuilt tools
	finalConfig, err := mergeConfigs(allConfigs...)
	if err != nil {
		logger.ErrorContext(ctx, err.Error())
		return isCustomConfigured, err
	}

	opts.Cfg.SourceConfigs = finalConfig.Sources
	opts.Cfg.AuthServiceConfigs = finalConfig.AuthServices
	opts.Cfg.EmbeddingModelConfigs = finalConfig.EmbeddingModels
	opts.Cfg.ToolConfigs = finalConfig.Tools
	opts.Cfg.PromptConfigs = finalConfig.Prompts
	opts.Cfg.ResourceConfigs = finalConfig.Resources
	opts.Cfg.ResourceTemplateConfigs = finalConfig.ResourceTemplates
	opts.Cfg.GroupConfigs = finalConfig.Groups

	return isCustomConfigured, nil
}
