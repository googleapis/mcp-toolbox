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
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.opentelemetry.io/otel/trace"
)

type MockToolConfig struct {
	Name       string                `yaml:"name"`
	Type       string                `yaml:"type"`
	Source     string                `yaml:"source"`
	Other      string                `yaml:"other"`
	Parameters parameters.Parameters `yaml:"parameters"`
}

func (m MockToolConfig) ToolConfigType() string {
	return m.Type
}

func (m MockToolConfig) Initialize() (tools.Tool, error) {
	return nil, nil
}

type MockSourceConfig struct {
	Name             string `yaml:"name"`
	Type             string `yaml:"type"`
	ConnectionString string `yaml:"connection_string"`
}

func (m MockSourceConfig) SourceConfigType() string {
	return m.Type
}

func (m MockSourceConfig) Initialize(context.Context, trace.Tracer) (sources.Source, error) {
	return nil, nil
}

func TestFormatParameters(t *testing.T) {
	tests := []struct {
		name         string
		params       []parameters.ParameterManifest
		envVars      map[string]string
		wantContains []string
		wantErr      bool
	}{
		{
			name:         "empty parameters",
			params:       []parameters.ParameterManifest{},
			wantContains: []string{""},
		},
		{
			name: "single required string parameter",
			params: []parameters.ParameterManifest{
				{
					Name:        "param1",
					Description: "A test parameter",
					Type:        "string",
					Required:    true,
				},
			},
			wantContains: []string{
				"#### Parameters",
				"| Name | Type | Description | Required | Default |",
				"| :--- | :--- | :--- | :--- | :--- |",
				"| param1 | string | A test parameter | Yes |  |",
			},
		},
		{
			name: "mixed parameters with defaults",
			params: []parameters.ParameterManifest{
				{
					Name:        "param1",
					Description: "Param 1",
					Type:        "string",
					Required:    true,
				},
				{
					Name:        "param2",
					Description: "Param 2",
					Type:        "integer",
					Default:     42,
					Required:    false,
				},
			},
			wantContains: []string{
				"| param1 | string | Param 1 | Yes |  |",
				"| param2 | integer | Param 2 | No | `42` |",
			},
		},
		{
			name: "parameter with env var default",
			params: []parameters.ParameterManifest{
				{
					Name:        "param1",
					Description: "Param 1",
					Type:        "string",
					Default:     "default-value",
					Required:    false,
				},
			},
			envVars: map[string]string{
				"MY_ENV_VAR": "default-value",
			},
			wantContains: []string{
				`param1 | string | Param 1 | No |  |`,
			},
		},
		{
			name: "parameter with env var default",
			params: []parameters.ParameterManifest{
				{
					Name:        "param1",
					Description: "Param 1",
					Type:        "string",
					Default:     "default-value",
					Required:    false,
				},
			},
			envVars: map[string]string{
				"MY_ENV_VAR": "default-value",
			},
			wantContains: []string{
				`param1 | string | Param 1 | No |  |`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatParameters(tt.params, tt.envVars)
			if (err != nil) != tt.wantErr {
				t.Errorf("formatParameters() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if len(tt.params) == 0 {
				if got != "" {
					t.Errorf("formatParameters() = %v, want empty string", got)
				}
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("formatParameters() result missing expected string: %s\nGot:\n%s", want, got)
				}
			}
		})
	}
}

func TestGenerateSkillMarkdown(t *testing.T) {
	toolsMap := map[string]tools.Tool{
		"tool1": testutils.NewMockTool("tool1", "First tool", "",
			[]parameters.Parameter{
				parameters.NewStringParameter("p1", "d1"),
			}, false, false),
	}

	got, err := generateSkillMarkdown("MySkill", "My Description", "Some extra notes", toolsMap, nil)
	if err != nil {
		t.Fatalf("generateSkillMarkdown() error = %v", err)
	}

	expectedSubstrings := []string{
		"name: MySkill",
		"description: My Description",
		"## Usage",
		"All scripts can be executed using Node.js",
		"**Bash:**",
		"`node <skill_dir>/scripts/<script_name>.js '{\"<param_name>\": \"<param_value>\"}'`",
		"**PowerShell:**",
		"`node <skill_dir>/scripts/<script_name>.js '{\"<param_name>\": \"<param_value>\"}'`",
		"Some extra notes",
		"## Scripts",
		"### tool1",
		"First tool",
		"#### Parameters",
		"| Name | Type | Description | Required | Default |",
	}

	for _, s := range expectedSubstrings {
		if !strings.Contains(got, s) {
			t.Errorf("generateSkillMarkdown() missing substring %q", s)
		}
	}
}

