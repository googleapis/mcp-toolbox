// Copyright 2024 Google LLC
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

package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	// Importing the cmd/internal package also import packages for side effect of registration
	"github.com/googleapis/mcp-toolbox/cmd/internal"
	"github.com/googleapis/mcp-toolbox/cmd/internal/invoke"
	"github.com/googleapis/mcp-toolbox/cmd/internal/migrate"
	"github.com/googleapis/mcp-toolbox/cmd/internal/serve"
	"github.com/googleapis/mcp-toolbox/cmd/internal/skills"
	"github.com/googleapis/mcp-toolbox/internal/auth"
	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/spf13/cobra"
)

var (
	// versionString stores the full semantic version, including build metadata.
	versionString string
	// versionNum indicates the numerical part fo the version
	//go:embed version.txt
	versionNum string
	// metadataString indicates additional build or distribution metadata.
	buildType string = "dev" // should be one of "dev", "binary", or "container"
	// commitSha is the git commit it was built from
	commitSha string
)

func init() {
	versionString = semanticVersion()
}

// semanticVersion returns the version of the CLI including a compile-time metadata.
func semanticVersion() string {
	metadataStrings := []string{buildType, runtime.GOOS, runtime.GOARCH}
	if commitSha != "" {
		metadataStrings = append(metadataStrings, commitSha)
	}
	v := strings.TrimSpace(versionNum) + "+" + strings.Join(metadataStrings, ".")
	return v
}

// GenerateCommand returns a new Command object with the specified IO streams
// This is used for integration test package
func GenerateCommand(out, err io.Writer) *cobra.Command {
	opts := internal.NewToolboxOptions(internal.WithIOStreams(out, err))
	return NewCommand(opts)
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// Initialize options
	opts := internal.NewToolboxOptions()

	if err := NewCommand(opts).Execute(); err != nil {
		fmt.Fprintf(opts.IOStreams.ErrOut, "Error: %v\n", err)
		exit := 1
		os.Exit(exit)
	}
}

// NewCommand returns a Command object representing an invocation of the CLI.
func NewCommand(opts *internal.ToolboxOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "toolbox",
		Version:       versionString,
		SilenceErrors: true,
	}

	// Do not print Usage on runtime error
	cmd.SilenceUsage = true

	// Set server version
	opts.Cfg.Version = versionString
	opts.VersionNum = strings.TrimSpace(versionNum)

	// set baseCmd in, out and err the same as cmd.
	cmd.SetIn(opts.IOStreams.In)
	cmd.SetOut(opts.IOStreams.Out)
	cmd.SetErr(opts.IOStreams.ErrOut)

	// setup flags that are common across all commands
	internal.PersistentFlags(cmd, opts)
	flags := cmd.Flags()
	internal.ConfigFileFlags(cmd, flags, opts)
	internal.ServeFlags(flags, opts)
	flags.BoolVar(&opts.Cfg.DisableReload, "disable-reload", false, "Disables dynamic reloading of tools file.")
	flags.BoolVar(&opts.Cfg.IgnoreUnknownTools, "ignore-unknown-tools", false, "Log warnings and skip unknown/unsupported tool types instead of failing to start.")
	flags.IntVar(&opts.Cfg.PollInterval, "poll-interval", 0, "Specifies the polling frequency (seconds) for configuration file updates.")
	// wrap RunE command so that we have access to original Command object
	cmd.RunE = func(*cobra.Command, []string) error { return run(cmd, opts) }

	// Register subcommands
	cmd.AddCommand(invoke.NewCommand(opts))
	cmd.AddCommand(skills.NewCommand(opts))
	cmd.AddCommand(serve.NewCommand(opts))
	cmd.AddCommand(migrate.NewCommand(opts))

	return cmd
}

func handleDynamicReload(ctx context.Context, cfg server.ServerConfig, s *server.Server) error {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		panic(err)
	}

	sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groupsMap, err := validateReloadEdits(ctx, cfg)
	if err != nil {
		errMsg := fmt.Errorf("unable to validate reloaded edits: %w", err)
		logger.WarnContext(ctx, errMsg.Error())
		return err
	}

	s.PrimitiveMgr.SetPrimitives(sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groupsMap)

	return nil
}

