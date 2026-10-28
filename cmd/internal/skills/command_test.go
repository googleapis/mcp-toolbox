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

package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/cmd/internal"
	"github.com/googleapis/mcp-toolbox/internal/group"
	_ "github.com/googleapis/mcp-toolbox/internal/sources/sqlite"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	_ "github.com/googleapis/mcp-toolbox/internal/tools/sqlite/sqlitesql"
	"github.com/spf13/cobra"
)

func invokeCommand(args []string) (string, error) {
	parentCmd := &cobra.Command{
		Use:           "toolbox",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	buf := new(bytes.Buffer)
	opts := internal.NewToolboxOptions(internal.WithIOStreams(buf, buf))
	internal.PersistentFlags(parentCmd, opts)

	parentCmd.SetOut(buf)
	parentCmd.SetErr(buf)

	cmd := NewCommand(opts)
	parentCmd.AddCommand(cmd)
	parentCmd.SetArgs(args)

	err := parentCmd.Execute()
	return buf.String(), err
}

func TestGenerateSkill(t *testing.T) {
	// Create a temporary directory for tests
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "skills")

	// Create a tools.yaml file with a sqlite tool
	toolsFileContent := `
sources:
  my-sqlite:
    kind: sqlite
    database: ":memory:"
tools:
  hello-sqlite:
    kind: sqlite-sql
    source: my-sqlite
    description: "hello tool"
    statement: "SELECT 'hello' as greeting"
`

	toolsFilePath := filepath.Join(tmpDir, "tools.yaml")
	if err := os.WriteFile(toolsFilePath, []byte(toolsFileContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	args := []string{
		"skills-generate",
		"--config", toolsFilePath,
		"--output-dir", outputDir,
		"--name", "hello-sqlite",
		"--description", "hello tool",
	}

	got, err := invokeCommand(args)
	if err != nil {
		t.Fatalf("command failed: %v\nOutput: %s", err, got)
	}

	// Verify generated directory structure
	skillPath := filepath.Join(outputDir, "hello-sqlite")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		t.Fatalf("skill directory not created: %s", skillPath)
	}

	// Check SKILL.md
	skillMarkdown := filepath.Join(skillPath, "SKILL.md")
	content, err := os.ReadFile(skillMarkdown)
	if err != nil {
		t.Fatalf("failed to read SKILL.md: %v", err)
	}

	expectedFrontmatter := `---
name: hello-sqlite
description: hello tool
---`
	if !strings.HasPrefix(string(content), expectedFrontmatter) {
		t.Errorf("SKILL.md does not have expected frontmatter format.\nExpected prefix:\n%s\nGot:\n%s", expectedFrontmatter, string(content))
	}

	if !strings.Contains(string(content), "## Usage") {
		t.Errorf("SKILL.md does not contain '## Usage' section")
	}

	if !strings.Contains(string(content), "## Scripts") {
		t.Errorf("SKILL.md does not contain '## Scripts' section")
	}

	if !strings.Contains(string(content), "### hello-sqlite") {
		t.Errorf("SKILL.md does not contain '### hello-sqlite' tool header")
	}

	// Check script file
	scriptFilename := "hello-sqlite.js"
	scriptPath := filepath.Join(skillPath, "scripts", scriptFilename)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		t.Fatalf("script file not created: %s", scriptPath)
	}

	scriptContent, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("failed to read script file: %v", err)
	}
	if !strings.Contains(string(scriptContent), "hello-sqlite") {
		t.Errorf("script file does not contain expected tool name")
	}

	// Check assets
	assetPath := filepath.Join(skillPath, "assets", "tools.yaml")
	if _, err := os.Stat(assetPath); os.IsNotExist(err) {
		t.Fatalf("asset file not created: %s", assetPath)
	}
	assetContent, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatalf("failed to read asset file: %v", err)
	}
	if !strings.Contains(string(assetContent), "hello-sqlite") {
		t.Errorf("asset file does not contain expected tool name")
	}
}

