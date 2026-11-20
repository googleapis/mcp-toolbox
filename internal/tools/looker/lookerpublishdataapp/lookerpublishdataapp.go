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

package lookerpublishdataapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
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

const resourceType string = "looker-publish-data-app"

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
		parameters.NewStringParameter("title", "The display title for the Data App dashboard and extension label (up to 255 characters)."),
		parameters.NewStringParameter("code", "The uncompressed JavaScript ES module source code payload (up to 5 MB). Must be a native ES module (<script type=\"module\">), not CommonJS or an IIFE bundle. To connect to Looker's extension host (avoiding 'Cannot connect to extension'), the module MUST establish the Looker Extension SDK handshake and mount into document.body: either import { DataApp, html } from the Looker Data Apps runtime (https://www.gstatic.com/looker-data-apps/versions/<version>/data-app.js, e.g. 1.3.0), extend DataApp, and end with the custom element registration guard (if (!customElements.get(elementName)) { customElements.define(elementName, AppClass); } export default AppClass;), or call connectExtensionHost() from @looker/extension-sdk and append the root element to document.body. Any relative imports (e.g. '/runtime/data-app.js') must be rewritten to absolute HTTPS URLs."),
		parameters.NewStringParameter("description", "Optional description for the Data App dashboard and extension.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("folder", "Optional target folder ID when creating a new Data App dashboard. Leave blank to use the authenticated user's personal folder. Ignored when dashboard_id is provided.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("dashboard_id", "Optional existing dashboard ID to update in-place. When omitted, creates a new full-bleed Data App dashboard.", parameters.WithStringDefault("")),
		parameters.NewStringParameter("rich_content_json", "Optional JSON string (e.g. '{\"data_app_queries_version\": 1, \"data_app_queries\": [...]}') to attach to the extension dashboard element.", parameters.WithStringRequired(false)),
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			lookercommon.DestructiveAnnotations(cfg.Annotations),
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

func parseOptionalString(paramsMap map[string]any, key string) (*string, error) {
	raw, exists := paramsMap[key]
	if !exists || raw == nil {
		return nil, nil
	}
	val, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("'%s' must be a string, got %T", key, raw)
	}
	return &val, nil
}

func fullBleedAppearance() map[string]any {
	return map[string]any{
		"show_dashboard_header": false,
		"page_side_margins":     0,
		"tile_space_between":    0,
		"tile_shadow":           false,
	}
}

func handleLookerError(err error) util.ToolboxError {
	if strings.Contains(err.Error(), "status=401") {
		return util.NewClientServerError("unauthorized error", http.StatusUnauthorized, err)
	}
	return util.ProcessGeneralError(err)
}

func selectDashboardLayoutID(layouts *[]v4.DashboardLayout) string {
	if layouts == nil || len(*layouts) == 0 {
		return ""
	}
	for _, l := range *layouts {
		if l.Active != nil && *l.Active && l.Id != nil && *l.Id != "" {
			return *l.Id
		}
	}
	if (*layouts)[0].Id != nil {
		return *(*layouts)[0].Id
	}
	return ""
}