// validateReloadEdits checks that the reloaded config configs can initialized without failing
func validateReloadEdits(
	ctx context.Context, cfg server.ServerConfig,
) (map[string]sources.Source, map[string]auth.AuthService, map[string]embeddingmodels.EmbeddingModel, map[string]tools.Tool, map[string]prompts.Prompt, map[string]resources.Resource, map[string]resources.ResourceTemplate, map[string]group.Group, error) {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		panic(err)
	}

	instrumentation, err := util.InstrumentationFromContext(ctx)
	if err != nil {
		panic(err)
	}

	logger.DebugContext(ctx, "Attempting to parse and validate reloaded config.")

	ctx, span := instrumentation.Tracer.Start(ctx, "toolbox/server/reload")
	defer span.End()

	sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groupsMap, err := server.InitializeConfigs(ctx, cfg)
	if err != nil {
		errMsg := fmt.Errorf("unable to initialize reloaded configs: %w", err)
		logger.WarnContext(ctx, errMsg.Error())
		return nil, nil, nil, nil, nil, nil, nil, nil, err
	}

	return sourcesMap, authServicesMap, embeddingModelsMap, toolsMap, promptsMap, resourcesMap, resourceTemplatesMap, groupsMap, nil
}

// Helper to check if a file has a newer ModTime than stored in the map
func checkModTime(path string, mTime time.Time, lastSeen map[string]time.Time) bool {
	if mTime.After(lastSeen[path]) {
		lastSeen[path] = mTime
		return true
	}
	return false
}

// Helper to scan watched files and check their modification times in polling system
func scanWatchedFiles(watchingFolder bool, folderToWatch string, watchedFiles map[string]bool, lastSeen map[string]time.Time) (map[string]bool, bool, error) {
	changed := false
	currentDiskFiles := make(map[string]bool)
	if watchingFolder {
		files, err := os.ReadDir(folderToWatch)
		if err != nil {
			return nil, changed, fmt.Errorf("error reading config folder %w", err)
		}
		for _, f := range files {
			if !f.IsDir() && (strings.HasSuffix(f.Name(), ".yaml") || strings.HasSuffix(f.Name(), ".yml")) {
				fullPath := filepath.Join(folderToWatch, f.Name())
				currentDiskFiles[fullPath] = true
				if info, err := f.Info(); err == nil {
					if checkModTime(fullPath, info.ModTime(), lastSeen) {
						changed = true
					}
				}
			}
		}
	} else {
		for f := range watchedFiles {
			if info, err := os.Stat(f); err == nil {
				currentDiskFiles[f] = true
				if checkModTime(f, info.ModTime(), lastSeen) {
					changed = true
				}
			}
		}
	}
	return currentDiskFiles, changed, nil
}

