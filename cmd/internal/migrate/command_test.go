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
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/cmd/internal"
	"github.com/spf13/cobra"
)

func invokeCommand(args []string) (string, error) {
	parentCmd := &cobra.Command{Use: "toolbox"}

	buf := new(bytes.Buffer)
	opts := internal.NewToolboxOptions(internal.WithIOStreams(buf, buf))
	internal.PersistentFlags(parentCmd, opts)

	cmd := NewCommand(opts)
	parentCmd.AddCommand(cmd)
	parentCmd.SetArgs(args)

	err := parentCmd.Execute()
	return buf.String(), err
}

func TestMigrate(t *testing.T) {
	toolsFileContent := `
sources:
  my-pg-instance:
    kind: cloud-sql-postgres
    project: my-project
    region: my-region
    instance: my-instance
    database: my_db
    user: my_user
    password: my_pass`

	toolsFileContentNew := `
kind: source
name: my-pg-instance
type: cloud-sql-postgres
project: my-project
region: my-region
instance: my-instance
database: my_db
user: my_user
password: my_pass
`
	toolsFileContent2 := `
tools:
  example_tool2:
    kind: postgres-sql
    source: my-pg-instance
    description: some description
    statement: SELECT * FROM SQL_STATEMENT;
    parameters:
      - name: country
        type: string
        description: some description`

	toolsFileContent2New := `
kind: tool
name: example_tool2
type: postgres-sql
source: my-pg-instance
description: some description
statement: SELECT * FROM SQL_STATEMENT;
parameters:
- name: country
  type: string
  description: some description
`

	t.Run("migrate tools file", func(t *testing.T) {
		tmpDir := t.TempDir()

		toolsFilePath := filepath.Join(tmpDir, "foo.yaml")
		if err := os.WriteFile(toolsFilePath, []byte(toolsFileContent), 0644); err != nil {
			t.Fatalf("failed to write tools file: %v", err)
		}

		args := []string{"migrate", "--tools-file", toolsFilePath}
		got, err := invokeCommand(args)
		if err != nil {
			t.Fatalf("command failed: %v\nOutput: %s", err, got)
		}

		// verify backup file
		backupFile := toolsFilePath + ".bak"
		_, err = os.Stat(backupFile)
		if err != nil {
			t.Fatalf("error verifying backup file: %v", err)
		}
		actualContent, err := os.ReadFile(backupFile)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContent)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContent, actualContent)
		}

		// check content of new file
		actualContent, err = os.ReadFile(toolsFilePath)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContentNew)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContentNew, actualContent)
		}
	})
	t.Run("migrate toolset to group", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Every spelling of a toolset: a nested bare list, a nested map (which
		// can carry an unsupported description), and an already-flat doc.
		toolsetFileContent := `
toolsets:
  bare_list_toolset:
    - example_tool
  map_toolset:
    description: nested description
    tools:
    - example_tool
---
kind: toolset
name: flat_toolset
description: flat description
tools:
- example_tool`
		toolsetFileContentNew := `
kind: group
name: bare_list_toolset
tools:
- example_tool
---
kind: group
name: map_toolset
tools:
- example_tool
---
kind: group
name: flat_toolset
tools:
- example_tool
`

		toolsFilePath := filepath.Join(tmpDir, "toolsets.yaml")
		if err := os.WriteFile(toolsFilePath, []byte(toolsetFileContent), 0644); err != nil {
			t.Fatalf("failed to write tools file: %v", err)
		}

		args := []string{"migrate", "--tools-file", toolsFilePath}
		got, err := invokeCommand(args)
		if err != nil {
			t.Fatalf("command failed: %v\nOutput: %s", err, got)
		}

		for _, name := range []string{"map_toolset", "flat_toolset"} {
			// The logger renders the message as a quoted string, so the
			// name's own quotes come through escaped.
			want := fmt.Sprintf(`toolset \"%s\": dropping description`, name)
			if !strings.Contains(got, want) {
				t.Errorf("missing warning %q in output: %s", want, got)
			}
		}
		if strings.Contains(got, `bare_list_toolset\": dropping description`) {
			t.Errorf("unexpected warning for a toolset with no description: %s", got)
		}

		actualContent, err := os.ReadFile(toolsFilePath)
		if err != nil {
			t.Fatalf("failed to read migrated file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsetFileContentNew)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsetFileContentNew, actualContent)
		}
	})
	t.Run("migrate tools files", func(t *testing.T) {
		tmpDir := t.TempDir()

		toolsFilePath1 := filepath.Join(tmpDir, "foo.yaml")
		if err := os.WriteFile(toolsFilePath1, []byte(toolsFileContent), 0644); err != nil {
			t.Fatalf("failed to write tools file: %v", err)
		}
		toolsFilePath2 := filepath.Join(tmpDir, "foo2.yaml")
		if err := os.WriteFile(toolsFilePath2, []byte(toolsFileContent2), 0644); err != nil {
			t.Fatalf("failed to write tools file: %v", err)
		}

		toolsFiles := toolsFilePath1 + "," + toolsFilePath2
		args := []string{"migrate", "--tools-files", toolsFiles}
		got, err := invokeCommand(args)
		if err != nil {
			t.Fatalf("command failed: %v\nOutput: %s", err, got)
		}

		// verify backup file1
		backupFile := toolsFilePath1 + ".bak"
		_, err = os.Stat(backupFile)
		if err != nil {
			t.Fatalf("error verifying backup file: %v", err)
		}
		actualContent, err := os.ReadFile(backupFile)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContent)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContent, actualContent)
		}

		// verify backup file2
		backupFile = toolsFilePath2 + ".bak"
		_, err = os.Stat(backupFile)
		if err != nil {
			t.Fatalf("error verifying backup file: %v", err)
		}
		actualContent, err = os.ReadFile(backupFile)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContent2)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContent2, actualContent)
		}

		// check content of new file1
		actualContent, err = os.ReadFile(toolsFilePath1)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContentNew)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContentNew, actualContent)
		}

		// check content of new file2
		actualContent, err = os.ReadFile(toolsFilePath2)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContent2New)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContent2New, actualContent)
		}
	})

	t.Run("migrate tools folder", func(t *testing.T) {
		tmpDir := t.TempDir()

		toolsFilePath1 := filepath.Join(tmpDir, "foo.yaml")
		if err := os.WriteFile(toolsFilePath1, []byte(toolsFileContent), 0644); err != nil {
			t.Fatalf("failed to write tools file: %v", err)
		}
		toolsFilePath2 := filepath.Join(tmpDir, "foo2.yaml")
		if err := os.WriteFile(toolsFilePath2, []byte(toolsFileContent2), 0644); err != nil {
			t.Fatalf("failed to write tools file: %v", err)
		}

		args := []string{"migrate", "--tools-folder", tmpDir}
		got, err := invokeCommand(args)
		if err != nil {
			t.Fatalf("command failed: %v\nOutput: %s", err, got)
		}

		// verify backup file1
		backupFile := toolsFilePath1 + ".bak"
		_, err = os.Stat(backupFile)
		if err != nil {
			t.Fatalf("error verifying backup file: %v", err)
		}
		actualContent, err := os.ReadFile(backupFile)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContent)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContent, actualContent)
		}

		// verify backup file2
		backupFile = toolsFilePath2 + ".bak"
		_, err = os.Stat(backupFile)
		if err != nil {
			t.Fatalf("error verifying backup file: %v", err)
		}
		actualContent, err = os.ReadFile(backupFile)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContent2)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContent2, actualContent)
		}

		// check content of new file1
		actualContent, err = os.ReadFile(toolsFilePath1)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContentNew)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContentNew, actualContent)
		}

		// check content of new file2
		actualContent, err = os.ReadFile(toolsFilePath2)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContent2New)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContent2New, actualContent)
		}
	})
}