func buildFullDashboardURL(hostURL string, dashURL string, dashboardID string) string {
	if dashURL == "" && dashboardID != "" {
		dashURL = "/dashboards/" + dashboardID
	}
	if dashURL == "" {
		return ""
	}
	if hostURL != "" && !strings.HasPrefix(dashURL, "http://") && !strings.HasPrefix(dashURL, "https://") {
		return hostURL + dashURL
	}
	return dashURL
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
	title, ok := paramsMap["title"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("'title' must be a string, got %T", paramsMap["title"]), nil)
	}
	if err := lookercommon.ValidateDataAppExtensionLabel(title); err != nil {
		return nil, util.NewAgentError(strings.ReplaceAll(err.Error(), "'label'", "'title'"), nil)
	}

	code, ok := paramsMap["code"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("'code' must be a string, got %T", paramsMap["code"]), nil)
	}
	if err := lookercommon.ValidateDataAppExtensionCode(code); err != nil {
		return nil, util.NewAgentError(err.Error(), nil)
	}

	descriptionPtr, err := parseOptionalString(paramsMap, "description")
	if err != nil {
		return nil, util.NewAgentError(err.Error(), nil)
	}

	folder := ""
	if folderPtr, err := parseOptionalString(paramsMap, "folder"); err != nil {
		return nil, util.NewAgentError(err.Error(), nil)
	} else if folderPtr != nil {
		folder = strings.TrimSpace(*folderPtr)
	}

	dashboardID := ""
	if dashIDPtr, err := parseOptionalString(paramsMap, "dashboard_id"); err != nil {
		return nil, util.NewAgentError(err.Error(), nil)
	} else if dashIDPtr != nil {
		dashboardID = strings.TrimSpace(*dashIDPtr)
	}

	richContentJSONPtr, err := parseOptionalString(paramsMap, "rich_content_json")
	if err != nil {
		return nil, util.NewAgentError(err.Error(), nil)
	}
	if richContentJSONPtr != nil && strings.TrimSpace(*richContentJSONPtr) != "" {
		if !json.Valid([]byte(*richContentJSONPtr)) {
			return nil, util.NewAgentError("'rich_content_json' must be a valid JSON string", nil)
		}
	}

	logger.DebugContext(ctx, "invoking looker-publish-data-app", "title", title, "dashboard_id", dashboardID, "folder", folder, "code_bytes", len(code))

	sdk, err := source.GetLookerSDK(ctx, string(accessToken))
	if err != nil {
		return nil, util.NewClientServerError("error getting sdk", http.StatusInternalServerError, err)
	}

	opts := source.LookerApiSettings()

	if dashboardID != "" {
		return publishUpdate(ctx, source, sdk, opts, dashboardID, title, code, descriptionPtr, richContentJSONPtr)
	}
	return publishCreate(ctx, source, sdk, opts, title, code, descriptionPtr, folder, richContentJSONPtr)
}