func TestGenerateSkill_Toolsets(t *testing.T) {
	// Create a temporary directory for tests
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "skills")

	// Create a tools.yaml file with a sqlite tool
	toolsFileContent := `
sources:
  my-sqlite:
    kind: sqlite
    database: ":memory:"
tools:
  hello-sqlite:
    kind: sqlite-sql
    source: my-sqlite
    description: "hello tool"
    statement: "SELECT 'hello' as greeting"
  bye-sqlite:
    kind: sqlite-sql
    source: my-sqlite
    description: "bye tool"
    statement: "SELECT 'bye' as greeting"
toolsets:
  greeting:
    tools:
      - hello-sqlite
  farewell:
    tools:
      - bye-sqlite
`

	toolsFilePath := filepath.Join(tmpDir, "tools.yaml")
	if err := os.WriteFile(toolsFilePath, []byte(toolsFileContent), 0644); err != nil {
		t.Fatalf("failed to write tools file: %v", err)
	}

	args := []string{
		"skills-generate",
		"--tools-file", toolsFilePath,
		"--output-dir", outputDir,
		"--name", "my-skill",
		"--description", "My toolset skills",
	}

	got, err := invokeCommand(args)
	if err != nil {
		t.Fatalf("command failed: %v\nOutput: %s", err, got)
	}

	// Verify generated directory structures
	// First toolset skill
	skillPath1 := filepath.Join(outputDir, "my-skill-greeting")
	if _, err := os.Stat(skillPath1); os.IsNotExist(err) {
		t.Fatalf("skill directory not created: %s", skillPath1)
	}

	skillMarkdown1 := filepath.Join(skillPath1, "SKILL.md")
	content1, err := os.ReadFile(skillMarkdown1)
	if err != nil {
		t.Fatalf("failed to read SKILL.md: %v", err)
	}

	if !strings.Contains(string(content1), "### hello-sqlite") {
		t.Errorf("SKILL.md does not contain '### hello-sqlite' tool header")
	}
	if strings.Contains(string(content1), "### bye-sqlite") {
		t.Errorf("SKILL.md should not contain '### bye-sqlite' tool header")
	}

	// Second toolset skill
	skillPath2 := filepath.Join(outputDir, "my-skill-farewell")
	if _, err := os.Stat(skillPath2); os.IsNotExist(err) {
		t.Fatalf("skill directory not created: %s", skillPath2)
	}

	skillMarkdown2 := filepath.Join(skillPath2, "SKILL.md")
	content2, err := os.ReadFile(skillMarkdown2)
	if err != nil {
		t.Fatalf("failed to read SKILL.md: %v", err)
	}

	if !strings.Contains(string(content2), "### bye-sqlite") {
		t.Errorf("SKILL.md does not contain '### bye-sqlite' tool header")
	}
	if strings.Contains(string(content2), "### hello-sqlite") {
		t.Errorf("SKILL.md should not contain '### hello-sqlite' tool header")
	}
}

