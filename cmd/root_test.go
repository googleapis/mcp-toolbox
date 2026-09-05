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
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/googleapis/mcp-toolbox/cmd/internal"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/telemetry"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/spf13/cobra"
)

func withDefaults(c server.ServerConfig) server.ServerConfig {
	data, _ := os.ReadFile("version.txt")
	version := strings.TrimSpace(string(data)) // Preserving 'data', new var for clarity
	c.Version = version + "+" + strings.Join([]string{"dev", runtime.GOOS, runtime.GOARCH}, ".")

	if c.Address == "" {
		c.Address = "127.0.0.1"
	}
	if c.Port == 0 {
		c.Port = 5000
	}
	if c.TelemetryServiceName == "" {
		c.TelemetryServiceName = "toolbox"
	}
	if c.AllowedOrigins == nil {
		c.AllowedOrigins = []string{"*"}
	}
	if c.AllowedHosts == nil {
		c.AllowedHosts = []string{"*"}
	}
	if c.UserAgentMetadata == nil {
		c.UserAgentMetadata = []string{}
	}
	if c.DisableExt == nil {
		c.DisableExt = []string{}
	}
	if c.HttpMaxRequestBytes == 0 {
		c.HttpMaxRequestBytes = server.DefaultHTTPMaxRequestBytes
	}
	return c
}

func invokeCommand(args []string) (*cobra.Command, *internal.ToolboxOptions, string, error) {
	buf := new(bytes.Buffer)
	opts := internal.NewToolboxOptions(internal.WithIOStreams(buf, buf))
	c := NewCommand(opts)

	// Keep the test output quiet
	c.SilenceUsage = true
	c.SilenceErrors = true

	// Capture output
	c.SetOut(buf)
	c.SetErr(buf)
	c.SetArgs(args)

	// Disable execute behavior
	c.RunE = func(*cobra.Command, []string) error {
		return nil
	}

	err := c.Execute()

	return c, opts, buf.String(), err
}

// invokeCommandWithContext executes the command with a context and returns the captured output.
func invokeCommandWithContext(ctx context.Context, args []string) (*cobra.Command, *internal.ToolboxOptions, string, error) {
	buf := new(bytes.Buffer)
	opts := internal.NewToolboxOptions(internal.WithIOStreams(buf, buf))
	c := NewCommand(opts)

	// Capture output using a buffer
	c.SetArgs(args)
	c.SilenceUsage = true
	c.SilenceErrors = true
	c.SetContext(ctx)

	err := c.Execute()
	return c, opts, buf.String(), err
}

func TestVersion(t *testing.T) {
	data, err := os.ReadFile("version.txt")
	if err != nil {
		t.Fatalf("failed to read version.txt: %v", err)
	}
	want := strings.TrimSpace(string(data))

	_, _, got, err := invokeCommand([]string{"--version"})
	if err != nil {
		t.Fatalf("error invoking command: %s", err)
	}

	if !strings.Contains(got, want) {
		t.Errorf("cli did not return correct version: want %q, got %q", want, got)
	}
}