func publishCreate(
	ctx context.Context,
	source compatibleSource,
	sdk *v4.LookerSDK,
	opts *rtl.ApiSettings,
	title string,
	code string,
	descriptionPtr *string,
	folder string,
	richContentJSONPtr *string,
) (any, util.ToolboxError) {
	logger, _ := util.LoggerFromContext(ctx)

	if folder == "" {
		mresp, err := sdk.Me("id,personal_folder_id", opts)
		if err != nil {
			return nil, handleLookerError(err)
		}
		if mresp.PersonalFolderId == nil || *mresp.PersonalFolderId == "" {
			return nil, util.NewAgentError("user does not have a personal folder. A folder must be specified", nil)
		}
		folder = *mresp.PersonalFolderId
	}

	dashs, err := sdk.FolderDashboards(folder, "title", opts)
	if err != nil {
		return nil, handleLookerError(err)
	}
	dashTitles := make([]string, 0, len(dashs))
	for _, dash := range dashs {
		if dash.Title != nil {
			dashTitles = append(dashTitles, *dash.Title)
		}
	}
	if slices.Contains(dashTitles, title) {
		lt, _ := json.Marshal(dashTitles)
		return nil, util.NewAgentError(fmt.Sprintf("title %s already used in folder. Currently used titles are %v. Make the call again with a unique title or pass dashboard_id to update an existing Data App", title, string(lt)), nil)
	}

	// Create the Data App Extension BEFORE CreateDashboard so invalid payloads never leave orphaned dashboards.
	extName := lookercommon.GenerateDataAppExtensionName(title)
	extReq := lookercommon.WriteDataAppExtension{
		Name:        &extName,
		Label:       &title,
		Description: descriptionPtr,
		Code:        &code,
	}
	extResp, err := lookercommon.CreateDataAppExtension(sdk, extReq, opts)
	if err != nil {
		return nil, handleLookerError(err)
	}

	desc := ""
	if descriptionPtr != nil {
		desc = *descriptionPtr
	}
	wd := v4.WriteDashboard{
		Title:       &title,
		Description: &desc,
		FolderId:    &folder,
	}
	dashResp, err := sdk.CreateDashboard(wd, opts)
	if err != nil {
		return nil, handleLookerError(err)
	}

	dashboardID := ""
	if dashResp.Id != nil {
		dashboardID = *dashResp.Id
	}
	dashURL := ""
	if dashResp.Url != nil {
		dashURL = *dashResp.Url
	}
	layoutID := selectDashboardLayoutID(dashResp.DashboardLayouts)

	// Set full-bleed container appearance.
	patchBody := map[string]any{
		"appearance": fullBleedAppearance(),
	}
	dashPath := fmt.Sprintf("/dashboards/%s", url.PathEscape(dashboardID))
	var patchResp map[string]any
	if err := sdk.AuthSession.Do(&patchResp, "PATCH", "/4.0", dashPath, nil, patchBody, opts); err != nil {
		return nil, handleLookerError(err)
	}

	// Mount the extension dashboard element.
	elemType := "extension"
	hideTitle := true
	extID := extResp.ExtensionId
	wde := v4.WriteDashboardElement{
		DashboardId:     &dashboardID,
		Type:            &elemType,
		Title:           &title,
		TitleHidden:     &hideTitle,
		ExtensionId:     &extID,
		RichContentJson: richContentJSONPtr,
	}
	elemFields := "id"
	elemResp, err := sdk.CreateDashboardElement(v4.RequestCreateDashboardElement{
		Body:   wde,
		Fields: &elemFields,
	}, opts)
	if err != nil {
		return nil, handleLookerError(err)
	}
	elementID := ""
	if elemResp.Id != nil {
		elementID = *elemResp.Id
	}

	// Fallback layout lookup if CreateDashboard response did not include dashboard_layouts.
	if layoutID == "" && dashboardID != "" {
		fetchedDash, err := sdk.Dashboard(dashboardID, "id,dashboard_layouts(id,active,type)", opts)
		if err != nil {
			return nil, handleLookerError(err)
		}
		layoutID = selectDashboardLayoutID(fetchedDash.DashboardLayouts)
	}

	// Switch the active layout to type "extension" after the element is mounted.
	if layoutID != "" {
		layoutType := "extension"
		active := true
		if _, err := sdk.UpdateDashboardLayout(layoutID, v4.WriteDashboardLayout{
			Type:   &layoutType,
			Active: &active,
		}, "", opts); err != nil {
			return nil, handleLookerError(err)
		}
	}

	hostURL, err := source.GetHostURL(ctx, sdk)
	if err != nil && logger != nil {
		logger.WarnContext(ctx, "failed to dynamically resolve public host URL, utilizing fallback", "error", err)
	}

	result := map[string]any{
		"dashboard_id":         dashboardID,
		"url":                  buildFullDashboardURL(hostURL, dashURL, dashboardID),
		"extension_id":         extID,
		"dashboard_element_id": elementID,
		"dashboard_layout_id":  layoutID,
	}
	if logger != nil {
		logger.DebugContext(ctx, "published new data app", "dashboard_id", dashboardID, "extension_id", extID, "dashboard_element_id", elementID, "dashboard_layout_id", layoutID)
	}
	return result, nil
}