func TestGenerateSkill_SpecificToolset(t *testing.T) {
	// Create a temporary directory for tests
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "skills")

	// Create a tools.yaml file with a sqlite tool
	toolsFileContent := `
sources:
  my-sqlite:
    kind: sqlite
    database: ":memory:"
tools:
  hello-sqlite:
    kind: sqlite-sql
    source: my-sqlite
    description: "hello tool"
    statement: "SELECT 'hello' as greeting"
  bye-sqlite:
    kind: sqlite-sql
    source: my-sqlite
    description: "bye tool"
    statement: "SELECT 'bye' as greeting"
toolsets:
  greeting:
    tools:
      - hello-sqlite
  farewell:
    tools:
      - bye-sqlite
`

	toolsFilePath := filepath.Join(tmpDir, "tools.yaml")
	if err := os.WriteFile(toolsFilePath, []byte(toolsFileContent), 0644); err != nil {
		t.Fatalf("failed to write tools file: %v", err)
	}

	args := []string{
		"skills-generate",
		"--tools-file", toolsFilePath,
		"--output-dir", outputDir,
		"--name", "my-specific-skill",
		"--description", "My toolset skill",
		"--toolset", "farewell",
	}

	got, err := invokeCommand(args)
	if err != nil {
		t.Fatalf("command failed: %v\nOutput: %s", err, got)
	}

	// Because we specified a toolset, it outputs directly into my-specific-skill
	skillPath := filepath.Join(outputDir, "my-specific-skill")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		t.Fatalf("skill directory not created: %s", skillPath)
	}

	// Ensure other toolsets are not generated
	skillPathGreeting := filepath.Join(outputDir, "my-specific-skill-greeting")
	if _, err := os.Stat(skillPathGreeting); !os.IsNotExist(err) {
		t.Fatalf("skill directory should not have been created: %s", skillPathGreeting)
	}

	skillMarkdown := filepath.Join(skillPath, "SKILL.md")
	content, err := os.ReadFile(skillMarkdown)
	if err != nil {
		t.Fatalf("failed to read SKILL.md: %v", err)
	}

	if !strings.Contains(string(content), "### bye-sqlite") {
		t.Errorf("SKILL.md does not contain '### bye-sqlite' tool header")
	}
	if strings.Contains(string(content), "### hello-sqlite") {
		t.Errorf("SKILL.md should not contain '### hello-sqlite' tool header")
	}
}

func TestGenerateSkill_SpecificGroup(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "skills")

	// On this lineage groups are seeded from toolsets, so --group targets a
	// toolset-derived group by name.
	toolsFileContent := `
sources:
  my-sqlite:
    kind: sqlite
    database: ":memory:"
tools:
  hello-sqlite:
    kind: sqlite-sql
    source: my-sqlite
    description: "hello tool"
    statement: "SELECT 'hello' as greeting"
  bye-sqlite:
    kind: sqlite-sql
    source: my-sqlite
    description: "bye tool"
    statement: "SELECT 'bye' as greeting"
toolsets:
  greeting:
    - hello-sqlite
  farewell:
    - bye-sqlite
`

	toolsFilePath := filepath.Join(tmpDir, "tools.yaml")
	if err := os.WriteFile(toolsFilePath, []byte(toolsFileContent), 0644); err != nil {
		t.Fatalf("failed to write tools file: %v", err)
	}

	args := []string{
		"skills-generate",
		"--config", toolsFilePath,
		"--output-dir", outputDir,
		"--name", "my-group-skill",
		"--description", "fallback description",
		"--group", "farewell",
	}

	got, err := invokeCommand(args)
	if err != nil {
		t.Fatalf("command failed: %v\nOutput: %s", err, got)
	}

	// --group produces a single skill named exactly --name.
	skillPath := filepath.Join(outputDir, "my-group-skill")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		t.Fatalf("skill directory not created: %s", skillPath)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "my-group-skill-farewell")); !os.IsNotExist(err) {
		t.Fatalf("--group should not produce a per-group suffixed directory")
	}

	content, err := os.ReadFile(filepath.Join(skillPath, "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to read SKILL.md: %v", err)
	}
	if !strings.Contains(string(content), "### bye-sqlite") {
		t.Errorf("SKILL.md does not contain '### bye-sqlite' tool header")
	}
	if strings.Contains(string(content), "### hello-sqlite") {
		t.Errorf("SKILL.md should not contain '### hello-sqlite' tool header")
	}
}

