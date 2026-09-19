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
package http

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"text/template"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const resourceType string = "http"

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
	HttpDefaultHeaders() map[string]string
	HttpBaseURL() string
	HttpQueryParams() map[string]string
	RunRequest(context.Context, *http.Request) (any, error)
}

type Config struct {
	tools.ConfigBase      `yaml:",inline"`
	Type                  string                 `yaml:"type" validate:"required"`
	Source                string                 `yaml:"source" validate:"required"`
	Path                  string                 `yaml:"path" validate:"required"`
	Method                tools.HTTPMethod       `yaml:"method" validate:"required"`
	Headers               map[string]string      `yaml:"headers"`
	RequestBody           string                 `yaml:"requestBody"`
	PathParams            parameters.Parameters  `yaml:"pathParams"`
	QueryParams           parameters.Parameters  `yaml:"queryParams"`
	BodyParams            parameters.Parameters  `yaml:"bodyParams"`
	HeaderParams          parameters.Parameters  `yaml:"headerParams"`
	SendGoogleAccessToken bool                   `yaml:"sendGoogleAccessToken"`
	Annotations           *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

// validate interface
var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(ctx context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	var googleAccessTokenProvider *adcTokenProvider
	if cfg.SendGoogleAccessToken {
		googleAccessTokenProvider = &adcTokenProvider{}
	}

	// Create a slice for all parameters
	allParameters := slices.Concat(cfg.PathParams, cfg.QueryParams, cfg.BodyParams, cfg.HeaderParams)

	// Verify no duplicate parameter names
	err := parameters.CheckDuplicateParameters(allParameters)
	if err != nil {
		return nil, err
	}

	// Create Toolbox manifest
	paramManifest := allParameters.Manifest()

	if paramManifest == nil {
		paramManifest = make([]parameters.ParameterManifest, 0)
	}

	// finish tool setup
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: paramManifest, AuthRequired: cfg.AuthRequired},
			allParameters,
		),
		googleAccessTokenProvider: googleAccessTokenProvider,
	}, nil
}

// validate interface
var _ tools.Tool = Tool{}

type Tool struct {
	tools.BaseTool[Config]
	googleAccessTokenProvider *adcTokenProvider
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

// Helper function to generate the HTTP request body upon Tool invocation.
func getRequestBody(bodyParams parameters.Parameters, requestBodyPayload string, paramsMap map[string]any) (string, error) {
	bodyParamValues, err := parameters.GetParams(bodyParams, paramsMap)
	if err != nil {
		return "", err
	}
	bodyParamsMap := bodyParamValues.AsMap()

	requestBodyStr, err := parameters.PopulateTemplateWithJSON("HTTPToolRequestBody", requestBodyPayload, bodyParamsMap)
	if err != nil {
		return "", err
	}
	return requestBodyStr, nil
}

// Helper function to generate the HTTP request URL upon Tool invocation.
func getURL(baseURL, path string, pathParams, queryParams parameters.Parameters, defaultQueryParams map[string]string, paramsMap map[string]any) (string, error) {
	// use Go template to replace path params
	pathParamValues, err := parameters.GetParams(pathParams, paramsMap)
	if err != nil {
		return "", err
	}
	pathParamsMap := pathParamValues.AsMap()

	funcMap := template.FuncMap{
		"pathEscape": func(v any) string {
			if s, ok := v.(string); ok {
				return url.PathEscape(s)
			}
			if v == nil {
				return ""
			}
			return url.PathEscape(fmt.Sprintf("%v", v))
		},
		"queryEscape": func(v any) string {
			if s, ok := v.(string); ok {
				return url.QueryEscape(s)
			}
			if v == nil {
				return ""
			}
			return url.QueryEscape(fmt.Sprintf("%v", v))
		},
	}

	templ, err := template.New("url").Funcs(funcMap).Parse(path)
	if err != nil {
		return "", fmt.Errorf("error parsing URL: %s", err)
	}
	var templatedPath bytes.Buffer
	err = templ.Execute(&templatedPath, pathParamsMap)
	if err != nil {
		return "", fmt.Errorf("error replacing pathParams: %s", err)
	}

	baseParsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("error parsing base URL: %s", err)
	}
	if baseParsedURL.Scheme == "" || baseParsedURL.Host == "" {
		return "", fmt.Errorf("base URL must include scheme and host")
	}

	relativePath := templatedPath.String()
	relParsedURL, err := url.Parse(relativePath)
	if err != nil {
		return "", fmt.Errorf("error parsing URL path: %s", err)
	}
	if relParsedURL.Scheme != "" || relParsedURL.Host != "" || relParsedURL.User != nil {
		return "", fmt.Errorf("path must be relative and cannot override base host")
	}

	// Reject dot segments before resolution
	for _, segment := range strings.Split(relParsedURL.Path, "/") {
		if segment == ".." {
			return "", fmt.Errorf("path cannot contain dot segments (..)")
		}
	}

	// Create URL based on BaseURL and Path
	// Attach query parameters
	parsedURL := baseParsedURL.ResolveReference(relParsedURL)

	// Verify final path stays within base path scope
	basePath := baseParsedURL.Path
	finalPath := parsedURL.Path
	if basePath != "/" {
		requiredPrefix := strings.TrimSuffix(basePath, "/") + "/"
		if finalPath != basePath && !strings.HasPrefix(finalPath, requiredPrefix) {
			return "", fmt.Errorf("resolved path %q escapes base path %q", finalPath, basePath)
		}
	}

	// Get existing query parameters from the URL
	queryParameters := parsedURL.Query()
	for key, value := range defaultQueryParams {
		queryParameters.Add(key, value)
	}
	parsedURL.RawQuery = queryParameters.Encode()

	// Set dynamic query parameters
	query := parsedURL.Query()
	for _, p := range queryParams {
		v, ok := paramsMap[p.GetName()]
		if !ok || v == nil {
			if !p.GetRequired() {
				// If the param is not required AND
				// Not provodid OR provided with a nil value
				// Omitted from the URL
				continue
			}
			v = ""
		}
		query.Add(p.GetName(), fmt.Sprintf("%v", v))
	}
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

// Helper function to generate the HTTP headers upon Tool invocation.
func getHeaders(headerParams parameters.Parameters, defaultHeaders map[string]string, paramsMap map[string]any) (map[string]string, error) {
	// Populate header params
	allHeaders := make(map[string]string)
	maps.Copy(allHeaders, defaultHeaders)
	for _, p := range headerParams {
		headerValue, ok := paramsMap[p.GetName()]
		if ok {
			if strValue, ok := headerValue.(string); ok {
				allHeaders[p.GetName()] = strValue
			} else {
				return nil, fmt.Errorf("header param %s got value of type %t, not string", p.GetName(), headerValue)
			}
		}
	}
	return allHeaders, nil
}

type adcTokenProvider struct {
	mu          sync.Mutex
	tokenSource oauth2.TokenSource
}

func (p *adcTokenProvider) getTokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	if p == nil {
		return nil, fmt.Errorf("google ADC token provider is not initialized")
	}
	if ctx == nil {
		return nil, fmt.Errorf("google ADC token provider invocation context is not initialized")
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tokenSource == nil {
		// ADC token sources may retain this context for later token refreshes.
		// Keep invocation values, such as a custom HTTP client, while ensuring
		// completion of the first invocation does not cancel future refreshes.
		credentials, err := google.FindDefaultCredentials(context.WithoutCancel(ctx), sources.CloudPlatformScope)
		if err != nil {
			return nil, fmt.Errorf("unable to initialize Google ADC: %w", err)
		}
		if credentials.TokenSource == nil {
			return nil, fmt.Errorf("google ADC did not provide a token source")
		}
		p.tokenSource = credentials.TokenSource
	}
	return p.tokenSource, nil
}

func (p *adcTokenProvider) Token(ctx context.Context) (*oauth2.Token, error) {
	tokenSource, err := p.getTokenSource(ctx)
	if err != nil {
		return nil, err
	}
	token, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("unable to get Google access token from ADC: %w", err)
	}
	return token, nil
}

