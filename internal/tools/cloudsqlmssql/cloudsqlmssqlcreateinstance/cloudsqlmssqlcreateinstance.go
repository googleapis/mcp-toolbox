// Copyright 2025 Google LLC
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

package cloudsqlmssqlcreateinstance

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/api/sqladmin/v1"
)

const resourceType string = "cloud-sql-mssql-create-instance"

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
	GetDefaultProject() string
	UseClientAuthorization() bool
	CreateInstance(context.Context, string, string, string, string, sqladmin.Settings, string) (any, error)
}

// Config defines the configuration for the create-instances tool.
type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	Source           string `yaml:"source" validate:"required"`
}

// validate interface
var _ tools.ToolConfig = Config{}

// ToolConfigType returns the type of the tool.
func (cfg Config) ToolConfigType() string {
	return resourceType
}

// Initialize initializes the tool from the configuration.
func (cfg Config) Initialize(context.Context) (tools.Tool, error) {

	if cfg.Description == "" {
		cfg.Description = "Creates a SQL Server instance using `Production` and `Development` presets. For the `Development` template, it chooses a 2 vCPU, 8 GiB RAM (`db-custom-2-8192`) configuration with Non-HA/zonal availability. For the `Production` template, it chooses a 4 vCPU, 26 GiB RAM (`db-custom-4-26624`) configuration with HA/regional availability. The Enterprise edition is used in both cases. The default database version is `SQLSERVER_2022_STANDARD`. The agent should ask the user if they want to use a different version."
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

// Tool represents the create-instances tool.
type Tool struct {
	tools.BaseTool[Config]
}

// Invoke executes the tool's logic.
func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramsMap := params.AsMap()

	project, ok := paramsMap["project"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'project' parameter: %s", paramsMap["project"]), nil)
	}
	name, ok := paramsMap["name"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'name' parameter: %s", paramsMap["name"]), nil)
	}
	dbVersion, ok := paramsMap["databaseVersion"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'databaseVersion' parameter: %s", paramsMap["databaseVersion"]), nil)
	}
	rootPassword, ok := paramsMap["rootPassword"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'rootPassword' parameter: %s", paramsMap["rootPassword"]), nil)
	}
	editionPreset, ok := paramsMap["editionPreset"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("error casting 'editionPreset' parameter: %s", paramsMap["editionPreset"]), nil)
	}
	settings := sqladmin.Settings{}
	switch strings.ToLower(editionPreset) {
	case "production":
		settings.AvailabilityType = "REGIONAL"
		settings.Edition = "ENTERPRISE"
		settings.Tier = "db-custom-4-26624"
		settings.DataDiskSizeGb = 250
		settings.DataDiskType = "PD_SSD"
	case "development":
		settings.AvailabilityType = "ZONAL"
		settings.Edition = "ENTERPRISE"
		settings.Tier = "db-custom-2-8192"
		settings.DataDiskSizeGb = 100
		settings.DataDiskType = "PD_SSD"
	default:
		return nil, util.NewAgentError(fmt.Sprintf("invalid 'editionPreset': %q. Must be either 'Production' or 'Development'", editionPreset), nil)
	}
	resp, err := source.CreateInstance(ctx, project, name, dbVersion, rootPassword, settings, string(accessToken))
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}

// Authorized always returns true: this admin tool intentionally bypasses
// per-request auth-service checks (server-side credentials handle access).
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

// buildParams builds the tool's parameters. A non-empty project means the source has a
// configured default project, which is baked into the project param; otherwise the plain form is used.
func buildParams(project string) parameters.Parameters {
	projectParam := parameters.NewStringParameter("project", "The project ID")
	if project != "" {
		projectParam = parameters.NewStringParameter("project", "The GCP project ID. This is pre-configured; do not ask for it unless the user explicitly provides a different one.", parameters.WithStringDefault(project))
	}
	return parameters.Parameters{
		projectParam,
		parameters.NewStringParameter("name", "The name of the instance"),
		parameters.NewStringParameter("databaseVersion", "The database version for SQL Server. If not specified, defaults to SQLSERVER_2022_STANDARD.", parameters.WithStringDefault("SQLSERVER_2022_STANDARD")),
		parameters.NewStringParameter("rootPassword", "The root password for the instance"),
		parameters.NewStringParameter("editionPreset", "The edition of the instance. Can be `Production` or `Development`. This determines the default machine type and availability. Defaults to `Development`.", parameters.WithStringDefault("Development")),
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