func TestServerConfigFlags(t *testing.T) {
	tcs := []struct {
		desc string
		args []string
		want server.ServerConfig
	}{
		{
			desc: "default values",
			args: []string{},
			want: withDefaults(server.ServerConfig{}),
		},
		{
			desc: "address short",
			args: []string{"-a", "127.0.1.1"},
			want: withDefaults(server.ServerConfig{
				Address: "127.0.1.1",
			}),
		},
		{
			desc: "address long",
			args: []string{"--address", "0.0.0.0"},
			want: withDefaults(server.ServerConfig{
				Address: "0.0.0.0",
			}),
		},
		{
			desc: "port short",
			args: []string{"-p", "5052"},
			want: withDefaults(server.ServerConfig{
				Port: 5052,
			}),
		},
		{
			desc: "port long",
			args: []string{"--port", "5050"},
			want: withDefaults(server.ServerConfig{
				Port: 5050,
			}),
		},
		{
			desc: "logging format",
			args: []string{"--logging-format", "JSON"},
			want: withDefaults(server.ServerConfig{
				LoggingFormat: "JSON",
			}),
		},
		{
			desc: "debug logs",
			args: []string{"--log-level", "WARN"},
			want: withDefaults(server.ServerConfig{
				LogLevel: "WARN",
			}),
		},
		{
			desc: "telemetry gcp",
			args: []string{"--telemetry-gcp"},
			want: withDefaults(server.ServerConfig{
				TelemetryGCP: true,
			}),
		},
		{
			desc: "telemetry otlp",
			args: []string{"--telemetry-otlp", "http://127.0.0.1:4553"},
			want: withDefaults(server.ServerConfig{
				TelemetryOTLP: "http://127.0.0.1:4553",
			}),
		},
		{
			desc: "telemetry service name",
			args: []string{"--telemetry-service-name", "toolbox-custom"},
			want: withDefaults(server.ServerConfig{
				TelemetryServiceName: "toolbox-custom",
			}),
		},
		{
			desc: "stdio",
			args: []string{"--stdio"},
			want: withDefaults(server.ServerConfig{
				Stdio: true,
			}),
		},
		{
			desc: "disable reload",
			args: []string{"--disable-reload"},
			want: withDefaults(server.ServerConfig{
				DisableReload: true,
			}),
		},
		{
			desc: "allowed origin",
			args: []string{"--allowed-origins", "http://foo.com,http://bar.com"},
			want: withDefaults(server.ServerConfig{
				AllowedOrigins: []string{"http://foo.com", "http://bar.com"},
			}),
		},
		{
			desc: "allowed hosts",
			args: []string{"--allowed-hosts", "http://foo.com,http://bar.com"},
			want: withDefaults(server.ServerConfig{
				AllowedHosts: []string{"http://foo.com", "http://bar.com"},
			}),
		},
		{
			desc: "http max request bytes",
			args: []string{"--http-max-request-bytes", "2097152"},
			want: withDefaults(server.ServerConfig{
				HttpMaxRequestBytes: 2097152,
			}),
		},
		{
			desc: "user agent metadata",
			args: []string{"--user-agent-metadata", "foo,bar"},
			want: withDefaults(server.ServerConfig{
				UserAgentMetadata: []string{"foo", "bar"},
			}),
		},
		{
			desc: "cert file",
			args: []string{"--tls-cert", "cert.pem"},
			want: withDefaults(server.ServerConfig{
				CertFile: "cert.pem",
			}),
		},
		{
			desc: "key file",
			args: []string{"--tls-key", "key.pem"},
			want: withDefaults(server.ServerConfig{
				KeyFile: "key.pem",
			}),
		},
		{
			desc: "disable ext",
			args: []string{"--disable-ext", "io.modelcontextprotocol/tasks"},
			want: withDefaults(server.ServerConfig{
				DisableExt: []string{"io.modelcontextprotocol/tasks"},
			}),
		},
		{
			desc: "tool suggestions",
			args: []string{"--tool-suggestions", "off"},
			want: withDefaults(server.ServerConfig{
				ToolSuggestions: tools.SuggestionsOff,
			}),
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, opts, _, err := invokeCommand(tc.args)
			if err != nil {
				t.Fatalf("unexpected error invoking command: %s", err)
			}

			if !cmp.Equal(opts.Cfg, tc.want) {
				t.Fatalf("got %v, want %v", opts.Cfg, tc.want)
			}
		})
	}
}

func TestConfigFlag(t *testing.T) {
	tcs := []struct {
		desc string
		args []string
		want string
	}{
		{
			desc: "default value",
			args: []string{},
			want: "",
		},
		{
			desc: "foo file",
			args: []string{"--config", "foo.yaml"},
			want: "foo.yaml",
		},
		{
			desc: "address long",
			args: []string{"--config", "bar.yaml"},
			want: "bar.yaml",
		},
		{
			desc: "deprecated flag",
			args: []string{"--tools-file", "foo.yaml"},
			want: "foo.yaml",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, opts, _, err := invokeCommand(tc.args)
			if err != nil {
				t.Fatalf("unexpected error invoking command: %s", err)
			}
			if opts.Config != tc.want {
				t.Fatalf("got %v, want %v", opts.Cfg, tc.want)
			}
		})
	}
}

