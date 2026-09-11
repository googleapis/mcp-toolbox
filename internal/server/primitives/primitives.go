// Copyright 2025 Google LLC
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

package primitives

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/googleapis/mcp-toolbox/internal/auth"
	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels"
	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

// PrimitiveManager contains available resources for the server. Should be initialized with NewPrimitiveManager().
// groups is the source of truth for named collections; toolset views (manifests)
// are derived from the group on demand by the callers that render them.
type PrimitiveManager struct {
	mu                sync.RWMutex
	sources           map[string]sources.Source
	authServices      map[string]auth.AuthService
	embeddingModels   map[string]embeddingmodels.EmbeddingModel
	tools             map[string]tools.Tool
	prompts           map[string]prompts.Prompt
	resources         map[string]resources.Resource
	resourceTemplates map[string]resources.ResourceTemplate
	groups            map[string]group.Group
	// resourceNameIndex maps a resource's name to its URI, the key of resources.
	resourceNameIndex map[string]string
}

// indexResourceNames builds the name to URI index for the resource map.
func indexResourceNames(resourcesMap map[string]resources.Resource) map[string]string {
	index := make(map[string]string, len(resourcesMap))
	for uri, res := range resourcesMap {
		if res == nil {
			continue
		}
		index[res.GetName()] = uri
	}
	return index
}

func NewPrimitiveManager(
	sourcesMap map[string]sources.Source,
	authServicesMap map[string]auth.AuthService,
	embeddingModelsMap map[string]embeddingmodels.EmbeddingModel,
	toolsMap map[string]tools.Tool,
	promptsMap map[string]prompts.Prompt,
	resourcesMap map[string]resources.Resource,
	resourceTemplatesMap map[string]resources.ResourceTemplate,
	groupsMap map[string]group.Group,
) *PrimitiveManager {
	primitiveMgr := &PrimitiveManager{
		mu:                sync.RWMutex{},
		sources:           sourcesMap,
		authServices:      authServicesMap,
		embeddingModels:   embeddingModelsMap,
		tools:             toolsMap,
		prompts:           promptsMap,
		resources:         resourcesMap,
		resourceTemplates: resourceTemplatesMap,
		groups:            groupsMap,
		resourceNameIndex: indexResourceNames(resourcesMap),
	}

	return primitiveMgr
}

func (r *PrimitiveManager) GetSource(sourceName string) (sources.Source, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	source, ok := r.sources[sourceName]
	return source, ok
}

func (r *PrimitiveManager) GetAuthService(authServiceName string) (auth.AuthService, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	authService, ok := r.authServices[authServiceName]
	return authService, ok
}

func (r *PrimitiveManager) GetEmbeddingModel(embeddingModelName string) (embeddingmodels.EmbeddingModel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	model, ok := r.embeddingModels[embeddingModelName]
	return model, ok
}

func (r *PrimitiveManager) GetTool(toolName string) (tools.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[toolName]
	return tool, ok
}

func (r *PrimitiveManager) GetPrompt(promptName string) (prompts.Prompt, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	prompt, ok := r.prompts[promptName]
	return prompt, ok
}

// GetResource returns a specific resource by name or by URI. Names are resolved
// first.
func (r *PrimitiveManager) GetResource(nameOrURI string) (resources.Resource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if uri, ok := r.resourceNameIndex[nameOrURI]; ok {
		resource, ok := r.resources[uri]
		return resource, ok
	}
	resource, ok := r.resources[nameOrURI]
	return resource, ok
}

// GetResourceTemplate returns a specific resource template by name.
func (r *PrimitiveManager) GetResourceTemplate(name string) (resources.ResourceTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, exists := r.resourceTemplates[name]
	return rt, exists
}

// GetGroup returns the group of the given name.
func (r *PrimitiveManager) GetGroup(groupName string) (group.Group, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, ok := r.groups[groupName]
	return g, ok
}

func (r *PrimitiveManager) SetPrimitives(sourcesMap map[string]sources.Source, authServicesMap map[string]auth.AuthService, embeddingModelsMap map[string]embeddingmodels.EmbeddingModel, toolsMap map[string]tools.Tool, promptsMap map[string]prompts.Prompt, resourcesMap map[string]resources.Resource, resourceTemplatesMap map[string]resources.ResourceTemplate, groupsMap map[string]group.Group) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sources = sourcesMap
	r.authServices = authServicesMap
	r.embeddingModels = embeddingModelsMap
	r.tools = toolsMap
	r.prompts = promptsMap
	r.resources = resourcesMap
	r.resourceTemplates = resourceTemplatesMap
	r.groups = groupsMap
	r.resourceNameIndex = indexResourceNames(resourcesMap)
}

// AuthServices returns a copy of the auth services map
func (r *PrimitiveManager) AuthServices() map[string]auth.AuthService {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copiedMap := make(map[string]auth.AuthService, len(r.authServices))
	for k, v := range r.authServices {
		copiedMap[k] = v
	}
	return copiedMap
}

// GetUIResourceFromURI returns a UI resource by matching its URI.
func (r *PrimitiveManager) GetUIResourceFromURI(uri string) (resources.Resource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if res, ok := r.resources[uri]; ok && res.IsUI() {
		return res, true
	}
	return nil, false
}

// MatchResourceTemplateURI matches a URI against a URI template containing {path}.
func MatchResourceTemplateURI(tmpl, uri string) (map[string]any, bool) {
	if strings.Contains(tmpl, "{path}") {
		regexPattern := regexp.QuoteMeta(tmpl)
		regexPattern = strings.ReplaceAll(regexPattern, "\\{path\\}", "(.*)")
		re, err := regexp.Compile("^" + regexPattern + "$")
		if err != nil {
			return nil, false
		}
		matches := re.FindStringSubmatch(uri)
		if len(matches) == 2 {
			return map[string]any{"path": matches[1]}, true
		}
	}
	return nil, false
}

// GetUIResourceTemplateByURI matches a URI against registered UI resource templates
// and returns the matching template along with extracted template parameters.
func (r *PrimitiveManager) GetUIResourceTemplateByURI(uri string) (resources.ResourceTemplate, map[string]any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, rt := range r.resourceTemplates {
		if rt.IsUI() {
			if params, ok := MatchResourceTemplateURI(rt.GetURITemplate(), uri); ok {
				return rt, params, true
			}
		}
	}
	return nil, nil, false
}

// GroupsList returns a copy of the groups list sorted alphabetically by name
func (r *PrimitiveManager) GroupsList() []group.Group {
	r.mu.RLock()
	defer r.mu.RUnlock()
	groupsList := make([]group.Group, 0, len(r.groups))
	for k, g := range r.groups {
		if k == "" {
			continue
		}
		groupsList = append(groupsList, g)
	}

	slices.SortFunc(groupsList, func(a, b group.Group) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return groupsList
}
