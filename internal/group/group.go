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

package group

import (
	"fmt"

	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

const (
	DefaultTTLMs      = 300000   // default TTL for groups in milliseconds (5 minutes)
	DefaultCacheScope = "public" // default cache scope for groups
)

// GroupConfig is the parsed configuration for a group: a single named collection
// that holds tools, prompts, resources, and resource templates. Its description
// doubles as the MCP server instructions for clients connected to the group.
type GroupConfig struct {
	Name                  string   `yaml:"name"`
	Description           string   `yaml:"description"`
	ToolNames             []string `yaml:"tools"`
	PromptNames           []string `yaml:"prompts"`
	ResourceNames         []string `yaml:"resources"`
	ResourceTemplateNames []string `yaml:"resourceTemplates"`
	CacheScope            string   `yaml:"cacheScope" validate:"omitempty,oneof=public private"`
	TTLMs                 *int     `yaml:"ttlMs" validate:"omitempty,gte=0"`
}

// Group is an initialized group: the source of truth for a named collection of
// tools, prompts, resources, and resource templates. It is a self-contained MCP primitive and does not depend on
// the legacy toolset/promptset types. It keeps O(1) membership sets for its tools,
// prompts, resources, and templates; manifests are generated on demand by
// callers from the resolved primitive maps.
type Group struct {
	GroupConfig
	toolNameSet             map[string]struct{}
	promptNameSet           map[string]struct{}
	resourceNameSet         map[string]struct{}
	resourceTemplateNameSet map[string]struct{}
}

// GroupManager defines the minimal view of the primitives.PrimitiveManager
// that the Group package needs.
// This is implemented to prevent import cycles.
type GroupManager interface {
	GetSource(string) (sources.Source, bool)
	GetTool(string) (tools.Tool, bool)
}

// Initialize validates the group name and checks that every declared tool,
// prompt, resource, and template exists in the provided maps, building the membership sets used by
// ContainsTool, ContainsPrompt, ContainsResource, and ContainsResourceTemplate.
func (gc GroupConfig) Initialize(toolsMap map[string]tools.Tool, promptsMap map[string]prompts.Prompt, resourcesMap map[string]resources.Resource, templatesMap map[string]resources.ResourceTemplate) (Group, error) {
	if !tools.IsValidName(gc.Name) {
		return Group{}, fmt.Errorf("invalid group name: %q", gc.Name)
	}

	toolNameSet := make(map[string]struct{}, len(gc.ToolNames))
	for _, name := range gc.ToolNames {
		if _, ok := toolsMap[name]; !ok {
			return Group{}, fmt.Errorf("tool does not exist: %q", name)
		}
		toolNameSet[name] = struct{}{}
	}

	promptNameSet := make(map[string]struct{}, len(gc.PromptNames))
	for _, name := range gc.PromptNames {
		if _, ok := promptsMap[name]; !ok {
			return Group{}, fmt.Errorf("prompt does not exist: %q", name)
		}
		promptNameSet[name] = struct{}{}
	}

	resourceNameSet := make(map[string]struct{}, len(gc.ResourceNames))
	for _, name := range gc.ResourceNames {
		if _, ok := resourcesMap[name]; !ok {
			return Group{}, fmt.Errorf("resource does not exist: %q", name)
		}
		resourceNameSet[name] = struct{}{}
	}

	templateNameSet := make(map[string]struct{}, len(gc.ResourceTemplateNames))
	for _, name := range gc.ResourceTemplateNames {
		if _, ok := templatesMap[name]; !ok {
			return Group{}, fmt.Errorf("resource template does not exist: %q", name)
		}
		templateNameSet[name] = struct{}{}
	}

	return Group{
		GroupConfig:             gc,
		toolNameSet:             toolNameSet,
		promptNameSet:           promptNameSet,
		resourceNameSet:         resourceNameSet,
		resourceTemplateNameSet: templateNameSet,
	}, nil
}

// NewGroup builds a Group directly from its config, deriving the membership sets
// from the declared tool, prompt, resource, and template names. It skips the existence checks
// performed by GroupConfig.Initialize and is intended for tests and callers that
// have already validated the names.
func NewGroup(config GroupConfig) Group {
	toolNameSet := make(map[string]struct{}, len(config.ToolNames))
	for _, name := range config.ToolNames {
		toolNameSet[name] = struct{}{}
	}
	promptNameSet := make(map[string]struct{}, len(config.PromptNames))
	for _, name := range config.PromptNames {
		promptNameSet[name] = struct{}{}
	}
	resourceNameSet := make(map[string]struct{}, len(config.ResourceNames))
	for _, name := range config.ResourceNames {
		resourceNameSet[name] = struct{}{}
	}
	templateNameSet := make(map[string]struct{}, len(config.ResourceTemplateNames))
	for _, name := range config.ResourceTemplateNames {
		templateNameSet[name] = struct{}{}
	}
	return Group{
		GroupConfig:             config,
		toolNameSet:             toolNameSet,
		promptNameSet:           promptNameSet,
		resourceNameSet:         resourceNameSet,
		resourceTemplateNameSet: templateNameSet,
	}
}

// GetTTLMs returns the time to live for the group
func (g Group) GetTTLMs() int {
	if g.TTLMs == nil {
		return DefaultTTLMs
	}
	return *g.TTLMs
}

// GetCacheScope returns the cache scope for the group
func (g Group) GetCacheScope() string {
	if g.CacheScope == "" {
		return DefaultCacheScope
	}
	return g.CacheScope
}

// ContainsTool reports whether the group includes a tool with the given name.
func (g Group) ContainsTool(name string) bool {
	_, ok := g.toolNameSet[name]
	return ok
}

// ContainsPrompt reports whether the group includes a prompt with the given name.
func (g Group) ContainsPrompt(name string) bool {
	_, ok := g.promptNameSet[name]
	return ok
}

// ToolsetManifest builds a tools.ToolsetManifest for the group's tools, resolving
// each declared tool name against toolsMap and generating its manifest from srcs.
// The group holds tool names rather than tool pointers, so callers pass the
// resolved tools and sources maps.
func (g Group) ToolsetManifest(serverVersion string, mgr GroupManager) (tools.ToolsetManifest, error) {
	toolsManifest := make(map[string]tools.Manifest, len(g.ToolNames))
	for _, name := range g.ToolNames {
		tool, ok := mgr.GetTool(name)
		if !ok {
			return tools.ToolsetManifest{}, fmt.Errorf("tool does not exist: %s", name)
		}
		srcName := tool.GetSourceName()
		var src sources.Source
		if srcName != "" {
			src, ok = mgr.GetSource(srcName)
			if !ok {
				return tools.ToolsetManifest{}, fmt.Errorf("unable to retrieve %s source for tool %q", srcName, name)
			}
		}
		m, err := tool.Manifest(src)
		if err != nil {
			return tools.ToolsetManifest{}, fmt.Errorf("error generating manifest for tool %q: %w", name, err)
		}
		toolsManifest[name] = m
	}
	return tools.ToolsetManifest{ServerVersion: serverVersion, ToolsManifest: toolsManifest}, nil
}

// ContainsResource reports whether the group includes a resource with the given name.
func (g Group) ContainsResource(name string) bool {
	_, ok := g.resourceNameSet[name]
	return ok
}

// ContainsResourceTemplate reports whether the group includes a resource template with the given name.
func (g Group) ContainsResourceTemplate(name string) bool {
	_, ok := g.resourceTemplateNameSet[name]
	return ok
}
