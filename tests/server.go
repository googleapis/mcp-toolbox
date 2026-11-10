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
	"os"

	yaml "github.com/goccy/go-yaml"
	"github.com/spf13/cobra"

	"github.com/googleapis/mcp-toolbox/cmd"
)

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

	cmd     *cobra.Command
	cancel  context.CancelFunc
	closers []io.Closer
	done    chan bool // closed once the cmd is completed
	err     error
}

// StartCmd returns a CmdExec representing a running instance of a toolbox command.
func StartCmd(ctx context.Context, toolsFile map[string]any, args ...string) (*CmdExec, func(), error) {
	b, err := yaml.Marshal(toolsFile)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to marshal config: %s", err)
	}
	path, cleanup, err := tmpFileWithCleanup(b)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to write config: %s", err)
	}

	args = append(args, "--config", path, "--enable-draft-specs")

	ctx, cancel := context.WithCancel(ctx)
	// Open a pipe for tracking the output from the cmd
	pr, pw, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("unable to open stdout pipe: %w", err)
	}

	c := cmd.GenerateCommand(pw, pw)
	c.SetArgs(args)

	t := &CmdExec{
		Out:     pr,
		cmd:     c,
		cancel:  cancel,
		closers: []io.Closer{pr, pw},
		done:    make(chan bool),
	}

	// Start the command in the background
	go func() {
		defer close(t.done)
		defer cancel()
		t.err = c.ExecuteContext(ctx)
	}()
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
	c.cancel()
	for _, c := range c.closers {
		c.Close()
	}
}
