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

package v20260728

import (
	"fmt"

	"github.com/googleapis/mcp-toolbox/internal/group"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/server/primitives"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// generateToolManifest generates Tool for list tools result
func generateToolManifest(name, desc string, authInvoke []string, params parameters.Parameters, annotations *tools.ToolAnnotations, urlParams map[string]string, uiMetadata map[string]any) Tool {
	var standardParams parameters.Parameters
	var secureParams parameters.Parameters
	for _, p := range params {
		if p.GetSecure() {
			secureParams = append(secureParams, p)
		} else {
			standardParams = append(standardParams, p)
		}
	}

	inputSchema, authParams := generateParamManifest(standardParams, urlParams)
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
		BaseMetadata: BaseMetadata{
			Name: name,
		},
		Description:     desc,
		ToolInputSchema: inputSchema,
		Annotations:     toolAnnotations,
	}
	if len(secureParams) > 0 {
		secureInputSchema, _ := generateParamManifest(secureParams, urlParams)
		mcpManifest.SecureInputSchema = &secureInputSchema
	}
	metadata := make(map[string]any)
	if len(authInvoke) > 0 {
		metadata["com.google.cloud/authInvoke"] = authInvoke
	}
	if len(authParams) > 0 {
		metadata["com.google.cloud/authParam"] = authParams
	}
	if uiMetadata != nil {
		metadata["ui"] = uiMetadata
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
func GenerateListToolsResult(pMgr *primitives.PrimitiveManager, g group.Group, urlParams map[string]string, supportsSecureParams bool) (ListToolsResult, error) {
	mcpManifest := make([]Tool, 0, len(g.ToolNames))
	for _, toolName := range g.ToolNames {
		tool, ok := pMgr.GetTool(toolName)
		if !ok {
			return ListToolsResult{}, fmt.Errorf("tool does not exist: %s", toolName)
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

		// Skip a Tool that requires secure params extension if the client doesn't support it.
		if tool.HasSecureParams() && !supportsSecureParams {
			continue
		}
		var uiMeta map[string]any
		if uiMetaOrig := tool.GetToolUIMetadata(); uiMetaOrig != nil {
			if uiMetaOrig.Resource != "" {
				var uri string
				if res, hasRes := pMgr.GetResource(uiMetaOrig.Resource); hasRes {
					uri = res.GetURI()
				} else if tmpl, hasTmpl := pMgr.GetResourceTemplate(uiMetaOrig.Resource); hasTmpl {
					uri = tmpl.GetURITemplate()
				} else {
					return ListToolsResult{}, fmt.Errorf("unable to retrieve UI resource %q for tool %q", uiMetaOrig.Resource, toolName)
				}
				uiMeta = map[string]any{
					"resourceUri": uri,
				}
				if len(uiMetaOrig.Visibility) > 0 {
					vis := make([]string, len(uiMetaOrig.Visibility))
					for i, v := range uiMetaOrig.Visibility {
						vis[i] = string(v)
					}
					uiMeta["visibility"] = vis
				}
			}
		}
		toolManifest := generateToolManifest(toolName, tool.GetDescription(), tool.GetAuthRequired(), params, tool.GetAnnotations(src), urlParams, uiMeta)
		mcpManifest = append(mcpManifest, toolManifest)
	}
	res := ListToolsResult{
		Tools: mcpManifest,
		Result: Result{
			ResultType: resultTypeComplete,
		},
		CacheableResult: CacheableResult{
			TtlMs:      g.GetTTLMs(),
			CacheScope: cacheScope(g.GetCacheScope()),
		},
	}
	return res, nil
}

// generatePromptManifest generates a version-specific Prompt manifest for list/prompts
func generatePromptManifest(name, desc string, args prompts.Arguments) Prompt {
	mcpArgs := make([]PromptArgument, 0, len(args))
	for _, arg := range args {
		promptArg := PromptArgument{
			BaseMetadata: BaseMetadata{Name: arg.GetName()},
			Description:  arg.GetDesc(),
			Required:     parameters.CheckParamRequired(arg.GetRequired(), arg.GetDefault()),
		}
		mcpArgs = append(mcpArgs, promptArg)
	}
	return Prompt{
		BaseMetadata: BaseMetadata{Name: name},
		Description:  desc,
		Arguments:    mcpArgs,
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
	res := ListPromptsResult{
		Prompts: mcpManifest,
		Result: Result{
			ResultType: resultTypeComplete,
		},
		CacheableResult: CacheableResult{
			TtlMs:      g.GetTTLMs(),
			CacheScope: cacheScope(g.GetCacheScope()),
		},
	}
	return res, nil
}

// generateAnnotations converts internal resource annotations to versioned Annotations
func generateAnnotations(internalAnns *resources.ResourceAnnotations) *Annotations {
	if internalAnns == nil || (internalAnns.LastModified == "" && len(internalAnns.Audience) == 0 && internalAnns.Priority == nil) {
		return nil
	}
	annotations := &Annotations{
		LastModified: internalAnns.LastModified,
	}
	for _, aud := range internalAnns.Audience {
		annotations.Audience = append(annotations.Audience, Role(aud))
	}
	if internalAnns.Priority != nil {
		annotations.Priority = internalAnns.Priority
	}
	return annotations
}

// generateResourceManifest generates a version-specific Resource manifest for list/resources
func generateResourceManifest(name, title, desc, uri, mimeType string, size *int64, internalAnns *resources.ResourceAnnotations) Resource {
	annotations := generateAnnotations(internalAnns)
	return Resource{
		BaseMetadata: BaseMetadata{
			Name:  name,
			Title: title,
		},
		Uri:         uri,
		Description: desc,
		MimeType:    mimeType,
		Size:        size,
		Annotations: annotations,
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
		mcpManifest = append(mcpManifest, generateResourceManifest(name, res.GetTitle(), res.GetDescription(), res.GetURI(), res.GetMimeType(), res.GetSize(), res.GetAnnotations()))
	}
	return ListResourcesResult{
		Resources: mcpManifest,
		Result: Result{
			ResultType: resultTypeComplete,
		},
		CacheableResult: CacheableResult{
			TtlMs:      g.GetTTLMs(),
			CacheScope: cacheScope(g.GetCacheScope()),
		},
	}, nil
}

// generateResourceTemplateManifest generates a version-specific ResourceTemplate manifest
func generateResourceTemplateManifest(name, title, desc, uriTemplate, mimeType string, internalAnns *resources.ResourceAnnotations) ResourceTemplate {
	annotations := generateAnnotations(internalAnns)
	return ResourceTemplate{
		BaseMetadata: BaseMetadata{
			Name:  name,
			Title: title,
		},
		UriTemplate: uriTemplate,
		Description: desc,
		MimeType:    mimeType,
		Annotations: annotations,
	}
}

// GenerateListResourceTemplatesResult generates the resources/templates/list result
func GenerateListResourceTemplatesResult(pMgr *primitives.PrimitiveManager, g group.Group) (ListResourceTemplatesResult, error) {
	mcpManifest := make([]ResourceTemplate, 0, len(g.ResourceTemplateNames))
	for _, name := range g.ResourceTemplateNames {
		tmpl, ok := pMgr.GetResourceTemplate(name)
		if !ok {
			return ListResourceTemplatesResult{}, fmt.Errorf("resource template does not exist: %s", name)
		}
		mcpManifest = append(mcpManifest, generateResourceTemplateManifest(name, tmpl.GetTitle(), tmpl.GetDescription(), tmpl.GetURITemplate(), tmpl.GetMimeType(), tmpl.GetAnnotations()))
	}
	return ListResourceTemplatesResult{
		ResourceTemplates: mcpManifest,
		Result: Result{
			ResultType: resultTypeComplete,
		},
		CacheableResult: CacheableResult{
			TtlMs:      g.GetTTLMs(),
			CacheScope: cacheScope(g.GetCacheScope()),
		},
	}, nil
}

// GenerateGetGroupResult generates the groups/get result for a single group's primitives.
func GenerateGetGroupResult(pMgr *primitives.PrimitiveManager, g group.Group, urlParams map[string]string, supportsSecureParams bool) (GetGroupResult, error) {
	listToolsResult, err := GenerateListToolsResult(pMgr, g, urlParams, supportsSecureParams)
	if err != nil {
		return GetGroupResult{}, fmt.Errorf("error generating tools manifest: %w", err)
	}
	listPromptsResult, err := GenerateListPromptsResult(pMgr, g)
	if err != nil {
		return GetGroupResult{}, fmt.Errorf("error generating prompts manifest: %w", err)
	}
	listResourcesResult, err := GenerateListResourcesResult(pMgr, g)
	if err != nil {
		return GetGroupResult{}, fmt.Errorf("error generating resources manifest: %w", err)
	}
	listTemplatesResult, err := GenerateListResourceTemplatesResult(pMgr, g)
	if err != nil {
		return GetGroupResult{}, fmt.Errorf("error generating resource templates manifest: %w", err)
	}

	return GetGroupResult{
		Name:              g.Name,
		Tools:             listToolsResult.Tools,
		Prompts:           listPromptsResult.Prompts,
		Resources:         listResourcesResult.Resources,
		ResourceTemplates: listTemplatesResult.ResourceTemplates,
	}, nil
}