func TestMigrateDryRun(t *testing.T) {
	toolsFileContent := `
sources:
  my-pg-instance:
    kind: cloud-sql-postgres
    project: my-project
    region: my-region
    instance: my-instance
    database: my_db
    user: my_user
    password: my_pass
`

	toolsFileContentNew := `
kind: source
name: my-pg-instance
type: cloud-sql-postgres
project: my-project
region: my-region
instance: my-instance
database: my_db
user: my_user
password: my_pass
`
	toolsFileContent2 := `
tools:
  example_tool2:
    kind: postgres-sql
    source: my-pg-instance
    description: |
      some description
      - string
    statement: SELECT * FROM SQL_STATEMENT;
    parameters:
      - name: country
        type: string
        description: some description`

	toolsFileContent2New := `
kind: tool
name: example_tool2
type: postgres-sql
source: my-pg-instance
description: |
  some description
  - string
statement: SELECT * FROM SQL_STATEMENT;
parameters:
- name: country
  type: string
  description: some description
`

	t.Run("migrate tools file", func(t *testing.T) {
		tmpDir := t.TempDir()

		toolsFilePath := filepath.Join(tmpDir, "foo.yaml")
		if err := os.WriteFile(toolsFilePath, []byte(toolsFileContent), 0644); err != nil {
			t.Fatalf("failed to write tools file: %v", err)
		}

		args := []string{"migrate", "--tools-file", toolsFilePath, "--dry-run"}
		got, err := invokeCommand(args)
		if err != nil {
			t.Fatalf("command failed: %v\nOutput: %s", err, got)
		}

		// verify original file
		actualContent, err := os.ReadFile(toolsFilePath)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContent)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContent, actualContent)
		}

		// check output
		if !strings.Contains(got, toolsFileContentNew) {
			t.Fatalf("expected output not found!\nExpected: %q\nGot: %q", toolsFileContentNew, got)
		}
	})
	t.Run("migrate tools files", func(t *testing.T) {
		tmpDir := t.TempDir()

		toolsFilePath1 := filepath.Join(tmpDir, "foo.yaml")
		if err := os.WriteFile(toolsFilePath1, []byte(toolsFileContent), 0644); err != nil {
			t.Fatalf("failed to write tools file: %v", err)
		}
		toolsFilePath2 := filepath.Join(tmpDir, "foo2.yaml")
		if err := os.WriteFile(toolsFilePath2, []byte(toolsFileContent2), 0644); err != nil {
			t.Fatalf("failed to write tools file: %v", err)
		}

		toolsFiles := toolsFilePath1 + "," + toolsFilePath2
		args := []string{"migrate", "--tools-files", toolsFiles, "--dry-run"}
		got, err := invokeCommand(args)
		if err != nil {
			t.Fatalf("command failed: %v\nOutput: %s", err, got)
		}

		// verify original file1
		actualContent, err := os.ReadFile(toolsFilePath1)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContent)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContent, actualContent)
		}

		// verify original file2
		actualContent, err = os.ReadFile(toolsFilePath2)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContent2)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContent2, actualContent)
		}

		// check output
		if !strings.Contains(got, toolsFileContentNew) {
			t.Fatalf("expected output not found!\nExpected: %q\nGot: %q", toolsFileContentNew, got)
		}
		if !strings.Contains(got, toolsFileContent2New) {
			t.Fatalf("expected output not found!\nExpected: %q\nGot: %q", toolsFileContent2New, got)
		}
	})

	t.Run("migrate tools folder", func(t *testing.T) {
		tmpDir := t.TempDir()

		toolsFilePath1 := filepath.Join(tmpDir, "foo.yaml")
		if err := os.WriteFile(toolsFilePath1, []byte(toolsFileContent), 0644); err != nil {
			t.Fatalf("failed to write tools file: %v", err)
		}
		toolsFilePath2 := filepath.Join(tmpDir, "foo2.yaml")
		if err := os.WriteFile(toolsFilePath2, []byte(toolsFileContent2), 0644); err != nil {
			t.Fatalf("failed to write tools file: %v", err)
		}

		args := []string{"migrate", "--tools-folder", tmpDir, "--dry-run"}
		got, err := invokeCommand(args)
		if err != nil {
			t.Fatalf("command failed: %v\nOutput: %s", err, got)
		}

		// verify original file1
		actualContent, err := os.ReadFile(toolsFilePath1)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContent)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContent, actualContent)
		}

		// verify original file2
		actualContent, err = os.ReadFile(toolsFilePath2)
		if err != nil {
			t.Fatalf("failed to read backup file: %v", err)
		}
		if !bytes.Equal(actualContent, []byte(toolsFileContent2)) {
			t.Fatalf("file content mismatch!\nExpected: %q\nGot: %q", toolsFileContent2, actualContent)
		}

		// check output
		if !strings.Contains(got, toolsFileContentNew) {
			t.Fatalf("expected output not found!\nExpected: %q\nGot: %q", toolsFileContentNew, got)
		}
		if !strings.Contains(got, toolsFileContent2New) {
			t.Fatalf("expected output not found!\nExpected: %q\nGot: %q", toolsFileContent2New, got)
		}
	})
}
