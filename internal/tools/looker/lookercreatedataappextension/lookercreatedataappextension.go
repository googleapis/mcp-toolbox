// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lookercreatedataappextension

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

const resourceType string = "looker-create-data-app-extension"

func init() {
	if !tools.Register(resourceType, newConfig) {
		panic(fmt.Sprintf("tool type %q already registered", resourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (tools.ToolConfig, error) {
	actual := Config{ConfigBase: tools.ConfigBase{Name: name}}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type compatibleSource interface {
	UseClientAuthorization() bool
	GetAuthTokenHeaderName() string
	LookerApiSettings() *rtl.ApiSettings
	GetLookerSDK(context.Context, string) (*v4.LookerSDK, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

// validate interface
var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	allParameters := parameters.Parameters{
		parameters.NewStringParameter("label", "The user-friendly display label for the Data App Extension (up to 255 characters)."),
		parameters.NewStringParameter("code", "The uncompressed JavaScript ES module source code payload (up to 5 MB). Must be a native ES module (<script type=\"module\">), not CommonJS or an IIFE bundle. Relative imports (e.g. '/runtime/data-app.js') must be rewritten to absolute CDN URLs (e.g. 'https://www.gstatic.com/looker-data-apps/versions/1.2.0/data-app.js' and 'https://www.gstatic.com/looker-data-apps/vendor/lit/3.3.1/lit.mjs'). Must end with the custom element registration guard: if (!customElements.get(elementName)) { customElements.define(elementName, AppClass); } export default AppClass;"),
		parameters.NewStringParameter("description", "Optional description of the Data App Extension.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("name", "Optional unique system name matching [a-zA-Z0-9-_]+. Leave empty to auto-generate a collision-free name.", parameters.WithStringDefault("")),
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			lookercommon.WriteAnnotations(cfg.Annotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
		),
	}, nil
}

// validate interface
var _ tools.Tool = Tool{}

type Tool struct {
	tools.BaseTool[Config]
}

func (t Tool) GetSourceName() string {
	return t.Cfg.Source
}

func (t Tool) ToConfig() tools.ToolConfig {
	return t.Cfg
}

func (t Tool) ValidateSource(source sources.Source) error {
	_, ok := source.(compatibleSource)
	if !ok {
		return fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return nil
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError("unable to get logger from ctx", http.StatusInternalServerError, err)
	}

	paramsMap := params.AsMap()
	label, ok := paramsMap["label"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("'label' must be a string, got %T", paramsMap["label"]), nil)
	}
	if err := lookercommon.ValidateDataAppExtensionLabel(label); err != nil {
		return nil, util.NewAgentError(err.Error(), nil)
	}

	code, ok := paramsMap["code"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("'code' must be a string, got %T", paramsMap["code"]), nil)
	}
	if err := lookercommon.ValidateDataAppExtensionCode(code); err != nil {
		return nil, util.NewAgentError(err.Error(), nil)
	}

	var descriptionPtr *string
	if rawDesc, exists := paramsMap["description"]; exists && rawDesc != nil {
		desc, ok := rawDesc.(string)
		if !ok {
			return nil, util.NewAgentError(fmt.Sprintf("'description' must be a string, got %T", rawDesc), nil)
		}
		descriptionPtr = &desc
	}

	name, ok := paramsMap["name"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("'name' must be a string, got %T", paramsMap["name"]), nil)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = lookercommon.GenerateDataAppExtensionName(label)
	} else if err := lookercommon.ValidateDataAppExtensionName(name); err != nil {
		return nil, util.NewAgentError(err.Error(), nil)
	}

	logger.DebugContext(ctx, "invoking looker-create-data-app-extension", "label", label, "name", name, "code_bytes", len(code))

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}

	req := lookercommon.WriteDataAppExtension{
		Name:        &name,
		Label:       &label,
		Description: descriptionPtr,
		Code:        &code,
	}

	resp, err := lookercommon.CreateDataAppExtension(sdk, req, source.LookerApiSettings())
	if err != nil {
		if strings.Contains(err.Error(), "status=401") {
			return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
		}
		return nil, util.ProcessGeneralError(err)
	}
	logger.DebugContext(ctx, "created data app extension", "id", resp.Id, "extension_id", resp.ExtensionId)

	return resp, nil
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}

func (t Tool) GetAuthTokenHeaderName(source sources.Source) (string, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return "", fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.GetAuthTokenHeaderName(), nil
}
