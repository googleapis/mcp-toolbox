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
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/googleapis/mcp-toolbox/cmd/internal"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/tools"

	"github.com/spf13/cobra"
)

// skillContent holds the tools and description for a single generated skill.
type skillContent struct {
	tools       map[string]tools.Tool
	description string
}

// skillsCmd is the command for generating skills.
type skillsCmd struct {
	*cobra.Command
	name            string
	description     string
	toolset         string
	group           string
	outputDir       string
	licenseHeader   string
	additionalNotes string
	invocationMode  string
	toolboxVersion  string
}

// NewCommand creates a new Command.
func NewCommand(opts *internal.ToolboxOptions) *cobra.Command {
	cmd := &skillsCmd{}
	cmd.Command = &cobra.Command{
		Use:   "skills-generate",
		Short: "Generate skills from tool configurations",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return run(cmd, opts)
		},
	}

	flags := cmd.Flags()
	internal.ConfigFileFlags(cmd.Command, flags, opts)
	flags.StringVar(&cmd.name, "name", "", "Name of the generated skill.")
	flags.StringVar(&cmd.description, "description", "", "Description of the generated skill. Used as a fallback when a group does not define its own description.")
	flags.StringVar(&cmd.toolset, "toolset", "", "Name of the toolset to convert into a skill. If not provided, all tools will be included.")
	flags.StringVar(&cmd.group, "group", "", "Name of the group to convert into a single skill. Uses the group's description, falling back to --description.")
	flags.StringVar(&cmd.outputDir, "output-dir", "skills", "Directory to output generated skills")
	flags.StringVar(&cmd.licenseHeader, "license-header", "", "Optional license header to prepend to generated node scripts.")
	flags.StringVar(&cmd.additionalNotes, "additional-notes", "", "Additional notes to add under the Usage section of the generated SKILL.md")
	flags.StringVar(&cmd.invocationMode, "invocation-mode", "npx", "Invocation mode for the generated scripts: 'binary' or 'npx'")
	flags.StringVar(&cmd.toolboxVersion, "toolbox-version", opts.VersionNum, "Version of @toolbox-sdk/server to use for npx approach")
	cmd.MarkFlagsMutuallyExclusive("group", "toolset")
	return cmd.Command
}

