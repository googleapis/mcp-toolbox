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

package v20250326

import (
	"fmt"

	"github.com/googleapis/mcp-toolbox/internal/resources"

	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// generateToolManifest generates Tool for list tools result
func generateToolManifest(name, desc string, authInvoke []string, params parameters.Parameters, annotations *tools.ToolAnnotations, urlParams map[string]string) Tool {
	inputSchema, authParams := generateParamManifest(params, urlParams)
	var toolAnnotations *ToolAnnotations
	if annotations != nil {
		toolAnnotations = &ToolAnnotations{
			DestructiveHint: annotations.DestructiveHint,
			IdempotentHint:  annotations.IdempotentHint,
			OpenWorldHint:   annotations.OpenWorldHint,
			ReadOnlyHint:    annotations.ReadOnlyHint,
		}
	}
	mcpManifest := Tool{
		Name:            name,
		Description:     desc,
		ToolInputSchema: inputSchema,
		Annotations:     toolAnnotations,
	}
	metadata := make(map[string]any)
	if len(authInvoke) > 0 {
		metadata["toolbox/authInvoke"] = authInvoke
	}
	if len(authParams) > 0 {
		metadata["toolbox/authParam"] = authParams
	}
	if len(metadata) > 0 {
		mcpManifest.Metadata = metadata
	}
	return mcpManifest
}

// generateParamManifest generates the input schema and get authParam
func generateParamManifest(ps parameters.Parameters, urlParams map[string]string) (InputSchema, map[string][]string) {
	properties := make(map[string]parameters.ParameterMcpManifest)
	required := make([]string, 0)
	authParam := make(map[string][]string)

	for _, p := range ps {
		// If the parameter is sourced from another param, skip it in the MCP manifest
		if p.GetValueFromParam() != "" {
			continue
		}

		name := p.GetName()
		if urlParams != nil {
			// If the parameter is sourced from URL params, skip it in the MCP manifest
			if _, exists := urlParams[name]; exists {
				continue
			}
		}

		paramManifest, authParamList := p.McpManifest()
		defaultV := p.GetDefault()
		if defaultV != nil {
			paramManifest.Default = defaultV
		}
		properties[name] = paramManifest
		// parameters that doesn't have a default value are added to the required field
		if parameters.CheckParamRequired(p.GetRequired(), defaultV) {
			required = append(required, name)
		}
		if len(authParamList) > 0 {
			authParam[name] = authParamList
		}
	}
	return InputSchema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}, authParam
}

// GenerateListToolsResult generates tools/list method result according to mcp schema
func GenerateListToolsResult(pMgr *primitives.PrimitiveManager, g group.Group, urlParams map[string]string) (ListToolsResult, error) {
	mcpManifest := make([]Tool, 0, len(g.ToolNames))
	for _, toolName := range g.ToolNames {
		tool, ok := pMgr.GetTool(toolName)
		if !ok {
			return ListToolsResult{}, fmt.Errorf("tool does not exist: %s", toolName)
		}
		// Skip a Tool that requires secure params as they are not supported in this protocol version.
		if tool.HasSecureParams() {
			continue
		}
		srcName := tool.GetSourceName()
		var src sources.Source
		if srcName != "" {
			src, ok = pMgr.GetSource(srcName)
			if !ok {
				return ListToolsResult{}, fmt.Errorf("unable to retrieve %s source for tool %q", srcName, tool.GetName())
			}
		}
		params, err := tool.GetParameters(src)
		if err != nil {
			return ListToolsResult{}, fmt.Errorf("error getting parameters for tool %q: %w", toolName, err)
		}
		toolManifest := generateToolManifest(toolName, tool.GetDescription(), tool.GetAuthRequired(), params, tool.GetAnnotations(src), urlParams)
		mcpManifest = append(mcpManifest, toolManifest)
	}
	return ListToolsResult{Tools: mcpManifest}, nil
}

