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

package tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	yaml "github.com/goccy/go-yaml"
	"github.com/spf13/cobra"

	"github.com/googleapis/mcp-toolbox/cmd"
)

const serverShutdownTimeout = 10 * time.Second

// tmpFileWithCleanup creates a temporary file with the content and returns the path and
// a function to clean it up, or any errors encountered instead
func tmpFileWithCleanup(content []byte) (string, func(), error) {
	// create a random file in the temp dir
	f, err := os.CreateTemp("", "*") // * indicates random string
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.Remove(f.Name()) }

	if _, err := f.Write(content); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return f.Name(), cleanup, err
}

// CmdExec represents an invocation of a toolbox command.
type CmdExec struct {
	Out io.ReadCloser

	cmd       *cobra.Command
	cancel    context.CancelFunc
	closers   []io.Closer
	closeOnce sync.Once
	done      chan bool // closed once the cmd is completed
	err       error
}

// StartCmd returns a CmdExec representing a running instance of a toolbox command.
// The returned cleanup function stops the server, waits for graceful shutdown so
// its listening port is released, drops idle HTTP keep-alive connections, and
// removes the temporary config file.
func StartCmd(ctx context.Context, toolsFile map[string]any, args ...string) (*CmdExec, func(), error) {
	b, err := yaml.Marshal(toolsFile)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to marshal config: %s", err)
	}
	path, removeTempFile, err := tmpFileWithCleanup(b)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to write config: %s", err)
	}

	args = append(args, "--config", path, "--enable-draft-specs")

	ctx, cancel := context.WithCancel(ctx)
	// Open a pipe for tracking the output from the cmd
	pr, pw, err := os.Pipe()
	if err != nil {
		cancel()
		removeTempFile()
		return nil, nil, fmt.Errorf("unable to open stdout pipe: %w", err)
	}

	c := cmd.GenerateCommand(pw, pw)
	c.SetArgs(args)

	t := &CmdExec{
		Out:     pr,
		cmd:     c,
		cancel:  cancel,
		closers: []io.Closer{pr},
		done:    make(chan bool),
	}

	// Start the command in the background. Close pw once ExecuteContext returns
	// so writers never race with an external Close and readers of Out see EOF.
	go func() {
		defer close(t.done)
		defer func() { _ = pw.Close() }()
		defer cancel()
		t.err = c.ExecuteContext(ctx)
	}()

	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			t.Close()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
			defer shutdownCancel()
			_ = t.Wait(shutdownCtx)
			http.DefaultClient.CloseIdleConnections()
			removeTempFile()
		})
	}
	return t, cleanup, nil
}

// Stop sends the TERM signal to the cmd and returns.
func (c *CmdExec) Stop() {
	c.cancel()
}

// Waits until the execution is completed and returns any error from the result.
func (c *CmdExec) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.err
	}
}

// Done returns true if the command has exited.
func (c *CmdExec) Done() bool {
	select {
	case <-c.done:
		return true
	default:
	}
	return false
}

// Close releases any resources associated with the instance.
func (c *CmdExec) Close() {
	c.closeOnce.Do(func() {
		c.cancel()
		for _, cl := range c.closers {
			_ = cl.Close()
		}
	})
}
