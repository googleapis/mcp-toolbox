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

package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/cmd/internal"
	"github.com/spf13/cobra"
)

// migrateCmd is the command for migrating configuration files.
type migrateCmd struct {
	*cobra.Command
	dryRun bool
}

func NewCommand(opts *internal.ToolboxOptions) *cobra.Command {
	cmd := &migrateCmd{}
	cmd.Command = &cobra.Command{
		Use:   "migrate",
		Short: "Migrate all configuration files to flat format",
		Long:  "Migrate all configuration files provided to the flat format, updating deprecated fields (including converting toolsets to groups) and ensuring compatibility.",
	}
	flags := cmd.Flags()
	internal.ConfigFileFlags(cmd.Command, flags, opts)
	flags.BoolVar(&cmd.dryRun, "dry-run", false, "Preview the converted format without applying actual changes.")
	cmd.RunE = func(*cobra.Command, []string) error { return runMigrate(cmd, opts) }
	return cmd.Command
}

func runMigrate(cmd *migrateCmd, opts *internal.ToolboxOptions) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	ctx, shutdown, err := opts.Setup(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	logger := opts.Logger
	filePaths, _, err := opts.GetCustomConfigFiles(ctx)
	if err != nil {
		errMsg := fmt.Errorf("error retrieving configuration file: %w", err)
		logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	logger.InfoContext(ctx, "migration process will start; any comments (except for top-level comments) presented in the original configuration files will not be preserved in the migrated files")
	var errs []error
	// process each files independently.
	for _, filePath := range filePaths {
		buf, err := os.ReadFile(filePath)
		if err != nil {
			errMsg := fmt.Errorf("unable to read tool file at %q: %w", filePath, err)
			logger.ErrorContext(ctx, errMsg.Error())
			errs = append(errs, errMsg)
			continue
		}
		newBuf, err := internal.ConvertConfig(ctx, buf)
		if err != nil {
			logger.ErrorContext(ctx, err.Error())
			errs = append(errs, err)
			continue
		}
		if cmp.Equal(buf, newBuf) {
			continue
		}

		if cmd.dryRun {
			logger.DebugContext(ctx, fmt.Sprintf("printing migration to output for file: %s", filePath))
			fmt.Fprintln(opts.IOStreams.Out, string(newBuf))
		} else {
			info, err := os.Stat(filePath)
			if err != nil {
				errMsg := fmt.Errorf("failed to stat file: %w", err)
				logger.ErrorContext(ctx, errMsg.Error())
				errs = append(errs, errMsg)
				continue
			}
			backupFile := filePath + ".bak"
			err = os.Rename(filePath, backupFile)
			if err != nil {
				errMsg := fmt.Errorf("failed to rename file: %w", err)
				logger.ErrorContext(ctx, errMsg.Error())
				errs = append(errs, errMsg)
				continue
			}
			logger.DebugContext(ctx, fmt.Sprintf("successfully renamed %s to %s", filePath, backupFile))

			// set the permission to the original file's permission.
			err = os.WriteFile(filePath, newBuf, info.Mode().Perm())
			if err != nil {
				errMsg := fmt.Errorf("failed to write to file: %w", err)
				// restoring original file
				if removeErr := os.Remove(filePath); removeErr != nil { // Attempt to remove the possibly partial file to ensure Rename succeeds.
					errMsg = errors.Join(errMsg, removeErr)
				}
				if restoreErr := os.Rename(backupFile, filePath); restoreErr != nil {
					fullRestoreErr := fmt.Errorf("failed to restore original file: %w", restoreErr)
					errMsg = errors.Join(errMsg, fullRestoreErr)
				}
				logger.ErrorContext(ctx, errMsg.Error())
				errs = append(errs, errMsg)
				continue
			}
			logger.DebugContext(ctx, fmt.Sprintf("migration completed for file: %s", filePath))
		}
	}

	logger.InfoContext(ctx, "migration ended!")
	// If errs is empty, errors.Join returns nil
	return errors.Join(errs...)
}