func publishUpdate(
	ctx context.Context,
	source compatibleSource,
	sdk *v4.LookerSDK,
	opts *rtl.ApiSettings,
	dashboardID string,
	title string,
	code string,
	descriptionPtr *string,
	richContentJSONPtr *string,
) (any, util.ToolboxError) {
	logger, _ := util.LoggerFromContext(ctx)

	dashFields := "id,url,title,dashboard_elements(id,type,extension_id),dashboard_layouts(id,active,type)"
	dashResp, err := sdk.Dashboard(dashboardID, dashFields, opts)
	if err != nil {
		return nil, handleLookerError(err)
	}

	dashURL := ""
	if dashResp.Url != nil {
		dashURL = *dashResp.Url
	}
	layoutID := selectDashboardLayoutID(dashResp.DashboardLayouts)

	var existingElemID string
	var existingExtID string
	if dashResp.DashboardElements != nil {
		for _, elem := range *dashResp.DashboardElements {
			isExtType := elem.Type != nil && *elem.Type == "extension"
			hasExtID := elem.ExtensionId != nil && strings.TrimSpace(*elem.ExtensionId) != ""
			if isExtType || hasExtID {
				if elem.Id != nil {
					existingElemID = *elem.Id
				}
				if elem.ExtensionId != nil {
					existingExtID = strings.TrimSpace(*elem.ExtensionId)
				}
				break
			}
		}
	}

	// Update existing db::<id> extension in-place, falling back to creating a new extension
	// if none is attached or if the referenced extension was deleted (404).
	var extID string
	if strings.HasPrefix(existingExtID, "db::") {
		updateReq := lookercommon.WriteDataAppExtension{
			Label:       &title,
			Description: descriptionPtr,
			Code:        &code,
		}
		updatedExt, updateErr := lookercommon.UpdateDataAppExtension(sdk, existingExtID, dashboardID, updateReq, opts)
		if updateErr == nil {
			extID = updatedExt.ExtensionId
			if extID == "" {
				extID = existingExtID
			}
		} else if strings.Contains(updateErr.Error(), "status=404") {
			if logger != nil {
				logger.WarnContext(ctx, "existing data app extension not found; creating replacement extension", "extension_id", existingExtID, "dashboard_id", dashboardID)
			}
		} else {
			return nil, handleLookerError(updateErr)
		}
	}

	if extID == "" {
		extName := lookercommon.GenerateDataAppExtensionName(title)
		createReq := lookercommon.WriteDataAppExtension{
			Name:        &extName,
			Label:       &title,
			Description: descriptionPtr,
			Code:        &code,
		}
		createdExt, createErr := lookercommon.CreateDataAppExtension(sdk, createReq, opts)
		if createErr != nil {
			return nil, handleLookerError(createErr)
		}
		extID = createdExt.ExtensionId
	}

	// Converge dashboard title, optional description, and full-bleed appearance.
	patchBody := map[string]any{
		"title":      title,
		"appearance": fullBleedAppearance(),
	}
	if descriptionPtr != nil {
		patchBody["description"] = *descriptionPtr
	}
	dashPath := fmt.Sprintf("/dashboards/%s", url.PathEscape(dashboardID))
	var patchResp map[string]any
	if err := sdk.AuthSession.Do(&patchResp, "PATCH", "/4.0", dashPath, nil, patchBody, opts); err != nil {
		return nil, handleLookerError(err)
	}

	// Converge extension dashboard element (update in-place or create if absent).
	elemType := "extension"
	hideTitle := true
	wde := v4.WriteDashboardElement{
		DashboardId:     &dashboardID,
		Type:            &elemType,
		Title:           &title,
		TitleHidden:     &hideTitle,
		ExtensionId:     &extID,
		RichContentJson: richContentJSONPtr,
	}
	var elementID string
	if existingElemID != "" {
		elemResp, err := sdk.UpdateDashboardElement(existingElemID, wde, "id", opts)
		if err != nil {
			return nil, handleLookerError(err)
		}
		elementID = existingElemID
		if elemResp.Id != nil && *elemResp.Id != "" {
			elementID = *elemResp.Id
		}
	} else {
		elemFields := "id"
		elemResp, err := sdk.CreateDashboardElement(v4.RequestCreateDashboardElement{
			Body:   wde,
			Fields: &elemFields,
		}, opts)
		if err != nil {
			return nil, handleLookerError(err)
		}
		if elemResp.Id != nil {
			elementID = *elemResp.Id
		}
	}

	// Converge active layout to type "extension".
	if layoutID != "" {
		layoutType := "extension"
		active := true
		if _, err := sdk.UpdateDashboardLayout(layoutID, v4.WriteDashboardLayout{
			Type:   &layoutType,
			Active: &active,
		}, "", opts); err != nil {
			return nil, handleLookerError(err)
		}
	}

	hostURL, err := source.GetHostURL(ctx, sdk)
	if err != nil && logger != nil {
		logger.WarnContext(ctx, "failed to dynamically resolve public host URL, utilizing fallback", "error", err)
	}

	result := map[string]any{
		"dashboard_id":         dashboardID,
		"url":                  buildFullDashboardURL(hostURL, dashURL, dashboardID),
		"extension_id":         extID,
		"dashboard_element_id": elementID,
		"dashboard_layout_id":  layoutID,
	}
	if logger != nil {
		logger.DebugContext(ctx, "updated published data app", "dashboard_id", dashboardID, "extension_id", extID, "dashboard_element_id", elementID, "dashboard_layout_id", layoutID)
	}
	return result, nil
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
