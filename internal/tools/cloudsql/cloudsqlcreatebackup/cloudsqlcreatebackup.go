// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloudsqlcreatebackup

import (
	"context"
	"fmt"
	"net/http"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/api/sqladmin/v1"
)

const resourceType string = "cloud-sql-create-backup"

var _ tools.ToolConfig = Config{}

type compatibleSource interface {
	GetDefaultProject() string
	GetService(context.Context, string) (*sqladmin.Service, error)
	UseClientAuthorization() bool
	InsertBackupRun(ctx context.Context, project, instance, location, backupDescription, accessToken string) (any, error)
}

// Config defines the configuration for the create-backup tool.
type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

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

// ToolConfigType returns the type of the tool.
func (cfg Config) ToolConfigType() string {
	return resourceType
}

// Initialize initializes the tool from the configuration.
func (cfg Config) Initialize(context.Context) (tools.Tool, error) {

	if cfg.Description == "" {
		cfg.Description = "Creates a backup on a Cloud SQL instance."
	}
	params := buildParams("")
	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: params.Manifest(), AuthRequired: cfg.AuthRequired},
			params,
		),
	}, nil
}

// Tool represents the create-backup tool.
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
	paramsMap := params.AsMap()

	project, ok := paramsMap["project"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'project' parameter: %v", paramsMap["project"]), nil)
	}
	instance, ok := paramsMap["instance"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'instance' parameter: %v", paramsMap["instance"]), nil)
	}

	location, _ := paramsMap["location"].(string)
	description, _ := paramsMap["backup_description"].(string)

	resp, err := source.InsertBackupRun(ctx, project, instance, location, description, string(accessToken))
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}

// Authorized checks if the tool is authorized.
func (t Tool) Authorized(verifiedAuthServices []string) bool {
	return true
}

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}

// buildParams builds the tool's parameters. A non-empty project means the source has a
// configured default project, which is baked into the project param; otherwise the plain form is used.
func buildParams(project string) parameters.Parameters {
	projectParam := parameters.NewStringParameter("project", "The project ID")
	if project != "" {
		projectParam = parameters.NewStringParameter("project", "The GCP project ID. This is pre-configured; do not ask for it unless the user explicitly provides a different one.", parameters.WithStringDefault(project))
	}
	return parameters.Parameters{
		projectParam,
		parameters.NewStringParameter("instance", "Cloud SQL instance ID. This does not include the project ID."),
		parameters.NewStringParameter("location", "Location of the backup run.", parameters.WithStringRequired(false)),
		parameters.NewStringParameter("backup_description", "The description of this backup run.", parameters.WithStringRequired(false)),
	}
}

// resolveParams builds the tool's parameters using the source's configured default GCP project.
func (t Tool) resolveParams(source sources.Source) (parameters.Parameters, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return nil, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return buildParams(s.GetDefaultProject()), nil
}

// GetParameters returns the tool's parameters, resolved against the source.
func (t Tool) GetParameters(source sources.Source) (parameters.Parameters, error) {
	return t.resolveParams(source)
}

// Manifest returns the tool's manifest, resolved against the source.
func (t Tool) Manifest(source sources.Source) (tools.Manifest, error) {
	allParameters, err := t.resolveParams(source)
	if err != nil {
		return tools.Manifest{}, err
	}
	return tools.Manifest{Description: t.Cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: t.Cfg.AuthRequired}, nil
}