func run(cmd *skillsCmd, opts *internal.ToolboxOptions) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	ctx, shutdown, err := opts.Setup(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	// skills-generate runs offline: source env vars are needed only to make the
	// config YAML parse, never to connect, so unset placeholders resolve to "".
	parser := internal.ConfigParser{AllowMissingEnvVars: true}
	_, err = opts.LoadConfig(ctx, &parser)
	if err != nil {
		return err
	}

	name, err := resolveSkillName(cmd.name, cmd.group, cmd.toolset, opts.PrebuiltConfigs)
	if err != nil {
		opts.Logger.ErrorContext(ctx, err.Error())
		return err
	}
	cmd.name = name

	if err := os.MkdirAll(cmd.outputDir, 0755); err != nil {
		errMsg := fmt.Errorf("error creating output directory: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	opts.Logger.InfoContext(ctx, "Generating skillagent skills...")

	// Collect the tools and description for each skill to generate.
	skillsToContents, err := cmd.collectContents(ctx, opts)
	if err != nil {
		errMsg := fmt.Errorf("error collecting skill contents: %w", err)
		opts.Logger.ErrorContext(ctx, errMsg.Error())
		return errMsg
	}

	if len(skillsToContents) == 0 {
		opts.Logger.InfoContext(ctx, "No tools found to generate.")
		return nil
	}

	// Iterate over keys to ensure deterministic order
	var skillNames []string
	for name := range skillsToContents {
		skillNames = append(skillNames, name)
	}
	sort.Strings(skillNames)

	for _, skillName := range skillNames {
		content := skillsToContents[skillName]
		allTools := content.tools
		if len(allTools) == 0 {
			opts.Logger.InfoContext(ctx, fmt.Sprintf("No tools found for skill '%s', skipping.", skillName))
			continue
		}

		// Generate the combined skill directory
		skillPath := filepath.Join(cmd.outputDir, skillName)
		if err := os.MkdirAll(skillPath, 0755); err != nil {
			errMsg := fmt.Errorf("error creating skill directory: %w", err)
			opts.Logger.ErrorContext(ctx, errMsg.Error())
			return errMsg
		}

		// Generate assets directory
		assetsPath := filepath.Join(skillPath, "assets")
		if err := os.MkdirAll(assetsPath, 0755); err != nil {
			errMsg := fmt.Errorf("error creating assets dir: %w", err)
			opts.Logger.ErrorContext(ctx, errMsg.Error())
			return errMsg
		}

		// Generate scripts directory
		scriptsPath := filepath.Join(skillPath, "scripts")
		if err := os.MkdirAll(scriptsPath, 0755); err != nil {
			errMsg := fmt.Errorf("error creating scripts dir: %w", err)
			opts.Logger.ErrorContext(ctx, errMsg.Error())
			return errMsg
		}

		var jsConfigArgs []string
		if len(opts.PrebuiltConfigs) > 0 {
			for _, pc := range opts.PrebuiltConfigs {
				jsConfigArgs = append(jsConfigArgs, `"--prebuilt"`, fmt.Sprintf(`"%s"`, pc))
			}
		}

		if opts.ConfigFolder != "" {
			folderName := filepath.Base(opts.ConfigFolder)
			destFolder := filepath.Join(assetsPath, folderName)
			if err := copyDir(opts.ConfigFolder, destFolder); err != nil {
				return err
			}
			jsConfigArgs = append(jsConfigArgs, `"--config-folder"`, fmt.Sprintf(`path.join(__dirname, "..", "assets", %q)`, folderName))
		} else if len(opts.Configs) > 0 {
			for _, f := range opts.Configs {
				baseName := filepath.Base(f)
				destPath := filepath.Join(assetsPath, baseName)
				if err := copyFile(f, destPath); err != nil {
					return err
				}
				jsConfigArgs = append(jsConfigArgs, `"--configs"`, fmt.Sprintf(`path.join(__dirname, "..", "assets", %q)`, baseName))
			}
		} else if opts.Config != "" {
			baseName := filepath.Base(opts.Config)
			destPath := filepath.Join(assetsPath, baseName)
			if err := copyFile(opts.Config, destPath); err != nil {
				return err
			}
			jsConfigArgs = append(jsConfigArgs, `"--config"`, fmt.Sprintf(`path.join(__dirname, "..", "assets", %q)`, baseName))
		}

		configArgsStr := strings.Join(jsConfigArgs, ", ")

		// Iterate over keys to ensure deterministic order
		var toolNames []string
		for name := range allTools {
			toolNames = append(toolNames, name)
		}
		sort.Strings(toolNames)

		for _, toolName := range toolNames {
			// Generate wrapper script in scripts directory
			scriptContent, err := generateScriptContent(toolName, configArgsStr, cmd.licenseHeader, cmd.invocationMode, cmd.toolboxVersion, parser.OptionalEnvVars)
			if err != nil {
				errMsg := fmt.Errorf("error generating script content for %s: %w", toolName, err)
				opts.Logger.ErrorContext(ctx, errMsg.Error())
				return errMsg
			}

			scriptFilename := filepath.Join(scriptsPath, fmt.Sprintf("%s.js", toolName))
			if err := os.WriteFile(scriptFilename, []byte(scriptContent), 0755); err != nil {
				errMsg := fmt.Errorf("error writing script %s: %w", scriptFilename, err)
				opts.Logger.ErrorContext(ctx, errMsg.Error())
				return errMsg
			}
		}

		// Generate SKILL.md
		skillContent, err := generateSkillMarkdown(skillName, content.description, cmd.additionalNotes, allTools, parser.EnvVars)
		if err != nil {
			errMsg := fmt.Errorf("error generating SKILL.md content: %w", err)
			opts.Logger.ErrorContext(ctx, errMsg.Error())
			return errMsg
		}
		skillMdPath := filepath.Join(skillPath, "SKILL.md")
		if err := os.WriteFile(skillMdPath, []byte(skillContent), 0644); err != nil {
			errMsg := fmt.Errorf("error writing SKILL.md: %w", err)
			opts.Logger.ErrorContext(ctx, errMsg.Error())
			return errMsg
		}

		opts.Logger.InfoContext(ctx, fmt.Sprintf("Successfully generated skill '%s' with %d tools.", skillName, len(allTools)))
	}

	return nil
}

// resolveSkillName returns the explicit --name when set. Otherwise, in the
// single-skill modes it defaults to the --group or --toolset name, and for
// prebuilt generation it defaults to the config name when exactly one
// --prebuilt config is given. Any other case requires --name.
func resolveSkillName(name, group, toolset string, prebuiltConfigs []string) (string, error) {
	if name != "" {
		return name, nil
	}
	if group != "" {
		return group, nil
	}
	if toolset != "" {
		return toolset, nil
	}
	if len(prebuiltConfigs) == 1 {
		return strings.ReplaceAll(prebuiltConfigs[0], "/", "-"), nil
	}
	return "", fmt.Errorf("--name is required unless --group or --toolset is set, or exactly one --prebuilt config is provided")
}

func (c *skillsCmd) collectContents(ctx context.Context, opts *internal.ToolboxOptions) (map[string]skillContent, error) {
	// Initialize tools and groups only; skills generation does not need live
	// sources, auth services, or embedding models.
	toolsMap, groupsMap, err := server.InitializeOfflineConfigs(ctx, opts.Cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize resources: %w", err)
	}

	return c.buildSkillContents(toolsMap, groupsMap)
}

// buildSkillContents maps each skill name to the tools and description it should
// be generated with. In group mode, a group's own description takes precedence
// over the --description flag, which acts as a fallback.
func (c *skillsCmd) buildSkillContents(toolsMap map[string]tools.Tool, groupsMap map[string]group.Group) (map[string]skillContent, error) {
	primitiveMgr := primitives.NewPrimitiveManager(nil, nil, nil, toolsMap, nil, nil, nil, groupsMap)

	skillsToContents := make(map[string]skillContent)

	getToolsFromGroup := func(g group.Group) map[string]tools.Tool {
		groupTools := make(map[string]tools.Tool)
		for _, name := range g.ToolNames {
			if tool, ok := toolsMap[name]; ok {
				groupTools[name] = tool
			}
		}
		return groupTools
	}

	if c.group != "" {
		g, ok := primitiveMgr.GetGroup(c.group)
		if !ok {
			return nil, fmt.Errorf("group %q not found", c.group)
		}

		skillsToContents[c.name] = skillContent{tools: getToolsFromGroup(g), description: c.descriptionFor(g)}
		return skillsToContents, nil
	}

	if c.toolset != "" {
		g, ok := primitiveMgr.GetGroup(c.toolset)
		if !ok {
			return nil, fmt.Errorf("toolset %q not found", c.toolset)
		}

		skillsToContents[c.name] = skillContent{tools: getToolsFromGroup(g), description: c.description}
		return skillsToContents, nil
	}

	if len(groupsMap) <= 1 {
		// Default to all tools if no named group found. The default nameless
		// group's description (if any) takes precedence over the flag.
		skillsToContents[c.name] = skillContent{tools: toolsMap, description: c.descriptionFor(groupsMap[""])}
		return skillsToContents, nil
	}

	// One skill per group
	for gName, g := range groupsMap {
		if gName == "" {
			continue
		}
		skillName := fmt.Sprintf("%s-%s", c.name, gName)
		skillsToContents[skillName] = skillContent{tools: getToolsFromGroup(g), description: c.descriptionFor(g)}
	}

	return skillsToContents, nil
}

// descriptionFor returns the group's own description when set, falling back to
// the --description flag otherwise.
func (c *skillsCmd) descriptionFor(g group.Group) string {
	if g.Description != "" {
		return g.Description
	}
	return c.description
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}
		return copyFile(path, destPath)
	})
}
