// Copyright 2025 Google LLC
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
package lookermakedashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"

	"github.com/googleapis/mcp-toolbox/internal/tools/looker/lookercommon"
	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

const resourceType string = "looker-make-dashboard"

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
	GetHostURL(context.Context, *v4.LookerSDK) (string, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
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

	allParameters := parameters.Parameters{}

	titleParameter := parameters.NewStringParameter("title", "The title of the Dashboard")
	allParameters = append(allParameters, titleParameter)
	descParameter := parameters.NewStringParameter("description", "The description of the Dashboard", parameters.WithStringDefault(""))
	allParameters = append(allParameters, descParameter)
	folderParameter := parameters.NewStringParameter("folder", "The folder id where the Dashboard will be created. Leave blank to use the user's personal folder", parameters.WithStringDefault(""))
	allParameters = append(allParameters, folderParameter)

	// finish tool setup
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
	logger.DebugContext(ctx, "params = ", params)

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}

	paramsMap := params.AsMap()
	title := paramsMap["title"].(string)
	description := paramsMap["description"].(string)
	folder := paramsMap["folder"].(string)

	mrespFields := "id,personal_folder_id"
	mresp, err := sdk.Me(mrespFields, source.LookerApiSettings())
	if err != nil {
		if strings.Contains(err.Error(), "status=401") {
			return nil, util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
		}
		return nil, util.ProcessGeneralError(err)
	}

	if folder == "" {
		if mresp.PersonalFolderId == nil || *mresp.PersonalFolderId == "" {
			return nil, util.NewAgentError("user does not have a personal folder. A folder must be specified", nil)
		}
		folder = *mresp.PersonalFolderId
	}

	dashs, err := sdk.FolderDashboards(folder, "title", source.LookerApiSettings())
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}

	dashTitles := []string{}
	for _, dash := range dashs {
		dashTitles = append(dashTitles, *dash.Title)
	}
	if slices.Contains(dashTitles, title) {
		lt, _ := json.Marshal(dashTitles)
		return nil, util.NewAgentError(fmt.Sprintf("title %s already used in folder. Currently used titles are %v. Make the call again with a unique title", title, string(lt)), nil)
	}

	wd := v4.WriteDashboard{
		Title:       &title,
		Description: &description,
		FolderId:    &folder,
	}
	resp, err := sdk.CreateDashboard(wd, source.LookerApiSettings())
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	logger.DebugContext(ctx, "resp = %v", resp)

	hostURL, err := source.GetHostURL(ctx, sdk)
	if err != nil {
		logger.WarnContext(ctx, "failed to dynamically resolve public host URL, utilizing fallback", "error", err)
	}

	data := make(map[string]any)
	if resp.Id != nil {
		data["id"] = *resp.Id
	}
	if resp.Url != nil {
		if hostURL != "" {
			data["url"] = hostURL + *resp.Url
		} else {
			data["url"] = *resp.Url
		}
	}
	logger.DebugContext(ctx, "data = %v", data)

	return data, nil
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