func TestConfigsFlag(t *testing.T) {
	tcs := []struct {
		desc string
		args []string
		want []string
	}{
		{
			desc: "no value",
			args: []string{},
			want: []string{},
		},
		{
			desc: "single file",
			args: []string{"--configs", "foo.yaml"},
			want: []string{"foo.yaml"},
		},
		{
			desc: "multiple files",
			args: []string{"--configs", "foo.yaml,bar.yaml"},
			want: []string{"foo.yaml", "bar.yaml"},
		},
		{
			desc: "deprecated flag",
			args: []string{"--tools-files", "foo.yaml,bar.yaml"},
			want: []string{"foo.yaml", "bar.yaml"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, opts, _, err := invokeCommand(tc.args)
			if err != nil {
				t.Fatalf("unexpected error invoking command: %s", err)
			}
			if diff := cmp.Diff(opts.Configs, tc.want); diff != "" {
				t.Fatalf("got %v, want %v", opts.Configs, tc.want)
			}
		})
	}
}

func TestConfigFolderFlag(t *testing.T) {
	tcs := []struct {
		desc string
		args []string
		want string
	}{
		{
			desc: "no value",
			args: []string{},
			want: "",
		},
		{
			desc: "folder set",
			args: []string{"--config-folder", "test-folder"},
			want: "test-folder",
		},
		{
			desc: "deprecated flag",
			args: []string{"--tools-folder", "test-folder"},
			want: "test-folder",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, opts, _, err := invokeCommand(tc.args)
			if err != nil {
				t.Fatalf("unexpected error invoking command: %s", err)
			}
			if opts.ConfigFolder != tc.want {
				t.Fatalf("got %v, want %v", opts.ConfigFolder, tc.want)
			}
		})
	}
}

func TestPrebuiltFlag(t *testing.T) {
	tcs := []struct {
		desc string
		args []string
		want []string
	}{
		{
			desc: "default value",
			args: []string{},
			want: []string{},
		},
		{
			desc: "single prebuilt flag",
			args: []string{"--prebuilt", "alloydb"},
			want: []string{"alloydb"},
		},
		{
			desc: "multiple prebuilt flags",
			args: []string{"--prebuilt", "alloydb", "--prebuilt", "bigquery"},
			want: []string{"alloydb", "bigquery"},
		},
		{
			desc: "comma separated prebuilt flags",
			args: []string{"--prebuilt", "alloydb,bigquery"},
			want: []string{"alloydb", "bigquery"},
		},
		{
			desc: "prebuilt toolset flag",
			args: []string{"--prebuilt", "alloydb-postgres/monitor"},
			want: []string{"alloydb-postgres/monitor"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, opts, _, err := invokeCommand(tc.args)
			if err != nil {
				t.Fatalf("unexpected error invoking command: %s", err)
			}
			if diff := cmp.Diff(opts.PrebuiltConfigs, tc.want); diff != "" {
				t.Fatalf("got %v, want %v, diff %s", opts.PrebuiltConfigs, tc.want, diff)
			}
		})
	}
}

func TestFailServerConfigFlags(t *testing.T) {
	tcs := []struct {
		desc string
		args []string
	}{
		{
			desc: "logging format",
			args: []string{"--logging-format", "fail"},
		},
		{
			desc: "debug logs",
			args: []string{"--log-level", "fail"},
		},
		{
			desc: "tool suggestions",
			args: []string{"--tool-suggestions", "fail"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, err := invokeCommand(tc.args)
			if err == nil {
				t.Fatalf("expected an error, but got nil")
			}
		})
	}
}