// generatePromptManifest generates a version-specific Prompt manifest for list/prompts
func generatePromptManifest(name, desc string, args prompts.Arguments) Prompt {
	mcpArgs := make([]PromptArgument, 0, len(args))
	for _, arg := range args {
		promptArg := PromptArgument{
			Name:        arg.GetName(),
			Description: arg.GetDesc(),
			Required:    parameters.CheckParamRequired(arg.GetRequired(), arg.GetDefault()),
		}
		mcpArgs = append(mcpArgs, promptArg)
	}
	return Prompt{
		Name:        name,
		Description: desc,
		Arguments:   mcpArgs,
	}
}

// GenerateListPromptsResult generates the list/prompts result
func GenerateListPromptsResult(pMgr *primitives.PrimitiveManager, g group.Group) (ListPromptsResult, error) {
	mcpManifest := make([]Prompt, 0, len(g.PromptNames))
	for _, promptName := range g.PromptNames {
		prompt, ok := pMgr.GetPrompt(promptName)
		if !ok {
			return ListPromptsResult{}, fmt.Errorf("prompt does not exist: %s", promptName)
		}
		promptManifest := generatePromptManifest(promptName, prompt.GetDesc(), prompt.GetArguments())
		mcpManifest = append(mcpManifest, promptManifest)
	}
	return ListPromptsResult{Prompts: mcpManifest}, nil
}

// generateResourceManifest generates a version-specific Resource manifest for list/resources

func generateAnnotations(internalAnns *resources.ResourceAnnotations) *Annotations {
	if internalAnns == nil || (len(internalAnns.Audience) == 0 && internalAnns.Priority == nil) {
		return nil
	}
	annotations := &Annotations{}
	if internalAnns.Priority != nil {
		annotations.Priority = internalAnns.Priority
	}
	for _, aud := range internalAnns.Audience {
		annotations.Audience = append(annotations.Audience, Role(aud))
	}
	return annotations
}

func generateResourceManifest(name, desc, uri, mimeType string, size *int64, internalAnns *resources.ResourceAnnotations) Resource {
	return Resource{
		Name:        name,
		Uri:         uri,
		Description: desc,
		MimeType:    mimeType,
		Size:        size,
		Annotations: generateAnnotations(internalAnns),
	}
}

// GenerateListResourcesResult generates the list/resources result
func GenerateListResourcesResult(pMgr *primitives.PrimitiveManager, g group.Group) (ListResourcesResult, error) {
	mcpManifest := make([]Resource, 0, len(g.ResourceNames))
	for _, name := range g.ResourceNames {
		res, ok := pMgr.GetResource(name)
		if !ok {
			return ListResourcesResult{}, fmt.Errorf("resource does not exist: %s", name)
		}
		mcpManifest = append(mcpManifest, generateResourceManifest(name, res.GetDescription(), res.GetURI(), res.GetMimeType(), res.GetSize(), res.GetAnnotations()))
	}
	return ListResourcesResult{Resources: mcpManifest}, nil
}

// generateResourceTemplateManifest generates a version-specific ResourceTemplate manifest
func generateResourceTemplateManifest(name, desc, uriTemplate, mimeType string, internalAnns *resources.ResourceAnnotations) ResourceTemplate {
	return ResourceTemplate{
		Name:        name,
		UriTemplate: uriTemplate,
		Description: desc,
		MimeType:    mimeType,
		Annotations: generateAnnotations(internalAnns),
	}
}

// GenerateListResourceTemplatesResult generates the list/resource templates result
func GenerateListResourceTemplatesResult(pMgr *primitives.PrimitiveManager, g group.Group) (ListResourceTemplatesResult, error) {
	mcpManifest := make([]ResourceTemplate, 0, len(g.ResourceTemplateNames))
	for _, name := range g.ResourceTemplateNames {
		tmpl, ok := pMgr.GetResourceTemplate(name)
		if !ok {
			return ListResourceTemplatesResult{}, fmt.Errorf("resource template does not exist: %s", name)
		}
		mcpManifest = append(mcpManifest, generateResourceTemplateManifest(name, tmpl.GetDescription(), tmpl.GetURITemplate(), tmpl.GetMimeType(), tmpl.GetAnnotations()))
	}
	return ListResourceTemplatesResult{ResourceTemplates: mcpManifest}, nil
}