func setGoogleAccessToken(
	ctx context.Context,
	req *http.Request,
	tokenProvider *adcTokenProvider,
) error {
	token, err := tokenProvider.Token(ctx)
	if err != nil {
		return err
	}
	if !token.Valid() {
		return fmt.Errorf("google ADC returned an invalid or expired access token")
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	return nil
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	// Combine Source and Tool headers.
	// In case of conflict, Tool header overrides Source header
	combinedHeaders := make(map[string]string)
	maps.Copy(combinedHeaders, source.HttpDefaultHeaders())
	maps.Copy(combinedHeaders, t.Cfg.Headers)

	paramsMap := params.AsMap()

	// Calculate request body
	requestBody, err := getRequestBody(t.Cfg.BodyParams, t.Cfg.RequestBody, paramsMap)
	if err != nil {
		return nil, util.NewAgentError("error populating request body", err)
	}

	// Calculate URL
	urlString, err := getURL(source.HttpBaseURL(), t.Cfg.Path, t.Cfg.PathParams, t.Cfg.QueryParams, source.HttpQueryParams(), paramsMap)
	if err != nil {
		return nil, util.NewAgentError("error populating path parameters", err)
	}

	req, err := http.NewRequestWithContext(ctx, string(t.Cfg.Method), urlString, strings.NewReader(requestBody))
	if err != nil {
		return nil, util.NewClientServerError("error creating http request", http.StatusInternalServerError, err)
	}

	// Calculate request headers
	allHeaders, err := getHeaders(t.Cfg.HeaderParams, combinedHeaders, paramsMap)
	if err != nil {
		return nil, util.NewAgentError("error populating request headers", err)
	}
	// Set request headers
	for k, v := range allHeaders {
		req.Header.Set(k, v)
	}
	if t.Cfg.SendGoogleAccessToken {
		if err := setGoogleAccessToken(ctx, req, t.googleAccessTokenProvider); err != nil {
			return nil, util.NewClientServerError("error authenticating HTTP request with Google ADC", http.StatusInternalServerError, err)
		}
	}

	resp, err := source.RunRequest(ctx, req)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	return resp, nil
}