func TestDefaultLoggingFormat(t *testing.T) {
	_, opts, _, err := invokeCommand([]string{})
	if err != nil {
		t.Fatalf("unexpected error invoking command: %s", err)
	}
	got := opts.Cfg.LoggingFormat.String()
	want := "standard"
	if got != want {
		t.Fatalf("unexpected default logging format flag: got %v, want %v", got, want)
	}
}

func TestDefaultLogLevel(t *testing.T) {
	_, opts, _, err := invokeCommand([]string{})
	if err != nil {
		t.Fatalf("unexpected error invoking command: %s", err)
	}
	got := opts.Cfg.LogLevel.String()
	want := "info"
	if got != want {
		t.Fatalf("unexpected default log level flag: got %v, want %v", got, want)
	}
}

func TestDefaultToolSuggestions(t *testing.T) {
	_, opts, _, err := invokeCommand([]string{})
	if err != nil {
		t.Fatalf("unexpected error invoking command: %s", err)
	}
	got := opts.Cfg.ToolSuggestions.String()
	want := "full"
	if got != want {
		t.Fatalf("unexpected default tool suggestions flag: got %v, want %v", got, want)
	}
}

// normalizeFilepaths is a helper function to allow same filepath formats for Mac and Windows.
// this prevents needing multiple "want" cases for TestResolveWatcherInputs
func normalizeFilepaths(m map[string]bool) map[string]bool {
	newMap := make(map[string]bool)
	for k, v := range m {
		newMap[filepath.ToSlash(k)] = v
	}
	return newMap
}

func TestResolveWatcherInputs(t *testing.T) {
	tcs := []struct {
		description      string
		toolsFile        string
		toolsFiles       []string
		toolsFolder      string
		wantWatchDirs    map[string]bool
		wantWatchedFiles map[string]bool
	}{
		{
			description:      "single config",
			toolsFile:        "tools_folder/example_tools.yaml",
			toolsFiles:       []string{},
			toolsFolder:      "",
			wantWatchDirs:    map[string]bool{"tools_folder": true},
			wantWatchedFiles: map[string]bool{"tools_folder/example_tools.yaml": true},
		},
		{
			description:      "default config (root dir)",
			toolsFile:        "tools.yaml",
			toolsFiles:       []string{},
			toolsFolder:      "",
			wantWatchDirs:    map[string]bool{".": true},
			wantWatchedFiles: map[string]bool{"tools.yaml": true},
		},
		{
			description:   "multiple files in different folders",
			toolsFile:     "",
			toolsFiles:    []string{"tools_folder/example_tools.yaml", "tools_folder2/example_tools.yaml"},
			toolsFolder:   "",
			wantWatchDirs: map[string]bool{"tools_folder": true, "tools_folder2": true},
			wantWatchedFiles: map[string]bool{
				"tools_folder/example_tools.yaml":  true,
				"tools_folder2/example_tools.yaml": true,
			},
		},
		{
			description:   "multiple files in same folder",
			toolsFile:     "",
			toolsFiles:    []string{"tools_folder/example_tools.yaml", "tools_folder/example_tools2.yaml"},
			toolsFolder:   "",
			wantWatchDirs: map[string]bool{"tools_folder": true},
			wantWatchedFiles: map[string]bool{
				"tools_folder/example_tools.yaml":  true,
				"tools_folder/example_tools2.yaml": true,
			},
		},
		{
			description: "multiple files in different levels",
			toolsFile:   "",
			toolsFiles: []string{
				"tools_folder/example_tools.yaml",
				"tools_folder/special_tools/example_tools2.yaml"},
			toolsFolder:   "",
			wantWatchDirs: map[string]bool{"tools_folder": true, "tools_folder/special_tools": true},
			wantWatchedFiles: map[string]bool{
				"tools_folder/example_tools.yaml":                true,
				"tools_folder/special_tools/example_tools2.yaml": true,
			},
		},
		{
			description:      "tools folder",
			toolsFile:        "",
			toolsFiles:       []string{},
			toolsFolder:      "tools_folder",
			wantWatchDirs:    map[string]bool{"tools_folder": true},
			wantWatchedFiles: map[string]bool{},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.description, func(t *testing.T) {
			gotWatchDirs, gotWatchedFiles := resolveWatcherInputs(tc.toolsFile, tc.toolsFiles, tc.toolsFolder)

			normalizedGotWatchDirs := normalizeFilepaths(gotWatchDirs)
			normalizedGotWatchedFiles := normalizeFilepaths(gotWatchedFiles)

			if diff := cmp.Diff(tc.wantWatchDirs, normalizedGotWatchDirs); diff != "" {
				t.Errorf("incorrect watchDirs: diff %v", diff)
			}
			if diff := cmp.Diff(tc.wantWatchedFiles, normalizedGotWatchedFiles); diff != "" {
				t.Errorf("incorrect watchedFiles: diff %v", diff)
			}

		})
	}
}