// watchChanges checks for changes in the provided yaml config(s) or folder.
func watchChanges(ctx context.Context, watchDirs map[string]bool, watchedFiles map[string]bool, s *server.Server, opts *internal.ToolboxOptions) {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		panic(err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		logger.WarnContext(ctx, fmt.Sprintf("error setting up new watcher %s", err))
		return
	}

	defer w.Close()

	watchingFolder := false
	var folderToWatch string

	// if watchedFiles is empty, indicates that user passed entire folder instead
	if len(watchedFiles) == 0 {
		watchingFolder = true

		// validate that watchDirs only has single element
		if len(watchDirs) > 1 {
			logger.WarnContext(ctx, "error setting watcher, expected single config folder if no file(s) are defined.")
			return
		}

		for onlyKey := range watchDirs {
			folderToWatch = onlyKey
			break
		}
	}

	for dir := range watchDirs {
		err := w.Add(dir)
		if err != nil {
			logger.WarnContext(ctx, fmt.Sprintf("Error adding path %s to watcher: %s", dir, err))
			break
		}
		logger.DebugContext(ctx, fmt.Sprintf("Added directory %s to watcher.", dir))
	}

	lastSeen := make(map[string]time.Time)
	var pollTickerChan <-chan time.Time
	pollTickerSecond := opts.Cfg.PollInterval
	if pollTickerSecond > 0 {
		ticker := time.NewTicker(time.Duration(pollTickerSecond) * time.Second)
		defer ticker.Stop()
		pollTickerChan = ticker.C // Assign the channel
		logger.DebugContext(ctx, fmt.Sprintf("NFS polling enabled every %v", pollTickerSecond))

		// Pre-populate lastSeen to avoid an initial spurious reload
		_, _, err = scanWatchedFiles(watchingFolder, folderToWatch, watchedFiles, lastSeen)
		if err != nil {
			logger.WarnContext(ctx, err.Error())
		}
	} else {
		logger.DebugContext(ctx, "NFS polling disabled (interval is 0)")
	}

	// debounce timer is used to prevent multiple writes triggering multiple reloads
	debounceDelay := 100 * time.Millisecond
	debounce := time.NewTimer(1 * time.Minute)
	debounce.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.DebugContext(ctx, "file watcher context cancelled")
			return
		case <-pollTickerChan:
			// Get files that are currently on disk
			currentDiskFiles, changed, err := scanWatchedFiles(watchingFolder, folderToWatch, watchedFiles, lastSeen)
			if err != nil {
				logger.WarnContext(ctx, err.Error())
				continue
			}

			// Check for Deletions
			// If it was in lastSeen but is NOT in currentDiskFiles, it's
			// deleted; we will need to reload the server.
			for path := range lastSeen {
				if !currentDiskFiles[path] {
					logger.DebugContext(ctx, fmt.Sprintf("File deleted (detected via polling): %s", path))
					delete(lastSeen, path)
					changed = true
				}
			}
			if changed {
				logger.DebugContext(ctx, "File change detected via polling")
				// once this timer runs out, it will trigger debounce.C
				debounce.Reset(debounceDelay)
			}
		case err, ok := <-w.Errors:
			if !ok {
				logger.WarnContext(ctx, "file watcher was closed unexpectedly")
				return
			}
			if err != nil {
				logger.WarnContext(ctx, fmt.Sprintf("file watcher error %s", err))
				return
			}

		case e, ok := <-w.Events:
			if !ok {
				logger.WarnContext(ctx, "file watcher already closed")
				return
			}

			// only check for events which indicate user saved a new config
			// multiple operations checked due to various file update methods across editors
			if !e.Has(fsnotify.Write | fsnotify.Create | fsnotify.Rename) {
				continue
			}

			cleanedFilename := filepath.Clean(e.Name)
			logger.DebugContext(ctx, fmt.Sprintf("%s event detected in %s", e.Op, cleanedFilename))

			folderChanged := watchingFolder &&
				(strings.HasSuffix(cleanedFilename, ".yaml") || strings.HasSuffix(cleanedFilename, ".yml"))

			if folderChanged || watchedFiles[cleanedFilename] {
				// indicates the write event is on a relevant file
				debounce.Reset(debounceDelay)
			}

		case <-debounce.C:
			debounce.Stop()
			logger.DebugContext(ctx, "File change detected, attempting to reload server...")

			// Create a copy to avoid mutating the original opts if validation/reload fails
			reloadedOpts := *opts
			reloadedOpts.PrebuiltConfigs = slices.Clone(opts.PrebuiltConfigs)
			reloadedOpts.Configs = slices.Clone(opts.Configs)
			reloadedOpts.Cfg.Version = versionString

			if _, err := reloadedOpts.LoadConfig(ctx, &internal.ConfigParser{}); err != nil {
				logger.WarnContext(ctx, fmt.Sprintf("Error reloading config: %s", err))
				continue
			}

			if err := handleDynamicReload(ctx, reloadedOpts.Cfg, s); err != nil {
				logger.WarnContext(ctx, fmt.Sprintf("Error reloading server: %s", err))
				continue
			}
		}
	}
}

func resolveWatcherInputs(toolsFile string, toolsFiles []string, toolsFolder string) (map[string]bool, map[string]bool) {
	var relevantFiles []string

	// map for efficiently checking if a file is relevant
	watchedFiles := make(map[string]bool)

	// dirs that will be added to watcher (fsnotify prefers watching directory then filtering for file)
	watchDirs := make(map[string]bool)

	if len(toolsFiles) > 0 {
		relevantFiles = toolsFiles
	} else if toolsFolder != "" {
		watchDirs[filepath.Clean(toolsFolder)] = true
	} else {
		relevantFiles = []string{toolsFile}
	}

	// extract parent dir for relevant files and dedup
	for _, f := range relevantFiles {
		cleanFile := filepath.Clean(f)
		watchedFiles[cleanFile] = true
		watchDirs[filepath.Dir(cleanFile)] = true
	}

	return watchDirs, watchedFiles
}