func TestGenerateSkill_GroupAndToolsetMutuallyExclusive(t *testing.T) {
	tmpDir := t.TempDir()
	toolsFilePath := filepath.Join(tmpDir, "tools.yaml")
	if err := os.WriteFile(toolsFilePath, []byte("tools: {}"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	args := []string{
		"skills-generate",
		"--config", toolsFilePath,
		"--name", "test",
		"--group", "a",
		"--toolset", "b",
	}
	if _, err := invokeCommand(args); err == nil {
		t.Fatal("expected error when --group and --toolset are both set, got nil")
	}
}

func TestGenerateSkill_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "skills")

	args := []string{
		"skills-generate",
		"--output-dir", outputDir,
		"--name", "test",
		"--description", "test",
	}

	_, err := invokeCommand(args)
	if err == nil {
		t.Fatal("expected command to fail when no configuration is provided and tools.yaml is missing")
	}

	// Should not have created the directory if no config was processed
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		t.Errorf("output directory should not have been created")
	}
}

func TestGenerateSkill_MissingArguments(t *testing.T) {
	tmpDir := t.TempDir()
	toolsFilePath := filepath.Join(tmpDir, "tools.yaml")
	if err := os.WriteFile(toolsFilePath, []byte("tools: {}"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing name",
			args: []string{"skills-generate", "--config", toolsFilePath, "--description", "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := invokeCommand(tt.args)
			if err == nil {
				t.Fatalf("expected command to fail due to missing arguments, but it succeeded\nOutput: %s", got)
			}
		})
	}
}