// helper function for testing file detection in dynamic reloading
func tmpFileWithCleanup(content []byte) (string, func(), error) {
	f, err := os.CreateTemp("", "*")
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

func TestSingleEdit(t *testing.T) {
	ctx, cancelCtx := context.WithTimeout(context.Background(), time.Minute)
	defer cancelCtx()

	pr, pw := io.Pipe()
	defer pw.Close()
	defer pr.Close()

	fileToWatch, cleanup, err := tmpFileWithCleanup([]byte("initial content"))
	if err != nil {
		t.Fatalf("error editing config %s", err)
	}
	defer cleanup()

	logger, err := log.NewStdLogger(pw, pw, "DEBUG")
	if err != nil {
		t.Fatalf("failed to setup logger %s", err)
	}
	ctx = util.WithLogger(ctx, logger)

	instrumentation, err := telemetry.CreateTelemetryInstrumentation(versionString)
	if err != nil {
		t.Fatalf("failed to setup instrumentation %s", err)
	}
	ctx = util.WithInstrumentation(ctx, instrumentation)

	mockServer := &server.Server{}

	cleanFileToWatch := filepath.Clean(fileToWatch)
	watchDir := filepath.Dir(cleanFileToWatch)

	watchedFiles := map[string]bool{cleanFileToWatch: true}
	watchDirs := map[string]bool{watchDir: true}

	go watchChanges(ctx, watchDirs, watchedFiles, mockServer, &internal.ToolboxOptions{})

	// escape backslash so regex doesn't fail on windows filepaths
	regexEscapedPathFile := strings.ReplaceAll(cleanFileToWatch, `\`, `\\\\*\\`)
	regexEscapedPathFile = path.Clean(regexEscapedPathFile)

	regexEscapedPathDir := strings.ReplaceAll(watchDir, `\`, `\\\\*\\`)
	regexEscapedPathDir = path.Clean(regexEscapedPathDir)

	begunWatchingDir := regexp.MustCompile(fmt.Sprintf(`DEBUG "Added directory %s to watcher."`, regexEscapedPathDir))
	_, err = testutils.WaitForString(ctx, begunWatchingDir, pr)
	if err != nil {
		t.Fatalf("timeout or error waiting for watcher to start: %s", err)
	}

	err = os.WriteFile(fileToWatch, []byte("modification"), 0777)
	if err != nil {
		t.Fatalf("error writing to file: %v", err)
	}

	// only check substring of DEBUG message due to some OS/editors firing different operations
	detectedFileChange := regexp.MustCompile(fmt.Sprintf(`event detected in %s"`, regexEscapedPathFile))
	_, err = testutils.WaitForString(ctx, detectedFileChange, pr)
	if err != nil {
		t.Fatalf("timeout or error waiting for file to detect write: %s", err)
	}
}

func TestMutuallyExclusiveFlags(t *testing.T) {
	testCases := []struct {
		desc      string
		args      []string
		errString string
	}{
		{
			desc:      "--config and --configs",
			args:      []string{"--config", "my.yaml", "--configs", "a.yaml,b.yaml"},
			errString: "if any flags in the group [config configs config-folder tools-file tools-files tools-folder] are set none of the others can be; [config configs] were all set",
		},
		{
			desc:      "--config-folder and --configs",
			args:      []string{"--config-folder", "./", "--configs", "a.yaml,b.yaml"},
			errString: "if any flags in the group [config configs config-folder tools-file tools-files tools-folder] are set none of the others can be; [config-folder configs] were all set",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			buf := new(bytes.Buffer)
			opts := internal.NewToolboxOptions(internal.WithIOStreams(buf, buf))
			cmd := NewCommand(opts)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected an error but got none")
			}
			if !strings.Contains(err.Error(), tc.errString) {
				t.Errorf("expected error message to contain %q, but got %q", tc.errString, err.Error())
			}
		})
	}
}