func TestGenerateScriptContent(t *testing.T) {
	tests := []struct {
		name          string
		toolName      string
		configArgs    string
		wantContains  []string
		licenseHeader string
		mode          string
		version       string
		optionalVars  []string
	}{
		{
			name:       "basic script (binary default)",
			toolName:   "test-tool",
			configArgs: `"--prebuilt", "test"`,
			mode:       "bin",
			wantContains: []string{
				`const toolName = "test-tool";`,
				`const configArgs = ["--prebuilt", "test"];`,
				`const toolboxArgs = ["--log-level", "error", ...configArgs, "invoke", toolName, "--user-agent-metadata", userAgent, ...args];`,
				`function mergeEnvVars(env) {`,
				`function prepareEnvironment() {`,
				`function main() {`,
				`main();`,
			},
		},
		{
			name:       "script with config",
			toolName:   "complex-tool",
			configArgs: `"--config", path.join(__dirname, "..", "assets", "test")`,
			mode:       "bin",
			wantContains: []string{
				`const toolName = "complex-tool";`,
				`const configArgs = ["--config", path.join(__dirname, "..", "assets", "test")];`,
			},
		},
		{
			name:          "script with license header",
			toolName:      "test-tool",
			configArgs:    `"--prebuilt", "test"`,
			licenseHeader: "// My License",
			mode:          "bin",
			wantContains: []string{
				"// My License",
			},
		},
		{
			name:         "script with optional vars",
			toolName:     "test-tool",
			configArgs:   `"--prebuilt", "test"`,
			optionalVars: []string{"OPTIONAL_A", "OPTIONAL_B"},
			mode:         "bin",
			wantContains: []string{
				"const OPTIONAL_VARS_TO_OMIT_IF_EMPTY = [",
				"    'OPTIONAL_A',",
				"    'OPTIONAL_B',",
				"];",
				"OPTIONAL_VARS_TO_OMIT_IF_EMPTY.forEach(varName => {",
				"if (env[varName] === '') {",
				"delete env[varName];",
			},
		},
		{
			name:       "npx mode script",
			toolName:   "npx-tool",
			configArgs: `"--prebuilt", "test"`,
			mode:       "npx",
			version:    "0.31.0",
			wantContains: []string{
				`const toolName = "npx-tool";`,
				`const npxArgs = ["--yes", "@toolbox-sdk/server@0.31.0"`,
			},
		},
		{
			name:       "claude code script",
			toolName:   "claude-tool",
			configArgs: `"--prebuilt", "test"`,
			mode:       "bin",
			wantContains: []string{
				`userAgent = "skills-claudecode";`,
				`const prefix = 'CLAUDE_PLUGIN_OPTION_';`,
				`if (key.startsWith(prefix)) {`,
				`env[key.substring(prefix.length)] = process.env[key];`,
			},
		},
		{
			name:       "codex ci script",
			toolName:   "codex-tool",
			configArgs: `"--prebuilt", "test"`,
			mode:       "bin",
			wantContains: []string{
				`userAgent = "skills-codex";`,
				`} else if (process.env.CODEX_CI === '1') {`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateScriptContent(tt.toolName, tt.configArgs, tt.licenseHeader, tt.mode, tt.version, tt.optionalVars)
			if err != nil {
				t.Fatalf("generateScriptContent() error = %v", err)
			}

			for _, s := range tt.wantContains {
				if !strings.Contains(got, s) {
					t.Errorf("generateScriptContent() missing substring %q\nGot:\n%s", s, got)
				}
			}
		})
	}
}