func TestBuildSkillContents(t *testing.T) {
	tests := []struct {
		name      string
		cmd       *skillsCmd
		toolsMap  map[string]tools.Tool
		groupsMap map[string]group.Group
		want      map[string]skillContent
		wantErr   bool
	}{
		{
			// len(groupsMap) > 1 (default group plus named groups) triggers group mode.
			name: "group mode: group description takes precedence over flag, flag is fallback",
			cmd:  &skillsCmd{name: "my-skill", description: "flag fallback"},
			groupsMap: map[string]group.Group{
				"": group.NewGroup(group.GroupConfig{Name: ""}),
				"with-desc": group.NewGroup(
					group.GroupConfig{Name: "with-desc", Description: "group's own description"}),
				"no-desc": group.NewGroup(
					group.GroupConfig{Name: "no-desc"}),
			},
			// The default nameless group is skipped, so it produces no skill.
			want: map[string]skillContent{
				"my-skill-with-desc": {tools: map[string]tools.Tool{}, description: "group's own description"},
				"my-skill-no-desc":   {tools: map[string]tools.Tool{}, description: "flag fallback"},
			},
		},
		{
			name: "toolset mode: uses flag description, ignores group description",
			cmd:  &skillsCmd{name: "my-skill", description: "flag desc", toolset: "my-toolset"},
			groupsMap: map[string]group.Group{
				"": group.NewGroup(group.GroupConfig{Name: ""}),
				"my-toolset": group.NewGroup(
					group.GroupConfig{Name: "my-toolset", Description: "ignored in toolset mode"}),
			},
			want: map[string]skillContent{
				"my-skill": {tools: map[string]tools.Tool{}, description: "flag desc"},
			},
		},
		{
			name:     "all-tools mode: falls back to flag when default group has no description",
			cmd:      &skillsCmd{name: "my-skill", description: "flag desc"},
			toolsMap: map[string]tools.Tool{},
			groupsMap: map[string]group.Group{
				"": group.NewGroup(group.GroupConfig{Name: ""}),
			},
			want: map[string]skillContent{
				"my-skill": {tools: map[string]tools.Tool{}, description: "flag desc"},
			},
		},
		{
			name:     "all-tools mode: default group description takes precedence over flag",
			cmd:      &skillsCmd{name: "my-skill", description: "flag desc"},
			toolsMap: map[string]tools.Tool{},
			groupsMap: map[string]group.Group{
				"": group.NewGroup(group.GroupConfig{Name: "", Description: "default group description"}),
			},
			want: map[string]skillContent{
				"my-skill": {tools: map[string]tools.Tool{}, description: "default group description"},
			},
		},
		{
			name: "single group flag: uses selected group's description",
			cmd:  &skillsCmd{name: "my-skill", description: "flag fallback", group: "with-desc"},
			groupsMap: map[string]group.Group{
				"": group.NewGroup(group.GroupConfig{Name: ""}),
				"with-desc": group.NewGroup(
					group.GroupConfig{Name: "with-desc", Description: "group's own description"}),
				"no-desc": group.NewGroup(
					group.GroupConfig{Name: "no-desc"}),
			},
			want: map[string]skillContent{
				"my-skill": {tools: map[string]tools.Tool{}, description: "group's own description"},
			},
		},
		{
			name: "single group flag: falls back to flag when group has no description",
			cmd:  &skillsCmd{name: "my-skill", description: "flag fallback", group: "no-desc"},
			groupsMap: map[string]group.Group{
				"": group.NewGroup(group.GroupConfig{Name: ""}),
				"with-desc": group.NewGroup(
					group.GroupConfig{Name: "with-desc", Description: "group's own description"}),
				"no-desc": group.NewGroup(
					group.GroupConfig{Name: "no-desc"}),
			},
			want: map[string]skillContent{
				"my-skill": {tools: map[string]tools.Tool{}, description: "flag fallback"},
			},
		},
		{
			name: "single group flag: unknown group errors",
			cmd:  &skillsCmd{name: "my-skill", group: "nope"},
			groupsMap: map[string]group.Group{
				"": group.NewGroup(group.GroupConfig{Name: ""}),
				"with-desc": group.NewGroup(
					group.GroupConfig{Name: "with-desc", Description: "group's own description"}),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cmd.buildSkillContents(tt.toolsMap, tt.groupsMap)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildSkillContents failed: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildSkillContents() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveSkillName(t *testing.T) {
	tests := []struct {
		name            string
		flagName        string
		group           string
		toolset         string
		prebuiltConfigs []string
		want            string
		wantErr         bool
	}{
		{
			name:     "explicit name wins",
			flagName: "my-skill",
			want:     "my-skill",
		},
		{
			name:            "explicit name wins over prebuilt",
			flagName:        "my-skill",
			prebuiltConfigs: []string{"alloydb-postgres"},
			want:            "my-skill",
		},
		{
			name:     "explicit name wins over group",
			flagName: "my-skill",
			group:    "greeting",
			want:     "my-skill",
		},
		{
			name:  "defaults to group",
			group: "greeting",
			want:  "greeting",
		},
		{
			name:    "defaults to toolset",
			toolset: "greeting",
			want:    "greeting",
		},
		{
			name:            "group wins over prebuilt",
			group:           "greeting",
			prebuiltConfigs: []string{"alloydb-postgres"},
			want:            "greeting",
		},
		{
			name:            "defaults to single prebuilt",
			prebuiltConfigs: []string{"alloydb-postgres"},
			want:            "alloydb-postgres",
		},
		{
			name:            "sanitizes slashes in single prebuilt",
			prebuiltConfigs: []string{"alloydb-postgres/some-toolset"},
			want:            "alloydb-postgres-some-toolset",
		},
		{
			name:    "no name and no prebuilt errors",
			wantErr: true,
		},
		{
			name:            "no name and multiple prebuilts errors",
			prebuiltConfigs: []string{"alloydb-postgres", "cloud-sql-postgres"},
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSkillName(tt.flagName, tt.group, tt.toolset, tt.prebuiltConfigs)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (got %q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateSkill_FlagValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{
			name:    "unexpected positional arg",
			args:    []string{"skills-generate", "--name", "test", "--description", "test", "extra"},
			wantSub: "unknown command \"extra\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := invokeCommand(tt.args)
			if err == nil {
				t.Fatalf("expected error containing %q, but got nil\nOutput: %s", tt.wantSub, got)
			}
			if !strings.Contains(err.Error(), tt.wantSub) && !strings.Contains(got, tt.wantSub) {
				t.Errorf("expected error or output to contain %q\nError: %v\nOutput: %s", tt.wantSub, err, got)
			}
		})
	}
}