func TestFileLoadingErrors(t *testing.T) {
	t.Run("non-existent config", func(t *testing.T) {
		buf := new(bytes.Buffer)
		opts := internal.NewToolboxOptions(internal.WithIOStreams(buf, buf))
		cmd := NewCommand(opts)
		// Use a file that is guaranteed not to exist
		nonExistentFile := filepath.Join(t.TempDir(), "non-existent-tools.yaml")
		cmd.SetArgs([]string{"--config", nonExistentFile})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected an error for non-existent file but got none")
		}
		if !strings.Contains(err.Error(), "unable to read config") {
			t.Errorf("expected error about reading file, but got: %v", err)
		}
	})

	t.Run("non-existent config-folder", func(t *testing.T) {
		buf := new(bytes.Buffer)
		opts := internal.NewToolboxOptions(internal.WithIOStreams(buf, buf))
		cmd := NewCommand(opts)
		nonExistentFolder := filepath.Join(t.TempDir(), "non-existent-folder")
		cmd.SetArgs([]string{"--config-folder", nonExistentFolder})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected an error for non-existent folder but got none")
		}
		if !strings.Contains(err.Error(), "unable to access config folder") {
			t.Errorf("expected error about accessing folder, but got: %v", err)
		}
	})
}

func TestPrebuiltAndCustomTools(t *testing.T) {
	t.Setenv("SQLITE_DATABASE", "\":memory:\"")
	// Setup custom config
	customContent := `
kind: tool
name: custom_tool
type: http
source: my-http
method: GET
path: /
description: "A custom tool for testing"
---
kind: source
name: my-http
type: http
baseUrl: http://example.com
`
	customFile := filepath.Join(t.TempDir(), "custom.yaml")
	if err := os.WriteFile(customFile, []byte(customContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Tool Conflict File
	// SQLite prebuilt has a tool named 'list_tables'
	toolConflictContent := `
kind: tool
name: list_tables
type: http
source: my-http
method: GET
path: /
description: "Conflicting tool"
---
kind: source
name: my-http
type: http
baseUrl: http://example.com
`
	toolConflictFile := filepath.Join(t.TempDir(), "tool_conflict.yaml")
	if err := os.WriteFile(toolConflictFile, []byte(toolConflictContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Source Conflict File
	// SQLite prebuilt has a source named 'sqlite-source'
	sourceConflictContent := `
kind: source
name: sqlite-source
type: http
baseUrl: http://example.com
---
kind: tool
name: dummy_tool
type: http
source: sqlite-source
method: GET
path: /
description: "Dummy"
`
	sourceConflictFile := filepath.Join(t.TempDir(), "source_conflict.yaml")
	if err := os.WriteFile(sourceConflictFile, []byte(sourceConflictContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Toolset Conflict File
	// SQLite prebuilt has a toolset named 'sqlite_database_tools'
	toolsetConflictContent := `
kind: source
name: dummy-src
type: http
baseUrl: http://example.com
---
kind: tool
name: dummy_tool
type: http
source: dummy-src
method: GET
path: /
description: "Dummy"
---
kind: toolset
name: sqlite_database_tools
tools:
- dummy_tool
`
	toolsetConflictFile := filepath.Join(t.TempDir(), "toolset_conflict.yaml")
	if err := os.WriteFile(toolsetConflictFile, []byte(toolsetConflictContent), 0644); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		desc      string
		args      []string
		wantErr   bool
		errString string
		cfgCheck  func(server.ServerConfig) error
	}{
		{
			desc:    "success mixed",
			args:    []string{"--prebuilt", "sqlite", "--config", customFile},
			wantErr: false,
			cfgCheck: func(cfg server.ServerConfig) error {
				if _, ok := cfg.ToolConfigs["custom_tool"]; !ok {
					return fmt.Errorf("custom tool not found")
				}
				if _, ok := cfg.ToolConfigs["list_tables"]; !ok {
					return fmt.Errorf("prebuilt tool 'list_tables' not found")
				}
				return nil
			},
		},
		{
			desc:      "sqlite called twice error",
			args:      []string{"--prebuilt", "sqlite", "--prebuilt", "sqlite"},
			wantErr:   true,
			errString: "resource conflicts detected",
		},
		{
			desc:      "tool conflict error",
			args:      []string{"--prebuilt", "sqlite", "--config", toolConflictFile},
			wantErr:   true,
			errString: "resource conflicts detected",
		},
		{
			desc:      "source conflict error",
			args:      []string{"--prebuilt", "sqlite", "--config", sourceConflictFile},
			wantErr:   true,
			errString: "resource conflicts detected",
		},
		{
			desc:      "toolset conflict error",
			args:      []string{"--prebuilt", "sqlite", "--config", toolsetConflictFile},
			wantErr:   true,
			errString: "resource conflicts detected",
		},
		{
			desc:    "success toolset filtering",
			args:    []string{"--prebuilt", "sqlite/sqlite_database_tools"},
			wantErr: false,
			cfgCheck: func(cfg server.ServerConfig) error {
				if _, ok := cfg.ToolConfigs["execute_sql"]; !ok {
					return fmt.Errorf("expected tool 'execute_sql' not found")
				}
				if _, ok := cfg.ToolConfigs["list_tables"]; !ok {
					return fmt.Errorf("expected tool 'list_tables' not found")
				}
				if len(cfg.ToolConfigs) != 2 {
					return fmt.Errorf("expected exactly 2 tools, got %d", len(cfg.ToolConfigs))
				}
				if _, ok := cfg.GroupConfigs["sqlite_database_tools"]; !ok {
					return fmt.Errorf("expected group 'sqlite_database_tools' not found")
				}
				// Legacy toolsets are folded into groups, and the default nameless
				// collection is seeded later as a derived group, so only the named
				// group remains in the parsed config.
				if len(cfg.GroupConfigs) != 1 {
					var names []string
					for k := range cfg.GroupConfigs {
						names = append(names, k)
					}
					return fmt.Errorf("expected exactly 1 group, got %d: %v", len(cfg.GroupConfigs), names)
				}
				return nil
			},
		},
		{
			desc:      "toolset not found error",
			args:      []string{"--prebuilt", "sqlite/nonexistent"},
			wantErr:   true,
			errString: "toolset 'nonexistent' not found in prebuilt configuration 'sqlite'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			_, opts, output, err := invokeCommandWithContext(ctx, tc.args)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error but got none")
				}
				if !strings.Contains(err.Error(), tc.errString) {
					t.Errorf("expected error message to contain %q, but got %q", tc.errString, err.Error())
				}
			} else {
				if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(output, "Server ready to serve!") {
					t.Errorf("server did not start successfully (no ready message found). Output:\n%s", output)
				}
				if tc.cfgCheck != nil {
					if err := tc.cfgCheck(opts.Cfg); err != nil {
						t.Errorf("config check failed: %v. Output:\n%s", err, output)
					}
				}
			}
		})
	}
}

func TestDefaultConfigBehavior(t *testing.T) {
	t.Setenv("SQLITE_DATABASE", "\":memory:\"")
	testCases := []struct {
		desc      string
		args      []string
		expectRun bool
		errString string
	}{
		{
			desc:      "no flags (defaults to tools.yaml)",
			args:      []string{},
			expectRun: false,
			errString: "tools.yaml", // Expect error because tools.yaml doesn't exist in test env
		},
		{
			desc:      "prebuilt only (skips tools.yaml)",
			args:      []string{"--prebuilt", "sqlite"},
			expectRun: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			_, _, output, err := invokeCommandWithContext(ctx, tc.args)

			if tc.expectRun {
				if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
					t.Fatalf("expected server start, got error: %v", err)
				}
				// Verify it actually started
				if !strings.Contains(output, "Server ready to serve!") {
					t.Errorf("server did not start successfully (no ready message found). Output:\n%s", output)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error reading default file, got nil")
				}
				if !strings.Contains(err.Error(), tc.errString) {
					t.Errorf("expected error message to contain %q, but got %q", tc.errString, err.Error())
				}
			}
		})
	}
}