func run(cmd *cobra.Command, opts *internal.ToolboxOptions) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// watch for sigterm / sigint signals
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	go func(sCtx context.Context) {
		var s os.Signal
		select {
		case <-sCtx.Done():
			// this should only happen when the context supplied when testing is canceled
			return
		case s = <-signals:
		}
		switch s {
		case syscall.SIGINT:
			opts.Logger.DebugContext(sCtx, "Received SIGINT signal to shutdown.")
		case syscall.SIGTERM:
			opts.Logger.DebugContext(sCtx, "Sending SIGTERM signal to shutdown.")
		}
		cancel()
	}(ctx)

	ctx, shutdown, err := opts.Setup(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	isCustomConfigured, err := opts.LoadConfig(ctx, &internal.ConfigParser{})
	if err != nil {
		return err
	}

	// Validate ToolboxUrl if MCP Auth is enabled
	var mcpAuthEnabled bool
	for _, authSvc := range opts.Cfg.AuthServiceConfigs {
		if authSvc.IsMCPEnabled() {
			mcpAuthEnabled = true
			break
		}
	}

	if mcpAuthEnabled {
		if opts.Cfg.EnableAPI {
			errMsg := fmt.Errorf("MCP Auth cannot be enabled together with the legacy HTTP API (--enable-api)")
			opts.Logger.ErrorContext(ctx, errMsg.Error())
			return errMsg
		}
		if opts.Cfg.ToolboxUrl == "" {
			opts.Cfg.ToolboxUrl = os.Getenv("TOOLBOX_URL")
		}
		if opts.Cfg.ToolboxUrl == "" {
			errMsg := fmt.Errorf("MCP Auth is enabled but Toolbox URL is missing. Please provide it via --toolbox-url flag or TOOLBOX_URL environment variable")
			opts.Logger.ErrorContext(ctx, errMsg.Error())
			return errMsg
		}
	}

	// start server
	s, err := server.NewServer(ctx, opts.Cfg)
	if err != nil {
		errMsg := fmt.Errorf("toolbox failed to initialize: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	useTLS := opts.Cfg.CertFile != "" || opts.Cfg.KeyFile != ""
	protocol := "http"
	if useTLS {
		protocol = "https"
	}

	// run server in background
	srvErr := make(chan error)
	if opts.Cfg.Stdio {
		go func() {
			defer close(srvErr)
			err = s.ServeStdio(ctx, opts.IOStreams.In, opts.IOStreams.Out)
			if err != nil {
				srvErr <- err
			}
		}()
	} else {
		err = s.Listen(ctx, opts.Cfg.CertFile, opts.Cfg.KeyFile)
		if err != nil {
			errMsg := fmt.Errorf("toolbox failed to start listener: %w", err)
			opts.Logger.ErrorContext(ctx, errMsg.Error())
			return errMsg
		}
		opts.Logger.InfoContext(ctx, "Server ready to serve!")
		if opts.Cfg.UI {
			opts.Logger.InfoContext(ctx, fmt.Sprintf("Toolbox UI is up and running at: %s://%s:%d/ui", protocol, opts.Cfg.Address, opts.Cfg.Port))
		}

		go func() {
			defer close(srvErr)
			err = s.Serve(ctx)
			if err != nil {
				srvErr <- err
			}
		}()
	}

	if isCustomConfigured && !opts.Cfg.DisableReload {
		watchDirs, watchedFiles := resolveWatcherInputs(opts.Config, opts.Configs, opts.ConfigFolder)

		// start watching the file(s) or folder for changes to trigger dynamic reloading
		go watchChanges(ctx, watchDirs, watchedFiles, s, opts)
	}

	// wait for either the server to error out or the command's context to be canceled
	select {
	case err := <-srvErr:
		if err != nil {
			errMsg := fmt.Errorf("toolbox crashed with the following error: %w", err)
			opts.Logger.ErrorContext(ctx, errMsg.Error())
			return errMsg
		}
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		opts.Logger.WarnContext(shutdownContext, "Shutting down gracefully...")
		err := s.Shutdown(shutdownContext)
		if err == context.DeadlineExceeded {
			return fmt.Errorf("graceful shutdown timed out... forcing exit")
		}
	}

	return nil
}
