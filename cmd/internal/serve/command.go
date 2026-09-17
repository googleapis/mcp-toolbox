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

package serve

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/googleapis/mcp-toolbox/cmd/internal"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/spf13/cobra"
)

func NewCommand(opts *internal.ToolboxOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Deploy the toolbox server",
		Long:  "Deploy the toolbox server",
	}
	flags := cmd.Flags()
	internal.ServeFlags(flags, opts)
	cmd.RunE = func(*cobra.Command, []string) error { return runServe(cmd, opts) }
	return cmd
}

func runServe(cmd *cobra.Command, opts *internal.ToolboxOptions) error {
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
			opts.Logger.DebugContext(sCtx, "Received SIGTERM signal to shutdown.")
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

	if err := internal.ValidateServeFlags(opts.Cfg); err != nil {
		opts.Logger.ErrorContext(ctx, err.Error())
		return err
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
	srvErr := make(chan error, 1)
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