func TestSubcommandWiring(t *testing.T) {
	buf := new(bytes.Buffer)
	opts := internal.NewToolboxOptions(internal.WithIOStreams(buf, buf))
	baseCmd := NewCommand(opts)

	tests := []struct {
		args         []string
		expectedName string
	}{
		{[]string{"invoke"}, "invoke"},
		{[]string{"skills-generate"}, "skills-generate"},
		{[]string{"serve"}, "serve"},
	}

	for _, tc := range tests {
		// Find returns the Command struct and the remaining args
		cmd, _, err := baseCmd.Find(tc.args)

		if err != nil {
			t.Fatalf("Failed to find command %v: %v", tc.args, err)
		}

		if cmd.Name() != tc.expectedName {
			t.Errorf("Expected command name %q, got %q", tc.expectedName, cmd.Name())
		}
	}
}

func TestIgnoreUnknownToolsFlag(t *testing.T) {
	invalidContent := `
kind: tool
name: invalid_tool
type: unregistered-tool-type
source: my-http
description: "A tool with unregistered type"
---
kind: source
name: my-http
type: http
baseUrl: http://example.com
`
	invalidFile := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(invalidFile, []byte(invalidContent), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("without ignore flag fails", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		_, _, _, err := invokeCommandWithContext(ctx, []string{"--config", invalidFile})
		if err == nil {
			t.Fatalf("expected error due to unregistered tool type, got nil")
		}
		if !strings.Contains(err.Error(), "unknown tool type") {
			t.Errorf("expected error containing 'unknown tool type', got: %v", err)
		}
	})

	t.Run("with ignore flag succeeds and skips tool", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		_, opts, output, err := invokeCommandWithContext(ctx, []string{"--config", invalidFile, "--ignore-unknown-tools"})
		if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(output, "Server ready to serve!") {
			t.Errorf("server did not start successfully. Output:\n%s", output)
		}
		if _, ok := opts.Cfg.ToolConfigs["invalid_tool"]; ok {
			t.Errorf("expected 'invalid_tool' to be skipped and filtered out, but it was found in ToolConfigs")
		}
	})
}

func TestMCPAuthEnableAPIClashCLI(t *testing.T) {
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "config.yaml")
	configContent := `
authServices:
  generic1:
    type: generic
    audience: aud
    mcpEnabled: true
    authorizationServer: https://example.com/oauth
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}

	buf := new(bytes.Buffer)
	opts := internal.NewToolboxOptions(internal.WithIOStreams(buf, buf))
	cmd := NewCommand(opts)
	cmd.SetArgs([]string{"--config", configFile, "--enable-api"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when running with MCP Auth and --enable-api, got nil")
	}
	if !strings.Contains(err.Error(), "MCP Auth cannot be enabled together with the legacy HTTP API") {
		t.Errorf("unexpected error: %v", err)
	}
}
